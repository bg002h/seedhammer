package md

import (
	"errors"
	"testing"
)

type tvec struct {
	name   string
	phrase string // verbatim tests/vectors/<name>.phrase.txt (single md1 line)
	n      int
	root   ScriptKind
	policy PolicyKind
	k, m   int
	keys   []KeyOrigin
	render bool
}

var parity = []tvec{
	{"wpkh_basic", "md1yqpqqxqq8xtwhw4xwn4qh", 1, ScriptWpkh, PolicySingle, 0, 0,
		[]KeyOrigin{{Index: 0, Fingerprint: "", OriginPath: "m", UseSite: "<0;1>/*"}}, true},
	{"pkh_basic", "md1yqpqqxzq2qwfv8urt848e", 1, ScriptPkh, PolicySingle, 0, 0,
		[]KeyOrigin{{Index: 0, Fingerprint: "", OriginPath: "m", UseSite: "<0;1>/*"}}, true},
	{"wsh_multi_2of2", "md1yppqqxppsg2vlumagltz27le", 2, ScriptWsh, PolicyMulti, 2, 2,
		[]KeyOrigin{{0, "", "m", "<0;1>/*"}, {1, "", "m", "<0;1>/*"}}, true},
	{"wsh_multi_2of3", "md1yzpqqxppsgsc8dua4tu0kekyl", 3, ScriptWsh, PolicyMulti, 2, 3,
		[]KeyOrigin{{0, "", "m", "<0;1>/*"}, {1, "", "m", "<0;1>/*"}, {2, "", "m", "<0;1>/*"}}, true},
	{"wsh_sortedmulti", "md1yzpqqxppcgsc9kdmw6d5dp08f", 3, ScriptWsh, PolicySortedMulti, 2, 3,
		[]KeyOrigin{{0, "", "m", "<0;1>/*"}, {1, "", "m", "<0;1>/*"}, {2, "", "m", "<0;1>/*"}}, true},
	{"tr_keyonly", "md1yqpqqxqsqgprhfjpjaz6d", 1, ScriptTr, PolicySingle, 0, 0,
		[]KeyOrigin{{0, "", "m", "<0;1>/*"}}, true},
	{"sh_wsh_multi", "md1yppqqxpsscy96gddy0v67f8tp", 2, ScriptSh, PolicyMulti, 2, 2,
		[]KeyOrigin{{0, "", "m", "<0;1>/*"}, {1, "", "m", "<0;1>/*"}}, true},
	{"wsh_with_fingerprints", "md1yppqqxppsg2z7zdatd7aljh7h2lqp277wajaesknu", 2, ScriptWsh, PolicyMulti, 2, 2,
		[]KeyOrigin{{0, "deadbeef", "m", "<0;1>/*"}, {1, "cafebabe", "m", "<0;1>/*"}}, true},
	{"wsh_divergent_paths", "md1yppqqxppsg2qknq2zc2ktzhwekmddzh", 2, ScriptWsh, PolicyMulti, 2, 2,
		[]KeyOrigin{{0, "", "m", "<0;1>/*"}, {1, "", "m", "<2;3>/*"}}, true},
}

func TestDecodeParity(t *testing.T) {
	for _, v := range parity {
		t.Run(v.name, func(t *testing.T) {
			tpl, err := Decode(v.phrase)
			if err != nil {
				t.Fatalf("Decode: %v", err)
			}
			if tpl.N != v.n || tpl.Root != v.root || tpl.Policy != v.policy ||
				tpl.K != v.k || tpl.M != v.m || tpl.Renderable != v.render {
				t.Fatalf("got %+v want n=%d root=%v pol=%v k=%d m=%d render=%v",
					tpl, v.n, v.root, v.policy, v.k, v.m, v.render)
			}
			if len(tpl.Keys) != len(v.keys) {
				t.Fatalf("keys=%d want %d", len(tpl.Keys), len(v.keys))
			}
			for i, k := range v.keys {
				if tpl.Keys[i] != k {
					t.Fatalf("key %d = %+v want %+v", i, tpl.Keys[i], k)
				}
			}
		})
	}
}

func TestDecodeChunkedRefused(t *testing.T) {
	// wsh_multi_chunked: the md1 chunk line (line 2 of phrase.txt, after the
	// "chunk-set-id:" comment) — verbatim.
	const chunk = "md1fz4awqqpqsgqpsgvyyxqql8saf74dwdyqv"
	if _, err := Decode(chunk); !errors.Is(err, ErrChunkedUnsupported) {
		t.Fatalf("chunked md1: want ErrChunkedUnsupported, got %v", err)
	}
}
