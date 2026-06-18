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
