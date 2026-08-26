package gui

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
