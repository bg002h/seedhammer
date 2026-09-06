package gui

import (
	"encoding/hex"
	"fmt"
	"strings"
	"testing"

	"seedhammer.com/codex32"
	"seedhammer.com/gui/widget"
	"seedhammer.com/hashlock"
	"seedhammer.com/sysw"
)

func TestComposerHashRowIsShortEnoughToDraw(t *testing.T) {
	var d [32]byte
	raw, _ := hex.DecodeString("0123456789abcdeffedcba98765432100123456789abcdeffedcba9876543210")
	copy(d[:], raw)
	got := composerHashRow(1, d)
	if !strings.HasPrefix(got, "hash 1  0123456789abcdef"[:8]) {
		t.Errorf("the row does not lead with the index and the digest head: %q", got)
	}
	if !strings.Contains(got, "..") {
		t.Errorf("the row does not elide the middle, so a 64-hex line would be cut: %q", got)
	}
	if len(got) > 32 {
		t.Errorf("the row is %d characters; §6c budgets about 28 so it draws inside the "+
			"436 px label rather than being cut", len(got))
	}
	assertChoiceLabelFits(t, got)
}

// TestComposerPayloadDigestsTakesOnlyWellFormedHashRecords: a malformed
// hash: record is ClassUnknown and inert (§6a), so it must not appear on the
// pick list -- and it changes no count but the not-understood one.
func TestComposerPayloadDigestsTakesOnlyWellFormedHashRecords(t *testing.T) {
	s := composerSessionWith([]string{
		composerTestHashRecord,
		"hash:00",             // 1 byte, not 32
		composerTestKeyRecord, // a different class entirely
	}, nil)
	got := composerPayloadDigests(s)
	if len(got) != 1 {
		t.Fatalf("composerPayloadDigests returned %d digests, want 1", len(got))
	}
}

// TestComposerHashRuleIsStatedAtEntry is §8i's fires-on-condition test and
// its fits assertion. The reference wallet's own README records months lost
// to hashing a passphrase directly, which is exactly what this line prevents.
func TestComposerHashRuleIsStatedAtEntry(t *testing.T) {
	assertModalBodyFits(t, "the §8i 32-byte preimage rule", errorScreenBody, composerCopyHashRule())
	if !strings.Contains(composerCopyHashRule(), "32-byte") {
		t.Error("the §8i line does not state the size the preimage must be")
	}
	if !strings.Contains(composerCopyHashRule(), "never be spent") {
		t.Error("the §8i line does not state the consequence of getting it wrong")
	}
}

func TestComposerHexKeysAreHexAndNothingElse(t *testing.T) {
	for _, r := range composerHexKeys {
		if r == '\n' {
			continue
		}
		if !strings.ContainsRune("0123456789abcdef", r) {
			t.Errorf("the hex pad offers %q, which is not a hex digit; §6c accepts a "+
				"digest only when exactly 64 valid hex characters are present", r)
		}
	}
}

// H2: `Which hash?`'s rows are built ONCE and each named row's index is recorded
// (spec §5; r2 review C-4). This test covers the ROW SET only -- the labels, the
// three recorded indices and the lead -- with 0, 1 and 2 payload digests.
//
// It does NOT drive composerHashEdit and therefore CANNOT see the dispatch
// switch at all; the round-0 comment here claimed it caught an index-arithmetic
// reversion, and that claim was false (r0 fidelity I-1). The dispatch is covered
// behaviourally by TestComposerHashEditDispatchesByRowLabel in
// composer_hashlock_test.go, which taps each row through composerHashEdit with
// two payload digests loaded -- the shape that distinguishes a surgical
// reversion (phrase row kept at the right index, hex+none merged into one
// clearing arm) from correct code. MUTATION for THIS test: swap the order of
// the phrase and hex appends in composerHashRows -> the labels-misplaced
// assertion fails.
func TestWhichHashRowsAreLabelKeyed(t *testing.T) {
	for _, n := range []int{0, 1, 2} {
		recs := make([]string, n)
		for i := range recs {
			recs[i] = "hash:" + strings.Repeat(fmt.Sprintf("%02x", 0xa0+i), 32)
		}
		s := composerSessionWith(recs, nil)
		rows := composerHashRows(s, nil)
		if got := len(rows.labels); got != n+3 {
			t.Fatalf("n=%d: %d rows, want %d", n, got, n+3)
		}
		if rows.labels[rows.phraseRow] != composerHashRowPhrase ||
			rows.labels[rows.hexRow] != "Type 64 hex" ||
			rows.labels[rows.noneRow] != "No hash lock" {
			t.Fatalf("n=%d: labels misplaced: %v", n, rows.labels)
		}
		if rows.phraseRow != n || rows.hexRow != n+1 || rows.noneRow != n+2 {
			t.Fatalf("n=%d: indices %d/%d/%d", n, rows.phraseRow, rows.hexRow, rows.noneRow)
		}
		if n == 0 && !strings.Contains(rows.lead, "Type a phrase below") {
			t.Errorf("no-payload lead missing: %q", rows.lead)
		}
		if n > 0 && rows.lead != "Which hash?" {
			t.Errorf("lead with payload digests: %q", rows.lead)
		}
	}
	if composerPickScreenMaxRows < 2+3 {
		t.Fatalf("composerPickScreenMaxRows = %d < the longest row set", composerPickScreenMaxRows)
	}
}

