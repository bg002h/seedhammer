package gui

import (
	"fmt"
	"image"
	"testing"
	"testing/synctest"
)

// composerNumberedLines is a body whose every row is identifiable on a frame,
// so a paging assertion can say WHICH rows were drawn rather than how many
// characters appeared.
func composerNumberedLines(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("entry %02d marker", i)
	}
	return out
}

// TestComposerPageLinesNeverOverflowsTheContentBox is the defect this
// primitive exists for: ChoiceScreen draws an over-long list past the frame
// with no clip and no cue (gui/gui.go:1993-2026).
//
// It asserts the MEASURE, not the pixels: composerPageLines reports how many
// rows it laid out, and that count must be strictly less than a list far
// longer than any frame can hold -- which is exactly the property
// ChoiceScreen lacks.
func TestComposerPageLinesNeverOverflowsTheContentBox(t *testing.T) {
	p := newPlatform()
	p.display = sh2DisplaySize
	ctx := NewContext(p)
	lines := composerNumberedLines(64)
	_, shown, _ := composerPageLines(ctx, &descriptorTheme, sh2DisplaySize, lines, 0, -1)
	if shown <= 0 {
		t.Fatalf("composerPageLines laid out %d rows of 64; a 232 px content box holds several", shown)
	}
	if shown >= len(lines) {
		t.Fatalf("composerPageLines claims all %d rows fit one 232 px frame -- that is "+
			"the ChoiceScreen defect (gui/gui.go:1993-2026) reproduced, not fixed", shown)
	}
	t.Logf("per-frame capacity at %v with body text: %d rows", sh2DisplaySize, shown)
	// And paging from the tail must not report rows past the end.
	_, tail, _ := composerPageLines(ctx, &descriptorTheme, sh2DisplaySize, lines, len(lines)-2, -1)
	if tail != 2 {
		t.Errorf("laying out from the second-to-last row drew %d rows, want 2", tail)
	}
}

// TestComposerPickScreenReachesARowOnASecondPage is the operator-visible half:
// a row that does not fit the first frame is still SELECTABLE, which on
// ChoiceScreen it is not (the row draws off-frame while Down still moves the
// invisible cursor onto it).
func TestComposerPickScreenReachesARowOnASecondPage(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p := newPlatform()
		p.display = sh2DisplaySize
		ctx := NewContext(p)
		rows := composerNumberedLines(24)
		var got int
		var ok bool
		frame, quit := runUI(ctx, func() {
			got, ok = composerPickScreen(ctx, &descriptorTheme, "Pick", "Choose one", rows)
		})
		defer quit()
		if _, seen := pumpUntil(frame, "entry 00 marker", 8); !seen {
			t.Fatal("the first page never drew")
		}
		// Page forward once (Button2), then take the first row of that page.
		click(&ctx.Router, Button2)
		content, seen := pumpUntil(frame, "marker", 8)
		if !seen {
			t.Fatalf("the second page never drew.\nLast frame: %q", content)
		}
		if uiContains(content, "entry 00 marker") {
			t.Errorf("Button2 did not advance the page; entry 00 is still drawn.\nFrame: %q", content)
		}
		click(&ctx.Router, Button3)
		for i := 0; i < 8; i++ {
			if _, more := frame(); !more {
				break
			}
		}
		if !ok {
			t.Fatal("the pick screen returned no selection after Button3")
		}
		if got == 0 {
			t.Errorf("selecting the first row of the SECOND page returned index 0, "+
				"so paging did not move the cursor with the page (got %d)", got)
		}
	})
}

// THE INK COMPARISON IS GONE, and its removal is the fix rather than a
// tidy-up.
//
// TestComposerReadScreenDrawsThePagerOnlyWhenASecondPageExists used to live
// here and assert `longInk > shortInk` between a one-line body and a 64-line
// body. ink() counts lit pixels for the WHOLE frame (gui/raster_test.go:24),
// so the difference is dominated by roughly ten drawn body rows: the assertion
// passed if the pager was always drawn, never drawn, or drawn correctly. It
// could not fail for the reason it stated, which is the one thing a gate must
// be able to do.
//
// Its replacement is behavioural and in composer_gates_test.go:
// TestComposerReadScreenWithholdsTheCheckmarkUntilTheLastPage, which fails in
// both directions -- a one-page screen that withholds its checkmark and a
// multi-page screen that does not.

var _ = image.Pt
