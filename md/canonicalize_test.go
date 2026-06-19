package md

import (
	"reflect"
	"testing"

	"seedhammer.com/codex32"
)

// decodePhraseToDescriptor decodes a single-string md1 phrase into the internal
// *descriptor AST (the front half of Decode), for canonicalize/encode tests.
func decodePhraseToDescriptor(t *testing.T, phrase string) *descriptor {
	t.Helper()
	syms, err := codex32.MDDataSymbols(phrase)
	if err != nil {
		t.Fatalf("MDDataSymbols(%q): %v", phrase, err)
	}
	if len(syms) == 0 || syms[0]&1 == 1 {
		t.Fatalf("phrase %q is chunked or empty", phrase)
	}
	b := symbolsToBytes(syms)
	d, err := decodePayloadValidated(b, 5*len(syms))
	if err != nil {
		t.Fatalf("decodePayloadValidated(%q): %v", phrase, err)
	}
	return d
}

// TestCanonicalizeIdempotentOnGoldens: decoding a renderable golden yields a
// canonical AST; canonicalize on it must be a structural no-op (deep-equal).
func TestCanonicalizeIdempotentOnGoldens(t *testing.T) {
	for _, name := range singleStringVectorNames {
		t.Run(name, func(t *testing.T) {
			phrase := loadPhrase(t, name)
			d := decodePhraseToDescriptor(t, phrase)
			c, err := canonicalize(d)
			if err != nil {
				t.Fatalf("canonicalize: %v", err)
			}
			if !reflect.DeepEqual(d, c) {
				t.Fatalf("canonicalize changed an already-canonical descriptor:\n in=%+v\nout=%+v", d, c)
			}
		})
	}
}

// TestCanonicalizeNormalizesPermuted: a hand-built descriptor whose tree uses
// @1 before @0 (with a divergent path decl + FP TLV keyed by the OLD indices)
// is re-assigned by first-use, and every per-@N entry moves with its key.
func TestCanonicalizeNormalizesPermuted(t *testing.T) {
	// wsh(multi(2, @1, @0)) with n=2, divergent paths [pathFor@0, pathFor@1],
	// fingerprints {0: fp0, 1: fp1}. First-use order is @1 then @0, so
	// perm[1]=0, perm[0]=1. After canonicalize:
	//   tree indices become [0,1] (was [1,0]),
	//   divergent[new0] = old divergent[inverse[0]=1], divergent[new1]=old[0],
	//   fingerprints re-keyed: {0:fp1, 1:fp0} sorted ascending.
	path0 := originPath{components: []pathComponent{{hardened: true, value: 84}}}
	path1 := originPath{components: []pathComponent{{hardened: true, value: 48}}}
	fp0 := [4]byte{0x00, 0x00, 0x00, 0x00}
	fp1 := [4]byte{0x11, 0x11, 0x11, 0x11}

	d := &descriptor{
		n: 2,
		pathDecl: pathDecl{
			n:         2,
			divergent: []originPath{path0, path1}, // index by OLD key id
		},
		useSite: useSitePath{
			hasMultipath: true,
			multipath:    []alternative{{value: 0}, {value: 1}},
		},
		tree: node{
			tag: tagWsh,
			body: childrenBody{children: []node{{
				tag:  tagMulti,
				body: multiKeysBody{k: 2, indices: []uint8{1, 0}}, // @1 then @0
			}}},
		},
		tlv: tlvSection{
			fpPresent:    true,
			fingerprints: []idxFP{{idx: 0, fp: fp0}, {idx: 1, fp: fp1}},
		},
	}

	c, err := canonicalize(d)
	if err != nil {
		t.Fatalf("canonicalize: %v", err)
	}

	// Tree indices re-assigned: @1 first → 0, @0 second → 1.
	mk := c.tree.body.(childrenBody).children[0].body.(multiKeysBody)
	if got := mk.indices; got[0] != 0 || got[1] != 1 {
		t.Fatalf("tree indices = %v, want [0 1]", got)
	}

	// Divergent paths moved: new[0] = old[1] (path1), new[1] = old[0] (path0).
	if !reflect.DeepEqual(c.pathDecl.divergent[0], path1) {
		t.Fatalf("divergent[0] = %+v, want path1 %+v", c.pathDecl.divergent[0], path1)
	}
	if !reflect.DeepEqual(c.pathDecl.divergent[1], path0) {
		t.Fatalf("divergent[1] = %+v, want path0 %+v", c.pathDecl.divergent[1], path0)
	}

	// Fingerprints re-keyed and re-sorted: old idx0(fp0)→new1, old idx1(fp1)→new0.
	if len(c.tlv.fingerprints) != 2 {
		t.Fatalf("fingerprints len = %d", len(c.tlv.fingerprints))
	}
	if c.tlv.fingerprints[0].idx != 0 || c.tlv.fingerprints[0].fp != fp1 {
		t.Fatalf("fingerprints[0] = %+v, want {0, fp1}", c.tlv.fingerprints[0])
	}
	if c.tlv.fingerprints[1].idx != 1 || c.tlv.fingerprints[1].fp != fp0 {
		t.Fatalf("fingerprints[1] = %+v, want {1, fp0}", c.tlv.fingerprints[1])
	}

	// Input must be untouched (operates on a clone).
	inMk := d.tree.body.(childrenBody).children[0].body.(multiKeysBody)
	if inMk.indices[0] != 1 || inMk.indices[1] != 0 {
		t.Fatalf("canonicalize mutated its input: %v", inMk.indices)
	}
}

