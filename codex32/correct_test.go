package codex32

import (
	"strings"
	"testing"
)

// Valid fork literals (codex32_test.go / mdmk_test.go) used as correction seeds.
const (
	tvMS1Short = "ms10testsxxxxxxxxxxxxxxxxxxxxxxxxxx4nzvca9cmczlw"
	tvMD1      = "md1yqpqqxqq8xtwhw4xwn4qh"
	tvMK1Reg   = "mk1qpzg69ppsnz4v7cjv3qfjhf76k4t5pt96u0psdrqfqvll8qh7h5athg837pmkf3dpug2mmjtfel6x"
	tvMK1Long  = "mk1qp0zgpzp3xqgpqqgqjyty8ssyqcq0tdd4kk6mtdd4kk6mtdd4kk6mtdd4kk6mtdd4kk6mtdd4kk6mtddq2vfczmkedtrj2rjl6la2h9ek48q"
	tvMS1Long  = "ms100c8vsm32zxfguhpchtlupzry9x8gf2tvdw0s3jn54khce6mua7lqpzygsfjd6an074rxvcemlh8wu3tk925acdefghjklmnpqrstuvwxy06fhpv80undvarhrak"
)

// corruptAt substitutes the data-part symbol at dataPos by XORing mask (a
// nonzero GF(32) value) into it — the codex32-alphabet substitution the Rust
// corrupt_at helper performs. hrpLen is 3 ("ms1"/"md1"/"mk1"). Seeds here are
// lowercase, so the corrected char is emitted lowercase.
func corruptAt(t *testing.T, s string, dataPos int, mask fe) string {
	t.Helper()
	if mask == 0 {
		t.Fatal("mask must be nonzero")
	}
	r := []rune(s)
	abs := 3 + dataPos
	orig, ok := feFromRune(r[abs])
	if !ok {
		t.Fatalf("bad seed char %q at %d", r[abs], abs)
	}
	r[abs] = rune((orig.Add(mask)).rune()) // (fe).rune() returns byte
	return string(r)
}

func TestCorrectMD1OneError_OrientationPin(t *testing.T) {
	// Asymmetric single error at data position 5: if the MSB/LSB orientation
	// boundary is flipped, the decoder locates L-1-5 (or garbage) and re-verify
	// fails. So this is also the orientation pin (SPEC §2.6).
	corrupted := corruptAt(t, tvMD1, 5, 0b10101)
	if corrupted == tvMD1 {
		t.Fatal("corruption was a no-op")
	}
	res, ok := Correct(corrupted)
	if !ok {
		t.Fatal("expected a correction")
	}
	if res.Corrected != tvMD1 {
		t.Fatalf("Corrected = %q, want %q", res.Corrected, tvMD1)
	}
	if len(res.Edits) != 1 {
		t.Fatalf("len(Edits) = %d, want 1", len(res.Edits))
	}
	e := res.Edits[0]
	if e.Pos != 3+5 {
		t.Errorf("Edit.Pos = %d, want %d", e.Pos, 3+5)
	}
	if e.Now != tvMD1[3+5] {
		t.Errorf("Edit.Now = %q, want %q (original char)", e.Now, tvMD1[3+5])
	}
	if e.Was != corrupted[3+5] {
		t.Errorf("Edit.Was = %q, want %q (corrupted char)", e.Was, corrupted[3+5])
	}
	_ = strings.TrimSpace
}
