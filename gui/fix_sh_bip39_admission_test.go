package gui

import (
	"errors"
	"strings"
	"testing"

	"seedhammer.com/bip39"
)

// FIX-sh-bip39-admission (bh-sh-I1, I2; T35), design/FIX_sh_bip39_admission.md.
// R5: the NFC scanner no longer hands an ambiguous abbreviation or a 3-, 6- or
// 9-word checksum-passing fragment to the flows as a mnemonic; it reports the
// existing "unknown format" status instead.
func TestFixShBip39ScanRefusesAmbiguousAndShort(t *testing.T) {
	scanAll := func(s string) (any, error) {
		sc := &scanner{}
		r := strings.NewReader(s)
		for {
			obj, err := sc.Scan(r)
			if !errors.Is(err, errScanInProgress) {
				return obj, err
			}
		}
	}
	const tail = " abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon bargain"
	if obj, err := scanAll("absent" + tail); err != nil {
		t.Fatalf("control: Scan(absent...) = %T, %v", obj, err)
	} else if _, ok := obj.(bip39.Mnemonic); !ok {
		t.Fatalf("control: Scan(absent...) = %T; want bip39.Mnemonic", obj)
	}
	for _, rec := range []string{
		"abs" + tail,
		"abandon abandon ability",
		"abandon abandon abandon abandon abandon able",
		strings.TrimSpace(strings.Repeat("abandon ", 9)),
	} {
		if obj, err := scanAll(rec); !errors.Is(err, errScanUnknownFormat) {
			t.Errorf("Scan(%q) = %T %v, %v; want errScanUnknownFormat", rec, obj, obj, err)
		}
	}
}
