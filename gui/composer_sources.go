package gui

import (
	"encoding/hex"
	"fmt"

	"seedhammer.com/bip32"
	"seedhammer.com/md"
	"seedhammer.com/mk"
	"seedhammer.com/sysw"
)

// The composer's seatable keys (SPEC §7d, C8). The seed source and its
// per-slot account rule are appended to this file by the seed task, which is
// why the import block above already carries what that half needs.
//
// THE COMPOSER DOES NOT CALL seatKeyCards, and §7d says why: that function
// seats a template that ALREADY declares its origins, by declaration match,
// for cards that ALREADY carry the template's stub (gui/key_card_seating.go
// :53-73). A composed template has no declarations yet and no card carries
// its stub, so layer 1 would refuse every card before an origin was ever
// compared. Seating here is SLOT-DIRECTED instead: the operator is asked, per
// emitted slot, which key goes in it, and seatKeyCards is what verifies the
// result afterwards (§12 item 6).
//
// CARD STUBS ARE IGNORED AT SEATING for the same reason. They are APPENDED
// when the card is re-minted, so one card seats into either engraved form and
// stays indexed to the wallets it already belonged to.
//
// THIS FILE CONSUMES FROM THE PAYLOAD, so its two functions are registered in
// gui/sysw_admit_oracle_test.go's syswConsumers and each HARD-CODES the one
// class it admits (§13 D7). A site that computed its class could not be
// reconciled against §3.3.2 at all.

// composerKeySources reads every key: record the payload holds.
//
// takeAll, not take: a composition seats a SET, and first-match would hand
// the flow one key for a four-slot policy. It inherits takeAll's refusal on
// an uncompared payload, which the door deliberately does not (the door
// counts through has()).
func composerKeySources(ctx *Context) []composerSource {
	if ctx.sysw == nil {
		return nil
	}
	records, ok := ctx.sysw.takeAll(sysw.ClassKey)
	if !ok {
		return nil
	}
	out := make([]composerSource, 0, len(records))
	for _, r := range records {
		kr, err := sysw.ParseKeyRecord(r)
		if err != nil {
			// Unreachable: a record that does not parse is ClassUnknown and
			// inert. Never consume a value from a call that returned an error.
			continue
		}
		out = append(out, composerSource{
			kind:        composerSourceKey,
			label:       composerKeyLabel(kr.Fingerprint, kr.Origin),
			fingerprint: kr.Fingerprint,
			fpPresent:   true,
			origin:      originComponents(kr.Origin),
			xpub:        kr.Xpub,
			seedID:      -1,
		})
	}
	return out
}

// composerCardSources reads every mk1 card the payload holds.
//
// cardSet, not takeAll: a card is a chunk SET, and one record of it completes
// nothing (F-76). cardSet groups the chunks so each card decodes.
func composerCardSources(ctx *Context) []composerSource {
	if ctx.sysw == nil {
		return nil
	}
	records, ok := ctx.sysw.cardSet(sysw.ClassMDMK)
	if !ok {
		return nil
	}
	var out []composerSource
	// A card's chunks are contiguous in `records` after grouping; mk.Decode
	// takes a complete set in any order and refuses an incomplete one, so a
	// growing window that decodes is exactly one card.
	for start := 0; start < len(records); {
		end := start + 1
		var card mk.Card
		decoded := false
		for ; end <= len(records); end++ {
			c, err := mk.Decode(records[start:end])
			if err == nil {
				card, decoded = c, true
				break
			}
		}
		if !decoded {
			// A record set that never decodes is an md1 card or a partial mk1;
			// neither is a seatable key. Advance by one rather than stopping,
			// so one unusable record cannot hide the cards after it.
			start++
			continue
		}
		path, err := bip32.ParsePath(card.Path)
		if err != nil {
			start = end
			continue
		}
		var fp [4]byte
		fpPresent := false
		if raw, err := hex.DecodeString(card.Fingerprint); err == nil && len(raw) == 4 {
			copy(fp[:], raw)
			fpPresent = true
		}
		out = append(out, composerSource{
			kind:        composerSourceCard,
			label:       composerKeyLabel(fp, path),
			fingerprint: fp,
			fpPresent:   fpPresent,
			origin:      originComponents(path),
			xpub:        card.Xpub,
			card:        card,
			seedID:      -1,
		})
		start = end
	}
	return out
}

