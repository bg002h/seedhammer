package backup

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"seedhammer.com/bspline"
	"seedhammer.com/engrave"
	"seedhammer.com/font/constant"
	"seedhammer.com/font/sh"
	"seedhammer.com/font/vector"
)

// A mixed-face composition. The block texts are SPACELESS so the greedy wrap
// fills every column exactly to its budget: a line that came out short, or one
// character long, is then a measurement and not a judgement.
func mixedBlocks(a, b int) []Block {
	return []Block{
		{Face: sh.Font, Text: strings.Repeat("a", a)},
		{Face: constant.Font, Text: strings.Repeat("b", b)},
	}
}

// rowBudget is the character budget of one plate row in one face, taken from
// the same lineLayout the wrap uses.
func rowBudget(fnt *vector.Face, size float32, row int) int {
	P := prodParams
	lay := textLayout(P, fnt, P.F(size), P.I(outerMargin), nil, freeTextQRScale)
	n, _ := lay.at(row)
	return n
}

// fillRows is a spaceless run of c long enough to occupy exactly n rows of fnt
// starting at plate row startRow -- the budgets, summed. Spaceless so the
// greedy wrap fills each row to its budget and no further.
func fillRows(c string, fnt *vector.Face, size float32, startRow, n int) string {
	total := 0
	for i := range n {
		total += rowBudget(fnt, size, startRow+i)
	}
	return strings.Repeat(c, total)
}

// TestFitBlocksWrapsEachBlockToItsOwnFaceGrid is the mixed-face plate's central
// claim, and the one place a defect would be invisible on every other
// assertion: font/sh is 44 columns at 3.0mm and font/constant is 39, so a wrap
// that measured both halves in ONE face still returns the right number of
// lines, the right size and the right code -- and puts eleven characters of
// every constant line past the edge of the plate.
//
// Asserted against the LIVE budget of the row each line lands on, in the face
// that line is cut in. No column count is written down here: thirteen
// font/constant glyphs changed in the commit this plate was built on, and more
// will.
func TestFitBlocksWrapsEachBlockToItsOwnFaceGrid(t *testing.T) {
	P := prodParams
	const size = 3.0
	rows := LinesPerPlate(P, size)
	start, end := bodyRows(rows, "T", "F")

	// Eleven rows of each: enough of the plate that no larger rung can hold it,
	// so the composition lands on the 3.0mm grid this test measures.
	const half = 11
	blocks := []Block{
		{Face: sh.Font, Text: fillRows("a", sh.Font, size, start, half)},
		{Face: constant.Font, Text: fillRows("b", constant.Font, size, start+half, half)},
	}
	f, err := FitBlocks(P, blocks, "T", "F", false)
	if err != nil {
		t.Fatalf("the composition does not fit: %v", err)
	}
	if f.SizeMM != size {
		t.Fatalf("landed at %.1fmm, want %.1fmm; this test measures the 3.0mm grid", f.SizeMM, size)
	}
	if len(f.Faces) != len(f.Lines) {
		t.Fatalf("%d lines but %d faces", len(f.Lines), len(f.Faces))
	}
	if start+len(f.Lines) > end {
		t.Fatalf("the composition needs rows %d..%d of %d..%d", start, start+len(f.Lines), start, end)
	}

	sawSH, sawConst := 0, 0
	for i, l := range f.Lines {
		row := start + i
		want := rowBudget(f.Faces[i], size, row)
		switch f.Faces[i] {
		case sh.Font:
			sawSH++
		case constant.Font:
			sawConst++
		default:
			t.Fatalf("line %d is cut in a face no block asked for", i)
		}
		// The last line of a block is whatever is left over; every other line
		// was filled to the budget of ITS OWN face at ITS OWN row.
		last := i+1 == len(f.Lines) || f.Faces[i+1] != f.Faces[i]
		if last {
			if len(l) > want {
				t.Errorf("line %d (row %d, %d chars) overruns its face's %d-column budget: %q",
					i, row, len(l), want, l)
			}
			continue
		}
		if len(l) != want {
			t.Errorf("line %d (row %d) is %d characters, want exactly %d -- it was wrapped to the "+
				"wrong face's grid, or to the wrong row's", i, row, len(l), want)
		}
	}
	if sawSH == 0 || sawConst == 0 {
		t.Fatalf("the plate came out %d sh rows and %d constant rows; this test is vacuous",
			sawSH, sawConst)
	}
	// Non-vacuity, stated as the defect it guards: the two faces must actually
	// disagree about a row's budget, or measuring "each in its own" proves
	// nothing.
	if a, b := rowBudget(sh.Font, size, start), rowBudget(constant.Font, size, start); a == b {
		t.Fatalf("both faces hold %d characters on row %d; there is no wrong answer to give", a, start)
	}
	t.Logf("%d sh rows, %d constant rows; an unobstructed row holds %d in sh and %d in constant",
		sawSH, sawConst, rowBudget(sh.Font, size, start+3), rowBudget(constant.Font, size, start+3))
}

