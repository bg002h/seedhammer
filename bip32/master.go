package bip32

import (
	"errors"

	"github.com/btcsuite/btcd/btcutil/v2/hdkeychain"
	"github.com/btcsuite/btcd/chaincfg/v2"
	"seedhammer.com/bip39"
)

// MasterKey derives the BIP-32 master private key of the BIP-39 mnemonic m
// under password. It is gui's deriveMasterKey derivation, moved here so the
// backup package can check a plate's fingerprint against its words without a
// second copy of it (F-702 F5).
//
// The 64-byte BIP-39 seed is seed-equivalent material and this is its only
// use, so it is wiped on every exit. The returned key is the CALLER's to Zero
// (MasterFingerprint does); this function cannot, it is the return value.
//
// seedHook, nil in production, is handed the seed slice right after its wipe
// is deferred, so a test holds the same backing array and can read it after
// the call returns (gui's deriveSeedHook, F-94).
//
// ok is false only when hdkeychain.NewMaster rejects the seed, a 1-in-2^127
// event; the key is then nil.
func MasterKey(m bip39.Mnemonic, net *chaincfg.Params, password string, seedHook func([]byte)) (mk *hdkeychain.ExtendedKey, ok bool) {
	seed := bip39.MnemonicSeed(m, password)
	defer clear(seed)
	if seedHook != nil {
		seedHook(seed)
	}
	mk, err := hdkeychain.NewMaster(seed, net)
	if err != nil {
		return nil, false
	}
	return mk, true
}

// MasterFingerprint returns the fingerprint of the master key mk and zeroes mk
// on every exit. The fingerprint is a uint32 computed from the PUBLIC key
// before the deferred Zero runs, so the zeroing cannot race the result.
func MasterFingerprint(mk *hdkeychain.ExtendedKey) (uint32, error) {
	defer mk.Zero()
	pkey, err := mk.ECPubKey()
	if err != nil {
		return 0, err
	}
	return Fingerprint(pkey), nil
}

// MnemonicFingerprint is the master fingerprint of m under password, with the
// BIP-39 seed and the master private key both wiped before it returns.
func MnemonicFingerprint(m bip39.Mnemonic, net *chaincfg.Params, password string) (uint32, error) {
	mk, ok := MasterKey(m, net, password, nil)
	if !ok {
		return 0, errors.New("failed to derive mnemonic master key")
	}
	return MasterFingerprint(mk)
}
