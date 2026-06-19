package gui

import "testing"

// TestEngraveBundleProgramNavigable asserts the new engraveBundle program is
// reachable by navigating Right from the start screen past engraveXpub, that it
// is the new navigable upper bound (a further Right wraps to backupWallet), and
// that it has a NON-BLANK title (R0-I-B: the title switch fails OPEN to a blank
// title, a silent defect, so this asserts the arm is present). It also exercises
// the start-screen layout for the new program (layoutMainPlates must have a case
// or it panics, R0-I-A).
func TestEngraveBundleProgramNavigable(t *testing.T) {
	ctx := NewContext(newPlatform())
	m := new(StartScreen)
	frame, quit := runUI(ctx, func() { m.Flow(ctx, &descriptorTheme) })
	defer quit()
	// Initial frame: backupWallet program.
	content, ok := frame()
	if !ok {
		t.Fatal("StartScreen produced no frame")
	}
	if !uiContains(content, "Backup Wallet") {
		t.Fatalf("initial program not Backup Wallet; got %q", content)
	}
	// Right → engraveXpub.
	click(&ctx.Router, Right)
	content, ok = frame()
	if !ok {
		t.Fatal("no frame after Right")
	}
	if !uiContains(content, "Account Xpub") {
		t.Fatalf("engraveXpub not reachable after Right; got %q", content)
	}
	// Right → engraveBundle (the new program), titled non-blank.
	click(&ctx.Router, Right)
	content, ok = frame()
	if !ok {
		t.Fatal("no frame after second Right")
	}
	if !uiContains(content, "Bundle") {
		t.Fatalf("engraveBundle not reachable/titled after second Right; got %q", content)
	}
	// Right again wraps back to backupWallet (engraveBundle is the navigable
	// upper bound; qaProgram stays out).
	click(&ctx.Router, Right)
	content, ok = frame()
	if !ok {
		t.Fatal("no frame after third Right")
	}
	if !uiContains(content, "Backup Wallet") {
		t.Fatalf("Right did not wrap to Backup Wallet; got %q", content)
	}
}
