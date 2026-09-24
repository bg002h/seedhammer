package gui

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"seedhammer.com/hashlock"
	"seedhammer.com/md"
)

// composerCopyRow is one operator-facing body, its §8 section, and the exact
// text the spec prints for it. `verbatim` is compared WORD FOR WORD after
// whitespace normalisation, so a reviewer diffing this table against SPEC
// §8 is diffing the shipped strings.
type composerCopyRow struct {
	fn       string // the composerCopy* function this row covers
	section  string // the §8 subsection
	got      string // what the function returns for the spec's own example
	verbatim string // SPEC §8, transcribed
}

// composerCopyTable is the whole of §8 as the device draws it.
//
// EVERY ROW IS A CONTRACT WITH THREE OTHER TESTS: the raster floor and the
// modal-fits assertion run over `got` (composer_copy_gate_test.go), and a
// fires-on-condition test drives the flow into the state that shows it
// (named in the section's own task). This table is what makes the count
// exact.
func composerCopyTable() []composerCopyRow {
	return []composerCopyRow{
		// §8a GAINED ITS SECOND SENTENCE in the fable review r0 fold (lens 1
		// I-1 = lens 3 I-1); filed as a §8 amendment so this table stays the
		// diff target for the spec.
		{"composerCopyKeylessPath", "8a", composerCopyKeylessPath(),
			"KEY-LESS PATH (EXPERIMENTAL) This path needs no signature. Whoever knows the preimage of its hash can spend it. If that preimage is ever engraved, the plate is bearer access. It also makes the WHOLE wallet un-importable, keyed paths included. Bitcoin Core, Nunchuk and Liana all refuse it. Only md can rebuild this wallet, and md cannot sign: no other wallet will watch it or spend from it."},
		{"composerCopyUnsortedKeys", "8b", composerCopyUnsortedKeys(),
			"UNSORTED KEYS (EXPERIMENTAL) You chose unsorted keys where sorted was possible. Key order is part of this wallet. Anyone restoring it must keep the same order. Sorted keys need none."},
		{"composerCopyLockEchoDays", "8c", composerCopyLockEchoDays(90, 15188),
			"90 days = 15188 units of 512 s (90.0 days)"},
		{"composerCopyLockEchoBlocks", "8c", composerCopyLockEchoBlocks(1000),
			"1000 blocks (about 6.9 days)"},
		{"composerCopyLockEchoHeight", "8c", composerCopyLockEchoHeight(905000),
			"Block 905000"},
		{"composerCopyLockEchoDate", "8c", composerCopyLockEchoDate(2027, 3, 1),
			"2027-03-01 00:00 UTC"},
		{"composerCopyPackedDateBound", "8c", composerCopyPackedDateBound("2026-09-01"),
			"This device cannot tell the time. The payload says it was packed on 2026-09-01, which may be long ago. Nothing here has checked that this is in the future."},
		{"composerCopyPackedHeightBound", "8c", composerCopyPackedHeightBound(905000),
			"This device cannot tell the time. The payload says the packed height was 905000, which may be long ago. Nothing here has checked that this is in the future."},
		{"composerCopyNoBound", "8c", composerCopyNoBound(),
			"This device cannot tell the time. Nothing here has checked that this is in the future."},
		{"composerCopyOwnWallet", "8d", composerCopyOwnWallet(),
			"A wallet built here is its own wallet. The same rules written by another tool give a different id and different addresses."},
		// §8f REWRITTEN in the fable review r0 fold (lens 2 I-1): its Nunchuk
		// claim was measured FALSE, 0 of 7 shapes, by running libnunchuk
		// 2.1.1. Filed as a §8 amendment so this table stays the diff target.
		{"composerCopyNUMS", "8f", composerCopyNUMS(),
			"KEY PATH: NONE (NUMS) Spends use the script paths only. Bitcoin Core imports this form. Nunchuk cannot import a NUMS policy at all: for Nunchuk, use wsh, or a tr policy whose first path is a single key. Liana needs its own unspendable key instead, which this device offers where Liana can import the policy; that is a different wallet with different addresses."},
		// §8y is F-449 stage 4 (SPEC_liana_unspendable_internal_key §0b and
		// §7): the kind-1 key-path line, the choice screen's lead and two
		// rows, and the RESET signal. Filed as a §8 amendment so this table
		// stays the diff target.
		{"composerCopyLianaKeyPath", "8y", composerCopyLianaKeyPath(),
			"KEY PATH: NONE (LIANA KEY) Spends use the script paths only. The key path is Liana's unspendable key, computed from this wallet's own keys. Liana (as of v15.0) and Bitcoin Core import this form. Nunchuk imports it only when the keys happen to be in sorted order. The same paths with the NUMS key are a different wallet with different addresses."},
		{"composerCopyUnspendableLead", "8y", composerCopyUnspendableLead(),
			"Which key path? The two options below are DIFFERENT WALLETS, with different addresses. It cannot be changed after engraving."},
		{"composerCopyUnspendableRowNUMS", "8y", composerCopyUnspendableRowNUMS(),
			"NUMS point: Bitcoin Core imports it. Liana and Nunchuk do not."},
		{"composerCopyUnspendableRowLiana", "8y", composerCopyUnspendableRowLiana(),
			"Liana key: Liana (v15.0) and Bitcoin Core import it. Nunchuk only by chance."},
		{"composerCopyLianaUnmet", "8y", composerCopyLianaUnmet(),
			"The Liana key was chosen, but this policy has a real key path, so there is no unspendable key to choose. Go back to the key path screen."},
		{"composerCopyLianaKeyDropped", "8y", composerCopyLianaKeyDropped("Path 1 is one key with no lock, so it became the key path, and there is no unspendable key to choose."),
			"LIANA KEY DROPPED Path 1 is one key with no lock, so it became the key path, and there is no unspendable key to choose. This policy is back on the NUMS key path: its Template-ID and addresses are not the ones the Liana key gave."},
		{"composerCopyMixedLockBases", "8g", composerCopyMixedLockBases(),
			"MIXED LOCK BASES Some paths lock by block height and others by time. Nunchuk will refuse this wallet; Bitcoin Core imports it. Taproot accepts both, because it checks each path on its own."},
		// §8x is the fable review r0 lens-5 I-1/I-2 addition: one consent
		// notice naming the FIRST of Liana's nine refusal classes that
		// applies (composerLianaOutsideModelClass), so an operator sees
		// before steel what "Failed to read the descriptor" would otherwise
		// tell them only after. The example below is the demo payload's own
		// class -- a plain multisig has no recovery path at all.
		{"composerCopyOutsideLianaModel", "8x", composerCopyOutsideLianaModel("no locked path"),
			"OUTSIDE LIANA'S MODEL Liana (as of v15.0) takes one unlocked path, at least one path locked by older in blocks, and no hash. This policy: no locked path. Bitcoin Core imports it."},
		{"composerCopySameSeedThreshold", "8g", composerCopySameSeedThreshold([]uint8{1, 2}, 2, 3),
			"SAME SEED, SAME PATH Slots @1 and @2 are the same seed. This path's 2-of-3 can be satisfied by one person. Liana will refuse it."},
		{"composerCopySameSeedBelow", "8g", composerCopySameSeedBelow([]uint8{1, 2}, 3),
			"SAME SEED, SAME PATH Slots @1 and @2 are the same seed. One person holds 2 of the 3 signatures this path needs. Liana will refuse it."},
		{"composerCopyHashEveryPath", "8h", composerCopyHashEveryPath(),
			"HASH ON EVERY PATH Every way to spend this wallet needs the preimage of a hash. It is not on this device and not on these plates. Back up every preimage separately."},
		{"composerCopyHashRule", "8i", composerCopyHashRule(),
			"The preimage must be a 32-byte value. A passphrase must be hashed to 32 bytes first, then hashed again. A hash of the passphrase itself can never be spent."},
		// THE FOUR-KIND FORM, deliberately: this row feeds §12 item 5's glyph,
		// raster and modal-fits gates, which measure the string they are given,
		// and the longest body is the one that can overflow. A policy with four
		// paths carrying one kind each reaches it. The one- and two-kind
		// wordings are asserted in TestComposerConsentHashRuleNamesTheKinds.
		{"composerCopyHashKindLead", "7.1", composerCopyHashKindLead(),
			"All four take a 32-byte preimage. sha256 is the usual choice."},
		{"composerCopyHashRuleForKinds", "8i", composerCopyHashRuleForKinds(
			[]md.HashKind{md.KindSha256, md.KindHash256, md.KindRipemd160, md.KindHash160}),
			"This wallet's hashes are sha256, hash256, ripemd160 and hash160. Each must be of a 32-byte value. A passphrase must be hashed to 32 bytes first, then hashed again. A hash of the passphrase itself can never be spent."},
		{"composerCopyEditClearsKeys", "8j", composerCopyEditClearsKeys(),
			"EDITING THE SHAPE CLEARS THE KEYS Slot numbers change with the shape. Every key you seated will be cleared. Continue?"},
		{"composerCopyPersonInTwoPaths", "8k", composerCopyPersonInTwoPaths(),
			"One person in two paths needs two keys: a second account from the same seed, or a second card."},
		{"composerCopyNothingChecked", "8l", composerCopyNothingChecked(),
			"Nothing outside this device has checked this policy. Before you fund it, restore these plates in your coordinator and compare your own first receive address."},
		{"composerCopyRefuseNoKeyedPath", "8m", composerCopyRefuseNoKeyedPath(),
			"Every wallet needs at least one path with a key."},
		{"composerCopyRefuseLockOnly", "8m", composerCopyRefuseLockOnly(),
			"A path with only a time lock means anyone can spend after it. Add a key or a hash."},
		{"composerCopyRefuseEmptyPath", "8m", composerCopyRefuseEmptyPath(),
			"This path has no key and no hash, so nothing can spend it. Add a key or a hash, or remove the path."},
		{"composerCopyRefuseKeylessTr", "8m", composerCopyRefuseKeylessTr(),
			"This build will not put a key-less path in taproot. Use wsh, or add a key."},
		{"composerCopyRefuseTwoKeylessPaths", "8m", composerCopyRefuseTwoKeylessPaths(),
			"A wallet can have one key-less path, not two. Two of them make this script malleable, and no wallet will import it. A time lock does not help. Give one of them a key, or fold them into one path."},
		{"composerCopyRefuseLegacyShape", "8m", composerCopyRefuseLegacyShape(),
			"Legacy wrappers hold one plain multisig only. Use wsh or tr."},
		{"composerCopyRefuseSlotCap", "8m", composerCopyRefuseSlotCap(),
			"This wallet already has 32 key slots."},
		{"composerCopyBelowBoundDate", "8o", composerCopyBelowBoundDate(),
			"That is before this payload was packed. Choose a later date."},
		{"composerCopyBelowBoundHeight", "8o", composerCopyBelowBoundHeight(),
			"That is before this payload was packed. Choose a later height."},
		{"composerCopyShortfall", "8p", composerCopyShortfall(4, 3, []uint8{3}),
			"4 slots, 3 keys available. Unfilled: slot @3."},
		{"composerCopySelfCheckFailed", "8q", composerCopySelfCheckFailed(),
			"The policy on this device does not match what you built. Go back and check the path list, or start again."},
		{"composerCopyKeysLoaded", "8r", composerCopyKeysLoaded(4),
			"Keys loaded: 4"},
		{"composerCopyKeysAndSeeds", "8r", composerCopyKeysAndSeeds(4, 1),
			"Keys loaded: 4, plus 1 seed."},
		{"composerCopySeedOnly", "8r", composerCopySeedOnly(),
			"A seed is loaded. It can fill any number of slots."},
		{"composerCopyNotUnderstood", "8r", composerCopyNotUnderstood(3),
			"3 payload records were not understood."},
		{"composerCopyNoKeys", "8r", composerCopyNoKeys(),
			"No keys loaded. This builds a key-less template."},
		{"composerCopyPayloadNotLoaded", "8r", composerCopyPayloadNotLoaded(),
			"A payload is in flash but not loaded. Load it from the carousel first."},
		{"composerCopyIdChanged", "8s", composerCopyIdChanged(),
			"The shape changed, so this id changed. Cards minted with the old stub will not seat here."},
		{"composerCopyDuplicateKeys", "8s", composerCopyDuplicateKeys(1, md.DuplicateRefusedByCore),
			"Check before funding: slot @1 repeats in one script, and Bitcoin Core refuses such a descriptor."},
		{"composerCopyDuplicateKeys", "8s", composerCopyDuplicateKeys(2, md.DuplicateFewerKeys),
			"Check before funding: slot @2 fills more than one seat, so fewer separate keys can spend this than its k-of-n says."},
		// F-533's kind. It is here because this table enumerates kinds BY HAND
		// and would otherwise stay green while a third kind shipped unmeasured.
		{"composerCopyDuplicateKeys", "8s", composerCopyDuplicateKeys(3, md.DuplicateTaprootInternalKey),
			"Check before funding: slot @3 fills both the key path and a script key, which BIP 388 forbids."},
		{"composerCopyDescriptorRepeatsAKey", "8s", composerCopyDescriptorRepeatsAKey(),
			"Check before funding: one key fills more than one seat of this wallet, so fewer separate keys can spend it than its k-of-n says."},
		{"composerCopyNoAddressesDuplicateKeys", "8s", composerCopyNoAddressesDuplicateKeys(),
			"No addresses: this device does not derive them for a wallet that reuses a key."},
		{"composerCopyOriginsChanged", "8s", composerCopyOriginsChanged(),
			"Same id, but the slot origins below changed. Cards minted for the old origins will not seat here."},
		{"composerCopySeatPrompt", "8s", composerCopySeatPrompt(2, 1, 2, 3),
			"Slot @2, Path 1 key 2 of 3: choose a key"},
		{"composerCopySeatKeyPathPrompt", "8s", composerCopySeatKeyPathPrompt(0),
			"Slot @0, key path (spends alone): choose a key"},
		{"composerCopyDateFloor", "8t", composerCopyDateFloor(),
			"This build will not write a date before 2009 as a time lock."},
		{"composerCopyDateCeiling", "8t", composerCopyDateCeiling(),
			"This build writes dates up to 2038-01-19. For a later time, use a block height instead."},
		{"composerCopyRelativeCeiling", "8u", composerCopyRelativeCeiling(),
			"Relative locks reach at most 455 days in blocks or 388 days in time. Use an absolute date."},
		// §7d's same-key refusal. NOT a §8 blockquote -- §7d states the rule and
		// §11 admits "a quoted string in its table", which is what this is
		// (review r0 M-4).
		{"composerCopySameXpub", "7d", composerCopySameXpub(0, 1),
			"Slots @0 and @1 hold the same key. Every slot needs a different key."},
		{"composerCopySameOriginFewFingerprints", "8v", composerCopySameOriginFewFingerprints(),
			"Two keys declare the same origin and not both carry a fingerprint. This template could not be restored. Use cards or records with fingerprints."},
		{"composerCopyHashlockNoPayloadLead", "H2-3", composerCopyHashlockNoPayloadLead(),
			"No hash record in the payload. Type a phrase below, or make one with ms hashlock on the host."},
		{"composerCopyHashlockPhraseLead", "H2-4.2", composerCopyHashlockPhraseLead(),
			"This screen does that hashing for you. Use a phrase you have never used anywhere else."},
		{"composerCopyHashlockRefusal", "H2-4.2", composerCopyHashlockRefusal(hashlock.ErrMS1Shaped),
			"That is a preimage plate, not a phrase. On the host, run ms hashlock with it and load the hash: record it prints."},
		{"composerCopyHashlockHardenedWarning", "H2-4.3a", composerCopyHashlockHardenedWarning(),
			"Even a 20-character phrase falls in about 72 days on one GPU, and shorter ones fall sooner. Choose it from a generator. If you have used this phrase anywhere else, press Back and choose another. Continue?"},
		{"composerCopyHashlockSHA256Warning", "H2-4.3b", composerCopyHashlockSHA256Warning(),
			"This is the brainwallet construction: anyone holding the digest tests 10^10 phrases per second. A phrase a person chose is not safe here; use six diceware words. If you have used this phrase anywhere else, press Back and choose another. Continue?"},
		{"composerCopyHashlockDerivingLead", "H2-4.4", composerCopyHashlockDerivingLead(),
			"Deriving. This takes about 10 seconds."},
		{"composerCopyHashlockConfirm", "H2-4.5", composerCopyHashlockConfirm("b867db87..edbc96cb", "hardened", 100,
			composerCopyHashlockRelation(-1), composerCopyHashlockOtherPath(), md.KindSha256),
			"hash  sha256 b867db87..edbc96cb method: hardened   chars: 100 no hash: record in the payload has this digest " +
				"another path has a different hash: back up every phrase " +
				// H6 R0 round 0 (journey I-2): the middle sentence used to read
				// "The phrase and method are not on this device", which §2.2's
				// retention and §6's plate make FALSE. SPEC_hashlock_H2_device
				// §4.5's blockquote is rewritten with it by Task 13, and H6 §0
				// lists it as the FIFTH record this stage falsifies.
				"Write down the phrase, method, hash kind and digest now. This composition holds them until it ends. Without them, this path can never be spent. " +
				"One phrase per policy. Never use it as a passphrase or password anywhere else."},
		{"composerCopyHashlockRelation", "H2-4.5", composerCopyHashlockRelation(0),
			"matches hash 1 in the payload"},
		{"composerCopyHashlockOtherPath", "H2-4.5", composerCopyHashlockOtherPath(),
			"another path has a different hash: back up every phrase"},
		{"composerCopyHashlockReconcile", "H2-4.5", composerCopyHashlockReconcile("b867db87..edbc96cb", "hardened", 100, md.KindSha256),
			"hash  sha256 b867db87..edbc96cb method: hardened   chars: 100 " +
				"Before you cut plates, run ms hashlock --kind sha256 --method hardened with this phrase on the host and check the digest matches. " +
				"If they differ, do not fund this wallet: build it again."},
		{"composerCopyHashEveryPathPhrase", "H2-4.7", composerCopyHashEveryPathPhrase(),
			"HASH ON EVERY PATH Every way to spend this wallet needs a hashlock preimage. It is not on this device and not on these plates. Back up every phrase and its method, and every preimage plate, separately."},
		// H5 §2: the FOR row is driven through composerAnyPathByPhrase, so it
		// needs a state whose PATH carries a digest that is in the phrase set --
		// a bool literal no longer exists to set.
		{"composerCopyHashEveryPathFor", "H2-4.7", composerCopyHashEveryPathFor(composerStateByPhraseForCopyTable()),
			"HASH ON EVERY PATH Every way to spend this wallet needs a hashlock preimage. It is not on this device and not on these plates. Back up every phrase and its method, and every preimage plate, separately."},
		// H6 §5.1's payload PREIMAGE-record confirm. NOT a §8 blockquote: H6's
		// §8 has no body for it, because the spec's §5.1 names
		// composerCopyHashlockConfirm for both payload carriers and that body
		// is phrase-shaped (`method`, `chars`, "write down this phrase"), all
		// three false of a record that carries X and nothing else. Filed as a
		// spec addition; the `verbatim` column is this build's own text, which
		// is what §11's "a quoted string in its table" admits.
		{"composerCopyHashlockPreimageConfirm", "H6-5.1", composerCopyHashlockPreimageConfirm("b867db87..edbc96cb",
			composerCopyHashlockRelation(-1), composerCopyHashlockOtherPath(), md.KindSha256),
			"hash  sha256 b867db87..edbc96cb from a preimage record in this payload " +
				"no hash: record in the payload has this digest " +
				"another path has a different hash: back up every phrase " +
				"Spending this path needs that preimage. It is in the payload and not on these plates. " +
				"Cut a preimage plate for it at Done, or keep the payload."},
		// H6 §5.3, §8.3, §8.4, §8.5, §10.1. The `verbatim` column is the plan's
		// own text for the §8.3 rows and the two §8.4 arms; the pick lead, the
		// plate refusal and §8.5's warning are quoted strings in this table for
		// §11's reason.
		{"composerCopyPreimagePlateLead", "H6-5.3", composerCopyPreimagePlateLead("b867db87..edbc96cb", 2, 100, "hardened", md.KindSha256),
			"hash  sha256 b867db87..edbc96cb   path 2 phrase: 100 characters   method: hardened"},
		{"composerCopyPreimageQRWarning", "H6-8.5", composerCopyPreimageQRWarning(),
			"The QR makes the phrase readable by any camera. A photograph of the plate is a copy of the phrase, and the phrase spends this path."},
		{"composerCopyPreimagePlateHeading", "H6-8.3", composerCopyPreimagePlateHeading(2),
			"Plus 2 preimage plate(s), cut first and NOT part of this backup:"},
		// hash160 + the longest form words: the widest this row gets, which is
		// the one the glyph/raster/fits gates should be measuring.
		{"composerCopyPreimagePlateRow", "H6-8.3", composerCopyPreimagePlateRow(2, md.KindHash160, "b867db87..689b8338", "phrase, hardened, QR"),
			"path 2  hash160 b867db87..689b8338  phrase, hardened, QR"},
		{"composerCopyPhraseLooksLikeDigest", "F-539", composerCopyPhraseLooksLikeDigest(64),
			"THIS LOOKS LIKE A DIGEST You typed 64 hex characters, the width of a digest. This device will hash those characters, so the wallet commits to the TEXT you typed and not to the digest it spells. If you meant a digest you already hold, go back and use the Type a digest row. Continue?"},
		{"composerCopyTwentyByteUnseen", "8", composerCopyTwentyByteUnseen(md.KindRipemd160),
			"20-BYTE HASH, NOT DERIVED HERE This is a ripemd160 digest and nothing on this device has seen a preimage for it. Whoever supplied it can spend this path. A 20-byte hash also leaves far less collision margin than sha256. Check you hold the preimage before you fund this wallet."},
		{"composerCopyPreimageCensusScope", "H6-8.3", composerCopyPreimageCensusScope(),
			"This is what this composition will cut. Plates cut in earlier runs are not known to " +
				"this device and are not listed."},
		{"composerCopyPreimageNotOnAnyPath", "H6-8.3", composerCopyPreimageNotOnAnyPath("b867db87..edbc96cb"),
			"preimage b867db87..edbc96cb: not on any path, will not be cut"},
		{"composerCopyPreimageDeclined", "H6-8.3", composerCopyPreimageDeclined("b867db87..edbc96cb"),
			"preimage b867db87..edbc96cb: declined, will not be cut"},
		{"composerCopyPreimageKeepApart", "H6-8.3", composerCopyPreimageKeepApart(),
			"Keep each preimage plate apart from the policy plates and from the others."},
		{"composerCopyPreimageOnlyNotice", "H6-8.3", composerCopyPreimageOnlyNotice(),
			"One preimage this composition holds is on no path of this policy. It will not be cut. Go back and set a path's hash to it, or leave it."},
		{"composerCopyPreimagePlateRefusal", "H6-5.3", composerCopyPreimagePlateRefusal(),
			"Couldn't build that preimage plate. Go back and choose a smaller form: the phrase without a QR, or the preimage string."},
		{"composerCopyAbortNoPreimage", "H6-8.4a", composerCopyAbortNoPreimage(),
			"NO PREIMAGE PLATE WAS CUT. The phrase dies with this composition. Do not fund this wallet."},
		{"composerCopyAbortPreimageCut", "H6-8.4b", composerCopyAbortPreimageCut(),
			"A PREIMAGE PLATE WAS CUT and no policy plate was. Store or destroy it now; do not leave it with the blanks."},
		{"composerCopyHashEveryPathHeld", "H6-10.1", composerCopyHashEveryPathHeld(),
			"HASH ON EVERY PATH Every way to spend this wallet needs the preimage of a hash. This composition holds the preimage for each one and can cut a plate for it at Done. Store those plates apart from these, and apart from each other."},
		{"composerCopyHashEveryPathHeldPhrase", "H6-10.1", composerCopyHashEveryPathHeldPhrase(),
			"HASH ON EVERY PATH Every way to spend this wallet needs a hashlock preimage. This composition holds the phrase and method for each one and can cut a plate at Done. Store those plates apart from these, and apart from each other."},
		// H6 §5.2's Hashlock plates flow. Quoted strings in this table for
		// §11's reason: H6's §8 carries no blockquote for this route's screens.
		{"composerCopyHashlockPlatesLead", "H6-5.2", composerCopyHashlockPlatesLead(2),
			"2 records can be cut as preimage plates. Whoever holds one can spend its path."},
		{"composerCopyHashlockPlatesEmpty", "H6-5.2", composerCopyHashlockPlatesEmpty(),
			"This payload holds no preimage or phrase record to cut."},
		{"composerCopyHashlockPlatesNotCut", "H6-5.2", composerCopyHashlockPlatesNotCut(),
			"That plate was not cut. The record is still in this payload, so you can cut it again from this list."},
		{"composerCopyPreimagesLoaded", "H6-5.2", composerCopyPreimagesLoaded(2),
			"2 preimage or phrase records loaded."},
		// H6 §9 and §8.8. These two are NOT composer copy; they live in
		// composer_copy.go so this table's four gates reach them, which is the
		// reason the file's own comment gives.
		{"composerCopyHashlockLooksLikeMS1", "H6-9", composerCopyHashlockLooksLikeMS1(),
			"This looks like an ms1 string. A seed plate comes from a payload; a marked hashlock plate comes from the Wallet Policy program, from a phrase typed there or a preimage packed on the host. Continue here to cut it as plain text."},
		{"composerCopyHashlockPhraseNotPassphrase", "H6-8.8", composerCopyHashlockPhraseNotPassphrase(),
			"This payload holds a HASHLOCK PHRASE, not a BIP-39 passphrase. They are not interchangeable: using one as the other opens a different wallet. A hashlock phrase is used in the Wallet Policy program."},
	}
}

