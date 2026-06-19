package gui

import (
	"errors"
	"fmt"

	"github.com/btcsuite/btcd/btcutil/v2/hdkeychain"
	"github.com/btcsuite/btcd/chaincfg/v2"
	"seedhammer.com/bip39"
	"seedhammer.com/bip85"
)

// validBip85Words is the set of child word counts the BIP-39 application
// supports (biptool's guard n<12||24<n||n%3!=0 -> exactly {12,18,24}).
func validBip85Words(n int) bool {
	return n == 12 || n == 18 || n == 24
}

// deriveBip85Child re-creates biptool's `derive bip39` (cmd/biptool/main.go:137-189)
// from a TYPED master mnemonic + optional passphrase: it walks the FULLY-hardened
// BIP-85 path m/83696968'/39'/0'/{words}'/{index}', extracts the leaf's 32-byte EC
// private key, runs bip85.Entropy (HMAC-SHA512), keeps the LEADING entLen bytes,
// and maps them to a child BIP-39 mnemonic via bip39.New.
//
// SECURITY: every secret buffer is scrubbed before return — the PBKDF2 seed, each
// intermediate ExtendedKey (.Zero), the privkey serialization, and the 64-byte
// HMAC output (wipeBytes). The caller still owns scrubbing the master and the
// returned child mnemonic (see bip85DeriveFlow). Deterministic: no CSPRNG.
func deriveBip85Child(m bip39.Mnemonic, passphrase string, words, index int) (bip39.Mnemonic, error) {
	if !validBip85Words(words) {
		return nil, fmt.Errorf("bip85: invalid child word count: %d", words)
	}
	if index < 0 {
		return nil, fmt.Errorf("bip85: invalid index: %d", index)
	}

	const h = hdkeychain.HardenedKeyStart
	seed := bip39.MnemonicSeed(m, passphrase)
	defer wipeBytes(seed)

	xkey, err := hdkeychain.NewMaster(seed, &chaincfg.MainNetParams)
	if err != nil {
		return nil, err
	}
	// Fully-hardened path: 83696968' / 39' / 0' (English) / words' / index'.
	path := []uint32{
		bip85.PathRoot,
		39 + h,
		0 + h,
		uint32(words) + h,
		uint32(index) + h,
	}
	k := xkey
	for _, p := range path {
		next, derr := k.Derive(p)
		k.Zero() // scrub master + each intermediate (Derive returns fresh buffers)
		if derr != nil {
			return nil, derr
		}
		k = next
	}
	// Leaf EC private key. ECPrivKey returns (*PrivateKey, error); it cannot fire
	// for a master+hardened walk, but never .Serialize() a nil.
	pkey, err := k.ECPrivKey()
	if err != nil {
		k.Zero()
		return nil, err
	}
	priv := pkey.Serialize() // 32-byte secret
	k.Zero()
	defer wipeBytes(priv)

	hmacOut := bip85.Entropy(priv) // 64-byte secret
	defer wipeBytes(hmacOut)

	entLen := (words*11 - words/3) / 8 // 12->16, 18->24, 24->32
	if entLen < 16 || entLen > 32 || entLen%4 != 0 {
		// Unreachable for words in {12,18,24}; guard so bip39.New never panics.
		return nil, errors.New("bip85: internal entropy-length error")
	}
	child := bip39.New(hmacOut[:entLen]) // LEADING entLen bytes
	return child, nil
}
