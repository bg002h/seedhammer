package backup

import (
	"slices"
	"strings"
	"testing"

	qrpkg "github.com/seedhammer/kortschak-qr"
	"seedhammer.com/font/sh"
)

// qrPlaceFor is a code's plate-absolute placement at a rung: the ONE object the
// fit resolves, anchored at the top margin, that the layout narrows against and
// the engraver draws from. nil for no code.
func qrPlaceFor(size float32, qrc *qrpkg.Code, scale int) *qrPlacement {
	if qrc == nil {
		return nil
	}
	P := prodParams
	p := qrPlaceAt(P, qrc, scale, P.F(size), P.I(outerMargin))
	return &p
}

// layAt builds the plate-absolute layout at a rung, with the given code.
func layAt(size float32, qrc *qrpkg.Code, scale int) (lineLayout, int) {
	P := prodParams
	return textLayout(P, sh.Font, P.F(size), P.I(outerMargin), qrPlaceFor(size, qrc, scale)), LinesPerPlate(P, size)
}

// bodyRows is the half-open range of plate rows the body may use. A title takes
// row 0 and a footer the last row -- but only when they exist, which is what
// makes spec 4's "Plain" column the full plate. Admission is the one place that
// reserves both unconditionally; see Admissible.
//
// It is a TEST helper and no longer production code. P3 gave the fit a y budget
// instead: a plate that mixes sizes has no row pitch, so yBudget states the same
// window in y and the wrap counts against that, and nothing in the package has
// asked the question in rows since. It survives here because the uniform-plate
// fixtures are written in the row-index form -- where the two are measured to
// agree at every rung and every title/footer combination -- and rewriting them
// in y would rewrite the very pins that hold P2 and P3 still. Left in fit.go it
// was dead code that read as a second, unused definition of the body window.
func bodyRows(rows int, title, footer string) (start, end int) {
	start, end = 0, rows
	if title != "" {
		start++
	}
	if footer != "" {
		end--
	}
	if end < start {
		end = start
	}
	return start, end
}

// TestInsetRowsPerRung pins spec 5.1: the screw-hole inset is a BAND, not two
// rows. At 3.0mm FIVE rows are inset, and a model that assumes "first and last"
// puts ink through a screw hole on three of them.
func TestInsetRowsPerRung(t *testing.T) {
	want := []struct {
		size      float32
		inset     []int
		holeChars int
	}{
		{6.0, []int{0, 1, 12}, 2},
		{5.0, []int{0, 1, 14}, 3},
		{4.4, []int{0, 1, 16}, 3},
		{3.8, []int{0, 1, 18, 19}, 4},
		{3.4, []int{0, 1, 2, 21, 22}, 4},
		{3.0, []int{0, 1, 2, 24, 25}, 4},
	}
	for _, w := range want {
		lay, rows := layAt(w.size, nil, freeTextQRScale)
		var inset []int
		for r := range rows {
			if _, offx := lay.at(r); offx != 0 {
				inset = append(inset, r)
			}
		}
		if !slices.Equal(inset, w.inset) {
			t.Errorf("%.1fmm inset rows = %v, want %v", w.size, inset, w.inset)
		}
		if lay.holeChars != w.holeChars {
			t.Errorf("%.1fmm holeChars = %d, want %d", w.size, lay.holeChars, w.holeChars)
		}
		// Every inset row loses holeChars on BOTH sides.
		for _, r := range inset {
			n, offx := lay.at(r)
			if want := lay.charPerLine - 2*lay.holeChars; n != want {
				t.Errorf("%.1fmm row %d budget = %d, want %d", w.size, r, n, want)
			}
			if want := lay.holeChars * lay.charWidth; offx != want {
				t.Errorf("%.1fmm row %d offx = %d, want %d", w.size, r, offx, want)
			}
		}
	}
}

// sumRows is the character capacity of rows [start, end).
func sumRows(lay lineLayout, start, end int) int {
	total := 0
	for r := start; r < end; r++ {
		n, _ := lay.at(r)
		total += n
	}
	return total
}

