package md

// The recursive decoder and shape walk as they stood at 55d31dd, before review
// r3 I-P1 made them iterative. Test-only: TestIterativeDecodeMatchesRecursive
// and TestIterativeWalkMatchesRecursive hold the iterative versions to these,
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
