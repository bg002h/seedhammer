package md

import (
	"encoding/hex"
	"testing"
)

func mustHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatalf("bad hex: %v", err)
	}
	return b
}

// multiInner asserts the single child of a Children body is a multi-family
// node with the given tag, k, and index count.
func multiInner(t *testing.T, d *descriptor, want tag, k uint8, n int) {
	t.Helper()
	ch, ok := d.tree.body.(childrenBody)
	if !ok || len(ch.children) != 1 {
		t.Fatalf("root body = %T (want single-child childrenBody)", d.tree.body)
	}
	mk, ok := ch.children[0].body.(multiKeysBody)
	if !ok || ch.children[0].tag != want || mk.k != k || len(mk.indices) != n {
		t.Fatalf("inner = tag %v body %+v", ch.children[0].tag, ch.children[0].body)
	}
}

func TestDecodePayloadAST(t *testing.T) {
	cases := []struct {
		name  string
		bytes string // verbatim tests/vectors/<name>.bytes.hex
		n     int
		root  tag
		check func(t *testing.T, d *descriptor)
	}{
		{"wpkh_basic", "2002001800", 1, tagWpkh, func(t *testing.T, d *descriptor) {
			if _, ok := d.tree.body.(keyArgBody); !ok {
				t.Fatalf("wpkh body = %T want keyArgBody", d.tree.body)
			}
		}},
		{"pkh_basic", "2002001840", 1, tagPkh, func(t *testing.T, d *descriptor) {
			if _, ok := d.tree.body.(keyArgBody); !ok {
				t.Fatalf("pkh body = %T want keyArgBody", d.tree.body)
			}
		}},
		{"wsh_multi_2of2", "20420018218214", 2, tagWsh, func(t *testing.T, d *descriptor) {
			multiInner(t, d, tagMulti, 2, 2)
		}},
		{"wsh_multi_2of3", "2082001821822180", 3, tagWsh, func(t *testing.T, d *descriptor) {
			multiInner(t, d, tagMulti, 2, 3)
		}},
		{"wsh_sortedmulti", "2082001821c22180", 3, tagWsh, func(t *testing.T, d *descriptor) {
			multiInner(t, d, tagSortedMulti, 2, 3)
		}},
		{"tr_keyonly", "2002001810", 1, tagTr, func(t *testing.T, d *descriptor) {
			tr, ok := d.tree.body.(trBody)
			if !ok || tr.isNums || tr.tree != nil {
				t.Fatalf("tr body = %+v", d.tree.body)
			}
		}},
		{"sh_wsh_multi", "2042001830860850", 2, tagSh, func(t *testing.T, d *descriptor) {
			ch, ok := d.tree.body.(childrenBody)
			if !ok || len(ch.children) != 1 || ch.children[0].tag != tagWsh {
				t.Fatalf("sh body = %T", d.tree.body)
			}
			inner := ch.children[0]
			wch, ok := inner.body.(childrenBody)
			if !ok || len(wch.children) != 1 {
				t.Fatalf("wsh body = %T", inner.body)
			}
			mk, ok := wch.children[0].body.(multiKeysBody)
			if !ok || wch.children[0].tag != tagMulti || mk.k != 2 || len(mk.indices) != 2 {
				t.Fatalf("inner multi = %v %+v", wch.children[0].tag, wch.children[0].body)
			}
		}},
		{"wsh_divergent_paths", "204200182182140b4c0a16", 2, tagWsh, func(t *testing.T, d *descriptor) {
			multiInner(t, d, tagMulti, 2, 2)
			if d.tlv.useSiteOverrides == nil || len(d.tlv.useSiteOverrides) != 1 {
				t.Fatalf("use-site overrides = %+v", d.tlv.useSiteOverrides)
			}
			if d.tlv.useSiteOverrides[0].idx != 1 {
				t.Fatalf("override idx = %d want 1", d.tlv.useSiteOverrides[0].idx)
			}
		}},
		{"wsh_with_fingerprints", "204200182182142f09bd5b7ddfcafebabe", 2, tagWsh, func(t *testing.T, d *descriptor) {
			multiInner(t, d, tagMulti, 2, 2)
			if d.tlv.fingerprints == nil || len(d.tlv.fingerprints) != 2 {
				t.Fatalf("fingerprints = %+v", d.tlv.fingerprints)
			}
			if d.tlv.fingerprints[0].idx != 0 || d.tlv.fingerprints[1].idx != 1 {
				t.Fatalf("fingerprint idxs = %+v", d.tlv.fingerprints)
			}
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b := mustHex(t, c.bytes)
			d, err := decodePayload(b, len(b)*8) // bytes.hex is byte-aligned
			if err != nil {
				t.Fatalf("decodePayload: %v", err)
			}
			if d.n != uint8(c.n) || d.tree.tag != c.root {
				t.Fatalf("n=%d root=%v want n=%d root=%v", d.n, d.tree.tag, c.n, c.root)
			}
			c.check(t, d)
		})
	}
}
