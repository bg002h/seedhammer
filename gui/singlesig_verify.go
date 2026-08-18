package gui

import (
	"github.com/btcsuite/btcd/chaincfg/v2"
	"seedhammer.com/bundle"
	"seedhammer.com/codex32"
)

// ─── T6a-2: verify-bundle for the single-sig flow ────────────────────────────
//
// singleSigVerifyFlow RE-TYPES the seed (D8 — a fresh, shorter residency
// window), RE-DERIVES the bundle deterministically, reads back the PUBLIC mk1 +
// md1 over NFC via the T5 bundleGatherer (R0-C1 — bundleGatherFlow yields
// bundleCard.strings for both kinds), hand-types the SECRET ms1 (full mode
// only; never NFC), assembles the read-back bundle.Bundle, and runs
// bundle.Verify. A FAIL is shown via showError, a PASS via showNotice. Watch-only
// verify omits the ms1 (both sides empty → bundle.Verify skips the ms1 leg).

// singleSigReadbackCards pulls exactly one mk1 and one md1 chunk-string set from
// a gathered card set (R0-C1: the T5 bundleGatherer's bundleCard.strings, NOT
// mk1GatherFlow/.collected()). It requires both a key card and a descriptor
// card; ok==false otherwise.
func singleSigReadbackCards(cards []bundleCard) (mk1, md1 []string, ok bool) {
	for _, c := range cards {
		switch c.kind {
		case cardMK1:
			if mk1 != nil {
				return nil, nil, false // more than one key card — ambiguous
			}
			mk1 = c.strings
		case cardMD1:
			if md1 != nil {
				return nil, nil, false // more than one descriptor — ambiguous
			}
			md1 = c.strings
		}
	}
	if len(mk1) == 0 || len(md1) == 0 {
		return nil, nil, false
	}
	return mk1, md1, true
}

// verifySingleSig assembles the read-back bundle and runs the deterministic
// comparator against the freshly re-derived bundle. For a watch-only verify the
// ms1Readback is "" AND the derived bundle's ms1 is dropped (both empty → the
// ms1 leg is skipped by bundle.Verify, R0-C1). Returns the comparator's first
// diverging-field error, or nil on PASS.
func verifySingleSig(derived bundle.Bundle, ms1Readback string, mk1, md1 []string) error {
	d := derived
	if ms1Readback == "" {
		// Watch-only: drop the derived ms1 so both sides are empty (the comparator
		// then skips the ms1 leg; a one-sided ms1 is a presence mismatch).
		d.MS1 = ""
	}
	readback := bundle.Bundle{MS1: ms1Readback, MK1: mk1, MD1: md1}
	return bundle.Verify(d, readback)
}

