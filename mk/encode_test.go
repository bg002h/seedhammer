package mk

import (
	"testing"

	"github.com/btcsuite/btcd/btcutil/v2/hdkeychain"
	"github.com/btcsuite/btcd/chaincfg/v2"
	"seedhammer.com/bip32"
	"seedhammer.com/bip39"
	"seedhammer.com/codex32"
)

// deriveTestXpub derives an account xpub at path from a fixed, checksum-valid
// mnemonic (mirrors fillDescriptor's compose template). Deterministic.
func deriveTestXpub(t *testing.T, path string, net *chaincfg.Params) string {
	t.Helper()
	m := make(bip39.Mnemonic, 12)
	for j := range m {
		m[j] = bip39.Word(j)
	}
	m = m.FixChecksum()
	seed := bip39.MnemonicSeed(m, "")
	mk, err := hdkeychain.NewMaster(seed, net)
	if err != nil {
		t.Fatal(err)
	}
	p, err := bip32.ParsePath(path)
	if err != nil {
		t.Fatal(err)
	}
	acct, err := bip32.Derive(mk, p)
	if err != nil {
		t.Fatal(err)
	}
	return acct.String()
}

// cardsEqual compares two Cards field by field (Card has a slice field, so it
// is not directly comparable with ==).
func cardsEqual(a, b Card) bool {
	if a.Network != b.Network || a.Path != b.Path || a.Fingerprint != b.Fingerprint || a.Xpub != b.Xpub {
		return false
	}
	if len(a.Stubs) != len(b.Stubs) {
		return false
	}
	for i := range a.Stubs {
		if a.Stubs[i] != b.Stubs[i] {
			return false
		}
	}
	return true
}

func TestEncodeRoundTrip(t *testing.T) {
	cases := []struct {
		path   string
		net    string
		params *chaincfg.Params
	}{
		// Paths are authored in the "h" hardened form to match the decoder's
		// display output (t4-M1); Encode re-parses both "h" and "'" via
		// bip32.ParsePath, so the round-trip input==output comparison holds.
		{"m/84h/0h/0h", "mainnet", &chaincfg.MainNetParams},
		{"m/44h/0h/0h", "mainnet", &chaincfg.MainNetParams},
		{"m/49h/0h/0h", "mainnet", &chaincfg.MainNetParams},
		{"m/86h/0h/0h", "mainnet", &chaincfg.MainNetParams},
		{"m/48h/0h/0h/2h", "mainnet", &chaincfg.MainNetParams},
		{"m/48h/0h/0h/1h", "mainnet", &chaincfg.MainNetParams},
		{"m/87h/0h/0h", "mainnet", &chaincfg.MainNetParams},
		{"m/84h/1h/0h", "testnet", &chaincfg.TestNet3Params},
		{"m/48h/1h/0h/2h", "testnet", &chaincfg.TestNet3Params},
	}
	for _, c := range cases {
		t.Run(c.path, func(t *testing.T) {
			xpub := deriveTestXpub(t, c.path, c.params)
			card := Card{
				Network:     c.net,
				Path:        c.path,
				Fingerprint: "",
				Stubs:       [][4]byte{{0, 0, 0, 0}},
				Xpub:        xpub,
			}
			strs, err := Encode(card)
			if err != nil {
				t.Fatalf("Encode: %v", err)
			}
			if len(strs) < 2 {
				t.Fatalf("expected >=2 chunks, got %d", len(strs))
			}
			for i, s := range strs {
				if !codex32.ValidMK(s) {
					t.Fatalf("chunk %d fails ValidMK: %s", i, s)
				}
			}
			got, err := Decode(strs)
			if err != nil {
				t.Fatalf("Decode(Encode): %v", err)
			}
			// Decode reconstructs Path with the "h" hardened form and Network from
			// the xpub version, so the round-trip Card should equal the input Card.
			if !cardsEqual(got, card) {
				t.Fatalf("round-trip mismatch:\n got %+v\nwant %+v", got, card)
			}
			// Determinism: identical strings across runs.
			strs2, err := Encode(card)
			if err != nil {
				t.Fatalf("Encode (2nd): %v", err)
			}
			if len(strs2) != len(strs) {
				t.Fatalf("non-deterministic chunk count: %d vs %d", len(strs2), len(strs))
			}
			for i := range strs {
				if strs2[i] != strs[i] {
					t.Fatalf("non-deterministic at chunk %d:\n %s\n %s", i, strs[i], strs2[i])
				}
			}
		})
	}
}

func TestEncodeWithFingerprint(t *testing.T) {
	xpub := deriveTestXpub(t, "m/84'/0'/0'", &chaincfg.MainNetParams)
	card := Card{
		Network:     "mainnet",
		Path:        "m/84h/0h/0h", // decoder displays the "h" hardened form (t4-M1)
		Fingerprint: "deadbeef",
		Stubs:       [][4]byte{{0xc0, 0xff, 0xee, 0x00}},
		Xpub:        xpub,
	}
	strs, err := Encode(card)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	for i, s := range strs {
		if !codex32.ValidMK(s) {
			t.Fatalf("chunk %d fails ValidMK: %s", i, s)
		}
	}
	got, err := Decode(strs)
	if err != nil {
		t.Fatalf("Decode(Encode): %v", err)
	}
	if !cardsEqual(got, card) {
		t.Fatalf("round-trip mismatch:\n got %+v\nwant %+v", got, card)
	}
}

