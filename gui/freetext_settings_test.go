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
