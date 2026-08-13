package gui

import (
	"image"
	"iter"
	"testing"

	"seedhammer.com/gui/op"
	"seedhammer.com/sysw"
)

// F-151. Every uiContains assertion in this package proves a string was
// SUBMITTED for drawing, never that it was drawn: ExtractText walks the op tree,
// so a body the panel renders as nothing still "appears". That is how the unload
// notice shipped as a blank white screen with three passing assertions on its
// wording, and it was caught by a human looking at the machine.
//
// So: rasterise the frame and count ink. This is opt-in rather than folded into
// runUITouch because drawing 480x320 on every frame of every touch test is a
// cost only the tests that care should pay.

// runUITouchRaster is runUITouch plus an ink count for the last frame — the
// number of pixels that differ from the background.
func runUITouchRaster(ctx *Context, ui func()) (frame func() (string, bool), drawer func() *op.Drawer, ink func() int, stop func()) {
	var last *op.Drawer
	var lastInk int
	next, quit := iter.Pull(func(yield func(content string) bool) {
		ctx.FrameCallback = func(o op.Op) {
			r := image.Rectangle{Max: ctx.Platform.DisplaySize()}
			d := new(op.Drawer)
			content := d.ExtractText(r, o)

			dst := image.NewRGBA(r)
			mask := image.NewRGBA(r)
			d.Draw(dst, mask, o)
			lastInk = countInk(dst)

			last = d
			ctx.Reset()
			ctx.Done = ctx.Done || !yield(content)
		}
		ui()
	})
	return next, func() *op.Drawer { return last }, func() int { return lastInk }, quit
}

// countInk counts pixels differing from the frame's own top-left pixel, which is
// background on every screen this device draws. Comparing against a NAMED colour
// would break the moment a theme changed; comparing against the frame's own
// corner cannot.
func countInk(m *image.RGBA) int {
	b := m.Bounds()
	bg := m.RGBAAt(b.Min.X, b.Min.Y)
	n := 0
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if m.RGBAAt(x, y) != bg {
				n++
			}
		}
	}
	return n
}

// assertFrameHasBody fails when a frame carries less ink than a screen with a
// title and a body must.
func assertFrameHasBody(t *testing.T, ink int, what string) {
	t.Helper()
	// CALIBRATED against the real defect rather than guessed. Measured on the
	// unload result screen: the body that shipped blank drew 2652 px, the fixed
	// one 6688. A floor of 2000 -- my first guess -- passed the defect. 4000
	// separates them with room on both sides.
	const floor = 4000
	if ink < floor {
		t.Errorf("%s drew only %d ink pixels (floor %d) — a screen with a title AND "+
			"a body draws far more, so this is near-blank whatever its text ops "+
			"claim. Suspect a body too long to lay out, or glyphs the face lacks.",
			what, ink, floor)
	}
}

// The regression that started F-151: the unload notice must actually be DRAWN,
// not merely submitted. Proven below to catch the original defect.
func TestUnloadNoticeIsActuallyDrawn(t *testing.T) {
	p := newPlatform()
	p.display = sh2DisplaySize
	ctx := NewContext(p)
	ctx.sysw = &syswSession{}
	ctx.sysw.load(&sysw.Payload{Public: []string{"text:6869"}}, [32]byte{}, false, false, true, true)

	frame, drawer, ink, quit := runUITouchRaster(ctx, func() {
		syswUnloadFlow(ctx, &descriptorTheme)
	})
	defer quit()
	content, ok := pumpUntil(frame, "Unload", 64)
	if !ok {
		t.Fatalf("the unload confirm never appeared; got %q", content)
	}
	click(&ctx.Router, Down) // {BACK, UNLOAD}: UNLOAD is index 1
	tapNavSlot(t, ctx, drawer(), Button3)
	content, ok = pumpUntil(frame, "Unloaded", 64)
	if !ok {
		t.Fatalf("the result screen never appeared; got %q", content)
	}
	t.Logf("unload result screen ink = %d px", ink())
	assertFrameHasBody(t, ink(), "the unload result screen")
}
