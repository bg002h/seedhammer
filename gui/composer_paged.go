package gui

import (
	"image"

	"seedhammer.com/gui/assets"
	"seedhammer.com/gui/op"
	"seedhammer.com/gui/widget"
)

// The composer's PAGED screens (SPEC §9 items 6 and 7).
//
// WHY THIS IS NOT ChoiceScreen. ChoiceScreen.Draw stacks its children with
// `h += c.Size.Y` (gui/gui.go:2019) and applies NO clip and NO bound against
// the content box, and ChoiceScreen.Choose holds no scroll offset at all
// (gui/gui.go:1910-1964). The content box is 232 px (320 minus the two
// leadingSize bands cut at gui/gui.go:1967-1970), so a list longer than that
// draws past the frame with no visual cue while Down still moves the cursor
// onto rows nobody can see. Nothing exercises that today because no shipped
// caller has an unbounded list. The composer has three: the payload's keys,
// the template's 32 slots, and eight paths plus four addresses at consent.
//
// WHY IT COPIES confirmReviewScreen. That function (gui/multisig_build.go
// :1877-1939) already measures rows against the box correctly, advances by
// the EXACT count it laid out, wraps at the end, and draws its pager only
// when a second page exists. Reimplementing the measurement would be a second
// answer to a question that must have one.
//
// PAGING IS FORWARD-ONLY WITH WRAP, and selection needs no backward page
// arithmetic: when the cursor leaves the top of the page Up sets start = sel,
// and when it leaves the bottom Down sets start = sel. Either way the cursor
// becomes the first row of the page that is then laid out, which is exact
// rather than a guess at how many rows the previous page held.

// composerPageLines lays out lines[start:] into the content box and returns
// the ops, HOW MANY were drawn, and each drawn row's TOUCH BAND.
//
// THE ONE MEASURE SITE. Every capacity number in SPEC §13 comes from this
// function, and every paged screen below calls it, so a screen's capacity and
// the number recorded for it cannot drift apart. The touch bands are returned
// from here for the same reason: a hit area measured anywhere else is a second
// answer to the question "where is row i", and the two would drift.
//
// THE BAND IS FULL-WIDTH, NOT THE GLYPH RECTANGLE, and that is ChoiceScreen's
// rule rather than a new one: it pads every choice to the widest one's width
// (`xoff := (maxW-c.Size.X)/2 + buttonPadX`, gui/gui.go) so a short row is not
// a smaller target than a long one. Here the common width is the width this
// function already wraps to, which is the same thing one level up. Without it
// the `Path N: how many keys?` picker -- whose rows are single digits -- would
// have an eight-pixel-wide target on a device operated by fingertip, and the
// row-tap fix would be one a hand could not use.
//
// bands[i] pairs with body[i], in draw order, in SCREEN coordinates. Note
// len(body) can exceed `shown` by one: the last row is drawn even when it
// falls outside the box, and is deliberately NOT counted (see below), so a
// caller wiring hit areas must wire `shown` of them and not len(body).
//
// sel is the highlighted row's absolute index, or -1 for a read-only screen.
func composerPageLines(ctx *Context, th *Colors, dims image.Point, lines []string, start, sel int) ([]op.Op, int, []image.Rectangle) {
	contentTop := leadingSize + 8
	contentBottom := dims.Y - leadingSize
	// ─── ONE BAND, AND EVERYTHING USES IT (W-3) ─────────────────────────────
	//
	// The text is WRAPPED and CENTRED inside the band to the LEFT of the
	// navigation column, and the touch targets use the same two bounds. It used
	// to wrap at `dims.X - 2*8` and centre across the WHOLE panel while only the
	// hit rect stopped at the column, so a line that measured near the wrap
	// bound was DRAWN under a button: on the S4 shots the Template screen's
	// `Template-ID: 531ab9e1777f018ae53694387dd0d128` lost its 32nd hex digit
	// under Back, and the key-less arm's `mk encode` lines lost their tails
	// under the pager.
	//
	// The emulator walk could not see it. op.Drawer.ExtractText collects a
	// glyph's rune wherever it lands, under a button included, so every
	// text-presence assertion passed on a screen the operator cannot read.
	// gui/composer_paged_geometry_test.go rasterises the body instead and looks
	// for ink inside the button rectangles.
	//
	// LONG LINES WRAP, they do not shrink: a 32-hex id with its label becomes
	// two lines, which costs a row of the per-frame budget and is the honest
	// trade. The budget below counts the wrapped height, so it stays correct by
	// construction.
	//
	// bandMargin is the SAME margin the left edge always had (the old
	// `(dims.X - (dims.X-2*8))/2`), applied on the right of the text as well so
	// a glyph never sits flush against a button it is not part of.
	const bandMargin = 8
	bandLeft := bandMargin
	bandRight := dims.X - assets.NavBtnPrimary.Bounds().Size().X - bandMargin
	lineWidth := bandRight - bandLeft
	body := make([]op.Op, 0, len(lines))
	bands := make([]image.Rectangle, 0, len(lines))
	shown := 0
	y := contentTop
	for i := start; i < len(lines); i++ {
		col := th.Text
		if i == sel {
			col = th.Background
		}
		lbl, sz := widget.Labelw(&ctx.B, ctx.Styles.body, lineWidth, col, lines[i])
		// The first row is drawn even if it alone overflows: a row too tall for
		// the box is a copy defect, and dropping it would make the screen blank
		// instead of showing what is wrong.
		if i > start && y+sz.Y > contentBottom {
			break
		}
		// Centred in the BAND, not on the panel: centring on the panel is what
		// pushed a wide line's right half under the column.
		pos := image.Pt(bandLeft+(lineWidth-sz.X)/2, y)
		if i == sel {
			bg := image.Rectangle{Max: sz}
			bg.Min.X -= buttonPadX
			bg.Max.X += buttonPadX
			bg.Min.Y -= buttonPadY
			bg.Max.Y += buttonPadY
			lbl = op.Layer(
				lbl,
				op.Compose(
					op.Color(&ctx.B, th.Text),
					op.RoundedRect2(&ctx.B, bg, cornerRadius),
				),
			)
		}
		body = append(body, lbl.Offset(pos))
		// The band matches the selection highlight's own vertical extent, so
		// what the operator sees highlighted is exactly what they can tap.
		bands = append(bands, image.Rect(bandLeft, y-buttonPadY, bandRight, y+sz.Y+buttonPadY))
		y += sz.Y + 6
		// COUNTED ONLY WHEN IT IS INSIDE THE BOX. It used to increment and
		// THEN break, so the last counted row could extend past the content
		// box -- and this count is the number §13 item 1 records as the
		// screen's per-frame capacity, so it would have recorded one more row
		// than the frame fully draws.
		if y > contentBottom {
			if shown > 0 {
				break
			}
			shown++
			break
		}
		shown++
	}
	return body, shown, bands
}

