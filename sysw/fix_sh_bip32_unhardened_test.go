package sysw

import "testing"

// FIX-sh-bip32-unhardened (bh-sh-M6): at the payload door a descriptor record
// whose origin has an unmarked element of 2^31 or more is ClassUnknown (inert),
// not ClassDescriptor carried as the hardened origin it wraps to.
func TestFixShBip32UnhardenedPayloadUnknown(t *testing.T) {
	ok := "wpkh([d34db33f/84h/1h/0h]" + skTP + "/<0;1>/*)"
	if got := Classify(ok); got != ClassDescriptor {
		t.Fatalf("control Classify(%q) = %v, want ClassDescriptor", ok, got)
	}
	bad := "wpkh([d34db33f/84h/1h/2147483648]" + skTP + "/<0;1>/*)"
	if got := Classify(bad); got != ClassUnknown {
		t.Errorf("Classify(%q) = %v, want ClassUnknown", bad, got)
	}
}
