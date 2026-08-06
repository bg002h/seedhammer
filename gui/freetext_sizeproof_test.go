package gui

import (
	"errors"
	"fmt"
	"image"
	"slices"
	"strings"
	"testing"

	"seedhammer.com/backup"
	"seedhammer.com/font/vector"
	"seedhammer.com/gui/op"
)

// The size-ladder plate: spec 1's two sides, spec 3's GUI surface and spec
// 3.1's edit path. Every vector asserted here is MEASURED against the real
// fonts and the real sh2.Params() -- see spec 1.1's two tables, which this file
// reproduces on the resolved Fitted rather than on the block list.

// ftSizes flattens a size vector into something a failure message can be read
// from at a glance.
func ftSizes(s []float32) string {
	parts := make([]string, len(s))
	for i, v := range s {
		parts[i] = fmt.Sprintf("%.1f", v)
	}
	return "[" + strings.Join(parts, " ") + "]"
}

// ftFaceNames names a face map the same way.
func ftFaceNames(f []*vector.Face) string {
	parts := make([]string, len(f))
	for i, v := range f {
		switch v {
		case ftFaceSH.Face:
			parts[i] = "sh"
		case ftFaceConst.Face:
			parts[i] = "const"
		default:
			parts[i] = "?"
		}
	}
	return "[" + strings.Join(parts, " ") + "]"
}

// ftRepeat is the expected size vector as (value, count) runs, so the tables
// below read like spec 1.1's.
func ftRepeat(runs ...any) []float32 {
	var out []float32
	for i := 0; i < len(runs); i += 2 {
		v := float32(runs[i].(float64))
		for range runs[i+1].(int) {
			out = append(out, v)
		}
	}
	return out
}

func ftFaceRepeat(runs ...any) []*vector.Face {
	var out []*vector.Face
	for i := 0; i < len(runs); i += 2 {
		f := runs[i].(ftFace).Face
		for range runs[i+1].(int) {
			out = append(out, f)
		}
	}
	return out
}

// ftSizeProofFor is the ladder proof for a side, looked up through the trigger
// table so a test cannot address a proof the flow would not reach.
func ftSizeProofFor(t *testing.T, trigger string) *ftProof {
	t.Helper()
	p, rung, ok := ftProofForTrigger(trigger)
	if !ok {
		t.Fatalf("%q is not a trigger", trigger)
	}
	// The ladder's rung is 0, and BOTH of ftFitAt's routing rules depend on it:
	// a non-zero size beside sized blocks is an error, not a uniform fit.
	if rung != 0 {
		t.Fatalf("%q resolved to rung %.1f; the ladder proofs must not be Sizeable", trigger, rung)
	}
	return p
}

// The two ladders, exactly as spec 1.1 measured them.
var (
	ftWantFrontSizes = ftRepeat(5.0, 9, 3.8, 7)
	ftWantFrontFaces = ftFaceRepeat(ftFaceSH, 4, ftFaceConst, 5, ftFaceSH, 3, ftFaceConst, 4)
	ftWantBackSizes  = ftRepeat(4.4, 8, 3.4, 6, 3.0, 6)
	ftWantBackFaces  = ftFaceRepeat(ftFaceSH, 4, ftFaceConst, 4, ftFaceSH, 3,
		ftFaceConst, 3, ftFaceSH, 3, ftFaceConst, 3)
)

// TestSizeProofPlansAreWellFormed pins the shape every OTHER test here depends
// on, and the two properties spec 7.13 calls out because ftPlan.Blocks cannot
// enforce them:
//
//   - every run of a SIZED plan declares Blocks >= 1, since the part-count
//     predicate is the SUM of those declarations and a run declaring 0 would
//     make the plan unreachable at its own part count;
//   - a sized plan has more than one run, because Blocks' single-run early
//     return has no split to walk.
func TestSizeProofPlansAreWellFormed(t *testing.T) {
	for _, tc := range []struct {
		trigger string
		side    string
		title   string
		rungs   []float32
		faces   []ftFace
		// runSizes is the size of every run IN ORDER, and it is not the same
		// claim as rungs: Rungs() deduplicates, so a band moved onto a rung the
		// plan already carries leaves that list untouched.
		runSizes []float32
	}{
		{ftProofTriggerSizeFront, "FRONT", "FRONT 5.0+3.8", []float32{5.0, 3.8},
			[]ftFace{ftFaceSH, ftFaceConst, ftFaceSH, ftFaceConst},
			[]float32{5.0, 5.0, 3.8, 3.8}},
		{ftProofTriggerSizeBack, "BACK", "BACK 4.4+3.4+3.0", []float32{4.4, 3.4, 3.0},
			[]ftFace{ftFaceSH, ftFaceConst, ftFaceSH, ftFaceConst, ftFaceSH, ftFaceConst},
			[]float32{4.4, 4.4, 3.4, 3.4, 3.0, 3.0}},
	} {
		p := ftSizeProofFor(t, tc.trigger)
		if p.Side != tc.side {
			t.Errorf("%s: Side is %q, want %q -- the prompt names the side, not the plan",
				tc.trigger, p.Side, tc.side)
		}
		if p.Sizeable {
			t.Errorf("%s: the proof is Sizeable, so %s4.4 would be ambiguous against the rung suffix",
				tc.trigger, tc.trigger)
		}
		if !p.NeedsWholePlate() {
			t.Errorf("%s: TextQR is not empty, so the prompted QR drop does not apply", tc.trigger)
		}
		if p.Footer != "" {
			t.Errorf("%s: the footer is %q; spec 5 measured a footer refusing this side outright",
				tc.trigger, p.Footer)
		}
		if !p.Plan.Sized() {
			t.Fatalf("%s: the plan states no sizes, so the plate would be one uniform rung", tc.trigger)
		}
		if len(p.Plan.Runs) < 2 {
			t.Fatalf("%s: a sized plan with %d run(s) never reaches Blocks' split, so its sizes "+
				"could never be cleared", tc.trigger, len(p.Plan.Runs))
		}
		if len(p.Plan.Runs) != len(tc.faces) || len(tc.faces) != len(tc.runSizes) {
			t.Fatalf("%s: %d runs, want %d", tc.trigger, len(p.Plan.Runs), len(tc.faces))
		}
		for i, r := range p.Plan.Runs {
			if r.Blocks < 1 {
				t.Errorf("%s: run %d declares %d blocks; the part-count predicate sums these",
					tc.trigger, i, r.Blocks)
			}
			if r.Face != tc.faces[i] {
				t.Errorf("%s: run %d is cut in %s, want %s", tc.trigger, i, r.Face.Name, tc.faces[i].Name)
			}
			if r.SizeMM != tc.runSizes[i] {
				t.Errorf("%s: run %d is cut at %.1fmm, want %.1fmm", tc.trigger, i, r.SizeMM, tc.runSizes[i])
			}
			if r.SizeMM == 0 {
				t.Errorf("%s: run %d states no size", tc.trigger, i)
			}
			if !slices.Contains(backup.FontSizes, r.SizeMM) {
				t.Errorf("%s: run %d is at %.1fmm, which is not a rung in FontSizes %v",
					tc.trigger, i, r.SizeMM, backup.FontSizes)
			}
		}
		if got := p.Plan.Rungs(); !slices.Equal(got, tc.rungs) {
			t.Errorf("%s: the plan's rungs are %v, want %v", tc.trigger, got, tc.rungs)
		}
		if got := p.Plan.declaredParts(); got != len(p.Plan.Runs) {
			t.Errorf("%s: the plan declares %d parts over %d runs; both ladders are one part per run",
				tc.trigger, got, len(p.Plan.Runs))
		}
		// The title is the SIDE and its rungs, and it is built here from the
		// side and the plan rather than by calling the constructor: a title
		// compared against itself passes however wrong it is.
		var rungs []string
		for _, r := range tc.rungs {
			rungs = append(rungs, fmt.Sprintf("%.1f", r))
		}
		if want := tc.side + " " + strings.Join(rungs, "+"); p.Title != want {
			t.Errorf("%s: the title is %q, want %q", tc.trigger, p.Title, want)
		}
		if p.Title != tc.title {
			t.Errorf("%s: the title is %q, want the measured %q", tc.trigger, p.Title, tc.title)
		}
		// One part per run, each of them the whole 95-character sweep: the
		// plate exists to show every glyph at every rung in both faces.
		parts := strings.Split(p.Text, "\n")
		if len(parts) != len(p.Plan.Runs) {
			t.Fatalf("%s: the pattern has %d parts against %d runs, so it would revert on load",
				tc.trigger, len(parts), len(p.Plan.Runs))
		}
		for i, part := range parts {
			if part != ftProofSweep {
				t.Errorf("%s: part %d is not the codepoint sweep", tc.trigger, i)
			}
		}
	}
}

