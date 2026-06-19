package md

import (
	"testing"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
)

// validPubkey0 is a known-good 33-byte compressed secp256k1 point (sourced from
// the gui descriptor goldens; verified on-curve via secp256k1.ParsePubKey).
var validPubkey0 = []byte{0x3, 0xa9, 0x39, 0x4a, 0x2f, 0x1a, 0x4f, 0x99, 0x61, 0x3a, 0x71, 0x69, 0x56, 0xc8, 0x54, 0xf, 0x6d, 0xba, 0x6f, 0x18, 0x93, 0x1c, 0x26, 0x39, 0x10, 0x72, 0x21, 0xb2, 0x67, 0xd7, 0x40, 0xaf, 0x23}

// xpubPayload builds a 65-byte Pubkeys TLV payload (32B chain code ‖ 33B
// compressed pubkey) from the given pubkey bytes; the chain code is filled with
// a fixed non-zero pattern (any 32 bytes are a structurally valid chain code).
func xpubPayload(pub []byte) [65]byte {
	var xpub [65]byte
	for i := 0; i < 32; i++ {
		xpub[i] = byte(0x10 + i)
	}
	copy(xpub[32:65], pub)
	return xpub
}

// singlesigWithPubkey builds a minimal n=1 wpkh(@0) descriptor carrying a
// single-entry Pubkeys TLV with the given 65-byte xpub. The wpkh canonical
// origin (m/84'/0'/0') satisfies the explicit-origin gate, so the only
// decode-side validator that can reject is validateXpubBytes.
func singlesigWithPubkey(xpub [65]byte) *descriptor {
	return &descriptor{
		n:        1,
		pathDecl: pathDecl{n: 1, shared: &originPath{}},
		useSite:  useSitePath{hasMultipath: true, multipath: []alternative{{value: 0}, {value: 1}}},
		tree:     node{tag: tagWpkh, body: keyArgBody{index: 0}},
		tlv: tlvSection{
			pubPresent: true,
			pubkeys:    []idxPub{{idx: 0, xpub: xpub}},
		},
	}
}

// TestValidateXpubBytesValid: a Pubkeys TLV whose 33-byte field is a valid
// secp256k1 point decodes without error (Rust validate.rs:216-226 → Ok).
func TestValidateXpubBytesValid(t *testing.T) {
	if _, err := secp256k1.ParsePubKey(validPubkey0); err != nil {
		t.Fatalf("test premise broken: validPubkey0 is not on-curve: %v", err)
	}
	d := singlesigWithPubkey(xpubPayload(validPubkey0))
	b, bitLen, err := encodePayload(d)
	if err != nil {
		t.Fatalf("encodePayload: %v", err)
	}
	if _, err := decodePayloadValidated(b, bitLen); err != nil {
		t.Fatalf("decode of valid-pubkey descriptor: %v", err)
	}
}

// TestEncodePayloadPathDeclNMismatch (#10a-M1): a defensively-malformed
// author-built AST whose pathDecl.n disagrees with descriptor.n is rejected by
// encodePayload before the key-index-width (kiw) is computed off the wrong n.
// canonicalize keeps the two in lockstep for any decoded descriptor, so this
// guard is unreachable on the public-API path; it hardens future direct callers.
func TestEncodePayloadPathDeclNMismatch(t *testing.T) {
	d := singlesigWithPubkey(xpubPayload(validPubkey0))
	// Tree/key count is n=1, but claim a 2-path shared decl: kiw would diverge.
	d.pathDecl.n = 2
	if _, _, err := encodePayload(d); err != errPathDeclNMismatch {
		t.Fatalf("encodePayload with n mismatch = %v, want errPathDeclNMismatch", err)
	}
}

// TestValidateXpubBytesOffCurve: a Pubkeys TLV whose 33-byte field is NOT a
// valid secp256k1 point is rejected with errInvalidXpubBytes (D4; Rust
// validate.rs:221 PublicKey::from_slice → InvalidXpubBytes). The chain-code
// prefix is intentionally unvalidated.
func TestValidateXpubBytesOffCurve(t *testing.T) {
	// 0x02 || 32 zero bytes is a structurally-valid compressed-pubkey prefix but
	// (x=0) is not an x-coordinate of any secp256k1 point → off-curve.
	bad := make([]byte, 33)
	bad[0] = 0x02
	if _, err := secp256k1.ParsePubKey(bad); err == nil {
		t.Fatal("test premise broken: bad pubkey unexpectedly parsed on-curve")
	}
	d := singlesigWithPubkey(xpubPayload(bad))
	b, bitLen, err := encodePayload(d)
	if err != nil {
		t.Fatalf("encodePayload (encode must not reject off-curve, only decode validates): %v", err)
	}
	if _, err := decodePayloadValidated(b, bitLen); err != errInvalidXpubBytes {
		t.Fatalf("decode of off-curve-pubkey descriptor = %v, want errInvalidXpubBytes", err)
	}
}
