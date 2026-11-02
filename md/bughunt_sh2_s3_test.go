package md

import (
	"testing"

	btcaddr "github.com/btcsuite/btcd/address/v2"
)

// Bug-hunt triage, sh2 slice S3 (report bughunt-2026-10-31-sh2.md).

// bhTapChunks builds the md1 chunks of tr(NUMS, {LEFT, RIGHT}) over two keys.
func bhTapChunks(t *testing.T, left, right node) []string {
	t.Helper()
	tree := node{tag: tagTapTree, body: childrenBody{children: []node{left, right}}}
	shared := originPath{components: toComponents(DefaultOrigin(ComposeTr, 0))}
	d := &descriptor{
		n:        2,
		pathDecl: pathDecl{n: 2, shared: &shared},
		useSite: useSitePath{
			hasMultipath: true,
			multipath:    []alternative{{hardened: false, value: 0}, {hardened: false, value: 1}},
		},
		tree: node{tag: tagTr, body: trBody{ik: InternalKeyNUMS, keyIndex: 0, tree: &tree}},
	}
	chunks, err := split(d)
	if err != nil {
		t.Fatalf("split: %v", err)
	}
	return chunks
}

// Bug 34: describeTapLeaf folds the pkh(K) leaf (wire tagPkH, miniscript
// c:pk_h) into TapLeafPK, the shape that emits <xonly> OP_CHECKSIG, so a caller
// that turns the description into a script builds the wrong script. The
// accessor's contract is to refuse a leaf it cannot describe.
func TestBughuntSh234PkhLeafDescribedAsPk(t *testing.T) {
	pk := func(i uint8) node { return node{tag: tagPkK, body: keyArgBody{index: i}} }
	pkh := func(i uint8) node { return node{tag: tagPkH, body: keyArgBody{index: i}} }

	// Positive control: tr(NUMS, {pk(@0), pk(@1)}) is two TapLeafPK leaves.
	_, _, ctl, err := TapLeavesChunks(bhTapChunks(t, pk(0), pk(1)))
	if err != nil {
		t.Fatalf("control TapLeavesChunks: %v", err)
	}
	if len(ctl) != 2 || ctl[0].Kind != TapLeafPK || ctl[1].Kind != TapLeafPK {
		t.Fatalf("control leaves = %+v, want two TapLeafPK", ctl)
	}

	// The emitter, given the same tree, builds the hash160 form for the pkh leaf
	// (this is the correct script the description must not contradict).
	chunks := bhTapChunks(t, pkh(0), pk(1))
	x0 := make([]byte, 32)
	x1 := make([]byte, 32)
	for i := range x0 {
		x0[i], x1[i] = byte(i+1), byte(i+101)
	}
	_, _, scripts, err := EmitTapLeavesChunks(chunks, map[uint8][]byte{0: x0, 1: x1})
	if err != nil {
		t.Fatalf("EmitTapLeavesChunks: %v", err)
	}
	want := append([]byte{opDUP, opHASH160, 0x14}, btcaddr.Hash160(x0)...)
	want = append(want, opEQUALVERIFY, opCHECKSIG)
	if len(scripts) != 2 || string(scripts[0].Script) != string(want) {
		t.Fatalf("emitter control: leaf 0 script %x, want %x", scripts[0].Script, want)
	}

	// The accessor: refuse the pkh leaf, or describe it as something other than pk.
	_, _, leaves, err := TapLeavesChunks(chunks)
	if err != nil {
		return // refusing is the documented behaviour
	}
	if len(leaves) > 0 && leaves[0].Kind == TapLeafPK {
		t.Errorf("TapLeavesChunks described the pkh(@0) leaf as TapLeafPK (%+v), which emits <xonly> OP_CHECKSIG instead of DUP HASH160 <hash> EQUALVERIFY CHECKSIG", leaves[0])
	}
}
