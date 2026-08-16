package gui

import (
	"errors"

	"github.com/btcsuite/btcd/chaincfg/v2"
	"seedhammer.com/bip32"
	"seedhammer.com/bundle"
	"seedhammer.com/md"
	"seedhammer.com/mk"
)

// ─── S5: the engrave tail ────────────────────────────────────────────────────
//
// Before S5 the tail derived ONE leg, at multisigSharedOrigin(), and engraved
// one mk1 and one ms1. Trace B holds three slots across two masters, so the tail
// owes the operator one mk1 PER HELD SLOT (derived at that slot's own origin)
// and one ms1 PER DISTINCT MASTER. Losing a master otherwise leaves a backup
// labelled "Full (seed + keys)" that cannot reconstruct the wallet.

// errBuildNoHeldSlot is a build in which the operator holds no slot at all.
// Nothing would be engraved for them, so there is nothing to derive; it is a
// structurally impossible assignment rather than an operator error, and it is
// refused rather than silently producing a keyless plate set.
var errBuildNoHeldSlot = errors.New("multisig build: no slot is held by this device")

// buildEngraveTail derives one leg per HELD slot and returns those legs plus the
// engrave card set.
//
// A slot is held when its source names a seed: `derived` (the key comes from a
// seed the operator entered) or `both` (the key is on a card AND the operator
// asserted it is theirs, which the gate has already proved). Its ORIGIN is the
// one that slot's key actually lives at:
//
//   - derived -> derivedSlotOrigin(script, account), §0.1a's template-aware
//     BIP-48 path at this slot's own account;
//   - both    -> the CARD's declared origin, because SPEC M-B makes the card
//     authoritative in a `both` slot.
//
// ONE ms1 PER DISTINCT SEED, keyed on THE ENGRAVED STRING and deliberately not
// on the registry entry: two slots held from one seed engrave two mk1s and ONE
// seed plate, because the second would be a duplicate secret on steel. The
// plan's C2 second scenario is the other direction and is the one that loses
// funds: a build across masters A and B in full mode must engrave BOTH ms1s.
//
// THE HELD SLOT INDICES LEAVE WITH THE LEGS (`slots`, parallel to `legs`), and
// they are the verify's obligation list. Only the engraver knows what it cut:
// this loop is the one place that decides which slots get a plate, and the
// verify's derive loop must restrict itself to exactly those (verifyFreshSlots).
// Re-deriving the set at the call site from `sources` would be a second copy of
// that rule, and two copies of a rule this one is how a verify starts asking for
// a plate the engrave never made.
func buildEngraveTail(sources []slotSource, script md.MultisigScript, reg *seedRegistry,
	net *chaincfg.Params, cards []mk.Card, engraveMd1 []string, full bool,
) ([]bundle.Bundle, []int, []bundleCard, error) {
	var (
		legs  []bundle.Bundle
		slots []int
		ms1s  []string
		mk1s  [][]string
	)
	// The seeds whose plate is already accounted for, keyed on THE ms1 STRING.
	//
	// NOT on s.SeedID, and the difference is a Critical this block shipped once:
	// buildMultisigPolicyFlow calls buildSeedForSlot once PER HELD SLOT
	// (gui/multisig_build.go:194-201) and buildSeedForSlot calls reg.add()
	// unconditionally (:510), so two slots held from the SAME words carry two
	// DIFFERENT SeedIDs. A SeedID-keyed dedupe therefore never fires for the shape
	// the product actually builds: Trace B full mode minted 3 seed plates for 2
	// seeds, numbered "1 of 3 / 2 of 3 / 3 of 3", telling an operator they hold
	// three distinct secrets when they hold two -- with the restore-doc inventory
	// vouching for the set.
	//
	// The ms1 string IS the identity of the secret the plate carries: two equal
	// strings encode identical entropy, so this key can never DROP a distinct
	// seed's only plate. That direction is the one that loses funds, which is
	// exactly why the master fingerprint is not the key either -- a 4-byte
	// collision there would silently omit a seed plate rather than duplicate one,
	// and a dedupe must only ever be able to fail SAFE.
	//
	// A passphrase does not split a plate: ms1 encodes the WORDS and never the
	// passphrase (which is why the plan requires the backup to say so out loud),
	// so one word-set engraved twice is a duplicate secret on steel with no
	// recovery benefit.
	engraved := map[string]bool{}
	for slot, s := range sources {
		var origin bip32.Path
		switch s.Kind {
		case slotFromSeed:
			origin = derivedSlotOrigin(script, s.Account)
		case slotFromBoth:
			if s.Card < 0 || s.Card >= len(cards) {
				return nil, nil, nil, errBuildSlotAssignment{Slot: slot}
			}
			o, err := bip32.ParsePath(cards[s.Card].Path)
			if err != nil {
				return nil, nil, nil, errBuildUnreadableCard{Slot: slot}
			}
			origin = o
		default:
			continue
		}
		seed, ok := reg.at(s.SeedID)
		if !ok {
			return nil, nil, nil, errBuildSlotAssignment{Slot: slot}
		}
		b, err := deriveMultisigLeg(seed.Mnemonic, seed.Passphrase, net, origin, engraveMd1, full)
		if err != nil {
			return nil, nil, nil, err
		}
		// The dedupe runs on the RESULT, not on a prediction of it. Deciding
		// beforehand needs an identity for "same seed" that the caller's bookkeeping
		// may not carry; comparing the minted string needs none, and the string is
		// the thing being deduplicated. deriveMultisigLeg wipes its entropy buffer
		// internally on every call, so paying for the extra encode costs no seed
		// exposure.
		if b.MS1 != "" {
			if engraved[b.MS1] {
				// This seed's plate is already in the set. Clear it from the leg too,
				// so no downstream consumer of `legs` can re-introduce the duplicate.
				b.MS1 = ""
			} else {
				engraved[b.MS1] = true
				ms1s = append(ms1s, b.MS1)
			}
		}
		legs = append(legs, b)
		slots = append(slots, slot)
		mk1s = append(mk1s, b.MK1)
	}
	if len(mk1s) == 0 {
		return nil, nil, nil, errBuildNoHeldSlot
	}
	return legs, slots, multisigEngraveCardsMulti(ms1s, mk1s, engraveMd1), nil
}
