package gui

import (
	"fmt"
	"math"
	"slices"
	"strings"
	"testing"

	"seedhammer.com/backup"
	"seedhammer.com/bspline"
	"seedhammer.com/engrave"
	"seedhammer.com/font/vector"
)

// Spec 1.1's two MEASURED composition tables, as spec 7.3 asks for them: per
// side and per block, the face, the size, the row count, the per-row character
// budgets and the device-unit y range, plus the total and the spare. Spec 7.4's
// character set per (rung, face) pair and spec 7.12's titles are here too,
// because all three are properties of the same two resolved plates.
//
// This file is what makes the back's 2.400mm of spare safe. The hazard is
// DISCRETE: a glyph change costing font/constant a row at 3.0mm moves the total
// by 3.000mm, which is more than any margin under discussion -- so margin does
// not mitigate it and a test does. Without these pins 5.400mm would not be
// enough either.
//
// Spec 1.2's correction is pinned here as well. BOTH screw-hole bands bite: the
// top one narrows rows whose top is above 10mm and the bottom one narrows rows
// whose bottom passes 75mm, and on both sides the bottom band lands inside the
// LAST block -- const@3.8 is [31 31 31 25] and const@3.0 is [39 31 31]. Neither
// narrowing changes a row count here, and that is a measured COINCIDENCE rather
// than a structural guarantee, which is exactly why the per-row budgets are
// pinned and not merely the row counts.

// The plate geometry the tables below are stated in. backup keeps outerMargin,
// innerMargin and plateSize unexported, so they are written down here and
// checked against backup's own answers by ftCheckPlateGeometry -- a wrong
// literal fails there rather than shifting every number in this file by a
// constant.
const (
	ftPlateMM  = 85.0 // plateSize: the plate, edge to edge
	ftMarginMM = 3.0  // outerMargin: the engraving margin
	ftInnerMM  = 10.0 // innerMargin: what a screw-hole row must clear
	// ftLimitMM is spec 2.4's budget limit for a plate with no footer, which is
	// what both ladder sides are: plateSize - outerMargin.
	ftLimitMM = ftPlateMM - ftMarginMM
)

// ftCharWidth is backup's fixedCharWidth, spelled out rather than called: it is
// unexported, and it is the quantity every budget below is a function of.
// ftCheckPlateGeometry proves this reproduction against backup.CharsPerLine at
// every rung in both faces, so a divergence fails loudly instead of agreeing
// with itself.
func ftCharWidth(fnt *vector.Face, fontSize int) int {
	w, _, ok := fnt.Decode('W')
	if !ok {
		panic("W not in font")
	}
	return int(float32(w*fontSize) / float32(fnt.Metrics().Height))
}

// ftHoleChars is how many characters a screw-hole band costs a row, per side.
func ftHoleChars(P engrave.Params, fnt *vector.Face, sizeMM float32) int {
	cw := ftCharWidth(fnt, P.F(sizeMM))
	return int(math.Ceil(float64(P.F(ftInnerMM)-P.F(ftMarginMM)) / float64(cw)))
}

// ftCheckPlateGeometry pins ftPlateMM, ftMarginMM and ftCharWidth against the
// two figures backup does export. Every y and every budget in this file is
// derived from those three.
func ftCheckPlateGeometry(t *testing.T, P engrave.Params) {
	t.Helper()
	inner := P.F(ftPlateMM) - 2*P.F(ftMarginMM)
	for _, size := range backup.FontSizes {
		if got, want := backup.LinesPerPlate(P, size), inner/P.F(size); got != want {
			t.Fatalf("LinesPerPlate(%.1f) = %d, but (%.1fmm - 2*%.1fmm)/%.1fmm is %d; the plate "+
				"constants in this file are not backup's", size, got, float32(ftPlateMM),
				float32(ftMarginMM), size, want)
		}
		for _, f := range []ftFace{ftFaceSH, ftFaceConst} {
			cw := ftCharWidth(f.Face, P.F(size))
			if got, want := backup.CharsPerLine(P, f.Face, size), inner/cw; got != want {
				t.Fatalf("CharsPerLine(%s, %.1f) = %d, but this file's character width %d gives %d",
					f.Name, size, got, cw, want)
			}
		}
	}
}

