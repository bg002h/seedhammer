package md

import (
	"encoding/hex"
	"testing"
)

// keylessWshSortedmulti2of3 builds the keyless template descriptor for
// wsh(sortedmulti(2,@0/<0;1>/*,@1/<0;1>/*,@2/<0;1>/*)) — the golden oracle
// (md1yzpqqxppcgsc9kdmw6d5dp08f) whose WalletDescriptorTemplateId is the pinned
// b02b44037119e6b6fd1d82f61aa17e21 (md inspect, descriptor-mnemonic@54dd765).
// n=3, shared "m" origin, use-site <0;1>/*, no pubkeys/fp/overrides.
func keylessWshSortedmulti2of3() *descriptor {
	empty := originPath{}
	return &descriptor{
		n:        3,
		pathDecl: pathDecl{n: 3, shared: &empty},
		useSite: useSitePath{
			hasMultipath: true,
			multipath:    []alternative{{value: 0}, {value: 1}},
		},
		tree: node{tag: tagWsh, body: childrenBody{children: []node{{
			tag: tagSortedMulti, body: multiKeysBody{k: 2, indices: []uint8{0, 1, 2}},
		}}}},
	}
}

// ─── Task 1: isWalletPolicy predicate (Some-AND-non-empty, I1) ───────────────
//
// Mirrors Rust md-codec encode.rs is_wallet_policy: pubkeys present AND
// non-empty. A keyless template (pubkeys:null) is NOT a wallet policy; a keyed
// policy IS; a desynced descriptor that left pubPresent set with an empty
// pubkeys slice must NOT slip through as a wallet-policy (the I1 bug class).
func TestIsWalletPolicy(t *testing.T) {
	full := cell7WpkhDescriptor() // keyed: pubPresent + 1 xpub

	tmpl := cell7WpkhDescriptor()
	tmpl.tlv.pubPresent = false
	tmpl.tlv.pubkeys = nil

	if !isWalletPolicy(full) {
		t.Fatal("keyed descriptor must be a wallet-policy")
	}
	if isWalletPolicy(tmpl) {
		t.Fatal("keyless template must NOT be a wallet-policy")
	}

	// I1: pubPresent stays true but the pubkeys slice is empty (a strip that
	// nulled the slice but forgot to clear the flag) → must be false.
	desync := cell7WpkhDescriptor()
	desync.tlv.pubkeys = nil // pubPresent still true, pubkeys empty
	if isWalletPolicy(desync) {
		t.Fatal("empty pubkeys must NOT be a wallet-policy (I1)")
	}
}

// ─── Task 2: WalletDescriptorTemplateId (identity.rs:71-104) ─────────────────

// TestWalletDescriptorTemplateId_Golden pins the full-16B golden from md inspect
// (descriptor-mnemonic@54dd765) for the keyless wsh(sortedmulti(2,@0,@1,@2)).
func TestWalletDescriptorTemplateId_Golden(t *testing.T) {
	const want = "b02b44037119e6b6fd1d82f61aa17e21"
	got, err := WalletDescriptorTemplateId(keylessWshSortedmulti2of3())
	if err != nil {
		t.Fatal(err)
	}
	if hex.EncodeToString(got[:]) != want {
		t.Fatalf("WDT-Id = %x, want %s", got, want)
	}
}

