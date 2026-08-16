package gui

import (
	"strings"
	"testing"

	"seedhammer.com/bip39"
	"seedhammer.com/md"
)

// ─── S5 I1: a supplied policy that seats ONE key at TWO slots ────────────────
//
// The supply path deliberately engraves a descriptor someone else authored,
// repeated keys and all (gui/multisig_build.go's §4.1 scoping note: the device
// is not the author there). F-188 then made that path cut a plate per MATCHED
// slot -- and for this shape the two plates are BYTE-IDENTICAL, because an mk1
// carries (origin, fingerprint, policy stub, xpub) and all four agree.
//
// The gatherer keys mk1 cards on the payload-derived chunk_set_id, so a
// byte-identical second card is a duplicate and can never be read back as two.
// The verify's length precheck then refused forever -- "Read back 1 key plate,
// but this run engraved 2 key plates. Present exactly the plates this run cut."
// -- while the operator was doing exactly that. An honest, announced, hours-long
// engrave that could never be verified.
//
// RULED (plan §0.1's ladder): engrave ONE plate for a byte-identical pair, and
// announce it.
//
//   - clause 1: no standard governs, and two byte-identical plates carry
//     identical information.
//   - clause 2, the funds-safety boundary: the collapse is NOT invisible. The
//     census states the plate count before the tail, the md1 carries the policy
//     showing both slots, and the restore doc lists the inventory. Detectable by
//     reading the output, therefore eligible to assume.
//   - clause 3: announced on the census screen, before the first cut.
//
// Refusing an admitted policy would be choosing the refusal arm for testability,
// which §0.1 forbids.
//
// mk1-IDENTITY DEDUPE IS THE RIGHT MECHANISM HERE. A previous block deleted such
// a dedupe as inert -- and it was, for the REUSED-SEED shape, where one seed sits
// at several accounts and the keys DIFFER. This is the other shape: the keys are
// identical. Same mechanism, different shape.

// s5DuplicateSlotMd1 is a supplied 2-of-2 declaring master B's key at BOTH
// slots, at the one shared origin.
//
// It goes through md.EncodeMultisig directly rather than assembleBuildPolicy,
// because the BUILD path refuses this (duplicateSlotPair, SPEC §4.1) and the
// SUPPLY path is where it is admitted. A coordinator bug or a copy-paste
// duplicate produces it.
func s5DuplicateSlotMd1(t *testing.T) []string {
	t.Helper()
	xpub, fp := dupTestSelf(t, fixtureMasterB)
	cc, pk, _, err := decodeXpubBytes(xpub)
	if err != nil {
		t.Fatalf("decodeXpubBytes: %v", err)
	}
	c := md.MultisigCosigner{
		ChainCode:        cc,
		CompressedPubkey: pk,
		Fingerprint:      fpBytes(fp),
		FpPresent:        true,
	}
	out, _, _, err := md.EncodeMultisig(md.EncodeMultisigRequest{
		Cosigners:    []md.MultisigCosigner{c, c},
		K:            2,
		Script:       md.MultisigWsh,
		OriginMode:   md.OriginShared,
		SharedOrigin: originComponents(multisigSharedOrigin()),
	})
	if err != nil {
		t.Fatalf("md.EncodeMultisig refused a policy seating ONE key at TWO slots: %v.\n"+
			"That admission is this block's whole subject; if md refuses it now, there is "+
			"nothing here to engrave and this file is testing an unreachable shape", err)
	}
	return out
}

