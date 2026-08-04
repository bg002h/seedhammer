package backup

import (
	"slices"
	"testing"

	"seedhammer.com/bspline"
	"seedhammer.com/engrave"
	"seedhammer.com/font/sh"
)

// prodParams is the PRODUCTION stroke width, 0.3mm
// (cmd/controller/platform_sh2.go), not backup_test.go's mm/3.
//
// The character grid is stroke-independent -- charWidth comes from the font --
// so CharsPerLine and LinesPerPlate are the same either way. The QR columns are
// not: at mm/3 a text that needs a 37-module QR in production reports 33, which
// reproduces spec 4's geometry column with a QR production does not satisfy.
// Every capacity assertion in this package therefore runs at 0.3mm, and this is
// the one place that says why.
var prodParams = engrave.Params{
	Millimeter:    mm,
	StrokeWidth:   int(0.3 * mm),
	StepperConfig: conf,
}

// TestFontSizeLadder pins spec 4's grid at every rung. These six pairs are the
// basis of every capacity number in the spec, so a change here is a change to
// what fits on a plate.
func TestFontSizeLadder(t *testing.T) {
	want := []struct {
		size          float32
		charsPerLine  int
		linesPerPlate int
	}{
		{6.0, 22, 13},
		{5.0, 26, 15},
		{4.4, 30, 17},
		{3.8, 34, 20},
		{3.4, 38, 23},
		{3.0, 44, 26},
	}
	if len(FontSizes) != len(want) {
		t.Fatalf("FontSizes has %d rungs, want %d: %v", len(FontSizes), len(want), FontSizes)
	}
	for i, w := range want {
		if FontSizes[i] != w.size {
			t.Errorf("FontSizes[%d] = %v, want %v", i, FontSizes[i], w.size)
		}
		if got := CharsPerLine(prodParams, sh.Font, w.size); got != w.charsPerLine {
			t.Errorf("CharsPerLine(%vmm) = %d, want %d", w.size, got, w.charsPerLine)
		}
		if got := LinesPerPlate(prodParams, w.size); got != w.linesPerPlate {
			t.Errorf("LinesPerPlate(%vmm) = %d, want %d", w.size, got, w.linesPerPlate)
		}
	}
}

// TestFontSizesDescending: every fit loop takes the first match, so the ladder
// must be sorted largest-first or auto-fit silently picks a smaller size than
// the composition can carry.
func TestFontSizesDescending(t *testing.T) {
	for i := 1; i < len(FontSizes); i++ {
		if FontSizes[i] >= FontSizes[i-1] {
			t.Fatalf("FontSizes not strictly descending at %d: %v", i, FontSizes)
		}
	}
}

// TestFixedCharWidthIsExactForEveryGlyph: fixedCharWidth reads 'W' and calls it
// the advance of every character. That is only sound while the face really is
// monospace, so check it rather than trust the comment.
func TestFixedCharWidthIsExactForEveryGlyph(t *testing.T) {
	wantAdv, _, ok := sh.Font.Decode('W')
	if !ok {
		t.Fatal("W not in font/sh")
	}
	for r := rune(0x20); r <= 0x7e; r++ {
		adv, _, ok := sh.Font.Decode(r)
		if !ok {
			t.Fatalf("font/sh cannot decode %q", r)
		}
		if adv != wantAdv {
			t.Errorf("advance of %q is %d, want %d -- font/sh is not monospace and fixedCharWidth is wrong", r, adv, wantAdv)
		}
	}
}

// TestTextFontSizeZeroFallsBackToUR is what keeps the three text-* goldens
// byte-identical: every existing caller builds a Text without FontSize, so zero
// must mean 3.8mm and nothing else.
func TestTextFontSizeZeroFallsBackToUR(t *testing.T) {
	var zero Text
	if zero.FontSize != 0 {
		t.Fatalf("zero Text.FontSize = %v, want 0", zero.FontSize)
	}
	if got := zero.fontMM(); got != plateFontSizeUR {
		t.Errorf("Text{}.fontMM() = %v, want %v (plateFontSizeUR)", got, plateFontSizeUR)
	}
	explicit := Text{FontSize: 6.0}
	if got := explicit.fontMM(); got != 6.0 {
		t.Errorf("Text{FontSize: 6}.fontMM() = %v, want 6", got)
	}
}

// TestEngraveTextHonoursFontSize checks the fallback where it actually bites:
// inside EngraveText. Zero and an explicit 3.8 must plan identically, and a
// different size must not -- otherwise FontSize is decorative and the fallback
// test above proves nothing about a plate.
func TestEngraveTextHonoursFontSize(t *testing.T) {
	const s = "abcdefghijklmnopqrstuvwxyz0123456789abcdefghijklmnopqrstuvwxyz"
	plan := func(size float32) []bspline.Knot {
		txt := Text{
			Paragraphs: []Paragraph{{Text: s}},
			Font:       sh.Font,
			FontSize:   size,
		}
		return slices.Collect(engrave.PlanEngraving(conf, EngraveText(params, txt)))
	}
	zero, explicitUR, bigger := plan(0), plan(plateFontSizeUR), plan(6.0)
	if !slices.Equal(zero, explicitUR) {
		t.Errorf("FontSize 0 planned %d knots, explicit %vmm planned %d -- the fallback is not plateFontSizeUR",
			len(zero), float32(plateFontSizeUR), len(explicitUR))
	}
	if slices.Equal(zero, bigger) {
		t.Error("FontSize 6.0 planned identically to the default -- EngraveText ignores FontSize")
	}
}