// TestSizeProofIsTheLadderEndToEnd is spec 7.10, and it is the ONLY thing that
// catches R0's C6: without the per-block sizes reaching the fit, six sweeps fit
// uniformly at 3.0mm, FitBlocks succeeds, the confirm screen reads "3.0mm 18
// lines" and the operator approves a five-rung survey and receives one rung.
//
// Driven through the real flow by touch, and asserted on what freetextPlateHook
// was HANDED -- Plate is {Duration, Spline}, stroke geometry with no sizes in
// it at all.
func TestSizeProofIsTheLadderEndToEnd(t *testing.T) {
	for _, tc := range []struct {
		trigger string
		sizes   []float32
		faces   []*vector.Face
	}{
		{ftProofTriggerSizeFront, ftWantFrontSizes, ftWantFrontFaces},
		{ftProofTriggerSizeBack, ftWantBackSizes, ftWantBackFaces},
	} {
		t.Run(tc.trigger, func(t *testing.T) {
			h, r := startFT(t)
			ftPastQR(h, false)
			ftTypeTrigger(h, tc.trigger)
			ftOK(h)
			if !uiContains(h.content, "REPLACES ALL THREE") {
				t.Fatalf("OK on %s did not offer the pattern; frame %q", tc.trigger, h.content)
			}
			h.tapWidget("proofYes")
			h.mustReach("lines")
			for _, step := range []string{"Title", "Footer", "Confirm"} {
				ftOK(h)
				h.mustReach(step)
			}
			ftOK(h)
			h.step()
			if !r.gotPlate {
				t.Fatal("the flow never built a plate")
			}
			if !r.got.Mixed {
				t.Errorf("the engraved plate is not Mixed: it was cut at the single size %.1fmm, "+
					"so the operator approved a size ladder and received one rung", r.got.SizeMM)
			}
			if !slices.Equal(r.got.Sizes, tc.sizes) {
				t.Errorf("the engraved plate is cut at %s,\n                        want %s",
					ftSizes(r.got.Sizes), ftSizes(tc.sizes))
			}
			if !slices.Equal(r.got.Faces, tc.faces) {
				t.Errorf("the engraved plate is cut in %s,\n                        want %s",
					ftFaceNames(r.got.Faces), ftFaceNames(tc.faces))
			}
			if len(r.got.Lines) != len(tc.sizes) {
				t.Errorf("the plate has %d rows, want %d", len(r.got.Lines), len(tc.sizes))
			}
			if r.got.Footer != "" {
				t.Errorf("the plate carries the footer %q; spec 5 measured a footer refusing "+
					"this side outright", r.got.Footer)
			}
			if r.got.QR != nil {
				t.Error("the ladder plate carries a QR; FitSized has no parameter for one")
			}
			// The title sits at the side's SMALLEST rung, not the first
			// block's: the first block's rule costs the back 1.4mm of spare.
			if want := slices.Min(tc.sizes); r.got.TitleSizeMM != want {
				t.Errorf("the title is cut at %.1fmm, want the smallest rung %.1fmm",
					r.got.TitleSizeMM, want)
			}
			// Every rung of the plan really is on the plate, in both faces.
			p := ftSizeProofFor(t, tc.trigger)
			for _, rung := range p.Plan.Rungs() {
				for _, face := range []ftFace{ftFaceSH, ftFaceConst} {
					found := false
					for i := range r.got.Sizes {
						if r.got.Sizes[i] == rung && r.got.Faces[i] == face.Face {
							found = true
						}
					}
					if !found {
						t.Errorf("no row is cut in %s at %.1fmm", face.Name, rung)
					}
				}
			}
		})
	}
}

// ---- spec 3.1: the edit path ------------------------------------------------

// ftSweeps is n copies of the sweep, joined the way the field holds them.
func ftSweeps(n int) string {
	parts := make([]string, n)
	for i := range parts {
		parts[i] = ftProofSweep
	}
	return strings.Join(parts, "\n")
}

// TestSizeProofEditShapes is spec 7.13, all FOUR shapes against the six-run
// BACK plan, asserted on the RESOLVED Fitted rather than on the block list --
// the block list is what looked right in the failing case.
//
// 5 parts and 7 parts are the two that matter. At 5 the ladder would be cut
// with const@3.0 -- the smallest and most load-bearing rung -- silently absent.
// At 7 the block count is still 6 and len(out) == len(Runs) still holds, so
// R3's predicate reported "exact shape": every band lands one run late, sh@3.0
// disappears, and the confirm screen cannot expose it because six rungs are
// reported and six rungs are cut.
func TestSizeProofEditShapes(t *testing.T) {
	P := proofParams()
	p := ftSizeProofFor(t, ftProofTriggerSizeBack)
	sh, cn := ftFaceSH, ftFaceConst
	for _, tc := range []struct {
		name  string
		parts int
		sizes []float32
		faces []*vector.Face
	}{
		{"6 parts, the exact shape: the ladder is KEPT", 6, ftWantBackSizes, ftWantBackFaces},
		// A deleted newline. Reverts to uniform auto-fit at 3.8mm over 19 rows;
		// what it must NOT be is a five-rung ladder ending at const@3.4.
		{"5 parts, a deleted newline: REVERTS", 5, ftRepeat(3.8, 19),
			ftFaceRepeat(sh, 4, cn, 4, sh, 3, cn, 4, sh, 4)},
		// An inserted newline. Reverts to uniform auto-fit at 3.4mm over 22
		// rows; what it must NOT be is six bands one run late.
		{"7 parts, an inserted newline: REVERTS", 7, ftRepeat(3.4, 22),
			ftFaceRepeat(sh, 3, cn, 3, sh, 3, cn, 3, sh, 3, cn, 7)},
		// The full collapse: one block in the first run's face.
		{"1 part, the full collapse: REVERTS", 1, ftRepeat(6.0, 5), ftFaceRepeat(sh, 5)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			text := ftSweeps(tc.parts)
			f, err := ftFitAt(P, p.Plan.Blocks(text), p.Title, p.Footer, false, 0)
			if err != nil {
				t.Fatalf("%d parts: the fit refused: %v", tc.parts, err)
			}
			if !slices.Equal(f.Sizes, tc.sizes) {
				t.Errorf("%d parts: cut at %s,\n            want %s",
					tc.parts, ftSizes(f.Sizes), ftSizes(tc.sizes))
			}
			if !slices.Equal(f.Faces, tc.faces) {
				t.Errorf("%d parts: cut in %s,\n            want %s",
					tc.parts, ftFaceNames(f.Faces), ftFaceNames(tc.faces))
			}
			kept := tc.parts == len(p.Plan.Runs)
			if f.Mixed != kept {
				t.Errorf("%d parts: Mixed=%v, want %v", tc.parts, f.Mixed, kept)
			}
			if kept {
				return
			}
			// A reverted plate is a plain free-text plate: ONE size everywhere,
			// which is what makes SizeMM printable again.
			for i, s := range f.Sizes {
				if s != f.SizeMM {
					t.Fatalf("%d parts: row %d is cut at %.1fmm on a plate reported as %.1fmm",
						tc.parts, i, s, f.SizeMM)
				}
			}
			// And it is NOT the partial ladder. Stated as the property rather
			// than as "not equal to a vector": a partial ladder is any plate
			// carrying more than one of the plan's rungs.
			seen := map[float32]bool{}
			for _, s := range f.Sizes {
				seen[s] = true
			}
			if len(seen) > 1 {
				t.Errorf("%d parts: the plate carries %d rungs %s; a shape-mismatched edit "+
					"must revert the WHOLE plate", tc.parts, len(seen), ftSizes(f.Sizes))
			}
		})
	}
}

// TestSizedPlanClearsOnPartCountNotRunCount is the case no ladder fixture can
// see. Both ladders declare one block per run, so declaredParts == len(Runs)
// for every real pattern and a len(Runs)-based predicate is indistinguishable
// from the specified one -- including under a mutation pass.
//
// This plan declares FOUR parts over THREE runs. At five parts the block list
// is still three long and len(out) == len(p.Runs) still holds, so R3's
// predicate keeps the sizes; the part count does not match, so the specified
// one clears them.
func TestSizedPlanClearsOnPartCountNotRunCount(t *testing.T) {
	P := proofParams()
	plan := &ftPlan{Runs: []ftFaceRun{
		{Face: ftFaceSH, Blocks: 2, SizeMM: 4.4},
		{Face: ftFaceConst, Blocks: 1, SizeMM: 3.4},
		{Face: ftFaceSH, Blocks: 1, SizeMM: 3.0},
	}}
	if plan.declaredParts() != 4 || len(plan.Runs) != 3 {
		t.Fatalf("this fixture needs declaredParts != len(Runs); got %d and %d",
			plan.declaredParts(), len(plan.Runs))
	}
	parts := []string{"ONE", "TWO", "THREE", "FOUR", "FIVE"}
	for _, tc := range []struct {
		n     int
		sizes []float32
		faces []*vector.Face
	}{
		// Three parts: the walk runs out inside run 1, so TWO blocks come back
		// and both predicates clear. The control.
		{3, ftRepeat(6.0, 3), ftFaceRepeat(ftFaceSH, 2, ftFaceConst, 1)},
		// Four: the declared shape. The sizes survive.
		{4, []float32{4.4, 4.4, 3.4, 3.0},
			ftFaceRepeat(ftFaceSH, 2, ftFaceConst, 1, ftFaceSH, 1)},
		// Five: THREE blocks again, len(out) == len(Runs), and the sizes must
		// still be cleared. This is the one that separates the two predicates.
		{5, ftRepeat(6.0, 5), ftFaceRepeat(ftFaceSH, 2, ftFaceConst, 1, ftFaceSH, 2)},
	} {
		t.Run(fmt.Sprintf("%d parts", tc.n), func(t *testing.T) {
			text := strings.Join(parts[:tc.n], "\n")
			blocks := plan.Blocks(text)
			if tc.n == 5 && len(blocks) != len(plan.Runs) {
				t.Fatalf("five parts produced %d blocks; this case only separates the two "+
					"predicates while the BLOCK count matches", len(blocks))
			}
			f, err := ftFitAt(P, blocks, "", "", false, 0)
			if err != nil {
				t.Fatalf("%d parts: the fit refused: %v", tc.n, err)
			}
			if !slices.Equal(f.Sizes, tc.sizes) {
				t.Errorf("%d parts: cut at %s, want %s", tc.n, ftSizes(f.Sizes), ftSizes(tc.sizes))
			}
			if !slices.Equal(f.Faces, tc.faces) {
				t.Errorf("%d parts: cut in %s, want %s", tc.n, ftFaceNames(f.Faces), ftFaceNames(tc.faces))
			}
			if want := tc.n == plan.declaredParts(); f.Mixed != want {
				t.Errorf("%d parts: Mixed=%v, want %v", tc.n, f.Mixed, want)
			}
		})
	}
}

