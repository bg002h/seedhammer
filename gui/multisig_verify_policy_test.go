package gui

import (
	"strings"
	"testing"

	"seedhammer.com/bip39"
	"seedhammer.com/md"
)

// ─── S5 C1: the obligation carries the POLICY, not just the slot indices ─────
//
// The slot set binds the verify to this run's slot INDICES. It does not bind it
// to this run's POLICY, and the policy is re-read from the evidence being judged
// (multisigVerifyFlow decodes the READBACK md1 and derives every leg against
// it). So the integers in expectedSlots are re-based onto whatever wallet the
// operator presents, and a byte-valid plate set from ANOTHER wallet satisfies
// them while the plates this run cut are never read.
//
// Executed, and it is the false GREEN this whole seam exists to remove: engrave
// wallet P (3-of-4), present wallet P' (the SAME four cosigners at 2-of-4), and
// the flow reported "Verify OK".
//
// The closing datum already exists at both call sites: they hold the md1 they
// just engraved. It travels with the obligation now, and the readback must EQUAL
// it.

// s5PolicyPair mints TWO wallets over the SAME cosigner set: P, Trace B's
// 3-of-4, and P', the same four keys at threshold 2-of-4.
//
// P' IS A DIFFERENT WALLET, not a corrupted P: it is byte-valid, self-consistent
// and its plates were cut by its own (earlier) run, with its own policy-id stub
// binding every key card to it. That is what makes it the attack -- every
// in-policy check the verify performs compares P' against P' and agrees.
//
// The realistic reach is the operator this verify exists for: a re-cut after
// changing k, with the superseded generation's steel on the same bench.
func s5PolicyPair(t *testing.T) (engravedMd1, otherMd1, otherPlate []string, slot int) {
	t.Helper()
	p, reg, sources, self, cards := s5TraceB(t)
	engravedMd1, _, _, err := assembleBuildPolicy(p, self, cards)
	if err != nil {
		t.Fatalf("Trace B did not assemble: %v", err)
	}
	sibling := p
	sibling.K = 2
	otherMd1, _, _, err = assembleBuildPolicy(sibling, self, cards)
	if err != nil {
		t.Fatalf("the 2-of-4 sibling did not assemble: %v", err)
	}

	// THE PREMISE, MEASURED. Two DIFFERENT wallets: a different spending
	// threshold, so different md1 bytes and a different policy-id stub. If these
	// ever became equal this test would be substituting a policy for itself and
	// would prove nothing.
	if strings.Join(engravedMd1, "|") == strings.Join(otherMd1, "|") {
		t.Fatalf("the 3-of-4 and the 2-of-4 encode to the SAME md1, so this test has no "+
			"second wallet to substitute:\n%v", engravedMd1)
	}

	m, err := bip39.ParseMnemonic(fixtureMasterB)
	if err != nil {
		t.Fatalf("ParseMnemonic(master B): %v", err)
	}
	_, otherKeys, err := md.ExpandWalletPolicyChunks(otherMd1)
	if err != nil {
		t.Fatalf("the sibling policy does not decode: %v", err)
	}
	otherSlots := allUserSlots(m, "", s5Net, otherKeys)
	if len(otherSlots) != 1 {
		t.Fatalf("master B fills slots %v of the sibling policy, want exactly 1", otherSlots)
	}
	slot = otherSlots[0]

	// The SAME slot index in the engraved policy, or the substitution would fail
	// on the index rather than on the policy and this test would pass for the
	// wrong reason.
	_, ownKeys, err := md.ExpandWalletPolicyChunks(engravedMd1)
	if err != nil {
		t.Fatalf("the engraved policy does not decode: %v", err)
	}
	ownSlots := allUserSlots(m, "", s5Net, ownKeys)
	if len(ownSlots) != 1 || ownSlots[0] != slot {
		t.Fatalf("master B fills slots %v of the ENGRAVED policy, want exactly [%d]; the "+
			"substitution has to be index-compatible or it is refused by the slot set",
			ownSlots, slot)
	}

	// The plate the SIBLING's own run cut for that slot, taken from the
	// production tail rather than hand-built.
	_, _, cardsOut, err := buildEngraveTail(sources, sibling.Script, reg, s5Net, cards, otherMd1, false)
	if err != nil {
		t.Fatalf("buildEngraveTail(sibling): %v", err)
	}
	var plates [][]string
	for _, c := range cardsOut {
		if c.kind == cardMK1 {
			plates = append(plates, c.strings)
		}
	}
	b, err := deriveMultisigLeg(m, "", s5Net, otherKeys[slot].OriginPath, otherMd1, false)
	if err != nil {
		t.Fatalf("deriveMultisigLeg(sibling @%d): %v", slot, err)
	}
	otherPlate = plates[s5PlateFor(t, plates, verifyLeg{Slot: slot, B: b})]
	return engravedMd1, otherMd1, otherPlate, slot
}

