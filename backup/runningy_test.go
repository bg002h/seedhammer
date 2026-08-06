package backup

import (
	"strings"
	"testing"

	"seedhammer.com/font/constant"
	"seedhammer.com/font/sh"
	"seedhammer.com/font/vector"
)

// fillAt is a spaceless run of c long enough to occupy exactly n rows of fnt at
// size, for a block laid out at baseY -- the block's OWN row budgets, summed,
// indexed from 0 as the wrap indexes them. Spaceless so the greedy wrap fills
// each row to its budget and no further.
func fillAt(c string, fnt *vector.Face, size float32, baseY, n int) string {
	P := prodParams
	lay := textLayout(P, fnt, P.F(size), baseY, nil)
	total := 0
	for i := range n {
		k, _ := lay.at(i)
		total += k
	}
	return strings.Repeat(c, total)
}

// TestMixedPlateCutsEachRowAtTheSizeItClaims is spec 7.2, and it is the first
// assertion in this change that a plate can carry more than one size at all.
//
// The sizes are written down HERE and the running y is accumulated HERE, from
// those literals. Reading f.Sizes[i] back to build the expectation would assert
// nothing: it is the value under test, and "the glyph height matches Sizes[i]"
// is satisfied by an engraver that ignores Sizes entirely.
//
// What is measured is per-row INK: its height and width, against the same
// string cut alone at the size that row claims, and its offset down the plate,
// against the running sum of the sizes above it. A row cut at the wrong size
// fails the first pair; a row placed at margin + i*fontSize instead of the
// running y fails the second.
func TestMixedPlateCutsEachRowAtTheSizeItClaims(t *testing.T) {
	P := prodParams
	const text = "WWWW"
	// Two rungs, so no row's size can be inferred from its neighbour's, and the
	// block boundary falls in the middle rather than at an end.
	sizes := []float32{5.0, 5.0, 3.0, 3.0, 3.0}
	faces := []*vector.Face{sh.Font, sh.Font, constant.Font, constant.Font, constant.Font}

	one := func(sizeMM float32, fnt *vector.Face) Fitted {
		return Fitted{
			Sizes: []float32{sizeMM}, Lines: []string{text},
			Faces: []*vector.Face{fnt}, TitleFace: fnt, FooterFace: fnt,
		}
	}
	// Non-vacuity: the two rungs must ink differently, or "cut at its own size"
	// has no wrong answer to give.
	big := inkBounds(t, P, EngraveFitted(P, one(5.0, sh.Font)))
	small := inkBounds(t, P, EngraveFitted(P, one(3.0, sh.Font)))
	if big.Max.Y-big.Min.Y == small.Max.Y-small.Min.Y {
		t.Fatalf("%q inks %d tall at both 5.0mm and 3.0mm; this test cannot see a wrong size",
			text, big.Max.Y-big.Min.Y)
	}

	f := Fitted{
		Mixed: true, // so SizeMM is 0 and an engraver that reads it divides by zero
		Sizes: sizes, Faces: faces,
		Lines:     make([]string, len(sizes)),
		TitleFace: faces[0], FooterFace: faces[len(faces)-1],
	}

	margin := P.I(outerMargin)
	y := margin
	for i := range sizes {
		// Row i alone carries ink, so the measured bounds ARE that row's.
		g := f
		g.Lines = make([]string, len(sizes))
		g.Lines[i] = text
		b := inkBounds(t, P, EngraveFitted(P, g))

		// The control: the same string in the same face at the size row i
		// claims, alone on a plate, so its ink sits at the top margin.
		c := inkBounds(t, P, EngraveFitted(P, one(sizes[i], faces[i])))

		if got, want := b.Max.Y-b.Min.Y, c.Max.Y-c.Min.Y; got != want {
			t.Errorf("row %d inks %d tall; %q at %.1fmm in its own face inks %d",
				i, got, text, sizes[i], want)
		}
		if got, want := b.Max.X-b.Min.X, c.Max.X-c.Min.X; got != want {
			t.Errorf("row %d inks %d wide; %q at %.1fmm in its own face inks %d",
				i, got, text, sizes[i], want)
		}
		// The running y: row i starts exactly the sum of the sizes ABOVE it
		// below the plate's top margin, in device units.
		if got, want := b.Min.Y-c.Min.Y, y-margin; got != want {
			t.Errorf("row %d inks from y=%d, %d below the control's %d; the running y puts it %d below",
				i, b.Min.Y, got, c.Min.Y, want)
		}
		y += P.F(sizes[i])
	}
}

