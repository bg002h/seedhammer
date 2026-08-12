package gui

import "seedhammer.com/sysw"

// syswSourceName is what a screen calls a source. F3's whole point is that
// provenance is READ OFF THE SCREEN rather than established by reading code
// (§3.2), so the words live in one place and every entry point says the same
// thing.
func syswSourceName(src syswSource) string {
	switch src {
	case srcNFC:
		return "an NFC tag"
	case srcPayload:
		return "the systemwide payload"
	default:
		return "the keyboard"
	}
}

// syswSourceAccept is the acceptance screen at the point a record ENTERS a
// program — the one screen §3.2 (as scoped 2026-08-12, §13 D5) still requires to
// name its source, because it is the one that needs no session reshaping: the
// flow that accepted the offer knows what it accepted.
//
// Returns false when the operator declines, and a declining caller must fall
// back to its own input path rather than proceeding with the record.
//
// WHAT IT RENDERS, AND WHAT IT DELIBERATELY DOES NOT.
//
// F3 and F4 only. F1 and F2 are payload-level facts about the whole container,
// and syswLoadWarnings already states them ONCE at load over every class the
// payload holds; repeating them per record would say the same sentence up to
// seven times for one payload. F1's own offer belongs there too, beside the
// warning: since §13 D10 it offers to UNLOAD rather than to erase, and
// syswLoadFlow makes it. (This sentence used to end "F1's erase offer belongs to
// the erase item (§5.3.2) which does not exist yet" — the ruling deleted that
// item, and a comment naming an unbuilt thing nobody is going to build is how a
// stale condition outlives its claim.) They are not filtered out of syswFlags —
// the rule stays whole, in one place — they are simply not RENDERED here.
//
// `unconfirmed` is false at every call site by construction: §12.6 is about
// ClassMDMK, and the classes that reach an acceptance screen are Mnemonic,
// FreeText and Passphrase. A ClassMDMK consumer that grows one must pass the
// session's own value, which is why this takes the class rather than assuming
// it.
// syswPassphraseFlow is the optional-BIP-39-passphrase step with the payload
// offered first — §3.3.2's `ClassPassphrase` cell for the four seam programs,
// which the spec recorded as "admitted, no consumption path" (§3.3.2's
// reachability note).
//
// IT IS A WRAPPER, AND THAT IS A DEPARTURE FROM PLAN STAGE 13b, which said to
// put the offer at passphraseFlow's head — "one edit serving all four callers".
// Measured with grep before writing anything: passphraseFlow has TEN non-test
// callers, and the edit as written would have broken two NORMATIVE rules.
//
//   - §3.3.2: BACKUP WALLET REFUSES ClassPassphrase. It engraves the mnemonic
//     itself and the passphrase is deliberately never engraved and never in the
//     QR, so a passphrase reaching it has nowhere to go — and backupWalletFlow
//     calls passphraseFlow (gui.go), for its fingerprint choice.
//   - §7.4: singleSigVerifyFlow and multisigVerifyFlow call it too. A verify
//     that took half its re-derivation input from the session would be the
//     cache answering a verification prompt on the operator's behalf, which is
//     the rule test 16 exists to make structural. Test 16 could not have caught
//     it either: it scans `*_verify.go` for `seedEntryFlow`, and the payload
//     access would have been inside passphraseFlow, in gui.go.
//
// slip39_polish.go is the tenth caller and takes a SLIP-39 passphrase — a
// different thing entirely, and its own screen says so at length.
//
// So the seam is a function the five sites in the four admitting programs call,
// and passphraseFlow itself stays exactly what it was. A site that has no way to
// name this function cannot reach the payload by any argument, which is §3.1's
// own argument for two entry points over one boolean.
// THE BODY IS HEX-ENCODED and must be decoded. `pass:` is a RESERVED prefix
// (§5.3.1) and the record's body is lowercase hex, because EPD §6.4 forbids the
// interior spaces and newlines a passphrase may contain. Returning the record
// verbatim would hand every one of these four programs the literal string
// `pass:6162…` as a BIP-39 passphrase — deriving a different wallet, silently,
// with no screen able to show the difference. engravePassphraseFlowFrom and
// engraveTextFlowFrom both decode; this is the third site and the trap is the
// same at all three.
func syswPassphraseFlow(ctx *Context, th *Colors) (string, bool) {
	if body, ok := syswOffer(ctx, th, sysw.ClassPassphrase, "Password from where?"); ok {
		if raw, err := sysw.DecodeBody(body); err == nil {
			return string(raw), true
		}
		// A record that classified as ClassPassphrase and will not decode cannot
		// happen through Classify, which decodes before it answers. Falling
		// through to the keyboard rather than returning the raw record is the
		// arm that keeps that true if it ever stops being.
	}
	return passphraseFlow(ctx, th)
}

func syswSourceAccept(ctx *Context, th *Colors, title string, c sysw.Class, src syswSource) bool {
	if src == srcTyped {
		// F3 is "always, for anything not typed". A screen for typed input
		// would be a confirmation the operator has nothing to check.
		return true
	}
	var sealed, weak bool
	if ctx.sysw != nil {
		sealed, weak = ctx.sysw.sealed, ctx.sysw.weak
	}
	var lines []string
	for _, f := range syswFlags(c, false, src, sealed, weak) {
		switch f {
		case flagSource:
			lines = append(lines, "Source: "+syswSourceName(src))
		case flagNFCNoIntegrity:
			lines = append(lines,
				"This secret arrived with NO integrity check at all — nothing "+
					"stands behind a tag's contents, and there is nothing to compare.")
		}
	}
	return confirmReviewScreen(ctx, th, title, lines)
}
