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

// extractReadbackMd1AndMk1s (verify-cluster H1): reads back the wallet-policy
// md1 AND EVERY operator mk1 plate from the gathered card set. Exactly one md1,
// at least one mk1, no ms1. (extractSuppliedMd1 is the SUPPLY filter — one md1,
// zero key cards — and must stay unchanged: it has two callers including the
// live engrave flow.)
//
// S5 CHANGED ONE ROW OF THIS TABLE, and it is the row that matters: two mk1s
// used to be "ambiguous, refuse", and now they are the ordinary shape of a build
// holding two slots. The refusal was what made the verify single-leg -- a
// three-plate build whose readback filter drops plates 2 and 3 can only ever
// check plate 1, whatever the comparator downstream does. Every other row is
// unchanged.
func TestExtractReadbackMd1AndMk1s(t *testing.T) {
	mk1 := bundleCard{kind: cardMK1, strings: []string{"mk1a", "mk1b"}}
	mk1b := bundleCard{kind: cardMK1, strings: []string{"mk1c", "mk1d"}}
	md1 := bundleCard{kind: cardMD1, strings: []string{"md1a", "md1b", "md1c"}}

	t.Run("one mk1 + one md1 → ok", func(t *testing.T) {
		gotMd1, gotMk1s, ok := extractReadbackMd1AndMk1s([]bundleCard{mk1, md1})
		if !ok {
			t.Fatal("valid mk1+md1 set rejected")
		}
		if !equalStringSlice(gotMd1, md1.strings) {
			t.Fatalf("md1 = %v, want %v", gotMd1, md1.strings)
		}
		if len(gotMk1s) != 1 || !equalStringSlice(gotMk1s[0], mk1.strings) {
			t.Fatalf("mk1s = %v, want [%v]", gotMk1s, mk1.strings)
		}
	})
	t.Run("SEVERAL mk1 + one md1 → ok, in gather order", func(t *testing.T) {
		gotMd1, gotMk1s, ok := extractReadbackMd1AndMk1s([]bundleCard{mk1, md1, mk1b})
		if !ok {
			t.Fatal("a two-plate readback was REFUSED. That refusal is what made the " +
				"verify single-leg: plates it drops can never be compared")
		}
		if !equalStringSlice(gotMd1, md1.strings) {
			t.Fatalf("md1 = %v, want %v", gotMd1, md1.strings)
		}
		if len(gotMk1s) != 2 {
			t.Fatalf("got %d mk1 plate(s), want 2", len(gotMk1s))
		}
		if !equalStringSlice(gotMk1s[0], mk1.strings) || !equalStringSlice(gotMk1s[1], mk1b.strings) {
			t.Fatalf("mk1s = %v, want [%v %v] in gather order", gotMk1s, mk1.strings, mk1b.strings)
		}
	})
	t.Run("missing mk1 → not ok", func(t *testing.T) {
		if _, _, ok := extractReadbackMd1AndMk1s([]bundleCard{md1}); ok {
			t.Fatal("set with no mk1 accepted")
		}
	})
	t.Run("missing md1 → not ok", func(t *testing.T) {
		if _, _, ok := extractReadbackMd1AndMk1s([]bundleCard{mk1}); ok {
			t.Fatal("set with no md1 accepted")
		}
	})
	t.Run("two md1 → ambiguous, not ok", func(t *testing.T) {
		if _, _, ok := extractReadbackMd1AndMk1s([]bundleCard{mk1, md1, md1}); ok {
			t.Fatal("ambiguous (two md1) set accepted")
		}
	})
	t.Run("stray ms1 → not ok", func(t *testing.T) {
		ms1 := bundleCard{kind: cardMS1, strings: []string{"ms1x"}}
		if _, _, ok := extractReadbackMd1AndMk1s([]bundleCard{mk1, md1, ms1}); ok {
			t.Fatal("set with a stray ms1 accepted")
		}
	})
}
