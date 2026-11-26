package md

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"runtime/debug"
	"strings"
	"testing"
	"unsafe"
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
	typed bool // treeTypeChecks, cross-checked against md-codec 2d5c9935 (unchanged at 1dac0e10)
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
	return depthCardKeys(t, tree, uint8(next))
}

// depthCardKeys encodes a tree whose key slots are already numbered 0..n-1 in
// first-occurrence order, and decodes it back.
func depthCardKeys(t *testing.T, tree node, next uint8) ([]string, node) {
	t.Helper()
	shared := originPath{components: toComponents(DefaultOrigin(ComposeWsh, 0))}
	if tree.tag == tagTr {
		shared = originPath{components: toComponents(DefaultOrigin(ComposeTr, 0))}
	}
	d := &descriptor{
		n:        next,
		pathDecl: pathDecl{n: next, shared: &shared},
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

// keyLeafSpine is a taptree 126 TapTree nodes deep whose 127 leaves are all
// keys, slots 0..31 reused in order (a card holds at most 32), under a tr with
// internal key ik. Unlike depthCases' taptree (older leaves), every leaf is one
// TapLeavesChunks can describe and a Liana-unspendable key can be computed
// over, so those entry points walk the whole spine instead of refusing at
// the first leaf.
func keyLeafSpine(ik InternalKeyKind) node {
	const levels = 126
	x := tpKey(tagPkK, levels%32)
	for i := levels - 1; i >= 0; i-- {
		x = tpPair(tagTapTree, tpKey(tagPkK, uint8(i%32)), x)
	}
	return node{tag: tagTr, body: trBody{ik: ik, tree: &x}}
}

// maxDepthCard is one card at the decoder's depth limit, for the stack test.
type maxDepthCard struct {
	name   string
	chunks []string
	tr     bool
	spine  bool // a key-leaf spine: TapLeavesChunks must describe it
	liana  bool
}

func maxDepthCards(t *testing.T) []maxDepthCard {
	t.Helper()
	var cards []maxDepthCard
	for _, c := range depthCases(t) {
		chunks, _ := depthCard(t, c.tree)
		cards = append(cards, maxDepthCard{name: c.name, chunks: chunks, tr: c.tree.tag == tagTr})
	}
	for _, ik := range []InternalKeyKind{InternalKeyNUMS, InternalKeyLianaUnspendable} {
		chunks, _ := depthCardKeys(t, keyLeafSpine(ik), 32)
		cards = append(cards, maxDepthCard{name: fmt.Sprintf("key-leaf spine, ik %d", ik), chunks: chunks,
			tr: true, spine: true, liana: ik == InternalKeyLianaUnspendable})
	}
	return cards
}

// cardEntryPoints runs every exported entry point that takes an md1 card on
// one max-depth card, and reports a result the card's kind says must not
// fail (so each entry point is shown to walk the deep tree, not to refuse it
// at the top).
func cardEntryPoints(c maxDepthCard) []error {
	var errs []error
	must := func(what string, err error) {
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %s: %w", c.name, what, err))
		}
	}
	segwitKeys, tapKeys := map[uint8][]byte{}, map[uint8][]byte{}
	xpubs := map[uint8][65]byte{}
	for i := 0; i < 32; i++ {
		k := make([]byte, 33)
		k[0], k[1] = 2, byte(i)
		segwitKeys[uint8(i)] = k
		tapKeys[uint8(i)] = k[1:]
		xpubs[uint8(i)] = [65]byte{1: byte(i)}
	}
	_, err := Reassemble(c.chunks)
	must("Reassemble", err)
	_, err = PolicyShapeChunks(c.chunks)
	must("PolicyShapeChunks", err)
	_, err = EmitWitnessScriptChunks(c.chunks, segwitKeys)
	if !c.tr {
		must("EmitWitnessScriptChunks", err)
	}
	_, _, _, err = EmitTapLeavesChunks(c.chunks, tapKeys)
	if c.tr {
		must("EmitTapLeavesChunks", err)
	}
	_, _, _, err = TapLeavesChunks(c.chunks)
	if c.spine {
		must("TapLeavesChunks", err)
	}
	_, err = LianaUnspendableKeyFor(c.chunks, xpubs)
	if c.liana {
		must("LianaUnspendableKeyFor", err)
	}
	_, _, err = DuplicateKeySlotChunks(c.chunks)
	must("DuplicateKeySlotChunks", err)
	must("TemplateEngraveShapeGuardChunks", TemplateEngraveShapeGuardChunks(c.chunks))
	depth, err := TapTreeDepthChunks(c.chunks)
	must("TapTreeDepthChunks", err)
	if c.spine && depth != 126 {
		errs = append(errs, fmt.Errorf("%s: TapTreeDepthChunks = %d, want 126", c.name, depth))
	}
	_, err = StripToTemplate(c.chunks)
	must("StripToTemplate", err)
	return errs
}

// typeCheckStackLimit and loadStackLimit are the stacks the max-depth work
// may use on the host. loadStackLimit is the device's 16 KB goroutine budget:
// a hostile max-depth card must decode (Reassemble) and summarise
// (PolicyShapeChunks) within it, because the decoder runs on every md1 load
// (review r3 I-P1). The type check alone is held to half of that (review r3
// n-3): its stack is constant, about 2.5 KB of arm frames. These are proxies,
// not device measurements: gc's frames are not TinyGo's Thumb frames. What
// they prove is that no stack grows with depth: the recursive decoder and walk
// they replaced needed 32-64 KB here, the recursive type check over 16 KB.
const (
	typeCheckStackLimit = 8 << 10
	loadStackLimit      = 16 << 10
)

// stackChildTest is TestMaxDepthCardsFitTheStack's own name. The parent runs
// the child with -test.run pinned to it and requires stackChildDone in the
// child's output, so a renamed test cannot pass by running nothing (review r3
// n-1).
const (
	stackChildTest = "TestMaxDepthCardsFitTheStack"
	stackChildDone = "MD_STACK_CHILD_DONE"
)

// TestMaxDepthCardsFitTheStack runs the type check, then every exported entry
// point that takes an md1 card (cardEntryPoints), on every max-depth card in
// a child process whose goroutine stacks are capped (debug.SetMaxStack: exceeding it is a fatal
// error, which the parent sees as a failed child).
func TestMaxDepthCardsFitTheStack(t *testing.T) {
	if os.Getenv("MD_STACK_CHILD") == "1" {
		var trees []node
		for _, c := range depthCases(t) {
			_, tree := depthCard(t, c.tree)
			trees = append(trees, tree)
		}
		cards := maxDepthCards(t)
		run := func(limit int, f func()) {
			debug.SetMaxStack(limit)
			done := make(chan struct{})
			go func() {
				f()
				close(done)
			}()
			<-done
		}
		run(typeCheckStackLimit, func() {
			for _, tree := range trees {
				treeTypeChecks(tree)
			}
		})
		var errs []error
		run(loadStackLimit, func() {
			for _, c := range cards {
				errs = append(errs, cardEntryPoints(c)...)
			}
		})
		debug.SetMaxStack(1 << 30)
		for _, err := range errs {
			t.Error(err)
		}
		fmt.Println(stackChildDone)
		return
	}
	if testing.Short() {
		t.Skip("spawns a child test process")
	}
	if name := t.Name(); name != stackChildTest {
		t.Fatalf("test is named %q; update stackChildTest", name)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^"+stackChildTest+"$", "-test.count=1", "-test.v")
	cmd.Env = append(os.Environ(), "MD_STACK_CHILD=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("a max-depth card exceeded its stack (type check %d B, load %d B): %v\n%s", typeCheckStackLimit, loadStackLimit, err, out)
	}
	if !strings.Contains(string(out), stackChildDone) {
		t.Fatalf("the child ran no stack check (no %s marker):\n%s", stackChildDone, out)
	}
}

// ─── review r3 m-1: the type check's heap is capped, fail closed ───────────

// nestedThreshTree is review r3 m-1's tree: thresh(1, 0×31, thresh(1, …)),
// levels deep, a key at the bottom. The walk refuses it (a thresh child is
// not on the allowlist), so policyShape never type-checks it; a direct call
// used to hold every completed sibling on the type stack (about 3,100 types).
func nestedThreshTree(levels int) node {
	x := tpWrap(tagCheck, tpKey(tagPkK, 0))
	for i := 0; i < levels; i++ {
		cs := make([]node, 32)
		for j := range cs[:31] {
			cs[j] = tpEmpty(tagFalse)
		}
		cs[31] = x
		x = tpThresh(1, cs...)
	}
	return tpWsh(x)
}

// TestTypeCheckHeapIsCappedOnHostileTrees: past msMaxPending pending entries
// the check stops and answers false, so a hostile tree cannot make it
// allocate without bound. The answer is Rust's anyway: such a tree is refused
// by the walk, and so by policyShape, before the type check runs.
func TestTypeCheckHeapIsCappedOnHostileTrees(t *testing.T) {
	tree := nestedThreshTree(100)
	if walkShape(tree).Complete {
		t.Fatal("the walk accepts the nested thresh; the cap's parity argument no longer holds")
	}
	if treeTypeChecks(tree) {
		t.Fatal("the nested thresh type-checks")
	}
	// The stacks stop at msMaxPending entries, each a type or a work frame (a
	// node, an int, a bool); slice growth at most doubles what is live.
	entry := unsafe.Sizeof(msType{}) + unsafe.Sizeof(node{}) + 2*unsafe.Sizeof(int(0))
	budget := uint64(2 * msMaxPending * entry)
	for _, levels := range []int{100, 400} {
		tree := nestedThreshTree(levels)
		var before, after runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&before)
		treeTypeChecks(tree)
		runtime.ReadMemStats(&after)
		if got := after.TotalAlloc - before.TotalAlloc; got > budget {
			t.Errorf("type check of the %d-level nested thresh allocated %d bytes, budget %d", levels, got, budget)
		}
	}
}

// capMarginTrees are the typed, walk-complete trees that hold the most
// pending entries at the decoder's depth limit: andor chains through X (as
// and_n) and through Z, an and_v chain, a 32-child thresh at the bottom of
// one, and the typed, walk-complete max-depth cases. The chains are tap
// leaves (125 levels under tr), where no ops limit refuses them first.
func capMarginTrees(t *testing.T) map[string]node {
	t.Helper()
	pk := func(i uint8) node { return tpWrap(tagCheck, tpKey(tagPkK, i)) }
	chain := func(levels int, leaf node, wrap func(x node) node) node {
		for i := 0; i < levels; i++ {
			leaf = wrap(leaf)
		}
		return leaf
	}
	andor := func(x, y, z node) node {
		return node{tag: tagAndOr, body: childrenBody{children: []node{x, y, z}}}
	}
	andV := func(x node) node { return tpPair(tagAndV, tpWrap(tagVerify, pk(1)), x) }
	wide := make([]node, 32)
	wide[0] = pk(0)
	for i := 1; i < 32; i++ {
		wide[i] = tpWrap(tagSwap, pk(uint8(i)))
	}
	trees := map[string]node{
		"tr(andor through Z)":          tpTrLeaf(chain(125, pk(0), func(x node) node { return andor(pk(1), pk(2), x) })),
		"tr(and_n through X)":          tpTrLeaf(chain(125, pk(0), func(x node) node { return andor(x, pk(1), tpEmpty(tagFalse)) })),
		"tr(and_v chain)":              tpTrLeaf(chain(125, pk(0), andV)),
		"tr(thresh(32) at the bottom)": tpTrLeaf(chain(124, tpThresh(32, wide...), andV)),
	}
	for _, c := range depthCases(t) {
		if c.walk && c.typed {
			trees[c.name] = c.tree
		}
	}
	return trees
}

// capInner is the script msNode types for a wsh tree or a one-leaf-path tr
// tree (the right-most leaf of a taptree spine), and its context.
func capInner(tree node) (node, msCtx) {
	if tree.tag == tagTr {
		inner := *tree.body.(trBody).tree
		for inner.tag == tagTapTree {
			inner = inner.body.(childrenBody).children[1]
		}
		return inner, ctxTap
	}
	return tree.body.(childrenBody).children[0], ctxSegwitv0
}

// TestTypeCheckCapLeavesWalkAcceptedTreesAlone: the cap must never change an
// answer the walk lets through. On capMarginTrees the check types the tree at
// half the cap, the same as with none: a margin of two.
func TestTypeCheckCapLeavesWalkAcceptedTreesAlone(t *testing.T) {
	for name, tree := range capMarginTrees(t) {
		t.Run(name, func(t *testing.T) {
			if !walkShape(tree).Complete {
				t.Fatal("the walk refuses this tree; it does not test the cap's margin")
			}
			inner, ctx := capInner(tree)
			t2, ok2 := msNodeCapped(inner, ctx, 1<<30)
			if !ok2 {
				t.Fatal("untyped with no cap; a refusal at any cap would agree, so it tests nothing")
			}
			t1, ok1 := msNodeCapped(inner, ctx, msMaxPending/2)
			if ok1 != ok2 || t1 != t2 {
				t.Errorf("at half the cap: (%v, %+v); uncapped: (%v, %+v)", ok1, t1, ok2, t2)
			}
		})
	}
}