// TestFitBlocksFaceBoundaryFollowsTheContent: the face changes exactly where
// one block's text ends, never a row early or late. A boundary off by one puts
// the last line of one proof half in the other half's face -- and its label
// then names a face it is not cut in.
func TestFitBlocksFaceBoundaryFollowsTheContent(t *testing.T) {
	P := prodParams
	for _, n := range []int{1, 2, 5, 9} {
		blocks := mixedBlocks(n*rowBudget(sh.Font, 3.0, 1), 3*rowBudget(constant.Font, 3.0, 12))
		f, err := FitBlocks(P, blocks, "T", "F", false)
		if err != nil {
			t.Fatalf("n=%d: %v", n, err)
		}
		// Wrapping block 0 ALONE must produce exactly the plate's first rows,
		// which is what "the boundary is where the content ends" means.
		alone, err := FitBlocks(P, blocks[:1], "T", "F", false)
		if err != nil {
			t.Fatalf("n=%d: block 0 alone: %v", n, err)
		}
		if alone.SizeMM != f.SizeMM {
			continue // a different rung; the row budgets differ and so may the split
		}
		if !slices.Equal(f.Lines[:len(alone.Lines)], alone.Lines) {
			t.Errorf("n=%d: the first block wraps differently beside the second one", n)
		}
		for i := range f.Faces {
			want := sh.Font
			if i >= len(alone.Lines) {
				want = constant.Font
			}
			if f.Faces[i] != want {
				t.Errorf("n=%d: row %d is cut in the wrong face; the boundary is not where block 0 ends "+
					"(%d lines)", n, i, len(alone.Lines))
				break
			}
		}
	}
}

// TestFitBlocksTitleAndFooterTakeTheBorderingFaces pins the rule stated on
// FitBlocks: the title is cut in the FIRST block's face and the footer in the
// LAST block's, so each screw-hole row belongs to the half it borders and the
// plate reads as two contiguous runs.
func TestFitBlocksTitleAndFooterTakeTheBorderingFaces(t *testing.T) {
	P := prodParams
	f, err := FitBlocks(P, mixedBlocks(80, 80), "T", "F", false)
	if err != nil {
		t.Fatal(err)
	}
	if f.TitleFace != sh.Font {
		t.Error("the title is not cut in the first block's face")
	}
	if f.FooterFace != constant.Font {
		t.Error("the footer is not cut in the last block's face")
	}
	// And a single-face plate is unchanged: both are simply the plate's face.
	one, err := FitBlocks(P, []Block{{Face: constant.Font, Text: "hello"}}, "T", "F", false)
	if err != nil {
		t.Fatal(err)
	}
	if one.TitleFace != constant.Font || one.FooterFace != constant.Font {
		t.Error("a single-face plate no longer cuts its title and footer in the plate's face")
	}
}

// TestSingleBlockIsTheOldSingleFacePath: everything the free-text plate did
// before mixed faces existed must still hold, exactly. Fit is now FitBlocks
// with one block, and EngraveFreeText is EngraveFitted with a uniform face map,
// so this asserts the two agree on the composition AND on the geometry -- the
// goldens cover the same ground from the other side.
func TestSingleBlockIsTheOldSingleFacePath(t *testing.T) {
	P := prodParams
	const text = "Dear heir,\n\nThe hardware wallet is in the safe.\nThe PIN is NOT written down anywhere."
	for _, fnt := range []*vector.Face{sh.Font, constant.Font} {
		for _, useQR := range []bool{false, true} {
			size, lines, qrc, err := Fit(P, fnt, text, "TO MY HEIR", "2026 COPY 1", useQR)
			if err != nil {
				t.Fatal(err)
			}
			f, err := FitBlocks(P, []Block{{Face: fnt, Text: text}}, "TO MY HEIR", "2026 COPY 1", useQR)
			if err != nil {
				t.Fatal(err)
			}
			if f.SizeMM != size || !slices.Equal(f.Lines, lines) {
				t.Errorf("Fit and FitBlocks disagree about the composition")
			}
			if (f.QR == nil) != (qrc == nil) {
				t.Errorf("Fit and FitBlocks disagree about the code")
			}
			for i, v := range f.Faces {
				if v != fnt {
					t.Fatalf("line %d of a one-block composition is cut in another face", i)
				}
			}
			a := blockSpline(t, EngraveFreeText(P, fnt, size, "TO MY HEIR", lines, "2026 COPY 1", qrc))
			b := blockSpline(t, EngraveFitted(P, f))
			if !slices.Equal(a, b) {
				t.Errorf("EngraveFreeText and EngraveFitted produce different geometry for the same "+
					"single-face plate (%d knots vs %d)", len(a), len(b))
			}
		}
	}
}

