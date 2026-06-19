package mk

import (
	"errors"
	"testing"
)

func TestParseHeader(t *testing.T) {
	// V1 chunk 0 (chunked, index 0 of 2) and chunk 1 (index 1 of 2).
	const c0 = "mk1qpzg69pqqsq3zg3ngj4thnxaq5zg3vs7zqsrqqdt4w46h2at4w46h2at4w46h2at4w46h2at4w46h2at4w46h2at4vp3kx98j76m4mjlwphf"
	const c1 = "mk1qpzg69ppsnz4v7cjv3qfjhf76k4t5pt96u0psdrqfqvll8qh7h5athg837pmkf3dpug2mmjtfel6x"
	h0, err := ParseHeader(c0)
	if err != nil {
		t.Fatalf("ParseHeader(c0): %v", err)
	}
	if !h0.Chunked || h0.TotalChunks != 2 || h0.ChunkIndex != 0 {
		t.Fatalf("c0 header = %+v; want chunked total=2 index=0", h0)
	}
	h1, err := ParseHeader(c1)
	if err != nil {
		t.Fatalf("ParseHeader(c1): %v", err)
	}
	// R0-C1 guard: chunk_index is 0-based verbatim (NOT value-1) — chunk 1 is index 1.
	if !h1.Chunked || h1.TotalChunks != 2 || h1.ChunkIndex != 1 {
		t.Fatalf("c1 header = %+v; want chunked total=2 index=1", h1)
	}
	// Both chunks share chunk_set_id.
	if h0.ChunkSetID != h1.ChunkSetID {
		t.Fatalf("chunk_set_id mismatch: %d vs %d", h0.ChunkSetID, h1.ChunkSetID)
	}
}

func TestFiveBitToBytes(t *testing.T) {
	// 8 zero symbols = 40 bits = 5 bytes, zero padding → ok.
	out, err := fiveBitToBytes([]byte{0, 0, 0, 0, 0, 0, 0, 0})
	if err != nil || len(out) != 5 {
		t.Fatalf("zero pad: out=%v err=%v", out, err)
	}
	// A symbol >= 32 → reject.
	if _, err := fiveBitToBytes([]byte{0, 32}); !errors.Is(err, errMalformedPadding) {
		t.Fatalf("symbol>=32: want errMalformedPadding, got %v", err)
	}
	// Non-zero trailing pad bits → reject (one symbol = 5 bits, all leftover, value 1).
	if _, err := fiveBitToBytes([]byte{1}); !errors.Is(err, errMalformedPadding) {
		t.Fatalf("nonzero pad: want errMalformedPadding, got %v", err)
	}
}

type vec struct {
	name    string
	strings []string
	network string
	path    string
	fp      string
	stubs   []string
	xpub    string
}

