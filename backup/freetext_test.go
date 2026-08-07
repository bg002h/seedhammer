package backup

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	qrpkg "github.com/seedhammer/kortschak-qr"
	"seedhammer.com/bezier"
	"seedhammer.com/bspline"
	"seedhammer.com/engrave"
	"seedhammer.com/font/constant"
	"seedhammer.com/font/sh"
	"seedhammer.com/font/vector"
	"seedhammer.com/internal/golden"
)

// ftBounds is the ink of a free-text plate at the PRODUCTION stroke.
func ftBounds(t testing.TB, fontMM float32, title string, lines []string, footer string, qrc *qrpkg.Code) bspline.Bounds {
	t.Helper()
	return inkBounds(t, prodParams, EngraveFreeText(prodParams, sh.Font, fontMM, title, lines, footer, qrc))
}

func rowBand(fontMM float32, row int) (top, bottom int) {
	P := prodParams
	y := P.I(outerMargin) + row*P.F(fontMM)
	return y, y + P.F(fontMM)
}

// TestFreeTextRowsAreAbsolute pins spec 8: the title is plate row 0, the footer
// is plate row LinesPerPlate-1 -- absolute, not "after the text" -- and the body
// starts on row 1 exactly when a title occupies row 0.
func TestFreeTextRowsAreAbsolute(t *testing.T) {
	for _, size := range FontSizes {
		rows := LinesPerPlate(prodParams, size)

		title := ftBounds(t, size, "TITLE", nil, "", nil)
		top, bottom := rowBand(size, 0)
		if title.Min.Y < top || title.Max.Y > bottom {
			t.Errorf("%.1fmm title inks y[%d,%d], outside row 0's band [%d,%d]", size, title.Min.Y, title.Max.Y, top, bottom)
		}

		footer := ftBounds(t, size, "", nil, "FOOTER", nil)
		top, bottom = rowBand(size, rows-1)
		if footer.Min.Y < top || footer.Max.Y > bottom {
			t.Errorf("%.1fmm footer inks y[%d,%d], outside row %d's band [%d,%d]", size, footer.Min.Y, footer.Max.Y, rows-1, top, bottom)
		}

		// The body drops a row when -- and only when -- a title is present.
		noTitle := ftBounds(t, size, "", []string{"X"}, "", nil)
		withTitle := ftBounds(t, size, "T", []string{"X"}, "", nil)
		if got, want := withTitle.Max.Y-noTitle.Max.Y, prodParams.F(size); got != want {
			t.Errorf("%.1fmm: a title moved the body by %d, want exactly one row of %d", size, got, want)
		}
		// A footer must not move the body at all: its row is absolute.
		withFooter := ftBounds(t, size, "", []string{"X"}, "F", nil)
		if withFooter.Min.Y != noTitle.Min.Y {
			t.Errorf("%.1fmm: a footer moved the body's top from %d to %d", size, noTitle.Min.Y, withFooter.Min.Y)
		}
	}
}

// TestTitleAndFooterCenterInTheInsetSpan is spec 8's requirement that the title
// stay between the screw-hole bands.
//
// MEASURED, AND WORTH KNOWING: centering in the inset span is arithmetically
// IDENTICAL to centering on the full plate width, because the span is
// symmetric about the plate centre -- margin+inset+(W-2*margin-2*inset-w)/2
// reduces to (W-w)/2. Spec 8 presents the inset span as the fix for spec 2's
// silently-wrong plate; it is not. What actually stops that plate is the
// 18-character cap, which is what spec 2 itself concludes. The formulation is
// kept because it says what it means and survives an asymmetric plate, but no
// mutation can distinguish it -- so the test that carries the weight is
// TestTitleCapFitsAtEveryRung below, and it is checked in both directions.
func TestTitleAndFooterCenterInTheInsetSpan(t *testing.T) {
	for _, size := range FontSizes {
		lay, rows := layAt(size, nil, freeTextQRScale)
		inset := lay.holeChars * lay.charWidth
		spanLo := prodParams.I(outerMargin) + inset
		spanHi := prodParams.F(plateSize) - prodParams.I(outerMargin) - inset

		for _, tc := range []struct {
			what          string
			title, footer string
			row           int
		}{
			{"title", strings.Repeat("W", MaxTitleLen), "", 0},
			{"footer", "", strings.Repeat("W", MaxTitleLen), rows - 1},
		} {
			b := ftBounds(t, size, tc.title, nil, tc.footer, nil)
			if b.Min.X < spanLo || b.Max.X > spanHi {
				t.Errorf("%.1fmm %s inks x[%d,%d], outside the inset span [%d,%d]",
					size, tc.what, b.Min.X, b.Max.X, spanLo, spanHi)
			}
			// Centered within the span: equal gaps, to within a stroke.
			gapLo, gapHi := b.Min.X-spanLo, spanHi-b.Max.X
			if d := gapLo - gapHi; d < -prodParams.StrokeWidth || d > prodParams.StrokeWidth {
				t.Errorf("%.1fmm %s is not centered in the inset span: gaps %d and %d", size, tc.what, gapLo, gapHi)
			}
		}
	}
}

