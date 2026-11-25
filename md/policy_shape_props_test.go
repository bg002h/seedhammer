package md

import (
	"sort"
	"strings"
	"testing"
)

// ─── seedhammer I-1: property tests of the thresh allowlist and the type rule ─
//
// The vendored cases (policy_shape_cases_test.go) are fixed trees whose
// answers are Rust's own output; a Go-only edit that loosened the allowlist
// would be caught only if one of those trees exercised it (review r1 m-1).
// These tests close that gap from the Go side: they ENUMERATE every wrapper
// stack up to three deep over every atom md can put in a thresh, and require
// the set the walk accepts to equal a short, independent list of spellings
// (DESIGN_thresh_summary section 3, the same list as md-codec's
// policy_shape_thresh_props.rs) -- and every accepted spelling to type-check
// (THE TYPE RULE, policy_typecheck.go), which is the property the Rust
// proptest asserts.

func tpKey(tg tag, slot uint8) node { return node{tag: tg, body: keyArgBody{index: slot}} }
func tpWrap(tg tag, c node) node {
	return node{tag: tg, body: childrenBody{children: []node{c}}}
}
func tpPair(tg tag, l, r node) node {
	return node{tag: tg, body: childrenBody{children: []node{l, r}}}
}
func tpThresh(k uint8, cs ...node) node {
	return node{tag: tagThresh, body: variableBody{k: k, children: cs}}
}
func tpLock(tg tag, v uint32) node { return node{tag: tg, body: timelockBody(v)} }
func tpEmpty(tg tag) node          { return node{tag: tg, body: emptyBody{}} }
func tpWsh(x node) node            { return tpWrap(tagWsh, x) }
func tpSh(x node) node             { return tpWrap(tagSh, x) }
func tpTrLeaf(x node) node {
	return node{tag: tagTr, body: trBody{ik: InternalKeyNUMS, tree: &x}}
}
func tpMulti(tg tag, k uint8, slots ...uint8) node {
	return node{tag: tg, body: multiKeysBody{k: k, indices: slots}}
}

// tpAtoms is every leaf a thresh child could be built on, slot 0 where it
// takes a key. The timelock operands straddle both edges of 1 <= v < 2^31.
func tpAtoms() []struct {
	name string
	n    node
} {
	return []struct {
		name string
		n    node
	}{
		{"pk", tpKey(tagPkK, 0)},
		{"pkh", tpKey(tagPkH, 0)},
		{"older(0)", tpLock(tagOlder, 0)},
		{"older(1)", tpLock(tagOlder, 1)},
		{"older(2^31-1)", tpLock(tagOlder, 1<<31-1)},
		{"older(2^31)", tpLock(tagOlder, 1<<31)},
		{"after(0)", tpLock(tagAfter, 0)},
		{"after(1)", tpLock(tagAfter, 1)},
		{"after(2^31-1)", tpLock(tagAfter, 1<<31-1)},
		{"after(2^31)", tpLock(tagAfter, 1<<31)},
		{"after(2^32-1)", tpLock(tagAfter, 1<<32-1)},
		{"sha256", node{tag: tagSha256, body: hash256Body{}}},
		{"1", tpEmpty(tagTrue)},
		{"0", tpEmpty(tagFalse)},
		{"multi", tpMulti(tagMulti, 1, 0)},
		{"desc_pkh", tpKey(tagPkh, 0)},
		{"desc_wpkh", tpKey(tagWpkh, 0)},
		{"raw_pkh", node{tag: tagRawPkH, body: hash160Body{}}},
	}
}

// tpWrappers are the one-child wrappers, plus l:/u: (or_i with a 0 arm), which
// the wire spells as or_i.
var tpWrappers = []struct {
	name string
	wrap func(node) node
}{
	{"c", func(x node) node { return tpWrap(tagCheck, x) }},
	{"v", func(x node) node { return tpWrap(tagVerify, x) }},
	{"s", func(x node) node { return tpWrap(tagSwap, x) }},
	{"a", func(x node) node { return tpWrap(tagAlt, x) }},
	{"d", func(x node) node { return tpWrap(tagDupIf, x) }},
	{"j", func(x node) node { return tpWrap(tagNonZero, x) }},
	{"n", func(x node) node { return tpWrap(tagZeroNotEqual, x) }},
	{"l", func(x node) node { return tpPair(tagOrI, tpEmpty(tagFalse), x) }},
	{"u", func(x node) node { return tpPair(tagOrI, x, tpEmpty(tagFalse)) }},
}

