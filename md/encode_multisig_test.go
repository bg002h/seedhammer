package md

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"seedhammer.com/codex32"
)

// ─── T6c Phase A: EncodeMultisig — wallet-policy sortedmulti md1 ─────────────

// TestEncodeMultisigRequestPlumbing constructs a request and asserts the fields
// are wired through (compile-time + value checks on the public surface).
func TestEncodeMultisigRequestPlumbing(t *testing.T) {
	req := EncodeMultisigRequest{
		Cosigners: []MultisigCosigner{
			{Fingerprint: [4]byte{1, 2, 3, 4}, FpPresent: true},
			{Fingerprint: [4]byte{5, 6, 7, 8}, FpPresent: false},
		},
		K:            2,
		Script:       MultisigWsh,
		OriginMode:   OriginShared,
		SharedOrigin: []PathComponent{{Hardened: true, Value: 48}},
	}
	if len(req.Cosigners) != 2 || req.K != 2 {
		t.Fatalf("request fields not plumbed: %+v", req)
	}
	if req.Script != MultisigWsh || req.OriginMode != OriginShared {
		t.Fatalf("enum fields not plumbed: %+v", req)
	}
	// SlotInfo is the ordering-verification handle element.
	s := SlotInfo{Index: 1, Fingerprint: [4]byte{5, 6, 7, 8}, FpPresent: false}
	if s.Index != 1 || s.FpPresent {
		t.Fatalf("SlotInfo not plumbed: %+v", s)
	}
	// Enum identity: the three script wrappers + two origin modes are distinct.
	if MultisigWsh == MultisigShWsh || MultisigShWsh == MultisigSh {
		t.Fatal("MultisigScript values not distinct")
	}
	if OriginShared == OriginDivergent {
		t.Fatal("OriginMode values not distinct")
	}
}

// mkXpub65 builds a 65-byte chainCode‖compressedPubkey from two hex strings.
func mkXpub65(t *testing.T, ccHex, pkHex string) (cc [32]byte, pk [33]byte) {
	t.Helper()
	ccb, err := hex.DecodeString(ccHex)
	if err != nil || len(ccb) != 32 {
		t.Fatalf("bad chaincode %q", ccHex)
	}
	pkb, err := hex.DecodeString(pkHex)
	if err != nil || len(pkb) != 33 {
		t.Fatalf("bad pubkey %q", pkHex)
	}
	copy(cc[:], ccb)
	copy(pk[:], pkb)
	return
}

// sharedOrigin4828 is m/48'/0'/0'/2' as RAW PathComponents (the T6b origin).
func sharedOrigin4828() []PathComponent {
	return []PathComponent{
		{Hardened: true, Value: 48}, {Hardened: true, Value: 0},
		{Hardened: true, Value: 0}, {Hardened: true, Value: 2},
	}
}

// TestEncodeMultisigSmoke: a 2-of-3 wsh(sortedmulti) over three distinct keys
// encodes to >=2 chunks, the returned stub == WalletPolicyIDStubChunks(out), and
// the slots reflect cosigner order with the right fp-presence.
func TestEncodeMultisigSmoke(t *testing.T) {
	cc, pk := mkXpub65(t, "101112131415161718191a1b1c1d1e1f202122232425262728292a2b2c2d2e2f", "03a9394a2f1a4f99613a716956c8540f6dba6f18931c2639107221b267d740af23")
	cc2, pk2 := mkXpub65(t, "101112131415161718191a1b1c1d1e1f202122232425262728292a2b2c2d2e2f", "02c6047f9441ed7d6d3045406e95c07cd85c778e4b8cef3ca7abac09b95c709ee5")
	req := EncodeMultisigRequest{
		Cosigners: []MultisigCosigner{
			{ChainCode: cc, CompressedPubkey: pk},
			{ChainCode: cc2, CompressedPubkey: pk2, Fingerprint: [4]byte{0xde, 0xad, 0xbe, 0xef}, FpPresent: true},
			{ChainCode: cc, CompressedPubkey: pk2},
		},
		K:            2,
		Script:       MultisigWsh,
		OriginMode:   OriginShared,
		SharedOrigin: sharedOrigin4828(),
	}
	out, stub, slots, err := EncodeMultisig(req)
	if err != nil {
		t.Fatalf("EncodeMultisig: %v", err)
	}
	if len(out) < 2 {
		t.Fatalf("want >=2 chunks, got %d", len(out))
	}
	for _, s := range out {
		if !codex32.ValidMD(s) {
			t.Fatalf("chunk not ValidMD: %s", s)
		}
	}
	wantStub, err := WalletPolicyIDStubChunks(out)
	if err != nil {
		t.Fatalf("WalletPolicyIDStubChunks: %v", err)
	}
	if stub != wantStub {
		t.Fatalf("returned stub %x != WalletPolicyIDStubChunks(out) %x", stub, wantStub)
	}
	if len(slots) != 3 {
		t.Fatalf("want 3 slots, got %d", len(slots))
	}
	for i, s := range slots {
		if int(s.Index) != i {
			t.Fatalf("slot %d Index = %d, want %d (order-preserving)", i, s.Index, i)
		}
	}
	if !slots[1].FpPresent || slots[1].Fingerprint != [4]byte{0xde, 0xad, 0xbe, 0xef} {
		t.Fatalf("slot 1 fp not plumbed: %+v", slots[1])
	}
	if slots[0].FpPresent {
		t.Fatalf("slot 0 should be fp-absent")
	}
}