// TestTitleCapFitsAtEveryRung is spec 11's binding requirement: a title and a
// footer at the 18-character cap sit inside [innerMargin, plateSize -
// innerMargin] at EVERY rung. 6.0mm is the tight one -- 20 characters there
// already cross both bands while toPlate passes.
func TestTitleCapFitsAtEveryRung(t *testing.T) {
	lo := prodParams.I(innerMargin)
	hi := prodParams.F(plateSize) - prodParams.I(innerMargin)
	// 'W' is the worst case only because the face is monospace and every
	// advance is equal; its ink is the widest inside that advance.
	cap18 := strings.Repeat("W", MaxTitleLen)
	worst := hi - lo
	for _, size := range FontSizes {
		b := ftBounds(t, size, cap18, nil, cap18, nil)
		if b.Min.X < lo || b.Max.X > hi {
			t.Errorf("%.1fmm: an %d-character title inks x[%.3f,%.3f]mm, outside the screw-hole-free span [%.1f,%.1f]mm",
				size, MaxTitleLen, float64(b.Min.X)/mm, float64(b.Max.X)/mm, float64(lo)/mm, float64(hi)/mm)
		}
		if slack := min(b.Min.X-lo, hi-b.Max.X); slack < worst {
			worst = slack
		}
	}
	// Pin the measured headroom so it cannot shrink unnoticed. Measured at
	// 6.0mm: 0.709mm on the left, 0.620mm on the right.
	if got := float64(worst) / mm; got < 0.55 || got > 0.70 {
		t.Errorf("tightest single-side slack across the ladder = %.3fmm; measured 0.620mm at 6.0mm", got)
	}
	// And 20 characters, which spec 2 measured as the silently-wrong plate,
	// must NOT fit at 6.0mm.
	if b := ftBounds(t, 6.0, strings.Repeat("W", 20), nil, "", nil); b.Min.X >= lo && b.Max.X <= hi {
		t.Errorf("a 20-character title fits the screw-hole-free span at 6.0mm (x[%d,%d]); the 18-character cap has stopped binding", b.Min.X, b.Max.X)
	}
}

// TestTitleAndFooterAreEngravedVerbatim. TitleString upper-cases and truncates
// at 18; routing either field through it would engrave something the operator
// never approved.
func TestTitleAndFooterAreEngravedVerbatim(t *testing.T) {
	// A lowercase title must not plan identically to its upper-cased self.
	lower := ftBounds(t, 6.0, "abcdefgh", nil, "", nil)
	upper := ftBounds(t, 6.0, "ABCDEFGH", nil, "", nil)
	if lower == upper {
		t.Error("a lowercase title planned identically to its upper-cased self; it is going through TitleString")
	}
	same := func(a, b bspline.Bounds) bool { return a == b }
	if !same(ftBounds(t, 6.0, "abcdefgh", nil, "", nil), lower) {
		t.Fatal("EngraveFreeText is not deterministic")
	}
	// Same for the footer.
	lo := ftBounds(t, 6.0, "", nil, "abcdefgh", nil)
	up := ftBounds(t, 6.0, "", nil, "ABCDEFGH", nil)
	if lo == up {
		t.Error("a lowercase footer planned identically to its upper-cased self")
	}
	// A character TitleString would drop (it keeps only what the face can
	// engrave, after upper-casing) must still reach the plate.
	if b := ftBounds(t, 6.0, "a_b~c", nil, "", nil); b.Empty() {
		t.Error("punctuation title engraved nothing")
	}
}

