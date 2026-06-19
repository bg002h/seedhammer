package gui

import (
	"testing"

	"seedhammer.com/codex32"
	"seedhammer.com/md"
	"seedhammer.com/mk"
)

// T5 bundle test fixtures (R0-M1). The bundle gatherer accumulates MULTIPLE
// DISTINCT cards, so the tests need complete, distinct-chunk-set-id chunk sets
// assembled from REACHABLE sources. The md `split` helper is unexported and
// cross-package-unreachable from gui, so we do NOT rely on it:
//
//   - mk1 (two distinct cards): mk.Encode is fully reachable. Two distinct csids
//     are minted by varying the Fingerprint between the two cards (NOT the path —
//     the depth/child invariant constrains the path). mk.Encode is deterministic,
//     so the csid is a pure function of the card bytecode.
//   - md1 (two distinct cards): the two chunked-md1 sets already reachable from
//     gui tests — wshSortedmultiChunks (csid 0x2d950, embedded in
//     gui/md1_gather_test.go) and wsh_multi_chunked (csid 0x157ae, via
//     loadChunkedVectorString).

const (
	// A valid account xpub whose encoded depth/child matches m/84'/0'/0'
	// (gui/derive_test.go). mk.Encode validates this depth/child invariant, so
	// distinct cards are minted by varying the Fingerprint (NOT the xpub or the
	// path) — the Fingerprint feeds the bytecode, so the derived chunk_set_id
	// differs.
	bundleXpubA = "xpub6CatWdiZiodmUeTDp8LT5or8nmbKNcuyvz7WyksVFkKB4RHwCD3XyuvPEbvqAQY3rAPshWcMLoP2fMFMKHPJ4ZeZXYVUhLv1VMrjPC7PW6V"
)

// mk1CardA returns a complete, BCH-valid, integrity-verified chunked mk1 set
// (>=2 chunks, csid distinct from mk1CardB). It is the "first key card".
func mk1CardA(t *testing.T) []string {
	t.Helper()
	return encodeMK1Card(t, mk.Card{
		Network:     "mainnet",
		Path:        "m/84'/0'/0'",
		Fingerprint: "73c5da0a",
		Stubs:       [][4]byte{{0, 0, 0, 0}},
		Xpub:        bundleXpubA,
	})
}

// mk1CardB returns a SECOND complete mk1 set with a DISTINCT csid (different
// Fingerprint → different bytecode → different derived chunk_set_id).
func mk1CardB(t *testing.T) []string {
	t.Helper()
	return encodeMK1Card(t, mk.Card{
		Network:     "mainnet",
		Path:        "m/84'/0'/0'",
		Fingerprint: "1a2b3c4d",
		Stubs:       [][4]byte{{0, 0, 0, 0}},
		Xpub:        bundleXpubA,
	})
}

func encodeMK1Card(t *testing.T, card mk.Card) []string {
	t.Helper()
	strs, err := mk.Encode(card)
	if err != nil {
		t.Fatalf("mk.Encode: %v", err)
	}
	if len(strs) < 2 {
		t.Fatalf("mk1 card must be >=2 chunks, got %d", len(strs))
	}
	for i, s := range strs {
		if !codex32.ValidMK(s) {
			t.Fatalf("chunk %d not ValidMK: %s", i, s)
		}
	}
	if _, err := mk.Decode(strs); err != nil {
		t.Fatalf("mk.Decode round-trip: %v", err)
	}
	return strs
}

// md1CardA returns the wshSortedmultiChunks set (csid 0x2d950): a complete,
// integrity-verified chunked md1 descriptor set with real xpubs.
func md1CardA(t *testing.T) []string {
	t.Helper()
	if _, err := md.DecodeChunks(wshSortedmultiChunks); err != nil {
		t.Fatalf("md.DecodeChunks(wshSortedmultiChunks): %v", err)
	}
	out := make([]string, len(wshSortedmultiChunks))
	copy(out, wshSortedmultiChunks)
	return out
}

// md1CardB returns the wsh_multi_chunked set (csid 0x157ae): a DISTINCT chunked
// md1 set (single chunk, no pubkeys) — distinct csid from md1CardA.
func md1CardB(t *testing.T) []string {
	t.Helper()
	s := loadChunkedVectorString(t, "wsh_multi_chunked")
	strs := []string{s}
	if _, err := md.DecodeChunks(strs); err != nil {
		t.Fatalf("md.DecodeChunks(wsh_multi_chunked): %v", err)
	}
	return strs
}

// mkCSID parses the chunk_set_id off an mk1 chunk string (test convenience).
func mkCSID(t *testing.T, s string) uint32 {
	t.Helper()
	h, err := mk.ParseHeader(s)
	if err != nil {
		t.Fatalf("mk.ParseHeader: %v", err)
	}
	return h.ChunkSetID
}

// mdCSID parses the chunk_set_id off an md1 chunk string (test convenience).
func mdCSID(t *testing.T, s string) uint32 {
	t.Helper()
	h, err := md.ParseChunkHeader(s)
	if err != nil {
		t.Fatalf("md.ParseChunkHeader: %v", err)
	}
	return h.ChunkSetID
}

// TestBundleFixturesDistinct asserts the four card fixtures are complete,
// integrity-verified, and pairwise distinct in chunk_set_id (so they accumulate
// as distinct cards rather than colliding).
func TestBundleFixturesDistinct(t *testing.T) {
	mkA, mkB := mk1CardA(t), mk1CardB(t)
	mdA, mdB := md1CardA(t), md1CardB(t)

	csidMKA, csidMKB := mkCSID(t, mkA[0]), mkCSID(t, mkB[0])
	if csidMKA == csidMKB {
		t.Fatalf("mk1 card csids collide: %#x", csidMKA)
	}
	csidMDA, csidMDB := mdCSID(t, mdA[0]), mdCSID(t, mdB[0])
	if csidMDA == csidMDB {
		t.Fatalf("md1 card csids collide: %#x", csidMDA)
	}
	// Documented expected csids (recon-refresh): md1CardA=0x2d950, md1CardB=0x157ae.
	if csidMDA != 0x2d950 {
		t.Fatalf("md1CardA csid = %#x, want 0x2d950", csidMDA)
	}
	if csidMDB != 0x157ae {
		t.Fatalf("md1CardB csid = %#x, want 0x157ae", csidMDB)
	}
}
