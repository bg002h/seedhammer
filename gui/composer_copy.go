package gui

import "fmt"

// Every operator-facing string the wallet-policy COMPOSER draws, in one file
// (SPEC_wallet_policy_composer.md §8).
//
// ONE FILE, AND THE REASON IS THE GATE, NOT TIDINESS. §12 item 5 requires the
// glyph check, the raster floor, the modal-fits assertion and a
// fires-on-condition test for EVERY §8 body. "Every" is only checkable if the
// bodies are enumerable, so composer_copy_test.go AST-scans this file and
// fails when a composerCopy* function is missing from its table. A body
// written inline at its screen is a body nobody counted.
//
// ASCII ONLY. A non-ASCII rune does not degrade one glyph, it blanks the
// WHOLE modal body (gui/font_coverage_test.go), and an em dash measured 2652
// raster pixels against 7419 for the same line with a hyphen.
//
// THE SPEC'S HARD WRAP IS NOT PART OF THE STRING. §8 wraps its blockquotes at
// about 48 columns because that is a readable document; the panel wraps at
// the real face and the real width. So each body is ONE paragraph, and the
// only newlines are after an all-caps heading line and between a statement
// and the instruction that follows it.

// composerConfirmBody appends the hold-to-confirm instruction to a body shown
// on a ConfirmWarningScreen.
//
// It is separate so the §8 text stays verbatim: the instruction describes the
// CONTROL, not the policy, and gui/multisig_build.go:879 carries the same
// sentence for the same reason. The shipped prose test requires it (
// gui/multisig_build_prose_test.go:84).
func composerConfirmBody(body string) string {
	return body + "\n\nHold button to confirm."
}

// composerSlotWord renders "slot @3" or "slots @3 and @4", so a refusal never
// reads "slots @3".
func composerSlotWord(slots []uint8) string {
	if len(slots) == 1 {
		return fmt.Sprintf("slot @%d", slots[0])
	}
	return "slots " + composerSlotList(slots)
}

// composerSlotList joins slot labels the way a person reads them: "@1 and @2"
// for two, "@1, @2 and @3" beyond.
func composerSlotList(slots []uint8) string {
	switch len(slots) {
	case 0:
		return ""
	case 1:
		return fmt.Sprintf("@%d", slots[0])
	}
	out := ""
	for i, s := range slots {
		switch {
		case i == 0:
			out = fmt.Sprintf("@%d", s)
		case i == len(slots)-1:
			out += fmt.Sprintf(" and @%d", s)
		default:
			out += fmt.Sprintf(", @%d", s)
		}
	}
	return out
}

// ─── §8a, §8b: the two EXPERIMENTAL confirm-to-proceed bodies ────────────────

func composerCopyKeylessPath() string {
	return "KEY-LESS PATH (EXPERIMENTAL)\n" +
		"This path needs no signature. Whoever knows the preimage of its hash can " +
		"spend it. If that preimage is ever engraved, the plate is bearer access."
}

func composerCopyUnsortedKeys() string {
	return "UNSORTED KEYS (EXPERIMENTAL)\n" +
		"You chose unsorted keys where sorted was possible. Key order is part of " +
		"this wallet. Anyone restoring it must keep the same order. Sorted keys " +
		"need none."
}

// ─── §8c: the five lock echoes plus the two bound lines ──────────────────────

// composerCopyLockEchoDays echoes a relative TIME lock. Both the operator's
// days and the encoded units are printed, with the units converted BACK to
// days, because ceil() to 512-second units does not round-trip: the operator
// is entitled to see what the wallet will actually enforce.
func composerCopyLockEchoDays(days, units uint32) string {
	back := float64(units) * 512 / 86400
	return fmt.Sprintf("%d days = %d units of 512 s (%.1f days)", days, units, back)
}

// composerCopyLockEchoBlocks echoes a relative BLOCK lock (§6b's table).
// 600 seconds a block is the same figure §4c's "455.1 days" ceiling comes
// from: 65535 * 600 / 86400.
func composerCopyLockEchoBlocks(blocks uint32) string {
	return fmt.Sprintf("%d blocks (about %.1f days)", blocks, float64(blocks)*600/86400)
}

func composerCopyLockEchoHeight(height uint32) string {
	return fmt.Sprintf("Block %d", height)
}

func composerCopyLockEchoDate(year, month, day int) string {
	return fmt.Sprintf("%04d-%02d-%02d 00:00 UTC", year, month, day)
}

