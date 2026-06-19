package gui

import (
	"strings"
	"testing"

	"github.com/btcsuite/btcd/chaincfg/v2"
	"seedhammer.com/bip32"
	"seedhammer.com/bip39"
)

// The canonical BIP-39 "abandon ... about" 12-word test seed.
func abandonAboutMnemonic() bip39.Mnemonic {
	m := make(bip39.Mnemonic, 12)
	for i := range m {
		m[i] = bip39.Word(0) // abandon
	}
	m[11] = bip39.Word(3) // about
	return m
}

// Golden values for the abandon-about seed (verified against the in-tree
// derivation; the master FP 73c5da0a is the canonical fingerprint for this
// seed).
const (
	knownAccountXpub84 = "xpub6CatWdiZiodmUeTDp8LT5or8nmbKNcuyvz7WyksVFkKB4RHwCD3XyuvPEbvqAQY3rAPshWcMLoP2fMFMKHPJ4ZeZXYVUhLv1VMrjPC7PW6V"
	knownMasterFP      = uint32(0x73c5da0a)
)

func mustPath(t *testing.T, s string) bip32.Path {
	t.Helper()
	p, err := bip32.ParsePath(s)
	if err != nil {
		t.Fatalf("ParsePath(%q): %v", s, err)
	}
	return p
}

func TestDeriveAccountXpub(t *testing.T) {
	m := abandonAboutMnemonic()
	xpub, mfp, err := deriveAccountXpub(m, "", &chaincfg.MainNetParams, mustPath(t, "m/84'/0'/0'"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(xpub, "xpub") {
		t.Fatalf("want xpub prefix, got %q", xpub)
	}
	if strings.Contains(xpub, "xprv") {
		t.Fatal("xprv leaked into output!")
	}
	if xpub != knownAccountXpub84 {
		t.Fatalf("xpub mismatch:\n got %s\nwant %s", xpub, knownAccountXpub84)
	}
	if mfp != knownMasterFP {
		t.Fatalf("master fingerprint = %08x, want %08x", mfp, knownMasterFP)
	}
}

func TestDeriveAccountXpubTestnet(t *testing.T) {
	m := abandonAboutMnemonic()
	xpub, _, err := deriveAccountXpub(m, "", &chaincfg.TestNet3Params, mustPath(t, "m/84'/1'/0'"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(xpub, "tpub") {
		t.Fatalf("want tpub prefix, got %q", xpub)
	}
	if strings.Contains(xpub, "tprv") {
		t.Fatal("tprv leaked into output!")
	}
}

// TestDeriveAccountXpubPassphrase confirms a passphrase changes the derived
// xpub (it feeds PBKDF2), and that the result is still public-only.
func TestDeriveAccountXpubPassphrase(t *testing.T) {
	m := abandonAboutMnemonic()
	bare, _, err := deriveAccountXpub(m, "", &chaincfg.MainNetParams, mustPath(t, "m/84'/0'/0'"))
	if err != nil {
		t.Fatal(err)
	}
	pp, _, err := deriveAccountXpub(m, "TREZOR", &chaincfg.MainNetParams, mustPath(t, "m/84'/0'/0'"))
	if err != nil {
		t.Fatal(err)
	}
	if pp == bare {
		t.Fatal("passphrase did not change the derived xpub")
	}
	if strings.Contains(pp, "xprv") {
		t.Fatal("xprv leaked with passphrase!")
	}
}
