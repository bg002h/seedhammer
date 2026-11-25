package gui

import (
	"errors"
	"strings"
	"testing"

	"seedhammer.com/address"
	"seedhammer.com/md"
)

// Bug hunt 2026-11-25 SH#6: a card whose keys have no multipath step (`@N/*`
// only) has no change chain. The Rust primary refuses chain 1 for it
// (md-codec 0.48.7 check_chain_in_range: `md address <card> --chain 1` ->
// "chain index 1 out of range; use-site multipath alt-count is 1"); this
// device derived the RECEIVE addresses again and titled them "Change
// addresses". Both routes now refuse with address.ErrNoChangeChain, and the
// receive chain still matches Rust.
//
// Cards minted with md encode (md 0.21.1, md-codec 0.48.7) from the keys of
// the corpus vectors keyed_wsh_or_b / keyed_tr_with_leaf; receive addresses
// are Rust's `md address <card> --count 2`.
func TestNoMultipathCardHasNoChangeChain(t *testing.T) {
	for _, tc := range []struct {
		name    string
		chunks  []string
		receive [2]string
	}{
		{"wsh(sortedmulti(1,@0/*,@1/*))", []string{
			"md1fsumkps9q2tvyyy5jmpprjszrsq59uyw0za5z4eutks2zhqjthgx8egtq4pcwl6u5p2us6t0e4w3t336sz7",
			"md1fsumkpsg0g2z0agd4urfpzanhaq3lcdlz64mrqgdrha0m7umapumfj075dhzfzvynh66nyaqlv4rd9uv3gm",
			"md1fsumkpskk2nlqmlvch5n4shwf72wvkp39rllcf36szwcer8h73mk4csl2yu529qqleaqkwxmzr2qzv74mfr",
			"md1fsumkpsape8wq0vdczfyy55tqsd5576trsa3p40nfpd7hsyjyf7vlx6hk2j6ckr4wf0m3s9gga8akzdzw69",
		}, [2]string{
			"bc1qmj9hd2t99ps58taef3dhl8snj25j9khrlnsnxx65pjzryudtm8yqr3ezme",
			"bc1qzhc2al5ww40n2twz2nkaw2v6ar7qlsfl28pegj8ncr28jhmxx46qlm2dz6",
		}},
		{"wsh(or_b(pk(@0/*),s:pk(@1/*)))", []string{
			"md1fmjr5ps9q2tvyyy5jmpprjsztzsu2shs3eutks2h83w6pg2uzfwaqcl9pvz58pmltjs9tjrg566ktzxh975t3",
			"md1fmjr5psg0g2z0agd4urfpzanhaq3lcdlz64mrqgdrha0m7umapumfj075dhzfzvynh66n94sucw9gdmu4dwvf",
			"md1fmjr5psj5lcxlmx9ayav9mj0jnn9svfgll7zvw5qnkxgeal5wa4wy86389z3gq870g9n6rjg50e48pjk5rhrn",
			"md1fmjr5ps7uq7cmsyjgffgkpqmffa5k8pmzr2lxjzma0qfygnue7d40v4943v82ujlhrqw884gfaq5fyds",
		}, [2]string{
			"bc1qpqw27jvp5xnjrc7qff5dt34xw96ncxljenpesz6c5r5zplpjzxcq5u4rfq",
			"bc1q8hkrvlfhemm2tf9snven80t24hsy0x46g4wl8m7waw5k598f6wesr9hfyj",
		}},
		{"tr(@0/*,pk(@1/*))", []string{
			"md1fs4wmps9q2tvyyy5jmpprjspy4p0prnchdq4w0za5zs4cyja6p372zc9gwrh7h9q2hyxs7ut2eh37l5t6sd",
			"md1fs4wmps2zsnl2rd0q6gghvalgy07r0ck4wcczrgalt7lhxlg0x6vnl4rdcjgnpya7k5edvsj72em88pl5hd",
			"md1fs4wmpsj5lcxlmx9ayav9mj0jnn9svfgll7zvw5qnkxgeal5wa4wy86389z3gq870g9n6zecfmlg2j48efx",
			"md1fs4wmps7fmsrmrwqjfp99zcyrd98kjcu8vgdtu6gt04upy3z0n8ek4aj5kk9satjt7uvqllwqcxa4u7lr9",
		}, [2]string{
			"bc1pw599hcpg5yuq6pzp4ty0tc67fcs0mdyldu3kxvm2jvj6lyxy565qc3q56t",
			"bc1pp9fw4pe0hektran993n0t67mg5x2j5ene33ju8zqu7605vu3qd2s4e6rws",
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, at := routeFor(t, tc.chunks)
			if at == nil {
				t.Fatalf("no address route (%v)", r)
			}
			for i, want := range tc.receive {
				got, err := at(uint32(i), false)
				if err != nil || got != want {
					t.Fatalf("receive %d (%v route) = (%s, %v), want Rust's %s", i, r, got, err, want)
				}
			}
			for i := uint32(0); i < 2; i++ {
				got, err := at(i, true)
				if !errors.Is(err, address.ErrNoChangeChain) {
					t.Fatalf("change %d (%v route) = (%s, %v), want address.ErrNoChangeChain "+
						"(Rust refuses chain 1 for a card no key of which has a multipath)", i, r, got, err)
				}
			}
			// The restore document names the absence instead of failing, and
			// never repeats the first receive address as the first change.
			tpl, keys, err := md.ExpandWalletPolicyChunks(tc.chunks)
			if err != nil {
				t.Fatal(err)
			}
			lines, hasAddr, err := multisigRestoreLines(tc.chunks, tpl, keys)
			if err != nil || !hasAddr {
				t.Fatalf("restore doc: hasAddr=%v err=%v", hasAddr, err)
			}
			joined := strings.Join(lines, "\n")
			if !strings.Contains(joined, "First change:\nnone (no change chain)") {
				t.Fatalf("restore doc does not say there is no change chain:\n%s", joined)
			}
			if strings.Count(joined, tc.receive[0]) != 1 {
				t.Fatalf("restore doc shows the first receive address %d times:\n%s", strings.Count(joined, tc.receive[0]), joined)
			}
		})
	}
}
