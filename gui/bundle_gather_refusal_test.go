package gui

import (
	"strings"
	"testing"
	"testing/synctest"
)

// ─── The gather's own refusals must be READABLE, not merely titled ───────────
//
// S2's D-4 landing gave bundleGatherFlow's three "Done" refusals the caller's
// title and left their BODIES carrying em-dashes. The execution review drove one
// of them from the Build path and measured 2652 ink pixels — the title-only
// value — so the operator met a screen titled "Cosigner Keys" with nothing on
// it, and the flow then continued with the surviving cards without ever saying a
// card had been dropped.
//
// TWO CORRECTIONS TO WHAT S2 BELIEVED WHEN IT WROTE THE FLOOR, both measured:
//
//  1. the glyph does not blank ITS LINE, it blanks THE WHOLE BODY. Five bodies
//     of different lengths all rastered at exactly 2652.
//  2. `uiContains` still returns TRUE on the blank frame, because the text ops
//     are submitted and only the drawing fails. Every content assertion in this
//     package is blind to it — including S2's own D-4 guard. Ink is the only
//     instrument that sees this class, which is why these tests raster.

// TestGatherPendingRefusalIsReadableFromBuild drives the pending-card refusal
// through the REAL Build flow, the way the review reproduced it: a payload
// holding two complete cosigner cards plus the first chunk only of a third.
//
// That shape is not contrived. classifyCosignerSupply sees two complete cards
// for two open slots, so the pre-gather refusal does not fire, and this message
// is what the operator reads.
func TestGatherPendingRefusalIsReadableFromBuild(t *testing.T) {
	sets := cosignerCardFixtures(t, 3) // A@0, B@0, C@0
	var records []string
	records = append(records, sets[1]...) // B@0, complete
	records = append(records, sets[2]...) // C@0, complete
	records = append(records, sets[0][0]) // A@0, FIRST CHUNK ONLY
	synctest.Test(t, func(t *testing.T) {
		p := newPlatform()
		p.display = sh2DisplaySize
		ctx := NewContext(p)
		ctx.sysw = sessionHolding(records...)
		frame, _, ink, quit := runUITouchRaster(ctx, func() {
			buildMultisigPolicyFlow(ctx, &descriptorTheme)
		})
		defer quit()

		// template wsh -> n=3 -> k=2 -> @0 -> fp omit.
		for i, downs := range []int{0, 1, 1, 0, 0} {
			stage := []string{"Choose policy type", "How many keys (n)?",
				"Required signatures (k of 3)?", "Which slot is your key?",
				"Include key fingerprints?"}[i]
			if c, ok := pumpUntil(frame, stage, 64); !ok {
				t.Fatalf("%s not shown; got %q", stage, c)
			}
			for j := 0; j < downs; j++ {
				click(&ctx.Router, Down)
				frame()
			}
			click(&ctx.Router, Button3)
			frame()
		}

		if c, ok := pumpUntil(frame, "mk1 keys: 2", 64); !ok {
			t.Fatalf("the gather never showed the two complete cards; got %q", c)
		}
		click(&ctx.Router, Button3) // Done adding cards -> the pending refusal
		frame()

		content, ok := pumpUntil(frame, "Dropped an incomplete card", 64)
		if !ok {
			t.Fatalf("the pending-card refusal was not reached; got %q", content)
		}
		// THE ASSERTION THAT MATTERS. A content check passes on a blank frame —
		// the review measured uiContains returning true on exactly this screen —
		// so the refusal is judged by INK.
		if ink() < buildWalkRasterFloor {
			t.Errorf("the pending-card refusal drew only %d ink pixels (floor %d): the "+
				"operator is told a card was dropped by a screen with no body, and the "+
				"flow then continues with the survivors. Content assertions cannot see "+
				"this; only ink can.", ink(), buildWalkRasterFloor)
		}
		t.Logf("pending-card refusal from Build: ink = %d px", ink())
	})
}

