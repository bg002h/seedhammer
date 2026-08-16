package gui

import (
	"errors"
	"strings"
	"testing"

	"github.com/btcsuite/btcd/chaincfg/v2"
	"seedhammer.com/bip39"
	"seedhammer.com/md"
	"seedhammer.com/mk"
)

// ─── S5: the verify covers EVERY leg ─────────────────────────────────────────
//
// multisigVerifyFlow took ONE bundle and recovered the operator's origin through
// findUserSlot, which returns the FIRST slot a seed matches -- so it structurally
// could not verify legs 2..n. Trace B engraves three key plates; the shipped
// verify read back one, compared one, and showed "Verify OK".
//
// A verify that checks one of three plates and reports success is a FALSE GREEN
// on the operator's only readback, and it is the last check that stands between
// a mis-cut plate and funds. So the comparator is a bijection: every re-derived
// leg must find its plate, and every plate must be claimed by a leg.

// s5TraceBEngraved assembles Trace B and runs the production engrave tail,
// returning the md1 and the plates that came out of it.
//
// It goes through assembleBuildPolicy + buildEngraveTail rather than
// hand-building a plate set, because the thing under test is whether the verify
// covers what the ENGRAVE actually produced.
func s5TraceBEngraved(t *testing.T, full bool) (md1 []string, mk1Plates [][]string, ms1Plates []string) {
	t.Helper()
	p, reg, sources, self, cards := s5TraceB(t)
	out, _, _, err := assembleBuildPolicy(p, self, cards)
	if err != nil {
		t.Fatalf("Trace B did not assemble: %v", err)
	}
	_, _, cardsOut, err := buildEngraveTail(sources, p.Script, reg, s5Net, cards, out, full)
	if err != nil {
		t.Fatalf("buildEngraveTail(full=%v): %v", full, err)
	}
	for _, c := range cardsOut {
		switch c.kind {
		case cardMK1:
			mk1Plates = append(mk1Plates, append([]string(nil), c.strings...))
		case cardMS1:
			if len(c.strings) != 1 {
				t.Fatalf("an ms1 plate carries %d string(s), want 1", len(c.strings))
			}
			ms1Plates = append(ms1Plates, c.strings[0])
		}
	}
	if len(mk1Plates) != 3 {
		t.Fatalf("Trace B engraved %d mk1 plate(s), want 3", len(mk1Plates))
	}
	return out, mk1Plates, ms1Plates
}

// s5ReDerivedLegs is what multisigVerifyFlow does for ONE re-typed seed: find
// every slot of the read-back policy that seed accounts for, and re-derive the
// leg at that slot's own origin.
func s5ReDerivedLegs(t *testing.T, phrase string, md1 []string, ms1Readback string, full bool) []verifyLeg {
	t.Helper()
	m, err := bip39.ParseMnemonic(phrase)
	if err != nil {
		t.Fatalf("ParseMnemonic(%.20s...): %v", phrase, err)
	}
	_, keys, err := md.ExpandWalletPolicyChunks(md1)
	if err != nil {
		t.Fatalf("ExpandWalletPolicyChunks: %v", err)
	}
	slots := allUserSlots(m, "", &chaincfg.MainNetParams, keys)
	if len(slots) == 0 {
		t.Fatalf("%.20s... matches no slot of the read-back policy", phrase)
	}
	out := make([]verifyLeg, 0, len(slots))
	for _, s := range slots {
		b, derr := deriveMultisigLeg(m, "", &chaincfg.MainNetParams, keys[s].OriginPath, md1, full)
		if derr != nil {
			t.Fatalf("deriveMultisigLeg(@%d): %v", s, derr)
		}
		out = append(out, verifyLeg{Slot: s, B: b, MS1Readback: ms1Readback})
	}
	return out
}

// s5PlateFor finds the read-back plate that carries a leg's key, by decoding
// both rather than by trusting the engrave order.
func s5PlateFor(t *testing.T, plates [][]string, l verifyLeg) int {
	t.Helper()
	want, err := mk.Decode(l.B.MK1)
	if err != nil {
		t.Fatalf("the @%d leg's own mk1 does not decode: %v", l.Slot, err)
	}
	for i, p := range plates {
		got, derr := mk.Decode(p)
		if derr != nil {
			continue
		}
		if got.Xpub == want.Xpub {
			return i
		}
	}
	t.Fatalf("no read-back plate carries the @%d leg's key", l.Slot)
	return -1
}

