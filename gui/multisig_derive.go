package gui

import (
	"errors"
	"fmt"

	"github.com/btcsuite/btcd/chaincfg/v2"
	"seedhammer.com/bip32"
	"seedhammer.com/bip39"
	"seedhammer.com/bundle"
	"seedhammer.com/codex32"
	"seedhammer.com/md"
	"seedhammer.com/mk"
)

// ─── T6b: derive the operator's leg of a SUPPLIED multisig bundle ────────────
//
// deriveMultisigLeg builds the operator's mk1 (policy-bound to the SUPPLIED md1)
// and ms1 (full only) for the matched slot. The md1 leg is the SUPPLIED chunk
// strings VERBATIM (I-2 — the device never re-encodes a multisig descriptor).
//
// mk1.Path is the matched slot origin (so compactFromXpub's depth/child gate
// passes — the xpub was derived AT this origin). mk1.Stubs is the policy_id_stub
// of the SUPPLIED md1 (I-4 — binds the key card to the supplied policy).
// Network is a LABEL only ("mainnet"); it is not serialized nor verified.
//
// SECURITY: gate m.Valid() before m.Entropy() (which panics on invalid);
// deriveAccountXpub scrubs the seed/master internally; the entropy buffer is
// wiped after ms1 encode. The caller scrubs the mnemonic []Word.
var errMultisigInvalidSeed = errors.New("multisig: invalid seed mnemonic")

func deriveMultisigLeg(m bip39.Mnemonic, passphrase string, net *chaincfg.Params, origin bip32.Path, suppliedMd1 []string, full bool) (bundle.Bundle, error) {
	if !m.Valid() {
		return bundle.Bundle{}, errMultisigInvalidSeed
	}

	xpub, masterFP, err := deriveAccountXpub(m, passphrase, net, origin)
	if err != nil {
		return bundle.Bundle{}, err
	}

	stub, err := md.WalletPolicyIDStubChunks(suppliedMd1)
	if err != nil {
		return bundle.Bundle{}, err
	}

	mk1, err := mk.Encode(mk.Card{
		Network:     "mainnet", // LABEL only (mainnet-only, I-8).
		Path:        origin.String(),
		Fingerprint: fmt.Sprintf("%08x", masterFP),
		Stubs:       [][4]byte{stub},
		Xpub:        xpub,
	})
	if err != nil {
		return bundle.Bundle{}, err
	}

	// md1 leg = the SUPPLIED strings VERBATIM (clone so the caller's slice can't
	// be mutated downstream).
	md1 := append([]string(nil), suppliedMd1...)

	b := bundle.Bundle{MK1: mk1, MD1: md1}
	if full {
		entropy := m.Entropy()
		ms1, err := codex32.EncodeMS1(entropy)
		wipeBytes(entropy)
		if err != nil {
			return bundle.Bundle{}, err
		}
		b.MS1 = ms1
	}
	return b, nil
}
