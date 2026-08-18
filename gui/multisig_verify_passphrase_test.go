package gui

import (
	"strings"
	"testing"

	"seedhammer.com/md"
)

// ─── F-191: a keystroke must not be reported as a wrong wallet ───────────────
//
// The engrave accepts a payload-borne passphrase (syswPassphraseFlow); the verify
// requires it RE-TYPED (passphraseFlow, deliberate per §7.4 -- comparing the
// engrave source against itself passes unconditionally). So a CORRECT seed with a
// mistyped or forgotten passphrase makes allUserSlots return empty, and the flow
// said:
//
//	"That seed is not a cosigner of the read-back policy, so it cannot prove any
//	 of these plates."
//
// That is the most alarming sentence this device can say. It tells an operator
// holding good plates and the right seed that their money is in a wallet they are
// not part of, and the cause is one wrong character. It is a FALSE RED that
// teaches exactly the wrong lesson: the operator's next move is to distrust
// perfectly good steel.
//
// The device KNOWS a passphrase was offered. It can say so, and one re-derivation
// with the empty passphrase settles the one case that can be settled outright.

// TestVerifyNamesThePassphraseBeforeCondemningTheSeed pins the three states apart.
func TestVerifyNamesThePassphraseBeforeCondemningTheSeed(t *testing.T) {
	// (a) A passphrase was typed, and the seed IS in the policy WITHOUT it. This
	// is the only arm the device can be certain about, and the empty re-derivation
	// is what buys the certainty.
	certain := multisigVerifyNoSlotBody(true, true)
	// (b) A passphrase was typed and the empty one does not match either. The
	// device cannot tell a wrong seed from a wrong passphrase here, and must not
	// pretend it can.
	unsure := multisigVerifyNoSlotBody(true, false)
	// (c) No passphrase was typed at all. Still not certain: the wallet may have
	// been built WITH one and the operator pressed Skip.
	skipped := multisigVerifyNoSlotBody(false, false)

	for name, msg := range map[string]string{"certain": certain, "unsure": unsure, "skipped": skipped} {
		low := strings.ToLower(msg)
		// NOTHING may say the flat thing any more. The old sentence asserted a
		// conclusion the device is never in a position to reach.
		if strings.Contains(low, "that seed is not a cosigner") {
			t.Errorf("%s: still says the seed is not a cosigner, which a mistyped "+
				"passphrase is enough to cause:\n%s", name, msg)
		}
		// EVERY arm names the passphrase, because in every one of them it is a
		// live explanation the operator can act on faster than re-cutting steel.
		if !strings.Contains(low, "passphrase") {
			t.Errorf("%s: never mentions the passphrase, so the cheapest explanation "+
				"is the one the operator is not offered:\n%s", name, msg)
		}
		if strings.ContainsAny(msg, "—–·‘’“”…") {
			t.Errorf("%s: carries a glyph the body face lacks, so its line does not "+
				"draw:\n%q", name, msg)
		}
	}

	// The certain arm says the thing that is certain: these plates MATCH this
	// seed. Getting this right is the whole value -- it converts the scariest
	// screen on the device into a typo report.
	//
	// R-M (S6b spec §3.2a) replaced this arm's wording wholesale -- "these
	// plates match this seed with NO passphrase" is the new affirmative claim,
	// where the arm used to say "That seed IS a cosigner of this policy". The
	// substring pinned here moved with it, verbatim-mandated text and all: R-M's
	// body is fixed and is asserted byte-for-byte elsewhere
	// (TestProvedInnocentBodyIsRMsAdoptedWording, multisig_verify_provedinnocent_test.go),
	// so this test's job stays what F-191 wrote it for -- confirming the CERTAIN
	// arm alone makes the affirmative claim, and the unsure/skipped arms do not.
	if !strings.Contains(strings.ToLower(certain), "match this seed") {
		t.Errorf("the certain arm does not tell the operator their seed IS in the "+
			"policy, which is the one fact the re-derivation proved:\n%s", certain)
	}
	// And the unsure arms do NOT claim it, because they have not proved it.
	for name, msg := range map[string]string{"unsure": unsure, "skipped": skipped} {
		if strings.Contains(strings.ToLower(msg), "match this seed") {
			t.Errorf("%s: claims the plates match this seed without having derived a "+
				"slot for it:\n%s", name, msg)
		}
	}

	// The three arms are pairwise distinct. A helper returning one string for
	// three states passes any test that checks only one of them.
	if certain == unsure || certain == skipped || unsure == skipped {
		t.Errorf("two of the three states read identically:\ncertain: %s\nunsure: %s\nskipped: %s",
			certain, unsure, skipped)
	}

	// The skipped arm has to tell the operator what to DO, and the action is
	// "add the passphrase and try again", not "re-cut your plates".
	if !strings.Contains(strings.ToLower(skipped), "add") {
		t.Errorf("the skipped arm never suggests adding the passphrase:\n%s", skipped)
	}

	// F-185: all three are showError bodies. They scroll, nothing says so, and
	// this machine has no button that scrolls them.
	assertModalBodyFits(t, "the verify's no-slot refusal (passphrase proved innocent)", errorScreenBody, certain)
	assertModalBodyFits(t, "the verify's no-slot refusal (passphrase typed)", errorScreenBody, unsure)
	assertModalBodyFits(t, "the verify's no-slot refusal (passphrase skipped)", errorScreenBody, skipped)
}

