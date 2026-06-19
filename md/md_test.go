package md

import (
	"encoding/hex"
	"errors"
	"testing"
)

type tvec struct {
	name   string
	phrase string // verbatim tests/vectors/<name>.phrase.txt (single md1 line)
	n      int
	root   ScriptKind
	policy PolicyKind
	k, m   int
	keys   []KeyOrigin
	render bool
}

var parity = []tvec{
	{"wpkh_basic", "md1yqpqqxqq8xtwhw4xwn4qh", 1, ScriptWpkh, PolicySingle, 0, 0,
		[]KeyOrigin{{Index: 0, Fingerprint: "", OriginPath: "m", UseSite: "<0;1>/*"}}, true},
	{"pkh_basic", "md1yqpqqxzq2qwfv8urt848e", 1, ScriptPkh, PolicySingle, 0, 0,
		[]KeyOrigin{{Index: 0, Fingerprint: "", OriginPath: "m", UseSite: "<0;1>/*"}}, true},
	{"wsh_multi_2of2", "md1yppqqxppsg2vlumagltz27le", 2, ScriptWsh, PolicyMulti, 2, 2,
		[]KeyOrigin{{0, "", "m", "<0;1>/*"}, {1, "", "m", "<0;1>/*"}}, true},
	{"wsh_multi_2of3", "md1yzpqqxppsgsc8dua4tu0kekyl", 3, ScriptWsh, PolicyMulti, 2, 3,
		[]KeyOrigin{{0, "", "m", "<0;1>/*"}, {1, "", "m", "<0;1>/*"}, {2, "", "m", "<0;1>/*"}}, true},
	{"wsh_sortedmulti", "md1yzpqqxppcgsc9kdmw6d5dp08f", 3, ScriptWsh, PolicySortedMulti, 2, 3,
		[]KeyOrigin{{0, "", "m", "<0;1>/*"}, {1, "", "m", "<0;1>/*"}, {2, "", "m", "<0;1>/*"}}, true},
	{"tr_keyonly", "md1yqpqqxqsqgprhfjpjaz6d", 1, ScriptTr, PolicySingle, 0, 0,
		[]KeyOrigin{{0, "", "m", "<0;1>/*"}}, true},
	{"sh_wsh_multi", "md1yppqqxpsscy96gddy0v67f8tp", 2, ScriptSh, PolicyMulti, 2, 2,
		[]KeyOrigin{{0, "", "m", "<0;1>/*"}, {1, "", "m", "<0;1>/*"}}, true},
	{"wsh_with_fingerprints", "md1yppqqxppsg2z7zdatd7aljh7h2lqp277wajaesknu", 2, ScriptWsh, PolicyMulti, 2, 2,
		[]KeyOrigin{{0, "deadbeef", "m", "<0;1>/*"}, {1, "cafebabe", "m", "<0;1>/*"}}, true},
	{"wsh_divergent_paths", "md1yppqqxppsg2qknq2zc2ktzhwekmddzh", 2, ScriptWsh, PolicyMulti, 2, 2,
		[]KeyOrigin{{0, "", "m", "<0;1>/*"}, {1, "", "m", "<2;3>/*"}}, true},
}

func TestDecodeParity(t *testing.T) {
	for _, v := range parity {
		t.Run(v.name, func(t *testing.T) {
			tpl, err := Decode(v.phrase)
			if err != nil {
				t.Fatalf("Decode: %v", err)
			}
			if tpl.N != v.n || tpl.Root != v.root || tpl.Policy != v.policy ||
				tpl.K != v.k || tpl.M != v.m || tpl.Renderable != v.render {
				t.Fatalf("got %+v want n=%d root=%v pol=%v k=%d m=%d render=%v",
					tpl, v.n, v.root, v.policy, v.k, v.m, v.render)
			}
			if len(tpl.Keys) != len(v.keys) {
				t.Fatalf("keys=%d want %d", len(tpl.Keys), len(v.keys))
			}
			for i, k := range v.keys {
				if tpl.Keys[i] != k {
					t.Fatalf("key %d = %+v want %+v", i, tpl.Keys[i], k)
				}
			}
		})
	}
}

func TestDecodeChunkedRefused(t *testing.T) {
	// wsh_multi_chunked: the md1 chunk line (line 2 of phrase.txt, after the
	// "chunk-set-id:" comment) — verbatim.
	const chunk = "md1fz4awqqpqsgqpsgvyyxqql8saf74dwdyqv"
	if _, err := Decode(chunk); !errors.Is(err, ErrChunkedUnsupported) {
		t.Fatalf("chunked md1: want ErrChunkedUnsupported, got %v", err)
	}
}

