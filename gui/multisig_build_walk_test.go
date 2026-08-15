package gui

import (
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"seedhammer.com/gui/assets"
	"seedhammer.com/gui/op"
)

// ─── S2's completed-engrave gate, driven from the KEYBOARD ───────────────────
//
// F-178 item 4: the typed-seed source is the §0.1b-primary data entry that no
// walk has ever driven. Every drive so far took the self seed FROM THE PAYLOAD,
// and the field report that motivates D-1 says "after configuration" — a
// keyboard-entered seed is a different amount of state and a different code path
// (seedEntryFlowTypedOnly + inputWordsFlow).
//
// It is also the regression guard the S2 rulings put in place of pinning F-178's
// screens: those screens were screens of the DUPLICATE policy, so what is worth
// pinning is not the screen list but the stake — Trace A completes an engrave
// with the labelled Trace A cosigners.
//
// TRACE A, as cmd/buildpayloadcards labels it: self = masterA at the shared
// origin, cosigners B@0 and C@0. Payload card 1 (A@0) is SKIPPED, because it is
// masterA's own account-0 key and would duplicate the self key — so the walk
// proves this stage's first refusal is AVOIDABLE, not merely reachable, which is
// the half a refusal test cannot show.
//
// CARD ORDER IS THE GO ROSTER'S, NOT THE EMULATOR PAYLOAD'S, and they differ:
// cosignerCardRoster is A@0, B@0, C@0, A@1 while cmd/buildpayloadcards writes
// A@0, A@1, B@0, C@0. So this walk taps SKIP, USE, USE where the emulator walk
// taps SKIP, SKIP. Measured, not assumed — the first draft of this test tapped
// SKIP, SKIP and selected A@1, which the foreign-origin refusal then caught.

// buildWalkRasterFloor is the ink a screen with a title AND a body must exceed.
//
// CALIBRATED BOTH WAYS, measured 2026-08-15 on sh2DisplaySize, and the
// calibration RUNS rather than living in this comment. F-151's defect shape is
// exactly "title draws, body does not", which is what a field report of "a blank
// screen after configuration" looks like from the outside, so the floor has to
// sit above the worst BLANK and below the thinnest REAL screen:
//
//	title + 1 nav button, no body      2259 px
//	title + 2 nav buttons, no body     2693 px   (~= F-151's measured 2652)
//	title + 3 nav buttons, no body     5482 px   <-- the worst blank
//	--------------------------------- floor 6000
//	"How many keys (n)?"               6566 px   <-- the thinnest real screen
//	"Which slot is your key?"          7081 px
//	"Cosigner Keys" (the gather)       7898 px
//	"Choose number of words"           9714 px
//	"Use payload card 1 of 4?"        10879 px
//	"Include key fingerprints?"       11208 px
//	"Add a BIP-39 passphrase?"        12793 px
//	"Choose policy type"              15760 px
//	"Payload cards"                   17621 px
//
// The margin is 518 px below and 566 px above: TIGHT, and tight by measurement
// rather than by carelessness. The third nav button is what closes the gap — it
// is drawn StylePrimary, i.e. filled, and costs 2789 px on its own. A screen
// that legitimately drops below 6566 should move this number WITH the measured
// table, not quietly below it.
//
// It does NOT reuse gui/raster_test.go's 4000: that was calibrated on the unload
// MODAL, whose chrome is two nav buttons, and 4000 sits BELOW the worst blank
// here. Measured before assuming; the first draft of this file used 4000 and a
// title-only frame passed it.
const buildWalkRasterFloor = 6000