// composerCopyPackedDateBound is §8c's fourth body: the disclaimer WITH the
// payload's pack date. It never withdraws the disclaimer and never says
// "now" (§6b), because a stale now: record can only weaken the below-bound
// refusal, never invent one.
func composerCopyPackedDateBound(packDate string) string {
	return "This device cannot tell the time. The payload says it was packed on " +
		packDate + ", which may be long ago. Nothing here has checked that this " +
		"is in the future."
}

// composerCopyPackedHeightBound is the same body with §6b's height clause:
// "heights read `the packed height was H`". §8c prints the date form
// verbatim and §6b rules the height wording; this is the two joined, and it
// is the one string in this file assembled from two spec sentences rather
// than quoted from one.
func composerCopyPackedHeightBound(height uint32) string {
	return fmt.Sprintf("This device cannot tell the time. The payload says the packed "+
		"height was %d, which may be long ago. Nothing here has checked that this "+
		"is in the future.", height)
}

func composerCopyNoBound() string {
	return "This device cannot tell the time. Nothing here has checked that this " +
		"is in the future."
}

// ─── §8d, §8f ────────────────────────────────────────────────────────────────

func composerCopyOwnWallet() string {
	return "A wallet built here is its own wallet. The same rules written by " +
		"another tool give a different id and different addresses."
}

func composerCopyNUMS() string {
	return "KEY PATH: NONE (NUMS)\n" +
		"Spends use the script paths only. Bitcoin Core and Nunchuk import this " +
		"form. Liana and BIP-388 signers need an unspendable xpub instead (see " +
		"F-449)."
}

// ─── §8g: C29, one seed at two slots INSIDE one path ─────────────────────────

// composerCopySameSeedThreshold is §8g's FIRST body: the shared seed's slots
// in this path REACH the threshold, so one person can satisfy the path alone.
func composerCopySameSeedThreshold(slots []uint8, k, n int) string {
	return fmt.Sprintf("SAME SEED, SAME PATH\nSlots %s are the same seed. This path's "+
		"%d-of-%d can be satisfied by one person. Liana will refuse it.",
		composerSlotList(slots), k, n)
}

// composerCopySameSeedBelow is §8g's SECOND body: shared, but short of the
// threshold, so it says how much of it one person holds.
func composerCopySameSeedBelow(slots []uint8, k int) string {
	return fmt.Sprintf("SAME SEED, SAME PATH\nSlots %s are the same seed. One person "+
		"holds %d of the %d signatures this path needs. Liana will refuse it.",
		composerSlotList(slots), len(slots), k)
}

// ─── §8h, §8i, §8j, §8k, §8l ─────────────────────────────────────────────────

func composerCopyHashEveryPath() string {
	return "HASH ON EVERY PATH\n" +
		"Every way to spend this wallet needs the preimage of a hash. It is not " +
		"on this device and not on these plates. Back the preimage up separately."
}

func composerCopyHashRule() string {
	return "The hash must be SHA-256 of a 32-byte value. A passphrase must be " +
		"hashed to 32 bytes first, then hashed again. A hash of the passphrase " +
		"itself can never be spent."
}

func composerCopyEditClearsKeys() string {
	return "EDITING THE SHAPE CLEARS THE KEYS\n" +
		"Slot numbers change with the shape. Every key you seated will be " +
		"cleared. Continue?"
}

func composerCopyPersonInTwoPaths() string {
	return "One person in two paths needs two keys: a second account from the " +
		"same seed, or a second card."
}

// composerCopyNothingChecked is §8l.
//
// §8l names it "Multisig Build's warning, reused", and the SURFACE is reused
// -- an unskippable ConfirmWarningScreen -- but the STRING is not: the
// shipped body (gui/multisig_build.go:872-879) is a different, longer text,
// and §8 is the normative copy for this program. The shipped body is NOT
// edited by this cycle; changing a shipped screen's warning is not this
// stage's work.
func composerCopyNothingChecked() string {
	return "Nothing outside this device has checked this policy. Before you fund " +
		"it, restore these plates in your coordinator and compare your own first " +
		"receive address."
}

// ─── §8m: the five structural refusals (§4e) ─────────────────────────────────

func composerCopyRefuseNoKeyedPath() string {
	return "Every wallet needs at least one path with a key."
}

func composerCopyRefuseLockOnly() string {
	return "A path with only a time lock means anyone can spend after it. Add a " +
		"key or a hash."
}

