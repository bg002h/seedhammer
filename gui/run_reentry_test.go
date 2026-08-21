package gui

// Host reproduction of the hardware hang: unlock a sealed payload, park on
// Cut/Skip, let (or force) §10.2.4's wipe, and re-enter the Sealed Payload
// entry in the restarted session. On the machine the re-entry checkmark draws
// its pressed style and then everything stops -- no frames, no touch, no
// screensaver -- which means the flow stopped calling ctx.Frame, since the
// saver, the timer and touch all live in runWithFlow's inner loop and that
// loop only runs between frames.
//
// Every existing uiFlow test drives the flow through runUITouch, which
// BYPASSES Run entirely -- so the session-restart path had never been
// exercised with the real uiFlow and a real payload. These tests close that
// hole: they drive runWithFlow + the REAL uiFlow + vector D end to end,
// twice through the carousel, with a wipe in between.
//
// Failure shapes, all loud:
//   - the flow durably blocks after re-entry -> every goroutine in the
//     synctest bubble is blocked and synctest panics with full stacks. That
//     panic IS the reproduction: the stack shows exactly where the flow
//     stopped calling ctx.Frame.
//   - the flow keeps drawing but never reaches the hash screen -> the tick
//     cap trips and the test fails with the last frames drawn.
//   - a driver tap misses its target -> recorded via t.Errorf and the run is
//     abandoned at the cap with the stall step named.
//
// What the cap CANNOT bound: a flow that spins on CPU without drawing and
// without blocking. Ticks only advance between frames, so that shape is
// go test's own timeout. boundedFlow cannot help here -- it wraps synthetic
// per-tick bodies, and this flow is the real uiFlow.

import (
	"fmt"
	"image"
	"image/draw"
	"io"
	"sort"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"seedhammer.com/gui/assets"
	"seedhammer.com/gui/op"
)

// fakeNFC models the one hardware component StartScreen.Flow spawns a
// goroutine around: an NFC reader with no tag present, whose Read blocks until
// Close. On the SH2 the checkmark dispatch runs Flow's deferred
// close(closer); r.Close(); <-closed handshake against exactly this shape, and
// no Run-level test had ever had a reader at all.
type fakeNFC struct {
	closed chan struct{}
}

func (f *fakeNFC) Read(p []byte) (int, error) {
	<-f.closed
	return 0, io.EOF
}

func (f *fakeNFC) Close() error {
	select {
	case <-f.closed:
	default:
		close(f.closed)
	}
	return nil
}

// hitPlatform is deadlinePlatform plus a real (1x1) framebuffer chunk.
//
// It exists because Run routes touch via ctx.Router.Events(d, ...) where d is
// Run's OWN drawer, populated by d.Draw(fb, ...) -- and testPlatform.NextChunk
// returns (nil, false), so at Run level no drawn frame ever had a single hit
// target and no pointer event could ever land. (Every previous Run-level test
// used p.tap() only for its len(evts) > 0 idle refresh, never for a click.)
//
// The framebuffer can be 1x1 because opInput registration records state.clip
// WITHOUT intersecting dst bounds (gui/op/op.go, case opInput) -- hit geometry
// stays exact while the pixel pass clips to one pixel and costs nothing.
type hitPlatform struct {
	*deadlinePlatform
	fb      *image.RGBA64
	pending bool
	// split delivers ONE queued event per AppendEvents call instead of the
	// whole batch. That is the hardware's shape: the panel delivers press and
	// release on separate ticks, which is why the machine renders a
	// pressed-style frame at all -- and the hang's last frame IS that frame.
	split bool
}

func newHitPlatform() *hitPlatform {
	return &hitPlatform{
		deadlinePlatform: newDeadlinePlatform(),
		fb:               image.NewRGBA64(image.Rect(0, 0, 1, 1)),
	}
}

