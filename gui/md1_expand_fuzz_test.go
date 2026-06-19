package gui

import (
	"testing"

	"seedhammer.com/bip380"
	"seedhammer.com/md"
)

// isBip380ExpressibleShape reports whether (root, policy, innerWsh) is one of
// the faithfully-projectable shapes (D2) — the ONLY shapes for which
// expandedToDescriptor may return expandOK.
func isBip380ExpressibleShape(root md.ScriptKind, policy md.PolicyKind, renderable bool) bool {
	if !renderable {
		return false
	}
	switch policy {
	case md.PolicySingle:
		return root == md.ScriptWpkh || root == md.ScriptPkh || root == md.ScriptTr
	case md.PolicySortedMulti:
		return root == md.ScriptWsh || root == md.ScriptSh
	}
	return false
}

// FuzzExpandedToDescriptor builds an arbitrary Template + ExpandedKey slice from
// fuzz bytes and projects it. expandedToDescriptor must never panic, must never
// return expandOK for a non-bip380 shape, and on expandOK must return a non-nil
// descriptor (and a nil descriptor otherwise).
func FuzzExpandedToDescriptor(f *testing.F) {
	f.Add([]byte{0, 0, 2, 3, 1, 1}) // wpkh single, 1 key, xpub present
	f.Add([]byte{3, 2, 2, 3, 1, 1}) // sh sortedmulti, innerWsh, 2 keys
	f.Add([]byte{4, 1, 3, 2, 0, 0}) // tr multi (unsorted) — must be unsupported
	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, b []byte) {
		at := func(i int) byte {
			if i < len(b) {
				return b[i]
			}
			return 0
		}
		root := md.ScriptKind(int(at(0)) % 5)   // 0..4
		policy := md.PolicyKind(int(at(1)) % 6) // 0..5
		n := int(at(2))%4 + 1                   // 1..4
		k := int(at(3))%n + 1                   // 1..n
		renderable := at(4)&1 == 1
		innerWsh := at(5)&1 == 1
		xpubPresent := at(5)&2 == 2
		wildcardHardened := at(5)&4 == 4

		tpl := md.Template{
			N: n, Root: root, Policy: policy, K: k, M: n,
			Renderable: renderable, InnerWsh: innerWsh,
		}
		keys := make([]md.ExpandedKey, n)
		for i := range keys {
			keys[i] = md.ExpandedKey{
				Index:       uint8(i),
				UseSite:     md.UseSite{HasMultipath: true, Multipath: []md.UseSiteAlt{{Value: 0}, {Value: 1}}, WildcardHardened: wildcardHardened},
				Xpub:        goldenXpub(i % 3),
				XpubPresent: xpubPresent,
			}
		}

		desc, status := expandedToDescriptor(tpl, keys)

		switch status {
		case expandOK:
			if desc == nil {
				t.Fatal("expandOK with nil descriptor")
			}
			if !isBip380ExpressibleShape(root, policy, renderable) {
				t.Fatalf("expandOK for non-bip380 shape root=%v policy=%v renderable=%v", root, policy, renderable)
			}
			// expandOK must imply a known, address-mappable script.
			if desc.Script == bip380.UnknownScript {
				t.Fatal("expandOK with UnknownScript")
			}
		case expandTemplateOnly, expandUnsupported:
			if desc != nil {
				t.Fatalf("status %v must return a nil descriptor", status)
			}
		default:
			t.Fatalf("unknown status %v", status)
		}
	})
}