// ftInk is where the tool actually CUTS, via bspline.Measure, which unions only
// engraved segments. A box over every knot would include the travel moves and
// answer a different question.
func ftInk(t testing.TB, P engrave.Params, e engrave.Engraving) bspline.Bounds {
	t.Helper()
	b := bspline.Measure(engrave.PlanEngraving(P.StepperConfig, e)).Bounds
	if b.Empty() {
		t.Fatal("engraving cut nothing")
	}
	return b
}

// ftBlockPin is one row of spec 1.1's table: one block of one side.
//
// budgets is the per-row CHARACTER BUDGET, which is not the same thing as the
// row's length -- the sweep is 95 characters and the last row of a block
// carries the remainder. It is pinned by wrapping a spaceless run measured to
// each row's budget, so a wrong number produces a wrong ROW rather than merely
// room for one; see ftBudgetProbe.
type ftBlockPin struct {
	face    ftFace
	sizeMM  float32
	rows    int
	budgets []int
	// fromMM and toMM are the block's device-unit y range, stated in the
	// millimetres spec 1.1 measured it in. Both convert exactly: every rung is
	// a whole number of device units at 6400 per millimetre.
	fromMM, toMM float32
}

// The FRONT, title at 3.8mm, body starting at y = 6.800mm. Spec 1.1.
var ftFrontTable = []ftBlockPin{
	{ftFaceSH, 5.0, 4, []int{20, 26, 26, 26}, 6.800, 26.800},
	{ftFaceConst, 5.0, 5, []int{23, 23, 23, 23, 23}, 26.800, 51.800},
	{ftFaceSH, 3.8, 3, []int{34, 34, 34}, 51.800, 63.200},
	{ftFaceConst, 3.8, 4, []int{31, 31, 31, 25}, 63.200, 78.400},
}

// The BACK, title at 3.0mm, body starting at y = 6.000mm. Spec 1.1.
var ftBackTable = []ftBlockPin{
	{ftFaceSH, 4.4, 4, []int{24, 30, 30, 30}, 6.000, 23.600},
	{ftFaceConst, 4.4, 4, []int{26, 26, 26, 26}, 23.600, 41.200},
	{ftFaceSH, 3.4, 3, []int{38, 38, 38}, 41.200, 51.400},
	{ftFaceConst, 3.4, 3, []int{34, 34, 34}, 51.400, 61.600},
	{ftFaceSH, 3.0, 3, []int{44, 44, 44}, 61.600, 70.600},
	{ftFaceConst, 3.0, 3, []int{39, 31, 31}, 70.600, 79.600},
}

// ftSideTable names a side by the numbers spec 1 states for it as a whole.
type ftSideTable struct {
	trigger  string
	blocks   []ftBlockPin
	titleMM  float32 // the title's own rung, the side's smallest
	totalMM  float32 // where the body ends
	spareMM  float32 // what is left of the 82.000mm limit
	sweeps   int     // one per block: the whole 95-character sweep, per rung per face
	titleLen int
	// titleSpan is the inset span the title is centred in, in COLUMNS of its
	// own face at its own rung. Spec 5 measured 26 on the front and 36 on the
	// back; the titles are 13 and 16.
	titleSpan int
}

var ftSideTables = []ftSideTable{
	{ftProofTriggerSizeFront, ftFrontTable, 3.8, 78.400, 3.600, 4, 13, 26},
	{ftProofTriggerSizeBack, ftBackTable, 3.0, 79.600, 2.400, 6, 16, 36},
}

// ftLadderPlate resolves a side exactly as the flow does, through the trigger
// table and ftEvaluate, so no test here can address a plate the operator could
// not reach.
func ftLadderPlate(t *testing.T, P engrave.Params, trigger string) (*ftProof, backup.Fitted) {
	t.Helper()
	p := ftSizeProofFor(t, trigger)
	f := ftEvaluate(P, p.Plan, p.Text, p.Title, p.Footer, false, 0)
	if f.err != nil {
		t.Fatalf("%s: the ladder does not fit: %v", trigger, f.err)
	}
	if !f.plate.Mixed {
		t.Fatalf("%s: the plate came out uniform at %.1fmm; it is not a ladder", trigger, f.plate.SizeMM)
	}
	return p, f.plate
}

