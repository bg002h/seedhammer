package bip32

import "testing"

// TestFixShBip32UnhardenedPathElementOutOfRangeText pins the refusal text of an
// element of 2^31 or more to `bip32: path element out of range: %q` on every
// word size. With strconv.ParseInt bitSize 0 the 32-bit device (and GOARCH=386)
// gave `bip32: invalid path element` instead (r1b M1).
func TestFixShBip32UnhardenedPathElementOutOfRangeText(t *testing.T) {
	for _, in := range []string{"2147483648", "4294967295", "4294967296"} {
		_, err := ParsePathElement(in)
		want := `bip32: path element out of range: "` + in + `"`
		if err == nil || err.Error() != want {
			t.Errorf("ParsePathElement(%q) error = %v, want %q", in, err, want)
		}
	}
	_, err := ParsePathElement("2147483648h")
	if want := `bip32: path element out of range: "2147483648"`; err == nil || err.Error() != want {
		t.Errorf("ParsePathElement(2147483648h) error = %v, want %q", err, want)
	}
}
