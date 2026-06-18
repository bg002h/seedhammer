package codex32

import (
	"errors"
	"testing"
)

func TestExportedLengthConstants(t *testing.T) {
	cases := []struct {
		name      string
		got, want int
	}{
		{"ShortCodeMinLength", ShortCodeMinLength, 48},
		{"ShortCodeMaxLength", ShortCodeMaxLength, 93},
		{"LongCodeMinLength", LongCodeMinLength, 125},
		{"LongCodeMaxLength", LongCodeMaxLength, 127},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s = %d, want %d", c.name, c.got, c.want)
		}
	}
	if ShortCodeMinLength != shortCodeMinLength ||
		ShortCodeMaxLength != shortCodeMaxLength ||
		LongCodeMinLength != longCodeMinLength ||
		LongCodeMaxLength != longCodeMaxLength {
		t.Error("exported length consts diverge from private originals")
	}
}

func TestDescribe(t *testing.T) {
	if got := Describe(nil); got != "" {
		t.Errorf("Describe(nil) = %q, want \"\"", got)
	}
	sentinels := []struct {
		in   error
		want string
	}{
		{errInvalidChecksum, "bad checksum"},
		{errInvalidLength, "wrong length"},
		{errInvalidCharacter, "invalid character"},
		{errInvalidCase, "mixed case"},
		{errInvalidThreshold, "bad threshold"},
		{errInvalidShareIndex, "bad share index"},
		{errIncompleteGroup, "incomplete group"},
		{errInsufficientShares, "invalid"}, // Interpolate-only → fallback
		{errors.New("other"), "invalid"},
	}
	for _, c := range sentinels {
		if got := Describe(c.in); got != c.want {
			t.Errorf("Describe(%v) = %q, want %q", c.in, got, c.want)
		}
	}
	// Real New errors (wrapped) classify correctly.
	if _, err := New("tooshort"); Describe(err) != "wrong length" {
		t.Errorf("Describe(New short) = %q, want \"wrong length\"", Describe(err))
	}
	if _, err := New("ms10fauxsxxxxxxxxxxxxxxxxxxxxxxxxxxve740yyge2ghp"); Describe(err) != "bad checksum" {
		t.Errorf("Describe(New bad-checksum) = %q, want \"bad checksum\"", Describe(err))
	}
}

func TestConsistentShares(t *testing.T) {
	mk := func(s string) String {
		v, err := New(s)
		if err != nil {
			t.Fatalf("New(%s): %v", s, err)
		}
		return v
	}
	// BIP-93 vector-2 shares: threshold 2, id NAME, indices A and C.
	a := mk("MS12NAMEA320ZYXWVUTSRQPNMLKJHGFEDCAXRPP870HKKQRM")
	c := mk("MS12NAMECACDEFGHJKLMNPQRSTUVWXYZ023FTR2GDZMPY6PN")
	// vector-3 share: threshold 3, id CASH (a field mismatch vs the vector-2 set).
	cash := mk("MS13CASHA320ZYXWVUTSRQPNMLKJHGFEDCA2A8D0ZEHN8A0T")

	if err := ConsistentShares(nil); err != nil {
		t.Errorf("nil set: %v, want nil", err)
	}
	if err := ConsistentShares([]String{a}); err != nil {
		t.Errorf("single share: %v, want nil", err)
	}
	if err := ConsistentShares([]String{a, c}); err != nil {
		t.Errorf("consistent pair: %v, want nil", err)
	}
	if err := ConsistentShares([]String{a, a}); !errors.Is(err, errRepeatedIndex) {
		t.Errorf("repeated index: %v, want errRepeatedIndex", err)
	}
	if err := ConsistentShares([]String{a, cash}); !errors.Is(err, errMismatchedThreshold) {
		t.Errorf("threshold mismatch: %v, want errMismatchedThreshold", err)
	}
}

func TestParsePrefix(t *testing.T) {
	// No separator yet: HRP-candidate chars, nothing Known, no error.
	f, err := ParsePrefix("ms")
	if err != nil {
		t.Fatalf("ParsePrefix(ms) err=%v", err)
	}
	if f.HRP != "" || f.ThresholdKnown || f.IdentifierKnown || f.ShareIndexKnown {
		t.Errorf("ParsePrefix(ms) = %+v, want all-unknown", f)
	}

	// Threshold known at len>=1 after the separator; id not yet.
	f, _ = ParsePrefix("ms12")
	if !f.ThresholdKnown || f.Threshold != 2 {
		t.Errorf("ParsePrefix(ms12) threshold: %+v", f)
	}
	if f.IdentifierKnown {
		t.Errorf("ParsePrefix(ms12) id should be unknown: %+v", f)
	}

	// Threshold '1' is forbidden.
	if _, err := ParsePrefix("ms11"); !errors.Is(err, errInvalidThreshold) {
		t.Errorf("ParsePrefix(ms11) err=%v, want errInvalidThreshold", err)
	}

	// Identifier known at len>=5.
	f, _ = ParsePrefix("ms12name")
	if !f.IdentifierKnown || f.Identifier != "name" {
		t.Errorf("ParsePrefix(ms12name) id: %+v", f)
	}

	// Share index + Unshared at len>=6 (BIP-93 vector 1 prefix).
	f, err = ParsePrefix("ms10tests")
	if err != nil {
		t.Fatalf("ParsePrefix(ms10tests) err=%v", err)
	}
	if !f.ShareIndexKnown || f.ShareIndex != 's' || !f.Unshared {
		t.Errorf("ParsePrefix(ms10tests) share: %+v", f)
	}

	// threshold-0 with a non-S index at len>=6 → determinable error.
	if _, err := ParsePrefix("ms10testa"); !errors.Is(err, errInvalidShareIndex) {
		t.Errorf("ParsePrefix(ms10testa) err=%v, want errInvalidShareIndex", err)
	}

	// threshold-0 leading but len<6 → NOT yet determinable (no error).
	if _, err := ParsePrefix("ms10te"); err != nil {
		t.Errorf("ParsePrefix(ms10te) err=%v, want nil (not determinable yet)", err)
	}

	// Non-bech32 char in the identifier ('b' is excluded from bech32).
	if _, err := ParsePrefix("ms12bbbb"); !errors.Is(err, errInvalidCharacter) {
		t.Errorf("ParsePrefix(ms12bbbb) err=%v, want errInvalidCharacter", err)
	}

	// Mixed case is determinable.
	if _, err := ParsePrefix("Ms10tests"); !errors.Is(err, errInvalidCase) {
		t.Errorf("ParsePrefix(Ms10tests) err=%v, want errInvalidCase", err)
	}

	// A full New-valid string parses cleanly with full fields.
	f, err = ParsePrefix("ms10testsxxxxxxxxxxxxxxxxxxxxxxxxxx4nzvca9cmczlw")
	if err != nil {
		t.Fatalf("ParsePrefix(full) err=%v", err)
	}
	if !f.Unshared || f.Identifier != "test" || f.HRP != "ms" {
		t.Errorf("ParsePrefix(full) = %+v", f)
	}

	// Uppercase (keypad-form) parses identically.
	f, err = ParsePrefix("MS12NAMEA320ZYXWVUTSRQPNMLKJHGFEDCAXRPP870HKKQRM")
	if err != nil {
		t.Fatalf("ParsePrefix(MS12NAME…) err=%v", err)
	}
	if f.Threshold != 2 || f.Identifier != "NAME" || f.ShareIndex != 'A' || f.Unshared {
		t.Errorf("ParsePrefix(MS12NAME…) = %+v", f)
	}
}