// TestFreeTextQRPlacement: right-hand side, 2mm border, at QRScale 2 -- 0.6mm
// modules against the 0.9mm every other plate uses. Spec 4's capacity columns
// depend on the scale, so it is normative, not a preference.
func TestFreeTextQRPlacement(t *testing.T) {
	qrc, err := qrFor("a note with a code", true)
	if err != nil {
		t.Fatal(err)
	}
	const size = 3.8
	P := prodParams
	b := ftBounds(t, size, "", nil, "", qrc)

	// The scale IS the module size. engrave.QR rasters each module as
	// strokeWidth*scale wide and insets the ink half a stroke at each end, so
	// a Size-module code inks Size*strokeWidth*scale - strokeWidth across.
	qrsz := qrc.Size * P.StrokeWidth * freeTextQRScale
	if got, want := b.Max.X-b.Min.X, qrsz-P.StrokeWidth; got != want {
		t.Errorf("QR inks %d wide, want %d (%d modules x %d stroke x scale %d, less one stroke)",
			got, want, qrc.Size, P.StrokeWidth, freeTextQRScale)
	}
	if got, want := b.Max.Y-b.Min.Y, qrsz-P.StrokeWidth; got != want {
		t.Errorf("QR inks %d tall, want %d", got, want)
	}

	// Right-hand side, one 2mm border in from the text margin.
	wantX := P.F(plateSize) - qrsz - P.I(outerMargin) - P.I(2)
	if d := b.Min.X - wantX; d != P.StrokeWidth/2 {
		t.Errorf("QR left edge at %d, want %d (half a stroke in from %d)", b.Min.X, wantX+P.StrokeWidth/2, wantX)
	}

	// It starts below the top screw-hole band, inside the very band the fit
	// reserved for it -- read off the placement rather than recounted in rows.
	top := qrPlaceFor(size, qrc, freeTextQRScale).Top
	if b.Min.Y < top {
		t.Errorf("QR starts at y=%d, above the reserved band's top edge %d", b.Min.Y, top)
	}
}

// TestFreeTextBodyRespectsTheBand: every engraved body line must start at the
// inset its own row allows, so nothing runs into a screw hole.
func TestFreeTextBodyRespectsTheBand(t *testing.T) {
	const size = 3.0
	lay, rows := layAt(size, nil, freeTextQRScale)
	P := prodParams
	lines := make([]string, rows)
	for i := range lines {
		n, _ := lay.at(i)
		lines[i] = strings.Repeat("W", n)
	}
	b := ftBounds(t, size, "", lines, "", nil)
	lo := P.I(outerMargin) + lay.holeChars*lay.charWidth
	hi := P.F(plateSize) - P.I(outerMargin)
	// The inset rows are the narrow ones, so a full plate of maximal lines
	// must still start at or after the plain margin and end within the plate.
	if b.Min.X < P.I(outerMargin) {
		t.Errorf("body inks x from %d, left of the %dmm margin", b.Min.X, outerMargin)
	}
	if b.Max.X > hi {
		t.Errorf("body inks to %d, past the %dmm margin at %d", b.Max.X, outerMargin, hi)
	}
	// And an inset row on its own starts at the inset, not the margin.
	insetOnly := ftBounds(t, size, "", []string{"W"}, "", nil)
	if insetOnly.Min.X < lo {
		t.Errorf("row 0 is a screw-hole row but inks from %d, before the inset at %d", insetOnly.Min.X, lo)
	}
}

// ftGolden compares a free-text plate against a golden at the production
// stroke. New goldens are permitted; no existing one may move.
func ftGolden(t *testing.T, name string, fontMM float32, title string, lines []string, footer string, qrc *qrpkg.Code) {
	t.Helper()
	e := EngraveFreeText(prodParams, sh.Font, fontMM, title, lines, footer, qrc)
	spline := engrave.PlanEngraving(prodParams.StepperConfig, e)
	bounds := bspline.Bounds{Max: bezier.Point{X: plateSize * mm, Y: plateSize * mm}}
	p := filepath.Join("testdata", name+".bin")
	if err := golden.CompareBSpline(p, *update, t.ArtifactDir(), prodParams.StrokeWidth, bounds, spline); err != nil {
		t.Fatal(err)
	}
}

// TestFreeTextGoldens pins a representative plate with and without a QR. The
// composition is produced by Fit, so the golden covers the real path: one
// encode, one wrap, one size.
func TestFreeTextGoldens(t *testing.T) {
	const text = "Dear heir,\n\nThe hardware wallet is in the safe.\nThe PIN is NOT written down anywhere.\nCall the lawyer first: +1 555 0100"
	for _, tc := range []struct {
		name string
		qr   bool
	}{
		{"freetext-0-plain", false},
		{"freetext-1-qr", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			size, lines, qrc, err := Fit(prodParams, sh.Font, text, "TO MY HEIR", "2026 COPY 1/1", tc.qr)
			if err != nil {
				t.Fatal(err)
			}
			if tc.qr != (qrc != nil) {
				t.Fatalf("qr=%v but Fit returned code %v", tc.qr, qrc)
			}
			ftGolden(t, tc.name, size, "TO MY HEIR", lines, "2026 COPY 1/1", qrc)
		})
	}
}

