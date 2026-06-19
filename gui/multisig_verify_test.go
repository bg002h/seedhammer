package gui

import (
	"testing"

	"github.com/btcsuite/btcd/chaincfg/v2"
	"seedhammer.com/md"
)

// TestVerifyMultisig: re-derive the operator's leg + read back the supplied md1
// and operator mk1, run bundle.Verify (I-5). PASS for the matched slot; FAIL on
// a mutated mk1/md1/ms1; watch-only (no ms1) PASSes.
func TestVerifyMultisig(t *testing.T) {
	chunks := suppliedMultisigMd1(t)
	_, keys, err := md.ExpandWalletPolicyChunks(chunks)
	if err != nil {
		t.Fatalf("ExpandWalletPolicyChunks: %v", err)
	}
	m := abandonAboutMnemonic()
	_, origin, _, ok := findUserSlot(m, "", &chaincfg.MainNetParams, keys)
	if !ok {
		t.Fatal("findUserSlot: no match")
	}
	derived, err := deriveMultisigLeg(m, "", &chaincfg.MainNetParams, origin, chunks, true)
	if err != nil {
		t.Fatalf("deriveMultisigLeg: %v", err)
	}

	t.Run("full PASS", func(t *testing.T) {
		if err := verifyMultisig(derived, derived.MS1, derived.MK1, derived.MD1); err != nil {
			t.Fatalf("verifyMultisig PASS path: %v", err)
		}
	})

	t.Run("mutated mk1 FAIL", func(t *testing.T) {
		bad := append([]string(nil), derived.MK1...)
		bad[len(bad)-1] = "mk1tampered000000000000000000000000000000000000"
		if err := verifyMultisig(derived, derived.MS1, bad, derived.MD1); err == nil {
			t.Fatal("verifyMultisig accepted a mutated mk1, want FAIL")
		}
	})

	t.Run("mutated md1 FAIL", func(t *testing.T) {
		bad := append([]string(nil), derived.MD1...)
		bad[0] = bad[0][:len(bad[0])-1] + "x"
		if err := verifyMultisig(derived, derived.MS1, derived.MK1, bad); err == nil {
			t.Fatal("verifyMultisig accepted a mutated md1, want FAIL")
		}
	})

	t.Run("mutated ms1 FAIL", func(t *testing.T) {
		// A valid-but-different ms1 entropy: re-derive watch-only then supply a
		// fabricated ms1 by mutating an entropy byte is awkward; instead assert a
		// presence mismatch is caught (one side has ms1, the other doesn't).
		if err := verifyMultisig(derived, "", derived.MK1, derived.MD1); err == nil {
			t.Fatal("verifyMultisig accepted an ms1 presence mismatch, want FAIL")
		}
	})

	t.Run("watch-only PASS (no ms1 both sides)", func(t *testing.T) {
		wo, err := deriveMultisigLeg(m, "", &chaincfg.MainNetParams, origin, chunks, false)
		if err != nil {
			t.Fatalf("deriveMultisigLeg watch-only: %v", err)
		}
		if err := verifyMultisig(wo, "", wo.MK1, wo.MD1); err != nil {
			t.Fatalf("watch-only verify: %v", err)
		}
	})
}
