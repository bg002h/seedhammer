package md

import (
	"testing"
)

// TestDuplicateKeySlotOnTheCorpusKeyReuseVectors pins the predicate over the
// corpus's three key-reuse policies, and names WHICH AUTHORITY decides each.
//
// IT WAS TestDuplicateKeySlotMatchesBitcoinCore, and the rename is F-533. Two
// of the three rows are no longer Core's answer, so a name promising they are
// would be the next reader's trap. Core's measurements have not changed and
// are still the reason the third row reads as it does.
//
// Core 31.1, on a throwaway regtest datadir, with the keys version-swapped to
// tpub and the stale BIP-380 checksum stripped:
//
//	keyed_tr_multi_a             ACCEPTED
//	keyed_tr_sortedmulti_a       ACCEPTED
//	keyed_wsh_timelock_hashlock  "... is not sane: contains duplicate public keys"
//
// That table is still TRUE and still the reason the two taproot rows are not
// DuplicateRefusedByCore: a taproot internal key sits OUTSIDE the miniscript,
// so tr(K, multi_a(2,K,K2)) repeats nothing within any one expression and Core
// imports it. What changed is that Core is no longer the only rule here. BIP
// 388 requires placeholders to be pairwise distinct across the WHOLE
// descriptor, the Rust primary refuses both taproot vectors under it, and the
// predicate now reports them as DuplicateTaprootInternalKey -- a third kind,
// precisely so Core's verdict stays available to the sentences that quote it
// instead of being overwritten (F-533).
//
// If a row disagrees with the authority named beside it, the predicate is
// wrong, not the table.
//
// MUTATION: scope the wsh case to a single or_i arm and the wsh row fails.
//
// AND ONE THAT DOES NOT, stated because an earlier version of this comment
// claimed it did (review I-3): counting the whole taptree at once instead of
// per leaf leaves BOTH tr rows green. It has to, and the reason is in
// TestDuplicateKeySlotScopesPerTapLeaf: no vendored vector separates the two
// scopings, because the corpus's only taproot reuse is between the internal key
// and a leaf, which both scopings report identically. That hand-built test is
// the ONLY coverage the scoping rule has, so do not delete it as redundant.
func TestDuplicateKeySlotOnTheCorpusKeyReuseVectors(t *testing.T) {
	for _, tc := range []struct {
		name string
		want DuplicateKind
		why  string
	}{
		{"keyed_tr_multi_a", DuplicateTaprootInternalKey,
			"Core ACCEPTS (the internal key is outside the miniscript); BIP 388 FORBIDS " +
				"it, because @0 is both the internal key and a leaf key"},
		{"keyed_tr_sortedmulti_a", DuplicateTaprootInternalKey,
			"Core ACCEPTS (the internal key is outside the miniscript); BIP 388 FORBIDS " +
				"it, because @0 is both the internal key and a leaf key"},
		{"keyed_wsh_timelock_hashlock", DuplicateRefusedByCore,
			"Core REFUSES: one slot repeats inside one miniscript"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			slot, kind, err := DuplicateKeySlotChunks(vectorChunksFor(t, tc.name))
			if err != nil {
				t.Fatalf("DuplicateKeySlotChunks: %v", err)
			}
			if kind != tc.want {
				t.Errorf("kind=%v (slot @%d), want %v -- %s", kind, slot, tc.want, tc.why)
			}
		})
	}
}

// TestDuplicateKeySlotIsQuietOnPoliciesWithoutReuse is the negative half. A
// predicate that answered "duplicate" for everything would pass the table above
// on one row out of three and put a warning on every wallet the device builds.
func TestDuplicateKeySlotIsQuietOnPoliciesWithoutReuse(t *testing.T) {
	for _, name := range []string{
		"keyed_compose_wsh_timelock_hashlock",
		"keyed_compose_tr_nums_three_leaves",
		"keyed_wpkh",
		"keyed_compose_preset_plain_multisig",
	} {
		t.Run(name, func(t *testing.T) {
			slot, kind, err := DuplicateKeySlotChunks(vectorChunksFor(t, name))
			if err != nil {
				t.Fatalf("DuplicateKeySlotChunks: %v", err)
			}
			if kind != DuplicateNone {
				t.Errorf("reported @%d as %v in a policy with no key reuse", slot, kind)
			}
		})
	}
}

