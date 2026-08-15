package gui

import (
	"testing"

	"seedhammer.com/sysw"
)

// ─── S1 tests 1 and 2 — the whole-class accessor ─────────────────────────────
//
// `take` returns the FIRST record of a class, which is the right shape for a
// seed or a passphrase: there is one of those and a program consumes it once.
// A cosigner SET is not that shape. S1 delivers "the payload supplies the whole
// cosigner set", so the session needs an accessor that yields every record of a
// class, in payload record order — order is identity-bearing for a multisig
// policy (md/encode_multisig.go's ordering contract), so "in order" is part of
// the contract and not a convenience.

// S1 test 1: a session holding three ClassMDMK records yields all three through
// takeAll, and `take` still yields the first only.
//
// Both halves matter. The second is what stops takeAll from being written as a
// change to `take` — the seed and passphrase seams depend on first-only, and a
// takeAll that quietly redefined `take` would hand a program a different record
// than the one it consumed yesterday.
func TestSyswTakeAllYieldsEveryMDMKRecord(t *testing.T) {
	// Three DISTINCT ClassMDMK records. Distinctness is what makes "all three"
	// and "in order" checkable at all.
	a := "md1yqpqqxqq8xtwhw4xwn4qh"
	cards := cosignerCardFixtures(t, 2)
	b, c := cards[0][0], cards[1][0]
	for i, r := range []string{a, b, c} {
		if got := sysw.Classify(r); got != sysw.ClassMDMK {
			t.Fatalf("INCONCLUSIVE: record %d classifies as %v, not ClassMDMK, so "+
				"this test would be asserting over an empty set", i, got)
		}
	}
	// A non-MDMK record in the middle, so "takeAll returns everything" and
	// "takeAll returns everything OF THE CLASS" are distinguishable.
	s := sessionHolding(a, testSeedPhrase, b, c)

	got, ok := s.takeAll(sysw.ClassMDMK)
	if !ok {
		t.Fatal("takeAll refused a loaded, compared session")
	}
	want := []string{a, b, c}
	if len(got) != len(want) {
		t.Fatalf("takeAll returned %d records, want %d — the whole cosigner set is "+
			"the deliverable, and a short read silently builds a smaller policy",
			len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("record %d = %q, want %q — payload record order fixes @N "+
				"assignment, so a reorder here mints a different policy id",
				i, got[i], want[i])
		}
	}

	// `take` is unchanged: first match only.
	one, ok := s.take(sysw.ClassMDMK)
	if !ok {
		t.Fatal("take refused the same session")
	}
	if one != a {
		t.Errorf("take returned %q, want the FIRST record %q — the seed and "+
			"passphrase seams are built on first-only", one, a)
	}

	// A compared payload holding NONE of the class is not a refusal: it is an
	// empty answer, and the two must be distinguishable or the Build path cannot
	// tell "not authenticated" from "no cards on board" when it writes its
	// refusal text.
	empty, ok := sessionHolding(testSeedPhrase).takeAll(sysw.ClassMDMK)
	if !ok {
		t.Error("takeAll reported a REFUSAL for a compared payload that simply " +
			"holds no cards of the class; the caller cannot then name the right route")
	}
	if len(empty) != 0 {
		t.Errorf("takeAll returned %d records from a payload holding none", len(empty))
	}
}

// S1 test 2: takeAll refuses while !loaded || !compared, inheriting `take`'s
// guard. §12.2 [compared]: a record may not be handed to a program until the
// payload it came from has been authenticated by one of the two routes.
//
// Without the guard an UNAUTHENTICATED payload's cards reach the constructor,
// and with fingerprints omitted by default (multisigFpChoices' index 0) the
// review screen shows "@1 (no fp)" for a swapped card exactly as it does for
// the right one — so the swap is invisible on the one surface that exists to
// catch it. That is why this is stakes, not spelling, and stays a refusal.
//
// MUTATION-CHECKED (spec M-D): the sub-test below is the "delete the guard"
// mutant executed in-place — it calls the same record loop with the guard
// bypassed and proves the loop WOULD have returned the card. So a guard deleted
// from takeAll makes the assertions above fail rather than pass vacuously.
func TestSyswTakeAllRefusesBeforeCompared(t *testing.T) {
	cards := cosignerCardFixtures(t, 1)
	card := cards[0][0]

	t.Run("not loaded", func(t *testing.T) {
		var s syswSession // zero value: loaded == false
		if got, ok := s.takeAll(sysw.ClassMDMK); ok || len(got) != 0 {
			t.Errorf("takeAll on an unloaded session returned (%d records, ok=%v); "+
				"want (0, false)", len(got), ok)
		}
	})

	t.Run("loaded but not compared", func(t *testing.T) {
		s := &syswSession{}
		s.load(&sysw.Payload{Public: []string{card}}, [32]byte{9}, false, true,
			false /* compared */, true)
		if !s.loaded {
			t.Fatal("INCONCLUSIVE: the session did not load, so the refusal below " +
				"would fire on !loaded and prove nothing about [compared]")
		}
		if s.compared {
			t.Fatal("INCONCLUSIVE: the session is already compared")
		}
		if got, ok := s.takeAll(sysw.ClassMDMK); ok || len(got) != 0 {
			t.Errorf("takeAll handed %d record(s) (ok=%v) from a payload nobody has "+
				"authenticated; an unauthenticated cosigner card reaching the "+
				"constructor is a swapped key the review screen cannot surface",
				len(got), ok)
		}

		// The mutation, run rather than described: with the guard bypassed the
		// SAME record loop returns the card. So the refusal above is the guard's
		// doing, not an empty session's.
		var mutant []string
		for _, r := range s.records {
			if r.class == sysw.ClassMDMK {
				mutant = append(mutant, r.body)
			}
		}
		if len(mutant) != 1 {
			t.Fatalf("INCONCLUSIVE: the guard-free loop found %d ClassMDMK records, "+
				"want 1 — the refusal above may be passing because the session is "+
				"empty rather than because the guard fired", len(mutant))
		}

		// And once compared, the same session yields it.
		s.compared = true
		if got, ok := s.takeAll(sysw.ClassMDMK); !ok || len(got) != 1 {
			t.Errorf("takeAll still refused after [compared] was earned: (%d, %v)",
				len(got), ok)
		}
	})
}