// TestWalletDescriptorTemplateId_OverrideGolden pins the full-16B golden for the
// SAME keyless wsh(sortedmulti(2,@0,@1,@2)) template WITH a per-cosigner use-site
// override on @1 (<2;3>/* instead of the shared <0;1>/*). This exercises the
// UseSitePathOverrides-TLV branch of the WDT-Id preimage (template_id.go:61-88 /
// Rust identity.rs:79-98) — the no-override golden above never reaches it.
//
// Golden from md inspect (descriptor-mnemonic@54dd765, md-codec v0.37.0):
//
//	$ md encode 'wsh(sortedmulti(2,@0/<0;1>/*,@1/<2;3>/*,@2/<0;1>/*))' --group-size 0
//	md1yzpqqxppcgscpdtq2zcknf8ygsz0m039
//	$ md inspect --json md1yzpqqxppcgscpdtq2zcknf8ygsz0m039  → wallet_descriptor_template_id
//	ca812f8fc0bfea0fd1329ebfd34e5b07
//
// The codec stores the override as use_site_path_overrides = [(1, <2;3>/*)] with
// the shared use_site_path left at <0;1>/* (verified via md inspect --json). The
// in-package descriptor below reproduces that exact stored form: identical to
// keylessWshSortedmulti2of3() plus the @1 override.
func TestWalletDescriptorTemplateId_OverrideGolden(t *testing.T) {
	const want = "ca812f8fc0bfea0fd1329ebfd34e5b07"

	withOverride := keylessWshSortedmulti2of3()
	withOverride.tlv.useSitePresent = true
	withOverride.tlv.useSiteOverrides = []idxUseSite{{
		idx: 1,
		path: useSitePath{
			hasMultipath: true,
			multipath:    []alternative{{value: 2}, {value: 3}},
		},
	}}

	got, err := WalletDescriptorTemplateId(withOverride)
	if err != nil {
		t.Fatal(err)
	}
	if hex.EncodeToString(got[:]) != want {
		t.Fatalf("WDT-Id (with override) = %x, want %s", got, want)
	}

	// Non-vacuous: the SAME template WITHOUT the override must produce a DIFFERENT
	// id (the no-override golden b02b4403…). A regression that dropped the
	// override-TLV from the preimage would make these collide and FAIL here.
	noOverride, err := WalletDescriptorTemplateId(keylessWshSortedmulti2of3())
	if err != nil {
		t.Fatal(err)
	}
	if hex.EncodeToString(noOverride[:]) == want {
		t.Fatalf("no-override WDT-Id %x must differ from the with-override golden %s "+
			"(the override-TLV is not entering the preimage)", noOverride, want)
	}
}

// TestWalletDescriptorTemplateId_OriginInvariant: the id is identical under three
// different origins (no origin/path-decl bits enter the preimage — identity.rs).
func TestWalletDescriptorTemplateId_OriginInvariant(t *testing.T) {
	base := keylessWshSortedmulti2of3() // shared "m" (empty)

	// bip84-style shared origin m/84'/0'/0'.
	bip84 := keylessWshSortedmulti2of3()
	o84 := originPath{components: []pathComponent{{true, 84}, {true, 0}, {true, 0}}}
	bip84.pathDecl = pathDecl{n: 3, shared: &o84}

	// bip48 multisig origin m/48'/0'/0'/2'.
	bip48 := keylessWshSortedmulti2of3()
	o48 := originPath{components: []pathComponent{{true, 48}, {true, 0}, {true, 0}, {true, 2}}}
	bip48.pathDecl = pathDecl{n: 3, shared: &o48}

	idBase, err := WalletDescriptorTemplateId(base)
	if err != nil {
		t.Fatal(err)
	}
	for name, d := range map[string]*descriptor{"bip84": bip84, "bip48": bip48} {
		id, err := WalletDescriptorTemplateId(d)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if id != idBase {
			t.Errorf("%s id %x != base id %x (must be origin-invariant)", name, id, idBase)
		}
	}
}

// TestWalletDescriptorTemplateId_Distinct: the id discriminates on
// (script family, k, N) and the use-site multipath. wsh-multi != wsh-sortedmulti;
// k=1 != k=2; N=2 != N=3.
func TestWalletDescriptorTemplateId_Distinct(t *testing.T) {
	base := keylessWshSortedmulti2of3() // wsh sortedmulti k=2 N=3
	idBase, err := WalletDescriptorTemplateId(base)
	if err != nil {
		t.Fatal(err)
	}

	mkVariant := func(mutate func(d *descriptor)) [16]byte {
		d := keylessWshSortedmulti2of3()
		mutate(d)
		id, err := WalletDescriptorTemplateId(d)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}

	// wsh(multi(...)) vs wsh(sortedmulti(...)) — different inner tag.
	multi := mkVariant(func(d *descriptor) {
		d.tree.body = childrenBody{children: []node{{tag: tagMulti, body: multiKeysBody{k: 2, indices: []uint8{0, 1, 2}}}}}
	})
	if multi == idBase {
		t.Error("wsh(multi) id == wsh(sortedmulti) id, want distinct")
	}

	// k=1 vs k=2.
	k1 := mkVariant(func(d *descriptor) {
		d.tree.body = childrenBody{children: []node{{tag: tagSortedMulti, body: multiKeysBody{k: 1, indices: []uint8{0, 1, 2}}}}}
	})
	if k1 == idBase {
		t.Error("k=1 id == k=2 id, want distinct")
	}

	// N=2 vs N=3.
	n2 := mkVariant(func(d *descriptor) {
		d.n = 2
		empty := originPath{}
		d.pathDecl = pathDecl{n: 2, shared: &empty}
		d.tree.body = childrenBody{children: []node{{tag: tagSortedMulti, body: multiKeysBody{k: 2, indices: []uint8{0, 1}}}}}
	})
	if n2 == idBase {
		t.Error("N=2 id == N=3 id, want distinct")
	}
}

