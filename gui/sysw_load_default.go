//go:build !refugium

package gui

// syswRefugiumLine is the default build's half of the Refugium warning
// (SPEC_systemwide_payloads.md 14f): the default build never shows it, and the
// text is not in this build at all.
func syswRefugiumLine(*syswSession) (string, bool) { return "", false }
