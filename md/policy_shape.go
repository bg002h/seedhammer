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
	// KeyPathLianaUnspendable — wire kind 1 (F-449): Liana's unspendable xpub,
	// derived from the leaf keys (SPEC §2). Script paths only, exactly as
	// KeyPathNUMS, and a DIFFERENT WALLET from the same tree at KeyPathNUMS:
	// different addresses, different ids. Named as md-codec 0.47.0 names it
	// (policy_shape.rs KeyPathKind::LianaUnspendable), not the spec's earlier
	// "NumsXpub". APPENDED, so the three older values keep their numbers.
	KeyPathLianaUnspendable
)

// keyPathOf is the ONE mapping from a taproot internal key to its KeyPathKind,
// shared by policyShape and summarize so the inspect screen and the consent
// screen cannot disagree about which kind a card carries. ok=false for a kind
// this port has no name for: the caller must treat the policy as one it cannot
// describe, never guess a neighbouring kind (SPEC §7a row 1).
func keyPathOf(ik InternalKeyKind) (KeyPathKind, bool) {
	switch ik {
	case InternalKeySlot:
		return KeyPathSpendable, true
	case InternalKeyNUMS:
		return KeyPathNUMS, true
	case InternalKeyLianaUnspendable:
		return KeyPathLianaUnspendable, true
	}
	return KeyPathNone, false
}

// Branch is one independently satisfiable spend path: a tapscript leaf, or the
// whole script for wsh/sh.
type Branch struct {
	// K, N are the threshold of the ONE multi/sortedmulti/multi_a/
	// sortedmulti_a node the branch contains, at any depth, AND ONLY WHEN
	// THAT MULTI ACCOUNTS FOR EVERY KEY THE BRANCH REFERENCES. The first half
	// is fable review r0 I-3 — a threshold behind a timelock or a hash must
	// report its k-of-n, which is what §7e's consent and composerSelfCheck
	// compare; the second is its round-1 review I-2 — a branch holding a
	// multi PLUS another key has no single k-of-n, and reporting the multi's
	// would say two of three signatures are needed where one other key spends
	// alone.
	//
	// Zero therefore means: no threshold node, more than one, or one that
	// does not account for the branch's keys. It does NOT mean "1-of-1",
	// which §5 lowers to pk and which Keys == 1 reports.
	K, N int
	// Keys counts distinct key placeholders the branch references. It is set
	// even when K/N are not, so a branch is never reported as keyless. For an
	// any-of entry (AnyN > 0) it counts the key CONDITIONS, none of which is
	// individually required.
	Keys int
	// Timelock/Hashlock report whether the branch requires one ANYWHERE within
	// it -- for a conjunction. For an any-of entry (AnyN > 0) Timelock and
	// Locks report timelock CONDITIONS, any AnyK of which (with the keys)
	// satisfy it, so a lock listed there may never be needed; Hashlock is
	// false and Hashlocks empty.
	//
	// Locks and Hashlocks carry the values, in wire order, so the
	// composer's consent surface (SPEC_wallet_policy_composer.md §7e) can say
	// "older 26280 blocks" instead of "time-locked"; a value is a fact from the
	// decoded tree, not a rendering, so the no-text rule above still holds. A
	// Lock carries its KIND, because the bare operand cannot: §4c's bands
	// overlap on the wire (blocks 1..65535 and units 4194305..4259839 both sit
	// inside the height band 1..499999999), so 26280 alone is equally
	// older(26280) and after(26280). Only sha256 digests are carried (the
	// composer emits no other hash); the other hash tags still set Hashlock.
	Timelock bool
	Hashlock bool
	Locks    []Lock
	// Hashlocks carries EVERY hashlock, whatever its kind (SPEC_hashlock_kinds
	// §7.4). It was Sha256Digests [][32]byte and recorded a digest only for
	// tagSha256 while setting Hashlock=true for all four -- so a decoded
	// ripemd160 card showed "hashlock" with NO digest, and the composer's
	// self-check then refused the wallet for having one hashlock in the shape
	// and zero decoded. Fail-closed, but it meant no non-sha256 wallet could be
	// composed on the device at all.
	//
	// This is a DECODE-SIDE behaviour change and §7.4 says to declare it as
	// one: policy_shape runs on payloads this device never composed.
	Hashlocks []*HashLock
	// Sorted is set with K/N: true for sortedmulti/sortedmulti_a, false for
	// multi/multi_a, so §7e's "unsorted where sorted was legal" mark is read
	// from the decoded md1 rather than from builder state.
	Sorted bool
	// Depth is the taptree depth of this leaf (0 for wsh/sh).
	Depth int
	// AnyK, AnyN: READ THESE FIRST (md-codec 0.49.0 Branch::any_of, seedhammer
	// I-1). Both zero: the branch is a conjunction, and every key, lock and
	// hashlock it lists is REQUIRED (K/N describe its one multi, if any).
	// AnyN > 0: the branch is a thresh(AnyK, c_1..c_AnyN) with
	// 1 <= AnyK < AnyN at a path root, every child spelled as one allowlisted
	// key or timelock (threshConditions). Keys and Locks are then its AnyN
	// CONDITIONS, any AnyK of which satisfy it, and none of them is
	// individually required; K, N, Sorted are zero and Hashlocks is empty. A
	// reader that ignores these repeats the defect they fix: a 2-of-3 shown
	// as needing all three.
	AnyK, AnyN int
}

