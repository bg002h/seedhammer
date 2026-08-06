package constant

import "testing"

// Where the INK goes, as opposed to where the centreline goes.
//
// TestGlyphsStayInTheirCell checks control points against the advance, on the X
// axis only. That is two blind spots in one, and both were hit within a day:
//
//   - it reported "PASS" for every candidate height of 'e', including ones that
//     put the glyph a full unit below the deepest descender in the face, because
//     it does not look at Y at all;
//   - it would have passed a '{' whose centreline sat exactly on the cell
//     boundary, with half a stroke of ink in the neighbouring glyph's cell,
//     because a control point on the boundary is inside it.
//
// The engraver lays down a 0.30mm stroke centred on the path, so ink reaches
// half a stroke beyond every control point. Two glyphs collide when their INK
// meets, not when their centrelines do, and no test in the tree measured that
// until this one.
//
// Both bounds below are WORST CASE OVER THE WHOLE ALPHABET rather than per
// glyph: the widest-right glyph against the widest-left one, and the deepest
// against the highest. That is conservative -- the pair may be a string nobody
// types -- and it is the right shape for a face where any character can follow
// any other, which is exactly what a passphrase is.

// strokeAt3mm is the stroke in FONT units at the smallest rung the ladder cuts.
//
// The stroke is a constant 0.30mm at every rung while the glyph scales, so the
// clearance is worst at the smallest size. 0.30mm is 1920 device units and a
// 3.0mm em is 19200, so one font unit is 19200/900 device units and the stroke
// is 1920 * 900 / 19200 = 90 of them.
const strokeAt3mm = 90

// advance is the face's cell width in font units, the same figure
// TestGlyphsStayInTheirCell uses.
const advance = 600

func inkExtremes(t *testing.T) (loX, hiX, loY, hiY int, right, deep rune) {
	t.Helper()
	loX, hiX, loY, hiY = 1<<30, -(1 << 30), 1<<30, -(1 << 30)
	for r := range runes {
		_, sp, ok := Font.Decode(r)
		if !ok {
			continue
		}
		for {
			k, ok := sp.Next()
			if !ok {
				break
			}
			loX = min(loX, k.Ctrl.X)
			loY = min(loY, k.Ctrl.Y)
			if k.Ctrl.X > hiX {
				hiX, right = k.Ctrl.X, r
			}
			if k.Ctrl.Y > hiY {
				hiY, deep = k.Ctrl.Y, r
			}
		}
	}
	return
}

// TestInkClearsTheNextGlyph: no two glyphs, in any order, may have their ink
// meet when set side by side.
//
// It found a live defect the moment it was written. Scaling '&' by 4/3 made it
// simultaneously the leftmost and the rightmost glyph in the face, at 33 and 567
// against a 600-unit advance, so "&&" overlapped its own ink by 24 units --
// 0.08mm of one ampersand's tail cut into the next one's shoulder. Every
// existing test passed on it, because every existing test looks at centrelines.
func TestInkClearsTheNextGlyph(t *testing.T) {
	loX, hiX, _, _, right, _ := inkExtremes(t)
	// Glyph A at 0 inks out to hiX + sw/2. Glyph B one advance along inks back
	// to advance + loX - sw/2. The gap is the difference.
	gap := advance + loX - hiX - strokeAt3mm
	if gap < 0 {
		t.Errorf("the widest-right glyph %q inks to %d and the widest-left inks from %d; "+
			"set adjacent their ink OVERLAPS by %d font units (%.3fmm at 3.0mm). "+
			"A glyph's centreline span must not exceed advance - stroke = %d units.",
			right, hiX, advance+loX, -gap, float64(-gap)*(19200.0/900)/6400, advance-strokeAt3mm)
	}
	// And the figure itself is pinned, so a change that eats the clearance is
	// visible even while it stays positive.
	if want := 10; gap != want {
		t.Errorf("neighbour ink clearance is %d font units, pinned at %d. "+
			"If a glyph was deliberately widened, re-measure and update this, and "+
			"say in the commit what the plate gains for the clearance it spends.",
			gap, want)
	}
}

// TestInkClearsTheRowBelow is the same question down the page. Rows are set one
// em apart, so the deepest descender of one row and the highest ink of the next
// are Height apart less what the two strokes take.
//
// MEASURED: 10 font units, 0.033mm at 3.0mm. That is genuinely tight, and it is
// the face as it already was -- '|' and '$' reach the top of the band and the
// descenders reach the bottom. Pinned rather than loosened, so the next glyph
// that grows vertically has to argue for it.
func TestInkClearsTheRowBelow(t *testing.T) {
	_, _, loY, hiY, _, deep := inkExtremes(t)
	m := Font.Metrics()
	gap := m.Height + loY - hiY - strokeAt3mm
	if gap < 0 {
		t.Errorf("the deepest glyph %q inks to %d and the highest inks from %d; one em apart "+
			"their ink OVERLAPS by %d font units (%.3fmm at 3.0mm)",
			deep, hiY, loY, -gap, float64(-gap)*(19200.0/900)/6400)
	}
	if want := 10; gap != want {
		t.Errorf("row ink clearance is %d font units, pinned at %d; a glyph grew past the "+
			"band the face already used", gap, want)
	}
}
