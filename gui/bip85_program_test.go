package gui

import "testing"

// TestBip85DeriveProgramNavigable asserts the new bip85Derive program is reachable
// by navigating Right past engraveMultisig, is the new navigable upper bound (a
// further Right wraps to backupWallet), has a NON-BLANK title, and does not panic
// on render (layoutMainPlates must have its case). qaProgram stays out.
func TestBip85DeriveProgramNavigable(t *testing.T) {
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
	// Right x4 -> engraveMultisig.
	for i := 0; i < 4; i++ {
		click(&ctx.Router, Right)
		content, ok = frame()
		if !ok {
			t.Fatalf("no frame after Right #%d", i+1)
		}
	}
	if !uiContains(content, "Multisig") {
		t.Fatalf("engraveMultisig not reachable after 4 Rights; got %q", content)
	}
	// Right -> bip85Derive (the new upper bound), titled non-blank.
	click(&ctx.Router, Right)
	content, ok = frame()
	if !ok {
		t.Fatal("no frame after fifth Right")
	}
	if !uiContains(content, "BIP-85") {
		t.Fatalf("bip85Derive not reachable/titled after fifth Right; got %q", content)
	}
	// Right again wraps to backupWallet.
	click(&ctx.Router, Right)
	content, ok = frame()
	if !ok {
		t.Fatal("no frame after sixth Right")
	}
	if !uiContains(content, "Backup Wallet") {
		t.Fatalf("Right did not wrap to Backup Wallet; got %q", content)
	}
}

// TestBip85DeriveLeftWrap asserts Left from backupWallet wraps to bip85Derive (the
// new navigable upper bound).
func TestBip85DeriveLeftWrap(t *testing.T) {
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
	if !uiContains(content, "BIP-85") {
		t.Fatalf("Left did not wrap to BIP-85; got %q", content)
	}
}