// TestEncodeMultisigTemplateParity (A1): the bare multi-key AST for the wsh and
// sh(wsh) wrappers encodes to the SAME bit layout as the Rust-sourced vendored
// template goldens. wsh_sortedmulti.bytes.hex carries tagSortedMulti directly at
// n=3 (indices [0,1,2]); sh_wsh_multi.bytes.hex carries tagSh⊃tagWsh⊃tagMulti at
// n=2 (indices [0,1]) — tag-only wrappers, identical layout to sortedmulti per
// VF10/VF2 — so we assert the WRAPPER bytes match by building a tagMulti-bodied
// tree for the sh(wsh) parity leg. The k / index-list are parameterized PER
// VECTOR (each vendored .descriptor.json declares its own n): wsh_sortedmulti is
// n=3 (k=2, [0,1,2]), sh_wsh_multi is n=2 (k=2, [0,1]). Using a fixed n=3 index
// list for the n=2 vector would reference placeholder @2 and validatePlaceholderUsage
// would reject it ("placeholder index out of range").
func TestEncodeMultisigTemplateParity(t *testing.T) {
	mkTree := func(rootTag tag, innerWsh bool, multiTag tag, k uint8, indices []uint8) node {
		mk := node{tag: multiTag, body: multiKeysBody{k: k, indices: indices}}
		switch {
		case rootTag == tagWsh:
			return node{tag: tagWsh, body: childrenBody{children: []node{mk}}}
		case rootTag == tagSh && innerWsh:
			inner := node{tag: tagWsh, body: childrenBody{children: []node{mk}}}
			return node{tag: tagSh, body: childrenBody{children: []node{inner}}}
		default:
			return node{tag: tagSh, body: childrenBody{children: []node{mk}}}
		}
	}
	for _, tc := range []struct {
		vector  string
		tree    node
		wantHex string
	}{
		// wsh_sortedmulti is the sortedmulti template golden (n=3, k=2, [0,1,2]) —
		// exact match.
		{"wsh_sortedmulti", mkTree(tagWsh, false, tagSortedMulti, 2, []uint8{0, 1, 2}), "2082001821c22180"},
		// sh_wsh_multi is the only vendored sh(wsh) wrapper golden (n=2, k=2, [0,1]);
		// it carries tagMulti, so build the matching tagMulti tree to assert the
		// wrapper layout at the vector's own n.
		{"sh_wsh_multi", mkTree(tagSh, true, tagMulti, 2, []uint8{0, 1}), "2042001830860850"},
	} {
		t.Run(tc.vector, func(t *testing.T) {
			// The vendored template golden has NO origin/usesite/pubkeys TLV; build
			// the matching bare descriptor (shared empty origin, bare-star use-site).
			d := loadDescriptor(t, tc.vector)
			got, _, err := encodePayload(&descriptor{
				n:        d.n,
				pathDecl: d.pathDecl,
				useSite:  d.useSite,
				tree:     tc.tree,
				tlv:      d.tlv,
			})
			if err != nil {
				t.Fatalf("encodePayload: %v", err)
			}
			want := loadBytesHex(t, tc.vector)
			if hex.EncodeToString(got) != hex.EncodeToString(want) {
				t.Fatalf("template bytes mismatch:\n got  %x\n want %x", got, want)
			}
			// Belt-and-suspenders: pin the exact expected hex per vector so a
			// silently-rewritten .bytes.hex can't make the test vacuously pass.
			if hex.EncodeToString(got) != tc.wantHex {
				t.Fatalf("template bytes != pinned wantHex:\n got  %x\n want %s", got, tc.wantHex)
			}
		})
	}
}

