package nonstandard

import (
	"strings"
	"testing"
)

// Bug 1 (BlueWallet half): a BlueWallet key must re-encode to the xpub it was
// read from, whatever Derivation header precedes the key lines.
func TestBughuntSh2_01_BlueWalletRoundTrip(t *testing.T) {
	const x = "xpub6F148LnjUhGrHfEN6Pa8VkwF8L6FJqYALxAkuHfacfVhMLVY4MRuUVMxr9pguAv67DHx1YFxqoKN8s4QfZtD9sR2xRCffTqi9E8FiFLAYk8"
	for _, deriv := range []string{"", "Derivation: m/48'/0'/0'\n", "Derivation: m/48'/0'/0'/2'\n"} {
		d, err := OutputDescriptor([]byte("Name: sh\nPolicy: 1 of 1\n" + deriv + "Format: P2WSH\n\n5A0804E3: " + x))
		if err != nil {
			t.Errorf("fix-01: BlueWallet file with %q refused: %v", deriv, err)
			continue
		}
		if got := d.Keys[0].String(); !strings.Contains(got, x) {
			t.Errorf("fix-01: BlueWallet key with %q re-encodes as %s, want %s", deriv, got, x)
		}
	}
}
