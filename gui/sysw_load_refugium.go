//go:build refugium

package gui

// syswRefugiumLine is SPEC_systemwide_payloads.md 14f: the Refugium build
// WARNS, and does not refuse, when a loaded payload (plaintext or sealed) holds
// a record whose class is secret by 3.3.1. It is decided by CLASS: an md1/mk1
// record that is secret only because it is unconfirmed does not raise it.
func syswRefugiumLine(s *syswSession) (string, bool) {
	for _, r := range s.records {
		if r.class.IsSecret() {
			return "Refugium build: secrets travel only in the Sealed Payload, never in this one. " +
				"Unload this payload unless you mean to engrave its secret now.", true
		}
	}
	return "", false
}
