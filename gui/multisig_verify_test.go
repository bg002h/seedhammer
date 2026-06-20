package gui

import (
	"strings"
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

// TestVerifyMultisigReadbackMk1 (T-H1, verify-cluster H1): route a readback
// []bundleCard through the PRODUCTION extractSuppliedMd1AndMk1 → verifyMultisig.
// On 3a23dbb the flow ignored the readback mk1 (passed reDerived.MK1 on both
// sides), so a WRONG engraved mk1 plate silently PASSED. This test routes the
// real readback mk1 and asserts:
//   - correct mk1 → PASS
//   - undecodable mutated mk1 → FAIL (mk1-decode leg)
//   - decodable-but-WRONG foreign mk1 (valid card, different policy) → FAIL
//     (stub-binding leg: "verify: readback mk1/md1 stub mismatch ...")
//   - masking proof: feeding reDerived.MK1 (today's self-compare) PASSES the
//     wrong-plate case — the discrimination the production flow lacked.
func TestVerifyMultisigReadbackMk1(t *testing.T) {
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

	// A foreign-but-VALID operator mk1: a different single-sig wallet's mk1 from
	// the SAME seed — it decodes fine (real card) but binds to a different policy
	// stub, so the stub-binding leg must reject it.
	foreign, _, _, _, err := deriveSingleSigBundle(m, "", &chaincfg.MainNetParams, singleSigPath(44), md.ScriptPkh)
	if err != nil {
		t.Fatalf("derive foreign mk1: %v", err)
	}

	t.Run("correct readback mk1 → PASS", func(t *testing.T) {
		cards := []bundleCard{
			{kind: cardMK1, strings: append([]string(nil), derived.MK1...)},
			{kind: cardMD1, strings: append([]string(nil), derived.MD1...)},
		}
		md1RB, mk1RB, ok := extractSuppliedMd1AndMk1(cards)
		if !ok {
			t.Fatal("helper rejected a valid mk1+md1 card set")
		}
		if err := verifyMultisig(derived, derived.MS1, mk1RB, md1RB); err != nil {
			t.Fatalf("correct readback: %v (want PASS)", err)
		}
	})

	t.Run("undecodable mutated mk1 → FAIL", func(t *testing.T) {
		mutated := append([]string(nil), derived.MK1...)
		mutated[len(mutated)-1] = "mk1tampered000000000000000000000000000000000000"
		cards := []bundleCard{
			{kind: cardMK1, strings: mutated},
			{kind: cardMD1, strings: append([]string(nil), derived.MD1...)},
		}
		md1RB, mk1RB, ok := extractSuppliedMd1AndMk1(cards)
		if !ok {
			t.Fatal("helper rejected mutated mk1 card set")
		}
		if err := verifyMultisig(derived, derived.MS1, mk1RB, md1RB); err == nil {
			t.Fatal("undecodable mk1 accepted, want FAIL")
		}
	})

	t.Run("decodable-but-wrong foreign mk1 → FAIL via stub binding", func(t *testing.T) {
		cards := []bundleCard{
			{kind: cardMK1, strings: append([]string(nil), foreign.MK1...)},
			{kind: cardMD1, strings: append([]string(nil), derived.MD1...)},
		}
		md1RB, mk1RB, ok := extractSuppliedMd1AndMk1(cards)
		if !ok {
			t.Fatal("helper rejected foreign mk1 card set")
		}
		err := verifyMultisig(derived, derived.MS1, mk1RB, md1RB)
		if err == nil {
			t.Fatal("decodable-but-wrong foreign mk1 accepted, want FAIL")
		}
		if !strings.Contains(err.Error(), "stub mismatch") {
			t.Errorf("foreign mk1 error %q does not name stub mismatch", err)
		}
	})

	t.Run("masking proof: self-compare PASSES the foreign mk1 (the bug)", func(t *testing.T) {
		// Today's flow passed reDerived.MK1 on the readback side, so the engraved
		// (here: foreign) plate was never compared — it PASSES. This is the bug
		// the production fix (routing the real readback mk1) closes.
		if err := verifyMultisig(derived, derived.MS1, derived.MK1, derived.MD1); err != nil {
			t.Fatalf("self-compare baseline: %v (want PASS, demonstrating the masked bug)", err)
		}
	})
}

// TestMultisigVerifyNoticeIsHonest (L2): the multisig success notice must scope
// its guarantee honestly — "operator key + secret verified; other cosigners'
// keys taken as supplied" — and must NOT carry the bare full-bundle over-claim
// ("the engraved bundle matches the seed"). We drive showNotice directly with the
// production copy and assert the rendered text via uiContains (which strips
// spaces). This guards against the over-claim silently returning.
func TestMultisigVerifyNoticeIsHonest(t *testing.T) {
	ctx := NewContext(newPlatform())
	frame, quit := runUI(ctx, func() {
		showNotice(ctx, &descriptorTheme, multisigVerifyOKTitle, multisigVerifyOKBody)
	})
	defer quit()
	content, ok := frame()
	if !ok {
		t.Fatal("no frame from showNotice")
	}
	if !uiContains(content, "taken as supplied") {
		t.Errorf("notice lacks the scoped wording; got %q", content)
	}
	if uiContains(content, "matches the seed") {
		t.Errorf("notice still carries the over-claim; got %q", content)
	}
}
