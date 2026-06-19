package md

import "sort"

// ─── canonicalize (port of canonicalize.rs:45-248, I-1 CRITICAL) ─────────────
//
// BIP-388 wallet policies require placeholder indices @0..@{n-1} to be
// introduced in canonical first-occurrence (pre-order, document) order. The
// encoder runs canonicalize on a CLONE before emitting bits; without it a
// non-canonical author-built AST (e.g. T6's seed-derived descriptor) yields a
// different csid than Rust → cross-tool ChunkSetIdMismatch. The decoder never
// needed this (it rejects non-canonical wires via validatePlaceholderUsage).

// canonicalize returns a canonicalized CLONE of d: the tree first-occurrence
// sequence is exactly [0,1,...,n-1], with the permutation applied atomically to
// the tree indices, the divergent path-decl entries, and every per-@N TLV map
// (re-keyed and re-sorted ascending by new index). If d is already canonical
// the returned clone is structurally identical. Mirrors
// canonicalize_placeholder_indices (canonicalize.rs:168-248).
//
// Returns errPlaceholderRange if the tree references @i with i>=n, and
// errPlaceholderNotReferenced if any @i for 0<=i<n is unreferenced (the
// permutation would otherwise be under-specified).
func canonicalize(d *descriptor) (*descriptor, error) {
	out := cloneDescriptor(d)
	n := int(out.n)

	// Defensive bounds check before walking — surface out-of-range references
	// as a typed error rather than ignoring them in walkCollectFirst.
	if err := checkPlaceholderBounds(out.tree, out.n); err != nil {
		return nil, err
	}

	seen := make([]bool, n)
	firstOccurrences := make([]uint8, 0, n)
	walkCollectFirst(out.tree, seen, &firstOccurrences)

	// Every @i must be referenced; otherwise the permutation is under-specified.
	for i, wasSeen := range seen {
		if !wasSeen {
			_ = i
			return nil, errPlaceholderNotReferenced
		}
	}

	// perm[old_idx] = new_idx, where new_idx is the position at which old_idx
	// was first observed in document order.
	perm := make([]uint8, n)
	for newIdx, oldIdx := range firstOccurrences {
		perm[oldIdx] = uint8(newIdx)
	}

	// Identity fast path: no work needed when perm is the identity.
	identity := true
	for i, p := range perm {
		if uint8(i) != p {
			identity = false
			break
		}
	}
	if identity {
		return out, nil
	}

	// Atomically apply the permutation to every index-bearing field.
	out.tree = remapIndices(out.tree, perm)

	if out.pathDecl.divergent != nil {
		// new_paths[new_idx] = old_paths[inverse[new_idx]] where perm[old]=new.
		inverse := make([]uint8, n)
		for old, newv := range perm {
			inverse[newv] = uint8(old)
		}
		oldPaths := out.pathDecl.divergent
		newPaths := make([]originPath, n)
		for newIdx := 0; newIdx < n; newIdx++ {
			newPaths[newIdx] = oldPaths[inverse[newIdx]]
		}
		out.pathDecl.divergent = newPaths
	}

	if out.tlv.useSitePresent {
		remapUseSiteVec(out.tlv.useSiteOverrides, perm)
	}
	if out.tlv.fpPresent {
		remapFPVec(out.tlv.fingerprints, perm)
	}
	if out.tlv.pubPresent {
		remapPubVec(out.tlv.pubkeys, perm)
	}
	if out.tlv.originPresent {
		remapOriginVec(out.tlv.originOverrides, perm)
	}

	return out, nil
}

