package passphrase_test

import (
	"testing"

	"seedhammer.com/font/constant"
	"seedhammer.com/passphrase"
)

// For EVERY rune ValidatePassphrase accepts, the engraving face must be able to
// decode it. A rune that validates but cannot be decoded panics at engrave time
// -- mid-plate, on a permanent medium.
//
// (The keyboard half of the three-way check lives in gui, which cannot be
// imported here without a cycle; see gui.TestKeyboardCoversPrintableASCII.)
func TestValidatorAgreesWithFace(t *testing.T) {
	for r := rune(0x20); r <= 0x7E; r++ {
		accepted := passphrase.ValidatePassphrase(string(r)) == nil
		_, _, decodable := constant.Font.Decode(r)
		if accepted != decodable {
			t.Errorf("rune %q: validator accepts=%v, face decodes=%v", r, accepted, decodable)
		}
	}
}

// And nothing outside printable ASCII is accepted, so the loop above is the
// whole domain.
func TestValidatorRejectsEverythingElse(t *testing.T) {
	for _, r := range []rune{0x00, 0x09, 0x0A, 0x1F, 0x7F, 0x80, 0xE9, 0x65E5} {
		if err := passphrase.ValidatePassphrase(string(r)); err == nil {
			t.Errorf("rune %#x was accepted", r)
		}
	}
	// 0x1F in particular: it is the visible-space mark's codepoint. A validated
	// passphrase must never contain it, or it would collide with the
	// substitution in spec 3.3.
}
