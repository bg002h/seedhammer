package md

import (
	"errors"
	"testing"

	"github.com/btcsuite/btcd/btcutil/v2/hdkeychain"
	"seedhammer.com/bip32"
)

const hard = hdkeychain.HardenedKeyStart

// ─── DecodeChunks ────────────────────────────────────────────────────────────

// TestDecodeChunksParity: DecodeChunks(split(d)) yields a Template equal to
// Decode of the equivalent single string, for both the force-chunked golden and
// the hand-built ≥4-chunk vector.
func TestDecodeChunksParity(t *testing.T) {
	for _, tc := range []struct {
		name string
		d    *descriptor
	}{
		{"wsh_multi_chunked", loadDescriptor(t, "wsh_multi_chunked")},
		{"chunked_md1_vector", chunkedMD1Vector()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			chunks, err := split(tc.d)
			if err != nil {
				t.Fatalf("split: %v", err)
			}
			got, err := DecodeChunks(chunks)
			if err != nil {
				t.Fatalf("DecodeChunks: %v", err)
			}
			want := summarize(tc.d)
			if got.N != want.N || got.Root != want.Root || got.Policy != want.Policy || got.K != want.K || got.M != want.M {
				t.Fatalf("DecodeChunks template = %+v, want %+v", got, want)
			}
		})
	}
}

// TestDecodeChunksTamperedSurfacesError: a tampered (wrong-but-consistent-csid)
// chunk set surfaces the Reassemble integrity error AND is matchable via
// errors.Is(err, ErrChunkSetIDMismatch) (R0-C1 — the EXPORTED sentinel).
func TestDecodeChunksTamperedSurfacesError(t *testing.T) {
	d := chunkedMD1Vector()
	id, _ := computeEncodingID(d)
	realCsid := deriveChunkSetID(id)
	wrong := splitWithCSID(t, d, (realCsid+1)&0xFFFFF)
	_, err := DecodeChunks(wrong)
	if !errors.Is(err, ErrChunkSetIDMismatch) {
		t.Fatalf("DecodeChunks(wrong csid) err = %v, want errors.Is ErrChunkSetIDMismatch", err)
	}
}

// TestDecodeChunksIncompleteExported: a dropped chunk surfaces the EXPORTED
// ErrChunkSetIncomplete (R0-C1).
func TestDecodeChunksIncompleteExported(t *testing.T) {
	chunks, err := split(chunkedMD1Vector())
	if err != nil {
		t.Fatalf("split: %v", err)
	}
	if len(chunks) < 2 {
		t.Skip("need >=2 chunks")
	}
	if _, err := DecodeChunks(chunks[1:]); !errors.Is(err, ErrChunkSetIncomplete) {
		t.Fatalf("DecodeChunks(dropped) err = %v, want errors.Is ErrChunkSetIncomplete", err)
	}
}

// ─── ExpandWalletPolicy ──────────────────────────────────────────────────────

// TestExpandWalletPolicyOriginCanonicalFallback: for a shared-empty-path
// wsh(sortedmulti) (the wsh_sortedmulti golden), the resolved OriginPath falls
// back to canonicalOrigin(tree) = m/48'/0'/0'/2' (R0-I1) — NOT empty — and the
// path uses in-band hardening (R0-I2: value + HardenedKeyStart, []uint32).
func TestExpandWalletPolicyOriginCanonicalFallback(t *testing.T) {
	d := loadDescriptor(t, "wsh_sortedmulti")
	keys, err := ExpandWalletPolicy(d)
	if err != nil {
		t.Fatalf("ExpandWalletPolicy: %v", err)
	}
	if len(keys) != 3 {
		t.Fatalf("got %d expanded keys, want 3", len(keys))
	}
	wantPath := bip32.Path{hard + 48, hard + 0, hard + 0, hard + 2}
	for i, k := range keys {
		if int(k.Index) != i {
			t.Fatalf("key %d Index = %d", i, k.Index)
		}
		if len(k.OriginPath) != len(wantPath) {
			t.Fatalf("key %d OriginPath len = %d (%v), want %d (%v)", i, len(k.OriginPath), k.OriginPath, len(wantPath), wantPath)
		}
		for j := range wantPath {
			if k.OriginPath[j] != wantPath[j] {
				t.Fatalf("key %d OriginPath[%d] = %#x, want %#x", i, j, k.OriginPath[j], wantPath[j])
			}
		}
		if k.XpubPresent {
			t.Fatalf("key %d XpubPresent=true, want false (golden has null pubkeys)", i)
		}
		if !k.UseSite.HasMultipath || len(k.UseSite.Multipath) != 2 {
			t.Fatalf("key %d use-site = %+v, want <0;1>", i, k.UseSite)
		}
	}
}

