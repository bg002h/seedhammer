package bip32

import (
	"bytes"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/btcsuite/btcd/chaincfg/v2"
	"seedhammer.com/bip39"
)

// BIP-39 English test vector 1 (github.com/trezor/python-mnemonic
// vectors.json): entropy 00..00 -> "abandon ... about". With passphrase
// "TREZOR" its 64-byte seed is abandonSeedHex. With no passphrase its BIP-32
// master fingerprint is 73c5da0a, the value BIP-84's own test vector prints
// for this mnemonic ("m/84'/0'/0'" under [73c5da0a/...]).
const (
	abandonMnemonic = "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"
	abandonSeedHex  = "c55257c360c07c72029aebc1b53c05ed0362ada38ead3e3e9efa3708e534955" +
		"31f09a6987599d18264c1e1c92f2cf141630c7a3c4ab7c81b2f001698e7463b04"
	abandonFingerprint = 0x73c5da0a
)

func abandon(t *testing.T) bip39.Mnemonic {
	t.Helper()
	m, err := bip39.ParseMnemonic(strings.ToUpper(abandonMnemonic))
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func allZero(b []byte) bool {
	for _, x := range b {
		if x != 0 {
			return false
		}
	}
	return true
}

func TestMnemonicFingerprintKnownVector(t *testing.T) {
	fp, err := MnemonicFingerprint(abandon(t), &chaincfg.MainNetParams, "")
	if err != nil {
		t.Fatal(err)
	}
	if fp != abandonFingerprint {
		t.Fatalf("fingerprint = %.8x, want %.8x", fp, uint32(abandonFingerprint))
	}
}

// TestMasterKeyWipesTheBIP39Seed pins MasterKey's `defer wipe(seed)`. The
// hook hands over the seed slice itself, so the read after the call observes
// the same backing array the wipe writes; the snapshot taken while it was live
// must equal the published seed first, or the zero check would prove nothing.
func TestMasterKeyWipesTheBIP39Seed(t *testing.T) {
	var captured, snapshot []byte
	mk, ok := MasterKey(abandon(t), &chaincfg.MainNetParams, "TREZOR", func(seed []byte) {
		captured = seed
		snapshot = append([]byte(nil), seed...)
	})
	if !ok {
		t.Fatal("MasterKey failed on BIP-39 test vector 1")
	}
	mk.Zero()
	want, _ := hex.DecodeString(abandonSeedHex)
	if !bytes.Equal(snapshot, want) {
		t.Fatalf("hook saw %x, want the BIP-39 seed %x", snapshot, want)
	}
	if len(captured) != 64 || !allZero(captured) {
		t.Fatalf("the 64-byte BIP-39 seed is still %x after MasterKey returned", captured)
	}
}

// TestMasterFingerprintZeroesTheKey pins MasterFingerprint's `defer mk.Zero()`.
// hdkeychain's Zero clears the chain code in place (ChainCode copies out of
// the same array) and only then drops the private flag.
func TestMasterFingerprintZeroesTheKey(t *testing.T) {
	mk, ok := MasterKey(abandon(t), &chaincfg.MainNetParams, "", nil)
	if !ok {
		t.Fatal("MasterKey failed")
	}
	if !mk.IsPrivate() || allZero(mk.ChainCode()) {
		t.Fatal("control: the key is not a live private key")
	}
	fp, err := MasterFingerprint(mk)
	if err != nil {
		t.Fatal(err)
	}
	if fp != abandonFingerprint {
		t.Fatalf("fingerprint = %.8x, want %.8x", fp, uint32(abandonFingerprint))
	}
	if mk.IsPrivate() || !allZero(mk.ChainCode()) {
		t.Fatal("the master private key is live after MasterFingerprint returned")
	}
}
