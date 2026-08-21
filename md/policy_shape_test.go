package md

import (
	"os"
	"strings"
	"testing"
)

func shapeFromVector(t *testing.T, name string) PolicyShape {
	t.Helper()
	raw, err := os.ReadFile(vectorPath(name, "phrase.txt"))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	var chunks []string
	for _, l := range strings.Split(string(raw), "\n") {
		l = strings.ReplaceAll(strings.TrimSpace(l), " ", "")
		if strings.HasPrefix(l, "md1") {
			chunks = append(chunks, l)
		}
	}
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
		body: trBody{isNums: true, keyIndex: 0},
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