// AnyOfMinKeys is, for an any-of entry (AnyN > 0), the fewest keys any
// satisfying choice needs: AnyK less the timelock conditions, floored at zero
// (Rust Branch::any_of_min_keys). Timelock conditions are counted as
// AnyN - Keys, which equals len(Locks) because the allowlist admits only
// well-formed timelock bodies. It assumes every lock can be met, so where two
// cannot both be (a height and a time) it errs toward FEWER keys: a warning,
// never an overstatement. Zero for a conjunction is NOT a claim; callers test
// AnyN first.
func (b Branch) AnyOfMinKeys() int {
	if b.AnyN == 0 {
		return 0
	}
	locks := b.AnyN - b.Keys
	if locks >= b.AnyK {
		return 0
	}
	return b.AnyK - locks
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
		kp, known := keyPathOf(b.ik)
		if !known {
			// THE HONESTY CONTRACT, applied to the key path: a summary that
			// named a kind it does not know would describe a different wallet.
			return PolicyShape{}
		}
		s.KeyPath = kp
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
		brs, ok := splitBranches(inner, 0, false)
		if !ok {
			return PolicyShape{}
		}
		s.Branches = append(s.Branches, brs...)
	default:
		// wpkh/pkh and anything else: a single-key or unclassifiable root.
		brs, ok := splitBranches(tree, 0, false)
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
	brs, ok := splitBranches(n, depth-1, false)
	if !ok {
		s.Complete = false
		return
	}
	s.Branches = append(s.Branches, brs...)
}

// splitBranches turns a node into one Branch per ALTERNATIVE: or_b/or_c/or_d/
// or_i are two alternatives each (recursively), andor(X,Y,Z) is (X and Y) or
// Z, and anything else is one branch. thresh(k,...) with k < n is also a set
// of alternatives but a combinatorial one; at a path root with every child on
// the allowlist it is ONE any-of entry (branchOf), anywhere else it is
// refused (md-codec 0.49.0, seedhammer I-1). ok=false propagates an unknown
// tag exactly as branchOf does.
//
// orBSecond is true only for the second child of an or_b: the one place the
// type system puts a wrapped (a:) thresh at a path root.
func splitBranches(n node, depth int, orBSecond bool) ([]Branch, bool) {
	switch n.tag {
	case tagOrB, tagOrC, tagOrD, tagOrI:
		b, ok := n.body.(childrenBody)
		if !ok || len(b.children) != 2 {
			return nil, false
		}
		left, ok := splitBranches(b.children[0], depth, false)
		if !ok {
			return nil, false
		}
		right, ok := splitBranches(b.children[1], depth, n.tag == tagOrB)
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
		left, ok := splitBranches(xy, depth, false)
		if !ok {
			return nil, false
		}
		right, ok := splitBranches(b.children[2], depth, false)
		if !ok {
			return nil, false
		}
		return append(left, right...), true
	default:
		br, ok := branchOf(n, depth, orBSecond)
		if !ok {
			return nil, false
		}
		return []Branch{br}, true
	}
}

