//go:build refugium

package gui

import "testing"

// In a SEALED payload the Refugium line is keyed on the record's CLASS, as in a
// plaintext one: an md1/mk1 that is secret only because it is unconfirmed does
// not raise it, a seed does.
func TestRefugiumLineSealedUnconfirmedMdmkAloneRaisesNone(t *testing.T) {
	alone := kitVectorRecords(t, "kit-mk1-alone")
	s := sessionWith(alone[2:3]...)
	s.sealed = true
	for _, l := range syswLoadWarnings(s) {
		if l == refugiumSecretLine {
			t.Errorf("a sealed payload holding an unconfirmed md1/mk1 alone raised the Refugium line: %q", l)
		}
	}
	seed := chainPayloadNamed(t, "chain-codex32").Records
	s = sessionWith(seed...)
	s.sealed = true
	if lines := syswLoadWarnings(s); len(lines) == 0 || lines[len(lines)-1] != refugiumSecretLine {
		t.Errorf("a sealed payload holding a secret lacks the Refugium line last: %q", lines)
	}
}
