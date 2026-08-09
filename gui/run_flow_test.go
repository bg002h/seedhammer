package gui

import (
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
