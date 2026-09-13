package address

import (
	"testing"

	"github.com/btcsuite/btcd/chaincfg/v2"
	"seedhammer.com/bip32"
	"seedhammer.com/bip380"
)

// DerivesSameKey's contract, pinned where the normalisations live (F-530).
//
// The caller in gui/ used to compare struct fields and was walked past by four
// spellings of one key (review C-1). These rows are that contract stated as
// tests, including the two exclusions that are DELIBERATE -- review M-1 found
// that reinstating them left all 1319 gui tests green, so the choice was
// documented and held by nothing.

func testKey(t *testing.T, children ...bip380.Derivation) bip380.Key {
	t.Helper()
	cc := make([]byte, 32)
	for i := range cc {
		cc[i] = byte(i + 1)
	}
	// A valid compressed point: the generator.
	pk := []byte{
		0x02, 0x79, 0xbe, 0x66, 0x7e, 0xf9, 0xdc, 0xbb, 0xac, 0x55, 0xa0,
		0x62, 0x95, 0xce, 0x87, 0x0b, 0x07, 0x02, 0x9b, 0xfc, 0xdb, 0x2d,
		0xce, 0x28, 0xd9, 0x59, 0xf2, 0x81, 0x5b, 0x16, 0xf8, 0x17, 0x98,
	}
	return bip380.Key{
		Network:   &chaincfg.MainNetParams,
		KeyData:   pk,
		ChainCode: cc,
		Children:  children,
	}
}

func rangeD(i, e uint32) bip380.Derivation {
	return bip380.Derivation{Type: bip380.RangeDerivation, Index: i, End: e}
}
func childD(i uint32, hard bool) bip380.Derivation {
	return bip380.Derivation{Type: bip380.ChildDerivation, Index: i, Hardened: hard}
}
func wildD(hard bool) bip380.Derivation {
	return bip380.Derivation{Type: bip380.WildcardDerivation, Hardened: hard}
}

func TestDerivesSameKeySpellings(t *testing.T) {
	for _, tc := range []struct {
		name string
		a, b []bip380.Derivation
		want bool
		why  string
	}{
		{"implicit vs explicit <0;1>/*", nil, []bip380.Derivation{rangeD(0, 1), wildD(false)}, true,
			"derivePubKey materialises exactly this default for empty Children"},
		{"child 0 vs range on receive", []bip380.Derivation{childD(0, false), wildD(false)},
			[]bip380.Derivation{rangeD(0, 1), wildD(false)}, true,
			"a range resolves to Index on receive -- the chain that gets funded"},
		{"hardened child vs not", []bip380.Derivation{childD(0, true), wildD(false)},
			[]bip380.Derivation{childD(0, false), wildD(false)}, true,
			"derivePubKey never consults Hardened"},
		{"hardened wildcard vs not", []bip380.Derivation{wildD(true)},
			[]bip380.Derivation{wildD(false)}, true, "same reason"},
		{"different fixed child", []bip380.Derivation{childD(0, false), wildD(false)},
			[]bip380.Derivation{childD(1, false), wildD(false)}, false,
			"different child index, different key, on both chains"},
		{"disjoint multipath", []bip380.Derivation{rangeD(0, 1), wildD(false)},
			[]bip380.Derivation{rangeD(2, 3), wildD(false)}, false,
			"BIP 388 permits one key at disjoint multipath sets"},
		{"different depth", []bip380.Derivation{wildD(false)},
			[]bip380.Derivation{childD(0, false), wildD(false)}, false,
			"a different number of derivations is a different key"},
		{"unsupported range", []bip380.Derivation{rangeD(0, 5), wildD(false)},
			[]bip380.Derivation{rangeD(0, 5), wildD(false)}, false,
			"derivePubKey refuses End != Index+1, so neither derives and neither can collide"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, b := testKey(t, tc.a...), testKey(t, tc.b...)
			if got := DerivesSameKey(a, b); got != tc.want {
				t.Errorf("DerivesSameKey = %v, want %v -- %s", got, tc.want, tc.why)
			}
			if got := DerivesSameKey(b, a); got != tc.want {
				t.Errorf("not symmetric: reversed gives %v, want %v", got, tc.want)
			}
		})
	}
}

// TestDerivesSameKeyIgnoresOriginMetadata is review M-1: the exclusion was
// documented and pinned by nothing, so reinstating the comparison left every
// test green.
//
// Two seats labelled with DIFFERENT origins still push identical bytes into the
// script -- MasterFingerprint and DerivationPath record where an xpub came from
// and take no part in CKDpub. A predicate comparing them would miss the
// duplicate exactly when the two seats were labelled differently, which is what
// a coordinator bug producing this shape would plausibly do.
func TestDerivesSameKeyIgnoresOriginMetadata(t *testing.T) {
	a := testKey(t)
	b := testKey(t)
	a.MasterFingerprint = 0xdeadbeef
	b.MasterFingerprint = 0xfeedface
	a.DerivationPath = bip32.Path{1, 2, 3}
	b.DerivationPath = bip32.Path{9}
	if !DerivesSameKey(a, b) {
		t.Fatal("two keys differing only in origin metadata are reported as different " +
			"keys. Origin records where the xpub came from; it never reaches the script, " +
			"and both of these push identical bytes")
	}

	// And the derivation agrees, so the claim above is measured rather than
	// asserted: same pubkey out, despite the differing metadata.
	pa, err := derivePubKey(a, 0, false)
	if err != nil {
		t.Fatalf("derivePubKey(a): %v", err)
	}
	pb, err := derivePubKey(b, 0, false)
	if err != nil {
		t.Fatalf("derivePubKey(b): %v", err)
	}
	if !pa.IsEqual(pb) {
		t.Fatal("the two keys derive DIFFERENT pubkeys, so ignoring origin metadata is " +
			"wrong and this test has been pinning the wrong contract")
	}
}
