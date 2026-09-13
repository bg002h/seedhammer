package gui

import (
	"testing"

	"seedhammer.com/md"
)

// TestPolicyAddressSourceIsTheDeviceFunction pins the one property
// cmd/policyprobe rests on: PolicyAddressSource is complexAddressSource, not a
// second opinion about it.
//
// WHY THIS MATTERS MORE THAN IT LOOKS. The policy differential harness compares
// the device against the Rust primary and Bitcoin Core. If the exported wrapper
// ever grew a body of its own — a network argument, a pre-filter, a fallback
// when !ok — the harness would go on reporting three-way agreement while the
// screen an operator actually reads had drifted away from the thing under test.
// A silent pass is the failure mode, so the equivalence is asserted rather than
// assumed.
//
// It is checked on BOTH verdicts, because agreeing only where the device
// succeeds would miss a wrapper that turned a refusal into an answer — and a
// refusal is a result this harness counts, not an error it swallows.
//
// MUTATION: give the wrapper any body of its own — derive at index+1, swap the
// change flag, or route through a non-mainnet network — and the address
// comparison below fails on the first index.
//
// WHAT IT CANNOT SEE, stated plainly: a byte-for-byte copy of
// complexAddressSource would produce identical output and pass. No Go test can
// distinguish a call from an identical copy; that one is caught by reading the
// diff, which is why the wrapper's doc comment says the body must stay one call.
func TestPolicyAddressSourceIsTheDeviceFunction(t *testing.T) {
	t.Run("derivable", func(t *testing.T) {
		chunks := loadVectorChunks(t, "keyed_compose_wsh_timelock_hashlock")
		_, keys, err := md.ExpandWalletPolicyChunks(chunks)
		if err != nil {
			t.Fatalf("expand: %v", err)
		}

		want, wok := complexAddressSource(chunks, keys)
		got, gok := PolicyAddressSource(chunks, keys)
		if !wok {
			t.Fatal("the device refuses a policy its own composer builds — fixture is wrong, not the wrapper")
		}
		if gok != wok {
			t.Fatalf("wrapper ok=%v, device ok=%v", gok, wok)
		}
		for _, change := range []bool{false, true} {
			for index := uint32(0); index < 3; index++ {
				wantAddr, wantErr := want(index, change)
				gotAddr, gotErr := got(index, change)
				if gotAddr != wantAddr {
					t.Errorf("at(%d, %v)\n wrapper %s\n device  %s", index, change, gotAddr, wantAddr)
				}
				if (gotErr == nil) != (wantErr == nil) {
					t.Errorf("at(%d, %v) error: wrapper %v, device %v", index, change, gotErr, wantErr)
				}
			}
		}
	})

	t.Run("refused", func(t *testing.T) {
		chunks := loadVectorChunks(t, "keyless_tr_with_leaf") // no xpubs: nobody can derive it
		_, keys, err := md.ExpandWalletPolicyChunks(chunks)
		if err != nil {
			t.Fatalf("expand: %v", err)
		}

		_, wok := complexAddressSource(chunks, keys)
		_, gok := PolicyAddressSource(chunks, keys)
		if wok {
			t.Fatal("a keyless policy claimed an address source — fixture is wrong, not the wrapper")
		}
		if gok != wok {
			t.Fatalf("the wrapper answers %v where the device answers %v", gok, wok)
		}
	})
}
