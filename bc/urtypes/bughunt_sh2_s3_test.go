package urtypes

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/btcsuite/btcd/btcutil/v2/hdkeychain"
	"github.com/btcsuite/btcd/chaincfg/v2"
	"seedhammer.com/bip32"
	"seedhammer.com/bip380"
)

// Bug-hunt triage, sh2 slice S3 (report bughunt-2026-10-31-sh2.md).

func bhKey(fill byte) bip380.Key {
	kd := bytes.Repeat([]byte{fill}, 33)
	kd[0] = 0x02
	return bip380.Key{
		Network:           &chaincfg.MainNetParams,
		MasterFingerprint: 0x01020300 + uint32(fill),
		DerivationPath: bip32.Path{
			hdkeychain.HardenedKeyStart + 48, hdkeychain.HardenedKeyStart,
			hdkeychain.HardenedKeyStart, hdkeychain.HardenedKeyStart + 2,
		},
		KeyData:           kd,
		ChainCode:         bytes.Repeat([]byte{fill + 1}, 32),
		ParentFingerprint: 0x0a0b0c0d,
	}
}

// bhHDKey returns a CBOR crypto-hdkey whose origin or children components are
// the given ones.
func bhHDKey(t *testing.T, origin, children []any) []byte {
	t.Helper()
	k := bhKey(0x30)
	h := hdkeyFor(k)
	if origin != nil {
		h.Origin.Components = origin
		h.Origin.Depth = 0
	}
	if children != nil {
		h.Children.Components = children
	}
	enc, err := encMode.Marshal(h)
	if err != nil {
		t.Fatal(err)
	}
	return enc
}

// Bug 36: parseKeypath only bounds child indices by MaxUint32, so an index of
// 2^31 or more, which BCR-2020-007 forbids (child-index = uint31 =
// uint32 .lt 0x80000000), aliases or wraps instead of being refused.
func TestBughuntSh236HDKeyIndexAtLeast2p31(t *testing.T) {
	// Positive controls: the boundary values are fine.
	for _, comps := range [][]any{
		{uint64(0), true},
		{uint64(0x7FFFFFFF), true},
		{uint64(0x7FFFFFFF), false},
	} {
		if _, err := Parse("crypto-hdkey", bhHDKey(t, comps, nil)); err != nil {
			t.Fatalf("control origin %v refused: %v", comps, err)
		}
	}
	for _, c := range []struct {
		name  string
		where string
		comps []any
	}{
		{"origin 2^31 unhardened", "origin", []any{uint64(0x80000000), false}},
		{"origin 2^31 hardened", "origin", []any{uint64(0x80000000), true}},
		{"origin 2^32-1 hardened", "origin", []any{uint64(0xFFFFFFFF), true}},
		{"children 2^31", "children", []any{uint64(0x80000000), false}},
		{"children range ending at 2^31", "children", []any{[]any{uint64(0), uint64(0x80000000)}, false}},
	} {
		t.Run(c.name, func(t *testing.T) {
			var enc []byte
			if c.where == "origin" {
				enc = bhHDKey(t, c.comps, nil)
			} else {
				enc = bhHDKey(t, nil, c.comps)
			}
			v, err := Parse("crypto-hdkey", enc)
			if err == nil {
				k := v.(bip380.Key)
				t.Errorf("accepted: origin path %s children %+v", k.DerivationPath.Encode(), k.Children)
			}
		})
	}
}

// Bug 37: a range derivation must be written ([low, high], is-hardened)
// (BCR-2020-007: "child-range-component = ([child-index, child-index],
// is-hardened) ; [low, high] where low < high"), and the repo's own parser
// reads that shape, so a key must survive EncodeKey -> Parse.
func TestBughuntSh237RangeDerivationShape(t *testing.T) {
	// Positive control: a plain child and a wildcard round-trip today.
	ctl := bhKey(0x10)
	ctl.Children = []bip380.Derivation{
		{Type: bip380.ChildDerivation, Index: 0},
		{Type: bip380.WildcardDerivation},
	}
	v, err := Parse("crypto-hdkey", EncodeKey(ctl))
	if err != nil {
		t.Fatalf("control round trip: %v", err)
	}
	if got := v.(bip380.Key).Children; len(got) != 2 || got[1].Type != bip380.WildcardDerivation {
		t.Fatalf("control children = %+v", got)
	}

	k := bhKey(0x10)
	k.Children = []bip380.Derivation{{Type: bip380.RangeDerivation, Index: 0, End: 1}}
	h := hdkeyFor(k)
	if want := fmt.Sprint([]any{[]any{uint32(0), uint32(1)}, false}); fmt.Sprint(h.Children.Components) != want {
		t.Errorf("range written as %v, want %v ([[low, high], is-hardened])", h.Children.Components, want)
	}
	v, err = Parse("crypto-hdkey", EncodeKey(k))
	if err != nil {
		t.Fatalf("EncodeKey output is refused by Parse: %v", err)
	}
	got := v.(bip380.Key).Children
	if len(got) != 1 || got[0] != k.Children[0] {
		t.Errorf("range round-tripped to %+v, want %+v", got, k.Children)
	}
}

// Bug 38: the script wrapper must fit its contents. BCR-2020-010's CDDL:
// "public-key-hash = #6.403(key_exp)", "witness-public-key-hash =
// #6.404(key_exp)" and "witness-script-hash = #6.401(script_exp)", while
// "sorted-multisig = #6.407(multikey)" is a script_exp, not a key_exp, and a
// bare crypto-hdkey is a key_exp, not a script_exp.
func TestBughuntSh238ScriptTypeMismatch(t *testing.T) {
	two := []bip380.Key{bhKey(0x10), bhKey(0x20)}
	desc := func(s bip380.Script, typ bip380.MultisigType, keys []bip380.Key) []byte {
		d := &bip380.Descriptor{Script: s, Type: typ, Threshold: 1, Keys: keys}
		return EncodeDescriptor(d)
	}
	// Positive controls: the legal shapes parse.
	for name, enc := range map[string][]byte{
		"wsh(sortedmulti)":     desc(bip380.P2WSH, bip380.SortedMulti, two),
		"sh(sortedmulti)":      desc(bip380.P2SH, bip380.SortedMulti, two),
		"sh(wsh(sortedmulti))": desc(bip380.P2SH_P2WSH, bip380.SortedMulti, two),
		"wpkh(key)":            desc(bip380.P2WPKH, bip380.Singlesig, two[:1]),
		"pkh(key)":             desc(bip380.P2PKH, bip380.Singlesig, two[:1]),
	} {
		if _, err := Parse("crypto-output", enc); err != nil {
			t.Fatalf("control %s refused: %v", name, err)
		}
	}
	for name, enc := range map[string][]byte{
		"wpkh(sortedmulti)": desc(bip380.P2WPKH, bip380.SortedMulti, two),
		"pkh(sortedmulti)":  desc(bip380.P2PKH, bip380.SortedMulti, two),
		"wsh(bare key)":     desc(bip380.P2WSH, bip380.Singlesig, two[:1]),
	} {
		v, err := Parse("crypto-output", enc)
		if err == nil {
			d := v.(*bip380.Descriptor)
			t.Errorf("%s accepted: Script=%v Type=%v keys=%d", name, d.Script, d.Type, len(d.Keys))
		}
	}
}