// TestOneRunSizedPlanStillClears covers ftPlan.Blocks' single-run early return,
// which never reaches the split at all.
//
// No shipped plan is one sized run -- TestSizeProofPlansAreWellFormed forbids
// it, because such a plan could not express a ladder -- so nothing else here
// exercises that branch, and a branch that stamped unconditionally would be a
// plate whose size survived ANY edit.
func TestOneRunSizedPlanStillClears(t *testing.T) {
	plan := &ftPlan{Runs: []ftFaceRun{{Face: ftFaceSH, Blocks: 1, SizeMM: 4.4}}}
	for _, tc := range []struct {
		text string
		want float32
	}{
		{"ONE", 4.4},    // one part, as declared
		{"ONE\nTWO", 0}, // two: the shape no longer matches
		{"", 4.4},       // one empty part is still one part
		{"A\nB\nC", 0},  // three
	} {
		blocks := plan.Blocks(tc.text)
		if len(blocks) != 1 {
			t.Fatalf("%q: a single-run plan produced %d blocks", tc.text, len(blocks))
		}
		if blocks[0].SizeMM != tc.want {
			t.Errorf("%q: the block carries %.1fmm, want %.1fmm", tc.text, blocks[0].SizeMM, tc.want)
		}
	}
}

// TestUnsizedPlansAreUntouchedByTheClearing: the clearing is scoped to SizeMM
// alone, so every plan that shipped before the ladder splits exactly as it did.
//
// The EXACT branch has to be reached, and none of the texts this test carried
// until spec 7.19's mutation pass reached it. declaredParts is the SUM of the
// runs' Blocks, and every shipped unsized plan leaves its last run's Blocks at
// 0 -- so ftPlanSH and ftPlanConst declare 0 parts, which strings.Split can
// never return, and ftPlanBoth declares its sh half's 4 against texts of 1, 2,
// 6 and 9. exact was false every time, sizeOf returned 0 down its FIRST branch,
// and a sizeOf that invented a rung for an unsized run was invisible. Measured:
// returning 3.0 for a zero SizeMM left this test green.
func TestUnsizedPlansAreUntouchedByTheClearing(t *testing.T) {
	reachedExact := 0
	for _, plan := range []*ftPlan{&ftPlanSH, &ftPlanConst, &ftPlanBoth} {
		if plan.Sized() {
			t.Errorf("%s carries sizes; the shipped plans must not", plan.Name())
		}
		texts := []string{"", "one", "one\ntwo", ftProofTextBoth, ftSweeps(6)}
		// The text whose part count MATCHES the plan's declaration, which is
		// the only shape that takes the stamping branch at all.
		if n := plan.declaredParts(); n >= 1 {
			texts = append(texts, ftSweeps(n))
			reachedExact++
		}
		for _, text := range texts {
			for i, b := range plan.Blocks(text) {
				if b.SizeMM != 0 {
					t.Errorf("%s: block %d of %.20q carries %.1fmm", plan.Name(), i, text, b.SizeMM)
				}
			}
		}
	}
	if reachedExact == 0 {
		t.Error("no unsized plan declares a part count a text can match, so none of them ever " +
			"reaches the branch that stamps a size and this test cannot see one being invented")
	}
}

// ---- ftFitAt's routing ------------------------------------------------------

// TestFitAtTestsSizedBlocksFirst is spec 3's routing order. FitBlocksAt IGNORES
// Block.SizeMM, and ftFitAt is shared with the BOTHPROOF!<rung> path, which
// does set a non-zero rung. Appending the FitSized test second engraves the
// ladder at one uniform rung: R0's C6 in a third hat.
func TestFitAtTestsSizedBlocksFirst(t *testing.T) {
	P := proofParams()
	p := ftSizeProofFor(t, ftProofTriggerSizeFront)
	blocks := p.Plan.Blocks(p.Text)
	// The premise: these blocks DO fit uniformly at a rung, so a router that
	// reached FitBlocksAt would succeed rather than fail loudly.
	if _, err := backup.FitBlocksAt(P, blocks, p.Title, p.Footer, false, 3.0); err != nil {
		t.Fatalf("the front composition does not fit uniformly at 3.0mm, so this test cannot "+
			"see the routing order: %v", err)
	}
	for _, size := range backup.FontSizes {
		f, err := ftFitAt(P, blocks, p.Title, p.Footer, false, size)
		if err == nil {
			t.Errorf("a %.1fmm rung beside sized blocks produced a plate cut at %s; it must be "+
				"an error, not a silent uniform fit", size, ftSizes(f.Sizes))
		}
	}
	// With no rung named it is the ladder, not an auto-fit.
	f, err := ftFitAt(P, blocks, p.Title, p.Footer, false, 0)
	if err != nil {
		t.Fatalf("the ladder does not fit: %v", err)
	}
	if !slices.Equal(f.Sizes, ftWantFrontSizes) {
		t.Errorf("cut at %s, want %s", ftSizes(f.Sizes), ftSizes(ftWantFrontSizes))
	}
	// A partially sized composition is NOT sized: "every block carries a size"
	// is the condition, and a block at 0 would reach FitSized as "sized 0mm".
	half := slices.Clone(blocks)
	half[len(half)-1].SizeMM = 0
	if _, err := ftFitAt(P, half, p.Title, p.Footer, false, 0); err != nil {
		t.Errorf("a composition with one unsized block did not auto-fit: %v", err)
	}
}

// ---- spec 7.16: the whole plate and the QR ----------------------------------

// TestSizeProofNeedsTheWholePlate is spec 7.16. The ladder carries no QR --
// FitSized has no parameter for one -- so the operator's choice has to be
// dropped, and the only thing that keeps that from being a silent substitution
// is that the prompt says it first and the loader clears the flag BEFORE the
// outcome resolves.
func TestSizeProofNeedsTheWholePlate(t *testing.T) {
	P := proofParams()
	for _, trigger := range []string{ftProofTriggerSizeFront, ftProofTriggerSizeBack} {
		p := ftSizeProofFor(t, trigger)
		if !p.NeedsWholePlate() {
			t.Fatalf("%s: the proof does not need the whole plate", trigger)
		}
		body := ftProofAsk(p) + " " + ftProofReplaces(p, ftProofOutcomeFor(P, p, 0, false)) + " " + ftProofKeep(p)
		for _, want := range []string{"REMOVES THE QR", "whole plate"} {
			if !strings.Contains(body, want) {
				t.Errorf("%s: the prompt never says %q, so removing the QR would be silent.\nprompt: %q",
					trigger, want, body)
			}
		}
		// And the loader clears it BEFORE resolving, which is what makes the
		// resolved outcome the plate that actually gets built.
		text, title, footer := "old", "old", "old"
		plan := &ftPlanSH
		useQR := true
		var size float32
		got := ftProofLoader(P, &text, &title, &footer, &plan, &useQR, &size)(p, 0)
		if useQR {
			t.Errorf("%s: the loader left the QR on", trigger)
		}
		if got != p.Text || text != p.Text {
			t.Errorf("%s: the loader stored a different pattern", trigger)
		}
		if plan != p.Plan {
			t.Errorf("%s: the face plan was not written", trigger)
		}
		if footer != "" {
			t.Errorf("%s: the loader wrote the footer %q; a footer refuses this plate outright",
				trigger, footer)
		}
		// The rung written is 0. Both of ftFitAt's routing rules depend on it:
		// a non-zero size beside sized blocks is an error.
		if size != 0 {
			t.Errorf("%s: the loader wrote rung %.1f", trigger, size)
		}
	}
}

// TestSizeProofDropsTheQRTheOperatorChose drives the REAL flow with the QR
// chosen. The ladder cannot carry a code, so the loader has to write over a
// choice the operator made one step earlier -- the ONE thing ftProofLoader is
// otherwise forbidden to touch.
//
// The flag itself is not observable here any more, and that is spec 3.0's doing.
// This test used to read it off the QR screen's selection on the way Back --
// choice == 0 was the write having happened -- but with a ladder in the field
// that screen now offers ONE answer whatever the flag says, so the reading would
// pass over a flag that was never cleared. The flag is pinned where it is still
// falsifiable: at the loader, by TestSizeProofNeedsTheWholePlate, and at the step
// itself by TestSizeProofQRStepReturnsFalseOverAStaleOptIn.
//
// What is asserted here instead is the SCREEN, which is a stronger statement than
// the old one: the operator is told the code is unavailable rather than being
// handed back a choice that would be discarded.
func TestSizeProofDropsTheQRTheOperatorChose(t *testing.T) {
	h, r := startFT(t)
	ftPastQR(h, true)
	ftTypeTrigger(h, ftProofTriggerSizeBack)
	ftOK(h)
	if !uiContains(h.content, "REMOVES THE QR") {
		t.Fatalf("the prompt does not say the QR is removed; frame %q", h.content)
	}
	h.tapWidget("proofYes")
	h.mustReach("lines")
	// Back to the QR screen, which now knows what is loaded.
	ftBackToQR(h)
	cs, ok := h.widget("qr").(*ChoiceScreen)
	if !ok {
		t.Fatal(`widget "qr" is not a *ChoiceScreen`)
	}
	if len(cs.Choices) != 1 || cs.Choices[0] != "No QR" {
		t.Errorf("the QR step offers %q over a loaded ladder", cs.Choices)
	}
	// Forward again and all the way through: no code, and the confirm screen
	// does not claim one.
	ftChoose(h, "qr", 0)
	ftPastFaceAndSize(h)
	h.mustReach("lines")
	for _, step := range []string{"Title", "Footer", "Confirm"} {
		ftOK(h)
		h.mustReach(step)
	}
	if uiContains(h.content, "QR: yes") {
		t.Errorf("the confirm screen claims a QR the plate cannot carry; frame %q", h.content)
	}
	ftOK(h)
	h.step()
	if !r.gotPlate {
		t.Fatal("the flow never built a plate")
	}
	if r.got.QR != nil {
		t.Error("the ladder plate carries a QR")
	}
	if !slices.Equal(r.got.Sizes, ftWantBackSizes) {
		t.Errorf("cut at %s, want %s", ftSizes(r.got.Sizes), ftSizes(ftWantBackSizes))
	}
}

