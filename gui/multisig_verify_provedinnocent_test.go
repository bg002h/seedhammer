package gui

import (
	"strings"
	"testing"
)

// TestProvedInnocentBodyIsRMsAdoptedWording is GATE 3.2a (R-M, S6b spec §3.2a).
//
// multisigVerifyNoSlotBody's provedInnocent arm used to end "Your plates are
// fine. Try again and skip the passphrase." R-M struck the skip advice
// (REQUIREMENTS §2bis: it reads as a procedural workaround and buries the
// finding) and ruled the operator must be told outright that this is NOT a
// passphrase-protected wallet.
//
// THE BODY IS FIXED TEXT, given verbatim in spec §3.2a -- this test pins it
// byte-for-byte, not just its themes, because a paraphrase that keeps the
// THEMES but drifts the WORDS is exactly the kind of fold this cycle's spec
// says never to make ("Do not reword it").
func TestProvedInnocentBodyIsRMsAdoptedWording(t *testing.T) {
	const want = "These plates match this seed with NO passphrase. This is not a " +
		"passphrase-protected wallet. If you meant to use one, these plates are not " +
		"that wallet: try the password again. If you continue without a passphrase, " +
		"these plates are complete as they are."
	if len(want) != 251 {
		t.Fatalf("this test's OWN copy of R-M's wording is %d characters, want the "+
			"pre-measured 251 -- the test fixture drifted from spec §3.2a, not the code", len(want))
	}

	got := multisigVerifyNoSlotBody(true /* passphraseTyped */, true /* provedInnocent */)
	if got != want {
		t.Errorf("multisigVerifyNoSlotBody(true, true) =\n%q\nwant (R-M's adopted wording, "+
			"REQUIREMENTS §2bis, verbatim)\n%q", got, want)
	}
}

// TestProvedInnocentBodyDoesNotClaimAPassphraseIsRequired is GATE 3.2a's second
// half: R-M explicitly forbids "A passphrase will be necessary to use the key"
// in this arm, because it fires precisely when the plates match the seed with
// NO passphrase -- so that claim is false here, not merely unwanted.
//
// It also pins the struck "skip the passphrase" advice's absence and the
// em-dash prohibition (gui/multisig_build.go:735-739 records an em dash in a
// modal body once meaning the body did not draw at all), as regression guards
// for the same string.
func TestProvedInnocentBodyDoesNotClaimAPassphraseIsRequired(t *testing.T) {
	body := multisigVerifyNoSlotBody(true, true)
	if strings.Contains(body, "will be necessary to use the key") {
		t.Errorf("the provedInnocent body claims a passphrase is necessary, which is FALSE "+
			"on this arm (it fires when the plates match the seed with NO passphrase): %q", body)
	}
	if strings.Contains(body, "skip the passphrase") {
		t.Errorf("the provedInnocent body still carries the struck \"skip the passphrase\" "+
			"advice (R-M struck it as a procedural workaround that buries the finding): %q", body)
	}
	if strings.Contains(body, "—") {
		t.Errorf("the provedInnocent body contains an em dash, which once meant a modal "+
			"body did not draw AT ALL (gui/multisig_build.go:735-739): %q", body)
	}
	if !strings.Contains(body, "not a") || !strings.Contains(body, "passphrase-protected wallet") {
		t.Errorf("the provedInnocent body does not state outright that this is NOT a "+
			"passphrase-protected wallet, which is the fact R-M requires: %q", body)
	}
}

// TestProvedInnocentBodyPassesTheModalFitClassCheck is GATE 3.2a's third half:
// the body must be DRAWN IN FULL, with headroom, on the "Verify Bundle"
// showError screen it is actually shown on.
//
// COVERAGE NOTE (spec §4/GATE 4 belongs to P6, which sweeps every modal this
// cycle touches -- this call is this phase's OWN use of the SAME one-line
// class check spec §4 says "exists and is a one-line call"; P6's sweep will
// re-run over this body too, as part of sweeping every modal S6b changed, and
// state its own coverage there.
func TestProvedInnocentBodyPassesTheModalFitClassCheck(t *testing.T) {
	body := multisigVerifyNoSlotBody(true, true)
	assertModalBodyFits(t, "R-M's provedInnocent arm (gate 3.2a)", errorScreenBody, body)
}
