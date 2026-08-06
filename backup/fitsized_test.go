package backup

import (
	"math"
	"strings"
	"testing"

	"seedhammer.com/bspline"
	"seedhammer.com/font/constant"
	"seedhammer.com/font/sh"
	"seedhammer.com/font/vector"
)

// insetFor is the left inset of a screw-hole row in fnt at sizeMM, derived from
// the FACE and the margins rather than read off a lineLayout. Reading
// holeChars*charWidth back off the layout under test would agree with itself
// whatever size that layout was built at, which is exactly the defect spec 7.6
// names.
func insetFor(fnt *vector.Face, sizeMM float32) int {
	P := prodParams
	cw := fixedCharWidth(fnt, P.F(sizeMM))
	return int(math.Ceil(float64(P.I(innerMargin)-P.I(outerMargin))/float64(cw))) * cw
}

// plainInk is where s inks when it is cut in fnt at sizeMM on a row that is in
// NEITHER screw-hole band: the plate margin plus the glyph's own left bearing,
// and no inset at all.
//
// It exists so a row's inset can be MEASURED rather than inferred. The bearing
// is a property of (s, fnt, sizeMM) and cancels when the same three are cut
// twice, so the difference between a row's ink and this one is the inset the
// engraver applied and nothing else.
func plainInk(t testing.TB, s string, fnt *vector.Face, sizeMM float32) bspline.Bounds {
	t.Helper()
	P := prodParams
	// Two 6.0mm blanks put the row at y = margin + 2*38400 = 96000, below the
	// 64000 top band and far above the 480000 bottom one, whatever rung s is
	// cut at.
	y := P.I(outerMargin) + 2*P.F(6.0)
	if y < P.I(innerMargin) || y+P.F(sizeMM) > P.F(plateSize)-P.I(innerMargin) {
		t.Fatalf("the reference row sits at y=%d, inside a screw-hole band; it is not a plain row", y)
	}
	return inkBounds(t, P, EngraveFitted(P, Fitted{
		Sizes:     []float32{6.0, 6.0, sizeMM},
		Lines:     []string{"", "", s},
		Faces:     []*vector.Face{fnt, fnt, fnt},
		TitleFace: fnt, FooterFace: fnt,
	}))
}

// insetOfLine is the left inset the engraver actually applied to line i of f,
// in device units, measured against the same line cut on a plain row.
//
// f must carry no title and no footer: blanking either would move the body
// (the title sets the wrap's start) and leaving one would put its centred ink
// in the bounds beside the row being measured.
func insetOfLine(t testing.TB, f Fitted, i int) int {
	t.Helper()
	P := prodParams
	if f.Title != "" || f.Footer != "" {
		t.Fatalf("insetOfLine measures a bare plate; this one carries %q / %q", f.Title, f.Footer)
	}
	// Every other line blank, so the measured ink IS line i's.
	g := f
	g.Lines = make([]string, len(f.Lines))
	g.Lines[i] = f.Lines[i]
	got := inkBounds(t, P, EngraveFitted(P, g))
	return got.Min.X - plainInk(t, f.Lines[i], f.Faces[i], f.Sizes[i]).Min.X
}

