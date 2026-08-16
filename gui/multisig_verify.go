package gui

import (
	"errors"
	"fmt"

	"github.com/btcsuite/btcd/chaincfg/v2"
	"seedhammer.com/bip39"
	"seedhammer.com/bundle"
	"seedhammer.com/codex32"
	"seedhammer.com/md"
	"seedhammer.com/mk"
)

// Multisig verify success copy (L2). HONEST scoping: on an air-gapped device the
// only cross-checkable facts are the operator's own key card (mk1, H1) + xpub/
// origin (findUserSlot) + the secret (ms1 entropy/language, M1). The md1 policy
// string is the supplied input compared to a clone of itself, and foreign
// cosigners' xpubs have no source of truth — so we do NOT claim a full-bundle
// guarantee.
const (
	multisigVerifyOKTitle = "Verify OK"
	multisigVerifyOKBody  = "Operator key and secret verified. Other cosigners' keys are taken as supplied."
)

// ─── T6b: verify-bundle for a SUPPLIED multisig bundle (user's slot only) ────
//
// verifyMultisig assembles the read-back bundle and runs the deterministic
// comparator against the freshly re-derived operator leg (mirror verifySingleSig,
// gui/singlesig_verify.go:49). It verifies ONLY the operator's slot (I-5); the
// other cosigner slots are public-given and unverified-by-design (bundle.Verify
// never inspects them).
//
// The ms1 leg follows bundle.Verify's native presence semantics (verify.go:71-79):
// a watch-only verify passes "" for BOTH the derived bundle's MS1 (the leg was
// re-derived with full=false) AND ms1Readback → both empty → the ms1 leg is
// SKIPPED. A full verify carries an ms1 on both sides → the recovered entropy is
// compared. An ms1 present on exactly one side is a PRESENCE MISMATCH and errors
// (we deliberately do NOT mask it by zeroing the derived MS1 — that would let a
// full bundle silently pass an empty readback). Returns the comparator's first
// diverging-field error, or nil on PASS.
func verifyMultisig(derived bundle.Bundle, ms1Readback string, mk1, md1 []string) error {
	readback := bundle.Bundle{MS1: ms1Readback, MK1: mk1, MD1: md1}
	return bundle.Verify(derived, readback)
}

// ─── S5: the verify covers EVERY leg ─────────────────────────────────────────
//
// Before S5 this flow took ONE bundle and recovered the operator's origin
// through findUserSlot, which returns the FIRST slot a seed matches -- so it
// structurally could not verify legs 2..n. Trace B engraves three key plates;
// the shipped verify read back one, compared one, and showed "Verify OK".
//
// A verify that checks one of three plates and reports success is a FALSE GREEN
// on the operator's ONLY readback of what the machine actually cut. It is
// treated as such: the comparator below is a BIJECTION, and both directions are
// funds-bearing.
//
//   - every re-derived leg must find its plate. A leg with no plate is a slot
//     the operator cannot prove membership of, and it is exactly the shipped
//     defect -- check what was shown, call the rest verified.
//   - every plate must be claimed by a leg. An unclaimed plate is steel in the
//     operator's hands that this policy has no slot for, and "Verify OK" over it
//     tells them it belongs to this wallet.

// verifyLeg is ONE re-derived leg plus the ms1 the operator hand-typed for ITS
// seed.
//
// The ms1 travels WITH the leg rather than being a flow-global, for SPEC 4.1's
// reason at the other end of the flow: a build across two masters engraves two
// seed plates, and comparing either leg against whichever ms1 the flow happened
// to hold is how a "Full" backup carrying master A twice verifies clean while
// master B -- which k=3 needs -- is gone.
type verifyLeg struct {
	// Slot is the policy slot this leg proves. Operator-facing: it is what the
	// failure names, so "which plate do I re-cut" has an answer.
	Slot int
	// B is the leg as freshly RE-DERIVED from a re-typed seed, never the leg the
	// engrave produced (§7.4: a readback taken from the session compares the
	// engrave source against itself and passes unconditionally).
	B bundle.Bundle
	// MS1Readback is the ms1 the operator typed for this leg's seed, or "" in a
	// watch-only verify.
	MS1Readback string
}

// errVerifyLegHasNoPlate is a leg the readback does not account for: no plate
// carries this slot's key. It names the SLOT, because that is the only thing the
// operator can act on.
type errVerifyLegHasNoPlate struct{ Slot int }

