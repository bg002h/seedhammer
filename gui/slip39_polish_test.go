package gui

import (
	"testing"

	slip39words "seedhammer.com/slip39"
)

const slip39Duckling = "duckling enlarge academic academic agency result length solution fridge kidney coal piece deal husband erode duke ajar critical decision keyboard"

func TestConfirmSLIP39Render(t *testing.T) {
	s, err := slip39words.ParseShare(slip39Duckling)
	if err != nil {
		t.Fatalf("ParseShare: %v", err)
	}
	ctx := NewContext(newPlatform())
	frame, quit := runUI(ctx, func() { confirmSLIP39Flow(ctx, &descriptorTheme, s) })
	defer quit()
	c, ok := frame()
	if !ok {
		t.Fatal("no frame")
	}
	if !uiContains(c, "id 7945") {
		t.Errorf("confirm should show id 7945; got %q", c)
	}
	if !uiContains(c, "member 1 of 1") {
		t.Errorf("confirm should show member 1 of 1; got %q", c)
	}
}

func TestEngraveSLIP39BackoutRecognized(t *testing.T) {
	s, err := slip39words.ParseShare(slip39Duckling)
	if err != nil {
		t.Fatalf("ParseShare: %v", err)
	}
	ctx := NewContext(newPlatform())
	click(&ctx.Router, Button1) // Back at the confirm screen
	if !engraveObjectFlow(ctx, &descriptorTheme, s) {
		t.Error("cancel at SLIP-39 confirm returned false (→ \"Unknown format\"); want true (recognized)")
	}
}
