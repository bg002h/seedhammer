package gui

import (
	"bytes"

	"github.com/btcsuite/btcd/chaincfg/v2"
	"seedhammer.com/bip32"
	"seedhammer.com/bip39"
	"seedhammer.com/md"
)

// ─── T6b: the D14 slot cross-match (the wrong-wallet guard) ──────────────────
//
// findUserSlot derives the operator's OWN account key from the TYPED seed at
// EACH xpub-present slot's own origin and matches it against that slot's
// embedded key on the CANONICAL (chainCode, compressedPubkey) pair — NEVER
// base58 (the supplied xpub carries different parentFP/depth metadata) and
// NEVER == on mismatched array/slice types (I-2). It returns the matched slot's
// index + origin.
//
// Outcomes:
//   - exactly one match  -> (index, origin, nil, true)
//   - zero matches       -> (_, _, _, false): REFUSE (the seed is not a cosigner;
//                           never engrave a backup for a wallet you are not in)
//   - >=2 matches        -> the SAME seed legitimately appears at >=2 cosigner
//                           slots under DISTINCT origins. Return the FIRST-by-index
//                           slot (deterministic; policy+stub identical across
//                           slots, only the mk1 Path differs) + every matched
//                           index in `reused` so the caller can show a notice.
//
// SECURITY: deriveAccountXpub scrubs its own seed/master/intermediates on every
// call; the caller scrubs the mnemonic []Word after the LAST derive here (the
// loop may derive at several slots before matching).
func findUserSlot(m bip39.Mnemonic, passphrase string, net *chaincfg.Params, keys []md.ExpandedKey) (slotIndex int, origin bip32.Path, reused []int, ok bool) {
	var matches []int
	for i, k := range keys {
		if !k.XpubPresent {
			continue
		}
		xpub, _, err := deriveAccountXpub(m, passphrase, net, k.OriginPath)
		if err != nil {
			continue // a malformed origin can't be the operator's slot.
		}
		cc, pk, _, err := decodeXpubBytes(xpub)
		if err != nil {
			continue
		}
		if bytes.Equal(cc[:], k.Xpub[0:32]) && bytes.Equal(pk[:], k.Xpub[32:65]) {
			matches = append(matches, i)
		}
	}
	if len(matches) == 0 {
		return 0, nil, nil, false
	}
	first := matches[0]
	if len(matches) >= 2 {
		return first, keys[first].OriginPath, matches, true
	}
	return first, keys[first].OriginPath, nil, true
}