func (p *hitPlatform) AppendEvents(deadline time.Time, evts []Event) []Event {
	if p.split && len(p.queued) > 0 {
		evts = append(evts, p.queued[0])
		p.queued = p.queued[1:]
		return evts
	}
	return p.deadlinePlatform.AppendEvents(deadline, evts)
}

func (p *hitPlatform) Dirty(r image.Rectangle) error {
	p.pending = true
	return p.deadlinePlatform.Dirty(r)
}

func (p *hitPlatform) NextChunk() (draw.RGBA64Image, bool) {
	if !p.pending {
		return nil, false
	}
	p.pending = false
	return p.fb, true
}

// reentryStep is one scripted action: when a drawn frame contains wait, act
// runs (queuing at most one tap) and the script advances. Frames that do not
// match are simply pumped, so extra redraws of the same screen are harmless.
type reentryStep struct {
	name string
	wait string
	act  func()
}

// reentryDriver walks the script from inside Run's onDraw observer -- the only
// same-goroutine seam a Run-level test has (see runSession's doc).
type reentryDriver struct {
	t     *testing.T
	p     *hitPlatform
	dims  image.Point
	steps []reentryStep
	step  int
	// d hit-tests the current frame. A separate drawer from Run's own, because
	// runWithFlow does not expose its drawer; ExtractText populates the same
	// input registrations Draw does.
	d      *op.Drawer
	text   string
	frames int
	// reentryTap is the frame index of session 2's checkmark tap on Sealed
	// Payload -- the exact input that kills the hardware. -1 until it happens.
	reentryTap int
	succeeded  bool
	dead       bool
	keys       map[rune]image.Point
}

// fail records a driver failure WITHOUT t.Fatal: this runs inside a live
// iterator, and Goexit through one is not safe (runSession's own rule).
func (drv *reentryDriver) fail(format string, args ...any) {
	drv.t.Errorf(format, args...)
	drv.dead = true
}

func (drv *reentryDriver) queueTap(pos image.Point) {
	drv.p.queued = append(drv.p.queued,
		PointerEvent{Pressed: true, Entered: true, Pos: pos}.Event(),
		PointerEvent{Pressed: false, Entered: true, Pos: pos}.Event(),
	)
}

// tapRight taps the carousel's right arrow, asserting the target really is the
// Right-bound Clickable so a layout drift cannot silently mis-tap.
func (drv *reentryDriver) tapRight() {
	pos := image.Pt(drv.dims.X-3, drv.dims.Y/2)
	tag, _, hit := drv.d.Hit(pos)
	if !hit {
		drv.fail("right arrow: no touch target at %v on %q", pos, drv.text)
		return
	}
	c, ok := tag.(*Clickable)
	if !ok || c.Button != Right {
		drv.fail("right arrow: target at %v is %v, not the Right arrow", pos, tag)
		return
	}
	drv.queueTap(pos)
}

// tapNav taps a nav slot by button -- tapNavSlot's geometry and assertion, but
// queued through the platform so Run itself routes the event.
func (drv *reentryDriver) tapNav(b Button) {
	sz := assets.NavBtnPrimary.Bounds().Size()
	ys := [3]int{leadingSize, (drv.dims.Y - sz.Y) / 2, drv.dims.Y - leadingSize - sz.Y}
	pos := image.Pt(drv.dims.X-sz.X/2, ys[int(b-Button1)]+sz.Y/2)
	tag, _, hit := drv.d.Hit(pos)
	if !hit {
		drv.fail("nav %v: no touch target at %v on %q", b, pos, drv.text)
		return
	}
	c, ok := tag.(*Clickable)
	if !ok || (c.Button != b && c.AltButton != b) {
		drv.fail("nav %v: the target at %v is %v, not a Clickable bound to %v", b, pos, tag, b)
		return
	}
	drv.queueTap(pos)
}

