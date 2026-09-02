package md

// Structural policy summary (Stage 2 of the arbitrary-tr()/wsh() plan).
//
// WHAT THIS IS FOR. `classifyPolicy` returns PolicyComplex for anything outside
// an enumerated shape list, and the consent screen then degrades to the
// honest-minimal form: script family, key-slot count, template-id, "cannot fully
// display on-device". That copy is CORRECT but it tells an operator almost
// nothing about the policy they are about to commit to steel.
//
// A structural summary is the cheaper, higher-value tier between "nothing" and a
// full miniscript text render: it is walked from the ALREADY-DECODED tree, needs
// no renderer, and cannot emit a malformed descriptor because it emits no
// descriptor at all.
//
// THE HONESTY CONTRACT, AND WHY IT IS A FIELD RATHER THAN A CONVENTION.
// SPEC §4.2/C3 refuses to summarize because "summarizing one tapscript leaf
// would omit the key-path and other-leaf spend conditions and mislead the
// operator". That objection is about a PARTIAL summary, and it is right. So this
// walk either understands EVERY node it meets and describes EVERY spend path, or
// it sets Complete=false and the caller must show nothing. A summary that
// silently skipped a branch would be worse than the honest-minimal screen it
// replaced: the operator would believe they had seen the whole policy.
//
// It deliberately does NOT reconstruct text. Naming a fragment is a rendering,
// and a rendering that cannot be re-parsed is the defect the whole cycle's
// invariant exists to prevent.

// KeyPathKind describes a taproot internal key.
type KeyPathKind uint8

const (
	// KeyPathNone — not a taproot policy.
	KeyPathNone KeyPathKind = iota
	// KeyPathNUMS — provably unspendable internal key: script paths only.
	KeyPathNUMS
	// KeyPathSpendable — a real key can spend directly, WITHOUT satisfying any
	// leaf. This is the condition a leaf-only summary would have hidden.
	KeyPathSpendable
)

// Branch is one independently satisfiable spend path: a tapscript leaf, or the
// whole script for wsh/sh.
type Branch struct {
	// K, N are set ONLY when the branch is exactly a threshold over KEYS
	// (multi/sortedmulti/multi_a/sortedmulti_a), possibly under wrappers.
	// Zero means "not a plain k-of-N" — NOT "1-of-1".
	K, N int
	// Keys counts distinct key placeholders the branch references. It is set
	// even when K/N are not, so a branch is never reported as keyless.
	Keys int
	// Timelock/Hashlock report whether the branch requires one ANYWHERE within
	// it. Locks and Sha256Digests carry the values, in wire order, so the
	// composer's consent surface (SPEC_wallet_policy_composer.md §7e) can say
	// "older 26280 blocks" instead of "time-locked"; a value is a fact from the
	// decoded tree, not a rendering, so the no-text rule above still holds. A
	// Lock carries its KIND, because the bare operand cannot: §4c's bands
	// overlap on the wire (blocks 1..65535 and units 4194305..4259839 both sit
	// inside the height band 1..499999999), so 26280 alone is equally
	// older(26280) and after(26280). Only sha256 digests are carried (the
	// composer emits no other hash); the other hash tags still set Hashlock.
	Timelock      bool
	Hashlock      bool
	Locks         []Lock
	Sha256Digests [][32]byte
	// Sorted is set with K/N: true for sortedmulti/sortedmulti_a, false for
	// multi/multi_a, so §7e's "unsorted where sorted was legal" mark is read
	// from the decoded md1 rather than from builder state.
	Sorted bool
	// Depth is the taptree depth of this leaf (0 for wsh/sh).
	Depth int
}

// PolicyShape is the structural summary of one decoded policy.
type PolicyShape struct {
	// Complete is the honesty contract. FALSE means the walk met a node it
	// could not classify, and the caller MUST NOT present any part of this
	// summary — fall back to the honest-minimal screen.
	Complete bool
	KeyPath  KeyPathKind
	Branches []Branch
	// TapDepth is the deepest leaf; 0 for a non-taproot or key-path-only tr.
	TapDepth int
}

