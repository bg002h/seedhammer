package md

import (
	"os"
	"os/exec"
	"runtime/debug"
	"testing"
)

// ─── review r2 I-1: the type rule on hostile, maximally deep cards ─────────
//
// The decoder accepts a tree 128 levels deep (maxDecodeDepth), and every
// decoded card reaches policyShape. The type check must finish on the deepest
// such card in a stack far below the device's 16 KB goroutine budget: it is
// iterative, so its stack does not grow with depth.

// depthCase is a decodable tree at the decoder's depth limit.
type depthCase struct {
	name  string
	tree  node
	typed bool // treeTypeChecks, cross-checked against md-codec 2d5c9935
	walk  bool // walkShape's Complete
}

func depthCases(t *testing.T) []depthCase {
	t.Helper()
	pk := func(i uint8) node { return tpKey(tagPkK, i) }
	v := func(x node) node { return tpWrap(tagVerify, x) }
	// chain builds `levels` nested nodes above leaf with wrap.
	chain := func(levels int, leaf node, wrap func(i int, x node) node) node {
		x := leaf
		for i := levels - 1; i >= 0; i-- {
			x = wrap(i, x)
		}
		return x
	}
	older := func(i int) node { return tpLock(tagOlder, uint32(1+i)) }
	// The decoder refuses a node at depth >= 128, so a wsh root (depth 0)
	// holds 126 levels above a leaf at depth 127; a tr leaf starts at 1.
	const wshLevels, tapLevels = 126, 125
	cases := []depthCase{
		// The review's example: typed, and the walk reads n: through, so the
		// whole path runs.
		{"wsh(n:^126 pk)", tpWsh(chain(wshLevels, pk(0), func(_ int, x node) node { return tpWrap(tagZeroNotEqual, x) })), true, true},
		// and_v's v:older child sits one level below the spine, so the spine
		// is 125 long. 125 x (v:older is 2 ops) is 250 > 201: untyped by the
		// ops rule.
		{"wsh(and_v chain)", tpWsh(chain(wshLevels-1, pk(0), func(i int, x node) node { return tpPair(tagAndV, v(older(i)), x) })), false, true},
		// or_d(0, ...) 126 deep: 3 ops each, untyped by the ops rule. The
		// walk splits it into 127 branches (a deep splitBranches recursion).
		{"wsh(or_d chain)", tpWsh(chain(wshLevels, pk(0), func(_ int, x node) node { return tpPair(tagOrD, tpEmpty(tagFalse), x) })), false, true},
		// Tap has no ops limit: typed, and the walk sees one long conjunction.
		{"tr(and_v chain)", tpTrLeaf(chain(tapLevels, pk(0), func(i int, x node) node { return tpPair(tagAndV, v(older(i)), x) })), true, true},
		{"tr(thresh(1,…) nest)", tpTrLeaf(chain(tapLevels, pk(0), func(_ int, x node) node { return tpThresh(1, x) })), true, false},
	}
	// A taptree spine 126 TapTree nodes deep, each with an older leaf.
	spine := chain(wshLevels, pk(0), func(i int, x node) node { return tpPair(tagTapTree, older(i), x) })
	cases = append(cases, depthCase{"tr(126-deep taptree)", node{tag: tagTr, body: trBody{ik: InternalKeyNUMS, tree: &spine}}, true, true})
	return cases
}

// depthCard encodes c's tree as an md1 card and decodes it back, so the test
// runs on what a device would see.
func depthCard(t *testing.T, tree node) ([]string, node) {
	t.Helper()
	next := 0
	tree = zzRenumberForDepth(tree, &next)
	shared := originPath{components: toComponents(DefaultOrigin(ComposeWsh, 0))}
	if tree.tag == tagTr {
		shared = originPath{components: toComponents(DefaultOrigin(ComposeTr, 0))}
	}
	d := &descriptor{
		n:        uint8(next),
		pathDecl: pathDecl{n: uint8(next), shared: &shared},
		useSite:  useSitePath{hasMultipath: true, multipath: []alternative{{value: 0}, {value: 1}}},
		tree:     tree,
	}
	chunks, err := split(d)
	if err != nil {
		t.Fatalf("split: %v", err)
	}
	back, err := Reassemble(chunks)
	if err != nil {
		t.Fatalf("Reassemble: %v", err)
	}
	return chunks, back.tree
}

