package md

import "crypto/sha256"

// Liana's unspendable internal key (F-449, SPEC_liana_unspendable_internal_key
// §2). Port of md-codec nums.rs liana_unspendable_xpub + to_miniscript.rs
// collect_leaf_pubkeys, md-codec 0.47.0.
//
// NETWORK-FREE BY DESIGN. md carries key material as 65 bytes (chain code ‖
// compressed pubkey, the Pubkeys TLV layout), never as base58, so this returns
// that material. The xpub/tpub version bytes are a RENDER-time choice (§2 step
// 5) and belong to whoever renders.

// numsHCompressed is BIP-341's H point, compressed (0x02 ‖ x). Same x as the
// raw NUMS internal key md renders for wire kind 0.
var numsHCompressed = [33]byte{
	0x02,
	0x50, 0x92, 0x9b, 0x74, 0xc1, 0xa0, 0x49, 0x54, 0xb7, 0x8b, 0x4b, 0x60, 0x35, 0xe9, 0x7a, 0x5e,
	0x07, 0x8a, 0x5a, 0x0f, 0x28, 0xec, 0x96, 0xd5, 0x47, 0xbf, 0xee, 0x9a, 0xce, 0x80, 0x3a, 0xc0,
}

// lianaUnspendableKey is §2's recipe: chain code = sha256 of the leaf keys'
// 33-byte compressed pubkeys concatenated IN THE ORDER GIVEN -- not sorted, not
// deduplicated -- and the public key is always H. Depth 0, parent fingerprint 0
// and child number 0 are implied by the caller's rendering.
func lianaUnspendableKey(leafPubkeys [][33]byte) [65]byte {
	h := sha256.New()
	for _, pk := range leafPubkeys {
		h.Write(pk[:])
	}
	var out [65]byte
	copy(out[:32], h.Sum(nil))
	copy(out[32:], numsHCompressed[:])
	return out
}
