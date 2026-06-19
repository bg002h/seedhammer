package gui

import (
	"fmt"
	"image"

	"seedhammer.com/address"
	"seedhammer.com/bip380"
	"seedhammer.com/gui/assets"
	"seedhammer.com/gui/layout"
	"seedhammer.com/gui/op"
	"seedhammer.com/gui/widget"
)

const addrMaxIndex = 49 // show indices 0..49; bounds the paging loop

// descriptorAddressFlow displays the descriptor's receive/change addresses for
// on-device verification. Display-only: no engrave, no NFC, no mutation. The
// caller opens this only when address.Supported(desc).
//
// Long bech32 addresses wrap across several rows, so this MEASURES each line and
// renders only the indices that FIT the content height, paging forward by the
// count actually shown — gap-free, so no index is ever dropped off-screen (spec
// §4.2). Recomputes only on entry / toggle / page (off any hot path; Measure
// emits no draw ops). dims is stable for the flow's lifetime.
func descriptorAddressFlow(ctx *Context, th *Colors, desc *bip380.Descriptor) {
	backBtn := &Clickable{Button: Button1}
	toggleBtn := &Clickable{Button: Button2}
	pageBtn := &Clickable{Button: Button3}

	dims := ctx.Platform.DisplaySize()
	lineWidth := dims.X - 2*8
	screen := layout.Rectangle{Max: dims}
	_, content := screen.CutTop(leadingSize)
	content, _ = content.CutBottom(leadingSize)
	contentTop := content.Min.Y + 8
	contentBottom := content.Max.Y

	start := uint32(0)
	change := false
	var lines []string
	shown := 0
	// recompute fills `lines` with the addresses that fit, starting at `start`:
	// the first index is included unconditionally (guarantees ≥1 shown + forward
	// progress); each subsequent index only while it fits the content height.
	recompute := func() bool {
		lines = lines[:0]
		shown = 0
		y := contentTop
		for i := uint32(0); start+i <= addrMaxIndex; i++ {
			idx := start + i
			var a string
			var err error
			if change {
				a, err = address.Change(desc, idx)
			} else {
				a, err = address.Receive(desc, idx)
			}
			if err != nil {
				showError(ctx, th, "Address", err.Error())
				return false
			}
			line := fmt.Sprintf("%d: %s", idx, a)
			sz := ctx.Styles.body.Measure(lineWidth, "%s", line)
			if i > 0 && y+sz.Y > contentBottom { // always include the first
				break
			}
			lines = append(lines, line)
			y += sz.Y + 6
			shown++
			if y > contentBottom { // a further line cannot fit
				break
			}
		}
		return true
	}
	if !recompute() {
		return
	}
	for !ctx.Done {
		if backBtn.Clicked(ctx) {
			return
		}
		if toggleBtn.Clicked(ctx) {
			change = !change
			start = 0
			if !recompute() {
				return
			}
		}
		if pageBtn.Clicked(ctx) {
			// Advance by the count shown (gap-free), bounded by addrMaxIndex.
			if start+uint32(shown) <= addrMaxIndex {
				start += uint32(shown)
				if !recompute() {
					return
				}
			}
		}
		title := "Receive addresses"
		if change {
			title = "Change addresses"
		}
		nav, _ := layoutNavigation(&ctx.B, th, dims, []NavButton{
			{Clickable: backBtn, Style: StyleSecondary, Icon: assets.IconBack},
			{Clickable: toggleBtn, Style: StyleSecondary, Icon: assets.IconEdit},
			{Clickable: pageBtn, Style: StylePrimary, Icon: assets.IconRight},
		}...)
		titleOp, _ := layoutTitle(ctx, dims.X, th.Text, title)
		body := make([]op.Op, 0, len(lines))
		y := contentTop
		for _, ln := range lines {
			lbl, sz := widget.Labelw(&ctx.B, ctx.Styles.body, lineWidth, th.Text, ln)
			body = append(body, lbl.Offset(image.Pt((dims.X-sz.X)/2, y)))
			y += sz.Y + 6
		}
		frameOps := append([]op.Op{nav, titleOp}, body...)
		frameOps = append(frameOps, op.Color(&ctx.B, th.Background))
		ctx.Frame(op.Layer(frameOps...))
	}
}
