package gui

import (
	"fmt"
	"strings"

	"github.com/btcsuite/btcd/chaincfg/v2"
	"seedhammer.com/bip39"
	"seedhammer.com/md"
	"seedhammer.com/sysw"
)

// ─── T6b: the SUPPLIED-multisig engrave orchestrator ─────────────────────────
//
// engraveMultisigFlow is the engraveMultisig program: gather a SUPPLIED
// multisig/miniscript wallet-policy md1 over NFC (PUBLIC) -> require a full
// policy (every slot xpub-present) -> take the seed through the ONE seed seam
// -> CROSS-MATCH the seed to EVERY descriptor slot it accounts for -> derive a
// leg per matched slot at that slot's own origin (one policy-bound mk1 each,
// one ms1 for the one seed; the supplied md1 engraved VERBATIM) -> state the
// plate count -> engrave (full = ms1 + every mk1 + md1; watch-only drops the
// ms1 and shows its hand-engrave reminder) -> offer verify-bundle -> show the
// multisig restore doc.
//
// SECURITY SPINE (mirror gui/singlesig.go):
//   - ONE SEED SEAM (I-7): the seed comes from seedEntryFlow ONLY, and nothing
//     in this flow reads a secret by another route. The single seam IS the rule;
//     "typed-only" is retired and was deleted here on 2026-08-15, because
//     seedEntryFlow is the SOURCE PICKER (systemwide payload / keyboard / scan,
//     gui/derive_xpub.go:88) and §0.1b rules the payload and the keyboard this
//     phase's PRIMARY data entries. A payload-borne ClassMnemonic reaches
//     derivation on purpose, and cmd/emu/walk_build_policy.js drives exactly
//     that. What survives is the split that is load-bearing:
//     seedEntryFlowTypedOnly (gui/derive_xpub.go:124), which the VERIFY flows
//     call so a payload-sourced secret is never compared against itself (§7.4).
//     ms1 is engraved onto owner-held steel only, never NFC.
//   - PER-LEG SCRUB (I-7): the entropy is gated + wiped inside deriveMultisigLeg,
//     on EVERY call, so the per-matched-slot loop in supplyEngraveTail costs no
//     more seed exposure than the single derive it replaced; the
//     seed/master/intermediates are scrubbed inside deriveAccountXpub (called
//     once per slot in the cross-match loop); the mnemonic []Word is zeroed when
//     this flow returns (defer), after its LAST derivation consumer.

// multisigSeedHook is a test-only seam to observe the typed mnemonic (to assert
// it is scrubbed on exit). nil in production. Mirrors singleSigSeedHook.
var multisigSeedHook func(bip39.Mnemonic)

// engraveMultisigFlow is the engraveMultisig program front-door (T6c Phase B):
// "Supply policy (md1)" runs the UNCHANGED T6b body (supplyMultisigPolicyFlow);
// "Build policy" runs the on-device authoring path (buildMultisigPolicyFlow).
// This adds NO program (I-LOCKSTEP: enum/guard/dispatch/title/plate/carousel
// untouched) — it only branches inside the existing program's flow function.
func engraveMultisigFlow(ctx *Context, th *Colors) {
	front := &ChoiceScreen{
		Title:   "Multisig",
		Lead:    "Supply or build a policy?",
		Choices: []string{"Supply policy (md1)", "Build policy"},
	}
	sel, ok := front.Choose(ctx, th)
	if !ok {
		return
	}
	if sel == 0 {
		supplyMultisigPolicyFlow(ctx, th)
		return
	}
	buildMultisigPolicyFlow(ctx, th)
}

