package gui

import "fmt"

// ─── T6b: synthesize the engrave cards for a multisig bundle ────────────────
//
// F-189, LANDED: the one-of-each adapter multisigEngraveCards is DELETED. It had
// no production caller after F-188 -- the supply path derives a leg per matched
// slot and emits through multisigEngraveCardsMulti directly -- and was kept
// "filed rather than deleted" with its test. A retired emitter is an invitation
// to reintroduce the rule it encoded: exactly that happened once already in this
// file's neighbourhood, where a review proposed relaxing a verify rule the
// deleted symmetry made look optional. The SHAPE it pinned (full = ms1, mk1,
// md1; watch-only = mk1, md1) is real and is not lost -- TestMultisigEngraveCards
// now asserts it against the surviving producer, which is the one an operator's
// plates actually come out of.
//
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
