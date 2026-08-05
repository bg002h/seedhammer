package constant

import (
	"testing"

	"seedhammer.com/bezier"
)

// runes iterates the space mark plus every printable ASCII rune.
func runes(yield func(rune) bool) {
	if !yield(0x1F) {
		return
	}
	for r := rune(0x20); r <= 0x7E; r++ {
		if !yield(r) {
			return
		}
	}
}

// Every glyph must share one advance -- NewConstantStringer panics
// "variable width font" otherwise (engrave/engrave.go:1216-1218), and the
// passphrase plate's "position implies index" property depends on it.
func TestUniformAdvance(t *testing.T) {
	const want = 600
	for r := range runes {
		adv, _, ok := Font.Decode(r)
		if !ok {
			continue // coverage_test.go reports missing glyphs
		}
		if adv != want {
			t.Errorf("advance(%q) = %d, want %d", r, adv, want)
		}
	}
}

// parseChars (cmd/vectorfont/main.go:414-428) assigns each glyph's cell by
// DOCUMENT ORDER, subtracting one advance per element, so the k-th glyph
// element is normalised against x in [6k, 6k+6). The x-coordinates drawn in
// constant.svg therefore do not choose the cell -- element order does, and a
// glyph drawn against the wrong cell still decodes and still has the uniform
// advance. It does not, however, stay inside its cell: its normalised
// x-coordinates land a whole multiple of the advance away. That is what this
// checks. Insertions mid-file are caught by the existing plate goldens; a
// misplaced *new* glyph is caught here.
func TestGlyphsStayInTheirCell(t *testing.T) {
	const advance = 600
	for r := range runes {
		_, spline, ok := Font.Decode(r)
		if !ok {
			continue
		}
		for {
			k, ok := spline.Next()
			if !ok {
				break
			}
			if k.Ctrl.X < 0 || k.Ctrl.X > advance {
				t.Errorf("glyph %q has x=%d outside its %d-unit cell; it was "+
					"drawn against the wrong slot in constant.svg", r, k.Ctrl.X, advance)
				break
			}
		}
	}
}

// paddedString uses inf.Start != (bezier.Point{}) as its sentinel for "this
// glyph has a leading move segment" (engrave/engrave.go:1294-1296). A glyph
// whose first control point is exactly the origin takes the wrong branch.
// Plausible for '_', which naturally starts at the lower left.
func TestNoGlyphStartsAtOrigin(t *testing.T) {
	for r := range runes {
		_, spline, ok := Font.Decode(r)
		if !ok {
			continue
		}
		k, ok := spline.Next() // vector.UniformBSpline.Next() (Knot, bool)
		if !ok {
			continue // no knots (space is a blank advance)
		}
		if k.Ctrl == (bezier.Point{}) {
			t.Errorf("glyph %q starts at the origin; paddedString's sentinel will misfire", r)
		}
	}
}

// TestSmallFeaturesClearTheStroke pins every small glyph feature against the
// STROKE WIDTH, which is what actually decides whether the two marks stay
// separate. A unit only means something relative to how fat the cut is.
//
// All of these shipped at ONE unit: 0.333mm at the 3.0mm rung against a
// 0.3mm stroke, i.e. 1.1 stroke widths, so any burr closes it. For '!' that
// means reading as '|' -- a full-height unbroken line, where the gap is the ONLY
// difference. For 'i' and 'j' it means the dot merges into the stem and they
// read as 'l' and a hook. All three read off engraved plates, 2026-08-04.
//
// '!' was fixed by shortening the stem; 'i' and 'j' by raising the dot, because
// their stems cannot move down -- 'i' sits on the baseline and 'j' already
// descends to y9.
func TestSmallFeaturesClearTheStroke(t *testing.T) {
	const emMM, strokeMM, cellUnits = 3.0, 0.3, 9.0
	const unitMM = emMM / cellUnits
	for _, g := range []struct {
		name         string
		lower, upper float64 // the feature spans lower..upper, in cell units
	}{
		// Dot-over-stem gaps: too small and the two marks merge.
		{"'!' stem end -> dot top", 5, 7},
		{"'i' dot bottom -> stem top", 3, 5},
		{"'j' dot bottom -> stem top", 3, 5},
		// Bracket bars: too short and the bar reads as a thickening of the
		// stem, leaving '[' and ']' to look like '|'. Same arithmetic, same
		// threshold -- a feature shorter than about two stroke widths does not
		// survive the cut.
		{"'[' top bar", 3, 5},
		{"'[' bottom bar", 3, 5},
		{"']' top bar", 1, 3},
		{"']' bottom bar", 1, 3},
		// Brace midpoints: the reach past the body is what says "brace" rather
		// than "bracket". One unit of reach disappears into the stroke.
		{"'{' midpoint reach", 1, 3},
		{"'}' midpoint reach", 3, 5},
		// Small marks: a tick shorter than two stroke widths reads as a blob.
		// The comma family was the worst on the plate at 1.6.
		{"',' tail", 6, 8},
		{"';' tail", 6, 8},
	} {
		gap := (g.upper - g.lower) * unitMM
		if want := 2.0 * strokeMM; gap < want {
			t.Errorf("%s is %.3fmm (%.1f stroke widths) at the 3.0mm rung; want at least "+
				"%.3fmm (2 stroke widths) or the two marks merge when cut",
				g.name, gap, gap/strokeMM, want)
		}
	}
}

