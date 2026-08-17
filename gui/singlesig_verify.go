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
// `rec` IS AN OUT-PARAMETER (S6a §4.7b-seam). This flow is void and stays void:
// its callers need no verdict, and what the restore document needs is not a
// verdict but the two RECORDED facts -- was a pass observed, was anything
// adverse observed.
//
// ELEVEN EXITS, AND THE ELEVENTH IS NOT A `return`. Ten explicit returns plus
// the fall-through at the closing brace, which is the ONLY success exit. An
// implementer told to "write the record at each return site" writes ten and
// never writes the pass at all, and the document then says NOT FULLY CHECKED on
// a run that fully checked.
//
// TWO OF THE ELEVEN ARE ADVERSE and eight write NEITHER bit. The adverse line
// this device prints says a check RAN and did not pass, so an exit may only set
// it if the device READ, compared, or accounted for what came back off the
// plates AND that step produced a negative result. Every other exit -- a Back, a
// refusal, a re-derivation that never got as far as reading a plate -- observed
// nothing about the steel, and claiming a check there would be a false statement
// about this device's own behaviour. Those land in the zero cell by writing
// nothing, which is how a prohibition fails safe.
//
// NO EXIT NAMES A verifyStatus. An exit that assigns a status has already lost
// the mode, and the mode is exactly what the pass line must not lose; the cell
// is derived once, downstream, from the two booleans.
func singleSigVerifyFlow(ctx *Context, th *Colors, full, template bool, rec *verifyRecord) {
	// A NIL RECORD IS DISCARDED, NOT DEREFERENCED -- multisigVerifyFlow's reason.
	if rec == nil {
		rec = &verifyRecord{}
	}
	// Re-type the seed (fresh residency) and re-derive deterministically.
	reMnemonic, ok := seedEntryFlowTypedOnly(ctx, th)
	if !ok {
		return
	}
	defer func() {
		for i := range reMnemonic {
			reMnemonic[i] = 0
		}
	}()
	purpose, script, ok := singleSigPickFlow(ctx, th)
	if !ok {
		return
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
		return
	}
	// For a TEMPLATE engrave the readback plates are the keyless template; the
	// re-derived comparator baseline must be templateized to match (C2).
	if template {
		reDerived, err = templateizeBundle(reDerived)
		if err != nil {
			showError(ctx, th, "Verify Bundle", "Couldn't re-build the template bundle.")
			return
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
		return
	}
	mk1, md1, ok := singleSigReadbackCards(cards)
	if !ok {
		// ADVERSE. bundleGatherFlow returned ok, so complete cards were read off the
		// reader and the operator pressed Done; the accounting over them then failed
		// -- two mk1s, two md1s, or a missing kind. "A plate could not be read or
		// accounted for" is an exact description of it, on the innocent world as
		// much as the guilty one, which is what separates this site from the ms1
		// typing below: there, no check runs at all.
		rec.adverse = true
		showError(ctx, th, "Verify Bundle", "Need one key card (mk1) and one descriptor (md1) read back.")
		return
	}

	// Hand-type the SECRET ms1 (full mode only; never NFC). Watch-only omits it.
	ms1Readback := ""
	if full {
		obj, ok := inputCodex32Flow(ctx, th, "Type ms1")
		if !ok {
			return
		}
		s, isStr := obj.(codex32.String)
		if !isStr {
			showError(ctx, th, "Verify Bundle", "That isn't an ms1 secret share.")
			return
		}
		// L1: capture + scrub the probe's secret entropy (codebase convention,
		// gui/ms1_decode.go:29) — DecodeMS1 allocates a fresh entropy slice we
		// otherwise abandon to the GC.
		_, _, ent, err := codex32.DecodeMS1(s)
		if err != nil {
			showError(ctx, th, "Verify Bundle", "That isn't a valid ms1 secret share.")
			return
		}
		wipeBytes(ent)
		ms1Readback = s.String()
	}

	if err := verifySingleSig(reDerived, ms1Readback, mk1, md1); err != nil {
		// ADVERSE. The comparator ran and disagreed.
		rec.adverse = true
		showError(ctx, th, "Verify Failed", "The read-back bundle does NOT match the seed. Check the engraved plates.")
		return
	}
	showNotice(ctx, th, "Verify OK", "The engraved bundle matches the seed.")
	// THE SUCCESS EXIT, and it is the fall-through rather than a return.
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
}
