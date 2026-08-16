package gui

import (
	"errors"
	"fmt"
	"strings"

	"github.com/btcsuite/btcd/chaincfg/v2"
	"seedhammer.com/bip39"
	"seedhammer.com/md"
)

// ─── F-188: the SUPPLY path's engrave tail ───────────────────────────────────
//
// Before F-188 this path called findUserSlot, engraved ONE plate for the first
// matched slot, and told the operator:
//
//	This key is reused at slots @0 and @1; engraving the first (@0).
//
// THE SENTENCE IS FALSE, AND THE FALSITY IS THE TELL. allUserSlots derives the
// seed at EACH slot's own OriginPath, so a seed filling @0 and @1 holds two
// DIFFERENT keys at two DIFFERENT origins. It is one seed at several accounts,
// never one key repeated. The message described a shape the code does not
// produce, and the plate it did not cut is a slot the operator cannot prove
// membership of.
//
// So the engrave rule and the verify rule now agree AT THE SOURCE: a plate per
// matched slot. Making the checker tolerate the disagreement was the other arm,
// and it was rejected twice -- dropping the leg-to-plate rule makes a LOST plate
// pass, and deduping legs by identical mk1 is inert for the real shape, because
// the keys are not identical.
//
// THAT LAST CLAUSE IS SCOPED TO THE REUSED-SEED SHAPE, and saying so is not
// pedantry: an mk1-identity dedupe DOES live in the tail below, for a DIFFERENT
// shape -- a SUPPLIED policy seating the same key at the same origin twice,
// where the plates really are byte-identical. Read as a blanket verdict this
// paragraph argues against a mechanism sitting a hundred lines down. It was
// never a verdict on the mechanism, only on what the mechanism buys for THIS
// defect, which is nothing.
//
// Operator ruling F-188, 2026-08-15 ("Build this"). It changes what goes on
// STEEL in a flow this plan does not otherwise own, so it was ruled rather than
// refactored. The Rust-primary rule does not bind: fork-native GUI/UX with no
// Rust counterpart (exempt clause b).

// errSupplyNoMatchedSlot is a supply engrave with no slot to cut a plate for.
//
// The caller refuses zero matches on its own screen before reaching here ("This
// seed is not a cosigner of the supplied policy"), so this fires only if a
// future caller loses the match list on the way in -- which is exactly the case
// where an empty plate set would be engraved in silence.
var errSupplyNoMatchedSlot = errors.New("multisig supply: the seed matched no slot, so there is nothing to engrave")