// ---- spec 3.0: the QR step must not offer a choice it will not honour --------

// TestSizeProofQRStepStatesTheQRIsUnavailable is spec 3.0's level 2, on the path
// spec 3.2 names as reachable on shipped firmware: the ladder's loader clears the
// QR under a prompt that says so, the operator presses Back, and the QR screen --
// which knew nothing about what was loaded -- offered "Add QR" again and had the
// answer discarded three functions downstream.
//
// Discarding it silently is the substitution this program exists to avoid, so the
// screen states the code is unavailable for this pattern instead of offering it.
// It also says WHY: an option that is simply missing teaches the operator nothing,
// and they will type against this plate again.
func TestSizeProofQRStepStatesTheQRIsUnavailable(t *testing.T) {
	for _, trigger := range []string{ftProofTriggerSizeFront, ftProofTriggerSizeBack} {
		t.Run(trigger, func(t *testing.T) {
			h, r := startFT(t)
			// Chosen deliberately at step 1, so the screen this test reaches is
			// the one that used to re-seed itself with "Add QR".
			ftPastQR(h, true)
			ftTypeTrigger(h, trigger)
			ftOK(h)
			h.tapWidget("proofYes")
			h.mustReach("lines")
			ftBackToQR(h)
			cs, ok := h.widget("qr").(*ChoiceScreen)
			if !ok {
				t.Fatal(`widget "qr" is not a *ChoiceScreen`)
			}
			if len(cs.Choices) != 1 || cs.Choices[0] != "No QR" {
				t.Errorf("the QR step offers %q with a ladder loaded, want only [\"No QR\"]", cs.Choices)
			}
			// What the OPERATOR sees, which is not the same claim as the widget's
			// field: the offer is off the frame and the reason is on it.
			if uiContains(h.content, "Add QR") {
				t.Errorf("the screen still offers \"Add QR\"; frame %q", h.content)
			}
			if !uiContains(h.content, "needs the whole plate") {
				t.Errorf("the screen does not say why the code is unavailable; frame %q", h.content)
			}
			if !uiContains(h.content, "carries no QR") {
				t.Errorf("the screen does not state the plate carries no code; frame %q", h.content)
			}
			// The privacy sentence belongs to a plate that HAS a code. Left on
			// this screen it would describe a photograph of a code that does not
			// exist.
			if uiContains(h.content, "Anyone who photographs") {
				t.Errorf("the screen still warns about photographing a code the plate "+
					"cannot carry; frame %q", h.content)
			}
			// The one answer is reachable by a finger -- h.point fails if it is
			// undrawn, off-panel or covered, which is what a lead grown past its
			// band would do to it.
			ftChoose(h, "qr", 0)
			ftPastFaceAndSize(h)
			h.mustReach("lines")
			for _, step := range []string{"Title", "Footer", "Confirm"} {
				ftOK(h)
				h.mustReach(step)
			}
			pages := strings.Join(ftConfirmPages(h), "\n")
			if !uiContains(pages, "QR: no") {
				t.Errorf("the confirm screen does not read \"QR: no\"; pages %q", pages)
			}
			ftOK(h)
			h.step()
			if !r.gotPlate {
				t.Fatal("the flow never built a plate")
			}
			if r.got.QR != nil {
				t.Error("the ladder plate carries a QR")
			}
		})
	}
}

// TestSizeProofQRStepBackStillLeavesTheProgram: the new state is a SCREEN, not a
// dead end. Back out of step 1 leaves the free-text program, and it has to go on
// doing that when the step is the unavailable state -- an operator who reached it
// by mistake would otherwise be trapped one Back from the top.
func TestSizeProofQRStepBackStillLeavesTheProgram(t *testing.T) {
	h, r := startFT(t)
	ftPastQR(h, false)
	ftTypeTrigger(h, ftProofTriggerSizeFront)
	ftOK(h)
	h.tapWidget("proofYes")
	h.mustReach("lines")
	ftBackToQR(h)
	cs, ok := h.widget("qr").(*ChoiceScreen)
	if !ok {
		t.Fatal(`widget "qr" is not a *ChoiceScreen`)
	}
	if len(cs.Choices) != 1 {
		t.Fatalf("this test needs the unavailable state; the screen offers %q", cs.Choices)
	}
	// ChoiceScreen owns its Back button privately, so it is tapped by nav slot.
	h.tapNav(Button1)
	if h.step() {
		t.Fatalf("Back out of the unavailable QR step drew another frame; frame %q", h.content)
	}
	if !r.done {
		t.Error("Back out of the unavailable QR step did not leave the program")
	}
}

// TestSizeProofQRStepReturnsFalseOverAStaleOptIn is the FLAG half of spec 3.0:
// useQR comes back false from the unavailable state, whatever was handed in.
//
// Driven at the step rather than through the flow, because the flow can no longer
// produce the state: the ladder's loader clears the flag and this screen will not
// hand it back, so a live opt-in beside a sized composition exists only when the
// step is called with one. That is exactly the input that tells a screen
// returning false apart from one returning what it was given, and it is the input
// a future caller could still supply.
//
// Both directions, on the same call: with the SAME prior, an unsized composition
// still opens on the opt-in. Without that half, "prior is ignored" is satisfied
// by a screen that ignores it everywhere.
func TestSizeProofQRStepReturnsFalseOverAStaleOptIn(t *testing.T) {
	p := ftSizeProofFor(t, ftProofTriggerSizeFront)
	ladder := p.Plan.Blocks(p.Text)
	if !ftSizedBlocks(ladder) {
		t.Fatal("this test needs a sized composition")
	}
	t.Run("sized", func(t *testing.T) {
		h := newPPHarness(t)
		var got, ok bool
		h.start(func() { got, ok = ftQRChoiceFlow(h.ctx, &descriptorTheme, true, ladder) })
		h.mustReach("QRCode")
		cs, isCS := h.widget("qr").(*ChoiceScreen)
		if !isCS {
			t.Fatal(`widget "qr" is not a *ChoiceScreen`)
		}
		if len(cs.Choices) != 1 {
			t.Fatalf("the step offers %q for a sized composition", cs.Choices)
		}
		if cs.choice != 0 {
			t.Errorf("the unavailable state opens on index %d with a prior opt-in behind it", cs.choice)
		}
		ftChoose(h, "qr", 0)
		for i := 0; i < 8 && !ok; i++ {
			h.frame()
		}
		if !ok {
			t.Fatal("the QR step never returned")
		}
		if got {
			t.Error("the QR step handed back an opt-in the plate cannot honour")
		}
	})
	t.Run("unsized", func(t *testing.T) {
		h := newPPHarness(t)
		var got, ok bool
		plain := ftPlanSH.Blocks("a short note")
		h.start(func() { got, ok = ftQRChoiceFlow(h.ctx, &descriptorTheme, true, plain) })
		h.mustReach("QRCode")
		cs := h.widget("qr").(*ChoiceScreen)
		if len(cs.Choices) != 2 {
			t.Fatalf("the step offers %q for an ordinary composition", cs.Choices)
		}
		if cs.choice != 1 {
			t.Fatalf("the same prior opt-in opens on index %d for an ordinary plate; "+
				"the sized half above proves nothing", cs.choice)
		}
		ftChoose(h, "qr", 1)
		for i := 0; i < 8 && !ok; i++ {
			h.frame()
		}
		if !ok {
			t.Fatal("the QR step never returned")
		}
		if !got {
			t.Error("an ordinary plate lost the opt-in the operator re-made")
		}
	})
}