// composerStateByPhraseForCopyTable is the smallest composition §8h's phrase
// form applies to: one path, whose hash is a digest the phrase set holds.
//
// It exists because H5 §2 replaced composerState.hashByPhrase with a value set
// plus a predicate over the CURRENT paths, so the table's row can no longer be
// driven by a struct literal. Building it here keeps composerCopyTable a table.
func composerStateByPhraseForCopyTable() *composerState {
	var raw [32]byte
	for i := range raw {
		raw[i] = byte(i)
	}
	d := composerTestLock(raw)
	st := &composerState{list: md.PathList{Wrapper: md.ComposeWsh, Paths: []md.SpendPath{{Hash: d}}}}
	composerNotePhraseDigest(st, d)
	return st
}

// TestComposerCopyIsVerbatimFromTheSpec compares every shipped string with
// SPEC §8 word for word.
//
// normalizeDrawn is deliberately the comparator: it is the same reduction
// assertModalBodyFits applies to a drawn frame, so a row that passes here
// passes there for the same reason -- and the spec's hard wrap, which is a
// document convention, is not mistaken for a difference in the string.
func TestComposerCopyIsVerbatimFromTheSpec(t *testing.T) {
	for _, r := range composerCopyTable() {
		if normalizeDrawn(r.got) != normalizeDrawn(r.verbatim) {
			t.Errorf("%s (SPEC §%s) does not match the spec.\n got:  %q\n want: %q",
				r.fn, r.section, r.got, r.verbatim)
		}
	}
}

