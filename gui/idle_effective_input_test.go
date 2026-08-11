package gui

import (
	"image"
	"testing"
	"testing/synctest"
	"time"

	"seedhammer.com/gui/op"
	"seedhammer.com/gui/widget"
)

// F-103's regression tests: §10.2.4's clock must be refreshed by EFFECTIVE
// input, not by event ARRIVAL.
//
// The hazard, observed on real hardware 2026-08-09 and traced in
// design/agent-reports/2026-08-10-f103-screen-film-mechanism.md: the factory
// protective film resting on the panel makes ft6x36 assert contact
// continuously, and processTouch's dedupe is EXACT EQUALITY on (touching, pos)
// (cmd/controller/platform_sh2.go:401), so every reading that drifts by one
// pixel is delivered as a fresh event. Run refreshed a.idle.start on
// `len(evts) > 0`, so the machine never went idle -- and because §10.2.4's
// warning is nested inside `if a.idle.active`, it never warned either. No
// countdown, no wipe, nothing on screen, forever.
//
// Two properties are pinned here and they pull in opposite directions:
//
//	TestSpuriousTouchDoesNotHoldOffTheWipe  a stream that changes nothing must
//	                                        NOT hold the clock
//	TestGenuineTapsStillHoldOffTheWipe      an operator who IS touching the
//	                                        machine must never be wiped out
//	                                        from under them
//
// Getting the first by breaking the second is a worse defect than the one
// being fixed, which is why the second exists as a test rather than as a
// sentence in a commit message.

// TestEffectiveInputClassification pins the definition itself, kind by kind.
//
// The two tests below observe the predicate only through §10.2.4's 3-minute
// window, which is a coarse instrument: several distinct wrong definitions
// produce the same wipe/no-wipe answer. This one names each case, and in
// particular pins the two properties that are invisible from the window --
// that the WHOLE batch is scanned (so *pressed is never left stale), and that
// a camera frame is not operator input.
func TestEffectiveInputClassification(t *testing.T) {
	center := image.Pt(10, 20)
	drift := image.Pt(11, 21)
	press := PointerEvent{Pressed: true, Entered: true, Pos: center}.Event()
	pressElsewhere := PointerEvent{Pressed: true, Entered: true, Pos: drift}.Event()
	release := PointerEvent{Pressed: false, Entered: true, Pos: image.Point{}}.Event()

	tests := []struct {
		name        string
		pressed     bool
		evts        []Event
		want        bool
		wantPressed bool
	}{
		{"no events at all", false, nil, false, false},
		{"no events while held", true, nil, false, true},
		{"press from released", false, []Event{press}, true, true},
		{"release from pressed", true, []Event{release}, true, false},
		// The defect: contact already asserted, reading drifts. This is the
		// film, one poll of it.
		{"drift under a held contact", true, []Event{pressElsewhere}, false, true},
		{"a run of drift under a held contact", true,
			[]Event{press, pressElsewhere, press, pressElsewhere}, false, true},
		// A release repeated while already released cannot happen through
		// processTouch's dedupe, but nothing in the type system says so.
		{"release while already released", false, []Event{release}, false, false},
		// A whole tap in ONE batch: effective, and it must leave the contact
		// state RELEASED. An early return at the press would leave it pressed
		// and swallow the next tap's press edge.
		{"press and release in one batch", false, []Event{press, release}, true, false},
		{"tap then drift in one batch", false,
			[]Event{press, release, pressElsewhere}, true, true},
		{"rune", false, []Event{RuneEvent{Rune: 'a'}.Event()}, true, false},
		{"button press", false,
			[]Event{ButtonEvent{Button: Center, Pressed: true}.Event()}, true, false},
		{"button release", false,
			[]Event{ButtonEvent{Button: Center, Pressed: false}.Event()}, true, false},
		// Machine output, not operator input -- and a 30 Hz stream of it is
		// the exact shape of F-103.
		{"camera frame", false, []Event{FrameEvent{}.Event()}, false, false},
		{"camera frame while held", true, []Event{FrameEvent{}.Event()}, false, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pressed := tc.pressed
			got := effectiveInput(tc.evts, &pressed)
			if got != tc.want {
				t.Errorf("effectiveInput(%v, pressed=%v) = %v, want %v",
					tc.evts, tc.pressed, got, tc.want)
			}
			if pressed != tc.wantPressed {
				t.Errorf("effectiveInput left pressed = %v, want %v -- the tracked "+
					"contact state is stale, so a later press edge will be missed",
					pressed, tc.wantPressed)
			}
		})
	}
}