func composerCopyRefuseKeylessTr() string {
	return "This build will not put a key-less path in taproot. Use wsh, or add a key."
}

func composerCopyRefuseLegacyShape() string {
	return "Legacy wrappers hold one plain multisig only. Use wsh or tr."
}

func composerCopyRefuseSlotCap() string {
	return "This wallet already has 32 key slots."
}

// ─── §8o, §8p, §8q ───────────────────────────────────────────────────────────

func composerCopyBelowBoundDate() string {
	return "That is before this payload was packed.\nChoose a later date."
}

func composerCopyBelowBoundHeight() string {
	return "That is before this payload was packed.\nChoose a later height."
}

// composerCopyShortfall is §8p. It names the counts and the unfilled slots
// and GUESSES NO CAUSE: the C5 lesson (a person in two paths needs two keys)
// is taught at the shape step by §8k, and a guess here would be a second,
// possibly wrong, explanation on the screen that refuses.
func composerCopyShortfall(slots, available int, unfilled []uint8) string {
	return fmt.Sprintf("%d slots, %d keys available.\nUnfilled: %s.",
		slots, available, composerSlotWord(unfilled))
}

func composerCopySelfCheckFailed() string {
	return "The policy on this device does not match what you built. Go back and " +
		"check the path list, or start again."
}

// ─── §8r: the door's key-state lines ─────────────────────────────────────────

func composerCopyKeysLoaded(n int) string {
	return fmt.Sprintf("Keys loaded: %d", n)
}

// composerCopyKeysAndSeeds pluralises the SEED noun with its own count (§7a),
// exactly as the not-understood line pluralises its record noun.
func composerCopyKeysAndSeeds(keys, seeds int) string {
	noun := "seeds"
	if seeds == 1 {
		noun = "seed"
	}
	return fmt.Sprintf("Keys loaded: %d, plus %d %s.", keys, seeds, noun)
}

// composerCopySeedOnly prints NO COUNT for the seeds, and that is §7a's rule
// rather than an omission: a seed fills any number of slots, so a count of
// seeds would answer a question the operator is not asking.
func composerCopySeedOnly() string {
	return "A seed is loaded. It can fill any number of slots."
}

func composerCopyNotUnderstood(n int) string {
	if n == 1 {
		return "1 payload record was not understood."
	}
	return fmt.Sprintf("%d payload records were not understood.", n)
}

func composerCopyNoKeys() string {
	return "No keys loaded. This builds a key-less template."
}

func composerCopyPayloadNotLoaded() string {
	return "A payload is in flash but not loaded.\nLoad it from the carousel first."
}

// ─── §8s: the stub screen's changed-id line and the two seating prompts ──────

func composerCopyIdChanged() string {
	return "The shape changed, so this id changed. Cards minted with the old stub " +
		"will not seat here."
}

// composerCopySeatPrompt names the OPERATOR's listed path index, never an
// emitted leaf index (§7d), beside the EMITTED slot index the labels use.
func composerCopySeatPrompt(slot uint8, path, keyIdx, keyCount int) string {
	return fmt.Sprintf("Slot @%d, Path %d key %d of %d: choose a key",
		slot, path, keyIdx, keyCount)
}

func composerCopySeatKeyPathPrompt(slot uint8) string {
	return fmt.Sprintf("Slot @%d, key path (spends alone): choose a key", slot)
}

// ─── §8t, §8u, §8v ───────────────────────────────────────────────────────────

func composerCopyDateFloor() string {
	return "This build will not write a date before 2009 as a time lock."
}

// composerCopyDateCeiling is the ceiling line §8 does not yet carry.
//
// §8t covers the FLOOR ("before 2009") and nothing covered the top, so a date
// past 2038-01-19 was refused as "that date does not exist" -- which is false
// of 2045-06-01 and leaves the operator retyping. The archetype §4d lists
// first is simple-timelocked-inheritance, where a twenty-year date is the
// ordinary case, so this is the sentence that use meets. Filed as a §8
// addition (F-456) so the spec's copy stays the enumerable source.
func composerCopyDateCeiling() string {
	return "This build writes dates up to 2038-01-19. For a later time, use a " +
		"block height instead."
}

func composerCopyRelativeCeiling() string {
	return "Relative locks reach at most 455 days in blocks or 388 days in time. " +
		"Use an absolute date."
}

func composerCopySameOriginFewFingerprints() string {
	return "Two keys declare the same origin and not both carry a fingerprint. " +
		"This template could not be restored. Use cards or records with " +
		"fingerprints."
}
