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
		{"composerCopyKeylessPath", "8a", composerCopyKeylessPath(),
			"KEY-LESS PATH (EXPERIMENTAL) This path needs no signature. Whoever knows the preimage of its hash can spend it. If that preimage is ever engraved, the plate is bearer access."},
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
		{"composerCopyNUMS", "8f", composerCopyNUMS(),
			"KEY PATH: NONE (NUMS) Spends use the script paths only. Bitcoin Core and Nunchuk import this form. Liana and BIP-388 signers need an unspendable xpub instead (see F-449)."},
		{"composerCopySameSeedThreshold", "8g", composerCopySameSeedThreshold([]uint8{1, 2}, 2, 3),
			"SAME SEED, SAME PATH Slots @1 and @2 are the same seed. This path's 2-of-3 can be satisfied by one person. Liana will refuse it."},
		{"composerCopySameSeedBelow", "8g", composerCopySameSeedBelow([]uint8{1, 2}, 3),
			"SAME SEED, SAME PATH Slots @1 and @2 are the same seed. One person holds 2 of the 3 signatures this path needs. Liana will refuse it."},
		{"composerCopyHashEveryPath", "8h", composerCopyHashEveryPath(),
			"HASH ON EVERY PATH Every way to spend this wallet needs the preimage of a hash. It is not on this device and not on these plates. Back up every preimage separately."},
		{"composerCopyHashRule", "8i", composerCopyHashRule(),
			"The hash must be SHA-256 of a 32-byte value. A passphrase must be hashed to 32 bytes first, then hashed again. A hash of the passphrase itself can never be spent."},
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
		{"composerCopyRefuseKeylessTr", "8m", composerCopyRefuseKeylessTr(),
			"This build will not put a key-less path in taproot. Use wsh, or add a key."},
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
			composerCopyHashlockRelation(-1), composerCopyHashlockOtherPath()),
			"hash  b867db87..edbc96cb method: hardened   chars: 100 no hash: record in the payload has this digest " +
				"another path has a different hash: back up every phrase " +
				// H6 R0 round 0 (journey I-2): the middle sentence used to read
				// "The phrase and method are not on this device", which §2.2's
				// retention and §6's plate make FALSE. SPEC_hashlock_H2_device
				// §4.5's blockquote is rewritten with it by Task 13, and H6 §0
				// lists it as the FIFTH record this stage falsifies.
				"Write down this phrase, the method and this digest now. This composition holds them until it ends. Without both, this path can never be spent. " +
				"One phrase per policy. Never use this phrase as a passphrase or a password anywhere else."},
		{"composerCopyHashlockRelation", "H2-4.5", composerCopyHashlockRelation(0),
			"matches hash 1 in the payload"},
		{"composerCopyHashlockOtherPath", "H2-4.5", composerCopyHashlockOtherPath(),
			"another path has a different hash: back up every phrase"},
		{"composerCopyHashlockReconcile", "H2-4.5", composerCopyHashlockReconcile("b867db87..edbc96cb", "hardened", 100),
			"hash  b867db87..edbc96cb method: hardened   chars: 100 " +
				"Before you cut plates, run ms hashlock with this phrase and method on the host and check the digest matches. " +
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
			composerCopyHashlockRelation(-1), composerCopyHashlockOtherPath()),
			"hash  b867db87..edbc96cb from a preimage record in this payload " +
				"no hash: record in the payload has this digest " +
				"another path has a different hash: back up every phrase " +
				"Spending this path needs that preimage. It is in the payload and not on these plates. " +
				"Cut a preimage plate for it at Done, or keep the payload."},
		// H6 §5.3, §8.3, §8.4, §8.5, §10.1. The `verbatim` column is the plan's
		// own text for the §8.3 rows and the two §8.4 arms; the pick lead, the
		// plate refusal and §8.5's warning are quoted strings in this table for
		// §11's reason.
		{"composerCopyPreimagePlateLead", "H6-5.3", composerCopyPreimagePlateLead("b867db87..edbc96cb", 2, 100, "hardened"),
			"hash  b867db87..edbc96cb   path 2 phrase: 100 characters   method: hardened"},
		{"composerCopyPreimageQRWarning", "H6-8.5", composerCopyPreimageQRWarning(),
			"The QR makes the phrase readable by any camera. A photograph of the plate is a copy of the phrase, and the phrase spends this path."},
		{"composerCopyPreimagePlateHeading", "H6-8.3", composerCopyPreimagePlateHeading(2),
			"Plus 2 preimage plate(s), cut first and NOT part of this backup:"},
		{"composerCopyPreimagePlateRow", "H6-8.3", composerCopyPreimagePlateRow(2, "b867db87..edbc96cb", "phrase, hardened, QR"),
			"path 2  b867db87..edbc96cb  phrase, hardened, QR"},
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
	}
}

// composerStateByPhraseForCopyTable is the smallest composition §8h's phrase
// form applies to: one path, whose hash is a digest the phrase set holds.
//
// It exists because H5 §2 replaced composerState.hashByPhrase with a value set
// plus a predicate over the CURRENT paths, so the table's row can no longer be
// driven by a struct literal. Building it here keeps composerCopyTable a table.
func composerStateByPhraseForCopyTable() *composerState {
	var d [32]byte
	for i := range d {
		d[i] = byte(i)
	}
	st := &composerState{list: md.PathList{Wrapper: md.ComposeWsh, Paths: []md.SpendPath{{Hash: &d}}}}
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
	if declared != 71 {
		t.Errorf("composer_copy.go declares %d bodies, the plan and the table know 71 -- "+
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
