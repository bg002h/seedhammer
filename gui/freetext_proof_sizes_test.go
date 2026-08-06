package gui

import (
	"fmt"
	"strings"
	"testing"

	"seedhammer.com/backup"
	"seedhammer.com/font/vector"
)

// TestMixedProofFitsEveryRung: the operator picks the size, so every rung on
// the ladder must yield a plate. A rung that refused would be an option on
// screen that cannot be taken.
func TestMixedProofFitsEveryRung(t *testing.T) {
	for _, size := range backup.FontSizes {
		text, plan, title, footer, err := ftBothAt(engraverParams, size, false)
		if err != nil {
			t.Errorf("%.1fmm: %v", size, err)
			continue
		}
		f, err := backup.FitBlocksAt(engraverParams, plan.Blocks(text), title, footer, false, size)
		if err != nil {
			t.Errorf("%.1fmm: the pattern ftBothAt returned does not fit at the rung it "+
				"was built for: %v", size, err)
			continue
		}
		// The chosen rung is the rung engraved. This is the whole reason
		// FitBlocksAt exists: a trimmed pattern also fits at larger rungs, and
		// auto-fit would silently engrave it at one of them.
		if f.SizeMM != size {
			t.Errorf("asked for %.1fmm, got %.1fmm", size, f.SizeMM)
		}
	}
}

// TestMixedProofAlwaysKeepsBothSweeps: the sweep is the irreducible core. A
// rung that dropped one would engrave a plate that looks like a proof and
// silently omits glyphs -- the exact failure a proof exists to prevent.
func TestMixedProofAlwaysKeepsBothSweeps(t *testing.T) {
	for _, size := range backup.FontSizes {
		text, plan, _, _, err := ftBothAt(engraverParams, size, false)
		if err != nil {
			t.Fatalf("%.1fmm: %v", size, err)
		}
		blocks := plan.Blocks(text)
		if len(blocks) != 2 {
			t.Fatalf("%.1fmm: %d blocks, want 2 (one per face)", size, len(blocks))
		}
		for i, half := range []string{"sh", "const"} {
			if !strings.Contains(blocks[i].Text, ftProofSweep) {
				t.Errorf("%.1fmm: the %s half has lost its codepoint sweep; a face is not "+
					"qualified by most of its glyphs", size, half)
			}
		}
	}
}

// TestMixedProofDropsInTheStatedOrder pins the sacrifice order against what a
// proof is FOR: seed words first (they are a sample, and every letter in them
// is in the sweep), then prose, then the confusable table, then the labels.
//
// Without this the ladder could reorder itself under a future edit and still
// pass every other test here -- each rung would fit, and each would carry a
// sweep, while quietly throwing away the confusable table before the prose.
func TestMixedProofDropsInTheStatedOrder(t *testing.T) {
	type has struct{ seed, prose, confusables, labels bool }
	// Each half is inspected SEPARATELY. Testing the concatenation lets one
	// half keep a section while the other loses it -- which is exactly the
	// mutation that slipped through the first version of this test.
	at := func(d ftDrop) has {
		sh, cons, footer := ftBothHalves(engraverParams, 3.0, d)
		return has{
			// Seed words live only in the const half by design.
			seed: strings.Contains(cons, ftProofSeedWords),
			prose: strings.Contains(sh, ftProofLowerPangram) &&
				strings.Contains(cons, ftProofLowerPangram),
			confusables: strings.Contains(sh, ftProofConfusables) &&
				strings.Contains(cons, ftProofConfusables),
			labels: strings.HasPrefix(sh, "SH 3.0mm") &&
				strings.HasPrefix(cons, "CONST 3.0mm") && footer == ftProofFooter,
		}
	}
	want := []has{
		{seed: true, prose: true, confusables: true, labels: true},  // nothing
		{seed: false, prose: true, confusables: true, labels: true}, // -seed
		{seed: false, prose: false, confusables: true, labels: true},
		{seed: false, prose: false, confusables: false, labels: true},
		{seed: false, prose: false, confusables: false, labels: false},
	}
	for d := ftDropNothing; d < ftDropLevels; d++ {
		if got := at(d); got != want[d] {
			t.Errorf("drop level %d carries %+v, want %+v", d, got, want[d])
		}
	}
}

