package md

// Bitcoin Core's duplicate-key sanity rule, evaluated on a decoded md1 tree.
//
// WHY THIS EXISTS, and why it is NOT the BIP-388 rule the Rust CLI enforces.
//
// Core refuses a descriptor whose miniscript repeats a public key: "is not sane:
// contains duplicate public keys". That is a SCRIPT-level rule, and it is scoped
// to one miniscript expression. BIP 388's disjointness rule is a WALLET-POLICY
// rule about a placeholder's key-expression set. The two disagree, on purpose,
// about taproot: an internal key sits OUTSIDE the miniscript, so tr(K, multi_a(
// 2, K, K2)) repeats nothing within any one expression and Core derives it
// happily, while BIP 388 still calls the reuse forbidden.
//
// Measured against Bitcoin Core 31.1 on a throwaway regtest datadir over the
// three key-reuse vectors in the vendored corpus:
//
//	keyed_tr_multi_a           ACCEPTED
//	keyed_tr_sortedmulti_a     ACCEPTED
//	keyed_wsh_timelock_hashlock  "... is not sane: contains duplicate public keys"
//
// This predicate answers CORE's question, because that is the one that predicts
// whether the wallet an operator is about to engrave will be accepted by the
// coordinator they will try to spend from. The device derives a fundable
// address for that wsh policy today and says nothing (F-514); a plate cut from
// it is a plate whose descriptor a Core-based coordinator refuses.
//
// It is a WARNING's predicate, never a refusal's. Refusing on-device would
// strand a card that may already be engraved, which is worse than telling the
// operator nothing at all.

// DuplicateKind says WHICH harm a repeated slot carries, because the two are
// different and an operator told the wrong one looks in the wrong place.
type DuplicateKind int

const (
	// DuplicateNone: no slot repeats within one expression.
	DuplicateNone DuplicateKind = iota
	// DuplicateInMiniscript: the repeat is inside a miniscript expression, so
	// Bitcoin Core refuses the descriptor outright --
	// "is not sane: contains duplicate public keys".
	DuplicateInMiniscript
	// DuplicateInMultisig: the repeat is inside a TOP-LEVEL multi/sortedmulti,
	// which Core parses as a MultisigDescriptor and never runs the miniscript
	// sanity check on. Core IMPORTS it. The harm is the other one: one key fills
	// two of the threshold's seats, so a 2-of-3 with one key twice can be spent
	// by that key alone.
	DuplicateInMultisig
)

// DuplicateKeySlot reports the lowest key slot that appears more than once
// inside a SINGLE miniscript expression, and which harm that carries.
//
// The scoping is the whole of the rule:
//
//   - under tr, every taptree LEAF is its own expression and the internal key is
//     in none of them, so a key shared between the internal key and a leaf, or
//     between two different leaves, is not a duplicate;
//   - under wsh, sh(wsh(...)) and bare sh(...), the script is one expression and
//     a slot repeated anywhere within it is a duplicate.
//
// The second return is false when there is none.
func DuplicateKeySlot(tree node) (uint8, DuplicateKind) {
	switch tree.tag {
	case tagTr:
		b, ok := tree.body.(trBody)
		if !ok || b.tree == nil {
			return 0, DuplicateNone
		}
		slot, dup := duplicateInTapTree(*b.tree)
		return slot, kindOf(dup, DuplicateInMiniscript)
	case tagWsh, tagSh:
		b, ok := tree.body.(childrenBody)
		if !ok || len(b.children) != 1 {
			return 0, DuplicateNone
		}
		inner := b.children[0]
		// sh(wsh(X)) -- unwrap so the discriminant below sees the script that
		// actually runs, not the wrapper around it.
		if tree.tag == tagSh && inner.tag == tagWsh {
			ib, ok := inner.body.(childrenBody)
			if !ok || len(ib.children) != 1 {
				return 0, DuplicateNone
			}
			inner = ib.children[0]
		}
		slot, dup := duplicateInExpression(inner)
		return slot, kindOf(dup, kindForRoot(inner))
	default:
		slot, dup := duplicateInExpression(tree)
		return slot, kindOf(dup, kindForRoot(tree))
	}
}

func kindOf(dup bool, k DuplicateKind) DuplicateKind {
	if !dup {
		return DuplicateNone
	}
	return k
}

// kindForRoot distinguishes the two harms by what sits at the TOP of the
// script.
//
// Measured on Bitcoin Core 25.0.0, a throwaway regtest datadir,
// getdescriptorinfo, with the same key twice:
//
//	wsh(sortedmulti(2,A,A,B))         ACCEPTED
//	wsh(multi(2,A,A,B))               ACCEPTED
//	wsh(and_v(v:pk(A),pk(A)))         "is not sane: contains duplicate public keys"
//	the corpus's keyed_wsh_timelock_hashlock  same refusal
//
// Core parses a top-level multi/sortedmulti under wsh/sh as a
// MultisigDescriptor, and "contains duplicate public keys" comes from
// miniscript's IsSane, which such a descriptor never reaches. So telling the
// operator that Core refuses it would be FALSE for that shape -- and the shape
// is not harmless, it is the more dangerous of the two: one key filling two
// seats of a threshold can meet it alone.
func kindForRoot(n node) DuplicateKind {
	switch n.tag {
	case tagMulti, tagSortedMulti, tagMultiA, tagSortedMultiA:
		return DuplicateInMultisig
	}
	return DuplicateInMiniscript
}

// DuplicateKeySlotChunks is DuplicateKeySlot over a gathered md1 chunk set.
func DuplicateKeySlotChunks(strs []string) (uint8, DuplicateKind, error) {
	d, err := Reassemble(strs)
	if err != nil {
		return 0, DuplicateNone, err
	}
	slot, kind := DuplicateKeySlot(d.tree)
	return slot, kind, nil
}

// duplicateInTapTree checks each LEAF separately, because each leaf is its own
// miniscript expression. A key in two different leaves is not a duplicate to
// Core, and reporting it as one would warn about a wallet Core accepts.
func duplicateInTapTree(n node) (uint8, bool) {
	if n.tag == tagTapTree {
		b, ok := n.body.(childrenBody)
		if !ok || len(b.children) != 2 {
			return 0, false
		}
		if slot, dup := duplicateInTapTree(b.children[0]); dup {
			return slot, true
		}
		return duplicateInTapTree(b.children[1])
	}
	return duplicateInExpression(n)
}

// duplicateInExpression counts key-slot occurrences in one expression and
// returns the LOWEST slot seen twice, so the answer does not depend on tree
// traversal order.
func duplicateInExpression(n node) (uint8, bool) {
	var counts [256]int
	countKeySlots(n, &counts)
	for slot := 0; slot < len(counts); slot++ {
		if counts[slot] > 1 {
			return uint8(slot), true
		}
	}
	return 0, false
}

// countKeySlots tallies every key-slot reference under n.
//
// A nested tr cannot occur inside a miniscript expression, so tr is not walked
// here; DuplicateKeySlot handles the root case and nothing else produces one.
func countKeySlots(n node, counts *[256]int) {
	switch b := n.body.(type) {
	case keyArgBody:
		counts[b.index]++
	case multiKeysBody:
		for _, idx := range b.indices {
			counts[idx]++
		}
	case childrenBody:
		for _, c := range b.children {
			countKeySlots(c, counts)
		}
	case variableBody:
		for _, c := range b.children {
			countKeySlots(c, counts)
		}
	}
}
