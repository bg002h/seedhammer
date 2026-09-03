package gui

import (
	"image"
	"testing"

	"seedhammer.com/gui/op"
	"seedhammer.com/gui/widget"
)

// W-2: the composer's paged PICK screens must be driveable by TOUCH.
//
// composerPickScreen moved its cursor only on ButtonFilter(Up)/ButtonFilter(Down)
// and drew its rows as bare labels -- no Clickable, no op.Input hit area, and no
// scroll arrows. SeedHammer II has no directional buttons: its only production
// input is the ft6x36 capacitive panel, which emits PointerEvents
// (cmd/controller/platform_sh2.go). The one other source of a directional
// ButtonEvent in this tree is cmd/controller/debug_sh2.go, a UART debug harness.
// So on the machine only the FIRST row of each page could ever be taken, which
// put `n = 2`, `k = 2` and -- once one path existed, because row 0 then becomes
// `Path 1: ...` -- `Done` out of reach. The composer could not be driven to a
// plate by a hand.
//
// WHY NOTHING CAUGHT IT. Every other composer test drives these screens with
// click(&ctx.Router, Down), a synthetic ButtonEvent no production path emits --
// e.g. composer_flow_test.go's `click(&ctx.Router, Down, Down) // 1 -> 3`. They
// were green throughout. This is the same shape as the StartScreen pager
// regression that start_screen_touch_test.go was written for, and it is guarded
// the same way: through runUITouch, by touch, on the real flow.
//
// Measured on the emulator against 60bee002 before the fix: 205 taps across the
// whole body of `Path 1: how many keys?` moved nothing, and the take still
// yielded n = 1.

// composerPickRowPoint returns a point inside visible row j of a
// composerPickScreen page, MEASURED THE WAY composerPageLines LAYS THE PAGE OUT
// rather than from a constant: content starts at leadingSize+8, the lead header
// and its spacer are drawn first, and every line advances by its own measured
// height plus a 6 px gap. Text is centred, so the horizontal midpoint is inside
// both the glyph rectangle and the full-width band.
//
// It measures into its OWN op.Buffer. Laying text into ctx.B between frames
// would append to the buffer the running flow is building.
//
// The caller passes the lead and the rows it expects, and every one of those
// strings is asserted against the live frame before this is called -- so a
// wording change fails the assertion rather than silently shifting the point.
func composerPickRowPoint(ctx *Context, lead string, rows []string, j int) image.Point {
	dims := ctx.Platform.DisplaySize()
	lineWidth := dims.X - 2*8
	var buf op.Buffer
	page := append([]string{lead, ""}, rows...)
	const rowBase = 2 // the header and its spacer, as composerPickScreen names it
	y := leadingSize + 8
	for i, line := range page {
		_, sz := widget.Labelw(&buf, ctx.Styles.body, lineWidth, descriptorTheme.Text, line)
		if i == j+rowBase {
			return image.Pt(dims.X/2, y+sz.Y/2)
		}
		y += sz.Y + 6
	}
	panic("composerPickRowPoint: row beyond the page")
}

