package gui

import "testing"

// TestEngraveXpubProgramNavigable asserts the new engraveXpub program is
// reachable by navigating Right from the start screen, is titled, and that the
// start-screen layout does not panic("invalid page") for any navigable program.
func TestEngraveXpubProgramNavigable(t *testing.T) {
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
	// Navigate Right to the new program.
	click(&ctx.Router, Right)
	content, ok = frame()
	if !ok {
		t.Fatal("StartScreen produced no frame after Right")
	}
	if !uiContains(content, "Account Xpub") {
		t.Fatalf("new program not reachable/titled after Right; got %q", content)
	}
	// Navigate Right again reaches engraveBundle; the navigable upper bound is now
	// engraveMultisig (inserted before qaProgram by T6b), so the
	// wrap-to-backupWallet boundary is past engraveMultisig — see
	// TestEngraveMultisigProgramNavigable / TestEngraveSingleSigProgramNavigable.
	click(&ctx.Router, Right)
	content, ok = frame()
	if !ok {
		t.Fatal("StartScreen produced no frame after second Right")
	}
	if !uiContains(content, "Bundle") {
		t.Fatalf("engraveBundle not reachable after second Right; got %q", content)
	}
}
