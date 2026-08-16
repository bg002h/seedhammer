package gui

import (
	"testing"

	"seedhammer.com/md"
	"seedhammer.com/mk"
)

// ─── I-1: a `both` slot's engraved origin is unpinned ────────────────────────
//
// Nothing in the suite distinguished "a `both` slot's mk1 is engraved at the
// CARD's declared origin" (correct, SPEC M-B) from "...at derivedSlotOrigin(
// script, s.Account)" (wrong). Swapping one for the other left the whole gui +
// oracle suite GREEN -- measured, --- PASS x810, --- FAIL x0 -- so the
// correctness of the shipped line was unobserved.
//
// THE CAUSE IS MEASURABLE. All seven buildEngraveTail call sites in the test
// tree use s5TraceB, whose three held slots are ALL slotFromSeed; every
// slotFromBoth fixture in the tree stops at buildSlotGate and never reaches the
// tail. Contrast the `derived` arm, which IS pinned (three tests), and the
// duplicate guard the tail relies on, which IS pinned (nine).
//
// WHAT THE MUTATION PRODUCES, with the delivered payload's own card: an operator
// holds @0, answers "my key is on a card", and asserts roster card A@1 declaring
// m/48h/0h/1h/2h. The gate PASSES -- S5 deleted S2's foreign-origin refusal and
// the gate derives at the card's own origin -- and the assembled policy declares
// m/48h/0h/1h/2h at @0. buildSlotSources never sets Account on a `both` slot, so
// it is always 0 and derivedSlotOrigin(wsh, 0) is always m/48h/0h/0h/2h. The same
// input then engraves a plate declaring m/48h/0h/0h/2h carrying a DIFFERENT key
// from the one the policy holds at @0: a key plate asserting membership of a
// wallet whose @0 is not that key, at a path the policy never declares. The only
// downstream check sits behind a "Verify now / Skip" picker, so on Skip the wrong
// plate is the operator's only record of a slot they can no longer prove.
//
// NO PRODUCTION CHANGE IS REQUIRED. The shipped line is correct; what was missing
// is the thing that holds it there.

// s5BothSlotBuild assembles a 2-of-2 wsh policy whose @0 is a `both` slot
// against master A's SECOND-account card, and runs it through the real gate and
// the real tail.
//
// Card A@1 is the fixture that separates the two origins: it declares
// m/48'/0'/1'/2' and carries the key master A really derives there, so a tail
// stamping derivedSlotOrigin(wsh, Account=0) produces a different path AND a
// different key.
func s5BothSlotBuild(t *testing.T) (assembled []string, cards []mk.Card, cardsOut []bundleCard) {
	t.Helper()
	reg := gateRegistry(t, fixtureMasterA, "")
	a1 := gateCard(t, 3) // master A at m/48'/0'/1'/2'
	b0 := gateCard(t, 1) // master B at the shared origin: the other cosigner

	// THE PREMISE, MEASURED: the card's declared origin is NOT the account-0 path
	// the mutation would stamp. Without this the two arms are indistinguishable
	// and the test proves nothing.
	derived := derivedSlotOrigin(md.MultisigWsh, 0).String()
	if a1.Path == derived {
		t.Fatalf("card A@1 declares %s, which IS derivedSlotOrigin(wsh, 0); this fixture "+
			"cannot tell the card's origin from the derived one", a1.Path)
	}

	p := buildPolicyParams{
		Script: md.MultisigWsh, N: 2, K: 2,
		SelfSlots: []int{0}, SelfFromCard: true,
	}
	cards = []mk.Card{a1, b0}
	sources := buildSlotSources(p, []int{0}, []int{0, 1}, reg)
	if sources[0].Kind != slotFromBoth {
		t.Fatalf("@0 is %v, want slotFromBoth: this file's whole subject is the tail's "+
			"`both` arm, and the projection has to produce it", sources[0].Kind)
	}
	if sources[0].Account != 0 {
		t.Fatalf("@0's Account is %d; buildSlotSources never sets it on a `both` slot, so "+
			"the mutation's derivedSlotOrigin(script, Account) is the account-0 path and "+
			"this fixture depends on that", sources[0].Account)
	}

	// REACHABILITY, ASSERTED: the gate must PROCEED, or the tail is never called
	// in production for this shape and the test below is about an unreachable
	// line. TestGateDerivesAtTheCardsOwnOrigin proves the same fixture passes.
	self, err := buildSelfKeys(sources, p.Script, reg, gateNet)
	if err != nil {
		t.Fatalf("buildSelfKeys: %v", err)
	}
	if len(self) != 0 {
		t.Fatalf("a `both` slot derived %d self key(s), want 0: SPEC M-B makes the CARD "+
			"authoritative there and step (4b) skips it", len(self))
	}
	if _, gerr := buildSlotGate(sources, p.Script, reg, cards, gateNet); gerr != nil {
		t.Fatalf("the gate refused the honest `both` fixture: %v. If it cannot proceed, "+
			"the tail's `both` arm is unreachable and this test is vacuous", gerr)
	}

	assembled, _, _, err = assembleBuildPolicy(p, self, cards)
	if err != nil {
		t.Fatalf("the `both` 2-of-2 did not assemble: %v", err)
	}
	_, _, cardsOut, err = buildEngraveTail(sources, p.Script, reg, gateNet, cards, assembled, false)
	if err != nil {
		t.Fatalf("buildEngraveTail: %v", err)
	}
	return assembled, cards, cardsOut
}