// TestVerifyCoversEveryLeg is this block's central claim.
//
// It runs a PASS arm and then corrupts the plate for EACH leg in turn --
// including the last, which is the "not just the first" half. A verify that
// checks legs[0] and stops passes the first corruption arm by accident and
// fails every other one.
func TestVerifyCoversEveryLeg(t *testing.T) {
	md1, plates, _ := s5TraceBEngraved(t, false) // watch-only: no ms1 on either side
	legs := s5ReDerivedLegs(t, fixtureMasterA, md1, "", false)
	legs = append(legs, s5ReDerivedLegs(t, fixtureMasterB, md1, "", false)...)
	if len(legs) != 3 {
		t.Fatalf("Trace B re-derived %d leg(s) from masters A and B, want 3. The verify "+
			"cannot cover plates it never derives a leg for", len(legs))
	}

	t.Run("every leg matched by its own plate PASSES", func(t *testing.T) {
		if err := verifyMultisigLegs(legs, plates, md1); err != nil {
			t.Fatalf("an honest three-plate readback FAILED: %v", err)
		}
	})

	// A foreign-but-VALID mk1: it decodes fine and binds to a different policy,
	// so a comparator that merely counts plates or checks decodability accepts it.
	m, err := bip39.ParseMnemonic(fixtureMasterA)
	if err != nil {
		t.Fatalf("ParseMnemonic: %v", err)
	}
	foreign, _, _, _, err := deriveSingleSigBundle(m, "", &chaincfg.MainNetParams,
		singleSigPath(44), md.ScriptPkh)
	if err != nil {
		t.Fatalf("deriving a foreign mk1: %v", err)
	}

	for _, l := range legs {
		t.Run("a WRONG plate for @"+string(rune('0'+l.Slot))+" FAILS", func(t *testing.T) {
			bad := make([][]string, len(plates))
			copy(bad, plates)
			bad[s5PlateFor(t, plates, l)] = append([]string(nil), foreign.MK1...)
			err := verifyMultisigLegs(legs, bad, md1)
			if err == nil {
				t.Fatalf("a readback in which @%d's plate was replaced by another wallet's "+
					"key card PASSED. That is a \"Verify OK\" over a plate the operator "+
					"cannot spend with", l.Slot)
			}
			want := "@" + string(rune('0'+l.Slot))
			if !strings.Contains(err.Error(), want) {
				t.Errorf("the failure %q does not name %s, so the operator cannot tell "+
					"WHICH plate to re-cut", err, want)
			}
		})
	}

	// A plate that decodes, carries the right KEY, and lies about its origin. It
	// pairs with its leg and must then fail the field comparison -- the leg of
	// the check that a pairing-only comparator would skip.
	t.Run("a plate carrying the right key at the wrong origin FAILS", func(t *testing.T) {
		last := legs[len(legs)-1]
		idx := s5PlateFor(t, plates, last)
		card, derr := mk.Decode(plates[idx])
		if derr != nil {
			t.Fatalf("decoding plate %d: %v", idx, derr)
		}
		orig := card.Path
		card.Path = "m/48h/0h/9h/2h"
		if card.Path == orig {
			t.Fatalf("the fixture plate already declares %s, so this arm changes nothing", orig)
		}
		relabelled, eerr := mk.Encode(card)
		if eerr != nil {
			t.Fatalf("re-encoding plate %d: %v", idx, eerr)
		}
		bad := make([][]string, len(plates))
		copy(bad, plates)
		bad[idx] = relabelled
		if err := verifyMultisigLegs(legs, bad, md1); err == nil {
			t.Fatalf("a plate declaring %s while carrying the key from %s PASSED. Its mk1 "+
				"asserts membership in a wallet at a path its own key is not at",
				card.Path, orig)
		}
	})

	t.Run("a plate no leg claims FAILS", func(t *testing.T) {
		extra := append([][]string(nil), plates...)
		extra = append(extra, append([]string(nil), foreign.MK1...))
		if err := verifyMultisigLegs(legs, extra, md1); err == nil {
			t.Fatal("a readback carrying a fourth key plate PASSED. An unclaimed plate is " +
				"a plate in the operator's hands that this policy has no slot for")
		}
	})

	t.Run("a leg with no plate at all FAILS", func(t *testing.T) {
		short := append([][]string(nil), plates[:len(plates)-1]...)
		err := verifyMultisigLegs(legs, short, md1)
		if err == nil {
			t.Fatal("a readback missing one of the three key plates PASSED. That is the " +
				"shipped defect verbatim: check what was shown, call the rest verified")
		}
	})

	t.Run("no legs at all FAILS rather than vacuously passing", func(t *testing.T) {
		if err := verifyMultisigLegs(nil, plates, md1); err == nil {
			t.Fatal("verifying ZERO legs returned nil. A comparator whose empty case is a " +
				"PASS reports success for a readback it never looked at")
		}
	})
}

