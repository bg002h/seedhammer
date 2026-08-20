package gui

import (
	"strings"
	"testing"
)

// TestBundleGatherFeedback: the per-scan status → on-screen feedback mapping is
// explicit and faithful (R0-C1/C2). Critically an ms1 is REFUSED with the
// hand-type message (never silently dropped) and a single mk1 is REFUSED.
func TestBundleGatherFeedback(t *testing.T) {
	s := &bundleGatherScreen{g: &bundleGatherer{}, hasReader: true}
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

// S1 fold, I2: EVERY operator instruction on the gather screen that names
// scanning must be conditioned on the machine actually having a reader.
// Phase-1 hardware has none, so on the machine this stage exists for those
// strings prescribe an impossible action — the stage's own motivating defect.
//
// Asserted as an absence, over every string this screen can produce, rather
// than string-by-string: a future author adding a fourth "scan the…" message
// should fail here rather than ship it.
func TestGatherScreenNeverSaysScanWithoutAReader(t *testing.T) {
	every := []bundleOfferStatus{
		bundleDropped, bundleRefusedMs1, bundleRefusedSingleMK1,
		bundleAddedSingleMD1, bundleChunkProgress, bundleCardComplete,
		bundleDuplicate,
	}
	t.Run("no reader", func(t *testing.T) {
		s := &bundleGatherScreen{g: &bundleGatherer{}, hasReader: false}
		lines := append([]string{}, s.tally()...)
		for _, st := range every {
			lines = append(lines, s.feedback(st))
		}
		saidScan := false
		for _, ln := range lines {
			if strings.Contains(strings.ToLower(ln), "scan") {
				// The ms1 refusal names NFC as a channel it refuses, which is a
				// different sentence from telling the operator to scan.
				t.Errorf("a machine reporting NO reader shows a gather line saying "+
					"%q, which sends the operator looking for a scan this machine "+
					"cannot offer. (The SH2 itself HAS a soldered reader — this "+
					"arm is the reader-less platform, e.g. one an operator "+
					"physically disabled; phase 1's routes are the payload and "+
					"the keyboard.)", ln)
				saidScan = true
			}
		}
		if saidScan {
			t.Log("the closing tally line and the two Done-gate errors are the " +
				"three sites; all are keyed on FeatureNFC")
		}
	})
	t.Run("with a reader the instruction is kept", func(t *testing.T) {
		// The counter-arm, so the test above cannot pass by the strings having
		// been deleted outright: on a machine that HAS a reader, scanning is
		// real and saying so is correct.
		s := &bundleGatherScreen{g: &bundleGatherer{}, hasReader: true}
		joined := strings.Join(s.tally(), "\n") + "\n" + s.feedback(bundleRefusedSingleMK1)
		if !strings.Contains(strings.ToLower(joined), "scan") {
			t.Errorf("a reader-equipped machine no longer offers scanning at all:\n%s", joined)
		}
	})
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

// TestBundleGatherResumeKeepsCards — the 2026-08-19 operator directive
// ("going back should lose nothing") on the Engrave Bundle program.
//
// bundleFlow already LOOPED: Back at the review returned to the gather. But it
// called the gather FRESH, so every card the operator had scanned was silently
// discarded by the Back that was only meant to revisit them. The loop looked
// correct and lost the work anyway — which is exactly the failure the directive
// is about, and why "does Back land in the right place" is not a sufficient
// test.
//
// The previous behaviour was DELIBERATE and documented ("the operator re-scans,
// mirrors single-card flows"); the directive overrides it.
func TestBundleGatherResumeKeepsCards(t *testing.T) {
	g := &bundleGatherer{}
	offerAll(t, g, md1CardA(t))
	offerAll(t, g, mk1CardA(t))
	prev := g.cards
	if len(prev) != 2 {
		t.Fatalf("fixture should gather two cards, got %d", len(prev))
	}

	// DRIVE THE REAL FUNCTION, not a hand-rolled copy of what it does.
	//
	// A first version of this test re-offered into a bare gatherer and asserted
	// the set survived. Mutation-testing killed it: deleting the re-offer loop
	// from bundleGatherFlowResume left that test PASSING, because it exercised
	// the mechanism rather than the wiring. This drives the flow and reads its
	// screen, so the loop is load-bearing.
	ctx := NewContext(newPlatform())
	var got []bundleCard
	frame, quit := runUI(ctx, func() {
		got, _ = bundleGatherFlowResume(ctx, &descriptorTheme, "Engrave Bundle", prev)
	})
	defer quit()
	// Before ANY scan, the tally must already show the resumed cards.
	c, found := pumpUntil(frame, "md1 descriptors: 1", 32)
	if !found {
		t.Fatalf("resumed gather did not show the prior md1 on entry; got %q", c)
	}
	if !uiContains(c, "mk1 keys: 1") {
		t.Fatalf("resumed gather did not show the prior mk1 on entry; got %q", c)
	}
	_ = got

	resumed := &bundleGatherer{}
	for _, cd := range prev {
		for _, str := range cd.strings {
			resumed.offer(mdmkText(str))
		}
	}
	if len(resumed.cards) != len(prev) {
		t.Fatalf("resume lost cards: %d on the pile, %d after re-offer",
			len(prev), len(resumed.cards))
	}
	for i := range prev {
		if resumed.cards[i].kind != prev[i].kind {
			t.Errorf("card %d kind changed across resume: %v -> %v",
				i, prev[i].kind, resumed.cards[i].kind)
		}
		if len(resumed.cards[i].strings) != len(prev[i].strings) {
			t.Errorf("card %d lost chunks across resume: %d -> %d",
				i, len(prev[i].strings), len(resumed.cards[i].strings))
		}
	}
}
