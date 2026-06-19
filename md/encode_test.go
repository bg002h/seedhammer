package md

import (
	"encoding/hex"
	"testing"

	"seedhammer.com/codex32"
)

// ─── Task 3.1: per-writer bit-cost pins (M-1), locked before integration
// goldens. Each asserts bw.bitLen() after a single writer call, mirroring the
// Rust bit-cost tests. ───────────────────────────────────────────────────────

func TestWriteTagBitCost(t *testing.T) {
	// tag.rs write: 6-bit primary. R0-I1 — the tag write is mandatory on every
	// node arm; pin its width here.
	var w bitWriter
	writeTag(&w, tagWpkh)
	if w.bitLen() != 6 {
		t.Fatalf("writeTag bitLen=%d want 6", w.bitLen())
	}
	// Round-trip every primary code through readTag.
	for tg := tag(0); tg <= tagTrue; tg++ {
		var ww bitWriter
		writeTag(&ww, tg)
		r := newBitReader(ww.intoBytes(), ww.bitLen())
		got, err := readTag(r)
		if err != nil || got != tg {
			t.Fatalf("tag %#x round-trip got %#x err %v", tg, got, err)
		}
	}
}

func TestWriteVarintBitCost(t *testing.T) {
	cases := []struct {
		value uint32
		bits  int
	}{
		{0, 4},   // varint.rs:110
		{1, 5},   // varint.rs:117
		{84, 11}, // varint.rs:124
	}
	for _, c := range cases {
		var w bitWriter
		if err := writeVarint(&w, c.value); err != nil {
			t.Fatalf("writeVarint(%d): %v", c.value, err)
		}
		if w.bitLen() != c.bits {
			t.Fatalf("writeVarint(%d) bitLen=%d want %d", c.value, w.bitLen(), c.bits)
		}
		// Round-trip through readVarint.
		r := newBitReader(w.intoBytes(), w.bitLen())
		got, err := readVarint(r)
		if err != nil || got != c.value {
			t.Fatalf("varint %d round-trip got %d err %v", c.value, got, err)
		}
	}
}

func TestWriteVarintExtensionRoundTrip(t *testing.T) {
	for _, v := range []uint32{16383, 16384, 1024, (1 << 29) - 1} {
		var w bitWriter
		if err := writeVarint(&w, v); err != nil {
			t.Fatalf("writeVarint(%d): %v", v, err)
		}
		r := newBitReader(w.intoBytes(), w.bitLen())
		got, err := readVarint(r)
		if err != nil || got != v {
			t.Fatalf("varint %d round-trip got %d err %v", v, got, err)
		}
	}
}

func TestWriteVarintOverflow(t *testing.T) {
	var w bitWriter
	if err := writeVarint(&w, 1<<30); err != errVarintOverflow {
		t.Fatalf("writeVarint(1<<30) = %v, want errVarintOverflow", err)
	}
}

func TestWriteOriginPathBitCostBIP84(t *testing.T) {
	// origin_path.rs:184 — depth(4) + 84'(1+11) + 0'(1+4) + 0'(1+4) = 26 bits.
	p := originPath{components: []pathComponent{
		{hardened: true, value: 84},
		{hardened: true, value: 0},
		{hardened: true, value: 0},
	}}
	var w bitWriter
	if err := writeOriginPath(&w, p); err != nil {
		t.Fatalf("writeOriginPath: %v", err)
	}
	if w.bitLen() != 26 {
		t.Fatalf("origin path bitLen=%d want 26", w.bitLen())
	}
}

func TestWritePathDeclBitCostBIP84(t *testing.T) {
	// origin_path.rs:243 — n(5) + 26 = 31 bits.
	p := pathDecl{n: 1, shared: &originPath{components: []pathComponent{
		{hardened: true, value: 84},
		{hardened: true, value: 0},
		{hardened: true, value: 0},
	}}}
	var w bitWriter
	if err := writePathDecl(&w, p); err != nil {
		t.Fatalf("writePathDecl: %v", err)
	}
	if w.bitLen() != 31 {
		t.Fatalf("path decl bitLen=%d want 31", w.bitLen())
	}
}

