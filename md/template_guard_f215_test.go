package md

import (
	"errors"
	"testing"
)

// F-215: the template-engrave guard refused two shapes on the grounds that the
// shipped off-device toolkit could not reconstruct them. One of those premises
// died; the other became unreachable. This pins what the guard does NOW.
//
// The measurement that moved it, taken on the current binaries rather than
// recalled: `tr(@0,sortedmulti_a(2,@0,@1))` encodes to one chunk, `md decode`
// returns the template verbatim at exit 0, `md verify` re-encodes it to its own
// template, and `md address` derives
// bc1p588jmtx4ptv76t9sclt6gt33eyydvsrea4njyayerqj2frw5m5aq5gzycw. Fully
// recoverable, which is the only thing this guard was ever asking.

func TestTemplateGuardAdmitsSortedMultiATapLeaf(t *testing.T) {
	// tr(NUMS, sortedmulti_a(2, @0, @1)) as an AST, which is what the guard sees.
	leaf := node{tag: tagSortedMultiA, body: multiKeysBody{k: 2, indices: []uint8{0, 1}}}
	tree := node{tag: tagTr, body: trBody{isNums: true, tree: &leaf}}
	if err := templateEngraveShapeGuard(&descriptor{tree: tree}); err != nil {
		t.Fatalf("a sortedmulti_a tap leaf is still refused: %v", err)
	}
}

// The OTHER shape stays refused, and the distinction is the point: a guard
// narrowed to nothing is a guard deleted, which is not what the measurement
// supports.
func TestTemplateGuardStillRefusesSortedMultiInACombinator(t *testing.T) {
	sm := node{tag: tagSortedMulti, body: multiKeysBody{k: 2, indices: []uint8{0, 1}}}
	// wsh(or_d(sortedmulti(...), ...)) — sortedmulti below a combinator.
	comb := node{tag: tagOrD, body: childrenBody{children: []node{sm, sm}}}
	tree := node{tag: tagWsh, body: childrenBody{children: []node{comb}}}
	err := templateEngraveShapeGuard(&descriptor{tree: tree})
	if !errors.Is(err, ErrTemplateUnsupportedShape) {
		t.Fatalf("sortedmulti under a combinator must still be refused; got %v", err)
	}
}

// A direct wsh(sortedmulti(...)) is on the canonical spine and always was fine.
func TestTemplateGuardAdmitsDirectWshSortedMulti(t *testing.T) {
	sm := node{tag: tagSortedMulti, body: multiKeysBody{k: 2, indices: []uint8{0, 1}}}
	tree := node{tag: tagWsh, body: childrenBody{children: []node{sm}}}
	if err := templateEngraveShapeGuard(&descriptor{tree: tree}); err != nil {
		t.Fatalf("wsh(sortedmulti(...)) was refused: %v", err)
	}
}