func (e errVerifyLegHasNoPlate) Error() string {
	return fmt.Sprintf("verify: no read-back key plate carries slot @%d's key", e.Slot)
}

// errVerifyPlateUnclaimed is a read-back plate no leg claims.
//
// It deliberately does NOT name a slot: there isn't one. Saying "@N" here would
// be inventing an attribution for a plate whose whole problem is that it belongs
// to nothing, and the per-leg failure above is the message that has a slot to
// name.
type errVerifyPlateUnclaimed struct{ Plate int }

func (e errVerifyPlateUnclaimed) Error() string {
	return fmt.Sprintf("verify: read-back key plate %d belongs to no leg of this policy",
		e.Plate+1)
}

// errVerifyNoLegs is a verify with nothing to compare. It is an ERROR and not a
// vacuous pass: a comparator whose empty case returns nil reports success for a
// readback it never looked at, which is the single most expensive false GREEN
// this flow can produce.
var errVerifyNoLegs = errors.New("verify: no leg was re-derived, so nothing was checked")

// verifyMultisigLegs compares EVERY leg against the read-back plate set, and
// requires the pairing to be a bijection.
//
// PAIRING IS BY THE mk1's XPUB, and the choice is load-bearing in both
// directions:
//
//   - it must be UNIQUE, or two legs race for one plate. The origin PATH is not
//     unique across masters -- Trace B's @0 (master A) and @2 (master B) both
//     declare m/48h/0h/0h/2h -- so a path-keyed pairing would hand master B's
//     plate to master A's leg and report a mismatch on an honest readback.
//   - it must leave the comparison with real work to do, or "the plate that
//     matches" is a tautology. It does: bundle.Verify still checks the master
//     fingerprint, the ORIGIN PATH, the md1 exact string, the mk1<->md1 stub
//     binding on BOTH sides, and the ms1's recovered entropy and wordlist. A
//     plate carrying the right key at a lying origin pairs here and fails there.
//
// A plate that does not decode pairs with nothing, so it surfaces as its leg's
// missing plate and then as an unclaimed plate -- never as a silent skip.
func verifyMultisigLegs(legs []verifyLeg, mk1s [][]string, md1 []string) error {
	if len(legs) == 0 {
		return errVerifyNoLegs
	}
	claimed := make([]bool, len(mk1s))
	for _, l := range legs {
		idx, ok := verifyClaimPlate(l.B.MK1, mk1s, claimed)
		if !ok {
			return errVerifyLegHasNoPlate{Slot: l.Slot}
		}
		claimed[idx] = true
		if err := verifyMultisig(l.B, l.MS1Readback, mk1s[idx], md1); err != nil {
			return fmt.Errorf("slot @%d: %w", l.Slot, err)
		}
	}
	for i, c := range claimed {
		if !c {
			return errVerifyPlateUnclaimed{Plate: i}
		}
	}
	return nil
}

// verifyClaimPlate finds the UNCLAIMED read-back plate carrying the same account
// xpub as `want`. An undecodable plate is skipped here and caught by the
// unclaimed sweep, so a corrupted plate can never be quietly dropped.
func verifyClaimPlate(want []string, mk1s [][]string, claimed []bool) (int, bool) {
	w, err := mk.Decode(want)
	if err != nil {
		return 0, false
	}
	for i, p := range mk1s {
		if claimed[i] {
			continue
		}
		got, derr := mk.Decode(p)
		if derr != nil {
			continue
		}
		if got.Xpub == w.Xpub {
			return i, true
		}
	}
	return 0, false
}

