package gui

import "seedhammer.com/bundle"

// ─── T6a-2: synthesize the engrave cards from the derived single-sig bundle ───
//
// singleSigEngraveCards turns the derived bundle into the []bundleCard the T5
// bundleEngrave sequences. The device DERIVED the ms1, so full mode ENGRAVES it
// (the flagship's purpose): full → [ms1, mk1, md1]; watch-only → [mk1, md1] (the
// ms1 is left for the operator to hand-engrave, and bundleEngrave shows the
// reminder via the cards-derived gate, R0-I2).
//
// Each card carries the derived strings VERBATIM (I-4) — no re-encode. The ms1
// is a single-string, single-plate card (its codex32 string engraves as one
// plate via validateMdmk, format-agnostic).
//
// SECURITY: the ms1 string is SECRET; it is engraved onto owner-held steel only,
// never sent over NFC. (The immutable-string residual until GC is accepted,
// consistent with the shipped ms1-display posture.)
func singleSigEngraveCards(b bundle.Bundle, full bool) []bundleCard {
	var cards []bundleCard
	if full {
		cards = append(cards, bundleCard{
			kind:    cardMS1,
			label:   "ms1 secret share",
			strings: []string{b.MS1},
			summary: "secret seed backup",
		})
	}
	cards = append(cards,
		bundleCard{
			kind:    cardMK1,
			label:   "mk1 key",
			strings: append([]string(nil), b.MK1...),
			summary: "account key card",
		},
		bundleCard{
			kind:    cardMD1,
			label:   "md1 descriptor",
			strings: append([]string(nil), b.MD1...),
			summary: "wallet policy descriptor",
		},
	)
	return cards
}
