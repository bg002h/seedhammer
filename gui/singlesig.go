package gui

import (
	"fmt"

	"github.com/btcsuite/btcd/chaincfg/v2"
	"seedhammer.com/bip32"
	"seedhammer.com/bip39"
	"seedhammer.com/bundle"
	"seedhammer.com/md"
	"seedhammer.com/passphrase"
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

// singleSigInputs collects every operator answer this flow needs, with BACK
// MEANING BACK (2026-08-19 operator directive: "going back should lose
// nothing").
//
// seed → script → passphrase → engrave-mode is a step machine: each Back
// returns to the previous screen with the earlier answers intact, and only Back
// on the FIRST step leaves the program — which is what backing out of
// Engrave Single-Sig means.
//
// EXTRACTED rather than inlined. The caller's body is long and its own control
// flow is delicate; folding a step machine into it means re-targeting `return`s
// that already mean "abandon". Keeping the machine self-contained means the
// caller changes by exactly one call, and the brace structure of the code that
// engraves is untouched.
//
// ok == false means the operator left; the caller returns as it always did.
func singleSigInputs(ctx *Context, th *Colors) (
	mnemonic bip39.Mnemonic, purpose int, script md.ScriptKind,
	passphrase string, full, ok bool,
) {
	// SCRUB ON EVERY ABANDONED PATH (D11). The seed is created HERE now, so a
	// Back that leaves the program must zero it here too — the caller's own
	// scrub defer sees only what this function RETURNS, and returning nil on
	// failure would hand it nothing to zero while the typed words sat in the
	// backing array. Caught by TestEngraveSingleSigFlowSeedScrubbed, which is
	// exactly the exit path it exercises.
	//
	// The failure paths below `return` BARE for this reason: a bare return keeps
	// the named `mnemonic` pointing at the array, so this defer can still reach
	// it. `return nil, ...` would drop the reference before the defer ran.
	defer func() {
		if !ok {
			for i := range mnemonic {
				mnemonic[i] = 0
			}
		}
	}()

	const (
		stepSeed = iota
		stepScript
		stepPassphrase
		stepMode
		stepDone
	)
	step := stepSeed
	for step != stepDone {
		if ctx.Done {
			return
		}
		switch step {
		case stepSeed:
			// Resumes holding the words, so a Back from the script picker does
			// not blank a seed the operator already typed.
			m, got := seedEntryFlowResume(ctx, th, mnemonic)
			if !got {
				return
			}
			mnemonic = m
			// The hook fires AS SOON AS the seed exists, which is where it fired
			// before this flow became a step machine. Leaving it at the caller
			// would mean a Back that leaves the program never reaches it — and
			// the D11 scrub test, which captures through this hook, would have
			// nothing to assert on exactly the exit path it exercises.
			if singleSigSeedHook != nil {
				singleSigSeedHook(mnemonic)
			}
			step = stepScript
		case stepScript:
			pp, sc, got := singleSigPickFlow(ctx, th)
			if !got {
				step = stepSeed
				continue
			}
			purpose, script = pp, sc
			step = stepPassphrase
		case stepPassphrase:
			ppChoice := &ChoiceScreen{Title: "Passphrase", Lead: "Add a BIP-39 passphrase?", Choices: []string{"Skip", "Add passphrase"}}
			sel, got := ppChoice.Choose(ctx, th)
			if !got {
				step = stepScript
				continue
			}
			passphrase = ""
			if sel == 1 {
				// §3.3.2 admits ClassPassphrase to this program, so the payload is
				// offered before the keyboard (plan stage 13b). NOT passphraseFlow:
				// see syswPassphraseFlow for the two normative rules a shared edit
				// inside passphraseFlow would have broken.
				if pass, pok := syswPassphraseFlow(ctx, th); pok {
					passphrase = pass
				}
			}
			step = stepMode
		case stepMode:
			// THE "FULL" ROW NAMES WHAT IT LEAVES OUT (F-198a). The passphrase
			// taken above is a LIVE derivation input, while the ms1 this run
			// engraves encodes the WORDS ONLY — so the words alone restore a
			// DIFFERENT wallet. buildFullModeLabel says so on the label, which is
			// what the operator reads BEFORE pressing.
			modeChoice := &ChoiceScreen{
				Title:   "Engrave Mode",
				Lead:    "What to engrave?",
				Choices: []string{buildFullModeLabel(passphrase != ""), "Watch-only (keys)"},
			}
			modeSel, got := modeChoice.Choose(ctx, th)
			if !got {
				step = stepPassphrase
				continue
			}
			full = modeSel == 0
			step = stepDone
		}
	}
	return mnemonic, purpose, script, passphrase, full, true
}

func engraveSingleSigFlow(ctx *Context, th *Colors) {
	// The seed, through the ONE seam (D12). seedEntryFlow offers every source this
	// machine has; it is not keyboard-only.
	mnemonic, purpose, script, passphrase, full, ok := singleSigInputs(ctx, th)
	if !ok {
		return
	}
	// Scrub the SECRET mnemonic on EVERY exit path (incl. abort), after its last
	// derivation consumer (D11).
	defer func() {
		for i := range mnemonic {
			mnemonic[i] = 0
		}
	}()

	// Wallet type (BIP-84 default + Advanced); mainnet-only.
	path := singleSigPath(purpose)

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
	//
	// PLATE MARKING (S6b spec 1.2/1.3, R2/R3/R4). Computed here because this is
	// the ONLY call site allowed to mark -- multisig marking is a later phase,
	// R-B -- and because everything the predicate needs is already in scope:
	// full (:107's "the set contains a seed", R-A's predicate) and passphrase
	// (:72). masterFP is already the COMBINED fingerprint when a passphrase
	// was entered (deriveSingleSigBundle derived WITH it) and the bare-seed
	// one otherwise, so no extra derivation is needed for THIS marking --
	// unlike the passphrase plate's own SEED FP (S6b's P3), which needs a
	// second KDF when a passphrase WAS entered.
	plateTitle, plateFooter := singleSigPlateMark(full, passphrase != "", masterFP)
	if bundleEngrave(ctx, th, "Engrave Single-Sig", cards, plateTitle, plateFooter) != bundleEngraveDone {
		return
	}

	// Offer the verify-bundle (re-type seed → re-derive → read back → compare),
	// AND RE-OFFER IT on either adverse arm (S6b P9, F2).
	//
	// THE RECORD IS DECLARED OUTSIDE THE LOOP, so that a Skip (never entering
	// the loop body) leaves it at its zero value and the document below says
	// the weakest true thing, and so the adverse bit stays STICKY across
	// retries: a first attempt that failed and a second that passed is
	// statusVerifiedOnRetry on the document, not a bare pass -- exactly the
	// multisig callers' own reasoning (gui/multisig.go).
	//
	// UNTIL P9 THIS WAS A ONE-SHOT `if`, and the comment here said so on
	// purpose: a FAILED comparison or an unreadable readback dead-ended
	// straight past the passphrase-plate offer to the restore document, over
	// steel that was often fine, with no route back except re-cutting the
	// entire set -- the exact class F-199 fixed on the multisig side in this
	// same diff. singleSigVerifyFn's bool return is "can the operator still
	// act on this with what's in their hand"; true only at its two ADVERSE
	// return sites (gui/singlesig_verify.go).
	var rec verifyRecord
	verifyLead, verifyChoices := "Verify the engraved plates?", []string{"Verify now", "Skip"}
	for {
		verifyChoice := &ChoiceScreen{Title: "Verify Bundle", Lead: verifyLead, Choices: verifyChoices}
		sel, ok := verifyChoice.Choose(ctx, th)
		if !ok || sel != 0 {
			break
		}
		if !singleSigVerifyFn(ctx, th, full, template, passphrase != "", &rec) {
			break
		}
		// multisigVerifyRetryLead (gui/multisig_verify.go) is now drawn by
		// THREE call sites, not two -- see that constant's own comment.
		verifyLead = multisigVerifyRetryLead
		verifyChoices = []string{"VERIFY AGAIN", "CONTINUE"}
	}

	// PASSPHRASE PLATE OFFER (S6b spec 2.6, R2/R5/R6/R7). After the verify
	// offer (a plate is offered only for a set already known good) and
	// before restoreDocFlow (the document must be able to report whether
	// one was cut). mnemonic is still LIVE here: this function's top-level
	// scrub defer, registered right after seedEntryFlow returns, only fires
	// when engraveSingleSigFlow itself returns, and nothing between there
	// and here consumes the mnemonic again.
	//
	// THE RETURN IS READ BELOW (S6b spec 6/6a, P4): whether the restore
	// document may say a passphrase plate exists conditions on whether one
	// was actually CUT, not on whether this offer was merely shown --
	// singleSigPassphrasePlateOffer already collapses "declined" and
	// "backed out mid-engrave" to the same passphrasePlateNotCut a bare
	// void return could not have distinguished from "cut".
	plateResult := singleSigPassphrasePlateOffer(ctx, th, mnemonic, passphrase, masterFP, path, b)

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
	//
	// plateResult == passphrasePlateCut is S6b spec 6/6a's condition: CUT, not
	// offered. buildPassphraseInventoryLines reads exactly this bool, never
	// "was the offer shown" -- see the comment at the call above.
	restoreDocFlow(ctx, th, xpub, masterFP, parentFP, script, path,
		buildVerifyStatusLine(rec),
		buildPlateInventoryLines(cards, oneSeedPassphraseFact(passphrase != ""), seedCapacityOne,
			plateResult == passphrasePlateCut))
}

// singleSigPlateMark computes S6b spec 1.2's mk1/md1 title and footer for
// engraveSingleSigFlow, the ONLY caller allowed to mark (R-B moved every
// multisig path to a later phase). Keyed on R-A's predicate -- "the set
// contains a seed" -- which for this flow is exactly full: watch-only mode
// never carries the ms1 (singleSigEngraveCards), so R-A leaves it unmarked.
//
// masterFP must already be the fingerprint this run's cards were derived
// with: the COMBINED one when hasPassphrase, the bare-seed one otherwise --
// exactly what deriveSingleSigBundle returns at gui/singlesig.go:107. That is
// the mechanism R4 exploits: restoring the words alone yields a fingerprint
// that does not match what the mk1/md1 plates encode, so a wrong-wallet
// restore self-diagnoses instead of failing silently.
func singleSigPlateMark(full, hasPassphrase bool, masterFP uint32) (title, footer string) {
	if !full {
		return "", ""
	}
	fp := passphrase.GroupFingerprint(fmt.Sprintf("%.8X", masterFP))
	if hasPassphrase {
		return "PASSWORD REQUIRED", "COMB FP: " + fp
	}
	return "", "SEED FP: " + fp
}

// singleSigBareSeedFPHook is a test-only seam observing whether/when the
// LAZY bare-seed fingerprint derivation (S6b R-K, spec 2.6) actually ran.
// GATE 2.6 requires proving the ~31s KDF does NOT run when no passphrase
// plate is engraved; on the machine running this suite that derivation
// completes in well under a second (31s is the real device's cost, not this
// runner's), so a test cannot tell "ran" from "didn't" by timing it and
// instead counts invocations through this hook. Fired immediately before the
// derivation, so it counts attempts, not successes. nil in production;
// mirrors singleSigSeedHook.
var singleSigBareSeedFPHook func()

// singleSigPassphrasePlateOffer implements S6b spec 2.6: the passphrase-
// plate offer, and the lazy bare-seed derivation it gates. Factored out of
// engraveSingleSigFlow so GATE 2.6 -- "the offer appears only when
// passphrase != ” [and] the ~31s derivation does NOT run when no
// passphrase plate is engraved" -- can be driven directly rather than only
// through the whole orchestrator.
//
// mnemonic must still be LIVE (not yet scrubbed) when this is called: the
// bare-seed derivation reads it. masterFP must already be the COMBINED
// fingerprint (deriveSingleSigBundle derived WITH the passphrase whenever
// pass != "", which is the only case this function does anything) --
// exactly what singleSigPlateMark's own doc records about the same value.
// b must be the FINAL bundle, post-templateizeBundle if that branch was
// taken: the policy id is computed from b.MD1 (spec 2.4c).
//
// PARAMETER NAMED "pass", not "passphrase" (whole-diff review C2 fold,
// 2026-08-18): this function calls the passphrase PACKAGE
// (ValidatePassphrase, below), and a parameter named "passphrase" shadows
// the package identifier for this function's entire body -- the review's
// own proposed one-liner (`passphrase.ValidatePassphrase(passphrase)`)
// does not compile as written, for exactly this reason.
//
// RETURNS whether the plate was CUT (S6b spec 6/6a, P4): passphrasePlateNotCut
// when pass == "" (no offer reached at all), when pass does not fit a plate
// (C2 below), when the offer is declined, when the bare-seed derivation
// errors, or whatever engravePassphraseFlowPreloaded itself reports (a
// declined offer and an aborted engrave are indistinguishable to the
// caller, on purpose -- both leave the restore document's shipped sentence
// unchanged).
func singleSigPassphrasePlateOffer(ctx *Context, th *Colors, mnemonic bip39.Mnemonic, pass string, masterFP uint32, path bip32.Path, b bundle.Bundle) passphrasePlateResult {
	if pass == "" {
		return passphrasePlateNotCut
	}
	// C2 (S6b whole-diff review): the wallet passphrase itself is
	// unbounded -- neither passphraseFlowTitled (gui.go, the keyboard) nor
	// the payload branch of syswPassphraseFlowTitled (sysw_source.go)
	// validates it, because any length and any byte is fine for
	// DERIVATION. It is not fine for THIS PLATE:
	// engravePassphraseFlowPreloaded's buffer is exactly passphrase.MaxLen
	// bytes and its copy() truncates silently -- and the plate it then
	// builds would still carry the FULL passphrase's fingerprints and
	// policy id, stamped DERIVED, over a passphrase it mutilated. Catch it
	// HERE, before the offer even asks, rather than let it reach
	// engravePassphraseFlowPreloaded: ValidatePassphrase also catches a
	// non-ASCII payload passphrase, earlier and more truthfully than the
	// entry step's own refusal loop would.
	if err := passphrase.ValidatePassphrase(pass); err != nil {
		showError(ctx, th, "Passphrase Plate", ppEntryError(err))
		return passphrasePlateNotCut
	}
	offer := &ChoiceScreen{Title: "Passphrase Plate", Lead: "Engrave a passphrase plate?", Choices: []string{"Skip", "Engrave"}}
	sel, ok := offer.Choose(ctx, th)
	if !ok || sel != 1 {
		return passphrasePlateNotCut
	}
	// LAZY (R-K/spec 2.6): the bare-seed fingerprint costs a second ~31s
	// KDF (gui/gui.go's and gui/unlock_platelist.go's own uses of the same
	// derivation document that cost) and is paid ONLY by a run that
	// actually wants this plate -- never merely because the offer was
	// shown or a passphrase was entered. The combined fingerprint (masterFP)
	// is free: it already fell out of deriveSingleSigBundle's derivation.
	if singleSigBareSeedFPHook != nil {
		singleSigBareSeedFPHook()
	}
	_, seedFP, derr := deriveAccountXpub(mnemonic, "", &chaincfg.MainNetParams, path)
	if derr != nil {
		showError(ctx, th, "Passphrase Plate", "Couldn't derive the bare-seed fingerprint.")
		return passphrasePlateNotCut
	}
	// S6b spec 2.4: "wallet policy id" is md.FormAwareStubChunks -- NOT
	// md.WalletPolicyIDStub, the keyed-only branch reached through
	// FormAwareStub. "Template-only md1" is a reachable choice on this same
	// flow, and the keyed function would mint 4 bytes matching no card this
	// run cut. b is the caller's FINAL bundle (post-templateizeBundle if
	// chosen), so this is computed from the post-template md1 (spec 2.4c).
	var policyID string
	if stub, serr := md.FormAwareStubChunks(b.MD1); serr == nil {
		policyID = fmt.Sprintf("%X", stub[:])
	}
	return engravePassphraseFlowPreloaded(ctx, th, []byte(pass),
		fmt.Sprintf("%.8X", seedFP), fmt.Sprintf("%.8X", masterFP), policyID)
}
