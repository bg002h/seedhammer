package gui

import (
	"fmt"
	"image"
	"testing"
	"testing/synctest"
	"time"

	"seedhammer.com/gui/op"
	"seedhammer.com/gui/widget"
	"seedhammer.com/seal"
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

// ---------------------------------------------------------------------------
// Task 4 -- §10.2.4's timer, warning and wipe. Every flow below goes through
// boundedFlow, per the same rule Task 3's tests follow.
// ---------------------------------------------------------------------------

// armedIdleFlow is the shared shape for "armed, walked away, no input"
// scenarios: it installs a bare *wipeGuard (job nil, so armed() reads true
// unconditionally) on the first tick of session 1, draws one content frame,
// then leaves Run's own idle/warning/wipe machinery to run unobserved. It
// stops the test as soon as session 2 begins -- the observable proof the
// wipe fired and the session restarted -- rather than looping wipes forever.
func armedIdleFlow(t *testing.T, trk *sessionTracker) func(*Context, string) {
	t.Helper()
	return boundedFlow(t, func(ctx *Context) bool {
		session, _ := trk.next(ctx)
		if ctx.wipe == nil {
			ctx.wipe = &wipeGuard{}
		}
		if session == 2 {
			return false
		}
		o, _ := widget.Label(&ctx.B, ctx.Styles.body, descriptorTheme.Text, "CONTENT")
		ctx.Frame(o)
		return true
	})
}

// TestRunWarningThenWipe -- step 4.1 "warning + wipe": armed, no input -> the
// warning is drawn, and the session restarts (the wipe fired).
//
// Kills 4.3's `armed := ctx.wipe.armed()` -> `armed := false` row: under that
// mutant the saver runs instead of the warning, the flow parks under it
// forever, and mustFinish reports the cap instead of finishing.
//
// Kills 4.3's `if armed {` -> `if false {` row the same way: forced into the
// saver branch regardless of armed, so it never wipes and never finishes.
func TestRunWarningThenWipe(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p := newDeadlinePlatform()
		var trk sessionTracker
		var sawWarning bool
		onDraw := func(o op.Op, text string) {
			if uiContains(text, "erased in") {
				sawWarning = true
			}
		}
		drawn := mustFinish(t, p, armedIdleFlow(t, &trk), onDraw)
		if trk.session != 2 {
			t.Fatalf("expected the wipe to restart the session (session 2), got %d sessions; drawn=%q", trk.session, drawn)
		}
		if !sawWarning {
			t.Fatal("expected the warning to be drawn before the wipe fired")
		}
	})
}

// TestRunWarningCountdownIsReal -- step 4.1 "the countdown is real". A
// presence-only assertion would let a frozen or negative counter pass, so
// this pins the exact text at the first warning frame and at a later one.
//
// Kills 4.3's `const wipeWarningDelay = 30 * time.Second` -> `= 0` row: with
// no delay, the very first idle tick already satisfies `now.Sub(wipeAt) >=
// 0`, so the wipe fires before any warning frame is ever drawn and texts is
// empty.
func TestRunWarningCountdownIsReal(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p := newDeadlinePlatform()
		var trk sessionTracker
		var texts []string
		onDraw := func(o op.Op, text string) {
			if uiContains(text, "WIPING SECRET DATA") {
				texts = append(texts, text)
			}
		}
		mustFinish(t, p, armedIdleFlow(t, &trk), onDraw)
		if len(texts) == 0 {
			t.Fatal("no warning frames drawn")
		}
		if !uiContains(texts[0], "erased in 30 seconds") {
			t.Errorf("first warning frame = %q, want it to contain %q", texts[0], "erased in 30 seconds")
		}
		found15 := false
		for _, txt := range texts {
			if uiContains(txt, "erased in 15 seconds") {
				found15 = true
				break
			}
		}
		if !found15 {
			t.Errorf("no later warning frame contains %q; got %d warning frames: %q", "erased in 15 seconds", len(texts), texts)
		}
	})
}