// TestFooterAndLastBodyRowAreDisjointOnAMixedPlate is spec 7.8, and it exists
// because the two formulas the earlier drafts handed over do not compose:
// limit = plateHeight - margin - F(footerSizeMM) sits BELOW the footer's own
// ink, so the last body row is laid over it. On a UNIFORM plate that never
// shows -- the old row counting consulted neither formula, so no golden would
// have moved -- which is why this fixture mixes sizes and puts a 3.8mm footer
// under a 3.0mm body, where the error is 3.000mm.
//
// Both halves of 7.8 are here: the composition that fills the budget is
// accepted and clears the footer, and the same composition one row taller is
// REFUSED. The counterfactual is asserted too, or "it fits" would be satisfied
// by a limit that is simply too small.
func TestFooterAndLastBodyRowAreDisjointOnAMixedPlate(t *testing.T) {
	P := prodParams
	const (
		footerMM = 3.8 // the rung where the two formulas differ by 3.000mm
		bigMM    = 5.0
		smallMM  = 3.0
		// MEASURED: with a 3.8mm footer the body may run to 75.200mm, and three
		// 5.0mm rows leave room for exactly nineteen 3.0mm rows below them.
		block1Rows = 3
		block2Rows = 19
	)
	start, limit := yBudget(P, "", "FOOTER", 0, footerMM)
	if want := P.I(outerMargin); start != want {
		t.Fatalf("a plate with no title starts its body at %d, want the top margin %d", start, want)
	}
	if want := footerRowY(P, footerMM); limit != want {
		t.Fatalf("limit = %d, want the footer's own top y %d", limit, want)
	}
	// The formula this replaced, and the room it would have handed the body.
	naive := P.F(plateSize) - P.I(outerMargin) - P.F(footerMM)
	if naive <= limit {
		t.Fatalf("the bottom-margin formula gives %d against the footer's %d; at %.1fmm the two "+
			"do not disagree and this fixture proves nothing", naive, limit, footerMM)
	}

	sizes := []float32{bigMM, smallMM}
	y2 := start + block1Rows*P.F(bigMM)
	blocks := func(extra int) []Block {
		return []Block{
			{Face: sh.Font, Text: fillAt("a", sh.Font, bigMM, start, block1Rows)},
			{Face: constant.Font, Text: fillAt("b", constant.Font, smallMM, y2, block2Rows+extra)},
		}
	}

	lines, faces, rowSizes, ok := wrapBlocks(P, blocks(0), sizes, nil, start, limit)
	if !ok {
		t.Fatalf("the composition measured to fill the budget was refused")
	}
	if got, want := len(lines), block1Rows+block2Rows; got != want {
		t.Fatalf("the composition wrapped to %d rows, want %d; the fill is not sized to the "+
			"budgets it is measured against", got, want)
	}
	// One row too tall is REFUSED -- the second half of 7.8.
	if _, _, _, ok := wrapBlocks(P, blocks(1), sizes, nil, start, limit); ok {
		t.Error("a composition one row taller than the budget was accepted")
	}
	// ...and the bottom-margin formula would have taken it, which is what makes
	// the refusal above a property of the FOOTER's y and not of a tight limit.
	if _, _, _, ok := wrapBlocks(P, blocks(1), sizes, nil, start, naive); !ok {
		t.Fatalf("the one-row-taller composition does not fit the %d bottom-margin limit either; "+
			"this fixture cannot tell the two formulas apart", naive)
	}
	// And that extra row really would have crossed the footer's ink, rather than
	// merely running past a number.
	extraTop := y2 + block2Rows*P.F(smallMM)
	if extraTop >= limit+P.F(footerMM) || extraTop+P.F(smallMM) <= limit {
		t.Fatalf("the refused row spans y[%d,%d), which misses the footer's row [%d,%d)",
			extraTop, extraTop+P.F(smallMM), limit, limit+P.F(footerMM))
	}

	f := Fitted{
		Mixed: true, Sizes: rowSizes, Lines: lines, Faces: faces,
		Footer: "FOOTER", FooterFace: constant.Font, FooterSizeMM: footerMM,
		TitleFace: sh.Font,
	}
	// The last body row alone, with the footer dropped: a footer never moves the
	// body, so this is that row's ink and nothing else's.
	body := f
	body.Footer, body.FooterSizeMM = "", 0
	body.Lines = make([]string, len(lines))
	body.Lines[len(lines)-1] = lines[len(lines)-1]
	last := inkBounds(t, P, EngraveFitted(P, body))
	// The footer alone.
	foot := f
	foot.Lines = make([]string, len(lines))
	ftr := inkBounds(t, P, EngraveFitted(P, foot))

	if last.Max.Y >= ftr.Min.Y {
		t.Errorf("the last body row inks to y=%d and the footer from y=%d: they overlap by %d (%.3fmm)",
			last.Max.Y, ftr.Min.Y, last.Max.Y-ftr.Min.Y, float64(last.Max.Y-ftr.Min.Y)/mm)
	}
	// Non-vacuity: the body must actually reach the footer's neighbourhood, or
	// disjointness is a fact about a half-empty plate.
	if gap := ftr.Min.Y - last.Max.Y; gap > P.F(smallMM) {
		t.Errorf("the last body row ends %d (%.3fmm) above the footer, more than a %.1fmm row; "+
			"this composition does not fill the plate and the assertion above is free",
			gap, float64(gap)/mm, smallMM)
	}
}