// blankingGlyphs are the runes measured to defeat the body face. Each blanks the
// ENTIRE body of the frame it appears in, not merely its own line, and every
// content-based assertion in this package reports the text as present anyway.
var blankingGlyphs = []string{"\u2014", "\u2013", "\u00b7", "\u2018", "\u2019", "\u201c", "\u201d"}

// stringLiterals returns the double-quoted literals in a chunk of Go source.
//
// LITERALS, NOT RAW SOURCE. The first version of this guard scanned the function
// text and fired on a COMMENT containing an em-dash, which is not a defect and
// would have taught the next author to delete the guard rather than the glyph.
// Prose may say what it likes; only what reaches the screen is checked.
func stringLiterals(src string) []string {
	var out []string
	inStr, esc := false, false
	var cur strings.Builder
	lines := strings.Split(src, "\n")
	for _, line := range lines {
		if i := strings.Index(strings.TrimSpace(line), "//"); i == 0 {
			continue // whole-line comment
		}
		for _, r := range line {
			switch {
			case esc:
				esc = false
				cur.WriteRune(r)
			case inStr && r == '\\':
				esc = true
			case r == '"':
				if inStr {
					out = append(out, cur.String())
					cur.Reset()
				}
				inStr = !inStr
			case inStr:
				cur.WriteRune(r)
			}
		}
		inStr, esc = false, false // literals do not span lines in this package
		cur.Reset()
	}
	return out
}

// TestGatherScreenTextCarriesNoBlankingGlyph is the cheap standing guard over
// EVERY string the shared gather screen can draw: the three "Done" refusals, the
// per-scan feedback line and the tally.
//
// The feedback strings matter as much as the refusals and were not in the
// review's list: they are appended to the GATHER's own body, so one of them
// would blank the card tally itself rather than a modal. They are set only from
// a scan, and the SH2 has a working reader (§0.1b), so they reach real hardware.
//
// A drive covers the one refusal reachable from a payload-fed Build; this covers
// the rest, which need a reader or an empty gather to reach.
func TestGatherScreenTextCarriesNoBlankingGlyph(t *testing.T) {
	for _, fn := range []string{
		"func bundleGatherFlow(",
		"func (s *bundleGatherScreen) feedback(",
		"func (s *bundleGatherScreen) tally(",
	} {
		body := funcBody(t, "bundle_flow.go", fn)
		lits := stringLiterals(body)
		if len(lits) == 0 {
			t.Fatalf("%s yielded no string literals, so this guard checks nothing", fn)
		}
		for _, lit := range lits {
			for _, g := range blankingGlyphs {
				if strings.Contains(lit, g) {
					t.Errorf("%s draws %q, which carries %q: the frame renders its "+
						"title and NOTHING else, and uiContains still reports the text "+
						"as present", fn, lit, g)
				}
			}
		}
	}
}

// TestStringLiteralScannerCanSee is the mutation proof for the scanner itself. A
// stringLiterals that silently returned nothing would make every guard above
// vacuous — the false-PASS shape this whole class exists to remove.
func TestStringLiteralScannerCanSee(t *testing.T) {
	src := "func f() {\n\t// a comment with \u2014 in it\n\ts := \"clean\"\n\tt := \"dirty \u2014 here\"\n}"
	got := stringLiterals(src)
	if len(got) != 2 {
		t.Fatalf("scanner found %d literal(s), want 2: %q", len(got), got)
	}
	if strings.Contains(got[0], "\u2014") {
		t.Errorf("scanner reported the clean literal as dirty: %q", got[0])
	}
	if !strings.Contains(got[1], "\u2014") {
		t.Errorf("scanner missed the glyph in %q", got[1])
	}
	// And the COMMENT must not be reported, or the guard fires on prose.
	for _, g := range got {
		if strings.Contains(g, "a comment") {
			t.Errorf("scanner picked up a comment as a literal: %q", g)
		}
	}
}