// keyPoints is unlockHarness.keyPoints against the Run-level frame: locate
// every word-keyboard key by hit-testing what was actually drawn. Cached; the
// keyboard does not move between words.
func (drv *reentryDriver) keyPoints() map[rune]image.Point {
	if drv.keys != nil {
		return drv.keys
	}
	type target struct {
		tag  op.Tag
		rect image.Rectangle
	}
	var found []target
	seen := make(map[op.Tag]bool)
	for y := 0; y < drv.dims.Y; y += 2 {
		for x := 0; x < drv.dims.X; x += 2 {
			tag, rect, ok := drv.d.Hit(image.Pt(x, y))
			if !ok {
				continue
			}
			c, isClk := tag.(*Clickable)
			if !isClk || c.Button != None {
				continue
			}
			if seen[tag] {
				continue
			}
			seen[tag] = true
			found = append(found, target{tag, rect})
		}
	}
	sort.Slice(found, func(i, j int) bool {
		if found[i].rect.Min.Y != found[j].rect.Min.Y {
			return found[i].rect.Min.Y < found[j].rect.Min.Y
		}
		return found[i].rect.Min.X < found[j].rect.Min.X
	})
	var rows [][]target
	for _, f := range found {
		if n := len(rows); n > 0 && rows[n-1][0].rect.Min.Y == f.rect.Min.Y {
			rows[n-1] = append(rows[n-1], f)
			continue
		}
		rows = append(rows, []target{f})
	}
	want := strings.Split(wordKeys, "\n")
	want[len(want)-1] += "⌫"
	if len(rows) != len(want) {
		drv.fail("the word keyboard drew %d rows of touch targets, want %d; frame %q",
			len(rows), len(want), drv.text)
		return nil
	}
	out := make(map[rune]image.Point)
	for i, rr := range want {
		rs := []rune(rr)
		if len(rows[i]) != len(rs) {
			drv.fail("keyboard row %d drew %d touch targets, want %d (%q)", i, len(rows[i]), len(rs), rr)
			return nil
		}
		for j, r := range rs {
			rect := rows[i][j].rect
			out[r] = rect.Min.Add(rect.Max).Div(2)
		}
	}
	drv.keys = out
	return out
}

func (drv *reentryDriver) tapKey(r rune) {
	keys := drv.keyPoints()
	if keys == nil {
		return
	}
	pt, ok := keys[r]
	if !ok {
		drv.fail("the word keyboard has no %q key", string(r))
		return
	}
	if _, _, hit := drv.d.Hit(pt); !hit {
		drv.fail("the %q key's touch target vanished at %v", string(r), pt)
		return
	}
	drv.queueTap(pt)
}

// carouselTitles is one lap of the payload-present carousel up to (not
// including) the Sealed Payload entry, in pager order.
var reentryCarouselTitles = []string{
	"Backup Wallet",
	"BIP-39 Password",
	"Engrave Text",
	"Account Xpub",
	"Engrave Bundle",
	"Engrave Single-Sig",
	"Engrave Multisig",
	"Wallet Policy",
	// Load Payload sits here, between Engrave Multisig and BIP-85: it is
	// UNCONDITIONAL and so was inserted mid-enum, leaving bip85Derive as the
	// bound lastNav returns and Sealed Payload last. This list mirrors the
	// carousel, so it moves when the carousel does.
	"Load Payload",
	"BIP-85",
}

// addCarouselSteps scripts eight right-taps to the Sealed Payload entry and
// then onSelect at it.
func (drv *reentryDriver) addCarouselSteps(session string, onSelect func()) {
	for _, ti := range reentryCarouselTitles {
		drv.steps = append(drv.steps, reentryStep{
			name: session + ": right-tap from " + ti,
			wait: ti,
			act:  drv.tapRight,
		})
	}
	drv.steps = append(drv.steps, reentryStep{
		name: session + ": checkmark on Sealed Payload",
		wait: "Sealed Payload",
		act:  onSelect,
	})
}

// reentryTickCap bounds the run. The full two-session script measures ~700
// ticks in the idle-wipe variant at the 1 s tick floor; 5000 is margin, and a
// stalled-but-still-drawing run costs at most 5000 cheap 1x1-fb frames.
const reentryTickCap = 5000

