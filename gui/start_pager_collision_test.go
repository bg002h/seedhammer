package gui

import (
	"image"
	"testing"

	"seedhammer.com/gui/op"
	"seedhammer.com/gui/widget"
)

// inkColumns rasterises one frame and reports, for the horizontal band
// [y0, y1), which columns carry ink.
//
// Pixels rather than layout arithmetic, deliberately. Re-deriving the placement
// formula in the test would assert the formula against itself — both sides move
// together when it is wrong, which is the trap this repo has already been caught
// by. What an operator sees is ink on a screen, so that is what is measured.
func inkColumns(t *testing.T, dims image.Point, o op.Op, y0, y1 int) []bool {
	t.Helper()
	r := image.Rectangle{Max: dims}
	d := new(op.Drawer)
	dst := image.NewRGBA(r)
	mask := image.NewRGBA(r)
	d.Draw(dst, mask, o)
	bg := dst.RGBAAt(r.Min.X, r.Min.Y)
	cols := make([]bool, r.Max.X)
	for y := y0; y < y1 && y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			if dst.RGBAAt(x, y) != bg {
				cols[x] = true
			}
		}
	}
	return cols
}

// TestStartScreenPagerDoesNotCollideWithTheVersion is F-154's gate.
//
// The carousel pager and the version label share the bottom strip. The pager was
// centred on the FULL width while the version is anchored to the right edge, so
// every program added pushed the dot row further right until they overlapped.
// Found by the Wallet Policy journey's own capture, on the emulator's
// framebuffer: at ten programs two dots were drawn AROUND "Fi" and "rm",
// enclosing the letters.
//
// THE METHOD IS A CLEAR GUTTER. The label's left edge is computed from the
// label's OWN measurement — legitimate, because the label's placement is not
// what is under test; the pager's is. The assertion is then that the columns
// immediately left of that edge carry no ink. A colliding dot cannot avoid
// them: dots are contiguous and wider than the gutter, so one overlapping the
// text necessarily inks the run-up to it.
//
// (The obvious alternative — render with and without the version string and
// diff the columns — is INVALID here, and measurably so: the fix makes the
// pager's position depend on the label's width, so the two renders move the
// pager and the difference is noise. It reported a 195-column "collision"
// against a screen that has none.)
//
// Run at the REAL device size. The default test display is square and much
// narrower, where this layout genuinely runs out of room — gating there would
// pin a limitation instead of the defect.
func TestStartScreenPagerDoesNotCollideWithTheVersion(t *testing.T) {
	// THE PRODUCTION SHAPE OF THE STRING, not a bare version number.
	// run_flow.go:231 calls uiFlow with `versionText` — "Firmware: <v>\nHardware:
	// <hw>" — so StartScreen.Version is a TWO-LINE label roughly 171px wide, not
	// the ~24px "emu" it looks like from uiFlow's signature.
	//
	// This test passed "emu" at first and was decoration: it computed the
	// label's left edge at x=452, measured a pager ending at x=314, and reported
	// a comfortable pass while the emulator's own framebuffer showed two dots
	// drawn around "Fi" and "rm". A gate aimed at the wrong label cannot see the
	// collision it exists for.
	const version = "Firmware: emu\nHardware: emulator (UNLOCKED)"
	const gutter = 4

	p := newPlatform()
	p.display = sh2DisplaySize
	// A payload makes Sealed Payload visible, so lastNav is at its maximum and
	// this is the most dots the device can draw.
	p.payload = payloadReaderFor(t, "E")
	ctx := NewContext(p)
	dims := p.DisplaySize()

	var cols []bool
	ctx.FrameCallback = func(o op.Op) {
		if cols == nil {
			cols = inkColumns(t, dims, o, dims.Y-leadingSize, dims.Y)
		}
		ctx.Reset()
		ctx.Done = true
	}
	uiFlow(ctx, version)
	if cols == nil {
		t.Fatal("the start screen drew no frame")
	}

	_, versz := widget.Labelw(&ctx.B, ctx.Styles.debug, 200, descriptorTheme.Text, version)
	labelLeft := dims.X - versz.X - 4
	if labelLeft <= gutter {
		t.Fatalf("the version label fills the strip (left edge x=%d); this test cannot measure", labelLeft)
	}

	// Non-vacuous: there must BE a pager to collide with.
	inked := 0
	for x := 0; x < labelLeft-gutter; x++ {
		if cols[x] {
			inked++
		}
	}
	if inked == 0 {
		t.Fatal("no ink left of the version label — the pager drew nothing, so this test measures nothing")
	}

	for x := labelLeft - gutter; x < labelLeft && x < len(cols); x++ {
		if cols[x] {
			t.Errorf("ink at x=%d, inside the %dpx gutter before the version label at x=%d: "+
				"the carousel dots have grown into it. Adding a program did this — see F-154.",
				x, gutter, labelLeft)
			return
		}
	}
}