// TestVerifyRefusesPlatesFromADifferentPolicy is C1's own arm.
//
// A readback that is internally perfect but belongs to ANOTHER wallet must FAIL,
// with a refusal that says what happened -- the operator is holding plates from
// a different wallet, and no amount of re-presenting these ones will fix it.
func TestVerifyRefusesPlatesFromADifferentPolicy(t *testing.T) {
	engravedMd1, otherMd1, otherPlate, slot := s5PolicyPair(t)
	records := append(append([]string(nil), otherMd1...), otherPlate...)

	// TOLERANT, because the refusal must land BEFORE the operator is asked for a
	// secret: nothing this flow does afterwards can produce a true answer about
	// another wallet's plates, so typing a seed for them buys exposure and no
	// information. s5DriveVerify fatals on exactly that outcome, which is why it
	// is not the driver here -- and its fatal message is the proof that the
	// refusal is pre-seed, since a post-seed refusal would drive fine.
	last := s5DriveVerifyTolerant(t, records, []int{slot}, engravedMd1, fixtureMasterB)
	t.Logf("final screen: %q", last)
	if uiContains(last, "Verify OK") {
		t.Fatalf("plates from a DIFFERENT wallet verified clean. Final screen: %q\n"+
			"The plate this run cut and the md1 this run cut were never read: the slot set "+
			"carries index @%d but not the policy that index means, so the readback supplied "+
			"the policy AND the evidence and compared them against each other", last, slot)
	}
	if !uiContains(last, "NOT the wallet policy this run engraved") {
		t.Errorf("the refusal does not name the cause. Final screen: %q\n"+
			"A generic failure sends the operator to re-cut good plates; the fact they can "+
			"act on is that this md1 is another wallet's", last)
	}
}

// TestVerifyStillPassesItsOwnPolicy is the non-vacuity arm: the same fixture's
// OWN md1 and OWN plate must still verify clean.
//
// Without it, "refuse everything" satisfies the test above.
func TestVerifyStillPassesItsOwnPolicy(t *testing.T) {
	_, otherMd1, otherPlate, slot := s5PolicyPair(t)
	records := append(append([]string(nil), otherMd1...), otherPlate...)

	last, _ := s5DriveVerify(t, records, []int{slot}, otherMd1, fixtureMasterB)
	if !uiContains(last, "Verify OK") {
		t.Fatalf("a run verifying ITS OWN policy and ITS OWN plate did not pass. Final "+
			"screen: %q\nThe md1 binding must reject another wallet, not every wallet", last)
	}
}

// ─── S5 M2: a mid-loop refusal must not walk out of a PARTIAL verify ─────────
//
// The three "this seed cannot advance the verify" screens all `return`ed, even
// with legs already verified. On the second or later seed that left the operator
// with some plates checked, some not, and NO verdict -- and on the build path the
// next thing they saw was the restore document. It is the same defect the ms1
// entry's Back was converted from `return` to `break` to prevent, in the same
// function, against the same reasoning.

// s5DriveVerifyTwoSeeds drives multisigVerifyFlow through TWO seed entries and
// returns the last frame.
//
// It exists because the single-seed drivers cannot reach the mid-loop refusals
// at all: those refusals only differ from a first-seed refusal once `legs` is
// non-empty, which takes a seed that covered something first.
func s5DriveVerifyTwoSeeds(t *testing.T, records []string, expected []int, engravedMd1 []string, first, second string) string {
	t.Helper()
	p := newPlatform()
	p.display = sh2DisplaySize
	ctx := NewContext(p)
	ctx.syswBundleSeeds = append([]string(nil), records...)

	frame, quit := runUI(ctx, func() {
		multisigVerifyFlow(ctx, &descriptorTheme, false, expected, engravedMd1, &verifyRecord{})
	})
	defer quit()

	if c, ok := pumpUntil(frame, "mk1 keys:", 64); !ok {
		t.Fatalf("the readback never reached the gatherer's tally; got %q", c)
	}
	click(&ctx.Router, Button3) // Done adding cards
	frame()

	typeSeed := func(phrase string) {
		t.Helper()
		if c, ok := pumpUntil(frame, "Choose number of words", 64); !ok {
			t.Fatalf("seed entry was not reached; got %q", c)
		}
		click(&ctx.Router, Button3) // 12 WORDS
		frame()
		driveWords(&ctx.Router, phrase)
		if c, ok := pumpUntil(frame, "passphrase", 240); !ok {
			t.Fatalf("the passphrase prompt was not reached; got %q", c)
		}
		click(&ctx.Router, Button3) // Skip
		frame()
	}

	typeSeed(first)
	if c, ok := pumpUntil(frame, "not checked yet", 64); !ok {
		t.Fatalf("the flow did not offer the next seed after the first one; got %q", c)
	}
	click(&ctx.Router, Button3) // TYPE THE NEXT SEED
	frame()
	typeSeed(second)

	// The refusal modal for the second seed, then whatever the flow says next.
	last := ""
	for i := 0; i < 96; i++ {
		c, ok := frame()
		if !ok {
			break
		}
		last = c
		// The mid-loop refusals, by the phrase each of them now carries. I-14
		// replaced "The plates still outstanding belong to a different seed." --
		// which the device cannot support, and which is false whenever a per-slot
		// passphrase diverged by one character -- so the needle moved with it.
		if uiContains(last, "were built from different words") ||
			uiContains(last, "DO fill another slot with no passphrase") ||
			uiContains(last, "cannot prove any of these plates") ||
			uiContains(last, "No slot matches that seed") {
			click(&ctx.Router, Button3) // dismiss the refusal
			continue
		}
		if uiContains(last, "Verify OK") || uiContains(last, "Verify Failed") ||
			uiContains(last, "Verify Incomplete") {
			break
		}
	}
	return last
}

