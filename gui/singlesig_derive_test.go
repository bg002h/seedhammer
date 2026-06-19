package gui

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/btcsuite/btcd/chaincfg/v2"
	"seedhammer.com/bundle"
	"seedhammer.com/codex32"
	"seedhammer.com/md"
	"seedhammer.com/mk"
)

func fp4(fp uint32) [4]byte {
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], fp)
	return b
}

// TestDeriveSingleSigBundleWpkh: the abandon-test seed at m/84'/0'/0' (BIP-84
// wpkh) derives ms1+mk1+md1 with a POLICY-BOUND, non-zero mk1 stub. We assert
// the DECODED mk1 fields (network/path/fingerprint/xpub) match T4's known card
// AND the stub == WalletPolicyIDStubChunks(md1) (R0-m1 — NOT raw-string vs T4's
// golden, since the bound stub changes the bytes); md1 round-trips to the wpkh
// wallet-policy with the masterFP embedded; ms1 decodes to the seed entropy;
// bundle.Verify(b, b) == nil (self-consistent incl. the stub binding).
func TestDeriveSingleSigBundleWpkh(t *testing.T) {
	m := abandonAboutMnemonic()
	path := singleSigPath(84)
	b, masterFP, parentFP, xpub, err := deriveSingleSigBundle(m, "", &chaincfg.MainNetParams, path, md.ScriptWpkh)
	if err != nil {
		t.Fatalf("deriveSingleSigBundle: %v", err)
	}

	// masterFP / xpub agree with T4's golden.
	if masterFP != knownMasterFP {
		t.Fatalf("masterFP = %08x, want %08x", masterFP, knownMasterFP)
	}
	if xpub != knownAccountXpub84 {
		t.Fatalf("xpub mismatch:\n got %s\nwant %s", xpub, knownAccountXpub84)
	}
	// parentFP is the REAL, non-zero parent fingerprint (R0-I1; threaded to the
	// restore doc in Task 6). The account key at m/84'/0'/0' has a non-zero
	// parent.
	if parentFP == 0 {
		t.Fatal("parentFP is zero; want the real non-zero account-key parent fingerprint")
	}

	// mk1: decoded-field assertion (R0-m1).
	card, err := mk.Decode(b.MK1)
	if err != nil {
		t.Fatalf("mk.Decode: %v", err)
	}
	if card.Network != "mainnet" {
		t.Fatalf("mk1 network = %q, want mainnet", card.Network)
	}
	if card.Path != "m/84h/0h/0h" {
		t.Fatalf("mk1 path = %q, want m/84h/0h/0h", card.Path)
	}
	if card.Fingerprint != "73c5da0a" {
		t.Fatalf("mk1 fingerprint = %q, want 73c5da0a", card.Fingerprint)
	}
	if card.Xpub != knownAccountXpub84 {
		t.Fatalf("mk1 xpub mismatch:\n got %s\nwant %s", card.Xpub, knownAccountXpub84)
	}
	// The bound, NON-ZERO stub == WalletPolicyIDStubChunks(md1).
	wantStub, err := md.WalletPolicyIDStubChunks(b.MD1)
	if err != nil {
		t.Fatalf("WalletPolicyIDStubChunks: %v", err)
	}
	if wantStub == ([4]byte{}) {
		t.Fatal("computed stub is zero; the policy-id stub must be non-zero")
	}
	if len(card.Stubs) != 1 || card.Stubs[0] != wantStub {
		t.Fatalf("mk1 stubs = %v, want exactly [%v] (policy-bound)", card.Stubs, wantStub)
	}
	if card.Stubs[0] == ([4]byte{0, 0, 0, 0}) {
		t.Fatal("mk1 stub is the T4 placeholder [0,0,0,0]; want the policy-bound stub")
	}

	// md1: round-trips to a wpkh wallet policy with the masterFP embedded
	// (EncodeSingleSig.fp == masterFP, NOT xpub.ParentFingerprint()).
	tpl, keys, err := md.ExpandWalletPolicyChunks(b.MD1)
	if err != nil {
		t.Fatalf("md.ExpandWalletPolicyChunks: %v", err)
	}
	if tpl.Root != md.ScriptWpkh {
		t.Fatalf("md1 root = %v, want ScriptWpkh", tpl.Root)
	}
	if len(keys) != 1 {
		t.Fatalf("md1 has %d keys, want 1", len(keys))
	}
	if !keys[0].XpubPresent || !keys[0].FingerprintPresent {
		t.Fatal("md1 key must carry both an xpub and a fingerprint TLV")
	}
	if keys[0].Fingerprint != fp4(masterFP) {
		t.Fatalf("md1 embedded fp = %x, want masterFP %x (not the xpub parent fp)", keys[0].Fingerprint, fp4(masterFP))
	}

	// ms1: decodes to the seed entropy.
	ms1str, err := codex32.New(b.MS1)
	if err != nil {
		t.Fatalf("codex32.New(ms1): %v", err)
	}
	_, _, ent, err := codex32.DecodeMS1(ms1str)
	if err != nil {
		t.Fatalf("DecodeMS1: %v", err)
	}
	if !bytes.Equal(ent, m.Entropy()) {
		t.Fatalf("ms1 entropy = %x, want %x", ent, m.Entropy())
	}

	// Self-consistent: verify the bundle against itself (incl. stub binding).
	if err := bundle.Verify(b, b); err != nil {
		t.Fatalf("bundle.Verify(b, b): %v (want nil)", err)
	}
}

