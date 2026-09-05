package gui

import (
	"encoding/hex"
	"fmt"
	"strings"
	"testing"
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
		rows := composerHashRows(s)
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