// ─── FOLD C — IMPORTANT-1 (spec §6): negative + Renderable classification ─────
//
// Provenance of the bit-packed payloads below: each is built white-box from
// the verified md-codec 0.36.0 wire layout (the same layout this package
// ports), via the local testBitWriter, and fed directly to
// decodePayloadValidated(bytes, bitLen). The intent is documented per case.
// Round-trip vectors (the md1 strings in TestDecodeRenderableClassification)
// are sourced from the md-codec encoder (the `md` CLI / a one-off
// encode_md1_string harness) — see that test's comments. Assertions are by
// error CATEGORY (errors.Is on the sentinel), never string-equality, because
// the Go error strings are independent of Rust's.

// testBitWriter is an MSB-first bit packer used ONLY by the tests to construct
// negative wire payloads from the documented layout. It mirrors md-codec's
// BitWriter (and this package's symbolsToBytes / readUnknownPayload packing).
type testBitWriter struct {
	out     []byte
	cur     byte
	curBits int
	bitLen  int
}

func (w *testBitWriter) write(v uint64, count int) {
	for i := count - 1; i >= 0; i-- {
		bit := byte((v >> uint(i)) & 1)
		w.cur = (w.cur << 1) | bit
		w.curBits++
		w.bitLen++
		if w.curBits == 8 {
			w.out = append(w.out, w.cur)
			w.cur = 0
			w.curBits = 0
		}
	}
}

// bytes returns the zero-padded byte slice and the exact bit length.
func (w *testBitWriter) bytes() ([]byte, int) {
	out := append([]byte(nil), w.out...)
	if w.curBits > 0 {
		out = append(out, w.cur<<uint(8-w.curBits))
	}
	return out, w.bitLen
}

// writeHeader writes the 5-bit header: bit4 = divergent, bits3..0 = version.
func (w *testBitWriter) writeHeader(divergent bool, version uint8) {
	var b uint64
	if divergent {
		b |= 1 << 4
	}
	b |= uint64(version & 0b1111)
	w.write(b, 5)
}

// writeSharedPathDecl writes n-1 (5 bits) then a single shared origin path of
// the given depth-0 (empty) form. depth=0 means "elided origin".
func (w *testBitWriter) writeEmptySharedPathDecl(n uint8) {
	w.write(uint64(n-1), 5) // n encoded as n-1
	w.write(0, 4)           // shared origin path: depth 0 (no components)
}

// writeStdUseSite writes the standard <0;1>/* use-site: has_mp(1)=1,
// alt_count-2(3)=0, alt0 {h=0, varint(0)}, alt1 {h=0, varint(1)}, wildcard(1)=0.
func (w *testBitWriter) writeStdUseSite() {
	w.write(1, 1) // has_multipath
	w.write(0, 3) // alt_count - 2 == 0 → 2 alternatives
	// alt 0: hardened=0, varint value 0 (4-bit len-prefix 1, then 1 bit = 0)
	w.write(0, 1)
	w.write(1, 4) // varint len = 1
	w.write(0, 1) // value 0
	// alt 1: hardened=0, varint value 1
	w.write(0, 1)
	w.write(1, 4) // varint len = 1
	w.write(1, 1) // value 1
	w.write(0, 1) // wildcard not hardened
}

