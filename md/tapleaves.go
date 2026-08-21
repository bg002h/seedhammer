package md

import "errors"

// Tap-leaf structure, for address derivation (Stage 3).
//
// WHY THIS IS STRUCTURE AND NOT SCRIPT. Emitting a tapscript leaf needs the
// DERIVED child pubkeys, and package `md` does no key derivation — it decodes a
// wire format. So `md` reports what each leaf IS and in what tree position, and
// the caller (which already has the expanded keys) emits the script. Putting
// derivation in here would drag bip32/secp256k1 into the codec to save one
// translation.
//
// DEPTH-FIRST ORDER + DEPTH IS THE WHOLE TOPOLOGY. A binary tree is
// reconstructible from its leaves in depth-first order paired with their depths
// — the same representation rust-miniscript's `TapTree::leaves()` uses. That
// matters because the taproot Merkle root depends on the tree's SHAPE, not just
// its leaf set: {{A,B},C} and {A,{B,C}} hold the same leaves and commit to
// different output keys, so a caller that flattened this list would compute a
// wrong address for a right-spine tree.

// TapLeafKind names the fragments this accessor can describe.
type TapLeafKind uint8

const (
	// TapLeafPK — a single-key leaf: `pk(K)`, emitted as <32-byte xonly>
	// OP_CHECKSIG.
	TapLeafPK TapLeafKind = iota
	// TapLeafMultiA — `multi_a(k,...)`, key order PRESERVED.
	TapLeafMultiA
	// TapLeafSortedMultiA — `sortedmulti_a(k,...)`, keys sorted lexicographically
	// by the DERIVED pubkey at emit time. The sort cannot happen here: it is on
	// the derived child keys, which this package does not have.
	TapLeafSortedMultiA
)

// TapLeaf is one leaf of a taproot script tree.
type TapLeaf struct {
	// Depth in the taptree. 0 means a single leaf directly under `tr()` with no
	// TapTree wrapper (the single-leaf wire optimization).
	Depth int
	Kind  TapLeafKind
	// KeyIndices are the `@N` placeholders the leaf references, in WRITTEN
	// order. For TapLeafSortedMultiA the caller sorts the derived keys; the
	// written order is preserved here so the two orderings stay distinguishable.
	KeyIndices []uint8
	// K is the threshold for the multi_a kinds; 1 for TapLeafPK.
	K int
}

// TapLeavesChunks decodes an md1 chunk set and returns its taproot script-tree
// leaves in depth-first order.
//
// It returns errNoTapTree for a policy that is not a taproot with a script tree,
// and refuses — rather than approximating — any leaf shape it cannot describe.
// An address derived from an approximated leaf commits funds to the wrong
// script, so a refusal here is the only safe failure.
func TapLeavesChunks(strs []string) (internalKeyIndex uint8, isNUMS bool, leaves []TapLeaf, err error) {
	d, err := Reassemble(strs)
	if err != nil {
		return 0, false, nil, err
	}
	return tapLeaves(d.tree)
}

func tapLeaves(tree node) (uint8, bool, []TapLeaf, error) {
	if tree.tag != tagTr {
		return 0, false, nil, errNoTapTree
	}
	b, ok := tree.body.(trBody)
	if !ok {
		return 0, false, nil, errNoTapTree
	}
	if b.tree == nil {
		return 0, false, nil, errNoTapTree
	}
	var out []TapLeaf
	if err := collectTapLeaves(*b.tree, 0, &out); err != nil {
		return 0, false, nil, err
	}
	return b.keyIndex, b.isNums, out, nil
}

func collectTapLeaves(n node, depth int, out *[]TapLeaf) error {
	if n.tag == tagTapTree {
		c, ok := n.body.(childrenBody)
		if !ok || len(c.children) != 2 {
			return errTapTreeShape
		}
		if err := collectTapLeaves(c.children[0], depth+1, out); err != nil {
			return err
		}
		return collectTapLeaves(c.children[1], depth+1, out)
	}
	leaf, err := describeTapLeaf(n)
	if err != nil {
		return err
	}
	leaf.Depth = depth
	*out = append(*out, leaf)
	return nil
}

// describeTapLeaf classifies ONE leaf, unwrapping the `c:` check wrapper that a
// bare key carries.
func describeTapLeaf(n node) (TapLeaf, error) {
	switch n.tag {
	case tagPkK, tagPkH:
		b, ok := n.body.(keyArgBody)
		if !ok {
			return TapLeaf{}, errTapLeafUnsupported
		}
		return TapLeaf{Kind: TapLeafPK, KeyIndices: []uint8{b.index}, K: 1}, nil
	case tagCheck:
		b, ok := n.body.(childrenBody)
		if !ok || len(b.children) != 1 {
			return TapLeaf{}, errTapLeafUnsupported
		}
		return describeTapLeaf(b.children[0])
	case tagMultiA, tagSortedMultiA:
		b, ok := n.body.(multiKeysBody)
		if !ok {
			return TapLeaf{}, errTapLeafUnsupported
		}
		kind := TapLeafMultiA
		if n.tag == tagSortedMultiA {
			kind = TapLeafSortedMultiA
		}
		idx := append([]uint8(nil), b.indices...)
		return TapLeaf{Kind: kind, KeyIndices: idx, K: int(b.k)}, nil
	default:
		// Timelocks, hashlocks and combinators are all REAL tapscript leaves
		// this accessor cannot yet emit. Refusing is not a judgement that the
		// policy is invalid — it is a statement that deriving its address is
		// unimplemented, and inventing one would commit funds to a script we
		// did not build.
		return TapLeaf{}, errTapLeafUnsupported
	}
}

var (
	// errNoTapTree — the policy is not a taproot carrying a script tree.
	errNoTapTree = errors.New("md: not a taproot script tree")
	// errTapTreeShape — a TapTree node without exactly two children; the
	// decoder guarantees binary trees, so this is defensive.
	errTapTreeShape = errors.New("md: tap tree node is not binary")
	// errTapLeafUnsupported — a leaf shape this accessor cannot describe.
	errTapLeafUnsupported = errors.New("md: tap leaf shape not supported for address derivation")
)

// ErrTapLeafUnsupported lets a caller distinguish "cannot derive this shape yet"
// from a decode failure, so the UI can say which.
var ErrTapLeafUnsupported = errTapLeafUnsupported
