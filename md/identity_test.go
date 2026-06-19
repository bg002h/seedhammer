package md

import "testing"

// TestDeriveChunkSetID: top-20-MSB-first extraction (chunk.rs:199-207). bytes
// AB CD EF... → 0xABCDE.
func TestDeriveChunkSetID(t *testing.T) {
	var id [16]byte
	id[0] = 0xAB
	id[1] = 0xCD
	id[2] = 0xEF
	if got := deriveChunkSetID(id); got != 0xABCDE {
		t.Fatalf("deriveChunkSetID = %#x, want 0xABCDE", got)
	}
	// Deterministic.
	if deriveChunkSetID(id) != deriveChunkSetID(id) {
		t.Fatal("deriveChunkSetID not deterministic")
	}
	// Range bound: result < 2^20.
	var maxID [16]byte
	maxID[0], maxID[1], maxID[2] = 0xFF, 0xFF, 0xFF
	if got := deriveChunkSetID(maxID); got >= (1 << 20) {
		t.Fatalf("deriveChunkSetID = %#x exceeds 20 bits", got)
	}
}

// TestComputeEncodingIDDeterministicAndPathSensitive (identity.rs:301-322):
// computeEncodingID is deterministic for a fixed descriptor, and two
// descriptors differing only in a derivation path yield different ids.
func TestComputeEncodingIDDeterministicAndPathSensitive(t *testing.T) {
	d := loadDescriptor(t, "wsh_with_fingerprints")
	a, err := computeEncodingID(d)
	if err != nil {
		t.Fatalf("computeEncodingID: %v", err)
	}
	b, err := computeEncodingID(d)
	if err != nil {
		t.Fatalf("computeEncodingID(2): %v", err)
	}
	if a != b {
		t.Fatalf("computeEncodingID not deterministic: %x vs %x", a, b)
	}

	// Path-sensitive: a descriptor with a divergent path decl vs a shared one
	// (same tree/keys) must produce a different id.
	base := &descriptor{
		n:        2,
		pathDecl: pathDecl{n: 2, shared: &originPath{}},
		useSite:  useSitePath{hasMultipath: true, multipath: []alternative{{value: 0}, {value: 1}}},
		tree: node{tag: tagWsh, body: childrenBody{children: []node{{
			tag: tagMulti, body: multiKeysBody{k: 2, indices: []uint8{0, 1}},
		}}}},
	}
	withPath := &descriptor{
		n: 2,
		pathDecl: pathDecl{n: 2, divergent: []originPath{
			{components: []pathComponent{{hardened: true, value: 84}, {hardened: true, value: 0}, {hardened: true, value: 0}}},
			{components: []pathComponent{{hardened: true, value: 48}, {hardened: true, value: 0}, {hardened: true, value: 0}}},
		}},
		useSite: useSitePath{hasMultipath: true, multipath: []alternative{{value: 0}, {value: 1}}},
		tree: node{tag: tagWsh, body: childrenBody{children: []node{{
			tag: tagMulti, body: multiKeysBody{k: 2, indices: []uint8{0, 1}},
		}}}},
	}
	ia, err := computeEncodingID(base)
	if err != nil {
		t.Fatalf("computeEncodingID(base): %v", err)
	}
	ib, err := computeEncodingID(withPath)
	if err != nil {
		t.Fatalf("computeEncodingID(withPath): %v", err)
	}
	if ia == ib {
		t.Fatal("computeEncodingID not path-sensitive: identical ids for different paths")
	}
}

// TestComputeEncodingIDIsSHA256Prefix: the id equals SHA-256(encodePayload)[0:16].
func TestComputeEncodingIDIsSHA256Prefix(t *testing.T) {
	d := loadDescriptor(t, "wpkh_basic")
	id, err := computeEncodingID(d)
	if err != nil {
		t.Fatalf("computeEncodingID: %v", err)
	}
	bytesOut, _, err := encodePayload(d)
	if err != nil {
		t.Fatalf("encodePayload: %v", err)
	}
	want := sha256First16(bytesOut)
	if id != want {
		t.Fatalf("computeEncodingID = %x, want SHA-256(payload)[0:16] = %x", id, want)
	}
}
