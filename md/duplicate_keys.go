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
// This predicate answers CORE's question for the two kinds named after Core's
// verdicts, because that is the one that predicts whether the wallet an
// operator is about to engrave will be accepted by the coordinator they will
// try to spend from.
//
// AND ONE QUESTION THAT IS NOT CORE'S (F-533, and this paragraph replaces the
// one that said the remedy was a second predicate). The tr arm also reports
// DuplicateTaprootInternalKey when a slot sits at the taproot internal key AND
// inside the taptree -- the shape in the first two rows above, which Core
// imports and BIP 388 forbids. It is a THIRD KIND rather than a wider reading
// of the other two: the Core sentences stay attached to the Core kinds and
// stay true, and the new shape gets copy in BIP 388's voice.
//
// WHY ONE PREDICATE AND NOT TWO. F-533 was filed calling for a second
// predicate, and gui/policy_address.go argued in as many words against one:
// "Two predicates would drift, and the drift would show up as a screen that
// warns and derives, or one that refuses in silence." The nesting result
// reconciles them. In the md1 wire a use-site is per-@N -- descriptor.useSite
// plus strictly-ascending per-idx overrides -- so one slot cannot carry two
// disjoint key expressions, which makes Core-duplicate a STRICT SUBSET of
// BIP-388-forbidden here. There is no policy that is Core-duplicate and
// BIP-388-legal, so no case exists where a refusal would lift while a duplicate
// remained, and the predicate gets WIDER rather than doubled.
//
// IT PORTS EXACTLY ONE BIP-388 RULE, not the primary's taxonomy. A slot in two
// different LEAVES is not reported: the wire cannot give one @N two different
// key expressions, so that shape is not forbidden here and derives correctly.
//
// IT IS NOW A REFUSAL'S PREDICATE TOO (F-531), and this paragraph used to say
// the opposite -- "a WARNING's predicate, never a refusal's", on the ground
// that refusing would strand an already-engraved card. The device declines to
// derive an ADDRESS for any policy this reports, on both address routes, while
// the card still decodes, displays, verifies and warns; so nothing is stranded,
// and what F-514 weighed was the wider refusal of the card itself.
//
// THAT MADE THE SCOPING ABOVE LOAD-BEARING IN A SECOND WAY, and F-533 is what
// closed it. As a warning's predicate, answering Core's question rather than
// BIP 388's was simply correct. As a refusal's, it was NARROWER than the
// operator's ruling on BIP-388-forbidden wallets: tr(@0, multi_a(2,@0,@1))
// repeats a slot across the internal key and a leaf, BIP 388 forbids it, the
// Rust primary refuses it, and this used to report DuplicateNone because Core
// imports it. Two vendored vectors sat in that gap -- keyed_tr_multi_a and
// keyed_tr_sortedmulti_a, now pinned fork-side against F-529's re-vendor. They
// report DuplicateTaprootInternalKey, and the corpus's ok/refused bucket is
// empty.

// DuplicateKind says WHICH harm a repeated slot carries, because the two are
// different and an operator told the wrong one looks in the wrong place.
type DuplicateKind int

