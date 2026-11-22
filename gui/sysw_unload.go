package gui

import (
	"fmt"
	"strings"

	"seedhammer.com/sysw"
)

// UNLOAD — SPEC_systemwide_payloads §13 D10.
//
// THE FIRMWARE NEVER WRITES FLASH. This stage was planned as an ERASE: an
// RP2350 flash-range erase behind interrupt masking, a Platform method for it,
// a hardware rehearsal, and a risk classification, because it would have been
// the tree's first flash write. The operator ruled it out and the reason is not
// caution, it is honesty — erasing implies the bytes are gone, and a session
// drop does not remove them. So:
//
//   - the operator may UNLOAD: the records are dropped and the region at
//     0x10D00000 IS UNTOUCHED;
//   - THE WORD "erase" DOES NOT APPEAR ON THE DEVICE, because saying it would
//     be a claim the operator might act on;
//   - overwriting the region stays a HOST operation (`me sysw wipe`), and the
//     result screen says so rather than leaving the operator to find out.
//
// Nothing here is irreversible: the flash is not touched and the session dies at
// power-off anyway (§3.2.1). This carries the same risk as any other menu item,
// and that is the point of the ruling.

// syswPayloadMenu is the `Load Payload` carousel entry.
//
// With nothing loaded it IS the load flow, unchanged — the entry keeps its one
// meaning on a machine that has not loaded anything, which is every machine at
// boot.
//
// With a payload loaded it offers both things an operator can now want, and
// LOAD AGAIN comes first deliberately: re-reading the region is journey J-E (a
// second payload), it already worked, and putting only UNLOAD here would have
// made a working journey cost an extra step to reach. Unload is the new option,
// not the replacement one.
func syswPayloadMenu(ctx *Context, th *Colors) {
	if ctx.sysw == nil || !ctx.sysw.loaded {
		syswLoadFlow(ctx, th, ctx.Platform.SyswReader(), false)
		return
	}
	// THE KIT DOOR (SPEC_systemwide_payloads.md 14e). Every time this menu opens
	// on a payload that offers a kit: a refused one first shows "Kit Refused" and
	// then today's menu unchanged; an eligible one leads with "Engrave kit now
	// (N cards, M plates)?" and ENGRAVE KIT. A payload with no mr1 candidate sees
	// neither.
	kit := kitCards(ctx.sysw)
	if kit.offers && !kit.eligible {
		kitRefusedScreen(ctx, th, kit)
	}
	if kit.eligible {
		cs := &ChoiceScreen{
			Title:   "Load Payload",
			Lead:    kitLead(ctx, kit),
			Choices: []string{"ENGRAVE KIT", "LOAD AGAIN", "UNLOAD"},
		}
		choice, ok := cs.Choose(ctx, th)
		if !ok {
			return
		}
		switch choice {
		case 0:
			kitListFlow(ctx, th)
		case 1:
			syswLoadFlow(ctx, th, ctx.Platform.SyswReader(), false)
		default:
			syswUnloadFlow(ctx, th)
		}
		return
	}
	// CONTENT-DERIVED (§3.3, G-P3.15). The lead names what this payload
	// actually holds -- "this payload holds: 1 transaction, 2 seeds" is the
	// ruling's own example -- because the operator cannot otherwise tell a
	// payload with the WRONG contents from the right one, and those have
	// different fixes: re-pack, versus carry on.
	choices := []string{"LOAD AGAIN", "UNLOAD"}
	// ...and one content-derived ENTRY. Offered only when the payload holds
	// something this program can consume, so it is never a button that leads
	// to "this payload holds no transaction".
	toTransaction := syswPayloadHasTransaction(ctx.sysw)
	if toTransaction {
		choices = append([]string{"ENGRAVE TRANSACTION"}, choices...)
	}
	cs := &ChoiceScreen{
		Title:   "Load Payload",
		Lead:    "Loaded. It holds: " + txPayloadHolds(ctx.sysw) + ".",
		Choices: choices,
	}
	choice, ok := cs.Choose(ctx, th)
	if !ok {
		// BACK is the exit and must be, for the same reason syswUnloadFlow's
		// BACK is choice 0: this screen appears at boot now, and a screen that
		// appears unbidden must cost nothing to leave.
		return
	}
	if toTransaction {
		if choice == 0 {
			engraveTransactionFlow(ctx, th)
			return
		}
		choice--
	}
	if choice == 1 {
		syswUnloadFlow(ctx, th)
		return
	}
	syswLoadFlow(ctx, th, ctx.Platform.SyswReader(), false)
}