// supplyMultisigPolicyFlow is the T6b body, with F-188's engrave rule: gather a
// SUPPLIED multisig/miniscript wallet-policy md1 over NFC (PUBLIC) -> require a
// full policy (every slot xpub-present) -> take the seed through the ONE seed
// seam -> CROSS-MATCH the seed to EVERY descriptor slot it accounts for ->
// derive a leg per matched slot at that slot's own origin -> state the plate
// count -> engrave (full = ms1 + every mk1 + md1; watch-only drops the ms1) ->
// offer verify-bundle over the slots this run cut -> show the multisig restore
// doc.
//
// IT WAS "UNCHANGED T6b" UNTIL F-188. What changed is the engrave rule and only
// the engrave rule: a seed the policy seats at several accounts now gets a plate
// per seat, because that is what the verify already demanded and what the
// operator needs to prove membership of each one. Everything else here -- the
// gather, the full-policy gate, the seed seam, the passphrase, the verbatim
// md1 -- is byte-for-byte the shipped flow.
func supplyMultisigPolicyFlow(ctx *Context, th *Colors) {
	// (1) Gather the SUPPLIED md1 over NFC (PUBLIC). Refuse a polluted/ambiguous
	// supply BEFORE any seed is typed (no secret exists yet).
	//
	// §3.3.2 admits ClassMDMK to this program for exactly this supplied-md1
	// path (plan stage 13c). A payload card is offered ONCE, before gathering,
	// and enters through the SAME offer() a scanned card does — so it is
	// deduplicated, chunk-assembled and validated identically. bundleFlow does
	// it this way and for the same reason: a separate insertion path would be a
	// second way for a card to become part of a bundle, and only one of them
	// would have the checks.
	if body, ok := syswOffer(ctx, th, sysw.ClassMDMK, "First card from where?"); ok {
		ctx.syswBundleSeeds = []string{body}
	}
	cards, ok := bundleGatherFlow(ctx, th, "Engrave Bundle")
	if !ok {
		return
	}
	suppliedMd1, ok := extractSuppliedMd1(cards)
	if !ok {
		showError(ctx, th, "Engrave Multisig", "Supply exactly one wallet-policy md1 (and no key cards).")
		return
	}

	// (2) Decode + full-policy gate (I-3).
	tpl, keys, err := md.ExpandWalletPolicyChunks(suppliedMd1)
	if err != nil {
		showError(ctx, th, "Engrave Multisig", "Couldn't decode the supplied wallet policy.")
		return
	}
	if !allSlotsHaveXpub(keys) {
		showError(ctx, th, "Engrave Multisig", "The supplied descriptor has no public keys to match.")
		return
	}
	// tpl/keys are threaded into the restore doc below (t6b-M2) so the policy is
	// decoded exactly once.

	// (3) The seed, through the ONE seam (I-7). seedEntryFlow offers every source
	// this machine has; it is not keyboard-only.
	mnemonic, ok := seedEntryFlow(ctx, th)
	if !ok {
		return
	}
	if multisigSeedHook != nil {
		multisigSeedHook(mnemonic)
	}
	// Scrub the SECRET mnemonic on EVERY exit path (incl. abort / no-match), after
	// its last derivation consumer (I-7).
	defer func() {
		for i := range mnemonic {
			mnemonic[i] = 0
		}
	}()

	// Optional passphrase.
	passphrase := ""
	ppChoice := &ChoiceScreen{Title: "Passphrase", Lead: "Add a BIP-39 passphrase?", Choices: []string{"Skip", "Add passphrase"}}
	if sel, ok := ppChoice.Choose(ctx, th); ok && sel == 1 {
		// §3.3.2 admits ClassPassphrase to this program, so the payload is
		// offered before the keyboard (plan stage 13b). NOT passphraseFlow: see
		// syswPassphraseFlow for the two normative rules a shared edit inside
		// passphraseFlow would have broken.
		if pass, ok := syswPassphraseFlow(ctx, th); ok {
			passphrase = pass
		}
	}

	// (4) CROSS-MATCH the seed to EVERY slot it accounts for (I-1). Refuse on
	// zero matches.
	//
	// allUserSlots, NOT findUserSlot, and the difference is F-188. findUserSlot
	// answers "which slot is the operator in" and returns the FIRST match; the
	// question this flow has to answer is "which slots does this seed HOLD", and
	// a policy putting one seed at several accounts answers it with several --
	// at DIFFERENT origins, carrying DIFFERENT keys. Cutting one plate and
	// calling the rest a reused key described a shape the code does not produce,
	// and left every matched slot but one unprovable by the operator.
	slots := allUserSlots(mnemonic, passphrase, &chaincfg.MainNetParams, keys)
	if len(slots) == 0 {
		showError(ctx, th, "Engrave Multisig", "This seed is not a cosigner of the supplied policy.")
		return
	}
	// (4a) SAY WHAT WILL BE CUT, not what is being dropped. The operator is about
	// to cut more plates than this policy's slot count suggests one seed should
	// need, and the reason is a property of the POLICY (it seats them at several
	// accounts) rather than of their seed. The plate census at (6a) states the
	// count; this states why it is not one.
	//
	// TWO SENTENCES, BECAUSE THE POLICY HAS TWO SHAPES AND ONLY ONE OF THEM WAS
	// EVER TRUE HERE. "each of those slots holds a DIFFERENT key" is right for the
	// reused-SEED shape (one seed at several accounts, several origins, several
	// keys) and FALSE for a supplied policy that seats one key at two slots -- the
	// same claims-a-shape-the-code-does-not-have defect F-188's own commit message
	// calls the tell, which is why it is not repeated here in the other direction.
	//
	// THE COLLAPSED ARM STATES NO COUNT. It is drawn before the engrave mode is
	// chosen, so no leg has been minted and any number here would be a second
	// prediction of the tail's behaviour. The census at (6a) carries the exact
	// figure and derives it from the tail's own return.
	if len(slots) >= 2 {
		if multisigSlotsShareAKey(keys, slots) {
			showNotice(ctx, th, "Engrave Multisig", fmt.Sprintf(
				"Your seed is at slots %s of this policy, and more than one of them holds "+
					"the SAME key at the SAME key path. Identical plates would carry "+
					"identical information, so this run cuts one plate per DISTINCT key, "+
					"not one per slot. The next screen states the exact count.",
				formatSlotList(slots)))
		} else {
			showNotice(ctx, th, "Engrave Multisig", fmt.Sprintf(
				"Your seed is at slots %s of this policy, at a different key path in each, so "+
					"each of those slots holds a DIFFERENT key. This run engraves %s, one per "+
					"slot.", formatSlotList(slots), plateWord(len(slots), "key plate", "key plates")))
		}
	}

	// (5) Full vs watch-only.
	modeChoice := &ChoiceScreen{
		Title:   "Engrave Mode",
		Lead:    "What to engrave?",
		Choices: []string{"Full (seed + keys)", "Watch-only (keys)"},
	}
	modeSel, ok := modeChoice.Choose(ctx, th)
	if !ok {
		return
	}
	full := modeSel == 0

	// (6) Derive the operator's leg PER MATCHED SLOT, at that slot's own origin
	// (F-188). The mnemonic is consumed for the LAST time here; deriveMultisigLeg
	// gates and wipes its own entropy buffer on every call, so several legs cost
	// no extra seed exposure. ONE ms1 for the one seed, however many slots it
	// fills.
	engravedSlots, cardsOut, err := supplyEngraveTail(mnemonic, passphrase,
		&chaincfg.MainNetParams, keys, slots, suppliedMd1, full)
	if err != nil {
		showError(ctx, th, "Engrave Multisig", "Couldn't derive the bundle from the seed.")
		return
	}

	// (6a) HOW MANY PLATES, BEFORE THE FIRST ONE (the plan's S4 prose
	// constraint). It binds harder here than anywhere else it applies: F-188 is
	// the change that makes these inputs cut MORE plates than they did
	// yesterday, so the operator has to learn the count before the first one
	// rather than discover it four plates in. Back here aborts before anything
	// is cut, which is the last moment that is free.
	//
	// THE TITLE DELIBERATELY DIFFERS FROM THE BUILD PATH'S CENSUS TITLE.
	// cmd/emu/needle_test.go requires every string a walk anchors on to have
	// exactly ONE production site, because a walk that cannot tell which flow
	// drew a screen proves nothing about the flow it claims to have driven
	// (F-169). The build census title is walk_s4_gate.js's anchor and lives in
	// gui/multisig_build.go; reusing that literal here made it two-site and the
	// gate said so, in the same run. Re-anchoring that walk was the alternative
	// and there is nothing to re-anchor it ON: every other string on that screen
	// comes from the shared buildPlateCensusLines. The BODY -- what the operator
	// actually reads, including the count -- is identical in both flows.
	//
	// The counter matches SOURCE BYTES, comments included, so this note may not
	// spell the build path's title either. That is not pedantry: a comment
	// quoting a needle costs the needle its uniqueness just as effectively as a
	// second screen does, and it took one test run to find out.
	//
	// AND THE COLLAPSE, FIRST ON THE PAGE (plan §0.1 clause 3). A supplied policy
	// seating one key at two slots cuts FEWER plates than its matched-slot count
	// suggests, and clause 3 puts an assumption upstream of steel on the
	// confirmation surface itself rather than in scrollback. It leads the list
	// because this screen is confirmable from any page: a note on page three is a
	// note the operator can commit past without reading.
	//
	// THE PREDICATE IS THE TAIL'S OWN RETURN, not a re-derivation of it. The tail
	// is the one place that decides which slots get a plate, so a note that asked
	// any other source could claim a collapse that did not happen, or miss one
	// that did.
	census := buildPlateCensusLines(cardsOut)
	if len(engravedSlots) < len(slots) {
		census = append([]string{fmt.Sprintf(
			"NOTE: slots %s hold only %s between them, so this run cuts %s, not one per slot.",
			formatSlotList(slots),
			plateWord(len(engravedSlots), "distinct key", "distinct keys"),
			plateWord(len(engravedSlots), "key plate", "key plates"))}, census...)
	}
	if !confirmReviewScreen(ctx, th, "Plates To Cut", census) {
		return
	}

	// (7) Engrave (full = ms1 + one mk1 per matched slot + md1; watch-only drops
	// the ms1 and bundleEngrave shows the hand-engrave reminder instead).
	bundleEngrave(ctx, th, cardsOut)

	// (8) Offer the verify-bundle.
	//
	// engravedSlots IS THE OBLIGATION LIST, and it is the tail's own return
	// rather than a recomputation of `slots`. The two are equal today by
	// construction, and passing the tail's list is what keeps them that way: the
	// verify's derive loop asks a re-typed seed which slots it FILLS, and only
	// the engraver knows which of those it actually cut steel for.
	//
	// IT IS NOT REDUNDANT NOW THAT THE ENGRAVE COVERS EVERY MATCHED SLOT, because
	// multisigVerifyFlow has another caller. On the BUILD path allUserSlots can
	// still exceed what was engraved: a payload cosigner card carrying a DIFFERENT
	// key derived from the same seed at another origin is not a duplicate
	// (duplicateSlotPair refuses only IDENTICAL keys) and is admitted, so the seed
	// accounts for a slot the operator never claimed. Dropping the expectation
	// would reopen that as a false RED.
	//
	// suppliedMd1 IS THE OTHER HALF OF THE OBLIGATION, and it is the md1 this run
	// engraved VERBATIM (I-2), so it is exactly what the operator will present.
	// Slot indices alone get re-based onto whatever policy the readback supplies,
	// which is how a byte-valid plate set from a DIFFERENT wallet reports
	// "Verify OK" while this run's steel is never read.
	verifyChoice := &ChoiceScreen{Title: "Verify Bundle", Lead: "Verify the engraved plates?", Choices: []string{"Verify now", "Skip"}}
	if sel, ok := verifyChoice.Choose(ctx, th); ok && sel == 0 {
		multisigVerifyFlow(ctx, th, full, engravedSlots, suppliedMd1)
	}

	// (9) Restore doc (display-only, PUBLIC — no secret). Reuses the tpl/keys
	// decoded at step (2) — no second ExpandWalletPolicyChunks (t6b-M2).
	multisigRestoreDocFlow(ctx, th, tpl, keys, nil)
}

// formatSlotList renders matched slot indices as "@a, @b and @c" for the
// reused-key notice (t6b-M1) so EVERY reused slot is named, not just the first
// two. Inputs of len 0/1 fall through to the obvious single-token forms.
func formatSlotList(slots []int) string {
	switch len(slots) {
	case 0:
		return ""
	case 1:
		return fmt.Sprintf("@%d", slots[0])
	}
	var b strings.Builder
	for i, s := range slots {
		switch {
		case i == 0:
			// first token, no separator
		case i == len(slots)-1:
			b.WriteString(" and ")
		default:
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "@%d", s)
	}
	return b.String()
}