// reentryConfig selects one hardware-fidelity axis combination.
type reentryConfig struct {
	// vector is the sealed payload driven. F is the hardware repro's shape
	// (pub_len 0, 15 secret ms1 records, so the flow entry has NO hash
	// notice); D adds a public section and the §6.6 hash screen.
	vector string
	// timerWipe selects the REAL §10.2.4 idle wipe (the hardware's 3:00 +
	// 30 s path) over the wipeNowHook-forced one; the restart mechanism they
	// trigger is the same (wiping = true, ctx.Done = true, break).
	timerWipe bool
	// nfc gives the platform a no-tag-present NFC reader, so StartScreen.Flow
	// spawns its scanner goroutine and runs the close handshake on every
	// checkmark dispatch, as the SH2 does.
	nfc bool
	// split delivers press and release on separate ticks, as the panel does.
	split bool
}

// driveSealedPayloadReentry runs the whole scenario under one config.
func driveSealedPayloadReentry(t *testing.T, cfg reentryConfig) {
	synctest.Test(t, func(t *testing.T) {
		p := newHitPlatform()
		// 1 s floor: idleTimeout is crossed in 180 ticks instead of 18,000.
		// Harmless to the script itself -- every scripted tick delivers queued
		// events and returns without sleeping.
		p.tickFloor = time.Second
		v := sealVector(t, cfg.vector)
		hasHash := v.PubLen > 0
		p.payload = payloadReaderFrom(t, v.blob(t))
		if cfg.nfc {
			p.nfc = func() io.ReadCloser { return &fakeNFC{closed: make(chan struct{})} }
		}
		p.split = cfg.split
		t.Cleanup(func() { wipeNowHook = nil })

		drv := &reentryDriver{t: t, p: p, dims: sh2DisplaySize, reentryTap: -1}

		// entryScreen is the first frame the payload flow draws, and therefore
		// also the session-2 proof that the re-entered flow kept drawing.
		entryScreen := "Enter the 12-word passphrase"
		if hasHash {
			entryScreen = "Public data hash"
		}

		// --- session 1: unlock the vector to the Cut/Skip screen ---
		drv.addCarouselSteps("s1", func() { drv.tapNav(Button3) })
		if hasHash {
			drv.steps = append(drv.steps,
				reentryStep{"s1: dismiss the hash notice", "Public data hash", func() { drv.tapNav(Button3) }})
		}
		drv.steps = append(drv.steps,
			reentryStep{"s1: dismiss the passphrase notice", "Enter the 12-word passphrase", func() { drv.tapNav(Button3) }},
		)
		for i, w := range vectorPassphrase(t, v) {
			title := fmt.Sprintf("Word %d of 12", i+1)
			for _, r := range strings.ToLower(w) {
				drv.steps = append(drv.steps, reentryStep{
					name: fmt.Sprintf("s1: word %d key %q", i+1, string(r)),
					wait: title,
					act:  func() { drv.tapKey(r) },
				})
			}
			drv.steps = append(drv.steps, reentryStep{
				name: fmt.Sprintf("s1: word %d OK", i+1),
				wait: title,
				act:  func() { drv.tapNav(Button3) },
			})
		}
		// The KDF's ~200 frames pump through here unmatched; the next step
		// fires on the ChoiceScreen §10.2.2 parks the operator on.
		if cfg.timerWipe {
			drv.steps = append(drv.steps,
				// Park. No input from here on, so §10.2.4's armed timer runs:
				// warning at 3:00, wipe at 3:30.
				reentryStep{"s1: parked on Cut/Skip", "SECRET seed material", func() {}},
				reentryStep{"s1: §10.2.4 warning observed", "erased in", func() {}},
			)
		} else {
			drv.steps = append(drv.steps,
				reentryStep{"s1: parked on Cut/Skip; wipe forced", "SECRET seed material", func() { armWipe(t) }},
			)
		}

		// --- session 2: the restarted flow re-enters Sealed Payload ---
		drv.addCarouselSteps("s2", func() {
			drv.reentryTap = drv.frames
			drv.tapNav(Button3)
		})
		drv.steps = append(drv.steps, reentryStep{
			name: "s2: the re-entered flow drew its entry screen",
			wait: entryScreen,
			act:  func() { drv.succeeded = true },
		})

		// --- run ---
		var drawn []string
		r := image.Rectangle{Max: sh2DisplaySize}
		observe := func(o op.Op) {
			drv.frames++
			d := new(op.Drawer)
			txt := d.ExtractText(r, o)
			drv.d, drv.text = d, txt
			drawn = append(drawn, txt)
			if drv.dead || drv.step >= len(drv.steps) {
				return
			}
			// Never act while queued events are still undelivered: under split
			// delivery a tap resolves over TWO ticks, and the intermediate
			// pressed-style frame still matches the previous screen's text --
			// acting on it would race the script ahead of its own clicks.
			if len(p.queued) > 0 {
				return
			}
			if st := drv.steps[drv.step]; uiContains(txt, st.wait) {
				st.act()
				drv.step++
			}
		}
		ticks, capped := 0, false
		for range runWithFlow(p, "test", uiFlow, observe) {
			ticks++
			if drv.succeeded {
				// Stop ranging: yield returns false, ctx.Done is set, uiFlow
				// unwinds cleanly. Never t.Fatal from inside the loop.
				break
			}
			if ticks > reentryTickCap {
				capped = true
				break
			}
		}

		// --- verdict ---
		if drv.succeeded {
			if drv.reentryTap < 0 {
				t.Fatal("bookkeeping: the hash screen appeared without the re-entry tap ever being recorded")
			}
			return // session 2 re-entered the flow and it kept drawing
		}
		tail := drawn
		if len(tail) > 6 {
			tail = tail[len(tail)-6:]
		}
		stepName := "<all steps consumed>"
		if drv.step < len(drv.steps) {
			stepName = drv.steps[drv.step].name
		}
		if drv.reentryTap >= 0 {
			t.Fatalf("HANG REPRODUCED: after the session-2 checkmark on Sealed Payload the flow "+
				"drew %d more frame(s) and never reached %q (stalled at step %q; "+
				"capped=%v after %d ticks, %d frames total).\nlast frames: %q",
				drv.frames-drv.reentryTap, entryScreen, stepName, capped, ticks, drv.frames, tail)
		}
		t.Fatalf("driver stalled BEFORE the re-entry tap, at step %q (capped=%v after %d ticks, "+
			"%d frames total) -- fix the script, this run proves nothing about the hang.\nlast frames: %q",
			stepName, capped, ticks, drv.frames, tail)
	})
}

// TestRunSealedPayloadReentryAfterWipe drives the scenario across the
// hardware-fidelity axes the host CAN model. "F idle-wipe nfc" is the hardware
// scenario verbatim: vector F's shape, the real §10.2.4 timer, and the NFC
// scanner goroutine live around every carousel dispatch. The others triangulate
// -- if one combination hangs and another does not, the differing axis is where
// the bug lives.
func TestRunSealedPayloadReentryAfterWipe(t *testing.T) {
	for _, tc := range []struct {
		name string
		cfg  reentryConfig
	}{
		{"F idle-wipe nfc split", reentryConfig{vector: "F", timerWipe: true, nfc: true, split: true}},
		{"F idle-wipe nfc", reentryConfig{vector: "F", timerWipe: true, nfc: true}},
		{"F forced-wipe nfc", reentryConfig{vector: "F", timerWipe: false, nfc: true}},
		{"D idle-wipe nfc", reentryConfig{vector: "D", timerWipe: true, nfc: true}},
		{"D forced-wipe bare", reentryConfig{vector: "D", timerWipe: false, nfc: false}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			driveSealedPayloadReentry(t, tc.cfg)
		})
	}
}
