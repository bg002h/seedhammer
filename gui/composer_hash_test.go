package gui

import (
	"encoding/hex"
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