// TestFitSizedInsetsABlockInsideTheTopBandAndNotOneBelowIt is spec 7.5.
//
// Whether a row clears the screw holes is a question about the row's own y, and
// on a plate that mixes sizes a block's y is the running sum of the sizes above
// it -- not its index times a pitch the plate does not have. The natural
// rewrite of the engraver's per-row inset builds every row's layout at
// baseY = outerMargin, which makes row 0 of that layout a top-band row for
// EVERY block: the block below the band is then inset too, by an inset it has
// no business taking, and no assertion on the sizes, the lines or the code can
// see it.
//
// Block 1 lies wholly inside the top band and block 2 begins below it, both
// asserted rather than assumed.
func TestFitSizedInsetsABlockInsideTheTopBandAndNotOneBelowIt(t *testing.T) {
	P := prodParams
	margin, inner := P.I(outerMargin), P.I(innerMargin)
	const (
		bigMM   = 5.0
		smallMM = 3.0
		band    = 2 // MEASURED: rows at y = 19200 and 51200, both under 64000
		below   = 3
	)
	y2 := margin + band*P.F(bigMM)
	if last := margin + (band-1)*P.F(bigMM); last >= inner {
		t.Fatalf("block 1's last row starts at y=%d, outside the %d top band; the block is not "+
			"wholly inside it", last, inner)
	}
	if y2 < inner {
		t.Fatalf("block 2 starts at y=%d, still inside the %d top band", y2, inner)
	}
	if y2+below*P.F(smallMM) > P.F(plateSize)-inner {
		t.Fatalf("block 2 reaches the bottom band; this fixture is about the TOP one")
	}

	blocks := []Block{
		{Face: sh.Font, Text: fillAt("a", sh.Font, bigMM, margin, band), SizeMM: bigMM},
		{Face: constant.Font, Text: fillAt("b", constant.Font, smallMM, y2, below), SizeMM: smallMM},
	}
	f, err := FitSized(P, blocks, "", "", 0, 0)
	if err != nil {
		t.Fatalf("the composition was refused: %v", err)
	}
	if got := len(f.Lines); got != band+below {
		t.Fatalf("the composition wrapped to %d rows, want %d; the fill is not sized to the "+
			"budgets it is measured against", got, band+below)
	}

	// The wrong answer has to exist, or "is inset" is satisfied by every plate.
	want := insetFor(sh.Font, bigMM)
	if want == 0 {
		t.Fatal("font/sh at 5.0mm insets a screw-hole row by 0; there is no wrong answer to give")
	}
	for i := range band {
		if got := insetOfLine(t, f, i); got != want {
			t.Errorf("row %d of the block inside the top band is inset by %d, want %d",
				i, got, want)
		}
	}
	if got := insetOfLine(t, f, band); got != 0 {
		t.Errorf("the first row of the block BELOW the top band is inset by %d, want 0; its band "+
			"membership is being decided at the plate's top margin rather than at the block's own y", got)
	}

	// And in absolute terms: the inset block clears the screw holes, the one
	// below it does not have to. Both halves fail together if the band is
	// ignored outright, which the relative measurement above cannot see.
	one := f
	one.Lines = make([]string, len(f.Lines))
	one.Lines[0] = f.Lines[0]
	if got := inkBounds(t, P, EngraveFitted(P, one)).Min.X; got < inner {
		t.Errorf("the top-band row inks from x=%d, inside the %d screw-hole margin", got, inner)
	}
	one.Lines = make([]string, len(f.Lines))
	one.Lines[band] = f.Lines[band]
	if got := inkBounds(t, P, EngraveFitted(P, one)).Min.X; got >= inner {
		t.Errorf("the row below the band inks from x=%d, past the %d screw-hole margin; it is being "+
			"inset as though it were in the band", got, inner)
	}
}