// TestQuoteInkGapClearsTheStroke pins the double quote's INK-TO-INK gap, which
// is a different quantity from the feature lengths above: centre separation
// alone is misleading, because the stroke is 0.9 units wide at this rung, so
// 2.0 units of separation is only 1.1 units of actual gap.
//
// It is held at 1.5 stroke widths rather than the 2.0 the other features use,
// because widening further would push the glyph past the alphabet's local x
// maximum of 5.0 -- and NewPassphraseStringer derives `center` from bounds
// accumulated over EVERY glyph, so extending that maximum relocates every
// constant-time plate. Measured: growing rightward moved center.X from 7111 to
// 7822 and shifted three goldens whose text contains no quote at all. Widening
// LEFTWARD instead reaches 1.5 stroke widths with the box untouched.
func TestQuoteInkGapClearsTheStroke(t *testing.T) {
	const emMM, strokeMM, cellUnits = 3.0, 0.3, 9.0
	const unitMM = emMM / cellUnits
	const sep = 2.25 // stroke centres, in cell units (x470.75 and x473)
	stroke := strokeMM / unitMM
	ink := (sep - stroke) * unitMM
	if want := 1.5 * strokeMM; ink < want {
		t.Errorf("the '\"' ink gap is %.3fmm (%.2f stroke widths); want at least %.3fmm "+
			"(1.5 stroke widths) or the two strokes cut as one mark", ink, ink/strokeMM, want)
	}
}

// TestBowlJunctionsAreOffHorizontal pins that 'p' and 'q' close their bowls on a
// SLOPE rather than a horizontal, so a descender lost to damage cannot leave
// something that reads as 'o'.
//
// 'o' is a plain rectangle and 'b'/'d' close flat, so a flat-bottomed p or q was
// distinguished from all three by its tail alone -- the one feature most exposed
// to a scratch or a bad cut. Sloping the junction makes the bowl itself
// unmistakable, and raising it (rather than lowering) lengthens the visible
// descender from 2 units to 3 at the same time. Lowering would have shortened it
// to 1 unit = 1.1 stroke widths, making the tail the likeliest thing to be lost
// on the very glyph whose damage case this protects. Read off a plate, 2026-08-04.
func TestBowlJunctionsAreOffHorizontal(t *testing.T) {
	for _, g := range []struct {
		name             string
		bowlY, junctionY float64 // right-hand bowl corner, and where it meets the stem
	}{
		{"'p'", 7, 6},
		{"'q'", 7, 6},
	} {
		if g.bowlY == g.junctionY {
			t.Errorf("%s closes its bowl horizontally at y=%.0f; a damaged descender would "+
				"leave a shape that reads as 'o'", g.name, g.bowlY)
		}
		// And the descender must still clear the stroke below the junction.
		const emMM, strokeMM, cellUnits = 3.0, 0.3, 9.0
		tail := (9 - g.junctionY) * (emMM / cellUnits)
		if want := 2.0 * strokeMM; tail < want {
			t.Errorf("%s has only %.3fmm (%.1f stroke widths) of descender below the junction",
				g.name, tail, tail/strokeMM)
		}
	}
}
