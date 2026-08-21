package md

import (
	"testing"
)

// ─── Task 8: template-engrave shape refusals (C4) ────────────────────────────
//
// templateEngraveShapeGuard refuses the render-gap shapes the shipped toolkit
// cannot reconstruct. It once named TWO; since F-215 (2026-08-21) only one is
// left, and the correction is the interesting part.
//
// tr(sortedmulti_a) is now ADMITTED. The refusal's premise — "no
// rust-miniscript renderer" — died with the S0 pin lift (PR #910 gave upstream
// Terminal::SortedMultiA) and R5. Re-measured on the current binaries before
// this test moved: the shape encodes to one chunk, `md decode` returns the
// template verbatim at exit 0, `md verify` re-encodes it to its own template,
// and `md address` derives a real address. Fully recoverable, which is the only
// thing this guard ever asked.
//
// sortedmulti nested under a COMBINATOR stays refused — and it is now
// unreachable from our own encoder besides (`md encode` rejects it by
// BIP-383/388), so the arm is defence against a card from some other producer.
//
// tr(NUMS, multi_a) is ADMITTED; a hardened use-site STRIPS fine on the fork
// (its refusal is an off-device derive/address concern, not the template wire).

// trNumsSortedMultiAGuard: tr(NUMS, sortedmulti_a(2,@0,@1,@2)). ADMITTED since F-215.
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

// wshOrILegacyMultiGuard: wsh(or_i(multi(2,@0,@1), multi(1,@0,@1))) — LEGACY
// multi nested under a combinator (or_i). ADMITTED (C1 regression: legacy multi
// renders fine inside combinators; only sortedmulti is the render-gap shape).
func wshOrILegacyMultiGuard() *descriptor {
	o := originPath{components: []pathComponent{{true, 48}, {true, 0}, {true, 0}, {true, 2}}}
	m := node{tag: tagMulti, body: multiKeysBody{k: 2, indices: []uint8{0, 1}}}
	other := node{tag: tagMulti, body: multiKeysBody{k: 1, indices: []uint8{0, 1}}}
	orI := node{tag: tagOrI, body: childrenBody{children: []node{m, other}}}
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
		// F-215: was in `refused` until the pin lift made it recoverable.
		"tr(sortedmulti_a)":           trNumsSortedMultiAGuard(),
		"tr(NUMS, multi_a)":           trNumsMultiAGuard(),
		"wsh(sortedmulti) canonical":  canonicalWshSortedMultiGuard(),
		"wsh(or_i(multi,...)) legacy": wshOrILegacyMultiGuard(),
		"single-sig wpkh":             keylessWpkhGuard(),
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

// TestTemplateGuardAdmitsDegrade2: the §5 degrade2 11-key general-miniscript
// wallet (wsh(or_i(and_v(...multi(3,...)),...))) uses LEGACY multi inside
// combinators — which the shipped toolkit ADMITS (its bundle --md1-form=template
// produced the committed degrade2_11key.tmpl golden). The guard MUST admit it
// (C1 exec-review regression: an over-broad multi-in-combinator refusal would
// reject this SPEC-in-scope wallet — DD3/DD7).
func TestTemplateGuardAdmitsDegrade2(t *testing.T) {
	tmpl := loadTemplateMD1(t, "degrade2_11key.tmpl.md1.txt")
	if err := TemplateEngraveShapeGuardChunks(tmpl); err != nil {
		t.Fatalf("template guard must ADMIT the §5 degrade2 general-miniscript wallet (legacy multi in a combinator is admissible); got %v", err)
	}
}
