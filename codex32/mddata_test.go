package codex32

import "testing"

func TestMDDataSymbols(t *testing.T) {
	// wpkh_basic phrase (single-string md1), verbatim from md-codec tests/vectors/wpkh_basic.phrase.txt
	const s = "md1yqpqqxqq8xtwhw4xwn4qh"
	syms, err := MDDataSymbols(s)
	if err != nil {
		t.Fatalf("MDDataSymbols(valid md1): %v", err)
	}
	for i, v := range syms {
		if v >= 32 {
			t.Fatalf("symbol %d = %d not 5-bit", i, v)
		}
	}
	_, data := splitHRP(s)
	if want := len(data) - mdmkShortSyms; len(syms) != want { // 13-sym checksum stripped
		t.Fatalf("len(syms)=%d want %d", len(syms), want)
	}
	// Single-payload header: first symbol LSB 0 (version 4 = 0b00100).
	if syms[0]&1 != 0 {
		t.Fatalf("single-string md1 sym0 LSB = 1, want 0 (got %05b)", syms[0])
	}
	if _, err := MDDataSymbols("mk1qpzg69ppsnz4v7cjv3qfjhf76k4t5pt96u0psdrqfqvll8qh7h5athg837pmkf3dpug2mmjtfel6x"); err == nil {
		t.Fatal("MDDataSymbols(mk1): want error")
	}
	if _, err := MDDataSymbols("not bech32"); err == nil {
		t.Fatal("MDDataSymbols(garbage): want error")
	}
}
