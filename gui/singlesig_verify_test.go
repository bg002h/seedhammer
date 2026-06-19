package gui

import (
	"strings"
	"testing"

	"github.com/btcsuite/btcd/chaincfg/v2"
	"seedhammer.com/bundle"
	"seedhammer.com/md"
)

// readbackCards builds a gathered card set (mk1 + md1) from a derived bundle, the
// shape the T5 bundleGatherer yields for the verify read-back (R0-C1).
func readbackCards(b bundle.Bundle) []bundleCard {
	return []bundleCard{
		{kind: cardMK1, label: "mk1 key", strings: append([]string(nil), b.MK1...)},
		{kind: cardMD1, label: "md1 descriptor", strings: append([]string(nil), b.MD1...)},
	}
}

// TestSingleSigVerifyReadbackExtractsCards (R0-C1): the read-back helper pulls
// the mk1 + md1 verbatim chunk strings from the gathered cards (via the T5
// bundleGatherer's bundleCard.strings — NOT mk1GatherFlow/.collected()).
func TestSingleSigVerifyReadbackExtractsCards(t *testing.T) {
	m := abandonAboutMnemonic()
	b, _, _, _, err := deriveSingleSigBundle(m, "", &chaincfg.MainNetParams, singleSigPath(84), md.ScriptWpkh)
	if err != nil {
		t.Fatalf("derive: %v", err)
	}
	mk1, md1, ok := singleSigReadbackCards(readbackCards(b))
	if !ok {
		t.Fatal("read-back did not yield exactly one mk1 and one md1")
	}
	if !equalStringSlice(mk1, b.MK1) {
		t.Fatalf("read-back mk1 not verbatim: %v vs %v", mk1, b.MK1)
	}
	if !equalStringSlice(md1, b.MD1) {
		t.Fatalf("read-back md1 not verbatim: %v vs %v", md1, b.MD1)
	}
}

// TestSingleSigVerifyReadbackRejectsIncomplete: a read-back missing the mk1 (or
// md1) card is not a complete bundle → ok=false.
func TestSingleSigVerifyReadbackRejectsIncomplete(t *testing.T) {
	m := abandonAboutMnemonic()
	b, _, _, _, _ := deriveSingleSigBundle(m, "", &chaincfg.MainNetParams, singleSigPath(84), md.ScriptWpkh)
	// Only an md1, no mk1.
	if _, _, ok := singleSigReadbackCards([]bundleCard{{kind: cardMD1, strings: b.MD1}}); ok {
		t.Fatal("read-back with no mk1 must be incomplete")
	}
	// Only an mk1, no md1.
	if _, _, ok := singleSigReadbackCards([]bundleCard{{kind: cardMK1, strings: b.MK1}}); ok {
		t.Fatal("read-back with no md1 must be incomplete")
	}
}

// TestSingleSigVerifyFullMatch: a correct full read-back (ms1 hand-typed + mk1/md1
// read back) → PASS.
func TestSingleSigVerifyFullMatch(t *testing.T) {
	m := abandonAboutMnemonic()
	b, _, _, _, err := deriveSingleSigBundle(m, "", &chaincfg.MainNetParams, singleSigPath(84), md.ScriptWpkh)
	if err != nil {
		t.Fatalf("derive: %v", err)
	}
	if err := verifySingleSig(b, b.MS1, b.MK1, b.MD1); err != nil {
		t.Fatalf("correct full read-back: %v (want PASS)", err)
	}
}

// TestSingleSigVerifyFullMutated: a mutated read-back (mk1/md1 from a DIFFERENT
// policy) → FAIL.
func TestSingleSigVerifyFullMutated(t *testing.T) {
	m := abandonAboutMnemonic()
	derived, _, _, _, _ := deriveSingleSigBundle(m, "", &chaincfg.MainNetParams, singleSigPath(84), md.ScriptWpkh)
	other, _, _, _, _ := deriveSingleSigBundle(m, "", &chaincfg.MainNetParams, singleSigPath(44), md.ScriptPkh)
	if err := verifySingleSig(derived, derived.MS1, other.MK1, other.MD1); err == nil {
		t.Fatal("mutated read-back (different policy) accepted, want FAIL")
	}
}

// TestSingleSigVerifyWatchOnly: a watch-only verify omits the ms1 (empty on both
// sides) → the mk1/md1/stub legs run and PASS; a mutated watch-only read-back
// FAILs.
func TestSingleSigVerifyWatchOnly(t *testing.T) {
	m := abandonAboutMnemonic()
	derived, _, _, _, err := deriveSingleSigBundle(m, "", &chaincfg.MainNetParams, singleSigPath(84), md.ScriptWpkh)
	if err != nil {
		t.Fatalf("derive: %v", err)
	}
	// Watch-only verify: ms1 empty (the derived side's ms1 must be dropped too).
	if err := verifySingleSig(derived, "", derived.MK1, derived.MD1); err != nil {
		t.Fatalf("watch-only correct read-back: %v (want PASS)", err)
	}
	other, _, _, _, _ := deriveSingleSigBundle(m, "", &chaincfg.MainNetParams, singleSigPath(86), md.ScriptTr)
	if err := verifySingleSig(derived, "", other.MK1, other.MD1); err == nil {
		t.Fatal("watch-only mutated read-back accepted, want FAIL")
	}
}

// TestSingleSigVerifyErrorNamesField: a FAIL surfaces an error naming the
// diverging field (for the operator-facing result screen).
func TestSingleSigVerifyErrorNamesField(t *testing.T) {
	m := abandonAboutMnemonic()
	derived, _, _, _, _ := deriveSingleSigBundle(m, "", &chaincfg.MainNetParams, singleSigPath(84), md.ScriptWpkh)
	other, _, _, _, _ := deriveSingleSigBundle(m, "", &chaincfg.MainNetParams, singleSigPath(44), md.ScriptPkh)
	err := verifySingleSig(derived, derived.MS1, other.MK1, other.MD1)
	if err == nil {
		t.Fatal("want FAIL")
	}
	low := strings.ToLower(err.Error())
	if !strings.Contains(low, "xpub") && !strings.Contains(low, "path") &&
		!strings.Contains(low, "md1") && !strings.Contains(low, "stub") &&
		!strings.Contains(low, "fingerprint") {
		t.Errorf("verify error %q names no recognizable field", err)
	}
}
