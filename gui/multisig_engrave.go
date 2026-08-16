package gui

import "fmt"

// ─── T6b: synthesize the engrave cards for a SUPPLIED multisig bundle ────────
//
// multisigEngraveCards mirrors singleSigEngraveCards (gui/singlesig_engrave.go):
// full -> [ms1, mk1, md1]; watch-only -> [mk1, md1] (the ms1 is left for the
// operator to hand-engrave; bundleEngrave shows the reminder via the
// cards-derived gate). The md1 strings are the SUPPLIED policy VERBATIM (I-2).
// The ms1 is a single-string, single-plate SECRET card, engraved onto
// owner-held steel only — never NFC.
//
// It is now a one-of-each adapter over multisigEngraveCardsMulti, so the SUPPLY
// path (gui/multisig.go) keeps byte-identical behaviour while the BUILD path
// gets S5's several mk1s and several ms1s. `full` is the presence of an ms1,
// which is what the old signature encoded in a separate bool.
func multisigEngraveCards(ms1 string, mk1, md1 []string, full bool) []bundleCard {
	var ms1s []string
	if full {
		ms1s = []string{ms1}
	}
	return multisigEngraveCardsMulti(ms1s, [][]string{mk1}, md1)
}

// multisigEngraveCardsMulti is S5's engrave set: EVERY ms1, then EVERY mk1, then
// the md1.
//
// THE ORDER IS A CONTRACT, not a rendering choice. oracle.ArtifactKindsFor
// declares built-policy-full as ["ms1","mk1","md1"] and
// oracle.CheckArtifactShape requires those kinds to arrive as CONSECUTIVE
// non-empty runs in exactly that sequence, so a set that interleaved an mk1
// between two ms1s would be a different restore than the one the inputs
// describe — and would fail shape rather than fail quietly. Trace B is the shape
// that makes this reachable: three held slots across two masters is 2 ms1s + 3
// mk1s + 1 md1.
//
// The single-card labels are UNCHANGED from T6b, so a one-leg build reads
// exactly as it always did; only a set with several of a kind numbers them.
func multisigEngraveCardsMulti(ms1s []string, mk1s [][]string, md1 []string) []bundleCard {
	cards := make([]bundleCard, 0, len(ms1s)+len(mk1s)+1)
	for i, s := range ms1s {
		cards = append(cards, bundleCard{
			kind:    cardMS1,
			label:   numberedLabel("ms1 secret share", i, len(ms1s)),
			strings: []string{s},
			summary: "secret seed backup",
		})
	}
	for i, k := range mk1s {
		cards = append(cards, bundleCard{
			kind:    cardMK1,
			label:   numberedLabel("mk1 key", i, len(mk1s)),
			strings: append([]string(nil), k...),
			summary: "account key card",
		})
	}
	cards = append(cards, bundleCard{
		kind:    cardMD1,
		label:   "md1 descriptor",
		strings: append([]string(nil), md1...),
		summary: "wallet policy descriptor",
	})
	return cards
}

// numberedLabel names one card of a kind. A lone card keeps the shipped label
// verbatim (the plate census and the restore-doc inventory both print it, and a
// "1 of 1" would be noise); several get a 1-based index, because an operator
// holding three mk1 plates needs to know which one the machine is cutting.
func numberedLabel(base string, i, n int) string {
	if n <= 1 {
		return base
	}
	return fmt.Sprintf("%s %d of %d", base, i+1, n)
}