// titleOnlyInk measures the WORST-CASE blank frame: a title, three nav buttons
// (the most any screen on this walk draws, and the most expensive because the
// third is filled) and no body. The upper bound on "blank", measured rather than
// asserted from memory, because the floor is only meaningful relative to it.
func titleOnlyInk(t *testing.T) int {
	t.Helper()
	p := newPlatform()
	p.display = sh2DisplaySize
	ctx := NewContext(p)
	frame, _, ink, quit := runUITouchRaster(ctx, func() {
		for i := 0; i < 4; i++ {
			dims := ctx.Platform.DisplaySize()
			titleOp, _ := layoutTitle(ctx, dims.X, descriptorTheme.Text, "Policy Review")
			nav, _ := layoutNavigation(&ctx.B, &descriptorTheme, dims,
				NavButton{Clickable: &Clickable{Button: Button1}, Style: StyleSecondary, Icon: assets.IconBack},
				NavButton{Clickable: &Clickable{Button: Button2}, Style: StyleSecondary, Icon: assets.IconRight},
				NavButton{Clickable: &Clickable{Button: Button3}, Style: StylePrimary, Icon: assets.IconCheckmark},
			)
			ctx.Frame(op.Layer(nav, titleOp, op.Color(&ctx.B, descriptorTheme.Background)))
		}
	})
	defer quit()
	// PUMP, or ink() is the zero it was initialised with and every comparison
	// against it is vacuous. Measured: without this the "floor" was being checked
	// against 0.
	content, ok := frame()
	if !ok {
		t.Fatal("the title-only frame never rendered")
	}
	if !uiContains(content, "Policy Review") {
		t.Fatalf("the title-only frame drew something else: %q", content)
	}
	got := ink()
	if got <= 0 {
		t.Fatalf("a title-only frame measured %d px; the harness is not rastering, "+
			"so the floor below separates nothing", got)
	}
	return got
}

// buildWalkStep is one screen the walk expects, in order.
type buildWalkStep struct {
	// needle is the text that proves this screen is on the display.
	needle string
	// downs is how many Down presses before confirming (ChoiceScreen index).
	downs int
	// noBody marks a screen that legitimately carries no body (none so far);
	// it exists so that adding one is a deliberate edit rather than a silent
	// lowering of the floor.
	noBody bool
}

