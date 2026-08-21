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

// TapLeafScript is one EMITTED tapscript leaf: its depth in the tree, and the
// script bytes themselves.
//
// The counterpart to `TapLeaf`, and it exists because describing a leaf and
// emitting one are different problems with different limits. `TapLeaf` names a
// shape from a fixed vocabulary — pk, multi_a, sortedmulti_a — and cannot say
// anything about a leaf built from timelocks, hashlocks and combinators.
// Emission has no such vocabulary: it walks the fragment tree the same way the
// segwit-v0 emitter does, so it covers everything that emitter covers.
type TapLeafScript struct {
	Depth  int
	Script []byte
}

// EmitTapLeavesChunks decodes an md1 chunk set and emits the tapscript for every
// leaf of its taproot tree, in depth-first order.
//
// `keys` maps each `@N` to its DERIVED 32-byte X-ONLY public key. X-only, not
// compressed: BIP-341 keys carry no parity byte, and a 33-byte push here builds
// a perfectly valid script for a different key.
//
// WHY THIS EXISTS ALONGSIDE TapLeavesChunks (F-214). That accessor describes
// leaves from a three-shape vocabulary, so every leaf built from a timelock or a
// hashlock was refused — and the constellation's own pathological wallet is
// FOUR such leaves. Measured: `TapLeavesChunks` on its taproot form returns
// ErrTapLeafUnsupported with zero leaves, while the primary Rust implementation
// derives its addresses without complaint. This closes that gap by reusing the
// fragment walker instead of growing the vocabulary — a tap leaf is ordinary
// miniscript, and the segwit-v0 emitter already knew how to walk it.
//
// The result feeds `address.TaprootScriptPathAddress` directly: its `LeafScript`
// is this type's shape, so the caller does no translation and there is no second
// place for depth to be got wrong.
func EmitTapLeavesChunks(strs []string, keys map[uint8][]byte) (internalKeyIndex uint8, isNUMS bool, leaves []TapLeafScript, err error) {
	d, err := Reassemble(strs)
	if err != nil {
		return 0, false, nil, err
	}
	if d.tree.tag != tagTr {
		return 0, false, nil, errNoTapTree
	}
	b, ok := d.tree.body.(trBody)
	if !ok || b.tree == nil {
		return 0, false, nil, errNoTapTree
	}
	var out []TapLeafScript
	if err := emitTapLeaves(*b.tree, 0, emitEnv{keys: keys, tap: true}, &out); err != nil {
		return 0, false, nil, err
	}
	return b.keyIndex, b.isNums, out, nil
}

// emitTapLeaves mirrors collectTapLeaves' walk exactly — same recursion, same
// depth accounting, same binary-tree refusal. Kept as a sibling rather than
// folded together because the two return different things and the shared part
// is four lines; a single walk parameterised by "describe or emit" would be
// harder to read than either.
func emitTapLeaves(n node, depth int, e emitEnv, out *[]TapLeafScript) error {
	if n.tag == tagTapTree {
		c, ok := n.body.(childrenBody)
		if !ok || len(c.children) != 2 {
			return errTapTreeShape
		}
		if err := emitTapLeaves(c.children[0], depth+1, e, out); err != nil {
			return err
		}
		return emitTapLeaves(c.children[1], depth+1, e, out)
	}
	var script []byte
	if err := emitFragment(n, e, &script); err != nil {
		return err
	}
	*out = append(*out, TapLeafScript{Depth: depth, Script: script})
	return nil
}