// TestQRStepStillOffersBothAnswersOffTheLadder is the negative pin, and spec
// 3.0's scope in one test: ONLY a sized composition loses the choice.
//
// Ordinary free text keeps both answers AND the prior opt-in across Back --
// deliberate behaviour, not an accident of the old screen. BOTHPROOF! keeps them
// too: a code is possible there in principle and merely does not fit, which the
// prompted drop and the capacity refusal already handle out loud, so narrowing
// that screen would remove a choice the plate can actually honour.
func TestQRStepStillOffersBothAnswersOffTheLadder(t *testing.T) {
	t.Run("ordinary free text", func(t *testing.T) {
		h, _ := startFT(t)
		ftPastQR(h, true)
		h.typeString("hello")
		ftBackToQR(h)
		cs, ok := h.widget("qr").(*ChoiceScreen)
		if !ok {
			t.Fatal(`widget "qr" is not a *ChoiceScreen`)
		}
		if !slices.Equal(cs.Choices, []string{"No QR", "Add QR"}) {
			t.Fatalf("an ordinary plate offers %q", cs.Choices)
		}
		if cs.choice != 1 {
			t.Errorf("the deliberate opt-in was not preserved across Back: choice %d", cs.choice)
		}
		if !uiContains(h.content, "Add QR") {
			t.Errorf("the frame does not offer \"Add QR\"; frame %q", h.content)
		}
		if !uiContains(h.content, "Anyone who photographs") {
			t.Errorf("the frame does not carry the ordinary lead; frame %q", h.content)
		}
		if uiContains(h.content, "needs the whole plate") {
			t.Errorf("an ordinary plate is told the QR is unavailable; frame %q", h.content)
		}
	})
	t.Run(ftProofTriggerBoth, func(t *testing.T) {
		h, _ := startFT(t)
		ftPastQR(h, false)
		ftTypeTrigger(h, ftProofTriggerBoth)
		ftOK(h)
		h.tapWidget("proofYes")
		h.mustReach("lines")
		if ftSizedBlocks(ftPlanBoth.Blocks(ftKbd(h).Fragment)) {
			t.Fatal("BOTHPROOF! loaded a SIZED composition; this test's premise has moved")
		}
		ftBackToQR(h)
		cs, ok := h.widget("qr").(*ChoiceScreen)
		if !ok {
			t.Fatal(`widget "qr" is not a *ChoiceScreen`)
		}
		if !slices.Equal(cs.Choices, []string{"No QR", "Add QR"}) {
			t.Fatalf("the mixed-face plate offers %q; a code is possible there in principle", cs.Choices)
		}
		if uiContains(h.content, "needs the whole plate") {
			t.Errorf("the mixed-face plate is told the QR is structurally unavailable; frame %q", h.content)
		}
		// And the answer is HONOURED as far as it can be: turning it on and
		// pressing OK reaches the capacity refusal that offers to remove it,
		// which is the remedy the ladder must never be offered.
		ftChoose(h, "qr", 1)
		ftPastFaceAndSize(h)
		h.mustReach("lines")
		ftOK(h)
		if !uiContains(h.content, "Remove the QR") {
			t.Errorf("BOTHPROOF! with a QR neither fits nor offers the drop; frame %q", h.content)
		}
	})
}

// ---- a stale QR choice must not narrow the admission band -------------------

// TestSizeProofAdmissionIgnoresAStaleQRChoice pins the agreement between
// admission and the router: ftFitAt sends a sized composition to FitSized,
// which has no QR parameter at all (spec 2.7) and ignores useQR, so
// AdmissibleBlocks must not reserve a QR band for it either.
//
// Asserted as an EQUALITY between the two flag values rather than against the
// measured figures, because the figures are what spec 6 pins elsewhere
// (TestAdmissibleBlocksVerdictDoesNotMove) and because the front sits four
// lines under the cliff: a test written only against the back's numbers passes
// on the front while the defect is still there. The property is that the flag
// makes NO difference to a sized composition, which holds on both sides.
func TestSizeProofAdmissionIgnoresAStaleQRChoice(t *testing.T) {
	P := proofParams()
	for _, trigger := range []string{ftProofTriggerSizeFront, ftProofTriggerSizeBack} {
		t.Run(trigger, func(t *testing.T) {
			p := ftSizeProofFor(t, trigger)
			if !ftSizedBlocks(p.Plan.Blocks(p.Text)) {
				t.Fatal("this test needs a sized composition")
			}
			off := ftEvaluate(P, p.Plan, p.Text, p.Title, p.Footer, false, 0)
			on := ftEvaluate(P, p.Plan, p.Text, p.Title, p.Footer, true, 0)
			if off.err != nil || on.err != nil {
				t.Fatalf("the ladder does not fit: qr off %v, qr on %v", off.err, on.err)
			}
			if !off.ok {
				t.Fatalf("the ladder is inadmissible with the QR off (%d/%d lines)",
					off.linesUsed, off.linesAvail)
			}
			if !on.ok {
				t.Errorf("a stale QR choice makes the ladder inadmissible: %d/%d lines with the "+
					"QR on against %d/%d with it off, while FitSized lays out %d rows either way",
					on.linesUsed, on.linesAvail, off.linesUsed, off.linesAvail, len(on.plate.Lines))
			}
			if on.linesUsed != off.linesUsed || on.linesAvail != off.linesAvail {
				t.Errorf("the QR flag moved the readout on a plate that cannot carry a code: "+
					"%d/%d lines with it on, %d/%d with it off",
					on.linesUsed, on.linesAvail, off.linesUsed, off.linesAvail)
			}
		})
	}
}

// TestSizeProofAdvancesWithTheQRReEnabled is the same defect through the real
// flow, on the path spec 3.2 names as reachable on shipped firmware: the loader
// clears the QR the operator chose, the operator goes Back to the QR screen and
// turns it on again, and nothing on that screen knows a ladder is loaded.
//
// The Text step must still ADVANCE. Before the fix the back was refused here
// with "Removing the QR frees about 476 characters" -- a remedy naming a code
// this plate structurally cannot carry.
//
// Spec 3.0 removed the RE-ENABLE from the flow: that screen no longer offers the
// code over a loaded ladder, so index 1 is not there to tap. The name still holds
// because the flag half is asserted directly below the walk -- the flow drives
// the answer the screen does offer, and ftEvaluate is then asked the same
// question with useQR genuinely true, which is the state the flow can no longer
// reach and the one that used to refuse the plate.
func TestSizeProofAdvancesWithTheQRReEnabled(t *testing.T) {
	for _, trigger := range []string{ftProofTriggerSizeFront, ftProofTriggerSizeBack} {
		t.Run(trigger, func(t *testing.T) {
			h, _ := startFT(t)
			ftPastQR(h, false)
			ftTypeTrigger(h, trigger)
			ftOK(h)
			h.tapWidget("proofYes")
			h.mustReach("lines")
			ftBackToQR(h)
			ftChoose(h, "qr", 0) // the one answer the ladder's QR step offers
			ftPastFaceAndSize(h)
			h.mustReach("lines")
			ftOK(h)
			if uiContains(h.content, "TooLong") || uiContains(h.content, "Remove the QR") {
				t.Fatalf("the Text step refuses a ladder that fits, over a code it cannot carry; frame %q",
					h.content)
			}
			h.mustReach("Title")

			// The flag half, where the flag can still be true.
			p := ftSizeProofFor(t, trigger)
			f := ftEvaluate(h.ctx.Platform.EngraverParams(), p.Plan, p.Text, p.Title, p.Footer, true, 0)
			if f.err != nil {
				t.Errorf("the ladder does not fit with the QR flag set: %v", f.err)
			}
			if !f.ok {
				t.Errorf("the ladder is inadmissible with the QR flag set (%d/%d lines), over a "+
					"code it cannot carry", f.linesUsed, f.linesAvail)
			}
		})
	}
}

// TestSizeProofRefusalDoesNotOfferAQRTheLadderCannotCarry is the other half of
// the same rule, at the refusal rather than the admission: an EDITED ladder that
// keeps its part count -- so its rungs survive -- but overflows them is refused
// by FitSized, and the refusal must not offer to remove a code the plate never
// had. Removing it frees nothing.
func TestSizeProofRefusalDoesNotOfferAQRTheLadderCannotCarry(t *testing.T) {
	P := proofParams()
	p := ftSizeProofFor(t, ftProofTriggerSizeFront)
	parts := strings.Split(p.Text, "\n")
	parts[0] += strings.Repeat("X", 24)
	grown := strings.Join(parts, "\n")
	// The premise: still a ladder, and refused by its own rungs -- with the QR
	// flag set, which is the state the flow can no longer reach and the one this
	// test exists for. Without both this would be measuring some other refusal.
	//
	// Admission now AGREES with that refusal, which it did not before: counted at
	// the 3.0mm anchor this composition was "ok" with room to spare while the fit
	// refused it, so the refusal quoted figures that contradicted it. Counted at
	// its own rungs it is inadmissible, and the numbers on the screen are the
	// numbers of the plate that was refused.
	f := ftEvaluate(P, p.Plan, grown, p.Title, p.Footer, true, 0)
	if !ftSizedBlocks(p.Plan.Blocks(grown)) {
		t.Fatal("the edit cleared the ladder's sizes; the premise has moved")
	}
	if !errors.Is(f.err, backup.ErrTooLarge) {
		t.Fatalf("the edited ladder fits its rungs (err %v); the premise has moved", f.err)
	}
	if f.ok {
		t.Errorf("the fit refuses the edited ladder and admission calls it ok (%d/%d lines): "+
			"the refusal's figures contradict the refusal", f.linesUsed, f.linesAvail)
	}
	if f.linesUsed <= f.linesAvail {
		t.Errorf("the refusal reports %d lines needed of %d available, which is not a refusal",
			f.linesUsed, f.linesAvail)
	}

	h, _ := startFT(t)
	ftPastQR(h, false)
	ftTypeTrigger(h, ftProofTriggerSizeFront)
	ftOK(h)
	h.tapWidget("proofYes")
	h.mustReach("lines")
	ftBackToQR(h)
	// Index 0 is the only answer this screen offers over a loaded ladder (spec
	// 3.0). The stale-flag half of this test is the ftEvaluate above, which is
	// called with useQR true.
	ftChoose(h, "qr", 0)
	ftPastFaceAndSize(h)
	h.mustReach("lines")
	ftSetText(h, grown)
	ftOK(h)
	if uiContains(h.content, "Remove the QR") || uiContains(h.content, "machine-readable") {
		t.Errorf("the refusal offers to remove a QR the ladder cannot carry; frame %q", h.content)
	}
	if !uiContains(h.content, "Shorten the Text field") {
		t.Errorf("the refusal does not name the one remedy there is; frame %q", h.content)
	}
	// The FIGURES on the screen are the refused plate's own. MEASURED: 17 rows
	// needed of the 16 this pattern's rungs hold. At the 3.0mm anchor the same
	// screen read 13 of 24 -- a refusal quoting a plate with eleven rows to spare.
	if f.linesUsed != 17 || f.linesAvail != 16 {
		t.Fatalf("the refused plate is %d/%d lines, want the measured 17/16", f.linesUsed, f.linesAvail)
	}
	if !uiContains(h.content, "needs 17 lines") || !uiContains(h.content, "hold 16") {
		t.Errorf("the refusal does not carry the refused plate's figures (17/16); frame %q", h.content)
	}
	if uiContains(h.content, "24") {
		t.Errorf("the refusal quotes the 3.0mm anchor's capacity; frame %q", h.content)
	}
	// And the SENTENCE those figures sit in. "A plate holds N, at the smallest
	// size" is true of a plate cut at one rung and false of a ladder, whose
	// capacity is a property of the pattern.
	if uiContains(h.content, "at the smallest size") {
		t.Errorf("the refusal describes a plate cut at one rung; frame %q", h.content)
	}
}

