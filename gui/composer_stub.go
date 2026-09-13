package gui

import (
	"fmt"
	"slices"

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

// composerStubChange says WHAT about this screen moved since the operator last
// read it. Three states, because the screen makes two separable promises and
// one bool cannot tell them apart.
type composerStubChange int

const (
	composerStubUnchanged composerStubChange = iota
	// composerStubIdMoved: the Template-ID (or its kind) differs, so the stub
	// differs, so a card minted against the old stub fails layer 1.
	composerStubIdMoved
	// composerStubOriginsMoved: the id is the same and the per-slot origins
	// this screen tells the operator to mint against are not. The card passes
	// layer 1 -- the stub is the top four bytes of an unchanged id -- and fails
	// layer 2 in slotMatchesCard with errSeatNoSlot.
	composerStubOriginsMoved
)

// composerStubDelta compares what the operator was last shown with what they
// are being shown now.
//
// WHY NOT "DID THE CHUNKS CHANGE", and why not "did the id change" either.
//
// It compared chunk sets once, and journey I-5 measured the banner firing while
// the Template-ID printed two lines below it was byte-identical -- so the line's
// sentence, "this id changed", was false. Narrowing it to the id alone fixed
// that sentence and BROKE something else: review I-2 constructed a policy where
// seating slot @0 at an unusual account shifts the advertised origins of the
// still-unseated slots, under an id that cannot move (WalletDescriptorTemplateId
// is origin-invariant by construction, md/template_id.go:22-38). The operator
// had already written those origins down and minted a cosigner card from them.
// Narrowing removed a warning that was doing real work.
//
// So the banner was RIGHT TO FIRE and its text was wrong. Both facts the screen
// instructs the operator to copy are compared, and the caller says which one
// moved instead of asserting the id did.
//
// Unreadable input reports the ID as moved. Between a spurious warning and a
// missing one on a screen about to become steel, the spurious one is the
// survivable mistake.
func composerStubDelta(shown, current []string) composerStubChange {
	if len(shown) == 0 {
		return composerStubUnchanged
	}
	wasID, wasKind, err := md.FormAwareIdChunks(shown)
	if err != nil {
		return composerStubIdMoved
	}
	nowID, nowKind, err := md.FormAwareIdChunks(current)
	if err != nil {
		return composerStubIdMoved
	}
	// The KIND is compared too (review M-3). A template id and a policy id are
	// both 16 bytes of hex and differ for the same wallet; md/template_id.go
	// exists to keep the two spaces apart, and comparing the bytes alone would
	// let a crossing read as no change at all.
	if wasID != nowID || wasKind != nowKind {
		return composerStubIdMoved
	}
	wasOrigins, err := composerAdvertisedOrigins(shown)
	if err != nil {
		return composerStubIdMoved
	}
	nowOrigins, err := composerAdvertisedOrigins(current)
	if err != nil {
		return composerStubIdMoved
	}
	if !slices.Equal(wasOrigins, nowOrigins) {
		return composerStubOriginsMoved
	}
	return composerStubUnchanged
}

// composerAdvertisedOrigins is the per-slot "expects a key at" facts, in the
// form the screen prints them, so what is COMPARED is what was SHOWN.
//
// Deriving these separately from the lines the screen draws would let the two
// drift, and a drift here is silent by construction: the comparison would go on
// succeeding against facts nobody was ever told.
func composerAdvertisedOrigins(chunks []string) ([]string, error) {
	_, keys, err := md.ExpandWalletPolicyChunks(chunks)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		if k.XpubPresent && k.FingerprintPresent {
			out = append(out, fmt.Sprintf("@%d %x %s", k.Index, k.Fingerprint, k.OriginPath))
			continue
		}
		out = append(out, fmt.Sprintf("@%d %s", k.Index, k.OriginPath))
	}
	return out, nil
}

// composerStubLines builds the screen. `keyedChunks` is nil until a policy
// has been seated; when present the keyed id and stub are added and the
// screen recommends stamping BOTH (--policy-id-stub is repeatable).
func composerStubLines(templateChunks, keyedChunks []string, change composerStubChange) ([]string, error) {
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
	switch change {
	case composerStubIdMoved:
		lines = append(lines, composerCopyIdChanged(), "")
	case composerStubOriginsMoved:
		lines = append(lines, composerCopyOriginsChanged(), "")
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
func composerStubFlow(ctx *Context, th *Colors, templateChunks, keyedChunks []string, change composerStubChange) bool {
	lines, err := composerStubLines(templateChunks, keyedChunks, change)
	if err != nil {
		showError(ctx, th, "Template", "Couldn't read back the template this device just built.")
		return false
	}
	return composerReadScreen(ctx, th, "Template", lines)
}
