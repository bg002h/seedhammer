package backup

import (
	"strings"
	"testing"

	"seedhammer.com/font/constant"
	"seedhammer.com/font/sh"
)

// TestFitBlocksAtEngravesTheRungAsked is the property the whole fixed-size path
// exists for. Auto-fit answers "as large as possible"; once the operator has
// CHOSEN a size and the content is what gives way, that is the wrong question,
// and a trimmed composition that also fits a larger rung would be engraved
// there instead.
func TestFitBlocksAtEngravesTheRungAsked(t *testing.T) {
	// Short enough to fit at every rung, so any rung returned other than the one
	// asked for is the bug rather than a refusal.
	blocks := []Block{{Face: sh.Font, Text: "abc"}}
	for _, size := range FontSizes {
		f, err := FitBlocksAt(prodParams, blocks, "", "", false, size)
		if err != nil {
			t.Errorf("%.1fmm: %v", size, err)
			continue
		}
		if f.SizeMM != size {
			t.Errorf("asked for %.1fmm, got %.1fmm", size, f.SizeMM)
		}
		if len(f.Lines) != len(f.Faces) {
			t.Errorf("%.1fmm: %d lines but %d faces", size, len(f.Lines), len(f.Faces))
		}
	}
}

// TestFitBlocksAtRejectsSizesOffTheLadder guards the one input that would lay a
// plate out at a size no capacity figure in this package is measured against.
// cmd/plateview -size 4.5 reaches this, and without the guard it would engrave
// silently at a rung nothing pins.
func TestFitBlocksAtRejectsSizesOffTheLadder(t *testing.T) {
	blocks := []Block{{Face: sh.Font, Text: "abc"}}
	for _, size := range []float32{0, 4.5, 2.9, 7.0, -3} {
		if _, err := FitBlocksAt(prodParams, blocks, "", "", false, size); err == nil {
			t.Errorf("%.1fmm is not in FontSizes %v but was accepted", size, FontSizes)
		}
	}
	// And an empty composition is refused rather than yielding a blank plate.
	if _, err := FitBlocksAt(prodParams, nil, "", "", false, 3.0); err == nil {
		t.Error("an empty block list was accepted")
	}
}

// TestFitBlocksAtRefusesWhatDoesNotFit: the fixed-size path must REFUSE rather
// than silently drop to a smaller rung, which is what auto-fit would do.
func TestFitBlocksAtRefusesWhatDoesNotFit(t *testing.T) {
	huge := []Block{{Face: sh.Font, Text: strings.Repeat("W", 4000)}}
	if _, err := FitBlocksAt(prodParams, huge, "", "", false, 6.0); err == nil {
		t.Error("4000 characters were accepted at the 6.0mm rung")
	}
	// The same text at the smallest rung is still too big, so this is not a
	// test that only passes because 6.0 is large.
	if _, err := FitBlocksAt(prodParams, huge, "", "", false, 3.0); err == nil {
		t.Error("4000 characters were accepted at the 3.0mm rung")
	}
}

// TestFitBlocksAtAgreesWithFitBlocks: at the rung auto-fit would have picked,
// the two paths must produce the same plate. Two entry points that laid the
// same composition out differently would mean the preview and the machine
// disagree.
func TestFitBlocksAtAgreesWithFitBlocks(t *testing.T) {
	blocks := []Block{
		{Face: sh.Font, Text: "the quick brown fox\njumps over the lazy dog"},
		{Face: constant.Font, Text: "PACK MY BOX WITH FIVE\nDOZEN LIQUOR JUGS"},
	}
	auto, err := FitBlocks(prodParams, blocks, "TITLE", "FOOTER", false)
	if err != nil {
		t.Fatal(err)
	}
	at, err := FitBlocksAt(prodParams, blocks, "TITLE", "FOOTER", false, auto.SizeMM)
	if err != nil {
		t.Fatalf("%.1fmm: auto-fit chose this rung but the fixed path refused it: %v",
			auto.SizeMM, err)
	}
	if at.SizeMM != auto.SizeMM || len(at.Lines) != len(auto.Lines) {
		t.Fatalf("auto-fit gave %.1fmm/%d lines, fixed gave %.1fmm/%d",
			auto.SizeMM, len(auto.Lines), at.SizeMM, len(at.Lines))
	}
	for i := range auto.Lines {
		if at.Lines[i] != auto.Lines[i] {
			t.Errorf("line %d: auto %q, fixed %q", i, auto.Lines[i], at.Lines[i])
		}
		if at.Faces[i] != auto.Faces[i] {
			t.Errorf("line %d is cut in a different face by the two paths", i)
		}
	}
}