// walkCollectFirst walks node in pre-order, recording the first occurrence of
// each placeholder index in firstOccurrences. seen[i] toggles true the first
// time @i is encountered. Port of canonicalize.rs:45-98.
func walkCollectFirst(n node, seen []bool, firstOccurrences *[]uint8) {
	switch b := n.body.(type) {
	case keyArgBody:
		if int(b.index) < len(seen) && !seen[b.index] {
			seen[b.index] = true
			*firstOccurrences = append(*firstOccurrences, b.index)
		}
	case trBody:
		// SPEC §7: is_nums=true → internal key is the NUMS H-point, not a
		// placeholder reference; skip registration.
		if !b.isNums {
			if int(b.keyIndex) < len(seen) && !seen[b.keyIndex] {
				seen[b.keyIndex] = true
				*firstOccurrences = append(*firstOccurrences, b.keyIndex)
			}
		}
		if b.tree != nil {
			walkCollectFirst(*b.tree, seen, firstOccurrences)
		}
	case childrenBody:
		for _, c := range b.children {
			walkCollectFirst(c, seen, firstOccurrences)
		}
	case variableBody:
		for _, c := range b.children {
			walkCollectFirst(c, seen, firstOccurrences)
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

// remapIndices returns a copy of n with perm[old_idx]->new_idx applied to every
// KeyArg index and Tr key_index (recursive). Port of canonicalize.rs:102-139.
func remapIndices(n node, perm []uint8) node {
	switch b := n.body.(type) {
	case keyArgBody:
		return node{tag: n.tag, body: keyArgBody{index: perm[b.index]}}
	case trBody:
		nb := trBody{isNums: b.isNums, keyIndex: b.keyIndex}
		if !b.isNums {
			nb.keyIndex = perm[b.keyIndex]
		}
		if b.tree != nil {
			t := remapIndices(*b.tree, perm)
			nb.tree = &t
		}
		return node{tag: n.tag, body: nb}
	case childrenBody:
		children := make([]node, len(b.children))
		for i, c := range b.children {
			children[i] = remapIndices(c, perm)
		}
		return node{tag: n.tag, body: childrenBody{children: children}}
	case variableBody:
		children := make([]node, len(b.children))
		for i, c := range b.children {
			children[i] = remapIndices(c, perm)
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

// remapUseSiteVec/remapFPVec/remapPubVec/remapOriginVec remap the idx column of
// a sparse TLV slice through perm and re-sort ascending (canonicalize.rs:144-149,
// remap_tlv_vec). They mutate the slice in place (the slice belongs to the
// cloned descriptor).
func remapUseSiteVec(v []idxUseSite, perm []uint8) {
	for i := range v {
		v[i].idx = perm[v[i].idx]
	}
	sort.SliceStable(v, func(i, j int) bool { return v[i].idx < v[j].idx })
}

func remapFPVec(v []idxFP, perm []uint8) {
	for i := range v {
		v[i].idx = perm[v[i].idx]
	}
	sort.SliceStable(v, func(i, j int) bool { return v[i].idx < v[j].idx })
}

func remapPubVec(v []idxPub, perm []uint8) {
	for i := range v {
		v[i].idx = perm[v[i].idx]
	}
	sort.SliceStable(v, func(i, j int) bool { return v[i].idx < v[j].idx })
}

func remapOriginVec(v []idxOrigin, perm []uint8) {
	for i := range v {
		v[i].idx = perm[v[i].idx]
	}
	sort.SliceStable(v, func(i, j int) bool { return v[i].idx < v[j].idx })
}

// checkPlaceholderBounds verifies every @N reference in node falls within 0..n.
// Returns errPlaceholderRange on the first violation. Port of
// canonicalize.rs:252-... check_placeholder_bounds.
func checkPlaceholderBounds(n node, max uint8) error {
	switch b := n.body.(type) {
	case keyArgBody:
		if b.index >= max {
			return errPlaceholderRange
		}
	case trBody:
		if !b.isNums && b.keyIndex >= max {
			return errPlaceholderRange
		}
		if b.tree != nil {
			if err := checkPlaceholderBounds(*b.tree, max); err != nil {
				return err
			}
		}
	case childrenBody:
		for _, c := range b.children {
			if err := checkPlaceholderBounds(c, max); err != nil {
				return err
			}
		}
	case variableBody:
		for _, c := range b.children {
			if err := checkPlaceholderBounds(c, max); err != nil {
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

// ─── deep clone ──────────────────────────────────────────────────────────────

// cloneDescriptor returns a deep copy of d so canonicalize never mutates its
// input (mirrors Rust's `d.clone()` in encode_payload).
func cloneDescriptor(d *descriptor) *descriptor {
	out := &descriptor{
		n:       d.n,
		useSite: cloneUseSite(d.useSite),
		tree:    cloneNode(d.tree),
	}
	out.pathDecl = clonePathDecl(d.pathDecl)
	out.tlv = cloneTLV(d.tlv)
	return out
}

// cloneSlice copies a slice while preserving its nil-vs-empty distinction (the
// decoder distinguishes make([]T,0,n) from nil; reflect.DeepEqual does too, so
// the clone must not collapse an empty non-nil slice to nil).
func cloneSlice[T any](s []T) []T {
	if s == nil {
		return nil
	}
	out := make([]T, len(s))
	copy(out, s)
	return out
}

func cloneUseSite(u useSitePath) useSitePath {
	return useSitePath{
		hasMultipath:     u.hasMultipath,
		multipath:        cloneSlice(u.multipath),
		wildcardHardened: u.wildcardHardened,
	}
}

func cloneOriginPath(p originPath) originPath {
	return originPath{components: cloneSlice(p.components)}
}

func clonePathDecl(p pathDecl) pathDecl {
	out := pathDecl{n: p.n}
	if p.shared != nil {
		s := cloneOriginPath(*p.shared)
		out.shared = &s
	}
	if p.divergent != nil {
		out.divergent = make([]originPath, len(p.divergent))
		for i, op := range p.divergent {
			out.divergent[i] = cloneOriginPath(op)
		}
	}
	return out
}

func cloneNode(n node) node {
	switch b := n.body.(type) {
	case childrenBody:
		children := make([]node, len(b.children))
		for i, c := range b.children {
			children[i] = cloneNode(c)
		}
		return node{tag: n.tag, body: childrenBody{children: children}}
	case variableBody:
		children := make([]node, len(b.children))
		for i, c := range b.children {
			children[i] = cloneNode(c)
		}
		return node{tag: n.tag, body: variableBody{k: b.k, children: children}}
	case multiKeysBody:
		return node{tag: n.tag, body: multiKeysBody{k: b.k, indices: cloneSlice(b.indices)}}
	case trBody:
		nb := trBody{isNums: b.isNums, keyIndex: b.keyIndex}
		if b.tree != nil {
			t := cloneNode(*b.tree)
			nb.tree = &t
		}
		return node{tag: n.tag, body: nb}
	default:
		// keyArgBody, hash256Body, hash160Body, timelockBody, emptyBody — value
		// types, copied by the node struct copy.
		return node{tag: n.tag, body: n.body}
	}
}

func cloneTLV(t tlvSection) tlvSection {
	out := tlvSection{
		useSitePresent: t.useSitePresent,
		fpPresent:      t.fpPresent,
		pubPresent:     t.pubPresent,
		originPresent:  t.originPresent,
	}
	if t.useSiteOverrides != nil {
		out.useSiteOverrides = make([]idxUseSite, len(t.useSiteOverrides))
		for i, e := range t.useSiteOverrides {
			out.useSiteOverrides[i] = idxUseSite{idx: e.idx, path: cloneUseSite(e.path)}
		}
	}
	out.fingerprints = cloneSlice(t.fingerprints)
	out.pubkeys = cloneSlice(t.pubkeys)
	if t.originOverrides != nil {
		out.originOverrides = make([]idxOrigin, len(t.originOverrides))
		for i, e := range t.originOverrides {
			out.originOverrides[i] = idxOrigin{idx: e.idx, path: cloneOriginPath(e.path)}
		}
	}
	if t.unknown != nil {
		out.unknown = make([]tlvUnknown, len(t.unknown))
		for i, u := range t.unknown {
			out.unknown[i] = tlvUnknown{tag: u.tag, payload: append([]byte(nil), u.payload...), bitLen: u.bitLen}
		}
	}
	return out
}
