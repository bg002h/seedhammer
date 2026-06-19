package gui

import (
	"reflect"
	"testing"

	"seedhammer.com/md"
)

// TestExtractSuppliedMd1 exercises the single-md1 supply contract (I-11): a
// gather producing exactly ONE cardMD1 yields its verbatim strings; 0 md1, >=2
// md1, or any cardMK1 present refuses. (A cardMS1 cannot arise from the gather
// path — ms1 is refused upstream at classify — so its clause is DEFENSIVE; the
// test for it documents intent but is not operator-reachable, n-1.)
func TestExtractSuppliedMd1(t *testing.T) {
	md1A := bundleCard{kind: cardMD1, label: "md1 descriptor", strings: []string{"md1aaa", "md1bbb"}}
	md1B := bundleCard{kind: cardMD1, label: "md1 descriptor", strings: []string{"md1ccc"}}
	mk1 := bundleCard{kind: cardMK1, label: "mk1 key", strings: []string{"mk1xxx"}}

	t.Run("exactly one md1 -> verbatim strings", func(t *testing.T) {
		got, ok := extractSuppliedMd1([]bundleCard{md1A})
		if !ok {
			t.Fatal("ok=false, want true for a single md1")
		}
		if !reflect.DeepEqual(got, []string{"md1aaa", "md1bbb"}) {
			t.Fatalf("strings = %v, want verbatim [md1aaa md1bbb]", got)
		}
	})
	t.Run("zero md1 -> refuse", func(t *testing.T) {
		if _, ok := extractSuppliedMd1(nil); ok {
			t.Fatal("ok=true for zero cards, want false")
		}
		if _, ok := extractSuppliedMd1([]bundleCard{mk1}); ok {
			t.Fatal("ok=true for mk1-only, want false (no md1)")
		}
	})
	t.Run("two md1 -> refuse", func(t *testing.T) {
		if _, ok := extractSuppliedMd1([]bundleCard{md1A, md1B}); ok {
			t.Fatal("ok=true for two md1, want false (ambiguous supply)")
		}
	})
	t.Run("any mk1 present -> refuse", func(t *testing.T) {
		if _, ok := extractSuppliedMd1([]bundleCard{md1A, mk1}); ok {
			t.Fatal("ok=true with a stray mk1, want false (polluted supply)")
		}
	})
	t.Run("defensive cardMS1 -> refuse (n-1, not gather-reachable)", func(t *testing.T) {
		ms1 := bundleCard{kind: cardMS1, label: "ms1", strings: []string{"ms1zzz"}}
		if _, ok := extractSuppliedMd1([]bundleCard{md1A, ms1}); ok {
			t.Fatal("ok=true with a cardMS1, want false (defensive)")
		}
	})
}

// TestAllSlotsHaveXpub: the full-policy gate (I-3). The full-policy fixture
// passes; a template-only multisig (the wsh_sortedmulti md golden, which carries
// NO pubkeys) refuses; an empty key set refuses.
func TestAllSlotsHaveXpub(t *testing.T) {
	// Full-policy fixture -> all slots xpub-present.
	chunks := suppliedMultisigMd1(t)
	_, keys, err := md.ExpandWalletPolicyChunks(chunks)
	if err != nil {
		t.Fatalf("ExpandWalletPolicyChunks(full): %v", err)
	}
	if !allSlotsHaveXpub(keys) {
		t.Fatal("full-policy fixture rejected by the gate")
	}

	// Template-only: a slot set with XpubPresent=false must refuse.
	tmplOnly := []md.ExpandedKey{
		{Index: 0, XpubPresent: true},
		{Index: 1, XpubPresent: false},
	}
	if allSlotsHaveXpub(tmplOnly) {
		t.Fatal("a missing-xpub slot passed the gate, want refuse")
	}
	if allSlotsHaveXpub(nil) {
		t.Fatal("empty key set passed the gate, want refuse")
	}
}
