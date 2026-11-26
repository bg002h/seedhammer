package md

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// Bug hunt 2026-11-25 SH#1: `Check` over a bare PkK/PkH is the bare key (the
// Rust primary's Check-idempotence, md-codec to_miniscript.rs). The legacy
// wire -- every pk()/pkh() written as Check(PkK)/Check(PkH), by the toolkit
// before 0.55.0 and md-cli before 0.30 -- must emit EXACTLY the script the
// current bare wire emits. Before the fix the emitter appended a second
// OP_CHECKSIG: `<K> CHECKSIG CHECKSIG`, unsatisfiable, at an address no Rust
// tool derives. gui/legacy_check_address_test.go pins the resulting addresses
// to Rust's; this file pins the scripts to the bare wire's.

// legacyCheckVectors: corpus vectors whose legacy twins live in
// testdata/legacy_check. wsh position (pk, s:pk, pkh, v:pkh) and tap-leaf
// position (a bare pk leaf, and v:pk inside and_v).
var legacyCheckVectors = []string{
	"keyed_wsh_or_b",
	"keyed_wsh_thresh",
	"keyed_compose_wsh_single_head_or_i",
	"keyed_tr_depth2",
	"keyed_tr_with_leaf",
	"keyed_compose_tr_extracted_later_four_paths",
}

// wrapKeyChecks re-creates the legacy wire: every PkK/PkH node wrapped in Check.
func wrapKeyChecks(n node) node {
	switch b := n.body.(type) {
	case keyArgBody:
		if n.tag == tagPkK || n.tag == tagPkH {
			return node{tag: tagCheck, body: childrenBody{children: []node{n}}}
		}
	case childrenBody:
		cs := make([]node, len(b.children))
		for i, c := range b.children {
			cs[i] = wrapKeyChecks(c)
		}
		return node{tag: n.tag, body: childrenBody{children: cs}}
	case variableBody:
		cs := make([]node, len(b.children))
		for i, c := range b.children {
			cs[i] = wrapKeyChecks(c)
		}
		return node{tag: n.tag, body: variableBody{k: b.k, children: cs}}
	case trBody:
		if b.tree != nil {
			t := wrapKeyChecks(*b.tree)
			b.tree = &t
		}
		return node{tag: n.tag, body: b}
	}
	return n
}

// unwrapKeyChecks is the inverse: the current (bare) wire of a legacy card.
func unwrapKeyChecks(n node) node {
	switch b := n.body.(type) {
	case childrenBody:
		if n.tag == tagCheck && len(b.children) == 1 {
			if k := b.children[0]; k.tag == tagPkK || k.tag == tagPkH {
				if _, ok := k.body.(keyArgBody); ok {
					return k
				}
			}
		}
		cs := make([]node, len(b.children))
		for i, c := range b.children {
			cs[i] = unwrapKeyChecks(c)
		}
		return node{tag: n.tag, body: childrenBody{children: cs}}
	case variableBody:
		cs := make([]node, len(b.children))
		for i, c := range b.children {
			cs[i] = unwrapKeyChecks(c)
		}
		return node{tag: n.tag, body: variableBody{k: b.k, children: cs}}
	case trBody:
		if b.tree != nil {
			t := unwrapKeyChecks(*b.tree)
			b.tree = &t
		}
		return node{tag: n.tag, body: b}
	}
	return n
}

func countKeyChecks(n node) int {
	c := 0
	switch b := n.body.(type) {
	case childrenBody:
		if n.tag == tagCheck && len(b.children) == 1 && (b.children[0].tag == tagPkK || b.children[0].tag == tagPkH) {
			c++
		}
		for _, x := range b.children {
			c += countKeyChecks(x)
		}
	case variableBody:
		for _, x := range b.children {
			c += countKeyChecks(x)
		}
	case trBody:
		if b.tree != nil {
			c += countKeyChecks(*b.tree)
		}
	}
	return c
}

func legacyCheckChunks(t *testing.T, file string) []string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "legacy_check", file+".md1.txt"))
	if err != nil {
		t.Fatal(err)
	}
	return md1Lines(string(raw))
}

// The legacy fixtures are what this port's encoder writes for the vector with
// every key wrapped -- regenerated here so they cannot drift from the vectors --
// and each really carries the Check-over-key wire.
func TestLegacyCheckFixturesAreTheWrappedVectors(t *testing.T) {
	for _, name := range legacyCheckVectors {
		t.Run(name, func(t *testing.T) {
			d, err := Reassemble(vectorChunksFor(t, name))
			if err != nil {
				t.Fatal(err)
			}
			d.tree = wrapKeyChecks(d.tree)
			got, err := split(d)
			if err != nil {
				t.Fatal(err)
			}
			if want := legacyCheckChunks(t, "legacy_"+name); !reflect.DeepEqual(got, want) {
				t.Fatalf("regenerated legacy card differs from the fixture:\n got  %q\n want %q", got, want)
			}
		})
	}
}