// s5DupSlotPremise measures the shape rather than assuming it: master B fills
// BOTH slots, and the two slots declare the SAME key at the SAME origin.
func s5DupSlotPremise(t *testing.T, md1 []string) (m bip39.Mnemonic, keys []md.ExpandedKey, slots []int) {
	t.Helper()
	m, err := bip39.ParseMnemonic(fixtureMasterB)
	if err != nil {
		t.Fatalf("ParseMnemonic(master B): %v", err)
	}
	_, keys, err = md.ExpandWalletPolicyChunks(md1)
	if err != nil {
		t.Fatalf("the duplicate-slot policy does not decode: %v", err)
	}
	if !allSlotsHaveXpub(keys) {
		t.Fatal("the duplicate-slot policy is not a FULL policy, so the supply flow's " +
			"own gate refuses it before this shape is reached")
	}
	slots = allUserSlots(m, "", s5Net, keys)
	if len(slots) != 2 {
		t.Fatalf("master B fills slots %v of the duplicate-slot policy, want [0 1]", slots)
	}
	if keys[slots[0]].Xpub != keys[slots[1]].Xpub {
		t.Fatalf("slots @%d and @%d declare DIFFERENT keys, so this fixture is the "+
			"reused-seed shape and not the identical-key one", slots[0], slots[1])
	}
	if keys[slots[0]].OriginPath.String() != keys[slots[1]].OriginPath.String() {
		t.Fatalf("slots @%d and @%d declare different origins (%s vs %s), so the two mk1s "+
			"would not be byte-identical and there is nothing to collapse",
			slots[0], slots[1], keys[slots[0]].OriginPath, keys[slots[1]].OriginPath)
	}
	return m, keys, slots
}

// TestSupplyTailCollapsesByteIdenticalPlates is the tail's own arm.
//
// The two legs mint byte-identical mk1s (asserted directly below, so this test
// cannot pass on a fixture whose plates merely happen to be one). ONE plate is
// cut, and the obligation list that leaves with the cards names ONE slot -- it
// has to, or the verify demands a plate that does not exist.
func TestSupplyTailCollapsesByteIdenticalPlates(t *testing.T) {
	md1 := s5DuplicateSlotMd1(t)
	m, keys, matched := s5DupSlotPremise(t, md1)

	// THE PLATES ARE BYTE-IDENTICAL, MEASURED at the deriver rather than assumed
	// from the keys agreeing.
	a, err := deriveMultisigLeg(m, "", s5Net, keys[matched[0]].OriginPath, md1, false)
	if err != nil {
		t.Fatalf("deriveMultisigLeg(@%d): %v", matched[0], err)
	}
	b, err := deriveMultisigLeg(m, "", s5Net, keys[matched[1]].OriginPath, md1, false)
	if err != nil {
		t.Fatalf("deriveMultisigLeg(@%d): %v", matched[1], err)
	}
	if strings.Join(a.MK1, "|") != strings.Join(b.MK1, "|") {
		t.Fatalf("the @%d and @%d legs mint DIFFERENT mk1s, so there is no identical pair "+
			"here and this test is not the collapse's subject:\n%v\n%v",
			matched[0], matched[1], a.MK1, b.MK1)
	}

	engraved, cards, err := supplyEngraveTail(m, "", s5Net, keys, matched, md1, true)
	if err != nil {
		t.Fatalf("supplyEngraveTail: %v", err)
	}
	if len(engraved) != 1 || engraved[0] != matched[0] {
		t.Fatalf("the tail's obligation list is %v, want [%d]. Two byte-identical plates "+
			"can never be read back as two (the gatherer keys mk1 cards on the "+
			"payload-derived chunk set id), so an obligation naming both is unsatisfiable "+
			"forever and the run can never be verified", engraved, matched[0])
	}
	mk1s, ms1s := 0, 0
	for _, c := range cards {
		switch c.kind {
		case cardMK1:
			mk1s++
		case cardMS1:
			ms1s++
		}
	}
	if mk1s != 1 {
		t.Errorf("the tail cut %d key plate(s) for one distinct key, want 1. Two "+
			"byte-identical plates carry identical information", mk1s)
	}
	if ms1s != 1 {
		t.Errorf("the tail cut %d seed plate(s) for ONE seed, want 1", ms1s)
	}
}

