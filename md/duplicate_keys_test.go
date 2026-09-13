package md

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func vectorChunksFor(t *testing.T, name string) []string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "vectors", name+".phrase.txt"))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	var out []string
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.ReplaceAll(strings.TrimSpace(line), " ", "")
		if strings.HasPrefix(line, "md1") {
			out = append(out, line)
		}
	}
	if len(out) == 0 {
		t.Fatalf("%s carries no md1 strings", name)
	}
	return out
}

// TestDuplicateKeySlotMatchesBitcoinCore pins the predicate to the only
// authority that matters for it: what Core actually does.
//
// These three vectors are the corpus's key-reuse policies, and Core 31.1 SPLITS
// them — measured on a throwaway regtest datadir, with the keys version-swapped
// to tpub and the stale BIP-380 checksum stripped:
//
//	keyed_tr_multi_a             ACCEPTED
//	keyed_tr_sortedmulti_a       ACCEPTED
//	keyed_wsh_timelock_hashlock  "... is not sane: contains duplicate public keys"
//
// A taproot internal key sits OUTSIDE the miniscript, so tr(K, multi_a(2,K,K2))
// repeats nothing within any one expression. In the wsh vector a slot really is
// repeated inside one miniscript, across both arms of the or_i.
//
// If this test ever disagrees with the table, the predicate is wrong, not the
// table: warning about a wallet Core accepts trains the operator to tap through
// the warning that matters.
//
// MUTATION: make duplicateInTapTree count the whole taptree at once instead of
// per leaf, and both tr rows fail. Scope the wsh case to a single or_i arm and
// the wsh row fails.
func TestDuplicateKeySlotMatchesBitcoinCore(t *testing.T) {
	for _, tc := range []struct {
		name    string
		wantDup bool
		why     string
	}{
		{"keyed_tr_multi_a", false, "Core ACCEPTS: the internal key is outside the miniscript"},
		{"keyed_tr_sortedmulti_a", false, "Core ACCEPTS: the internal key is outside the miniscript"},
		{"keyed_wsh_timelock_hashlock", true, "Core REFUSES: one slot repeats inside one miniscript"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			slot, dup, err := DuplicateKeySlotChunks(vectorChunksFor(t, tc.name))
			if err != nil {
				t.Fatalf("DuplicateKeySlotChunks: %v", err)
			}
			if dup != tc.wantDup {
				t.Errorf("duplicate=%v (slot @%d), want %v -- %s",
					dup, slot, tc.wantDup, tc.why)
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
			slot, dup, err := DuplicateKeySlotChunks(vectorChunksFor(t, name))
			if err != nil {
				t.Fatalf("DuplicateKeySlotChunks: %v", err)
			}
			if dup {
				t.Errorf("reported @%d duplicated in a policy with no key reuse", slot)
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
		return node{tag: tagTr, body: trBody{isNums: true, tree: &tree}}
	}

	t.Run("one key in two leaves is not a duplicate", func(t *testing.T) {
		// Core sees two expressions, each holding @0 once.
		if slot, dup := DuplicateKeySlot(trWith(branch(key(0), key(0)))); dup {
			t.Errorf("reported @%d duplicated across two LEAVES; each leaf is its own "+
				"miniscript expression and Core accepts this", slot)
		}
	})

	t.Run("one key twice in ONE leaf is a duplicate", func(t *testing.T) {
		// multi(2,@0,@0) inside a single leaf: one expression, key repeated.
		leaf := node{tag: tagMulti, body: multiKeysBody{k: 2, indices: []uint8{0, 0}}}
		slot, dup := DuplicateKeySlot(trWith(branch(leaf, key(1))))
		if !dup {
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
