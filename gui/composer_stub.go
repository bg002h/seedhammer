package gui

import (
	"fmt"

	"seedhammer.com/md"
)

// The stub-teaching screen (SPEC §7c, C9, §9 item 6).
//
// SHOWN UNCONDITIONALLY once the shape is complete, and RE-SHOWN after any
// shape edit. The template id is key-independent and origin-invariant but NOT
// shape-invariant: the wrapper, the path list, every lock operand and every
// hash digest enter it, so an operator who wrote the stub down and then
// changed a digit is holding a stub that will not seat. §8s says so, and
// gui/key_card_seating.go:63-73 is why it matters -- layer 1 refuses a card whose
// stub set does not include this template's, before any origin is compared.
//
// THE ORIGINS COME FROM THE DECODED CHUNKS. ExpandWalletPolicyChunks resolves
// each slot's origin through the same precedence the consuming path uses
// (md/expand.go:115-135), so the "expects a key at" line the operator writes
// down is the origin slotMatchesCard will actually compare a card against.
// Reading it off composerState would print a promise instead of a fact.
//
// PAGED, because the body grows one line per slot and the grammar admits 32.

// composerStubLines builds the screen. `keyedChunks` is nil until a policy
// has been seated; when present the keyed id and stub are added and the
// screen recommends stamping BOTH (--policy-id-stub is repeatable).
func composerStubLines(templateChunks, keyedChunks []string, changed bool) ([]string, error) {
	tid, tkind, err := md.FormAwareIdChunks(templateChunks)
	if err != nil {
		return nil, err
	}
	tstub, err := md.FormAwareStubChunks(templateChunks)
	if err != nil {
		return nil, err
	}
	_, keys, err := md.ExpandWalletPolicyChunks(templateChunks)
	if err != nil {
		return nil, err
	}

	var lines []string
	if changed {
		lines = append(lines, composerCopyIdChanged(), "")
	}
	// The LABELS ARE LITERAL (§7c): "Template-ID:" and "Policy-ID:" for the
	// 32-hex ids, "mk1 stub (template):" and "mk1 stub (policy):" for the
	// 8-hex stubs. tkind renders the first pair itself, so a template can
	// never be labelled with a policy's word.
	lines = append(lines,
		fmt.Sprintf("%s: %x", tkind, tid),
		fmt.Sprintf("mk1 stub (template): %x", tstub),
	)
	if len(keyedChunks) > 0 {
		kid, kkind, err := md.FormAwareIdChunks(keyedChunks)
		if err != nil {
			return nil, err
		}
		kstub, err := md.FormAwareStubChunks(keyedChunks)
		if err != nil {
			return nil, err
		}
		lines = append(lines,
			fmt.Sprintf("%s: %x", kkind, kid),
			fmt.Sprintf("mk1 stub (policy): %x", kstub),
			"Stamp BOTH stubs on each key card:",
			fmt.Sprintf("--policy-id-stub %x --policy-id-stub %x", tstub, kstub),
		)
	}
	lines = append(lines,
		"",
		"mk encode --xpub <xpub> --origin-fingerprint <fp>",
		fmt.Sprintf("  --origin-path <path> --policy-id-stub %x", tstub),
		"",
		composerCopyOwnWallet(),
		"",
	)
	// One line per slot. A slot that will stay UNSEATED names the origin a
	// card must declare; a seated one names the source's own declaration
	// instead (§7c).
	for _, k := range keys {
		if k.FingerprintPresent {
			lines = append(lines, fmt.Sprintf("Slot @%d: %x %s",
				k.Index, k.Fingerprint, k.OriginPath))
			continue
		}
		lines = append(lines, fmt.Sprintf("Slot @%d expects a key at %s",
			k.Index, k.OriginPath))
	}
	return lines, nil
}

// composerStubFlow shows the screen. Back returns false so the caller can
// send the operator back to the shape.
func composerStubFlow(ctx *Context, th *Colors, templateChunks, keyedChunks []string, changed bool) bool {
	lines, err := composerStubLines(templateChunks, keyedChunks, changed)
	if err != nil {
		showError(ctx, th, "Template", "Couldn't read back the template this device just built.")
		return false
	}
	return composerReadScreen(ctx, th, "Template", lines)
}
