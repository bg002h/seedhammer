package seal

import "testing"

// FIX-sh-bip32-unhardened (bh-sh-M6): a sealed record whose origin has an
// unmarked element of 2^31 or more classifies as ClassUnknown, not
// ClassDescriptor. Not reachable from a screen: AdmitSection's lowercase pass
// refuses any record carrying an extended key before it classifies.
const fixShBip32K = "tpubD6NzVbkrYhZ4Wgf68pWWfTRzdXMoepBvepfiAKtNoE7RvbAbWVfWzQxH1jbkfNk3iJ9zR65Yw6u3B2QZzpkMSTN4y8Lfm1t44HbpZX7efhZ"

func TestFixShBip32UnhardenedSealUnknown(t *testing.T) {
	ok := "wpkh([d34db33f/84h/1h/2147483647]" + fixShBip32K + "/0/*)"
	if got := Classify([]byte(ok)); got != ClassDescriptor {
		t.Fatalf("control Classify(%q) = %v, want ClassDescriptor", ok, got)
	}
	bad := "wpkh([d34db33f/84h/1h/2147483648]" + fixShBip32K + "/0/*)"
	if got := Classify([]byte(bad)); got != ClassUnknown {
		t.Errorf("Classify(%q) = %v, want ClassUnknown", bad, got)
	}
}
