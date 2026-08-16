package gui

import (
	"seedhammer.com/address"
	"seedhammer.com/md"
)

// ─── T6b: multisig restore doc (NET-NEW render path, I-6) ────────────────────
//
// This is the multisig sibling of the single-sig restoreDocFlow, which takes
// single-sig scalars and CANNOT be reused (gui/singlesig_restore.go:118, R0-I3).
// Faithful-or-refuse: address-verify ONLY for the bip380-expressible (sortedmulti)
// subset (expandOK); a non-bip380 / template-only md1 yields a nil descriptor
// (gui/md1_expand.go:36-49), so this path is display-only with NO address.*
// call — a wrong-address verify is structurally impossible. Display-only, no
// secret.

// multisigRestoreLines builds the restore-doc display lines from a decoded
// supplied md1. On expandOK it shows the descriptor + first receive/change
// addresses (hasAddr=true). Otherwise it shows the descriptor template
// read-only with an "addresses unavailable" note and NO address (hasAddr=false).
func multisigRestoreLines(tpl md.Template, keys []md.ExpandedKey) (lines []string, hasAddr bool, err error) {
	desc, status := expandedToDescriptor(tpl, keys)
	if status != expandOK || desc == nil {
		// Display-only: no descriptor we can derive addresses from (faithful-or-refuse).
		lines = []string{
			"Wallet policy (read-only):",
		}
		lines = append(lines, chunkString(desc4Display(tpl), 20)...)
		lines = append(lines, "Addresses unavailable for this policy shape.")
		return lines, false, nil
	}
	recv0, err := address.Receive(desc, 0)
	if err != nil {
		return nil, false, err
	}
	change0, err := address.Change(desc, 0)
	if err != nil {
		return nil, false, err
	}
	// THE TYPE LINE IS ON THIS BRANCH TOO, and it was not until S3.
	//
	// SPEC §4.4 says of the three sh-rooted names that "the restore document is
	// the one that matters most", and §2.2 D-3 names gui/multisig_restore.go:51
	// as the call site that matters. MEASURED 2026-08-15, running S3's own gate
	// for the first time: that call site is desc4Display, which sits on the
	// display-only branch ABOVE, and a full-policy build never reaches it. Every
	// md1 the Build flow authors is bip380-expressible, so it lands HERE, where
	// the script type was named nowhere at all. The scriptName fix alone would
	// therefore have left S3's gate (an emulator walk of an sh(wsh) build showing
	// the nested-segwit NAME on the restore doc) with nothing to read.
	//
	// THE NAME IS NOT SPELT OUT IN THIS COMMENT ON PURPOSE. cmd/emu/needle_test.go
	// counts a walk needle's production sites by blunt substring match over gui's
	// source, comments included, so quoting it here would make the walk's anchor
	// look two-sited and cost it its uniqueness proof. Same reason
	// buildCosignerGatherTitle's comment does not quote its old title.
	//
	// The descriptor below does distinguish the two — it reads "sh(wsh(" against
	// "sh(" — but only to a reader who parses BIP-380 by eye off a string chunked
	// 20 characters to a line. The plain-language name is what §4.4 is about, and
	// it costs two lines.
	lines = []string{"Type:"}
	lines = append(lines, chunkString(desc4Display(tpl), 20)...)
	lines = append(lines, "Descriptor:")
	lines = append(lines, chunkString(desc.Encode(), 20)...)
	lines = append(lines, "First receive:", recv0, "First change:", change0)
	return lines, true, nil
}

// desc4Display is a short, PUBLIC summary of a wallet policy for the restore doc
// (no secret, no address). It reuses the shipped summary helpers used by the
// bundle review screen, and since S3 it names the sh-NESTING (SPEC §4.4) instead
// of collapsing sh(wsh) and bare sh onto one string.
//
// BOTH restore-doc branches call it: the display-only path uses it INSTEAD of a
// descriptor, and the expandOK path uses it as the plain-language heading above
// one.
func desc4Display(tpl md.Template) string {
	return scriptName(tpl) + " " + policyLine(tpl)
}

// multisigRestoreDocFlow displays the multisig restore doc on a plain, paged,
// read-only screen (the 0-alloc gate posture; reuse the single-sig
// restoreDocScreen). Display-only — no secret, no engrave. The caller passes the
// already-decoded tpl/keys (t6b-M2) so the wallet policy is not re-expanded.
// `extra` is appended verbatim below the policy. S4 uses it for the SET
// INVENTORY -- how many plates this backup is, and what each of them is -- which
// is the one fact that tells a reader holding a pile of steel in five years
// whether they are holding all of it. It is passed in rather than derived here
// because only the flow that engraved knows what it cut.
//
// BOTH ENGRAVING CALLERS PASS ONE. This comment used to end "the supply path has
// no set of its own and passes nil", which was false when it was written and
// grew worse: supplyEngraveTail returns `cardsOut` and F-188 made that path cut
// several plates. Passing nil there meant the front-door path's restore document
// stated no plate count and -- through buildPlateInventoryLines' passphrase arm
// -- never said that a BIP-39 passphrase is a required spending factor absent
// from every plate in the set.
func multisigRestoreDocFlow(ctx *Context, th *Colors, tpl md.Template, keys []md.ExpandedKey, extra []string) {
	lines, _, err := multisigRestoreLines(tpl, keys)
	if err != nil {
		showError(ctx, th, "Restore Doc", "Couldn't derive the restore addresses.")
		return
	}
	restoreDocScreen(ctx, th, append(lines, extra...))
}
