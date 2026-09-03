package gui

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"
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
			"HASH ON EVERY PATH Every way to spend this wallet needs the preimage of a hash. It is not on this device and not on these plates. Back the preimage up separately."},
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
	}
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
	if declared != 41 {
		t.Errorf("composer_copy.go declares %d bodies, the plan and the table know 41 -- "+
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