func TestDecodeNegative(t *testing.T) {
	// kiw for n: ⌈log₂(n)⌉. n=2 → kiw=1; n=1 → kiw=0.
	cases := []struct {
		name    string
		build   func() ([]byte, int)
		wantErr error
	}{
		{
			// Header version field != 4 (set to 3) → WireVersionMismatch.
			name: "wire_version_not_4",
			build: func() ([]byte, int) {
				w := &testBitWriter{}
				w.writeHeader(false, 3)
				return w.bytes()
			},
			wantErr: errWireVersion,
		},
		{
			// Root tag = 0x3F extension prefix → consumes 4-bit subcode, rejects.
			name: "reserved_extension_root_tag",
			build: func() ([]byte, int) {
				w := &testBitWriter{}
				w.writeHeader(false, 4)
				w.writeEmptySharedPathDecl(1)
				w.writeStdUseSite()
				w.write(0x3F, 6) // extension prefix tag
				w.write(0, 4)    // 4-bit subcode
				return w.bytes()
			},
			wantErr: errTagOutOfRange,
		},
		{
			// Root tag = 0x24 (first reserved 6-bit tag, > tagTrue) → out of range.
			name: "reserved_root_tag_0x24",
			build: func() ([]byte, int) {
				w := &testBitWriter{}
				w.writeHeader(false, 4)
				w.writeEmptySharedPathDecl(1)
				w.writeStdUseSite()
				w.write(0x24, 6)
				return w.bytes()
			},
			wantErr: errTagOutOfRange,
		},
		{
			// Root tag = multi (0x06) — a valid tag but NOT a canonical root
			// (root ∉ {Sh,Wsh,Wpkh,Pkh,Tr}) → OperatorContextViolation.
			name: "non_canonical_root_multi",
			build: func() ([]byte, int) {
				w := &testBitWriter{}
				w.writeHeader(false, 4)
				w.writeEmptySharedPathDecl(2) // n=2 → kiw=1
				w.writeStdUseSite()
				w.write(uint64(tagMulti), 6)
				w.write(2-1, 5) // k-1 = 1 → k=2
				w.write(2-1, 5) // count-1 = 1 → n=2
				w.write(0, 1)   // idx @0 (kiw=1)
				w.write(1, 1)   // idx @1
				return w.bytes()
			},
			wantErr: errOperatorContext,
		},
		{
			// K > N inside a wsh(multi): k=3, count=2 → KGreaterThanN.
			name: "k_greater_than_n",
			build: func() ([]byte, int) {
				w := &testBitWriter{}
				w.writeHeader(false, 4)
				w.writeEmptySharedPathDecl(2) // n=2 → kiw=1
				w.writeStdUseSite()
				w.write(uint64(tagWsh), 6)
				w.write(uint64(tagMulti), 6)
				w.write(3-1, 5) // k-1 = 2 → k=3
				w.write(2-1, 5) // count-1 = 1 → n=2  (k>count)
				return w.bytes()
			},
			wantErr: errKGreaterThanN,
		},
		{
			// Deeply nested sh(sh(sh(...))) exceeding MAX_DECODE_DEPTH=128.
			name: "depth_exceeded",
			build: func() ([]byte, int) {
				w := &testBitWriter{}
				w.writeHeader(false, 4)
				w.writeEmptySharedPathDecl(1)
				w.writeStdUseSite()
				// 130 nested Sh wrappers, then a wpkh(@0) leaf — the recursion
				// trips the depth>=128 guard before reaching the leaf.
				for i := 0; i < 130; i++ {
					w.write(uint64(tagSh), 6)
				}
				w.write(uint64(tagWpkh), 6)
				// no key index needed — guard fires first
				return w.bytes()
			},
			wantErr: errDepthExceeded,
		},
		{
			// Truncation: header says version 4 but the stream ends before the
			// path decl / use-site / root tag can be read.
			name: "truncated_after_header",
			build: func() ([]byte, int) {
				w := &testBitWriter{}
				w.writeHeader(false, 4) // only 5 bits total
				return w.bytes()
			},
			wantErr: errTruncated,
		},
		{
			// Placeholder index out of range: wpkh(@1) but n=1 (only @0 valid).
			// kiw for n=1 is 0, so a key index cannot encode @1 on the wire;
			// instead use n=2 (kiw=1) with a single wpkh(@1) — @0 is never
			// referenced → PlaceholderNotReferenced (also a validator reject,
			// distinct from range). Build the in-range-but-unreferenced case.
			name: "placeholder_not_referenced",
			build: func() ([]byte, int) {
				w := &testBitWriter{}
				w.writeHeader(false, 4)
				w.writeEmptySharedPathDecl(2) // n=2 → kiw=1
				w.writeStdUseSite()
				w.write(uint64(tagWpkh), 6)
				w.write(1, 1) // wpkh(@1) — @0 never referenced
				return w.bytes()
			},
			wantErr: errPlaceholderNotReferenced,
		},
		{
			// Placeholder first-occurrence out of order: wsh(multi(2,@1,@0)).
			// Both @0 and @1 referenced, but @1 first appears before @0 in
			// pre-order → PlaceholderFirstOccurrenceOutOfOrder.
			name: "placeholder_order",
			build: func() ([]byte, int) {
				w := &testBitWriter{}
				w.writeHeader(false, 4)
				w.writeEmptySharedPathDecl(2) // n=2 → kiw=1
				w.writeStdUseSite()
				w.write(uint64(tagWsh), 6)
				w.write(uint64(tagMulti), 6)
				w.write(2-1, 5) // k=2
				w.write(2-1, 5) // count=2
				w.write(1, 1)   // @1 first
				w.write(0, 1)   // @0 second
				return w.bytes()
			},
			wantErr: errPlaceholderOrder,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b, bitLen := c.build()
			var d *descriptor
			var err error
			func() {
				defer func() {
					if p := recover(); p != nil {
						t.Fatalf("decode panicked: %v", p)
					}
				}()
				d, err = decodePayloadValidated(b, bitLen)
			}()
			if err == nil {
				t.Fatalf("want error %v, got nil (d=%+v)", c.wantErr, d)
			}
			if !errors.Is(err, c.wantErr) {
				t.Fatalf("want errors.Is(_, %v); got %v", c.wantErr, err)
			}
			if d != nil {
				t.Fatalf("partial descriptor returned alongside error: %+v", d)
			}
		})
	}
}