// PolicyShapeChunks decodes an md1 chunk set and summarizes its structure.
func PolicyShapeChunks(strs []string) (PolicyShape, error) {
	d, err := Reassemble(strs)
	if err != nil {
		return PolicyShape{}, err
	}
	return policyShape(d.tree), nil
}

// policyShape walks a decoded tree. It never errors: an unrecognised shape is
// reported as Complete=false, because "I do not understand this" is a RESULT the
// caller must act on, not an exception to swallow.
func policyShape(tree node) PolicyShape {
	s := PolicyShape{Complete: true}
	switch tree.tag {
	case tagTr:
		b, ok := tree.body.(trBody)
		if !ok {
			return PolicyShape{}
		}
		if b.isNums {
			s.KeyPath = KeyPathNUMS
		} else {
			s.KeyPath = KeyPathSpendable
		}
		if b.tree != nil {
			walkTapTree(*b.tree, 1, &s)
		}
	case tagWsh, tagSh:
		b, ok := tree.body.(childrenBody)
		if !ok || len(b.children) != 1 {
			return PolicyShape{}
		}
		inner := b.children[0]
		// sh(wsh(X)) — unwrap one more level so the branch describes the script
		// that actually runs, not the wrapper around it.
		if tree.tag == tagSh && inner.tag == tagWsh {
			ib, ok := inner.body.(childrenBody)
			if !ok || len(ib.children) != 1 {
				return PolicyShape{}
			}
			inner = ib.children[0]
		}
		brs, ok := splitBranches(inner, 0)
		if !ok {
			return PolicyShape{}
		}
		s.Branches = append(s.Branches, brs...)
	default:
		// wpkh/pkh and anything else: a single-key or unclassifiable root.
		brs, ok := splitBranches(tree, 0)
		if !ok {
			return PolicyShape{}
		}
		s.Branches = append(s.Branches, brs...)
	}
	return s
}

// walkTapTree descends a binary taptree, appending one Branch per LEAF. Every
// leaf is visited: a summary that stopped early would hide a spend path.
func walkTapTree(n node, depth int, s *PolicyShape) {
	if n.tag == tagTapTree {
		b, ok := n.body.(childrenBody)
		if !ok || len(b.children) != 2 {
			s.Complete = false
			return
		}
		walkTapTree(b.children[0], depth+1, s)
		walkTapTree(b.children[1], depth+1, s)
		return
	}
	if depth-1 > s.TapDepth {
		s.TapDepth = depth - 1
	}
	brs, ok := splitBranches(n, depth-1)
	if !ok {
		s.Complete = false
		return
	}
	s.Branches = append(s.Branches, brs...)
}

// splitBranches turns a node into one Branch per ALTERNATIVE: or_b/or_c/or_d/
// or_i are two alternatives each (recursively), andor(X,Y,Z) is (X and Y) or
// Z, and anything else is one branch. thresh(k,...) with k < n is also a set
// of alternatives but a combinatorial one; it stays one branch, honestly
// described by collect. ok=false propagates an unknown tag exactly as
// branchOf does.
func splitBranches(n node, depth int) ([]Branch, bool) {
	switch n.tag {
	case tagOrB, tagOrC, tagOrD, tagOrI:
		b, ok := n.body.(childrenBody)
		if !ok || len(b.children) != 2 {
			return nil, false
		}
		left, ok := splitBranches(b.children[0], depth)
		if !ok {
			return nil, false
		}
		right, ok := splitBranches(b.children[1], depth)
		if !ok {
			return nil, false
		}
		return append(left, right...), true
	case tagAndOr:
		b, ok := n.body.(childrenBody)
		if !ok || len(b.children) != 3 {
			return nil, false
		}
		// The X-and-Y half is summarized as a conjunction; the synthetic node is
		// never emitted or encoded, only walked.
		xy := node{tag: tagAndV, body: childrenBody{children: []node{b.children[0], b.children[1]}}}
		left, ok := splitBranches(xy, depth)
		if !ok {
			return nil, false
		}
		right, ok := splitBranches(b.children[2], depth)
		if !ok {
			return nil, false
		}
		return append(left, right...), true
	default:
		br, ok := branchOf(n, depth)
		if !ok {
			return nil, false
		}
		return []Branch{br}, true
	}
}

