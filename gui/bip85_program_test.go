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
	// Right x6 -> engraveMultisig (engravePassphrase is position 2 and
	// engraveText position 3).
	for i := 0; i < 6; i++ {
		click(&ctx.Router, Right)
		content, ok = frame()
		if !ok {
			t.Fatalf("no frame after Right #%d", i+1)
		}
	}
	if !uiContains(content, "Multisig") {
		t.Fatalf("engraveMultisig not reachable after 6 Rights; got %q", content)
	}
	// Right -> walletPolicy, which sits beside the other engrave programs.
	click(&ctx.Router, Right)
	content, ok = frame()
	if !ok {
		t.Fatal("no frame after the Right onto Wallet Policy")
	}
	if !uiContains(content, "Wallet Policy") {
		t.Fatalf("walletPolicy not reachable after Multisig; got %q", content)
	}
	// Right -> engraveTransaction, inserted beside the other engrave programs.
	click(&ctx.Router, Right)
	content, ok = frame()
	if !ok {
		t.Fatal("no frame after the Right onto Engrave Transaction")
	}
	if !uiContains(content, "Engrave Transaction") {
		t.Fatalf("engraveTransaction not reachable after Wallet Policy; got %q", content)
	}
	// Right -> loadPayload, which sits between engraveTransaction and
	// bip85Derive: it is unconditional, so it was inserted mid-enum and
	// bip85Derive stayed the bound lastNav returns.
	click(&ctx.Router, Right)
	content, ok = frame()
	if !ok {
		t.Fatal("no frame after the Right onto Load Payload")
	}
	if !uiContains(content, "Load Payload") {
		t.Fatalf("loadPayload not reachable between Multisig and BIP-85; got %q", content)
	}
	// Right -> bip85Derive (the new upper bound), titled non-blank.
	click(&ctx.Router, Right)
	content, ok = frame()
	if !ok {
		t.Fatal("no frame after sixth Right")
	}
	if !uiContains(content, "BIP-85") {
		t.Fatalf("bip85Derive not reachable/titled after sixth Right; got %q", content)
	}
	// Right again wraps to backupWallet.
	click(&ctx.Router, Right)
	content, ok = frame()
	if !ok {
		t.Fatal("no frame after seventh Right")
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