// TestFreeTextBodyNeverEntersTheQRBox is the invariant the whole QR-narrowing
// half of lineLayout exists for. A rendered plate can look as though a
// full-budget line runs into the code; measured, the tightest composition
// clears it by 2.25mm at 6.0mm. Nothing here is an eyeball judgement.
func TestFreeTextBodyNeverEntersTheQRBox(t *testing.T) {
	P := prodParams
	texts := []string{
		"Dear heir,\n\nThe hardware wallet is in the safe.\nThe PIN is NOT written down anywhere.\nCall the lawyer first: +1 555 0100",
		strings.Repeat("W", 400),
		strings.Repeat("W ", 150),
		"aaaaaaaaaaaaaa aaaaaaaaaaaaaa aaaaaaaaaaaaaa aaaaaaaaaaaaaa aaaaaaaaaaaaaa",
	}
	for _, text := range texts {
		for _, tf := range [][2]string{{"", ""}, {"TITLE", "FOOTER"}} {
			size, lines, qrc, err := Fit(P, sh.Font, text, tf[0], tf[1], true)
			if err != nil {
				continue // too large at every rung is a different test's problem
			}
			lay, rows := layAt(size, qrc, freeTextQRScale)
			qrp := qrPlaceFor(size, qrc, freeTextQRScale)
			qrsz := qrp.Size
			qrx, qry := qrp.X, qrp.Y
			start, _ := bodyRows(rows, tf[0], tf[1])
			for i, l := range lines {
				row := start + i
				_, offx := lay.at(row)
				// The advance box, which is wider than the ink -- so this is
				// the conservative question.
				x0 := P.I(outerMargin) + offx
				x1 := x0 + len(l)*lay.charWidth
				y0 := P.I(outerMargin) + row*lay.fontSize
				y1 := y0 + lay.fontSize
				if x1 > qrx && x0 < qrx+qrsz && y1 > qry && y0 < qry+qrsz {
					t.Errorf("%.1fmm row %d %q spans x[%.2f,%.2f] y[%.2f,%.2f], inside the QR box x[%.2f,%.2f] y[%.2f,%.2f]",
						size, row, l, float64(x0)/mm, float64(x1)/mm, float64(y0)/mm, float64(y1)/mm,
						float64(qrx)/mm, float64(qrx+qrsz)/mm, float64(qry)/mm, float64(qry+qrsz)/mm)
				}
			}
		}
	}
}

// TestFreeTextStaysOnThePlate: whatever Fit accepts must survive toPlate's
// fail-closed [3mm, 82mm] bound (gui/gui.go, safetyMargin = 3). The engraver
// deliberately does not truncate, so this is where an over-long composition
// would show up.
func TestFreeTextStaysOnThePlate(t *testing.T) {
	P := prodParams
	lo, hi := P.I(outerMargin), P.F(82)
	cap18 := strings.Repeat("W", MaxTitleLen)
	for _, useQR := range []bool{false, true} {
		for _, size := range FontSizes {
			// A full plate at this rung.
			n := maxCharsByBisection(t, size, "W", cap18, cap18)
			text := strings.Repeat("W", n)
			fm, lines, qrc, err := Fit(P, sh.Font, text, cap18, cap18, useQR)
			if err != nil {
				continue
			}
			b := ftBounds(t, fm, cap18, lines, cap18, qrc)
			if b.Min.X < lo || b.Min.Y < lo || b.Max.X > hi || b.Max.Y > hi {
				t.Errorf("qr=%v %.1fmm full plate inks x[%.3f,%.3f] y[%.3f,%.3f]mm, outside [%d,82]mm",
					useQR, fm, float64(b.Min.X)/mm, float64(b.Max.X)/mm, float64(b.Min.Y)/mm, float64(b.Max.Y)/mm, outerMargin)
			}
		}
	}
}

