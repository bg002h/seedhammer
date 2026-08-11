package gui

import (
	"strings"
	"testing"

	"seedhammer.com/seal"
)

// §10.2.1a's distinguishability requirement, driven through the WHOLE flow.
//
// seal.ErrCodex32TooLong existing is not the requirement -- being TOLD is. The
// switch in unlockSealedFlow has a `default` arm that renders every unhandled
// error as "Payload unreadable.", so a new sentinel with no case of its own is
// invisible: the operator authenticates successfully, waits out the derivation
// and is then told their intact backup has been tampered with. That is the
// false diagnosis §10.2.1a exists to remove, and only a flow-level test can see
// it -- seal's own tests would stay green through it.
//
// The vector is the 127-character long code from
// `head -c 64 /dev/zero | biptool seed -seedlen 64 -id entr`, the same string
// seal/engraveable_test.go refuses. Its length and classification are asserted
// here rather than trusted, so a mistyped copy fails loudly instead of
// degenerating into a test of the allow-list.
const guiMS1Len127 = "ms10entrsqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqmk6rc3gq4c88nvp"

func TestUnlockNamesAnUnengraveableSecretInsteadOfCallingItUnreadable(t *testing.T) {
	if len(guiMS1Len127) != 127 {
		t.Fatalf("vector is %d characters, want 127", len(guiMS1Len127))
	}
	if got := seal.Classify([]byte(guiMS1Len127)); got != seal.ClassCodex32Secret {
		t.Fatalf("vector must classify as a codex32 secret, got %v", got)
	}
	if len(guiMS1Len127) <= seal.MaxEngraveableCodex32Len {
		t.Fatalf("vector is not over the %d limit", seal.MaxEngraveableCodex32Len)
	}

	d := sealVector(t, "D")
	// A public section, so the §6.6 hash screen is shown and the flow is driven
	// exactly as a real both-sections payload would be. The refusal happens in
	// the ENCRYPTED section, after the AEAD tag verified.
	blob := sealBlobForTest(t, d.Public, []string{guiMS1Len127},
		fixturePassphrase, fixtureIterations)

	h := newUnlockHarness(t, payloadReaderFrom(t, blob))
	h.toPassphrase(true)
	h.typePassphrase(strings.Fields(fixturePassphrase))
	got := h.mustReach("cannot engrave")

	// The two halves of the requirement: it must NAME the limit, and it must not
	// read as "someone replaced my payload". Both are checked on the frame that
	// was actually drawn.
	if !strings.Contains(strings.Join(strings.Fields(got), " "), "90") {
		t.Errorf("the screen must name the limit; got %q", got)
	}
	if strings.Contains(got, "unreadable") {
		t.Errorf("the screen still says the payload is unreadable; got %q", got)
	}
	if !uiContains(got, "Nothing was opened") {
		t.Errorf("the screen must say nothing was opened; got %q", got)
	}

	// And the flow leaves rather than looping for another passphrase: the
	// payload is refused WHOLE, and retrying cannot help.
	h.tapNav(Button3)
	for i := 0; i < 128 && !*h.done; i++ {
		if _, ok := h.frame(); !ok {
			break
		}
	}
	if !*h.done {
		t.Fatalf("the refusal did not leave the flow; last frame %q", h.content)
	}
}