// TestVerifyReportsIncompleteAfterAMidLoopRefusal is M2's arm.
//
// Trace B engraves three plates for slots {0,1,2}. Master A covers @0 and @1;
// the operator then types master C, which IS a cosigner of this policy (the
// payload card at @3) but whose slot this run did not engrave. The flow refuses
// that seed -- correctly -- and must then REPORT the partial verify rather than
// walking out over two plates it did check and one it did not.
func TestVerifyReportsIncompleteAfterAMidLoopRefusal(t *testing.T) {
	md1, plates, _ := s5TraceBEngraved(t, false)
	records := append([]string(nil), md1...)
	for _, p := range plates {
		records = append(records, p...)
	}

	// THE PREMISE, MEASURED: master C is a cosigner of the read-back policy and
	// fills NO engraved slot. That is the arm under test; if C stopped being a
	// cosigner this would exercise the "not a cosigner" arm instead and the test
	// would still pass while checking something else.
	mc, err := bip39.ParseMnemonic(fixtureMasterC)
	if err != nil {
		t.Fatalf("ParseMnemonic(master C): %v", err)
	}
	_, keys, err := md.ExpandWalletPolicyChunks(md1)
	if err != nil {
		t.Fatalf("ExpandWalletPolicyChunks: %v", err)
	}
	cSlots := allUserSlots(mc, "", s5Net, keys)
	if len(cSlots) == 0 {
		t.Fatalf("master C fills no slot of Trace B, so this drives the wrong refusal arm")
	}
	for _, s := range cSlots {
		if s == 0 || s == 1 || s == 2 {
			t.Fatalf("master C fills engraved slot @%d, so it would ADVANCE the verify "+
				"instead of being refused", s)
		}
	}

	last := s5DriveVerifyTwoSeeds(t, records, []int{0, 1, 2}, md1, fixtureMasterA, fixtureMasterC)
	t.Logf("final screen: %q", last)
	if uiContains(last, "Verify OK") {
		t.Fatalf("a verify that checked 2 of 3 plates reported a PASS: %q", last)
	}
	if !uiContains(last, "Verify Incomplete") {
		t.Fatalf("the flow walked out of a PARTIAL verify with no report. Final screen: %q\n"+
			"Two plates were verified and one was not; silence after a verify is "+
			"indistinguishable from a pass to the operator who walks away, which is the "+
			"reason the ms1 entry's Back was converted from return to break in this same "+
			"function", last)
	}
	// AND IT SAYS WHAT WAS CHECKED, BY SLOT. The number used to be `len(legs)` --
	// a count of slots RE-DERIVED FROM A TYPED SEED, printed as "key plates
	// checked" with the comparator structurally unreachable on this path (C-1).
	// It is now produced by verifyMultisigLegsPartial, and the screen names the
	// slots in both directions so the operator knows WHICH plate is outstanding.
	if !uiContains(last, "Checked 2 key plates") {
		t.Errorf("the incomplete report does not say how much was checked: %q", last)
	}
	for _, want := range []string{"@0", "@1", "@2", "NOT verified"} {
		if !uiContains(last, want) {
			t.Errorf("the incomplete report does not carry %q, so the operator cannot "+
				"tell which slot is still unproved: %q", want, last)
		}
	}
}