// TestDeriveSingleSigBundleShWpkh: BIP-49 sh-wpkh derives with a policy-bound
// stub and an sh-wpkh md1 shape (the decoder summarizes sh(wpkh) to Root==
// ScriptSh). Self-consistent.
func TestDeriveSingleSigBundleShWpkh(t *testing.T) {
	m := abandonAboutMnemonic()
	path := singleSigPath(49)
	b, masterFP, parentFP, xpub, err := deriveSingleSigBundle(m, "", &chaincfg.MainNetParams, path, md.ScriptShWpkh)
	if err != nil {
		t.Fatalf("deriveSingleSigBundle: %v", err)
	}
	if parentFP == 0 {
		t.Fatal("parentFP is zero")
	}
	if xpub == "" || masterFP == 0 {
		t.Fatal("missing xpub/masterFP")
	}

	card, err := mk.Decode(b.MK1)
	if err != nil {
		t.Fatalf("mk.Decode: %v", err)
	}
	if card.Path != "m/49h/0h/0h" {
		t.Fatalf("mk1 path = %q, want m/49h/0h/0h", card.Path)
	}
	wantStub, err := md.WalletPolicyIDStubChunks(b.MD1)
	if err != nil {
		t.Fatalf("WalletPolicyIDStubChunks: %v", err)
	}
	if len(card.Stubs) != 1 || card.Stubs[0] != wantStub || card.Stubs[0] == ([4]byte{0, 0, 0, 0}) {
		t.Fatalf("mk1 stub not policy-bound: %v (want %v, non-zero)", card.Stubs, wantStub)
	}

	tpl, keys, err := md.ExpandWalletPolicyChunks(b.MD1)
	if err != nil {
		t.Fatalf("md.ExpandWalletPolicyChunks: %v", err)
	}
	// The decoder summarizes sh(wpkh) wire to Root==ScriptSh (recon Topic-0 note).
	if tpl.Root != md.ScriptSh {
		t.Fatalf("sh-wpkh md1 root = %v, want ScriptSh", tpl.Root)
	}
	if len(keys) != 1 || keys[0].Fingerprint != fp4(masterFP) {
		t.Fatalf("sh-wpkh md1 fp = %x, want masterFP %x", keys[0].Fingerprint, fp4(masterFP))
	}

	if err := bundle.Verify(b, b); err != nil {
		t.Fatalf("bundle.Verify(b, b) sh-wpkh: %v (want nil)", err)
	}
}

// TestDeriveSingleSigBundleMatchesGoldenWpkh: the derived wpkh bundle's ms1 +
// md1 byte-match the vendored abandon-seed wpkh golden (bundle/verify_test.go) —
// these legs are the encoders under test (codex32.EncodeMS1 / md.EncodeSingleSig)
// and are deterministic. The mk1 leg is asserted by its DECODED fields + the
// bound stub (R0-m1), NOT by raw-string vs the vendored golden: the vendored
// wpkhMK1 decodes to the same card but was emitted by an earlier mk.Encode and
// is not byte-identical to the current deterministic encoder (re-encoding the
// golden's own decoded card reproduces THIS bundle's mk1, not the vendored
// string) — exactly the stub-changes-the-bytes / encoder-drift class R0-m1
// guards against.
func TestDeriveSingleSigBundleMatchesGoldenWpkh(t *testing.T) {
	m := abandonAboutMnemonic()
	b, masterFP, _, _, err := deriveSingleSigBundle(m, "", &chaincfg.MainNetParams, singleSigPath(84), md.ScriptWpkh)
	if err != nil {
		t.Fatalf("deriveSingleSigBundle: %v", err)
	}
	const wantMS1 = "ms10entrsqqqqqqqqqqqqqqqqqqqqqqqqqqqqcj9sxraq34v7f"
	if b.MS1 != wantMS1 {
		t.Fatalf("ms1 = %q, want %q", b.MS1, wantMS1)
	}
	wantMD1 := []string{
		"md1fgdxlpqpqpm6jzzqqvqpdqw0za5zs4gyy55aq4vsmnhy4s6wyaypu34c7raqu8np",
		"md1fgdxlpqf2zcgefcpupmel75q5435j7seugaj5jr7qyur6vt76es5cdeyrq7zdy0d",
		"md1fgdxlpq3xa2dk8vwpj7gx74hwqxqdp083jehp5tdrfa0n5zdfkqcdlrvnh5r62jn",
	}
	if len(b.MD1) != len(wantMD1) {
		t.Fatalf("md1 chunk count = %d, want %d", len(b.MD1), len(wantMD1))
	}
	for i := range wantMD1 {
		if b.MD1[i] != wantMD1[i] {
			t.Fatalf("md1[%d] mismatch:\n got %s\nwant %s", i, b.MD1[i], wantMD1[i])
		}
	}
	// mk1: decoded fields + bound stub (R0-m1). The mk1 must round-trip and carry
	// the SAME bound stub as the vendored golden card (proving the binding is the
	// same), even though the byte string differs by encoder version.
	card, err := mk.Decode(b.MK1)
	if err != nil {
		t.Fatalf("mk.Decode: %v", err)
	}
	if card.Fingerprint != "73c5da0a" || card.Xpub != knownAccountXpub84 || card.Path != "m/84h/0h/0h" {
		t.Fatalf("mk1 decoded fields mismatch: %+v", card)
	}
	stub, _ := md.WalletPolicyIDStubChunks(b.MD1)
	if len(card.Stubs) != 1 || card.Stubs[0] != stub {
		t.Fatalf("mk1 stub %v not bound to md1 stub %v", card.Stubs, stub)
	}
	_ = masterFP
}
