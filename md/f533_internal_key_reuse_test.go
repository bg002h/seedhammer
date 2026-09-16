package md

import "testing"

// ─── F-533: the taproot internal key is a use-site like any other ────────────
//
// BIP 388 requires a wallet policy's key placeholders to be pairwise distinct
// across the WHOLE descriptor. Core's duplicate-key rule is per miniscript
// EXPRESSION, and a taproot internal key is in none of them, so tr(@0,
// multi_a(2,@0,@1)) repeats nothing Core can see and Core 31.1 imports it --
// measured, and the table in duplicate_keys.go still says so.
//
// This is the one BIP-388 rule the predicate now also carries: a key slot may
// not appear at both the taproot internal key and inside the taptree. Nothing
// wider. A slot in two different LEAVES stays legal here (there is no md1 wire
// shape that makes it BIP-388-forbidden either -- a use-site is per-@N, so two
// occurrences of one slot always carry the same key expression), and
// TestDuplicateKeySlotScopesPerTapLeaf still pins that.

// trInternal builds tr(internal, tree), choosing NUMS or a placeholder.
func trInternal(isNums bool, keyIndex uint8, tree *node) node {
	return node{tag: tagTr, body: trBody{isNums: isNums, keyIndex: keyIndex, tree: tree}}
}

func tapLeafKey(slot uint8) node {
	return node{tag: tagPkK, body: keyArgBody{index: slot}}
}

func tapBranch(l, r node) node {
	return node{tag: tagTapTree, body: childrenBody{children: []node{l, r}}}
}

// TestTaprootInternalKeyReuseIsItsOwnKind is F-533's whole behaviour, on the
// hand-built shapes, where the NUMS guard can be seen failing.
//
// MUTATION 1 -- drop `!isNums` from the tagTr arm: "NUMS internal key beside a
// leaf using @0" fails, and so does every NUMS corpus vector whose taptree
// touches slot 0 (eight of them are ok/agrees in the policy corpus).
//
// MUTATION 2 -- drop the internal-key term entirely: the two reuse rows fail
// and the corpus's ok/refused bucket returns to 2.
//
// MUTATION 3 -- report slot 0 instead of b.keyIndex: the "@1 at the internal
// key" row fails, which is the row that proves the answer is the slot that was
// actually reused rather than the first one.
func TestTaprootInternalKeyReuseIsItsOwnKind(t *testing.T) {
	leaf0 := tapLeafKey(0)
	leaf1 := tapLeafKey(1)

	for _, tc := range []struct {
		name string
		tree node
		want DuplicateKind
		slot uint8
		why  string
	}{
		{
			name: "placeholder internal key, same slot in a leaf",
			tree: trInternal(false, 0, ptrNode(tapBranch(leaf0, leaf1))),
			want: DuplicateTaprootInternalKey, slot: 0,
			why: "BIP 388 forbids @0 at the internal key AND in a leaf",
		},
		{
			name: "placeholder internal key @1, same slot in a leaf",
			tree: trInternal(false, 1, ptrNode(tapBranch(leaf0, leaf1))),
			want: DuplicateTaprootInternalKey, slot: 1,
			why: "the slot reported must be the one reused, not the lowest in the tree",
		},
		{
			name: "NUMS internal key beside a leaf using @0",
			tree: trInternal(true, 0, ptrNode(tapBranch(leaf0, leaf1))),
			want: DuplicateNone,
			why: "SPEC 7: is_nums=true means the H-point, not a reference to slot 0. " +
				"Counting it would refuse most of the taproot corpus",
		},
		{
			name: "placeholder internal key, a DIFFERENT slot in every leaf",
			tree: trInternal(false, 2, ptrNode(tapBranch(leaf0, leaf1))),
			want: DuplicateNone,
			why:  "no slot appears twice anywhere; this is an ordinary taproot policy",
		},
		{
			name: "placeholder internal key, no taptree at all",
			tree: trInternal(false, 0, nil),
			want: DuplicateNone,
			why:  "key-path-only tr: there is no second use-site to collide with",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			slot, kind := DuplicateKeySlot(tc.tree)
			if kind != tc.want {
				t.Fatalf("kind=%v (@%d), want %v -- %s", kind, slot, tc.want, tc.why)
			}
			if tc.want != DuplicateNone && slot != tc.slot {
				t.Errorf("reported @%d, want @%d -- %s", slot, tc.slot, tc.why)
			}
		})
	}
}

// TestTaprootInternalKeyReuseOnThePinnedVectors is the same claim on real
// cards: the two policies that sat in F-533's gap.
//
// They are read through the fork-side PIN (F-529), so this keeps its evidence
// if the vendored corpus loses them.
func TestTaprootInternalKeyReuseOnThePinnedVectors(t *testing.T) {
	for _, name := range pinnedKeyReuseVectors {
		t.Run(name, func(t *testing.T) {
			slot, kind, err := DuplicateKeySlotChunks(vectorChunksFor(t, name))
			if err != nil {
				t.Fatalf("DuplicateKeySlotChunks: %v", err)
			}
			if kind != DuplicateTaprootInternalKey {
				t.Errorf("kind=%v (@%d), want DuplicateTaprootInternalKey -- this card "+
					"puts one slot at the taproot internal key and again inside a leaf, "+
					"which BIP 388 forbids and the Rust primary refuses", kind, slot)
			}
			if slot != 0 {
				t.Errorf("reported @%d, want @0", slot)
			}
		})
	}
}

// TestAWithinLeafDuplicateStillWinsOverTheInternalKey pins the PRECEDENCE,
// because both harms can be present at once and only one sentence is shown.
//
// The within-expression repeat keeps the kind it has always had. It is a fact
// about what Core does with the descriptor, it is the older and better-measured
// of the two, and leaving it first means this change moves NO policy that
// already reported a duplicate -- the whole corpus delta is the two vectors
// that reported DuplicateNone before.
//
// MUTATION: test the internal key BEFORE duplicateInTapTree and this fails.
func TestAWithinLeafDuplicateStillWinsOverTheInternalKey(t *testing.T) {
	// tr(@0, taptree{ multi_a(2,@1,@1), pk(@0) }) -- @1 repeats inside ONE
	// leaf, and @0 is both the internal key and the other leaf.
	repeat := node{tag: tagMultiA, body: multiKeysBody{k: 2, indices: []uint8{1, 1}}}
	tree := trInternal(false, 0, ptrNode(tapBranch(repeat, tapLeafKey(0))))
	slot, kind := DuplicateKeySlot(tree)
	if kind != DuplicateFewerKeys || slot != 1 {
		t.Errorf("kind=%v (@%d), want DuplicateFewerKeys @1 -- a slot repeated inside "+
			"one leaf is the older finding and keeps the screen", kind, slot)
	}
}

func ptrNode(n node) *node { return &n }
