package md

import (
	"encoding/hex"
	"testing"
)

// ─── T6a-1 Task 2W: WalletPolicyId (compute_wallet_policy_id) port ───────────

// buildSingleSigDescriptor builds the in-package wallet-policy *descriptor for a
// golden set (the same AST EncodeSingleSig builds), so the id tests need no
// re-decode.
func buildSingleSigDescriptor(t *testing.T, m singlesigMeta) *descriptor {
	t.Helper()
	cc, pk, fp, origin, script := metaInputs(t, m)
	comps := make([]pathComponent, len(origin))
	for i, c := range origin {
		comps[i] = pathComponent{hardened: c.Hardened, value: c.Value}
	}
	sharedOrigin := originPath{components: comps}
	var xpub [65]byte
	copy(xpub[:32], cc[:])
	copy(xpub[32:], pk[:])
	tree, err := singleSigTree(script)
	if err != nil {
		t.Fatal(err)
	}
	return &descriptor{
		n:        1,
		pathDecl: pathDecl{n: 1, shared: &sharedOrigin},
		useSite: useSitePath{
			hasMultipath: true,
			multipath:    []alternative{{value: 0}, {value: 1}},
		},
		tree: tree,
		tlv: tlvSection{
			pubPresent:   true,
			pubkeys:      []idxPub{{idx: 0, xpub: xpub}},
			fpPresent:    true,
			fingerprints: []idxFP{{idx: 0, fp: fp}},
		},
	}
}

// cell7WpkhDescriptor reproduces the in-source golden cell_7_wpkh_descriptor
// (identity.rs:385-419): wpkh @0, origin m/84'/0'/0', use-site <0;1>/*, fp
// deadbeef, xpub = [0x11;32] ‖ 0x02 ‖ [0x22;32]. Its id is the pinned golden
// 6650b980 3b3c6621 0140540d a8d765a0 (identity.rs:547-550).
func cell7WpkhDescriptor() *descriptor {
	var xpub [65]byte
	for i := 0; i < 32; i++ {
		xpub[i] = 0x11
	}
	xpub[32] = 0x02
	for i := 33; i < 65; i++ {
		xpub[i] = 0x22
	}
	origin := originPath{components: []pathComponent{
		{hardened: true, value: 84},
		{hardened: true, value: 0},
		{hardened: true, value: 0},
	}}
	return &descriptor{
		n:        1,
		pathDecl: pathDecl{n: 1, shared: &origin},
		useSite: useSitePath{
			hasMultipath: true,
			multipath:    []alternative{{value: 0}, {value: 1}},
		},
		tree: node{tag: tagWpkh, body: keyArgBody{index: 0}},
		tlv: tlvSection{
			pubPresent:   true,
			pubkeys:      []idxPub{{idx: 0, xpub: xpub}},
			fpPresent:    true,
			fingerprints: []idxFP{{idx: 0, fp: [4]byte{0xDE, 0xAD, 0xBE, 0xEF}}},
		},
	}
}

// TestWalletPolicyIdGolden pins the ready-made Rust golden (R0-M3).
func TestWalletPolicyIdGolden(t *testing.T) {
	const want = "6650b9803b3c66210140540da8d765a0"
	id, err := WalletPolicyId(cell7WpkhDescriptor())
	if err != nil {
		t.Fatal(err)
	}
	if hex.EncodeToString(id[:]) != want {
		t.Errorf("WalletPolicyId(cell_7_wpkh)=%x, want %s", id, want)
	}
}

// TestWalletPolicyIdToolkitDifferential pins the FULL 16-byte id + the [0:4]
// stub for all 4 vendored goldens vs the toolkit (R0-M4).
func TestWalletPolicyIdToolkitDifferential(t *testing.T) {
	for _, set := range singlesigSets {
		t.Run(set, func(t *testing.T) {
			m := loadSinglesigMeta(t, set)
			d := buildSingleSigDescriptor(t, m)
			id, err := WalletPolicyId(d)
			if err != nil {
				t.Fatal(err)
			}
			if hex.EncodeToString(id[:]) != m.WPID {
				t.Errorf("WalletPolicyId=%x, want %s", id, m.WPID)
			}
			stub, err := WalletPolicyIDStub(d)
			if err != nil {
				t.Fatal(err)
			}
			if hex.EncodeToString(stub[:]) != m.Stub {
				t.Errorf("stub=%x, want %s", stub, m.Stub)
			}
			if hex.EncodeToString(stub[:]) != m.WPID[:8] {
				t.Errorf("stub %x != WPID[0:4] %s", stub, m.WPID[:8])
			}
		})
	}
}

// TestWalletPolicyIdPresenceSignificant: nulling pubkeys + fp yields a DIFFERENT
// id (identity.rs:610-617).
func TestWalletPolicyIdPresenceSignificant(t *testing.T) {
	full := cell7WpkhDescriptor()
	idFull, err := WalletPolicyId(full)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := cell7WpkhDescriptor()
	tmpl.tlv.pubPresent = false
	tmpl.tlv.pubkeys = nil
	tmpl.tlv.fpPresent = false
	tmpl.tlv.fingerprints = nil
	idTmpl, err := WalletPolicyId(tmpl)
	if err != nil {
		t.Fatal(err)
	}
	if idFull == idTmpl {
		t.Error("template-only id == full id, want different (presence-significant)")
	}
}

// TestWalletPolicyIdEncodingStable: an origin-elided form (origin via TLV
// override) and the explicit path_decl form of the SAME wallet yield the SAME
// id (identity.rs:572-588) — proves record_bytes is built from the RESOLVED
// path (override > path_decl-as-is), exposing any display-accessor/
// canonicalOrigin divergence (R0-I2).
func TestWalletPolicyIdEncodingStable(t *testing.T) {
	baseline := cell7WpkhDescriptor() // origin via path_decl.Shared = m/84'/0'/0'

	// Elided form: empty path_decl shared, identical origin supplied via the
	// OriginPathOverrides TLV.
	elided := cell7WpkhDescriptor()
	empty := originPath{}
	elided.pathDecl = pathDecl{n: 1, shared: &empty}
	elided.tlv.originPresent = true
	elided.tlv.originOverrides = []idxOrigin{{idx: 0, path: originPath{components: []pathComponent{
		{hardened: true, value: 84},
		{hardened: true, value: 0},
		{hardened: true, value: 0},
	}}}}

	idBase, err := WalletPolicyId(baseline)
	if err != nil {
		t.Fatal(err)
	}
	idElided, err := WalletPolicyId(elided)
	if err != nil {
		t.Fatal(err)
	}
	if idBase != idElided {
		t.Errorf("origin-elided id %x != explicit-origin id %x (want equal)", idElided, idBase)
	}
}

// TestWalletPolicyIdNotEncodingID: WalletPolicyId differs from computeEncodingID
// (the chunk-set-id source — a DISTINCT preimage).
func TestWalletPolicyIdNotEncodingID(t *testing.T) {
	for _, set := range singlesigSets {
		t.Run(set, func(t *testing.T) {
			m := loadSinglesigMeta(t, set)
			d := buildSingleSigDescriptor(t, m)
			wpid, err := WalletPolicyId(d)
			if err != nil {
				t.Fatal(err)
			}
			encid, err := computeEncodingID(d)
			if err != nil {
				t.Fatal(err)
			}
			if wpid == encid {
				t.Error("WalletPolicyId == computeEncodingID, want distinct")
			}
		})
	}
}
