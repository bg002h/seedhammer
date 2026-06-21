package md

import (
	"testing"
)

// ─── Task 8: template-engrave shape refusals (C4) ────────────────────────────
//
// templateEngraveShapeGuard refuses the render-gap shapes the shipped toolkit
// cannot reconstruct: tr(sortedmulti_a) and sortedmulti/multi nested under a
// combinator. These shapes ENCODE/STRIP fine today (no parse refusal) — the
// guard is NEW refusal code on the template-engrave path. tr(NUMS, multi_a) is
// ADMITTED (the toolkit ships it); a hardened use-site STRIPS fine on the fork
// (its refusal is an off-device derive/address concern, not the template wire).

// trNumsSortedMultiAGuard: tr(NUMS, sortedmulti_a(2,@0,@1,@2)). REFUSED.
func trNumsSortedMultiAGuard() *descriptor {
	o := originPath{components: []pathComponent{{true, 48}, {true, 0}, {true, 0}, {true, 2}}}
	leaf := node{tag: tagSortedMultiA, body: multiKeysBody{k: 2, indices: []uint8{0, 1, 2}}}
	return &descriptor{
		n:        3,
		pathDecl: pathDecl{n: 3, shared: &o},
		useSite:  useSitePath{hasMultipath: true, multipath: []alternative{{value: 0}, {value: 1}}},
		tree:     node{tag: tagTr, body: trBody{isNums: true, keyIndex: 0, tree: &leaf}},
	}
}

// trNumsMultiAGuard: tr(NUMS, multi_a(2,@0,@1,@2)). ADMITTED.
func trNumsMultiAGuard() *descriptor {
	o := originPath{components: []pathComponent{{true, 48}, {true, 0}, {true, 0}, {true, 2}}}
	leaf := node{tag: tagMultiA, body: multiKeysBody{k: 2, indices: []uint8{0, 1, 2}}}
	return &descriptor{
		n:        3,
		pathDecl: pathDecl{n: 3, shared: &o},
		useSite:  useSitePath{hasMultipath: true, multipath: []alternative{{value: 0}, {value: 1}}},
		tree:     node{tag: tagTr, body: trBody{isNums: true, keyIndex: 0, tree: &leaf}},
	}
}

// wshOrISortedMultiGuard: wsh(or_i(sortedmulti(2,@0,@1), <other>)) — sortedmulti
// nested under a combinator (or_i). REFUSED.
func wshOrISortedMultiGuard() *descriptor {
	o := originPath{components: []pathComponent{{true, 48}, {true, 0}, {true, 0}, {true, 2}}}
	sm := node{tag: tagSortedMulti, body: multiKeysBody{k: 2, indices: []uint8{0, 1}}}
	other := node{tag: tagSortedMulti, body: multiKeysBody{k: 1, indices: []uint8{0, 1}}}
	orI := node{tag: tagOrI, body: childrenBody{children: []node{sm, other}}}
	return &descriptor{
		n:        2,
		pathDecl: pathDecl{n: 2, shared: &o},
		useSite:  useSitePath{hasMultipath: true, multipath: []alternative{{value: 0}, {value: 1}}},
		tree:     node{tag: tagWsh, body: childrenBody{children: []node{orI}}},
	}
}

// canonicalWshSortedMultiGuard: wsh(sortedmulti(2,@0,@1,@2)) — sortedmulti
// DIRECTLY under wsh (the canonical shape). ADMITTED.
func canonicalWshSortedMultiGuard() *descriptor {
	return keylessWshSortedmulti2of3()
}

func TestTemplateEngraveShapeGuard(t *testing.T) {
	refused := map[string]*descriptor{
		"tr(sortedmulti_a)":          trNumsSortedMultiAGuard(),
		"wsh(or_i(sortedmulti,...))": wshOrISortedMultiGuard(),
	}
	for name, d := range refused {
		t.Run("refuse/"+name, func(t *testing.T) {
			// It encodes/strips today (C4 — no parse refusal).
			if _, err := split(d); err != nil {
				t.Fatalf("precondition: %s must still ENCODE today (C4); got %v", name, err)
			}
			// The guard refuses it.
			if err := templateEngraveShapeGuard(d); err == nil {
				t.Fatalf("templateEngraveShapeGuard must REFUSE %s", name)
			}
		})
	}

	admitted := map[string]*descriptor{
		"tr(NUMS, multi_a)":          trNumsMultiAGuard(),
		"wsh(sortedmulti) canonical": canonicalWshSortedMultiGuard(),
		"single-sig wpkh":            keylessWpkhGuard(),
	}
	for name, d := range admitted {
		t.Run("admit/"+name, func(t *testing.T) {
			if err := templateEngraveShapeGuard(d); err != nil {
				t.Fatalf("templateEngraveShapeGuard must ADMIT %s; got %v", name, err)
			}
		})
	}
}

// keylessWpkhGuard: wpkh(@0) single-sig template. ADMITTED.
func keylessWpkhGuard() *descriptor {
	o := originPath{components: []pathComponent{{true, 84}, {true, 0}, {true, 0}}}
	return &descriptor{
		n:        1,
		pathDecl: pathDecl{n: 1, shared: &o},
		useSite:  useSitePath{hasMultipath: true, multipath: []alternative{{value: 0}, {value: 1}}},
		tree:     node{tag: tagWpkh, body: keyArgBody{index: 0}},
	}
}

// TestTemplateGuardHardenedUseSiteStrips: a hardened use-site (/*h) STRIPS fine
// on the fork (the refusal is an off-device derive/address concern, not the
// template wire). The guard does NOT refuse it.
func TestTemplateGuardHardenedUseSiteStrips(t *testing.T) {
	d := keylessWpkhGuard()
	d.useSite.wildcardHardened = true
	if err := templateEngraveShapeGuard(d); err != nil {
		t.Fatalf("hardened use-site must NOT be refused by the template shape guard (off-device concern); got %v", err)
	}
	chunks, err := split(d)
	if err != nil {
		t.Fatalf("hardened use-site template must STRIP/encode on the fork; got %v", err)
	}
	if _, err := StripToTemplate(chunks); err != nil {
		t.Fatalf("StripToTemplate of a hardened use-site template must succeed; got %v", err)
	}
}