const (
	// DuplicateNone: no slot repeats within one expression.
	DuplicateNone DuplicateKind = iota
	// DuplicateRefusedByCore: the repeat is inside a WSH/SH miniscript
	// expression, so Bitcoin Core refuses the descriptor outright --
	// "is not sane: contains duplicate public keys".
	//
	// THIS NAME IS A CLAIM ABOUT A VERSION, and the file it lives in has no
	// version in it. Everything here was measured on Core 25.0.0. The two kinds
	// are asymmetric for that reason: DuplicateFewerKeys states a property of
	// the POLICY and cannot go stale, while this one states a verdict of
	// somebody else's software.
	//
	// WHAT WOULD FALSIFY IT: Core 25 has no tapscript miniscript at all, which
	// is why every taproot shape is FewerKeys below. If a later Core runs
	// IsSane on tapscript, a tapleaf multi_a with a repeated key plausibly
	// starts failing it, and the tr arm becomes wrong in the other direction --
	// silently, because nothing here watches Core's version. Re-measure before
	// trusting this on a newer node.
	DuplicateRefusedByCore
	// DuplicateFewerKeys: Core IMPORTS this shape, so the harm is the other one
	// -- one key fills more than one seat, and the wallet needs fewer separate
	// keys than its k-of-n says.
	//
	// NAMED FOR THE HARM, NOT THE PLACE. It was DuplicateInMultisig, and that
	// name encoded an assumption that turned out to be false: WHERE the repeat
	// sits does not predict what Core does. A tapleaf multi_a is "miniscript"
	// by shape and Core still imports it, because Core 25.0.0 has no tapscript
	// miniscript at all. Naming the kinds after the sentence they produce keeps
	// the next such surprise from silently picking the wrong one.
	DuplicateFewerKeys
	// DuplicateTaprootInternalKey: one key slot sits at the taproot internal
	// key AND inside the taptree. F-533.
	//
	// THIS IS THE ONE KIND THAT IS NOT A CORE VERDICT, and that is why it is a
	// separate kind rather than a wider read of the two above. Core 31.1
	// ACCEPTS this shape -- measured, see the table at the top of this file --
	// so both of the sentences those kinds produce would be FALSE here. BIP
	// 388 forbids it: a wallet policy's key placeholders must be pairwise
	// distinct across the whole descriptor, and the internal key is part of
	// the descriptor even though it is outside every miniscript expression.
	//
	// ITS HARM IS THE STRONGER FORM OF THE ONE F-531 CITED. A repeated seat in
	// a multisig signs the same sighash twice; a key that is both the internal
	// key and a leaf key signs a key-path sighash AND a script-path sighash --
	// two genuinely different messages, which is the pubkey-reuse insecurity
	// BIP 388's disjointness rule points at.
	//
	// WHAT WOULD FALSIFY IT: nothing Core does. Unlike DuplicateRefusedByCore
	// this states a rule of BIP 388, which is a written specification rather
	// than somebody else's software, so it cannot go stale on a version bump.
	DuplicateTaprootInternalKey
)