// syswPayloadHasTransaction reports whether the loaded payload holds anything
// the Engrave Transaction program can consume.
//
// Asked through the ADMISSION TABLE rather than by listing classes here: the
// table is §3.3.2 transcribed, and a second list of "what the transaction
// program eats" is a second answer to a question that already has one.
func syswPayloadHasTransaction(s *syswSession) bool {
	if s == nil || !s.loaded || !s.compared {
		return false
	}
	for _, r := range s.records {
		if admits(progTransaction, r.class) {
			return true
		}
	}
	return false
}

// syswUnloadFlow confirms, drops the session, and states plainly what did and
// did not happen. Returns true when a payload was unloaded.
//
// THE CONFIRM SCREEN IS THE FEATURE. Someone will unload by accident, and the
// screen that told them the cost beforehand is the difference between a shrug
// and a hunt for a passphrase. Reload is one carousel entry away — `Load
// Payload` is unconditional and syswLoadFlow assigns a fresh session — but
// `[compared]` (§12.2) must be RE-EARNED every time, and the three cases are
// genuinely asymmetric.
func syswUnloadFlow(ctx *Context, th *Colors) bool {
	if ctx.sysw == nil || !ctx.sysw.loaded {
		return false
	}
	cs := &ChoiceScreen{
		Title:   "Payload",
		Lead:    syswReloadCost(ctx.sysw),
		Choices: []string{"BACK", "UNLOAD"},
	}
	// BACK is choice 0 and therefore the DEFAULT: the screen exists because the
	// operator may not have meant it, so the resting position is the one that
	// costs nothing.
	choice, ok := cs.Choose(ctx, th)
	if !ok || choice == 0 {
		return false
	}
	// ctx.sysw = nil, and NOTHING ELSE. No Eraser, no Platform method, no
	// _tinygo.go file, no flash call anywhere.
	ctx.sysw = nil
	// SHORT, and plain ASCII. Reported from the panel: the previous body -- one
	// ~110-character sentence carrying an em dash and backticks -- rendered as an
	// almost entirely blank screen with only the checkmark. The test below
	// asserted its words and PASSED, because the frame extractor reads text ops
	// whether or not the device can draw them, so the harness cannot see this
	// class of defect at all (filed as F-151).
	showNotice(ctx, th, "Payload Unloaded",
		"Still in flash.\nWipe it from the host.")
	return true
}

// syswReloadCost words the confirmation FROM THE LOADED PAYLOAD'S OWN STATE.
//
// The three cases, each measured in the code rather than assumed:
//
//   - UNSEALED: sysw/open.go returns before DeriveKey, so there is NO KDF at
//     all. Reloading costs one digest comparison and is near-instant.
//   - SEALED, with a digest: a full passphrase entry and its KDF.
//   - SEALED with pub_len == 0: `[digest-shown]` (§12.4) shows nothing, so the
//     AEAD open is the ONLY route to `[compared]` (§12.2). The passphrase is the
//     only way back — the most expensive unload in the system, and the screen
//     says so most emphatically.
//
// A screen that named the passphrase in every case would be wrong twice over:
// wrong for the plaintext payload, which needs none, and unable to distinguish
// the case where it is the sole route.
func syswReloadCost(s *syswSession) string {
	const reload = "You can load it again from the menu. "
	switch {
	case s.sealed && !s.digestShown:
		return reload + "You will need the PASSPHRASE, and nothing else will do: " +
			"this payload shows no digest, so opening it is the only way back. Unload it?"
	case s.sealed:
		return reload + "You will need the passphrase. Unload it?"
	default:
		return reload + "You will need to compare the digest again. Unload it?"
	}
}

