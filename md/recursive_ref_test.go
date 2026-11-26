package md

import btcaddr "github.com/btcsuite/btcd/address/v2"

// The recursive tree walks as they stood before review r3 I-P1 made them
// iterative: the decoder, validators, encoder and shape walk at 55d31dd, and
// the script emitter and the other card entry points' walks at caa93e1.
// Test-only: iterative_equiv_test.go holds the iterative versions to these,
// result for result and error for error. Only the names changed (a Rec
// suffix); keep the bodies as they were, so the comparison stays against the
// code that shipped.

func readNodeDepthRec(r *bitReader, kiw uint8, wireVersion uint8, depth uint8) (node, error) {
	if depth >= maxDecodeDepth {
		return node{}, errDepthExceeded
	}
	t, err := readTag(r)
	if err != nil {
		return node{}, err
	}
	var b body
	switch t {
	case tagPkK, tagPkH, tagWpkh, tagPkh:
		idx, err := r.read(int(kiw))
		if err != nil {
			return node{}, err
		}
		b = keyArgBody{index: uint8(idx)}
	case tagSh, tagWsh, tagCheck, tagVerify, tagSwap, tagAlt, tagDupIf, tagNonZero, tagZeroNotEqual:
		child, err := readNodeDepthRec(r, kiw, wireVersion, depth+1)
		if err != nil {
			return node{}, err
		}
		b = childrenBody{children: []node{child}}
	case tagAndV, tagAndB, tagOrB, tagOrC, tagOrD, tagOrI:
		l, err := readNodeDepthRec(r, kiw, wireVersion, depth+1)
		if err != nil {
			return node{}, err
		}
		r2, err := readNodeDepthRec(r, kiw, wireVersion, depth+1)
		if err != nil {
			return node{}, err
		}
		b = childrenBody{children: []node{l, r2}}
	case tagAndOr:
		a, err := readNodeDepthRec(r, kiw, wireVersion, depth+1)
		if err != nil {
			return node{}, err
		}
		bb, err := readNodeDepthRec(r, kiw, wireVersion, depth+1)
		if err != nil {
			return node{}, err
		}
		c, err := readNodeDepthRec(r, kiw, wireVersion, depth+1)
		if err != nil {
			return node{}, err
		}
		b = childrenBody{children: []node{a, bb, c}}
	case tagTapTree:
		l, err := readNodeDepthRec(r, kiw, wireVersion, depth+1)
		if err != nil {
			return node{}, err
		}
		r2, err := readNodeDepthRec(r, kiw, wireVersion, depth+1)
		if err != nil {
			return node{}, err
		}
		b = childrenBody{children: []node{l, r2}}
	case tagMulti, tagSortedMulti, tagMultiA, tagSortedMultiA:
		kRaw, err := r.read(5)
		if err != nil {
			return node{}, err
		}
		countRaw, err := r.read(5)
		if err != nil {
			return node{}, err
		}
		k := uint8(kRaw) + 1
		count := int(countRaw) + 1
		if int(k) > count {
			return node{}, errKGreaterThanN
		}
		indices := make([]uint8, 0, count)
		for i := 0; i < count; i++ {
			idx, err := r.read(int(kiw))
			if err != nil {
				return node{}, err
			}
			indices = append(indices, uint8(idx))
		}
		b = multiKeysBody{k: k, indices: indices}
	case tagThresh:
		kRaw, err := r.read(5)
		if err != nil {
			return node{}, err
		}
		countRaw, err := r.read(5)
		if err != nil {
			return node{}, err
		}
		k := uint8(kRaw) + 1
		count := int(countRaw) + 1
		if int(k) > count {
			return node{}, errKGreaterThanN
		}
		children := make([]node, 0, count)
		for i := 0; i < count; i++ {
			c, err := readNodeDepthRec(r, kiw, wireVersion, depth+1)
			if err != nil {
				return node{}, err
			}
			children = append(children, c)
		}
		b = variableBody{k: k, children: children}
	case tagTr:
		// is_nums(1) | [kind(1) iff is_nums && v==8] | [key_index(kiw) iff
		// !is_nums] | has_tree(1) | [tree] -- tree.rs read_node's Tr arm.
		isNums, err := r.readBool()
		if err != nil {
			return node{}, err
		}
		ik := InternalKeySlot
		if isNums {
			ik = InternalKeyNUMS
			if wireVersion == wfUnspendableVersion {
				kind, err := r.readBool()
				if err != nil {
					return node{}, err
				}
				if kind { // SPEC §3d: kind bit 1 = Liana unspendable, 0 = NUMS.
					ik = InternalKeyLianaUnspendable
				}
			}
		}
		var keyIndex uint8
		if !isNums {
			idx, err := r.read(int(kiw))
			if err != nil {
				return node{}, err
			}
			keyIndex = uint8(idx)
		}
		hasTree, err := r.readBool()
		if err != nil {
			return node{}, err
		}
		var sub *node
		if hasTree {
			child, err := readNodeDepthRec(r, kiw, wireVersion, depth+1)
			if err != nil {
				return node{}, err
			}
			sub = &child
		}
		b = trBody{ik: ik, keyIndex: keyIndex, tree: sub}
	case tagAfter, tagOlder:
		v, err := r.read(32)
		if err != nil {
			return node{}, err
		}
		b = timelockBody(uint32(v))
	case tagSha256, tagHash256:
		var h hash256Body
		for i := range h {
			v, err := r.read(8)
			if err != nil {
				return node{}, err
			}
			h[i] = uint8(v)
		}
		b = h
	case tagHash160, tagRipemd160, tagRawPkH:
		var h hash160Body
		for i := range h {
			v, err := r.read(8)
			if err != nil {
				return node{}, err
			}
			h[i] = uint8(v)
		}
		b = h
	case tagFalse, tagTrue:
		b = emptyBody{}
	default:
		return node{}, errTagOutOfRange
	}
	return node{tag: t, body: b}, nil
}

