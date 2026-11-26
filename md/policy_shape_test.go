package md

import (
	"testing"
)

// shapeFromVector loads a corpus vector through vectorChunksFor, which PREFERS
// a fork-side pin.
//
// F-614: this used to read testdata/vectors/<name>.phrase.txt directly, and
// `keyed_tr_sortedmulti_a` below is one of the two vectors F-529 says a
// re-vendor from the primary would DELETE. Every other consumer of those two
// names already went through a pin-preferring loader (md's vectorChunksFor,
// gui's loadVectorChunks); this one did not, so it was the single place where
// the re-vendor would turn a pinned-fixture test into a "read: no such file"
// failure instead of reading the pin that exists for exactly that day.
func shapeFromVector(t *testing.T, name string) PolicyShape {
	t.Helper()
	chunks := vectorChunksFor(t, name)
	s, err := PolicyShapeChunks(chunks)
	if err != nil {
		t.Fatalf("%s: PolicyShapeChunks: %v", name, err)
	}
	return s
}

// TestPolicyShapeDescribesRealCards runs the summary over the vendored corpus,
// which is the only place these shapes exist as real wire bytes.
func TestPolicyShapeDescribesRealCards(t *testing.T) {
	for _, tc := range []struct {
		vector   string
		keyPath  KeyPathKind
		branches int
		tapDepth int
		wantK    int
		wantN    int
	}{
		{vector: "keyed_wpkh", keyPath: KeyPathNone, branches: 1},
		{vector: "keyed_wsh_multi_2of3", keyPath: KeyPathNone, branches: 1, wantK: 2, wantN: 3},
		{vector: "keyed_wsh_sortedmulti_2of3", keyPath: KeyPathNone, branches: 1, wantK: 2, wantN: 3},
		// A key-path-only taproot has NO leaves. Reporting a branch here would
		// invent a spend path that does not exist.
		{vector: "keyed_tr_keyonly", keyPath: KeyPathSpendable, branches: 0},
		{vector: "keyed_tr_with_leaf", keyPath: KeyPathSpendable, branches: 1, tapDepth: 0},
		// A tap leaf that IS a plain threshold over keys: K/N must be reported,
		// unlike the bare-pk leaves above.
		{vector: "keyed_tr_sortedmulti_a", keyPath: KeyPathSpendable, branches: 1, wantK: 2, wantN: 2},
		// THE SHAPE THE CYCLE IS ABOUT: three leaves, unbalanced, depth 2.
		{vector: "keyed_tr_depth2", keyPath: KeyPathSpendable, branches: 3, tapDepth: 2},
	} {
		t.Run(tc.vector, func(t *testing.T) {
			s := shapeFromVector(t, tc.vector)
			if !s.Complete {
				t.Fatalf("%s: walk reported INCOMPLETE — the caller would show nothing", tc.vector)
			}
			if s.KeyPath != tc.keyPath {
				t.Errorf("KeyPath = %v, want %v", s.KeyPath, tc.keyPath)
			}
			if len(s.Branches) != tc.branches {
				t.Fatalf("branches = %d, want %d (%+v)", len(s.Branches), tc.branches, s.Branches)
			}
			if s.TapDepth != tc.tapDepth {
				t.Errorf("TapDepth = %d, want %d", s.TapDepth, tc.tapDepth)
			}
			if tc.wantN != 0 {
				if s.Branches[0].K != tc.wantK || s.Branches[0].N != tc.wantN {
					t.Errorf("branch 0 = %d-of-%d, want %d-of-%d",
						s.Branches[0].K, s.Branches[0].N, tc.wantK, tc.wantN)
				}
			}
			for i, b := range s.Branches {
				if b.Keys == 0 {
					t.Errorf("branch %d reports ZERO keys; no spend path is keyless", i)
				}
			}
		})
	}
}

