package gui

import (
	"testing"
	"testing/synctest"

	"seedhammer.com/gui/op"
	"seedhammer.com/gui/widget"
)

// The wipe ABANDONS its Context, so the buffer holding the last frame drawn
// before the wipe is never reused and never overwritten. Reset (which
// Context.Frame already ran) truncates args and clears only refs, and op.Glyph
// encodes every rendered rune into args -- so without an explicit Scrub the
// rendered seed comes back verbatim from the backing array.
//
// Mutation this pins: delete `ctx.B.Scrub()` from run_flow.go's session tail.
func TestWipeScrubsTheAbandonedFrameBuffer(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p := newDeadlinePlatform()

		var first *Context
		session := 0
		wipeNowHook = func() bool { return session == 1 }
		defer func() { wipeNowHook = nil }()

		flow := boundedFlow(t, func(ctx *Context) bool {
			if first == nil {
				session++
				first = ctx
			}
			// Render seed-shaped text so args carries glyph words.
			o, _ := widget.Labelw(&ctx.B, ctx.Styles.body, sh2DisplaySize.X-16,
				descriptorTheme.Text, "abandon ability able about above absent")
			ctx.Frame(o)
			return true
		})

		drawn, parked := runSession(t, p, func(ctx *Context, v string) {
			if first != nil && ctx != first {
				return // second session: return immediately so the run ends
			}
			flow(ctx, v)
		}, nil)
		_ = drawn
		_ = parked

		if first == nil {
			t.Fatal("flow never ran")
		}
		args, refs := first.B.Residue()
		if args != 0 || refs != 0 {
			t.Errorf("the abandoned session's buffer still holds residue after the wipe: "+
				"%d non-zero args, %d non-nil refs -- ctx.B.Scrub() did not run", args, refs)
		}
		var _ op.Op
	})
}