func walkShapeRec(tree node) PolicyShape {
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
			walkTapTreeRec(*b.tree, 1, &s)
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
		brs, ok := splitBranchesRec(inner, 0, false)
		if !ok {
			return PolicyShape{}
		}
		s.Branches = append(s.Branches, brs...)
	default:
		// wpkh/pkh and anything else: a single-key or unclassifiable root.
		brs, ok := splitBranchesRec(tree, 0, false)
		if !ok {
			return PolicyShape{}
		}
		s.Branches = append(s.Branches, brs...)
	}
	return s
}

func walkTapTreeRec(n node, depth int, s *PolicyShape) {
	if n.tag == tagTapTree {
		b, ok := n.body.(childrenBody)
		if !ok || len(b.children) != 2 {
			s.Complete = false
			return
		}
		walkTapTreeRec(b.children[0], depth+1, s)
		walkTapTreeRec(b.children[1], depth+1, s)
		return
	}
	if depth-1 > s.TapDepth {
		s.TapDepth = depth - 1
	}
	brs, ok := splitBranchesRec(n, depth-1, false)
	if !ok {
		s.Complete = false
		return
	}
	s.Branches = append(s.Branches, brs...)
}

func splitBranchesRec(n node, depth int, orBSecond bool) ([]Branch, bool) {
	switch n.tag {
	case tagOrB, tagOrC, tagOrD, tagOrI:
		b, ok := n.body.(childrenBody)
		if !ok || len(b.children) != 2 {
			return nil, false
		}
		left, ok := splitBranchesRec(b.children[0], depth, false)
		if !ok {
			return nil, false
		}
		right, ok := splitBranchesRec(b.children[1], depth, n.tag == tagOrB)
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
		left, ok := splitBranchesRec(xy, depth, false)
		if !ok {
			return nil, false
		}
		right, ok := splitBranchesRec(b.children[2], depth, false)
		if !ok {
			return nil, false
		}
		return append(left, right...), true
	default:
		br, ok := branchOfRec(n, depth, orBSecond)
		if !ok {
			return nil, false
		}
		return []Branch{br}, true
	}
}