// TestRunTapDuringWarningResetsAndReturnsContent -- step 4.1 "tap during the
// warning". p.tap() is called from inside onDraw, the only same-goroutine
// injection point: Run blocks the test goroutine for the whole warning
// sequence (it happens inside the flow's own blocked ctx.Frame call), and the
// parked flow cannot tap for itself.
func TestRunTapDuringWarningResetsAndReturnsContent(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p := newDeadlinePlatform()
		var trk sessionTracker
		tapped := false
		var warningSeen, postTapDraw bool
		onDraw := func(o op.Op, text string) {
			if uiContains(text, "erased in") {
				warningSeen = true
				if !tapped {
					tapped = true
					p.tap()
				}
			}
			if uiContains(text, "CONTENT AGAIN") {
				postTapDraw = true
			}
		}
		flow := boundedFlow(t, func(ctx *Context) bool {
			_, tick := trk.next(ctx)
			if ctx.wipe == nil {
				ctx.wipe = &wipeGuard{}
			}
			if tick > 1 {
				o, _ := widget.Label(&ctx.B, ctx.Styles.body, descriptorTheme.Text, "CONTENT AGAIN")
				ctx.Frame(o)
				return false
			}
			o, _ := widget.Label(&ctx.B, ctx.Styles.body, descriptorTheme.Text, "CONTENT")
			ctx.Frame(o)
			return true
		})
		mustFinish(t, p, flow, onDraw)
		if !warningSeen {
			t.Fatal("the warning never appeared before the tap -- premise broken")
		}
		if trk.session != 1 {
			t.Fatalf("expected no wipe (the tap should cancel it), got %d sessions", trk.session)
		}
		if !postTapDraw {
			t.Fatal("the flow never regained control and drew again after the tap during the warning -- " +
				"content did not return")
		}
	})
}

// TestRunNotArmedNeverWarns -- step 4.1 "not armed": no warning ever, saver
// behaviour unchanged. ctx.wipe stays nil throughout, exactly as it is
// outside a secret session.
//
// Uses runSession, not mustFinish, and asserts parked == true explicitly: the
// unarmed screensaver legitimately parks the flow forever once it activates
// (gui/run_harness_test.go's own documented behaviour), so a park here is the
// expected outcome, not a failure.
//
// Kills 4.3's `armed := ctx.wipe.armed()` -> `armed := true` row. Both the
// mutated and unmutated code eventually report parked == true here (an
// armed-forced flow just cycles wipe after wipe until runSession's own tick
// cap trips), so parked alone does not discriminate the mutant -- the warning
// assertion does: under the real code armed() reads false because ctx.wipe is
// nil, so no warning is ever drawn; under the mutant it is drawn.
func TestRunNotArmedNeverWarns(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p := newDeadlinePlatform()
		var warned bool
		onDraw := func(o op.Op, text string) {
			if uiContains(text, "WIPING SECRET DATA") || uiContains(text, "erased in") {
				warned = true
			}
		}
		flow := boundedFlow(t, func(ctx *Context) bool {
			// ctx.wipe deliberately left nil: no secret session is open, so
			// armed() must read false and the ordinary screensaver applies.
			o, _ := widget.Label(&ctx.B, ctx.Styles.body, descriptorTheme.Text, "PARKED")
			ctx.Frame(o)
			return true
		})
		drawn, parked := runSession(t, p, flow, onDraw)
		if !parked {
			t.Fatalf("expected the unarmed flow to park under the ordinary screensaver forever; "+
				"got %d frames: %q", len(drawn), drawn)
		}
		if warned {
			t.Error("a warning was drawn while unarmed -- armed() must be false when ctx.wipe is nil")
		}
	})
}

