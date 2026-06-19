package codex32

import "testing"

func TestMKDataSymbols(t *testing.T) {
	// V1 chunk 1 (the shorter regular-code chunk) — BCH-valid mk1.
	const s = "mk1qpzg69ppsnz4v7cjv3qfjhf76k4t5pt96u0psdrqfqvll8qh7h5athg837pmkf3dpug2mmjtfel6x"
	syms, err := MKDataSymbols(s)
	if err != nil {
		t.Fatalf("MKDataSymbols(valid mk1): %v", err)
	}
	// Every symbol is a 5-bit value.
	for i, v := range syms {
		if v >= 32 {
			t.Fatalf("symbol %d = %d not in 0..31", i, v)
		}
	}
	// Data part minus the stripped 13-symbol regular checksum.
	_, data := splitHRP(s)
	if want := len(data) - mdmkShortSyms; len(syms) != want {
		t.Fatalf("len(syms) = %d, want %d", len(syms), want)
	}
	// First two symbols are the string-layer header: version 0, type 0x01 (chunked).
	if syms[0] != 0 || syms[1] != 0x01 {
		t.Fatalf("header syms = %d,%d; want 0,1", syms[0], syms[1])
	}
	// Non-mk1 input → error.
	if _, err := MKDataSymbols("ms10testsxxxxxxxxxxxxxxxxxxxxxxxx"); err == nil {
		t.Fatal("MKDataSymbols(non-mk1): want error, got nil")
	}
	if _, err := MKDataSymbols("not a bech32 string"); err == nil {
		t.Fatal("MKDataSymbols(garbage): want error, got nil")
	}
}