// TestDuplicateKeySlotScopesPerTapLeaf pins the scoping rule directly, because
// nothing in the vendored corpus can.
//
// A probe over every tr vector found NONE where per-leaf and whole-taptree
// counting disagree: the corpus's only taproot reuse is between the internal
// key and a leaf, which both scopings report identically. So the fixture is
// built as a tree rather than decoded from a card — the alternative was to
// claim a mutation reds when it does not, which is how a scoping rule ends up
// shipped on nobody's evidence.
//
// Core's rule is per miniscript EXPRESSION, and each taptree leaf is one. A key
// in two different leaves is therefore not a duplicate to Core, and a device
// that warned about it would be warning about a wallet Core accepts.
//
// MUTATION: count the whole taptree at once (duplicateInExpression on the tree)
// and "one key in two leaves" fails.
func TestDuplicateKeySlotScopesPerTapLeaf(t *testing.T) {
	key := func(slot uint8) node {
		return node{tag: tagPkK, body: keyArgBody{index: slot}}
	}
	branch := func(l, r node) node {
		return node{tag: tagTapTree, body: childrenBody{children: []node{l, r}}}
	}
	// Through the PUBLIC entry point, with a real tr root. An earlier draft
	// called duplicateInTapTree directly and so could not see a mutation of
	// DuplicateKeySlot's own scoping decision -- the test passed while the
	// thing it was named for was broken.
	trWith := func(tree node) node {
		return node{tag: tagTr, body: trBody{ik: InternalKeyNUMS, tree: &tree}}
	}

	t.Run("one key in two leaves is not a duplicate", func(t *testing.T) {
		// Core sees two expressions, each holding @0 once.
		if slot, kind := DuplicateKeySlot(trWith(branch(key(0), key(0)))); kind != DuplicateNone {
			t.Errorf("reported @%d duplicated across two LEAVES; each leaf is its own "+
				"miniscript expression and Core accepts this", slot)
		}
	})

	t.Run("one key twice in ONE leaf is a duplicate", func(t *testing.T) {
		// multi(2,@0,@0) inside a single leaf: one expression, key repeated.
		leaf := node{tag: tagMulti, body: multiKeysBody{k: 2, indices: []uint8{0, 0}}}
		slot, kind := DuplicateKeySlot(trWith(branch(leaf, key(1))))
		if kind == DuplicateNone {
			t.Fatal("a key repeated inside ONE leaf went unreported; that is the shape " +
				"Core refuses as \"contains duplicate public keys\"")
		}
		if slot != 0 {
			t.Errorf("reported @%d, want @0", slot)
		}
	})

	t.Run("the lowest duplicated slot is reported", func(t *testing.T) {
		leaf := node{tag: tagMulti, body: multiKeysBody{k: 2, indices: []uint8{3, 1, 3, 1}}}
		slot, dup := duplicateInExpression(leaf)
		if !dup || slot != 1 {
			t.Errorf("got @%d dup=%v, want @1 -- the answer must not depend on "+
				"traversal order", slot, dup)
		}
	})
}

// TestDuplicateKindSplitsByWhatCoreDoes pins the discriminant, which is the
// half of this predicate that decides WHICH SENTENCE the operator reads.
//
// Measured on Bitcoin Core 25.0.0, getdescriptorinfo on a throwaway regtest
// datadir, same key twice:
//
//	wsh(sortedmulti(2,A,A,B))  ACCEPTED
//	wsh(multi(2,A,A,B))        ACCEPTED
//	wsh(and_v(v:pk(A),pk(A)))  "is not sane: contains duplicate public keys"
//
// Core parses a top-level multi/sortedmulti under wsh/sh as a
// MultisigDescriptor, and the duplicate-key refusal comes from miniscript's
// IsSane, which such a descriptor never reaches. Telling the operator Core
// refuses it would be false — and that shape is the MORE dangerous one, since
// one key filling two seats drops the distinct keys needed from k to
// max(1, k-m+1) (NOT "can meet the threshold alone", retracted at review I-5),
// so it needs the other
// sentence rather than no sentence.
//
// MUTATION: make kindForRoot always return DuplicateRefusedByCore and the
// multisig rows fail.
func TestDuplicateKindSplitsByWhatCoreDoes(t *testing.T) {
	dup := func(tag tag) node {
		return node{tag: tag, body: multiKeysBody{k: 2, indices: []uint8{0, 0, 1}}}
	}
	wsh := func(inner node) node {
		return node{tag: tagWsh, body: childrenBody{children: []node{inner}}}
	}
	shWsh := func(inner node) node {
		return node{tag: tagSh, body: childrenBody{children: []node{wsh(inner)}}}
	}
	trLeaf := func(leaf node) node {
		tree := node{tag: tagTapTree, body: childrenBody{children: []node{
			leaf, {tag: tagPkK, body: keyArgBody{index: 9}},
		}}}
		return node{tag: tagTr, body: trBody{ik: InternalKeyNUMS, tree: &tree}}
	}
	// and_v(v:pk(@0), pk(@0)) — a miniscript expression, not a bare threshold.
	nested := node{tag: tagAndV, body: childrenBody{children: []node{
		{tag: tagPkK, body: keyArgBody{index: 0}},
		{tag: tagPkK, body: keyArgBody{index: 0}},
	}}}

	for _, tc := range []struct {
		name string
		tree node
		want DuplicateKind
	}{
		{"wsh(sortedmulti)", wsh(dup(tagSortedMulti)), DuplicateFewerKeys},
		{"wsh(multi)", wsh(dup(tagMulti)), DuplicateFewerKeys},
		{"sh(wsh(sortedmulti))", shWsh(dup(tagSortedMulti)), DuplicateFewerKeys},
		{"wsh(and_v(pk,pk))", wsh(nested), DuplicateRefusedByCore},
		// TAPROOT IS ALWAYS THE FEWER-KEYS SENTENCE, and this row is the one
		// that would have caught my wrong belief. I reasoned that a tapleaf
		// multi_a IS miniscript to Core and so would be refused. Measured on
		// Core 25.0.0: tr(A,multi_a(2,B,B)) and tr(A,sortedmulti_a(2,B,B)) are
		// ACCEPTED, because Core has no tapscript miniscript at all --
		// "Miniscript expressions can only be used in wsh". Telling the
		// operator Core refuses it would be false for every taproot shape.
		{"tr(multi_a with a repeat in one leaf)", trLeaf(dup(tagMultiA)), DuplicateFewerKeys},
		{"tr(sortedmulti_a with a repeat in one leaf)", trLeaf(dup(tagSortedMultiA)), DuplicateFewerKeys},
		{"tr(and_v(pk,pk) in one leaf)", trLeaf(nested), DuplicateFewerKeys},
	} {
		t.Run(tc.name, func(t *testing.T) {
			slot, kind := DuplicateKeySlot(tc.tree)
			if kind != tc.want {
				t.Errorf("%s reported %v (@%d), want %v -- the operator would read the "+
					"sentence that is not true of this shape", tc.name, kind, slot, tc.want)
			}
		})
	}
}