type tpSpelling struct {
	name string
	n    node
}

// tpSpellings is every wrapper stack of depth 0..3 over every atom, named like
// miniscript ("s:c:pk").
func tpSpellings() []tpSpelling {
	var out []tpSpelling
	var grow func(prefix []string, n node, depth int, atom string)
	grow = func(prefix []string, n node, depth int, atom string) {
		out = append(out, tpSpelling{strings.Join(append(append([]string{}, prefix...), atom), ":"), n})
		if depth == 3 {
			return
		}
		for _, w := range tpWrappers {
			// The new wrapper goes OUTSIDE the stack: it is the name's first letter.
			grow(append([]string{w.name}, prefix...), w.wrap(n), depth+1, atom)
		}
	}
	for _, a := range tpAtoms() {
		grow(nil, a.n, 0, a.name)
	}
	return out
}

// tpExpected is the independent list (DESIGN_thresh_summary section 3):
// KEY = pk/pkh bare or under one c:; T = older/after with 1 <= v < 2^31;
// LOCK = ln:T, un:T, nl:T, nu:T. The first child is KEY or LOCK; a later child
// is s:KEY without pkh, a:KEY, s:LOCK or a:LOCK.
func tpExpected() (first, later map[string]bool) {
	keys := []string{"pk", "pkh", "c:pk", "c:pkh"}
	var locks []string
	for _, t := range []string{"older(1)", "older(2^31-1)", "after(1)", "after(2^31-1)"} {
		locks = append(locks, "l:n:"+t, "u:n:"+t, "n:l:"+t, "n:u:"+t)
	}
	first, later = map[string]bool{}, map[string]bool{}
	for _, s := range append(append([]string{}, keys...), locks...) {
		first[s] = true
	}
	for _, s := range []string{"s:pk", "s:c:pk", "a:pk", "a:c:pk", "a:pkh", "a:c:pkh"} {
		later[s] = true
	}
	for _, l := range locks {
		later["s:"+l] = true
		later["a:"+l] = true
	}
	return first, later
}

func tpSorted(m map[string]bool) []string {
	var s []string
	for k := range m {
		s = append(s, k)
	}
	sort.Strings(s)
	return s
}

// TestThreshAllowlistIsExactlyTheSpellings: the walk accepts a child at a
// position iff the independent list names it there, over every enumerated
// stack, and every thresh it accepts type-checks in wsh, sh and a tap leaf.
func TestThreshAllowlistIsExactlyTheSpellings(t *testing.T) {
	wantFirst, wantLater := tpExpected()
	gotFirst, gotLater := map[string]bool{}, map[string]bool{}
	spellings := tpSpellings()
	for _, sp := range spellings {
		for _, pos := range []struct {
			got  map[string]bool
			tree node
		}{
			{gotFirst, tpThresh(1, sp.n, tpWrap(tagSwap, tpKey(tagPkK, 1)))},
			{gotLater, tpThresh(1, tpKey(tagPkK, 1), sp.n)},
		} {
			s := walkShape(tpWsh(pos.tree))
			accepted := s.Complete && len(s.Branches) == 1 && s.Branches[0].AnyN == 2
			if !accepted && s.Complete {
				// A thresh the list refuses is no summary at all, never a
				// summary of something else.
				t.Errorf("%s: refused as an any-of, yet the walk is complete: %+v", sp.name, s)
				continue
			}
			if !accepted {
				continue
			}
			pos.got[sp.name] = true
			for _, root := range []node{tpWsh(pos.tree), tpSh(pos.tree), tpTrLeaf(pos.tree)} {
				if !treeTypeChecks(root) {
					t.Errorf("%s: accepted, but its thresh does not type-check under root tag %d", sp.name, root.tag)
				}
			}
		}
	}
	for _, c := range []struct {
		pos       string
		got, want map[string]bool
	}{{"first", gotFirst, wantFirst}, {"later", gotLater, wantLater}} {
		for _, name := range tpSorted(c.got) {
			if !c.want[name] {
				t.Errorf("%s child: the walk accepts %s, which the list does not name", c.pos, name)
			}
		}
		for _, name := range tpSorted(c.want) {
			if !c.got[name] {
				t.Errorf("%s child: the walk refuses %s, which the list names", c.pos, name)
			}
		}
	}
	// A floor on the enumeration itself, so a broken generator cannot pass.
	if len(spellings) < 10_000 || len(gotFirst) != 20 || len(gotLater) != 38 {
		t.Errorf("enumerated %d spellings, accepted %d first / %d later; want >= 10000, 20, 38",
			len(spellings), len(gotFirst), len(gotLater))
	}
}

