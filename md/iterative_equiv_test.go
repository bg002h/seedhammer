package md

// Review r3 I-P1 rewrote every tree walk on the md1 load path (decode,
// placeholder validation, canonicalisation, re-encoding, the shape walk) from
// recursion to an explicit stack, so a max-depth card fits the device's 16 KB
// goroutine stack. The rewrite is structure only: these tests hold each
// iterative function to the recursive original kept in recursive_ref_test.go,
// result for result, error for error (value and message), reader position and
// bits written, on the vendored cases, the max-depth cards, and generated
// trees and bitstreams, well-formed and malformed.

import (
	"bytes"
	"fmt"
	"math/rand"
	"os"
	"reflect"
	"strconv"
	"testing"
)

// equivRounds is how many generated inputs each test draws. MD_EQUIV_ROUNDS
// raises it for a one-off deep run.
func equivRounds(t *testing.T, def int) int {
	t.Helper()
	if v := os.Getenv("MD_EQUIV_ROUNDS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			t.Fatalf("MD_EQUIV_ROUNDS: %v", err)
		}
		return n
	}
	return def
}

// catch runs f and returns what it panicked with, nil if it did not. Both
// versions must panic alike, or not at all.
func catch(f func()) (p any) {
	defer func() { p = recover() }()
	f()
	return nil
}

func sameErr(a, b error) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return reflect.DeepEqual(a, b) && a.Error() == b.Error()
}

// walkTags are the tags the shape walk and the type rule recognise; the
// generator draws from them half the time, so generated trees reach deep into
// the walk instead of failing at the first unknown tag.
var walkTags = []tag{
	tagPkK, tagPkH, tagCheck, tagVerify, tagSwap, tagAlt, tagDupIf, tagNonZero,
	tagZeroNotEqual, tagAndV, tagAndB, tagAndOr, tagOrB, tagOrC, tagOrD, tagOrI,
	tagThresh, tagAfter, tagOlder, tagSha256, tagMulti, tagSortedMulti,
	tagMultiA, tagFalse, tagTrue,
}

// eqTree draws a tree of at most depth levels: any tag, and usually the body
// the decoder would build for it, sometimes a wrong one (another body kind, a
// wrong arity, k outside 1..n, a nil body).
func eqTree(r *rand.Rand, depth int) node {
	var t tag
	if r.Intn(2) == 0 {
		t = walkTags[r.Intn(len(walkTags))]
	} else {
		t = tag(r.Intn(int(tagTrue) + 1))
	}
	if r.Intn(12) == 0 {
		return node{tag: t, body: eqBody(r, tag(r.Intn(int(tagTrue)+1)), depth)}
	}
	if r.Intn(40) == 0 {
		return node{tag: t}
	}
	return node{tag: t, body: eqBody(r, t, depth)}
}

func eqChildren(r *rand.Rand, n, depth int) []node {
	if depth <= 0 {
		n = 0
	}
	cs := make([]node, n)
	for i := range cs {
		cs[i] = eqTree(r, depth-1)
	}
	return cs
}

func eqArity(r *rand.Rand, want int) int {
	if r.Intn(15) == 0 {
		return r.Intn(4)
	}
	return want
}

func eqBody(r *rand.Rand, t tag, depth int) body {
	switch t {
	case tagPkK, tagPkH, tagWpkh, tagPkh:
		return keyArgBody{index: uint8(r.Intn(6))}
	case tagSh, tagWsh, tagCheck, tagVerify, tagSwap, tagAlt, tagDupIf, tagNonZero, tagZeroNotEqual:
		return childrenBody{children: eqChildren(r, eqArity(r, 1), depth)}
	case tagAndV, tagAndB, tagOrB, tagOrC, tagOrD, tagOrI, tagTapTree:
		return childrenBody{children: eqChildren(r, eqArity(r, 2), depth)}
	case tagAndOr:
		return childrenBody{children: eqChildren(r, eqArity(r, 3), depth)}
	case tagMulti, tagSortedMulti, tagMultiA, tagSortedMultiA:
		n := r.Intn(6)
		idx := make([]uint8, n)
		for i := range idx {
			idx[i] = uint8(r.Intn(6))
		}
		return multiKeysBody{k: uint8(r.Intn(n + 2)), indices: idx}
	case tagThresh:
		n := r.Intn(5)
		return variableBody{k: uint8(r.Intn(n + 2)), children: eqChildren(r, n, depth)}
	case tagTr:
		b := trBody{ik: InternalKeyKind(r.Intn(4)), keyIndex: uint8(r.Intn(6))}
		if depth > 0 && r.Intn(4) != 0 {
			sub := eqTree(r, depth-1)
			b.tree = &sub
		}
		return b
	case tagAfter, tagOlder:
		vs := []uint32{0, 1, 500_000_000, 1 << 22, 1<<31 - 1, 1 << 31, 0xFFFFFFFF}
		return timelockBody(vs[r.Intn(len(vs))])
	case tagSha256, tagHash256:
		var h hash256Body
		h[0] = byte(r.Intn(256))
		return h
	case tagHash160, tagRipemd160, tagRawPkH:
		var h hash160Body
		h[0] = byte(r.Intn(256))
		return h
	}
	return emptyBody{}
}

