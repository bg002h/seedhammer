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

var _ = errors.Is // keep errors imported for Tasks 2-3 in this file