// fakeRunningJob returns an *engraveJob that reports engraveRunning without
// spawning a real engraving goroutine.
//
// engraveJob.Status() has a "restart if requested" branch: if State is
// engraveRunning and e.errs is nil, it calls e.Start(), which spawns a
// goroutine that immediately calls e.pl.Wakeup() on a nil Platform -- a
// panic. Start() no-ops when e.errs != nil ("job is already running"), so a
// non-nil, never-signalled channel keeps Status() side-effect-free.
func fakeRunningJob() *engraveJob {
	return &engraveJob{
		status: engraveStatus{State: engraveRunning},
		errs:   make(chan error, 1),
	}
}

// TestRunPostCutWindowRestartsFromCutEnd -- step 4.1 "post-cut": a job
// running past idleTimeout, then ending: the window restarts from cut end,
// and a tap on the plate-done screen reaches the flow.
//
// The job starts "running" (armed() false), so the ordinary screensaver
// activates once idle -- proven by p.onDirty, the saver's only visibility
// (see run_harness_test.go). Once the saver has drawn a few frames -- well
// past idleTimeout -- the job flips to done, simulating the cut finishing
// while the operator is away.
//
// Control returns to the flow TWICE, and the two returns must not be
// conflated: the first happens on the very cut-end tick itself (armed's
// false->true edge clears a.idle.active on the same tick, per run_flow.go's
// own comment), well before any new warning exists to tap away -- a flow
// that stopped there would never see a second, POST-cut warning at all. So
// this flow keeps drawing after that first return, letting Run go idle a
// second time, and only stops once a frame drawn AFTER the tap is observed.
//
// Kills 4.3's `a.idle.start = now // row 2: fresh window at cut end` deletion
// row: without that reset, idle.start still reflects a point from before the
// job ever started running, so the moment armed flips true, idleWakeup and
// wipeAt are already both in the past and the wipe fires INSTANTLY, with no
// warning frame ever drawn -- caught by the warningSeen check below.
func TestRunPostCutWindowRestartsFromCutEnd(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p := newDeadlinePlatform()
		job := fakeRunningJob()
		var flippedDone bool
		p.onDirty = func(n int) {
			// saver.State.Draw makes TWO Dirty calls per frame (see
			// run_harness_test.go's own doc on p.dirties), each frame
			// throttled to minFrameTime (40ms) -- so this must wait past
			// wipeWarningDelay's 30s of SAVER time, i.e. past roughly
			// 30s/40ms*2 = 1500 dirty calls, before flipping the job to
			// done: the mutation row this test exists to kill leaves
			// a.idle.start stale, computing wipeAt from the ORIGINAL idle
			// crossing. Flipping too soon would still land wipeAt in the
			// future under the mutant too, and the test would false-PASS.
			// 5000 gives a large margin (~100s of saver time).
			if !flippedDone && n > 5000 {
				job.status.State = engraveDone
				flippedDone = true
			}
		}
		var trk sessionTracker
		tapped := false
		var warningSeen, postTapDraw bool
		onDraw := func(o op.Op, text string) {
			if uiContains(text, "erased in") {
				warningSeen = true
				if !tapped {
					tapped = true
					p.tap()
				}
			}
			if tapped && !postTapDraw && uiContains(text, "CONTENT AGAIN") {
				postTapDraw = true
				// Queue a second tap so Run does not have a chance to go
				// idle again -- and possibly warn or wipe a second time --
				// before this ctx.Frame call returns control to the flow,
				// which is what actually stops the loop below.
				p.tap()
			}
		}
		flow := boundedFlow(t, func(ctx *Context) bool {
			trk.next(ctx)
			if ctx.wipe == nil {
				ctx.wipe = &wipeGuard{job: job}
			}
			if postTapDraw {
				return false
			}
			label := "CONTENT"
			if flippedDone {
				label = "CONTENT AGAIN"
			}
			o, _ := widget.Label(&ctx.B, ctx.Styles.body, descriptorTheme.Text, label)
			ctx.Frame(o)
			return true
		})
		mustFinish(t, p, flow, onDraw)
		if !flippedDone {
			t.Fatal("never observed the saver run long enough to flip the job to done -- premise broken")
		}
		if !warningSeen {
			t.Fatal("no warning shown after the cut finished -- the window did not restart from cut end")
		}
		if trk.session != 1 {
			t.Fatalf("expected no wipe (the tap on the post-cut warning should cancel it), got %d sessions", trk.session)
		}
		if !postTapDraw {
			t.Fatal("the flow never regained control after the tap on the post-cut warning")
		}
	})
}

