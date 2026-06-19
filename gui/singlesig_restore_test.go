package gui

import (
	"strings"
	"testing"

	"github.com/btcsuite/btcd/btcutil/v2/hdkeychain"
	"github.com/btcsuite/btcd/chaincfg/v2"
	"seedhammer.com/address"
	"seedhammer.com/bip380"
	"seedhammer.com/md"
	"seedhammer.com/mk"
)

// TestSingleSigRestoreDescriptorScripts: building the restore descriptor DIRECTLY
// (bypassing the classifier) yields the right bip380.Script for all 4 single-sig
// types, INCLUDING sh-wpkh (which the classifier would drop). Each descriptor's
// receive/change addresses derive + round-trip via address.Find.
func TestSingleSigRestoreDescriptorScripts(t *testing.T) {
	cc, pk, pfp, err := decodeXpubBytes(knownAccountXpub84)
	if err != nil {
		t.Fatalf("decodeXpubBytes: %v", err)
	}
	_ = cc
	_ = pk
	cases := []struct {
		name    string
		purpose int
		script  md.ScriptKind
		want    bip380.Script
	}{
		{"pkh", 44, md.ScriptPkh, bip380.P2PKH},
		{"sh-wpkh", 49, md.ScriptShWpkh, bip380.P2SH_P2WPKH},
		{"wpkh", 84, md.ScriptWpkh, bip380.P2WPKH},
		{"tr", 86, md.ScriptTr, bip380.P2TR},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			desc, err := singleSigRestoreDescriptor(knownAccountXpub84, knownMasterFP, pfp, c.script, singleSigPath(c.purpose))
			if err != nil {
				t.Fatalf("singleSigRestoreDescriptor: %v", err)
			}
			if desc.Script != c.want || desc.Type != bip380.Singlesig {
				t.Fatalf("%s: desc {Script:%v Type:%v}, want %v/Singlesig", c.name, desc.Script, desc.Type, c.want)
			}
			// Explicit <0;1>/* children (R0-m4).
			if len(desc.Keys) != 1 || len(desc.Keys[0].Children) != 2 {
				t.Fatalf("%s: want one key with explicit <0;1>/* children, got %d keys", c.name, len(desc.Keys))
			}
			ch := desc.Keys[0].Children
			if ch[0].Type != bip380.RangeDerivation || ch[0].Index != 0 || ch[0].End != 1 {
				t.Fatalf("%s: child[0] = %+v, want RangeDerivation 0..1", c.name, ch[0])
			}
			if ch[1].Type != bip380.WildcardDerivation {
				t.Fatalf("%s: child[1] = %+v, want WildcardDerivation", c.name, ch[1])
			}
			// Addresses derive + round-trip.
			r0, err := address.Receive(desc, 0)
			if err != nil {
				t.Fatalf("%s: Receive: %v", c.name, err)
			}
			if _, err := address.Change(desc, 0); err != nil {
				t.Fatalf("%s: Change: %v", c.name, err)
			}
			if _, _, found, err := address.Find(desc, r0, 20); err != nil || !found {
				t.Fatalf("%s: Find(%s) found=%v err=%v", c.name, r0, found, err)
			}
		})
	}
}

// TestSingleSigRestoreWpkhKnownAddress: the wpkh restore descriptor for the
// abandon-test seed derives the canonical, well-known BIP-84 receive #0 address.
func TestSingleSigRestoreWpkhKnownAddress(t *testing.T) {
	_, _, pfp, err := decodeXpubBytes(knownAccountXpub84)
	if err != nil {
		t.Fatalf("decodeXpubBytes: %v", err)
	}
	desc, err := singleSigRestoreDescriptor(knownAccountXpub84, knownMasterFP, pfp, md.ScriptWpkh, singleSigPath(84))
	if err != nil {
		t.Fatalf("descriptor: %v", err)
	}
	r0, err := address.Receive(desc, 0)
	if err != nil {
		t.Fatalf("Receive: %v", err)
	}
	const wantRecv0 = "bc1qcr8te4kr609gcawutmrza0j4xv80jy8z306fyu"
	if r0 != wantRecv0 {
		t.Fatalf("BIP-84 abandon receive #0 = %s, want %s", r0, wantRecv0)
	}
}