// TestVerifyCoversEveryMastersSecret is the ms1 half, and it is the one that is
// invisible in a watch-only run: Trace B in FULL mode engraves TWO seed plates,
// one per master, and each must be checked against the master it claims.
//
// The swap arm is the point. Two ms1 plates were cut; if the comparator checks
// either one against whichever leg it happens to hold, a backup carrying master
// A's seed twice verifies clean -- and master B, which k=3 needs, is gone.
func TestVerifyCoversEveryMastersSecret(t *testing.T) {
	md1, plates, ms1s := s5TraceBEngraved(t, true)
	if len(ms1s) != 2 {
		t.Fatalf("full Trace B engraved %d ms1 plate(s), want 2", len(ms1s))
	}
	entA := s5Entropy(t, fixtureMasterA)
	entB := s5Entropy(t, fixtureMasterB)
	ms1A, ms1B := "", ""
	for _, s := range ms1s {
		switch s5DecodeMS1(t, s) {
		case entA:
			ms1A = s
		case entB:
			ms1B = s
		}
	}
	if ms1A == "" || ms1B == "" {
		t.Fatalf("the engraved ms1 plates do not carry both masters (A found: %v, B found: %v)",
			ms1A != "", ms1B != "")
	}

	t.Run("each master's seed plate matched to its own legs PASSES", func(t *testing.T) {
		legs := s5ReDerivedLegs(t, fixtureMasterA, md1, ms1A, true)
		legs = append(legs, s5ReDerivedLegs(t, fixtureMasterB, md1, ms1B, true)...)
		if err := verifyMultisigLegs(legs, plates, md1); err != nil {
			t.Fatalf("an honest full readback FAILED: %v", err)
		}
	})

	t.Run("master B's legs against master A's seed plate FAILS", func(t *testing.T) {
		legs := s5ReDerivedLegs(t, fixtureMasterA, md1, ms1A, true)
		legs = append(legs, s5ReDerivedLegs(t, fixtureMasterB, md1, ms1A, true)...)
		if err := verifyMultisigLegs(legs, plates, md1); err == nil {
			t.Fatal("a \"Full\" backup whose two seed plates both carry master A PASSED. " +
				"Master B is unrecoverable and k=3 cannot be met")
		}
	})
}

// TestAllUserSlotsFindsEverySlotOneSeedFills is the recovery half of the fix.
// findUserSlot returns the FIRST match and that is what made the shipped verify
// structurally single-leg: master A fills @0 AND @1 of Trace B, and a verify
// built on "the first slot" can never re-derive the second.
func TestAllUserSlotsFindsEverySlotOneSeedFills(t *testing.T) {
	md1, _, _ := s5TraceBEngraved(t, false)
	_, keys, err := md.ExpandWalletPolicyChunks(md1)
	if err != nil {
		t.Fatalf("ExpandWalletPolicyChunks: %v", err)
	}
	mA, err := bip39.ParseMnemonic(fixtureMasterA)
	if err != nil {
		t.Fatalf("ParseMnemonic: %v", err)
	}
	got := allUserSlots(mA, "", &chaincfg.MainNetParams, keys)
	want := []int{0, 1}
	if len(got) != len(want) {
		t.Fatalf("master A accounts for slots %v of Trace B, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("master A accounts for slots %v, want %v", got, want)
		}
	}
	// findUserSlot's own contract is UNCHANGED by the refactor: first match.
	first, _, ok := findUserSlot(mA, "", &chaincfg.MainNetParams, keys)
	if !ok || first != want[0] {
		t.Fatalf("findUserSlot returned (%d, %v), want (%d, true)", first, ok, want[0])
	}
}

