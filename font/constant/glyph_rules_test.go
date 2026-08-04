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