var parityVectors = []vec{
	{
		name: "V1_bip48_mainnet_1_stub_with_fp",
		strings: []string{
			"mk1qpzg69pqqsq3zg3ngj4thnxaq5zg3vs7zqsrqqdt4w46h2at4w46h2at4w46h2at4w46h2at4w46h2at4w46h2at4vp3kx98j76m4mjlwphf",
			"mk1qpzg69ppsnz4v7cjv3qfjhf76k4t5pt96u0psdrqfqvll8qh7h5athg837pmkf3dpug2mmjtfel6x",
		},
		network: "mainnet", path: "m/48'/0'/0'/2'", fp: "aabbccdd", stubs: []string{"11223344"},
		xpub: "xpub6Den8YwXbKQvkwukmx7Uukicw4qDgMEPuuUkhMp3Rn557YSN2uVQnCMQNSfgDtennU9nES3Wbbmz1LAPBydhNpED8NU4mf1SFF41hM7vFrc",
	},
	{
		name: "V2_bip84_mainnet_1_stub_with_fp",
		strings: []string{
			"mk1qpydzkpqqsqupllwqr02m0h0qvzg3vs7zqsrqq4g4z52329g4z52329g4z52329g4z52329g4z52329g4z52329g4qpy6m8lr3sdrxkguwax",
			"mk1qpydzkppfdkdzdssxt9fh54wh8vsp2jdghv74kq2e9prxaxy2xnj2ng8vm68nf54c0vrdlfrgjzpd",
		},
		network: "mainnet", path: "m/84'/0'/0'", fp: "deadbeef", stubs: []string{"c0ffee00"},
		xpub: "xpub6BmeGmRo4LosAcU21HDaGcvtaQ7GrqQcY48nBkE22qM6KVwQUjRJ1BGzk84SFVHgLcd61Vcnhr8petHexjjn5WbQ9PriVrRhphw4oCp2z6a",
	},
	{
		name: "V3_bip48_testnet_1_stub_with_fp",
		strings: []string{
			"mk1qpx3t8pqqsqh0zye4ggzqvzqz5zrtp70zqsrqqaf4x56n2df4x56n2df4x56n2df4x56n2df4x56n2df4x56n2df4yp9xx3y0h0ccw664dfd",
			"mk1qpx3t8pprlnqdqf52q7jwgcnxgnuseav37nvs0zn06dyfs79hk7uk8lrxlyw57x7v7rzx74tlflqh",
		},
		network: "testnet", path: "m/48'/1'/0'/2'", fp: "10203040", stubs: []string{"778899aa"},
		xpub: "tpubDE2QenmnfFWFjr6TXWBdoZken4gKkeo3W3iCQjW64pqrtbVAP9DDmGhMRnnwwtgey511kwptHzGF5JKrrHzJJWB3ZAy4AYubz369CSz2dhS",
	},
	{
		name: "V4_bip84_mainnet_1_stub_no_fp",
		strings: []string{
			"mk1qpg4ncpqqqq6hn00qypsfz9jrcgzqvqy46h2at4w46h2at4w46h2at4w46h2at4w46h2at4w46h2at4w46hqx3380xk55vxz9s95rk7jsdyt",
			"mk1qpg4ncpp45u4z3s5w5d8zzzl9ugwr3a9j0jwqv80kku8y889tv9uttaemyjd5u8sp67lj8p",
		},
		network: "mainnet", path: "m/84'/0'/0'", fp: "", stubs: []string{"abcdef01"},
		xpub: "xpub6BmeGmSNQzwjso6raQ8ea1aioo7PfaivP5sPryaBZT57AjX3eYRGTyc2T8stCLcQKnA4Pw3a5FA5iChz37gUuJbo5cwqvXdNebE5WBfWeHx",
	},
	{
		name: "V5_explicit_path_4_components_with_fp",
		strings: []string{
			"mk1qp2eufzqqsq42enh3qqsyqcylczgln5qsqyd9zvqsqyt3qyqsqyg0qyqsqyqfz9jrcgzqvq947h6lta047h6lta047h67xj4jt7g69atcpze",
			"mk1qp2eufzp47h6lta047h6lta047h6lta047h6ltcrvtq2q3k6en5xmhgrg0rd8378ns3q3wsdnjw0yjndq3kjr5sljrm3ydu6j4m83w45h234",
			"mk1qp2eufzzrscsjqdk69lrveg2fm",
		},
		network: "mainnet", path: "m/9999'/1234'/56'/7'", fp: "01020304", stubs: []string{"55667788"},
		xpub: "xpub6Den8YxgJdggPygKKEv3wiQwQ6PSGUouW98xC4obAJAqvuWcBMHuxeuXHxyZtAJHLqE7U1JdEXrNwbNPNCn1F79n4ZuBTLnzF7mPbLR3ZvB",
	},
	{
		name: "V6_3_stubs_mainnet_with_fp",
		strings: []string{
			"mk1qpv7yspqqspaatgqq8026qqzm6ksqqlsph90upgy3zepuypqxqr2et9v4jk2et9v4jk2et9v4jk2et9v4jk2et9v4jk2cfr7h56h70u9lsha",
			"mk1qpv7yspp4jk2et9v4splqp4p34t9838d75u3lu36v8crl7paydlgsrhxzxrl48ehngpguzk8j6a47h024849cnxk4n",
		},
		network: "mainnet", path: "m/48'/0'/0'/2'", fp: "f00dcafe", stubs: []string{"dead0001", "dead0002", "dead0003"},
		xpub: "xpub6Den8YxxyxkcXmP7ygCeb7Bf1Ptqw1aQNa9iaigk6EPeoZHkeHmequH8aYiT3mUALmPo7ThDTZJf5cu5eziSYeW4fsbfdFubwdBgRetAhFa",
	},
	{
		name: "V7_max_path_components_no_fp",
		strings: []string{
			"mk1qp0zgpzqqqqepyvjj0lq4qyqszqq3qvqszqq3q5qszqq3quqszqq3pyqszqq3pvqszqq3p5qszqq3puqszqq3zyqszqqse9ppcgqls67s8nv",
			"mk1qp0zgpzp3xqgpqqgqjyty8ssyqcq0tdd4kk6mtdd4kk6mtdd4kk6mtdd4kk6mtdd4kk6mtdd4kk6mtddq2vfczmkedtrj2rjl6la2h9ek48q",
			"mk1qp0zgpzzw87un0hnrmqxcdtq7vjf6mhfuhvrc4mz2ktwqhm0qwv5qvsnckdz0yclv6ky",
		},
		network: "mainnet", path: "m/0'/1'/2'/3'/4'/5'/6'/7'/8'/9'", fp: "", stubs: []string{"90919293"},
		xpub: "xpub6QwbHG5Nw7rYLo6utUHsXUqaaojc3YDdq84Ho7HV3mHuiJ1NNXB1GzUdBCMVph1HfRMMuRjW2VVVr8k5Fz7YGrKVGwVYPBcXr6dZKQenNqk",
	},
}