// branchOf summarizes ONE spend path. ok=false means an unrecognised tag, or a
// thresh this summary refuses, which forces Complete=false upstream.
func branchOf(n node, depth int, orBSecond bool) (Branch, bool) {
	if root, ok := anyOfRoot(n, orBSecond); ok {
		b, ok := root.body.(variableBody)
		if !ok {
			return Branch{}, false
		}
		if int(b.k) != len(b.children) {
			// md-codec 0.49.0 (seedhammer I-1): k < n is a disjunction. One
			// any-of entry when every child is on the allowlist and the key
			// slots are distinct; otherwise refused. Before this, the thresh
			// fell to collect below and was summarised as a CONJUNCTION of all
			// its children: a 2-of-3 shown as needing all three keys.
			if b.k == 0 || int(b.k) > len(b.children) {
				return Branch{}, false
			}
			slots, locks, ok := threshConditions(b.children)
			if !ok {
				return Branch{}, false
			}
			seen := map[uint8]struct{}{}
			for _, i := range slots {
				if _, dup := seen[i]; dup {
					return Branch{}, false
				}
				seen[i] = struct{}{}
			}
			return Branch{
				Depth:    depth,
				Keys:     len(slots),
				Timelock: len(locks) > 0,
				Locks:    locks,
				AnyK:     int(b.k),
				AnyN:     len(b.children),
			}, true
		}
	}
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
	} else if k, nkeys, sorted, ok := soleMulti(n); ok && nkeys == br.Keys {
		// A THRESHOLD BEHIND A LOCK OR A HASH IS STILL A THRESHOLD (fable
		// review r0 I-3). §5 lowers a multi-key path carrying a timelock to
		// and_v(v:multi(k,...),older(n)) and one carrying a hash deeper still,
		// and plainMulti looks through WRAPPERS but not through and_v -- so
		// K/N stayed zero for exactly the paths whose threshold an operator
		// most needs read back. The consent printed "2 key(s), custom" for a
		// 1-of-2 recovery tier and for a 9-of-9, identically; §7e requires it
		// to "name, per path in listed order: its k-of-n or single key", and
		// composerSelfCheck fell back to comparing the key COUNT there, so a
		// mis-tapped k was invisible on both the promise and the check.
		//
		// TWO CONDITIONS, AND THE SECOND IS ROUND-1 REVIEW I-2. The walk
		// requires the branch to contain EXACTLY ONE multi node -- two
		// thresholds in one branch have no single k-of-n -- AND requires that
		// multi to account for EVERY key the branch references (`nkeys ==
		// br.Keys`).
		//
		// Without the second, a branch holding one multi PLUS other key
		// material reported the multi's threshold as the BRANCH's.
		// splitBranches splits or_*/andor but not an `or` sitting under and_v
		// or a wrapper, so that branch is constructible and the wire carries
		// it: wsh(and_v(v:older(10), or_d(multi(2,@0,@1,@2), c:pk_k(@3))))
		// reported 2-of-3 for a branch @3 can spend ALONE after 10 blocks --
		// the exact misreading the anti-invention rule below names, arriving
		// through the arm that was widened to fix a different one.
		//
		// `br.Keys` is DISTINCT placeholders and `nkeys` is the multi's index
		// count, so the equality is "the branch's keys are exactly this
		// threshold's keys". A multi that repeated a slot would make nkeys
		// exceed br.Keys and fall to 0/0, which is the honest answer for a
		// policy md.DuplicateKeySlot refuses anyway.
		br.K, br.N, br.Sorted = k, nkeys, sorted
	}
	return br, true
}

