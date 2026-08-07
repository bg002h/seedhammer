package gui

import (
	"slices"
	"testing"

	"seedhammer.com/backup"
)

// The Font and Size screens are tested through their OPTION BUILDERS rather
// than by driving frames: the builders are where every decision this feature
// makes actually lives, and a frame-driving test would assert the widget works
// -- which ChoiceScreen's own tests already cover.

// TestFaceOptionsOfferBothFacesWithSHFirst pins the two things a reorder would
// silently change: which faces are reachable, and which one is the default.
// engraveTextFlow starts at &ftPlanSH and ChoiceScreen starts at index 0, so
// the default is a property of this ORDERING and nothing else states it.
func TestFaceOptionsOfferBothFacesWithSHFirst(t *testing.T) {
	labels, plans := ftFaceOptions(&ftPlanSH)
	if len(labels) != 2 || len(plans) != 2 {
		t.Fatalf("the Font screen offers %d labels and %d plans, want 2 and 2", len(labels), len(plans))
	}
	if plans[0] != &ftPlanSH {
		t.Errorf("index 0 selects %q, want sh -- index 0 IS the default", plans[0].Name())
	}
	if plans[1] != &ftPlanConst {
		t.Errorf("index 1 selects %q, want constant", plans[1].Name())
	}
	// Read from the faces, never spelled again here, so the screen and
	// ftFaceSummary cannot drift apart.
	if labels[0] != ftFaceSH.Name || labels[1] != ftFaceConst.Name {
		t.Errorf("labels are %q, want [%q %q]", labels, ftFaceSH.Name, ftFaceConst.Name)
	}
}

// TestSizeOptionsAreAutoFitPlusEveryPinnedRung ranges over backup.FontSizes in
// the test too. A hand-written list on either side is how an unpinned size gets
// offered: FontSizes is the only set every capacity number in backup is
// measured against, so an offered rung must be a rung from it.
func TestSizeOptionsAreAutoFitPlusEveryPinnedRung(t *testing.T) {
	labels, sizes := ftSizeOptions(&ftPlanSH, 0)
	if len(sizes) != len(backup.FontSizes)+1 {
		t.Fatalf("the Size screen offers %d entries, want auto-fit plus %d rungs",
			len(sizes), len(backup.FontSizes))
	}
	if sizes[0] != 0 {
		t.Errorf("index 0 selects %v, want 0 (auto-fit) -- index 0 IS the default", sizes[0])
	}
	if labels[0] != ftSizeAutoFit {
		t.Errorf("index 0 reads %q, want %q", labels[0], ftSizeAutoFit)
	}
	if got, want := sizes[1:], backup.FontSizes; !slices.Equal(got, want) {
		t.Errorf("the rungs offered are %v, want exactly backup.FontSizes %v", got, want)
	}
	for i, s := range sizes[1:] {
		if !slices.Contains(backup.FontSizes, s) {
			t.Errorf("entry %d offers %.1fmm, which is not a pinned rung", i+1, s)
		}
	}
}

// TestBothFacesOfferTheSameSizes. Operator requirement, 2026-08-06: font/sh
// gets size selection and auto-fit exactly as font/constant does. Neither face
// is the "real" one and neither is a special case, so the two lists must be
// element-for-element identical -- asserted rather than assumed, because the
// only thing making it true today is that ftSizeOptions branches on
// ftPlanIsProof and not on which face it was handed.
func TestBothFacesOfferTheSameSizes(t *testing.T) {
	shLabels, shSizes := ftSizeOptions(&ftPlanSH, 0)
	cnLabels, cnSizes := ftSizeOptions(&ftPlanConst, 0)
	if !slices.Equal(shSizes, cnSizes) {
		t.Errorf("sh offers sizes %v and constant offers %v; they must be identical", shSizes, cnSizes)
	}
	if !slices.Equal(shLabels, cnLabels) {
		t.Errorf("sh offers labels %q and constant offers %q; they must be identical", shLabels, cnLabels)
	}
	if len(shSizes) != len(backup.FontSizes)+1 || shSizes[0] != 0 {
		t.Errorf("sh does not get auto-fit plus every rung: %v", shSizes)
	}
	// And both reach every rung on a real plate, not just on the screen.
	P := newPlatform().EngraverParams()
	for _, rung := range backup.FontSizes {
		for _, plan := range []*ftPlan{&ftPlanSH, &ftPlanConst} {
			f := ftEvaluate(P, plan, "~", "", "", false, rung)
			if f.err != nil || !f.ok {
				t.Errorf("%s at %.1fmm: not admissible (err=%v ok=%v)", plan.Name(), rung, f.err, f.ok)
				continue
			}
			for i, s := range f.plate.Sizes {
				if s != rung {
					t.Errorf("%s at %.1fmm: row %d is cut at %.1fmm", plan.Name(), rung, i, s)
				}
			}
		}
	}
}