// TestCapacityColumns pins spec 4's table at the PRODUCTION stroke. charWidth
// comes from the font, so the Plain grid is the same at mm/3 -- but the QR
// columns are not, and a test written at mm/3 reproduces the same numbers with
// a 33-module QR the machine does not satisfy.
func TestCapacityColumns(t *testing.T) {
	// A hypothetical 37-module code. Spec 4 is explicit that this column is
	// GEOMETRY ONLY and not an attainable capacity: 37 modules is v5-L, at most
	// 106 bytes, and the QR encodes the Text.
	fake37 := &qrpkg.Code{Size: 37}
	want := []struct {
		size          float32
		plain, tf     int
		qrS2, qrS3    int
		rows, perLine int
	}{
		{6.0, 274, 238, 198, 161, 13, 22},
		{5.0, 372, 332, 278, 228, 15, 26},
		{4.4, 492, 444, 384, 309, 17, 30},
		{3.8, 648, 596, 519, 436, 20, 34},
		{3.4, 834, 774, 678, 576, 23, 38},
		{3.0, 1104, 1032, 897, 759, 26, 44},
	}
	for _, w := range want {
		lay, rows := layAt(w.size, nil, freeTextQRScale)
		if rows != w.rows || lay.charPerLine != w.perLine {
			t.Errorf("%.1fmm grid = %dx%d, want %dx%d", w.size, lay.charPerLine, rows, w.perLine, w.rows)
		}
		if got := sumRows(lay, 0, rows); got != w.plain {
			t.Errorf("%.1fmm plain capacity = %d, want %d", w.size, got, w.plain)
		}
		if got := sumRows(lay, 1, rows-1); got != w.tf {
			t.Errorf("%.1fmm +title+footer capacity = %d, want %d", w.size, got, w.tf)
		}
		for _, q := range []struct {
			scale int
			want  int
		}{{2, w.qrS2}, {3, w.qrS3}} {
			lay, rows := layAt(w.size, fake37, q.scale)
			if got := sumRows(lay, 1, rows-1); got != q.want {
				t.Errorf("%.1fmm +title+footer+QR(37) at scale %d = %d, want %d", w.size, q.scale, got, q.want)
			}
		}
	}
}

// TestProductionStrokeIsWhatWeMeasure: the guard against a capacity test
// silently drifting back to backup_test.go's mm/3 params.
func TestProductionStrokeIsWhatWeMeasure(t *testing.T) {
	if got, want := prodParams.StrokeWidth, int(0.3*mm); got != want {
		t.Fatalf("prodParams.StrokeWidth = %d, want %d (0.3mm, cmd/controller/platform_sh2.go)", got, want)
	}
	if prodParams.StrokeWidth == params.StrokeWidth {
		t.Fatal("prodParams and backup_test.go's params have the same stroke; the distinction this file rests on is gone")
	}
}

// TestQRSizeComesFromTheEncoder. qr.Encode is mode-adaptive, so a
// bytes-per-version table is simultaneously too pessimistic for uppercase and
// blind to the double step one keystroke can cause.
func TestQRSizeComesFromTheEncoder(t *testing.T) {
	tests := []struct {
		name string
		s    string
		want int
	}{
		{"106 lowercase", strings.Repeat("a", 106), 37},
		{"106 uppercase", strings.Repeat("A", 106), 33},
		{"106 digits", strings.Repeat("1", 106), 29},
		{"114 uppercase", strings.Repeat("A", 114), 33},
		// One keystroke, two version steps: the string leaves alphanumeric
		// mode and lands in byte mode.
		{"114 uppercase + one lowercase", strings.Repeat("A", 114) + "a", 41},
	}
	for _, tc := range tests {
		qrc, err := qrFor(tc.s, true)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if qrc.Size != tc.want {
			t.Errorf("%s: %d modules, want %d", tc.name, qrc.Size, tc.want)
		}
	}
	if qrc, err := qrFor("anything", false); qrc != nil || err != nil {
		t.Errorf("qrFor(want=false) = (%v, %v), want (nil, nil)", qrc, err)
	}
}

