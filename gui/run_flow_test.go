package gui

import (
	"fmt"
	"testing"
	"testing/synctest"

	"seedhammer.com/gui/op"
	"seedhammer.com/gui/widget"
)

// Step 1.2's smoke tests: the harness itself, exercised against the two
// shapes every later task's tests will take -- a flow that returns and a
// flow that keeps drawing until it decides to stop.

// A flow that returns immediately must finish under mustFinish: Run never
// blocks waiting for a frame that is never drawn.
func TestRunHarnessFinishesOnImmediateReturn(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p := newDeadlinePlatform()
		mustFinish(t, p, func(ctx *Context, version string) {}, nil)
	})
}

// A flow that loops `for !ctx.Done` drawing a label per tick must produce
// frames whose text assertDrawn finds -- proving onDraw actually observes
// what the flow drew, not just that Run ticked.
//
// onDraw taps p on every frame. Without it, the SECOND ctx.Frame call already
// sleeps out to the wakeup this platform's first call scheduled (~idleTimeout
// ahead), the screensaver activates, and the flow parks inside its own second
// call -- which is step 1.3's scenario, not this one. Tapping from onDraw is
// the harness's own documented seam for exactly this (see runSession's doc).
func TestRunHarnessDrawsFlowFrames(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p := newDeadlinePlatform()
		const label = "HARNESS TICK"
		flow := func(ctx *Context, version string) {
			for n := 0; !ctx.Done; n++ {
				o, _ := widget.Label(&ctx.B, ctx.Styles.body, descriptorTheme.Text, label)
				ctx.Frame(o)
				if n >= 2 {
					ctx.Done = true
				}
			}
		}
		onDraw := func(o op.Op, text string) {
			p.tap()
		}
		drawn := mustFinish(t, p, flow, onDraw)
		assertDrawn(t, drawn, label)
	})
}

// Step 1.3: prove deadlinePlatform's deadline is actually honoured by
// observing Run's screensaver activate once a flow stops refreshing input
// past idleTimeout. If the platform were not driving the clock (the
// pre-Task-1 state, where testPlatform.AppendEvents ignores its deadline),
// idleTimeout would never be crossed and this test would park for an
// unrelated reason -- or never even get run.
func TestRunHarnessHonoursDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p := newDeadlinePlatform()
		// A flow that draws once and keeps re-drawing without ever supplying
		// new input. Nothing taps p, so the idle clock is never refreshed and
		// Run's saver must eventually take over.
		flow := func(ctx *Context, version string) {
			for !ctx.Done {
				o, _ := widget.Label(&ctx.B, ctx.Styles.body, descriptorTheme.Text, "PARKED")
				ctx.Frame(o)
			}
		}

		var contentDraws int
		var dirtiesAfterLastDraw int
		onDraw := func(o op.Op, text string) {
			contentDraws++
			// The content path's own Dirty call (inside runWithFlow's draw
			// closure) always precedes this observer call, so this records
			// "dirties as of the last frame that WAS observed".
			dirtiesAfterLastDraw = p.dirties
		}

		drawn, parked := runSession(t, p, flow, onDraw)
		// Use runSession, not mustFinish: a parked flow is the expected
		// outcome here -- the unarmed screensaver legitimately parks the flow
		// forever once it activates. Assert on parked explicitly, per the
		// harness's own doc: a test that ignores the second return value
		// passes vacuously.
		if !parked {
			t.Fatalf("expected the flow to park once the screensaver activated; got %d frames: %q", len(drawn), drawn)
		}
		if contentDraws == 0 {
			t.Fatalf("expected at least one content frame drawn before the saver activated")
		}
		// The discriminator, since a raw dirties count proves nothing on its
		// own (Dirty is also called by the content path): a saver frame is a
		// Dirty with NO following onDraw, because saver.State.Draw writes
		// straight to the platform, bypassing the op pipeline -- and hence
		// onDraw -- entirely. If the deadline were not honoured, idleTimeout
		// would never be crossed, the saver would never activate, and
		// p.dirties would never advance past the last content draw's own
		// Dirty call.
		if p.dirties <= dirtiesAfterLastDraw {
			t.Fatalf("screensaver never activated: no Dirty call after the last drawn content frame "+
				"(dirties=%d, after last content draw=%d) -- the deadline is not being honoured",
				p.dirties, dirtiesAfterLastDraw)
		}
	})
}