// filmPlatform models the 2026-08-09 hardware observation: contact asserted
// forever, position drifting, NEVER released.
func filmPlatform() *deadlinePlatform {
	p := newDeadlinePlatform()
	p.poll = func() []Event {
		// A 9-long cycle of distinct positions, so no two consecutive
		// readings are equal and processTouch's dedupe suppresses none of
		// them. Pressed stays true: an object resting on the panel does not
		// lift.
		return []Event{PointerEvent{
			Pressed: true,
			Entered: true,
			Pos: image.Pt(
				sh2DisplaySize.X/2+p.polls%3,
				sh2DisplaySize.Y/2+(p.polls/3)%3,
			),
		}.Event()}
	}
	return p
}

// TestSpuriousTouchDoesNotHoldOffTheWipe is F-103's regression test.
//
// This is the experiment the 2026-08-10 investigation ran once and deleted
// instead of committing. Pre-fix it drove 100,000 spurious polls over ~1000 s
// of fake time -- 4.8x past the 3:30 deadline -- and observed zero warnings
// and zero wipes. mustFinish's cap IS that experiment: maxRunFrames is
// 100,000 and one poll costs one tick, so under the pre-fix `len(evts) > 0`
// refresh this test fails with "Run exceeded 100000 ticks", having reproduced
// the total, silent, permanent disablement exactly. Measured on this branch
// before the fix: 100,000 polls, no warning, no wipe.
//
// Mutations this kills: `effective` -> `len(evts) > 0`; effectiveInput ->
// always true; inverting effectiveInput's result.
func TestSpuriousTouchDoesNotHoldOffTheWipe(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p := filmPlatform()
		var trk sessionTracker
		var startAt, warnAt time.Time
		flow := boundedFlow(t, func(ctx *Context) bool {
			session, tick := trk.next(ctx)
			if ctx.wipe == nil {
				// The Cut/Skip screen's exact shape: a guard installed with
				// no engrave job, so armed() reads true unconditionally.
				ctx.wipe = &wipeGuard{}
			}
			if session == 1 && tick == 1 {
				startAt = time.Now()
			}
			if session == 2 {
				return false
			}
			o, _ := widget.Label(&ctx.B, ctx.Styles.body, descriptorTheme.Text, "CUT THIS PLATE")
			ctx.Frame(o)
			return true
		})
		onDraw := func(o op.Op, text string) {
			if warnAt.IsZero() && uiContains(text, "erased in") {
				warnAt = time.Now()
			}
		}
		drawn := mustFinish(t, p, flow, onDraw)

		// The premise FIRST. A generator that stopped producing readings, or
		// a platform whose fake clock never advanced, would make every
		// assertion below pass for a reason that has nothing to do with the
		// fix. Crossing the 3:30 deadline at a 10ms floor takes ~21,000
		// polls, so anything under that means the panel was not being read
		// for the whole window.
		if p.polls < 21000 {
			t.Fatalf("INCONCLUSIVE: the panel was polled only %d times; the spurious "+
				"stream did not span the §10.2.4 window", p.polls)
		}
		if warnAt.IsZero() {
			t.Fatalf("the §10.2.4 warning never appeared under %d spurious touch polls -- "+
				"raw event ARRIVAL is still refreshing the idle clock (F-103); drawn=%d frames",
				p.polls, len(drawn))
		}
		if trk.session != 2 {
			t.Fatalf("expected the wipe to fire and restart the session (session 2), got %d "+
				"sessions under %d spurious touch polls", trk.session, p.polls)
		}
		// Not just "eventually": ON SCHEDULE. A fix that merely made the
		// window longer would still leave a secret resident for an unbounded
		// time.
		got := warnAt.Sub(startAt)
		const tol = 5 * time.Second
		if got < idleTimeout-tol || got > idleTimeout+tol {
			t.Errorf("the warning appeared %v after the guard was installed, want ~%v -- "+
				"spurious panel readings are still moving the §10.2.4 clock", got, idleTimeout)
		}
	})
}

