package gui

// F-106 diagnostic: does §10.2.4's timer start WITHOUT a touch?
//
// On the machine it does not. Armed, parked on Cut/Skip, no input: 4:15
// elapsed with no warning, no wipe and no screensaver. One deliberate touch on
// a blank area then produced the warning at EXACTLY 3:00 and the wipe at
// EXACTLY 3:30 -- so the schedule is sound and only its START is broken.
//
// The whole ./gui/ suite already covers the park-with-no-input case and PASSES
// (TestRunWarningThenWipe, and TestRunSealedPayloadReentryAfterWipe's
// "F idle-wipe nfc" drives the REAL uiFlow through a real unlock and observes
// the warning with no taps at all). So the defect is below the harness, in one
// of the two things every host test replaces:
//
//	1. THE CLOCK. Every existing test runs in a synctest bubble, where time is
//	   fake and advances only when every goroutine blocks.
//	2. THE EVENT LOOP. deadlinePlatform.AppendEvents is a time.Sleep. The SH2's
//	   is a select over a REUSED *time.Timer, a wakeup channel and a touch
//	   interrupt (cmd/controller/platform_sh2.go:369).
//
// This file removes both substitutions at once: real wall-clock time, and an
// AppendEvents whose structure is the SH2's line for line. If the warning does
// not arrive here either, F-106 is a gui/ or timer-reuse defect and needs no
// hardware. If it DOES arrive, the remaining difference is the panel itself --
// the FT6x36 delivering events while nobody is touching it, which refreshes
// a.idle.start forever through Run's `len(evts) > 0` term -- and that can only
// be settled on the machine.
//
// It costs 3.5 minutes of REAL time, so it is opt-in and never runs in the
// suite: SH2_REALCLOCK=1 go test ./gui/ -run TestIdleTimerUnderSH2ShapedEventLoop -v

import (
	"image"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"seedhammer.com/gui/op"
	"seedhammer.com/gui/widget"
)

// sh2ShapedPlatform replicates cmd/controller/platform_sh2.go's AppendEvents
// STRUCTURE, not its drivers: one *time.Timer reused across every call, a
// buffered wakeup channel, and a touch channel standing in for the pin
// interrupt. The reuse is the part worth modelling -- a timer that is left
// armed on a non-timer return path, or a Reset that does not re-arm after a
// fire, would show up here and nowhere else.
type sh2ShapedPlatform struct {
	*testPlatform
	timer *time.Timer
	touch chan struct{}

	// counters. ticks is every AppendEvents call; evtTicks is the subset that
	// returned at least one event, which is exactly Run's `len(evts) > 0`
	// refresh condition -- the one thing that can hold the idle clock at now
	// forever.
	ticks    int
	evtTicks int
	// longest deadline actually requested, so a run that never asks for the
	// 3-minute wait is distinguishable from one that asks and is not served.
	longest time.Duration
}

func newSH2ShapedPlatform() *sh2ShapedPlatform {
	p := newPlatform()
	p.display = sh2DisplaySize
	return &sh2ShapedPlatform{
		testPlatform: p,
		timer:        time.NewTimer(0),
		touch:        make(chan struct{}, 1),
	}
}

// AppendEvents is platform_sh2.go:369 with the device reads removed.
func (p *sh2ShapedPlatform) AppendEvents(deadline time.Time, evts []Event) []Event {
	p.ticks++
	if d := time.Until(deadline); d > p.longest {
		p.longest = d
	}
	// Don't starve touch input.
	select {
	case <-p.touch:
		p.evtTicks++
		return append(evts, p.touchEvent())
	default:
	}
	p.timer.Reset(time.Until(deadline))
	for {
		select {
		case <-p.timer.C:
			return evts
		case <-p.wakeups:
			return evts
		case <-p.touch:
			p.evtTicks++
			return append(evts, p.touchEvent())
		}
	}
}

func (p *sh2ShapedPlatform) touchEvent() Event {
	pos := image.Pt(sh2DisplaySize.X/2, sh2DisplaySize.Y/2)
	return PointerEvent{Pressed: true, Entered: true, Pos: pos}.Event()
}