// ---- spec 7.17: the confirm screen ------------------------------------------

// TestSizeProofConfirmNamesTheRungs is spec 7.17(a): the one screen the
// operator approves must say what the plate is. ftSizeLabel and
// ftConfirmSummary each carry their own "%.1fmm" off SizeMM, which is 0 on a
// ladder -- so the defect is the literal string "0.0mm" on the confirm screen,
// which a fits-the-panel assertion passes happily.
func TestSizeProofConfirmNamesTheRungs(t *testing.T) {
	for _, tc := range []struct {
		trigger string
		rungs   []float32
	}{
		{ftProofTriggerSizeFront, []float32{5.0, 3.8}},
		{ftProofTriggerSizeBack, []float32{4.4, 3.4, 3.0}},
	} {
		t.Run(tc.trigger, func(t *testing.T) {
			h, _ := startFT(t)
			ftPastQR(h, false)
			ftTypeTrigger(h, tc.trigger)
			ftOK(h)
			h.tapWidget("proofYes")
			h.mustReach("lines")
			// The READOUT, on the text step, is the first thing that would say
			// 0.0mm.
			if uiContains(h.content, "0.0mm") {
				t.Errorf("the readout says 0.0mm; frame %q", h.content)
			}
			for _, step := range []string{"Title", "Footer", "Confirm"} {
				ftOK(h)
				h.mustReach(step)
			}
			pages := strings.Join(ftConfirmPages(h), "\n")
			if uiContains(pages, "0.0mm") {
				t.Errorf("the confirm screen says 0.0mm; pages %q", pages)
			}
			// The rung LIST, in plate order and joined the way the plate's own
			// title joins it -- not each rung with its own "mm", which is what
			// a plate cut at one size says.
			var rungs []string
			for _, r := range tc.rungs {
				rungs = append(rungs, fmt.Sprintf("%.1f", r))
			}
			list := strings.Join(rungs, "+") + "mm"
			if !uiContains(pages, list) {
				t.Errorf("the confirm screen does not name the rungs %q; pages %q", list, pages)
			}
			for _, r := range tc.rungs {
				if !uiContains(pages, fmt.Sprintf("%.1f", r)) {
					t.Errorf("the confirm screen does not name the %.1fmm rung; pages %q", r, pages)
				}
			}
			// A rung that is NOT on this plate must not be named as one, or
			// "names the rungs" is satisfied by printing all six.
			for _, r := range backup.FontSizes {
				if slices.Contains(tc.rungs, r) {
					continue
				}
				if uiContains(pages, fmt.Sprintf("%.1fmm", r)) {
					t.Errorf("the confirm screen names %.1fmm, which is not on this plate; pages %q",
						r, pages)
				}
			}
		})
	}
}

// ftRenderConfirmPages renders every page of the confirm screen for a fit built
// OUTSIDE the flow, and returns the text those pages drew.
//
// It exists because spec 3.0 closed the last flow path that reaches this screen
// with useQR set: the QR step will not hand a ladder the flag back. So the state
// that tells a summary READING THE FIT apart from one reading the flow's flag can
// only be built directly -- ftEvaluate with the flag on, over a composition whose
// plate carries no code.
func ftRenderConfirmPages(t *testing.T, ctx *Context, f ftFit, plan *ftPlan, title, footer string) string {
	t.Helper()
	dims := ctx.Platform.DisplaySize()
	area := ppConfirmArea(dims)
	var pages []string
	for start, guard := 0, 0; ; guard++ {
		v := ftConfirmBody(ctx, &descriptorTheme, area.Dx(), area.Dy(), start, f, plan, title, footer)
		d := new(op.Drawer)
		pages = append(pages, d.ExtractText(image.Rectangle{Max: dims}, v.Content))
		if v.Shown < 1 {
			t.Fatalf("the page from row %d drew no rows", start)
		}
		start += v.Shown
		if start >= v.Total {
			break
		}
		if guard > 200 {
			t.Fatal("the confirm pager never finished")
		}
	}
	return strings.Join(pages, "\n")
}

// TestSizeProofConfirmReportsTheQRFromTheFit is spec 7.17(a2), written as the
// item words it: load a ladder with the QR set and the confirm screen must still
// read "QR: no" and must not carry the privacy warning.
//
// USEQR TRUE is the whole item. Every other ladder test leaves the flag false, so
// the state that distinguishes the shipped ftConfirmSummary (which reads
// f.plate.QR) from a regression reading the flow's flag is never rendered.
//
// The flow used to reach it by re-enabling the code at the QR step; spec 3.0
// removed that, so the flag-true half is now built and DRAWN directly, and the
// flow half walks the answer the screen does offer. Both render the same
// ftConfirmBody.
//
// The warning is asserted BOTH ways, on the same panel: absent for the ladder,
// present for an ordinary plate that really does carry a code. Absence alone is
// satisfied by a warning that never renders at all.
func TestSizeProofConfirmReportsTheQRFromTheFit(t *testing.T) {
	for _, trigger := range []string{ftProofTriggerSizeFront, ftProofTriggerSizeBack} {
		t.Run(trigger, func(t *testing.T) {
			h, r := startFT(t)
			// The flag half: useQR TRUE, and the plate carries no code.
			p := ftSizeProofFor(t, trigger)
			P := h.ctx.Platform.EngraverParams()
			stale := ftEvaluate(P, p.Plan, p.Text, p.Title, p.Footer, true, 0)
			if stale.err != nil {
				t.Fatalf("the ladder does not fit with the QR flag set: %v", stale.err)
			}
			if stale.plate.QR != nil {
				t.Fatal("FitSized produced a code; this test's premise has moved")
			}
			drawn := ftRenderConfirmPages(t, h.ctx, stale, p.Plan, p.Title, p.Footer)
			if !uiContains(drawn, "QR: no") {
				t.Errorf("with useQR set the confirm screen does not read \"QR: no\"; pages %q", drawn)
			}
			if uiContains(drawn, "QR: yes") {
				t.Errorf("with useQR set the confirm screen claims a QR the plate cannot carry; pages %q",
					drawn)
			}
			if uiContains(drawn, ftWarnQR) {
				t.Errorf("with useQR set the confirm screen carries the QR privacy warning for a plate "+
					"with no code; pages %q", drawn)
			}

			// And the same screen through the flow.
			ftPastQR(h, true)
			ftTypeTrigger(h, trigger)
			ftOK(h)
			h.tapWidget("proofYes")
			h.mustReach("lines")
			ftBackToQR(h)
			ftChoose(h, "qr", 0)
			ftPastFaceAndSize(h)
			h.mustReach("lines")
			for _, step := range []string{"Title", "Footer", "Confirm"} {
				ftOK(h)
				h.mustReach(step)
			}
			pages := strings.Join(ftConfirmPages(h), "\n")
			if !uiContains(pages, "QR: no") {
				t.Errorf("the confirm screen does not read \"QR: no\"; pages %q", pages)
			}
			if uiContains(pages, "QR: yes") {
				t.Errorf("the confirm screen claims a QR the plate cannot carry; pages %q", pages)
			}
			if uiContains(pages, ftWarnQR) {
				t.Errorf("the confirm screen carries the QR privacy warning for a plate with no code; pages %q",
					pages)
			}
			// The plate itself, so "QR: no" is the truth about the steel and not
			// a second reading of the same flag.
			ftOK(h)
			h.step()
			if !r.gotPlate {
				t.Fatal("the flow never built a plate")
			}
			if r.got.QR != nil {
				t.Error("the ladder plate carries a QR")
			}
		})
	}
	// Non-vacuity: the same assertions on a plate that DOES carry a code.
	t.Run("a plate with a QR still warns", func(t *testing.T) {
		h, _ := startFT(t)
		ftPastQR(h, true)
		h.typeString("hello")
		for _, step := range []string{"Title", "Footer", "Confirm"} {
			ftOK(h)
			h.mustReach(step)
		}
		pages := strings.Join(ftConfirmPages(h), "\n")
		if !uiContains(pages, "QR: yes") {
			t.Errorf("the confirm screen does not read \"QR: yes\" for a plate that carries one; pages %q",
				pages)
		}
		if !uiContains(pages, ftWarnQR) {
			t.Errorf("the confirm screen omits the QR privacy warning for a plate that carries a code, "+
				"so the ladder's absence assertion proves nothing; pages %q", pages)
		}
	})
}

