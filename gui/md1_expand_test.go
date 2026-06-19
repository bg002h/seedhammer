package gui

import (
	"testing"

	"github.com/btcsuite/btcd/btcutil/v2/hdkeychain"
	"seedhammer.com/address"
	"seedhammer.com/bip32"
	"seedhammer.com/bip380"
	"seedhammer.com/md"
)

const hard = hdkeychain.HardenedKeyStart

// Three known-good (chainCode, compressedPubkey) pairs from gui_test.go's
// descriptor golden — packed into the 65-byte Pubkeys-TLV layout
// (xpub[0:32]=chainCode, xpub[32:65]=pubkey).
var goldenChainCodes = [3][]byte{
	{0xdb, 0xe8, 0xc, 0xbb, 0x4e, 0xe, 0x41, 0x8b, 0x6, 0xf4, 0x70, 0xd2, 0xaf, 0xe7, 0xa8, 0xc1, 0x7b, 0xe7, 0x1, 0xab, 0x20, 0x6c, 0x59, 0xa6, 0x5e, 0x65, 0xa8, 0x24, 0x1, 0x6a, 0x6c, 0x70},
	{0x43, 0x8e, 0xff, 0x7b, 0x3b, 0x36, 0xb6, 0xd1, 0x1a, 0x60, 0xa2, 0x2c, 0xcb, 0x93, 0x6, 0xee, 0xa3, 0x5, 0xb0, 0x43, 0x9f, 0x1e, 0xa0, 0x9d, 0x59, 0x28, 0x1, 0x5d, 0xe3, 0x73, 0x81, 0x16},
	{0x95, 0xb3, 0x49, 0x13, 0x93, 0x7f, 0xa5, 0xf1, 0xc6, 0x20, 0x5b, 0x52, 0x5b, 0xb5, 0x7d, 0xe1, 0x51, 0x76, 0x25, 0xe0, 0x45, 0x86, 0xb5, 0x95, 0xbe, 0x68, 0xe7, 0x13, 0x62, 0xd3, 0xed, 0xc5},
}

var goldenPubkeys = [3][]byte{
	{0x3, 0xa9, 0x39, 0x4a, 0x2f, 0x1a, 0x4f, 0x99, 0x61, 0x3a, 0x71, 0x69, 0x56, 0xc8, 0x54, 0xf, 0x6d, 0xba, 0x6f, 0x18, 0x93, 0x1c, 0x26, 0x39, 0x10, 0x72, 0x21, 0xb2, 0x67, 0xd7, 0x40, 0xaf, 0x23},
	{0x2, 0x21, 0x96, 0xad, 0xc2, 0x5f, 0xde, 0x16, 0x9f, 0xe9, 0x2e, 0x70, 0x76, 0x90, 0x59, 0x10, 0x22, 0x75, 0xd2, 0xb4, 0xc, 0xc9, 0x87, 0x76, 0xea, 0xab, 0x92, 0xb8, 0x2a, 0x86, 0x13, 0x5e, 0x92},
	{0x2, 0xfb, 0x72, 0x50, 0x7f, 0xc2, 0xd, 0xdb, 0xa9, 0x29, 0x91, 0xb1, 0x7c, 0x4b, 0xb4, 0x66, 0x13, 0xa, 0xd9, 0x3a, 0x88, 0x6e, 0x73, 0x17, 0x50, 0x33, 0xbb, 0x43, 0xe3, 0xbc, 0x78, 0x5a, 0x6d},
}

func goldenXpub(i int) [65]byte {
	var x [65]byte
	copy(x[0:32], goldenChainCodes[i])
	copy(x[32:65], goldenPubkeys[i])
	return x
}

// stdUseSite is the standard <0;1>/* use-site.
var stdUseSite = md.UseSite{HasMultipath: true, Multipath: []md.UseSiteAlt{{Value: 0}, {Value: 1}}}

func expandedKey(idx int, fp [4]byte) md.ExpandedKey {
	return md.ExpandedKey{
		Index:              uint8(idx),
		OriginPath:         bip32.Path{hard + 48, hard + 0, hard + 0, hard + 2},
		UseSite:            stdUseSite,
		Fingerprint:        fp,
		FingerprintPresent: true,
		Xpub:               goldenXpub(idx),
		XpubPresent:        true,
	}
}