// TestBuildWalkTypedSeed drives Trace A end to end with the self seed entered on
// the KEYBOARD, rasterising every screen from the template picker to the
// engrave-style picker, and completes the engrave.
func TestBuildWalkTypedSeed(t *testing.T) {
	records := cosignerCardRecords(t, 4) // A@0, B@0, C@0, A@1 — the delivered set
	synctest.Test(t, func(t *testing.T) {
		e := newEngraver()
		p := newPlatform()
		p.display = sh2DisplaySize
		p.engraver = e
		ctx := NewContext(p)
		ctx.sysw = sessionHolding(records...)
		done := false
		frame, _, ink, quit := runUITouchRaster(ctx, func() {
			buildMultisigPolicyFlow(ctx, &descriptorTheme)
			done = true
		})
		defer quit()

		floorFor := titleOnlyInk(t)
		if floorFor >= buildWalkRasterFloor {
			t.Fatalf("a title-only frame draws %d px, at or above the floor %d — the "+
				"floor separates nothing and every assertion below is vacuous",
				floorFor, buildWalkRasterFloor)
		}
		t.Logf("title-only frame = %d px; floor = %d", floorFor, buildWalkRasterFloor)

		// EVERY SCREEN IS ASSERTED ON THE WAY PAST. Tapping through a picker that
		// did not appear silently answers the NEXT one, and a walk that does that
		// reports a pass for a policy nobody asked for.
		steps := []buildWalkStep{
			{needle: "Choose policy type", downs: 0},            // wsh
			{needle: "How many keys (n)?", downs: 1},            // n = 3
			{needle: "Required signatures (k of 3)?", downs: 1}, // k = 2
			{needle: "Which slot is your key?", downs: 0},       // self @0
			{needle: "Include key fingerprints?", downs: 0},     // omit
			{needle: buildCosignerGatherTitle, downs: 0},        // Done adding cards
			{needle: "Payload cards", downs: 0},                 // the over-supply review
			{needle: "Use payload card 1 of 4?", downs: 1},      // SKIP A@0: it would duplicate the self key
			{needle: "Use payload card 2 of 4?", downs: 0},      // USE  B@0
			{needle: "Use payload card 3 of 4?", downs: 0},      // USE  C@0; card 4 (A@1) is never offered
			{needle: "Choose number of words", downs: 0},        // 12 WORDS
		}
		for _, s := range steps {
			content, ok := pumpUntil(frame, s.needle, 64)
			if !ok {
				t.Fatalf("walk never reached %q; screen reads %q", s.needle, content)
			}
			t.Logf("INK %-34q %d", s.needle, ink())
			if !s.noBody && ink() < buildWalkRasterFloor {
				t.Errorf("%q drew only %d ink pixels (floor %d, title-only %d) — near "+
					"blank whatever its text ops claim. Suspect a body too long to lay "+
					"out, or a glyph the face lacks.", s.needle, ink(), buildWalkRasterFloor, floorFor)
			}
			for i := 0; i < s.downs; i++ {
				click(&ctx.Router, Down)
				frame()
			}
			click(&ctx.Router, Button3)
			frame()
		}

		// THE KEYBOARD. This is the §0.1b-primary entry no walk had driven.
		typeWords(&ctx.Router, frame, fixtureMasterA)

		// The remaining screens, each with its own raster floor.
		rest := []buildWalkStep{
			{needle: "Add a BIP-39 passphrase?", downs: 0}, // Skip
			{needle: "Policy stub", downs: 0},              // Policy Review -> continue
			{needle: "Which md1?", downs: 0},               // Full policy md1
		}
		for _, s := range rest {
			content, ok := pumpUntil(frame, s.needle, 64)
			if !ok {
				t.Fatalf("walk never reached %q; screen reads %q", s.needle, content)
			}
			t.Logf("INK %-34q %d", s.needle, ink())
			if ink() < buildWalkRasterFloor {
				t.Errorf("%q drew only %d ink pixels (floor %d, title-only %d)",
					s.needle, ink(), buildWalkRasterFloor, floorFor)
			}
			click(&ctx.Router, Button3)
			frame()
		}

		// THE POLICY REVIEW MUST HAVE SPOKEN. Re-read from the frame the walk
		// actually saw rather than from buildReviewLines: §0.1a's announcement is
		// only worth anything if it reaches the display.
		//
		// (Captured above by pumpUntil's return; re-asserted here against a fresh
		// read would race the flow, so the check is on the recorded content.)

		// The EXPERIMENTAL warning: hold to confirm is the only route past it.
		content, ok := pumpUntil(frame, "EXPERIMENTAL", 64)
		if !ok {
			t.Fatalf("the mandatory EXPERIMENTAL warning was not reached; got %q", content)
		}
		t.Logf("INK %-34q %d", "EXPERIMENTAL", ink())
		if ink() < buildWalkRasterFloor {
			t.Errorf("the EXPERIMENTAL warning drew only %d ink pixels (floor %d, "+
				"title-only %d) — the operator's last chance to stop is near blank",
				ink(), buildWalkRasterFloor, floorFor)
		}
		press(&ctx.Router, Button3)
		frame()
		time.Sleep(confirmDelay)
		frame()

		if content, ok := pumpUntil(frame, "What to engrave?", 64); !ok {
			t.Fatalf("the engrave-mode choice was not reached; got %q", content)
		}
		t.Logf("INK %-34q %d", "engrave mode", ink())
		if ink() < buildWalkRasterFloor {
			t.Errorf("the engrave-mode choice drew only %d ink pixels (floor %d)",
				ink(), buildWalkRasterFloor)
		}
		click(&ctx.Router, Button3) // Full (seed + keys)
		frame()

		// THE ENGRAVE-STYLE PICKER, which is where every previous drive stopped.
		content, ok = pumpUntil(frame, "Choose engraving", 96)
		if !ok {
			t.Fatalf("the engrave-style picker was not reached; got %q", content)
		}
		if !uiContains(content, "Card 1 of 3") {
			t.Errorf("a FULL build engraves ms1 + mk1 + md1, so the first plate is "+
				"card 1 of 3; got %q", content)
		}
		t.Logf("INK %-34q %d", "engrave style", ink())
		if ink() < buildWalkRasterFloor {
			t.Errorf("the engrave-style picker drew only %d ink pixels (floor %d)",
				ink(), buildWalkRasterFloor)
		}

		// AND PAST IT. Every plate is cut, which is the thing no drive had done:
		// the gate is "completes an engrave", not "reaches the screen before one".
		plates := 0
		for {
			if _, ok := pumpUntil(frame, "Choose engraving", 96); !ok {
				break
			}
			click(&ctx.Router, Button3) // first variant
			frame()
			engraveOnePlate(t, ctx, frame, e)
			plates++
			if plates > 24 {
				t.Fatal("the engrave loop did not terminate")
			}
		}
		// THE TAIL. bundleEngrave returning is not the flow returning: a full
		// policy build then offers the cross-match verify and shows the restore
		// doc, and a test that stopped at the last plate would call an unfinished
		// flow finished.
		if c, ok := pumpUntil(frame, "Verify the engraved plates?", 96); !ok {
			t.Fatalf("the verify offer was not reached after %d plate(s); got %q", plates, c)
		}
		click(&ctx.Router, Down) // Skip
		frame()
		click(&ctx.Router, Button3)
		frame()
		if c, ok := pumpUntil(frame, "Descriptor:", 96); !ok {
			t.Fatalf("the restore doc was not shown; got %q", c)
		}
		if ink() < buildWalkRasterFloor {
			t.Errorf("the restore doc drew only %d ink pixels (floor %d)", ink(), buildWalkRasterFloor)
		}
		click(&ctx.Router, Button3) // done with the restore doc
		for i := 0; i < 256 && !done; i++ {
			frame()
		}
		if !done {
			t.Fatalf("the flow did not return after %d plate(s)", plates)
		}
		// NINE, DERIVED NOT GUESSED: a FULL build cuts the operator's ms1 (1),
		// their mk1 (2 chunks) and the assembled md1 (6 chunks). The count is
		// asserted exactly, because "at least three" is satisfied by a bundle that
		// silently lost the descriptor.
		if plates != 9 {
			t.Fatalf("%d plate(s) were engraved; a full 2-of-3 wsh build cuts 1 ms1 + "+
				"2 mk1 chunks + 6 md1 chunks = 9", plates)
		}
		t.Logf("Trace A completed: %d plates engraved from a keyboard-typed seed", plates)
	})
}