// DuplicateKeySlot reports a key slot used more than once by this policy, and
// which harm that carries. DuplicateNone means there is none.
//
// The scoping is the whole of the rule, and it is TWO rules since F-533:
//
//   - under tr, every taptree LEAF is its own miniscript expression and the
//     internal key is in none of them, so the lowest slot repeated within one
//     leaf is a Core duplicate (DuplicateFewerKeys). A key shared between two
//     different leaves is NOT;
//   - also under tr, a PLACEHOLDER internal key -- is_nums=false -- that occurs
//     again anywhere in the taptree is DuplicateTaprootInternalKey, which is BIP
//     388's rule rather than Core's. The within-leaf finding is reported first
//     when both apply;
//   - under wsh, sh(wsh(...)) and bare sh(...), the script is one expression and
//     a slot repeated anywhere within it is a duplicate.
//
// The slot returned is the lowest repeated one for the within-expression kinds,
// and the internal key's own slot for DuplicateTaprootInternalKey.
func DuplicateKeySlot(tree node) (uint8, DuplicateKind) {
	switch tree.tag {
	case tagTr:
		b, ok := tree.body.(trBody)
		if !ok || b.tree == nil {
			return 0, DuplicateNone
		}
		// TAPROOT IS ALWAYS THE FEWER-KEYS SENTENCE, measured rather than
		// reasoned. Core 25.0.0 rejects miniscript under tr outright --
		// "Miniscript expressions can only be used in wsh" -- so a tapleaf
		// multi_a or sortedmulti_a never reaches CheckDuplicateKey, and
		// tr(A,multi_a(2,B,B)), tr(A,sortedmulti_a(2,B,B)) and
		// tr(A,multi_a(2,A,A,B)) are all ACCEPTED.
		//
		// The earlier code said DuplicateRefusedByCore here, which told the
		// operator Core refuses a descriptor Core imports. I believed tapscript
		// multi_a WAS miniscript to Core and was wrong; the fix came from
		// running getdescriptorinfo, not from reading the parser.
		//
		// THE WITHIN-LEAF FINDING IS TESTED FIRST, and the order is a decision
		// rather than an accident: both harms can be present at once, only one
		// sentence is shown, and this one is the older and better-measured of
		// the two. Keeping it first also means F-533 moved no policy that
		// already reported a duplicate -- the whole corpus delta is two vectors
		// that reported DuplicateNone before
		// (TestAWithinLeafDuplicateStillWinsOverTheInternalKey).
		if slot, dup := duplicateInTapTree(*b.tree); dup {
			return slot, DuplicateFewerKeys
		}
		// F-533: THE INTERNAL KEY IS A USE-SITE TOO, and BIP 388 counts it.
		//
		// This is the one rule in this file that is not Core's. Core's question
		// is per miniscript EXPRESSION and the internal key is in none of them,
		// so tr(@0, multi_a(2,@0,@1)) repeats nothing Core can see and Core
		// imports it. BIP 388 requires a policy's placeholders to be pairwise
		// distinct across the whole descriptor, the Rust primary refuses these,
		// and the operator's standing ruling of 2026-08-30 is that a
		// BIP-388-forbidden wallet is not one this constellation supports. It
		// gets its OWN kind so the copy can say what is actually true of it --
		// the two Core-voiced sentences would both be false here.
		//
		// ONLY WHEN !isNums, and this qualifier is the whole difference between
		// a rule and a catastrophe. SPEC 7: is_nums=true means the internal key
		// is the NUMS H-point and keyIndex is not a reference to anything --
		// canonicalize.go's walkCollectFirst skips registration on exactly this
		// condition. Counting it unconditionally reads the NUMS placeholder as
		// slot 0 and falsely refuses every NUMS taproot policy whose taptree
		// touches @0, which in the vendored corpus is eight cards that derive
		// correctly today.
		//
		// ONE RULE, NOT A PORT OF THE PRIMARY'S TAXONOMY. A slot in two
		// different LEAVES is still not reported: the md1 wire cannot give one
		// @N two different key expressions (use-site is per-@N, with only
		// strictly-ascending per-idx overrides), so that shape is not
		// BIP-388-forbidden here and derives correctly.
		if !b.isNums() {
			var counts [256]int
			countKeySlots(*b.tree, &counts)
			if counts[b.keyIndex] > 0 {
				return b.keyIndex, DuplicateTaprootInternalKey
			}
		}
		return 0, DuplicateNone
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
//	tr(A,multi_a(2,B,B))              ACCEPTED  -- no tapscript miniscript
//	tr(A,sortedmulti_a(2,B,B))        ACCEPTED
//	wsh(and_v(v:pk(A),pk(A)))         "is not sane: contains duplicate public keys"
//	the corpus's keyed_wsh_timelock_hashlock  same refusal
//
// Core parses a top-level multi/sortedmulti under wsh/sh as a
// MultisigDescriptor, and "contains duplicate public keys" comes from
// miniscript's IsSane, which such a descriptor never reaches. So telling the
// operator that Core refuses it would be FALSE for that shape -- and the shape
// is not harmless, it is the more dangerous of the two: a slot filling m >= 2
// seats drops the distinct keys needed from k to max(1, k-m+1). (This said "can
// meet it alone", which is false for every k > m and was retracted at review
// I-5; the retraction had reached gui/composer_copy.go and not here.)
func kindForRoot(n node) DuplicateKind {
	switch n.tag {
	case tagMulti, tagSortedMulti, tagMultiA, tagSortedMultiA:
		return DuplicateFewerKeys
	}
	return DuplicateRefusedByCore
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
	case trBody:
		// F-612: a `tr` NESTED INSIDE A TAPLEAF hid its internal key and its
		// whole subtree here -- this switch had no trBody arm, so such a policy
		// reported DuplicateNone however many times a slot repeated inside it.
		//
		// It is unreachable today, and that is the uncomfortable part: it is
		// unreachable only because `emitFragment` has no `tagTr` case, which is
		// a property of a DIFFERENT function in a different file. Nothing
		// connected the two, so teaching emitFragment taproot would have made
		// this live silently. TestEmitFragmentHasNoTaprootCase is the wire that
		// now connects them.
		//
		// !isNums for the same reason as DuplicateKeySlot's own arm (SPEC §7):
		// a NUMS internal key is the H-point, not a reference to a slot, and
		// keyIndex is a meaningless zero there.
		if !b.isNums() {
			counts[b.keyIndex]++
		}
		if b.tree != nil {
			countKeySlots(*b.tree, counts)
		}
	}
}