// TestRunWarningBufferDoesNotGrow -- A-C1 restored, the phase's most
// expensive finding. warnBufHook is the only seam that can observe which
// buffer the warning went into, or that it is not growing: op.Buffer's
// fields are unexported and a.warnBuf is a closure local.
//
// Kills 4.3's `a.warnBuf.Reset()` deletion row: without the reset, args grows
// by a roughly fixed increment every warning tick, so the last sample vastly
// exceeds the first.
//
// Kills 4.3's `&a.warnBuf` -> `&ctx.B` row (A-C1 itself): if the warning
// builds into ctx.B instead, warnBufHook -- which only ever reads a.warnBuf
// -- reports (0, 0) on every call.
func TestRunWarningBufferDoesNotGrow(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p := newDeadlinePlatform()
		t.Cleanup(func() { warnBufHook = nil })
		var sizes [][2]int
		warnBufHook = func(args, refs int) {
			sizes = append(sizes, [2]int{args, refs})
		}
		var trk sessionTracker
		mustFinish(t, p, armedIdleFlow(t, &trk), nil)

		if len(sizes) < 2 {
			t.Fatalf("expected multiple warning frames' worth of buffer samples, got %d", len(sizes))
		}
		first := sizes[0]
		if first[0] == 0 && first[1] == 0 {
			t.Fatal("warnBufHook reported (0, 0) on the first warning frame -- the warning never built " +
				"into a.warnBuf (building into ctx.B instead?)")
		}
		for i, sz := range sizes {
			if sz[0] > first[0]*3+8 || sz[1] > first[1]*3+8 {
				t.Errorf("warning buffer grew across frames: frame %d = %v, frame 0 = %v -- "+
					"a.warnBuf.Reset() does not appear to be running", i, sz, first)
			}
		}
	})
}

// TestWipeWarningOpClampsNegativeRemaining -- kills 4.3's `if secs < 0 {` ->
// `if false {` row. Unreachable from Run itself: wipeAt.Sub(now) is only ever
// computed after `now.Sub(wipeAt) >= 0` has already been ruled out, so remaining
// is never negative in practice. A direct unit call is the only way to reach it.
func TestWipeWarningOpClampsNegativeRemaining(t *testing.T) {
	var buf op.Buffer
	o := wipeWarningOp(&buf, NewStyles(), &descriptorTheme, sh2DisplaySize, -5*time.Second)
	d := new(op.Drawer)
	text := d.ExtractText(image.Rectangle{Max: sh2DisplaySize}, o)
	if !uiContains(text, "erased in 0 seconds") {
		t.Errorf("wipeWarningOp with a negative remaining drew %q, want it to contain %q",
			text, "erased in 0 seconds")
	}
}

// ---------------------------------------------------------------------------
// Task 5 -- F-93: the screensaver must not park a derivation.
// ---------------------------------------------------------------------------