// TestPolicyShapeNeverClaimsAPlainThresholdItCannotSee is the anti-invention
// case: K/N must stay zero for a branch that is not a bare threshold over keys.
// Reporting "1-of-1" for an `and_v(v:pk(A),older(144))` would tell an operator
// the timelock is optional.
func TestPolicyShapeNeverClaimsAPlainThresholdItCannotSee(t *testing.T) {
	tree := node{tag: tagWsh, body: childrenBody{children: []node{{
		tag: tagAndV,
		body: childrenBody{children: []node{
			{tag: tagVerify, body: childrenBody{children: []node{{tag: tagPkK, body: keyArgBody{index: 0}}}}},
			{tag: tagOlder, body: timelockBody(144)},
		}},
	}}}}
	s := policyShape(tree)
	if !s.Complete {
		t.Fatal("and_v(v:pk,older) should be understood")
	}
	if len(s.Branches) != 1 {
		t.Fatalf("branches = %d, want 1", len(s.Branches))
	}
	b := s.Branches[0]
	if b.K != 0 || b.N != 0 {
		t.Errorf("K/N = %d/%d, want 0/0 — this is not a plain k-of-N", b.K, b.N)
	}
	if !b.Timelock {
		t.Error("the older(144) was not reported as a timelock")
	}
	if b.Keys != 1 {
		t.Errorf("Keys = %d, want 1", b.Keys)
	}
	// THE MIXED BRANCH, and this is the case the guard did NOT cover when
	// Branch.K/N widened to "the one multi at any depth" (round-1 review
	// I-2). The fixture above holds no multi node at all, so it never enters
	// the widened arm and passed either way; the risk the widening created is
	// a branch holding one multi PLUS other key material.
	//
	//   wsh(and_v(v:older(10), or_d(multi(2,@0,@1,@2), c:pk_k(@3))))
	//
	// splitBranches splits or_*/andor but not an `or` under and_v, so this is
	// ONE branch with Keys == 4 and a multi of 3. Reporting 2-of-3 for it
	// labels a branch @3 can spend ALONE after 10 blocks as a 2-of-3 -- the
	// exact misreading this test's own docstring names.
	mixed := node{tag: tagWsh, body: childrenBody{children: []node{{
		tag: tagAndV,
		body: childrenBody{children: []node{
			{tag: tagVerify, body: childrenBody{children: []node{{tag: tagOlder, body: timelockBody(10)}}}},
			{tag: tagOrD, body: childrenBody{children: []node{
				{tag: tagMulti, body: multiKeysBody{k: 2, indices: []uint8{0, 1, 2}}},
				{tag: tagCheck, body: childrenBody{children: []node{{tag: tagPkK, body: keyArgBody{index: 3}}}}},
			}}},
		}},
	}}}}
	// SINCE bug hunt 2026-11-25 SH#2 (md-codec 0.48.5's rule) the walk no
	// longer merges an `or` under and_v into one branch: it refuses the
	// summary, which is stronger than the 0/0 this case used to pin -- no
	// line at all reaches the consent, so neither a k-of-n nor a "2 key(s)"
	// can misdescribe the path @3 spends alone.
	ms := policyShape(mixed)
	if ms.Complete || len(ms.Branches) != 0 {
		t.Fatalf("an or_d under and_v was summarized (complete=%v, %d branch(es) %+v); "+
			"@3 can spend alone after 10 blocks, so the summary must be refused",
			ms.Complete, len(ms.Branches), ms.Branches)
	}
	// The same shape without the timelock, so the finding's second tree is
	// pinned too: a branch is mixed because of its KEYS, not its locks.
	mixedNoLock := node{tag: tagWsh, body: childrenBody{children: []node{{
		tag: tagAndV,
		body: childrenBody{children: []node{
			{tag: tagVerify, body: childrenBody{children: []node{
				{tag: tagMulti, body: multiKeysBody{k: 2, indices: []uint8{0, 1, 2}}}}}},
			{tag: tagCheck, body: childrenBody{children: []node{{tag: tagPkK, body: keyArgBody{index: 3}}}}},
		}},
	}}}}
	ns := policyShape(mixedNoLock)
	if !ns.Complete || len(ns.Branches) != 1 {
		t.Fatalf("INCONCLUSIVE: complete=%v branches=%d", ns.Complete, len(ns.Branches))
	}
	if nb := ns.Branches[0]; nb.K != 0 || nb.N != 0 {
		t.Errorf("K/N = %d/%d for and_v(v:multi(2,@0,@1,@2), c:pk_k(@3)), want 0/0 "+
			"(Keys = %d)", nb.K, nb.N, nb.Keys)
	}
}

// TestPolicyShapeRefusesAnUnknownTag is the honesty contract itself.
//
// If the walk meets something it cannot classify it must report Complete=false
// so the caller shows the honest-minimal screen. A summary that quietly dropped
// the branch would be strictly worse than no summary: the operator would believe
// they had seen the whole policy.
func TestPolicyShapeRefusesAnUnknownTag(t *testing.T) {
	// tagTr nested inside a script is not constructible by the decoder, which
	// makes it a stand-in for "a tag this walk has not been taught".
	tree := node{tag: tagWsh, body: childrenBody{children: []node{{
		tag:  tagTr,
		body: trBody{ik: InternalKeyNUMS, keyIndex: 0},
	}}}}
	s := policyShape(tree)
	if s.Complete {
		t.Fatal("an unrecognised tag was summarized instead of refused")
	}
	if len(s.Branches) != 0 {
		t.Errorf("a refused walk still produced %d branch(es); the caller might show them", len(s.Branches))
	}
}

