package seal

import (
	"strings"
	"testing"
)

// FIX-sh-bip39-admission (bh-sh-I1, I2; T35), design/FIX_sh_bip39_admission.md.
// R5: a record that is an ambiguous abbreviation or a 3-, 6- or 9-word
// checksum-passing fragment is not classified as a BIP-39 mnemonic.
func TestFixShBip39ClassifyRefusesAmbiguousAndShort(t *testing.T) {
	const tail = " abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon bargain"
	if c := Classify([]byte("absent" + tail)); c != ClassMnemonic {
		t.Fatalf("control: Classify(absent...) = %v", c)
	}
	for _, rec := range []string{
		"abs" + tail,
		"abandon abandon ability", // checksum passes (triage-sh-t1)
		"abandon abandon abandon abandon abandon able",   // 6 words, checksum passes
		strings.TrimSpace(strings.Repeat("abandon ", 9)), // 9 words, checksum passes
	} {
		if c := Classify([]byte(rec)); c != ClassUnknown {
			t.Errorf("Classify(%q) = %v; want %v", rec, c, ClassUnknown)
		}
	}
}
