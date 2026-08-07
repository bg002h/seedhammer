package gui

import "testing"

// The Passes and Engraving-settings screens are tested through their option
// builders and through the flow itself, never through a label -- the same
// idiom freetext_speed_test.go uses for Speed.

func TestPassOptionsAreTheAgreedRungs(t *testing.T) {
	want := []int{1, 2, 3, 4, 5, 8}
	labels, got := ftPassOptions(true, 0)
	if len(got) != len(want) {
		t.Fatalf("the Passes screen offers %d entries, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("entry %d offers %d passes, want %d", i, got[i], want[i])
		}
	}
	if got[0] != 1 {
		t.Errorf("index 0 offers %d passes; index 0 IS the default and must be 1", got[0])
	}
	// ChoiceScreen does not scroll and op.Layer draws content OVER the title, so
	// a list past roughly seven entries is silently covered rather than clipped.
	if len(labels) > 7 {
		t.Errorf("%d entries will silently overdraw ChoiceScreen's title", len(labels))
	}
}

func TestPassesLockedWithoutAProof(t *testing.T) {
	_, got := ftPassOptions(false, 0)
	if len(got) != 1 {
		t.Fatalf("the Passes screen offers %d entries with no proof loaded, want 1", len(got))
	}
	if got[0] != 0 {
		t.Errorf("taking the only entry set passes to %d, want it left alone", got[0])
	}
}

// TestSettingsScreenPinnedAtTwoEntries pins the gear's top level at exactly the
// two rows it ships with today. ChoiceScreen does not scroll and op.Layer draws
// content OVER its own title past roughly seven entries, so a third row added
// here without thought would be silently covered rather than clipped -- this
// test is what makes that addition think twice.
func TestSettingsScreenPinnedAtTwoEntries(t *testing.T) {
	h := newPPHarness(t)
	speed := float32(0)
	passes := 0
	h.start(func() {
		ftSettingsFlow(h.ctx, &descriptorTheme, h.ctx.Platform.EngraverParams(), true, &speed, &passes)
	})
	h.mustReach("Engraving")
	cs, ok := h.widget("settings").(*ChoiceScreen)
	if !ok {
		t.Fatal(`widget "settings" is not a *ChoiceScreen`)
	}
	if len(cs.Choices) != 2 {
		t.Errorf("the settings screen offers %d entries, want 2 (Speed, Passes)", len(cs.Choices))
	}
	if len(cs.Choices) > 7 {
		t.Errorf("%d entries will silently overdraw ChoiceScreen's title", len(cs.Choices))
	}
	h.tapNav(Button1) // Back out, so t.Cleanup's quit doesn't race a live goroutine.
}

// TestSettingsFlowLoopsAndCarriesChosenValues drives the two-level flow itself
// -- not just the option builders -- so a break in the LOOP or in the pointer
// wiring fails here even though every option-builder test above still passes.
// Picking a value must return to the settings list rather than leaving it, and
// only Back at the top level may leave.
func TestSettingsFlowLoopsAndCarriesChosenValues(t *testing.T) {
	h := newPPHarness(t)
	speed := float32(0)
	passes := 0
	var done bool
	h.start(func() {
		ftSettingsFlow(h.ctx, &descriptorTheme, h.ctx.Platform.EngraverParams(), true, &speed, &passes)
		done = true
	})

	h.mustReach("Engraving")
	ftChoose(h, "settings", 0) // Speed row.
	h.mustReach("Speed")
	slowest := len(ftSpeedRungs) - 1
	ftChoose(h, "speed", slowest)

	h.mustReach("Engraving")
	if done {
		t.Fatal("choosing a value left the settings flow; it must loop back to the list")
	}
	if speed != ftSpeedRungs[slowest] {
		t.Errorf("settings carried speed %v out, want %v", speed, ftSpeedRungs[slowest])
	}

	ftChoose(h, "settings", 1) // Passes row.
	h.mustReach("Passes")
	most := len(ftPassRungs) - 1
	ftChoose(h, "passes", most)

	h.mustReach("Engraving")
	if done {
		t.Fatal("choosing a value left the settings flow; it must loop back to the list")
	}
	if passes != ftPassRungs[most] {
		t.Errorf("settings carried passes %d out, want %d", passes, ftPassRungs[most])
	}

	h.tapNav(Button1) // Back at the top level.
	if !done {
		t.Error("Back at the settings list did not leave ftSettingsFlow")
	}
}

// TestSettingsFlowLocksPassesAndSpeedWithoutAProof: the gate on both sub-screens
// is proofLoaded, not the plan (see ftSpeedOptions and ftPassOptions), so a
// caller that reaches the gear off an ordinary plate must not be able to move
// either value.
func TestSettingsFlowLocksPassesAndSpeedWithoutAProof(t *testing.T) {
	h := newPPHarness(t)
	speed := float32(0)
	passes := 0
	h.start(func() {
		ftSettingsFlow(h.ctx, &descriptorTheme, h.ctx.Platform.EngraverParams(), false, &speed, &passes)
	})

	h.mustReach("Engraving")
	ftChoose(h, "settings", 0)
	h.mustReach("Speed")
	if cs, ok := h.widget("speed").(*ChoiceScreen); !ok || len(cs.Choices) != 1 {
		t.Errorf("the Speed screen offered more than one entry with no proof loaded")
	}
	ftChoose(h, "speed", 0)

	h.mustReach("Engraving")
	ftChoose(h, "settings", 1)
	h.mustReach("Passes")
	if cs, ok := h.widget("passes").(*ChoiceScreen); !ok || len(cs.Choices) != 1 {
		t.Errorf("the Passes screen offered more than one entry with no proof loaded")
	}
	ftChoose(h, "passes", 0)

	h.mustReach("Engraving")
	if speed != 0 {
		t.Errorf("taking the only Speed entry set speed to %v, want it left alone", speed)
	}
	if passes != 0 {
		t.Errorf("taking the only Passes entry set passes to %d, want it left alone", passes)
	}
	h.tapNav(Button1)
}

