package gui

import (
	"strings"
	"testing"
)

// The Clear button exists because the Text field is UNCAPPED: a loaded proof
// pattern runs to several hundred characters and the operator cleared one by
// backspace, over 300 taps, to run a speed test. It is prompted because the
// field is the only copy of the composition until the plate is built.

// TestClearAsksFirstAndKeepsTheTextOnNo. The default answer must be the
// harmless one -- "Keep the text" is index 0, and ChoiceScreen starts there.
func TestClearAsksFirstAndKeepsTheTextOnNo(t *testing.T) {
	const text = "the wallet is in the safe and the PIN is not written down"
	h, _ := startFT(t)
	ftPastQR(h, false)
	ftSetText(h, text)

	h.tapWidget("clear")
	h.mustReach("Clear")
	if !uiContains(h.content, "Keepthetext") {
		t.Errorf("the prompt does not offer to keep the text; frame %q", h.content)
	}
	// Index 0 is "Keep the text".
	ftChoose(h, "clearPrompt", 0)
	h.mustReach("lines")
	if got := ftKbd(h).Fragment; got != text {
		t.Errorf("declining the prompt changed the field to %q", got)
	}
}

// TestClearEmptiesTheFieldOnYes.
func TestClearEmptiesTheFieldOnYes(t *testing.T) {
	h, _ := startFT(t)
	ftPastQR(h, false)
	ftSetText(h, strings.Repeat("x", 300))

	h.tapWidget("clear")
	h.mustReach("Clear")
	ftChoose(h, "clearPrompt", 1)
	h.mustReach("lines")
	if got := ftKbd(h).Fragment; got != "" {
		t.Errorf("the field still holds %d characters after clearing", len(got))
	}
}

// TestClearIsNotOfferedOnAnEmptyField: the screen must not present an action
// that would do nothing, and Back/Clear/OK is the WHOLE nav budget --
// layoutNavigation indexes a fixed [3]int by Button-Button1, so a fourth
// affordance panics rather than laying out badly.
func TestClearIsNotOfferedOnAnEmptyField(t *testing.T) {
	h, _ := startFT(t)
	ftPastQR(h, false)
	if got := ftKbd(h).Fragment; got != "" {
		t.Fatalf("this test needs an empty field; got %q", got)
	}
	clear, ok := h.widget("clear").(*Clickable)
	if !ok {
		t.Fatal(`widget "clear" is not a *Clickable`)
	}
	if _, drawn := h.drawer().TagBounds(clear); drawn {
		t.Error("Clear is drawn over an empty field")
	}
	// And it appears as soon as there is something to clear.
	ftSetText(h, "x")
	if _, drawn := h.drawer().TagBounds(clear); !drawn {
		t.Error("Clear is not drawn over a non-empty field")
	}
}

// TestClearPromptDefaultsToKeeping pins the ORDER, not just the behaviour.
// ChoiceScreen starts at index 0, so the safe answer being first is the only
// thing making the default harmless -- nothing else states it, and a reorder
// would silently make "Clear it" the pre-selected answer on a screen whose whole
// job is to prevent an accidental discard.
func TestClearPromptDefaultsToKeeping(t *testing.T) {
	h, _ := startFT(t)
	ftPastQR(h, false)
	ftSetText(h, "some text worth keeping")
	h.tapWidget("clear")
	h.mustReach("Clear")

	cs, ok := h.widget("clearPrompt").(*ChoiceScreen)
	if !ok {
		t.Fatal(`widget "clearPrompt" is not a *ChoiceScreen`)
	}
	if len(cs.Choices) != 2 {
		t.Fatalf("the prompt offers %d choices, want 2", len(cs.Choices))
	}
	if !strings.Contains(strings.ToLower(cs.Choices[0]), "keep") {
		t.Errorf("index 0 is %q; the PRE-SELECTED answer must be the harmless one", cs.Choices[0])
	}
	if cs.choice != 0 {
		t.Errorf("the prompt opens on choice %d, want 0", cs.choice)
	}
}
