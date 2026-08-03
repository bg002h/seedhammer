package constant

import (
	"testing"
)

// TestPrintableASCIICoverage asserts the engraving face can render every
// printable ASCII character. A BIP-39 passphrase is case-sensitive free text,
// so anything the face cannot decode either panics at engrave time
// (engrave/engrave.go:1365) or forces a refusal at entry.
func TestPrintableASCIICoverage(t *testing.T) {
	var missing []rune
	for r := rune(0x20); r <= 0x7E; r++ {
		if _, _, ok := Font.Decode(r); !ok {
			missing = append(missing, r)
		}
	}
	if len(missing) > 0 {
		t.Errorf("face cannot decode %d of 95 printable ASCII runes: %q",
			len(missing), string(missing))
	}
}

// TestSpaceMarkPresent guards the visible-space mark. TestPrintableASCIICoverage
// spans 0x20-0x7E only and the glyph-rule tests skip runes the face cannot
// decode, so a forgotten space_mark would otherwise surface as a construction
// panic in the passphrase ConstantStringer rather than here.
func TestSpaceMarkPresent(t *testing.T) {
	if _, _, ok := Font.Decode(0x1F); !ok {
		t.Fatal("visible-space mark (0x1F) missing from the face")
	}
}