// composerReadScreen is a paged read-only screen: Button3 continues, Button1
// goes back, Button2 pages, and the pager icon is drawn ONLY when a second
// page exists.
//
// The icon gate is confirmReviewScreen's ruling, inherited rather than
// re-argued (gui/multisig_build.go:1919-1931): a control that is present and
// inert teaches the operator that controls here may be inert, on a device
// whose other buttons cut steel.
func composerReadScreen(ctx *Context, th *Colors, title string, lines []string) bool {
	backBtn := &Clickable{Button: Button1}
	contBtn := &Clickable{Button: Button3, AltButton: Center}
	pageBtn := &Clickable{Button: Button2}
	start := 0
	// THE CHECKMARK IS WITHHELD UNTIL THE LAST PAGE HAS BEEN LAID OUT ONCE.
	//
	// §7e's consent carries the per-path lines, the key-path line, both ids
	// and receive AND change addresses 0..1 -- and the addresses are the only
	// thing that proves WHICH wallet this is. The shipped confirmReviewScreen
	// accepts Button3 on the first frame whatever page is showing, so an
	// operator could consent to a wallet whose proof was never drawn. That is
	// inherited behaviour and this is new code, so the composer's own paged
	// screen does not inherit it.
	//
	// ONCE, not every time: paging back to page 1 does not re-arm the gate.
	// The operator has seen it; making them page to the end again would be a
	// control that punishes reading.
	seenEnd := false
	for !ctx.Done {
		if backBtn.Clicked(ctx) {
			return false
		}
		if seenEnd && contBtn.Clicked(ctx) {
			return true
		}
		dims := ctx.Platform.DisplaySize()
		// The bands are discarded: this screen has no cursor, so a row is not
		// a control and giving it a hit area would be the present-and-inert
		// affordance the icon gate below exists to avoid.
		body, shown, _ := composerPageLines(ctx, th, dims, lines, start, -1)
		if start+shown >= len(lines) {
			seenEnd = true
		}
		if pageBtn.Clicked(ctx) {
			if start+shown < len(lines) {
				start += shown
			} else {
				start = 0
			}
			continue
		}
		titleOp, _ := layoutTitle(ctx, dims.X, th.Text, title)
		navs := []NavButton{{Clickable: backBtn, Style: StyleSecondary, Icon: assets.IconBack}}
		if start > 0 || shown < len(lines) {
			navs = append(navs, NavButton{Clickable: pageBtn, Style: StyleSecondary, Icon: assets.IconRight})
		}
		if seenEnd {
			navs = append(navs, NavButton{Clickable: contBtn, Style: StylePrimary, Icon: assets.IconCheckmark})
		} else {
			// The Button3 handler is still DRAINED above; what is withheld is
			// the icon, so the control is never present-and-inert (the ruling
			// confirmReviewScreen states at gui/multisig_build.go:1919-1925).
			contBtn.Clicked(ctx)
		}
		nav, _ := layoutNavigation(&ctx.B, th, dims, navs...)
		frameOps := append([]op.Op{nav, titleOp}, body...)
		frameOps = append(frameOps, op.Color(&ctx.B, th.Background))
		ctx.Frame(op.Layer(frameOps...))
	}
	return false
}