// TestDecodeRenderableClassification exercises the decode-side Renderable
// classification (NOT a hand-built Template literal) over real md-codec-encoded
// md1 strings. All strings below were produced by the md-codec 0.36.0 encoder
// (`md encode` CLI for the first two, a one-off encode_md1_string harness for
// the tr(NUMS,…) case which the CLI's string parser can't express) and copied
// verbatim. Their bytecodes are recorded for provenance in the comments.
func TestDecodeRenderableClassification(t *testing.T) {
	t.Run("wsh_and_v_complex_renderable_false", func(t *testing.T) {
		// `md encode "wsh(and_v(v:pk(@0/48'/0'/0'/2'/<0;1>/*),older(144)))"`
		// → md1yqfdsssj5qqcynxjnsqqqqys4xvkthlcn66xz
		// bytecode: 2012d84212a001824cd29c00000090 (120 bits).
		// Valid + passes all 5 validators (explicit shared origin m/48'/0'/0'/2'),
		// but the wsh body is and_v(...), outside §4.2 → Renderable=false.
		const s = "md1yqfdsssj5qqcynxjnsqqqqys4xvkthlcn66xz"
		tpl, err := Decode(s)
		if err != nil {
			t.Fatalf("Decode: %v", err)
		}
		if tpl.Renderable {
			t.Fatalf("wsh(and_v(...)) must be Renderable=false; got %+v", tpl)
		}
		if tpl.Policy != PolicyComplex {
			t.Fatalf("policy = %v, want PolicyComplex", tpl.Policy)
		}
		if tpl.Root != ScriptWsh || tpl.N != 1 {
			t.Fatalf("root/N = %v/%d, want ScriptWsh/1", tpl.Root, tpl.N)
		}
		if tpl.K != 0 || tpl.M != 0 {
			t.Fatalf("complex shape must make no k/m policy claim; got k=%d m=%d", tpl.K, tpl.M)
		}
		// Keys/origins still populated even though Renderable=false.
		if len(tpl.Keys) != 1 || tpl.Keys[0].OriginPath != "m/48h/0h/0h/2h" {
			t.Fatalf("keys = %+v, want [@0 origin m/48h/0h/0h/2h]", tpl.Keys)
		}
	})

	t.Run("sh_multi_explicit_origin_renderable_true", func(t *testing.T) {
		// `md encode "sh(multi(2,@0/48'/0'/0'/1'/<0;1>/*,@1/48'/0'/0'/1'/<0;1>/*))"`
		// → md1ypfdsss3cqpsvvzzs5wdf8hn5h5kle
		// bytecode: 2052d84211c00306304280 (81 bits).
		// sh(multi) is renderable ONLY with explicit origins (sh(multi) is not
		// in the canonical-origin table). Shared origin m/48'/0'/0'/1'.
		const s = "md1ypfdsss3cqpsvvzzs5wdf8hn5h5kle"
		tpl, err := Decode(s)
		if err != nil {
			t.Fatalf("Decode: %v", err)
		}
		if !tpl.Renderable {
			t.Fatalf("explicit-origin sh(multi) must be Renderable=true; got %+v", tpl)
		}
		if tpl.Policy != PolicyMulti || tpl.K != 2 || tpl.M != 2 {
			t.Fatalf("policy/k/m = %v/%d/%d, want PolicyMulti/2/2", tpl.Policy, tpl.K, tpl.M)
		}
		if tpl.Root != ScriptSh || tpl.N != 2 {
			t.Fatalf("root/N = %v/%d, want ScriptSh/2", tpl.Root, tpl.N)
		}
		for i, k := range tpl.Keys {
			if k.OriginPath != "m/48h/0h/0h/1h" {
				t.Fatalf("key %d origin = %q, want m/48h/0h/0h/1h", i, k.OriginPath)
			}
		}
	})

	t.Run("sh_multi_elided_origin_missing_explicit", func(t *testing.T) {
		// `md encode "sh(multi(2,@0/<0;1>/*,@1/<0;1>/*))"` (no origin path)
		// → md1yppqqxp3sg2gqhhzr55an9eu
		// `md decode` of this string errors: "non-canonical wrapper requires
		// explicit origin for @0" — i.e. MissingExplicitOrigin. sh(multi) is
		// not canonical, so elided origins are a decode REJECT (never a Template).
		const s = "md1yppqqxp3sg2gqhhzr55an9eu"
		tpl, err := Decode(s)
		if !errors.Is(err, errMissingExplicitOrigin) {
			t.Fatalf("elided-origin sh(multi): want errMissingExplicitOrigin, got %v (tpl=%+v)", err, tpl)
		}
		if tpl.N != 0 || tpl.Renderable || tpl.Policy != PolicySingle || len(tpl.Keys) != 0 {
			t.Fatalf("expected zero Template on reject, got %+v", tpl)
		}
	})

	t.Run("tr_nums_sortedmulti_a_renderable_false", func(t *testing.T) {
		// Sourced from a one-off md-codec encode_md1_string harness (the `md`
		// CLI's rust-miniscript string parser rejects sortedmulti_a with
		// synthetic xpubs). Descriptor:
		//   tr(NUMS, sortedmulti_a(2,@0,@1,@2)) with is_nums=true, shared origin
		//   m/48'/0'/0'/2', standard <0;1>/* use-site, n=3.
		//   encode_md1_string → md1yzfdsssj5qqcrjggscp5uv69zawle9l
		//   encode_payload bytes: 2092d84212a00181c90886 (88 bits)
		// `md decode` confirms it round-trips to
		//   tr(50929b74…ac0, sortedmulti_a(2,@0/<0;1>/*,@1/<0;1>/*,@2/<0;1>/*)).
		// Exercises the is_nums variable-width cursor (spec §2.13): is_nums
		// suppresses the kiw-bit key-index field. Taproot script-path is refused
		// (FOLD A / §4.2) → Renderable=false.
		const s = "md1yzfdsssj5qqcrjggscp5uv69zawle9l"
		tpl, err := Decode(s)
		if err != nil {
			t.Fatalf("Decode: %v", err)
		}
		if tpl.Renderable {
			t.Fatalf("tr(NUMS, sortedmulti_a) must be Renderable=false (taproot script-path refused); got %+v", tpl)
		}
		if tpl.Policy != PolicyComplex {
			t.Fatalf("policy = %v, want PolicyComplex (no tapscript-multisig policy claim)", tpl.Policy)
		}
		if tpl.Root != ScriptTr || tpl.N != 3 {
			t.Fatalf("root/N = %v/%d, want ScriptTr/3", tpl.Root, tpl.N)
		}
		if tpl.K != 0 || tpl.M != 0 {
			t.Fatalf("must make no k/m claim; got k=%d m=%d", tpl.K, tpl.M)
		}
		// The is_nums cursor decoded all 3 placeholders with shared origin.
		if len(tpl.Keys) != 3 {
			t.Fatalf("keys = %d, want 3 (is_nums cursor)", len(tpl.Keys))
		}
		for i, k := range tpl.Keys {
			if k.OriginPath != "m/48h/0h/0h/2h" {
				t.Fatalf("key %d origin = %q, want m/48h/0h/0h/2h", i, k.OriginPath)
			}
		}
	})
}