// TestIdleTimerUnderSH2ShapedEventLoop parks an ARMED session with no input and
// waits, on the real clock, for the warning (3:00) and the wipe (3:30).
func TestIdleTimerUnderSH2ShapedEventLoop(t *testing.T) {
	if os.Getenv("SH2_REALCLOCK") == "" {
		t.Skip("real-clock diagnostic: costs ~3.5 min of wall time; set SH2_REALCLOCK=1 to run")
	}
	p := newSH2ShapedPlatform()
	var trk sessionTracker
	start := time.Now()
	var warnAt, wipeAt time.Time
	r := image.Rectangle{Max: sh2DisplaySize}
	// The wipe is timed from INSIDE the flow, not from the range body. Session 2
	// returns without drawing, so it never yields and the range body never sees
	// it -- measured: the first cut of this test reported sessions=2 and a zero
	// wipe time in the same breath.
	flow := boundedFlow(t, func(ctx *Context) bool {
		session, _ := trk.next(ctx)
		if ctx.wipe == nil {
			ctx.wipe = &wipeGuard{}
		}
		if session == 2 {
			if wipeAt.IsZero() {
				wipeAt = time.Now()
			}
			return false
		}
		o, _ := widget.Label(&ctx.B, ctx.Styles.body, descriptorTheme.Text, "CONTENT")
		ctx.Frame(o)
		return true
	})
	observe := func(o op.Op) {
		d := new(op.Drawer)
		if txt := d.ExtractText(r, o); warnAt.IsZero() && uiContains(txt, "erased in") {
			warnAt = time.Now()
			t.Logf("warning drawn at %v (ticks=%d evtTicks=%d)", time.Since(start).Round(time.Second), p.ticks, p.evtTicks)
		}
	}
	// The watchdog is the failure mode this test exists to catch: on the machine
	// nothing at all happened for 4:15. A wall-clock check in the range body
	// alone cannot catch that, because the body only runs per TICK and this run
	// reaches 3:00 in TWO of them -- the platform is blocked inside one
	// AppendEvents the whole time. So the watchdog delivers a WAKEUP, which
	// unblocks that select, produces a tick, and lets the loop exit cleanly.
	// A wakeup carries no event, so it cannot itself refresh the idle clock and
	// mask the very defect being measured.
	const cap = 5 * time.Minute
	var timedOut atomic.Bool
	wd := time.AfterFunc(cap, func() {
		timedOut.Store(true)
		p.Wakeup()
	})
	defer wd.Stop()
	for range runWithFlow(p, "test", flow, observe) {
		if timedOut.Load() {
			break
		}
	}
	t.Logf("elapsed=%v sessions=%d ticks=%d evtTicks=%d longestDeadline=%v",
		time.Since(start).Round(time.Second), trk.session, p.ticks, p.evtTicks, p.longest.Round(time.Second))
	if timedOut.Load() {
		t.Fatalf("F-106 REPRODUCED ON HOST: armed and idle for %v with no input and no warning "+
			"(ticks=%d, evtTicks=%d, longest deadline requested=%v). The defect is above the panel.",
			cap, p.ticks, p.evtTicks, p.longest.Round(time.Second))
	}
	if warnAt.IsZero() {
		t.Fatalf("the session ended without the warning ever being drawn (sessions=%d ticks=%d)", trk.session, p.ticks)
	}
	// Bounds, not equality: the real clock has jitter and the flow redraws.
	if d := warnAt.Sub(start); d < idleTimeout || d > idleTimeout+15*time.Second {
		t.Errorf("warning at %v, want ~%v after the last input", d.Round(time.Second), idleTimeout)
	}
	if wipeAt.IsZero() {
		t.Fatalf("the warning was drawn but the wipe never restarted the session (sessions=%d)", trk.session)
	}
	if d := wipeAt.Sub(start); d < idleTimeout+wipeWarningDelay || d > idleTimeout+wipeWarningDelay+15*time.Second {
		t.Errorf("wipe at %v, want ~%v after the last input", d.Round(time.Second), idleTimeout+wipeWarningDelay)
	}
}
