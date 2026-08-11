package gui

import (
	"testing"
	"testing/synctest"
	"time"

	"seedhammer.com/bspline"
	"seedhammer.com/gui/op"
)

// TestIdleWindowIsNotDoubledByALateArmEdge is F-106's regression test.
//
// THE TRAP THIS EXISTS TO AVOID. gui/idle_realclock_diag_test.go already
// reproduces platform_sh2.go's timer structure line for line and lands the
// warning at a correct 3m0s -- so a passing host test proves NOTHING here
// unless it reproduces the shape that actually breaks: a loop PARKED across the
// deadline with an arm edge installed on the frame just before it parked.
//
// The mechanism, measured on hardware (b2b-idleprobe3, 256b38c): the guard is
// installed inside yield(), but ctx.Wakeup was already computed from the
// pre-edge clock and `armed` was sampled only AFTER AppendEvents returned. So
// the loop slept to the 3:00 deadline, processed the edge there, and row 2
// stamped a.idle.start = now at the exact instant the wipe should have fired.
// Warning at 6:00, wipe at 6:30. Deterministic 2x.
//
// This test installs the guard AFTER the first frame, exactly as
// unlockSecretSession does, then parks -- and measures when the warning is
// actually drawn.
//
// Mutation this pins: delete the pre-block syncArmed call in run_flow.go.
func TestIdleWindowIsNotDoubledByALateArmEdge(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p := newDeadlinePlatform()

		var armedAt, warnAt time.Time
		sessions := 0
		flow := func(ctx *Context, version string) {
			sessions++
			if sessions > 1 {
				// The wipe fired and restarted the session; end cleanly.
				ctx.Done = true
				return
			}
			// One frame with NO guard, so the idle clock is the session's own
			// and ctx.Wakeup is computed from it. This is what makes the edge
			// below LATE rather than coincident with the session start.
			ctx.Frame(op.Layer())

			// Install the guard exactly as unlockSecretSession does, then park.
			ctx.wipe = &wipeGuard{}
			armedAt = time.Now()
			for !ctx.Done {
				ctx.Frame(op.Layer())
			}
		}
		onDraw := func(o op.Op, text string) {
			if warnAt.IsZero() && uiContains(text, "decrypted seed material") {
				warnAt = time.Now()
			}
		}
		mustFinish(t, p, flow, onDraw)

		if armedAt.IsZero() {
			t.Fatal("INCONCLUSIVE: the guard was never installed")
		}
		if warnAt.IsZero() {
			t.Fatal("the §10.2.4 warning never appeared at all")
		}

		got := warnAt.Sub(armedAt)
		// The window is measured from the arm edge. Generous tolerance: the
		// harness ticks at a 10ms floor and the flow draws a frame per tick, so
		// the warning lands within a tick or two of the deadline. The defect
		// being pinned is a factor of TWO, so a tolerance of seconds cannot mask
		// it.
		const tol = 5 * time.Second
		if got < idleTimeout-tol || got > idleTimeout+tol {
			t.Errorf("the §10.2.4 warning appeared %v after the guard was installed, want ~%v.\n"+
				"2x (%v) means the arm edge was processed one wakeup LATE: the loop slept to the "+
				"idle deadline and row 2 restarted the window at the instant the wipe should have "+
				"fired (F-106).", got, idleTimeout, 2*idleTimeout)
		}
	})
}