// TestFitSizedCutsOneFaceAtTwoSizesOnItsOwnGrid is spec 7.6, and it is C4's
// pin. The front ladder is sh@5.0, const@5.0, sh@3.8, const@3.8: one FACE at
// two SIZES on one plate. The cache this replaced keyed on the face alone and
// hardcoded baseY = outerMargin, so the second run of a face got the first
// run's character grid -- a budget measured at the wrong pitch and a left inset
// measured at the wrong character width, on rows whose band membership was
// decided at the wrong fontSize.
//
// Both halves are asserted: the per-row BUDGET each block wrapped to, and the
// left INSET each screw-hole row was engraved with. The two sizes must disagree
// about both, which is checked rather than assumed.
func TestFitSizedCutsOneFaceAtTwoSizesOnItsOwnGrid(t *testing.T) {
	P := prodParams
	margin := P.I(outerMargin)
	const (
		bigMM    = 5.0
		smallMM  = 3.0
		bigRows  = 3
		smallRow = 21
	)
	y2 := margin + bigRows*P.F(bigMM)

	blocks := []Block{
		{Face: sh.Font, Text: fillAt("a", sh.Font, bigMM, margin, bigRows), SizeMM: bigMM},
		{Face: sh.Font, Text: fillAt("b", sh.Font, smallMM, y2, smallRow), SizeMM: smallMM},
	}
	if blocks[0].Face != blocks[1].Face {
		t.Fatal("both blocks must be the SAME face; this item is about one face at two sizes")
	}
	f, err := FitSized(P, blocks, "", "", 0, 0)
	if err != nil {
		t.Fatalf("the composition was refused: %v", err)
	}
	if got := len(f.Lines); got != bigRows+smallRow {
		t.Fatalf("the composition wrapped to %d rows, want %d -- a block wrapped to the other "+
			"block's grid takes a different number of rows", got, bigRows+smallRow)
	}
	if len(f.Sizes) != len(f.Lines) {
		t.Fatalf("%d lines but %d sizes", len(f.Lines), len(f.Sizes))
	}

	// The grid. MEASURED at 0.3mm stroke: font/sh holds 20 characters on each
	// of the two 5.0mm rows inside the top band and 26 on the plain one, then
	// 44 on each plain 3.0mm row and 36 on each of the two in the bottom band.
	// Written down here, not read off the fit: the row lengths are what a block
	// wrapped at the other block's size gets wrong.
	wantBudget := make([]int, 0, bigRows+smallRow)
	wantBudget = append(wantBudget, 20, 20, 26)
	for range smallRow - 2 {
		wantBudget = append(wantBudget, 44)
	}
	wantBudget = append(wantBudget, 36, 36)
	for i, l := range f.Lines {
		wantSize := float32(bigMM)
		if i >= bigRows {
			wantSize = smallMM
		}
		if f.Sizes[i] != wantSize {
			t.Errorf("row %d is sized %.1fmm, want %.1fmm", i, f.Sizes[i], wantSize)
		}
		if len(l) != wantBudget[i] {
			t.Errorf("row %d is %d characters, want exactly %d -- it was wrapped to the grid of "+
				"the other size", i, len(l), wantBudget[i])
		}
	}

	// The inset. Same face, two sizes, two different insets -- so a row cut at
	// the wrong size takes a visibly wrong one.
	bigInset, smallInset := insetFor(sh.Font, bigMM), insetFor(sh.Font, smallMM)
	if bigInset == smallInset {
		t.Fatalf("font/sh insets a screw-hole row by %d at both %.1fmm and %.1fmm; this test "+
			"cannot see a grid taken at the wrong size", bigInset, float32(bigMM), float32(smallMM))
	}
	// Row 0 is in the TOP band, at 5.0mm.
	if got := insetOfLine(t, f, 0); got != bigInset {
		t.Errorf("the first %.1fmm row is inset by %d, want %d (%.1fmm's inset is %d)",
			float32(bigMM), got, bigInset, float32(smallMM), smallInset)
	}
	// The block's last two rows are in the BOTTOM band, at 3.0mm.
	for _, i := range []int{bigRows + smallRow - 2, bigRows + smallRow - 1} {
		y := y2 + (i-bigRows)*P.F(smallMM)
		if y+P.F(smallMM) <= P.F(plateSize)-P.I(innerMargin) {
			t.Fatalf("row %d ends at y=%d, above the bottom band; this fixture is not the plate it names",
				i, y+P.F(smallMM))
		}
		if got := insetOfLine(t, f, i); got != smallInset {
			t.Errorf("the %.1fmm row %d is inset by %d, want %d (%.1fmm's inset is %d) -- its grid "+
				"was taken at the other size", float32(smallMM), i, got, smallInset, float32(bigMM), bigInset)
		}
	}
}

// TestEngraveFittedDoesNotPanicOnAMixedPlate is spec 7.9, and it is C5's pin.
// SizeMM is valid only when !Mixed, so a mixed plate carries 0 there -- and
// params.F(0) is 0, which makes LinesPerPlate a divide by zero and
// fixedCharWidth return a 0 the width division then divides by. Both arrive
// inside EngraveFitted, which is reached only AFTER the confirm screen: a panic
// there is a crash mid-flow with a plate clamped in the machine.
//
// The plate carries a title and a footer at sizes of their own, because the
// title is the one row whose size an engraver reaching for the nearest constant
// would take from SizeMM.
func TestEngraveFittedDoesNotPanicOnAMixedPlate(t *testing.T) {
	P := prodParams
	const (
		titleMM  = 3.0
		footerMM = 3.8
		bigMM    = 5.0
		smallMM  = 3.0
	)
	start, limit := yBudget(P, "TITLE", "FOOTER", titleMM, footerMM)
	const bigRows, smallRows = 3, 10
	y2 := start + bigRows*P.F(bigMM)
	blocks := []Block{
		{Face: sh.Font, Text: fillAt("a", sh.Font, bigMM, start, bigRows), SizeMM: bigMM},
		{Face: constant.Font, Text: fillAt("b", constant.Font, smallMM, y2, smallRows), SizeMM: smallMM},
	}
	f, err := FitSized(P, blocks, "TITLE", "FOOTER", titleMM, footerMM)
	if err != nil {
		t.Fatalf("the composition was refused: %v", err)
	}
	// The plate must really mix, or "does not panic" is a fact about a uniform
	// one and SizeMM is never 0.
	if !f.Mixed {
		t.Fatal("the fixture came out uniform; a plate that does not mix cannot reach C5")
	}
	if f.SizeMM != 0 {
		t.Fatalf("a mixed plate reports SizeMM = %.1fmm; 0 is what the engraver must not divide by",
			f.SizeMM)
	}

	// No recover: a panic here IS the failure, and it is the failure this item
	// exists to catch.
	knots := blockSpline(t, EngraveFitted(P, f))
	if len(knots) == 0 {
		t.Fatal("the mixed plate planned no strokes; a lazily-unrun engraving cannot panic either")
	}
	// The title and the footer are actually cut, at their own sizes -- so the
	// assertion above is not passing because centerInset returned early.
	b := inkBounds(t, P, EngraveFitted(P, f))
	if b.Min.Y >= start {
		t.Errorf("the plate's topmost ink is at y=%d, at or below the body's start %d; the title "+
			"is not being engraved", b.Min.Y, start)
	}
	if b.Max.Y <= limit {
		t.Errorf("the plate's lowest ink is at y=%d, above the footer's row %d; the footer is not "+
			"being engraved", b.Max.Y, limit)
	}
}