// TestQRModuleCountIsMonotone: capacity is only monotone if the module count
// never shrinks as characters are appended. Checked over a long run in both the
// alphanumeric and the byte case.
func TestQRModuleCountIsMonotone(t *testing.T) {
	for _, cls := range []struct{ name, ch string }{
		{"lowercase (byte mode)", "a"},
		{"uppercase (alphanumeric until it is not)", "A"},
	} {
		prev := 0
		for n := 1; n <= 400; n++ {
			qrc, err := qrFor(strings.Repeat(cls.ch, n), true)
			if err != nil {
				t.Fatalf("%s at %d: %v", cls.name, n, err)
			}
			if qrc.Size < prev {
				t.Errorf("%s: %d chars -> %d modules, down from %d at %d chars", cls.name, n, qrc.Size, prev, n-1)
			}
			prev = qrc.Size
		}
	}
}

// maxCharsByBisection finds the largest n at which n characters of ch still fit
// at `size` or larger. Bisection is sound because the fit predicate is monotone
// in n (TestQRModuleCountIsMonotone is half of why), and the boundary is
// verified explicitly rather than assumed.
func maxCharsByBisection(t *testing.T, size float32, ch, title, footer string) int {
	t.Helper()
	fits := func(n int) bool {
		fm, _, _, err := Fit(prodParams, sh.Font, strings.Repeat(ch, n), title, footer, true)
		return err == nil && fm >= size
	}
	lo, hi := 0, 2000
	if fits(hi) {
		t.Fatalf("%.1fmm: %d characters still fit; widen the search", size, hi)
	}
	for lo+1 < hi {
		mid := (lo + hi) / 2
		if fits(mid) {
			lo = mid
		} else {
			hi = mid
		}
	}
	if lo > 0 && !fits(lo) {
		t.Fatalf("%.1fmm: bisection landed on %d, which does not fit", size, lo)
	}
	if fits(lo + 1) {
		t.Fatalf("%.1fmm: bisection landed on %d but %d fits too", size, lo, lo+1)
	}
	return lo
}

// TestTrueMaximaAtEveryRung pins spec 4's second table, solved by ITERATION.
// The geometry column above it is unreachable -- 37 modules is at most 106
// bytes, and no text of that column's own length produces one.
//
// NOTE: the spec presents this table without saying which composition it
// measures. Measured, it is the +title+footer case; the plain figures are
// [214 268 322 408 474 564] / [220 279 357 450 535 667]. Both are pinned so the
// ambiguity cannot be resolved by accident later.
func TestTrueMaximaAtEveryRung(t *testing.T) {
	tests := []struct {
		title, footer  string
		lower, upper   []int
		compositionFor string
	}{
		{"T", "F", []int{178, 230, 284, 367, 429, 520}, []int{195, 255, 318, 398, 501, 616}, "+title+footer"},
		{"", "", []int{214, 268, 322, 408, 474, 564}, []int{220, 279, 357, 450, 535, 667}, "plain"},
	}
	for _, tc := range tests {
		for i, size := range FontSizes {
			if got := maxCharsByBisection(t, size, "a", tc.title, tc.footer); got != tc.lower[i] {
				t.Errorf("%s %.1fmm lowercase max = %d, want %d", tc.compositionFor, size, got, tc.lower[i])
			}
			if got := maxCharsByBisection(t, size, "A", tc.title, tc.footer); got != tc.upper[i] {
				t.Errorf("%s %.1fmm uppercase max = %d, want %d", tc.compositionFor, size, got, tc.upper[i])
			}
		}
	}
}

// TestAdmissionIsAnchoredAt3mm: spec 6. Admission must not move with the fitted
// size, or deleting a character could raise the rung and retroactively
// invalidate what was already accepted.
func TestAdmissionIsAnchoredAt3mm(t *testing.T) {
	// A text that fits at 3.0mm but at no larger rung: 900 characters is past
	// 3.4mm's plain capacity of 834 and inside 3.0mm's 1104.
	s := strings.Repeat("a", 900)
	if _, avail, ok := Admissible(prodParams, sh.Font, s, "", "", false); !ok {
		t.Fatalf("%d characters inadmissible with %d lines available", len(s), avail)
	}
	fm, _, _, err := Fit(prodParams, sh.Font, s, "", "", false)
	if err != nil {
		t.Fatal(err)
	}
	if fm != 3.0 {
		t.Fatalf("Fit chose %.1fmm; this test needs a text that only fits the smallest rung", fm)
	}

	// linesAvail reserves BOTH the title and the footer row unconditionally.
	rows := LinesPerPlate(prodParams, 3.0)
	if _, avail, _ := Admissible(prodParams, sh.Font, "x", "", "", false); avail != rows-2 {
		t.Errorf("linesAvail = %d, want %d (%d rows less the title and footer rows)", avail, rows-2, rows)
	}
}

