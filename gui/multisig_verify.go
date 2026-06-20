package gui

import (
	"github.com/btcsuite/btcd/chaincfg/v2"
	"seedhammer.com/bundle"
	"seedhammer.com/codex32"
	"seedhammer.com/md"
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

// multisigVerifyFlow drives the on-device verify-bundle for the multisig flow:
// re-type the seed (fresh residency), gather the supplied md1 + the operator's
// engraved mk1 plate over NFC (extractSuppliedMd1AndMk1), re-cross-match to
// recover the operator's origin, re-derive the leg, hand-type the ms1 (full
// only; never NFC), and report PASS/FAIL — comparing the READ-BACK mk1 against
// the re-derived mk1 (H1: never the re-derived value against itself). `full`
// reports whether an ms1 was engraved (and so must be hand-typed for verify).
func multisigVerifyFlow(ctx *Context, th *Colors, derived bundle.Bundle, full bool) {
	reMnemonic, ok := seedEntryFlow(ctx, th)
	if !ok {
		return
	}
	defer func() {
		for i := range reMnemonic {
			reMnemonic[i] = 0
		}
	}()

	passphrase := ""
	ppChoice := &ChoiceScreen{Title: "Passphrase", Lead: "Add a BIP-39 passphrase?", Choices: []string{"Skip", "Add passphrase"}}
	if sel, ok := ppChoice.Choose(ctx, th); ok && sel == 1 {
		if pass, ok := passphraseFlow(ctx, th); ok {
			passphrase = pass
		}
	}

	// Read back the PUBLIC cards over NFC via the T5 gatherer.
	cards, ok := bundleGatherFlow(ctx, th)
	if !ok {
		return
	}
	suppliedMd1, suppliedMk1, ok := extractSuppliedMd1AndMk1(cards)
	if !ok {
		showError(ctx, th, "Verify Bundle", "Read back one wallet-policy md1 AND the operator key card (mk1).")
		return
	}
	_, keys, err := md.ExpandWalletPolicyChunks(suppliedMd1)
	if err != nil {
		showError(ctx, th, "Verify Bundle", "Couldn't decode the read-back wallet policy.")
		return
	}
	_, origin, _, ok := findUserSlot(reMnemonic, passphrase, &chaincfg.MainNetParams, keys)
	if !ok {
		showError(ctx, th, "Verify Bundle", "The seed is not a cosigner of the read-back policy.")
		return
	}
	reDerived, err := deriveMultisigLeg(reMnemonic, passphrase, &chaincfg.MainNetParams, origin, suppliedMd1, full)
	if err != nil {
		showError(ctx, th, "Verify Bundle", "Couldn't re-derive the bundle from the seed.")
		return
	}

	// Hand-type the SECRET ms1 (full mode only; never NFC).
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

	if err := verifyMultisig(reDerived, ms1Readback, suppliedMk1, suppliedMd1); err != nil {
		showError(ctx, th, "Verify Failed", "The read-back bundle does NOT match the seed. Check the engraved plates.")
		return
	}
	showNotice(ctx, th, "Verify OK", "The engraved bundle matches the seed.")
}