// TestEngraveFittedCutsEachLineInItsOwnFace is the geometry-level assertion
// that Fitted.Faces is READ rather than decorative.
//
// Nothing recoverable from a finished plate says which face a row was cut in --
// it is stroke geometry with no text in it -- so the only way to see a wrong
// face is that the strokes differ. Each mutation below is one an assertion on
// the size, the lines or the code cannot distinguish from the truth.
func TestEngraveFittedCutsEachLineInItsOwnFace(t *testing.T) {
	P := prodParams
	f, err := FitBlocks(P, mixedBlocks(200, 150), "TITLE", "FOOTER", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Faces) < 4 {
		t.Fatalf("the fixture is only %d rows; too small to tell the halves apart", len(f.Faces))
	}
	want := blockSpline(t, EngraveFitted(P, f))

	swapFace := func(v *vector.Face) *vector.Face {
		if v == sh.Font {
			return constant.Font
		}
		return sh.Font
	}
	for _, mut := range []struct {
		name string
		make func(Fitted) Fitted
	}{
		{"both halves in the first face", func(g Fitted) Fitted {
			g.Faces = slices.Clone(g.Faces)
			for i := range g.Faces {
				g.Faces[i] = f.Faces[0]
			}
			return g
		}},
		{"both halves in the last face", func(g Fitted) Fitted {
			g.Faces = slices.Clone(g.Faces)
			for i := range g.Faces {
				g.Faces[i] = f.Faces[len(f.Faces)-1]
			}
			return g
		}},
		{"the halves swapped", func(g Fitted) Fitted {
			g.Faces = slices.Clone(g.Faces)
			for i := range g.Faces {
				g.Faces[i] = swapFace(g.Faces[i])
			}
			return g
		}},
		{"the title in the other face", func(g Fitted) Fitted {
			g.TitleFace = swapFace(g.TitleFace)
			return g
		}},
		{"the footer in the other face", func(g Fitted) Fitted {
			g.FooterFace = swapFace(g.FooterFace)
			return g
		}},
	} {
		got := blockSpline(t, EngraveFitted(P, mut.make(f)))
		if slices.Equal(got, want) {
			t.Errorf("%s produces identical geometry; the face map is not being engraved", mut.name)
		}
	}
}

// TestEngraveFittedRefusesAFaceMapThatDoesNotCoverTheLines: a short face map
// would otherwise engrave the uncovered rows in whatever face was at hand,
// which is a wrong plate rather than a failure.
//
// The size map here is COMPLETE and the Faces guard is evaluated first, so the
// panic observed is the face map's. Left short, this test would recover from
// the Sizes guard instead and go on passing while testing nothing it names.
func TestEngraveFittedRefusesAFaceMapThatDoesNotCoverTheLines(t *testing.T) {
	P := prodParams
	f := Fitted{
		SizeMM: 3.0,
		Lines:  []string{"one", "two"},
		Faces:  []*vector.Face{sh.Font},
		Sizes:  []float32{3.0, 3.0},
	}
	defer func() {
		v := recover()
		if v == nil {
			t.Error("a face map covering one of two lines was accepted")
			return
		}
		// Which guard fired is asserted, not just that one did: recovering from
		// ANY panic is how a fixture goes on passing after the thing it names
		// stopped being reached.
		if got := fmt.Sprint(v); !strings.Contains(got, "faces") {
			t.Errorf("a short face map panicked with %q; another guard answered for it", got)
		}
	}()
	EngraveFitted(P, f)
}

