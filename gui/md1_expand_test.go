package gui

import (
	"encoding/binary"
	"testing"

	"github.com/btcsuite/btcd/btcutil/v2/hdkeychain"
	"github.com/btcsuite/btcd/chaincfg/v2"
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

// TestExpandedToDescriptorShWpkh (Task 2): a hand-built sh(wpkh) Template
// (Root=ScriptSh, Policy=PolicySingle, InnerWpkh=true) with one xpub-present key
// → expandOK + a P2SH_P2WPKH/Singlesig descriptor; address.Supported lights up.
func TestExpandedToDescriptorShWpkh(t *testing.T) {
	tpl := md.Template{N: 1, Root: md.ScriptSh, Policy: md.PolicySingle, Renderable: true, InnerWpkh: true}
	keys := []md.ExpandedKey{expandedKey(0, [4]byte{0x5a, 0x8, 0x4, 0xe3})}
	desc, status := expandedToDescriptor(tpl, keys)
	if status != expandOK {
		t.Fatalf("status = %v, want expandOK", status)
	}
	if desc.Script != bip380.P2SH_P2WPKH || desc.Type != bip380.Singlesig {
		t.Fatalf("desc = {Script:%v Type:%v}, want P2SH_P2WPKH/Singlesig", desc.Script, desc.Type)
	}
	if !address.Supported(desc) {
		t.Fatal("address.Supported(P2SH_P2WPKH singlesig) = false, want true (verify must light up)")
	}
	// The derived receive address round-trips and is a mainnet P2SH (3…).
	addr, err := address.Receive(desc, 0)
	if err != nil {
		t.Fatalf("address.Receive: %v", err)
	}
	if len(addr) == 0 || addr[0] != '3' {
		t.Fatalf("receive addr = %q, want a mainnet P2SH (3…) address", addr)
	}
	if _, _, found, err := address.Find(desc, addr, 20); err != nil || !found {
		t.Fatalf("Find(%s) found=%v err=%v", addr, found, err)
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

// TestShWpkhGoldenAddress (Task 3, A1/I1 — load-bearing): a BIP-49 sh(wpkh) md1
// for the abandon seed, decoded through the REAL projection path
// (EncodeSingleSig(ScriptShWpkh) -> ExpandWalletPolicyChunks -> expandedToDescriptor),
// derives the byte-exact P2SH-P2WPKH receive[0]/change[0] golden.
func TestShWpkhGoldenAddress(t *testing.T) {
	m := abandonAboutMnemonic()
	path := singleSigPath(49) // m/49'/0'/0'
	xpub, masterFP, err := deriveAccountXpub(m, "", &chaincfg.MainNetParams, path)
	if err != nil {
		t.Fatalf("deriveAccountXpub: %v", err)
	}
	const wantAcctXpub = "xpub6C6nQwHaWbSrzs5tZ1q7m5R9cPK9eYpNMFesiXsYrgc1P8bvLLAet9JfHjYXKjToD8cBRswJXXbbFpXgwsswVPAZzKMa1jUp2kVkGVUaJa7"
	if xpub != wantAcctXpub {
		t.Fatalf("BIP-49 account xpub = %s, want %s", xpub, wantAcctXpub)
	}
	cc, pk, _, err := decodeXpubBytes(xpub)
	if err != nil {
		t.Fatalf("decodeXpubBytes: %v", err)
	}
	var fp [4]byte
	binary.BigEndian.PutUint32(fp[:], masterFP)

	strs, err := md.EncodeSingleSig(cc, pk, fp, originComponents(path), md.ScriptShWpkh)
	if err != nil {
		t.Fatalf("EncodeSingleSig: %v", err)
	}
	tpl, keys, err := md.ExpandWalletPolicyChunks(strs)
	if err != nil {
		t.Fatalf("ExpandWalletPolicyChunks: %v", err)
	}
	// The real decode now renders it (Task 1 touch-point under test).
	if tpl.Root != md.ScriptSh || tpl.Policy != md.PolicySingle || !tpl.Renderable || !tpl.InnerWpkh {
		t.Fatalf("decoded tpl = {Root:%v Policy:%v Renderable:%v InnerWpkh:%v}, want ScriptSh/PolicySingle/true/true", tpl.Root, tpl.Policy, tpl.Renderable, tpl.InnerWpkh)
	}
	if tpl.InnerWsh {
		t.Fatal("InnerWsh = true for sh(wpkh); want false (discriminants independent)")
	}

	desc, status := expandedToDescriptor(tpl, keys)
	if status != expandOK {
		t.Fatalf("status = %v, want expandOK", status)
	}
	if desc.Script != bip380.P2SH_P2WPKH || desc.Type != bip380.Singlesig {
		t.Fatalf("desc = {Script:%v Type:%v}, want P2SH_P2WPKH/Singlesig", desc.Script, desc.Type)
	}
	r0, err := address.Receive(desc, 0)
	if err != nil {
		t.Fatalf("address.Receive: %v", err)
	}
	c0, err := address.Change(desc, 0)
	if err != nil {
		t.Fatalf("address.Change: %v", err)
	}
	const wantRecv0 = "37VucYSaXLCAsxYyAPfbSi9eh4iEcbShgf"
	const wantChange0 = "34K56kSjgUCUSD8GTtuF7c9Zzwokbs6uZ7"
	if r0 != wantRecv0 {
		t.Fatalf("BIP-49 sh(wpkh) receive[0] = %s, want %s", r0, wantRecv0)
	}
	if c0 != wantChange0 {
		t.Fatalf("BIP-49 sh(wpkh) change[0] = %s, want %s", c0, wantChange0)
	}
}

// TestShWpkhNoCollision (Task 3, A2/I2): for the SAME key material, the
// sh(wpkh) P2SH-P2WPKH receive[0] differs from both the sh(wsh(sortedmulti))
// P2SH-P2WSH and the bare sh(sortedmulti) P2SH receive[0]. Built directly so
// the three sh shapes share one key set.
func TestShWpkhNoCollision(t *testing.T) {
	k1 := []md.ExpandedKey{expandedKey(0, [4]byte{0x5a, 0x8, 0x4, 0xe3})}
	k2 := []md.ExpandedKey{
		expandedKey(0, [4]byte{0x5a, 0x8, 0x4, 0xe3}),
		expandedKey(1, [4]byte{0xdd, 0x4f, 0xad, 0xee}),
	}
	shWpkh := md.Template{N: 1, Root: md.ScriptSh, Policy: md.PolicySingle, Renderable: true, InnerWpkh: true}
	bare := md.Template{N: 2, Root: md.ScriptSh, Policy: md.PolicySortedMulti, K: 1, M: 2, Renderable: true, InnerWsh: false}
	nested := md.Template{N: 2, Root: md.ScriptSh, Policy: md.PolicySortedMulti, K: 1, M: 2, Renderable: true, InnerWsh: true}

	dWpkh, sWpkh := expandedToDescriptor(shWpkh, k1)
	dBare, sBare := expandedToDescriptor(bare, k2)
	dNested, sNested := expandedToDescriptor(nested, k2)
	if sWpkh != expandOK || sBare != expandOK || sNested != expandOK {
		t.Fatalf("statuses = %v/%v/%v, want all expandOK", sWpkh, sBare, sNested)
	}
	if dWpkh.Script != bip380.P2SH_P2WPKH || dBare.Script != bip380.P2SH || dNested.Script != bip380.P2SH_P2WSH {
		t.Fatalf("scripts = %v/%v/%v, want P2SH_P2WPKH/P2SH/P2SH_P2WSH", dWpkh.Script, dBare.Script, dNested.Script)
	}
	aWpkh, _ := address.Receive(dWpkh, 0)
	aBare, _ := address.Receive(dBare, 0)
	aNested, _ := address.Receive(dNested, 0)
	if aWpkh == aBare || aWpkh == aNested || aBare == aNested {
		t.Fatalf("collision: P2SH_P2WPKH=%s P2SH=%s P2SH_P2WSH=%s must be pairwise-distinct", aWpkh, aBare, aNested)
	}
}
