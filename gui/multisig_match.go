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
//     never engrave a backup for a wallet you are not in)
//   - >=2 matches        -> the SAME seed legitimately appears at >=2 cosigner
//     slots under DISTINCT origins, holding a DIFFERENT key
//     at each. Return the FIRST-by-index slot
//     (deterministic) + every matched index in `reused`.
//
// USE allUserSlots WHEN THE QUESTION IS "WHICH SLOTS", NOT "WHICH SLOT". This
// function answers the membership question -- is this seed in the policy at all,
// and where does the first match live -- which is what the build path's slot gate
// asks (gui/multisig_build_slots.go). It is the WRONG question for anything that
// engraves or verifies: F-188's supply path cuts a plate per matched slot and
// takes its list from allUserSlots, because "the first match" is what made the
// engrave and the verify disagree in the first place.
//
// `reused` HAS NO PRODUCTION CONSUMER since F-188. It fed the "This key is
// reused at slots ..." notice, which was false -- the keys at those slots are
// different keys at different origins -- and the flow that showed it now
// engraves all of them instead.
//
// SECURITY: deriveAccountXpub scrubs its own seed/master/intermediates on every
// call; the caller scrubs the mnemonic []Word after the LAST derive here (the
// loop may derive at several slots before matching).
func findUserSlot(m bip39.Mnemonic, passphrase string, net *chaincfg.Params, keys []md.ExpandedKey) (slotIndex int, origin bip32.Path, reused []int, ok bool) {
	matches := allUserSlots(m, passphrase, net, keys)
	if len(matches) == 0 {
		return 0, nil, nil, false
	}
	first := matches[0]
	if len(matches) >= 2 {
		return first, keys[first].OriginPath, matches, true
	}
	return first, keys[first].OriginPath, nil, true
}

// allUserSlots reports EVERY slot the (seed, passphrase) pair accounts for, in
// ascending slot order. It is findUserSlot's loop, extracted, and findUserSlot
// is now a thin wrapper over it -- so the comparison rule that matters
// (canonical chainCode ‖ compressedPubkey, NEVER base58, NEVER `==` on
// mismatched array/slice types) has exactly ONE site. Two copies of a
// funds-safety comparison is how the two come apart.
//
// IT EXISTS BECAUSE "THE FIRST MATCH" MADE THE VERIFY STRUCTURALLY SINGLE-LEG.
// findUserSlot returns matches[0], which is the right answer for the question
// it was written for ("which slot is the operator in") and the wrong one for
// S5's ("which slots does this seed have to prove"). Trace B holds master A at
// @0 AND @1, so a verify built on the first match can never re-derive the
// second -- it would check one of three engraved plates and report Verify OK.
//
// A slot with no xpub is skipped rather than refused: a keyless template has
// nothing to match against, and that is D1's business rather than this
// function's. A malformed origin is skipped for the same reason it is in
// findUserSlot -- a path this device cannot parse is not a path it derived at.
func allUserSlots(m bip39.Mnemonic, passphrase string, net *chaincfg.Params, keys []md.ExpandedKey) []int {
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
	return matches
}
