package gui

import (
	"strings"
	"testing"
)

// TestComposerInvariantRefusesTwoSlotsAtOneOriginWithOneFingerprint is §4f's
// invariant and §8v. The asymmetric case is the dangerous one and is what
// this asserts first: slotMatchesCard skips the fingerprint test when the
// TEMPLATE declares none, so one card fills both slots and the operator is
// shown a mis-seated key as reviewed.
func TestComposerInvariantRefusesTwoSlotsAtOneOriginWithOneFingerprint(t *testing.T) {
	origin := composerTestOrigin(2, 0) // m/48'/0'/0'/2'
	for _, tc := range []struct {
		name string
		a, b composerAssignment
		want bool
	}{
		{"same origin, neither has a fingerprint",
			composerAssignment{src: 0, origin: origin},
			composerAssignment{src: 1, origin: origin}, true},
		{"same origin, only one has a fingerprint",
			composerAssignment{src: 0, origin: origin, fingerprint: [4]byte{1}, fpPresent: true},
			composerAssignment{src: 1, origin: origin}, true},
		{"same origin, two DIFFERENT fingerprints",
			composerAssignment{src: 0, origin: origin, fingerprint: [4]byte{1}, fpPresent: true},
			composerAssignment{src: 1, origin: origin, fingerprint: [4]byte{2}, fpPresent: true}, false},
		{"same origin, the SAME fingerprint twice",
			composerAssignment{src: 0, origin: origin, fingerprint: [4]byte{1}, fpPresent: true},
			composerAssignment{src: 1, origin: origin, fingerprint: [4]byte{1}, fpPresent: true}, true},
		{"different origins",
			composerAssignment{src: 0, origin: composerTestOrigin(2, 0)},
			composerAssignment{src: 1, origin: composerTestOrigin(2, 1)}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st := &composerState{list: composerTwoPathList(),
				assigned: []composerAssignment{tc.a, tc.b}}
			if got := composerInvariantViolation(st); got != tc.want {
				t.Errorf("composerInvariantViolation = %v, want %v", got, tc.want)
			}
		})
	}
	assertModalBodyFits(t, "the §8v same-origin refusal", errorScreenBody,
		composerCopySameOriginFewFingerprints())
}

// TestComposerRefusesTwoSlotsResolvingToTheSameXpub is §7d's refusal, and
// BIP-388 line 193's pairwise-distinct rule. md refuses it only at ENCODE, so
// without this the operator would meet a codec error instead of a review that
// names both slots.
func TestComposerRefusesTwoSlotsResolvingToTheSameXpub(t *testing.T) {
	st := &composerState{list: composerTwoPathList(), assigned: []composerAssignment{
		{src: 0, xpub: composerTestXpubA}, {src: 1, xpub: composerTestXpubB},
		{src: 2, xpub: composerTestXpubA}, {src: 3, xpub: ""},
	}}
	a, b, dup := composerDuplicateXpub(st)
	if !dup {
		t.Fatal("two slots holding one xpub are not detected")
	}
	if a != 0 || b != 2 {
		t.Errorf("the refusal names slots @%d and @%d, want @0 and @2", a, b)
	}
	// An UNSEATED slot is not a duplicate of another unseated slot: both have
	// no xpub, and refusing on that would refuse every keyless template.
	st.assigned = []composerAssignment{{src: -1}, {src: -1}}
	if _, _, dup := composerDuplicateXpub(st); dup {
		t.Error("two unseated slots were reported as the same key")
	}
}

// TestComposerC29WarningFiresInsideOnePathAndNotAcross is C29, both arms, and
// §12 item 5's condition test for §8g's two bodies.
func TestComposerC29WarningFiresInsideOnePathAndNotAcross(t *testing.T) {
	fp := [4]byte{0xaa}
	// composerTwoPathList is 2-of-3 then 1-of-1: slots @0..@2 are path 1.
	inOne := &composerState{list: composerTwoPathList(), assigned: []composerAssignment{
		{src: 0, fingerprint: fp, fpPresent: true},
		{src: 0, fingerprint: fp, fpPresent: true},
		{src: 1},
		{src: 2},
	}}
	inOne.sources = []composerSource{
		{kind: composerSourceSeed, fingerprint: fp, fpPresent: true},
		{kind: composerSourceKey}, {kind: composerSourceKey},
	}
	shared := composerSharedSeedInPath(inOne)
	if len(shared) != 1 {
		t.Fatalf("one seed at two slots INSIDE one path gives %d warnings, want 1", len(shared))
	}
	// The FIRST body when the shared slots reach the threshold, the second
	// otherwise (§8g's own heading). Path 1 is 2-of-3 and two slots are
	// shared, so the threshold is reached.
	body := composerSharedSeedBody(shared[0])
	if !strings.Contains(body, "can be satisfied by one person") {
		t.Errorf("two shared slots in a 2-of-3 use the below-threshold body:\n%s", body)
	}

	// ACROSS paths is C5's normal case: an informational line plus §8k, never
	// the warning.
	across := &composerState{list: composerTwoPathList(), assigned: []composerAssignment{
		{src: 0, fingerprint: fp, fpPresent: true},
		{src: 1}, {src: 2},
		{src: 0, fingerprint: fp, fpPresent: true},
	}}
	across.sources = inOne.sources
	if got := composerSharedSeedInPath(across); len(got) != 0 {
		t.Errorf("one seed across two paths raised %d C29 warnings; C5 makes it normal", len(got))
	}
	if !composerPersonInTwoPaths(across) {
		t.Error("one fingerprint in two paths does not trip the §8k informational line")
	}
	assertModalBodyFits(t, "the §8g at-threshold body", errorScreenBody,
		composerCopySameSeedThreshold([]uint8{1, 2}, 2, 3))
	assertModalBodyFits(t, "the §8g below-threshold body", errorScreenBody,
		composerCopySameSeedBelow([]uint8{1, 2}, 3))
	assertModalBodyFits(t, "the §8k two-paths line", errorScreenBody, composerCopyPersonInTwoPaths())
}

// TestComposerMappingLinesPrintOriginsVerbatimAndSayWhatIsNotChecked is
// §7d's mapping review and F-217: the account and every interior component
// are declarations this device cannot verify, and the screen must say so
// rather than imply a check it did not run.
func TestComposerMappingLinesPrintOriginsVerbatimAndSayWhatIsNotChecked(t *testing.T) {
	st := &composerState{list: composerTwoPathList(), assigned: []composerAssignment{
		{src: 0, origin: composerTestOrigin(2, 0), fingerprint: [4]byte{0x73, 0xc5, 0xda, 0x0a}, fpPresent: true, xpub: composerTestXpubA},
		{src: 1, origin: composerTestOrigin(2, 1), fingerprint: [4]byte{0x73, 0xc5, 0xda, 0x0a}, fpPresent: true, xpub: composerTestXpubB},
		{src: -1}, {src: -1},
	}}
	st.sources = []composerSource{{kind: composerSourceKey}, {kind: composerSourceKey}}
	joined := strings.Join(composerMappingLines(st), "\n")
	for _, want := range []string{
		"@0", "@1", "73c5da0a",
		"48'/0'/0'/2'", "48'/0'/1'/2'",
		"cannot confirm",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("the mapping review does not say %q:\n%s", want, joined)
		}
	}
}