// TestExpandedToDescriptorWshSortedmultiRoundTrip (Task 3.a): a wsh(sortedmulti)
// with real xpubs → expandOK + a P2WSH/SortedMulti *bip380.Descriptor whose
// derived receive address round-trips through address.Find.
func TestExpandedToDescriptorWshSortedmultiRoundTrip(t *testing.T) {
	tpl := md.Template{N: 3, Root: md.ScriptWsh, Policy: md.PolicySortedMulti, K: 2, M: 3, Renderable: true}
	keys := []md.ExpandedKey{
		expandedKey(0, [4]byte{0x5a, 0x8, 0x4, 0xe3}),
		expandedKey(1, [4]byte{0xdd, 0x4f, 0xad, 0xee}),
		expandedKey(2, [4]byte{0x9b, 0xac, 0xd5, 0xc0}),
	}
	desc, status := expandedToDescriptor(tpl, keys)
	if status != expandOK {
		t.Fatalf("status = %v, want expandOK", status)
	}
	if desc.Script != bip380.P2WSH || desc.Type != bip380.SortedMulti || desc.Threshold != 2 {
		t.Fatalf("desc = {Script:%v Type:%v Threshold:%d}, want P2WSH/SortedMulti/2", desc.Script, desc.Type, desc.Threshold)
	}
	if len(desc.Keys) != 3 {
		t.Fatalf("got %d keys, want 3", len(desc.Keys))
	}
	// Round-trip: derive a receive address, then Find it.
	addr, err := address.Receive(desc, 0)
	if err != nil {
		t.Fatalf("address.Receive: %v", err)
	}
	chain, index, found, err := address.Find(desc, addr, 20)
	if err != nil || !found {
		t.Fatalf("address.Find(%s) = chain=%d index=%d found=%v err=%v", addr, chain, index, found, err)
	}
	if chain != 0 || index != 0 {
		t.Fatalf("Find located receive addr at chain=%d index=%d, want 0/0", chain, index)
	}
}

// TestExpandedToDescriptorSinglesig (Task 3.b): wpkh / pkh / tr-keyonly →
// expandOK + Singlesig with the right Script, address round-trips.
func TestExpandedToDescriptorSinglesig(t *testing.T) {
	cases := []struct {
		name   string
		root   md.ScriptKind
		script bip380.Script
	}{
		{"wpkh", md.ScriptWpkh, bip380.P2WPKH},
		{"pkh", md.ScriptPkh, bip380.P2PKH},
		{"tr", md.ScriptTr, bip380.P2TR},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tpl := md.Template{N: 1, Root: tc.root, Policy: md.PolicySingle, Renderable: true}
			keys := []md.ExpandedKey{expandedKey(0, [4]byte{0x5a, 0x8, 0x4, 0xe3})}
			desc, status := expandedToDescriptor(tpl, keys)
			if status != expandOK {
				t.Fatalf("status = %v, want expandOK", status)
			}
			if desc.Script != tc.script || desc.Type != bip380.Singlesig {
				t.Fatalf("desc = {Script:%v Type:%v}, want %v/Singlesig", desc.Script, desc.Type, tc.script)
			}
			addr, err := address.Receive(desc, 0)
			if err != nil {
				t.Fatalf("address.Receive: %v", err)
			}
			if _, _, found, err := address.Find(desc, addr, 20); err != nil || !found {
				t.Fatalf("Find(%s) found=%v err=%v", addr, found, err)
			}
		})
	}
}

// TestExpandedToDescriptorShNesting (Task 3.b2, R0-C2): a bare sh(sortedmulti)
// maps to P2SH and a sh(wsh(sortedmulti)) maps to P2SH_P2WSH — DIFFERENT scripts
// (and thus different verify addresses). NEVER map sh+sortedmulti to
// P2SH_P2WSH unconditionally.
func TestExpandedToDescriptorShNesting(t *testing.T) {
	keys := []md.ExpandedKey{
		expandedKey(0, [4]byte{0x5a, 0x8, 0x4, 0xe3}),
		expandedKey(1, [4]byte{0xdd, 0x4f, 0xad, 0xee}),
	}
	bareTpl := md.Template{N: 2, Root: md.ScriptSh, Policy: md.PolicySortedMulti, K: 1, M: 2, Renderable: true, InnerWsh: false}
	nestedTpl := md.Template{N: 2, Root: md.ScriptSh, Policy: md.PolicySortedMulti, K: 1, M: 2, Renderable: true, InnerWsh: true}

	bareDesc, bs := expandedToDescriptor(bareTpl, keys)
	if bs != expandOK || bareDesc.Script != bip380.P2SH {
		t.Fatalf("bare sh(sortedmulti) → status=%v script=%v, want expandOK/P2SH", bs, bareDesc.Script)
	}
	nestedDesc, ns := expandedToDescriptor(nestedTpl, keys)
	if ns != expandOK || nestedDesc.Script != bip380.P2SH_P2WSH {
		t.Fatalf("sh(wsh(sortedmulti)) → status=%v script=%v, want expandOK/P2SH_P2WSH", ns, nestedDesc.Script)
	}
	// The two MUST derive to different addresses.
	a1, err1 := address.Receive(bareDesc, 0)
	a2, err2 := address.Receive(nestedDesc, 0)
	if err1 != nil || err2 != nil {
		t.Fatalf("Receive errs: %v / %v", err1, err2)
	}
	if a1 == a2 {
		t.Fatalf("P2SH and P2SH_P2WSH produced the SAME address %s — C2 violation", a1)
	}
}

