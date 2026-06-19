package gui

import (
	"github.com/btcsuite/btcd/btcutil/v2/hdkeychain"
	"github.com/btcsuite/btcd/chaincfg/v2"
	"seedhammer.com/bip32"
	"seedhammer.com/bip39"
)

// deriveAccountXpub derives the account xpub at path from a SECRET mnemonic +
// optional passphrase, scrubbing all private material as it goes: the 64-byte
// PBKDF2 seed buffer, the master key, and every intermediate ExtendedKey. It
// returns the base58 account xpub (PUBLIC) plus the master-key fingerprint.
//
// SECURITY: this is a backup/engraving path, never a signer. The account key is
// neutered (.Neuter) so NO xprv / private material is ever serialized. The only
// output is the public xpub. The caller is responsible for scrubbing the
// mnemonic []Word once the flow completes (see deriveXpubFlow).
func deriveAccountXpub(m bip39.Mnemonic, passphrase string, net *chaincfg.Params, path bip32.Path) (xpub string, masterFP uint32, err error) {
	seed := bip39.MnemonicSeed(m, passphrase)
	defer wipeBytes(seed)
	master, err := hdkeychain.NewMaster(seed, net)
	if err != nil {
		return "", 0, err
	}
	pk, err := master.ECPubKey()
	if err != nil {
		master.Zero()
		return "", 0, err
	}
	masterFP = bip32.Fingerprint(pk) // capture BEFORE zeroing master
	k := master
	for _, c := range path {
		next, derr := k.Derive(c)
		k.Zero() // scrub master + each intermediate (Derive returns fresh buffers, no aliasing)
		if derr != nil {
			return "", 0, derr
		}
		k = next
	}
	acct, err := k.Neuter() // public-only
	if err != nil {
		k.Zero()
		return "", 0, err
	}
	// R0-C1 (CRITICAL): Neuter ALIASES k's chainCode/parentFP by reference, so
	// serialize the xpub BEFORE zeroing k — otherwise acct.String() reads zeroed
	// buffers and emits a silently-wrong-but-valid xpub (the WRONG key on a
	// permanent backup). Do not reorder.
	xpub = acct.String()
	k.Zero() // now safe — scrubs the final private account key
	return xpub, masterFP, nil
}