// ─── H6 Task 8b: `Which hash?` gains two bands (spec §5.1) ───────────────────
//
// The fixtures are built by the ENCODERS, never pasted: composerTestPreimageX
// is the sha256 preimage of the anchor phrase, so the preimage record's digest
// is hashlockAnchorSHA_H and the two bands can be made to collide on purpose.

// composerTestPreimageRecord is a ClassPreimage plate string -- the ms1
// kind-0x03 string under the id `hash` (SPEC_ms_hashlock §1 rule 2), built by
// codex32.EncodeMS1Preimage rather than transcribed.
func composerTestPreimageRecord(t *testing.T, x [32]byte) string {
	t.Helper()
	s, err := codex32.EncodeMS1Preimage(x)
	if err != nil {
		t.Fatalf("encoding a preimage plate string: %v", err)
	}
	return s
}

// composerTestPhraseRecord is a ClassPhrase record: `phrase:<hex of
// "<method>,<phrase>">`, built by sysw.PhraseRecordString.
func composerTestPhraseRecord(method sysw.HashlockMethod, phrase string) string {
	return sysw.PhraseRecordString(method, phrase)
}

// composerTestPreimageX is the SHA-256 preimage of the anchor phrase, so
// hashlock.Digest of it is hashlockAnchorSHA_H -- the collision the
// `(in payload)` annotation exists for.
func composerTestPreimageX() [32]byte {
	return hashlock.PreimageSHA256([]byte(hashlockAnchorPhrase))
}

// TestWhichHashRowsCarryTheTwoNewBands is §5.1's six bands, IN ORDER, by label,
// with 0, 1 and 2 records of each new class.
//
// MUTATION: append the phrase-record band before the preimage band -> the
// order assertion fails.
// MUTATION: drop the phrase-record band -> the row count fails.
func TestWhichHashRowsCarryTheTwoNewBands(t *testing.T) {
	x := composerTestPreimageX()
	for _, n := range []int{0, 1, 2} {
		t.Run(fmt.Sprintf("%d of each", n), func(t *testing.T) {
			var recs []string
			for i := 0; i < n; i++ {
				recs = append(recs, "hash:"+strings.Repeat(fmt.Sprintf("%02x", 0xa0+i), 32))
			}
			var secret []string
			for i := 0; i < n; i++ {
				var y [32]byte
				copy(y[:], x[:])
				y[31] = byte(i)
				secret = append(secret, composerTestPreimageRecord(t, y))
			}
			for i := 0; i < n; i++ {
				secret = append(secret, composerTestPhraseRecord(sysw.HashlockSHA256,
					fmt.Sprintf("%s %d", hashlockAnchorPhrase, i)))
			}
			s := composerSessionWith(recs, secret)
			st := composerStateWithPaths(t, 1)
			rows := composerHashRows(s, st)
			if got, want := len(rows.labels), 3*n+3; got != want {
				t.Fatalf("%d rows, want %d: %v", got, want, rows.labels)
			}
			if len(rows.preimages) != n || len(rows.phrases) != n {
				t.Fatalf("preimages=%d phrases=%d, want %d of each", len(rows.preimages), len(rows.phrases), n)
			}
			// The band order of §5.1: hash:, preimage, phrase record, then the
			// three shipped rows.
			if rows.preimageRow != n || rows.phraseRecRow != 2*n ||
				rows.phraseRow != 3*n || rows.hexRow != 3*n+1 || rows.noneRow != 3*n+2 {
				t.Fatalf("band starts %d/%d/%d/%d/%d with n=%d",
					rows.preimageRow, rows.phraseRecRow, rows.phraseRow, rows.hexRow, rows.noneRow, n)
			}
			for i := 0; i < n; i++ {
				if got := rows.labels[rows.preimageRow+i]; !strings.HasPrefix(got, fmt.Sprintf("preimage %d ", i+1)) {
					t.Errorf("preimage row %d reads %q", i, got)
				}
				if got := rows.labels[rows.phraseRecRow+i]; got != fmt.Sprintf("phrase record %d (derive to see the digest)", i+1) {
					t.Errorf("phrase-record row %d reads %q", i, got)
				}
			}
			if rows.labels[rows.phraseRow] != composerHashRowPhrase ||
				rows.labels[rows.hexRow] != "Type 64 hex" ||
				rows.labels[rows.noneRow] != "No hash lock" {
				t.Fatalf("the three shipped rows moved: %v", rows.labels)
			}
			if len(rows.labels) > composerPickScreenMaxRows {
				t.Fatalf("the row set is %d rows, over composerPickScreenMaxRows = %d",
					len(rows.labels), composerPickScreenMaxRows)
			}
		})
	}
}

