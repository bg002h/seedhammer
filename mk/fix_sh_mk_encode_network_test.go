package mk

import (
	"errors"
	"testing"

	"github.com/btcsuite/btcd/chaincfg/v2"
)

// FIX-sh-mk-encode-network (bh-sh-I10, ruling T36): the network comes from the
// xpub version; a non-empty Card.Network that disagrees is refused.

func fixNetCard(network, path string, xpub string) Card {
	return Card{Network: network, Path: path, Fingerprint: "73c5da0a", Stubs: [][4]byte{{1, 2, 3, 4}}, Xpub: xpub}
}

// R1: "mainnet" with a tpub is refused.
func TestMkEncodeNetworkMainnetLabelTpubRefused(t *testing.T) {
	tpub := deriveTestXpub(t, "m/84'/1'/0'", &chaincfg.TestNet3Params)
	strs, err := Encode(fixNetCard("mainnet", "m/84'/1'/0'", tpub))
	if !errors.Is(err, errEncodeNetwork) {
		got, derr := Decode(strs)
		t.Fatalf("Encode(mainnet, tpub): err=%v, want errEncodeNetwork; Decode -> Network=%q err=%v", err, got.Network, derr)
	}
	if strs != nil {
		t.Fatalf("Encode(mainnet, tpub) returned strings with the error: %q", strs)
	}
}

// R1: "testnet" with an xpub is refused.
func TestMkEncodeNetworkTestnetLabelXpubRefused(t *testing.T) {
	xpub := deriveTestXpub(t, "m/84'/0'/0'", &chaincfg.MainNetParams)
	strs, err := Encode(fixNetCard("testnet", "m/84'/0'/0'", xpub))
	if !errors.Is(err, errEncodeNetwork) {
		got, derr := Decode(strs)
		t.Fatalf("Encode(testnet, xpub): err=%v, want errEncodeNetwork; Decode -> Network=%q err=%v", err, got.Network, derr)
	}
}

// R2: a label outside the field's vocabulary never agrees (exact match; no
// case folding, no trimming, no signet/regtest aliases for a tpub).
func TestMkEncodeNetworkOffVocabularyRefused(t *testing.T) {
	xpub := deriveTestXpub(t, "m/84'/0'/0'", &chaincfg.MainNetParams)
	tpub := deriveTestXpub(t, "m/84'/1'/0'", &chaincfg.SigNetParams)
	cases := []Card{
		fixNetCard("Mainnet", "m/84'/0'/0'", xpub),
		fixNetCard(" mainnet", "m/84'/0'/0'", xpub),
		fixNetCard("bitcoin", "m/84'/0'/0'", xpub),
		fixNetCard("signet", "m/84'/1'/0'", tpub),
		fixNetCard("regtest", "m/84'/1'/0'", tpub),
		fixNetCard("TESTNET", "m/84'/1'/0'", tpub),
	}
	for _, c := range cases {
		if _, err := Encode(c); !errors.Is(err, errEncodeNetwork) {
			t.Errorf("Encode(Network=%q): err=%v, want errEncodeNetwork", c.Network, err)
		}
	}
}

// R3: an empty Network is accepted and the card decodes with the xpub's network.
func TestMkEncodeNetworkEmptyTakesXpubVersion(t *testing.T) {
	xpub := deriveTestXpub(t, "m/84'/0'/0'", &chaincfg.MainNetParams)
	tpub := deriveTestXpub(t, "m/84'/1'/0'", &chaincfg.TestNet3Params)
	for _, c := range []struct {
		card Card
		want string
	}{
		{fixNetCard("", "m/84'/0'/0'", xpub), "mainnet"},
		{fixNetCard("", "m/84'/1'/0'", tpub), "testnet"},
	} {
		strs, err := Encode(c.card)
		if err != nil {
			t.Fatalf("Encode(empty Network, %s): %v", c.want, err)
		}
		got, err := Decode(strs)
		if err != nil || got.Network != c.want {
			t.Fatalf("Decode: Network=%q err=%v, want %q", got.Network, err, c.want)
		}
	}
}

