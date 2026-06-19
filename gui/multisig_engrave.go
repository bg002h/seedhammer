package gui

// ─── T6b: synthesize the engrave cards for a SUPPLIED multisig bundle ────────
//
// multisigEngraveCards mirrors singleSigEngraveCards (gui/singlesig_engrave.go):
// full -> [ms1, mk1, md1]; watch-only -> [mk1, md1] (the ms1 is left for the
// operator to hand-engrave; bundleEngrave shows the reminder via the
// cards-derived gate). The md1 strings are the SUPPLIED policy VERBATIM (I-2).
// The ms1 is a single-string, single-plate SECRET card, engraved onto
// owner-held steel only — never NFC.
func multisigEngraveCards(ms1 string, mk1, md1 []string, full bool) []bundleCard {
	var cards []bundleCard
	if full {
		cards = append(cards, bundleCard{
			kind:    cardMS1,
			label:   "ms1 secret share",
			strings: []string{ms1},
			summary: "secret seed backup",
		})
	}
	cards = append(cards,
		bundleCard{
			kind:    cardMK1,
			label:   "mk1 key",
			strings: append([]string(nil), mk1...),
			summary: "account key card",
		},
		bundleCard{
			kind:    cardMD1,
			label:   "md1 descriptor",
			strings: append([]string(nil), md1...),
			summary: "wallet policy descriptor",
		},
	)
	return cards
}