// TestVerifyPairsByKeyNotByOrigin pins the pairing RULE, which every other test
// in this file is blind to.
//
// verifyClaimPlate pairs a leg to its plate by XPUB. The obvious alternative is
// the ORIGIN PATH, and on this build the two are indistinguishable: Trace B's @0
// and @2 both declare m/48h/0h/0h/2h -- one account of master A, one of master B
// -- so the path is not unique across masters, but the claim loop is greedy over
// UNCLAIMED plates, so while the plates arrive in the SAME ORDER as the legs the
// collision resolves itself and both rules agree.
//
// Measured, not supposed: switching verifyClaimPlate to compare Path instead of
// Xpub left this whole file GREEN.
//
//	MUT-PAIR-RAN want-path m/48h/0h/0h/2h got-path m/48h/0h/0h/2h xpub-equal true
//	MUT-PAIR-RAN want-path m/48h/0h/1h/2h got-path m/48h/0h/1h/2h xpub-equal true
//	MUT-PAIR-RAN want-path m/48h/0h/0h/2h got-path m/48h/0h/0h/2h xpub-equal true
//	PAIR_MUTATED_EXIT=0
//
// A readback carries no such ordering guarantee: the operator presents plates in
// whatever order they pick them up, and nothing binds that to the engrave order.
// So this drives the same honest set REVERSED. Under key-pairing it passes;
// under path-pairing @0's leg claims master B's plate and an honest readback
// FAILS.
func TestVerifyPairsByKeyNotByOrigin(t *testing.T) {
	md1, plates, _ := s5TraceBEngraved(t, false) // watch-only: no ms1 on either side
	legs := s5ReDerivedLegs(t, fixtureMasterA, md1, "", false)
	legs = append(legs, s5ReDerivedLegs(t, fixtureMasterB, md1, "", false)...)
	if len(legs) != 3 {
		t.Fatalf("Trace B re-derived %d leg(s), want 3", len(legs))
	}

	// THE PREMISE, ASSERTED RATHER THAN ASSUMED. This test only distinguishes the
	// two rules while two legs share an origin. If the account numbering ever
	// stops colliding, the test must say it has stopped testing its subject
	// rather than keep reporting green.
	byPath := map[string][]int{}
	for _, l := range legs {
		c, err := mk.Decode(l.B.MK1)
		if err != nil {
			t.Fatalf("the @%d leg's own mk1 does not decode: %v", l.Slot, err)
		}
		byPath[c.Path] = append(byPath[c.Path], l.Slot)
	}
	shared := false
	for p, slots := range byPath {
		if len(slots) > 1 {
			shared = true
			t.Logf("origin %s is declared by slots %v -- the collision this test needs", p, slots)
		}
	}
	if !shared {
		t.Fatalf("no two legs of Trace B share an origin path (%v), so this test can no longer "+
			"tell key-pairing from path-pairing and is asserting nothing", byPath)
	}

	rev := make([][]string, len(plates))
	for i, p := range plates {
		rev[len(plates)-1-i] = p
	}
	if err := verifyMultisigLegs(legs, rev, md1); err != nil {
		t.Fatalf("an honest three-plate readback presented in REVERSE order FAILED: %v. Every "+
			"plate carries the key some leg derived; only the order changed, and a readback "+
			"has no order to promise", err)
	}
}

