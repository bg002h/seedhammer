//go:build js

package main

import (
	"errors"
	"image"
	"image/draw"
	"io"
	"syscall/js"
	"time"

	"seedhammer.com/engrave"
	"seedhammer.com/gui"
	"seedhammer.com/internal/sh2"
)

// platform runs the real firmware GUI against a browser canvas.
//
// It implements gui.Platform, which is the whole trick: the emulator adds no
// screens and reimplements no flow. Everything above this file is the shipped
// firmware, compiled for GOOS=js instead of a Cortex-M33, so a screen that
// looks right here looks right on the machine.
type platform struct {
	fb    *image.RGBA
	dirty image.Rectangle
	// pending is set by Dirty and cleared by the NextChunk that hands the
	// buffer out; drew records that a frame actually produced pixels, so an
	// empty dirty rect does not blit a stale canvas.
	pending bool
	drew    bool

	events  chan gui.Event
	wakeups chan struct{}

	// The JS side. buf is a Uint8ClampedArray the same length as fb.Pix; Go
	// cannot hand a []byte to putImageData directly, so each frame is copied
	// through it.
	ctx2d js.Value
	img   js.Value
	buf   js.Value
}

func newPlatform() *platform {
	size := image.Pt(sh2.DisplayWidth, sh2.DisplayHeight)
	p := &platform{
		fb: image.NewRGBA(image.Rectangle{Max: size}),
		// Buffered so a tap that lands while the GUI is laying out a frame is
		// queued rather than dropped -- the browser delivers events on the JS
		// side, which cannot block waiting for Go to be ready for them.
		events:  make(chan gui.Event, 16),
		wakeups: make(chan struct{}, 1),
	}

	doc := js.Global().Get("document")
	canvas := doc.Call("getElementById", "screen")
	canvas.Set("width", size.X)
	canvas.Set("height", size.Y)
	p.ctx2d = canvas.Call("getContext", "2d")
	p.img = p.ctx2d.Call("createImageData", size.X, size.Y)
	p.buf = js.Global().Get("Uint8ClampedArray").New(len(p.fb.Pix))

	p.listen(canvas)
	return p
}

// listen wires the browser's pointer events to the firmware's. The SeedHammer
// II is a touch device with no buttons, so this is the ONLY input path -- the
// same one the machine has.
func (p *platform) listen(canvas js.Value) {
	send := func(pressed bool) js.Func {
		return js.FuncOf(func(_ js.Value, args []js.Value) any {
			e := args[0]
			e.Call("preventDefault")
			rect := canvas.Call("getBoundingClientRect")
			// The canvas is scaled by CSS; map back to device pixels or every
			// tap lands somewhere other than where it was aimed.
			sx := float64(sh2.DisplayWidth) / rect.Get("width").Float()
			sy := float64(sh2.DisplayHeight) / rect.Get("height").Float()
			x := (e.Get("clientX").Float() - rect.Get("left").Float()) * sx
			y := (e.Get("clientY").Float() - rect.Get("top").Float()) * sy
			p.post(gui.PointerEvent{
				Pressed: pressed,
				Pos:     image.Pt(int(x), int(y)),
			}.Event())
			return nil
		})
	}
	canvas.Call("addEventListener", "pointerdown", send(true))
	canvas.Call("addEventListener", "pointerup", send(false))
}

// post queues an event, dropping it if the GUI has fallen far enough behind
// that 16 are already waiting. Blocking here would deadlock: this runs on a JS
// callback, and the goroutine that drains the queue only runs when JS yields.
func (p *platform) post(e gui.Event) {
	select {
	case p.events <- e:
	default:
	}
}

func (p *platform) DisplaySize() image.Point {
	return image.Pt(sh2.DisplayWidth, sh2.DisplayHeight)
}

func (p *platform) Dirty(r image.Rectangle) error {
	r = r.Intersect(p.fb.Rect)
	p.dirty = r
	p.pending = !r.Empty()
	return nil
}

// NextChunk hands the dirty region out ONCE and blits when the GUI asks for
// the chunk after it.
//
// The device splits a frame into DMA-sized bands because it has 16KB to draw
// 480x320 into. A browser has no such constraint, so one chunk is the whole
// rect -- the contract is only that the loop terminates on !ok, not that a
// particular number of chunks appear.
func (p *platform) NextChunk() (draw.RGBA64Image, bool) {
	if !p.pending {
		if p.drew {
			p.blit()
			p.drew = false
		}
		return nil, false
	}
	p.pending = false
	p.drew = true
	// Bounds are in SCREEN coordinates, as the machine's are: the drawer
	// positions content by them.
	return p.fb.SubImage(p.dirty).(*image.RGBA), true
}

func (p *platform) blit() {
	js.CopyBytesToJS(p.buf, p.fb.Pix)
	p.img.Get("data").Call("set", p.buf)
	p.ctx2d.Call("putImageData", p.img, 0, 0)
}

// AppendEvents blocks until an event arrives, a wakeup is posted, or the
// deadline passes -- exactly as the machine's does.
//
// The blocking is load-bearing rather than incidental: Go's wasm runtime hands
// control back to the browser when every goroutine is parked, so a select on
// channels here is what keeps the page responsive. A spin loop would freeze the
// tab.
func (p *platform) AppendEvents(deadline time.Time, evts []gui.Event) []gui.Event {
	// Don't starve input, as platform_sh2.go does not.
	select {
	case e := <-p.events:
		return append(evts, e)
	default:
	}
	d := time.Until(deadline)
	if d <= 0 {
		return evts
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
	case <-p.wakeups:
	case e := <-p.events:
		return append(evts, e)
	}
	return evts
}

func (p *platform) Wakeup() {
	select {
	case p.wakeups <- struct{}{}:
	default:
	}
}

func (p *platform) Engraver(stall bool) (gui.Engraver, error) {
	return &emuEngraver{}, nil
}

func (p *platform) EngraverParams() engrave.Params { return sh2.Params() }

// NFCReader returns nil: this emulator has no tag source.
//
// nil is a SUPPORTED value, not a stub -- gui checks it (verify_address.go,
// mk1_inspect.go, md1_gather.go) and offers Back-only where a scan would go.
// Returning a reader that never yields would hang those screens instead.
func (p *platform) NFCReader() io.ReadCloser { return nil }

// Features reports no secure boot, so the version line reads "(UNLOCKED)".
// That is true here and worth saying: nothing about a browser build is signed.
func (p *platform) Features() gui.Features { return 0 }

func (p *platform) HardwareVersion() string { return "emulator" }

// LockBoot is refused rather than faked. Locking boot on the real machine is
// an irreversible OTP write, and a UI that reports success for it here would
// be rehearsing the one operation whose rehearsal must not lie.
func (p *platform) LockBoot() error {
	return errors.New("secure boot cannot be locked in the emulator")
}