func TestDecodeParity(t *testing.T) {
	for _, v := range parityVectors {
		t.Run(v.name, func(t *testing.T) {
			card, err := Decode(v.strings)
			if err != nil {
				t.Fatalf("Decode: %v", err)
			}
			if card.Network != v.network {
				t.Errorf("network = %q, want %q", card.Network, v.network)
			}
			if card.Path != v.path {
				t.Errorf("path = %q, want %q", card.Path, v.path)
			}
			if card.Fingerprint != v.fp {
				t.Errorf("fp = %q, want %q", card.Fingerprint, v.fp)
			}
			if card.Xpub != v.xpub {
				t.Errorf("xpub = %q, want %q", card.Xpub, v.xpub)
			}
			if len(card.Stubs) != len(v.stubs) {
				t.Fatalf("stub count = %d, want %d", len(card.Stubs), len(v.stubs))
			}
			for i, want := range v.stubs {
				if got := hexStub(card.Stubs[i]); got != want {
					t.Errorf("stub %d = %s, want %s", i, got, want)
				}
			}
		})
	}
}

func hexStub(b [4]byte) string {
	const hexdig = "0123456789abcdef"
	out := make([]byte, 8)
	for i, c := range b {
		out[i*2] = hexdig[c>>4]
		out[i*2+1] = hexdig[c&0xf]
	}
	return string(out)
}

func TestDecodeReassemblyOrderIndependent(t *testing.T) {
	v := parityVectors[0]
	rev := []string{v.strings[1], v.strings[0]} // reversed chunk order
	card, err := Decode(rev)
	if err != nil || card.Xpub != v.xpub {
		t.Fatalf("reversed-order Decode: xpub=%q err=%v", card.Xpub, err)
	}
}

func TestDecodeNegative(t *testing.T) {
	cases := []struct {
		name    string
		strings []string
	}{
		// Corpus schema-2 reject vectors (assert rejection, not error-string equality).
		{"N5_bch_uncorrectable", []string{
			"mk1qpzg69pqpqpqql46hm02m0h0qvzg3vs7zqsrplj52329g4z52329g4z52329g4z52329g4z52329g4z52329g4z52spqcw0rafrc8fnsh6sz",
			"mk1qpzg69ppu3e2uhvfj0nkp8hyauemx38khpye5yjexa9a7550sgjqnpdlq0y74taw9wyd9vvg6cecl",
		}},
		{"N6_unsupported_card_type", []string{"mk1qzqqqqqqqqqqqqqvy5namurdhk04"}},
		{"N7_malformed_padding", []string{"mk1qqqqr396edwcs33vch"}},
		{"N11_cross_chunk_hash_mismatch", []string{
			"mk1qpzg69pqqsqu4l46hm02m0h0qvzg3vs7zqsrplj52329g4z52329g4z52329g4z52329g4z52329g4z52329g4z52spqcw0rafrc8fnsh6sz",
			"mk1qpzg69ppu3e2uhvfj0nkp8hyauemx38khpye5yjexa9a7550sgjqnpdlq0y74t63da7ac22u7at6k",
		}},
		{"N15_invalid_path_indicator", []string{
			"mk1qpzg69pqqqqu4l46hcqqfz9jrcgzqv872329g4z52329g4z52329g4z52329g4z52329g4z52329g4z5232qyr8yw2h96fyy7xfz6vg5y8j6",
			"mk1qpzg69pp3xf7wcy7unhn8v6y76uynxsjtym5hh6j37pzgzv9hupk53wd0sv3njltfwe4x4g",
		}},
		// Constructed cases.
		{"empty_input", nil},
		{"count_below_total", []string{parityVectors[0].strings[0]}}, // 1 of 2
		{"duplicate_index", []string{parityVectors[0].strings[0], parityVectors[0].strings[0]}},
		{"mixed_chunk_sets", []string{parityVectors[0].strings[0], parityVectors[2].strings[1]}}, // V1 idx0 + V3 idx1
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			card, err := Decode(c.strings)
			if err == nil {
				t.Fatalf("Decode(%s): want error, got Card %+v", c.name, card)
			}
			// Card has a slice field ([][4]byte) → not comparable; check fields.
			if card.Network != "" || card.Path != "" || card.Fingerprint != "" || card.Xpub != "" || len(card.Stubs) != 0 {
				t.Fatalf("Decode(%s): want zero Card on error, got %+v", c.name, card)
			}
		})
	}
}