type multisigMeta struct {
	Script       string `json:"script"`
	K            uint8  `json:"k"`
	N            int    `json:"n"`
	OriginMode   string `json:"origin_mode"`
	SharedOrigin string `json:"shared_origin"`
	FpPresent    bool   `json:"fp_present"`
	Cosigners    []struct {
		ChainCode        string `json:"chaincode"`
		CompressedPubkey string `json:"compressed_pubkey"`
		Fingerprint      string `json:"fingerprint"`
		FpPresent        bool   `json:"fp_present"`
		Origin           string `json:"origin"`
	} `json:"cosigners"`
	PayloadHex string `json:"payload_hex"`
	WPID       string `json:"wallet_policy_id"`
	Stub       string `json:"wallet_policy_id_stub"`
}

func loadMultisigMeta(t *testing.T, name string) multisigMeta {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "vectors", name+".meta.json"))
	if err != nil {
		t.Fatalf("read %s.meta.json: %v", name, err)
	}
	var m multisigMeta
	if err := jsonUnmarshalStrict(raw, &m); err != nil {
		t.Fatalf("unmarshal %s.meta.json: %v", name, err)
	}
	return m
}

func loadMultisigChunks(t *testing.T, name string) []string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "vectors", name+".md1.txt"))
	if err != nil {
		t.Fatalf("read %s.md1.txt: %v", name, err)
	}
	var chunks []string
	for _, l := range strings.Split(string(raw), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			chunks = append(chunks, l)
		}
	}
	return chunks
}

func multisigScriptFromName(t *testing.T, s string) MultisigScript {
	t.Helper()
	switch s {
	case "wsh_sortedmulti":
		return MultisigWsh
	case "sh_wsh_sortedmulti":
		return MultisigShWsh
	case "sh_sortedmulti":
		return MultisigSh
	default:
		t.Fatalf("unknown multisig script %q", s)
		return 0
	}
}

// reqFromMeta builds the EncodeMultisigRequest the meta.json describes.
func reqFromMeta(t *testing.T, m multisigMeta) EncodeMultisigRequest {
	t.Helper()
	req := EncodeMultisigRequest{K: m.K, Script: multisigScriptFromName(t, m.Script)}
	if m.OriginMode == "divergent" {
		req.OriginMode = OriginDivergent
	} else {
		req.OriginMode = OriginShared
		req.SharedOrigin = parsePathComponents(t, m.SharedOrigin)
	}
	for _, c := range m.Cosigners {
		cc, pk := mkXpub65(t, c.ChainCode, c.CompressedPubkey)
		mc := MultisigCosigner{ChainCode: cc, CompressedPubkey: pk, FpPresent: c.FpPresent}
		if c.FpPresent {
			fb, err := hex.DecodeString(c.Fingerprint)
			if err != nil || len(fb) != 4 {
				t.Fatalf("bad fp %q", c.Fingerprint)
			}
			copy(mc.Fingerprint[:], fb)
		}
		if req.OriginMode == OriginDivergent {
			mc.Origin = parsePathComponents(t, c.Origin)
		}
		req.Cosigners = append(req.Cosigners, mc)
	}
	return req
}