// ftOneRow is a bare plate carrying s alone in fnt at sizeMM, cut on the very
// first body row -- the plate's top margin, since there is no title.
//
// It is the CONTROL the ladder's rows are measured against: the glyph's own
// vertical bearing is a property of (s, fnt, sizeMM) and cancels when the same
// three are cut twice, so the difference between a ladder row's ink and this
// one's is the running y and nothing else.
func ftOneRow(s string, fnt *vector.Face, sizeMM float32) backup.Fitted {
	return backup.Fitted{
		Sizes:      []float32{sizeMM},
		Lines:      []string{s},
		Faces:      []*vector.Face{fnt},
		TitleFace:  fnt,
		FooterFace: fnt,
	}
}

// ftOnlyRow is f with every body line blanked but row i, so the ink measured on
// it is that row's. The title stays: removing it would move the whole body up
// by its row, which is the very quantity being measured.
func ftOnlyRow(f backup.Fitted, i int) backup.Fitted {
	g := f
	g.Lines = make([]string, len(f.Lines))
	g.Lines[i] = f.Lines[i]
	return g
}

// ftBudgetProbe is the ladder's own composition with every block's text
// replaced by a spaceless run measured to exactly the sum of that block's
// pinned budgets, and `extra` characters added to block `at` (-1 for none).
//
// A greedy wrap fills each row to its budget and no further, so the exact run
// spans exactly the pinned rows and every row's LENGTH is its BUDGET -- which
// the 95-character sweep can never show, because each block's last row carries
// the remainder. A budget written too LARGE gives a row that cannot hold its
// share and the block spills into another row.
//
// A budget written too SMALL is the direction the exact run cannot see, and it
// is the one spec 1.2 is about: a block's last budget is where the bottom band
// bites, and the exact run's last row is the remainder, which equals the pinned
// last budget by construction whatever the real one is. Measured -- removing
// the bottom band's half of the hole predicate left the per-row lengths
// identical. That is what `extra` is for: one character past the sum must cost
// the block a ROW, and one more row of any rung overflows both sides' spare, so
// the fit must refuse it.
func ftBudgetProbe(table []ftBlockPin, at, extra int) string {
	parts := make([]string, len(table))
	for i, b := range table {
		total := 0
		for _, n := range b.budgets {
			total += n
		}
		if i == at {
			total += extra
		}
		parts[i] = strings.Repeat("W", total)
	}
	return strings.Join(parts, "\n")
}

