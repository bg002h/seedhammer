package gui

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/btcsuite/btcd/btcutil/v2/hdkeychain"
	"github.com/btcsuite/btcd/chaincfg/v2"
	"seedhammer.com/bip32"
	"seedhammer.com/bip39"
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

// composerSeedHook is a TEST-OBSERVATION seam, and nothing else. It fires
// once, right after the seed entry returns, so a test can capture the words
// before they are scrubbed. It is nil in production and IT SCRUBS NOTHING --
// exactly as buildMultisigSeedHook does not (gui/multisig_build.go:36-38).
//
// THE SCRUB IS THE REGISTRY'S, AND IT IS ONE SITE. composerFlow installs
// `defer st.reg.scrub()` at the top, before any seed exists, so every exit --
// a Back, a refusal screen, a ctx.Done unwind, a panic -- is covered by
// construction rather than by an implementer remembering to add a scrub to a
// new return. Copying this hook and not that defer would copy the wrong half.
var composerSeedHook func(bip39.Mnemonic)

// composerSeedSource takes a seed and registers it.
//
// The payload is offered before the keyboard, because §3.3.2 now admits
// ClassMnemonic here (Task A3) and seedEntryFlowTitled is the shared seam
// that does the offering. The passphrase is asked PER SEED, at that seed's
// entry (SPEC 4.1's rule, which buildSeedForSlot states at
// gui/multisig_build.go:725-737): one flow-global passphrase applied to N
// seeds would mint keys the operator can only re-derive with a pairing they
// never chose.
func composerSeedSource(ctx *Context, th *Colors, st *composerState) (composerSource, bool) {
	label := fmt.Sprintf("seed %d", st.reg.count()+1)
	mnemonic, ok := seedEntryFlowTitled(ctx, th, "Seed for the policy", label)
	if !ok {
		return composerSource{}, false
	}
	if composerSeedHook != nil {
		composerSeedHook(mnemonic)
	}
	// Registered IMMEDIATELY, before the passphrase screens can return early:
	// from this line the deferred scrub owns these words.
	seedID, err := st.reg.add(label, mnemonic, "", &chaincfg.MainNetParams)
	if err != nil {
		showError(ctx, th, "Seed", "Couldn't read that seed.")
		return composerSource{}, false
	}
	pp := &ChoiceScreen{
		Title:   "Passphrase " + label,
		Lead:    "Add a BIP-39 passphrase?",
		Choices: []string{"Skip", "Add passphrase"},
	}
	if sel, ok := pp.Choose(ctx, th); ok && sel == 1 {
		if pass, ok := syswPassphraseFlowTitled(ctx, th, "Passphrase "+label); ok {
			if err := st.reg.bindPassphrase(seedID, pass, &chaincfg.MainNetParams); err != nil {
				showError(ctx, th, "Seed", "Couldn't apply that passphrase.")
				return composerSource{}, false
			}
		}
	}
	seed, _ := st.reg.at(seedID)
	var fp [4]byte
	binary.BigEndian.PutUint32(fp[:], seed.MasterFP)
	return composerSource{
		kind: composerSourceSeed, label: label,
		fingerprint: fp, fpPresent: true, seedID: seedID,
	}, true
}

// composerSeedAccountFor is §4f's account rule: the slot's ordinal among the
// slots THIS MASTER fills, in ascending emitted slot index.
//
// KEYED ON THE MASTER, NOT THE SEED ID, and buildSlotSources states the
// reason at gui/multisig_build.go:593-601: keying on the id would mint the
// SAME key twice whenever one master was registered twice -- which is what an
// operator does when they type one seed for two slots -- and md's duplicate
// key refusal would then reject a legitimate multi-account wallet.
// srcIdx indexes st.sources, NOT composerSource.seedID -- two different
// numbers on the same struct, and the parameter used to carry the other one's
// name while every caller passed this one.
func composerSeedAccountFor(st *composerState, slot uint8, srcIdx int) uint32 {
	want := st.sources[srcIdx].fingerprint
	n := uint32(0)
	for i := 0; i < int(slot) && i < len(st.assigned); i++ {
		a := st.assigned[i]
		if a.src < 0 || a.src >= len(st.sources) {
			continue
		}
		s := st.sources[a.src]
		if s.kind == composerSourceSeed && s.fingerprint == want {
			n++
		}
	}
	return n
}

// composerSeedDerive fills one assignment from a seed at its own account.
func composerSeedDerive(st *composerState, slot uint8, srcIdx int) (composerAssignment, error) {
	src := st.sources[srcIdx]
	account := composerSeedAccountFor(st, slot, srcIdx)
	origin := md.DefaultOrigin(st.list.Wrapper, account)
	seed, ok := st.reg.at(src.seedID)
	if !ok {
		return composerAssignment{}, errors.New("composer: no seed for that slot")
	}
	path := make(bip32.Path, 0, len(origin))
	for _, c := range origin {
		v := c.Value
		if c.Hardened {
			v += hdkeychain.HardenedKeyStart
		}
		path = append(path, v)
	}
	xpub, masterFP, err := deriveAccountXpub(seed.Mnemonic, seed.Passphrase, &chaincfg.MainNetParams, path)
	if err != nil {
		return composerAssignment{}, err
	}
	var fp [4]byte
	binary.BigEndian.PutUint32(fp[:], masterFP)
	return composerAssignment{
		src: srcIdx, account: account, origin: origin,
		fingerprint: fp, fpPresent: true, xpub: xpub,
	}, nil
}
