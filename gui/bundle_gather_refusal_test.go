package gui

import (
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

		// template wsh -> n=3 -> k=2 -> @0 -> no further slots -> fp omit.
		// (S5's @S picker is multi-select; the "another slot?" default row is
		// "NO, THAT IS ALL", so the held set stays {@0}.)
		for i, downs := range []int{0, 1, 1, 0, 0, 0} {
			stage := []string{"Choose policy type", "How many keys (n)?",
				"Required signatures (k of 3)?", "Which slot is your key?",
				"Do you hold another slot?", "Include key fingerprints?"}[i]
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

// The blocklist that used to live here is GONE, and its replacement is
// gui/font_coverage_test.go (S3b).
//
// It was `blankingGlyphs`, seven runes measured one at a time, checked against
// the string literals of three named functions. Two later inventories built the
// same way came back 27 and 21 sites and both missed four whole classes -- the
// checkmark, the ellipsis, the arrow and the keyboard sentinel -- because a list
// of the runes somebody has already been bitten by cannot catch the next one.
//
// font/bitmap indexes glyphs by rune up to unicode.MaxASCII, so the real
// predicate is "does any face have this glyph", and asking the face needs no
// list at all. TestProductionStringsAreDrawable asks it of EVERY production
// string literal in gui/*.go, so this file's three functions are covered as a
// consequence rather than by being named.
//
// The drive test above stays: it is the only thing here that proves a REACHED
// screen draws ink, which a source lookup cannot do.