// R3: a matching Network round-trips unchanged; a signet-derived key carries
// the testnet version, so "testnet" is its matching name.
func TestMkEncodeNetworkMatchingRoundTrips(t *testing.T) {
	xpub := deriveTestXpub(t, "m/84'/0'/0'", &chaincfg.MainNetParams)
	tpub := deriveTestXpub(t, "m/84'/1'/0'", &chaincfg.TestNet3Params)
	spub := deriveTestXpub(t, "m/84'/1'/1'", &chaincfg.SigNetParams)
	for _, card := range []Card{
		fixNetCard("mainnet", "m/84'/0'/0'", xpub),
		fixNetCard("testnet", "m/84'/1'/0'", tpub),
		fixNetCard("testnet", "m/84'/1'/1'", spub),
	} {
		strs, err := Encode(card)
		if err != nil {
			t.Fatalf("Encode(%s): %v", card.Network, err)
		}
		got, err := Decode(strs)
		if err != nil || got.Network != card.Network || got.Xpub != card.Xpub {
			t.Fatalf("round trip %s: got %+v err=%v", card.Network, got, err)
		}
	}
}

// R4: DerivedChunkSetID shares the builder, so it refuses the same card.
func TestMkDerivedChunkSetIDNetworkMismatchRefused(t *testing.T) {
	tpub := deriveTestXpub(t, "m/84'/1'/0'", &chaincfg.TestNet3Params)
	id, err := DerivedChunkSetID(fixNetCard("mainnet", "m/84'/1'/0'", tpub))
	if !errors.Is(err, errEncodeNetwork) || id != 0 {
		t.Fatalf("DerivedChunkSetID(mainnet, tpub) = %#x, %v; want 0, errEncodeNetwork", id, err)
	}
}

// R5: order. A bad xpub stays errEncodeXpub (no network can be read from it);
// a mismatch is reported before a bad fingerprint.
func TestMkEncodeNetworkCheckOrder(t *testing.T) {
	if _, err := Encode(fixNetCard("testnet", "m/84'/0'/0'", "xpubNOTAKEY")); !errors.Is(err, errEncodeXpub) {
		t.Errorf("bad xpub + label: err=%v, want errEncodeXpub", err)
	}
	tpub := deriveTestXpub(t, "m/84'/1'/0'", &chaincfg.TestNet3Params)
	c := fixNetCard("mainnet", "m/84'/1'/0'", tpub)
	c.Fingerprint = "zz"
	if _, err := Encode(c); !errors.Is(err, errEncodeNetwork) {
		t.Errorf("mismatch + bad fingerprint: err=%v, want errEncodeNetwork", err)
	}
}

// R6: the error text.
func TestMkEncodeNetworkErrorText(t *testing.T) {
	const want = "mk: network does not match the xpub version"
	if errEncodeNetwork.Error() != want {
		t.Fatalf("errEncodeNetwork = %q, want %q", errEncodeNetwork.Error(), want)
	}
}

// R7: an unsupported public version is still refused at Encode, and Decode keeps its text.
func TestMkEncodeNetworkUnsupportedVersionRefused(t *testing.T) {
	p := chaincfg.MainNetParams
	p.Name, p.Net = "probeynet", 0x1234abcd
	p.HDPrivateKeyID = [4]byte{0x04, 0x9d, 0x78, 0x78}
	p.HDPublicKeyID = [4]byte{0x04, 0x9d, 0x7c, 0xb2}
	if err := chaincfg.Register(&p); err != nil {
		t.Fatal(err)
	}
	ypub := deriveTestXpub(t, "m/84'/0'/0'", &p)
	for _, n := range []string{"", "mainnet"} {
		if _, err := Encode(fixNetCard(n, "m/84'/0'/0'", ypub)); !errors.Is(err, errEncodeXpub) {
			t.Errorf("Network=%q ypub: err=%v, want errEncodeXpub", n, err)
		}
	}
}

func TestMkDecodeVersionErrorText(t *testing.T) {
	_, _, err := reconstructXpub(make([]byte, xpubCompactBytes), nil)
	if err == nil || err.Error() != "mk: invalid xpub version: 00000000" {
		t.Fatalf("got %v", err)
	}
}