// TestComposerCopyIsDrawable is the shipped prose guard, applied to all 39.
//
// A rune the body face lacks does not degrade one glyph: it blanks the whole
// modal body (gui/font_coverage_test.go). The banned set here is the one
// gui/multisig_build_prose_test.go:91 refuses, verbatim.
func TestComposerCopyIsDrawable(t *testing.T) {
	for _, r := range composerCopyTable() {
		if strings.ContainsAny(r.got, "—–·‘’“”…") {
			t.Errorf("%s carries a glyph the body face lacks, so its line does not draw:\n%q", r.fn, r.got)
		}
		for _, ch := range r.got {
			if ch > 126 || (ch < 32 && ch != '\n') {
				t.Errorf("%s carries the non-ASCII or control rune %q; device strings are ASCII only", r.fn, ch)
			}
		}
	}
}

// TestComposerCopyTableCoversEveryBody is the reason this file exists.
//
// It parses composer_copy.go and requires every composerCopy* declaration to
// appear in the table. A body added later without a row would otherwise ship
// with none of §12 item 5's four gates on it, and nothing would say so.
func TestComposerCopyTableCoversEveryBody(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "composer_copy.go", nil, 0)
	if err != nil {
		t.Fatalf("parsing composer_copy.go: %v", err)
	}
	covered := map[string]bool{}
	for _, r := range composerCopyTable() {
		covered[r.fn] = true
	}
	var declared int
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv != nil || !strings.HasPrefix(fn.Name.Name, "composerCopy") {
			continue
		}
		declared++
		if !covered[fn.Name.Name] {
			t.Errorf("%s is declared in composer_copy.go but is in no row of "+
				"composerCopyTable -- so SPEC §12 item 5's glyph, raster, "+
				"modal-fits and fires-on-condition gates do not reach it",
				fn.Name.Name)
		}
		delete(covered, fn.Name.Name)
	}
	for stray := range covered {
		t.Errorf("composerCopyTable names %s, which composer_copy.go does not declare", stray)
	}
	// 40 SINCE THE DATE CEILING. §8t covered the floor and §8 had no body for
	// the top, so a date past 2038-01-19 was refused as "that date does not
	// exist" -- false of 2045-06-01, on the archetype §4d lists first. The new
	// body is filed as a §8 addition (F-456) so the spec stays the source this
	// table is diffed against.
	// 41 SINCE REVIEW r0 M-4 moved §7d's same-xpub refusal in here: it was an
	// fmt.Sprintf at its own showError, so §12 item 5's four gates did not
	// reach it and this scan did not count it.
	// 42 SINCE H2 TASK 3 added composerCopyHashlockNoPayloadLead (the
	// no-payload lead on `Which hash?`, SPEC_hashlock_H2_device §4.1).
	// 53 SINCE H2 TASK 4 added the phrase route's eleven bodies: the phrase
	// lead, the phrase-rule refusal, both method warnings, the deriving
	// lead, the confirm body, its relation line and its other-path line, the
	// reconciliation screen, and the two §8h forms
	// (SPEC_hashlock_H2_device §4.2-§4.7). The last two arrived in the R0
	// round 0 fold: composerCopyHashlockOtherPath (journey I-1) and
	// composerCopyHashlockReconcile (adversarial I-1 = fidelity I-2 =
	// journey I-3, the line §8h's guard had made unreachable).
	// 54 SINCE H6 TASK 8b added composerCopyHashlockPreimageConfirm (§5.1's
	// confirm for a payload PREIMAGE record, whose fields the phrase-shaped
	// confirm body does not have).
	// 67 SINCE H6 TASK 9 added the Done review's thirteen: §5.3's masked pick
	// lead and the plate refusal, §8.5's QR warning, §8.3's five row forms and
	// its stand-alone notice, both §8.4 arms and both §10.1 held arms.
	// 71 SINCE H6 TASK 10 added the Hashlock plates flow's four: the list lead,
	// the empty-payload refusal, this flow's own abort (which is NEITHER §8.4
	// arm) and the door's preimage count.
	// 73 SINCE H6 TASK 11 added §9's shared ms1-shaped warning and §8.8's
	// Password-program notice. Neither is composer copy; both live in this file
	// so the four gates above reach them.
	// 74 SINCE F-497 added §8.3's census scope line, which says the block lists
	// what THIS composition will cut and that plates cut in earlier runs are
	// unknown to the device (operator ruling 2026-09-06: the limitation is
	// accepted and the copy owns it).
	// 75 SINCE review I-2 split §8s's changed-id line in two. The id line cannot
	// state the case where the id is UNCHANGED and the per-slot origins moved --
	// which seating can do, because the Template-ID is origin-invariant by
	// construction -- and a card minted for a stale origin passes the stub check
	// and is refused by slotMatchesCard. Two failures at two layers need two
	// sentences, or the operator is sent to look in the wrong place.
	// 76 SINCE F-514: the device derived a fundable address, in silence, for a
	// policy whose descriptor Bitcoin Core refuses as "contains duplicate
	// public keys". The address is correct; what was missing was any way for
	// the operator to learn before funding it that the coordinator they will
	// spend from will not import the wallet.
	// 77 SINCE F-531: the device now DECLINES to derive an address for a policy
	// that reuses a key slot, and a refusal needs a sentence of its own. The
	// F-514 body above qualifies an address; this one explains its absence, and
	// folding them into one string would have made that body claim a refusal on
	// the screens where addresses are still shown.
	// 78 SINCE F-530 carried the same rule to a descriptor that arrived with no
	// md1 behind it -- a scanned QR or a payload record. It needs its OWN
	// sentence because the md1 one says "slot @N", and a scanned descriptor has
	// no @N on any screen the operator has seen.
	// 79 SINCE SPEC_hashlock_kinds §7.2 SPLIT §8i IN TWO. The entry body fires
	// on row selection, before a kind exists, so it can name none; the consent
	// body fires on a decided policy, where §6's both-or-neither rule says the
	// kind must be named. One string could not do both jobs once four kinds
	// existed -- it asserted SHA-256 at a moment when the answer might be
	// ripemd160.
	// 80 SINCE SPEC_hashlock_kinds §7.1 gave the hash KIND its own pick screen,
	// which needs a lead. It is SHORT on purpose: composerPickScreen draws the
	// lead as the first body row and pages the rest, so a long one pushes kind
	// rows off the first page. The first draft did exactly that and hid
	// hash160 behind Button2 (TestComposerHashKindScreenDrawsAllFourRows).
	// 81 SINCE SPEC_hashlock_kinds §8 WAS FINALLY IMPLEMENTED. It was a
	// normative section of a GREEN spec with no code, no copy and no test
	// behind it, found missing independently by the journey walk and the
	// adversarial lens after the rest of phase 4 had shipped.
	// 82 SINCE F-539 turned the digest-shaped REFUSAL into a confirmable
	// warning (operator ruling 2026-09-16). The refusal it replaced was not a
	// composerCopy* body at all -- it was an arm of composerCopyHashlockRefusal
	// -- so the count moves by one even though one string replaced another.
	// 83 SINCE the composer fable review r0 C-1 gave §8m a sixth structural
	// refusal: a policy admits at most one key-less path, because two of them
	// lower to an or_i with two unsafe arms and the script is malleable. No
	// existing body could carry it -- §8a's confirm describes ONE key-less
	// path and is true of it, and the lock-only and key-less-under-tr lines
	// name different conditions.
	// 84 SINCE fable review r0 lens 2 I-2 added the mixed-lock-bases notice.
	// It is filed under §8g because it is that section's register -- a
	// coordinator names a wallet it will refuse -- and because §8's existing
	// lock sections (§8c, §8o) are about OPERANDS and bands, not about who
	// imports the result.
	// 85 SINCE fable review r0 lens 4 M-5 split §8m's lock-only line in two.
	// ErrComposeLockOnlyPath refuses "neither keys nor a hash", which is TWO
	// operator-visible states, and the lock-only body was being drawn on a row
	// reading "Path 2: empty" -- naming a time lock nobody had set.
	// 86 SINCE fable review r0 lens 5 I-1/I-2 added composerCopyOutsideLianaModel
	// (§8x): 17 of 56 composable shapes import into Liana 8.0, and the device
	// named only two of the other nine refusal classes (NUMS at §8f, same-seed
	// at §8g) before this row. No existing body could carry it -- §8f and §8g
	// each name ONE class unconditionally, and this one names whichever of
	// nine applies, in Liana's own order of refusal.
	// 87 SINCE F-449 STAGE 4 TASK 5 added §8y's kind-1 key-path line: the
	// §8f body is false for kind 1 about Nunchuk, so it could not be reused.
	// 92 SINCE TASK 6 added the key-path choice screen's lead and two rows,
	// the RESET signal that names which fact dropped a Liana choice, and the
	// unmet-request refusal composerCompose raises (R0 m1).
	if declared != 92 {
		t.Errorf("composer_copy.go declares %d bodies, the plan and the table know 92 -- "+
			"if that is deliberate, update both", declared)
	}
}

