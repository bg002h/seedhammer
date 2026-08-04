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
		// Pins the ORDERING when both faults apply. Without this row the
		// implementation can be rewritten to count length first and the whole
		// suite still passes -- verified by mutation in the Phase B review.
		{"too long AND non-ascii", long + "aé", ErrNonASCII},
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

func TestValidateFingerprint(t *testing.T) {
	tests := []struct {
		name, in, want string
		wantErr        error
	}{
		{"empty is allowed", "", "", nil},
		// All-whitespace is treated as absent, same as empty. Deliberate: the
		// field is optional and " " is not a fingerprint.
		{"all whitespace is absent", "   ", "", nil},
		{"lowercase", "a1b2c3d4", "A1B2C3D4", nil},
		{"uppercase", "A1B2C3D4", "A1B2C3D4", nil},
		{"grouped 4-4", "A1B2 C3D4", "A1B2C3D4", nil},
		{"odd spacing", " a1 b2c3 d4 ", "A1B2C3D4", nil},
		{"too short", "A1B2C3D", "", ErrBadFingerprint},
		{"too long", "A1B2C3D4E", "", ErrBadFingerprint},
		{"non-hex", "A1B2C3DG", "", ErrBadFingerprint},
		{"non-ascii", "A1B2C3Dé", "", ErrBadFingerprint},
	}
	for _, tc := range tests {
		got, err := ValidateFingerprint(tc.in)
		if !errors.Is(err, tc.wantErr) {
			t.Errorf("%s: err %v, want %v", tc.name, err, tc.wantErr)
			continue
		}
		if got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}

// The canonical form is what is stored and compared; grouping is presentation.
func TestFingerprintCanonicalIsStable(t *testing.T) {
	forms := []string{"a1b2c3d4", "A1B2C3D4", "A1B2 C3D4", "a1b2 c3d4", " A1B2C3D4 "}
	want := "A1B2C3D4"
	for _, f := range forms {
		got, err := ValidateFingerprint(f)
		if err != nil {
			t.Fatalf("%q: %v", f, err)
		}
		if got != want {
			t.Errorf("%q canonicalised to %q, want %q", f, got, want)
		}
	}
}

func TestGroupFingerprint(t *testing.T) {
	if got := GroupFingerprint("A1B2C3D4"); got != "A1B2 C3D4" {
		t.Errorf("got %q, want %q", got, "A1B2 C3D4")
	}
	if got := GroupFingerprint(""); got != "" {
		t.Errorf("empty should pass through, got %q", got)
	}
}
