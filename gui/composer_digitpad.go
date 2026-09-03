package gui

import (
	"image"

	"seedhammer.com/gui/assets"
	"seedhammer.com/gui/layout"
	"seedhammer.com/gui/op"
	"seedhammer.com/gui/widget"
)

// The composer's digit pad (SPEC §6b, C25).
//
// NO DIGIT-ONLY WIDGET EXISTS in this tree: the passphrase keyboard's digit
// page is mixed with punctuation, and a grep for a numeric widget finds a hex
// length message and a comment. So this is NewKeyboard with a digits-only
// alphabet -- the same primitive codex32Keys already uses with a digit-leading
// row (gui/codex32_polish.go:242) -- driven the way inputCodex32Flow drives
// it (gui/gui.go:1262-1352).
//
// THE OPERATOR NEVER TYPES A RAW OPERAND (§6b). They type a count of blocks,
// a count of days, a block height or eight date digits, and the encoding is
// computed for them. That is why this widget knows nothing about locks: it
// returns the digits and the caller owns the meaning.
//
// NewKeyboard appends the backspace key and the trailing row break itself
// (gui/gui.go:1464-1465), so the alphabet below is digits alone.
const composerDigitKeys = "123\n456\n789\n0"

// composerDigitEntry collects up to maxDigits digits.
//
// `echo` is the caller's validator AND its echo line in one call: it returns
// the line drawn under the fragment and whether the fragment may be
// accepted. One function rather than two, because the echo and the
// acceptance are the same judgement -- splitting them is how a screen comes
// to draw a valid-looking echo above a confirm that does nothing.
//
// The confirm icon is drawn ONLY when the fragment is acceptable, which is
// confirmReviewScreen's ruling on inert controls applied here
// (gui/multisig_build.go:1919-1925).
func composerDigitEntry(ctx *Context, th *Colors, title, lead string, maxDigits int, echo func(string) (string, bool)) (string, bool) {
	kbd := NewKeyboard(ctx, composerDigitKeys)
	backBtn := &Clickable{Button: Button1}
	okBtn := &Clickable{Button: Button3}
	for !ctx.Done {
		for kbd.Update(ctx) {
		}
		if len(kbd.Fragment) > maxDigits {
			// The pad cannot refuse a keypress, so the cap is applied here. It
			// is a TRUNCATION rather than a refusal screen because the operator
			// can see the field and the next backspace fixes it.
			kbd.Fragment = kbd.Fragment[:maxDigits]
		}
		frag := kbd.Fragment
		line, valid := echo(frag)

		if backBtn.Clicked(ctx) {
			// Back is a decline everywhere on this device. Returning the
			// partial fragment would hand a half-typed operand to a lock.
			return "", false
		}
		// Button3 is always DRAINED, so it cannot block the queue head in a
		// direct-call test, and acted on only when the fragment is acceptable
		// -- the same shape inputCodex32Flow uses (gui/gui.go:1277-1280).
		clicked := okBtn.Clicked(ctx)
		if valid && clicked {
			return frag, true
		}

		dims := ctx.Platform.DisplaySize()
		screen := layout.Rectangle{Max: dims}
		_, content := screen.CutTop(leadingSize)
		content, _ = content.CutBottom(8)

		kbdOp, kbdsz := kbd.Layout(ctx, th)
		kbdOp = kbdOp.Offset(content.S(kbdsz))

		shown := frag
		if shown == "" {
			shown = " "
		}
		word, frgSize := widget.Labelw(&ctx.B, ctx.Styles.word, dims.X-50, th.Background, shown)
		frgSize.X = max(frgSize.X, 100)
		r := image.Rectangle{Max: frgSize}
		r.Min.Y -= 3
		r.Max.Y += buttonPadY
		r.Min.X -= buttonPadX
		r.Max.X += buttonPadX
		top, _ := content.CutBottom(kbdsz.Y)
		wordOff := top.Center(frgSize)
		word = op.Layer(
			word,
			op.Compose(
				op.Color(&ctx.B, th.Text),
				op.RoundedRect2(&ctx.B, r, cornerRadius),
			),
		).Offset(wordOff)

		var infoOps []op.Op
		lineY := wordOff.Y + frgSize.Y + 8
		for _, s := range []string{lead, line} {
			if s == "" {
				continue
			}
			lbl, sz := widget.Labelw(&ctx.B, ctx.Styles.body, dims.X-2*8, th.Text, s)
			y := lineY
			if lim := top.Max.Y - sz.Y; y > lim {
				y = lim
			}
			infoOps = append(infoOps, lbl.Offset(image.Pt((dims.X-sz.X)/2, y)))
			lineY = y + sz.Y + 4
		}

		navBtns := []NavButton{{Clickable: backBtn, Style: StyleSecondary, Icon: assets.IconBack}}
		if valid {
			navBtns = append(navBtns, NavButton{Clickable: okBtn, Style: StylePrimary, Icon: assets.IconCheckmark})
		}
		nav, _ := layoutNavigation(&ctx.B, th, dims, navBtns...)
		titleOp, _ := layoutTitle(ctx, dims.X, th.Text, title)

		frameOps := []op.Op{kbdOp, word}
		frameOps = append(frameOps, infoOps...)
		frameOps = append(frameOps, nav, titleOp, op.Color(&ctx.B, th.Background))
		ctx.Frame(op.Layer(frameOps...))
	}
	return "", false
}
