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

// H0: the preimage kind is recognised by its shape and prefix byte and by
// nothing else, and DecodeMS1 keeps refusing it (the seed decoder must not
// learn a kind that is not a seed).
func TestIsPreimageReadsThePrefixByteOnly(t *testing.T) {
	const plate = "ms10hashsqw46h2at4w46h2at4w46h2at4w46h2at4w46h2at4w46h2at4w46kzv2ncy60u7z9c"
	s, err := New(plate)
	if err != nil {
		t.Fatalf("New(plate): %v", err)
	}
	if !IsPreimage(s) {
		t.Fatalf("IsPreimage(plate) = false, want true (Seed()[0] = %#x)", s.Seed()[0])
	}
	if id, _, _ := s.Split(); id != "hash" {
		t.Errorf("id = %q, want hash", id)
	}
	// Every population the predicate must NOT touch, and the one it must.
	// MUTATIONS, each measured against exactly one row: dropping `!f.Unshared`
	// calls the share row a preimage; dropping `len(d) == 33` calls the plain
	// 16-byte BIP-93 row one (the 16-byte row is a control against an
	// OVER-WIDE predicate -- it is not evidence that a 33-byte plain seed
	// cannot collide, and one can: see the seam row
	// bip93-plain-33-byte-payload-0x03, refused on both sides by design,
	// post-impl review I-1); `d[0] != msPrefixEntr` in place of
	// `== msPrefixPreimage` calls the 33-byte 0x31 row one; keying on the id
	// `hash` instead of the prefix misses the entr-id row. The mnem row is
	// 17 bytes and is refused by the length test alone. Seam-corpus rows
	// where one exists (sysw/testdata/codex32_seam_vectors.json); the 0x31
	// row is codex32.NewSeed("ms", 0, "test", 's', 33 bytes beginning 0x31).
	for _, c := range []struct {
		name, s string
		want    bool
	}{
		{"constellation-entr-128 (prefix 0x00)", "ms10entrsqqqqqqqqqqqqqqqqqqqqqqqqqqqqcj9sxraq34v7f", false},
		{"mnem-english16 (prefix 0x02)", "ms10entrsqgqqc83yukgh23xkvmp59xf2eldpk4cdrq2y4h82yz", false},
		{"bip93-plain-payload-0x03 (16-byte seed beginning 0x03)", "ms10testsqv0qqqqqqqqqqqqqqqqqqqqqqq8mzk8tjfdnjn5", false},
		{"bip93-share-payload-0x03 (a 2-of-N share beginning 0x03)", "ms12testaqv0qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqdq7pl8qdc5tsp", false},
		{"bip93-plain-33-byte-payload-0x31 (unshared, 33 bytes, first byte 0x31)", "ms10testsxy0qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqq5dayejmh0wrfk", false},
		{"preimage-shape-entr-id (unshared, 33 bytes, 0x03, id entr)", "ms10entrsqv0qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqq5gz69g08wwtz9", true},
	} {
		e, err := New(c.s)
		if err != nil {
			t.Fatalf("New(%s): %v", c.name, err)
		}
		if got := IsPreimage(e); got != c.want {
			t.Errorf("IsPreimage(%s) = %v, want %v", c.name, got, c.want)
		}
	}
	if _, _, _, err := DecodeMS1(s); err != errMSBadPrefix {
		t.Errorf("DecodeMS1(plate) err = %v, want errMSBadPrefix: the seed decoder must not decode a preimage", err)
	}
}