// TestSingleSigRestoreEncodeXpubMatchesMK1 (R0-I1, the real golden): the restore
// descriptor's Encode() xpub BYTE-MATCHES the engraved mk1 card's xpub for all 4
// scripts. This proves the real parentFP is threaded — an address match alone
// would hide a dropped parentFP (which re-serializes a non-canonical xpub).
func TestSingleSigRestoreEncodeXpubMatchesMK1(t *testing.T) {
	m := abandonAboutMnemonic()
	cases := []struct {
		purpose int
		script  md.ScriptKind
	}{
		{44, md.ScriptPkh},
		{49, md.ScriptShWpkh},
		{84, md.ScriptWpkh},
		{86, md.ScriptTr},
	}
	for _, c := range cases {
		b, masterFP, parentFP, xpub, err := deriveSingleSigBundle(m, "", &chaincfg.MainNetParams, singleSigPath(c.purpose), c.script)
		if err != nil {
			t.Fatalf("purpose %d derive: %v", c.purpose, err)
		}
		// The engraved mk1's xpub.
		card, err := mk.Decode(b.MK1)
		if err != nil {
			t.Fatalf("mk.Decode: %v", err)
		}
		desc, err := singleSigRestoreDescriptor(xpub, masterFP, parentFP, c.script, singleSigPath(c.purpose))
		if err != nil {
			t.Fatalf("purpose %d descriptor: %v", c.purpose, err)
		}
		enc := desc.Encode()
		// The descriptor's serialized xpub (Key.String()) must contain the mk1
		// xpub VERBATIM (canonical).
		if !strings.Contains(enc, card.Xpub) {
			t.Fatalf("purpose %d: desc.Encode() %q does not contain the engraved mk1 xpub %q (parentFP not threaded?)", c.purpose, enc, card.Xpub)
		}
	}
}

// TestSingleSigRestoreCleanOfXprv: the descriptor + its encoding carry NO private
// material (no xprv).
func TestSingleSigRestoreCleanOfXprv(t *testing.T) {
	_, _, pfp, _ := decodeXpubBytes(knownAccountXpub84)
	for _, c := range []struct {
		purpose int
		script  md.ScriptKind
	}{{44, md.ScriptPkh}, {49, md.ScriptShWpkh}, {84, md.ScriptWpkh}, {86, md.ScriptTr}} {
		desc, err := singleSigRestoreDescriptor(knownAccountXpub84, knownMasterFP, pfp, c.script, singleSigPath(c.purpose))
		if err != nil {
			t.Fatalf("descriptor: %v", err)
		}
		enc := desc.Encode()
		if strings.Contains(enc, "xprv") || strings.Contains(enc, "tprv") {
			t.Fatalf("descriptor leaks private material: %s", enc)
		}
	}
}

// TestSingleSigRestoreDocLines: the restore-doc display lines include the master
// fingerprint, the descriptor, and the first receive + change addresses (the
// plain screen content), without any private material.
func TestSingleSigRestoreDocLines(t *testing.T) {
	_, _, pfp, _ := decodeXpubBytes(knownAccountXpub84)
	desc, err := singleSigRestoreDescriptor(knownAccountXpub84, knownMasterFP, pfp, md.ScriptWpkh, singleSigPath(84))
	if err != nil {
		t.Fatalf("descriptor: %v", err)
	}
	lines, err := singleSigRestoreLines(knownMasterFP, desc)
	if err != nil {
		t.Fatalf("singleSigRestoreLines: %v", err)
	}
	joined := strings.Join(lines, "\n")
	if !strings.Contains(strings.ToLower(joined), "73c5da0a") {
		t.Errorf("restore lines missing master fp:\n%s", joined)
	}
	if !strings.Contains(joined, "wpkh(") {
		t.Errorf("restore lines missing descriptor:\n%s", joined)
	}
	if !strings.Contains(joined, "bc1qcr8te4kr609gcawutmrza0j4xv80jy8z306fyu") {
		t.Errorf("restore lines missing first receive address:\n%s", joined)
	}
	if strings.Contains(joined, "xprv") {
		t.Errorf("restore lines leak private material:\n%s", joined)
	}
}

// guard against an unused import if hdkeychain helpers are removed.
var _ = hdkeychain.HardenedKeyStart
