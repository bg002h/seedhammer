package gui

import (
	"image"
	"iter"
	"testing"

	"seedhammer.com/gui/op"
)

// The StartScreen program pager regression this file guards against:
//
// The pager was extended from one program to six, which turned on the left/right
// arrows and the page dots, but navigation stayed bound to ButtonFilter(Left) and
// ButtonFilter(Right). SeedHammer II has no directional buttons -- its only
// production input is the ft6x36 capacitive panel, which emits PointerEvents --
// so five of the six programs were unreachable on real hardware.
//
// Every existing pager test drives navigation with click(&ctx.Router, Right),
// which synthesizes a ButtonEvent that no production code path emits. They passed
// throughout. A test that cannot distinguish "the state machine advances" from
// "the user can advance it" cannot catch this class of bug, so the tests below
// drive the pager the way the hardware does: by touch.

// runUITouch is runUI plus access to the op.Drawer for the most recent frame.
// Pointer events are hit-tested against what was actually drawn (EventRouter.Events
// calls d.Hit), so routing touch requires the frame's Drawer -- unlike button
// events, which need no drawer at all.
func runUITouch(ctx *Context, ui func()) (frame func() (string, bool), drawer func() *op.Drawer, stop func()) {
	var last *op.Drawer
	next, quit := iter.Pull(func(yield func(content string) bool) {
		ctx.FrameCallback = func(o op.Op) {
			r := image.Rectangle{Max: ctx.Platform.DisplaySize()}
			d := new(op.Drawer)
			content := d.ExtractText(r, o)
			last = d
			ctx.Reset()
			ctx.Done = ctx.Done || !yield(content)
		}
		ui()
	})
	return next, func() *op.Drawer { return last }, quit
}

// tap delivers a press+release at pos, routed against the drawn frame, which is
// what the touch driver produces for a fingertip.
func tap(r *EventRouter, d *op.Drawer, pos image.Point) {
	r.Events(d,
		PointerEvent{Pressed: true, Entered: true, Pos: pos}.Event(),
		PointerEvent{Pressed: false, Entered: true, Pos: pos}.Event(),
	)
}

// arrowPoints returns points inside the left and right arrow touch targets. The
// pager row is centered vertically, and each target runs out to its screen edge.
func arrowPoints(ctx *Context) (left, right image.Point) {
	dims := ctx.Platform.DisplaySize()
	return image.Pt(2, dims.Y/2), image.Pt(dims.X-3, dims.Y/2)
}

// TestStartScreenPagerTouchable is the direct regression test: tapping the right
// arrow must advance the program. If the arrows are decorative -- drawn without an
// op.Input hit area, or navigation bound only to button events -- the title never
// changes and this fails.
func TestStartScreenPagerTouchable(t *testing.T) {
	ctx := NewContext(newPlatform())
	m := new(StartScreen)
	frame, drawer, quit := runUITouch(ctx, func() { m.Flow(ctx, &descriptorTheme) })
	defer quit()

	content, ok := frame()
	if !ok {
		t.Fatal("StartScreen produced no frame")
	}
	if !uiContains(content, "Backup Wallet") {
		t.Fatalf("initial program not Backup Wallet; got %q", content)
	}

	_, right := arrowPoints(ctx)
	tap(&ctx.Router, drawer(), right)
	content, ok = frame()
	if !ok {
		t.Fatal("no frame after tapping the right arrow")
	}
	if uiContains(content, "Backup Wallet") {
		t.Fatalf("tapping the right arrow did not advance the program; still %q.\n"+
			"The pager is unreachable by touch, which is the only input SeedHammer II has.", content)
	}
	if !uiContains(content, "BIP-39 Password") {
		t.Fatalf("right arrow should advance to the BIP-39 Password program; got %q", content)
	}
}

// TestStartScreenPagerTouchLeftWraps asserts the left arrow is independently wired
// (not just a second copy of the right one) and wraps backwards to the last
// program.
func TestStartScreenPagerTouchLeftWraps(t *testing.T) {
	ctx := NewContext(newPlatform())
	m := new(StartScreen)
	frame, drawer, quit := runUITouch(ctx, func() { m.Flow(ctx, &descriptorTheme) })
	defer quit()

	if _, ok := frame(); !ok {
		t.Fatal("StartScreen produced no frame")
	}

	left, _ := arrowPoints(ctx)
	tap(&ctx.Router, drawer(), left)
	content, ok := frame()
	if !ok {
		t.Fatal("no frame after tapping the left arrow")
	}
	if !uiContains(content, "BIP-85") {
		t.Fatalf("left arrow from Backup Wallet should wrap to the BIP-85 program; got %q", content)
	}
}

// TestStartScreenPagerTouchReachesEveryProgram walks the whole pager by touch and
// back to the start. A partially-wired pager -- one arrow live, or a wrap that
// skips an entry -- passes the single-step tests above but fails here.
func TestStartScreenPagerTouchReachesEveryProgram(t *testing.T) {
	ctx := NewContext(newPlatform())
	m := new(StartScreen)
	frame, drawer, quit := runUITouch(ctx, func() { m.Flow(ctx, &descriptorTheme) })
	defer quit()

	if _, ok := frame(); !ok {
		t.Fatal("StartScreen produced no frame")
	}
	_, right := arrowPoints(ctx)

	// Titles in pager order, starting one step to the right of backupWallet.
	want := []string{
		"BIP-39 Password",
		"Engrave Text",
		"Account Xpub",
		"Engrave Bundle",
		"Engrave Single-Sig",
		"Engrave Multisig",
		"Wallet Policy",
		"Load Payload",
		"BIP-85",
		"Backup Wallet", // wraps
	}
	for i, title := range want {
		tap(&ctx.Router, drawer(), right)
		content, ok := frame()
		if !ok {
			t.Fatalf("no frame after tap #%d", i+1)
		}
		if !uiContains(content, title) {
			t.Fatalf("tap #%d: expected program %q, got %q", i+1, title, content)
		}
	}
}