// multisigVerifyFlow drives the on-device verify-bundle for the multisig flow:
// gather the engraved md1 + EVERY operator mk1 plate over NFC
// (extractReadbackMd1AndMk1s), then for each seed the operator re-types
// (fresh residency) re-derive EVERY leg that seed accounts for, hand-type that
// seed's ms1 (full only; never NFC), and report PASS/FAIL — comparing the
// READ-BACK plates against the re-derived legs (H1: never a re-derived value
// against itself). `full` reports whether an ms1 was engraved (and so must be
// hand-typed for verify).
//
// TWO THINGS CHANGED AT S5, and both are consequences of a build holding several
// slots.
//
//  1. THE GATHER RUNS FIRST. It used to run after the seed. The readback is what
//     says how many legs there are to prove, so the flow cannot know how many
//     seeds to ask for until it has seen the plates; and gathering first matches
//     the build path's own posture (TestBuildFlow_GatherBeforeSeed) — no secret
//     is resident while a public set is being resolved.
//  2. SEVERAL SEEDS. Trace B's three plates span two masters, and one seed can
//     only prove its own. So the flow loops: type a seed, cover the slots it
//     accounts for, and if plates remain, offer the next one. Declining is
//     allowed and is reported as an INCOMPLETE verify — never as a pass over
//     plates nobody checked.
//
// ONE SCRUB SITE, DEFERRED BEFORE THE FIRST SEED EXISTS, is the same design the
// build flow's seedRegistry uses and for the same reason: every exit below (a
// Back, a refusal modal, a ctx.Done unwind, a panic) is covered by construction
// rather than by an implementer remembering to add a wipe to a new return.
func multisigVerifyFlow(ctx *Context, th *Colors, full bool) {
	// Read back the PUBLIC cards over NFC via the T5 gatherer.
	//
	// NO PAYLOAD OFFER HERE, deliberately (plan stage 13c). §3.3.2 admits
	// ClassMDMK to this program, but a verify READBACK must come from the
	// plate's own cards: §7.4's reasoning applied to the bundle rather than to
	// the seed — a readback taken from the session would compare the engrave
	// source against itself and pass unconditionally, certifying a wrong plate.
	// The passphrase step below uses passphraseFlow for the same reason.
	cards, ok := bundleGatherFlow(ctx, th, "Engrave Bundle")
	if !ok {
		return
	}
	readbackMd1, readbackMk1s, ok := extractReadbackMd1AndMk1s(cards)
	if !ok {
		showError(ctx, th, "Verify Bundle", "Read back one wallet-policy md1 AND the operator key card(s) (mk1).")
		return
	}
	_, keys, err := md.ExpandWalletPolicyChunks(readbackMd1)
	if err != nil {
		showError(ctx, th, "Verify Bundle", "Couldn't decode the read-back wallet policy.")
		return
	}

	var typed []bip39.Mnemonic
	defer func() {
		for _, m := range typed {
			for i := range m {
				m[i] = 0
			}
		}
	}()

	var legs []verifyLeg
	covered := make(map[int]bool, len(keys))
	for len(legs) < len(readbackMk1s) {
		reMnemonic, ok := seedEntryFlowTypedOnly(ctx, th)
		if !ok {
			break
		}
		// Owned by the deferred scrub from this line on, before any screen below
		// can return early.
		typed = append(typed, reMnemonic)

		passphrase := ""
		ppChoice := &ChoiceScreen{Title: "Passphrase", Lead: "Add a BIP-39 passphrase?", Choices: []string{"Skip", "Add passphrase"}}
		if sel, ok := ppChoice.Choose(ctx, th); ok && sel == 1 {
			if pass, ok := passphraseFlow(ctx, th); ok {
				passphrase = pass
			}
		}

		slots := allUserSlots(reMnemonic, passphrase, &chaincfg.MainNetParams, keys)
		fresh := make([]int, 0, len(slots))
		for _, s := range slots {
			if !covered[s] {
				fresh = append(fresh, s)
			}
		}
		if len(fresh) == 0 {
			// TWO DIFFERENT PROBLEMS, TWO DIFFERENT MESSAGES. A seed that is in the
			// policy but already checked is an operator repeating themselves; a seed
			// that is in no slot at all is the wrong wallet, and telling them to
			// "try another" would send them looking for a seed that exists.
			if len(slots) == 0 {
				showError(ctx, th, "Verify Bundle", "That seed is not a cosigner of the "+
					"read-back policy, so it cannot prove any of these plates.")
			} else {
				showError(ctx, th, "Verify Bundle", "That seed's slots have already been "+
					"checked. The plates still outstanding belong to a different seed.")
			}
			return
		}

		// Hand-type the SECRET ms1 (full mode only; never NFC). ONE PER SEED: a
		// build across two masters engraves two seed plates, and each must be
		// checked against the master it claims.
		ms1Readback := ""
		if full {
			s, ok := multisigVerifyMS1Entry(ctx, th)
			if !ok {
				return
			}
			ms1Readback = s
		}

		for _, s := range fresh {
			b, derr := deriveMultisigLeg(reMnemonic, passphrase, &chaincfg.MainNetParams,
				keys[s].OriginPath, readbackMd1, full)
			if derr != nil {
				showError(ctx, th, "Verify Bundle", "Couldn't re-derive the bundle from the seed.")
				return
			}
			legs = append(legs, verifyLeg{Slot: s, B: b, MS1Readback: ms1Readback})
			covered[s] = true
		}
		if len(legs) >= len(readbackMk1s) {
			break
		}
		next := &ChoiceScreen{
			Title:   "Verify Bundle",
			Lead:    fmt.Sprintf("%s not checked yet. Next seed?", plateWord(len(readbackMk1s)-len(legs), "key plate is", "key plates are")),
			Choices: []string{"TYPE THE NEXT SEED", "STOP HERE"},
		}
		if sel, ok := next.Choose(ctx, th); !ok || sel != 0 {
			break
		}
	}

	// Back at the FIRST seed entry is abandoning the verify, not completing a
	// partial one, and it is the shipped behaviour: return in silence. It is the
	// only way to reach zero legs, because every other exit above returns on its
	// own screen.
	if len(legs) == 0 {
		return
	}

	// NOT A PASS, AND NOT SILENCE. Stopping early with plates still outstanding is
	// a legitimate operator choice and it produces an INCOMPLETE verify, which is
	// a different thing from a failed one and from a clean one. Reporting it as
	// either would be the false GREEN this whole rewrite exists to remove.
	if len(legs) < len(readbackMk1s) {
		showError(ctx, th, "Verify Incomplete", fmt.Sprintf(
			"Checked %d of the %d key plates read back. The rest were NOT verified. "+
				"Run verify again with the remaining seeds before funding this wallet.",
			len(legs), len(readbackMk1s)))
		return
	}

	if err := verifyMultisigLegs(legs, readbackMk1s, readbackMd1); err != nil {
		showError(ctx, th, "Verify Failed", "The read-back bundle does NOT match the seed. Check the engraved plates.")
		return
	}
	showNotice(ctx, th, multisigVerifyOKTitle, multisigVerifyOKMessage(len(legs), full))
}