// TestEngraveFittedRefusesASizeMapThatDoesNotCoverTheLines is the Sizes half of
// the guard above, and it exists for the same reason: Sizes is parallel to
// Lines and is the ONLY channel a row's size travels on, so a short size map
// would leave the uncovered rows to be cut at whatever size was at hand -- a
// wrong plate rather than a failure.
//
// The face map here is COMPLETE, so the Faces guard cannot fire and the panic
// observed is this guard's.
func TestEngraveFittedRefusesASizeMapThatDoesNotCoverTheLines(t *testing.T) {
	P := prodParams
	f := Fitted{
		SizeMM: 3.0,
		Lines:  []string{"one", "two"},
		Faces:  []*vector.Face{sh.Font, sh.Font},
		Sizes:  []float32{3.0},
	}
	defer func() {
		v := recover()
		if v == nil {
			t.Error("a size map covering one of two lines was accepted")
			return
		}
		if got := fmt.Sprint(v); !strings.Contains(got, "sizes") {
			t.Errorf("a short size map panicked with %q; another guard answered for it", got)
		}
	}()
	EngraveFitted(P, f)
}

// blockSpline plans an engraving into its knots, so two plates can be compared
// as the strokes the machine will actually cut.
func blockSpline(t *testing.T, e engrave.Engraving) []bspline.Knot {
	t.Helper()
	return slices.Collect(engrave.PlanEngraving(prodParams.StepperConfig, e))
}

// TestCompositionTextIsTheTypedText: the QR is a copy of the PLATE, so the text
// the blocks describe has to be character-for-character the text that was
// split into them. A separator that was consumed, doubled or substituted would
// make a scanner return something the plate does not say.
func TestCompositionTextIsTheTypedText(t *testing.T) {
	for _, s := range []string{
		"", "one", "one\ntwo", "one\n\nthree", "\nleading", "trailing\n", "a\nb\nc\nd",
	} {
		parts := strings.Split(s, "\n")
		for cut := 1; cut < len(parts); cut++ {
			blocks := []Block{
				{Face: sh.Font, Text: strings.Join(parts[:cut], "\n")},
				{Face: constant.Font, Text: strings.Join(parts[cut:], "\n")},
			}
			if got := CompositionText(blocks); got != s {
				t.Errorf("splitting %q at block %d and rejoining gives %q", s, cut, got)
			}
		}
		if got := CompositionText([]Block{{Face: sh.Font, Text: s}}); got != s {
			t.Errorf("a one-block composition of %q reads back as %q", s, got)
		}
	}
}

// TestMaxCharsAtBlocksCountsEachRowInItsOwnFace: the refusal message's "removing
// the QR frees about N characters" is computed over the plate's rows, and on a
// mixed plate those rows do not share a column count. Counted in one face the
// figure is wrong by the difference between the grids on every row of the other
// half.
func TestMaxCharsAtBlocksCountsEachRowInItsOwnFace(t *testing.T) {
	P := prodParams
	const size = 3.0
	blocks := mixedBlocks(300, 300)
	mixed := MaxCharsAtBlocks(P, blocks, size, false)
	allSH := MaxCharsAt(P, sh.Font, size, CompositionText(blocks), false)
	allConst := MaxCharsAt(P, constant.Font, size, CompositionText(blocks), false)
	if allSH == allConst {
		t.Fatalf("both faces report %d characters a plate; this test cannot see a wrong face", allSH)
	}
	if mixed == allSH || mixed == allConst {
		t.Errorf("a mixed plate reports %d characters, the same as counting every row in one face "+
			"(%d sh / %d constant)", mixed, allSH, allConst)
	}
	if lo, hi := min(allSH, allConst), max(allSH, allConst); mixed <= lo || mixed >= hi {
		t.Errorf("a mixed plate reports %d characters, outside the %d..%d the two faces bracket",
			mixed, lo, hi)
	}
	// And a one-block composition is unchanged.
	for _, fnt := range []*vector.Face{sh.Font, constant.Font} {
		one := []Block{{Face: fnt, Text: "hello"}}
		if got, want := MaxCharsAtBlocks(P, one, size, false), MaxCharsAt(P, fnt, size, "hello", false); got != want {
			t.Errorf("%v: MaxCharsAtBlocks says %d, MaxCharsAt %d", fmt.Sprintf("%p", fnt), got, want)
		}
	}
}

