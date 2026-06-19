package gui

import (
	"github.com/btcsuite/btcd/chaincfg/v2"
	"seedhammer.com/bip39"
)

// ─── T6a-2: the single-sig flagship orchestrator ─────────────────────────────
//
// engraveSingleSigFlow is the engraveSingleSig program: ONE hand-typed BIP-39
// seed → wallet-type pick (BIP-84 default + Advanced) → optional passphrase →
// full-or-watch-only → derive ms1+mk1+md1 (policy-bound) → engrave (full = 3
// cards incl. the secret ms1; watch-only = 2 cards + the ms1 reminder) → offer
// verify-bundle → watch-only restore doc.
//
// SECURITY SPINE:
//   - TYPED-ONLY seed (D12): the seed comes from seedEntryFlow ONLY; this flow
//     NEVER routes an NFC-scanned object into derivation (the scan.go bip39 /
//     codex32 footgun). ms1 is engraved onto owner-held steel only, never NFC.
//   - PER-LEG SCRUB (D11): the entropy is gated on mnemonic validity and wiped
//     inside deriveSingleSigBundle; the seed/master/intermediates are scrubbed
//     inside deriveAccountXpub; the mnemonic []Word is zeroed when this flow
//     returns (defer), after its last derivation consumer. The restore doc is
//     fully PUBLIC (masterFP/parentFP/xpub carry no secret).

// singleSigSeedHook is a test-only seam to observe the typed mnemonic slice (to
// assert it is scrubbed on exit, D11). nil in production.
var singleSigSeedHook func(bip39.Mnemonic)

func engraveSingleSigFlow(ctx *Context, th *Colors) {
	// TYPED-ONLY seed (D12). Never a scan.
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
		if pass, ok := passphraseFlow(ctx, th); ok {
			passphrase = pass
		}
	}

	// Full (engrave ms1+mk1+md1) vs watch-only (mk1+md1 + ms1 reminder).
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

	// Derive the 3 legs (entropy gated + wiped inside; seed/master scrubbed inside
	// deriveAccountXpub). The mnemonic is consumed for the LAST time here.
	b, masterFP, parentFP, xpub, err := deriveSingleSigBundle(mnemonic, passphrase, &chaincfg.MainNetParams, path, script)
	if err != nil {
		showError(ctx, th, "Engrave Single-Sig", "Couldn't derive the single-sig bundle from the seed.")
		return
	}

	// Engrave (full = ms1+mk1+md1; watch-only = mk1+md1, + the ms1 reminder via
	// bundleEngrave's cards-derived gate).
	cards := singleSigEngraveCards(b, full)
	bundleEngrave(ctx, th, cards)

	// Offer the verify-bundle (re-type seed → re-derive → read back → compare).
	verifyChoice := &ChoiceScreen{Title: "Verify Bundle", Lead: "Verify the engraved plates?", Choices: []string{"Verify now", "Skip"}}
	if sel, ok := verifyChoice.Choose(ctx, th); ok && sel == 0 {
		singleSigVerifyFlow(ctx, th, b, full)
	}

	// Watch-only restore doc (display-only, PUBLIC — no secret).
	restoreDocFlow(ctx, th, xpub, masterFP, parentFP, script, path)
}