// multisigVerifyMS1Entry hand-types ONE ms1 and returns its canonical string.
//
// Extracted so the per-seed loop above reads as a loop rather than as sixteen
// lines of secret handling repeated inline. The L1 scrub is the codebase
// convention (gui/ms1_decode.go): DecodeMS1 allocates a fresh entropy slice this
// probe would otherwise abandon to the GC.
func multisigVerifyMS1Entry(ctx *Context, th *Colors) (string, bool) {
	obj, ok := inputCodex32Flow(ctx, th, "Type ms1")
	if !ok {
		return "", false
	}
	s, isStr := obj.(codex32.String)
	if !isStr {
		showError(ctx, th, "Verify Bundle", "That isn't an ms1 secret share.")
		return "", false
	}
	_, _, ent, err := codex32.DecodeMS1(s)
	if err != nil {
		showError(ctx, th, "Verify Bundle", "That isn't a valid ms1 secret share.")
		return "", false
	}
	wipeBytes(ent)
	return s.String(), true
}

// multisigVerifyOKMessage is the success notice, SCOPED TO WHAT WAS CHECKED.
//
// The one-leg text is the shipped constant unchanged, because its honesty is
// pinned (TestMultisigVerifyNoticeIsHonest: it must scope the guarantee and must
// not carry the full-bundle over-claim). A multi-leg run says HOW MANY plates it
// checked, for the same reason the one-leg text says what it did not check: a
// bare "Verify OK" after a three-plate readback is indistinguishable from a bare
// "Verify OK" after a one-plate one, and the operator has no other way to learn
// which they got.
//
// IT DOES NOT CLAIM THE SEED PLATES WERE COUNTED. Only what the operator typed
// was compared: a secret is never read back over the reader (§7.4), so the
// device cannot know how many ms1 plates exist and must not imply it checked
// them all. `full` is the mode, so a watch-only run does not claim a secret it
// never asked for.
//
// NO EM-DASH: it is a zero-pixel glyph in the body face and a line carrying one
// does not draw at all (F-78/F-151).
func multisigVerifyOKMessage(legs int, full bool) string {
	if legs <= 1 {
		return multisigVerifyOKBody
	}
	if full {
		return fmt.Sprintf("All %d operator key plates verified, and the ms1 you typed "+
			"for each seed. Other cosigners' keys are taken as supplied.", legs)
	}
	return fmt.Sprintf("All %d operator key plates verified. Other cosigners' keys "+
		"are taken as supplied.", legs)
}