// branchOf summarizes ONE spend path. ok=false means an unrecognised tag, which
// forces Complete=false upstream.
func branchOf(n node, depth int) (Branch, bool) {
	br := Branch{Depth: depth}
	keys := map[uint8]struct{}{}
	if !collect(n, &br, keys) {
		return Branch{}, false
	}
	br.Keys = len(keys)
	// A bare threshold-over-keys, possibly wrapped: report k-of-N and whether
	// it was the sorted spelling.
	if k, nkeys, sorted, ok := plainMulti(n); ok {
		br.K, br.N, br.Sorted = k, nkeys, sorted
	}
	return br, true
}

// plainMulti unwraps wrappers to find a bare multi/sortedmulti/multi_a/
// sortedmulti_a. Anything else returns ok=false, so K/N stay zero rather than
// being invented for a shape that is not a plain threshold over keys.
func plainMulti(n node) (int, int, bool, bool) {
	for {
		switch n.tag {
		case tagMulti, tagSortedMulti, tagMultiA, tagSortedMultiA:
			if b, ok := n.body.(multiKeysBody); ok {
				return int(b.k), len(b.indices), n.tag == tagSortedMulti || n.tag == tagSortedMultiA, true
			}
			return 0, 0, false, false
		case tagCheck, tagVerify, tagSwap, tagAlt, tagDupIf, tagNonZero, tagZeroNotEqual:
			b, ok := n.body.(childrenBody)
			if !ok || len(b.children) != 1 {
				return 0, 0, false, false
			}
			n = b.children[0]
		default:
			return 0, 0, false, false
		}
	}
}

// collect walks one branch, recording key references and the presence of
// time/hash locks. It returns false on ANY tag it does not know — the whole
// point of Complete.
func collect(n node, br *Branch, keys map[uint8]struct{}) bool {
	switch n.tag {
	case tagPkK, tagPkH, tagPkh, tagRawPkH, tagWpkh:
		if b, ok := n.body.(keyArgBody); ok {
			keys[b.index] = struct{}{}
			return true
		}
		// A raw key hash carries no placeholder; it is still a known shape.
		return true
	case tagMulti, tagSortedMulti, tagMultiA, tagSortedMultiA:
		b, ok := n.body.(multiKeysBody)
		if !ok {
			return false
		}
		for _, i := range b.indices {
			keys[i] = struct{}{}
		}
		return true
	case tagAfter, tagOlder:
		br.Timelock = true
		if v, ok := n.body.(timelockBody); ok {
			br.Locks = append(br.Locks, lockFromWire(n.tag, uint32(v)))
		}
		return true
	case tagSha256:
		br.Hashlock = true
		if h, ok := n.body.(hash256Body); ok {
			br.Sha256Digests = append(br.Sha256Digests, [32]byte(h))
		}
		return true
	case tagHash256, tagRipemd160, tagHash160:
		br.Hashlock = true
		return true
	case tagTrue, tagFalse:
		return true
	case tagThresh:
		b, ok := n.body.(variableBody)
		if !ok {
			return false
		}
		for _, c := range b.children {
			if !collect(c, br, keys) {
				return false
			}
		}
		return true
	case tagAndB, tagAndV, tagAndOr, tagOrB, tagOrC, tagOrD, tagOrI,
		tagCheck, tagVerify, tagSwap, tagAlt, tagDupIf, tagNonZero, tagZeroNotEqual,
		tagWsh, tagSh:
		b, ok := n.body.(childrenBody)
		if !ok {
			return false
		}
		for _, c := range b.children {
			if !collect(c, br, keys) {
				return false
			}
		}
		return true
	default:
		// tagTr and tagTapTree cannot appear inside a branch; anything else is
		// a tag this walk has not been taught. Refuse rather than guess.
		return false
	}
}