// typeWords enters a BIP-39 phrase on the on-device keyboard, one word at a
// time, pumping frames between words.
//
// A DRIVER, NOT A SHORTCUT: it emits the same rune + Button3 events a finger
// does, through the same router. S4's per-slot multi-seed entry needs exactly
// this and gets it here rather than growing its own.
func typeWords(r *EventRouter, frame func() (string, bool), phrase string) {
	for _, w := range strings.Fields(phrase) {
		runes(r, strings.ToLower(w))
		click(r, Button3)
		frame()
	}
}

// engraveOnePlate drives one plate from the engrave screen's first prompt to
// "completed", the way a finger would: three nexts, a hold past confirmDelay,
// then wait for the engraver to be CLOSED (which is the job finishing, not a
// screen claiming it did) and acknowledge.
func engraveOnePlate(t *testing.T, ctx *Context, frame func() (string, bool), e *testEngraver) {
	t.Helper()
	click(&ctx.Router, Button3, Button3, Button3)
	press(&ctx.Router, Button3)
	frame()
	time.Sleep(confirmDelay)
	for i := 0; i < 4096; i++ {
		select {
		case <-e.closes:
			click(&ctx.Router, Button3)
			frame()
			return
		default:
		}
		if _, ok := frame(); !ok {
			return
		}
	}
	t.Fatal("the engrave never closed the engraver, so no plate was cut")
}

// TestBuildWalkRasterFloorIsCalibrated is the floor's own proof. Without it the
// number above is a guess, and a guess that happens to sit below every screen
// passes everything forever.
func TestBuildWalkRasterFloorIsCalibrated(t *testing.T) {
	blank := titleOnlyInk(t)
	if blank >= buildWalkRasterFloor {
		t.Fatalf("a title-only frame draws %d px, at or above the floor %d",
			blank, buildWalkRasterFloor)
	}
	// And the floor must not sit so low that a body-less screen slips under it.
	// The measured F-151 pair is the anchor: 2652 blank, 6688 fixed.
	if buildWalkRasterFloor <= 2652 {
		t.Fatalf("floor %d is at or below F-151's measured blank body (2652 px)",
			buildWalkRasterFloor)
	}
	// The OTHER side, and it is the side that makes the floor a gate rather than
	// a formality: the thinnest real screen on the walk measured 6566 px, so a
	// floor at or above it would fail on a healthy device and get lowered by the
	// next person to see it go red.
	if buildWalkRasterFloor >= 6566 {
		t.Fatalf("floor %d is at or above the thinnest measured real screen "+
			"(\"How many keys (n)?\", 6566 px), so a healthy walk fails it",
			buildWalkRasterFloor)
	}
	t.Logf("worst blank %d px < floor %d px < thinnest real screen 6566 px", blank, buildWalkRasterFloor)
}