// TestSizeProofConfirmFitsThePanel is spec 7.17(b), measured as RECTANGLES on
// the real panel. It cannot be done from ExtractText: that collects the runes
// of every drawn text op regardless of occlusion, so a summary drawn off the
// bottom edge reads as fully present -- and the ladder's summary is the longest
// this screen has ever had to draw, six face-and-size runs on the back.
func TestSizeProofConfirmFitsThePanel(t *testing.T) {
	pf := newPlatform()
	pf.display = sh2DisplaySize
	ctx := NewContext(pf)
	dims := ctx.Platform.DisplaySize()
	if dims != sh2DisplaySize {
		t.Fatalf("running at %v, not the real %v panel", dims, sh2DisplaySize)
	}
	area := ppConfirmArea(dims)
	th := &descriptorTheme
	P := ctx.Platform.EngraverParams()
	for _, trigger := range []string{ftProofTriggerSizeFront, ftProofTriggerSizeBack} {
		p := ftSizeProofFor(t, trigger)
		f := ftEvaluate(P, p.Plan, p.Text, p.Title, p.Footer, false, 0)
		if f.err != nil {
			t.Fatalf("%s: %v", trigger, f.err)
		}
		if !f.ok {
			t.Fatalf("%s: the ladder is inadmissible (%d/%d lines), so the flow would refuse OK "+
				"on the text step and never reach the confirm screen", trigger, f.linesUsed, f.linesAvail)
		}
		start, guard := 0, 0
		for {
			v := ftConfirmBody(ctx, th, area.Dx(), area.Dy(), start, f, p.Plan, p.Title, p.Footer)
			if v.Size.Y > area.Dy() {
				t.Fatalf("%s: the page from row %d needs %dpx of a %dpx area: the rung line and "+
					"the warnings are off the %v panel", trigger, start, v.Size.Y, area.Dy(), dims)
			}
			if v.Size.X > area.Dx() {
				t.Fatalf("%s: the page from row %d is %dpx wide in a %dpx area", trigger, start, v.Size.X, area.Dx())
			}
			if v.Shown < 1 {
				t.Fatalf("%s: the page from row %d drew no rows: the pager cannot advance", trigger, start)
			}
			start += v.Shown
			guard++
			if start >= v.Total {
				break
			}
			if guard > 200 {
				t.Fatalf("%s: the pager never finished", trigger)
			}
		}
	}
}

// ---- the readouts -----------------------------------------------------------

// TestSizeProofReadoutCountsTheRowsItCuts is the follow-up
// `sizeproof-admission-count-at-its-own-rungs` at the screen the operator reads
// while typing: the line count beside the rungs must be the rows the plate is
// CUT with, not the rows a uniform 3.0mm plate would need for the same
// characters.
//
// Both numbers are asserted. The literal row count is what a font change must
// fail against -- it is spec 1.1's measurement, 16 rows on the front and 20 on
// the back. The equality with len(f.plate.Lines) is what a readout describing
// some OTHER plate must fail against, and it is not self-referential: one side
// comes from AdmissibleSized and the other from FitSized, two layouts that have
// to arrive at the same plate.
//
// The 3.0mm anchor's answer is asserted to DIFFER, so this cannot pass by the
// sibling collapsing back onto it.
func TestSizeProofReadoutCountsTheRowsItCuts(t *testing.T) {
	P := proofParams()
	for _, tc := range []struct {
		trigger string
		rows    int
		avail   int
	}{
		{ftProofTriggerSizeFront, 16, 16},
		{ftProofTriggerSizeBack, 20, 20},
	} {
		t.Run(tc.trigger, func(t *testing.T) {
			p := ftSizeProofFor(t, tc.trigger)
			f := ftEvaluate(P, p.Plan, p.Text, p.Title, p.Footer, false, 0)
			if f.err != nil {
				t.Fatalf("the ladder does not fit: %v", f.err)
			}
			if got := len(f.plate.Lines); got != tc.rows {
				t.Fatalf("FitSized lays out %d rows, want spec 1.1's %d", got, tc.rows)
			}
			if f.linesUsed != len(f.plate.Lines) {
				t.Errorf("the readout counts %d rows and the plate is cut with %d",
					f.linesUsed, len(f.plate.Lines))
			}
			if f.linesUsed != tc.rows || f.linesAvail != tc.avail {
				t.Errorf("the readout is %d/%d lines, want %d/%d",
					f.linesUsed, f.linesAvail, tc.rows, tc.avail)
			}
			if !f.ok {
				t.Errorf("the ladder is inadmissible at its own rungs")
			}
			anchor, anchorAvail, anchorOK := backup.AdmissibleBlocks(
				P, p.Plan.Blocks(p.Text), p.Title, p.Footer, false)
			if anchor == f.linesUsed {
				t.Fatalf("the 3.0mm anchor also reports %d rows; this side cannot tell the two apart",
					anchor)
			}
			t.Logf("own rungs %d/%d ok=%v, 3.0mm anchor %d/%d ok=%v",
				f.linesUsed, f.linesAvail, f.ok, anchor, anchorAvail, anchorOK)
			// The string the readout actually draws.
			if want := fmt.Sprintf("%d/%d lines", tc.rows, tc.avail); !strings.Contains(ftSizeLabel(f), want) {
				t.Errorf("the readout is %q, want it to carry %q", ftSizeLabel(f), want)
			}
			// And on the panel, through the flow.
			h, _ := startFT(t)
			ftPastQR(h, false)
			ftTypeTrigger(h, tc.trigger)
			ftOK(h)
			h.tapWidget("proofYes")
			h.mustReach("lines")
			if !uiContains(h.content, fmt.Sprintf("%d/%d lines", tc.rows, tc.avail)) {
				t.Errorf("the text step does not read %d/%d lines; frame %q",
					tc.rows, tc.avail, h.content)
			}
		})
	}
}

// TestSizeLabelsNeverPrintZero pins both readouts directly, on a Fitted that is
// Mixed. Each site has its OWN "%.1fmm" and neither can be covered by the
// other's test.
func TestSizeLabelsNeverPrintZero(t *testing.T) {
	P := proofParams()
	p := ftSizeProofFor(t, ftProofTriggerSizeBack)
	f := ftEvaluate(P, p.Plan, p.Text, p.Title, p.Footer, false, 0)
	if f.err != nil || !f.plate.Mixed || f.plate.SizeMM != 0 {
		t.Fatalf("this test needs a Mixed plate with SizeMM 0; got err=%v Mixed=%v SizeMM=%.1f",
			f.err, f.plate.Mixed, f.plate.SizeMM)
	}
	if got := ftSizeLabel(f); strings.Contains(got, "0.0mm") {
		t.Errorf("the readout is %q", got)
	} else if !strings.Contains(got, "4.4") || !strings.Contains(got, "3.0") {
		t.Errorf("the readout %q names neither end of the ladder", got)
	}
	if got := ftPlateRungs(f.plate); got != "4.4+3.4+3.0mm" {
		t.Errorf("the confirm screen's size field is %q, want %q", got, "4.4+3.4+3.0mm")
	}
	// A uniform plate is unchanged, in both readouts.
	u := ftEvaluate(P, &ftPlanSH, "a short note", "", "", false, 0)
	if u.err != nil {
		t.Fatal(u.err)
	}
	plain := fmt.Sprintf("%.1fmm", u.plate.SizeMM)
	if got := ftPlateRungs(u.plate); got != plain {
		t.Errorf("a uniform plate's size field is %q, want %q", got, plain)
	}
	if got := ftSizeLabel(u); !strings.HasPrefix(got, plain) {
		t.Errorf("a uniform plate's readout is %q, want it to open with %q", got, plain)
	}
}

// TestFaceSummaryGroupsByFaceAndSize: the front cuts font/sh at 5.0mm and again
// at 3.8mm, so a summary grouped by FACE alone reports two runs where the plate
// has four and says nothing about which rung each is.
func TestFaceSummaryGroupsByFaceAndSize(t *testing.T) {
	P := proofParams()
	p := ftSizeProofFor(t, ftProofTriggerSizeFront)
	f := ftEvaluate(P, p.Plan, p.Text, p.Title, p.Footer, false, 0)
	if f.err != nil {
		t.Fatal(f.err)
	}
	got := ftFaceSummary(p.Plan, f.plate.Faces, f.plate.Sizes)
	if n := strings.Count(got, " + "); n != 3 {
		t.Errorf("the summary %q has %d separators, want 3 (four runs)", got, n)
	}
	for _, want := range []string{"sh 5.0 4", "constant 5.0 5", "sh 3.8 3", "constant 3.8 4"} {
		if !strings.Contains(got, want) {
			t.Errorf("the summary %q does not carry the run %q", got, want)
		}
	}
	// The GROUPING itself, on a face map where the two are distinguishable.
	// Neither ladder can show it: their bands alternate the two faces, so a run
	// break falls on every size change anyway and grouping by face alone gives
	// the same four runs. Measured -- that mutation left this test green until
	// this case existed. Here one face is cut at two rungs BACK TO BACK, which
	// is what a ladder would look like if its faces did not alternate.
	sh := ftFaceSH.Face
	twoRungs := ftFaceSummary(p.Plan,
		[]*vector.Face{sh, sh, sh, sh}, []float32{5.0, 5.0, 3.8, 3.8})
	if want := "sh 5.0 2 + sh 3.8 2"; twoRungs != want {
		t.Errorf("one face at two rungs summarises as %q, want %q -- the runs are grouped by face "+
			"alone, so the plate reads as one run at a size that is true of half of it",
			twoRungs, want)
	}
	// And the same face map with ONE size is one run, so the split above is the
	// size doing the work rather than the row index.
	if want := "sh 4"; ftFaceSummary(p.Plan, []*vector.Face{sh, sh, sh, sh}, nil) != want {
		t.Errorf("four rows in one face at one size summarise as %q, want %q",
			ftFaceSummary(p.Plan, []*vector.Face{sh, sh, sh, sh}, nil), want)
	}
	byFace := ftFaceSummary(p.Plan, f.plate.Faces, nil)
	if byFace == got {
		t.Errorf("the summary is the same with and without the sizes (%q); it is not printing them", got)
	}
	// A uniform plate reads exactly as it always did: the size is already on
	// the line's head, so repeating it per run would be noise.
	u := ftEvaluate(P, &ftPlanBoth, ftProofTextBoth, ftProofTitleBoth, ftProofFooter, false, 0)
	if u.err != nil {
		t.Fatal(u.err)
	}
	sized := ftFaceSummary(&ftPlanBoth, u.plate.Faces, u.plate.Sizes)
	plain := ftFaceSummary(&ftPlanBoth, u.plate.Faces, nil)
	if sized != plain {
		t.Errorf("a uniform plate's summary changed from %q to %q", plain, sized)
	}
}