// soleMulti reports the threshold of the ONE multi/sortedmulti/multi_a/
// sortedmulti_a node a branch contains, at any depth. ok is false when the
// branch has none or more than one.
//
// IT IS NOT SUFFICIENT ON ITS OWN: the caller also requires the multi's key
// count to equal the branch's distinct-placeholder count, because a branch
// can hold one multi and other key material beside it (round-1 review I-2).
// "The one multi" and "the branch's threshold" are different questions, and
// this function answers only the first.
func soleMulti(n node) (k, nkeys int, sorted, ok bool) {
	var found int
	var walk func(node)
	walk = func(n node) {
		switch n.tag {
		case tagMulti, tagSortedMulti, tagMultiA, tagSortedMultiA:
			found++
			if b, ok := n.body.(multiKeysBody); ok {
				k, nkeys = int(b.k), len(b.indices)
				sorted = n.tag == tagSortedMulti || n.tag == tagSortedMultiA
			} else {
				// A malformed body is not a threshold this can report.
				found++
			}
			return
		}
		switch b := n.body.(type) {
		case childrenBody:
			for _, c := range b.children {
				walk(c)
			}
		case variableBody:
			for _, c := range b.children {
				walk(c)
			}
		}
	}
	walk(n)
	if found != 1 {
		return 0, 0, false, false
	}
	return k, nkeys, sorted, true
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
	case tagSha256, tagHash256, tagRipemd160, tagHash160:
		br.Hashlock = true
		// Every kind records its digest, at its own width. Switched on the tag
		// with no default so a fifth hash tag is a missing case here rather
		// than a silently dropped digest.
		var kind HashKind
		switch n.tag {
		case tagSha256:
			kind = KindSha256
		case tagHash256:
			kind = KindHash256
		case tagRipemd160:
			kind = KindRipemd160
		case tagHash160:
			kind = KindHash160
		}
		switch h := n.body.(type) {
		case hash256Body:
			if l, ok := NewHashLock(kind, h[:]); ok {
				br.Hashlocks = append(br.Hashlocks, l)
			}
		case hash160Body:
			if l, ok := NewHashLock(kind, h[:]); ok {
				br.Hashlocks = append(br.Hashlocks, l)
			}
		}
		return true
	case tagTrue, tagFalse:
		return true
	case tagThresh:
		// md-codec 0.49.0 (seedhammer I-1): inside a path only k == n is a
		// conjunction, and only when every child is on the allowlist -- a child
		// outside it can feed the ADD chain a number other than 0 or 1, so
		// wsh(thresh(3,pk(@0),s:pk(@1),av:pk(@2))), which @2 spends ALONE, was
		// summarised as a three-key conjunction. A k < n thresh that is not a
		// path root has no single-entry summary. Children are not walked with
		// collect.
		b, ok := n.body.(variableBody)
		if !ok || int(b.k) != len(b.children) {
			return false
		}
		slots, locks, ok := threshConditions(b.children)
		if !ok {
			return false
		}
		for _, i := range slots {
			keys[i] = struct{}{}
		}
		if len(locks) > 0 {
			br.Timelock = true
			br.Locks = append(br.Locks, locks...)
		}
		return true
	case tagAndOr, tagOrB, tagOrC, tagOrD, tagOrI:
		// A DISJUNCTION INSIDE A BRANCH (port of md-codec 0.48.5's rule,
		// policy_shape.rs `collect`). splitBranches splits an `or` only at the
		// top of a branch; one under and_v or a wrapper used to be walked here
		// like a conjunction, merging both alternatives into ONE Branch and
		// leaving Complete true. wsh(and_v(v:pk(@0),or_d(pk(@1),older(52560))))
		// then summarised as "Spend path: 1 / 2 key(s), custom +timelock" while
		// @0 can spend ALONE after the lock (bug hunt 2026-11-25 SH#2). Showing
		// every alternative would need a cross product of branches; the summary
		// refuses instead, so the consent falls to its honest-minimal screen.
		//
		// The one exception is Rust's too: `l:X` = or_i(0,X), `u:X` =
		// or_i(X,0) and `and_n(X,Y)` = andor(X,Y,0) carry a `0` alternative
		// that is never satisfiable, so they are not disjunctions.
		b, ok := n.body.(childrenBody)
		if !ok {
			return false
		}
		zeroArm := false
		switch n.tag {
		case tagOrI:
			for _, c := range b.children {
				if c.tag == tagFalse {
					zeroArm = true
				}
			}
		case tagAndOr:
			zeroArm = len(b.children) > 2 && b.children[2].tag == tagFalse
		}
		if !zeroArm {
			return false
		}
		for _, c := range b.children {
			if !collect(c, br, keys) {
				return false
			}
		}
		return true
	case tagAndB, tagAndV,
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

// anyOfRoot is the thresh an any-of entry is rooted at: n itself, or, only as
// the second child of an or_b, the thresh under an a:. No other wrapper is
// looked through (Rust any_of_root).
func anyOfRoot(n node, orBSecond bool) (node, bool) {
	if n.tag == tagThresh {
		return n, true
	}
	if orBSecond && n.tag == tagAlt {
		if c, ok := soleChild(n); ok && c.tag == tagThresh {
			return c, true
		}
	}
	return node{}, false
}

// threshConditions is the thresh child ALLOWLIST (md-codec 0.49.0
// thresh_conditions; DESIGN_thresh_summary section 1). It returns the key slots
// and the locks, each in wire order, when EVERY child is spelled exactly as one
// listed form, and ok=false otherwise. Each listed spelling is one the
// miniscript type rules make leave exactly 0 or 1 on the stack (BIP-379:
// "X_1 is Bdu; others are Wdu"), so "k of n" is exact. Nothing that is not on
// the list is looked through: this port has no type checker, so the list IS
// the type check.
//
//   - X_1: KEY, or LOCK.
//   - later: s: over a pk KEY (never pkh: it is not "o"), a: over any KEY,
//     s: or a: over a LOCK.
func threshConditions(children []node) (slots []uint8, locks []Lock, ok bool) {
	if len(children) == 0 {
		return nil, nil, false
	}
	take := func(c node, allowPkH bool) bool {
		if slot, isPkH, ok := threshKey(c); ok {
			if isPkH && !allowPkH {
				return false
			}
			slots = append(slots, slot)
			return true
		}
		l, ok := threshLock(c)
		if !ok {
			return false
		}
		locks = append(locks, l)
		return true
	}
	if !take(children[0], true) {
		return nil, nil, false
	}
	for _, c := range children[1:] {
		inner, ok := soleChild(c)
		if !ok {
			return nil, nil, false
		}
		switch c.tag {
		case tagSwap:
			ok = take(inner, false)
		case tagAlt:
			ok = take(inner, true)
		default:
			ok = false
		}
		if !ok {
			return nil, nil, false
		}
	}
	return slots, locks, true
}

// soleChild is the one child of a one-child node.
func soleChild(n node) (node, bool) {
	b, ok := n.body.(childrenBody)
	if !ok || len(b.children) != 1 {
		return node{}, false
	}
	return b.children[0], true
}

// threshKey is KEY: pk_k/pk_h over a placeholder, bare (the wire's spelling of
// pk/pkh) or under exactly one c:. It reports the slot and whether it is pkh.
func threshKey(n node) (slot uint8, isPkH, ok bool) {
	if n.tag == tagCheck {
		if n, ok = soleChild(n); !ok {
			return 0, false, false
		}
	}
	if n.tag != tagPkK && n.tag != tagPkH {
		return 0, false, false
	}
	b, ok := n.body.(keyArgBody)
	if !ok {
		return 0, false, false
	}
	return b.index, n.tag == tagPkH, true
}

// threshTimelock is T: after/older with an operand 1 <= v < 2^31 (BIP-379).
func threshTimelock(n node) (Lock, bool) {
	if n.tag != tagAfter && n.tag != tagOlder {
		return Lock{}, false
	}
	v, ok := n.body.(timelockBody)
	if !ok || v < 1 || uint32(v) >= 1<<31 {
		return Lock{}, false
	}
	return lockFromWire(n.tag, uint32(v)), true
}

// zeroArmOther is the non-0 child of an or_i with exactly two children, exactly
// one of them 0.
func zeroArmOther(n node) (node, bool) {
	if n.tag != tagOrI {
		return node{}, false
	}
	b, ok := n.body.(childrenBody)
	if !ok || len(b.children) != 2 {
		return node{}, false
	}
	z0, z1 := b.children[0].tag == tagFalse, b.children[1].tag == tagFalse
	switch {
	case z0 && !z1:
		return b.children[1], true
	case z1 && !z0:
		return b.children[0], true
	}
	return node{}, false
}

// threshLock is LOCK: ln:T / un:T (or_i with 0, over n:T) or nl:T / nu:T (n:
// over or_i with 0, over T).
func threshLock(n node) (Lock, bool) {
	switch n.tag {
	case tagOrI:
		other, ok := zeroArmOther(n)
		if !ok || other.tag != tagZeroNotEqual {
			return Lock{}, false
		}
		t, ok := soleChild(other)
		if !ok {
			return Lock{}, false
		}
		return threshTimelock(t)
	case tagZeroNotEqual:
		c, ok := soleChild(n)
		if !ok {
			return Lock{}, false
		}
		t, ok := zeroArmOther(c)
		if !ok {
			return Lock{}, false
		}
		return threshTimelock(t)
	}
	return Lock{}, false
}