// TestMixedProofLabelsStateTheRungTheyAreCutAt: on permanent steel the label is
// the only record of what was tested, so a label naming a size or a grid the
// plate does not have is worse than no label. The 3.0mm pattern could afford
// constants; with the rung chosen by the operator these must be measured.
func TestMixedProofLabelsStateTheRungTheyAreCutAt(t *testing.T) {
	for _, size := range backup.FontSizes {
		text, plan, title, _, err := ftBothAt(engraverParams, size, false)
		if err != nil {
			t.Fatalf("%.1fmm: %v", size, err)
		}
		// Built HERE from a literal format, not by calling ftProofTitleBothAt:
		// comparing that function against itself passes however wrong it is.
		// The label branch below already did this; the title did not, and the
		// pre-ship review showed ftProofTitleBothAt could be hardcoded back to
		// "SH+CONST 3.0mm" with the whole suite still green.
		if want := fmt.Sprintf("SH+CONST %.1fmm", size); title != want {
			t.Errorf("%.1fmm: title %q, want %q", size, title, want)
		}
		blocks := plan.Blocks(text)
		// Where labels survive, they must state THIS rung and THIS grid.
		//
		// The expected string is built HERE, from the measurement primitives,
		// rather than by calling ftProofLabel -- comparing a function against
		// itself passes however wrong it is, and the first version of this test
		// did exactly that and let a hardcoded "3.0mm" through.
		for i, spec := range []struct{ prefix, want string }{
			{"SH", fmt.Sprintf("SH %.1fmm %dx%d", size,
				backup.CharsPerLine(engraverParams, ftFaceSH.Face, size),
				backup.LinesPerPlate(engraverParams, size))},
			{"CONST", fmt.Sprintf("CONST %.1fmm %dx%d", size,
				backup.CharsPerLine(engraverParams, ftFaceConst.Face, size),
				backup.LinesPerPlate(engraverParams, size))},
		} {
			first := strings.SplitN(blocks[i].Text, "\n", 2)[0]
			if !strings.HasPrefix(first, spec.prefix+" ") {
				continue // labels dropped at this rung; the footer carries the map
			}
			if first != spec.want {
				t.Errorf("%.1fmm: %s half is labelled %q, want %q", size, spec.prefix, first, spec.want)
			}
		}
	}
}

// TestMixedProofNamesTheFacesWhenLabelsGo: the labels are what say which half
// is which, so the rung that drops them must say it somewhere else. Otherwise a
// plate found in a drawer a year later cannot be read at all.
func TestMixedProofNamesTheFacesWhenLabelsGo(t *testing.T) {
	for _, size := range backup.FontSizes {
		text, plan, _, footer, err := ftBothAt(engraverParams, size, false)
		if err != nil {
			t.Fatalf("%.1fmm: %v", size, err)
		}
		blocks := plan.Blocks(text)
		labelled := strings.HasPrefix(blocks[0].Text, "SH ")
		if !labelled && footer != ftProofFooterFaceMap {
			t.Errorf("%.1fmm: the in-body labels are gone but the footer is %q, so nothing "+
				"on the plate says which half is which", size, footer)
		}
		if len(ftProofFooterFaceMap) > backup.MaxTitleLen {
			t.Errorf("the face-map footer is %d characters, over the %d cap",
				len(ftProofFooterFaceMap), backup.MaxTitleLen)
		}
		// And it must actually NAME the faces. Asserting only that the footer
		// equals the constant leaves the constant free to say anything.
		for _, face := range []string{"SH", "CONST"} {
			if !strings.Contains(ftProofFooterFaceMap, face) {
				t.Errorf("the face-map footer %q does not name %q, so the plate that "+
					"drops its labels says nothing about which half is which",
					ftProofFooterFaceMap, face)
			}
		}
	}
}