// TestPolicyShapeReportsEveryLeafOfADeepTree pins the objection SPEC §4.2/C3
// raises: a summary must not describe one leaf and stay silent about the others.
func TestPolicyShapeReportsEveryLeafOfADeepTree(t *testing.T) {
	leaf := func(i uint8) node {
		return node{tag: tagPkK, body: keyArgBody{index: i}}
	}
	branch := func(l, r node) node {
		return node{tag: tagTapTree, body: childrenBody{children: []node{l, r}}}
	}
	// {{A,B},{C,{D,E}}} — five leaves, depths 2,2,2,3,3.
	tree := node{tag: tagTr, body: trBody{
		keyIndex: 0,
		tree:     ptr(branch(branch(leaf(1), leaf(2)), branch(leaf(3), branch(leaf(4), leaf(5))))),
	}}
	s := policyShape(tree)
	if !s.Complete {
		t.Fatal("a well-formed deep taptree must be understood")
	}
	if len(s.Branches) != 5 {
		t.Fatalf("branches = %d, want 5 — a leaf was dropped", len(s.Branches))
	}
	if s.TapDepth != 3 {
		t.Errorf("TapDepth = %d, want 3", s.TapDepth)
	}
	if s.KeyPath != KeyPathSpendable {
		t.Error("the key-path spend condition was lost — the exact omission C3 warns about")
	}
}

func ptr(n node) *node { return &n }

// TestPolicyShapeRefusesADisjunctionInsideABranch: bug hunt 2026-11-25 SH#2,
// the port of md-codec 0.48.5 (tests/bughunt.rs
// bughunt_md_05b_every_nested_disjunction_tag_marks_incomplete). An `or`/andor
// under and_v is two spend paths the summary cannot list without a cross
// product, so it refuses; walking it as a conjunction printed ONE path
// ("2 key(s), custom +timelock") for wsh(and_v(v:pk(@0),or_d(pk(@1),
// older(52560)))), where @0 spends alone after the lock.
func TestPolicyShapeRefusesADisjunctionInsideABranch(t *testing.T) {
	key := func(i uint8) node { return node{tag: tagPkK, body: keyArgBody{index: i}} }
	under := func(inner node) node {
		return node{tag: tagWsh, body: childrenBody{children: []node{{
			tag: tagAndV,
			body: childrenBody{children: []node{
				{tag: tagVerify, body: childrenBody{children: []node{key(0)}}},
				inner,
			}},
		}}}}
	}
	// The finding's own tree.
	finding := under(node{tag: tagOrD, body: childrenBody{children: []node{
		key(1), {tag: tagOlder, body: timelockBody(52560)},
	}}})
	if s := policyShape(finding); s.Complete || len(s.Branches) != 0 {
		t.Errorf("wsh(and_v(v:pk(@0),or_d(pk(@1),older(52560)))): complete=%v branches=%+v, "+
			"want incomplete with no branches", s.Complete, s.Branches)
	}
	// Every disjunction tag, the trees the Rust test builds.
	for _, tg := range []tag{tagAndOr, tagOrB, tagOrC, tagOrD, tagOrI} {
		kids := []node{key(1), key(2)}
		if tg == tagAndOr {
			kids = append(kids, key(3))
		}
		s := policyShape(under(node{tag: tg, body: childrenBody{children: kids}}))
		if s.Complete || len(s.Branches) != 0 {
			t.Errorf("tag 0x%02x under and_v: complete=%v branches=%+v, want incomplete with none",
				uint8(tg), s.Complete, s.Branches)
		}
	}
	// NOT disjunctions, as in Rust: l:X = or_i(0,X), u:X = or_i(X,0) and
	// and_n(X,Y) = andor(X,Y,0) have an unsatisfiable `0` alternative, so the
	// branch is still one path and stays summarised.
	f := node{tag: tagFalse, body: emptyBody{}}
	for name, inner := range map[string]node{
		"l:":    {tag: tagOrI, body: childrenBody{children: []node{f, key(1)}}},
		"u:":    {tag: tagOrI, body: childrenBody{children: []node{key(1), f}}},
		"and_n": {tag: tagAndOr, body: childrenBody{children: []node{key(1), key(2), f}}},
	} {
		s := policyShape(under(inner))
		if !s.Complete || len(s.Branches) != 1 {
			t.Errorf("%s under and_v: complete=%v branches=%d, want one summarised branch",
				name, s.Complete, len(s.Branches))
		}
	}
}
