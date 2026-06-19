package codex32

import (
	"bytes"
	"encoding/hex"
	"testing"
)

func mustHexT(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// Rust-sourced parity vectors: codex32.New(ms1).Seed() decoded via DecodeMS1
// must reproduce the known prefix/language/entropy byte-for-byte.
func TestDecodeMS1Parity(t *testing.T) {
	cases := []struct {
		name, ms1, entropyHex string
		wantPrefix, wantLang  int
	}{
		{"entr16-zero", "ms10entrsqqqqqqqqqqqqqqqqqqqqqqqqqqqqcj9sxraq34v7f", "00000000000000000000000000000000", 0x00, 0},
		{"entr20-nonzero", "ms10entrsqqqjx3t83x4ummcpydzk0zdtehhszg69vucrgd4pcjx3kkj", "0123456789abcdef0123456789abcdef01234567", 0x00, 0},
		{"entr32-zero", "ms10entrsqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqcwugpdxtfme2w", "0000000000000000000000000000000000000000000000000000000000000000", 0x00, 0},
		{"mnem-english16", "ms10entrsqgqqc83yukgh23xkvmp59xf2eldpk4cdrq2y4h82yz", "0c1e24e5917544d666c342992acfda1b", 0x02, 0},
		{"mnem-japanese16", "ms10entrsqgqsc83yukgh23xkvmp59xf2eldpkpefrcjje3drdq", "0c1e24e5917544d666c342992acfda1b", 0x02, 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s, err := New(c.ms1)
			if err != nil {
				t.Fatalf("New(%q): %v", c.ms1, err)
			}
			prefix, lang, entropy, err := DecodeMS1(s)
			if err != nil {
				t.Fatalf("DecodeMS1: %v", err)
			}
			if prefix != c.wantPrefix || lang != c.wantLang {
				t.Errorf("prefix=%#x lang=%d, want %#x/%d", prefix, lang, c.wantPrefix, c.wantLang)
			}
			if want := mustHexT(t, c.entropyHex); !bytes.Equal(entropy, want) {
				t.Errorf("entropy=%x, want %x", entropy, want)
			}
		})
	}
}

// Refusal: an unknown prefix byte or a non-BIP-39 entropy length → error, no panic.
func TestDecodeMS1Refusal(t *testing.T) {
	mk := func(data []byte) String {
		s, err := NewSeed("ms", 0, "entr", 's', data)
		if err != nil {
			t.Fatalf("NewSeed: %v", err)
		}
		return s
	}
	z16 := make([]byte, 16)
	// Unknown prefix 0x01 + 16B → errMSBadPrefix.
	if _, _, _, err := DecodeMS1(mk(append([]byte{0x01}, z16...))); err == nil {
		t.Error("unknown prefix accepted")
	}
	// entr prefix + 15B entropy (not in {16,20,24,28,32}) → errMSBadLength.
	if _, _, _, err := DecodeMS1(mk(append([]byte{0x00}, make([]byte, 15)...))); err == nil {
		t.Error("bad entropy length accepted")
	}
	// mnem prefix + language 10 (>9) → errMSBadLanguage.
	if _, _, _, err := DecodeMS1(mk(append([]byte{0x02, 10}, z16...))); err == nil {
		t.Error("invalid language accepted")
	}
}
