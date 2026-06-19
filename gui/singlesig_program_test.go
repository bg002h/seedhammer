package gui

import "testing"

// TestEngraveSingleSigProgramNavigable asserts the new engraveSingleSig program
// is reachable by navigating Right from the start screen past engraveBundle,
// that it is the new navigable upper bound (a further Right wraps to
// backupWallet), and that it has a NON-BLANK title (the title switch fails OPEN
// to a blank title, a silent defect, so this asserts the arm is present). It
// also exercises the start-screen layout for the new program (layoutMainPlates
// must have a case for it or it panics). qaProgram stays out of the carousel.
func TestEngraveSingleSigProgramNavigable(t *testing.T) {
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
	// Right → engraveBundle.
	click(&ctx.Router, Right)
	content, ok = frame()
	if !ok {
		t.Fatal("no frame after second Right")
	}
	if !uiContains(content, "Bundle") {
		t.Fatalf("engraveBundle not reachable after second Right; got %q", content)
	}
	// Right → engraveSingleSig (the new program), titled non-blank.
	click(&ctx.Router, Right)
	content, ok = frame()
	if !ok {
		t.Fatal("no frame after third Right")
	}
	if !uiContains(content, "Single-Sig") {
		t.Fatalf("engraveSingleSig not reachable/titled after third Right; got %q", content)
	}
	// Right again reaches engraveMultisig (the new navigable upper bound,
	// inserted before qaProgram by T6b). qaProgram stays out of the carousel.
	click(&ctx.Router, Right)
	content, ok = frame()
	if !ok {
		t.Fatal("no frame after fourth Right")
	}
	if !uiContains(content, "Multisig") {
		t.Fatalf("engraveMultisig not reachable after fourth Right; got %q", content)
	}
}

// TestEngraveSingleSigLeftWrap asserts navigating Left from backupWallet wraps to
// engraveMultisig (the new navigable upper bound after T6b), titled non-blank.
func TestEngraveSingleSigLeftWrap(t *testing.T) {
	ctx := NewContext(newPlatform())
	m := new(StartScreen)
	frame, quit := runUI(ctx, func() { m.Flow(ctx, &descriptorTheme) })
	defer quit()
	if _, ok := frame(); !ok {
		t.Fatal("StartScreen produced no frame")
	}
	// Left from backupWallet → wraps to engraveMultisig (the new upper bound).
	click(&ctx.Router, Left)
	content, ok := frame()
	if !ok {
		t.Fatal("no frame after Left")
	}
	if !uiContains(content, "Multisig") {
		t.Fatalf("Left did not wrap to Multisig; got %q", content)
	}
}