// TestGearIsOnTheTextKeyboardOnly and TestGearIsNotOnThePassphraseKeyboard pin
// where the gear key lives: on the free-text keyboard, and NEVER on the
// passphrase keyboard -- offering engraving settings while a passphrase is
// being typed is a defect (see newPPKeyboard's settings parameter).
func TestGearIsOnTheTextKeyboardOnly(t *testing.T) {
	h, _ := startFT(t)
	ftPastQR(h, false)
	if !ftHasKey(h, ppSettings) {
		t.Error("the text keyboard has no gear key")
	}
}

func TestGearIsNotOnThePassphraseKeyboard(t *testing.T) {
	h := newPPHarness(t)
	h.start(func() { engravePassphraseFlow(h.ctx, &descriptorTheme) })
	h.mustReach("Passphrase")
	if ftHasKey(h, ppSettings) {
		t.Error("the passphrase keyboard offers engraving settings")
	}
}

// ftHasKey reports whether the ACTIVE page's grid carries an action.
func ftHasKey(h *ppHarness, a ppAction) bool {
	kbd, ok := h.widget("kbd").(*PassphraseKeyboard)
	if !ok {
		return false
	}
	// keys() returns [][]ppKey -- rows of keys, not a flat slice
	// (passphrase_keyboard.go:246).
	for _, row := range kbd.keys() {
		for _, k := range row {
			if k.action == a {
				return true
			}
		}
	}
	return false
}

// TestGearOpensSettingsByTouch is the WIRING half that TestGearIsOnTheText-
// KeyboardOnly does not cover. That test (and every structural test in this
// file) only checks that the key exists in the grid, drawn and reachable, or
// drives ftSettingsFlow directly -- neither notices a keyboard whose gear key
// is present but does nothing when pressed. Either of these would leave every
// other test in the package green:
//
//   - deleting the `if kbd.Settings() { ftSettingsFlow(...); continue }`
//     block from ftTextEntryFlow
//   - deleting `case ppSettings: k.settingsReq = true` from commit()
//
// So this test taps the drawn key -- via its Clickable tag, exactly as
// TestTextKeyboardEveryKeyReachableByTouch and TestNewlineKeyTypesANewline-
// ByTouch do, never a synthesized button event -- and checks the flow
// actually reacts: it reaches the settings screen, Back returns to the SAME
// Text screen with the field untouched, and the latch does not immediately
// reopen the screen it was just consumed to open.
func TestGearOpensSettingsByTouch(t *testing.T) {
	h, _ := startFT(t)
	ftPastQR(h, false)
	ftSetText(h, "abc")

	kbd := ftKbd(h)
	gear := ppTagFor(kbd, func(k ppKey) bool { return k.action == ppSettings })
	if gear == nil {
		t.Fatal("no settings key on the current page")
	}
	h.tapAt(h.point(gear, "settings key"))
	h.next("after tapping the settings key")
	h.mustReach("Engraving")

	// Back out at the settings screen's top level -- ftSettingsFlow's own
	// Back, not the Text screen's.
	h.tapNav(Button1)
	h.mustReach("lines") // back on the Text screen's counter readout

	if got := ftKbd(h).Fragment; got != "abc" {
		t.Errorf("Fragment = %q after a settings round trip, want unchanged %q", got, "abc")
	}

	// SINGLE-SHOT: Settings() clears the latch when it is consumed, so the
	// very next frame must not reopen Engraving on its own.
	c := h.next("after returning from settings")
	if uiContains(c, "Engraving") {
		t.Error("the settings screen reopened on its own after Back -- the gear latch is not single-shot")
	}
}

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

	h, _ := startFT(t)
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
	ftOK(h)
	h.step()

	if !seen {
		t.Fatal("the flow never handed a plate to the engraver")
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
}

// ftTapKey taps the ACTIVE page's key carrying the given action. h.point fails
// if it is undrawn, off-panel or covered -- which is what a gear appended past
// the grid's right edge would be.
func ftTapKey(h *ppHarness, a ppAction) {
	h.t.Helper()
	kbd, ok := h.widget("kbd").(*PassphraseKeyboard)
	if !ok {
		h.t.Fatal(`widget "kbd" is not a *PassphraseKeyboard`)
	}
	for i := range kbd.keys() {
		for j := range kbd.keys()[i] {
			if k := &kbd.keys()[i][j]; k.action == a {
				h.tapAt(h.point(&k.clk, "keyboard key"))
				h.next("after tapping the %v key", a)
				return
			}
		}
	}
	h.t.Fatalf("no key with action %v on the active page", a)
}