// eqCorpusTrees are the fixed trees every test runs: the vendored cases and
// the max-depth cards, as built and as decoded.
func eqCorpusTrees(t *testing.T) []node {
	t.Helper()
	var trees []node
	for _, c := range policyShapeCases(t) {
		trees = append(trees, buildNode(t, c.Tree))
		if c.MD1 != nil {
			d, err := Reassemble(c.MD1)
			if err != nil {
				t.Fatal(err)
			}
			trees = append(trees, d.tree)
		}
	}
	for _, c := range depthCases(t) {
		_, tree := depthCard(t, c.tree)
		trees = append(trees, c.tree, tree)
	}
	return trees
}

// eqEncode is the tree's bits at a kiw and version, or ok=false when the
// recursive writer refuses it.
func eqEncode(n node, kiw, version uint8) ([]byte, int, bool) {
	var w bitWriter
	if catch(func() {
		if err := writeNodeRec(&w, n, kiw, version); err != nil {
			w.bytes = nil
		}
	}) != nil || w.bytes == nil {
		return nil, 0, false
	}
	return w.intoBytes(), w.bitLen(), true
}

// decodeAgrees reads b with both decoders and reports any difference.
func decodeAgrees(b []byte, bitLimit int, kiw, version, depth uint8) string {
	r1, r2 := newBitReader(b, bitLimit), newBitReader(b, bitLimit)
	var n1, n2 node
	var e1, e2 error
	p1 := catch(func() { n1, e1 = readNodeDepthRec(r1, kiw, version, depth) })
	p2 := catch(func() { n2, e2 = readNodeDepth(r2, kiw, version, depth) })
	switch {
	case !reflect.DeepEqual(p1, p2):
		return fmt.Sprintf("panic %v vs %v", p1, p2)
	case !sameErr(e1, e2):
		return fmt.Sprintf("error %v vs %v", e1, e2)
	case !reflect.DeepEqual(n1, n2):
		return fmt.Sprintf("tree %+v vs %+v", n1, n2)
	case r1.bitPos != r2.bitPos:
		return fmt.Sprintf("reader at bit %d vs %d", r1.bitPos, r2.bitPos)
	}
	return ""
}

