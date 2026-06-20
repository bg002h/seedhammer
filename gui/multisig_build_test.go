package gui

import (
	"testing"

	"seedhammer.com/md"
)

// TestMultisigScriptChoices: exactly the 3 sortedmulti wrappers, order-locked to
// the MultisigScript enum, wsh first (highlighted by default).
func TestMultisigScriptChoices(t *testing.T) {
	c := multisigScriptChoices()
	if len(c) != 3 {
		t.Fatalf("template choices = %d, want 3", len(c))
	}
	if multisigScriptFor(0) != md.MultisigWsh ||
		multisigScriptFor(1) != md.MultisigShWsh ||
		multisigScriptFor(2) != md.MultisigSh {
		t.Fatalf("template mapping wrong: 0=%v 1=%v 2=%v",
			multisigScriptFor(0), multisigScriptFor(1), multisigScriptFor(2))
	}
}

// TestMultisigNChoices: n picker offers exactly "2".."5" (n in 2..5).
func TestMultisigNChoices(t *testing.T) {
	c := multisigNChoices()
	want := []string{"2", "3", "4", "5"}
	if len(c) != len(want) {
		t.Fatalf("n choices = %v, want %v", c, want)
	}
	for i := range want {
		if c[i] != want[i] {
			t.Fatalf("n choices[%d] = %q, want %q", i, c[i], want[i])
		}
	}
	if multisigNFor(0) != 2 || multisigNFor(3) != 5 {
		t.Fatalf("n mapping wrong: 0=%d 3=%d, want 2 and 5", multisigNFor(0), multisigNFor(3))
	}
}

// TestMultisigKChoices: k picker is built from the chosen n as "1".."n" (k<=n,
// k>=1), so k>n is structurally unreachable.
func TestMultisigKChoices(t *testing.T) {
	for n := 2; n <= 5; n++ {
		c := multisigKChoices(n)
		if len(c) != n {
			t.Fatalf("n=%d: k choices = %v, want %d entries", n, c, n)
		}
		if c[0] != "1" {
			t.Fatalf("n=%d: k choices[0] = %q, want 1", n, c[0])
		}
		if multisigKFor(0) != 1 || multisigKFor(n-1) != n {
			t.Fatalf("n=%d: k mapping wrong: 0=%d last=%d", n, multisigKFor(0), multisigKFor(n-1))
		}
	}
}

// TestMultisigSelfSlotChoices: the self-slot picker offers "@0".."@{n-1}".
func TestMultisigSelfSlotChoices(t *testing.T) {
	for n := 2; n <= 5; n++ {
		c := multisigSelfSlotChoices(n)
		if len(c) != n {
			t.Fatalf("n=%d: self-slot choices = %v, want %d entries", n, c, n)
		}
		if c[0] != "@0" || c[n-1] != ("@"+string(rune('0'+n-1))) {
			t.Fatalf("n=%d: self-slot choices = %v", n, c)
		}
	}
}

// TestMultisigFpChoices: the fp-presence picker offers exactly No / Yes
// (Omit / Include), index 0 == Omit (default).
func TestMultisigFpChoices(t *testing.T) {
	c := multisigFpChoices()
	if len(c) != 2 {
		t.Fatalf("fp choices = %v, want 2", c)
	}
	if multisigIncludeFpFor(0) != false || multisigIncludeFpFor(1) != true {
		t.Fatalf("fp mapping wrong: 0=%v 1=%v, want false,true",
			multisigIncludeFpFor(0), multisigIncludeFpFor(1))
	}
}