// TestEnteringATitleNeverInvalidatesAcceptedText is the property the
// unconditional reservation exists for, checked at the boundary rather than in
// the middle: over a sweep of lengths, admission must not depend on whether a
// title or footer has been typed.
func TestEnteringATitleNeverInvalidatesAcceptedText(t *testing.T) {
	for _, useQR := range []bool{false, true} {
		for n := 1; n <= 1200; n += 7 {
			s := strings.Repeat("a", n)
			bare, availBare, okBare := Admissible(prodParams, sh.Font, s, "", "", useQR)
			full, availFull, okFull := Admissible(prodParams, sh.Font, s, "A Title Here", "and a footer", useQR)
			if okBare != okFull || bare != full || availBare != availFull {
				t.Fatalf("qr=%v n=%d: admission changed when a title was entered: (%d/%d %v) -> (%d/%d %v)",
					useQR, n, bare, availBare, okBare, full, availFull, okFull)
			}
		}
	}
}

// TestRefusalFigureIsComputedLive pins spec 11's number. Reading the QR size
// from spec 4's geometry column would suggest about 135; the true figure for a
// 700-character text is 640, because that text needs an 89-module code, not a
// 37-module one.
func TestRefusalFigureIsComputedLive(t *testing.T) {
	const s = 700
	text := strings.Repeat("a", s)
	withQR := MaxCharsAt(prodParams, sh.Font, 3.0, text, true)
	noQR := MaxCharsAt(prodParams, sh.Font, 3.0, text, false)
	if freed := noQR - withQR; freed != 640 {
		t.Errorf("dropping the QR frees %d characters (%d -> %d), want 640", freed, withQR, noQR)
	}
	if noQR != 1104 {
		t.Errorf("MaxCharsAt without a QR = %d, want spec 4's plain column, 1104", noQR)
	}
	// And the figure a length table would have produced, so the difference is
	// not a coincidence of arithmetic.
	fake37 := &qrpkg.Code{Size: 37}
	lay, rows := layAt(3.0, fake37, freeTextQRScale)
	if got := sumRows(lay, 0, rows); got != 969 {
		t.Errorf("plain + 37-module QR = %d, want spec 6's 969", got)
	}
	if withQR == 969 {
		t.Error("MaxCharsAt is reading a fixed 37-module code, not encoding the text")
	}
}

// TestUnencodableTextIsHandledEverywhere. qr.Encode fails at 2954 bytes and the
// Text field is uncapped, so this input is reachable from the keyboard. None of
// the three entry points may panic, and none may hand out a nil *qr.Code as if
// it were a code.
func TestUnencodableTextIsHandledEverywhere(t *testing.T) {
	huge := strings.Repeat("a", 2954)
	if _, err := qrFor(huge, true); err == nil {
		t.Fatalf("qr.Encode accepted %d bytes; this test needs an input it refuses", len(huge))
	}
	if _, err := qrFor(strings.Repeat("a", 2953), true); err != nil {
		t.Fatalf("qr.Encode refused 2953 bytes (%v); the boundary has moved", err)
	}

	if _, _, qrc, err := Fit(prodParams, sh.Font, huge, "", "", true); err == nil || qrc != nil {
		t.Errorf("Fit(unencodable) = (%v, %v), want (nil, error)", qrc, err)
	}
	used, avail, ok := Admissible(prodParams, sh.Font, huge, "", "", true)
	if ok {
		t.Error("Admissible(unencodable) = true")
	}
	if avail != LinesPerPlate(prodParams, 3.0)-2 {
		t.Errorf("linesAvail = %d on an encode failure; the readout must keep working", avail)
	}
	_ = used
	if got := MaxCharsAt(prodParams, sh.Font, 3.0, huge, true); got != 0 {
		t.Errorf("MaxCharsAt(unencodable) = %d, want 0", got)
	}
	// Without a QR the same text is merely too long, not unencodable.
	if got := MaxCharsAt(prodParams, sh.Font, 3.0, huge, false); got != 1104 {
		t.Errorf("MaxCharsAt(unencodable, no QR) = %d, want 1104", got)
	}
}