// TestFitSizedRefusesASizeStringMismatch is spec 7.14's first half, and it is
// spec 2.3's invariant on the entry point that can still refuse: TitleSizeMM is
// non-zero EXACTLY when Title is non-empty, and likewise the footer.
//
// P1 landed the EngraveFitted half as a panic. This is the matching error
// return, and the distinction is the whole point: EngraveFitted is reached only
// after the operator has approved the plate, so the same refusal arriving there
// arrives too late to be a refusal.
func TestFitSizedRefusesASizeStringMismatch(t *testing.T) {
	P := prodParams
	blocks := []Block{{Face: sh.Font, Text: "a note", SizeMM: 3.0}}
	for _, tc := range []struct {
		name                  string
		title, footer         string
		titleSize, footerSize float32
		wantErrDetail         string
	}{
		{name: "a title with a zero size", title: "TITLE", titleSize: 0, wantErrDetail: "title"},
		{name: "a size with no title", title: "", titleSize: 3.0, wantErrDetail: "title"},
		{name: "a footer with a zero size", footer: "FOOTER", footerSize: 0, wantErrDetail: "footer"},
		{name: "a size with no footer", footer: "", footerSize: 3.0, wantErrDetail: "footer"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, err := FitSized(P, blocks, tc.title, tc.footer, tc.titleSize, tc.footerSize)
			if err == nil {
				t.Fatalf("accepted, returning a plate of %d rows", len(f.Lines))
			}
			// WHICH guard refused is asserted, not merely that one did: an
			// error from the wrap or the rung check would answer for this one
			// and the invariant would stop being tested.
			if !strings.Contains(err.Error(), tc.wantErrDetail) {
				t.Errorf("refused with %q, which does not name the %s", err, tc.wantErrDetail)
			}
		})
	}
	// And all four consistent combinations are accepted, or the guard above is
	// satisfied by a function that refuses everything.
	for _, tc := range []struct {
		title, footer         string
		titleSize, footerSize float32
	}{
		{"", "", 0, 0},
		{"TITLE", "", 3.0, 0},
		{"", "FOOTER", 0, 3.0},
		{"TITLE", "FOOTER", 3.0, 3.8},
	} {
		if _, err := FitSized(P, blocks, tc.title, tc.footer, tc.titleSize, tc.footerSize); err != nil {
			t.Errorf("title %q@%.1f footer %q@%.1f was refused: %v",
				tc.title, tc.titleSize, tc.footer, tc.footerSize, err)
		}
	}
}

