package gui

import (
	"strings"
	"testing"

	"seedhammer.com/md"
)

// ─── §0.1a: the BIP-48 script-type origin must be SPOKEN ─────────────────────
//
// BIP-48 assigns 2' to NATIVE segwit, 1' to NESTED segwit, and nothing at all to
// legacy P2SH. Before S5 the device derived at 2' UNCONDITIONALLY, so an sh(wsh)
// build stamped the native-segwit path onto steel and a BIP-48-aware coordinator
// reading that plate derived at the wrong script-type path. Nothing in S1-S6
// said so.
//
// S5 MADE THE DEFAULT TEMPLATE-AWARE (§0.1a, "not earlier than S5"), so the
// announcement changed with it: sh(wsh) now derives at the path BIP-48 assigns
// it, and the sentence states the assignment instead of warning about a gap.
// derivedSlotOrigin is the single site for that mapping, and this test reads the
// expected path off it rather than restating a literal that could drift.
//
// RULED: ANNOUNCE, DO NOT REFUSE. §0.1 clause 2 decides it — the origin is
// printed with the xpub, carried in every mk1 and shown on the restore doc, so a
// wrong assumption is detectable by reading the artifacts. That makes it
// eligible to assume, and therefore obligatory to announce, on the confirmation
// surface itself (clause 3) rather than in scrollback.

// TestBuildReviewAnnouncesTheBip48Origin drives all three template choices.
func TestBuildReviewAnnouncesTheBip48Origin(t *testing.T) {
	stub := [4]byte{0x7b, 0x71, 0x64, 0x21}
	slots := []md.SlotInfo{{Index: 0}, {Index: 1}}
	joined := func(s md.MultisigScript) string {
		return strings.Join(buildReviewLines(s, stub, slots, nil, false, nil), "\n")
	}

	// Every script type must state the origin the build is USING. An operator who
	// cannot read the path off this screen cannot check it against anything.
	t.Run("every template states the origin in force", func(t *testing.T) {
		for _, s := range []md.MultisigScript{md.MultisigWsh, md.MultisigShWsh, md.MultisigSh} {
			want := derivedSlotOrigin(s, 0).String()
			if !strings.Contains(joined(s), want) {
				t.Errorf("script %v: the review does not state the origin in force (%s):\n%s",
					s, want, joined(s))
			}
		}
	})

	// wsh: the assumption is BIP-48's own recommended default, so the
	// announcement cites BIP-48 and stops. There is nothing to warn about.
	t.Run("native segwit names BIP-48 as the authority", func(t *testing.T) {
		got := joined(md.MultisigWsh)
		if !strings.Contains(got, "BIP-48") {
			t.Errorf("native segwit does not name its authority:\n%s", got)
		}
		if strings.Contains(got, "nested") || strings.Contains(got, "No BIP assigns") {
			t.Errorf("native segwit carries another template's caveat:\n%s", got)
		}
	})

	// sh(wsh): from S5 the build DERIVES at BIP-48's nested-segwit path, so the
	// announcement cites the assignment and names the path in force. It must no
	// longer claim to be using the native-segwit 2' path, which was true only
	// until S5 and would now send the operator to the wrong place.
	t.Run("nested segwit names BIP-48's nested assignment as the path in force", func(t *testing.T) {
		got := joined(md.MultisigShWsh)
		for _, want := range []string{"BIP-48", "nested segwit", "m/48h/0h/0h/1h"} {
			if !strings.Contains(got, want) {
				t.Errorf("nested segwit review is missing %q:\n%s", want, got)
			}
		}
		if strings.Contains(got, multisigSharedOrigin().String()) {
			t.Errorf("nested segwit review still names the native-segwit path %s, which "+
				"this build no longer derives at:\n%s", multisigSharedOrigin(), got)
		}
	})

	// legacy sh: NO BIP assigns a path at all, so there is no authority to cite
	// and the device must own the choice out loud. Citing BIP-48 here would be a
	// false claim of authority, which is worse than saying nothing.
	t.Run("legacy P2SH owns the choice, citing no authority it does not have", func(t *testing.T) {
		got := joined(md.MultisigSh)
		if !strings.Contains(got, "No BIP assigns") {
			t.Errorf("legacy P2SH does not say that no BIP assigns it a path:\n%s", got)
		}
		if strings.Contains(got, "the BIP-48 path for") {
			t.Errorf("legacy P2SH claims a BIP-48 assignment that does not exist:\n%s", got)
		}
	})

	// The three announcements must be PAIRWISE DISTINCT. The defect this guards
	// is the S3 one restated: a helper that returns the same sentence for two
	// templates passes any test that only ever checks one of them.
	t.Run("the three announcements are pairwise distinct", func(t *testing.T) {
		w, sw, sh := joined(md.MultisigWsh), joined(md.MultisigShWsh), joined(md.MultisigSh)
		if w == sw || w == sh || sw == sh {
			t.Errorf("two templates announce the same thing:\nwsh:\n%s\nsh(wsh):\n%s\nsh:\n%s",
				w, sw, sh)
		}
	})

	// §0.1's corollary, and it is the half that keeps the warning worth reading:
	// the announcement is about a path the DEVICE chose from the script type. It
	// must not be phrased as a warning about the operator's own inputs, and it
	// must draw — no em-dash, which is a zero-pixel glyph in the body face and
	// takes the whole line with it (measured 2026-08-15: a modal body containing
	// one rasters at 2652 px against 7419 for the same text with a hyphen).
	t.Run("the announcement can actually be drawn", func(t *testing.T) {
		for _, s := range []md.MultisigScript{md.MultisigWsh, md.MultisigShWsh, md.MultisigSh} {
			for _, line := range buildReviewLines(s, stub, slots, nil, false, nil) {
				if strings.ContainsAny(line, "\u2014\u2013\u00b7\u2018\u2019\u201c\u201d") {
					t.Errorf("script %v: review line %q carries a glyph the body face "+
						"lacks, so the line does not draw", s, line)
				}
			}
		}
	})
}