// TestFitReturnsTheCodeItMeasured: one encode per composition. If Fit handed
// back only a size, the builder would have to encode again, and a second encode
// is a second chance to disagree.
func TestFitReturnsTheCodeItMeasured(t *testing.T) {
	const text = "the quick brown fox jumps over the lazy dog"
	_, _, qrc, err := Fit(prodParams, sh.Font, text, "", "", true)
	if err != nil {
		t.Fatal(err)
	}
	if qrc == nil {
		t.Fatal("Fit returned no code with qr=true")
	}
	want, err := qrpkg.Encode(text, qrpkg.L)
	if err != nil {
		t.Fatal(err)
	}
	if qrc.Size != want.Size || !slices.Equal(qrc.Bitmap, want.Bitmap) {
		t.Error("Fit's code is not qr.Encode(text, qr.L)")
	}
	if _, _, qrc, err := Fit(prodParams, sh.Font, text, "", "", false); qrc != nil || err != nil {
		t.Errorf("Fit(qr=false) = (%v, %v), want (nil, nil)", qrc, err)
	}
}

// TestFitPicksTheLargestRungThatHolds walks the ladder rather than trusting it.
func TestFitPicksTheLargestRungThatHolds(t *testing.T) {
	for i, size := range FontSizes {
		n := maxCharsByBisection(t, size, "a", "", "")
		fm, lines, _, err := Fit(prodParams, sh.Font, strings.Repeat("a", n), "", "", true)
		if err != nil {
			t.Fatalf("%.1fmm: %d characters refused", size, n)
		}
		if fm != size {
			t.Errorf("%.1fmm: %d characters fitted at %.1fmm, want exactly %.1fmm", size, n, fm, size)
		}
		if got := len(strings.Join(lines, "")); got != n {
			t.Errorf("%.1fmm: fitted lines carry %d characters, want %d", size, got, n)
		}
		// One more character must drop a rung -- or be refused at the last.
		fm2, _, _, err := Fit(prodParams, sh.Font, strings.Repeat("a", n+1), "", "", true)
		if i == len(FontSizes)-1 {
			if err == nil {
				t.Errorf("%d characters accepted past the smallest rung (at %.1fmm)", n+1, fm2)
			}
		} else if err != nil || fm2 >= size {
			t.Errorf("%.1fmm: %d characters gave (%.1fmm, %v), want a smaller rung", size, n+1, fm2, err)
		}
	}
}

// TestFitRefusesRatherThanSplits: spec 6, user directive. Text that does not
// fit one plate is refused; nothing is silently dropped or continued.
func TestFitRefusesRatherThanSplits(t *testing.T) {
	_, lines, _, err := Fit(prodParams, sh.Font, strings.Repeat("a", 1200), "", "", false)
	if err != ErrTooLarge {
		t.Errorf("err = %v, want ErrTooLarge", err)
	}
	if lines != nil {
		t.Errorf("a refused fit returned %d lines; a partial result must not be engravable", len(lines))
	}
}

// TestPlateBoundsAtEveryRung pins spec 4.1. toPlate is fail-closed against
// [3mm, 82mm] (gui/gui.go, safetyMargin = 3), and the ladder has only 0.2mm of
// horizontal headroom at 6.0mm -- tight, but real, and worth knowing if a font
// metric ever changes.
func TestPlateBoundsAtEveryRung(t *testing.T) {
	const bound = 82
	P := prodParams
	worstX, worstY := 0, 0
	for _, size := range FontSizes {
		lay, rows := layAt(size, nil, freeTextQRScale)
		right := P.I(outerMargin) + lay.charPerLine*lay.charWidth
		bottom := P.I(outerMargin) + rows*lay.fontSize
		if right > P.F(bound) {
			t.Errorf("%.1fmm widest line reaches %.3fmm, past the %dmm bound", size, float64(right)/mm, bound)
		}
		if bottom > P.F(bound) {
			t.Errorf("%.1fmm bottom edge reaches %.3fmm, past the %dmm bound", size, float64(bottom)/mm, bound)
		}
		worstX = max(worstX, right)
		worstY = max(worstY, bottom)
	}
	// Pin the measured worst cases so shrinking headroom is visible, not silent.
	if got := float64(worstX) / mm; got < 81.7 || got > 81.9 {
		t.Errorf("widest line across the ladder = %.3fmm, spec 4.1 measured 81.8mm", got)
	}
	if got := float64(worstY) / mm; got < 81.1 || got > 81.3 {
		t.Errorf("lowest bottom edge across the ladder = %.3fmm, spec 4.1 measured 81.2mm", got)
	}
}