// TestComposerCopyTableCoversTheSameXpubRefusal is review r0 M-4.
//
// §11: "the copy of each refusal is a blockquote in §8 or a quoted string in
// its table, so the glyph and modal-fits gates cover it." The same-xpub body
// §7d requires was neither -- it was an fmt.Sprintf at its own showError, and
// TestComposerCopyTableCoversEveryBody only scans composerCopy* declarations,
// so nothing counted it and none of §12 item 5's four gates reached it.
func TestComposerCopyTableCoversTheSameXpubRefusal(t *testing.T) {
	body := composerCopySameXpub(0, 1)
	var found bool
	for _, r := range composerCopyTable() {
		if r.fn == "composerCopySameXpub" {
			found = true
			if normalizeDrawn(r.got) != normalizeDrawn(body) {
				t.Errorf("the table's row and the function disagree:\n got:  %q\n want: %q",
					r.got, body)
			}
		}
	}
	if !found {
		t.Error("composerCopySameXpub is not in composerCopyTable, so §12 item 5's glyph, " +
			"raster, modal-fits and fires-on-condition gates do not reach the same-xpub " +
			"refusal (§11)")
	}
	assertModalBodyFits(t, "the §7d same-xpub refusal", errorScreenBody, body)
}

// TestComposerLockEchoesAreGrammatical pins F-628 (lens 1 N-1).
//
// The echoes read "1 blocks". A relative block lock of exactly 1 is REACHABLE
// from the pad -- composerLockEntry admits 1..65535 -- so an operator can meet
// this string, which is why it is a fix and not a note.
//
// MUTATION: drop either plural() call and the matching row fails.
func TestComposerLockEchoesAreGrammatical(t *testing.T) {
	for _, tc := range []struct {
		name, got, want string
	}{
		{"one block", composerCopyLockEchoBlocks(1), "1 block (about 0.0 days)"},
		{"two blocks", composerCopyLockEchoBlocks(2), "2 blocks (about 0.0 days)"},
		{"1000 blocks (the §8c example, unchanged)",
			composerCopyLockEchoBlocks(1000), "1000 blocks (about 6.9 days)"},
		{"one day, one unit", composerCopyLockEchoDays(1, 1), "1 day = 1 unit of 512 s (0.0 days)"},
		{"the pad minimum: one day", composerCopyLockEchoDays(1, composerDaysToUnits(1)),
			"1 day = 169 units of 512 s (1.0 days)"},
		{"90 days (the §8c example, unchanged)",
			composerCopyLockEchoDays(90, 15188), "90 days = 15188 units of 512 s (90.0 days)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Errorf("\n got  %q\n want %q", tc.got, tc.want)
			}
		})
	}
}