type typeRuleCase struct {
	name     string
	tree     node
	typed    bool
	complete bool // policyShape's Complete: walk && typed
}

// typeRuleCases are the trees review r1 m-1 named, and the context rules the
// vendored file does not reach. Each answer is the one rust-miniscript gives
// (the reasoning is in the comment). A differential probe of
// descriptor-mnemonic a7680cf2 agreed on every row md can encode as a card, but
// the tap-leaf B rows, which follow md review r3 ahead of the pin; tr(multi)
// and the two 97/98-key rows have no card (md refuses a multi leaf and n > 32),
// so their answers are checked against rust-miniscript's source arithmetic
// only.
func typeRuleCases() []typeRuleCase {
	pk := func(i uint8) node { return tpKey(tagPkK, i) }
	s := func(x node) node { return tpWrap(tagSwap, x) }
	a := func(x node) node { return tpWrap(tagAlt, x) }
	v := func(x node) node { return tpWrap(tagVerify, x) }
	keys := func(n int) []uint8 {
		out := make([]uint8, n)
		for i := range out {
			out[i] = uint8(i)
		}
		return out
	}
	// bigThresh is thresh(1, pk, s:pk, ...) over n keys.
	bigThresh := func(n int) node {
		cs := []node{pk(0)}
		for i := 1; i < n; i++ {
			cs = append(cs, s(pk(uint8(i))))
		}
		return tpThresh(1, cs...)
	}
	return []typeRuleCase{
		// The review's I-1: thresh is B where or_b needs W, so the
		// combination spends with no signature. The walk alone summarizes it.
		{"or_b(pk,thresh)", tpWsh(tpPair(tagOrB, pk(0), tpThresh(1, pk(1), s(pk(2))))), false, false},
		// The legal spelling: a:thresh is W. k == n is a conjunction.
		{"or_b(pk,a:thresh k==n)", tpWsh(tpPair(tagOrB, pk(0), a(tpThresh(2, pk(1), s(pk(2)))))), true, true},
		// and k < n is an any-of root, allowed only there.
		{"or_b(pk,a:thresh k<n)", tpWsh(tpPair(tagOrB, pk(0), a(tpThresh(1, pk(1), s(pk(2)))))), true, true},
		// or_i(B, B): a thresh is a fine left arm.
		{"or_i(thresh,pk)", tpWsh(tpPair(tagOrI, tpThresh(1, pk(0), s(pk(1))), pk(2))), true, true},
		// A descriptor-level wrapper is no miniscript fragment.
		{"thresh(desc pkh)", tpWsh(tpThresh(1, tpKey(tagPkh, 0), s(pk(1)))), false, false},
		{"thresh(desc wpkh)", tpWsh(tpThresh(1, tpKey(tagWpkh, 0), s(pk(1)))), false, false},
		{"thresh(raw_pkh)", tpWsh(tpThresh(1, node{tag: tagRawPkH, body: hash160Body{}}, s(pk(1)))), false, false},
		// after(2^31) is not an AbsLockTime.
		{"thresh(ln:after(2^31))", tpWsh(tpThresh(1, pk(0), s(tpPair(tagOrI, tpEmpty(tagFalse), tpWrap(tagZeroNotEqual, tpLock(tagAfter, 1<<31)))))), false, false},
		{"after(2^31-1)", tpWsh(tpPair(tagAndV, v(pk(0)), tpLock(tagAfter, 1<<31-1))), true, true},
		{"older(0)", tpWsh(tpPair(tagAndV, v(pk(0)), tpLock(tagOlder, 0))), false, false},
		// Context: multi only outside tap, multi_a only in tap.
		{"wsh(multi)", tpWsh(tpMulti(tagMulti, 1, 0, 1)), true, true},
		{"wsh(multi_a)", tpWsh(tpMulti(tagMultiA, 1, 0, 1)), false, false},
		{"tr(multi)", tpTrLeaf(tpMulti(tagMulti, 1, 0, 1)), false, false},
		{"tr(multi_a)", tpTrLeaf(tpMulti(tagMultiA, 1, 0, 1)), true, true},
		{"tr(sortedmulti_a)", tpTrLeaf(tpMulti(tagSortedMultiA, 1, 0, 1)), true, true},
		{"tr(and_v(v:pk,sortedmulti_a))", tpTrLeaf(tpPair(tagAndV, v(pk(2)), tpMulti(tagSortedMultiA, 1, 0, 1))), false, false},
		{"wsh(and_v(v:pk,sortedmulti))", tpWsh(tpPair(tagAndV, v(pk(2)), tpMulti(tagSortedMulti, 1, 0, 1))), false, false},
		// The root must be B in wsh/sh and in every tap leaf. The leaf
		// rule is md review r3's; at the vendored pin a7680cf2 Rust still
		// calls tr(v:pk) typed (the one row this table leads it on).
		{"wsh(v:pk)", tpWsh(v(pk(0))), false, false},
		{"sh(v:pk)", tpSh(v(pk(0))), false, false},
		{"tr(v:pk)", tpTrLeaf(v(pk(0))), false, false},
		{"tr({pk,v:pk})", node{tag: tagTr, body: trBody{ik: InternalKeyNUMS, tree: func() *node {
			x := tpPair(tagTapTree, pk(0), v(pk(1)))
			return &x
		}()}}, false, false},
		// Legacy's 520-byte redeem script: multi over 15 keys is 2+15*34+1 =
		// 513, over 16 is 547. sortedmulti is built unchecked (no size rule).
		{"sh(multi 15)", tpSh(tpMulti(tagMulti, 1, keys(15)...)), true, true},
		{"sh(multi 16)", tpSh(tpMulti(tagMulti, 1, keys(16)...)), false, false},
		{"sh(sortedmulti 20)", tpSh(tpMulti(tagSortedMulti, 1, keys(20)...)), true, true},
		{"sh(sortedmulti 21)", tpSh(tpMulti(tagSortedMulti, 1, keys(21)...)), false, false},
		// Segwitv0's standard 3600 bytes: thresh(1, pk, s:pk, ...) over n
		// keys costs 2 (k, EQUAL) + 35 + 36(n-1) + (n-1) ADDs = 37n, so 97
		// keys is 3589 and 98 is 3626.
		{"wsh(thresh 97)", tpWsh(bigThresh(97)), true, true},
		{"wsh(thresh 98)", tpWsh(bigThresh(98)), false, false},
	}
}

// TestTypeRuleCases pins the type rule and policyShape's Complete on
// typeRuleCases.
func TestTypeRuleCases(t *testing.T) {
	for _, c := range typeRuleCases() {
		t.Run(c.name, func(t *testing.T) {
			if got := treeTypeChecks(c.tree); got != c.typed {
				t.Errorf("treeTypeChecks = %v, want %v", got, c.typed)
			}
			if got := policyShape(c.tree).Complete; got != c.complete {
				t.Errorf("policyShape Complete = %v, want %v", got, c.complete)
			}
		})
	}
}
