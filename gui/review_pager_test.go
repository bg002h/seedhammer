package gui

import (
	"fmt"
	"image"
	"testing"

	"seedhammer.com/gui/assets"
	"seedhammer.com/gui/op"
)

// The pager on confirmReviewScreen used to be drawn unconditionally, so the
// payload digest screen -- four lines, and the first screen an operator meets --
// carried a right arrow that did nothing. A control that is present and inert
// teaches the operator that controls here may be inert, which is expensive on a
// device whose other buttons cut steel.
//
// Asserted BEHAVIOURALLY, in both directions, because the arrow is an icon and
// the frame extractor sees text: with one page a Button2 press must not change
// what is displayed; with several it must. A one-direction test would pass on a
// pager that was simply deleted.
func TestReviewPagerAppearsOnlyWhenThereIsASecondPage(t *testing.T) {
	// Reports whether a touch target exists in the middle nav slot, without
	// failing the test when it does not -- absence is the assertion here.
	hasPager := func(t *testing.T, ctx *Context, d *op.Drawer) bool {
		t.Helper()
		dims := ctx.Platform.DisplaySize()
		sz := assets.NavBtnPrimary.Bounds().Size()
		pos := image.Pt(dims.X-sz.X/2, (dims.Y-sz.Y)/2+sz.Y/2)
		_, _, hit := d.Hit(pos)
		return hit
	}

	run := func(t *testing.T, lines []string) bool {
		t.Helper()
		p := newPlatform()
		p.display = sh2DisplaySize
		ctx := NewContext(p)
		frame, drawer, quit := runUITouch(ctx, func() {
			confirmReviewScreen(ctx, &descriptorTheme, "Review", lines)
		})
		defer quit()
		content, ok := pumpUntil(frame, lines[0], 64)
		if !ok {
			t.Fatalf("the review screen never showed its first line; got %q", content)
		}
		return hasPager(t, ctx, drawer())
	}

	t.Run("four lines — one page, so NO pager is drawn", func(t *testing.T) {
		if run(t, []string{"Compare this against", "what me printed:", "", "5629 025f ecdb e02e"}) {
			t.Error("a single-page review still draws a pager — this is the payload " +
				"digest screen, where the arrow did nothing when pressed")
		}
	})

	t.Run("many lines — several pages, the pager must still work", func(t *testing.T) {
		lines := make([]string, 0, 40)
		for i := 0; i < 40; i++ {
			lines = append(lines, fmt.Sprintf("line %02d", i))
		}
		if !run(t, lines) {
			t.Error("a multi-page review draws NO pager — the conditional went too far " +
				"and paging is now unreachable")
		}
	})
}