func TestWriteUseSiteBitCostStandard(t *testing.T) {
	// use_site_path.rs:134 — <0;1>/* = has-mp(1)+count-2(3)+alt0(1+4)+alt1(1+5)+wildcard(1) = 16.
	us := useSitePath{hasMultipath: true, multipath: []alternative{{value: 0}, {value: 1}}}
	var w bitWriter
	if err := writeUseSitePath(&w, us); err != nil {
		t.Fatalf("writeUseSitePath: %v", err)
	}
	if w.bitLen() != 16 {
		t.Fatalf("use-site bitLen=%d want 16", w.bitLen())
	}
}

func TestWriteUseSiteBareStarBitCost(t *testing.T) {
	// has-mp(0) + wildcard(0) = 2 bits.
	us := useSitePath{hasMultipath: false}
	var w bitWriter
	if err := writeUseSitePath(&w, us); err != nil {
		t.Fatalf("writeUseSitePath: %v", err)
	}
	if w.bitLen() != 2 {
		t.Fatalf("bare-star bitLen=%d want 2", w.bitLen())
	}
}

func TestWriteNodeSortedMultiBitCost(t *testing.T) {
	// tree.rs:411 — sortedmulti(2-of-3) MultiKeys at n=3 (kiw=2):
	// Tag(6) + (k-1)(5) + (n-1)(5) + 3×kiw(2) = 22 bits.
	n := node{tag: tagSortedMulti, body: multiKeysBody{k: 2, indices: []uint8{0, 1, 2}}}
	var w bitWriter
	if err := writeNode(&w, n, 2); err != nil {
		t.Fatalf("writeNode: %v", err)
	}
	if w.bitLen() != 22 {
		t.Fatalf("sortedmulti bitLen=%d want 22", w.bitLen())
	}
}

func TestWriteNodeKeyArgN1ZeroBits(t *testing.T) {
	// tree.rs:336 — at n=1, kiw=0, key-arg emits zero bits: Tag(6)+0 = 6.
	n := node{tag: tagPkK, body: keyArgBody{index: 0}}
	var w bitWriter
	if err := writeNode(&w, n, 0); err != nil {
		t.Fatalf("writeNode: %v", err)
	}
	if w.bitLen() != 6 {
		t.Fatalf("keyarg n=1 bitLen=%d want 6", w.bitLen())
	}
}

func TestWriteNodeTrNumsSuppressesKiw(t *testing.T) {
	// tree.rs:693 — tr(NUMS) at any kiw: Tag(6)+is_nums(1)+has_tree(1) = 8.
	n := node{tag: tagTr, body: trBody{isNums: true}}
	var w bitWriter
	if err := writeNode(&w, n, 2); err != nil {
		t.Fatalf("writeNode: %v", err)
	}
	if w.bitLen() != 8 {
		t.Fatalf("tr-nums bitLen=%d want 8", w.bitLen())
	}
}

func TestWriteHeaderCommonByte(t *testing.T) {
	// header.rs:114 — version=4, shared → common byte 0x20.
	var w bitWriter
	writeHeader(&w, header{version: wfRedesignVersion, divergentPaths: false})
	if got := w.intoBytes(); len(got) != 1 || got[0] != 0x20 {
		t.Fatalf("header bytes=%#v want [0x20]", got)
	}
}

func TestKiw(t *testing.T) {
	// encode.rs:37 — kiw = 32-leadingZeros(n-1), clamp 0 at n in {0,1}.
	cases := map[uint8]uint8{0: 0, 1: 0, 2: 1, 3: 2, 4: 2, 5: 3, 8: 3, 9: 4, 16: 4, 17: 5, 32: 5}
	for n, want := range cases {
		if got := kiw(n); got != want {
			t.Fatalf("kiw(%d)=%d want %d", n, got, want)
		}
	}
}

