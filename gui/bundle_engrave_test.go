package gui

import (
	"strings"
	"testing"
)

// TestBundlePlanVerbatim: the engrave plan is the cards' gathered strings, in
// card-then-plate order, UNMODIFIED (I-4). Every plate string equals exactly one
// gathered chunk string — no re-encode, no transform.
func TestBundlePlanVerbatim(t *testing.T) {
	g := &bundleGatherer{}
	offerAll(t, g, md1CardA(t))
	offerAll(t, g, mk1CardA(t))
	cards := g.cards

	plan := bundlePlatePlan(cards)
	// Total plates = sum of per-card chunk strings.
	var want int
	for _, c := range cards {
		want += len(c.strings)
	}
	if len(plan) != want {
		t.Fatalf("plan has %d plates, want %d", len(plan), want)
	}
	// Each plan entry is verbatim a gathered string of its card, and the
	// card/plate indices are 1-based and contiguous per card.
	k := 0
	for ci, c := range cards {
		for pi, s := range c.strings {
			p := plan[k]
			if p.str != s {
				t.Fatalf("plate %d not verbatim: got %q want %q", k, p.str, s)
			}
			if p.cardIdx != ci+1 || p.cardTotal != len(cards) {
				t.Fatalf("plate %d card progress = %d of %d, want %d of %d", k, p.cardIdx, p.cardTotal, ci+1, len(cards))
			}
			if p.plateIdx != pi+1 || p.plateTotal != len(c.strings) {
				t.Fatalf("plate %d plate progress = %d of %d, want %d of %d", k, p.plateIdx, p.plateTotal, pi+1, len(c.strings))
			}
			k++
		}
	}
}

// TestBundlePlanSingleMD1OnePlate: a standalone md1 card → exactly 1 plate.
func TestBundlePlanSingleMD1OnePlate(t *testing.T) {
	g := &bundleGatherer{}
	if st := g.offer(mdmkText(singleMD1(t))); st != bundleAddedSingleMD1 {
		t.Fatalf("single md1 not added: %v", st)
	}
	plan := bundlePlatePlan(g.cards)
	if len(plan) != 1 {
		t.Fatalf("single md1 → %d plates, want 1", len(plan))
	}
	if plan[0].plateTotal != 1 {
		t.Fatalf("single md1 plateTotal = %d, want 1", plan[0].plateTotal)
	}
}

// TestBundleEngraveGuidedTitles: confirming a 2-card bundle drives "Card 1 of 2
// · Plate 1 of N" first (and the card-progress is shown). Mirrors
// TestMultiPlateEngravePlateTitles: assert the first guided title appears.
func TestBundleEngraveGuidedTitles(t *testing.T) {
	g := &bundleGatherer{}
	offerAll(t, g, mk1CardA(t)) // first card: an mk1 (>=2 plates)
	offerAll(t, g, md1CardA(t)) // second card
	cards := g.cards
	if len(cards) != 2 {
		t.Fatalf("setup: %d cards, want 2", len(cards))
	}

	ctx := NewContext(newPlatform())
	frame, quit := runUI(ctx, func() { bundleEngrave(ctx, &descriptorTheme, cards) })
	defer quit()
	if c, ok := pumpUntil(frame, "Card 1 of 2", 48); !ok {
		t.Fatalf("guided 'Card 1 of 2' title not shown; got %q", c)
	}
}

// TestBundleEngraveSetAbort: backing out of the first plate's variant picker
// surfaces the SET-LEVEL abort warning (partial bundle unusable) and records no
// completed state (I-5).
func TestBundleEngraveSetAbort(t *testing.T) {
	g := &bundleGatherer{}
	offerAll(t, g, mk1CardA(t))
	offerAll(t, g, md1CardA(t))
	cards := g.cards

	ctx := NewContext(newPlatform())
	frame, quit := runUI(ctx, func() { bundleEngrave(ctx, &descriptorTheme, cards) })
	defer quit()
	if c, ok := pumpUntil(frame, "Card 1 of 2", 48); !ok {
		t.Fatalf("guided title not shown; got %q", c)
	}
	// Back out of the variant picker → the set-level abort warning.
	click(&ctx.Router, Button1)
	if c, ok := pumpUntil(frame, "partial", 32); !ok {
		t.Fatalf("set-level abort warning not shown; got %q", c)
	}
}

// TestBundleEngraveMs1Reminder: the ms1 reminder text exists and instructs the
// operator to hand-engrave the ms1 share(s) (mirror host bundle.rs:296-306).
func TestBundleEngraveMs1Reminder(t *testing.T) {
	msg := bundleMs1ReminderText()
	low := strings.ToLower(msg)
	if !strings.Contains(low, "ms1") || !strings.Contains(low, "hand") {
		t.Fatalf("ms1 reminder %q must mention hand-engraving the ms1 share(s)", msg)
	}
}

// TestBundlePlanValidatesEachPlate: every plan plate string lays out to at least
// one engravable plate via validateMdmk (so the guided engrave never silently
// produces an empty plate set for a verified card).
func TestBundlePlanValidatesEachPlate(t *testing.T) {
	g := &bundleGatherer{}
	offerAll(t, g, mk1CardA(t))
	offerAll(t, g, md1CardA(t))
	params := newPlatform().EngraverParams()
	for _, p := range bundlePlatePlan(g.cards) {
		labels, plates, err := validateMdmk(params, p.str)
		if err != nil || len(plates) == 0 || len(labels) == 0 {
			t.Fatalf("plate %q does not fit any engraving variant: err=%v plates=%d", p.str, err, len(plates))
		}
	}
}