func TestIterativeDecodeMatchesRecursive(t *testing.T) {
	r := rand.New(rand.NewSource(20261126))
	check := func(what string, b []byte, bitLimit int, kiw, version, depth uint8) {
		t.Helper()
		if d := decodeAgrees(b, bitLimit, kiw, version, depth); d != "" {
			t.Fatalf("%s (kiw %d, v%d, depth %d, %x/%d bits): %s", what, kiw, version, depth, b, bitLimit, d)
		}
	}
	versions := []uint8{wfRedesignVersion, wfUnspendableVersion, 0}
	// Every fixed tree, encoded whole, truncated, and with bits flipped; and
	// started below the root, so the depth limit falls inside it.
	for i, tree := range eqCorpusTrees(t) {
		for _, version := range versions[:2] {
			b, bits, ok := eqEncode(tree, 5, version)
			if !ok {
				continue
			}
			what := fmt.Sprintf("corpus tree %d", i)
			check(what, b, bits, 5, version, 0)
			check(what, b, bits, 5, version, uint8(r.Intn(130)))
			check(what, b, r.Intn(bits+1), 5, version, 0)
			for f := 0; f < 8; f++ {
				fb := bytes.Clone(b)
				pos := r.Intn(bits)
				fb[pos/8] ^= 0x80 >> (pos % 8)
				check(what+" flipped", fb, bits, 5, version, 0)
			}
		}
	}
	// Generated trees, encoded.
	for i := 0; i < equivRounds(t, 20000); i++ {
		tree := eqTree(r, 1+r.Intn(8))
		kiw := uint8(r.Intn(6))
		version := versions[r.Intn(2)]
		b, bits, ok := eqEncode(tree, kiw, version)
		if !ok {
			continue
		}
		check("generated tree", b, bits, kiw, version, 0)
		if bits > 0 {
			check("generated tree, truncated", b, r.Intn(bits), kiw, version, 0)
			pos := r.Intn(bits)
			b[pos/8] ^= 0x80 >> (pos % 8)
			check("generated tree, flipped", b, bits, kiw, version, uint8(r.Intn(4)))
		}
	}
	// Random bitstreams: mostly malformed, with every error the reader has.
	for i := 0; i < equivRounds(t, 50000); i++ {
		b := make([]byte, 1+r.Intn(48))
		r.Read(b)
		limit := len(b) * 8
		if r.Intn(3) == 0 {
			limit = r.Intn(limit + 1)
		}
		check("random bits", b, limit, uint8(r.Intn(6)), versions[r.Intn(3)], uint8(r.Intn(3)))
	}
}

// shapeAgreesRec compares the iterative and recursive shape walks on one tree.
func shapeAgreesRec(tree node) string {
	var s1, s2 PolicyShape
	p1 := catch(func() { s1 = walkShapeRec(tree) })
	p2 := catch(func() { s2 = walkShape(tree) })
	if !reflect.DeepEqual(p1, p2) {
		return fmt.Sprintf("panic %v vs %v", p1, p2)
	}
	if !reflect.DeepEqual(s1, s2) {
		return fmt.Sprintf("shape %+v vs %+v", s1, s2)
	}
	return ""
}

func TestIterativeWalkMatchesRecursive(t *testing.T) {
	r := rand.New(rand.NewSource(20261127))
	for i, tree := range eqCorpusTrees(t) {
		if d := shapeAgreesRec(tree); d != "" {
			t.Fatalf("corpus tree %d: %s", i, d)
		}
	}
	roots := []tag{tagWsh, tagSh, tagTr, tagWpkh, tagPkh, tagAndV}
	complete := 0
	for i := 0; i < equivRounds(t, 40000); i++ {
		inner := eqTree(r, 1+r.Intn(8))
		var tree node
		switch root := roots[r.Intn(len(roots))]; root {
		case tagWsh, tagSh:
			tree = node{tag: root, body: childrenBody{children: []node{inner}}}
		case tagTr:
			// A taptree of generated leaves, sometimes a malformed TapTree node.
			leaf := inner
			for d := r.Intn(4); d > 0; d-- {
				kids := []node{leaf, eqTree(r, 3)}
				if r.Intn(2) == 0 {
					kids[0], kids[1] = kids[1], kids[0]
				}
				if r.Intn(20) == 0 {
					kids = kids[:1]
				}
				leaf = node{tag: tagTapTree, body: childrenBody{children: kids}}
			}
			tree = node{tag: tagTr, body: trBody{ik: InternalKeyKind(r.Intn(4)), tree: &leaf}}
		default:
			tree = inner
		}
		if d := shapeAgreesRec(tree); d != "" {
			t.Fatalf("generated tree %+v: %s", tree, d)
		}
		if walkShape(tree).Complete {
			complete++
		}
	}
	// The generator must reach the walk's accepting paths, not only its
	// refusals, or the comparison above proves little.
	if rounds := equivRounds(t, 40000); complete < rounds/50 {
		t.Errorf("only %d of %d generated trees walked complete", complete, rounds)
	}
}