// TestExpandedToDescriptorUnsortedMultiUnsupported (Task 3.c, D2): an unsorted
// multi / multi_a / sortedmulti_a / complex template → expandUnsupported (NEVER
// a descriptor).
func TestExpandedToDescriptorUnsortedMultiUnsupported(t *testing.T) {
	keys := []md.ExpandedKey{expandedKey(0, [4]byte{}), expandedKey(1, [4]byte{})}
	for _, pol := range []md.PolicyKind{md.PolicyMulti, md.PolicyMultiA, md.PolicySortedMultiA, md.PolicyComplex} {
		render := pol != md.PolicyComplex
		tpl := md.Template{N: 2, Root: md.ScriptWsh, Policy: pol, K: 1, M: 2, Renderable: render}
		desc, status := expandedToDescriptor(tpl, keys)
		if status != expandUnsupported {
			t.Fatalf("policy %v → status=%v, want expandUnsupported", pol, status)
		}
		if desc != nil {
			t.Fatalf("policy %v → non-nil descriptor (must NOT build)", pol)
		}
	}
}

// TestExpandedToDescriptorNoPubkeysTemplateOnly (Task 3.d, D3): a template whose
// keys lack xpubs → expandTemplateOnly (no descriptor, no verify).
func TestExpandedToDescriptorNoPubkeysTemplateOnly(t *testing.T) {
	tpl := md.Template{N: 1, Root: md.ScriptWpkh, Policy: md.PolicySingle, Renderable: true}
	keys := []md.ExpandedKey{{Index: 0, UseSite: stdUseSite, XpubPresent: false}}
	desc, status := expandedToDescriptor(tpl, keys)
	if status != expandTemplateOnly {
		t.Fatalf("status = %v, want expandTemplateOnly", status)
	}
	if desc != nil {
		t.Fatal("template-only must NOT build a descriptor")
	}
}

// TestExpandedToDescriptorHardenedWildcardUnsupported (Task 3.e, D5): a hardened
// trailing wildcard → expandUnsupported.
func TestExpandedToDescriptorHardenedWildcardUnsupported(t *testing.T) {
	tpl := md.Template{N: 1, Root: md.ScriptWpkh, Policy: md.PolicySingle, Renderable: true}
	k := expandedKey(0, [4]byte{})
	k.UseSite = md.UseSite{HasMultipath: true, Multipath: []md.UseSiteAlt{{Value: 0}, {Value: 1}}, WildcardHardened: true}
	if desc, status := expandedToDescriptor(tpl, []md.ExpandedKey{k}); status != expandUnsupported || desc != nil {
		t.Fatalf("hardened wildcard → status=%v desc!=nil=%v, want expandUnsupported/nil", status, desc != nil)
	}
}

// TestExpandedToDescriptorHardenedMultipathAltUnsupported (D5): a hardened
// multipath alternative → expandUnsupported.
func TestExpandedToDescriptorHardenedMultipathAltUnsupported(t *testing.T) {
	tpl := md.Template{N: 1, Root: md.ScriptWpkh, Policy: md.PolicySingle, Renderable: true}
	k := expandedKey(0, [4]byte{})
	k.UseSite = md.UseSite{HasMultipath: true, Multipath: []md.UseSiteAlt{{Value: 0}, {Hardened: true, Value: 1}}}
	if desc, status := expandedToDescriptor(tpl, []md.ExpandedKey{k}); status != expandUnsupported || desc != nil {
		t.Fatalf("hardened multipath alt → status=%v, want expandUnsupported", status)
	}
}

// TestExpandedToDescriptorExoticRangeUnsupported (Task 3.e2, R0-I2): a multipath
// <a;b>/* with b != a+1 → expandUnsupported (address.derivePubKey only supports
// End==Index+1; reject early so it fails display-only, not late at verify).
func TestExpandedToDescriptorExoticRangeUnsupported(t *testing.T) {
	tpl := md.Template{N: 1, Root: md.ScriptWpkh, Policy: md.PolicySingle, Renderable: true}
	k := expandedKey(0, [4]byte{})
	// <0;5>/* — End(5) != Index(0)+1.
	k.UseSite = md.UseSite{HasMultipath: true, Multipath: []md.UseSiteAlt{{Value: 0}, {Value: 5}}}
	if desc, status := expandedToDescriptor(tpl, []md.ExpandedKey{k}); status != expandUnsupported || desc != nil {
		t.Fatalf("exotic <0;5>/* range → status=%v, want expandUnsupported", status)
	}
}