// tappingPlatform is an operator using the machine: a press/release pair every
// `every` polls, and nothing in between. At a 1s floor and every=20 that is a
// tap every 20 seconds, which is slower than anyone types twelve words on a
// touch keyboard -- deliberately, so the test has margin against the 3:00
// deadline rather than sitting on it.
func tappingPlatform(every int) (*deadlinePlatform, *int) {
	p := newDeadlinePlatform()
	p.tickFloor = time.Second
	taps := 0
	p.poll = func() []Event {
		if p.polls%every != 0 {
			return nil
		}
		taps++
		pos := image.Pt(sh2DisplaySize.X/2, sh2DisplaySize.Y/2)
		// Press AND release, in ONE batch. That is what deadlinePlatform.tap
		// delivers, and a predicate that stopped scanning a batch at its
		// first effective event would leave its notion of the contact state
		// stale -- so the pair being in one batch is load-bearing here.
		return []Event{
			PointerEvent{Pressed: true, Entered: true, Pos: pos}.Event(),
			PointerEvent{Pressed: false, Entered: true, Pos: pos}.Event(),
		}
	}
	return p, &taps
}

// TestGenuineTapsStillHoldOffTheWipe is the other half of F-103, and the half
// that is easy to break while fixing the first.
//
// An operator standing at the machine tapping through a passphrase keyboard
// must never be wiped out from under them, so a run four times longer than the
// §10.2.4 window with a tap every 20 s must produce no warning and no wipe.
//
// Mutation this kills: effectiveInput -> always false (nothing can refresh the
// clock any more, so the wipe fires under an operator's hands).
func TestGenuineTapsStillHoldOffTheWipe(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p, taps := tappingPlatform(20)
		// 4x the window. Long enough that a clock which is not being
		// refreshed cannot possibly survive it.
		const runFor = 4 * idleTimeout
		var trk sessionTracker
		var start time.Time
		warned := false
		flow := boundedFlow(t, func(ctx *Context) bool {
			session, tick := trk.next(ctx)
			if ctx.wipe == nil {
				ctx.wipe = &wipeGuard{}
			}
			if session == 1 && tick == 1 {
				start = time.Now()
			}
			if session > 1 || time.Since(start) > runFor {
				return false
			}
			o, _ := widget.Label(&ctx.B, ctx.Styles.body, descriptorTheme.Text, "TYPING")
			ctx.Frame(o)
			return true
		})
		onDraw := func(o op.Op, text string) {
			if uiContains(text, "erased in") || uiContains(text, "WIPING SECRET DATA") {
				warned = true
			}
		}
		mustFinish(t, p, flow, onDraw)

		// Premise: the operator really did tap, many times, across the whole
		// run. runFor/tickFloor/every = 720/20 = 36 taps.
		if *taps < 30 {
			t.Fatalf("INCONCLUSIVE: only %d taps were delivered over %v; the operator was "+
				"not touching the machine, so nothing here tests that touching helps",
				*taps, runFor)
		}
		if elapsed := time.Since(start); elapsed < runFor {
			t.Fatalf("INCONCLUSIVE: the run lasted %v, less than the %v it must span to "+
				"cross the §10.2.4 window at all", elapsed, runFor)
		}
		if warned {
			t.Error("§10.2.4 warned at an operator who was tapping the screen every 20s -- " +
				"genuine input is no longer holding off the wipe")
		}
		if trk.session != 1 {
			t.Errorf("the wipe fired (%d sessions) while the operator was tapping every 20s -- "+
				"an operator typing a passphrase would lose it mid-entry", trk.session)
		}
	})
}
