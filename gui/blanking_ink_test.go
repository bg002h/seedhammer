package gui

import (
	"strings"
	"testing"

	"seedhammer.com/sysw"
)

// ─── S3b: the two worst-case refusals, judged by INK ──────────────────────────
//
// TestProductionStringsAreDrawable is a SOURCE lookup: it proves no production
// literal carries a rune the faces lack. This file proves the other half, on the
// two screens where a blank body is the worst possible outcome, by measuring
// pixels — because the failure mode is precisely that the text is present in the
// op tree and absent from the glass, and no content assertion can tell those
// apart (gui/raster_test.go:11).
//
// The two screens:
//
//  1. PAYLOAD WARNINGS carrying BOTH F1 lines. This is the worst case rather
//     than the obvious one: the em-dash sat in the "could not confirm" sentence,
//     while "A SECRET is stored unencrypted in flash." is clean ASCII and sits in
//     the SAME body. Blanking is per-BODY, not per-line, so the broken sentence
//     took the clean secret warning down with it. An operator learning a seed is
//     sitting unencrypted in flash is the single most consequential sentence this
//     device draws, and it was collateral damage from a punctuation mark.
//
//  2. THE NFC NO-INTEGRITY ACCEPT, the sentence that tells an operator nothing
//     stands behind a tag's contents. It is the last thing said before a secret
//     from an unauthenticated source is used.

// bodyInkMargin is how far above a body-less frame a real body must sit.
//
// Measured on this build: the worst title-only frame is 5482 px (three nav
// buttons, the third filled) and the thinnest REAL screen in the package is 6566
// px, so 500 fits between them with room on both sides. It is a margin rather
// than an absolute floor so that it keeps its meaning if the chrome changes.
const bodyInkMargin = 500

// confirmReviewInk renders confirmReviewScreen with `lines` and returns the ink
// of its first frame.
//
// FIRST FRAME, AND NO pumpUntil. pumpUntil advances by uiContains, which returns
// true on the blank frame this test exists to catch; using it here would make the
// measurement conditional on the very assertion being disproven. The screen draws
// synchronously on entry, so frame one is the screen.
func confirmReviewInk(t *testing.T, title string, lines []string) int {
	t.Helper()
	p := newPlatform()
	p.display = sh2DisplaySize
	ctx := NewContext(p)
	frame, _, ink, quit := runUITouchRaster(ctx, func() {
		confirmReviewScreen(ctx, &descriptorTheme, title, lines)
	})
	defer quit()
	if _, ok := frame(); !ok {
		t.Fatalf("%q never rendered a frame", title)
	}
	got := ink()
	if got <= 0 {
		t.Fatalf("%q measured %d px; the harness is not rastering, so every "+
			"comparison against this is vacuous", title, got)
	}
	return got
}

// TestPayloadWarningsBodyIsActuallyDrawn is worst case (1).
func TestPayloadWarningsBodyIsActuallyDrawn(t *testing.T) {
	// A seed AND a lone md1 chunk: the seed raises the plain F1 line, the lone
	// chunk raises the "could not confirm" one, so both are in one body. The
	// lines come from PRODUCTION (syswLoadWarnings), never from a copy in this
	// test, so a future edit to the sentence is covered without touching here.
	var s syswSession
	s.load(&sysw.Payload{Public: []string{gSeed, gMD1A}}, [32]byte{}, false, true, true, true)
	lines := syswLoadWarnings(&s)
	if len(lines) < 2 {
		t.Fatalf("INCONCLUSIVE: expected both F1 lines, got %d: %q", len(lines), lines)
	}
	// Sanity on the fixture, by SUBSTRING over the source strings rather than
	// over a frame: this is not a content assertion about what was drawn.
	joined := strings.Join(lines, " ")
	if !strings.Contains(joined, "could not confirm") || !strings.Contains(joined, "SECRET") {
		t.Fatalf("INCONCLUSIVE: the two F1 causes are not both present: %q", lines)
	}

	blank := titleOnlyInk(t)
	got := confirmReviewInk(t, "Payload Warnings", lines)
	t.Logf("Payload Warnings body ink = %d px (worst body-less frame %d, margin %d)",
		got, blank, bodyInkMargin)
	if got < blank+bodyInkMargin {
		t.Errorf("the Payload Warnings body drew %d ink pixels against a body-less "+
			"frame of %d: the operator is told NOTHING, on the screen that says a "+
			"secret is sitting unencrypted in flash. Lines: %q", got, blank, lines)
	}
}

// TestNFCNoIntegrityBodyIsActuallyDrawn is worst case (2).
func TestNFCNoIntegrityBodyIsActuallyDrawn(t *testing.T) {
	// syswSourceAccept builds the body from syswFlags; a secret class arriving
	// over NFC is what raises flagNFCNoIntegrity.
	p := newPlatform()
	p.display = sh2DisplaySize
	ctx := NewContext(p)
	ctx.sysw = &syswSession{}
	frame, _, ink, quit := runUITouchRaster(ctx, func() {
		syswSourceAccept(ctx, &descriptorTheme, "Scanned Secret", sysw.ClassMnemonic, srcNFC)
	})
	defer quit()
	if _, ok := frame(); !ok {
		t.Fatal("the NFC accept screen never rendered a frame")
	}
	got := ink()
	blank := titleOnlyInk(t)
	t.Logf("NFC no-integrity body ink = %d px (worst body-less frame %d, margin %d)",
		got, blank, bodyInkMargin)
	if got < blank+bodyInkMargin {
		t.Errorf("the NFC no-integrity accept drew %d ink pixels against a body-less "+
			"frame of %d: the one sentence warning that nothing stands behind a tag's "+
			"contents was not drawn at all", got, blank)
	}
}

// TestBuildWalkRasterFloorClearsTheWorstBlank is F-183's other half.
//
// buildWalkRasterFloor is a measured CONSTANT with its calibration table in the
// comment above it. A constant is fine; a constant nobody re-measures is F-163's
// construct. This asserts it still sits above the worst body-less frame the
// layout can draw, so a chrome change that lifts the blank above the floor turns
// this red instead of silently making every floor check vacuous.
func TestBuildWalkRasterFloorClearsTheWorstBlank(t *testing.T) {
	blank := titleOnlyInk(t)
	t.Logf("worst body-less frame = %d px, buildWalkRasterFloor = %d", blank, buildWalkRasterFloor)
	if buildWalkRasterFloor <= blank {
		t.Errorf("buildWalkRasterFloor is %d but a title-only frame draws %d: every "+
			"test using this floor would pass a completely blank body",
			buildWalkRasterFloor, blank)
	}
}