// TestSizeLadderComposition is spec 7.3: the two measured tables, as
// assertions.
func TestSizeLadderComposition(t *testing.T) {
	P := proofParams()
	ftCheckPlateGeometry(t, P)
	for _, side := range ftSideTables {
		t.Run(side.trigger, func(t *testing.T) {
			p, plate := ftLadderPlate(t, P, side.trigger)

			// The table's own arithmetic first: the blocks are contiguous, each
			// spans rows*size, and the last one ends at the stated total. A
			// typo in a y below would otherwise be compared only against
			// itself.
			for i, b := range side.blocks {
				if len(b.budgets) != b.rows {
					t.Fatalf("block %d states %d rows and %d budgets", i, b.rows, len(b.budgets))
				}
				if got, want := P.F(b.toMM)-P.F(b.fromMM), b.rows*P.F(b.sizeMM); got != want {
					t.Fatalf("block %d spans y[%d,%d), %d units, but %d rows of %.1fmm is %d",
						i, P.F(b.fromMM), P.F(b.toMM), got, b.rows, b.sizeMM, want)
				}
				if i > 0 && b.fromMM != side.blocks[i-1].toMM {
					t.Fatalf("block %d starts at %.3fmm and block %d ended at %.3fmm; the blocks "+
						"are not contiguous", i, b.fromMM, i-1, side.blocks[i-1].toMM)
				}
			}
			last := side.blocks[len(side.blocks)-1]
			if last.toMM != side.totalMM {
				t.Fatalf("the last block ends at %.3fmm, but the side's total is %.3fmm", last.toMM, side.totalMM)
			}
			if got := ftLimitMM - side.totalMM; math.Abs(float64(got-side.spareMM)) > 1e-4 {
				t.Fatalf("the body ends %.3fmm short of the %.3fmm limit, not the measured %.3fmm",
					got, float32(ftLimitMM), side.spareMM)
			}
			// The body's first row starts at the margin plus the TITLE's own
			// row, and the title is cut at the side's smallest rung.
			if want := P.F(ftMarginMM) + P.F(side.titleMM); P.F(side.blocks[0].fromMM) != want {
				t.Fatalf("the body starts at %d, want the %.1fmm margin plus a %.1fmm title row, %d",
					P.F(side.blocks[0].fromMM), float32(ftMarginMM), side.titleMM, want)
			}
			if plate.TitleSizeMM != side.titleMM {
				t.Errorf("the title is cut at %.1fmm, want %.1fmm", plate.TitleSizeMM, side.titleMM)
			}
			if len(side.blocks) != len(p.Plan.Runs) {
				t.Fatalf("the table has %d blocks against the plan's %d runs", len(side.blocks), len(p.Plan.Runs))
			}

			// (1) Face, size and row count, row by row down the plate.
			var wantSizes []float32
			var wantFaces []*vector.Face
			for _, b := range side.blocks {
				for range b.rows {
					wantSizes = append(wantSizes, b.sizeMM)
					wantFaces = append(wantFaces, b.face.Face)
				}
			}
			if !slices.Equal(plate.Sizes, wantSizes) {
				t.Fatalf("the plate is cut at %s,\n                     want %s",
					ftSizes(plate.Sizes), ftSizes(wantSizes))
			}
			if !slices.Equal(plate.Faces, wantFaces) {
				t.Fatalf("the plate is cut in %s,\n                     want %s",
					ftFaceNames(plate.Faces), ftFaceNames(wantFaces))
			}
			if len(plate.Lines) != len(wantSizes) {
				t.Fatalf("the plate has %d rows, want %d", len(plate.Lines), len(wantSizes))
			}

			// (2) The per-row BUDGETS, twice over and independently.
			//
			// First analytically, from the plate geometry and the row's own y:
			// this is where spec 1.2's two bands are stated, and it is what
			// says WHY a budget is what it is rather than only that it is.
			topBand, bottomBand := 0, 0
			for _, b := range side.blocks {
				cols := backup.CharsPerLine(P, b.face.Face, b.sizeMM)
				hole := ftHoleChars(P, b.face.Face, b.sizeMM)
				for j, want := range b.budgets {
					y := P.F(b.fromMM) + j*P.F(b.sizeMM)
					top := y < P.F(ftInnerMM)
					bottom := y+P.F(b.sizeMM) > P.F(ftPlateMM)-P.F(ftInnerMM)
					got := cols
					if top || bottom {
						got -= 2 * hole
					}
					if top {
						topBand++
					}
					if bottom {
						bottomBand++
					}
					if got != want {
						t.Errorf("%s@%.1f row %d at y=%d holds %d characters (%d columns, %d per "+
							"screw-hole band, top=%v bottom=%v); the table says %d",
							b.face.Name, b.sizeMM, j, y, got, cols, hole, top, bottom, want)
					}
				}
			}
			// Spec 1.2: BOTH bands bite on both sides, and the bottom one lands
			// inside the LAST block. A side where one of them narrowed nothing
			// would pin half of what this item exists for.
			if topBand == 0 || bottomBand == 0 {
				t.Errorf("%d rows are narrowed by the top band and %d by the bottom one; spec 1.2 "+
					"measured both biting on this side", topBand, bottomBand)
			}
			for i, b := range side.blocks[:len(side.blocks)-1] {
				for j := range b.budgets {
					y := P.F(b.fromMM) + j*P.F(b.sizeMM)
					if y+P.F(b.sizeMM) > P.F(ftPlateMM)-P.F(ftInnerMM) {
						t.Errorf("block %d row %d is in the bottom band; spec 1.2 measured it landing "+
							"in the LAST block", i, j)
					}
				}
			}

			// Then observably, by wrapping a run measured to each block's
			// budgets through the SAME path the plate takes. Every row's length
			// is then its budget -- the last row included, which the 95-
			// character sweep can never show because it carries the remainder.
			//
			// The analytic pass above is a check on the TABLE, not on the
			// package: it re-derives the arithmetic here, so a change to the
			// real predicate leaves it agreeing with itself. This probe and the
			// overflow sweep below are what bind the table to production.
			probe := ftBudgetProbe(side.blocks, -1, 0)
			pf, err := ftFitAt(P, p.Plan.Blocks(probe), p.Title, p.Footer, false, 0)
			if err != nil {
				t.Fatalf("the budget probe does not fit: %v; a budget in the table is wrong", err)
			}
			if !slices.Equal(pf.Sizes, wantSizes) || !slices.Equal(pf.Faces, wantFaces) {
				t.Fatalf("the budget probe wraps to %s in %s, not the plate's %s in %s; the sum of "+
					"some block's budgets is not that block's rows",
					ftSizes(pf.Sizes), ftFaceNames(pf.Faces), ftSizes(wantSizes), ftFaceNames(wantFaces))
			}
			row := 0
			for i, b := range side.blocks {
				for j, want := range b.budgets {
					if got := len(pf.Lines[row]); got != want {
						t.Errorf("block %d (%s@%.1f) row %d wrapped %d characters; its budget is %d",
							i, b.face.Name, b.sizeMM, j, got, want)
					}
					row++
				}
			}

			// (3) The device-unit y ranges, MEASURED. Each block's first row is
			// cut alone and its ink compared against the same line cut on a
			// bare plate's first row, so the difference is the running y and
			// not the glyph's own bearing.
			row = 0
			for i, b := range side.blocks {
				got := ftInk(t, P, backup.EngraveFitted(P, ftOnlyRow(plate, row)))
				ctrl := ftInk(t, P, backup.EngraveFitted(P, ftOneRow(plate.Lines[row], b.face.Face, b.sizeMM)))
				if want := P.F(b.fromMM) - P.F(ftMarginMM); got.Max.Y-ctrl.Max.Y != want {
					t.Errorf("block %d (%s@%.1f) is cut %d device units below the top margin, want %d "+
						"(%.3fmm)", i, b.face.Name, b.sizeMM, got.Max.Y-ctrl.Max.Y, want, b.fromMM)
				}
				row += b.rows
			}

			// (4) The TOTAL, and the spare. The whole plate's lowest ink lies
			// inside the last row's own band, so the body really does end where
			// the table says and the spare below it is real steel.
			whole := ftInk(t, P, backup.EngraveFitted(P, plate))
			lastTop := P.F(last.toMM) - P.F(last.sizeMM)
			if whole.Max.Y > P.F(last.toMM) || whole.Max.Y <= lastTop {
				t.Errorf("the plate inks to y=%d (%.3fmm); the last row spans [%d,%d) and the body "+
					"is measured to end at %.3fmm", whole.Max.Y, float64(whole.Max.Y)/float64(P.Millimeter),
					lastTop, P.F(last.toMM), last.toMM)
			}
			if whole.Max.Y >= P.F(ftLimitMM) {
				t.Errorf("the plate inks to y=%d, at or past the %.3fmm limit", whole.Max.Y, float32(ftLimitMM))
			}

			// And no budget is UNDERSTATED. One character past a block's own
			// budgets must cost that block a row, and one more row of any rung
			// on either side overflows the spare -- so every one of these is
			// refused, and a budget that is really larger than the table says
			// shows up here as a plate that still fits.
			//
			// This is the half the exact probe is blind to. A block's last
			// budget is where the bottom band bites, and the exact run's last
			// row is the remainder, which equals the pinned last budget by
			// construction. Measured: with the bottom band's half of the hole
			// predicate removed, const@3.8's real budgets are [31 31 31 31] and
			// every row of the exact probe still came out [31 31 31 25].
			for i, b := range side.blocks {
				text := ftBudgetProbe(side.blocks, i, 1)
				if f, err := ftFitAt(P, p.Plan.Blocks(text), p.Title, p.Footer, false, 0); err == nil {
					t.Errorf("block %d (%s@%.1f) holds one character more than its budgets %v and the "+
						"plate still fits, wrapping to %s; the %.3fmm of spare is at least one %.1fmm "+
						"row, or that block's budgets are understated",
						i, b.face.Name, b.sizeMM, b.budgets, ftSizes(f.Sizes), side.spareMM, b.sizeMM)
				}
			}
		})
	}
}

