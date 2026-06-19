package gui

import (
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/btcsuite/btcd/btcutil/v2/hdkeychain"
	"github.com/btcsuite/btcd/chaincfg/v2"
	"seedhammer.com/bip32"
	"seedhammer.com/bip39"
	"seedhammer.com/bundle"
	"seedhammer.com/codex32"
	"seedhammer.com/md"
	"seedhammer.com/mk"
)

// ─── T6a-2: derive the single-sig constellation trio (ms1 + mk1 + md1) ────────
//
// deriveSingleSigBundle turns ONE typed seed into the three single-sig
// constellation legs, with the mk1's policy_id_stub POLICY-BOUND to the derived
// md1 (NON-ZERO; NOT T4's placeholder). It returns the bundle plus the account
// masterFP, the REAL parent fingerprint of the account xpub (R0-I1 — captured in
// the same decode, threaded to the restore doc so its exported xpub is
// canonical), and the base58 account xpub.
//
// Order (recon Topic-4): xpub → md.EncodeSingleSig(md1) →
// md.WalletPolicyIDStubChunks(md1) → mk.Encode(stub). ms1 = EncodeMS1(entropy).
//
// SECURITY: the seed/master/intermediates are scrubbed INSIDE deriveAccountXpub.
// The entropy buffer is gated on mnemonic validity, then wiped on every exit
// path. The mnemonic []Word is the caller's to scrub (Task 7). The only public
// outputs are the bundle strings + masterFP/parentFP/xpub.

var errSingleSigInvalidSeed = errors.New("singlesig: invalid seed mnemonic")

func deriveSingleSigBundle(m bip39.Mnemonic, passphrase string, net *chaincfg.Params, path bip32.Path, script md.ScriptKind) (b bundle.Bundle, masterFP uint32, parentFP uint32, xpub string, err error) {
	// Gate Entropy() validity FIRST — it panics on an invalid mnemonic
	// (bip39.go:158). A typed seed should always be valid here, but never panic.
	if !m.Valid() {
		return bundle.Bundle{}, 0, 0, "", errSingleSigInvalidSeed
	}

	// (1) Account xpub (PUBLIC) + masterFP. deriveAccountXpub scrubs the seed,
	// master, and every intermediate key internally.
	xpub, masterFP, err = deriveAccountXpub(m, passphrase, net, path)
	if err != nil {
		return bundle.Bundle{}, 0, 0, "", err
	}

	// (2) Decode the xpub → (chainCode, compressedPubkey) AND the REAL parentFP,
	// in the SAME decode (R0-I1; mk/encode.go compactFromXpub pattern).
	chainCode, compressedPubkey, parentFP, err := decodeXpubBytes(xpub)
	if err != nil {
		return bundle.Bundle{}, 0, 0, "", err
	}

	// (3) md1 — EncodeSingleSig.fp = the account masterFP (NOT parentFP).
	var fp [4]byte
	binary.BigEndian.PutUint32(fp[:], masterFP)
	md1, err := md.EncodeSingleSig(chainCode, compressedPubkey, fp, originComponents(path), script)
	if err != nil {
		return bundle.Bundle{}, 0, 0, "", err
	}

	// (4) Policy-id stub from the md1 strings (POLICY-BOUND, non-zero).
	stub, err := md.WalletPolicyIDStubChunks(md1)
	if err != nil {
		return bundle.Bundle{}, 0, 0, "", err
	}

	// (5) mk1 — bound stub, NO stubZeroWarning. Reuse the account metadata.
	mk1, err := mk.Encode(mk.Card{
		Network:     networkName(net),
		Path:        path.String(),
		Fingerprint: fmt.Sprintf("%08x", masterFP),
		Stubs:       [][4]byte{stub},
		Xpub:        xpub,
	})
	if err != nil {
		return bundle.Bundle{}, 0, 0, "", err
	}

	// (6) ms1 — SECRET. Gate validity (done above), wipe the entropy after use.
	entropy := m.Entropy()
	ms1, err := codex32.EncodeMS1(entropy)
	wipeBytes(entropy)
	if err != nil {
		return bundle.Bundle{}, 0, 0, "", err
	}

	return bundle.Bundle{MS1: ms1, MK1: mk1, MD1: md1}, masterFP, parentFP, xpub, nil
}

// decodeXpubBytes parses a base58 account xpub into its 32-byte chain code,
// 33-byte compressed pubkey, and uint32 parent fingerprint, in a single
// hdkeychain decode (R0-I1; the mk/encode.go:117,161-164 pattern). Public-only —
// it refuses a private (xprv) input.
func decodeXpubBytes(xpub string) (chainCode [32]byte, compressedPubkey [33]byte, parentFP uint32, err error) {
	key, err := hdkeychain.NewKeyFromString(xpub)
	if err != nil {
		return chainCode, compressedPubkey, 0, err
	}
	if key.IsPrivate() {
		return chainCode, compressedPubkey, 0, errors.New("singlesig: refusing to encode a private key")
	}
	cc := key.ChainCode() // returns a copy
	if len(cc) != 32 {
		return chainCode, compressedPubkey, 0, errors.New("singlesig: bad chain code length")
	}
	copy(chainCode[:], cc)
	pub, err := key.ECPubKey()
	if err != nil {
		return chainCode, compressedPubkey, 0, err
	}
	pubBytes := pub.SerializeCompressed()
	if len(pubBytes) != 33 {
		return chainCode, compressedPubkey, 0, errors.New("singlesig: bad pubkey length")
	}
	copy(compressedPubkey[:], pubBytes)
	parentFP = key.ParentFingerprint()
	return chainCode, compressedPubkey, parentFP, nil
}

// originComponents converts a bip32.Path (in-band hardening: value +
// HardenedKeyStart) to the encoder's raw []md.PathComponent form (R0-M5: the
// encoder takes the raw component, NOT the in-band convention).
func originComponents(path bip32.Path) []md.PathComponent {
	comps := make([]md.PathComponent, len(path))
	for i, c := range path {
		hardened := c >= hdkeychain.HardenedKeyStart
		v := c
		if hardened {
			v -= hdkeychain.HardenedKeyStart
		}
		comps[i] = md.PathComponent{Hardened: hardened, Value: v}
	}
	return comps
}

// networkName maps the chaincfg params to the mk1 network label. The single-sig
// flagship is mainnet-only, but keep the testnet label for completeness.
func networkName(net *chaincfg.Params) string {
	if net == &chaincfg.MainNetParams {
		return "mainnet"
	}
	return "testnet"
}
