package gui

import (
	"github.com/btcsuite/btcd/chaincfg/v2"
	"seedhammer.com/bip39"
	"seedhammer.com/md"
)

// ─── T6a-2: the single-sig flagship orchestrator ─────────────────────────────
//
// engraveSingleSigFlow is the engraveSingleSig program: ONE BIP-39 seed →
// wallet-type pick (BIP-84 default + Advanced) → optional passphrase →
// full-or-watch-only → derive ms1+mk1+md1 (policy-bound) → engrave (full = 3
// cards incl. the secret ms1; watch-only = 2 cards + the ms1 reminder) → offer
// verify-bundle → watch-only restore doc.
//
// SECURITY SPINE:
//   - ONE SEED SEAM (D12): the seed comes from seedEntryFlow ONLY, and nothing
//     in this flow reads a secret by another route. The single seam IS the rule;
//     "typed-only" is retired and was deleted here on 2026-08-15, because
//     seedEntryFlow is the SOURCE PICKER (systemwide payload / keyboard / scan,
//     gui/derive_xpub.go:88) and §0.1b rules the payload and the keyboard this
//     phase's PRIMARY data entries. A payload-borne ClassMnemonic reaches
//     derivation on purpose. What survives is the split that is load-bearing:
//     seedEntryFlowTypedOnly (gui/derive_xpub.go:140), which the VERIFY flows
//     call so a payload-sourced secret is never compared against itself (§7.4).
//     ms1 is engraved onto owner-held steel only, never NFC.
//   - PER-LEG SCRUB (D11): the entropy is gated on mnemonic validity and wiped
//     inside deriveSingleSigBundle; the seed/master/intermediates are scrubbed
//     inside deriveAccountXpub; the mnemonic []Word is zeroed when this flow
//     returns (defer), after its last derivation consumer. The restore doc is
//     fully PUBLIC (masterFP/parentFP/xpub carry no secret).

// singleSigSeedHook is a test-only seam to observe the typed mnemonic slice (to
// assert it is scrubbed on exit, D11). nil in production.
var singleSigSeedHook func(bip39.Mnemonic)

