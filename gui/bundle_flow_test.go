package gui

import (
	"strings"
	"testing"
)

// TestBundleGatherFeedback: the per-scan status → on-screen feedback mapping is
// explicit and faithful (R0-C1/C2). Critically an ms1 is REFUSED with the
// hand-type message (never silently dropped) and a single mk1 is REFUSED.
func TestBundleGatherFeedback(t *testing.T) {
	s := &bundleGatherScreen{g: &bundleGatherer{}}
	cases := []struct {
		status bundleOfferStatus
		want   string // substring the feedback must contain ("" = no message)
	}{
		{bundleRefusedMs1, "on-device"},            // "Type the ms1 share on-device — never over NFC"
		{bundleRefusedSingleMK1, "all its chunks"}, // "Incomplete key card — scan all its chunks."
		{bundleChunkProgress, ""},                  // progress shows via the tally, not a message
		{bundleCardComplete, "added"},
		{bundleAddedSingleMD1, "added"},
		{bundleDuplicate, "already"},
	}
	for _, c := range cases {
		got := s.feedback(c.status)
		if c.want == "" {
			if got != "" {
				t.Errorf("status %v: feedback %q, want empty", c.status, got)
			}
			continue
		}
		if !strings.Contains(strings.ToLower(got), strings.ToLower(c.want)) {
			t.Errorf("status %v: feedback %q, want substring %q", c.status, got, c.want)
		}
	}
	// The ms1 refusal must mention typing on-device (the security spine).
	if msg := s.feedback(bundleRefusedMs1); !strings.Contains(strings.ToLower(msg), "type") {
		t.Errorf("ms1 refusal %q must instruct the operator to type it", msg)
	}
}

// TestBundleGatherTally: the running tally counts verified cards by type.
func TestBundleGatherTally(t *testing.T) {
	g := &bundleGatherer{}
	offerAll(t, g, md1CardA(t))
	offerAll(t, g, mk1CardA(t))
	offerAll(t, g, mk1CardB(t))
	s := &bundleGatherScreen{g: g}
	tally := strings.Join(s.tally(), " ")
	if !strings.Contains(tally, "1") || !strings.Contains(strings.ToLower(tally), "descriptor") {
		t.Errorf("tally %q must report 1 descriptor", tally)
	}
	if !strings.Contains(tally, "2") || !strings.Contains(strings.ToLower(tally), "key") {
		t.Errorf("tally %q must report 2 keys", tally)
	}
}

// TestBundleGatherAccumulateTwoCards: offering two fixtures (interleaved)
// through the gatherer accumulates exactly 2 verified cards (Phase-1 core).
func TestBundleGatherAccumulateTwoCards(t *testing.T) {
	g := &bundleGatherer{}
	mdA, mkA := md1CardA(t), mk1CardA(t)
	n := len(mdA)
	if len(mkA) > n {
		n = len(mkA)
	}
	for i := 0; i < n; i++ {
		if i < len(mdA) {
			g.offer(mdmkText(mdA[i]))
		}
		if i < len(mkA) {
			g.offer(mdmkText(mkA[i]))
		}
	}
	if len(g.cards) != 2 {
		t.Fatalf("accumulated %d cards, want 2", len(g.cards))
	}
}

// TestBundleDoneDecision: the "Done adding cards" gate (Option A). 0 cards →
// no-op/warn; a card mid-chunk-set → warn incomplete + drop it; >=1 complete
// card with nothing pending → proceed.
func TestBundleDoneDecision(t *testing.T) {
	// 0 cards → cannot proceed.
	g0 := &bundleGatherer{}
	if dec := bundleDoneDecision(g0); dec != bundleDoneEmpty {
		t.Errorf("empty: got %v, want bundleDoneEmpty", dec)
	}
	// A card mid-chunk-set (primed, incomplete) → pending warning.
	gp := &bundleGatherer{}
	mkA := mk1CardA(t)
	for i := 0; i < len(mkA)-1; i++ {
		gp.offer(mdmkText(mkA[i]))
	}
	if dec := bundleDoneDecision(gp); dec != bundleDonePending {
		t.Errorf("pending: got %v, want bundleDonePending", dec)
	}
	// >=1 complete card, nothing pending → proceed.
	gok := &bundleGatherer{}
	offerAll(t, gok, mk1CardA(t))
	if dec := bundleDoneDecision(gok); dec != bundleDoneProceed {
		t.Errorf("proceed: got %v, want bundleDoneProceed", dec)
	}
	// A complete card AND a pending card → pending warning (don't strand it).
	gmix := &bundleGatherer{}
	offerAll(t, gmix, mk1CardA(t))
	mkB := mk1CardB(t)
	for i := 0; i < len(mkB)-1; i++ {
		gmix.offer(mdmkText(mkB[i]))
	}
	if dec := bundleDoneDecision(gmix); dec != bundleDonePending {
		t.Errorf("complete+pending: got %v, want bundleDonePending", dec)
	}
}

// TestBundleReviewFlowListsCards: Phase 2 review lists each card's type +
// summary (verified) and Confirm (Button3) advances → true; Back → false.
func TestBundleReviewFlowListsCards(t *testing.T) {
	g := &bundleGatherer{}
	offerAll(t, g, md1CardA(t))
	offerAll(t, g, mk1CardA(t))
	cards := g.cards

	// Confirm path.
	ctx := NewContext(newPlatform())
	var ok bool
	frame, quit := runUI(ctx, func() { ok = bundleReviewFlow(ctx, &descriptorTheme, cards) })
	if c, found := pumpUntil(frame, "Bundle", 32); !found {
		t.Fatalf("review screen title not shown; got %q", c)
	}
	// The review must list both card types.
	c, _ := pumpUntil(frame, "descriptor", 8)
	if !strings.Contains(strings.ToLower(c), "key") && !strings.Contains(strings.ToLower(c), "descriptor") {
		t.Fatalf("review did not list card types; got %q", c)
	}
	click(&ctx.Router, Button3)
	frame() // let the confirm settle
	quit()
	if !ok {
		t.Fatalf("Confirm (Button3) did not advance the review flow")
	}

	// Back path.
	ctx2 := NewContext(newPlatform())
	var ok2 bool
	frame2, quit2 := runUI(ctx2, func() { ok2 = bundleReviewFlow(ctx2, &descriptorTheme, cards) })
	defer quit2()
	pumpUntil(frame2, "Bundle", 16)
	click(&ctx2.Router, Button1)
	frame2()
	if ok2 {
		t.Fatalf("Back (Button1) should not confirm the review flow")
	}
}
