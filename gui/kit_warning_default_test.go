//go:build !refugium

package gui

import "testing"

// TestDefaultBuildNoRefugiumWarning: the untagged build never shows the
// section 14f line, for the payloads the Refugium row raises it on.
func TestDefaultBuildNoRefugiumWarning(t *testing.T) {
	const line = "Refugium build: secrets travel only in the Sealed Payload, never in this one. Unload this payload unless you mean to engrave its secret now."
	seed := chainPayloadNamed(t, "chain-codex32").Records
	for _, recs := range [][]string{seed, kitWithMs1(t)} {
		for _, l := range syswLoadWarnings(sessionWith(recs...)) {
			if l == line {
				t.Errorf("the default build shows the Refugium line")
			}
		}
	}
}
