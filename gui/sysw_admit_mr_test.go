package gui

import (
	"errors"
	"testing"

	"seedhammer.com/seal"
	"seedhammer.com/sysw"
)

const mrSingleCard = "mr1q2qyyter2scsqzpl40veqyppqqxpxpgnxjakqqn88sqhs32rd4eysmyjsmg0770kkufgjegg"

// S1 test 28 (14d "Admission"): ClassMr is admitted at Engrave Bundle and at no
// other program, and the Sealed Payload's allow-list still refuses an mr1
// record in both sections.
func TestAdmitMrAtBundleOnly(t *testing.T) {
	if got := sysw.Classify(mrSingleCard); got != sysw.ClassMr {
		t.Fatalf("Classify(single-card) = %v, want ClassMr", got)
	}
	if !admits(progBundle, sysw.ClassMr) {
		t.Error("Engrave Bundle must admit ClassMr (the kit door)")
	}
	for p := progBackupWallet; p <= progTransaction; p++ {
		if p != progBundle && admits(p, sysw.ClassMr) {
			t.Errorf("program %d admits ClassMr; only Engrave Bundle may", p)
		}
	}
	for _, sec := range []seal.Section{seal.SectionPublic, seal.SectionEncrypted} {
		if _, err := seal.AdmitSection([][]byte{[]byte(mrSingleCard)}, sec); !errors.Is(err, seal.ErrRecordNotPermitted) {
			t.Errorf("section %v: seal admits an mr1 record (err %v), want ErrRecordNotPermitted", sec, err)
		}
	}
}

func TestTxClassNameMr(t *testing.T) {
	if got := txClassName(sysw.ClassMr); got != "mr1 string" {
		t.Errorf("txClassName(ClassMr) = %q, want %q", got, "mr1 string")
	}
}