// TestCutEndingDuringTheParkStartsAFreshWindow pins §10.2.4 row 2 for the edge
// NEITHER syncArmed call can see: a cut that finishes while the loop is inside
// AppendEvents.
//
// R0 round 0 corrected this test's original premise. It was written to pin the
// post-block syncArmed call, and it did not: between the post-block sample and
// the next iteration's pre-block sample there is no blocking call at all, so
// the pre-block call subsumes the post-block one up to one loop turnaround --
// never a wakeup. Deleting the post-block call leaves the whole ./gui/ suite
// green, measured.
//
// What actually carries row 2 for engrave-side edges is pl.Wakeup():
// engraveJob.Start's goroutine ends with `defer e.pl.Wakeup()`
// (gui/engraver.go:110) and cmd/controller/platform_sh2.go:384 returns
// early on it. Drop that wakeup and the arm edge is processed at whatever the
// next wakeup happens to be -- and when that wakeup IS the idle deadline, row 2
// restarts the window there. F-106's exact shape, recurring for cut ends.
//
// The original version of this test could not witness any of it: it never set
// p.engraver, so runEngraving failed at its first statement with "engraver
// unavailable", close(cutting) was a no-op on a dead job, and the measurement
// was a startup failure observed at the next wakeup. It passed by coincidence.
//
// Mutation this pins: delete `defer e.pl.Wakeup()` from engraveJob.Start.
func TestCutEndingDuringTheParkStartsAFreshWindow(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p := newDeadlinePlatform()
		// 10ms ticks would need ~27,000 frames to cross idleTimeout twice and
		// blow maxRunFrames. The floor is a field for exactly this trade.
		p.tickFloor = 2 * time.Second
		// Without this, testPlatform.Engraver returns "engraver unavailable"
		// and the job dies before it cuts anything. That was C1.
		p.engraver = newEngraver()

		// The cut ends on the ENGRAVE goroutine's own schedule, not on the
		// flow's. That is not a stylistic choice: while the loop is inside
		// AppendEvents the flow is suspended inside yield() and cannot run at
		// all, so a flow-driven `close(cutting)` can only fire once something
		// has ALREADY un-parked the loop -- which is the very thing under test.
		//
		// A REAL job: wipeGuard.armed() calls Status(), whose restart branch
		// spawns a goroutine and dereferences the platform. A hand-built
		// &engraveJob{} nil-panics there -- which this test found the hard way.
		const cutDuration = idleTimeout / 2
		var cutEndAt time.Time
		cutDone := make(chan struct{})
		job := newEngraverJob(p, func(yield func(bspline.Knot) bool) {
			time.Sleep(cutDuration)
			cutEndAt = time.Now()
			// close/receive is the happens-before that lets the assertions
			// below read cutEndAt without racing this goroutine.
			close(cutDone)
		}, p.EngraverParams().StepperConfig, suppressStalls)
		g := &wipeGuard{job: job}

		var warnAt time.Time
		sessions := 0
		flow := func(ctx *Context, version string) {
			sessions++
			if sessions > 1 {
				ctx.Done = true
				return
			}
			// armed() is TRUE for a job that has not started (only
			// engraveRunning and engraveStopping disarm), so this arms at t=0
			// and Start() below disarms on the next frame. The edge that
			// matters is the third one: disarmed -> armed at the cut's end,
			// while the loop is parked to the 3:00 idle deadline and the
			// screensaver has not yet activated.
			ctx.wipe = g
			ctx.Frame(op.Layer())
			job.Start()
			for !ctx.Done {
				ctx.Frame(op.Layer())
			}
		}
		onDraw := func(o op.Op, text string) {
			if warnAt.IsZero() && uiContains(text, "decrypted seed material") {
				warnAt = time.Now()
			}
		}
		mustFinish(t, p, flow, onDraw)

		// The measurement basis FIRST: a job that never ran cannot produce a
		// cut-end edge, and the timing assertion below would then be measuring
		// a startup failure. That was C1 -- the original test never set
		// p.engraver, so it measured engraveRunning -> engraveFailed and passed
		// by coincidence.
		select {
		case <-cutDone:
		default:
			t.Fatal("INCONCLUSIVE: the spline never ran to its end, so no cut-end edge occurred")
		}
		if st := job.Status(); st.State != engraveDone || st.Error != "" {
			t.Fatalf("INCONCLUSIVE: the job did not run to completion (state %v, err %q), "+
				"so nothing here is a cut END", st.State, st.Error)
		}
		if warnAt.IsZero() {
			t.Fatal("the §10.2.4 warning never appeared at all")
		}

		got := warnAt.Sub(cutEndAt)
		const tol = 5 * time.Second
		if got < idleTimeout-tol || got > idleTimeout+tol {
			t.Errorf("the warning appeared %v after the cut finished, want ~%v -- "+
				"§10.2.4 row 2 says a finished cut starts a FRESH window, and %v means the "+
				"edge was processed one wakeup late: the loop slept to the idle deadline and "+
				"row 2 restarted the window at the instant the wipe should have fired",
				got, idleTimeout, 2*idleTimeout)
		}
	})
}