// TestWalletDescriptorTemplateIdStub: the stub is the top-4 bytes of the id.
func TestWalletDescriptorTemplateIdStub(t *testing.T) {
	stub, err := WalletDescriptorTemplateIdStub(keylessWshSortedmulti2of3())
	if err != nil {
		t.Fatal(err)
	}
	if stub != ([4]byte{0xb0, 0x2b, 0x44, 0x03}) {
		t.Fatalf("stub = %x, want b02b4403", stub)
	}
}

// ─── Task 3: form-aware stub selector (is_wallet_policy ? WPID : WDT-Id) ─────

// FormAwareStubChunks takes the FORK's own wire strings (the format split()
// emits and Reassemble accepts — distinct from the descriptor-mnemonic CLI's
// single-string dialect). We therefore encode the goldens with the fork's own
// split() and assert the selector picks the right id space, mirroring Rust
// mk-cli derive_stub_from_md1 (mod.rs:72-82). The WDT-Id / WalletPolicyId values
// themselves are pinned to the CLI goldens in the descriptor-form tests above.
func TestFormAwareStubChunks(t *testing.T) {
	// keyless template → WDT-Id top4 (b02b4403, the CLI golden).
	tmplChunks, err := split(keylessWshSortedmulti2of3())
	if err != nil {
		t.Fatalf("split template: %v", err)
	}
	got, err := FormAwareStubChunks(tmplChunks)
	if err != nil {
		t.Fatal(err)
	}
	if got != ([4]byte{0xb0, 0x2b, 0x44, 0x03}) {
		t.Fatalf("template stub = %x, want b02b4403", got)
	}

	// keyed policy → WalletPolicyId top4 (byte-identical to today's selector).
	keyedChunks, err := split(cell7WpkhDescriptor())
	if err != nil {
		t.Fatalf("split keyed: %v", err)
	}
	g2, err := FormAwareStubChunks(keyedChunks)
	if err != nil {
		t.Fatal(err)
	}
	w2, err := WalletPolicyIDStubChunks(keyedChunks)
	if err != nil {
		t.Fatal(err)
	}
	if g2 != w2 {
		t.Fatalf("keyed FormAwareStub %x != WalletPolicyIDStub %x (must select WalletPolicyId)", g2, w2)
	}
}

// TestFormAwareStub exercises the *descriptor form directly.
func TestFormAwareStub(t *testing.T) {
	// keyless template descriptor → WDT-Id.
	got, err := FormAwareStub(keylessWshSortedmulti2of3())
	if err != nil {
		t.Fatal(err)
	}
	if got != ([4]byte{0xb0, 0x2b, 0x44, 0x03}) {
		t.Fatalf("template FormAwareStub = %x, want b02b4403", got)
	}

	// keyed descriptor → WalletPolicyId (must equal WalletPolicyIDStub).
	keyed := cell7WpkhDescriptor()
	g2, err := FormAwareStub(keyed)
	if err != nil {
		t.Fatal(err)
	}
	w2, err := WalletPolicyIDStub(keyed)
	if err != nil {
		t.Fatal(err)
	}
	if g2 != w2 {
		t.Fatalf("keyed FormAwareStub %x != WalletPolicyIDStub %x", g2, w2)
	}
}

// TestFormAwareStubOwnReadback (C2): the stub minted from a descriptor at an
// emit site (FormAwareStub) MUST equal the stub recomputed from that
// descriptor's own engraved+re-decoded md1 at the verify site
// (FormAwareStubChunks(split(d))). This is the device's own-readback invariant
// at the codec layer — for a keyless template AND a keyed policy.
func TestFormAwareStubOwnReadback(t *testing.T) {
	cases := map[string]*descriptor{
		"keyless template": keylessWshSortedmulti2of3(),
		"keyed policy":     cell7WpkhDescriptor(),
	}
	for name, d := range cases {
		t.Run(name, func(t *testing.T) {
			emitStub, err := FormAwareStub(d)
			if err != nil {
				t.Fatal(err)
			}
			chunks, err := split(d)
			if err != nil {
				t.Fatalf("split: %v", err)
			}
			verifyStub, err := FormAwareStubChunks(chunks)
			if err != nil {
				t.Fatal(err)
			}
			if emitStub != verifyStub {
				t.Fatalf("own-readback mismatch: emit %x != verify %x (C2)", emitStub, verifyStub)
			}
		})
	}
}