// composerKeyLabel is §7d's label: fingerprint AND origin.
//
// BOTH, because two keys sharing a fingerprint is the NORMAL case (C5: one
// person in two paths holds two accounts from one master), and a fingerprint
// alone would render them identically on the one screen whose job is to tell
// them apart.
func composerKeyLabel(fp [4]byte, origin bip32.Path) string {
	return fmt.Sprintf("%x %s", fp, origin)
}

// composerSourceRow is one pick-list row. A used source is not offered again
// (C8's "remaining"); a SEED is never used up (C12), so its row stays.
func composerSourceRow(s composerSource) string {
	if s.kind == composerSourceSeed {
		return s.label + "  (any slots)"
	}
	return s.label
}

// composerSeatPrompt is §8s's prompt for one emitted slot.
//
// "Path N" IS THE OPERATOR'S LISTED PATH INDEX, never an emitted leaf index
// (§7d, stated twice there). Under tr the internal key is extracted as @0 and
// spends alone, which gets its own prompt.
func composerSeatPrompt(st *composerState, slot uint8) string {
	path, keyIdx, keyCount, keyPath := composerSlotPosition(st.list, slot)
	if keyPath {
		return composerCopySeatKeyPathPrompt(slot)
	}
	return composerCopySeatPrompt(slot, path, keyIdx, keyCount)
}

// composerSlotPosition maps an EMITTED slot index back to the operator's
// path, and reports whether it is the extracted taproot internal key.
//
// The emitted numbering is §5's: by first appearance in the emitted text,
// with an extracted internal key at @0. So under tr the FIRST-LISTED
// unlocked, unhashed one-key path becomes @0 and is no longer a leaf, and
// every other slot shifts. This walks the same rule rather than guessing it,
// which is why any edit that could move it discards assignments (§8j).
func composerSlotPosition(list md.PathList, slot uint8) (path, keyIdx, keyCount int, keyPath bool) {
	order := composerSlotOrder(list)
	if int(slot) >= len(order) {
		return 0, 0, 0, false
	}
	p := order[slot]
	return p.path, p.keyIdx, p.keyCount, p.keyPath
}

type composerSlotPos struct {
	path, keyIdx, keyCount int
	keyPath                bool
}

// composerSlotOrder lists, per emitted slot index, which of the operator's
// paths and which key within it that slot is.
//
// IT MUST AGREE WITH md.Compose's numbering. It is checked against
// md.Composed.Slots() by TestComposerSlotOrderAgreesWithTheCodec below, so a
// divergence is a test failure rather than a wrong prompt beside a right
// slot -- which is the shape that seats a key into the wrong seat silently.
func composerSlotOrder(list md.PathList) []composerSlotPos {
	var out []composerSlotPos
	internal := -1
	if list.Wrapper == md.ComposeTr {
		for i, p := range list.Paths {
			if p.Keys != nil && p.Keys.N == 1 && p.Lock == nil && p.Hash == nil {
				internal = i
				break
			}
		}
	}
	if internal >= 0 {
		out = append(out, composerSlotPos{path: internal + 1, keyIdx: 1, keyCount: 1, keyPath: true})
	}
	for i, p := range list.Paths {
		if i == internal || p.Keys == nil {
			continue
		}
		for k := 0; k < int(p.Keys.N); k++ {
			out = append(out, composerSlotPos{path: i + 1, keyIdx: k + 1, keyCount: int(p.Keys.N)})
		}
	}
	return out
}
