package gui

import "testing"

// TestEngraveMultisigProgramNavigable asserts the new engraveMultisig program is
// reachable by navigating Right past engraveSingleSig, is the new navigable upper
// bound (a further Right wraps to backupWallet), has a NON-BLANK title, and does
// not panic on render (layoutMainPlates must have its case). qaProgram stays out.
func TestEngraveMultisigProgramNavigable(t *testing.T) {
	ctx := NewContext(newPlatform())
	m := new(StartScreen)
	frame, quit := runUI(ctx, func() { m.Flow(ctx, &descriptorTheme) })
	defer quit()
	content, ok := frame()
	if !ok {
		t.Fatal("StartScreen produced no frame")
	}
	if !uiContains(content, "Backup Wallet") {
		t.Fatalf("initial program not Backup Wallet; got %q", content)
	}
	// Right x3 -> engraveSingleSig.
	for i := 0; i < 3; i++ {
		click(&ctx.Router, Right)
		content, ok = frame()
		if !ok {
			t.Fatalf("no frame after Right #%d", i+1)
		}
	}
	if !uiContains(content, "Single-Sig") {
		t.Fatalf("engraveSingleSig not reachable after 3 Rights; got %q", content)
	}
	// Right -> engraveMultisig (the new upper bound), titled non-blank.
	click(&ctx.Router, Right)
	content, ok = frame()
	if !ok {
		t.Fatal("no frame after fourth Right")
	}
	if !uiContains(content, "Multisig") {
		t.Fatalf("engraveMultisig not reachable/titled after fourth Right; got %q", content)
	}
	// Right again wraps to backupWallet.
	click(&ctx.Router, Right)
	content, ok = frame()
	if !ok {
		t.Fatal("no frame after fifth Right")
	}
	if !uiContains(content, "Backup Wallet") {
		t.Fatalf("Right did not wrap to Backup Wallet; got %q", content)
	}
}

// TestEngraveMultisigLeftWrap asserts Left from backupWallet wraps to
// engraveMultisig (the new navigable upper bound).
func TestEngraveMultisigLeftWrap(t *testing.T) {
	ctx := NewContext(newPlatform())
	m := new(StartScreen)
	frame, quit := runUI(ctx, func() { m.Flow(ctx, &descriptorTheme) })
	defer quit()
	if _, ok := frame(); !ok {
		t.Fatal("StartScreen produced no frame")
	}
	click(&ctx.Router, Left)
	content, ok := frame()
	if !ok {
		t.Fatal("no frame after Left")
	}
	if !uiContains(content, "Multisig") {
		t.Fatalf("Left did not wrap to Multisig; got %q", content)
	}
}