// supplyEngraveTail derives one leg per MATCHED slot, at that slot's own origin,
// and returns the slots it actually cut a plate for plus the engrave card set.
//
// THE SLOT LIST LEAVES WITH THE CARDS, and that is deliberate: it is the
// verify's obligation list, and only the engraver has it. This loop is the one
// place that decides which slots get a plate, so re-deriving the set at the call
// site would be a second copy of the rule -- and two copies of this particular
// rule is how a verify starts demanding a plate the engrave never made.
//
// ONE ms1 PER DISTINCT SEED, keyed on THE ENGRAVED STRING, exactly as
// buildEngraveTail does it. The supply path has exactly ONE seed, so this yields
// exactly one seed plate however many slots that seed fills: a second would be a
// duplicate secret on steel with no recovery benefit. The key is the ms1 string
// rather than any identity the caller carries, because the string IS the
// identity of the secret the plate holds -- two equal strings encode identical
// entropy, so this can never DROP a distinct seed's only plate, which is the
// direction that loses funds.
//
// The emitter is multisigEngraveCardsMulti, unchanged and unforked: every ms1,
// then every mk1, then the md1. That order is a CONTRACT (oracle's artifact
// shape requires those kinds as consecutive runs in that sequence), and a second
// emitter here would be a second place for it to drift.
//
// ONE PLATE PER DISTINCT mk1, keyed on THE ENGRAVED STRING, exactly as the ms1
// dedupe above and for the same reason one level over.
//
// A SUPPLIED policy may seat the SAME (xpub, origin) at two slots -- the supply
// path engraves a descriptor someone else authored, repeated keys and all
// (§4.1's scoping, gui/multisig_build.go; duplicateSlotPair guards only the
// BUILD path). For that shape the per-slot legs mint BYTE-IDENTICAL mk1s, and
// two byte-identical plates carry identical information.
//
// WITHOUT THE DEDUPE THE RUN IS PERMANENTLY UNVERIFIABLE, which is the harm this
// closes and it is not hypothetical. The gatherer keys mk1 cards on the
// payload-derived chunk_set_id, so a byte-identical second card is a duplicate
// and the readback can STRUCTURALLY never yield two. The verify's length
// precheck then refused every attempt -- "Read back 1 key plate, but this run
// engraved 2 key plates. Present exactly the plates this run cut." -- while the
// operator was doing exactly that. An honest, announced, hours-long engrave that
// the device then disowns forever.
//
// RULED under plan §0.1's ladder rather than refused: no standard governs
// (clause 1); the collapse is DETECTABLE BY READING THE OUTPUT -- the census
// states the plate count before the tail, the md1 carries the policy showing
// both slots, the restore doc lists the inventory -- so it is eligible to assume
// (clause 2, the funds-safety boundary); and it is announced on the census
// screen, before the first cut (clause 3). Refusing an admitted policy instead
// would be choosing the refusal arm because it makes a cleaner one-armed test,
// which §0.1 forbids.
//
// THIS IS THE SAME MECHANISM A PREVIOUS BLOCK DELETED AS INERT, AND BOTH ARE
// RIGHT. It was inert for the REUSED-SEED shape (one seed at several accounts:
// different origins, so different keys and different mk1s -- nothing to dedupe,
// and deduping there would have DROPPED a plate the operator needs). It is
// correct and necessary for THIS shape, where the keys are identical. Same
// mechanism, different shape; the earlier deletion is not an argument against it
// here.
//
// THE OBLIGATION LIST COLLAPSES WITH THE PLATES, necessarily: `slots` gains an
// entry only where a plate was appended, so it can never name steel that does
// not exist.
//
// SECURITY: deriveMultisigLeg gates and wipes its own entropy buffer on every
// call, so deriving several legs from one mnemonic costs no extra seed exposure.
// The caller still owns the mnemonic and scrubs it on every exit path (I-7).
func supplyEngraveTail(m bip39.Mnemonic, passphrase string, net *chaincfg.Params,
	keys []md.ExpandedKey, matched []int, suppliedMd1 []string, full bool,
) ([]int, []bundleCard, error) {
	var (
		slots []int
		ms1s  []string
		mk1s  [][]string
	)
	engraved := map[string]bool{}
	cut := map[string]bool{}
	for _, s := range matched {
		// A slot outside the policy is an ERROR, never a skip. Skipping would
		// engrave a smaller set than the caller asked for and then hand the verify
		// a list that matches the steel, so the whole run would look consistent
		// while a slot the operator holds went uncut.
		if s < 0 || s >= len(keys) {
			return nil, nil, fmt.Errorf("multisig supply: matched slot @%d is not in this "+
				"policy's %d slot(s)", s, len(keys))
		}
		b, err := deriveMultisigLeg(m, passphrase, net, keys[s].OriginPath, suppliedMd1, full)
		if err != nil {
			return nil, nil, err
		}
		// The dedupe runs on the RESULT rather than on a prediction of it: comparing
		// the minted string needs no identity bookkeeping, and the string is the
		// thing being deduplicated.
		if b.MS1 != "" && !engraved[b.MS1] {
			engraved[b.MS1] = true
			ms1s = append(ms1s, b.MS1)
		}
		// Same rule for the KEY plate, on the same key -- the minted string. A
		// distinct key can never be dropped by it, because a distinct key mints a
		// distinct mk1; only a byte-identical repeat collapses.
		mk1Key := strings.Join(b.MK1, "|")
		if cut[mk1Key] {
			continue
		}
		cut[mk1Key] = true
		mk1s = append(mk1s, b.MK1)
		slots = append(slots, s)
	}
	if len(mk1s) == 0 {
		return nil, nil, errSupplyNoMatchedSlot
	}
	return slots, multisigEngraveCardsMulti(ms1s, mk1s, suppliedMd1), nil
}

// multisigSlotsShareAKey reports whether any two of the MATCHED slots declare the
// same key at the same origin -- the shape whose plates collapse in the tail
// above.
//
// IT DOES NOT DECIDE ANYTHING THE OPERATOR IS SHOWN A NUMBER FOR. The exact
// plate count comes from the tail's own return, through the census; this only
// picks WHICH SENTENCE the multi-slot notice says, because that notice is drawn
// before the engrave mode is chosen and so before any leg exists to be minted.
// Keeping the count out of it is deliberate: a second predictor of the tail's
// behaviour is a second thing that can disagree with the tail.
//
// (xpub, origin) is the pair, not the xpub alone: an mk1 carries both, so two
// slots holding one key at DIFFERENT origins mint different plates and nothing
// collapses.
func multisigSlotsShareAKey(keys []md.ExpandedKey, matched []int) bool {
	seen := make(map[string]bool, len(matched))
	for _, s := range matched {
		if s < 0 || s >= len(keys) {
			continue
		}
		id := string(keys[s].Xpub[:]) + "@" + keys[s].OriginPath.String()
		if seen[id] {
			return true
		}
		seen[id] = true
	}
	return false
}
