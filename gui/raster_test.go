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
//
// THE FLOOR IS MEASURED, NOT WRITTEN DOWN (F-183, filed from S3). It used to be
// `const floor = 4000`, calibrated on the unload result screen: the body that
// shipped blank drew 2652 px and the fixed one 6688, so 4000 separated them.
// That was right for its one caller and wrong as a general instrument, which is
// what the generic name and message invited it to become. A title-only frame
// with THREE nav buttons draws 5482 px -- the third is StylePrimary and so
// filled -- and on any such screen the old floor passed a completely blank body.
// Latent rather than live, since the one caller is a two-button modal; latent is
// the state in which this class has repeatedly shipped.
//
// titleOnlyInk searches 1..3 nav buttons and returns the WORST, so the floor now
// tracks the chrome rather than a number measured once on one screen.
func assertFrameHasBody(t *testing.T, ink int, what string) {
	t.Helper()
	blank := titleOnlyInk(t)
	floor := blank + bodyInkMargin
	if ink < floor {
		t.Errorf("%s drew only %d ink pixels (floor %d, from a worst-case body-less "+
			"frame of %d) -- a screen with a title AND a body draws far more, so this "+
			"is near-blank whatever its text ops claim. Suspect a body too long to "+
			"lay out, or glyphs the face lacks.",
			what, ink, floor, blank)
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