// ---------------------------------------------------------------------------
// Task 3 -- making ctx.Done survivable.
//
// Every flow below goes through boundedFlow, per step 3.2: an unwrapped flow
// is what lets the hoist-`wiping` mutant (3.3's row 4) spin forever, since a
// session whose own end is never recognised (wiping stuck true) keeps
// restarting past what its own body accounts for.
//
// There is no discard guard here (see run_flow.go's FrameCallback comment):
// the Critical found while writing these tests -- `ctx.Done = ctx.Done ||
// !yield(o)` discards a Done set from inside the very yield(o) call the wipe
// makes -- meant the wipe never persisted. The fix (an early return once Done
// is true, so yield is never called again) makes the guard's job structural
// rather than a downstream check: wiping implies Done implies no further
// yield implies no further content frame, full stop.
// ---------------------------------------------------------------------------

// sessionTracker counts SESSIONS (fresh *Context values), not ticks. Every
// Task 3 test drives a flow whose per-tick body is called by boundedFlow, and
// needs to know which session -- and which tick within it -- it is currently
// in, so it is factored out once rather than reimplemented three times.
type sessionTracker struct {
	last    *Context
	session int
	tick    int
}

// next reports the current (session, tick), advancing session and resetting
// tick whenever ctx is a NEW *Context -- which is exactly what the session
// loop hands the flow on every restart, per Task 3's own design (a FRESH
// Context, not a scrubbed one).
func (s *sessionTracker) next(ctx *Context) (session, tick int) {
	if ctx != s.last {
		s.session++
		s.last = ctx
		s.tick = 0
	}
	s.tick++
	return s.session, s.tick
}

// armWipe sets wipeNowHook to fire exactly once, then clear itself -- so a
// test that arms a second wipe later cannot accidentally see the first one's
// hook still installed.
func armWipe(t *testing.T) {
	t.Helper()
	wipeNowHook = func() bool {
		wipeNowHook = nil
		return true
	}
}

// sessionLabel draws "SESSION %d", the second-session-specific marker step
// 3.1 requires: a constant label such as "PARKED" is already in `drawn` from
// before the wipe, so the obvious assertion would false-PASS the
// break->return mutant -- the single most important mutant in this plan.
func sessionLabel(ctx *Context, session int) op.Op {
	o, _ := widget.Label(&ctx.B, ctx.Styles.body, descriptorTheme.Text, fmt.Sprintf("SESSION %d", session))
	return o
}

