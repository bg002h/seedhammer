package sh

import "testing"

// Every printable ASCII rune must decode, because engrave.String panics on a
// rune the face lacks and the free-text keyboard can emit all 95.
func TestSHCoversPrintableASCII(t *testing.T) {
	var missing []rune
	for r := rune(0x20); r <= 0x7E; r++ {
		if r == ' ' {
			continue // space inks nothing; advance-only
		}
		if _, _, ok := Font.Decode(r); !ok {
			missing = append(missing, r)
		}
	}
	if len(missing) > 0 {
		t.Fatalf("font/sh is missing %d printable ASCII glyphs: %q", len(missing), missing)
	}
}
