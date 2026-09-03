package gui

import (
	"errors"
	"fmt"

	"seedhammer.com/md"
	"seedhammer.com/mk"
)

// Minting and re-minting the composer's key cards (SPEC §7d, §7f).
//
// EVERY SEATED SLOT YIELDS A CARD IN FORM B, whatever it was seated from: a
// key: record is MINTED as an mk1, a payload mk1 is RE-MINTED with both stubs
// APPENDED to the ones it already carries, and a seed-derived slot is minted
// likewise. Appending rather than replacing is what lets one card seat into
// either engraved form AND stay indexed to the wallets it already belonged to
// (§7d) -- reStubMk1 (gui/template_engrave.go:41-48) REPLACES, which is right
// for its own flow and wrong here.
//
// mk.Encode is deterministic (mk/encode.go:39), so a re-mint is exact.

// composerMintCard builds the mk1 for one seated slot.
//
// `keyedChunks` is nil for a partially seated composition, and md.ComposerStubs
// then returns the TEMPLATE stub alone -- which is §7f's rule, because the
// policy id does not exist until every slot is seated.
func composerMintCard(st *composerState, slot uint8, templateChunks, keyedChunks []string) ([]string, error) {
	if int(slot) >= len(st.assigned) {
		return nil, errors.New("composer: no such slot")
	}
	a := st.assigned[slot]
	if a.src < 0 {
		return nil, fmt.Errorf("composer: slot @%d is unseated and has no card", slot)
	}
	stubs, err := md.ComposerStubs(templateChunks, keyedChunks)
	if err != nil {
		return nil, err
	}
	src := st.sources[a.src]
	card := mk.Card{
		Network:     "mainnet", // LABEL only: mainnet by construction (§4f, gui/policy_address.go:61).
		Path:        composerOriginText(a.origin),
		Fingerprint: fmt.Sprintf("%x", a.fingerprint),
		Xpub:        a.xpub,
	}
	if src.kind == composerSourceCard {
		// RE-MINT: the payload card verbatim, with its own stubs kept in order
		// and the composer's appended.
		card = src.card
		card.Path = composerOriginText(a.origin)
	}
	return mk.Encode(mk.AppendStubs(card, stubs...))
}

// composerMintCards mints every seated slot's card, in emitted slot order --
// which is the order the census lists them and the order the restore document
// reads.
func composerMintCards(st *composerState, templateChunks, keyedChunks []string) ([]bundleCard, error) {
	var out []bundleCard
	for i := range st.assigned {
		if st.assigned[i].src < 0 {
			continue
		}
		strs, err := composerMintCard(st, uint8(i), templateChunks, keyedChunks)
		if err != nil {
			return nil, err
		}
		out = append(out, bundleCard{
			kind:    cardMK1,
			label:   fmt.Sprintf("mk1 key @%d", i),
			strings: strs,
			summary: composerOriginText(st.assigned[i].origin),
		})
	}
	return out, nil
}
