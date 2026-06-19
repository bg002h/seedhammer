package codex32

import "testing"

// TestMDChecksumSymbolsRoundTrip: the generated 13 regular checksum symbols,
// rendered after "md1" + data, produce a ValidMD string — i.e.
// MDChecksumSymbols is the GENERATE inverse of the ValidMD verify path
// (POLYMOD_INIT = 0x23181b3, NOT codex32's 1; I-7). md1 is regular-only.
func TestMDChecksumSymbolsRoundTrip(t *testing.T) {
	// A representative data-symbol body (header symbol + payload syms).
	data := []byte{0, 1, 2, 3, 4, 5, 16, 16, 16, 16, 16, 16}
	ck := MDChecksumSymbols(data)
	if len(ck) != mdmkShortSyms {
		t.Fatalf("checksum len = %d, want %d", len(ck), mdmkShortSyms)
	}
	var b []byte
	b = append(b, "md1"...)
	for _, s := range data {
		b = append(b, fe(s).rune())
	}
	for _, s := range ck {
		b = append(b, fe(s).rune())
	}
	s := string(b)
	if !ValidMD(s) {
		t.Fatalf("generated string fails ValidMD: %s", s)
	}
	// The data symbols recovered from the string equal the input.
	got, err := MDDataSymbols(s)
	if err != nil {
		t.Fatalf("MDDataSymbols: %v", err)
	}
	if len(got) != len(data) {
		t.Fatalf("data symbol count = %d, want %d", len(got), len(data))
	}
	for i := range data {
		if got[i] != data[i] {
			t.Fatalf("data symbol %d = %d, want %d", i, got[i], data[i])
		}
	}
}

// TestAssembleMD1Valid: assembleMD1 over arbitrary data symbols yields a
// ValidMD string with HRP "md1" and the 13-symbol regular checksum.
func TestAssembleMD1Valid(t *testing.T) {
	for _, n := range []int{1, 5, 12, 32, 64} {
		data := make([]byte, n)
		for i := range data {
			data[i] = byte((i*5 + 1) & 0x1f)
		}
		s := AssembleMD1(data)
		if len(s) < 3 || s[:3] != "md1" {
			t.Fatalf("n=%d: missing md1 HRP: %q", n, s)
		}
		if !ValidMD(s) {
			t.Fatalf("n=%d: AssembleMD1 fails ValidMD: %s", n, s)
		}
	}
}
