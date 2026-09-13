package gui

import (
	"testing"

	"seedhammer.com/md"
)

// TestPolicyAddressAtIsTheDeviceFunction pins the one property cmd/policyprobe
// rests on: PolicyAddressAt is policyAddressAt, not a second opinion about it.
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
// WHAT IT CANNOT SEE, stated plainly: a byte-for-byte copy of policyAddressAt
// would produce identical output and pass. No Go test can distinguish a call
// from an identical copy; that one is caught by reading the diff, which is why
// the wrapper's doc comment says the body must stay one call.
func TestPolicyAddressAtIsTheDeviceFunction(t *testing.T) {
	t.Run("derivable", func(t *testing.T) {
		chunks := loadVectorChunks(t, "keyed_compose_wsh_timelock_hashlock")
		tpl, keys, err := md.ExpandWalletPolicyChunks(chunks)
		if err != nil {
			t.Fatalf("expand: %v", err)
		}

		want, wok := policyAddressAt(chunks, tpl, keys)
		got, gok := PolicyAddressAt(chunks, tpl, keys)
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
		tpl, keys, err := md.ExpandWalletPolicyChunks(chunks)
		if err != nil {
			t.Fatalf("expand: %v", err)
		}

		_, wok := policyAddressAt(chunks, tpl, keys)
		_, gok := PolicyAddressAt(chunks, tpl, keys)
		if wok {
			t.Fatal("a keyless policy claimed an address source — fixture is wrong, not the wrapper")
		}
		if gok != wok {
			t.Fatalf("the wrapper answers %v where the device answers %v", gok, wok)
		}
	})
}

// TestPolicyAddressAtReachesTheFlatRoute is the regression that sent the probe
// back for repair: the harness wrapped complexAddressSource, which is only the
// SECOND of policyAddressAt's two routes, and so reported every flat shape as a
// device refusal.
//
// The fixture is chosen to separate the two routes rather than to exercise a
// feature: `keyed_wpkh` is a single-key policy that the flat
// *bip380.Descriptor route derives and complexAddressSource declines outright.
// That split is the whole assertion — the first check proves the fixture still
// has the property the test needs, so a corpus change that made
// complexAddressSource start accepting it would fail loudly here instead of
// quietly turning this test into a tautology.
//
// MUTATION: point PolicyAddressAt back at complexAddressSource and the second
// check fails with "the wrapper declined a policy the device derives".
func TestPolicyAddressAtReachesTheFlatRoute(t *testing.T) {
	chunks := loadVectorChunks(t, "keyed_wpkh")
	tpl, keys, err := md.ExpandWalletPolicyChunks(chunks)
	if err != nil {
		t.Fatalf("expand: %v", err)
	}

	if _, ok := complexAddressSource(chunks, keys); ok {
		t.Fatal("complexAddressSource now accepts keyed_wpkh: this fixture no longer separates the two routes, pick another")
	}
	at, ok := PolicyAddressAt(chunks, tpl, keys)
	if !ok {
		t.Fatal("the wrapper declined a policy the device derives: it is not reaching the flat route")
	}
	addr, err := at(0, false)
	if err != nil {
		t.Fatalf("receive[0]: %v", err)
	}
	if addr == "" {
		t.Fatal("the flat route returned an empty address")
	}
	t.Logf("keyed_wpkh receive[0] = %s", addr)
}