// composerPickScreenMaxRows bounds the per-visible-row hit areas below.
//
// The content box is 232 px (320 minus the two 44 px leadingSize bands) and a
// body row is a wrapped text label plus a 6 px gap -- at this font no row is
// under 14 px, so at most 12 fit. 24 is double that, and a page that somehow
// laid out more would simply leave the surplus rows untappable rather than
// index past the array: today's behaviour, not a panic.
const composerPickScreenMaxRows = 24

// composerPickScreen is composerReadScreen with a cursor: a TAP ON A ROW
// selects it, Up/Down move the selection, Button2 pages, Button3 takes the
// highlighted row, Button1 declines. `lead` is drawn as the first body row
// rather than in the lead band, so a long prompt (the §8s seating prompts are
// long) wraps with the rows instead of being cut by the 44 px band.
//
// ─── W-2: THE ROWS ARE TOUCH TARGETS, AND WITHOUT THAT THIS SCREEN WAS DEAD ──
//
// Measured on the emulator 2026-09-03 against 60bee002, before this: 205 taps
// across the whole body of `Path 1: how many keys?` moved nothing, and the take
// still yielded n = 1. The cursor moved ONLY on ButtonFilter(Up)/(Down), and
// SeedHammer II has no directional buttons -- its only production input is the
// ft6x36 panel, which emits PointerEvents (cmd/controller/platform_sh2.go; the
// sole other source of a directional ButtonEvent in this tree is
// cmd/controller/debug_sh2.go, a UART debug harness). So on the machine the
// only reachable row was the first of each page, which put `n = 2`, `k = 2` and
// -- once one path existed, since row 0 becomes `Path 1: ...` -- `Done` out of
// reach. Four production call sites depended on it: composerCountPick, the
// Spend paths list, `Which hash?` and `Seat keys`. The composer could not be
// driven to a plate by a hand.
//
// EVERY COMPOSER TEST WAS GREEN THROUGHOUT, because every one of them drives
// this screen with click(&ctx.Router, Down) -- a synthetic ButtonEvent no
// production path emits. That is the same shape as the StartScreen pager
// regression, and the guard is the same: gui/composer_pick_touch_test.go drives
// the real flow through runUITouch, by touch, and fails on 60bee002.
//
// A ROW TAP SELECTS, IT DOES NOT TAKE. Button3 still takes, so a mis-aimed tap
// costs a second tap and never an engraved plate -- ChoiceScreen's contract
// (`if c.click.Clicked(ctx) { s.choice = i }`), which is the one the operator
// already knows from every other list on this device. No auto-repeat, no drag
// and no arrows are added: the Clickables are zero-value, so Clickable.Next's
// repeat arm -- which fires only for Up/Down/Left/Right -- cannot reach them.
func composerPickScreen(ctx *Context, th *Colors, title, lead string, rows []string) (int, bool) {
	backBtn := &Clickable{Button: Button1}
	takeBtn := &Clickable{Button: Button3, AltButton: Center}
	pageBtn := &Clickable{Button: Button2}
	// ONE PER VISIBLE ROW INDEX, re-used across pages, and declared OUTSIDE the
	// frame loop: a Clickable carries press state between frames, and the tag a
	// frame registers is the address polled on the next one -- a per-frame slice
	// would hand the router a pointer that no longer belongs to anything.
	var rowHits [composerPickScreenMaxRows]Clickable
	inp := new(InputTracker)
	// THE LEAD IS A PER-PAGE HEADER, NOT THE FIRST BODY ROW.
	//
	// It used to be lines[0] with a spacer, which put the §8s prompt ("Slot
	// @2, Path 1 key 2 of 3: choose a key") on the FIRST page only. On the screen
	// this primitive exists for -- a payload holding more keys than a frame --
	// an operator paging for the right key lost which slot they were filling,
	// and paging is forward-only with wrap, so recovering it cost a full
	// cycle. Prepending it to every page costs the same measurement loop.
	lines := rows
	sel := 0
	start := 0
	for !ctx.Done {
		if backBtn.Clicked(ctx) {
			return 0, false
		}
		if takeBtn.Clicked(ctx) {
			return sel, true
		}
		dims := ctx.Platform.DisplaySize()
		page := append([]string{lead, ""}, lines[start:]...)
		const rowBase = 2 // the header and its spacer, redrawn on every page
		pageOps, drawn, bands := composerPageLines(ctx, th, dims, page, 0, sel-start+rowBase)
		shown := drawn - rowBase
		if shown < 1 {
			// A header that fills the frame would leave no room for a row and
			// the list could never advance. One row always draws.
			shown = 1
		}
		body := pageOps
		// The hit areas, one per row this page COUNTED as inside the box. Not
		// len(bands): composerPageLines draws a final overflowing row without
		// counting it, and making that one tappable would hand the operator a
		// row the frame cut in half -- which is the thing its own "counted only
		// when it is inside the box" rule exists to prevent.
		for j := 0; j < shown && j < len(rowHits); j++ {
			b := rowBase + j
			if b >= len(bands) || start+j >= len(lines) {
				break
			}
			if rowHits[j].Clicked(ctx) {
				sel = start + j
			}
			body = append(body, op.Input(&ctx.B, &rowHits[j]).Clip(bands[b]))
		}
		for {
			e, ok := inp.Next(ctx, ButtonFilter(Up), ButtonFilter(Down))
			if !ok {
				break
			}
			be, ok := e.AsButton()
			if !ok || !be.Pressed {
				continue
			}
			switch be.Button {
			case Up:
				if sel > 0 {
					sel--
				}
			case Down:
				if sel < len(lines)-1 {
					sel++
				}
			}
			if sel < start || sel >= start+shown {
				// The cursor left the page. Making it the FIRST row of the next
				// layout is exact; computing the previous page's size is not.
				start = sel
			}
		}
		if pageBtn.Clicked(ctx) {
			if start+shown < len(lines) {
				start += shown
			} else {
				start = 0
			}
			// THE CURSOR IS CLAMPED INTO THE PAGE IN BOTH DIRECTIONS. Only the
			// upward clamp existed, so after a wrap to start = 0 the cursor
			// stayed on a row belonging to a later page: that frame drew NO
			// highlight and Button3 returned the invisible row -- seating a
			// key the operator never saw selected. Up/Down already clamp the
			// page to the cursor (start = sel); this is the same rule applied
			// the other way.
			if sel < start {
				sel = start
			}
			if sel >= start+shown {
				sel = start
			}
			continue
		}
		titleOp, _ := layoutTitle(ctx, dims.X, th.Text, title)
		navs := []NavButton{{Clickable: backBtn, Style: StyleSecondary, Icon: assets.IconBack}}
		if start > 0 || shown < len(lines) {
			navs = append(navs, NavButton{Clickable: pageBtn, Style: StyleSecondary, Icon: assets.IconRight})
		}
		navs = append(navs, NavButton{Clickable: takeBtn, Style: StylePrimary, Icon: assets.IconCheckmark})
		nav, _ := layoutNavigation(&ctx.B, th, dims, navs...)
		frameOps := append([]op.Op{nav, titleOp}, body...)
		frameOps = append(frameOps, op.Color(&ctx.B, th.Background))
		ctx.Frame(op.Layer(frameOps...))
	}
	return 0, false
}