// TestRunKeepAwakeDuringDerivationDoesNotParkUnderTheScreensaver -- step 5.1.
// Drives a REAL derivation (newDeriver is `var newDeriver = seal.NewDeriver`,
// a concrete *seal.Deriver -- there is no fake to swap in) past idleTimeout
// and asserts it still completes.
//
// p.tickFloor = 1 * time.Second, per the plan's own arithmetic: at the
// default 10ms floor, crossing idleTimeout (3 min) takes ~18,000 ticks =
// 9,000,000 iterations -- 4.5x seal.MaxIterations. At a 1s floor it is 180
// ticks = 90,000 iterations. 100,000 iterations here (200 ticks, 200s of
// bubble time) clears that crossing with margin while staying legal and
// fast.
//
// Without the floor this test would be a false PASS: unlockDerive calls
// ctx.WakeupAt(time.Now()) before every ctx.Frame, so every deadline is
// already expired, time.Until is <= 0, nothing durably blocks, and a
// synctest bubble's clock never advances -- the saver could never fire even
// under the `ctx.keepAwake` -> `false` mutant, and the mutant would pass too.
//
// mustFinish's own maxRunFrames cap (via runSession) is what converts the
// `ctx.keepAwake` -> `false` mutant (4.3's row, attributed to this step) from
// a hang into a kill: under that mutant the screensaver activates and its
// `continue` branch never returns control to unlockDerive's blocked
// ctx.Frame call, so mustFinish reports "Run exceeded 100000 ticks" instead
// of the derivation ever completing. No boundedFlow wrapper is needed here --
// unlike Task 3's discard-guard mutant, the screensaver's `continue` still
// calls the session's own top-of-loop yield() every tick (that is how the
// saver's Dirty calls are throttled at all), so runSession's tick counter
// keeps advancing and the cap trips on its own.
func TestRunKeepAwakeDuringDerivationDoesNotParkUnderTheScreensaver(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p := newDeadlinePlatform()
		p.tickFloor = 1 * time.Second
		var derivedKey []byte
		var derivedOK bool
		flow := func(ctx *Context, version string) {
			h := seal.Header{Iterations: 100_000}
			pass := []byte("does not matter, this test never checks the key value")
			derivedKey, derivedOK = unlockDerive(ctx, &descriptorTheme, h, pass)
			ctx.Done = true
		}
		mustFinish(t, p, flow, nil)
		if !derivedOK {
			t.Fatal("unlockDerive returned ok=false -- cancelled or failed to complete")
		}
		if len(derivedKey) != seal.KeyLen {
			t.Errorf("derived key length = %d, want %d", len(derivedKey), seal.KeyLen)
		}
	})
}

// TestRunKeepAwakeCannotPostponeAnArmedWipe -- step 5.3: `&& !armed` is
// normative, not caution. An armed session that calls KeepAwake every tick
// (exactly what a derivation does) must still wipe on schedule -- KeepAwake
// holds off the screensaver and nothing else.
//
// Kills 4.3's `(ctx.keepAwake && !armed)` -> `(ctx.keepAwake)` row: under
// that mutant every tick's KeepAwake call refreshes a.idle.start even while
// armed, so idleWakeup keeps sliding into the future and the session never
// restarts -- trk.session stays 1 until boundedFlow's cap panics.
func TestRunKeepAwakeCannotPostponeAnArmedWipe(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p := newDeadlinePlatform()
		var trk sessionTracker
		flow := boundedFlow(t, func(ctx *Context) bool {
			session, _ := trk.next(ctx)
			if ctx.wipe == nil {
				ctx.wipe = &wipeGuard{}
			}
			if session == 2 {
				return false
			}
			// Called every tick, exactly as unlockDerive calls it on every
			// slice -- BEFORE ctx.Frame, since Run reads and clears the flag
			// from inside that call.
			ctx.KeepAwake()
			o, _ := widget.Label(&ctx.B, ctx.Styles.body, descriptorTheme.Text, "CONTENT")
			ctx.Frame(o)
			return true
		})
		drawn := mustFinish(t, p, flow, nil)
		if trk.session != 2 {
			t.Fatalf("expected an armed session calling KeepAwake to still wipe on schedule "+
				"(session 2), got %d sessions; drawn=%q", trk.session, drawn)
		}
	})
}
