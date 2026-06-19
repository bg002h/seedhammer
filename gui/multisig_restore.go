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
	lines = []string{"Descriptor:"}
	lines = append(lines, chunkString(desc.Encode(), 20)...)
	lines = append(lines, "First receive:", recv0, "First change:", change0)
	return lines, true, nil
}

// desc4Display is a short, PUBLIC summary of an un-renderable template for the
// display-only path (no secret, no address). It reuses the shipped summary
// helpers used by the bundle review screen.
func desc4Display(tpl md.Template) string {
	return scriptName(tpl.Root) + " " + policyLine(tpl)
}

// multisigRestoreDocFlow displays the multisig restore doc on a plain, paged,
// read-only screen (the 0-alloc gate posture; reuse the single-sig
// restoreDocScreen). Display-only — no secret, no engrave.
func multisigRestoreDocFlow(ctx *Context, th *Colors, suppliedMd1 []string) {
	tpl, keys, err := md.ExpandWalletPolicyChunks(suppliedMd1)
	if err != nil {
		showError(ctx, th, "Restore Doc", "Couldn't decode the wallet policy.")
		return
	}
	lines, _, err := multisigRestoreLines(tpl, keys)
	if err != nil {
		showError(ctx, th, "Restore Doc", "Couldn't derive the restore addresses.")
		return
	}
	restoreDocScreen(ctx, th, lines)
}