// TestRunWipeUnwindsAndRestartsTheFlow -- step 3.1. wipeNowHook fires on tick
// 3 of session 1; the flow must unwind (running its own defer) and the
// session loop must re-enter it with a FRESH Context, drawing "SESSION 2".
//
// Kills the `break`->`return` mutant (3.3 row 1): under that mutant the wipe
// exits runWithFlow entirely instead of restarting the flow, so session 2
// never happens and "SESSION 2" is never drawn.
//
// Kills the FrameCallback-revert mutant (3.3 row 2, the Critical): reverted
// to `ctx.Done = ctx.Done || !yield(o)`, the wipe's own `ctx.Done = true`
// (set from inside this very yield(o) call) is discarded, so ctx.Done never
// durably becomes true, session 2 never happens, and boundedFlow's own cap
// eventually panics -- this is the exact failure this test reported before
// the fix.
//
// The invariant assertion below (EXTRA FRAME never drawn) replaces the
// deleted discard-guard test: with the FrameCallback fix, that guard is
// unreachable dead code (wiping implies Done implies no further yield), so
// the property it was meant to provide is pinned here instead, where the
// mutation row that actually kills it lives.
func TestRunWipeUnwindsAndRestartsTheFlow(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p := newDeadlinePlatform()
		t.Cleanup(func() { wipeNowHook = nil })
		var trk sessionTracker
		deferRuns := 0
		const wipeAtTick = 3
		inner := boundedFlow(t, func(ctx *Context) bool {
			session, tick := trk.next(ctx)
			if session == 1 && tick == wipeAtTick {
				armWipe(t)
			}
			ctx.Frame(sessionLabel(ctx, session))
			if session == 1 && ctx.Done {
				// Fact 3: two real screens (SeedScreen.Confirm,
				// EngraveScreen.Engrave) call ctx.Frame ONE MORE TIME after
				// ctx.Done has already gone true, by fall-through. This
				// reproduces that shape directly, in the SAME session, right
				// after the wipe. The FrameCallback fix's early return must
				// make this reach nothing: no further content frame drawn.
				o, _ := widget.Label(&ctx.B, ctx.Styles.body, descriptorTheme.Text, "EXTRA FRAME")
				ctx.Frame(o)
				return false
			}
			// End the test cleanly once session 2 has drawn a few frames -- a
			// NORMAL flow return, not another wipe.
			if session == 2 && tick >= wipeAtTick {
				return false
			}
			return true
		})
		flow := func(ctx *Context, version string) {
			defer func() { deferRuns++ }()
			inner(ctx, version)
		}
		onDraw := func(o op.Op, text string) { p.tap() }
		drawn := mustFinish(t, p, flow, onDraw)

		if trk.session != 2 {
			t.Fatalf("expected exactly 2 sessions, got %d", trk.session)
		}
		assertDrawn(t, drawn, "SESSION 2")
		if drawnContains(drawn, "EXTRA FRAME") {
			t.Error("a frame drawn after ctx.Done went true reached the draw path -- " +
				"the FrameCallback fix's early return is not stopping yield from being called again")
		}
		if deferRuns != 2 {
			t.Errorf("expected the flow's own defer to run once per session (2), got %d -- "+
				"the unwind is not running the parked flow's defers", deferRuns)
		}
	})
}

// TestRunTwoWipesEachRestartCleanly -- kills the hoist-`wiping`-above-`for{}`
// mutant (3.3 row 4). Every session's transition here depends SOLELY on
// wipeNowHook, with no tick-based fallback until the third and final
// session -- so if `wiping` is not reset per session, session 2's own wipe
// still fires normally (ctx.Done is per-Context and unaffected by the hoist),
// but the SESSION LOOP's tail check (`if !wiping { return }`) never sees a
// false again: every session from then on, wiped or not, is treated as
// needing a restart, and the flow keeps being re-entered past what its own
// body accounts for -- until a session with no exit condition of its own
// spins in boundedFlow's cap, which is what actually stops it.
func TestRunTwoWipesEachRestartCleanly(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p := newDeadlinePlatform()
		t.Cleanup(func() { wipeNowHook = nil })
		var trk sessionTracker
		const wipeAtTick = 3
		flow := boundedFlow(t, func(ctx *Context) bool {
			session, tick := trk.next(ctx)
			if session < 3 && tick == wipeAtTick {
				armWipe(t)
			}
			ctx.Frame(sessionLabel(ctx, session))
			if session == 3 && tick >= wipeAtTick {
				return false
			}
			return true
		})
		onDraw := func(o op.Op, text string) { p.tap() }
		drawn := mustFinish(t, p, flow, onDraw)

		if trk.session != 3 {
			t.Fatalf("expected exactly 3 sessions (two wipes), got %d", trk.session)
		}
		assertDrawn(t, drawn, "SESSION 3")
	})
}