// TestCutEndingAfterTheDeadlineStartsAFreshWindow is the engrave-side twin of
// F-106: the cut ends AFTER the loop has already gone idle, so the arm edge
// lands on a tick where a.idle.active is already true.
//
// It exists to pin the one-character trap R0 round 0 named. The post-block
// `armed := syncArmed(now)` is one edit away from the baseline's own
// `armed := ctx.wipe.armed()`, and that edit is not equivalent: a fresh sample
// with no stamp enters the warning branch on a tick whose a.idle.start is still
// the PRE-edge clock, so the warning draws at the cut's end instead of three
// minutes after it. Measured: with that substitution the whole ./gui/ suite is
// otherwise green, because in every other scenario the suite reaches, the
// pre-block call has already stamped.
//
// Mutations this pins: post-block `syncArmed(now)` -> `ctx.wipe.armed()`; and
// deleting `defer e.pl.Wakeup()` is NOT pinned here (the saver's own poll
// un-parks this one) -- that is TestCutEndingDuringTheParkStartsAFreshWindow.
func TestCutEndingAfterTheDeadlineStartsAFreshWindow(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p := newDeadlinePlatform()
		p.tickFloor = 2 * time.Second
		p.engraver = newEngraver()

		// PAST the idle deadline, so the loop has already activated the
		// screensaver when the cut ends. 10s is far outside the 5s tolerance
		// below, so the two clocks can never be confused for one another.
		const cutDuration = idleTimeout + 10*time.Second
		var cutEndAt time.Time
		cutDone := make(chan struct{})
		job := newEngraverJob(p, func(yield func(bspline.Knot) bool) {
			time.Sleep(cutDuration)
			cutEndAt = time.Now()
			close(cutDone)
		}, p.EngraverParams().StepperConfig, suppressStalls)

		var warnAt time.Time
		sessions := 0
		flow := func(ctx *Context, version string) {
			sessions++
			if sessions > 1 {
				ctx.Done = true
				return
			}
			ctx.wipe = &wipeGuard{job: job}
			ctx.Frame(op.Layer())
			job.Start()
			for !ctx.Done {
				ctx.Frame(op.Layer())
			}
		}
		onDraw := func(o op.Op, text string) {
			if warnAt.IsZero() && uiContains(text, "decrypted seed material") {
				warnAt = time.Now()
			}
		}
		mustFinish(t, p, flow, onDraw)

		select {
		case <-cutDone:
		default:
			t.Fatal("INCONCLUSIVE: the spline never ran to its end, so no cut-end edge occurred")
		}
		if st := job.Status(); st.State != engraveDone || st.Error != "" {
			t.Fatalf("INCONCLUSIVE: the job did not run to completion (state %v, err %q), "+
				"so nothing here is a cut END", st.State, st.Error)
		}
		if warnAt.IsZero() {
			t.Fatal("the §10.2.4 warning never appeared at all")
		}

		got := warnAt.Sub(cutEndAt)
		const tol = 5 * time.Second
		if got < idleTimeout-tol || got > idleTimeout+tol {
			t.Errorf("the warning appeared %v after the cut finished, want ~%v -- "+
				"§10.2.4 row 2 says a finished cut starts a FRESH window even when the loop "+
				"was ALREADY idle when it finished; a value near zero means the arm edge was "+
				"read without restarting the clock, so the warning drew on the pre-edge "+
				"deadline and the wipe would follow %v later", got, idleTimeout, wipeWarningDelay)
		}
	})
}
