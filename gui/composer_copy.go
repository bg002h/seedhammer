package gui

import (
	"fmt"

	"seedhammer.com/hashlock"

	"seedhammer.com/md"
)

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

// composerCopyKeylessPath is §8a. It names TWO consequences, and the second
// arrived with the fable review r0 (lens 1 I-1 and lens 3 I-1 found it
// independently, from opposite ends).
//
// The body used to stop at bearer access, which is true and is not the whole
// price. ONE key-less path makes the WHOLE wallet un-importable -- keyed paths
// included -- because the descriptor as a whole is what a coordinator refuses:
// measured on Bitcoin Core v25.0 and v31.1, `getdescriptorinfo` reports
// "witnesses without signature exist" and never issues a checksum, so
// `importdescriptors` cannot even be reached; libnunchuk 2.1.1 refuses;
// Liana refuses any hashlock path. The Rust primary ADMITS the shape, so
// nothing upstream of this screen says it either, and the operator who funds
// one discovers it at restore -- with the money already in.
//
// THAT IS WHY IT SAYS "ANY OTHER WALLET" AND NOT "md ONLY CAN RESTORE IT".
// md restores the wallet in the sense of reconstructing the descriptor and
// deriving addresses; it does not sign. The honest sentence is about what can
// WATCH and SPEND, which is what an operator about to fund is deciding.
func composerCopyKeylessPath() string {
	return "KEY-LESS PATH (EXPERIMENTAL)\n" +
		"This path needs no signature. Whoever knows the preimage of its hash can " +
		"spend it. If that preimage is ever engraved, the plate is bearer access.\n" +
		"It also makes the WHOLE wallet un-importable, keyed paths included. " +
		"Bitcoin Core, Nunchuk and Liana all refuse it. Only md can rebuild this " +
		"wallet, and md cannot sign: no other wallet will watch it or spend from it."
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

// composerCopyNUMS is §8f. It used to say "Bitcoin Core and Nunchuk import
// this form", and the Nunchuk half was FALSE (fable review r0 lens 2 I-1).
//
// MEASURED BY RUNNING libnunchuk 2.1.1, not by reading it: every NUMS-keyed
// tr shape the composer can emit is refused by `Utils::ParseWalletDescriptor`
// -- the function the desktop app calls -- in both the multipath and the
// single-chain spellings, 14 of 14, code=-1017. Two mechanisms, and between
// them they cover the whole space: `sortedmulti_a` is a descriptor function
// and not a miniscript fragment, so its template validator refuses a plain
// k-of-n under tr outright; and every other NUMS shape is stamped
// DISABLE_KEY_PATH, which re-renders the key path as an unspendable XPUB and
// then requires the re-rendered string to match the input, which a raw 32-byte
// H never does.
//
// THE WAY OUT IS NAMED, AND IT IS NOT THE UNSPENDABLE XPUB. That form is the
// one Nunchuk accepts and it is a DIFFERENT WALLET: measured, one preset
// derives bc1pm0udr8a... through the xpub form and bc1pac935qv... through the
// raw-H form this device cuts, because the xpub form derives per-index
// children of H and the raw form does not. The F-449 sentence stays because
// it is true of Liana and BIP-388 signers, for whom the xpub wallet is the
// wallet they wanted; sending a NUNCHUK operator there would hand them a
// different wallet under the name of this one.
func composerCopyNUMS() string {
	return "KEY PATH: NONE (NUMS)\n" +
		"Spends use the script paths only. Bitcoin Core imports this form. " +
		"Nunchuk cannot import a NUMS policy at all: for Nunchuk, use wsh, or a " +
		"tr policy whose first path is a single key. Liana and BIP-388 signers " +
		"need an unspendable xpub instead (see F-449), which is a different " +
		"wallet with different addresses."
}

// composerCopyMixedLockBases is the fable review r0 lens-2 I-2 notice, in the
// register of §8g's Liana line: one sentence naming the wallet that refuses
// and one naming the way round it.
//
// libnunchuk 2.1.1's MiniscriptTimeline walks the WHOLE wsh script and throws
// "Timelock mixing" on the first lock whose base -- TIME vs HEIGHT -- differs
// from any earlier one, regardless of relative-vs-absolute and regardless of
// which `or` branch it sits in. ParseDescriptors swallows the exception, so
// the app shows only "Could not parse descriptor" and the operator has
// nothing to act on. Bitcoin Core imports the same descriptor: miniscript's
// own rule only forbids mixing inside ONE satisfaction.
//
// THE AXIS IS THE BASE, NOT relative-vs-absolute, and getting that wrong
// would put this notice on a shipped preset: decaying-multisig mixes
// older(blocks) with after(height), which are both HEIGHT, and imports.
func composerCopyMixedLockBases() string {
	return "MIXED LOCK BASES\n" +
		"Some paths lock by block height and others by time. Nunchuk will refuse " +
		"this wallet; Bitcoin Core imports it. Taproot accepts both, because it " +
		"checks each path on its own."
}

// composerCopyOutsideLianaModel is the fable review r0 lens-5 I-1/I-2 notice:
// one FIXED head naming Liana's own model, and one VARIABLE sentence naming
// the first way, in Liana's own order of refusal, that this policy sits
// outside it (composerLianaOutsideModelClass picks `class`).
//
// LIANA v8.0's MODEL, MEASURED BY RUNNING `LianaDescriptor::from_str` -- the
// exact call the GUI's "Import the wallet" screen makes
// (`step/descriptor/mod.rs:47`), which discards the reason and shows only
// "Failed to read the descriptor" -- IS: exactly one unlocked path, at least
// one path locked by `older` in BLOCKS, and no hash anywhere. 17 of the 56
// composable shapes fit it; the other 39 fall into nine classes, and the
// device named three of them (NUMS at §8f, same-seed at §8g twice) before
// this notice. The demo payload's own plain 2-of-3 is in the largest silent
// class -- no recovery path at all -- and an operator who composes "me now, my
// heir after 2027-01-01" (`after`, the most natural inheritance clock) or the
// hashlock-gated preset with a KEYED hash path was told nothing.
//
// ONE SENTENCE, NOT NINE. §7e's consent has one line budget per fact and a
// policy usually fails more than one of Liana's checks at once (I-2's X24
// fails the second-unlocked-path check AND could be misread as "no unlocked
// path" if it had none) -- so this names the FIRST class that applies, in
// Liana's own order of refusal, and composerLianaOutsideModelClass is the
// only place that order is written down.
func composerCopyOutsideLianaModel(class string) string {
	return "OUTSIDE LIANA'S MODEL\n" +
		"Liana takes one unlocked path, at least one path locked by older in " +
		"blocks, and no hash. This policy: " + class + ". Bitcoin Core imports it."
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

// §8h, the plain form: every path is hashed and NO current path's digest came
// from a phrase typed here (composerCopyHashEveryPathFor).
//
// "EVERY PREIMAGE", NOT "THE PREIMAGE" (r0 journey I-2). Two paths can carry two
// DIFFERENT digests, which is two different preimages the operator must hold,
// and the shipped sentence named one. That is the same undercount H5 §2 item 5
// removed from the phrase form above, on the sibling body that chooses against
// it -- leaving one counted and one not would make the two forms disagree about
// what spending needs.
func composerCopyHashEveryPath() string {
	return "HASH ON EVERY PATH\n" +
		"Every way to spend this wallet needs the preimage of a hash. It is not " +
		"on this device and not on these plates. Back up every preimage separately."
}

// §8i SPLITS IN TWO (SPEC_hashlock_kinds §7.2), and WHEN each body fires is
// what forces the split -- not a wish for two wordings.
//
// composerCopyHashRule is the ENTRY body. It fires on row selection in
// `Path N hash`, which is before either typed arm is entered and therefore
// BEFORE A KIND EXISTS -- so there is no kind to name and it must not invent
// one. The shipped body said the hash must be "SHA-256 of a 32-byte value".
// That was true of a one-kind world and is FALSE the moment a hash256 or
// ripemd160 wallet can be built here, which is what this cycle does.
//
// It is reworded to the part that holds for all four -- the preimage is 32
// bytes -- and names no function at all. Do NOT "fix" this later by naming
// sha256 as the default: the screen below it offers four kinds, and a rule
// modal asserting one of them is worse than one asserting none.
func composerCopyHashRule() string {
	return "The preimage must be a 32-byte value. A passphrase must be " +
		"hashed to 32 bytes first, then hashed again. A hash of the passphrase " +
		"itself can never be spent."
}

// composerCopyHashRuleForKinds is the CONSENT body. By then the policy is
// decided, so the kinds ARE known, and §6's both-or-neither rule applies: where
// the kind axis can be read, it is named.
//
// IT TAKES A SET, and that is not over-engineering. §7.2 says "names the kind",
// singular, but a policy may carry two paths with two different kinds -- naming
// one of them would be a false statement about the other path, printed on the
// one screen the operator consents from. The spec's singular is a gap its own
// §5 data model allows; this resolves it toward saying less confidently rather
// than saying something wrong.
func composerCopyHashRuleForKinds(kinds []md.HashKind) string {
	const tail = " A passphrase must be hashed to 32 bytes first, then hashed " +
		"again. A hash of the passphrase itself can never be spent."
	switch len(kinds) {
	case 0:
		// Unreachable from the consent call site, which tests len>0 first.
		// Returning the kind-generic body rather than panicking keeps a future
		// caller honest instead of crashing an operator's consent screen.
		return composerCopyHashRule()
	case 1:
		return "The hash must be " + kinds[0].Token() + " of a 32-byte value." + tail
	}
	return "This wallet's hashes are " + composerKindList(kinds) +
		". Each must be of a 32-byte value." + tail
}

// composerCopyHashKindLead is SPEC_hashlock_kinds §7.1's kind screen.
//
// TWO SENTENCES, AND THE LENGTH IS A HARD CONSTRAINT RATHER THAN A STYLE
// PREFERENCE. composerPickScreen draws the lead as the first body row and pages
// the rest, so a long lead pushes rows off the FIRST PAGE -- silently, with no
// gate of its own. The first draft of this body ran three sentences and
// measured (emulator frame, sh2DisplaySize) at THREE kind rows visible:
// `hash160` was reachable only by pressing Button2, on a four-row screen where
// nothing suggests a second page exists.
// TestComposerHashKindScreenDrawsAllFourRows is the gate that keeps it honest.
//
// WHAT IT SAYS, AND WHY THOSE TWO THINGS. The preimage is 32 bytes under all
// four kinds, and an operator who reads a `40 hex` row as "type a 40-character
// secret" has misread the one thing that would make their wallet unspendable.
// And the default is named, because this screen HAS one -- row 0 is taken by an
// operator who presses straight through, so saying so is honest rather than
// leading.
//
// The per-kind widths are NOT here: they are on the rows, where they belong,
// stated in the hex characters the next screen will actually demand.
func composerCopyHashKindLead() string {
	return "All four take a 32-byte preimage. sha256 is the usual choice."
}

// composerKindList joins kind tokens the way composerSlotList joins slots.
func composerKindList(kinds []md.HashKind) string {
	out := ""
	for i, k := range kinds {
		switch {
		case i == 0:
			out = k.Token()
		case i == len(kinds)-1:
			out += " and " + k.Token()
		default:
			out += ", " + k.Token()
		}
	}
	return out
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

// ─── §8m: the six structural refusals (§4e) ──────────────────────────────────

// composerCopyRefuseTwoKeylessPaths is §8m's sixth line (fable review r0 C-1).
//
// IT NAMES THE TIMELOCK BECAUSE THE TIMELOCK IS THE OPERATOR'S NEXT IDEA. A
// key-less path is admitted one at a time, so the shape an operator reaches
// this refusal with is a second bearer path they have already confirmed once
// under §8a -- and the obvious repair, "put a delay on one of them", does not
// work: malleability asks for a SIGNATURE on one arm, and `older` is not one.
// A refusal that left that unsaid would send them round the loop.
func composerCopyRefuseTwoKeylessPaths() string {
	return "A wallet can have one key-less path, not two. Two of them make this " +
		"script malleable, and no wallet will import it. A time lock does not " +
		"help. Give one of them a key, or fold them into one path."
}

func composerCopyRefuseNoKeyedPath() string {
	return "Every wallet needs at least one path with a key."
}

// composerCopyRefuseEmptyPath is §8m's body for a path that has NOTHING --
// no key, no hash, no lock (fable review r0 lens 4 M-5).
//
// It is a separate body because the lock-only one was being drawn on it and
// was FALSE there: "A path with only a time lock means anyone can spend after
// it" names a lock the operator never set, on a row that reads "Path 2:
// empty". A refusal that says the wrong true thing is worse than one that
// says an unpolished true thing (the rule composerRefusalBody's own header
// states), and this is that rule applied to a second state inside one codec
// sentinel.
//
// THE PATH IS REFUSED, NOT REMOVED, and that is the choice this finding
// offered. composerAddPath removes a path that ends empty AT CREATION,
// because there the operator never had one. An EDIT reaches this state from a
// path that already exists in a list the operator is reading, and may carry a
// timelock they set; deleting it silently would renumber the list under them
// and discard that work, which is the very class -- a silent default at a
// moment that deserved a screen -- that the rest of this review is about.
func composerCopyRefuseEmptyPath() string {
	return "This path has no key and no hash, so nothing can spend it. Add a key " +
		"or a hash, or remove the path."
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

// composerCopyDuplicateKeys is the §8s warning for a policy that reuses a key
// slot, in the words of the harm that reuse actually carries.
//
// THREE HARMS, THREE SENTENCES since F-533. Two of them are Bitcoin Core's
// verdicts and are split because Core treats those shapes differently, and an
// operator told the wrong one looks in the wrong place. The third is not
// Core's at all -- a slot at the taproot key path AND in the script, which
// Core imports and BIP 388 forbids -- and it gets its own branch below rather
// than a reworded fallthrough, for the reason stated there.
// Measured on Core 25.0.0 via getdescriptorinfo, same key twice:
//
//	wsh(sortedmulti(2,A,A,B))  ACCEPTED  -- Core imports it happily
//	wsh(and_v(v:pk(A),pk(A)))  "is not sane: contains duplicate public keys"
//
// Core parses a top-level multi/sortedmulti as a MultisigDescriptor and never
// runs miniscript's sanity check on it. An earlier draft of this copy told the
// operator Core refuses the descriptor in BOTH cases; for the multisig one that
// is simply false, and a false sentence on the screen that consents to steel is
// worse than no sentence.
//
// The multisig case is the MORE dangerous of the two, which is why it keeps a
// warning rather than losing one: a slot filling m >= 2 seats drops the
// distinct keys needed from k to max(1, k-m+1). (An earlier draft of this
// comment said such a key "can meet the threshold alone", which is false for
// every k > m and was retracted at review I-5. The sentence the function
// returns does not make that claim.)
//
// NO LONGER ONLY A WARNING: the screens that carry it now also DECLINE to
// derive an address (F-531). This paragraph used to say the opposite -- "the
// device derives a correct, fundable address either way" -- and for the
// top-level multisig shape that was not even true: the flat route derived the
// address of a DIFFERENT script, one seat short. The refusal follows the
// operator's standing ruling of 2026-08-30 on BIP-388-forbidden wallets.
//
// THE CARD IS STILL NOT STRANDED, which was F-514's reason for preferring a
// warning. It decodes, displays, verifies and warns; what it no longer does is
// offer an address to fund. See composerCopyNoAddressesDuplicateKeys, the
// sentence that says so.
func composerCopyDuplicateKeys(slot uint8, kind md.DuplicateKind) string {
	// LEADING WITH THE ACTION, because the Inspect screen PAGES and this
	// sentence was longer than one page: at width 20 it measured 261px against
	// a 224px viewport, so "Check before you fund it." fell below the fold
	// (review I-8). Raising the wrap width is not available -- md1PolicyFlow
	// re-chunks anything over 20 bytes and the mid-word cuts come back -- so
	// the sentence is shorter and the instruction comes first. What an operator
	// reads on page one is now the thing to do.
	if kind == md.DuplicateFewerKeys {
		return fmt.Sprintf("Check before funding: slot @%d fills more than one seat, "+
			"so fewer separate keys can spend this than its k-of-n says.", slot)
	}
	// F-533, AND IT NEEDS ITS OWN BRANCH RATHER THAN THE FALLTHROUGH BELOW.
	// This function was an `if` plus an unconditional return, so a new kind
	// silently inherited "Bitcoin Core refuses such a descriptor" -- a sentence
	// this repo MEASURED FALSE for exactly the two cards that reach here. Core
	// 31.1 imports both (md/duplicate_keys.go's table): the internal key sits
	// outside the miniscript, so nothing repeats within one expression. BIP 388
	// is the rule that forbids it, so BIP 388 is what the sentence names.
	//
	// "KEY PATH" IS THE WORD THIS FIRMWARE ALREADY USES for the taproot
	// internal key -- composerCopySeatKeyPathPrompt says "slot @N, key path
	// (spends alone)" on the screen where the operator seated it. "Internal
	// key" is BIP 341's word, not one this device has ever shown them.
	//
	// MEASURED, not counted: 94 characters, SIX lines word-wrapped at the
	// Inspect screen's width of 20, against a page that holds SEVEN. (The same
	// measurement reproduces the two numbers recorded below: the 96-character
	// Core sentence is 6 lines and the 122-character version it replaced was
	// 8.) Through showError -- the modal this body actually reaches since
	// F-531, via duplicateRefusalBody -- it draws in full with 418 characters
	// of headroom alongside composerCopyNoAddressesDuplicateKeys, and
	// modal_fits_test.go holds that.
	if kind == md.DuplicateTaprootInternalKey {
		return fmt.Sprintf("Check before funding: slot @%d fills both the key path "+
			"and a script key, which BIP 388 forbids.", slot)
	}
	// 96 chars, 6 lines, so it FITS page one of the Inspect screen, which holds
	// 7. The shipped version was 122 chars and 8 lines, and page one ended on a
	// dangling ("duplicate public with keys"). overleaf (review M-9). Line count
	// decides, not length: 122 chars landed on 8 only because of where the words
	// break, so measure a replacement rather than counting it.
	return fmt.Sprintf("Check before funding: slot @%d repeats in one script, and "+
		"Bitcoin Core refuses such a descriptor.", slot)
}

// composerCopyDescriptorRepeatsAKey is the F-530 sibling of
// composerCopyDuplicateKeys, for a descriptor that arrived WITHOUT an md1.
//
// IT NAMES NO SLOT, because there is none to name. The md1 sentence says
// "slot @N", which is the operator's own label on a card they hold; a scanned
// descriptor has key expressions at positions and no @N anywhere on any screen.
// Quoting an index they have never seen would be the M-1 defect (a warning
// citing a referent the screen does not show), so this names the FACT instead.
//
// THE HARM IS THE FEWER-KEYS ONE, always. bip380.Parse admits single keys and
// top-level sortedmulti and nothing else, and Bitcoin Core parses a top-level
// sortedmulti as a MultisigDescriptor that never reaches miniscript's IsSane --
// so Core imports this shape rather than refusing it, and the refused-by-Core
// sentence would be false here for every descriptor that can reach this screen.
func composerCopyDescriptorRepeatsAKey() string {
	return "Check before funding: one key fills more than one seat of this " +
		"wallet, so fewer separate keys can spend it than its k-of-n says."
}

// composerCopyNoAddressesDuplicateKeys is the sentence that turns the F-531
// refusal into something an operator can act on.
//
// IT EXISTS BECAUSE A SILENT REFUSAL IS A WORSE SCREEN THAN A WRONG ADDRESS IS
// A SCREEN. "This device can't derive addresses for this policy" is true of the
// shape and useless about it: it reads as a device limitation, and the operator
// goes looking for a better tool instead of learning that their wallet reuses a
// key. This names the cause and leaves the card readable.
//
// It is a SECOND line, under composerCopyDuplicateKeys rather than folded into
// it, because that sentence is also shown where addresses ARE derived and must
// not start claiming a refusal that did not happen.
func composerCopyNoAddressesDuplicateKeys() string {
	return "No addresses: this device does not derive them for a wallet that " +
		"reuses a key."
}

// composerCopyOriginsChanged is the §8s line for the case the id line cannot
// state: the id is UNCHANGED and the per-slot origins this screen told the
// operator to mint against are not.
//
// It needs its own sentence because the two failures happen at different
// layers, and an operator told the wrong one looks in the wrong place. A card
// minted against a stale origin passes the stub check -- the stub is the top
// four bytes of an id that did not move -- and is refused by slotMatchesCard.
// Saying "this id changed" there would be false, and saying nothing at all was
// the defect review I-2 constructed.
func composerCopyOriginsChanged() string {
	return "Same id, but the slot origins below changed. Cards minted for the " +
		"old origins will not seat here."
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

// composerCopySameXpub is §7d's same-key refusal.
//
// IT LIVES HERE, and that is review r0 M-4's fix rather than a move for
// tidiness: §11 requires every refusal's copy to be a §8 blockquote or a quoted
// string in this table, and this body was an fmt.Sprintf at its own showError.
// composer_copy_test.go's AST scan only counts composerCopy* declarations, so
// nothing counted it and none of §12 item 5's four gates reached it.
func composerCopySameXpub(a, b uint8) string {
	return fmt.Sprintf("Slots @%d and @%d hold the same key. Every slot needs a "+
		"different key.", a, b)
}

// ─── H2: hashlock phrase route (SPEC_hashlock_H2_device §4) ──────────────────

func composerCopyHashlockNoPayloadLead() string {
	return "No hash record in the payload. Type a phrase below, or make one with " +
		"ms hashlock on the host."
}

// The first sentence answers the §8i rule modal the operator has just dismissed
// ("A passphrase must be hashed to 32 bytes first, then hashed again") -- that
// modal fires on the phrase row too, immediately in front of the one route that
// does the hashing itself, and read cold it says this route cannot work
// (r0 journey I-5). Stating it here costs no new gate row and no new screen.
// IT NAMES THE KIND (F-571). Three screens used to sit between §7.1's kind
// screen and the first screen that names the kind -- the phrase screen, the
// method pick and Deriving -- and the only way to check which kind was tapped
// was Back, which dropped every character typed.
//
// The pad, the sibling arm, has named its kind since the journey walk found the
// same gap there: "an off-by-one tap on the kind screen lands on a same-width
// sibling and nothing here would differ." On this arm the reasoning is stronger,
// because there is no width to read at all -- a phrase looks identical under all
// four kinds.
//
// IT GOES IN THE TITLE, NOT HERE, and geometry settled that. This lead has a
// TWO-LINE budget (§3.2(c)) and the masked readout takes the space below it:
// adding the kind as a sentence measured 3 lines of 23 px and drove the readout
// budget to -2 px, so the asterisks stopped being drawn at all. Measured, not
// predicted -- TestHashlockPhraseLeadIsDrawnInsideTheBand and the readout-budget
// test both went red.
//
// So the phrase screen names its kind in the title, as `<kind> phrase`, exactly
// as the pad draws `<kind> hash`. The lead is unchanged.
func composerCopyHashlockPhraseLead() string {
	return "This screen does that hashing for you. Use a phrase you have never " +
		"used anywhere else."
}

func composerCopyHashlockRefusal(err error) string {
	switch err {
	case hashlock.ErrEmpty:
		return "Type a hashlock phrase, or press Back."
	case hashlock.ErrNotPrintableASCII:
		return "A hashlock phrase is printable ASCII only."
	case hashlock.ErrMS1Shaped:
		return "That is a preimage plate, not a phrase. On the host, run ms hashlock " +
			"with it and load the hash: record it prints."
	case hashlock.ErrTooLong:
		return "A hashlock phrase is at most 100 characters."
	}
	return err.Error()
}

// composerCopyPhraseLooksLikeDigest is F-539's warning, and it is a CONFIRM
// rather than a refusal (operator ruling 2026-09-16): "we should warn user
// whenever the hashlock phrase looks like a digest and force user to confirm
// but we should not always refuse."
//
// It replaced a refusal that said "That is a preimage in hex, not a phrase" --
// a claim about the operator's INTENT, which the device cannot know. This says
// what will happen instead, and leaves the judgement where it belongs.
//
// `chars` is named because it is the tell: an operator who meant a phrase
// rarely typed exactly 40 or 64 hex characters by accident, and one who meant a
// digest recognises the number immediately.
func composerCopyPhraseLooksLikeDigest(chars int) string {
	return fmt.Sprintf("THIS LOOKS LIKE A DIGEST\n"+
		"You typed %d hex characters, the width of a digest. This device will "+
		"hash those characters, so the wallet commits to the TEXT you typed and "+
		"not to the digest it spells. If you meant a digest you already hold, go "+
		"back and use the %s row. Continue?", chars, composerHashRowHex)
}

func composerCopyHashlockHardenedWarning() string {
	return "Even a 20-character phrase falls in about 72 days on one GPU, and " +
		"shorter ones fall sooner. Choose it from a generator. If you have used " +
		"this phrase anywhere else, press Back and choose another. Continue?"
}

func composerCopyHashlockSHA256Warning() string {
	return "This is the brainwallet construction: anyone holding the digest tests " +
		"10^10 phrases per second. A phrase a person chose is not safe here; use " +
		"six diceware words. If you have used this phrase anywhere else, press " +
		"Back and choose another. Continue?"
}

func composerCopyHashlockDerivingLead() string {
	return "Deriving. This takes about 10 seconds."
}

// composerCopyHashlockConfirm is the §4.5 body. relation is "" when the payload
// holds no hash: record; otherwise the matches/no-match line. otherPath is ""
// unless another path of this policy already carries a different hash.
//
// "THE PHRASE AND METHOD ARE NOT ON THIS DEVICE" WAS FALSIFIED BY THIS STAGE
// and is rewritten here (H6 R0 round 0, journey I-2). §2.2 stores both in
// hashlockHeld for the composition's lifetime and §6 engraves both onto a
// plate: in hashlockPhraseRoute the falsification is ONE STATEMENT WIDE, since
// the next production statement after this modal is accepted is
// composerHoldHashlockMaterial. It is the same sentence, in the same direction
// of error, that §10.1's held arms were written to fix -- "saying a backup does
// not exist when it is about to be cut is the direction that costs the operator
// a plate" -- but those arms are guarded by composerEveryPathHashed, which is
// false the moment one path is keyed, i.e. on the ordinary mixed hashlock
// wallet. So the stage fixed the sentence on the banner drawn NOWHERE and left
// it false on the modal drawn on EVERY phrase route. The write-down instruction
// stays: it is what the operator should do whether or not a plate is cut.
//
// THE REPLACEMENT IS ONE CHARACTER SHORTER THAN WHAT IT REPLACES, and that is a
// measurement rather than taste. This body has 107 characters of headroom
// against modalBodyMargin = 80, so there are 27 characters of room, not the
// "well inside the margin" the finding assumed: the suggested wording ("...and
// can cut a plate for them at Done") measures 364 drawn and headroom 64, which
// TestConfirmScreensThisBlockTouchesAreDrawnInFull REFUSES. Where the plate is
// offered is §5.3's own screen; what this modal owes the operator is a true
// statement about where the material lives.
//
// THE HEADROOM NUMBER, CORRECTED (H5 §6 records; tests M-1 = journey N-1). The
// comment on composerCopyHashlockReconcile used to claim this body's measured
// headroom was 186; it is 107, and it was 107 before H5 touched it. The number
// that is true is logged by TestConfirmScreensThisBlockTouchesAreDrawnInFull on
// every run, which is why no literal is asserted here -- headroom is a LINE
// budget, not a character budget (modal_fits_test.go), so H5 §1's longer
// write-down sentence adds no line and does not move it.
func composerCopyHashlockConfirm(first8last8, method string, chars int, relation, otherPath string, kind md.HashKind) string {
	// The kind sits with the digest, because §6's binding rule is that where
	// both axes could be read, BOTH are named or neither is -- and the line
	// below names the method.
	b := "hash  " + kind.Token() + " " + first8last8 + "\n" +
		fmt.Sprintf("method: %s   chars: %d", method, chars) + "\n"
	if relation != "" {
		b += relation + "\n"
	}
	if otherPath != "" {
		b += otherPath + "\n"
	}
	return b +
		// SHORTENED, not margin-lowered. Adding the kind here and on the hash
		// row took the longest variant to 64 characters of headroom under an
		// 80-character margin, and modal_fits' own message says why that is
		// not good enough: F-185's fix failed exactly here, because a
		// +65-character edit still FIT and so could re-break the screen
		// without turning a test red.
		//
		// "Without BOTH" was also wrong the moment this listed three things.
		"Write down the phrase, method, hash kind and digest now. " +
		"This composition holds them until it ends. " +
		"Without them, this path can never be spent.\n" +
		"One phrase per policy. Never use it as a passphrase or password anywhere else."
}

// composerCopyHashlockPreimageConfirm is §5.1's confirm body for a PREIMAGE
// RECORD the payload delivered.
//
// A SEPARATE BODY FROM composerCopyHashlockConfirm, and the reason is what the
// operator holds. That body's fields are `method` and `chars`, and its
// instruction is to write down the phrase and the method -- a preimage record
// has none of the three. Reusing it would draw `method: hardened   chars: 0` on
// the screen that gates funds, which is a measurement of nothing wearing the
// clothes of one.
//
// WHAT REPLACES THE WRITE-DOWN LINE is the thing that IS true here: the
// preimage is in the payload, in flash, and a plate is the way it leaves.
func composerCopyHashlockPreimageConfirm(first8last8, relation, otherPath string, kind md.HashKind) string {
	// §6's "both axes or neither" is satisfied here either way -- this screen
	// prints no `method:` line, so there is no collision to resolve. The kind
	// is named anyway for a simpler reason: the three sibling hashlock screens
	// now read `hash  <kind> <digest>`, and four screens showing a digest in
	// two different shapes is its own hazard when the operator is comparing
	// one against a card.
	//
	// Route 5 (§7.1): a payload preimage-plate record carries no kind in its
	// grammar, so this is sha256 in practice -- but it is passed rather than
	// assumed here, so the screen cannot drift from the lock it describes.
	b := "hash  " + kind.Token() + " " + first8last8 + "\n" +
		"from a preimage record in this payload\n"
	if relation != "" {
		b += relation + "\n"
	}
	if otherPath != "" {
		b += otherPath + "\n"
	}
	return b +
		"Spending this path needs that preimage. It is in the payload and not on " +
		"these plates. Cut a preimage plate for it at Done, or keep the payload."
}

func composerCopyHashlockRelation(i int) string {
	if i < 0 {
		return "no hash: record in the payload has this digest"
	}
	return fmt.Sprintf("matches hash %d in the payload", i+1)
}

// §4.5's reconciliation screen, drawn right after HOLD for every phrase-set
// hash.
//
// §4.5's drop-order step 2 says to move this line into the phrase-route §8h at
// Done, and the build gate did -- but §8h is guarded by composerEveryPathHashed
// (composer_state.go at the fork baseline c4a64fc), so on the ordinary
// wallet with one keyed path and one
// hashlocked path it was drawn NOWHERE (r0 adversarial I-1 = fidelity I-2 =
// journey I-3, all three tracing the same loss). Its own screen after HOLD is
// reachable for every policy that has a phrase-set hash.
//
// IT CARRIES THE OPERAND IT ASKS ABOUT (H5 §1, F-487). "Check the digest
// matches" was asked one frame AFTER the confirm modal took the digest off the
// panel, so the operator was told to compare against something no longer on
// screen. The token, the method and the character count come back here, spelled
// exactly as the confirm modal spells them
// (TestHashlockReconcileHeaderIsSpelledLikeTheConfirmModal), and `chars: <n>` is
// H2 §4.5's reconciliation field arriving at the moment of reconciliation --
// it is the one signal that shows a stray space against the host card's
// phrase_chars.
//
// AND IT SAYS WHAT A MISMATCH MEANS. A divergence found here is a path that
// could never have been spent; the remedy is to build the policy again, before
// it is funded, and not to fund it and hope.
//
// "BEFORE YOU CUT PLATES", NOT "BEFORE YOU FUND" (r0 journey M-2). This screen
// is drawn inside composerShapeFlow, and the stub screen, seating and engraving
// all follow it in the same composerFlow -- roughly 21 minutes per plate. The
// digest is IN the engraved md1, so a divergence found after the plates are cut
// costs every plate. Funding is the funds-safety deadline and the mismatch
// sentence keeps it; the operator standing here is at the cheapest moment to
// act, and the first sentence now names that one instead of a later one.
//
// Measured on errorScreenBody at sh2DisplaySize, longest variant (`hardened`,
// `chars: 100`): see the row in TestModalsThisBlockTouchesAreDrawnInFull.
func composerCopyHashlockReconcile(first8last8, method string, chars int, kind md.HashKind) string {
	// §13.2, THE CYCLE'S OPERATOR-FACING CRITICAL. This screen tells the
	// operator to check a digest and discard the wallet if it differs -- and it
	// used to supply only `method:`, the OTHER axis (§5). `ms hashlock` with no
	// kind returns the sha256 digest, both values are 64 hex, and nothing but a
	// label distinguished them. On a correct hash256 wallet the check FAILED
	// and an operator complying exactly discarded a good wallet and re-cut five
	// plates.
	//
	// BOTH FLAGS ARE NAMED, and the second one is why this was re-folded.
	// The first fix named `--kind` and left "and method" as prose. Review then
	// measured the host: `--kind` is optional and omitting it LISTS EVERY
	// KIND'S DIGEST -- it fails SAFE. `--method` is optional and omitting it
	// runs `unwrap_or(Method::Hardened)` (ms-cli/src/cmd/hashlock.rs:220) --
	// it fails SILENTLY WRONG. So the screen was naming the flag that cannot
	// hurt you and inferring the one that can: on a `method: sha256` wallet an
	// operator who typed the command as written got a mismatch, and this same
	// screen then told them not to fund a CORRECT wallet and to build it
	// again -- five plates at ~21 minutes each.
	//
	// That is §13.2's own mechanism, one axis over, inside the fold that fixed
	// §13.2. The rule it stated -- "what the operator types is what decides
	// which digest comes back" -- applies to every axis the command takes, not
	// to the one the cycle happened to be about.
	//
	// The tokens are the host's own: hashlockMethod.String() emits
	// "sha256"/"hardened" and md.HashKind.Token() the four kind names, which
	// are exactly clap's value_enum spellings. The printed command is
	// copyable as it stands.
	return "hash  " + kind.Token() + " " + first8last8 + "\n" +
		fmt.Sprintf("method: %s   chars: %d", method, chars) + "\n" +
		"Before you cut plates, run ms hashlock --kind " + kind.Token() +
		" --method " + method +
		" with this phrase on the host and check the digest matches. " +
		"If they differ, do not fund this wallet: build it again."
}

// composerCopyHashlockOtherPath is the confirm modal's second relation line
// (r0 journey I-1): another path of this policy already carries a DIFFERENT
// hash, so spending will need more than this one phrase. COUNT-FREE on purpose
// (post-impl e2e I-1): "two phrases" was a hard-coded number, wrong on any
// wallet with three or more hashlocks -- an undercount at the moment the
// operator is counting what to back up.
func composerCopyHashlockOtherPath() string {
	return "another path has a different hash: back up every phrase"
}

// §8h, the phrase-route form (SPEC_hashlock_H2_device §4.7 as H5 §2 folds it).
// The reconciliation line lives in composerCopyHashlockReconcile instead; see
// there.
//
// "EVERY ... AND EVERY", NOT "THE ... OR THE" (H5 §2 item 5, journey I-3). This
// banner is drawn when EVERY path is hashed and at least one of those hashes
// came from a phrase -- which on a mixed wallet means one path needs the phrase
// and another needs a preimage plate, so BOTH backups are required, one per
// path. The shipped sentence offered a choice between them, and a choice is an
// undercount at the one screen whose job is to say what spending needs.
//
// IT OVERCOUNTS ON THE TWO PURE WALLETS, DELIBERATELY (r0 journey M-4). An
// all-phrase wallet has no preimage PLATE and a phrase re-typed as 64 hex has
// none either, and both are named one anyway. Counting exactly would need three
// variants of this body; overcounting asks the operator to look for a backup
// they do not have, and undercounting lets them stop looking for one they do.
// The safe direction is the one that keeps looking, so this stays as written --
// recorded here so the next reader does not re-open it.
func composerCopyHashEveryPathPhrase() string {
	return "HASH ON EVERY PATH\n" +
		"Every way to spend this wallet needs a hashlock preimage. It is not on " +
		"this device and not on these plates. Back up every phrase and its " +
		"method, and every preimage plate, separately."
}

// composerCopyHashEveryPathFor chooses among §8h's FOUR arms (H6 §10.1).
//
// THE HELD ARMS COME FIRST because they are the true statement when they apply:
// the shipped two say the preimage "is not on this device", and H6 §2.2 makes
// that false for a composition that holds it. Saying a backup does not exist
// when it is about to be cut is the direction that costs the operator a plate.
func composerCopyHashEveryPathFor(st *composerState) string {
	if composerEveryHashedPathHeld(st) {
		if composerEveryHeldPathHasAPhrase(st) {
			return composerCopyHashEveryPathHeldPhrase()
		}
		return composerCopyHashEveryPathHeld()
	}
	if composerAnyPathByPhrase(st) {
		return composerCopyHashEveryPathPhrase()
	}
	return composerCopyHashEveryPath()
}

// ─── H6 §8.3, §8.4, §8.5, §10.1: the preimage plates' copy ───────────────────

// composerCopyPreimagePlateLead is §5.3 item 7's masked pick lead: the digest,
// the path, and the phrase's LENGTH and method -- no character of the phrase.
//
// `phrase: <n> characters` IS THE WHOLE AFFORDANCE. It is what a person
// comparing this screen against a host card can check without the secret ever
// reaching the panel, and it is the one signal that shows a stray space.
// TWO LINES, AND THE NUMBER IS MEASURED. composerPickScreen draws the lead as a
// per-page header through composerPageLines, so every line the lead spends is a
// ROW the operator loses: at sh2DisplaySize a four-line lead leaves 3 of this
// screen's 4 rows on the first page, and `do not cut this preimage` is the row
// an operator reaches for to UNDO. Two lines leave all four
// (TestComposerPreimagePlatePickDrawsAllFourRows).
func composerCopyPreimagePlateLead(first8last8 string, path, chars int, method string, kind md.HashKind) string {
	// THE THIRD SCREEN. §13.2: "the fold that wrote that rule missed the screen
	// it condemned" -- this one prints `method:` and no kind, which §5's rule
	// condemns as directly as the other two.
	head := "hash  " + kind.Token() + " " + first8last8
	if path > 0 {
		head += fmt.Sprintf("   path %d", path)
	}
	if chars == 0 {
		return head + "\npreimage held, phrase not: only the string form can be cut"
	}
	return head + fmt.Sprintf("\nphrase: %d characters   method: %s", chars, method)
}

// composerCopyPreimageQRWarning is §8.5, confirm-to-proceed on the model of
// ftWarnQR (gui/freetext_flow.go:1214-1216), which already warns for strictly
// less dangerous content.
func composerCopyPreimageQRWarning() string {
	return "The QR makes the phrase readable by any camera. A photograph of the " +
		"plate is a copy of the phrase, and the phrase spends this path."
}

// composerCopyPreimagePlateHeading is §8.3's heading.
//
// "plate(s)" IS THE SPEC'S OWN SPELLING and is kept verbatim, against this
// file's house style (composerSlotWord renders "slot @3" or "slots @3 and @4"
// so a refusal never reads "slots @3"). Changing spec copy inside a build gate
// would put the shipped string and the document that is diffed against it out
// of step; it is filed instead.
func composerCopyPreimagePlateHeading(n int) string {
	return fmt.Sprintf("Plus %d preimage plate(s), cut first and NOT part of this backup:", n)
}

// composerCopyPreimageCensusScope is F-497's scope line: what this census is a
// list OF.
//
// The census reports what THIS composition will cut. The device keeps no record
// of past runs — a plate cut for the same digest last week is invisible to it —
// so a reader who takes this block for an inventory of what exists on steel is
// reading it for more than it can say. The operator ruled 2026-09-06 that the
// limitation is accepted and the copy must own it, rather than the device
// growing durable state and a migration to remove it.
//
// IT SITS WITH THE ROWS AND NOWHERE ELSE. The stand-alone notice form lists no
// plates to cut at all, so a caveat about the completeness of a list would be a
// caveat about nothing.
func composerCopyPreimageCensusScope() string {
	return "This is what this composition will cut. Plates cut in earlier runs are " +
		"not known to this device and are not listed."
}

// composerCopyPreimagePlateRow is §8.3's per-plate row.
// THE KIND IS NAMED HERE BECAUSE THE METHOD ALREADY IS (§13.3, §6's
// both-or-neither rule). `form` ends in the preimage METHOD -- "phrase,
// sha256, QR" -- so on a hash160 wallet this row printed the bare token
// `sha256` and meant the other axis by it. The census is the operator's only
// inventory of what they must store apart, and it described all four
// constructions identically: two plates for one preimage under two kinds
// differ by sixteen hex characters and nothing else.
func composerCopyPreimagePlateRow(path int, kind md.HashKind, first8last8, form string) string {
	return fmt.Sprintf("path %d  %s %s  %s", path, kind.Token(), first8last8, form)
}

// composerCopyTwentyByteUnseen is SPEC_hashlock_kinds §8, which this cycle
// specified as normative and then implemented nowhere -- no code, no copy, no
// test -- until two review lenses found it missing independently.
//
// IT FIRES FOR ripemd160 AND hash160 ONLY WHEN THE DEVICE DID NOT DERIVE THE
// PREIMAGE (§2 decision 3): a payload-supplied or typed digest warns, the
// phrase route stays silent. Provenance is composerState.hashlockHeld, which
// only the deriving routes populate, so there is no new plumbing -- exactly as
// §8 says.
//
// WHAT IT SAYS AND WHY EACH CLAUSE IS TRUE. The first sentence is the funds
// fact and it holds for any supplied digest: nothing on this device has seen a
// preimage for it, so whoever did supply it is who can spend that path. The
// second is what makes the 20-byte kinds the ones that warn -- half the width
// is far less collision margin, and a digest chosen by someone else is exactly
// the case where that margin is load-bearing. It does NOT claim the wallet is
// unsafe: a ripemd160 hashlock whose preimage you hold is fine, and saying
// otherwise would teach operators to skip the warning that matters.
//
// §8's own stated residual, accepted there and repeated here so it is not
// rediscovered as a bug: hashlockHeld is per-digest-ever-seen rather than
// per-assignment, so a digest derived here earlier in the SAME composition and
// later arriving from a payload suppresses a warning it should show.
func composerCopyTwentyByteUnseen(kind md.HashKind) string {
	return "20-BYTE HASH, NOT DERIVED HERE\n" +
		"This is a " + kind.Token() + " digest and nothing on this device has " +
		"seen a preimage for it. Whoever supplied it can spend this path. A " +
		"20-byte hash also leaves far less collision margin than sha256. Check " +
		"you hold the preimage before you fund this wallet."
}

// composerCopyPreimageNotOnAnyPath is §8.3's row for a retained preimage no
// CURRENT path carries: LISTED, never cut, and nothing is deleted from
// hashlockHeld to achieve it (§2.2 item 2).
func composerCopyPreimageNotOnAnyPath(first8last8 string) string {
	return "preimage " + first8last8 + ": not on any path, will not be cut"
}

// composerCopyPreimageDeclined is §8.3's row for a plate the operator declined.
func composerCopyPreimageDeclined(first8last8 string) string {
	return "preimage " + first8last8 + ": declined, will not be cut"
}

func composerCopyPreimageKeepApart() string {
	return "Keep each preimage plate apart from the policy plates and from the others."
}

// composerCopyPreimageOnlyNotice is §8.3's stand-alone notice form, for a review
// whose ONLY preimage entry is one no path carries. The terse row says what
// happened; on a review with nothing else to read there is room to say what to
// do about it.
func composerCopyPreimageOnlyNotice() string {
	return "One preimage this composition holds is on no path of this policy. It " +
		"will not be cut. Go back and set a path's hash to it, or leave it."
}

// composerCopyAbortNoPreimage is §8.4a.
func composerCopyAbortNoPreimage() string {
	return "NO PREIMAGE PLATE WAS CUT. The phrase dies with this composition. " +
		"Do not fund this wallet."
}

// composerCopyAbortPreimageCut is §8.4b.
func composerCopyAbortPreimageCut() string {
	return "A PREIMAGE PLATE WAS CUT and no policy plate was. Store or destroy it " +
		"now; do not leave it with the blanks."
}

// composerCopyHashEveryPathHeld is §10.1's THIRD §8h arm: every hashed path's
// digest has material this composition holds, and none of it is a phrase.
func composerCopyHashEveryPathHeld() string {
	return "HASH ON EVERY PATH\n" +
		"Every way to spend this wallet needs the preimage of a hash. This " +
		"composition holds the preimage for each one and can cut a plate for it " +
		"at Done. Store those plates apart from these, and apart from each other."
}

// composerCopyHashEveryPathHeldPhrase is §10.1's FOURTH arm: the same, and the
// phrase and method are held too.
func composerCopyHashEveryPathHeldPhrase() string {
	return "HASH ON EVERY PATH\n" +
		"Every way to spend this wallet needs a hashlock preimage. This " +
		"composition holds the phrase and method for each one and can cut a plate " +
		"at Done. Store those plates apart from these, and apart from each other."
}

// composerCopyPreimagePlateRefusal is the refusal when a decided preimage plate
// cannot be built or does not fit the plate. It names the form so the operator
// can choose a smaller one rather than being told only that something failed.
func composerCopyPreimagePlateRefusal() string {
	return "Couldn't build that preimage plate. Go back and choose a smaller " +
		"form: the phrase without a QR, or the preimage string."
}

// ─── H6 §5.2: the Hashlock plates flow's copy ────────────────────────────────

// composerCopyHashlockPlatesLead is the list's lead. It says what the rows ARE,
// because a preimage plate is not a backup of this device's state -- it is
// bearer access to whatever path its digest locks.
func composerCopyHashlockPlatesLead(n int) string {
	if n == 1 {
		return "1 record can be cut as a preimage plate. Whoever holds that plate can spend its path."
	}
	return fmt.Sprintf("%d records can be cut as preimage plates. Whoever holds one can spend its path.", n)
}

// composerCopyHashlockPlatesEmpty is unreachable from the door, whose predicate
// asks the same question. It exists so the flow refuses rather than drawing an
// empty picker.
func composerCopyHashlockPlatesEmpty() string {
	return "This payload holds no preimage or phrase record to cut."
}

// composerCopyHashlockPlatesNotCut is this flow's abort, and it is NOT either
// §8.4 arm. Those say the phrase "dies with this composition" and speak about a
// run that also cuts policy plates; here the material stays in the payload, in
// flash, and saying a secret is gone when it is not is false in the dangerous
// direction.
func composerCopyHashlockPlatesNotCut() string {
	return "That plate was not cut. The record is still in this payload, so you can " +
		"cut it again from this list."
}

// composerCopyPreimagesLoaded is §5.2 step 2's door count. Without it the door
// draws composerCopyNoKeys above a route offered for exactly the records that
// lead says are not there.
func composerCopyPreimagesLoaded(n int) string {
	if n == 1 {
		return "1 preimage or phrase record loaded."
	}
	return fmt.Sprintf("%d preimage or phrase records loaded.", n)
}

// ─── H6 §9 and §8.8: two bodies about a string being mistaken for another ────
//
// THEY LIVE IN THIS FILE FOR THE GATE, not because they are composer copy.
// TestComposerCopyTableCoversEveryBody scans composer_copy.go's composerCopy*
// declarations and requires a row for each, and that row is what carries §12
// item 5's four gates -- the glyph check, the raster floor, the modal-fits
// measurement and a fires-on-condition test. A body declared beside its screen
// instead would ship with none of them and nothing would say so; that is the
// defect the same test's own comment records.

// composerCopyHashlockLooksLikeMS1 is §9's warning, shown by BOTH the free-text
// and the passphrase programs when what has been typed looks like an ms1 string.
//
// NEVER A REFUSAL. Both programs cut what the operator typed; this tells them
// what the string looks like and where the marked plate comes from, and lets
// them continue.
//
// THE PASSPHRASE PROGRAM SHOWS THE SAME BODY. The string is about to become a
// BIP-39 passphrase rather than a plate, but the sentence that matters -- what
// it looks like, and where a marked hashlock plate comes from -- is identical,
// and two near-identical bodies is how one of them goes stale.
func composerCopyHashlockLooksLikeMS1() string {
	return "This looks like an ms1 string. A seed plate comes from a payload; a " +
		"marked hashlock plate comes from the Wallet Policy program, from a phrase " +
		"typed there or a preimage packed on the host. Continue here to cut it as " +
		"plain text."
}

// composerCopyHashlockPhraseNotPassphrase is §8.8's notice at progPassword.
//
// WHAT IT REPLACES IS SILENCE, and silence is what routes the operator around
// the guard. The refusal at progPassword is correct and structural
// (syswOfferAlt returns before any screen is drawn when the payload holds no
// ClassPassphrase), so an operator who packs a hashlock phrase, taps, and opens
// the program whose NAME matches what they are holding gets the ordinary
// passphrase keyboard -- no offer, no mention, no reason. The obvious next move
// is the harmful one: re-pack the phrase as a `pass:` record so it "works",
// which is the substitution ruling L2 exists to prevent and whose stated stake
// is a different wallet.
func composerCopyHashlockPhraseNotPassphrase() string {
	return "This payload holds a HASHLOCK PHRASE, not a BIP-39 passphrase. They are " +
		"not interchangeable: using one as the other opens a different wallet. A " +
		"hashlock phrase is used in the Wallet Policy program."
}
