package nonstandard

import (
	"fmt"
	"testing"
)

// FIX-sh-bip32-unhardened (bh-sh-M6): a BlueWallet "Derivation:" header with
// an unmarked element of 2^31 or more is refused, not read as hardened.
func TestFixShBip32UnhardenedBlueWallet(t *testing.T) {
	file := func(deriv string) []byte {
		return []byte(fmt.Sprintf("Name: t\nPolicy: 2 of 2\nDerivation: %s\nFormat: P2WSH\n\n%s\n\n%s\n", deriv, bhBWKey1, bhBWKey2))
	}
	if _, err := OutputDescriptor(file("m/48'/0'/0'/2147483647")); err != nil {
		t.Fatalf("control m/48'/0'/0'/2147483647 refused: %v", err)
	}
	for _, deriv := range []string{"m/48'/0'/0'/2147483648", "m/48'/0'/2147483648/2'"} {
		if d, err := OutputDescriptor(file(deriv)); err == nil {
			t.Errorf("Derivation %s accepted; origin %v", deriv, d.Keys[0].DerivationPath)
		}
	}
}
