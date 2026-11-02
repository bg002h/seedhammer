package bip39

import (
	"strings"
	"testing"
)

// Bug-hunt triage, sh2 slice S3 (report bughunt-2026-10-31-sh2.md).

func bhValid(m Mnemonic) (ok bool, panicked any) {
	defer func() { panicked = recover() }()
	ok = m.Valid()
	return
}

// Bug 12: Mnemonic.Valid panics on an empty mnemonic (index -1) and on 27 or
// more words ("entropy too long"); ParseMnemonic reaches the second with a
// 27-word string of valid words.
func TestBughuntSh212ValidPanicsOnEmptyAndLong(t *testing.T) {
	// Positive control: a valid 12-word mnemonic is valid.
	const good = "legal winner thank year wave sausage worth useful legal winner thank yellow"
	m, err := ParseMnemonic(good)
	if err != nil {
		t.Fatalf("control: %v", err)
	}
	if ok, p := bhValid(m); !ok || p != nil {
		t.Fatalf("control: Valid = %v, panic = %v", ok, p)
	}
	words := func(n int) Mnemonic { return make(Mnemonic, n) } // n copies of word 0 ("abandon")
	for _, n := range []int{0, 27, 30} {
		ok, p := bhValid(words(n))
		if p != nil {
			t.Errorf("Mnemonic of %d words: Valid panicked: %v", n, p)
		} else if ok {
			t.Errorf("Mnemonic of %d words: Valid = true", n)
		}
	}
	// ParseMnemonic with 27 valid words must refuse, not panic.
	func() {
		defer func() {
			if p := recover(); p != nil {
				t.Errorf("ParseMnemonic of 27 valid words panicked: %v", p)
			}
		}()
		if _, err := ParseMnemonic(strings.TrimSpace(strings.Repeat("abandon ", 27))); err == nil {
			t.Errorf("ParseMnemonic accepted 27 words")
		}
	}()
}