func jsonUnmarshalStrict(b []byte, v any) error {
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

var multisigFullSets = []string{"multisig_wsh_full", "multisig_sh_wsh_full", "multisig_sh_full", "multisig_wsh_fp", "multisig_wsh_divergent"}

// TestEncodeMultisigFullPolicyParity (A2): EncodeMultisig fed the meta inputs
// reproduces the vendored chunk strings byte-for-byte, the reassembled payload
// equals payload_hex, and the WalletPolicyId/stub match.
func TestEncodeMultisigFullPolicyParity(t *testing.T) {
	for _, name := range multisigFullSets {
		t.Run(name, func(t *testing.T) {
			m := loadMultisigMeta(t, name)
			req := reqFromMeta(t, m)
			out, stub, _, err := EncodeMultisig(req)
			if err != nil {
				t.Fatalf("EncodeMultisig: %v", err)
			}
			want := loadMultisigChunks(t, name)
			if len(out) != len(want) {
				t.Fatalf("chunk count: got %d want %d", len(out), len(want))
			}
			for i := range out {
				if out[i] != want[i] {
					t.Fatalf("chunk %d:\n got  %s\n want %s", i, out[i], want[i])
				}
			}
			d, err := Reassemble(out)
			if err != nil {
				t.Fatalf("Reassemble: %v", err)
			}
			gotPayload, _, err := encodePayload(d)
			if err != nil {
				t.Fatalf("encodePayload: %v", err)
			}
			if hex.EncodeToString(gotPayload) != m.PayloadHex {
				t.Fatalf("payload:\n got  %x\n want %s", gotPayload, m.PayloadHex)
			}
			id, _ := WalletPolicyIdChunks(out)
			if hex.EncodeToString(id[:]) != m.WPID {
				t.Fatalf("WalletPolicyId: got %x want %s", id, m.WPID)
			}
			if hex.EncodeToString(stub[:]) != m.Stub {
				t.Fatalf("stub: got %x want %s", stub, m.Stub)
			}
			tpl, _, err := ExpandWalletPolicyChunks(out)
			if err != nil {
				t.Fatalf("ExpandWalletPolicyChunks: %v", err)
			}
			switch req.Script {
			case MultisigWsh:
				if tpl.Root != ScriptWsh || tpl.InnerWsh {
					t.Fatalf("%s: Root=%v InnerWsh=%v, want Wsh/false", name, tpl.Root, tpl.InnerWsh)
				}
			case MultisigShWsh:
				if tpl.Root != ScriptSh || !tpl.InnerWsh {
					t.Fatalf("%s: Root=%v InnerWsh=%v, want Sh/true", name, tpl.Root, tpl.InnerWsh)
				}
			case MultisigSh:
				if tpl.Root != ScriptSh || tpl.InnerWsh {
					t.Fatalf("%s: Root=%v InnerWsh=%v, want Sh/false", name, tpl.Root, tpl.InnerWsh)
				}
			}
			if tpl.Policy != PolicySortedMulti {
				t.Fatalf("%s: Policy=%v, want SortedMulti", name, tpl.Policy)
			}
		})
	}
}

// t6bChunks is the vendored T6b multisig fixture (copied from
// gui/testdata/t6b_multisig_full.md1.txt — the md package cannot import gui
// testdata, so the 6 strings are inlined; they are guarded by A3 byte-equality
// AND by the gui-side TestSuppliedMultisigFixtureIsFullPolicy).
var t6bChunks = []string{
	"md1fvgfqzspqjtvyyy4qqxppcgsc27rczqg3yyc5z5tpwxqergd3c8g7ruszzg3ryssjfstllhxufdm4",
	"md1fvgfqzs2jvfeg9y4zktpd9chs82fefgh35nuevya8z62kep2q7md6duvfx8px8ygw3q3umhs2q3cu",
	"md1fvgfqzss8ygdjvlt5pterdm5rru59s2su80aw2q4wgdpapgfl4pkhsdyytkwl5zq9ner9ltnl8fnz",
	"md1fvgfqzsllphut2hvvpp5wl4l0mn058ndxfl63kufyfsjwlt2vkk2nlqmlvch5n4sk08xmsudrng93",
	"md1fvgfqz3qhwf72vyq3zgf3g9gkzuvpjxsmrsw3u8eqyy3zxfp9ycnjs2f29vkz6ts908m9qqcmg97l",
	"md1fvgfqz3f0qtrqglu5g8kh6mfsg4qxa9wq0nv9cauwfwxw70984wkqnw2uwz0w27h0f8nmf46cm8",
}

// TestEncodeMultisigT6bByteExact (A3): fed the three decoded T6b cosigners
// (fp-ABSENT, k=2, shared origin m/48'/0'/0'/2', wsh) in @0/@1/@2 order,
// EncodeMultisig reproduces the fixture chunk-for-chunk AND yields
// WalletPolicyId 7b716421db8b9f462967d04e0f8a3fd5. This proves a device could
// re-author the exact T6b card.
func TestEncodeMultisigT6bByteExact(t *testing.T) {
	cc0, pk0 := mkXpub65(t, "101112131415161718191a1b1c1d1e1f202122232425262728292a2b2c2d2e2f", "03a9394a2f1a4f99613a716956c8540f6dba6f18931c2639107221b267d740af23")
	cc1, pk1 := mkXpub65(t, "bba0c7ca160a870efeb940ab90d0f4284fea1b5e0d2117677e823fc37e2d5763", "021a3bf5fbf737d0f36993fd46dc4913093beb532d654fe0dfd98bd27585dc9f29")
	cc2, pk2 := mkXpub65(t, "101112131415161718191a1b1c1d1e1f202122232425262728292a2b2c2d2e2f", "02c6047f9441ed7d6d3045406e95c07cd85c778e4b8cef3ca7abac09b95c709ee5")
	req := EncodeMultisigRequest{
		Cosigners: []MultisigCosigner{
			{ChainCode: cc0, CompressedPubkey: pk0},
			{ChainCode: cc1, CompressedPubkey: pk1},
			{ChainCode: cc2, CompressedPubkey: pk2},
		},
		K: 2, Script: MultisigWsh, OriginMode: OriginShared, SharedOrigin: sharedOrigin4828(),
	}
	out, stub, slots, err := EncodeMultisig(req)
	if err != nil {
		t.Fatalf("EncodeMultisig: %v", err)
	}
	if len(out) != len(t6bChunks) {
		t.Fatalf("chunk count: got %d want %d", len(out), len(t6bChunks))
	}
	for i := range out {
		if out[i] != t6bChunks[i] {
			t.Fatalf("chunk %d:\n got  %s\n want %s", i, out[i], t6bChunks[i])
		}
	}
	id, _ := WalletPolicyIdChunks(out)
	if hex.EncodeToString(id[:]) != "7b716421db8b9f462967d04e0f8a3fd5" {
		t.Fatalf("WalletPolicyId = %x, want 7b716421db8b9f462967d04e0f8a3fd5", id)
	}
	if hex.EncodeToString(stub[:]) != "7b716421" {
		t.Fatalf("stub = %x, want 7b716421", stub)
	}
	for i, s := range slots {
		if s.FpPresent {
			t.Fatalf("slot %d fp-present, want absent (T6b is fp-absent)", i)
		}
	}
}

// TestEncodeMultisigRoundTrip (A4/I2/I6): EncodeMultisig output decodes back to
// the same template (root/policy/k/n/innerWsh) and per-@N {xpub, fp(+presence),
// origin, use-site} IN ORDER; and WalletPolicyIdChunks(out) == the returned stub
// prefix and equals WalletPolicyId of Reassemble(out) (identity zero-change).
func TestEncodeMultisigRoundTrip(t *testing.T) {
	for _, name := range multisigFullSets {
		t.Run(name, func(t *testing.T) {
			m := loadMultisigMeta(t, name)
			req := reqFromMeta(t, m)
			out, stub, slots, err := EncodeMultisig(req)
			if err != nil {
				t.Fatalf("EncodeMultisig: %v", err)
			}
			tpl, keys, err := ExpandWalletPolicyChunks(out)
			if err != nil {
				t.Fatalf("ExpandWalletPolicyChunks: %v", err)
			}
			if tpl.K != int(req.K) || tpl.N != len(req.Cosigners) {
				t.Fatalf("K/N = %d/%d, want %d/%d", tpl.K, tpl.N, req.K, len(req.Cosigners))
			}
			if len(keys) != len(req.Cosigners) {
				t.Fatalf("recovered %d keys, want %d", len(keys), len(req.Cosigners))
			}
			for i, k := range keys {
				if int(k.Index) != i {
					t.Fatalf("key %d Index = %d (order not preserved)", i, k.Index)
				}
				var wantXpub [65]byte
				copy(wantXpub[:32], req.Cosigners[i].ChainCode[:])
				copy(wantXpub[32:], req.Cosigners[i].CompressedPubkey[:])
				if k.Xpub != wantXpub {
					t.Fatalf("key %d xpub mismatch", i)
				}
				if k.FingerprintPresent != req.Cosigners[i].FpPresent {
					t.Fatalf("key %d fp-present = %v, want %v", i, k.FingerprintPresent, req.Cosigners[i].FpPresent)
				}
				if k.FingerprintPresent && k.Fingerprint != req.Cosigners[i].Fingerprint {
					t.Fatalf("key %d fp = %x, want %x", i, k.Fingerprint, req.Cosigners[i].Fingerprint)
				}
				if !k.UseSite.HasMultipath || len(k.UseSite.Multipath) != 2 {
					t.Fatalf("key %d use-site = %+v, want <0;1>", i, k.UseSite)
				}
			}
			// Identity zero-change: chunks-id == descriptor-id; stub is its prefix.
			idChunks, err := WalletPolicyIdChunks(out)
			if err != nil {
				t.Fatalf("WalletPolicyIdChunks: %v", err)
			}
			d, err := Reassemble(out)
			if err != nil {
				t.Fatalf("Reassemble: %v", err)
			}
			idDesc, err := WalletPolicyId(d)
			if err != nil {
				t.Fatalf("WalletPolicyId: %v", err)
			}
			if idChunks != idDesc {
				t.Fatalf("WalletPolicyIdChunks %x != WalletPolicyId(Reassemble) %x", idChunks, idDesc)
			}
			if [4]byte(idChunks[:4]) != stub {
				t.Fatalf("stub %x != id prefix %x", stub, idChunks[:4])
			}
			// slots reflect order + fp presence.
			for i, s := range slots {
				if int(s.Index) != i || s.FpPresent != req.Cosigners[i].FpPresent {
					t.Fatalf("slot %d = %+v inconsistent with cosigner", i, s)
				}
			}
			_ = tpl
		})
	}
}

// TestEncodeMultisigRefuse (A6/I5): invalid k/n, divergent-count/origin
// mismatch, and empty origins yield typed errors. The shipped split guards
// surface via errors.Is; the assembler's own guards are matched directly.
func TestEncodeMultisigRefuse(t *testing.T) {
	cc, pk := mkXpub65(t, "101112131415161718191a1b1c1d1e1f202122232425262728292a2b2c2d2e2f", "03a9394a2f1a4f99613a716956c8540f6dba6f18931c2639107221b267d740af23")
	cosigner := MultisigCosigner{ChainCode: cc, CompressedPubkey: pk}
	three := []MultisigCosigner{cosigner, cosigner, cosigner}

	mkReq := func(mut func(*EncodeMultisigRequest)) EncodeMultisigRequest {
		r := EncodeMultisigRequest{
			Cosigners: append([]MultisigCosigner(nil), three...),
			K:         2, Script: MultisigWsh, OriginMode: OriginShared, SharedOrigin: sharedOrigin4828(),
		}
		mut(&r)
		return r
	}

	for _, tc := range []struct {
		name    string
		req     EncodeMultisigRequest
		wantErr error // matched via errors.Is when non-nil; else just "must error"
	}{
		{"k>n", mkReq(func(r *EncodeMultisigRequest) { r.K = 4 }), errKGreaterThanN},
		{"k=0", mkReq(func(r *EncodeMultisigRequest) { r.K = 0 }), errThresholdRange},
		{"empty-shared-origin", mkReq(func(r *EncodeMultisigRequest) { r.SharedOrigin = nil }), errMultisigEmptySharedOrigin},
		{"divergent-empty-origin", mkReq(func(r *EncodeMultisigRequest) {
			r.OriginMode = OriginDivergent
			r.SharedOrigin = nil
			// all three cosigners have nil Origin → empty divergent
		}), errMultisigEmptyDivergent},
		{"zero-cosigners", mkReq(func(r *EncodeMultisigRequest) { r.Cosigners = nil; r.K = 1 }), errKeyCountRange},
		{"bad-script", mkReq(func(r *EncodeMultisigRequest) { r.Script = MultisigScript(99) }), errMultisigBadScript},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, _, err := EncodeMultisig(tc.req)
			if err == nil {
				t.Fatalf("%s: got nil error, want %v", tc.name, tc.wantErr)
			}
			if tc.wantErr != nil && !errors.Is(err, tc.wantErr) {
				t.Fatalf("%s: err = %v, want errors.Is %v", tc.name, err, tc.wantErr)
			}
		})
	}
}