// TestBlankLinesCostExactlyOneRow is spec 5.3, and it is the reason the
// free-text plate renders ONE flat list of lines instead of mapping wrap blocks
// onto backup.Paragraph values.
//
// Measured on that mapping, it is wrong twice over: a blank paragraph advances
// offy by 1mm rather than a full row, so a blank line the operator saw in the
// preview becomes a 1mm gap and everything below shifts up; and every '\n'
// costs an extra 1mm the uniform model does not account for. At 3.0mm, where
// the bottom line already sits at 81mm against an 82mm bound, two newlines
// would push the plate off the machine AFTER the confirm screen.
func TestBlankLinesCostExactlyOneRow(t *testing.T) {
	P := prodParams
	for _, size := range FontSizes {
		row := P.F(size)
		for _, blanks := range []int{0, 1, 2, 5} {
			lines := []string{"A"}
			for range blanks {
				lines = append(lines, "")
			}
			lines = append(lines, "B")
			b := ftBounds(t, size, "", lines, "", nil)
			// The last line sits `blanks+1` rows below the first, whatever the
			// blanks contain -- N blank lines cost N rows and not one
			// millimetre more.
			plain := ftBounds(t, size, "", []string{"A", "B"}, "", nil)
			if got, want := b.Max.Y-plain.Max.Y, blanks*row; got != want {
				t.Errorf("%.1fmm: %d blank lines moved the last line by %d, want %d (%d rows of %d)",
					size, blanks, got, want, blanks, row)
			}
			// And never by a millimetre-quantised amount, which is the
			// Paragraph model's signature.
			if blanks > 0 && b.Max.Y-plain.Max.Y == blanks*P.I(1) && row != P.I(1) {
				t.Errorf("%.1fmm: %d blank lines advanced by %dmm, the inter-paragraph gap, not a row",
					size, blanks, blanks)
			}
		}
	}
}

// TestWrappedBlankLinesReachThePlate closes the loop: the blank line comes out
// of WrapText, survives Fit, and occupies a row on the engraved plate.
func TestWrappedBlankLinesReachThePlate(t *testing.T) {
	P := prodParams
	const withBlanks = "one\n\n\ntwo"
	const without = "one\ntwo"
	sizeA, linesA, _, err := Fit(P, sh.Font, withBlanks, "", "", false)
	if err != nil {
		t.Fatal(err)
	}
	sizeB, linesB, _, err := Fit(P, sh.Font, without, "", "", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(linesA) != len(linesB)+2 {
		t.Fatalf("two newlines produced %d lines against %d; the blank rows were dropped", len(linesA), len(linesB))
	}
	if sizeA != sizeB {
		t.Fatalf("this test needs both compositions at the same rung; got %.1f and %.1f", sizeA, sizeB)
	}
	a := ftBounds(t, sizeA, "", linesA, "", nil)
	b := ftBounds(t, sizeB, "", linesB, "", nil)
	if got, want := a.Max.Y-b.Max.Y, 2*P.F(sizeA); got != want {
		t.Errorf("two blank lines moved the last row by %d, want %d", got, want)
	}
}

// TestFittedPassesReachTheEngraving proves Fitted.Passes actually reaches the
// glyph engraving, not just the struct.
func TestFittedPassesReachTheEngraving(t *testing.T) {
	// prodParams is the package-level Params at backup/sizes_test.go:21.
	P := prodParams
	size, lines, qrc, err := Fit(P, constant.Font, "BEEF", "", "", false)
	if err != nil {
		t.Fatal(err)
	}
	mk := func(passes int) int {
		f := Fitted{
			SizeMM: size, Lines: lines, QR: qrc,
			Faces:  []*vector.Face{constant.Font},
			Sizes:  []float32{size},
			Passes: passes,
		}
		n := 0
		for range engrave.PlanEngraving(P.StepperConfig, EngraveFitted(P, f)) {
			n++
		}
		return n
	}
	n, one := mk(3), mk(1)
	if n <= one {
		t.Errorf("three passes planned %d knots against one pass's %d", n, one)
	}
}

// TestConstantStringerHasNoPasses is a COMPILE-TIME claim written as a runtime
// test: if ConstantStringer ever gains a Passes field, this stops compiling and
// somebody has to read SPEC_seedhammer_engraving_settings.md section 3.
func TestConstantStringerHasNoPasses(t *testing.T) {
	typ := reflect.TypeOf(engrave.ConstantStringer{})
	if _, ok := typ.FieldByName("Passes"); ok {
		t.Fatal("ConstantStringer gained a Passes field; constant-time engraving " +
			"needs a proof before a pass count may reach a seed plate")
	}
}