// TestVerifyPassphraseArmIsDecidedByARederivation.
//
// The wording above is only worth anything if the flow picks the right arm, and
// the decision rests on ONE fact: does this seed fill a slot when derived with
// the EMPTY passphrase? That is a real derivation against the read-back policy's
// real keys, so it is exercised here against a real policy rather than asserted
// from the switch's shape.
func TestVerifyPassphraseArmIsDecidedByARederivation(t *testing.T) {
	p, _, _, self, cards := s5TraceB(t)
	assembled, _, _, err := assembleBuildPolicy(p, self, cards)
	if err != nil {
		t.Fatalf("Trace B did not assemble: %v", err)
	}
	_, keys, err := md.ExpandWalletPolicyChunks(assembled)
	if err != nil {
		t.Fatalf("expanding the assembled policy: %v", err)
	}

	m := bip39FromWords(t, fixtureMasterA)

	// The policy was built from master A with NO passphrase, so master A fills
	// slots with the empty passphrase and fills none with a wrong one. That is
	// exactly F-191's scenario: right seed, wrong keystroke.
	withEmpty := allUserSlots(m, "", s5Net, keys)
	if len(withEmpty) == 0 {
		t.Fatal("INCONCLUSIVE: master A fills no slot of its own policy with the " +
			"empty passphrase, so this test cannot show the re-derivation working")
	}
	withWrong := allUserSlots(m, "not the passphrase", s5Net, keys)
	if len(withWrong) != 0 {
		t.Fatalf("INCONCLUSIVE: a wrong passphrase still filled %d slot(s); F-191's "+
			"scenario does not arise", len(withWrong))
	}
	t.Logf("master A fills %d slot(s) with the empty passphrase and %d with a wrong one",
		len(withEmpty), len(withWrong))

	// So the flow's rule -- "a passphrase was typed AND the empty one matches" --
	// selects the CERTAIN arm for this operator, and the certain arm is the one
	// that does not condemn their seed.
	if !multisigVerifySeedIsInnocent(m, "not the passphrase", s5Net, keys) {
		t.Error("the re-derivation did not clear a correct seed carrying a wrong " +
			"passphrase, so the operator is told their wallet is not theirs")
	}
	// And it does not clear a seed that genuinely is not in the policy. Without
	// this the predicate could be `return true` and pass everything above.
	//
	// NOT fixtureMasterC, which was the first draft and was WRONG: Trace B's
	// cosigner card IS master C at account 0, so C fills @3 and the assertion
	// failed for the right reason. BIP-39's all-"zoo" vector is a fourth master
	// that appears in no slot of this policy, and the check below proves that
	// rather than assuming it.
	other := bip39FromWords(t, "zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo wrong")
	if n := len(allUserSlots(other, "", s5Net, keys)); n != 0 {
		t.Fatalf("INCONCLUSIVE: the supposedly-foreign seed fills %d slot(s) of this "+
			"policy, so it cannot show the predicate refusing to clear one", n)
	}
	if multisigVerifySeedIsInnocent(other, "whatever", s5Net, keys) {
		t.Error("the re-derivation cleared a seed that fills no slot at all, which " +
			"would replace a true refusal with a passphrase excuse")
	}
	// A seed offered with NO passphrase is never "cleared by the empty
	// re-derivation": there is nothing to re-derive, and claiming otherwise would
	// route a genuinely foreign seed to the certain arm.
	if multisigVerifySeedIsInnocent(other, "", s5Net, keys) {
		t.Error("a seed offered with no passphrase was cleared by the empty " +
			"re-derivation, which is the same derivation that already failed")
	}
}
