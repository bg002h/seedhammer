//go:build !refugium

package gui

import "testing"

// The two whole-flow tests of the gear that reach it through a proof
// composition (ftProofTriggerConst). The proof triggers are compiled out of the
// Refugium build (Refugium plan F7 §4.4, prooftriggers_refugium.go), so these
// two moved here from freetext_settings_test.go and freetext_speed_test.go
// unchanged, and the rest of those files still runs under -tags refugium.

// Moved from freetext_settings_test.go.
// TestFlowCarriesPassesToTheEngraver drives the WHOLE program, picks a pass
// count through the gear, and asserts the plate handed to the engraver
// carries it. Everything else could pass with the value wired to a screen and
// never to the planner -- that exact failure has already happened twice in
// this program, once for speed and once for the gear's own wiring.
func TestFlowCarriesPassesToTheEngraver(t *testing.T) {
	var got Plate
	var seen bool
	freetextEngraveHook = func(p Plate) { got, seen = p, true }
	t.Cleanup(func() { freetextEngraveHook = nil })

	h, r := startFT(t)
	ftPastQR(h, false)
	ftTypeTrigger(h, ftProofTriggerConst)
	ftOK(h)
	h.tapWidget("proofYes")
	h.mustReach("lines")
	loaded := ftKbd(h).Fragment // the pattern the loader wrote, for the baseline

	// Tap the gear: it is a KEY in the grid, so it is tapped through the
	// keyboard's own key bounds, not as a nav button.
	ftTapKey(h, ppSettings)
	ftChoose(h, "settings", 1) // Passes
	ftChoose(h, "passes", 1)   // 2 passes
	h.tapNav(Button1)          // leave settings
	h.mustReach("lines")
	ftOK(h)
	h.mustReach("Title")
	ftOK(h)
	h.mustReach("Footer")
	ftOK(h)
	h.mustReach("Confirm")
	// The invisible-note risk this task exists to close: nothing before this
	// line ever inspected the RENDERED confirm screen, so a call site that
	// silently dropped the pass count (reverting to ftSpeedNote(params,
	// speed)) would still engrave at 2 passes and pass every other assertion
	// here. uiContains lowercases and strips spaces from the needle, and
	// ExtractText never sees the space glyph either (it inks nothing), so
	// "passes: 2" collapses to "passes:2" on both sides. The VALUE is
	// asserted, not mere presence -- a passes+1 bug would show "passes: 3"
	// and this would still fail.
	if !uiContains(h.content, "passes: 2") {
		t.Errorf("the confirm screen does not name the chosen pass count; frame %q", h.content)
	}
	ftOK(h)
	h.step()

	if !seen {
		t.Fatal("the flow never handed a plate to the engraver")
	}
	// THE INDEPENDENT WITNESS, and it has to come before the two baselines
	// below. Everything after this line compares the engraved plate against
	// plates built by ftBuildPlate itself, so a bug INSIDE ftBuildPlate -- the
	// one line that sets fitted.Passes -- cancels out of both sides and is
	// invisible. Mutating it to `fitted.Passes = passes + 1` left all 46
	// packages green (whole-branch review, 2026-08-06): the operator picks 2,
	// the confirm screen says 2, and each glyph is cut 3 times into steel.
	//
	// freetextPlateHook reports the backup.Fitted as EngraveFitted received it,
	// so this reads the value the ENGRAVER was given rather than one this test
	// re-derived. It must precede the ftBuildPlate calls below: those fire the
	// same hook and overwrite r.got.
	if !r.gotPlate {
		t.Fatal("the flow never built a fitted composition")
	}
	if r.got.Passes != 2 {
		t.Errorf("the plate was built with Passes=%d, want the 2 the operator chose", r.got.Passes)
	}
	// The same composition at one pass is the baseline. Capture the loaded
	// pattern from the field itself rather than rebuilding it -- ftProofOutcomeFor
	// is what wrote it, and re-deriving it here would test the test. The title
	// and footer are NOT empty: accepting the proof loads ftProofTitleConst and
	// ftProofFooter into those fields too (ftProofLoader), and both Title and
	// Footer screens were OK'd without retyping, so the engraved plate still
	// carries them -- a baseline built with "" would differ from the real
	// plate in more than the pass count, and durations would diverge for the
	// wrong reason.
	P := h.ctx.Platform.EngraverParams()
	one, err := ftBuildPlate(P, &ftPlanConst, loaded, ftProofTitleConst, ftProofFooter, false, 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	if got.Duration <= one.Duration {
		t.Errorf("two passes planned %d ticks against one pass's %d", got.Duration, one.Duration)
	}
	// The DIRECTION check above passes for ANY pass count above 1 -- 8 passes
	// on a 2-pass choice would still read "more ticks than one pass" and reach
	// steel uncaught. So the plate the operator's 2-pass choice actually
	// produced must match a plate explicitly built at 2 passes, exactly.
	two, err := ftBuildPlate(P, &ftPlanConst, loaded, ftProofTitleConst, ftProofFooter, false, 0, 2)
	if err != nil {
		t.Fatal(err)
	}
	if got.Duration != two.Duration {
		t.Errorf("the plate runs %d ticks, the two-pass composition %d", got.Duration, two.Duration)
	}
}

// Moved from freetext_speed_test.go.
// TestFlowCarriesTheChosenSpeedToTheEngraver drives the WHOLE program and
// asserts the plate handed to the engraver was planned at the feed the operator
// picked. Everything above it tests ftParamsAtSpeed and ftBuildPlate directly,
// which a flow that never called them would pass.
//
// Mutation-tested: dropping ftParamsAtSpeed from the engrave step left the
// entire suite green until this test existed.
func TestFlowCarriesTheChosenSpeedToTheEngraver(t *testing.T) {
	var got Plate
	var seen bool
	freetextEngraveHook = func(p Plate) { got, seen = p, true }
	t.Cleanup(func() { freetextEngraveHook = nil })

	h, _ := startFT(t)
	ftPastQR(h, false)
	// A proof composition is what unlocks the feed.
	ftTypeTrigger(h, ftProofTriggerConst)
	ftOK(h)
	h.tapWidget("proofYes")
	h.mustReach("lines")

	// Pick the slowest rung, which is the furthest from the default, through
	// the gear -- the flow no longer stops on a Speed step of its own.
	ftTapKey(h, ppSettings)
	ftChoose(h, "settings", 0) // Speed
	slowest := len(ftSpeedRungs) - 1
	ftChoose(h, "speed", slowest)
	h.tapNav(Button1) // leave settings
	h.mustReach("lines")
	ftOK(h)
	h.mustReach("Title")
	ftOK(h)
	h.mustReach("Footer")
	ftOK(h)
	h.mustReach("Confirm")
	ftOK(h)
	h.step()

	if !seen {
		t.Fatal("the flow never handed a plate to the engraver")
	}
	P := h.ctx.Platform.EngraverParams()
	want := ftParamsAtSpeed(P, ftSpeedRungs[slowest]).EngravingSpeed
	if got.Conf.EngravingSpeed != want {
		t.Errorf("engraved at %d microsteps/s, want %d (%.1fmm/s) -- the chosen feed did not reach the plate",
			got.Conf.EngravingSpeed, want, ftSpeedRungs[slowest])
	}
	if got.Conf.EngravingSpeed == P.EngravingSpeed {
		t.Errorf("this test needs a feed different from the machine default to mean anything")
	}
}
