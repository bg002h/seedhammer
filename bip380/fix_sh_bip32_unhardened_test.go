package bip380

import (
	"testing"

	"seedhammer.com/bip32"
)

// FIX-sh-bip32-unhardened (bughunt-1119 bh-sh-M6, regraded I). BIP-380: key
// origin and use-site steps are "/NUM or /NUMh path elements to indicate the
// unhardened or hardened derivation steps", and its invalid vector "Derivation
// index out of range: xprv.../2147483648" refuses an unmarked 2^31. The parser
// carried an unmarked 2147483648 as 0h.

const fixShBip32K = "tpubD6NzVbkrYhZ4Wgf68pWWfTRzdXMoepBvepfiAKtNoE7RvbAbWVfWzQxH1jbkfNk3iJ9zR65Yw6u3B2QZzpkMSTN4y8Lfm1t44HbpZX7efhZ"

func TestFixShBip32UnhardenedOriginRefused(t *testing.T) {
	for _, origin := range []string{"84h/0h/2147483648", "84h/0h/4294967295", "2147483648", "48'/1'/2147483648/2'"} {
		desc := "wpkh([d34db33f/" + origin + "]" + fixShBip32K + "/0/*)"
		if d, err := Parse(desc); err == nil {
			t.Errorf("Parse(%q) accepted; origin %v", desc, bip32.Path(d.Keys[0].DerivationPath))
		}
	}
}

func TestFixShBip32UnhardenedChildRefused(t *testing.T) {
	for _, use := range []string{"/2147483648/*", "/0/2147483648", "/4294967295/*"} {
		desc := "wpkh([d34db33f/84h/1h/0h]" + fixShBip32K + use + ")"
		if d, err := Parse(desc); err == nil {
			t.Errorf("Parse(%q) accepted; children %+v", desc, d.Keys[0].Children)
		}
	}
	// A range end of 2^31 was already refused; it stays refused.
	desc := "wpkh([d34db33f/84h/1h/0h]" + fixShBip32K + "/<0;2147483648>/*)"
	if _, err := Parse(desc); err == nil {
		t.Errorf("Parse(%q) accepted", desc)
	}
}

// Control: the largest unhardened and hardened elements still parse to the
// same values.
func TestFixShBip32UnhardenedBoundaryAccepted(t *testing.T) {
	desc := "wpkh([d34db33f/84h/1h/2147483647]" + fixShBip32K + "/2147483647/*)"
	d, err := Parse(desc)
	if err != nil {
		t.Fatalf("Parse(%q): %v", desc, err)
	}
	k := d.Keys[0]
	if got := bip32.Path(k.DerivationPath).String(); got != "m/84h/1h/2147483647" {
		t.Errorf("origin = %s, want m/84h/1h/2147483647", got)
	}
	if len(k.Children) != 2 || k.Children[0].Index != 2147483647 || k.Children[0].Hardened {
		t.Errorf("children = %+v, want unhardened 2147483647 then *", k.Children)
	}
	desc = "wpkh([d34db33f/2147483647h]" + fixShBip32K + "/0/*)"
	d, err = Parse(desc)
	if err != nil {
		t.Fatalf("Parse(%q): %v", desc, err)
	}
	if got := bip32.Path(d.Keys[0].DerivationPath); len(got) != 1 || got[0] != 0xFFFFFFFF {
		t.Errorf("origin = %v, want [0xffffffff]", got)
	}
}