// TestSizeLadderSweepsEveryGlyphPerRungAndFace is spec 7.4: the character SET
// per (rung, face) PAIR, never per plate.
//
// "95 characters in both faces" is satisfiable by a full sweep at one rung and
// a truncated one at another -- which is precisely the plate this pair of
// triggers exists to avoid, since what the ladder answers is which glyphs
// survive as the size drops. A rung missing a glyph answers nothing about it.
func TestSizeLadderSweepsEveryGlyphPerRungAndFace(t *testing.T) {
	P := proofParams()
	// The sweep itself: every printable ASCII character, once each, in
	// codepoint order. Stated here as the RANGE rather than compared against
	// the constant, which would pass however short the constant became.
	var want []byte
	for c := byte(0x20); c <= 0x7e; c++ {
		want = append(want, c)
	}
	if len(want) != 95 {
		t.Fatalf("printable ASCII is %d characters", len(want))
	}
	if ftProofSweep != string(want) {
		t.Fatalf("the sweep is not every printable ASCII character in order:\n got %q\nwant %q",
			ftProofSweep, string(want))
	}

	for _, side := range ftSideTables {
		t.Run(side.trigger, func(t *testing.T) {
			_, plate := ftLadderPlate(t, P, side.trigger)
			type pair struct {
				face *vector.Face
				size float32
			}
			// Grouped by (rung, face) in PLATE ORDER, from the fit's own
			// parallel maps -- the faces the rows were wrapped in and the sizes
			// they will be cut at, not the plan's declaration of them.
			var order []pair
			text := map[pair]string{}
			for i, l := range plate.Lines {
				k := pair{plate.Faces[i], plate.Sizes[i]}
				if _, ok := text[k]; !ok {
					order = append(order, k)
				}
				text[k] += l
			}
			if len(order) != side.sweeps {
				t.Fatalf("the plate carries %d (rung, face) pairs, want %d -- one sweep per band",
					len(order), side.sweeps)
			}
			// Every rung of the plan, in BOTH faces. A pair absent here is a
			// rung this side claims to prove and does not.
			for _, b := range side.blocks {
				k := pair{b.face.Face, b.sizeMM}
				got, ok := text[k]
				if !ok {
					t.Fatalf("nothing on the plate is cut in %s at %.1fmm", b.face.Name, b.sizeMM)
				}
				if got != ftProofSweep {
					t.Errorf("%s@%.1f carries %d characters %q,\n            want the whole %d-character sweep %q",
						b.face.Name, b.sizeMM, len(got), got, len(ftProofSweep), ftProofSweep)
				}
				// And every one of them is a glyph THAT face can cut: a
				// character the face cannot decode engraves nothing at all, and
				// a blank on the plate is not an answer about legibility.
				for _, r := range got {
					if _, _, ok := b.face.Face.Decode(r); !ok {
						t.Errorf("%s cannot decode %q, so %.1fmm proves nothing about it",
							b.face.Name, r, b.sizeMM)
					}
				}
			}
			// The pairs are the CROSS of the side's rungs and the two faces, so
			// no rung is proven in one face only.
			for _, rung := range ftPlanRungsOf(t, side.trigger) {
				for _, f := range []ftFace{ftFaceSH, ftFaceConst} {
					if _, ok := text[pair{f.Face, rung}]; !ok {
						t.Errorf("%.1fmm is not proven in %s", rung, f.Name)
					}
				}
			}
		})
	}
}