// singleSigVerifyFlow drives the on-device verify-bundle. full reports whether
// an ms1 was engraved (and so must be hand-typed for the verify). It re-types +
// re-derives the seed (the comparator baseline is re-derived internally, NOT
// passed in), reads back the public cards over NFC, optionally hand-types the
// ms1, and reports PASS/FAIL.
//
// `rec` IS AN OUT-PARAMETER (S6a §4.7b-seam): what the restore document needs
// is not a verdict but the two RECORDED facts -- was a pass observed, was
// anything adverse observed.
//
// THE BOOL RETURN (S6b P9, F2) IS A DIFFERENT THING FROM `rec`: it answers
// "can the caller offer a retry", not "what should the document say". Before
// P9 this flow was void and its only caller was a one-shot `if` -- a FAILED
// comparison or an unreadable readback dead-ended straight to the passphrase-
// plate offer and the restore document, over steel that was often fine, with
// no route back except re-cutting the entire set. The sibling multisig flow
// (multisigVerifyFlow) solved this with a five-way result enum its two
// callers loop on; this flow keeps the simpler two-way question its ONE
// caller actually asks -- "is this a state the operator can still act on with
// what's in their hand" -- as the return value, so `engraveSingleSigFlow` can
// wrap the offer in the same escapable retry loop.
//
// ELEVEN EXITS, ALL OF THEM RETURNS NOW. TWO OF THE ELEVEN ARE ADVERSE
// (return true: an accounting failure or a disagreeing comparator, both
// states the operator can fix by re-presenting plates or retyping) and eight
// write NEITHER bit and return false (a Back, a refusal, a re-derivation that
// never got as far as reading a plate -- nothing about the steel was
// observed, so recording a check there would be a false statement about this
// device's own behaviour, and retrying would mean asking to re-answer a
// question the operator already declined). The eleventh is the success
// return: also false (nothing left to retry), and the ONLY site that writes
// rec.pass.
//
// NO EXIT NAMES A verifyStatus. An exit that assigns a status has already lost
// the mode, and the mode is exactly what the pass line must not lose; the cell
// is derived once, downstream, from the two booleans.
func singleSigVerifyFlow(ctx *Context, th *Colors, full, template, engravedWithPassphrase bool, rec *verifyRecord) bool {
	// A NIL RECORD IS DISCARDED, NOT DEREFERENCED -- multisigVerifyFlow's reason.
	if rec == nil {
		rec = &verifyRecord{}
	}
	// Re-type the seed (fresh residency) and re-derive deterministically.
	reMnemonic, ok := seedEntryFlowTypedOnly(ctx, th)
	if !ok {
		return false
	}
	defer func() {
		for i := range reMnemonic {
			reMnemonic[i] = 0
		}
	}()
	purpose, script, ok := singleSigPickFlow(ctx, th)
	if !ok {
		return false
	}
	passphrase := ""
	ppChoice := &ChoiceScreen{Title: "Passphrase", Lead: "Add a BIP-39 passphrase?", Choices: []string{"Skip", "Add passphrase"}}
	if sel, ok := ppChoice.Choose(ctx, th); ok && sel == 1 {
		if pass, ok := passphraseFlow(ctx, th); ok {
			passphrase = pass
		}
	}
	reDerived, _, _, _, err := deriveSingleSigBundle(reMnemonic, passphrase, &chaincfg.MainNetParams, singleSigPath(purpose), script)
	if err != nil {
		showError(ctx, th, "Verify Bundle", "Couldn't re-derive the bundle from the seed.")
		return false
	}
	// For a TEMPLATE engrave the readback plates are the keyless template; the
	// re-derived comparator baseline must be templateized to match (C2).
	if template {
		reDerived, err = templateizeBundle(reDerived)
		if err != nil {
			showError(ctx, th, "Verify Bundle", "Couldn't re-build the template bundle.")
			return false
		}
	}

	// Read back the PUBLIC mk1 + md1 over NFC via the T5 gatherer.
	//
	// NO PAYLOAD OFFER HERE, deliberately (plan stage 13c). §3.3.2 admits
	// ClassMDMK to this program, but a verify READBACK must come from the
	// plate's own cards: §7.4's reasoning applied to the bundle rather than to
	// the seed — a readback taken from the session would compare the engrave
	// source against itself and pass unconditionally, certifying a wrong plate.
	// The passphrase step above uses passphraseFlow for the same reason.
	cards, ok := bundleGatherFlow(ctx, th, "Engrave Bundle")
	if !ok {
		return false
	}
	mk1, md1, ok := singleSigReadbackCards(cards)
	if !ok {
		// ADVERSE, AND RETRYABLE (S6b P9 F2). bundleGatherFlow returned ok, so
		// complete cards were read off the reader and the operator pressed Done;
		// the accounting over them then failed -- two mk1s, two md1s, or a
		// missing kind. "A plate could not be read or accounted for" is an exact
		// description of it, on the innocent world as much as the guilty one,
		// which is what separates this site from the ms1 typing below: there, no
		// check runs at all. It is retryable because it names precisely what to
		// re-present -- F-199's exact shape on the multisig side -- and the
		// caller's loop re-offers rather than dead-ending.
		rec.adverse = true
		showError(ctx, th, "Verify Bundle", "Need one key card (mk1) and one descriptor (md1) read back.")
		return true
	}

	// Hand-type the SECRET ms1 (full mode only; never NFC). Watch-only omits it.
	ms1Readback := ""
	if full {
		obj, ok := inputCodex32Flow(ctx, th, "Type ms1")
		if !ok {
			return false
		}
		s, isStr := obj.(codex32.String)
		if !isStr {
			showError(ctx, th, "Verify Bundle", "That isn't an ms1 secret share.")
			return false
		}
		// L1: capture + scrub the probe's secret entropy (codebase convention,
		// gui/ms1_decode.go:29) — DecodeMS1 allocates a fresh entropy slice we
		// otherwise abandon to the GC.
		_, _, ent, err := codex32.DecodeMS1(s)
		if err != nil {
			showError(ctx, th, "Verify Bundle", "That isn't a valid ms1 secret share.")
			return false
		}
		wipeBytes(ent)
		ms1Readback = s.String()
	}

	if err := verifySingleSig(reDerived, ms1Readback, mk1, md1); err != nil {
		// ADVERSE, AND RETRYABLE (S6b P9 F2). The comparator ran and disagreed;
		// a mistyped seed or passphrase is a state the operator can fix by
		// retyping, so the caller's loop re-offers rather than dead-ending into
		// the passphrase-plate offer and a permanently damning restore document
		// over steel that may be fine.
		rec.adverse = true
		// F-204 (S6b spec §3.2), extended by S6b P9's F3: THE COPY IS
		// CONDITIONAL, NOT A STRING SWAP, and now has THREE arms, not two. The
		// multisig sibling (multisigVerifyNoSlotBody, gui/multisig_verify.go)
		// suspects the PASSPHRASE before the plates -- one wrong character
		// re-types a whole different wallet -- and this screen used to blame the
		// plates unconditionally. `passphrase` is in scope from the prompt above
		// (:108-121) and `engravedWithPassphrase` is the caller's own fact
		// (gui/singlesig.go: `passphrase != ""` at the engrave, S6b P9 F3) --
		// together they decide which lead is TRUE:
		//
		//   - a passphrase was TYPED here: the mismatch is at least as likely a
		//     mistyped passphrase as a bad plate, so suspect it FIRST.
		//   - no passphrase was typed here, but the engrave USED one: there is a
		//     passphrase to blame -- the one just omitted -- and saying "check
		//     the plates" is a false lead against steel that may be perfect. This
		//     is F3: the caller holds this fact and previously plumbed none of it.
		//   - no passphrase was typed here, and the engrave used none either:
		//     there really is no passphrase to blame. The original wording stays
		//     true of this arm and is left alone rather than churned for its own
		//     sake.
		switch {
		case passphrase != "":
			showError(ctx, th, "Verify Failed", "The read-back bundle does NOT match the seed. "+
				"Check the passphrase before you doubt the plates: one wrong character "+
				"derives a different wallet.")
		case engravedWithPassphrase:
			showError(ctx, th, "Verify Failed", "The read-back bundle does NOT match the seed. "+
				"This set was engraved WITH a passphrase, and none was typed just now. Add "+
				"the passphrase and try again before you doubt the plates.")
		default:
			showError(ctx, th, "Verify Failed", "The read-back bundle does NOT match the seed. Check the engraved plates.")
		}
		return true
	}
	showNotice(ctx, th, "Verify OK", "The engraved bundle matches the seed.")
	// THE SUCCESS EXIT. `rec.pass` is written ONLY here, and nowhere else in
	// the function.
	//
	// legs IS 1 ON A TEMPLATE ENGRAVE TOO. templateizeBundle strips the MD1 to a
	// template and RE-STUBS the mk1 -- reStubMk1(b.MK1, stub) -- so the key plate
	// compared is a real key plate in both forms and `template` does not change
	// this count.
	//
	// suppliedCosigners IS 0 BY CONSTRUCTION, not by omission. There is no wallet
	// policy and no key on this path that this run did not itself derive and
	// compare, so there is no cosigner key for the count to miss -- and it is that
	// 0 which keeps "Other cosigners' keys are taken as supplied" OFF a single-sig
	// document, where it would misdescribe the wallet.
	rec.pass = &passRecord{
		full:              full,
		legs:              1,
		suppliedCosigners: 0,
	}
	// NOTHING LEFT TO RETRY: a clean pass needs no further action from the
	// caller's loop.
	return false
}

// singleSigVerifyFn is the in-file test seam engraveSingleSigFlow's retry
// loop dispatches through (S6b P9, F2) -- mirroring multisigVerifyFn
// (gui/multisig_verify.go). A real second attempt means driving a whole
// readback (gather, seed entry, passphrase, and for a full engrave the ms1)
// twice, which is affordable for a targeted proof (see
// TestSingleSigVerifyRetryProducesAnHonestStatusVerifiedOnRetryLine) but not
// for a walk that also has to prove the generic loop/label mechanics this
// flow now shares with both multisig callers -- substituting the verdict
// source here is what makes that walk cheap.
var singleSigVerifyFn = singleSigVerifyFlow
