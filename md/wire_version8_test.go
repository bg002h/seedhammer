package md

import (
	"bytes"
	"encoding/hex"
	"errors"
	"slices"
	"testing"

	"seedhammer.com/codex32"
)

// ─── SPEC §8.4 Go leg: the round trip through the DISPATCH ───────────────────
//
// Not through decodePayload alone: version 5 was unusable only because of the
// bit-0 dispatch (SPEC §3c), which decodePayload never sees. So every leg below
// enters where the device does -- ParseChunkHeader, Decode, Reassemble.

func TestVersion8SingleStringThroughTheDispatch(t *testing.T) {
	s := loadPhraseChunks(t, "liana_taproot")
	if len(s) != 1 {
		t.Fatalf("liana_taproot is %d strings, want the one single-string v8 card", len(s))
	}
	h, err := ParseChunkHeader(s[0])
	if err != nil || h.Chunked {
		t.Fatalf("ParseChunkHeader = %+v, %v; want a single (non-chunked) md1", h, err)
	}
	if _, err := Decode(s[0]); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	d, err := decodeSingleForTest(s)
	if err != nil {
		t.Fatal(err)
	}
	if b := d.tree.body.(trBody); b.ik != InternalKeyLianaUnspendable {
		t.Fatalf("decoded internal key %d, want InternalKeyLianaUnspendable", b.ik)
	}
	// Re-encode: the SAME string Rust minted (byte parity via the string).
	if got, err := encodeMD1String(d); err != nil || got != s[0] {
		t.Fatalf("encodeMD1String = %q, %v; want %q", got, err, s[0])
	}
	// Its kind-0 twin still decodes as NUMS at version 4, unchanged.
	twin, err := decodeSingleForTest(loadPhraseChunks(t, "nums_taproot"))
	if err != nil {
		t.Fatal(err)
	}
	if b := twin.tree.body.(trBody); b.ik != InternalKeyNUMS || twin.wireVersion() != wfRedesignVersion {
		t.Fatalf("nums_taproot: ik %d version %d, want NUMS at 4", b.ik, twin.wireVersion())
	}
}

func TestVersion8ChunkSetsThroughTheDispatch(t *testing.T) {
	for _, name := range []string{"keyed_tr_liana_kofn_recovery", "keyed_tr_liana_nested_two_recoveries"} {
		chunks := loadPhraseChunks(t, name)
		for i, c := range chunks {
			h, err := ParseChunkHeader(c)
			if err != nil || !h.Chunked || h.Version != wfUnspendableVersion {
				t.Fatalf("%s chunk %d: %+v, %v; want a chunked header at version 8", name, i, h, err)
			}
		}
		d, err := Reassemble(chunks)
		if err != nil {
			t.Fatalf("%s: Reassemble: %v", name, err)
		}
		if b := d.tree.body.(trBody); b.ik != InternalKeyLianaUnspendable {
			t.Fatalf("%s: internal key %d, want InternalKeyLianaUnspendable", name, b.ik)
		}
		// The chunk WRITER carries the derived version in every chunk header
		// (chunk.rs:280), so re-splitting reproduces Rust's chunk set exactly.
		again, err := split(d)
		if err != nil {
			t.Fatalf("%s: split: %v", name, err)
		}
		if !slices.Equal(again, chunks) {
			t.Fatalf("%s: split(Reassemble(x)) != x\n  go:   %v\n  rust: %v", name, again, chunks)
		}
	}
}

// withVersion rewrites an md1 string's 4-bit wire version and re-checksums it,
// producing a BCH-valid card at a version no encoder emits.
func withVersion(t *testing.T, s string, v uint8) string {
	t.Helper()
	syms, err := codex32.MDDataSymbols(s)
	if err != nil {
		t.Fatal(err)
	}
	syms = append([]byte(nil), syms...)
	if syms[0]&1 == 1 { // chunked: symbol 0 is [v3 v2 v1 v0 chunked=1]
		syms[0] = v<<1 | 1
	} else { // single: symbol 0 is [divergent v3 v2 v1 v0]
		syms[0] = syms[0]&0b10000 | v&0b1111
	}
	return codex32.AssembleMD1(syms)
}