func TestEncodeStubZero(t *testing.T) {
	xpub := deriveTestXpub(t, "m/84'/0'/0'", &chaincfg.MainNetParams)
	card := Card{
		Network: "mainnet",
		Path:    "m/84'/0'/0'",
		Stubs:   [][4]byte{{0, 0, 0, 0}},
		Xpub:    xpub,
	}
	strs, err := Encode(card)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	got, err := Decode(strs)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if len(got.Stubs) != 1 || got.Stubs[0] != [4]byte{0, 0, 0, 0} {
		t.Fatalf("stub-0: got stubs %v", got.Stubs)
	}
}

func TestEncodeRejectsBadInput(t *testing.T) {
	xpub := deriveTestXpub(t, "m/84'/0'/0'", &chaincfg.MainNetParams)
	// Depth/child invariant: path component count must equal xpub depth and the
	// terminal component must equal the xpub's child number.
	mismatched := Card{
		Network: "mainnet",
		Path:    "m/84'/0'", // depth 2, but the xpub is depth 3
		Stubs:   [][4]byte{{0, 0, 0, 0}},
		Xpub:    xpub,
	}
	if _, err := Encode(mismatched); err == nil {
		t.Fatal("Encode: want error for depth/child mismatch, got nil")
	}
	// Empty stubs are rejected (stub_count must be >= 1).
	noStubs := Card{
		Network: "mainnet",
		Path:    "m/84'/0'/0'",
		Stubs:   nil,
		Xpub:    xpub,
	}
	if _, err := Encode(noStubs); err == nil {
		t.Fatal("Encode: want error for empty stubs, got nil")
	}
	// Invalid xpub string.
	badXpub := Card{
		Network: "mainnet",
		Path:    "m/84'/0'/0'",
		Stubs:   [][4]byte{{0, 0, 0, 0}},
		Xpub:    "not-an-xpub",
	}
	if _, err := Encode(badXpub); err == nil {
		t.Fatal("Encode: want error for invalid xpub, got nil")
	}
}

// TestEncodeGoldenRoundTrip gates golden parity on decode -> re-encode ->
// re-decode (NOT byte-equality): the mk_test.go golden vectors use explicit
// chunk_set_ids, not a SHA-derived csid, so byte-identical re-emission is
// impossible (and the decoder does not validate the csid value). For each
// golden string set: c1 := Decode(golden); strs := Encode(c1); each chunk must
// pass ValidMK; c2 := Decode(strs); assert c1 == c2.
// TestEncodeChunksGuard exercises the defensive chunk-count guard in
// encodeChunks directly (real cards never approach maxChunks, so the guard is
// unreachable through Encode). At the maxChunks boundary the masked total-1 /
// index symbols must not wrap; one chunk over the limit must surface an error.
func TestEncodeChunksGuard(t *testing.T) {
	// chunkedFragmentBytes*maxChunks bytes of stream -> exactly maxChunks frags.
	// encodeChunks appends crossChunkHashBytes internally, so size the bytecode
	// so total stream == chunkedFragmentBytes*maxChunks after the hash append.
	atLimitBytecode := make([]byte, chunkedFragmentBytes*maxChunks-crossChunkHashBytes)
	strs, err := encodeChunks(atLimitBytecode)
	if err != nil {
		t.Fatalf("encodeChunks at maxChunks=%d: unexpected error: %v", maxChunks, err)
	}
	if len(strs) != maxChunks {
		t.Fatalf("encodeChunks at limit: got %d chunks, want %d", len(strs), maxChunks)
	}
	// The last chunk's header must report total-1 = maxChunks-1 and index =
	// maxChunks-1 without wrapping (would be visible as a malformed/short read).
	last := strs[len(strs)-1]
	h, err := ParseHeader(last)
	if err != nil {
		t.Fatalf("ParseHeader(last chunk): %v", err)
	}
	if h.TotalChunks != maxChunks || h.ChunkIndex != maxChunks-1 {
		t.Fatalf("boundary header wrapped: total=%d index=%d, want total=%d index=%d",
			h.TotalChunks, h.ChunkIndex, maxChunks, maxChunks-1)
	}

	// One fragment over the limit must error rather than silently wrap.
	overBytecode := make([]byte, chunkedFragmentBytes*maxChunks+1-crossChunkHashBytes)
	if _, err := encodeChunks(overBytecode); err == nil {
		t.Fatalf("encodeChunks over maxChunks: expected error, got nil")
	}
}

func TestEncodeGoldenRoundTrip(t *testing.T) {
	for _, v := range parityVectors {
		t.Run(v.name, func(t *testing.T) {
			c1, err := Decode(v.strings)
			if err != nil {
				t.Fatalf("Decode(golden): %v", err)
			}
			strs, err := Encode(c1)
			if err != nil {
				t.Fatalf("Encode(c1): %v", err)
			}
			for i, s := range strs {
				if !codex32.ValidMK(s) {
					t.Fatalf("chunk %d fails ValidMK: %s", i, s)
				}
			}
			c2, err := Decode(strs)
			if err != nil {
				t.Fatalf("Decode(Encode(c1)): %v", err)
			}
			if !cardsEqual(c1, c2) {
				t.Fatalf("golden round-trip mismatch:\n c1 %+v\n c2 %+v", c1, c2)
			}
		})
	}
}
