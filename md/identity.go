package md

import "crypto/sha256"

// ─── Identity (port of identity.rs:39-45 + chunk.rs:175-179). ────────────────

// computeEncodingID is the 128-bit canonical identifier for an md1 encoding:
// the first 16 bytes of SHA-256 over the canonical bit-packed payload bytes
// (identity.rs:39-45). It transitively canonicalizes (encodePayload runs
// canonicalize), so a non-canonical input still yields the canonical id.
func computeEncodingID(d *descriptor) ([16]byte, error) {
	bytesOut, _, err := encodePayload(d)
	if err != nil {
		return [16]byte{}, err
	}
	return sha256First16(bytesOut), nil
}

// sha256First16 returns SHA-256(b)[0:16].
func sha256First16(b []byte) [16]byte {
	h := sha256.Sum256(b)
	var id [16]byte
	copy(id[:], h[:16])
	return id
}

// deriveChunkSetID derives the 20-bit chunk-set-id from a 16-byte encoding id by
// taking its top 20 bits MSB-first (chunk.rs:175-179). The OR of the three
// byte-bounded shifts is already ≤20 bits, so no mask is needed (M-7: matches
// Rust, which has no & 0xFFFFF). Result is in 0..=0xFFFFF.
func deriveChunkSetID(id [16]byte) uint32 {
	return (uint32(id[0]) << 12) | (uint32(id[1]) << 4) | (uint32(id[2]) >> 4)
}