// TestEngraveFittedInsetsEachRowInItsOwnFace closes the one mutation the rest
// of this suite could not see: the left inset of a screw-hole row is a whole
// number of characters OF THAT ROW'S FACE, and taking it from the plate's first
// face instead is invisible on any plate whose second half never reaches a
// screw-hole band. The shipped mixed proof is exactly such a plate -- it stops
// eleven rows short of the bottom band -- so the API has to be pinned here
// rather than through the pattern.
//
// Constructed so ONE line carries all the ink and it lands on the last plate
// row, which is a screw-hole row. The same line at the same row in the same
// face must engrave identically whether the rows above it are cut in another
// face or not: the inset belongs to the line, not to the plate.
func TestEngraveFittedInsetsEachRowInItsOwnFace(t *testing.T) {
	P := prodParams
	const size = 3.0
	rows := LinesPerPlate(P, size)
	row := rows - 1

	insetOf := func(fnt *vector.Face) int {
		lay := textLayout(P, fnt, P.F(size), P.I(outerMargin), nil, freeTextQRScale)
		_, offx := lay.at(row)
		return offx
	}
	if insetOf(sh.Font) == insetOf(constant.Font) {
		t.Fatalf("both faces inset row %d by %d; no wrong answer is available here",
			row, insetOf(sh.Font))
	}
	if insetOf(constant.Font) == 0 {
		t.Fatalf("row %d is not a screw-hole row; this test measures nothing", row)
	}

	// Every line empty but the last, so the ink bounds ARE that line's.
	//
	// Sizes is UNIFORM at SizeMM, and that is the only fill that measures what
	// this test names. insetOf's reference is plate-absolute, so any other fill
	// still satisfies the length guard and every assertion below while silently
	// comparing a different row. Title and Footer are empty, so their sizes are
	// 0 and Mixed is false.
	mk := func(rest *vector.Face) Fitted {
		lines := make([]string, rows)
		faces := make([]*vector.Face, rows)
		sizes := make([]float32, rows)
		for i := range faces {
			faces[i] = rest
			sizes[i] = size
		}
		lines[row] = "WWWW"
		faces[row] = constant.Font
		return Fitted{SizeMM: size, Sizes: sizes, Lines: lines, Faces: faces,
			TitleFace: faces[0], FooterFace: constant.Font}
	}
	mixed := inkBounds(t, P, EngraveFitted(P, mk(sh.Font)))
	alone := inkBounds(t, P, EngraveFitted(P, mk(constant.Font)))
	if mixed != alone {
		t.Errorf("a font/constant row inks x[%d,%d] under a font/sh half and x[%d,%d] on its own; "+
			"its inset is being measured in the plate's first face rather than its own",
			mixed.Min.X, mixed.Max.X, alone.Min.X, alone.Max.X)
	}
	// And it really is inset: a plate that ignored the band entirely would put
	// this row at the outer margin.
	if mixed.Min.X < P.I(outerMargin)+insetOf(constant.Font) {
		t.Errorf("the row inks from x=%d, inside the %d screw-hole inset", mixed.Min.X,
			P.I(outerMargin)+insetOf(constant.Font))
	}
}

// TestBlockSplittingDoesNotChangeTheLayoutWithinAFace: splitting a text into
// blocks is a FACE decision, never a layout one. Cut at any '\n' and cut in one
// face throughout, the plate must come out line for line identical to the same
// text as a single block -- which is what makes a two-block composition of one
// face indistinguishable from the single-face path, and what makes a trailing
// newline's empty last block faithful rather than a spurious blank row.
func TestBlockSplittingDoesNotChangeTheLayoutWithinAFace(t *testing.T) {
	P := prodParams
	for _, text := range []string{
		"one\ntwo\nthree",
		"a\nb\nc\nd\ne\n",
		"Dear heir,\n\nThe wallet is in the safe.\nThe PIN is not written down.",
		strings.Repeat("word ", 60) + "\n" + strings.Repeat("other ", 40),
	} {
		for _, fnt := range []*vector.Face{sh.Font, constant.Font} {
			whole, err := FitBlocks(P, []Block{{Face: fnt, Text: text}}, "T", "F", false)
			if err != nil {
				t.Fatalf("%.20q: %v", text, err)
			}
			parts := strings.Split(text, "\n")
			for cut := 1; cut < len(parts); cut++ {
				split, err := FitBlocks(P, []Block{
					{Face: fnt, Text: strings.Join(parts[:cut], "\n")},
					{Face: fnt, Text: strings.Join(parts[cut:], "\n")},
				}, "T", "F", false)
				if err != nil {
					t.Fatalf("%.20q cut at %d: %v", text, cut, err)
				}
				if split.SizeMM != whole.SizeMM || !slices.Equal(split.Lines, whole.Lines) {
					t.Errorf("%.20q cut at block %d lays out differently from the same text unsplit:\n"+
						" split %.1fmm %q\n whole %.1fmm %q", text, cut,
						split.SizeMM, split.Lines, whole.SizeMM, whole.Lines)
				}
			}
		}
	}
}