// TestSizeSuffixOnlyMatchesRealRungs: the rung is named by appending it to the
// trigger, so the parser decides what is a keyword and what is ordinary text.
// A suffix that is not a rung must fall through to text -- an operator who
// types BOTHPROOF!5 and gets a plate at some nearby size was not asked.
func TestSizeSuffixOnlyMatchesRealRungs(t *testing.T) {
	for _, tc := range []struct {
		typed string
		want  float32
		ok    bool
	}{
		{"BOTHPROOF!", 0, true}, // no rung: the pattern's own size
		{"BOTHPROOF!3.0", 3.0, true},
		{"BOTHPROOF!4.4", 4.4, true},
		{"BOTHPROOF!6", 6.0, true},
		{"BOTHPROOF!6.0", 6.0, true},
		// Not rungs. FontSizes is the only set every capacity number in backup
		// is measured against.
		{"BOTHPROOF!4.5", 0, false},
		{"BOTHPROOF!2", 0, false},
		{"BOTHPROOF!7", 0, false},
		{"BOTHPROOF!x", 0, false},
		{"BOTHPROOF!3.0 ", 0, false},
		// The single-face patterns are tuned to land at 3.0mm and have no drop
		// order, so they take no suffix at all.
		{"TEXTPROOF!4.4", 0, false},
		{"CONSTPROOF!6", 0, false},
		// And the base triggers stay exact matches.
		{"TEXTPROOF!", 0, true},
		{"CONSTPROOF!", 0, true},
	} {
		p, size, ok := ftProofForTrigger(tc.typed)
		if ok != tc.ok {
			t.Errorf("%q: matched=%v, want %v", tc.typed, ok, tc.ok)
			continue
		}
		if ok && size != tc.want {
			t.Errorf("%q: rung %.1f, want %.1f", tc.typed, size, tc.want)
		}
		if ok && p == nil {
			t.Errorf("%q: matched with no proof", tc.typed)
		}
	}
}

// TestNamedRungIsTheRungEngraved is the end-to-end version of the property
// FitBlocksAt exists for: what the operator named is what the engraver uses.
//
// The loader and the plate builder are separate functions with separate
// arguments, so this walks BOTH -- a loader that trimmed the pattern while the
// builder auto-fit it would engrave the trimmed content at some larger size,
// and every unit test either side of that seam would still pass.
func TestNamedRungIsTheRungEngraved(t *testing.T) {
	for _, want := range backup.FontSizes {
		var text, title, footer string
		useQR := false
		var size float32
		plan := &ftPlanSH
		load := ftProofLoader(engraverParams, &text, &title, &footer, &plan, &useQR, &size, new(bool))

		p, rung, ok := ftProofForTrigger(fmt.Sprintf("BOTHPROOF!%.1f", want))
		if !ok {
			t.Fatalf("%.1fmm is a rung in FontSizes but the trigger did not match it", want)
		}
		load(p, rung)
		if size != want {
			t.Errorf("%.1fmm: the loader recorded rung %.1f", want, size)
		}
		f := ftEvaluate(engraverParams, plan, text, title, footer, useQR, size)
		if f.err != nil {
			t.Errorf("%.1fmm: evaluate refused the pattern it was just given: %v", want, f.err)
			continue
		}
		if f.plate.SizeMM != want {
			t.Errorf("%.1fmm: the readout would say %.1fmm", want, f.plate.SizeMM)
		}
		if _, err := ftBuildPlate(engraverParams, plan, text, title, footer, useQR, size); err != nil {
			t.Errorf("%.1fmm: build refused: %v", want, err)
		}
	}
}

// TestProofPromptMatchesWhatTheLoaderWrites is the consent property: the
// sentence the operator reads before accepting must name the fields that
// actually get written.
//
// It failed before the pre-ship review fold. The prompt and the loader derived
// the same answer twice and disagreed once a rung was named -- BOTHPROOF!4.4
// asked for consent to "Title becomes SH+CONST 3.0mm ... cut at 4.4mm", a
// sentence contradicting itself and describing a plate the machine would not
// cut. Every prompt test called ftProofReplaces(p, 0), so the rung path was
// uncovered.
//
// This walks the REAL loader, not just the resolver, so a loader that stopped
// using ftProofOutcomeFor is a failure rather than a silent second answer.
func TestProofPromptMatchesWhatTheLoaderWrites(t *testing.T) {
	p, _, ok := ftProofForTrigger(ftProofTriggerBoth)
	if !ok {
		t.Fatal("no mixed proof")
	}
	for _, rung := range append([]float32{0}, backup.FontSizes...) {
		var text, title, footer string
		useQR := false
		var size float32
		plan := &ftPlanSH
		ftProofLoader(engraverParams, &text, &title, &footer, &plan, &useQR, &size, new(bool))(p, rung)

		out := ftProofOutcomeFor(engraverParams, p, rung, false)
		prompt := ftProofReplaces(p, out)

		if title == "" || footer == "" {
			t.Fatalf("rung %.1f: loader wrote an empty title or footer", rung)
		}
		if !strings.Contains(prompt, title) {
			t.Errorf("rung %.1f: the loader writes title %q but the prompt says:\n  %s",
				rung, title, prompt)
		}
		if !strings.Contains(prompt, footer) {
			t.Errorf("rung %.1f: the loader writes footer %q but the prompt says:\n  %s",
				rung, footer, prompt)
		}
		// And the size named in the prompt is the size the loader recorded.
		if rung != 0 && !strings.Contains(prompt, fmt.Sprintf("%.1fmm", rung)) {
			t.Errorf("rung %.1f: the prompt does not state the rung:\n  %s", rung, prompt)
		}
		if size != rung {
			t.Errorf("rung %.1f: the loader recorded %.1f", rung, size)
		}
	}
}

