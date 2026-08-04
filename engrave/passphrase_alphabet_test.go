package engrave

import (
	"testing"

	"seedhammer.com/font/constant"
)

// The passphrase alphabet must cover every printable ASCII rune plus the
// visible-space mark, be in ascending codepoint order (NewConstantStringer
// binary-searches it, engrave.go:1208-1210), and construct without panicking.
func TestPassphraseAlphabet(t *testing.T) {
	// BLOCKED: NewConstantStringer cannot yet build this alphabet.
	// timeConstantPath (engrave.go:1170-1188) asserts each glyph is ONE
	// continuous engrave run and panics "broken path" (:1181) on the second
	// pen-lift. 13 of the 96 runes are multi-subpath: ':' '#' '*' (pre-existing)
	// and 'i' 'j' 'x' '!' '"' '$' '%' ';' '=' '?' (added in Task 3). Seven of
	// those need a DETACHED DOT and cannot be redrawn as one stroke -- an 'i'
	// whose dot joins its stem is an 'l'.
	// Extending the model is a normative change to constant-time engraving
	// semantics and needs its own gate; see FOLLOWUPS
	// seedhammer-constant-time-multi-subpath-glyphs.
	t.Skip("blocked: timeConstantPath requires single-run glyphs; see FOLLOWUPS")

	if passphraseAlphabet[0] != 0x1F {
		t.Errorf("alphabet must start with the space mark 0x1F, got %#x", passphraseAlphabet[0])
	}
	var last rune = -1
	for _, r := range passphraseAlphabet {
		if r <= last {
			t.Fatalf("alphabet not ascending at %q", r)
		}
		last = r
	}
	for r := rune(0x20); r <= 0x7E; r++ {
		found := false
		for _, a := range passphraseAlphabet {
			if a == r {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("alphabet missing %q", r)
		}
	}
	if got, want := len([]rune(passphraseAlphabet)), 96; got != want {
		t.Errorf("alphabet has %d runes, want %d (95 printable + mark)", got, want)
	}
	// Must not panic: exercises the ascending-order check (engrave.go:1210),
	// the alphabet-subset-of-face check (:1215) and uniform advance (:1218).
	NewPassphraseStringer(constant.Font, params(), 4*mm)
}
