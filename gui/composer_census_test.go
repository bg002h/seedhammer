package gui

import (
	"strings"
	"testing"
)

// TestComposerDescriptorCeilingIsMeasuredNotWrittenDown is the §13 item 1
// number, and the reason it is a search: a constant here goes stale the
// first time the plate geometry, the stroke width or the font moves, and it
// goes stale SILENTLY, inside a refusal.
func TestComposerDescriptorCeilingIsMeasuredNotWrittenDown(t *testing.T) {
	p := newPlatform()
	n := composerDescriptorCeilingChars(p)
	if n <= 0 {
		t.Fatalf("the measured descriptor ceiling is %d characters", n)
	}
	t.Logf("concrete descriptor plate ceiling: %d characters at this platform's params", n)
	// The search is EXACT at its own boundary, which is what makes the number
	// quotable in a refusal.
	if !composerDescriptorPlateFits(p, strings.Repeat("a", n)) {
		t.Errorf("the ceiling %d does not itself fit", n)
	}
	if composerDescriptorPlateFits(p, strings.Repeat("a", n+1)) {
		t.Errorf("one character past the ceiling %d still fits, so the search stopped short", n)
	}
	// C10's two-path wallet is 688 characters (brainstorm record). Whether it
	// fits is the MEASUREMENT this test records; it is not asserted either
	// way, because asserting a number nobody has measured is how a plan pins
	// a hope.
	t.Logf("C10's 688-character two-path wallet fits: %v",
		composerDescriptorPlateFits(p, strings.Repeat("a", 688)))
}

// TestComposerCensusLinesSayHowRecoveryDetectsAnError is §7f's last clause:
// recovery-time error detection differs by form and the census says so.
func TestComposerCensusLinesSayHowRecoveryDetectsAnError(t *testing.T) {
	cards := []bundleCard{{
		kind: cardMD1, label: "md1 template",
		strings: []string{"md1abc"}, summary: "key-less wallet policy",
	}}
	lines := composerCensusLines(newPlatform().EngraverParams(), cards)
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "This engraves") {
		t.Errorf("the census does not carry buildPlateCensusLines' plate count:\n%s", joined)
	}
	for _, want := range []string{"error correction", "only its checksum"} {
		if !strings.Contains(joined, want) {
			t.Errorf("the census does not say %q; md1/mk1 carry BCH and a plain "+
				"descriptor plate carries only its BIP-380 checksum:\n%s", want, joined)
		}
	}
}