// TestFitSizedNoFooterPlateRunsToTheBottomMargin is spec 7.14's second half.
//
// Spec 5 mandates a plate with NO footer for both ladder sides, and the budget
// formula the earlier drafts handed over reads the footer's own row whether or
// not there is one -- LinesPerPlate(params, 0) is height/0. Stated as the
// OBSERVABLE outcome the item asks for: the limit is the bottom margin, and the
// plate lays out and engraves. "LinesPerPlate is never called with 0" is not a
// thing a Go expression can assert.
func TestFitSizedNoFooterPlateRunsToTheBottomMargin(t *testing.T) {
	P := prodParams
	const titleMM = 3.0
	start, limit := yBudget(P, "TITLE", "", titleMM, 0)
	if want := P.I(outerMargin) + P.F(titleMM); start != want {
		t.Errorf("a titled plate starts its body at %d, want the margin plus the title's own row %d",
			start, want)
	}
	if want := P.F(plateSize) - P.I(outerMargin); limit != want {
		t.Errorf("a plate with no footer ends its body at %d, want the bottom margin %d", limit, want)
	}
	// The footer's own row is a DIFFERENT number, or "the bottom margin" is
	// satisfied by a formula that never branched.
	if _, withFooter := yBudget(P, "TITLE", "FOOTER", titleMM, 3.8); withFooter == limit {
		t.Fatalf("a plate with a 3.8mm footer ends at the same %d; this test cannot tell the two "+
			"branches apart", limit)
	}

	// The whole budget, used: the last row's bottom lands exactly on the limit.
	const bigMM, smallMM = 5.0, 3.0
	const bigRows = 3
	y2 := start + bigRows*P.F(bigMM)
	smallRows := (limit - y2) / P.F(smallMM)
	if smallRows < 1 {
		t.Fatalf("%d rows of %.1fmm fit below the first block; the fixture proves nothing",
			smallRows, float32(smallMM))
	}
	blocks := []Block{
		{Face: sh.Font, Text: fillAt("a", sh.Font, bigMM, start, bigRows), SizeMM: bigMM},
		{Face: constant.Font, Text: fillAt("b", constant.Font, smallMM, y2, smallRows), SizeMM: smallMM},
	}
	f, err := FitSized(P, blocks, "TITLE", "", titleMM, 0)
	if err != nil {
		t.Fatalf("a composition filling the no-footer budget was refused: %v", err)
	}
	if got, want := len(f.Lines), bigRows+smallRows; got != want {
		t.Fatalf("the composition wrapped to %d rows, want %d", got, want)
	}
	if f.Footer != "" || f.FooterSizeMM != 0 {
		t.Errorf("a plate asked for no footer came back with %q at %.1fmm", f.Footer, f.FooterSizeMM)
	}
	// No recover: the divide by zero this item guards is a panic, so a panic
	// here IS the failure.
	if knots := blockSpline(t, EngraveFitted(P, f)); len(knots) == 0 {
		t.Fatal("the no-footer plate planned no strokes")
	}
	// One more row is refused, so the budget really did reach the bottom margin
	// rather than stopping short of it.
	over := []Block{
		blocks[0],
		{Face: constant.Font, Text: fillAt("b", constant.Font, smallMM, y2, smallRows+1), SizeMM: smallMM},
	}
	if _, err := FitSized(P, over, "TITLE", "", titleMM, 0); err == nil {
		t.Error("a composition one row past the bottom margin was accepted")
	}
}

// TestFitSizedReportsTheCommonSizeWhenThereIsOne: Mixed is a MEASUREMENT of the
// composition, never a constant.
//
// Every ladder composition mixes, so hardcoding Mixed = true and SizeMM = 0
// passes every fixture this change ships -- and puts "0.0mm" on the confirm
// screen for the all-one-rung composition FitSized is a public entry point for.
// Spec 2.3's rule counts the title and the footer too, so !Mixed means
// literally every glyph on the plate is one size, which is what makes SizeMM
// printable.
func TestFitSizedReportsTheCommonSizeWhenThereIsOne(t *testing.T) {
	P := prodParams
	one := func(size float32) []Block {
		return []Block{
			{Face: sh.Font, Text: "the first block", SizeMM: size},
			{Face: constant.Font, Text: "the second block", SizeMM: size},
		}
	}
	for _, tc := range []struct {
		name                  string
		blocks                []Block
		title, footer         string
		titleSize, footerSize float32
		wantMixed             bool
		wantSize              float32
	}{
		{name: "one rung, bare", blocks: one(3.0), wantSize: 3.0},
		{name: "one rung, title and footer at the same rung",
			blocks: one(3.0), title: "T", footer: "F", titleSize: 3.0, footerSize: 3.0, wantSize: 3.0},
		{name: "one rung at 5.0", blocks: one(5.0), wantSize: 5.0},
		{name: "two rungs in the body", blocks: []Block{
			{Face: sh.Font, Text: "the first block", SizeMM: 5.0},
			{Face: constant.Font, Text: "the second block", SizeMM: 3.0},
		}, wantMixed: true},
		{name: "a smaller title over a one-rung body",
			blocks: one(5.0), title: "T", titleSize: 3.0, wantMixed: true},
		{name: "a larger footer under a one-rung body",
			blocks: one(3.0), footer: "F", footerSize: 3.8, wantMixed: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, err := FitSized(P, tc.blocks, tc.title, tc.footer, tc.titleSize, tc.footerSize)
			if err != nil {
				t.Fatal(err)
			}
			if f.Mixed != tc.wantMixed {
				t.Errorf("Mixed = %v, want %v", f.Mixed, tc.wantMixed)
			}
			if f.SizeMM != tc.wantSize {
				t.Errorf("SizeMM = %.1fmm, want %.1fmm", f.SizeMM, tc.wantSize)
			}
			// SizeMM is readable exactly when the plate does not mix -- the
			// pairing is the property, not either half alone.
			if (f.SizeMM == 0) != f.Mixed {
				t.Errorf("SizeMM = %.1fmm with Mixed = %v; a mixed plate has no printable size and "+
					"a uniform one always has", f.SizeMM, f.Mixed)
			}
		})
	}
}