// THE PIN: a legacy card emits byte-for-byte the scripts of its bare twin, in
// wsh position and in tap-leaf position, and the describer agrees.
func TestLegacyCheckOverKeyEmitsTheBareScript(t *testing.T) {
	files := []string{"legacy_bughunt_sh1_or_d"}
	for _, n := range legacyCheckVectors {
		files = append(files, "legacy_"+n)
	}
	for _, file := range files {
		t.Run(file, func(t *testing.T) {
			legacy := legacyCheckChunks(t, file)
			d, err := Reassemble(legacy)
			if err != nil {
				t.Fatalf("Reassemble: %v", err)
			}
			if countKeyChecks(d.tree) == 0 {
				t.Fatal("INCONCLUSIVE: the fixture carries no Check over a bare key")
			}
			d.tree = unwrapKeyChecks(d.tree)
			if countKeyChecks(d.tree) != 0 {
				t.Fatal("INCONCLUSIVE: unwrap left a Check over a key")
			}
			bare, err := split(d)
			if err != nil {
				t.Fatal(err)
			}
			// Distinct per-slot keys: 33 bytes for wsh, 32 x-only for tap.
			keys33 := map[uint8][]byte{}
			keys32 := map[uint8][]byte{}
			for i := uint8(0); i < d.n; i++ {
				keys33[i] = append([]byte{0x02}, bytes.Repeat([]byte{i + 1}, 32)...)
				keys32[i] = bytes.Repeat([]byte{i + 1}, 32)
			}
			if d.tree.tag == tagTr {
				_, ikL, leavesL, err := EmitTapLeavesChunks(legacy, keys32)
				if err != nil {
					t.Fatalf("EmitTapLeavesChunks(legacy): %v", err)
				}
				_, ikB, leavesB, err := EmitTapLeavesChunks(bare, keys32)
				if err != nil {
					t.Fatalf("EmitTapLeavesChunks(bare): %v", err)
				}
				if ikL != ikB || !reflect.DeepEqual(leavesL, leavesB) {
					t.Fatalf("tap leaves differ:\n legacy %x\n bare   %x", leavesL, leavesB)
				}
				for _, l := range leavesL {
					if bytes.Contains(l.Script, []byte{opCHECKSIG, opCHECKSIG}) {
						t.Fatalf("leaf %x carries OP_CHECKSIG OP_CHECKSIG", l.Script)
					}
				}
				iL, kL, dL, errL := TapLeavesChunks(legacy)
				iB, kB, dB, errB := TapLeavesChunks(bare)
				if (errL == nil) != (errB == nil) || iL != iB || kL != kB || !reflect.DeepEqual(dL, dB) {
					t.Fatalf("TapLeavesChunks differ: legacy (%v %v %+v %v), bare (%v %v %+v %v)",
						iL, kL, dL, errL, iB, kB, dB, errB)
				}
				return
			}
			sL, err := EmitWitnessScriptChunks(legacy, keys33)
			if err != nil {
				t.Fatalf("EmitWitnessScriptChunks(legacy): %v", err)
			}
			sB, err := EmitWitnessScriptChunks(bare, keys33)
			if err != nil {
				t.Fatalf("EmitWitnessScriptChunks(bare): %v", err)
			}
			if !bytes.Equal(sL, sB) {
				t.Fatalf("witness scripts differ:\n legacy %x\n bare   %x", sL, sB)
			}
			if bytes.Contains(sL, []byte{opCHECKSIG, opCHECKSIG}) {
				t.Fatalf("script %x carries OP_CHECKSIG OP_CHECKSIG", sL)
			}
		})
	}
}

// Only a BARE key is unwrapped, as in Rust. Check over anything else keeps its
// OP_CHECKSIG in the emitter (Rust refuses Check(Check(PkK)) at type-check;
// this port has no type checker, see the PR), and the describer no longer
// calls Check(Check(PkK)) a plain pk leaf.
func TestCheckIdempotenceIsOnlyForABareKey(t *testing.T) {
	pk := node{tag: tagPkK, body: keyArgBody{index: 0}}
	check := func(n node) node { return node{tag: tagCheck, body: childrenBody{children: []node{n}}} }
	key := append([]byte{0x02}, bytes.Repeat([]byte{7}, 32)...)
	e := emitEnv{keys: map[uint8][]byte{0: key}}
	var bare, once, twice []byte
	if err := emitFragment(pk, e, &bare); err != nil {
		t.Fatal(err)
	}
	if err := emitFragment(check(pk), e, &once); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(bare, once) {
		t.Fatalf("Check(PkK) = %x, want the bare key's %x", once, bare)
	}
	if err := emitFragment(check(check(pk)), e, &twice); err != nil {
		t.Fatal(err)
	}
	if want := append(append([]byte(nil), bare...), opCHECKSIG); !bytes.Equal(twice, want) {
		t.Fatalf("Check(Check(PkK)) = %x, want %x (only the inner Check is idempotent)", twice, want)
	}
	if _, err := describeTapLeaf(check(pk)); err != nil {
		t.Fatalf("describe Check(PkK): %v", err)
	}
	if l, err := describeTapLeaf(check(check(pk))); err == nil {
		t.Fatalf("describe Check(Check(PkK)) = %+v, want refusal", l)
	}
}
