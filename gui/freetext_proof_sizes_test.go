package gui

import (
	"fmt"
	"strings"
	"testing"

	"seedhammer.com/backup"
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
		if want := ftProofTitleBothAt(size); title != want {
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
	}
}