// TestWhichHashAnnotatesAHashRowThePayloadAlsoCarries is §5.1 Step 2. Bands 1
// and 2 draw the same digest text, adjacent, with DIFFERENT consequences --
// band 1 assigns and holds nothing, band 2 cuts a plate -- so the hash: row
// says so.
//
// MUTATION: drop the annotation -> the two rows are byte-identical apart from
// their leading word and this test fails on the `(in payload)` assertion.
func TestWhichHashAnnotatesAHashRowThePayloadAlsoCarries(t *testing.T) {
	x := composerTestPreimageX()
	h := hashlock.Digest(&x)
	s := composerSessionWith(
		[]string{"hash:" + hex.EncodeToString(h[:]), composerTestHashRecord},
		[]string{composerTestPreimageRecord(t, x)},
	)
	st := composerStateWithPaths(t, 1)
	rows := composerHashRows(s, st)
	if !strings.HasSuffix(rows.labels[0], "  (in payload)") {
		t.Errorf("the hash: row whose digest the payload also carries is not annotated: %q", rows.labels[0])
	}
	if strings.Contains(rows.labels[1], "(in payload)") {
		t.Errorf("a hash: row with no matching record was annotated: %q", rows.labels[1])
	}
}

// TestWhichHashRowsDrawOnOneLine is §5.1's MEASUREMENT, at sh2DisplaySize, in
// composerPageLines' own band -- the ONE measure site (composer_paged.go).
// Every row of every band must draw on ONE line; a picker band that wraps
// halves the number of choices a frame can carry.
func TestWhichHashRowsDrawOnOneLine(t *testing.T) {
	p := newPlatform()
	p.display = sh2DisplaySize
	ctx := NewContext(p)
	dims := sh2DisplaySize
	_, width := composerTextBand(dims)
	_, one := widget.Labelw(&ctx.B, ctx.Styles.body, width, descriptorTheme.Text, "X")
	if one.Y <= 0 {
		t.Fatalf("a one-line body row measured %v; this test cannot count lines", one)
	}
	var d [32]byte
	copy(d[:], mustHexBytes(t, hashlockAnchorSHA_H))
	for _, row := range []string{
		composerHashRow(10, d),
		composerHashInPayloadRow(10, d),
		composerHashPreimageRow(10, d),
		composerHashPhraseRow(10, nil),
		composerHashPhraseRow(10, &d),
		composerHashRowPhrase, "Type 64 hex", "No hash lock",
	} {
		_, sz := widget.Labelw(&ctx.B, ctx.Styles.body, width, descriptorTheme.Text, row)
		lines := (sz.Y + one.Y - 1) / one.Y
		t.Logf("%-46q %2d chars %3d px %d line(s)", row, len(row), sz.X, lines)
		if lines != 1 {
			t.Errorf("%q wraps to %d lines in the %d px band; a picker row must draw on one",
				row, lines, width)
		}
	}
	t.Logf("band width %d px, one line %d px, band height %d px",
		width, one.Y, dims.Y-2*leadingSize-8)
}