// TestExpandWalletPolicyOverrideAndDivergent: origin precedence override >
// divergent[idx] (R0-I1). The hand-built descriptor sets a divergent baseline
// per @N and an OriginPathOverrides entry on @1; @1 must reflect the override,
// @0 the divergent baseline.
func TestExpandWalletPolicyOverrideAndDivergent(t *testing.T) {
	d := &descriptor{
		n: 2,
		pathDecl: pathDecl{n: 2, divergent: []originPath{
			{components: []pathComponent{{hardened: true, value: 48}, {hardened: false, value: 7}}},
			{components: []pathComponent{{hardened: true, value: 48}, {hardened: false, value: 8}}},
		}},
		useSite: useSitePath{hasMultipath: true, multipath: []alternative{{value: 0}, {value: 1}}},
		tree: node{tag: tagWsh, body: childrenBody{children: []node{{
			tag: tagSortedMulti, body: multiKeysBody{k: 1, indices: []uint8{0, 1}},
		}}}},
		tlv: tlvSection{
			originPresent:   true,
			originOverrides: []idxOrigin{{idx: 1, path: originPath{components: []pathComponent{{hardened: true, value: 99}}}}},
		},
	}
	keys, err := ExpandWalletPolicy(d)
	if err != nil {
		t.Fatalf("ExpandWalletPolicy: %v", err)
	}
	want0 := bip32.Path{hard + 48, 7}
	want1 := bip32.Path{hard + 99} // override wins over divergent[1]
	if !pathEq(keys[0].OriginPath, want0) {
		t.Fatalf("@0 origin = %v, want %v", keys[0].OriginPath, want0)
	}
	if !pathEq(keys[1].OriginPath, want1) {
		t.Fatalf("@1 origin = %v (override should win), want %v", keys[1].OriginPath, want1)
	}
}

// TestExpandWalletPolicyXpubPresent: a descriptor carrying a Pubkeys TLV
// surfaces XpubPresent=true with the 65-byte split, and FingerprintPresent.
func TestExpandWalletPolicyXpubPresent(t *testing.T) {
	d := singlesigWithPubkey(xpubPayload(validPubkey0))
	d.tlv.fpPresent = true
	d.tlv.fingerprints = []idxFP{{idx: 0, fp: [4]byte{0xde, 0xad, 0xbe, 0xef}}}
	keys, err := ExpandWalletPolicy(d)
	if err != nil {
		t.Fatalf("ExpandWalletPolicy: %v", err)
	}
	if len(keys) != 1 {
		t.Fatalf("got %d keys, want 1", len(keys))
	}
	if !keys[0].XpubPresent {
		t.Fatal("XpubPresent=false, want true")
	}
	if !keys[0].FingerprintPresent || keys[0].Fingerprint != [4]byte{0xde, 0xad, 0xbe, 0xef} {
		t.Fatalf("fingerprint = %v present=%v", keys[0].Fingerprint, keys[0].FingerprintPresent)
	}
	// chain code = xpub[0:32], pubkey = xpub[32:65].
	if keys[0].Xpub[0] != 0x10 || keys[0].Xpub[32] != validPubkey0[0] {
		t.Fatalf("xpub byte-split wrong: [0]=%#x [32]=%#x", keys[0].Xpub[0], keys[0].Xpub[32])
	}
}

func pathEq(a, b bip32.Path) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// ─── nesting discriminant (R0-C2) ────────────────────────────────────────────

// TestInnerWshDiscriminant: a bare sh(sortedmulti) and a sh(wsh(sortedmulti))
// both summarize to ScriptSh + PolicySortedMulti, but the nesting discriminant
// Template.InnerWsh distinguishes them (C2 — required so Task 3 maps them to
// DIFFERENT scripts, P2SH vs P2SH_P2WSH, and never verifies a legacy P2SH
// against a nested-segwit address).
func TestInnerWshDiscriminant(t *testing.T) {
	bareSh := &descriptor{
		n:        2,
		pathDecl: pathDecl{n: 2, shared: &originPath{}},
		useSite:  useSitePath{hasMultipath: true, multipath: []alternative{{value: 0}, {value: 1}}},
		tree: node{tag: tagSh, body: childrenBody{children: []node{{
			tag: tagSortedMulti, body: multiKeysBody{k: 1, indices: []uint8{0, 1}},
		}}}},
	}
	nestedSh := &descriptor{
		n:        2,
		pathDecl: pathDecl{n: 2, shared: &originPath{}},
		useSite:  useSitePath{hasMultipath: true, multipath: []alternative{{value: 0}, {value: 1}}},
		tree: node{tag: tagSh, body: childrenBody{children: []node{{
			tag: tagWsh, body: childrenBody{children: []node{{
				tag: tagSortedMulti, body: multiKeysBody{k: 1, indices: []uint8{0, 1}},
			}}},
		}}}},
	}
	bareT := summarize(bareSh)
	nestedT := summarize(nestedSh)
	if bareT.Root != ScriptSh || bareT.Policy != PolicySortedMulti {
		t.Fatalf("bare sh: root/policy = %v/%v", bareT.Root, bareT.Policy)
	}
	if nestedT.Root != ScriptSh || nestedT.Policy != PolicySortedMulti {
		t.Fatalf("nested sh: root/policy = %v/%v", nestedT.Root, nestedT.Policy)
	}
	if bareT.InnerWsh {
		t.Fatal("bare sh(sortedmulti): InnerWsh=true, want false")
	}
	if !nestedT.InnerWsh {
		t.Fatal("sh(wsh(sortedmulti)): InnerWsh=false, want true")
	}
}