// TestProofCompositionIsStateNotAChoice. A proof trigger can leave a plan that
// is not in the Font list, and the pickers sit BEFORE the text screen, so Back
// out of Text lands on them. Both screens must then show ONE entry naming the
// composition -- state, not a decision -- exactly as ftQRChoiceFlow does for a
// sized composition. Otherwise a pinned proof plate can be half-edited into a
// composition nothing measures.
func TestProofCompositionIsStateNotAChoice(t *testing.T) {
	for _, plan := range []*ftPlan{&ftPlanBoth, &ftPlanSizeFront, &ftPlanSizeBack} {
		labels, plans := ftFaceOptions(plan)
		if len(labels) != 1 || len(plans) != 1 {
			t.Errorf("%s: the Font screen offers %d entries, want exactly 1 (state, not a choice)",
				plan.Name(), len(labels))
			continue
		}
		if plans[0] != plan {
			t.Errorf("%s: taking the only Font entry changes the plan to %q", plan.Name(), plans[0].Name())
		}
		if labels[0] != plan.Name() {
			t.Errorf("%s: the Font entry reads %q, want the composition's own name", plan.Name(), labels[0])
		}

		// The size screen is the same shape, and taking its only entry must not
		// move the rung either.
		const prior float32 = 4.4
		slabels, ssizes := ftSizeOptions(plan, prior)
		if len(slabels) != 1 || len(ssizes) != 1 {
			t.Errorf("%s: the Size screen offers %d entries, want exactly 1", plan.Name(), len(slabels))
			continue
		}
		if ssizes[0] != prior {
			t.Errorf("%s: taking the only Size entry moved the rung to %v, want it unchanged at %v",
				plan.Name(), ssizes[0], prior)
		}
	}
	// A sized plan states its own rungs, so the entry names them rather than
	// reading "Auto-fit" over a plate that is nothing of the sort.
	if labels, _ := ftSizeOptions(&ftPlanSizeFront, 0); labels[0] == ftSizeAutoFit {
		t.Errorf("a size ladder's only Size entry reads %q", ftSizeAutoFit)
	}
}

// TestFaceAndSizeReachConstAtEveryRung is the case this feature exists for and
// the one that is UNREACHABLE today: ftProofForTrigger returns rung 0 for every
// non-sizeable trigger, so a short text could only ever be cut at the largest
// rung that fits it.
func TestFaceAndSizeReachConstAtEveryRung(t *testing.T) {
	P := newPlatform().EngraverParams()
	for _, rung := range backup.FontSizes {
		f := ftEvaluate(P, &ftPlanConst, "~", "", "", false, rung)
		if f.err != nil || !f.ok {
			t.Errorf("constant at %.1fmm: not admissible (err=%v ok=%v)", rung, f.err, f.ok)
			continue
		}
		for i, s := range f.plate.Sizes {
			if s != rung {
				t.Errorf("constant at %.1fmm: row %d is cut at %.1fmm", rung, i, s)
			}
		}
		for i, face := range f.plate.Faces {
			if face != ftFaceConst.Face {
				t.Errorf("constant at %.1fmm: row %d is not cut in font/constant", rung, i)
			}
		}
	}
}

// TestAutoFitIsUnchangedByTheDefaults is what proves the feature is ADDITIVE:
// taking index 0 on both screens must reproduce exactly what the program built
// before either screen existed.
func TestAutoFitIsUnchangedByTheDefaults(t *testing.T) {
	P := newPlatform().EngraverParams()
	const text = "Dear heir the wallet is in the safe and the PIN is not written down at all"

	_, plans := ftFaceOptions(&ftPlanSH)
	_, sizes := ftSizeOptions(&ftPlanSH, 0)
	defPlan, defSize := plans[0], sizes[0]

	got, err := ftBuildPlate(P, defPlan, text, "TO MY HEIR", "2026 COPY 1", false, defSize, 0)
	if err != nil {
		t.Fatal(err)
	}
	// The pre-feature call: the hardcoded plan and the zero size.
	want, err := ftBuildPlate(P, &ftPlanSH, text, "TO MY HEIR", "2026 COPY 1", false, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if g, w := ftSpline(t, got), ftSpline(t, want); !slices.Equal(g, w) {
		t.Errorf("the screens' defaults do not reproduce the pre-feature plate (%d knots vs %d)", len(g), len(w))
	}
}

// TestSizeChoiceCanBeRefused: choosing a rung that the typed text no longer
// fits must reach the existing refusal, not the engraver. ftRefuse already
// handles it -- but before this feature that path could only be reached through
// a proof trigger, so it was never exercised from a plain composition.
func TestSizeChoiceCanBeRefused(t *testing.T) {
	P := newPlatform().EngraverParams()
	long := ""
	for range 60 {
		long += "the wallet is in the safe and the PIN is not written down "
	}
	f := ftEvaluate(P, &ftPlanConst, long, "", "", false, backup.FontSizes[0])
	if f.ok && f.err == nil {
		t.Errorf("a plate-busting text at the largest rung was admitted; the refusal is unreachable")
	}
}