// TestSubDayLockIsUnreachableFromThePad pins the OTHER half of F-628 rather
// than writing copy for it.
//
// A 3-unit `older` echoes "0 days = 3 units of 512 s (0.0 days)" -- a lock
// that claims zero days while enforcing 25.6 minutes. The review saw it only
// because the harness fed units directly: the pad takes DAYS, refuses 0, and
// one day is already 169 units.
//
// So the string is wrong and no operator can reach it. Inventing copy for an
// unreachable state would add a body the spec does not carry and nobody will
// ever read; asserting the unreachability instead means that if a future pad
// ever admits sub-day locks, THIS test fails and the copy gets written then --
// by someone who knows what the new pad offers.
func TestSubDayLockIsUnreachableFromThePad(t *testing.T) {
	if got := composerDaysToUnits(1); got != 169 {
		t.Fatalf("one day is %d units, want 169 -- §6b's conversion moved, so the "+
			"sub-day reasoning below no longer holds", got)
	}
	// The pad's day band refuses zero, so `days` is never 0 at the echo.
	if _, ok := composerDaysBandEcho("0"); ok {
		t.Error("the pad now accepts 0 days: a sub-day lock is reachable, and " +
			"composerCopyLockEchoDays would echo \"0 days = N units\" -- write the " +
			"sub-day copy now (F-628)")
	}
	if _, ok := composerDaysBandEcho("1"); !ok {
		t.Error("the pad no longer accepts 1 day; this pin is measuring the wrong thing")
	}
}