// TestProofE2ENamedRungDrivesThePrompt closes the gap the round-1 review found:
// every other test here calls ftProofLoader or ftProofOutcomeFor DIRECTLY, so
// nothing exercised ftProofOffer -- the one call site that actually builds the
// prompt the operator reads. Hardcoding rung 0 into ftProofOffer's resolver
// call reproduced the original round-0 consent bug with the whole suite green.
//
// This drives engraveTextFlow by touch: type BOTHPROOF!<rung>, read the prompt
// off the rendered frame, accept, and assert the plate that reaches
// freetextPlateHook is the one the prompt described.
func TestProofE2ENamedRungDrivesThePrompt(t *testing.T) {
	// BOTH QR choices. The mixed proof needs the whole plate, so accepting it
	// DROPS a QR the operator had chosen -- and the prompt must describe the
	// plate after that drop, not before. With QR off the two expressions agree
	// and the case proves nothing; this is the one that separates them.
	for _, tc := range []struct {
		rung float32
		qr   bool
	}{{4.4, false}, {6.0, false}, {4.4, true}, {6.0, true}} {
		rung := tc.rung
		t.Run(fmt.Sprintf("%.1fmm-qr=%v", rung, tc.qr), func(t *testing.T) {
			h, r := startFT(t)
			ftPastQR(h, tc.qr)
			ftTypeTrigger(h, fmt.Sprintf("%s%.1f", ftProofTriggerBoth, rung))
			ftOK(h)

			if !uiContains(h.content, "REPLACES ALL THREE") {
				t.Fatalf("OK on the rung trigger did not offer the pattern; frame %q", h.content)
			}
			// What the operator is told, off the REAL frame. Resolved with the
			// QR already dropped, because that is the plate that gets built.
			want := ftProofOutcomeFor(proofParams(), &ftProofs[2], rung, false)
			if !uiContains(h.content, want.Title) {
				t.Errorf("the prompt does not name the title %q it will write; frame %q",
					want.Title, h.content)
			}
			if !uiContains(h.content, want.Footer) {
				t.Errorf("the prompt does not name the footer %q it will write; frame %q",
					want.Footer, h.content)
			}
			if !uiContains(h.content, fmt.Sprintf("%.1fmm", rung)) {
				t.Errorf("the prompt does not state the %.1fmm rung; frame %q", rung, h.content)
			}

			h.tapWidget("proofYes")
			h.mustReach("lines")

			// Straight through to the engrave, and the plate must be the rung's.
			for _, step := range []string{"Title", "Footer", "Confirm"} {
				ftOK(h)
				if step == "Title" {
					ftPastSpeed(h)
				}
				h.mustReach(step)
			}
			ftOK(h)
			h.step()
			if !r.gotPlate {
				t.Fatal("the flow never built a plate")
			}
			if r.got.SizeMM != rung {
				t.Errorf("the operator asked for %.1fmm and the plate is %.1fmm", rung, r.got.SizeMM)
			}
			if r.got.Title != want.Title {
				t.Errorf("plate title %q, prompt said %q", r.got.Title, want.Title)
			}
			if r.got.Footer != want.Footer {
				t.Errorf("plate footer %q, prompt said %q", r.got.Footer, want.Footer)
			}
			// The QR really is gone, whatever the operator chose earlier.
			if r.got.QR != nil {
				t.Errorf("the mixed plate carries a QR; it needs the whole plate")
			}
			// A mixed plate must still be mixed at every rung.
			faces := map[*vector.Face]bool{}
			for _, f := range r.got.Faces {
				faces[f] = true
			}
			if len(faces) != 2 {
				t.Errorf("the %.1fmm mixed plate is cut in %d face(s), want 2", rung, len(faces))
			}
		})
	}
}