// TestUnboundedWrapCallersReportUntruncatedCounts is spec 7.11. start and limit
// became device-unit y in this change, and math.MaxInt has to keep meaning
// "unbounded" through the new (limit - y) / fontSize arithmetic -- fit.go says
// why: "a refusal that reported 26 / 26 for a text needing 300 lines would tell
// the operator nothing about how much to cut".
//
// A bounded wrap does not merely clip here, it fails: wrapBlocks returns no
// lines at all when the text does not fit, so the readout would report 0.
func TestUnboundedWrapCallersReportUntruncatedCounts(t *testing.T) {
	P := prodParams
	const size = 3.0
	rows := LinesPerPlate(P, size)
	// Far past the plate: 60 rows against the 26 it holds.
	const want = 60
	// AdmissibleBlocks lays the body out from the margin plus its unconditional
	// title row, so that is the baseY these budgets are taken at.
	text := fillAt("a", sh.Font, size, P.I(outerMargin)+P.F(size), want)

	used, avail, ok := Admissible(P, sh.Font, text, "", "", false)
	if avail != rows-2 {
		t.Fatalf("linesAvail = %d, want %d", avail, rows-2)
	}
	if ok {
		t.Fatalf("a %d-row text was admitted against %d lines", want, avail)
	}
	if used != want {
		t.Errorf("linesUsed = %d for a text needing %d rows; the count is truncated, and the "+
			"operator is told nothing about how much to cut", used, want)
	}

	// rowFaces is the other unbounded caller. Its whole answer is which face
	// each PLATE row would be cut in, so a truncated wrap returns no faces at
	// all and every row falls through to the LAST block's -- the opposite of
	// the truth on a composition whose first block overflows the plate.
	blocks := []Block{
		{Face: sh.Font, Text: text},
		{Face: constant.Font, Text: "x"},
	}
	faces := rowFaces(P, blocks, size, nil, rows)
	for r, fnt := range faces {
		if fnt != sh.Font {
			t.Fatalf("plate row %d is cut in the last block's face; the first block alone needs "+
				"%d rows and should own every one of the plate's %d", r, want, rows)
		}
	}
}