// TestFitLinesMatchWrapText: Fit must not have its own idea of the lines. This
// is the fit-check third of spec 5's single-wrap-function invariant.
func TestFitLinesMatchWrapText(t *testing.T) {
	const text = "hello there this is a note to an heir\nkeep it safe and do not photograph it"
	size, lines, qrc, err := Fit(prodParams, sh.Font, text, "TITLE", "FOOTER", true)
	if err != nil {
		t.Fatal(err)
	}
	lay, rows := layAt(size, qrc, freeTextQRScale)
	start, end := bodyRows(rows, "TITLE", "FOOTER")
	want, ok := WrapText(text, widthFor(lay, start), end-start)
	if !ok {
		t.Fatal("the composition Fit accepted does not wrap")
	}
	if !slices.Equal(lines, want) {
		t.Errorf("Fit's lines differ from WrapText's:\n got %q\nwant %q", lines, want)
	}
}

// TestBodyRowsReserveOnlyWhatIsUsed. Spec 4's Plain column is the whole plate,
// so an absent title must not cost a row -- while admission (above) reserves
// both regardless.
func TestBodyRowsReserveOnlyWhatIsUsed(t *testing.T) {
	tests := []struct {
		title, footer string
		start, end    int
	}{
		{"", "", 0, 26},
		{"T", "", 1, 26},
		{"", "F", 0, 25},
		{"T", "F", 1, 25},
	}
	for _, tc := range tests {
		if s, e := bodyRows(26, tc.title, tc.footer); s != tc.start || e != tc.end {
			t.Errorf("bodyRows(26, %q, %q) = (%d, %d), want (%d, %d)", tc.title, tc.footer, s, e, tc.start, tc.end)
		}
	}
	// Degenerate plate: never a negative span.
	if s, e := bodyRows(1, "T", "F"); s > e {
		t.Errorf("bodyRows(1, T, F) = (%d, %d), an inverted range", s, e)
	}
}

// TestFitQREncodesTheTextOnly is spec 2, asserted where qrFor is visible and at
// MODULE level: a decoder that ignores trailing data would return the text and
// pass while the modules differed. The title and footer must leave no trace in
// the code, whatever they are.
func TestFitQREncodesTheTextOnly(t *testing.T) {
	const text = "the note that goes on the plate"
	want, err := qrpkg.Encode(text, qrpkg.L)
	if err != nil {
		t.Fatal(err)
	}
	for _, tf := range [][2]string{
		{"", ""},
		{"A TITLE", ""},
		{"", "A FOOTER"},
		{"A TITLE", "A FOOTER"},
		{strings.Repeat("W", MaxTitleLen), strings.Repeat("m", MaxTitleLen)},
	} {
		_, _, got, err := Fit(prodParams, sh.Font, text, tf[0], tf[1], true)
		if err != nil {
			t.Fatalf("title=%q footer=%q: %v", tf[0], tf[1], err)
		}
		if got == nil {
			t.Fatalf("title=%q footer=%q: no code", tf[0], tf[1])
		}
		if got.Size != want.Size || !slices.Equal(got.Bitmap, want.Bitmap) {
			t.Errorf("title=%q footer=%q changed the code: %d modules vs %d", tf[0], tf[1], got.Size, want.Size)
		}
	}
	// The assertion is not vacuous: a code over the concatenation differs.
	other, err := qrpkg.Encode(text+"A TITLE"+"A FOOTER", qrpkg.L)
	if err != nil {
		t.Fatal(err)
	}
	if slices.Equal(other.Bitmap, want.Bitmap) {
		t.Fatal("text and text+title+footer encode identically; this test proves nothing")
	}
}
