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

// applyKnown corrupts s at the given (dataPos,mask) pairs and asserts Correct
// recovers the original byte-for-byte with the expected edit count.
func assertRoundTrip(t *testing.T, valid string, pos []int, mask []fe) {
	t.Helper()
	c := valid
	for i := range pos {
		c = corruptAt(t, c, pos[i], mask[i])
	}
	if c == valid {
		t.Fatal("corruption was a no-op")
	}
	res, ok := Correct(c)
	if !ok {
		t.Fatalf("expected a correction for %d errors", len(pos))
	}
	if res.Corrected != valid {
		t.Fatalf("Corrected = %q, want %q", res.Corrected, valid)
	}
	if len(res.Edits) != len(pos) {
		t.Fatalf("len(Edits) = %d, want %d", len(res.Edits), len(pos))
	}
	for _, e := range res.Edits {
		if e.Now != valid[e.Pos] {
			t.Errorf("Edit at %d: Now=%q, want original %q", e.Pos, e.Now, valid[e.Pos])
		}
	}
}

func TestCorrectRoundTrips(t *testing.T) {
	cases := []struct {
		name  string
		valid string
		pos   []int
		mask  []fe
	}{
		{"md1/2err", tvMD1, []int{2, 14}, []fe{0b11001, 0b00111}},
		{"md1/4err", tvMD1, []int{0, 5, 11, 20}, []fe{0b00001, 0b10000, 0b11111, 0b01010}},
		{"mk1reg/1err", tvMK1Reg, []int{40}, []fe{0b10101}},
		{"mk1reg/4err", tvMK1Reg, []int{3, 17, 50, 76}, []fe{1, 16, 31, 10}},
		{"mk1long/1err", tvMK1Long, []int{60}, []fe{0b01110}},
		{"mk1long/4err", tvMK1Long, []int{0, 5, 18, 28}, []fe{0b00001, 0b10000, 0b11111, 0b01010}},
		{"ms1short/1err", tvMS1Short, []int{7}, []fe{0b01011}},
		{"ms1short/4err", tvMS1Short, []int{0, 11, 23, 44}, []fe{1, 16, 31, 10}},
		{"ms1long/4err", tvMS1Long, []int{0, 30, 60, 120}, []fe{0b00001, 0b10000, 0b11111, 0b01010}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { assertRoundTrip(t, tc.valid, tc.pos, tc.mask) })
	}
}

func TestCorrectFiveErrorsNotSilentOriginal(t *testing.T) {
	// >t errors: Correct must NEVER silently return the original. It may fail
	// (false) or return a different (bogus) string — the human diff-gate is the
	// backstop (SPEC §2.3). Mirrors the Rust 5-error contract.
	c := tvMK1Long
	for i, p := range []int{0, 5, 10, 15, 20} {
		c = corruptAt(t, c, p, fe(i+1))
	}
	res, ok := Correct(c)
	if ok && res.Corrected == tvMK1Long {
		t.Fatal("5-error corruption must not silently recover the original")
	}
}

func TestCorrectSuppressesUncorrectable(t *testing.T) {
	// A valid string has zero syndromes -> nothing to correct -> (_,false).
	if _, ok := Correct(tvMD1); ok {
		t.Error("a valid string must not yield a correction")
	}
	// Random garbage of a valid length may fail (expected) — but if Correct
	// ever claims a fix, the mandatory re-verify means it MUST be md-valid
	// (MINOR-3: a phantom, non-verifying "fix" is a hard failure).
	garbage := "md1" + strings.Repeat("q", len(tvMD1)-3-1) + "p"
	if res, ok := Correct(garbage); ok && !ValidMD(res.Corrected) {
		t.Errorf("Correct returned a non-re-verifying fix: %q", res.Corrected)
	}
}

func TestNegativeCrossConstant(t *testing.T) {
	// A one-error-corrupted VALID ms1 string, decoded under the md constants
	// (different POLYMOD_INIT + target), must NOT yield an md-valid string.
	// Guards against a single shared constant table cross-validating (SPEC §2.5).
	corrupted := corruptAt(t, tvMS1Short, 7, 0b01011)
	_, data := splitHRP(corrupted)
	eng := &engine{
		generator: newShortChecksum().generator,
		residue:   unpackSyms(0, mdmkPolymodInitLo, mdmkShortSyms),
		target:    unpackSyms(mdRegularTargetHi, mdRegularTargetLo, mdmkShortSyms),
	}
	if err := eng.inputHRP("ms"); err != nil {
		t.Fatal(err)
	}
	if err := eng.inputData(data); err != nil {
		t.Fatal(err)
	}
	n := mdmkShortSyms
	coeffs := make([]fe, n)
	for i := 0; i < n; i++ {
		coeffs[i] = eng.residue[n-1-i] ^ eng.target[n-1-i]
	}
	pos, mags, ok := decodeErrors(coeffs, len(data), betaGf1024, regularJStart)
	if ok {
		// If it did "decode", applying it must NOT yield an md-valid string.
		r := []rune(corrupted)
		for i, k := range pos {
			abs := 3 + k
			orig, _ := feFromRune(r[abs])
			r[abs] = rune((orig.Add(mags[i])).rune()) // (fe).rune() returns byte
		}
		if ValidMD("md" + string(r)[2:]) {
			t.Fatal("ms data cross-validated under md constants")
		}
	}
	// Positive control (MINOR-2): the SAME corrupted ms1 string MUST correct
	// under its own (ms) constants — so this test isn't merely vacuous.
	res, ok := Correct(corrupted)
	if !ok || res.Corrected != tvMS1Short {
		t.Fatalf("ms1 should self-correct under ms constants: ok=%v got=%q", ok, res.Corrected)
	}
}

func TestCorrectCasePreserved(t *testing.T) {
	// Uppercase input must yield an uppercase, re-verifying correction.
	upper := strings.ToUpper(tvMD1)
	corrupted := corruptUpper(t, upper, 5, 0b10101)
	res, ok := Correct(corrupted)
	if !ok {
		t.Fatal("expected a correction")
	}
	if res.Corrected != upper {
		t.Fatalf("Corrected = %q, want %q", res.Corrected, upper)
	}
}

// corruptUpper is corruptAt for an uppercase string (emits an uppercase char).
func corruptUpper(t *testing.T, s string, dataPos int, mask fe) string {
	t.Helper()
	r := []rune(s)
	abs := 3 + dataPos
	orig, ok := feFromRune(r[abs])
	if !ok {
		t.Fatalf("bad seed char %q", r[abs])
	}
	r[abs] = rune(feToByte(orig.Add(mask), true))
	return string(r)
}
