package sysw

import "testing"

// The eight strings the pre-flash conformance review measured as disagreeing
// between the Rust primary and this port, plus the rows that already agreed so
// a fix cannot pass by making everything Unknown.
//
// Rust is normative. Every expectation here is RUST's answer, taken from the
// review's measured table, not from what this port happens to do.
func TestClassifyMatchesTheRustPrimary(t *testing.T) {
	const md1 = "md1ytpqqxpp3zcpydzk0zdt492xzr7r9qxfc"
	const seed = "abandon abandon abandon abandon abandon abandon " +
		"abandon abandon abandon abandon abandon about"

	cases := []struct {
		name  string
		input string
		want  Class
	}{
		{"md1 plain", md1, ClassMDMK},
		{"md1 trailing space", md1 + " ", ClassMDMK},
		{"md1 leading space", " " + md1, ClassMDMK},
		{"seed lowercase", seed, ClassMnemonic},
		{"seed UPPERCASE", "ABANDON ABANDON ABANDON ABANDON ABANDON ABANDON " +
			"ABANDON ABANDON ABANDON ABANDON ABANDON ABOUT", ClassUnknown},
		{"seed Mixed case", "Abandon abandon abandon abandon abandon abandon " +
			"abandon abandon abandon abandon abandon About", ClassUnknown},
		{"three-letter prefixes", "aban aban aban aban aban aban " +
			"aban aban aban aban aban abou", ClassUnknown},
		{"three words, checksum-valid", "abandon abandon about", ClassUnknown},
		{"text: empty body", "text:", ClassFreeText},
		{"text: odd hex", "text:abc", ClassUnknown},
	}
	for _, c := range cases {
		if got := Classify(c.input); got != c.want {
			t.Errorf("%s: Classify(%.40q) = %v, want %v (Rust's answer)",
				c.name, c.input, got, c.want)
		}
	}
}

// The two ms1 rows need a BCH-valid string, so they are built rather than
// pasted: an over-long one, and one whose HRP is not `ms`.
func TestClassifyRejectsMs1RustWouldRefuse(t *testing.T) {
	long := ""
	for len(long) <= MaxEngraveableMs1Len {
		long += "q"
	}
	if got := Classify("ms1" + long); got == ClassCodex32Secret {
		t.Errorf("an ms1 longer than %d chars must not classify as a secret: got %v",
			MaxEngraveableMs1Len, got)
	}
	// HRP pin: whatever follows, a non-`ms` HRP is never a codex32 SECRET here.
	for _, hrp := range []string{"aa1", "xx1", "md1", "mk1"} {
		if got := Classify(hrp + "qqqqqqqqqqqqqqqqqqqqqqqqqqqqqq"); got == ClassCodex32Secret {
			t.Errorf("HRP %q must not classify as Codex32Secret, got %v", hrp, got)
		}
	}
}