// kitRefusedScreen is the "Kit Refused" screen: the reason's device sentence, a
// blank line, and what it means (SPEC_systemwide_payloads.md 14e). Its only way
// on is to dismiss it.
func kitRefusedScreen(ctx *Context, th *Colors, v kitVerdict) {
	showError(ctx, th, "Kit Refused", kitSentence(v.refusal, v.records)+"\n\n"+
		"Nothing from this payload can be engraved as a kit. Write the kit payload again.")
}

// kitSentence is section 14g's sentence for a refusal, with the DEVICE
// placeholders: `<n>` is the record's position in the loaded payload counted
// from 1, public section first (the order of v.records).
func kitSentence(r sysw.MrRefusal, records []string) string {
	d := r.Detail
	rec := func(i int) string { return fmt.Sprintf("record %d of this payload", i+1) }
	R := func(i int) string { return fmt.Sprintf("Record %d of this payload", i+1) }
	trimmed := func(i int) string {
		if i < 0 || i >= len(records) {
			return ""
		}
		return strings.TrimSpace(records[i])
	}
	switch r.Reason {
	case sysw.MrCase:
		return R(d.Record) + " is an mr1 string with capital letters; the mr1 strings of a kit are all lowercase, as mr prints them."
	case sysw.MrCharacter:
		return R(d.Record) + " begins mr1 but holds a character no mr1 string uses."
	case sysw.MrLength:
		return fmt.Sprintf("%s begins mr1 but is %d characters long; an mr1 string is 17 to 96 characters long.", R(d.Record), len(trimmed(d.Record)))
	case sysw.MrChecksum:
		return R(d.Record) + " is an mr1 string whose checksum does not match: at least one character is wrong."
	case sysw.MrReservedSymbol:
		p := trimmed(d.Record)
		if len(p) > 4 {
			p = p[:4]
		}
		return fmt.Sprintf("%s begins %s, which no mr1 card or chunk begins with.", R(d.Record), p)
	case sysw.MrChunkShort:
		return R(d.Record) + " is an mr1 chunk too short to hold its chunk header."
	case sysw.MrChunkPadding:
		return R(d.Record) + " is an mr1 chunk that does not end the way mr ends a chunk (0 to 4 zero bits after its last byte)."
	case sysw.SetCount:
		return fmt.Sprintf("The mr1 card that begins %s disagrees with itself: %s says the card has %d chunks, but its first chunk says %d.",
			d.Begins, rec(d.Record), d.Count, d.FirstCount)
	case sysw.SetIndex:
		return fmt.Sprintf("The mr1 card that begins %s has %d chunks, numbered 0 to %d, but %s is numbered %d.",
			d.Begins, d.Count, d.Count-1, rec(d.Record), d.Index)
	case sysw.SetDuplicate:
		return fmt.Sprintf("The mr1 card that begins %s holds chunk %d twice: records %d and %d of this payload. "+
			"Each copy of a card holds each chunk once, and a copy must be complete before the next copy starts.",
			d.Begins, d.Index, d.Records[0]+1, d.Records[1]+1)
	case sysw.SetMissing:
		return fmt.Sprintf("The mr1 card that begins %s is incomplete: chunk %d of its %d chunks is missing "+
			"(chunks are numbered from 0), or a chunk repeated after a complete copy began a second, incomplete copy.",
			d.Begins, d.Index, d.Count)
	case sysw.SetSlices:
		return fmt.Sprintf("The mr1 card that begins %s is not divided into chunks the way mr divides a card, so mr did not write it.", d.Begins)
	case sysw.SetID:
		return fmt.Sprintf("The mr1 card that begins %s does not match its own chunk-set id: its chunks come from different cards, or one was altered.", d.Begins)
	case sysw.CardSplit:
		return fmt.Sprintf("The %s card that begins %s is split: %s sits between its strings but is not part of it. Keep each card's strings together.",
			d.Kind, d.Begins, rec(d.Record))
	case sysw.KitOtherRecord:
		return fmt.Sprintf("%s is not an md1, mk1 or mr1 string (it is: %s), so this payload is not a kit.", R(d.Record), txClassName(d.Class))
	case sysw.KitUnconfirmedMDMK:
		return R(d.Record) + " is an md1 or mk1 string whose card does not decode: it is incomplete or damaged."
	}
	return "This payload is not a kit."
}