// An UNSUPPORTED version is refused with an error that NAMES it (SPEC §6a),
// single-string and chunked. 12 is the one remaining even version (§3a).
func TestAnUnsupportedWireVersionIsRefusedByName(t *testing.T) {
	single := withVersion(t, loadPhraseChunks(t, "liana_taproot")[0], 12)
	chunk := withVersion(t, loadPhraseChunks(t, "keyed_tr_liana_kofn_recovery")[0], 12)

	_, errDecode := Decode(single)
	_, errHeader := ParseChunkHeader(chunk)
	_, errReassemble := Reassemble([]string{chunk})
	for what, err := range map[string]error{"Decode(single)": errDecode,
		"ParseChunkHeader(chunk)": errHeader, "Reassemble(chunk)": errReassemble} {
		var wv *WireVersionError
		if !errors.As(err, &wv) || wv.Got != 12 || !errors.Is(err, ErrUnsupportedWireVersion) {
			t.Errorf("%s = %v; want a *WireVersionError{Got: 12} matching ErrUnsupportedWireVersion", what, err)
		}
	}
	// Both accepted versions still parse: the refusal is the set, not "!= 4".
	for _, v := range []uint8{wfRedesignVersion, wfUnspendableVersion} {
		if _, err := ParseChunkHeader(withVersion(t, loadPhraseChunks(t, "keyed_tr_liana_kofn_recovery")[0], v)); err != nil {
			t.Errorf("version %d refused: %v", v, err)
		}
	}
}

// ─── SPEC §8.5 Go leg: identity DISTINCTNESS between the kind-0/kind-1 twins ─
//
// keyed_compose_preset_kofn_recovery and keyed_tr_liana_kofn_recovery carry the
// SAME template body, keys and fingerprints; only the internal key's kind
// differs. Equality of each with Rust is TestKeyedConformanceAgreesWithRust;
// this asserts the twins never collide -- in the full ids AND in both 4-byte
// mk1 stub flavours, which distinct ids do not imply (SPEC §3e).
func TestKind0AndKind1TwinsNeverShareAnIdentity(t *testing.T) {
	k0, err := Reassemble(vectorChunksFor(t, "keyed_compose_preset_kofn_recovery"))
	if err != nil {
		t.Fatal(err)
	}
	k1, err := Reassemble(vectorChunksFor(t, "keyed_tr_liana_kofn_recovery"))
	if err != nil {
		t.Fatal(err)
	}
	type idFn struct {
		name string
		f    func(*descriptor) ([]byte, error)
	}
	for _, fn := range []idFn{
		{"WalletPolicyId", func(d *descriptor) ([]byte, error) { x, e := WalletPolicyId(d); return x[:], e }},
		{"WalletDescriptorTemplateId", func(d *descriptor) ([]byte, error) { x, e := WalletDescriptorTemplateId(d); return x[:], e }},
		{"WalletPolicyIDStub", func(d *descriptor) ([]byte, error) { x, e := WalletPolicyIDStub(d); return x[:], e }},
		{"WalletDescriptorTemplateIdStub", func(d *descriptor) ([]byte, error) { x, e := WalletDescriptorTemplateIdStub(d); return x[:], e }},
		{"FormAwareStub", func(d *descriptor) ([]byte, error) { x, e := FormAwareStub(d); return x[:], e }},
		{"md1 encoding id", func(d *descriptor) ([]byte, error) { x, e := computeEncodingID(d); return x[:], e }},
	} {
		a, errA := fn.f(k0)
		b, errB := fn.f(k1)
		if errA != nil || errB != nil {
			t.Fatalf("%s: %v / %v", fn.name, errA, errB)
		}
		if hex.EncodeToString(a) == hex.EncodeToString(b) {
			t.Errorf("%s: kind 0 and kind 1 share %x -- two wallets with different addresses, one identity", fn.name, a)
		}
	}
}

// canonicalize has TWO trBody copy sites (remapIndices and cloneNode), and a
// canonical card only ever reaches cloneNode -- remapIndices is skipped on the
// identity permutation. So this feeds it a NON-canonical kind-1 tree: the same
// multi_a with its placeholders written out of first-occurrence order. A copy
// site that rebuilt the kind from the is_nums bit would re-encode it as NUMS at
// version 4; the kind must survive, so the bytes equal the canonical card's.
func TestCanonicalizeKeepsTheKindOnANonCanonicalTree(t *testing.T) {
	d, err := decodeSingleForTest(loadPhraseChunks(t, "liana_taproot"))
	if err != nil {
		t.Fatal(err)
	}
	want, _, err := encodePayload(d)
	if err != nil {
		t.Fatal(err)
	}
	tb := d.tree.body.(trBody)
	leaf := tb.tree.body.(multiKeysBody)
	if !slices.Equal(leaf.indices, []uint8{0, 1, 2}) {
		t.Fatalf("liana_taproot's leaf is %v, want the canonical [0 1 2] this test permutes", leaf.indices)
	}
	permuted := node{tag: tb.tree.tag, body: multiKeysBody{k: leaf.k, indices: []uint8{1, 2, 0}}}
	nc := *d
	nc.tree = node{tag: tagTr, body: trBody{ik: tb.ik, tree: &permuted}}
	got, _, err := encodePayload(&nc)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, want) {
		t.Fatalf("non-canonical kind-1 tree encodes to %x, want the canonical %x", got, want)
	}
}