func branchOfRec(n node, depth int, orBSecond bool) (Branch, bool) {
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
	if !collectRec(n, &br, keys) {
		return Branch{}, false
	}
	br.Keys = len(keys)
	// A bare threshold-over-keys, possibly wrapped: report k-of-N and whether
	// it was the sorted spelling.
	if k, nkeys, sorted, ok := plainMulti(n); ok {
		br.K, br.N, br.Sorted = k, nkeys, sorted
	} else if k, nkeys, sorted, ok := soleMultiRec(n); ok && nkeys == br.Keys {
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

func soleMultiRec(n node) (k, nkeys int, sorted, ok bool) {
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

func collectRec(n node, br *Branch, keys map[uint8]struct{}) bool {
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
			if !collectRec(c, br, keys) {
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
			if !collectRec(c, br, keys) {
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

func walkForPlaceholdersRec(n node, seen []bool, firstOccurrences *[]uint8) error {
	switch b := n.body.(type) {
	case keyArgBody:
		if int(b.index) >= len(seen) {
			return errPlaceholderRange
		}
		if !seen[b.index] {
			seen[b.index] = true
			*firstOccurrences = append(*firstOccurrences, b.index)
		}
	case childrenBody:
		for _, c := range b.children {
			if err := walkForPlaceholdersRec(c, seen, firstOccurrences); err != nil {
				return err
			}
		}
	case variableBody:
		for _, c := range b.children {
			if err := walkForPlaceholdersRec(c, seen, firstOccurrences); err != nil {
				return err
			}
		}
	case multiKeysBody:
		for _, index := range b.indices {
			if int(index) >= len(seen) {
				return errPlaceholderRange
			}
			if !seen[index] {
				seen[index] = true
				*firstOccurrences = append(*firstOccurrences, index)
			}
		}
	case trBody:
		if !b.isNums() {
			if int(b.keyIndex) >= len(seen) {
				return errNUMSConflict
			}
			if !seen[b.keyIndex] {
				seen[b.keyIndex] = true
				*firstOccurrences = append(*firstOccurrences, b.keyIndex)
			}
		}
		if b.tree != nil {
			if err := walkForPlaceholdersRec(*b.tree, seen, firstOccurrences); err != nil {
				return err
			}
		}
	case hash256Body, hash160Body, timelockBody, emptyBody:
	}
	return nil
}

func validateTapScriptTreeRec(n node) error {
	if n.tag == tagTapTree {
		if b, ok := n.body.(childrenBody); ok {
			for _, c := range b.children {
				if err := validateTapScriptTreeRec(c); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if isForbiddenLeafTag(n.tag) {
		return errForbiddenTapLeaf
	}
	return nil
}

func walkCollectFirstRec(n node, seen []bool, firstOccurrences *[]uint8) {
	switch b := n.body.(type) {
	case keyArgBody:
		if int(b.index) < len(seen) && !seen[b.index] {
			seen[b.index] = true
			*firstOccurrences = append(*firstOccurrences, b.index)
		}
	case trBody:
		// SPEC §7: is_nums=true → internal key is the NUMS H-point, not a
		// placeholder reference; skip registration.
		if !b.isNums() {
			if int(b.keyIndex) < len(seen) && !seen[b.keyIndex] {
				seen[b.keyIndex] = true
				*firstOccurrences = append(*firstOccurrences, b.keyIndex)
			}
		}
		if b.tree != nil {
			walkCollectFirstRec(*b.tree, seen, firstOccurrences)
		}
	case childrenBody:
		for _, c := range b.children {
			walkCollectFirstRec(c, seen, firstOccurrences)
		}
	case variableBody:
		for _, c := range b.children {
			walkCollectFirstRec(c, seen, firstOccurrences)
		}
	case multiKeysBody:
		for _, idx := range b.indices {
			if int(idx) < len(seen) && !seen[idx] {
				seen[idx] = true
				*firstOccurrences = append(*firstOccurrences, idx)
			}
		}
	case hash256Body, hash160Body, timelockBody, emptyBody:
	}
}

func remapIndicesRec(n node, perm []uint8) node {
	switch b := n.body.(type) {
	case keyArgBody:
		return node{tag: n.tag, body: keyArgBody{index: perm[b.index]}}
	case trBody:
		nb := trBody{ik: b.ik, keyIndex: b.keyIndex}
		if !b.isNums() {
			nb.keyIndex = perm[b.keyIndex]
		}
		if b.tree != nil {
			t := remapIndicesRec(*b.tree, perm)
			nb.tree = &t
		}
		return node{tag: n.tag, body: nb}
	case childrenBody:
		children := make([]node, len(b.children))
		for i, c := range b.children {
			children[i] = remapIndicesRec(c, perm)
		}
		return node{tag: n.tag, body: childrenBody{children: children}}
	case variableBody:
		children := make([]node, len(b.children))
		for i, c := range b.children {
			children[i] = remapIndicesRec(c, perm)
		}
		return node{tag: n.tag, body: variableBody{k: b.k, children: children}}
	case multiKeysBody:
		indices := make([]uint8, len(b.indices))
		for i, idx := range b.indices {
			indices[i] = perm[idx]
		}
		return node{tag: n.tag, body: multiKeysBody{k: b.k, indices: indices}}
	default:
		// hash256Body, hash160Body, timelockBody, emptyBody — no indices.
		return n
	}
}

func checkPlaceholderBoundsRec(n node, max uint8) error {
	switch b := n.body.(type) {
	case keyArgBody:
		if b.index >= max {
			return errPlaceholderRange
		}
	case trBody:
		if !b.isNums() && b.keyIndex >= max {
			return errPlaceholderRange
		}
		if b.tree != nil {
			if err := checkPlaceholderBoundsRec(*b.tree, max); err != nil {
				return err
			}
		}
	case childrenBody:
		for _, c := range b.children {
			if err := checkPlaceholderBoundsRec(c, max); err != nil {
				return err
			}
		}
	case variableBody:
		for _, c := range b.children {
			if err := checkPlaceholderBoundsRec(c, max); err != nil {
				return err
			}
		}
	case multiKeysBody:
		for _, idx := range b.indices {
			if idx >= max {
				return errPlaceholderRange
			}
		}
	case hash256Body, hash160Body, timelockBody, emptyBody:
	}
	return nil
}

func cloneNodeRec(n node) node {
	switch b := n.body.(type) {
	case childrenBody:
		children := make([]node, len(b.children))
		for i, c := range b.children {
			children[i] = cloneNodeRec(c)
		}
		return node{tag: n.tag, body: childrenBody{children: children}}
	case variableBody:
		children := make([]node, len(b.children))
		for i, c := range b.children {
			children[i] = cloneNodeRec(c)
		}
		return node{tag: n.tag, body: variableBody{k: b.k, children: children}}
	case multiKeysBody:
		return node{tag: n.tag, body: multiKeysBody{k: b.k, indices: cloneSlice(b.indices)}}
	case trBody:
		nb := trBody{ik: b.ik, keyIndex: b.keyIndex}
		if b.tree != nil {
			t := cloneNodeRec(*b.tree)
			nb.tree = &t
		}
		return node{tag: n.tag, body: nb}
	default:
		// keyArgBody, hash256Body, hash160Body, timelockBody, emptyBody — value
		// types, copied by the node struct copy.
		return node{tag: n.tag, body: n.body}
	}
}

func needsV8Rec(n node) bool {
	switch b := n.body.(type) {
	case trBody:
		return b.ik == InternalKeyLianaUnspendable || (b.tree != nil && needsV8Rec(*b.tree))
	case childrenBody:
		for _, c := range b.children {
			if needsV8Rec(c) {
				return true
			}
		}
	case variableBody:
		for _, c := range b.children {
			if needsV8Rec(c) {
				return true
			}
		}
	}
	return false
}

func writeNodeRec(w *bitWriter, n node, keyIndexWidth uint8, wireVersion uint8) error {
	writeTag(w, n.tag)
	switch b := n.body.(type) {
	case keyArgBody:
		w.write(uint64(b.index), int(keyIndexWidth))
	case childrenBody:
		for _, c := range b.children {
			if err := writeNodeRec(w, c, keyIndexWidth, wireVersion); err != nil {
				return err
			}
		}
	case variableBody:
		// Thresh: (k-1) 5b, (len-1) 5b, children. Enforce k,n∈1..32 & k≤n.
		if b.k < 1 || b.k > 32 {
			return errThresholdRange
		}
		if len(b.children) < 1 || len(b.children) > 32 {
			return errChildCount
		}
		if int(b.k) > len(b.children) {
			return errKGreaterThanN
		}
		w.write(uint64(b.k-1), 5)
		w.write(uint64(len(b.children)-1), 5)
		for _, c := range b.children {
			if err := writeNodeRec(w, c, keyIndexWidth, wireVersion); err != nil {
				return err
			}
		}
	case multiKeysBody:
		// Multi-family: (k-1) 5b, (n-1) 5b, then RAW kiw-width indices.
		if b.k < 1 || b.k > 32 {
			return errThresholdRange
		}
		if len(b.indices) < 1 || len(b.indices) > 32 {
			return errChildCount
		}
		if int(b.k) > len(b.indices) {
			return errKGreaterThanN
		}
		w.write(uint64(b.k-1), 5)
		w.write(uint64(len(b.indices)-1), 5)
		for _, idx := range b.indices {
			w.write(uint64(idx), int(keyIndexWidth))
		}
	case trBody:
		// is_nums 1b; [kind 1b iff is_nums && v==8]; if !is_nums key_index@kiw;
		// has_tree 1b; optional subtree.
		w.write(uint64(b2u(b.isNums())), 1)
		if b.isNums() {
			if wireVersion == wfUnspendableVersion {
				w.write(uint64(b2u(b.ik == InternalKeyLianaUnspendable)), 1)
			} else if b.ik == InternalKeyLianaUnspendable {
				// Kind 1 has no representation below version 8; writing it
				// would silently emit a NUMS wallet. Fail closed (Rust
				// debug_asserts the same state at tree.rs write_node).
				return errLianaNeedsVersion8
			}
		}
		if !b.isNums() {
			w.write(uint64(b.keyIndex), int(keyIndexWidth))
		}
		w.write(uint64(b2u(b.tree != nil)), 1)
		if b.tree != nil {
			if err := writeNodeRec(w, *b.tree, keyIndexWidth, wireVersion); err != nil {
				return err
			}
		}
	case timelockBody:
		w.write(uint64(uint32(b)), 32)
	case hash256Body:
		for _, by := range b {
			w.write(uint64(by), 8)
		}
	case hash160Body:
		for _, by := range b {
			w.write(uint64(by), 8)
		}
	case emptyBody:
		// nothing
	default:
		return errOperatorContext
	}
	return nil
}

func emitFragmentRec(n node, e emitEnv, out *[]byte) error {
	switch n.tag {
	case tagPkK:
		// A BARE `PkK` CARRIES AN IMPLICIT `c:` — it emits OP_CHECKSIG too.
		//
		// The md1 wire deliberately stores a bare key at every key-check
		// position and reserves `Tag::Check` for wrapping NON-key children
		// (SPEC §5.1 Q12); the renderer reconstructs the `pk(K)` shorthand from
		// it. So the wire's `PkK` is miniscript's `c:pk_k`, and emitting only
		// the push produces a script that is missing its signature check.
		//
		// This was invisible until a vector contained a bare `pk()`: the
		// timelock/hashlock vector is all `multi` and hash/timelock fragments,
		// so it passed with the push-only version. `keyed_wsh_or_b` caught it.
		b, ok := n.body.(keyArgBody)
		if !ok {
			return ErrScriptUnsupported
		}
		k, ok := e.keys[b.index]
		if !ok {
			return ErrScriptUnsupported
		}
		pushData(out, k)
		*out = append(*out, opCHECKSIG)
		return nil

	case tagPkH:
		// `pkh(K)` on the wire is miniscript's `c:pk_h(K)`, the same implicit
		// `c:` as PkK above (SPEC §5.1 Q12), so the arm emits the whole
		// DUP HASH160 <hash160(K)> EQUALVERIFY CHECKSIG. The key is hashed AS
		// SUPPLIED: 33-byte compressed in segwit-v0, 32-byte x-only under tap
		// (BIP-342's pk_h hashes the x-only key), exactly as PkK pushes it.
		//
		// The composer is the first producer of this fragment on this device
		// (Multisig Build only ever wrote sortedmulti); md/compose_pkh_emit_test.go
		// pins it to the Rust primary's addresses for five vectors.
		b, ok := n.body.(keyArgBody)
		if !ok {
			return ErrScriptUnsupported
		}
		k, ok := e.keys[b.index]
		if !ok {
			return ErrScriptUnsupported
		}
		*out = append(*out, opDUP, opHASH160)
		pushData(out, btcaddr.Hash160(k))
		*out = append(*out, opEQUALVERIFY, opCHECKSIG)
		return nil

	case tagCheck:
		// `c:X` — X then OP_CHECKSIG.
		c, ok := n.body.(childrenBody)
		if !ok || len(c.children) != 1 {
			return ErrScriptUnsupported
		}
		// CHECK-IDEMPOTENCE over a bare key (port of md-codec to_miniscript.rs
		// `to-miniscript-check-pkh-double-wrap`). A bare PkK/PkH already IS
		// `c:pk_k`/`c:pk_h` (the arms above emit its OP_CHECKSIG), so
		// `Check(PkK)` and `Check(PkH)` denote the SAME fragment as the bare
		// tag. The toolkit before 0.55.0 and md-cli before 0.30 wrote that
		// shape for every pk()/pkh() outside tap context, and such cards are
		// already engraved. Appending a second OP_CHECKSIG built
		// `<K> CHECKSIG CHECKSIG`: an unsatisfiable script whose address
		// belongs to no wallet the Rust tools derive (bug hunt 2026-11-25
		// SH#1). Only a bare-key child with a key-argument body is unwrapped,
		// exactly as Rust matches it; every other child keeps the c: below.
		if k := c.children[0]; k.tag == tagPkK || k.tag == tagPkH {
			if _, ok := k.body.(keyArgBody); ok {
				return emitFragmentRec(k, e, out)
			}
		}
		if err := emitFragmentRec(c.children[0], e, out); err != nil {
			return err
		}
		*out = append(*out, opCHECKSIG)
		return nil

	case tagVerify:
		// `v:X` — THE SUBTLE ONE. It does not append OP_VERIFY blindly: if the
		// wrapped fragment ends in a verifiable opcode, that opcode is REPLACED
		// by its VERIFY form. Appending instead produces a longer script that
		// still parses and hashes to a different address.
		c, ok := n.body.(childrenBody)
		if !ok || len(c.children) != 1 {
			return ErrScriptUnsupported
		}
		before := len(*out)
		if err := emitFragmentRec(c.children[0], e, out); err != nil {
			return err
		}
		if len(*out) == before {
			return ErrScriptUnsupported
		}
		switch (*out)[len(*out)-1] {
		case opCHECKSIG:
			(*out)[len(*out)-1] = opCHECKSIGVERIFY
		case opCHECKMULTISIG:
			(*out)[len(*out)-1] = opCHECKMULTISIGVER
		case opEQUAL:
			(*out)[len(*out)-1] = opEQUALVERIFY
		case opNUMEQUAL:
			// `v:multi_a(...)`: multi_a terminates in OP_NUMEQUAL, and miniscript
			// folds it to OP_NUMEQUALVERIFY exactly as it folds CHECKSIG and
			// EQUAL. Missing here until the composer's corpus carried the first
			// verify-wrapped multi_a (keyed_compose_tr_nums_three_leaves), which
			// derived a WRONG taproot address against Rust's
			// (composer-S2-implementation-report F-1). A Go-only convergence
			// fix: the Rust primary emits scripts through rust-miniscript, whose
			// verify fold has always covered NUMEQUAL.
			(*out)[len(*out)-1] = opNUMEQUALVERIFY
		default:
			*out = append(*out, opVERIFY)
		}
		return nil

	case tagAndV:
		// `and_v(X,Y)` — concatenation, nothing more.
		c, ok := n.body.(childrenBody)
		if !ok || len(c.children) != 2 {
			return ErrScriptUnsupported
		}
		if err := emitFragmentRec(c.children[0], e, out); err != nil {
			return err
		}
		return emitFragmentRec(c.children[1], e, out)

	case tagOrI:
		// `or_i(X,Y)` — OP_IF X OP_ELSE Y OP_ENDIF.
		c, ok := n.body.(childrenBody)
		if !ok || len(c.children) != 2 {
			return ErrScriptUnsupported
		}
		*out = append(*out, opIF)
		if err := emitFragmentRec(c.children[0], e, out); err != nil {
			return err
		}
		*out = append(*out, opELSE)
		if err := emitFragmentRec(c.children[1], e, out); err != nil {
			return err
		}
		*out = append(*out, opENDIF)
		return nil

	case tagAndB:
		// `and_b(X,Y)` — X Y OP_BOOLAND.
		c, ok := n.body.(childrenBody)
		if !ok || len(c.children) != 2 {
			return ErrScriptUnsupported
		}
		if err := emitFragmentRec(c.children[0], e, out); err != nil {
			return err
		}
		if err := emitFragmentRec(c.children[1], e, out); err != nil {
			return err
		}
		*out = append(*out, opBOOLAND)
		return nil

	case tagOrB:
		// `or_b(X,Z)` — X Z OP_BOOLOR.
		c, ok := n.body.(childrenBody)
		if !ok || len(c.children) != 2 {
			return ErrScriptUnsupported
		}
		if err := emitFragmentRec(c.children[0], e, out); err != nil {
			return err
		}
		if err := emitFragmentRec(c.children[1], e, out); err != nil {
			return err
		}
		*out = append(*out, opBOOLOR)
		return nil

	case tagOrC:
		// `or_c(X,Z)` — X OP_NOTIF Z OP_ENDIF.
		c, ok := n.body.(childrenBody)
		if !ok || len(c.children) != 2 {
			return ErrScriptUnsupported
		}
		if err := emitFragmentRec(c.children[0], e, out); err != nil {
			return err
		}
		*out = append(*out, opNOTIF)
		if err := emitFragmentRec(c.children[1], e, out); err != nil {
			return err
		}
		*out = append(*out, opENDIF)
		return nil

	case tagOrD:
		// `or_d(X,Z)` — X OP_IFDUP OP_NOTIF Z OP_ENDIF.
		//
		// The OP_IFDUP is what distinguishes it from or_c and is easy to drop:
		// without it the script still parses and hashes elsewhere. It is the
		// degrading-multisig idiom's fragment, so it earns its own vector.
		c, ok := n.body.(childrenBody)
		if !ok || len(c.children) != 2 {
			return ErrScriptUnsupported
		}
		if err := emitFragmentRec(c.children[0], e, out); err != nil {
			return err
		}
		*out = append(*out, opIFDUP, opNOTIF)
		if err := emitFragmentRec(c.children[1], e, out); err != nil {
			return err
		}
		*out = append(*out, opENDIF)
		return nil

	case tagThresh:
		// `thresh(k,X1,...,Xn)` — X1 (Xi OP_ADD)... <k> OP_EQUAL.
		//
		// The threshold is over SUB-POLICIES, not keys, which is why this is
		// not multi: each branch contributes 0 or 1 and the sum is compared.
		b, ok := n.body.(variableBody)
		if !ok || len(b.children) == 0 {
			return ErrScriptUnsupported
		}
		if err := emitFragmentRec(b.children[0], e, out); err != nil {
			return err
		}
		for _, c := range b.children[1:] {
			if err := emitFragmentRec(c, e, out); err != nil {
				return err
			}
			*out = append(*out, opADD)
		}
		pushNumber(out, uint32(b.k))
		*out = append(*out, opEQUAL)
		return nil

	case tagSwap:
		// `s:X` — OP_SWAP X.
		c, ok := n.body.(childrenBody)
		if !ok || len(c.children) != 1 {
			return ErrScriptUnsupported
		}
		*out = append(*out, opSWAP)
		return emitFragmentRec(c.children[0], e, out)

	case tagAlt:
		// `a:X` — OP_TOALTSTACK X OP_FROMALTSTACK.
		c, ok := n.body.(childrenBody)
		if !ok || len(c.children) != 1 {
			return ErrScriptUnsupported
		}
		*out = append(*out, opTOALTSTACK)
		if err := emitFragmentRec(c.children[0], e, out); err != nil {
			return err
		}
		*out = append(*out, opFROMALTSTACK)
		return nil

	case tagDupIf:
		// `d:X` — OP_DUP OP_IF X OP_ENDIF.
		c, ok := n.body.(childrenBody)
		if !ok || len(c.children) != 1 {
			return ErrScriptUnsupported
		}
		*out = append(*out, opDUP, opIF)
		if err := emitFragmentRec(c.children[0], e, out); err != nil {
			return err
		}
		*out = append(*out, opENDIF)
		return nil

	case tagZeroNotEqual:
		// `n:X` — X OP_0NOTEQUAL.
		c, ok := n.body.(childrenBody)
		if !ok || len(c.children) != 1 {
			return ErrScriptUnsupported
		}
		if err := emitFragmentRec(c.children[0], e, out); err != nil {
			return err
		}
		*out = append(*out, op0NOTEQUAL)
		return nil

	case tagNonZero:
		// `j:X` — OP_SIZE OP_0NOTEQUAL OP_IF X OP_ENDIF.
		c, ok := n.body.(childrenBody)
		if !ok || len(c.children) != 1 {
			return ErrScriptUnsupported
		}
		*out = append(*out, opSIZE, op0NOTEQUAL, opIF)
		if err := emitFragmentRec(c.children[0], e, out); err != nil {
			return err
		}
		*out = append(*out, opENDIF)
		return nil

	case tagTrue:
		*out = append(*out, 0x51) // OP_1
		return nil

	case tagFalse:
		*out = append(*out, op0)
		return nil

	case tagOlder:
		v, ok := n.body.(timelockBody)
		if !ok {
			return ErrScriptUnsupported
		}
		pushNumber(out, uint32(v))
		*out = append(*out, opCSV)
		return nil

	case tagAfter:
		v, ok := n.body.(timelockBody)
		if !ok {
			return ErrScriptUnsupported
		}
		pushNumber(out, uint32(v))
		*out = append(*out, opCLTV)
		return nil

	case tagSha256, tagHash256:
		h, ok := n.body.(hash256Body)
		if !ok {
			return ErrScriptUnsupported
		}
		op := byte(opSHA256)
		if n.tag == tagHash256 {
			op = opHASH256
		}
		// OP_SIZE <32> OP_EQUALVERIFY <hashop> <h> OP_EQUAL
		*out = append(*out, opSIZE)
		pushNumber(out, 32)
		*out = append(*out, opEQUALVERIFY, op)
		pushData(out, h[:])
		*out = append(*out, opEQUAL)
		return nil

	case tagRipemd160, tagHash160:
		h, ok := n.body.(hash160Body)
		if !ok {
			return ErrScriptUnsupported
		}
		op := byte(opRIPEMD160)
		if n.tag == tagHash160 {
			op = opHASH160
		}
		*out = append(*out, opSIZE)
		pushNumber(out, 32)
		*out = append(*out, opEQUALVERIFY, op)
		pushData(out, h[:])
		*out = append(*out, opEQUAL)
		return nil

	case tagMulti, tagSortedMulti:
		// BIP-342 disabled OP_CHECKMULTISIG in tapscript. A `multi()` inside a
		// tap leaf has no emission, and inventing the multi_a equivalent would
		// change the policy.
		if e.tap {
			return ErrScriptUnsupported
		}
		b, ok := n.body.(multiKeysBody)
		if !ok {
			return ErrScriptUnsupported
		}
		ks := make([][]byte, 0, len(b.indices))
		for _, i := range b.indices {
			k, ok := e.keys[i]
			if !ok {
				return ErrScriptUnsupported
			}
			ks = append(ks, k)
		}
		if n.tag == tagSortedMulti {
			sortByteSlices(ks)
		}
		pushNumber(out, uint32(b.k))
		for _, k := range ks {
			pushData(out, k)
		}
		pushNumber(out, uint32(len(ks)))
		*out = append(*out, opCHECKMULTISIG)
		return nil

	case tagMultiA, tagSortedMultiA:
		// `multi_a(k, ...)` — BIP-342's replacement for CHECKMULTISIG:
		//
		//	<k0> OP_CHECKSIG <k1> OP_CHECKSIGADD … <kn-1> OP_CHECKSIGADD <k> OP_NUMEQUAL
		//
		// The first key CHECKSIGs to seed the accumulator; every later one adds
		// to it. Tapscript-only, for the same reason `multi` is not.
		if !e.tap {
			return ErrScriptUnsupported
		}
		b, ok := n.body.(multiKeysBody)
		if !ok {
			return ErrScriptUnsupported
		}
		ks := make([][]byte, 0, len(b.indices))
		for _, i := range b.indices {
			k, ok := e.keys[i]
			if !ok {
				return ErrScriptUnsupported
			}
			ks = append(ks, k)
		}
		// sortedmulti_a sorts the DERIVED keys, so the order depends on the
		// address index — it cannot be precomputed from the template.
		if n.tag == tagSortedMultiA {
			sortByteSlices(ks)
		}
		for i, k := range ks {
			pushData(out, k)
			if i == 0 {
				*out = append(*out, opCHECKSIG)
			} else {
				*out = append(*out, opCHECKSIGADD)
			}
		}
		pushNumber(out, uint32(b.k))
		*out = append(*out, opNUMEQUAL)
		return nil

	default:
		return ErrScriptUnsupported
	}
}

func guardNodeRec(n node, inCombinator bool) error {
	switch n.tag {
	case tagSortedMultiA:
		// ADMITTED SINCE 2026-08-21 (F-215). This arm refused on the grounds that
		// `sortedmulti_a` "has no rust-miniscript renderer" — true when written,
		// and false since the S0 pin lift (PR #910 gave upstream
		// `Terminal::SortedMultiA`) and R5.
		//
		// Re-measured on the current binaries before this line changed, because
		// an enumerated safety argument goes stale silently:
		//
		//	md encode  tr(@0,sortedmulti_a(2,@0,@1))  -> 1 chunk
		//	md decode  -> exit 0, the template verbatim
		//	md verify  -> re-encodes to its own template
		//	md address -> bc1p588jmtx4ptv76t9sclt6gt33eyydvsrea4njyayerqj2frw5m5aq5gzycw
		//
		// Fully recoverable with the shipped tooling, which is the only thing
		// this guard was ever asking. Refusing it now blocks a legitimate
		// template engrave (D4) and nothing else. Convergence toward the
		// primary, which admits the shape at every stage.
		return nil
	case tagMultiA:
		// multi_a renders (tr(NUMS, multi_a) is shipped) → admit; nothing below.
		return nil
	case tagSortedMulti:
		if inCombinator {
			// sortedmulti nested under a combinator → refuse (no rust-miniscript renderer).
			return errTemplateUnsupportedShape
		}
		return nil
	case tagMulti:
		// Legacy multi renders fine inside combinators — the shipped toolkit ADMITS
		// it (e.g. the §5 degrade2 wallet's wsh(or_i(and_v(...multi(3,...)),...)));
		// only sortedmulti is the render-gap shape. Always admitted (C1, exec-review).
		return nil
	case tagWsh:
		// wsh wrapper: its sole child stays on the canonical spine (NOT a
		// combinator) so a direct wsh(sortedmulti) is admitted; deeper wrapping is
		// handled by the child's own arm.
		if b, ok := n.body.(childrenBody); ok {
			for _, c := range b.children {
				if err := guardNodeRec(c, false); err != nil {
					return err
				}
			}
		}
		return nil
	case tagSh:
		// sh wrapper: a direct sh(sortedmulti/multi) and sh(wsh(...)) stay on the
		// spine; everything else its child decides.
		if b, ok := n.body.(childrenBody); ok {
			for _, c := range b.children {
				if err := guardNodeRec(c, false); err != nil {
					return err
				}
			}
		}
		return nil
	case tagTr:
		// Taproot: a key-path-only tr is fine; a script tree is walked as a
		// COMBINATOR context (its leaves are reached via tap branching) — so a
		// sortedmulti leaf there is refused, and sortedmulti_a is refused by its own
		// arm. multi_a and legacy multi are admitted.
		if b, ok := n.body.(trBody); ok && b.tree != nil {
			return guardNodeRec(*b.tree, true)
		}
		return nil
	case tagTapTree:
		if b, ok := n.body.(childrenBody); ok {
			for _, c := range b.children {
				if err := guardNodeRec(c, true); err != nil {
					return err
				}
			}
		}
		return nil
	default:
		// A combinator / miniscript fragment (or_i, and_v, thresh, …): descend with
		// inCombinator=true so any sortedmulti below is refused (legacy multi admitted).
		if b, ok := n.body.(childrenBody); ok {
			for _, c := range b.children {
				if err := guardNodeRec(c, true); err != nil {
					return err
				}
			}
		}
		if b, ok := n.body.(variableBody); ok {
			for _, c := range b.children {
				if err := guardNodeRec(c, true); err != nil {
					return err
				}
			}
		}
		return nil
	}
}

func duplicateInTapTreeRec(n node) (uint8, bool) {
	if n.tag == tagTapTree {
		b, ok := n.body.(childrenBody)
		if !ok || len(b.children) != 2 {
			return 0, false
		}
		if slot, dup := duplicateInTapTreeRec(b.children[0]); dup {
			return slot, true
		}
		return duplicateInTapTreeRec(b.children[1])
	}
	return duplicateInExpression(n)
}

func countKeySlotsRec(n node, counts *[256]int) {
	switch b := n.body.(type) {
	case keyArgBody:
		counts[b.index]++
	case multiKeysBody:
		for _, idx := range b.indices {
			counts[idx]++
		}
	case childrenBody:
		for _, c := range b.children {
			countKeySlotsRec(c, counts)
		}
	case variableBody:
		for _, c := range b.children {
			countKeySlotsRec(c, counts)
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
			countKeySlotsRec(*b.tree, counts)
		}
	}
}

func tapBranchDepthRec(n node) int {
	if n.tag != tagTapTree {
		return 0 // a script leaf contributes no branch level
	}
	cb, ok := n.body.(childrenBody)
	if !ok {
		return 0
	}
	deepest := 0
	for _, c := range cb.children {
		if dch := tapBranchDepthRec(c); dch > deepest {
			deepest = dch
		}
	}
	return deepest + 1
}

func collectTapLeavesRec(n node, depth int, out *[]TapLeaf) error {
	if n.tag == tagTapTree {
		c, ok := n.body.(childrenBody)
		if !ok || len(c.children) != 2 {
			return errTapTreeShape
		}
		if err := collectTapLeavesRec(c.children[0], depth+1, out); err != nil {
			return err
		}
		return collectTapLeavesRec(c.children[1], depth+1, out)
	}
	leaf, err := describeTapLeaf(n)
	if err != nil {
		return err
	}
	leaf.Depth = depth
	*out = append(*out, leaf)
	return nil
}

func emitTapLeavesRec(n node, depth int, e emitEnv, out *[]TapLeafScript) error {
	if n.tag == tagTapTree {
		c, ok := n.body.(childrenBody)
		if !ok || len(c.children) != 2 {
			return errTapTreeShape
		}
		if err := emitTapLeavesRec(c.children[0], depth+1, e, out); err != nil {
			return err
		}
		return emitTapLeavesRec(c.children[1], depth+1, e, out)
	}
	var script []byte
	if err := emitFragmentRec(n, e, &script); err != nil {
		return err
	}
	*out = append(*out, TapLeafScript{Depth: depth, Script: script})
	return nil
}

func collectKeyOccurrencesRec(n node, out *[]uint8) {
	switch b := n.body.(type) {
	case keyArgBody:
		*out = append(*out, b.index)
	case multiKeysBody:
		*out = append(*out, b.indices...)
	case childrenBody:
		for _, c := range b.children {
			collectKeyOccurrencesRec(c, out)
		}
	case variableBody:
		for _, c := range b.children {
			collectKeyOccurrencesRec(c, out)
		}
	}
}
