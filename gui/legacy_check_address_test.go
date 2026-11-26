package gui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Bug hunt 2026-11-25 SH#1, end to end: a card on the LEGACY wire --
// Check(PkK)/Check(PkH), written by the toolkit before 0.55.0 and md-cli before
// 0.30, and already engraved -- shows the addresses the Rust primary derives
// for it, through the route the device takes (routeFor mirrors
// gatheredDescriptorFlow). Before the fix the emitter doubled OP_CHECKSIG and
// the device showed, for the report's card, bc1qedfsrc... where Rust gives
// bc1qhzn56d....
//
// The expected addresses are Rust's for the LEGACY strings themselves:
// `md address <card> --count 2` and `... --chain 1`, md 0.21.1 (md-codec
// 0.48.7), descriptor-mnemonic c8cde234, 2026-11-25. For the six cards made
// from corpus vectors they are also the vector's recorded addresses, which the
// test checks too, so the legacy card and its bare twin are one wallet.
func TestLegacyCheckWireShowsRustsAddresses(t *testing.T) {
	for _, tc := range []struct {
		file, vector    string
		receive, change [2]string
	}{
		{"legacy_bughunt_sh1_or_d", "",
			[2]string{"bc1qhzn56dvpfvw4l5eq7lac2ap6v0jfsx4p222srpupjp0vymsfn3wsen6k8f", "bc1qcswkglea0z2m5wx52khh02u7tfmv2hp8zl9xa4wrz9hrdhdfhlpqdte4qg"},
			[2]string{"bc1qesm6axskacde4yh29udz8wyy6awmm723ct3tncydqqzeas6sy4zqav8uqm", "bc1qgpcx79wwsqzzew6pgfsqnwaucenemgf53dyyutvwj9cxmtqxxr5q94mpsa"}},
		{"legacy_keyed_wsh_or_b", "keyed_wsh_or_b",
			[2]string{"bc1qkxn9tp0ajyvsegdjgtjhjvxg6rn9eg0hz97dzhpevftgk9pu2crs4e80wp", "bc1qvxee3n7r7re00730ln44pljelpmqpycvmrae9hwu42hey8ukej8srfhq3w"},
			[2]string{"bc1qj868p5ft0c6kymmqheagxq9ltyevf5r9lv08tth4gcfn90ucwl4q3g5qu0", "bc1q9hsqgw7cedx5ns5egplp69ncuu3hqaacm7rjrusf5nluqe24j3eqcd64gu"}},
		{"legacy_keyed_wsh_thresh", "keyed_wsh_thresh",
			[2]string{"bc1q83afalnrsx6jrxyzkgn0ju2z3lg8t6cp52tq3cantqy57rlsfueqjtpct5", "bc1qnn569ln8m5n9nsk4nt27hlxdqvlr9t87s0ukutx5vshk4fl0jm9qrsgjlg"},
			[2]string{"bc1qaep06u3z0jxc26mkn5w7fncqr7xn02gs2xtd5ylurndpllqzhlgsrc5e8c", "bc1qygwg73vtkyqfe8y7mzqhpfhwy6yyfc8wumusmz9rt60es3utprxq2jhx03"}},
		{"legacy_keyed_compose_wsh_single_head_or_i", "keyed_compose_wsh_single_head_or_i",
			[2]string{"bc1qxu2qtx7w7xd85qmmm2jzkdysg3zd7dwv89ez55guugshz7ls96gq22gkhs", "bc1q97qcq6feunv637efvmejm4zdh5kg8xlcgnzy3hxkfsajstjyw48qjl4j4p"},
			[2]string{"bc1qq7pahjvykvfhhzw5aeh0pscvupv0sdf62pprfg5n0hyhfyag5fss8zyy66", "bc1q0xs97cysnee57xhmqquzmphnd4m3p74nsnncsv2km8gunrey7g2qx0v4jq"}},
		{"legacy_keyed_tr_depth2", "keyed_tr_depth2",
			[2]string{"bc1p2cycz66cyzltrvy5xd6v6g0jz3m5f9tjtz0aqkx9a5xet23su5rsa4mewu", "bc1pa4sqhg0qw69zhqyaw3wv75ke49rwdf9cdc4taah3z5p58lp3zc8se4amnr"},
			[2]string{"bc1pq67e92udm4tldq48jjdaravtekrw6k7u9apf6alhq2sfe8m9t6hsseycm5", "bc1pf0lcpa0n7whukgcapvl022964qyu40h4cq7uc06puphjl28ntf3q9uck2h"}},
		{"legacy_keyed_tr_with_leaf", "keyed_tr_with_leaf",
			[2]string{"bc1p2ps82seqkfnl6a02eznxqmyqye5ruvdrpagvw6qrg3k6gd7x95mq4cmjyc", "bc1pne93ph6acfeh49mts45l5nvqtqytme5m7nuhmaa20m4y6lptz24s0rmhep"},
			[2]string{"bc1pga3t0asl8tkt97rjyncf52q6358gws2v32s7kvh7zqedtm4cd6ys5gxv4t", "bc1p2p3nvx74mq4yvgd3t0akhmsdc7qgfhztth7zc7vex2fjjkx3752qjag2kg"}},
		{"legacy_keyed_compose_tr_extracted_later_four_paths", "keyed_compose_tr_extracted_later_four_paths",
			[2]string{"bc1pxtgs2egtltsccdg9lwfnanfvvknl7040j9vfhcglgeffw7fnjsns3kh6zy", "bc1pf274rsgf4c3m6wneevrtyksju3dpvng6lpw74vj8c2dnper4rlaqhjhefp"},
			[2]string{"bc1p5zacakwqcprvxqp4gv53x8wm22fz4pl5q6qu06zkzk27dfn96k3qf98ap0", "bc1px4cj57se2lgargeq28t2p5cxk86t7quza9yga683nkwsmkgcf9ks7t33qj"}},
	} {
		t.Run(tc.file, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join("..", "md", "testdata", "legacy_check", tc.file+".md1.txt"))
			if err != nil {
				t.Fatal(err)
			}
			var chunks []string
			for _, l := range strings.Split(string(raw), "\n") {
				if l = strings.TrimSpace(l); strings.HasPrefix(l, "md1") {
					chunks = append(chunks, l)
				}
			}
			r, at := routeFor(t, chunks)
			if at == nil {
				t.Fatalf("no address route (%v): the device would show none for a card Rust derives", r)
			}
			for chain, want := range map[string][2]string{"0": tc.receive, "1": tc.change} {
				for i := 0; i < 2; i++ {
					if tc.vector != "" {
						if v := vectorAddress(t, tc.vector, chain, i); v != want[i] {
							t.Fatalf("INCONCLUSIVE: Rust's legacy-card address %s differs from vector %s's %s", want[i], tc.vector, v)
						}
					}
					got, err := at(uint32(i), chain == "1")
					if err != nil {
						t.Fatalf("chain %s index %d (%v route): %v", chain, i, r, err)
					}
					if got != want[i] {
						t.Errorf("chain %s index %d (%v route):\n device %s\n rust   %s", chain, i, r, got, want[i])
					}
				}
			}
		})
	}
}
