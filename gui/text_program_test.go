package gui

import (
	"image"
	"testing"
)

// ftProgramTitles is the pager in order with NO payload present: eight
// programs, with Engrave Text third -- inserted after BIP-39 Password (spec
// 7.2), never appended, because the last navigable program is the wrap and
// pager bound, and inserting earlier leaves those sites untouched.
//
// Since Phase B1 the last navigable program is unlockPayload, NOT bip85Derive,
// and it is CONDITIONAL (§10.1): with a payload present the lap is nine. The
// bound is StartScreen.lastNav(), and gui.go's compile-time guard -- keyed to
// unlockPayload -- turns a program inserted between it and qaProgram into a
// build failure rather than pager drift. The nine-program case is
// TestStartScreenFitsAtNinePagerDots below and gui/unlock_program_test.go.
var ftProgramTitles = []string{
	"Backup Wallet",
	"BIP-39 Password",
	"Engrave Text",
	"Account Xpub",
	"Engrave Bundle",
	"Engrave Single-Sig",
	"Engrave Multisig",
	"BIP-85",
}

// TestEngraveTextProgramNavigableByTouch walks the whole pager by touch at the
// panel the machine actually has.
//
// Touch, not synthesized ButtonEvents: five of six programs were once
// unreachable on hardware while every button-driven pager test passed.
func TestEngraveTextProgramNavigableByTouch(t *testing.T) {
	p := newPlatform()
	p.display = sh2DisplaySize
	ctx := NewContext(p)
	m := new(StartScreen)
	frame, drawer, quit := runUITouch(ctx, func() { m.Flow(ctx, &descriptorTheme) })
	defer quit()

	content, ok := frame()
	if !ok {
		t.Fatal("StartScreen produced no frame")
	}
	if !uiContains(content, ftProgramTitles[0]) {
		t.Fatalf("initial program is not %q; got %q", ftProgramTitles[0], content)
	}
	_, right := arrowPoints(ctx)
	// One full lap, ending back at the start: a wrap that skips an entry fails
	// here even though every single step passes.
	for i := 1; i <= len(ftProgramTitles); i++ {
		want := ftProgramTitles[i%len(ftProgramTitles)]
		tap(&ctx.Router, drawer(), right)
		content, ok = frame()
		if !ok {
			t.Fatalf("no frame after tap #%d", i)
		}
		if !uiContains(content, want) {
			t.Fatalf("tap #%d: expected program %q, got %q", i, want, content)
		}
	}
	// And the new program is the THIRD entry, not merely present somewhere.
	if got := ftProgramTitles[2]; got != "Engrave Text" {
		t.Fatalf("Engrave Text is not the third program; the pager order is %v", ftProgramTitles)
	}
	if int(engraveText) != 2 {
		t.Errorf("engraveText = %d, want 2 (third program)", int(engraveText))
	}
	if int(bip85Derive)+1 != len(ftProgramTitles) {
		t.Errorf("%d navigable programs, want %d", int(bip85Derive)+1, len(ftProgramTitles))
	}
}

// TestStartScreenFitsAtEightPagerDots. The pager row is
// (dot+space)*npages-space wide, so every program added widens it. On a 480px
// panel with no scrolling, a pager that outgrows the glass takes the arrows --
// the only way to change program -- off the screen.
func TestStartScreenFitsAtEightPagerDots(t *testing.T) {
	p := newPlatform()
	p.display = sh2DisplaySize
	ctx := NewContext(p)
	// bip85Derive is the no-payload bound, which is the state this test
	// exercises: a StartScreen built with new(StartScreen) has no payload.
	_, sz := layoutMainPager(&ctx.B, &descriptorTheme, backupWallet, bip85Derive)
	if sz.X > sh2DisplaySize.X {
		t.Errorf("the %d-dot pager is %dpx wide, past the %dpx panel", int(bip85Derive)+1, sz.X, sh2DisplaySize.X)
	}
	// And both arrows are still hit-testable on the real panel.
	m := new(StartScreen)
	frame, drawer, quit := runUITouch(ctx, func() { m.Flow(ctx, &descriptorTheme) })
	defer quit()
	if _, ok := frame(); !ok {
		t.Fatal("StartScreen produced no frame")
	}
	left, right := arrowPoints(ctx)
	for _, tc := range []struct {
		name string
		at   image.Point
	}{{"left arrow", left}, {"right arrow", right}} {
		if _, _, hit := drawer().Hit(tc.at); !hit {
			t.Errorf("%s has no touch target at %v on a %v panel", tc.name, tc.at, sh2DisplaySize)
		}
	}
}

// TestStartScreenFitsAtNinePagerDots is the same check one dot further out.
//
// §10.1's unlockPayload entry adds a NINTH dot when a payload is present, and
// the row is (dot+space)*npages-space wide, so nothing that passed at eight
// proves anything at nine. If nine does not fit, the dot pager needs a
// different treatment at nine -- and this is where that is discovered, not on
// hardware.
func TestStartScreenFitsAtNinePagerDots(t *testing.T) {
	p := newPlatform()
	p.display = sh2DisplaySize
	ctx := NewContext(p)
	_, sz := layoutMainPager(&ctx.B, &descriptorTheme, backupWallet, unlockPayload)
	t.Logf("the 9-dot pager measures %dpx on a %dpx panel", sz.X, sh2DisplaySize.X)
	if sz.X > sh2DisplaySize.X {
		t.Errorf("the 9-dot pager is %dpx wide, past the %dpx panel", sz.X, sh2DisplaySize.X)
	}
	// And with the payload present the arrows are still hit-testable.
	m := &StartScreen{hasPayload: true}
	frame, drawer, quit := runUITouch(ctx, func() { m.Flow(ctx, &descriptorTheme) })
	defer quit()
	if _, ok := frame(); !ok {
		t.Fatal("StartScreen produced no frame")
	}
	left, right := arrowPoints(ctx)
	for _, tc := range []struct {
		name string
		at   image.Point
	}{{"left arrow", left}, {"right arrow", right}} {
		if _, _, hit := drawer().Hit(tc.at); !hit {
			t.Errorf("%s has no touch target at %v on a %v panel", tc.name, tc.at, sh2DisplaySize)
		}
	}
}

// TestEngraveTextProgramSelectable presses OK on the new entry. Navigability
// proves it is in the carousel and the flow tests drive it directly; without
// this, the feature ships with a dead menu item and a fully green suite.
func TestEngraveTextProgramSelectable(t *testing.T) {
	p := newPlatform()
	p.display = sh2DisplaySize
	ctx := NewContext(p)
	h := &ppHarness{t: t, ctx: ctx, widgets: make(map[string]any)}
	passphraseWidgetHook = func(name string, w any) { h.widgets[name] = w }
	t.Cleanup(func() { passphraseWidgetHook = nil })
	h.start(func() { uiFlow(ctx, "test") })

	_, right := arrowPoints(ctx)
	for range 2 {
		tap(&ctx.Router, h.drawer(), right)
		h.next("after stepping the pager")
	}
	if !uiContains(h.content, "Engrave Text") {
		t.Fatalf("two steps right is not the Engrave Text program; got %q", h.content)
	}
	h.tapNav(Button3)
	// A needle only the free-text program renders. The passphrase program's QR
	// step says "machine-readable copy of the passphrase", so "machine-readable"
	// alone would pass on the wrong flow; this phrase appears nowhere else.
	h.mustReach("photographs the plate")
}