// ftPlanRungsOf is the side's distinct rungs, read off the shipped plan.
func ftPlanRungsOf(t *testing.T, trigger string) []float32 {
	t.Helper()
	return ftSizeProofFor(t, trigger).Plan.Rungs()
}

// TestSizeLadderTitlesFitTheirScrewHoleRow is spec 7.12 and spec 5's identity
// decision.
//
// EngraveFitted engraves Title VERBATIM -- never through TitleString, which
// upper-cases and truncates -- so nothing downstream trims a title that is too
// long. It has to fit three things at once: MaxTitleLen, the INSET SPAN of its
// own face at its own rung (it is centred between the screw-hole bands, not on
// the plate), and its face's glyph table.
func TestSizeLadderTitlesFitTheirScrewHoleRow(t *testing.T) {
	P := proofParams()
	ftCheckPlateGeometry(t, P)
	for _, side := range ftSideTables {
		t.Run(side.trigger, func(t *testing.T) {
			p, plate := ftLadderPlate(t, P, side.trigger)
			title := p.Title
			if plate.Title != title {
				t.Fatalf("the plate's title is %q and the proof's is %q", plate.Title, title)
			}
			if len(title) != side.titleLen {
				t.Errorf("the title %q is %d characters, want the measured %d", title, len(title), side.titleLen)
			}
			if len(title) > backup.MaxTitleLen {
				t.Errorf("the title %q is %d characters, past MaxTitleLen %d",
					title, len(title), backup.MaxTitleLen)
			}
			// The face: the title takes blocks[0].Face, which is font/sh on
			// both sides, and its rung is the side's smallest.
			if plate.TitleFace != ftFaceSH.Face {
				t.Errorf("the title is not cut in %s", ftFaceSH.Name)
			}
			if plate.TitleSizeMM != side.titleMM {
				t.Errorf("the title is cut at %.1fmm, want %.1fmm", plate.TitleSizeMM, side.titleMM)
			}
			// It decodes in the face it is cut in, character for character. A
			// rune the face cannot decode engraves nothing, so the plate would
			// carry a title with a hole in it.
			for _, r := range title {
				if _, _, ok := plate.TitleFace.Decode(r); !ok {
					t.Errorf("%q is not in the title's face", r)
				}
			}
			// And it survives TitleString unchanged, so the verbatim engraving
			// and the sanitised form are the same string -- the plate cannot
			// carry something the ordinary title path would have rejected.
			if got := backup.TitleString(plate.TitleFace, title); got != title {
				t.Errorf("TitleString turns the title %q into %q; the plate engraves the former",
					title, got)
			}

			// The INSET SPAN, in columns of the title's own face and rung.
			cols := backup.CharsPerLine(P, plate.TitleFace, side.titleMM)
			span := cols - 2*ftHoleChars(P, plate.TitleFace, side.titleMM)
			if span != side.titleSpan {
				t.Fatalf("the inset span at %s %.1fmm is %d columns of %d, want the measured %d",
					ftFaceSH.Name, side.titleMM, span, cols, side.titleSpan)
			}
			if len(title) > span {
				t.Errorf("the title %q is %d characters and the span it is centred in holds %d",
					title, len(title), span)
			}

			// MEASURED: the title alone on the plate. Its ink must stay inside
			// the INSET SPAN it is centred in, which is a whole number of the
			// face's characters and is therefore TIGHTER than the screw-hole
			// clearance -- centring a title on the full plate width is what
			// spec 8 measured crossing both bands while every check passed.
			cw := ftCharWidth(plate.TitleFace, P.F(side.titleMM))
			inset := ftHoleChars(P, plate.TitleFace, side.titleMM) * cw
			lo, hi := P.F(ftMarginMM)+inset, P.F(ftPlateMM)-P.F(ftMarginMM)-inset
			if lo < P.F(ftInnerMM) || hi > P.F(ftPlateMM)-P.F(ftInnerMM) {
				t.Fatalf("the inset span [%d,%d] reaches outside the screw-hole clearance [%d,%d]",
					lo, hi, P.F(ftInnerMM), P.F(ftPlateMM)-P.F(ftInnerMM))
			}
			bare := plate
			bare.Lines = make([]string, len(plate.Lines))
			b := ftInk(t, P, backup.EngraveFitted(P, bare))
			if b.Min.X < lo || b.Max.X > hi {
				t.Errorf("the title inks x[%d,%d], outside the inset span [%d,%d] it is centred in",
					b.Min.X, b.Max.X, lo, hi)
			}
			if b.Min.Y < P.F(ftMarginMM) || b.Max.Y > P.F(ftMarginMM)+P.F(side.titleMM) {
				t.Errorf("the title inks y[%d,%d], outside its own row [%d,%d)",
					b.Min.Y, b.Max.Y, P.F(ftMarginMM), P.F(ftMarginMM)+P.F(side.titleMM))
			}
			// The column bound is CONSERVATIVE, and the ink measurement above
			// has a wrong answer to give. Every title the span admits inks
			// inside it, and some longer one does not.
			//
			// The two bounds are not the same number, which is worth stating
			// rather than assuming: centerInset centres the string's MEASURED
			// ink, so the leading glyph's left bearing absorbs the first sliver
			// of overflow and a title one character past the span can still ink
			// inside. Measured at font/sh 3.8mm, 27 characters do. The direction that
			// matters is the one asserted -- the span never ADMITS a title whose
			// ink crosses -- and the sweep below finds where the ink does give
			// way, so this assertion is not free.
			for n := 1; n <= span; n++ {
				fits := bare
				fits.Title = strings.Repeat("W", n)
				if fb := ftInk(t, P, backup.EngraveFitted(P, fits)); fb.Min.X < lo || fb.Max.X > hi {
					t.Fatalf("a %d-character title inks x[%d,%d) outside the %d-column span [%d,%d]; "+
						"the span admits a title the plate cannot hold", n, fb.Min.X, fb.Max.X, span, lo, hi)
				}
			}
			crossed := 0
			for n := span + 1; n <= 2*span && crossed == 0; n++ {
				over := bare
				over.Title = strings.Repeat("W", n)
				ob := ftInk(t, P, backup.EngraveFitted(P, over))
				if ob.Min.X < lo || ob.Max.X > hi {
					crossed = n
				}
			}
			if crossed == 0 {
				t.Errorf("no title up to %d characters inks outside the %d-column span [%d,%d]; the "+
					"measurement above cannot fail", 2*span, span, lo, hi)
			} else if crossed <= span {
				t.Errorf("a %d-character title crosses the span, which holds %d", crossed, span)
			}
		})
	}
}

// TestSizeLadderTitlesNameTheirOwnSide guards the one thing spec 4 says a
// mis-pick costs: both sides prove IDENTICAL faces, so the title is the only
// mark on the steel that says which half was cut.
func TestSizeLadderTitlesNameTheirOwnSide(t *testing.T) {
	seen := map[string]string{}
	for _, side := range ftSideTables {
		p := ftSizeProofFor(t, side.trigger)
		if other, dup := seen[p.Title]; dup {
			t.Errorf("%s and %s carry the same title %q", side.trigger, other, p.Title)
		}
		seen[p.Title] = side.trigger
		if !strings.HasPrefix(p.Title, p.Side+" ") {
			t.Errorf("%s: the title %q does not open with the side %q", side.trigger, p.Title, p.Side)
		}
		var rungs []string
		for _, b := range side.blocks {
			r := fmt.Sprintf("%.1f", b.sizeMM)
			if !slices.Contains(rungs, r) {
				rungs = append(rungs, r)
			}
		}
		if want := p.Side + " " + strings.Join(rungs, "+"); p.Title != want {
			t.Errorf("%s: the title is %q, want %q -- the side and the rungs this table measures",
				side.trigger, p.Title, want)
		}
	}
}