// ─── SPEC §3d: the kind bit, pinned on the WIRE ──────────────────────────────

// A symmetric polarity inversion (write AND read both flipped) round-trips
// perfectly, so no encode-then-decode test can see it (SPEC §8.10). These are
// golden bytes, identical to Rust's
// wire_version_8.rs::the_kind_bit_polarity_is_pinned_on_the_wire_not_just_round_tripped:
// Tag::Tr (000001) | is_nums 1 | kind | has_tree 0, MSB-first, zero-padded.
func TestKindBitPolarityIsPinnedOnTheWire(t *testing.T) {
	for _, c := range []struct {
		name     string
		ik       InternalKeyKind
		keyIndex uint8
		kiw      uint8
		version  uint8
		want     []byte
		bits     int
	}{
		{"liana at v8", InternalKeyLianaUnspendable, 0, 0, wfUnspendableVersion, []byte{0x07, 0x00}, 9},
		{"nums at v8", InternalKeyNUMS, 0, 0, wfUnspendableVersion, []byte{0x06, 0x00}, 9},
		{"nums at v4 (no kind bit)", InternalKeyNUMS, 0, 0, wfRedesignVersion, []byte{0x06}, 8},
		// R0 m1: a Slot at v8 carries NO kind bit. Tag::Tr | is_nums 0 |
		// key_index 01 (kiw 2) | has_tree 0 = 10 bits = [0x04, 0x80], the same
		// golden as Rust's polarity test.
		{"slot 1 at v8 (no kind bit)", InternalKeySlot, 1, 2, wfUnspendableVersion, []byte{0x04, 0x80}, 10},
	} {
		n := node{tag: tagTr, body: trBody{ik: c.ik, keyIndex: c.keyIndex}}
		var w bitWriter
		if err := writeNode(&w, n, c.kiw, c.version); err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if w.bitLen() != c.bits || !bytes.Equal(w.intoBytes(), c.want) {
			t.Errorf("%s: %d bits %x, want %d bits %x", c.name, w.bitLen(), w.intoBytes(), c.bits, c.want)
		}
		// R0 m1: pin the READ side too. A reader that always yields Liana at v8,
		// or that reads a kind bit for a Slot key, round-trips every minimal
		// vector and passes every other test; only reading GOLDEN bytes back
		// separates it (Rust twin: the same rows in wire_version_8.rs).
		got, err := readNode(newBitReader(c.want, len(c.want)*8), c.kiw, c.version)
		if err != nil {
			t.Errorf("%s: readNode(golden %x): %v", c.name, c.want, err)
			continue
		}
		gb, ok := got.body.(trBody)
		if got.tag != tagTr || !ok || gb.ik != c.ik || gb.keyIndex != c.keyIndex || gb.tree != nil {
			t.Errorf("%s: golden %x read back as %+v, want ik %d keyIndex %d", c.name, c.want, got, c.ik, c.keyIndex)
		}
	}
}

func TestLianaHasNoRepresentationBelowVersion8(t *testing.T) {
	var w bitWriter
	err := writeNode(&w, node{tag: tagTr, body: trBody{ik: InternalKeyLianaUnspendable}}, 0, wfRedesignVersion)
	if err != errLianaNeedsVersion8 {
		t.Fatalf("writeNode(Liana, v4) = %v, want errLianaNeedsVersion8", err)
	}
}

// ─── SPEC §3d/§3e: the version is DERIVED from the tree ──────────────────────

func TestWireVersionIsDerivedFromTheTree(t *testing.T) {
	v4, v8 := 0, 0
	for _, name := range composeVectorNames {
		chunks := vectorChunksFor(t, name)
		d, err := Reassemble(chunks)
		if err != nil {
			d, err = decodeSingleForTest(chunks)
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
		}
		b, isTr := d.tree.body.(trBody)
		if d.tree.tag != tagTr || !isTr {
			continue
		}
		want := uint8(wfRedesignVersion)
		if b.ik == InternalKeyLianaUnspendable {
			want = wfUnspendableVersion
			v8++
		} else {
			v4++
		}
		if got := d.wireVersion(); got != want {
			t.Errorf("%s: wireVersion %d, want %d", name, got, want)
		}
	}
	if v4 == 0 || v8 == 0 {
		t.Fatalf("root-tr populations: %d at v4, %d at v8 -- one half proves nothing", v4, v8)
	}
}

// decodeSingleForTest decodes a single-string md1 to the internal descriptor
// (Decode returns only the summary Template).
func decodeSingleForTest(chunks []string) (*descriptor, error) {
	if len(chunks) != 1 {
		return nil, errChunkSetEmpty
	}
	b, bits, err := unwrapString(chunks[0])
	if err != nil {
		return nil, err
	}
	return decodePayloadValidated(b, bits)
}
