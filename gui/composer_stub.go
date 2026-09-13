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
	// These two legs return OriginsMoved, NOT IdMoved. By the time they run the
	// ids have been proved equal, so claiming the id changed would print that
	// sentence above a byte-identical Template-ID -- journey I-5's exact shape,
	// reintroduced by the error path of its own fix. Unreachable today
	// (ExpandWalletPolicy has no error returns), which is precisely why it is
	// worth getting right now rather than the day it becomes reachable.
	wasOrigins, err := composerAdvertisedOrigins(shown)
	if err != nil {
		return composerStubOriginsMoved
	}
	nowOrigins, err := composerAdvertisedOrigins(current)
	if err != nil {
		return composerStubOriginsMoved
	}
	// INTERSECTION, not equality. A slot that was advertising an origin and is
	// now seated simply LEAVES the instruction set; that is the operator
	// answering the instruction, not the instruction changing under them. Plain
	// equality counted the departure as a drift and warned on the one action
	// this screen is asking for.
	//
	// What is compared is every slot still asking for a card in BOTH readings,
	// which is exactly the set an operator could have minted for and could
	// still be holding.
	for idx, was := range wasOrigins {
		if now, ok := nowOrigins[idx]; ok && now != was {
			return composerStubOriginsMoved
		}
	}
	return composerStubUnchanged
}

// composerAdvertisedOrigins is the per-slot "expects a key at" facts -- the
// origins this screen is INSTRUCTING the operator to mint a card against.
//
// ONLY THE SLOTS STILL ADVERTISING ONE. A slot that is already seated is not an
// instruction, it is a report of what was seated, and including it made this
// comparison fire on a sole-slot policy the moment it was seated at a
// non-default account: "Cards minted for the old origins will not seat here"
// with no unseated slot left to mint for. That is the very class of false
// warning this predicate exists to remove, so it was worth a second pass to
// notice. Review I-2's case survives the restriction, because there it is @1
// and @2 -- both unseated -- that move.
//
// The split is FingerprintPresent, the SAME predicate composerStubLines uses to
// choose between "Slot @N: <fp> <path>" and "Slot @N expects a key at <path>".
// Sharing the predicate is the point: this compares exactly the rows the screen
// phrases as an instruction, so the comparison cannot drift away from the text
// it is guarding. (It is not XpubPresent. A declared origin carries a
// fingerprint and no xpub on a keyless template, so XpubPresent is false on
// every row and skipping by it skips nothing -- measured, by writing that
// version first and watching the sole-slot test still fire.)
func composerAdvertisedOrigins(chunks []string) (map[uint8]string, error) {
	_, keys, err := md.ExpandWalletPolicyChunks(chunks)
	if err != nil {
		return nil, err
	}
	out := make(map[uint8]string, len(keys))
	for _, k := range keys {
		if k.FingerprintPresent {
			continue // seated: a report of what was seated, not an instruction
		}
		out[k.Index] = k.OriginPath.String()
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
