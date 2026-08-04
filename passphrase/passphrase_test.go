package passphrase

import (
	"errors"
	"strings"
	"testing"
)

func TestValidatePassphrase(t *testing.T) {
	long := strings.Repeat("a", 100)
	tests := []struct {
		name string
		in   string
		want error
	}{
		{"empty", "", ErrEmpty},
		{"single", "a", nil},
		{"exactly 100", long, nil},
		{"101 chars", long + "a", ErrTooLong},
		{"space is legal", "correct horse", nil},
		{"leading space", " x", nil},
		{"trailing space", "x ", nil},
		{"all printable ascii", allASCII(), nil},
		{"non-ascii accent", "café", ErrNonASCII},
		{"non-ascii cjk", "日本", ErrNonASCII},
		{"emoji", "a\U0001F600", ErrNonASCII},
		{"invalid utf8", "a\xff", ErrNonASCII},
		{"tab", "a\tb", ErrNonASCII},
		{"newline", "a\nb", ErrNonASCII},
		{"del", "a\x7f", ErrNonASCII},
		{"nul", "a\x00", ErrNonASCII},
	}
	for _, tc := range tests {
		got := ValidatePassphrase(tc.in)
		if !errors.Is(got, tc.want) {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}

// allASCII returns every printable ASCII rune once.
func allASCII() string {
	var b strings.Builder
	for r := rune(0x20); r <= 0x7E; r++ {
		b.WriteRune(r)
	}
	return b.String()
}

// An error must never quote the passphrase back.
func TestErrorsDoNotLeakContent(t *testing.T) {
	const secret = "hunter2é"
	err := ValidatePassphrase(secret)
	if err == nil {
		t.Fatal("expected an error")
	}
	if strings.Contains(err.Error(), "hunter2") {
		t.Errorf("error leaks passphrase content: %q", err.Error())
	}
}
