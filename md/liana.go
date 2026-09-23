package md

import (
	"crypto/sha256"
	"errors"
)

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

var (
	errLianaNotKind1   = errors.New("md: internal key is not Liana-unspendable")
	errLianaNoLeaves   = errors.New("md: Liana-unspendable internal key over a tree with no leaves")
	errLianaMissingKey = errors.New("md: a leaf key has no xpub on this card; the Liana internal key cannot be computed")
)

// collectKeyOccurrences appends every placeholder index in n, left-first
// pre-order. A TapTree node's two children are visited left then right, which
// is the leaves' left-to-right order; inside a leaf it is the fragment order.
func collectKeyOccurrences(n node, out *[]uint8) {
	switch b := n.body.(type) {
	case keyArgBody:
		*out = append(*out, b.index)
	case multiKeysBody:
		*out = append(*out, b.indices...)
	case childrenBody:
		for _, c := range b.children {
			collectKeyOccurrences(c, out)
		}
	case variableBody:
		for _, c := range b.children {
			collectKeyOccurrences(c, out)
		}
	}
}

// LianaUnspendableKeyFor is the 65-byte key material (chain code ‖ the
// compressed H point) of a kind-1 chunk set's internal key: SPEC §2 over the
// leaf keys the CALLER supplies, keyed by placeholder index, taken in the
// tree's key-occurrence order (collectKeyOccurrences: rust-miniscript's
// TapTree::leaves() + Miniscript::iter_pk(), one entry per key OCCURRENCE,
// which is what Rust feeds the recipe). The device derives the key path at 0/i and 1/i from
// it (SPEC §2, "derivation, not just rendering"), whatever the wallet's
// use-site -- §6 row 2 refuses any other use-site at mint.
//
// THE KEYS COME FROM THE CALLER, NOT FROM THE CARD'S Pubkeys TLV (F-449 stage
// 4 R0 I1). SPEC §7a.2 says the recipe runs "over the collected leaf keys", and
// on the Wallet Policy route those are the seated mk1 key cards while the md1
// is a key-less TEMPLATE with no TLV at all. Reading the TLV made every
// template-plus-cards kind-1 wallet underivable. A keyed card's own keys reach
// here the same way: ExpandWalletPolicyChunks reads them out of its TLV first.
//
// NEVER the derived-key-sorted order address.MultiALeafScript builds a
// sortedmulti_a script from (SPEC §6 row 1, fable M-8): this reads the WRITTEN
// indices off the wire tree, before any derivation.
//
// THE ONLY KEY WALK (F-449 stage 4 R1 N3): stage 3's TLV walk lianaLeafPubkeys
// was deleted, and its tests moved here, so the two routes cannot diverge.
//
// An error for a set that is not kind 1, or when a leaf's key was not supplied:
// the recipe needs every real leaf key, and there is no fallback.
func LianaUnspendableKeyFor(strs []string, xpubs map[uint8][65]byte) ([65]byte, error) {
	d, err := Reassemble(strs)
	if err != nil {
		return [65]byte{}, err
	}
	b, ok := d.tree.body.(trBody)
	if d.tree.tag != tagTr || !ok || b.ik != InternalKeyLianaUnspendable {
		return [65]byte{}, errLianaNotKind1
	}
	if b.tree == nil {
		return [65]byte{}, errLianaNoLeaves
	}
	var idx []uint8
	collectKeyOccurrences(*b.tree, &idx)
	pks := make([][33]byte, 0, len(idx))
	for _, i := range idx {
		x, ok := xpubs[i]
		if !ok {
			return [65]byte{}, errLianaMissingKey
		}
		var pk [33]byte
		copy(pk[:], x[32:65])
		pks = append(pks, pk)
	}
	return lianaUnspendableKey(pks), nil
}