// TestAdmissibleBlocksVerdictDoesNotMove is spec 7.20, and it is the ONLY thing
// in this phase that fails if either caller translation is carried over wrong.
// Both were invisible to the compiler and to every golden: start changed from a
// ROW INDEX to a device-unit y and the old literals stay type-correct, int to
// int, so the wrong value is silent.
//
// The verdict this pins gates OK on the text step AND the confirm step
// (gui/freetext_flow.go), for ordinary QR-carrying operator plates -- the path
// seeds and passphrases take. An admissible plate would be refused, or an
// inadmissible one accepted, with the readout disagreeing with the fit.
//
// MEASURED at 3.0mm in font/sh. Carrying the literal 1 over as a y puts the
// body one device unit below the plate's top EDGE, which drops FOUR rows into
// the top screw-hole band instead of two: 24 rows then hold 1024 characters
// rather than 1032, and the at-capacity text below reports 25 lines and is
// refused.
func TestAdmissibleBlocksVerdictDoesNotMove(t *testing.T) {
	P := prodParams
	rows := LinesPerPlate(P, 3.0)
	avail := rows - 2
	if avail != 24 {
		t.Fatalf("the 3.0mm plate offers %d body lines, want 24; every number below is measured at 24", avail)
	}
	for _, tc := range []struct {
		name  string
		text  string
		useQR bool
		// modules is 0 for the no-QR cases. Pinned because qr.Encode picks its
		// mode from the character SET: a fixture that changed case would be a
		// different plate reporting the same length.
		modules  int
		wantUsed int
		wantOK   bool
	}{
		// The sharp one. 1032 = the exact capacity of the 24 rows the body gets
		// when it starts at margin + one row.
		{"no QR, exactly at capacity", strings.Repeat("a", 1032), false, 0, 24, true},
		{"no QR, one line over", strings.Repeat("a", 1033), false, 0, 25, false},
		{"QR, exactly at capacity", strings.Repeat("A", 616), true, 69, 24, true},
		{"QR, one line over", strings.Repeat("A", 617), true, 69, 25, false},
		// The QR ANCHOR case. At capacity the anchor is INVISIBLE -- moving the
		// band down one row swaps a plain row in at the top for a plain row out
		// at the bottom, so the 24-row total is unchanged and both cases above
		// report 24 either way. Measured: the two disagree over 207 lengths, and
		// this is one of them. Anchoring the placement at start instead of at
		// outerMargin reports 15 rows here.
		{"QR, the band's own rows", strings.Repeat("A", 375), true, 57, 16, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.useQR {
				qrc, err := qrFor(tc.text, true)
				if err != nil {
					t.Fatal(err)
				}
				if qrc.Size != tc.modules {
					t.Fatalf("the fixture encodes to %d modules, want %d; this is a different plate",
						qrc.Size, tc.modules)
				}
			}
			used, got, ok := Admissible(P, sh.Font, tc.text, "", "", tc.useQR)
			if got != avail {
				t.Errorf("linesAvail = %d, want %d", got, avail)
			}
			if used != tc.wantUsed || ok != tc.wantOK {
				t.Errorf("%d characters: (linesUsed %d, ok %v), want (%d, %v)",
					len(tc.text), used, ok, tc.wantUsed, tc.wantOK)
			}
		})
	}
}

// TestMaxCharsAtBlocksPinsTheFaceBoundaryOnAScrewHoleRow is spec 7.20's second
// half: rowFaces' start translation, from the row index 0 to the plate's top
// MARGIN. The literal carried over is the plate's top EDGE, three millimetres
// higher, which moves the screw-hole bands by a row and with them the row a
// block's text runs out on.
//
// TestMaxCharsAtBlocksCountsEachRowInItsOwnFace asserts only that the mixed
// figure lies strictly between the two single-face ones, which a boundary
// shifted by a row satisfies comfortably. This pins the figure itself, on a
// composition whose face boundary lands ON a screw-hole row -- where the two
// faces disagree most and where the shift is a whole row of characters.
func TestMaxCharsAtBlocksPinsTheFaceBoundaryOnAScrewHoleRow(t *testing.T) {
	P := prodParams
	const size = 3.0
	rows := LinesPerPlate(P, size)
	margin := P.I(outerMargin)

	// Block 1 fills plate rows 0..23 exactly, so block 2 begins on row 24 --
	// the first row of the BOTTOM screw-hole band. Under the top-edge anchor
	// the same text needs 25 rows, and the boundary lands on row 25 instead.
	const boundary = 24
	block1 := fillAt("a", sh.Font, size, margin, boundary)
	blocks := []Block{
		{Face: sh.Font, Text: block1},
		{Face: constant.Font, Text: strings.Repeat("b", 200)},
	}

	faces := rowFaces(P, blocks, size, nil, rows)
	for r, fnt := range faces {
		want := sh.Font
		if r >= boundary {
			want = constant.Font
		}
		if fnt != want {
			t.Fatalf("plate row %d is cut in the wrong face; block 1 fills rows 0..%d",
				r, boundary-1)
		}
	}
	// The boundary row must be a screw-hole row, and the two faces must disagree
	// about it, or moving it costs nothing.
	shLay := textLayout(P, sh.Font, P.F(size), margin, nil)
	cLay := textLayout(P, constant.Font, P.F(size), margin, nil)
	shN, offx := shLay.at(boundary)
	cN, _ := cLay.at(boundary)
	if offx == 0 {
		t.Fatalf("row %d is not a screw-hole row; this fixture is not the plate it names", boundary)
	}
	if shN == cN {
		t.Fatalf("both faces hold %d characters on row %d; the boundary has no wrong side", shN, boundary)
	}

	// MEASURED: 1032 characters over the 24 font/sh rows plus 31 on each of the
	// two font/constant screw-hole rows. The top-edge anchor gives 1099, because
	// row 24 is then counted in font/sh's 36 columns rather than in
	// font/constant's 31.
	if got, want := MaxCharsAtBlocks(P, blocks, size, false), 1094; got != want {
		t.Errorf("MaxCharsAtBlocks = %d, want %d (row %d counted in %d columns, not %d)",
			got, want, boundary, cN, shN)
	}
}