// TestIterativeTreeHelpersMatchRecursive holds the other walks on the load
// path to their recursive originals: placeholder validation, the tap-leaf
// check, canonicalisation's walks and rebuilds, and the encoder.
func TestIterativeTreeHelpersMatchRecursive(t *testing.T) {
	r := rand.New(rand.NewSource(20261128))
	perm := make([]uint8, 256)
	for i := range perm {
		perm[i] = uint8(r.Intn(256))
	}
	compare := func(what string, f1, f2 func() any) {
		t.Helper()
		var v1, v2 any
		p1 := catch(func() { v1 = f1() })
		p2 := catch(func() { v2 = f2() })
		if !reflect.DeepEqual(p1, p2) {
			t.Fatalf("%s: panic %v vs %v", what, p1, p2)
		}
		if !reflect.DeepEqual(v1, v2) {
			t.Fatalf("%s: %+v vs %+v", what, v1, v2)
		}
	}
	type seenResult struct {
		Seen  []bool
		First []uint8
		Err   string
	}
	errText := func(err error) string {
		if err == nil {
			return ""
		}
		return fmt.Sprintf("%#v %s", err, err)
	}
	one := func(what string, tree node) {
		t.Helper()
		n := uint8(r.Intn(8))
		compare(what+": walkForPlaceholders", func() any {
			seen, first := make([]bool, n), []uint8(nil)
			err := walkForPlaceholdersRec(tree, seen, &first)
			return seenResult{seen, first, errText(err)}
		}, func() any {
			seen, first := make([]bool, n), []uint8(nil)
			err := walkForPlaceholders(tree, seen, &first)
			return seenResult{seen, first, errText(err)}
		})
		compare(what+": walkCollectFirst", func() any {
			seen, first := make([]bool, n), []uint8(nil)
			walkCollectFirstRec(tree, seen, &first)
			return seenResult{seen, first, ""}
		}, func() any {
			seen, first := make([]bool, n), []uint8(nil)
			walkCollectFirst(tree, seen, &first)
			return seenResult{seen, first, ""}
		})
		compare(what+": validateTapScriptTree", func() any { return errText(validateTapScriptTreeRec(tree)) },
			func() any { return errText(validateTapScriptTree(tree)) })
		compare(what+": checkPlaceholderBounds", func() any { return errText(checkPlaceholderBoundsRec(tree, n)) },
			func() any { return errText(checkPlaceholderBounds(tree, n)) })
		compare(what+": needsV8", func() any { return needsV8Rec(tree) }, func() any { return needsV8(tree) })
		compare(what+": cloneNode", func() any { return cloneNodeRec(tree) }, func() any { return cloneNode(tree) })
		compare(what+": remapIndices", func() any { return remapIndicesRec(tree, perm) },
			func() any { return remapIndices(tree, perm) })
		kiw, version := uint8(r.Intn(6)), []uint8{wfRedesignVersion, wfUnspendableVersion}[r.Intn(2)]
		type written struct {
			Bytes []byte
			Bits  int
			Err   string
		}
		compare(what+": writeNode", func() any {
			var w bitWriter
			err := writeNodeRec(&w, tree, kiw, version)
			return written{w.intoBytes(), w.bitLen(), errText(err)}
		}, func() any {
			var w bitWriter
			err := writeNode(&w, tree, kiw, version)
			return written{w.intoBytes(), w.bitLen(), errText(err)}
		})
	}
	for i, tree := range eqCorpusTrees(t) {
		one(fmt.Sprintf("corpus tree %d", i), tree)
	}
	for i := 0; i < equivRounds(t, 20000); i++ {
		one("generated tree", eqTree(r, 1+r.Intn(8)))
	}
	// A clone shares no slice with its source: changing the clone's deepest
	// key leaves the source as it was.
	for _, c := range depthCases(t) {
		src := c.tree
		before := cloneNodeRec(src)
		cp := cloneNode(src)
		eqMutateKeys(cp)
		if !reflect.DeepEqual(src, before) {
			t.Fatalf("%s: mutating cloneNode's copy changed its source", c.name)
		}
	}
}

// eqMutateKeys overwrites every key index and multi index in n, in place.
func eqMutateKeys(n node) {
	work := []node{n}
	for len(work) > 0 {
		n := work[len(work)-1]
		work = work[:len(work)-1]
		switch b := n.body.(type) {
		case childrenBody:
			for i := range b.children {
				if k, ok := b.children[i].body.(keyArgBody); ok {
					k.index ^= 0x55
					b.children[i].body = k
				}
			}
			work = append(work, b.children...)
		case variableBody:
			work = append(work, b.children...)
		case multiKeysBody:
			for i := range b.indices {
				b.indices[i] ^= 0x55
			}
		case trBody:
			if b.tree != nil {
				if k, ok := b.tree.body.(keyArgBody); ok {
					k.index ^= 0x55
					b.tree.body = k
				}
				work = append(work, *b.tree)
			}
		}
	}
}
