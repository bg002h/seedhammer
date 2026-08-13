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
	"seedhammer.com/seal"
	"seedhammer.com/sysw"
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

	// toolpath decodes every step the GUI writes to an engraver. See
	// toolpath.go; exposed to the page as window.shToolpath.
	toolpath *toolpathRecorder
	// nfc is a tag source the PAGE supplies; see nfc.go. Without it the
	// emulator cannot walk any NFC journey, which spec §8.2 calls out.
	nfc *nfcSource
	// plan is the plate currently being cut, rendered whole at the moment the
	// engrave begins. See plate.go. It arrives through gui.PlateAware, which
	// exists in this build and NOT in the firmware's.
	plan *platePlan

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
		events:   make(chan gui.Event, 16),
		wakeups:  make(chan struct{}, 1),
		toolpath: newToolpathRecorder(),
		plan:     new(platePlan),
		nfc:      new(nfcSource),
	}
	installToolpathAPI(p.toolpath, p.plan)
	installNFCAPI(p.nfc)

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
	// One recorder across every job, so a plate that is aborted and resumed
	// records as ONE motion -- which is the thing being compared. The RECORDING
	// still restarts for a new plate; see beginPlate.
	return &emuEngraver{
		job:    jobRecorder{rec: p.toolpath},
		plan:   p.plan,
		params: p.EngraverParams(),
	}, nil
}

func (p *platform) EngraverParams() engrave.Params { return sh2.Params() }

// NFCReader hands out the record the PAGE presented, once, and nil after that.
//
// The comment here used to read "returns nil: this emulator has no tag source",
// which stopped being true when nfc.go was added and was never updated. It
// matters now: stage 10 made the consuming half of NFC real, so this is the
// source every J-C walk in a browser goes through.
//
// nil is still a SUPPORTED value, not a stub -- gui checks it (verify_address.go,
// mk1_inspect.go, md1_gather.go, derive_xpub.go) and offers Back-only where a
// scan would go.
//
// AND CALLING THIS CONSUMES THE TAG. reader() marks the pending record done, so
// no flow may call it merely to ask whether a reader exists: doing that to
// decide whether to draw a SCAN row would eat the operator's tag before they
// chose anything. syswSeedPicker says the same at its own site.
func (p *platform) NFCReader() io.ReadCloser { return p.nfc.reader() }

// PayloadReader returns a reader over the built-in EMULATOR-ONLY test sealed
// payload (sealed_test_payload.go), so §10.1's detection finds MNEMBLOB and
// the Sealed Payload entry appears — letting an operator walk the whole
// passphrase/KDF/decrypt/plate flow in the browser with no hardware.
//
// This USED TO return nil unconditionally ("a browser build has no XIP
// flash"), which was true and is still true — the change below is not a
// browser flash read, it is a fixed in-memory blob. It is still deliberately
// NOT a seal.FileReader fed from a flag: this file is //go:build js, built
// GOOS=js GOARCH=wasm and run from a static page through wasm_exec.js, so
// there is no flag.Parse() in the package, no real argv under the browser
// glue, and — unlike Node — no globalThis.fs for os.Open to talk to, so a
// FileReader's os.Open would simply fail every read in an actual browser.
// The automated coverage runs through gui's test platform instead. If an
// OPERATOR-SUPPLIED browser-side payload is ever wanted on top of this, the
// mechanism is a syscall/js read of location.search or a JS global set from
// the host page — nothing here forecloses that.
// SyswReader serves the built-in systemwide test container, so Load Payload
// and the FROM PAYLOAD input default can be walked in the browser.
//
// This USED TO return nil, on the same reasoning the paragraph above once
// carried: "a browser build has no systemwide region." Also true, also beside
// the point — nil is a supported value, and §10.1 detection finding nothing is
// a faithful emulation of a machine with an EMPTY region. What it cannot
// emulate is a machine with a FULL one, which is every interesting screen this
// program has: the boot offer, the digest comparison, the pickers acquiring a
// payload row. Load Payload was the only one of the ten programs the simulator
// could not walk at all.
//
// See sysw_test_payload.go for the blob's provenance and why it carries three
// record classes rather than one.
func (p *platform) SyswReader() sysw.Reader {
	return embeddedSyswReader{data: []byte(syswTestPayload)}
}

func (p *platform) PayloadReader() seal.Reader {
	return embeddedPayloadReader{data: []byte(sealedTestPayload)}
}

// Features reports no secure boot, so the version line reads "(UNLOCKED)".
// That is true here and worth saying: nothing about a browser build is signed.
//
// It DOES report FeatureNFC, and that is not a fib: nfcSource above is a real
// tag source, fed from `window.shNFC`, and §8.2 makes walking the NFC journeys
// in a browser part of this work. Reporting no reader here would take the SCAN
// row off every seed entry in the emulator and leave J-C unwalkable by the very
// tool that exists to walk it. The bit reports the READER, not a pending tag —
// which is why it is a constant rather than a peek at p.nfc, a peek that would
// consume the tag it looked at.
func (p *platform) Features() gui.Features { return gui.FeatureNFC }

func (p *platform) HardwareVersion() string { return "emulator" }

// LockBoot is refused rather than faked. Locking boot on the real machine is
// an irreversible OTP write, and a UI that reports success for it here would
// be rehearsing the one operation whose rehearsal must not lie.
func (p *platform) LockBoot() error {
	return errors.New("secure boot cannot be locked in the emulator")
}