// TestSupplyDuplicateSlotVerifiesItsOwnOutput is the arm that matters: the
// engrave this run performs must be verifiable by the operator who performed it.
//
// It drives the REAL verify over the plates the tail actually minted, with the
// obligation list the tail actually returned, through the REAL gatherer -- which
// is where the second identical card disappears.
func TestSupplyDuplicateSlotVerifiesItsOwnOutput(t *testing.T) {
	md1 := s5DuplicateSlotMd1(t)
	m, keys, matched := s5DupSlotPremise(t, md1)

	engraved, cards, err := supplyEngraveTail(m, "", s5Net, keys, matched, md1, false)
	if err != nil {
		t.Fatalf("supplyEngraveTail: %v", err)
	}
	records := append([]string(nil), md1...)
	for _, c := range cards {
		if c.kind == cardMK1 {
			records = append(records, c.strings...)
		}
	}

	last := s5DriveVerifyTolerant(t, records, engraved, md1, fixtureMasterB)
	t.Logf("final screen: %q", last)
	if !uiContains(last, "Verify OK") {
		t.Fatalf("this run's OWN complete output could not be verified. Final screen: %q\n"+
			"The operator presented exactly the plates this run cut, for a policy the "+
			"device admitted and engraved. A verify that cannot be satisfied by the "+
			"honest readback is a permanent false RED on an hours-long engrave", last)
	}
}

// TestSupplyFlowAnnouncesTheCollapseBeforeTheFirstCut is §0.1 clause 3: the
// assumption is announced on the confirmation surface itself, upstream of steel.
//
// Two screens are asserted, and they carry different weight. The CENSUS note is
// the load-bearing one -- it is derived from the tail's own return, so it cannot
// claim a collapse that did not happen or miss one that did. The earlier
// multi-slot notice is the explanation, and it must stop asserting a DIFFERENT
// key per slot, which is false for this policy.
func TestSupplyFlowAnnouncesTheCollapseBeforeTheFirstCut(t *testing.T) {
	md1 := s5DuplicateSlotMd1(t)
	_, _, slots := s5DupSlotPremise(t, md1)

	d := s5DriveSupply(t, md1, fixtureMasterB, true /* full */)
	t.Logf("multi-slot notice: %q", d.announce)
	t.Logf("census screen: %q", d.census)
	t.Logf("first engrave screen: %q", d.engrave)

	if !uiContains(d.census, "hold only 1 distinct key between them") {
		t.Errorf("the census does not announce the collapse. The operator is about to cut "+
			"FEWER plates than this policy's %d matched slots suggest, and clause 3 puts "+
			"that on the confirmation surface itself, not in scrollback:\n%q",
			len(slots), d.census)
	}
	if !uiContains(d.census, "mk1 key") {
		t.Errorf("the census names no key plate at all:\n%q", d.census)
	}
	if uiContains(d.census, "mk1 key 2 of 2") {
		t.Errorf("the census still numbers TWO key plates for one distinct key:\n%q", d.census)
	}
	if d.announce == "" {
		t.Fatal("a seed matched at two slots produced no notice at all")
	}
	if uiContains(d.announce, "DIFFERENT key") {
		t.Errorf("the multi-slot notice still asserts that each slot holds a DIFFERENT "+
			"key:\n%q\nBoth slots of this policy declare the SAME key at the SAME key "+
			"path. This is the claims-a-shape-the-code-does-not-have defect the F-188 "+
			"commit message calls the tell", d.announce)
	}
	if !uiContains(d.announce, "SAME key") {
		t.Errorf("the multi-slot notice does not say what is actually true of this "+
			"policy:\n%q", d.announce)
	}
	// AND THE CARD SET, as the engrave counter states it: ms1 + one mk1 + md1.
	if !uiContains(d.engrave, "Card 1 of 3") {
		t.Errorf("the engrave set is not ms1 + 1 mk1 + md1. The first plate announces %q, "+
			"want \"Card 1 of 3\"", d.engrave)
	}
}