// TestVerifyFreshSlotsIsTheEngraversList replaces the I-1 guard that used to sit
// here (TestReusedKeyVerifiesAgainstItsONEPlate).
//
// That test pinned a same-key dedupe, and it pinned nothing: its first assertion
// matched a leg against ITSELF -- true of any comparison that is reflexive -- and
// the shape it claimed to model does not exist. A policy holding one seed at
// several slots holds it at several ORIGINS, so the keys differ and the dedupe
// never fired. Deleting the whole mechanism left `go test ./gui/` green (exit 0,
// measured), which is the definition of inert.
//
// The rule that replaces it is not about keys at all: only the ENGRAVER knows
// which slots it cut a plate for, so it says so, and the derive loop proves those
// and no others. The two directions are pinned together here because a fix to
// either alone is wrong -- the manufactured leg must go, and a plate that was
// genuinely not read back must still FAIL.
func TestVerifyFreshSlotsIsTheEngraversList(t *testing.T) {
	md1, plates, _ := s5TraceBEngraved(t, false)
	m, err := bip39.ParseMnemonic(fixtureMasterA)
	if err != nil {
		t.Fatalf("ParseMnemonic: %v", err)
	}
	_, keys, err := md.ExpandWalletPolicyChunks(md1)
	if err != nil {
		t.Fatalf("ExpandWalletPolicyChunks: %v", err)
	}
	filled := allUserSlots(m, "", &chaincfg.MainNetParams, keys)
	if len(filled) != 2 || filled[0] != 0 || filled[1] != 1 {
		t.Fatalf("master A fills slots %v of Trace B, want [0 1]; this test's subject is a "+
			"seed that fills MORE slots than the engrave cut plates for", filled)
	}

	t.Run("a seed filling two slots proves only the ONE this run engraved", func(t *testing.T) {
		fresh, err := verifyFreshSlots([]int{0}, filled, map[int]bool{})
		if err != nil {
			t.Fatalf("verifyFreshSlots: %v", err)
		}
		if len(fresh) != 1 || fresh[0] != 0 {
			t.Fatalf("a supply engrave of @0 owes %v legs, want [0]. Every extra leg is one "+
				"this run cut no plate for, and its verify FAILS over correct steel", fresh)
		}
	})

	t.Run("a slot an earlier seed already covered is not re-derived", func(t *testing.T) {
		fresh, err := verifyFreshSlots([]int{0, 1}, filled, map[int]bool{0: true})
		if err != nil {
			t.Fatalf("verifyFreshSlots: %v", err)
		}
		if len(fresh) != 1 || fresh[0] != 1 {
			t.Fatalf("got %v, want [1]: a covered slot re-derived is a second leg racing the "+
				"first for one plate", fresh)
		}
	})

	t.Run("an expected slot the seed does NOT fill is never manufactured", func(t *testing.T) {
		// @2 is master B's. Master A cannot prove it, and inventing a leg for it
		// here would report the wrong seed as a plate failure.
		fresh, err := verifyFreshSlots([]int{0, 1, 2}, filled, map[int]bool{})
		if err != nil {
			t.Fatalf("verifyFreshSlots: %v", err)
		}
		if len(fresh) != 2 || fresh[0] != 0 || fresh[1] != 1 {
			t.Fatalf("got %v, want [0 1]: master A fills @0 and @1 only", fresh)
		}
	})

	t.Run("an empty expectation is REFUSED, not vacuously satisfied", func(t *testing.T) {
		fresh, err := verifyFreshSlots(nil, filled, map[int]bool{})
		if err == nil {
			t.Fatalf("an empty engraved-slot set returned %v and no error. A verify with "+
				"nothing to prove reports success over a readback it never looked at", fresh)
		}
		if !errors.Is(err, errVerifyNoExpectedSlots) {
			t.Errorf("got %v, want errVerifyNoExpectedSlots", err)
		}
	})

	t.Run("one leg verifies against its one plate", func(t *testing.T) {
		legs := s5ReDerivedLegs(t, fixtureMasterA, md1, "", false)
		one := legs[:1]
		idx := s5PlateFor(t, plates, one[0])
		if err := verifyMultisigLegs(one, [][]string{plates[idx]}, md1); err != nil {
			t.Fatalf("a single-leg engrave's own plate FAILED its verify: %v. This is the "+
				"supply path's shipped shape", err)
		}
	})

	t.Run("a MISSING plate still FAILS", func(t *testing.T) {
		// Three legs the engrave DID cut plates for, one plate not read back.
		// Nothing about the expectation excuses it: the bijection is untouched.
		legs := s5ReDerivedLegs(t, fixtureMasterA, md1, "", false)
		legs = append(legs, s5ReDerivedLegs(t, fixtureMasterB, md1, "", false)...)
		short := append([][]string(nil), plates[:len(plates)-1]...)
		if err := verifyMultisigLegs(legs, short, md1); err == nil {
			t.Fatal("three legs against two plates PASSED. A plate that was not read back " +
				"is a slot the operator cannot prove membership of")
		}
	})

	t.Run("ZERO plates FAILS", func(t *testing.T) {
		legs := s5ReDerivedLegs(t, fixtureMasterA, md1, "", false)
		err := verifyMultisigLegs(legs[:1], nil, md1)
		if err == nil {
			t.Fatal("a leg against an EMPTY plate set PASSED. Nothing was read back, so " +
				"nothing was compared")
		}
		if !strings.Contains(err.Error(), "@0") {
			t.Errorf("the failure %q does not name @0, so the operator cannot tell WHICH "+
				"plate to present", err)
		}
	})
}
