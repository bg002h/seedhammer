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