// ---- the prompt -------------------------------------------------------------

// TestSizeProofPromptNamesTheSideAndTheRungs is spec 4 and spec 3's
// ftProofReplaces row. Both sides prove IDENTICAL faces, so the plan name gives
// near-identical prompts and a mis-pick is engraved where a mistype is refused.
//
// And with an empty footer the sentence must not read "Footer becomes ." --
// which is what naming the field's new value gives when the new value is
// nothing at all.
func TestSizeProofPromptNamesTheSideAndTheRungs(t *testing.T) {
	P := proofParams()
	front := ftSizeProofFor(t, ftProofTriggerSizeFront)
	back := ftSizeProofFor(t, ftProofTriggerSizeBack)
	if front.Plan.Name() != back.Plan.Name() && !strings.HasPrefix(back.Plan.Name(), front.Plan.Name()) {
		t.Logf("the two plans name themselves %q and %q", front.Plan.Name(), back.Plan.Name())
	}
	seen := map[string]string{}
	for _, p := range []*ftProof{front, back} {
		out := ftProofOutcomeFor(P, p, 0, false)
		if out.Footer != "" {
			t.Errorf("%s: the resolver returns the footer %q; the ladder carries none", p.Trigger, out.Footer)
		}
		if out.Plan != p.Plan {
			t.Errorf("%s: the resolver returned another plan", p.Trigger)
		}
		if out.SizeMM != 0 {
			t.Errorf("%s: the resolver returned rung %.1f", p.Trigger, out.SizeMM)
		}
		s := ftProofReplaces(p, out)
		if strings.Contains(s, "Footer becomes .") || strings.Contains(s, "Footer becomes ,") {
			t.Errorf("%s: the prompt names an empty footer as a value:\n  %s", p.Trigger, s)
		}
		if !strings.Contains(s, "Footer is CLEARED") {
			t.Errorf("%s: the prompt does not say the footer is cleared:\n  %s", p.Trigger, s)
		}
		// The whole PHRASE, not the bare side: the title the same sentence
		// quotes opens with the side, so strings.Contains(s, "FRONT") is true
		// however the plate is named. Measured -- naming out.Plan.Name() here
		// instead of the side left this test green until it read the phrase.
		if want := "cut in " + p.Name() + " at "; !strings.Contains(s, want) {
			t.Errorf("%s: the prompt does not say %q:\n  %s", p.Trigger, want, s)
		}
		if bad := "cut in " + p.Plan.Name(); strings.Contains(s, bad) {
			t.Errorf("%s: the prompt names the face plan %q, which both sides share:\n  %s",
				p.Trigger, p.Plan.Name(), s)
		}
		if !strings.Contains(s, p.Title) {
			t.Errorf("%s: the prompt does not name the title it writes:\n  %s", p.Trigger, s)
		}
		// The rungs, and NOT the "3.0mm" ftRungLabel prints for an unsized
		// proof with no rung named.
		var rungs []string
		for _, r := range p.Plan.Rungs() {
			rungs = append(rungs, fmt.Sprintf("%.1f", r))
		}
		if want := strings.Join(rungs, "+") + "mm"; !strings.Contains(s, want) {
			t.Errorf("%s: the prompt does not state the rungs %q:\n  %s", p.Trigger, want, s)
		}
		if !slices.Contains(p.Plan.Rungs(), 3.0) && strings.Contains(s, "3.0mm") {
			t.Errorf("%s: the prompt states 3.0mm, a rung this plate does not carry:\n  %s", p.Trigger, s)
		}
		ask := ftProofAsk(p)
		if other, dup := seen[ask]; dup {
			t.Errorf("%s and %s ask the same question %q", p.Trigger, other, ask)
		}
		seen[ask] = p.Trigger
		if want := "the " + p.Name() + " test pattern"; !strings.Contains(ask, want) {
			t.Errorf("%s: the question %q does not say %q", p.Trigger, ask, want)
		}
	}
}

// TestNonSizeableProofIgnoresARung: ftProofOutcomeFor's rung branch rebuilds
// the MIXED pattern, whatever proof it was handed. Reached with a rung and a
// ladder -- which cmd/plateview can do, since -size is a flag -- it would
// return the BOTHPROOF! pattern under the ladder's own trigger.
func TestNonSizeableProofIgnoresARung(t *testing.T) {
	P := proofParams()
	for i := range ftProofs {
		p := &ftProofs[i]
		if p.Sizeable {
			continue
		}
		out := ftProofOutcomeFor(P, p, 4.4, false)
		if out.Text != p.For(false) || out.Title != p.Title || out.Plan != p.Plan {
			t.Errorf("%s: a rung handed to a proof that takes none returned another proof's pattern",
				p.Trigger)
		}
		if out.SizeMM != 0 {
			t.Errorf("%s: a rung handed to a proof that takes none was recorded as %.1f",
				p.Trigger, out.SizeMM)
		}
	}
}

// ---- spec 7.18: the preview -------------------------------------------------

// TestSizeProofPreviewIsTheDevicePlate is spec 7.18. fittedPreviewAt used
// backup.FitBlocks directly, which ignores Block.SizeMM, so
// "plateview -plate sizeproof-front" with no -size fitted the four-block
// composition UNIFORMLY and previewed a plate the device will not cut.
func TestSizeProofPreviewIsTheDevicePlate(t *testing.T) {
	P := proofParams()
	for _, tc := range []struct {
		plate   string
		trigger string
	}{
		{"sizeproof-front", ftProofTriggerSizeFront},
		{"sizeproof-back", ftProofTriggerSizeBack},
	} {
		t.Run(tc.plate, func(t *testing.T) {
			pv, err := BuildPreview(P, tc.plate, PreviewOpts{})
			if err != nil {
				t.Fatalf("%s: %v", tc.plate, err)
			}
			p := ftSizeProofFor(t, tc.trigger)
			f := ftEvaluate(P, p.Plan, p.Text, p.Title, p.Footer, false, 0)
			if f.err != nil {
				t.Fatal(f.err)
			}
			if !slices.Equal(pv.Sizes, f.plate.Sizes) {
				t.Errorf("the preview's per-row sizes are %s, the device's %s",
					ftSizes(pv.Sizes), ftSizes(f.plate.Sizes))
			}
			if len(pv.Rows) != len(f.plate.Lines) {
				t.Fatalf("the preview has %d rows, the device %d", len(pv.Rows), len(f.plate.Lines))
			}
			for i := range pv.Rows {
				if pv.Rows[i].Text != f.plate.Lines[i] {
					t.Errorf("row %d differs: preview %q, device %q", i, pv.Rows[i].Text, f.plate.Lines[i])
				}
			}
			// No footer the device would not engrave, and the title the loader
			// writes.
			if pv.Footer != "" {
				t.Errorf("the preview carries the footer %q", pv.Footer)
			}
			if pv.Title != p.Title {
				t.Errorf("the preview's title is %q, want %q", pv.Title, p.Title)
			}
			if pv.HasQR {
				t.Error("the preview carries a QR")
			}
			// And the plate GEOMETRY is the device's, not merely the row list:
			// a preview that reported the ladder and rendered a uniform plate
			// would pass every assertion above.
			want, err := ftBuildPlate(P, p.Plan, p.Text, p.Title, p.Footer, false, 0)
			if err != nil {
				t.Fatal(err)
			}
			if pv.Plate.Duration != want.Duration {
				t.Errorf("the preview runs %d ticks, the device's plate %d",
					pv.Plate.Duration, want.Duration)
			}
			if g, w := ftSpline(t, pv.Plate), ftSpline(t, want); !slices.Equal(g, w) {
				t.Errorf("the preview is not the device's plate (%d knots vs %d)", len(g), len(w))
			}
			// -size must not re-fit a ladder at one rung: it is refused, not
			// silently flattened.
			if _, err := BuildPreview(P, tc.plate, PreviewOpts{SizeMM: 4.4}); err == nil {
				t.Error("-size 4.4 produced a plate; a ladder cannot also be fitted at one rung")
			}
		})
	}
}

// TestPreviewSizesCoverEveryFreeTextPlate: Sizes is parallel to Rows for every
// plate the free-text fitter lays out, or the listing beside a row would be
// another plate's size.
func TestPreviewSizesCoverEveryFreeTextPlate(t *testing.T) {
	P := proofParams()
	for _, name := range PreviewPlates() {
		pv, err := BuildPreview(P, name, PreviewOpts{Text: "a note"})
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if len(pv.Sizes) != len(pv.Rows) {
			t.Errorf("%s: %d sizes against %d rows", name, len(pv.Sizes), len(pv.Rows))
		}
		for i, s := range pv.Sizes {
			if s == 0 {
				t.Errorf("%s: row %d reports no size", name, i)
			}
		}
	}
}