// TestFitSizedValidatesTheCompositionBeforeLayingItOut covers the rest of spec
// 2.7's validation. Every one of these is a refusal at the FIT, where the
// operator has approved nothing yet.
func TestFitSizedValidatesTheCompositionBeforeLayingItOut(t *testing.T) {
	P := prodParams
	// An empty composition, as FitBlocksAt refuses it.
	if _, err := FitSized(P, nil, "", "", 0, 0); err == nil {
		t.Error("an empty block list was accepted")
	}
	// A block with no size of its own. FitSized is the one entry point that
	// resolves Block.SizeMM, so the zero that means "the size the plate is
	// fitted at" everywhere else has no meaning here -- and a zero reaching
	// Fitted.Sizes is spec 2.3's per-entry invariant, which P1 landed in
	// EngraveFitted as a panic. This is the matching error.
	f, err := FitSized(P, []Block{
		{Face: sh.Font, Text: "sized", SizeMM: 3.0},
		{Face: constant.Font, Text: "unsized"},
	}, "", "", 0, 0)
	if err == nil {
		t.Fatalf("a block with no size was accepted, returning %d rows sized %v", len(f.Lines), f.Sizes)
	}
	if !strings.Contains(err.Error(), "is sized 0mm") {
		t.Errorf("a zero-sized block was refused with %q, which does not name the zero size; "+
			"another guard answered for spec 2.3's per-entry check", err)
	}
	// Sizes off the ladder, the same guard FitBlocksAt applies. Every capacity
	// figure in this package is measured at a rung.
	for _, size := range []float32{4.5, 2.9, 7.0, -3} {
		if _, err := FitSized(P, []Block{{Face: sh.Font, Text: "x", SizeMM: size}}, "", "", 0, 0); err == nil {
			t.Errorf("%.1fmm is not a rung in FontSizes %v but was accepted", size, FontSizes)
		}
	}
	// A composition that does not fit is refused rather than truncated.
	huge := []Block{{Face: sh.Font, Text: strings.Repeat("W", 4000), SizeMM: 3.0}}
	if _, err := FitSized(P, huge, "", "", 0, 0); err == nil {
		t.Error("4000 characters were accepted on a 3.0mm plate")
	}
	// The title takes the FIRST block's face and the footer the LAST, exactly
	// as fitBlocksAt does.
	g, err := FitSized(P, []Block{
		{Face: sh.Font, Text: "first", SizeMM: 3.0},
		{Face: constant.Font, Text: "last", SizeMM: 3.0},
	}, "T", "F", 3.0, 3.0)
	if err != nil {
		t.Fatal(err)
	}
	if g.TitleFace != sh.Font {
		t.Error("the title is not cut in the first block's face")
	}
	if g.FooterFace != constant.Font {
		t.Error("the footer is not cut in the last block's face")
	}
	// No QR, and no placement: spec 2.1.1's "QR != nil implies !Mixed" is
	// structurally true here rather than checked.
	if g.QR != nil || g.qrAt != nil {
		t.Errorf("FitSized produced a code (%v) or a placement (%v); it has no QR parameter",
			g.QR != nil, g.qrAt != nil)
	}
}