// TestComposerPickScreenRowsAreTouchable drives the REAL composer flow
// (walletPolicyFlow -> composerFlow) and makes the three choices W-2 put out of
// reach, BY TOUCH:
//
//  1. `3` on `Path 1: how many keys?`  -> the threshold picker must offer 1 2 3
//  2. `2` on `Path 1: how many must sign?` -> the path list must read 2-of-3
//  3. `Done` on a path list holding one path -> the key-order question
//
// The ChoiceScreens on the way in are driven with button clicks on purpose:
// they already register per-row hit areas (ChoiceScreen.Draw wraps each child in
// op.Input) and are covered elsewhere. Every tap below lands on a
// composerPickScreen, which is the screen under test.
//
// A ROW TAP SELECTS; Button3 TAKES. So each assertion is on the screen AFTER the
// take -- a selection is a colour inversion and ExtractText cannot see it, which
// is exactly why this test proves the tap by its consequence.
func TestComposerPickScreenRowsAreTouchable(t *testing.T) {
	p := newEngravedAwarePlatform()
	p.engraver = newEngraver()
	p.display = sh2DisplaySize
	ctx := NewContext(p)
	// No payload: ctx.sysw stays nil, so the shape is the whole of this walk and
	// nothing has to be seated to reach the key-order question.

	frame, drawer, quit := runUITouch(ctx, func() { walletPolicyFlow(ctx, &descriptorTheme) })
	defer quit()

	got, ok := pumpUntil(frame, "Build a new policy", 24)
	if !ok {
		t.Fatalf("the composer door never drew.\nLast frame: %q", got)
	}
	click(&ctx.Router, Down) // Scan cards -> Build a new policy
	click(&ctx.Router, Button3)

	if got, ok = pumpUntil(frame, "Which script?", 24); !ok {
		t.Fatalf("the wrapper picker never drew.\nLast frame: %q", got)
	}
	click(&ctx.Router, Down) // -> Segwit (wsh)
	click(&ctx.Router, Button3)

	if got, ok = pumpUntil(frame, "Start from?", 24); !ok {
		t.Fatalf("the preset picker never drew.\nLast frame: %q", got)
	}
	click(&ctx.Router, Button3) // row 0 = Build my own paths (W-1)

	// The EMPTY path list. Row 0 is "Add a spend path", so this one needs no
	// tap -- which is precisely why W-2 stayed invisible until a second screen
	// wanted a row that was not first.
	if got, ok = pumpUntil(frame, "Add a spend path", 24); !ok {
		t.Fatalf("the path list never drew.\nLast frame: %q", got)
	}
	click(&ctx.Router, Button3)

	if got, ok = pumpUntil(frame, "What can spend on this path?", 24); !ok {
		t.Fatalf("the spend-kind picker never drew.\nLast frame: %q", got)
	}
	click(&ctx.Router, Button3) // Keys

	// ─── TAP 1: the `3` row on the key-count picker ────────────────────────
	const nLead = "Path 1: how many keys?"
	nRows := []string{"1", "2", "3"}
	if got, ok = pumpUntil(frame, nLead, 24); !ok {
		t.Fatalf("the key-count picker never drew.\nLast frame: %q", got)
	}
	for _, r := range nRows {
		if !uiContains(got, r) {
			t.Fatalf("the key-count picker does not offer row %q, so the point measured "+
				"for it would be meaningless.\nFrame: %q", r, got)
		}
	}
	tap(&ctx.Router, drawer(), composerPickRowPoint(ctx, nLead, nRows, 2))
	click(&ctx.Router, Button3) // take the highlighted row

	const kLead = "Path 1: how many must sign?"
	if got, ok = pumpUntil(frame, kLead, 24); !ok {
		t.Fatalf("the threshold picker never drew.\nLast frame: %q", got)
	}
	if !uiContains(got, kLead+" 1 2 3") {
		t.Fatalf("tapping the `3` row did not select it: the threshold picker offers "+
			"1..n for the n just taken, and this frame is not 1 2 3.\nFrame: %q\n"+
			"composerPickScreen's rows are unreachable by touch, which is the only "+
			"input SeedHammer II has (W-2).", got)
	}
	if uiContains(got, kLead+" 1 2 3 4") {
		t.Fatalf("the threshold picker offers more than 3 rows, so n was not 3.\nFrame: %q", got)
	}

	// ─── TAP 2: the `2` row on the threshold picker ─────────────────────────
	tap(&ctx.Router, drawer(), composerPickRowPoint(ctx, kLead, []string{"1", "2", "3"}, 1))
	click(&ctx.Router, Button3)

	// ─── TAP 3: `Done` on a path list that now holds a path ─────────────────
	//
	// Row 0 is the path itself, so Done is reachable ONLY by moving the cursor.
	// This is the step that made the composer a dead end on the machine.
	const pathLead = "slots: 3" // composerSlotsKeysLine with no sources loaded
	pathRows := []string{"Path 1: 2-of-3", "Add a spend path", "Change the script", "Done"}
	if got, ok = pumpUntil(frame, pathRows[0], 24); !ok {
		t.Fatalf("the path list does not show a 2-of-3 path, so the `2` tap did not "+
			"land either.\nLast frame: %q", got)
	}
	if !uiContains(got, pathLead) {
		t.Fatalf("the path list's lead is not %q, so the row points measured below "+
			"would be shifted.\nFrame: %q", pathLead, got)
	}
	for _, r := range pathRows {
		if !uiContains(got, r) {
			t.Fatalf("the path list does not offer row %q.\nFrame: %q", r, got)
		}
	}
	tap(&ctx.Router, drawer(), composerPickRowPoint(ctx, pathLead, pathRows, 3))
	click(&ctx.Router, Button3)

	if got, ok = pumpUntil(frame, "Sorted keys, or your order?", 24); !ok {
		t.Fatalf("tapping `Done` did not leave the path list: the walk never reached "+
			"the key-order question.\nLast frame: %q\n"+
			"With one path present row 0 is the path itself, so Done is reachable only "+
			"by moving the cursor -- and on SeedHammer II a tap is the only thing that "+
			"can move it (W-2).", got)
	}
}
