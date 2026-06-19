package codex32

import "testing"

// TestMKChecksumSymbolsRoundTrip checks that the generated checksum symbols,
// rendered and appended to the rendered data part, produce a ValidMK string —
// i.e. MKChecksumSymbols is the inverse of the verify path (uses POLYMOD_INIT,
// not codex32's residue init 1).
func TestMKChecksumSymbolsRoundTrip(t *testing.T) {
	for _, long := range []bool{false, true} {
		// Build a data-symbol body whose rendered length + checksum lands in the
		// code's ValidMK bracket: regular needs data-part len in [14,93], long in
		// [96,108]. data-part len == len(data) + checksum syms.
		var data []byte
		if long {
			// 90 data syms + 15 checksum = 105 chars (in [96,108]).
			data = make([]byte, 90)
			for i := range data {
				data[i] = byte((i*7 + 3) & 0x1f)
			}
		} else {
			// 8-symbol chunked header + 8 fragment syms; 16 + 13 = 29 (in [14,93]).
			data = []byte{0, 1, 2, 3, 4, 5, 0, 1, 16, 16, 16, 16, 16, 16, 16, 16}
		}
		ck := MKChecksumSymbols(data, long)
		want := mdmkShortSyms
		if long {
			want = mdmkLongSyms
		}
		if len(ck) != want {
			t.Fatalf("long=%v: checksum len = %d, want %d", long, len(ck), want)
		}
		// Render "mk1" + data + checksum and verify it is BCH-valid.
		var b []byte
		b = append(b, "mk1"...)
		for _, s := range data {
			b = append(b, fe(s).rune())
		}
		for _, s := range ck {
			b = append(b, fe(s).rune())
		}
		s := string(b)
		if !ValidMK(s) {
			t.Fatalf("long=%v: generated string fails ValidMK: %s", long, s)
		}
		// The data symbols recovered from the string equal the input.
		got, err := MKDataSymbols(s)
		if err != nil {
			t.Fatalf("long=%v: MKDataSymbols: %v", long, err)
		}
		if len(got) != len(data) {
			t.Fatalf("long=%v: data symbol count = %d, want %d", long, len(got), len(data))
		}
		for i := range data {
			if got[i] != data[i] {
				t.Fatalf("long=%v: data symbol %d = %d, want %d", long, i, got[i], data[i])
			}
		}
	}
}
