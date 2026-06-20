package gui

import (
	"fmt"
	"strings"

	"github.com/btcsuite/btcd/chaincfg/v2"
	"seedhammer.com/bip39"
	"seedhammer.com/md"
)

// ─── T6b: the SUPPLIED-multisig engrave orchestrator ─────────────────────────
//
// engraveMultisigFlow is the engraveMultisig program: gather a SUPPLIED
// multisig/miniscript wallet-policy md1 over NFC (PUBLIC) -> require a full
// policy (every slot xpub-present) -> hand-type the seed (TYPED-ONLY, never a
// scan) -> CROSS-MATCH the seed to one descriptor slot -> derive the operator's
// leg (ms1 + policy-bound mk1; the supplied md1 engraved VERBATIM) -> engrave
// (full = ms1+mk1+md1; watch-only = mk1+md1 + the ms1 reminder) -> offer
// verify-bundle -> show the multisig restore doc.
//
// SECURITY SPINE (mirror gui/singlesig.go):
//   - TYPED-ONLY seed (I-7): the seed comes from seedEntryFlow ONLY; this flow
//     NEVER routes an NFC-scanned object into derivation. ms1 is engraved onto
//     owner-held steel only, never NFC.
//   - PER-LEG SCRUB (I-7): the entropy is gated + wiped inside deriveMultisigLeg;
//     the seed/master/intermediates are scrubbed inside deriveAccountXpub (called
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

// supplyMultisigPolicyFlow is the UNCHANGED T6b body: gather a SUPPLIED
// multisig/miniscript wallet-policy md1 over NFC (PUBLIC) -> require a full
// policy (every slot xpub-present) -> hand-type the seed (TYPED-ONLY, never a
// scan) -> CROSS-MATCH the seed to one descriptor slot -> derive the operator's
// leg (ms1 + policy-bound mk1; the supplied md1 engraved VERBATIM) -> engrave
// (full = ms1+mk1+md1; watch-only = mk1+md1 + the ms1 reminder) -> offer
// verify-bundle -> show the multisig restore doc.
func supplyMultisigPolicyFlow(ctx *Context, th *Colors) {
	// (1) Gather the SUPPLIED md1 over NFC (PUBLIC). Refuse a polluted/ambiguous
	// supply BEFORE any seed is typed (no secret exists yet).
	cards, ok := bundleGatherFlow(ctx, th)
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

	// (3) TYPED-ONLY seed (I-7). Never a scan.
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
		if pass, ok := passphraseFlow(ctx, th); ok {
			passphrase = pass
		}
	}

	// (4) CROSS-MATCH the seed to one slot (I-1). Refuse on zero matches.
	idx, origin, reused, ok := findUserSlot(mnemonic, passphrase, &chaincfg.MainNetParams, keys)
	if !ok {
		showError(ctx, th, "Engrave Multisig", "This seed is not a cosigner of the supplied policy.")
		return
	}
	if len(reused) >= 2 {
		showError(ctx, th, "Engrave Multisig",
			fmt.Sprintf("This key is reused at slots %s; engraving the first (@%d).", formatSlotList(reused), idx))
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

	// (6) Derive the operator's leg. The mnemonic is consumed for the LAST time
	// here (entropy gated + wiped inside).
	b, err := deriveMultisigLeg(mnemonic, passphrase, &chaincfg.MainNetParams, origin, suppliedMd1, full)
	if err != nil {
		showError(ctx, th, "Engrave Multisig", "Couldn't derive the bundle from the seed.")
		return
	}

	// (7) Engrave (full = ms1+mk1+md1; watch-only = mk1+md1 + the ms1 reminder).
	cardsOut := multisigEngraveCards(b.MS1, b.MK1, b.MD1, full)
	bundleEngrave(ctx, th, cardsOut)

	// (8) Offer the verify-bundle.
	verifyChoice := &ChoiceScreen{Title: "Verify Bundle", Lead: "Verify the engraved plates?", Choices: []string{"Verify now", "Skip"}}
	if sel, ok := verifyChoice.Choose(ctx, th); ok && sel == 0 {
		multisigVerifyFlow(ctx, th, b, full)
	}

	// (9) Restore doc (display-only, PUBLIC — no secret). Reuses the tpl/keys
	// decoded at step (2) — no second ExpandWalletPolicyChunks (t6b-M2).
	multisigRestoreDocFlow(ctx, th, tpl, keys)
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