// TestCanonicalizePlaceholderNotReferenced (M-8): a descriptor whose tree never
// references @1 (with n=2) is rejected — the permutation is under-specified.
func TestCanonicalizePlaceholderNotReferenced(t *testing.T) {
	d := &descriptor{
		n:        2,
		pathDecl: pathDecl{n: 2, shared: &originPath{}},
		useSite:  useSitePath{hasMultipath: true, multipath: []alternative{{value: 0}, {value: 1}}},
		tree: node{
			tag:  tagWpkh,
			body: keyArgBody{index: 0}, // only @0 referenced; @1 missing
		},
	}
	if _, err := canonicalize(d); err != errPlaceholderNotReferenced {
		t.Fatalf("canonicalize = %v, want errPlaceholderNotReferenced", err)
	}
}

// TestCanonicalizeIdentityFastPath (M-8): an already-canonical descriptor is
// returned structurally identical (the perm==identity fast path).
func TestCanonicalizeIdentityFastPath(t *testing.T) {
	d := &descriptor{
		n:        1,
		pathDecl: pathDecl{n: 1, shared: &originPath{}},
		useSite:  useSitePath{hasMultipath: true, multipath: []alternative{{value: 0}, {value: 1}}},
		tree:     node{tag: tagWpkh, body: keyArgBody{index: 0}},
	}
	c, err := canonicalize(d)
	if err != nil {
		t.Fatalf("canonicalize: %v", err)
	}
	if !reflect.DeepEqual(d, c) {
		t.Fatalf("identity fast path changed the descriptor:\n in=%+v\nout=%+v", d, c)
	}
	// And it is a distinct object (a clone), not the same pointer.
	if d == c {
		t.Fatal("canonicalize returned the same pointer, not a clone")
	}
}

// TestCanonicalizeOutOfRange: a tree referencing @i with i>=n is rejected.
func TestCanonicalizeOutOfRange(t *testing.T) {
	d := &descriptor{
		n:        1,
		pathDecl: pathDecl{n: 1, shared: &originPath{}},
		useSite:  useSitePath{hasMultipath: true, multipath: []alternative{{value: 0}, {value: 1}}},
		tree:     node{tag: tagWpkh, body: keyArgBody{index: 5}}, // @5 with n=1
	}
	if _, err := canonicalize(d); err != errPlaceholderRange {
		t.Fatalf("canonicalize = %v, want errPlaceholderRange", err)
	}
}