// zzRenumberForDepth gives each key occurrence a fresh slot in DFS order, so
// the card is valid (one key per slot, first-occurrence order).
func zzRenumberForDepth(n node, next *int) node {
	switch b := n.body.(type) {
	case keyArgBody:
		b.index = uint8(*next)
		*next++
		return node{tag: n.tag, body: b}
	case childrenBody:
		cs := make([]node, len(b.children))
		for i, c := range b.children {
			cs[i] = zzRenumberForDepth(c, next)
		}
		return node{tag: n.tag, body: childrenBody{children: cs}}
	case variableBody:
		cs := make([]node, len(b.children))
		for i, c := range b.children {
			cs[i] = zzRenumberForDepth(c, next)
		}
		return node{tag: n.tag, body: variableBody{k: b.k, children: cs}}
	case trBody:
		if b.tree != nil {
			t := zzRenumberForDepth(*b.tree, next)
			b.tree = &t
		}
		return node{tag: n.tag, body: b}
	}
	return n
}

// TestTypeRuleCompletesOnMaxDepthCards: every max-depth card decodes (so it
// is at the limit, not past it) and the summary completes with the verdict
// Rust gives.
func TestTypeRuleCompletesOnMaxDepthCards(t *testing.T) {
	for _, c := range depthCases(t) {
		t.Run(c.name, func(t *testing.T) {
			chunks, tree := depthCard(t, c.tree)
			if got := treeTypeChecks(tree); got != c.typed {
				t.Errorf("treeTypeChecks = %v, want %v", got, c.typed)
			}
			if got := walkShape(tree).Complete; got != c.walk {
				t.Errorf("walkShape Complete = %v, want %v", got, c.walk)
			}
			shape, err := PolicyShapeChunks(chunks)
			if err != nil {
				t.Fatal(err)
			}
			if want := c.walk && c.typed; shape.Complete != want {
				t.Errorf("PolicyShapeChunks Complete = %v, want %v", shape.Complete, want)
			}
		})
	}
	// One level deeper is refused by the decoder, so the cases are at the
	// limit.
	deeper := tpWsh(tpWrap(tagZeroNotEqual, depthCases(t)[0].tree.body.(childrenBody).children[0]))
	next := 0
	deeper = zzRenumberForDepth(deeper, &next)
	shared := originPath{components: toComponents(DefaultOrigin(ComposeWsh, 0))}
	d := &descriptor{n: 1, pathDecl: pathDecl{n: 1, shared: &shared},
		useSite: useSitePath{hasMultipath: true, multipath: []alternative{{value: 0}, {value: 1}}}, tree: deeper}
	if chunks, err := split(d); err == nil {
		if _, err := Reassemble(chunks); err == nil {
			t.Errorf("a 129-level tree decoded; the max-depth cases are not at the limit")
		}
	}
}

// typeCheckStackLimit is the stack the type check may use on the host: the
// device's 16 KB goroutine budget. A proxy, not a device measurement: gc's
// amd64 frames are not TinyGo's Thumb frames. What it proves is that the stack
// does not grow with depth: the iterative check also passes at 8 KB, while the
// recursive one it replaced overflowed 32 KB.
const typeCheckStackLimit = 16 << 10

// TestTypeRuleStackIsBoundedAtMaxDepth runs treeTypeChecks on every decoded
// max-depth tree in a child process whose goroutine stacks are capped at
// typeCheckStackLimit (debug.SetMaxStack: exceeding it is a fatal error, which
// the parent sees as a failed child).
func TestTypeRuleStackIsBoundedAtMaxDepth(t *testing.T) {
	if os.Getenv("MD_TYPECHECK_STACK_CHILD") == "1" {
		var trees []node
		for _, c := range depthCases(t) {
			_, tree := depthCard(t, c.tree)
			trees = append(trees, tree)
		}
		debug.SetMaxStack(typeCheckStackLimit)
		done := make(chan struct{})
		go func() {
			for _, tree := range trees {
				treeTypeChecks(tree)
			}
			close(done)
		}()
		<-done
		return
	}
	if testing.Short() {
		t.Skip("spawns a child test process")
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestTypeRuleStackIsBoundedAtMaxDepth$", "-test.count=1")
	cmd.Env = append(os.Environ(), "MD_TYPECHECK_STACK_CHILD=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("type check exceeded a %d-byte stack at max depth: %v\n%s", typeCheckStackLimit, err, out)
	}
}
