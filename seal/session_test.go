package seal

import "testing"

// §10.2.2's session lifecycle and §10.2.4's residency timer both need one
// question answered the same way: which records are SECRET, and is any of them
// still in memory.

// TestIsSecretIsTheSpecTable — §6.3 is explicit that an xpub and a wallet policy
// leak privacy but do not spend coins, so md1/mk1 are NOT secret wherever they
// travelled. §11.2 requires vector F's THREE ms1 records to be the ones offered
// first while its twelve mk1/md1 records are ordinary plates; widening IsSecret
// to ClassMDMK would offer fifteen.
func TestIsSecretIsTheSpecTable(t *testing.T) {
	for _, tc := range []struct {
		c    Classification
		want bool
	}{
		{ClassCodex32Secret, true},
		{ClassMnemonic, true},
		{ClassMDMK, false},
		{ClassDescriptor, false},
		{ClassAddress, false},
		{ClassDebugCommand, false},
		{ClassUnknown, false},
	} {
		if got := IsSecret(tc.c); got != tc.want {
			t.Errorf("IsSecret(%v) = %v, want %v", tc.c, got, tc.want)
		}
	}
}

// unlockedVector runs the whole pipeline over a vector and hands back the
// Payload with its secret section populated — the state §10.2.2's session
// begins in.
func unlockedVector(t *testing.T, name string) *Payload {
	t.Helper()
	v := vectorNamed(t, name)
	var o Opener
	p, err := o.Inspect(v.Blob(t))
	if err != nil {
		t.Fatalf("vector %s: Inspect: %v", name, err)
	}
	if err := o.UnlockWithKey(v.Blob(t), p, mustHex(t, *v.DerivedKeyHex)); err != nil {
		t.Fatalf("vector %s: UnlockWithKey: %v", name, err)
	}
	return p
}

// TestRecordsResidentGoesFalseOnlyWhenTheSECRETSAreGone — §10.2.4's condition.
//
// Vector F is ms1 x3 / mk1 x6 / md1 x6, so it discriminates the two halves of
// the predicate at once: wiping the twelve CARDS must leave it true (they are
// not secret and the ms1 records are still there), and wiping the three ms1
// records must take it false even though the twelve cards stay resident.
func TestRecordsResidentGoesFalseOnlyWhenTheSECRETSAreGone(t *testing.T) {
	p := unlockedVector(t, "F")
	if len(p.Secret) != 15 {
		t.Fatalf("premise broken: vector F must carry 15 secret-section records, got %d", len(p.Secret))
	}
	var secretAt, cardAt []int
	for i, r := range p.Secret {
		if IsSecret(r.Class) {
			secretAt = append(secretAt, i)
		} else {
			cardAt = append(cardAt, i)
		}
	}
	if len(secretAt) != 3 || len(cardAt) != 12 {
		t.Fatalf("premise broken: vector F must be 3 secrets + 12 cards, got %d + %d",
			len(secretAt), len(cardAt))
	}
	if !p.RecordsResident() {
		t.Fatal("a freshly unlocked payload reports no resident secrets")
	}

	// BOTH DIRECTIONS, and the second one is the only one that discriminates.
	//
	// Wiping the twelve CARDS first and finding it still true is satisfied by a
	// predicate that ignores IsSecret entirely — the three ms1 records are
	// still non-zero either way. Measured: a mutant that dropped the IsSecret
	// filter SURVIVED a version of this test that only walked that direction.
	// Wiping the three SECRETS first and finding it false while twelve non-zero
	// cards are still resident is what the filter actually buys.
	t.Run("secrets first, cards left resident", func(t *testing.T) {
		q := unlockedVector(t, "F")
		for _, i := range secretAt {
			q.WipeSecretAt(i)
		}
		live := 0
		for _, i := range cardAt {
			if !allZero(q.Secret[i].Record) {
				live++
			}
		}
		if live != len(cardAt) {
			t.Fatalf("premise broken: %d of %d cards should still be resident", live, len(cardAt))
		}
		if q.RecordsResident() {
			t.Errorf("every secret is zeroed but %d md1/mk1 cards are still resident, and "+
				"RecordsResident() is true; the predicate is not keyed on IsSecret", live)
		}
	})

	t.Run("cards first, then the secrets one at a time", func(t *testing.T) {
		q := unlockedVector(t, "F")
		for _, i := range cardAt {
			q.WipeSecretAt(i)
		}
		if !q.RecordsResident() {
			t.Error("wiping the twelve md1/mk1 cards took RecordsResident false")
		}
		// It stays true until the LAST secret goes — which is exactly the
		// statement B2b must design its timer against: what §10.2.2's early
		// wipe removes is one plate's worth of residency at a time, not the
		// session's.
		for n, i := range secretAt {
			q.WipeSecretAt(i)
			want := n < len(secretAt)-1
			if got := q.RecordsResident(); got != want {
				t.Errorf("after wiping %d of %d secrets, RecordsResident() = %v, want %v",
					n+1, len(secretAt), got, want)
			}
		}
	})
}

// TestWipeSecretAtZeroesExactlyOneRecord. §10.2.2 wipes per RECORD as each plate
// leaves the screen, so Payload.Wipe is too coarse for the offer loop — and a
// WipeSecretAt that zeroed more than its own index would destroy a plate the
// operator has not been offered yet.
func TestWipeSecretAtZeroesExactlyOneRecord(t *testing.T) {
	p := unlockedVector(t, "F")
	for i := range p.Secret {
		if allZero(p.Secret[i].Record) {
			t.Fatalf("premise broken: record %d is already zero", i)
		}
	}
	p.WipeSecretAt(1)
	for i := range p.Secret {
		zero := allZero(p.Secret[i].Record)
		if i == 1 && !zero {
			t.Errorf("WipeSecretAt(1) left record 1 non-zero: %q", p.Secret[i].Record)
		}
		if i != 1 && zero {
			t.Errorf("WipeSecretAt(1) also zeroed record %d", i)
		}
	}
	// Idempotent: §10.2.2's early wipe means the record is usually already zero
	// by the time unlockSecretPlate's deferred backstop runs.
	p.WipeSecretAt(1)
	if !allZero(p.Secret[1].Record) {
		t.Error("a second WipeSecretAt un-zeroed the record")
	}
}

// TestWipeSecretAtOutOfRangeIsANoOp. On a device a panic is a brick.
func TestWipeSecretAtOutOfRangeIsANoOp(t *testing.T) {
	p := unlockedVector(t, "A")
	n := len(p.Secret)
	for _, i := range []int{-1, n, n + 1, 1 << 20} {
		p.WipeSecretAt(i)
	}
	if allZero(p.Secret[0].Record) {
		t.Error("an out-of-range WipeSecretAt zeroed a real record")
	}
	// And on the zero value, where p.Secret is nil.
	var empty Payload
	empty.WipeSecretAt(0)
	if empty.RecordsResident() {
		t.Error("a payload with no secret section reports resident secrets")
	}
}