// TestBuildTailEngravesABothSlotAtTheCardsOwnOrigin is the missing pin.
func TestBuildTailEngravesABothSlotAtTheCardsOwnOrigin(t *testing.T) {
	assembled, cards, cardsOut := s5BothSlotBuild(t)

	var mk1s [][]string
	for _, c := range cardsOut {
		if c.kind == cardMK1 {
			mk1s = append(mk1s, c.strings)
		}
	}
	if len(mk1s) != 1 {
		t.Fatalf("the tail cut %d key plate(s) for a build holding ONE slot, want 1", len(mk1s))
	}
	leg, err := mk.Decode(mk1s[0])
	if err != nil {
		t.Fatalf("the @0 leg's mk1 does not decode: %v", err)
	}

	// (1) THE ORIGIN ON THE PLATE IS THE CARD'S OWN.
	if leg.Path != cards[0].Path {
		t.Errorf("the @0 leg's mk1 declares origin %s, but the card the operator asserted "+
			"is theirs declares %s.\nSPEC M-B makes the CARD authoritative in a `both` "+
			"slot: the plate must carry the path the policy declares, not a path derived "+
			"from an Account field that `both` slots never set", leg.Path, cards[0].Path)
	}

	// (2) AND THE KEY ON THE PLATE IS THE KEY THE POLICY HOLDS AT @0. The origin
	// alone is not enough: a plate can declare the right path and carry the wrong
	// key, which is the shape that makes the mutation lose funds rather than
	// merely mislabel a plate.
	_, keys, err := md.ExpandWalletPolicyChunks(assembled)
	if err != nil {
		t.Fatalf("the assembled policy does not decode: %v", err)
	}
	cc, pk, _, err := decodeXpubBytes(leg.Xpub)
	if err != nil {
		t.Fatalf("the leg's xpub does not decode: %v", err)
	}
	var got [65]byte
	copy(got[0:32], cc[:])
	copy(got[32:65], pk[:])
	if got != keys[0].Xpub {
		t.Fatalf("the @0 leg's mk1 carries a key the policy does NOT hold at @0.\n" +
			"The plate would assert membership of a wallet whose @0 is a different key, " +
			"and the only check downstream sits behind a Verify/Skip picker -- so on Skip " +
			"this plate is the operator's only record of a slot they can no longer prove")
	}
	// And the policy really does declare the card's origin at @0, so (1) and (2)
	// are about the same slot rather than agreeing by accident.
	if keys[0].OriginPath.String() != cards[0].Path {
		t.Errorf("the assembled policy declares %s at @0 while the card declares %s",
			keys[0].OriginPath, cards[0].Path)
	}
}
