//go:build refugium

package gui

// The Refugium build's twin of prooftriggers_default.go: the same names, no
// trigger (Refugium plan F7 §4.1). Every comparison is also gated by
// proofTriggersEnabled, which is false here, so these are belt and braces:
// even with the gate removed, an empty table matches nothing and
// ppIsPassProofTrigger matches nothing -- including the empty string, which a
// trigger "disabled" by setting it to "" would have matched.

// ftProofs is empty: the free-text program offers no proof in this build.
var ftProofs []ftProof

// ppIsPassProofTrigger matches nothing in this build.
func ppIsPassProofTrigger(string) bool { return false }

// ppPassProofKeep has nothing to say: the test-pattern prompt it words is
// never drawn in this build.
func ppPassProofKeep(bool) string { return "" }