// ─── Task 3.5: PRIMARY GATE — byte-exact golden parity (§5.1). ────────────────

func TestEncodePayloadGoldens(t *testing.T) {
	for _, name := range byteParityVectorNames {
		t.Run(name, func(t *testing.T) {
			d := loadDescriptor(t, name)
			gotBytes, gotBits, err := encodePayload(d)
			if err != nil {
				t.Fatalf("encodePayload: %v", err)
			}
			want := loadBytesHex(t, name)
			if hex.EncodeToString(gotBytes) != hex.EncodeToString(want) {
				t.Fatalf("byte mismatch:\n got %s\nwant %s", hex.EncodeToString(gotBytes), hex.EncodeToString(want))
			}
			// bitLen must be consistent: ceil(bits/8) == len(bytes), and the
			// payload round-trips through the decoder at the reported bitLen.
			if (gotBits+7)/8 != len(gotBytes) {
				t.Fatalf("bitLen %d inconsistent with %d bytes", gotBits, len(gotBytes))
			}
			if _, err := decodePayloadValidated(gotBytes, gotBits); err != nil {
				t.Fatalf("re-decode at bitLen %d: %v", gotBits, err)
			}
		})
	}
}

// TestEncodePayloadCanonicalEquality (Task 2/3): encodePayload of a permuted
// descriptor equals encodePayload of its canonical form — canonicalize runs
// inside encodePayload, so a non-canonical input produces the canonical bytes.
func TestEncodePayloadCanonicalEquality(t *testing.T) {
	// Canonical: wsh(multi(2,@0,@1)) n=2.
	canon := &descriptor{
		n:        2,
		pathDecl: pathDecl{n: 2, shared: &originPath{}},
		useSite:  useSitePath{hasMultipath: true, multipath: []alternative{{value: 0}, {value: 1}}},
		tree: node{tag: tagWsh, body: childrenBody{children: []node{{
			tag: tagMulti, body: multiKeysBody{k: 2, indices: []uint8{0, 1}},
		}}}},
	}
	// Permuted: indices [1,0] — first-use @1 then @0.
	permuted := &descriptor{
		n:        2,
		pathDecl: pathDecl{n: 2, shared: &originPath{}},
		useSite:  useSitePath{hasMultipath: true, multipath: []alternative{{value: 0}, {value: 1}}},
		tree: node{tag: tagWsh, body: childrenBody{children: []node{{
			tag: tagMulti, body: multiKeysBody{k: 2, indices: []uint8{1, 0}},
		}}}},
	}
	cb, _, err := encodePayload(canon)
	if err != nil {
		t.Fatalf("encode canon: %v", err)
	}
	pb, _, err := encodePayload(permuted)
	if err != nil {
		t.Fatalf("encode permuted: %v", err)
	}
	if hex.EncodeToString(cb) != hex.EncodeToString(pb) {
		t.Fatalf("encodePayload(permuted) %s != encodePayload(canonical) %s",
			hex.EncodeToString(pb), hex.EncodeToString(cb))
	}
}

// ─── Task 5: full-string parity — encodeMD1String == .phrase.txt for SINGLE
// vectors (R0-M3: force-chunked excluded). ValidMD is the independent BCH check.

func TestEncodeMD1StringGoldens(t *testing.T) {
	for _, name := range singleStringVectorNames {
		t.Run(name, func(t *testing.T) {
			d := loadDescriptor(t, name)
			got, err := encodeMD1String(d)
			if err != nil {
				t.Fatalf("encodeMD1String: %v", err)
			}
			want := loadPhrase(t, name)
			if got != want {
				t.Fatalf("md1 string mismatch:\n got %s\nwant %s", got, want)
			}
			if !codex32.ValidMD(got) {
				t.Fatalf("encodeMD1String output fails ValidMD: %s", got)
			}
		})
	}
}