// crossCheckHex re-decodes a recorded bytecode payload to confirm the embedded
// md1 string's provenance (the byte payload + bit length the md-codec encoder
// reported decode to the same structure). Belt-and-suspenders for the FOLD C
// round-trip strings.
func TestDecodeRenderableBytecodeProvenance(t *testing.T) {
	cases := []struct {
		name       string
		hexPayload string
		bitLen     int
		renderable bool
		policy     PolicyKind
	}{
		{"wsh_and_v", "2012d84212a001824cd29c00000090", 120, false, PolicyComplex},
		{"sh_multi", "2052d84211c00306304280", 81, true, PolicyMulti},
		{"tr_nums_sortedmulti_a", "2092d84212a00181c90886", 88, false, PolicyComplex},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b, err := hex.DecodeString(c.hexPayload)
			if err != nil {
				t.Fatalf("bad hex: %v", err)
			}
			d, err := decodePayloadValidated(b, c.bitLen)
			if err != nil {
				t.Fatalf("decodePayloadValidated: %v", err)
			}
			tpl := summarize(d)
			if tpl.Renderable != c.renderable || tpl.Policy != c.policy {
				t.Fatalf("render/policy = %v/%v, want %v/%v", tpl.Renderable, tpl.Policy, c.renderable, c.policy)
			}
		})
	}
}
