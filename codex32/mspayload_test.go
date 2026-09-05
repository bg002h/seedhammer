package codex32

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"os"
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

// hashlockCorpus is the vendored ms-codec 0.8.0 corpus, read from the hashlock
// package's own testdata by a path RELATIVE TO THIS PACKAGE (`go test` runs with
// codex32/ as its working directory and hashlock/ is a sibling). One vendored
// copy, one provenance pin (hashlock/testdata/hashlock-v0.8.provenance.json) --
// never a second copy or a literal transcribed into this file.
type hashlockCorpus struct {
	Kind []struct {
		PreimageHex   string `json:"preimage_hex"`
		Digest        string `json:"digest"`
		MS1           string `json:"ms1"`
		Entr32PairMS1 string `json:"entr32_pair_ms1"`
	} `json:"kind"`
	Derivation []struct {
		Phrase    string `json:"phrase"`
		HardenedX string `json:"hardened_x"`
	} `json:"derivation"`
}

func loadHashlockCorpus(t *testing.T) hashlockCorpus {
	t.Helper()
	raw, err := os.ReadFile("../hashlock/testdata/hashlock-v0.8.json")
	if err != nil {
		t.Fatalf("reading the vendored hashlock corpus: %v", err)
	}
	var c hashlockCorpus
	if err := json.Unmarshal(raw, &c); err != nil {
		t.Fatalf("parsing the vendored hashlock corpus: %v", err)
	}
	if len(c.Kind) < 1 || c.Kind[0].PreimageHex == "" || c.Kind[0].Entr32PairMS1 == "" {
		t.Fatalf("corpus shape: %d kind rows", len(c.Kind))
	}
	return c
}

// H2 (SPEC_hashlock_H2_device §6, §7.4): the 0x03 kind has ONE decoder of its own;
// DecodeMS1 keeps refusing it (H0), and the two never share a code path.
//
// Every value here comes from the corpus, never from a literal this file
// transcribed. MUTATION: `copy(preimage[:], d[:32])` in place of `d[1:]` -> the
// full-width comparison below fails with
// `preimage = 03abab...abab, want the corpus's preimage_hex abab...abab`. The `x[0] == 0 && x[31] == 0`
// smoke check this replaced could NOT fail on that mutation (r0 adversarial C-2:
// under it x[0] = 0x03 and x[31] = 0xab, so the && is false and the mutant
// reported PASS -- executed and confirmed for this fold).
func TestDecodeMS1PreimageIsShapeExact(t *testing.T) {
	c := loadHashlockCorpus(t)
	plate := c.Kind[0].MS1
	s, err := New(plate)
	if err != nil {
		t.Fatal(err)
	}
	x, err := DecodeMS1Preimage(s)
	if err != nil {
		t.Fatalf("DecodeMS1Preimage(plate): %v", err)
	}
	if want := mustHexT(t, c.Kind[0].PreimageHex); !bytes.Equal(x[:], want) {
		t.Fatalf("preimage = %x, want the corpus's preimage_hex %x", x, want)
	}
	if _, _, _, err := DecodeMS1(s); err != errMSBadPrefix {
		t.Errorf("DecodeMS1(plate) = %v, want errMSBadPrefix (H0 contract)", err)
	}

	// §7.4's acceptance-record case: the plate ms hashlock actually wrote on
	// the host (design/agent-reports/ms-hashlock-H1-acceptance.md, H1 item 3)
	// decodes to the corpus ANCHOR row's hardened_x. This is the one row that
	// ties this decoder to a host-produced artifact rather than to a corpus
	// string; a decoder that agreed with the corpus but not with ms would pass
	// every other row here.
	const acceptancePlate = "ms10hashsq0p7jaf9gsjjpkjvll2l274w8a388xgqzlewp73scptwxgtjugspvs8tklufg89hqj"
	ap, err := New(acceptancePlate)
	if err != nil {
		t.Fatalf("New(the H1 acceptance plate): %v", err)
	}
	ax, err := DecodeMS1Preimage(ap)
	if err != nil {
		t.Fatalf("DecodeMS1Preimage(the H1 acceptance plate): %v", err)
	}
	if len(c.Derivation) == 0 || c.Derivation[0].Phrase != "correct horse battery staple" {
		t.Fatal("derivation row 0 is not the anchor phrase -- the corpus and this test have drifted")
	}
	if want := mustHexT(t, c.Derivation[0].HardenedX); !bytes.Equal(ax[:], want) {
		t.Errorf("the H1 acceptance plate decodes to %x, want the anchor row's hardened_x %x", ax, want)
	}

	// §7.1's "kind: the entr32 pair" lockstep clause. The SAME 32 bytes under
	// Tag::ENTR are a SEED, not a preimage: the preimage decoder must refuse
	// the sibling on its prefix byte, and DecodeMS1 -- which refuses the hash
	// plate -- must decode it. That is the pair, driven in both directions.
	pair, err := New(c.Kind[0].Entr32PairMS1)
	if err != nil {
		t.Fatalf("New(entr32_pair_ms1): %v", err)
	}
	if _, err := DecodeMS1Preimage(pair); err != errMSBadPrefix {
		t.Errorf("DecodeMS1Preimage(entr32_pair_ms1) err = %v, want errMSBadPrefix", err)
	}
	prefix, lang, entropy, err := DecodeMS1(pair)
	if err != nil {
		t.Fatalf("DecodeMS1(entr32_pair_ms1): %v", err)
	}
	if prefix != msPrefixEntr || lang != 0 {
		t.Errorf("DecodeMS1(entr32_pair_ms1) prefix/language = %d/%d, want %d/0", prefix, lang, msPrefixEntr)
	}
	if want := mustHexT(t, c.Kind[0].PreimageHex); !bytes.Equal(entropy, want) {
		t.Errorf("entr32_pair_ms1 seed = %x, want the same 32 bytes as the hash plate %x", entropy, want)
	}
	for _, tc := range []struct {
		name, s string
		want    error
	}{
		{"entr single", "ms10entrsqqqqqqqqqqqqqqqqqqqqqqqqqqqqcj9sxraq34v7f", errMSBadPrefix},
		{"a 2-of-N share beginning 0x03", "ms12testaqv0qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqdq7pl8qdc5tsp", errMSBadPrefix},
		{"the entr-id 0x03 shape (kind is the prefix byte)", "ms10entrsqv0qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqq5gz69g08wwtz9", nil},
	} {
		e, err := New(tc.s)
		if err != nil {
			t.Fatalf("New(%s): %v", tc.name, err)
		}
		if _, err := DecodeMS1Preimage(e); err != tc.want {
			t.Errorf("DecodeMS1Preimage(%s) err = %v, want %v", tc.name, err, tc.want)
		}
	}
	// An unshared 0x03 string whose payload is not 33 bytes: the length rule.
	d17 := make([]byte, 17)
	d17[0] = 0x03
	short, err := NewSeed("ms", 0, "hash", 's', d17)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeMS1Preimage(short); err != errMSBadLength {
		t.Errorf("17-byte 0x03 payload: err = %v, want errMSBadLength", err)
	}
}