func engraveSingleSigFlow(ctx *Context, th *Colors) {
	// The seed, through the ONE seam (D12). seedEntryFlow offers every source this
	// machine has; it is not keyboard-only.
	mnemonic, ok := seedEntryFlow(ctx, th)
	if !ok {
		return
	}
	if singleSigSeedHook != nil {
		singleSigSeedHook(mnemonic)
	}
	// Scrub the SECRET mnemonic on EVERY exit path (incl. abort), after its last
	// derivation consumer (D11).
	defer func() {
		for i := range mnemonic {
			mnemonic[i] = 0
		}
	}()

	// Wallet type (BIP-84 default + Advanced); mainnet-only.
	purpose, script, ok := singleSigPickFlow(ctx, th)
	if !ok {
		return
	}
	path := singleSigPath(purpose)

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

	// Full (engrave ms1+mk1+md1) vs watch-only (mk1+md1 + ms1 reminder).
	//
	// THE "FULL" ROW NAMES WHAT IT LEAVES OUT (F-198a), on this path too. The
	// passphrase taken above is a LIVE derivation input -- it reaches
	// deriveSingleSigBundle below and changes the master fingerprint and every
	// key under it -- while the ms1 this run engraves encodes the WORDS ONLY.
	// Measured on the same twelve words: the ms1 string is byte-identical with
	// and without a passphrase, and the master fingerprint is not. So the words
	// alone restore a DIFFERENT wallet, with no error anywhere, and
	// "Full (seed + keys)" over that promises a backup that does not reach the
	// money.
	//
	// The label is where it has to be said, because the label is what the
	// operator reads BEFORE pressing; a note anywhere else is a note read after
	// the decision. buildFullModeLabel already returns the correct string and was
	// wired to both multisig paths -- single-sig was the last holdout, which left
	// the asymmetry the wrong way round: the two paths that told the truth are
	// the multisig ones, and the flagship was the one that lied.
	modeChoice := &ChoiceScreen{
		Title:   "Engrave Mode",
		Lead:    "What to engrave?",
		Choices: []string{buildFullModeLabel(passphrase != ""), "Watch-only (keys)"},
	}
	modeSel, ok := modeChoice.Choose(ctx, th)
	if !ok {
		return
	}
	full := modeSel == 0

	// Derive the 3 legs (entropy gated + wiped inside; seed/master scrubbed inside
	// deriveAccountXpub). The mnemonic is consumed for the LAST time here.
	b, masterFP, parentFP, xpub, err := deriveSingleSigBundle(mnemonic, passphrase, &chaincfg.MainNetParams, path, script)
	if err != nil {
		showError(ctx, th, "Engrave Single-Sig", "Couldn't derive the single-sig bundle from the seed.")
		return
	}

	// Wallet-policy form: default FULL policy (recommended); opt-in TEMPLATE-only
	// behind the loud warning + recovery estimate (DD5/S4/S6). Single-sig always
	// classifies PolicySingle (no complex/depth consent here — I3). Aborting the
	// warning falls back to the full policy.
	template := false
	formChoice := &ChoiceScreen{
		Title:   "Engrave wallet policy",
		Lead:    "Which md1?",
		Choices: []string{"Full policy md1", "Template-only md1"},
	}
	if sel, ok := formChoice.Choose(ctx, th); ok && sel == 1 {
		// Refuse render-gap shapes BEFORE engrave (C4). Single-sig is always a
		// recoverable shape, but the guard is the single template-engrave gate.
		if gerr := md.TemplateEngraveShapeGuardChunks(b.MD1); gerr != nil {
			showError(ctx, th, "Engrave Single-Sig", "This wallet shape can't be safely engraved as a template (unrecoverable with the shipped toolkit). Use the full policy.")
			return
		}
		if confirmReviewScreen(ctx, th, "Template-only md1", templateWarningLines()) {
			tb, terr := templateizeBundle(b)
			if terr != nil {
				showError(ctx, th, "Engrave Single-Sig", "Couldn't build the template bundle.")
				return
			}
			b = tb
			template = true
		}
	}

	// Engrave (full = ms1+mk1+md1; watch-only = mk1+md1, + the ms1 reminder via
	// bundleEngrave's cards-derived gate).
	cards := singleSigEngraveCards(b, full)

	// HOW MANY PLATES, BEFORE THE FIRST ONE (F-202). The operator commits to a 2-
	// or 3-plate cut -- minutes per plate -- and until now no screen on this path
	// stated the count. Back here aborts before anything is cut, which is the
	// last moment that is free.
	//
	// THE TITLE IS THE OTHER FRONT-DOOR PATH'S, not the build path's. The build
	// census title is walk_s4_gate.js's anchor, and cmd/emu/needle_test.go
	// requires a walk's anchor to have exactly one production site; reusing it
	// here would make it two-site and break that gate. gui/multisig.go carries
	// the same note for the same reason. The BODY -- what the operator actually
	// reads, including the count -- comes from the shared buildPlateCensusLines
	// and is identical on all three paths.
	//
	// AND THIS NOTE MAY NOT SPELL THE BUILD TITLE EITHER. That counter matches
	// SOURCE BYTES, comments included (F-184), so a comment quoting a needle
	// costs it its uniqueness exactly as a second screen does. The first draft of
	// this comment named the string and turned the gate red; it is recorded here
	// because reading the warning next door did not prevent committing it.
	if !confirmReviewScreen(ctx, th, "Plates To Cut", buildPlateCensusLines(cards)) {
		return
	}

	// AN ABORT ENDS THE PROGRAM HERE (F-197). Everything below this line vouches
	// for a COMPLETE set: the verify offer over plates that were never all cut
	// (the md1 is emitted last, so the readback dies reading as "your plates are
	// unreadable"), and the restore document headed "This backup is N plates ...
	// If any of them is missing, this backup is incomplete." The abort modal is
	// the operator's last screen, and bundleAbortWarningText says so.
	//
	// The two multisig callers gained this gate at S5's I-12 and this one did
	// not, so a fix described as covering every engraving caller covered two of
	// the three that carry a post-engrave tail.
	if bundleEngrave(ctx, th, "Engrave Single-Sig", cards) != bundleEngraveDone {
		return
	}

	// Offer the verify-bundle (re-type seed → re-derive → read back → compare).
	//
	// THE RECORD IS DECLARED HERE, NOT INSIDE THE OFFER, so that a Skip leaves it
	// at its zero value and the document below says the weakest true thing. This
	// path has no retry loop -- the offer is a one-shot `if` -- so `rec` is written
	// at most once, and statusVerifiedOnRetry is unreachable from it by
	// construction rather than by an assertion.
	var rec verifyRecord
	verifyChoice := &ChoiceScreen{Title: "Verify Bundle", Lead: "Verify the engraved plates?", Choices: []string{"Verify now", "Skip"}}
	if sel, ok := verifyChoice.Choose(ctx, th); ok && sel == 0 {
		singleSigVerifyFlow(ctx, th, full, template, &rec)
	}

	// Watch-only restore doc (display-only, PUBLIC — no secret).
	//
	// THE STATUS IS WHAT THE VERIFY ABOVE RECORDED, and on a Skip that is the zero
	// cell -- `rec` is untouched and buildVerifyStatusLine renders the weakest of
	// the four lines. An empty string here would render as SILENCE, and silence is
	// what reads as a pass to the stranger holding the steel: an omission that
	// STRENGTHENS the claim, which is the one failure direction S6a G2 forbids.
	//
	// IT IS BUILT FROM THE RECORD, NOT FROM A STATUS. buildVerifyStatusLine
	// derives the cell itself, because a verifyStatus has already lost the mode
	// and the mode is what stops a watch-only pass line claiming an ms1
	// comparison that never ran.
	//
	// AND IT CARRIES THIS RUN'S SET (F-198b). It passed nil, and the document was
	// not one with a missing sentence -- it had no inventory AT ALL: four lines,
	// master fingerprint, descriptor and two addresses, saying no plate count, no
	// completeness claim and, the half that loses funds, nothing about the BIP-39
	// passphrase the run above may have derived from. A reader holding this pile
	// of steel in five years could not learn that a third spending factor was
	// ever in play.
	//
	// ONE SEED, so ONE FACT and seedCapacityOne. This flow has a single seed seam
	// by construction (seedEntryFlow, and nothing else in it reads a secret), so
	// its inventory takes the single-seed arm and its walk-away ruling names the
	// one seed the device is holding rather than the BUILD path's registry. It
	// carries no fingerprint because the single-seed arm renders none: with one
	// seed there is nothing to tell apart.
	restoreDocFlow(ctx, th, xpub, masterFP, parentFP, script, path,
		buildVerifyStatusLine(rec),
		buildPlateInventoryLines(cards, oneSeedPassphraseFact(passphrase != ""), seedCapacityOne))
}
