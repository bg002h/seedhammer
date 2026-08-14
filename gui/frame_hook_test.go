package gui

import (
	"image"
	"testing"
	"testing/synctest"

	"seedhammer.com/gui/op"
	"seedhammer.com/gui/widget"
)

// frameAwarePlatform is a deadlinePlatform that also accepts drawn frames.
//
// It EXTRACTS in the callback and keeps the string, never the op -- which is
// the contract frame_hook.go states and the one thing an implementation can get
// wrong silently. Retaining the op would read whatever the next frame built
// over the same buffer, and the symptom would be "the walk asserted on the
// wrong screen", not a crash.
type frameAwarePlatform struct {
	*deadlinePlatform
	frames []string
}

func (p *frameAwarePlatform) Frame(content op.Op) {
	d := new(op.Drawer)
	p.frames = append(p.frames, d.ExtractText(image.Rectangle{Max: sh2DisplaySize}, content))
}

// newFrameAwarePlatform is the pair a test drives: the wrapper Run is given,
// and the concrete platform the test still needs for tap().
func newFrameAwarePlatform() *frameAwarePlatform {
	return &frameAwarePlatform{deadlinePlatform: newDeadlinePlatform()}
}

// TestFrameHookSeesEveryDrawnFrameInOrder pins the seam cmd/emu's window.shScreen
// hangs on: a Platform that implements FrameAware is handed each frame's content
// after it is drawn, once, in draw order.
//
// ORDER AND MULTIPLICITY ARE THE ASSERTION, not merely presence. A walk reads
// the screen to decide whether the tap it just sent landed, so a hook that
// coalesced two frames, or delivered them late, would report the PREVIOUS
// screen as the current one -- and a walk that asserts on the previous screen
// passes for four steps in a row while sitting still. That is the exact failure
// this hook exists to make impossible, observed on this cycle's first walk
// attempt.
//
// Each frame carries a distinct label for that reason: a test that drew the
// same string three times could not tell a hook that fired three times from one
// that fired once and was asked three times.
func TestFrameHookSeesEveryDrawnFrameInOrder(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p := newFrameAwarePlatform()
		// No spaces: ExtractText collects the runes of drawn glyphs, and a
		// space inks nothing, so "FRAME ZERO" would arrive as "FRAMEZERO"
		// anyway. Spelling it without one keeps the expectation literal.
		labels := []string{"FRAMEZERO", "FRAMEONE", "FRAMETWO"}
		flow := func(ctx *Context, version string) {
			for n := 0; !ctx.Done; n++ {
				if n >= len(labels) {
					ctx.Done = true
					return
				}
				o, _ := widget.Label(&ctx.B, ctx.Styles.body, descriptorTheme.Text, labels[n])
				ctx.Frame(o)
			}
		}
		// Tap on every frame, for the reason TestRunHarnessDrawsFlowFrames
		// does: without new input the second frame sleeps out to the wakeup the
		// first scheduled, the screensaver takes over and the flow parks under
		// it -- which would fail this test for a reason that has nothing to do
		// with the hook.
		onDraw := func(o op.Op, text string) { p.tap() }

		drawn := mustFinish(t, p, flow, onDraw)

		if len(p.frames) != len(labels) {
			t.Fatalf("FrameAware.Frame called %d times, want %d -- drawn=%q hook=%q",
				len(p.frames), len(labels), drawn, p.frames)
		}
		for i, want := range labels {
			if p.frames[i] != want {
				t.Errorf("frame %d reached the hook as %q, want %q -- the hook is not seeing "+
					"the frames in the order they were drawn", i, p.frames[i], want)
			}
		}
	})
}

// TestFrameHookSeesRunsOwnFramesToo is the half that stops the hook from being
// hung on the FLOW instead of on the SCREEN.
//
// Run draws frames the flow never composed: §10.2.4's countdown warning is
// built from Run's own buffer (run_flow.go's warnBuf) and pushed through the
// same draw closure. A hook installed at ctx.Frame -- the obvious wrong place,
// and the one a reader reaches for first -- would see every flow frame and miss
// that warning entirely. An emulator walk would then read the screen as
// whatever was underneath while the machine was counting down to erasing a
// seed.
//
// The assertion is EQUALITY with run_harness_test.go's onDraw, which is
// documented as everything drawn, content frames and Run's own warning alike.
// Equality rather than containment: a hook that fired twice per frame would
// satisfy "saw the warning" and still be wrong.
func TestFrameHookSeesRunsOwnFramesToo(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p := newFrameAwarePlatform()
		var trk sessionTracker

		var observed []string
		var sawWarning bool
		onDraw := func(o op.Op, text string) {
			observed = append(observed, text)
			if uiContains(text, "erased in") {
				sawWarning = true
			}
		}
		mustFinish(t, p, armedIdleFlow(t, &trk), onDraw)

		// Positive control. Without it this test would pass vacuously on a
		// session that drew no warning at all -- which is exactly what the
		// mutants armedIdleFlow exists to catch look like.
		if !sawWarning {
			t.Fatal("no warning frame was drawn, so this test proves nothing about the hook -- " +
				"the armed-idle session did not reach §10.2.4's countdown")
		}
		if len(p.frames) != len(observed) {
			t.Fatalf("the hook saw %d frames, the draw observer saw %d -- one of them is not "+
				"on the screen's path\nhook=%q\nobserver=%q", len(p.frames), len(observed), p.frames, observed)
		}
		for i := range observed {
			if p.frames[i] != observed[i] {
				t.Errorf("frame %d: hook saw %q, draw observer saw %q", i, p.frames[i], observed[i])
			}
		}
	})
}
