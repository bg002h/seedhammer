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

const (
	addrPageSize = 5
	addrMaxStart = 50 // do not advance the window's start past this
)

// descriptorAddressFlow displays the descriptor's receive/change addresses for
// on-device verification. Display-only: no engrave, no NFC, no mutation. The
// caller opens this only when address.Supported(desc). Addresses are recomputed
// only on entry and on toggle/page events (off any hot path). (T1 / spec §4.2.)
func descriptorAddressFlow(ctx *Context, th *Colors, desc *bip380.Descriptor) {
	backBtn := &Clickable{Button: Button1}
	toggleBtn := &Clickable{Button: Button2}
	pageBtn := &Clickable{Button: Button3}
	start := uint32(0)
	change := false
	var lines []string
	recompute := func() bool {
		lines = lines[:0]
		for i := uint32(0); i < addrPageSize; i++ {
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
			lines = append(lines, fmt.Sprintf("%d: %s", idx, a))
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
			if start+addrPageSize <= addrMaxStart {
				start += addrPageSize
				if !recompute() {
					return
				}
			}
		}
		dims := ctx.Platform.DisplaySize()
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
		screen := layout.Rectangle{Max: dims}
		_, content := screen.CutTop(leadingSize)
		content, _ = content.CutBottom(leadingSize)
		body := make([]op.Op, 0, len(lines))
		y := content.Min.Y + 8
		for _, ln := range lines {
			lbl, sz := widget.Labelw(&ctx.B, ctx.Styles.body, dims.X-2*8, th.Text, ln)
			body = append(body, lbl.Offset(image.Pt((dims.X-sz.X)/2, y)))
			y += sz.Y + 6
		}
		frameOps := append([]op.Op{nav, titleOp}, body...)
		frameOps = append(frameOps, op.Color(&ctx.B, th.Background))
		ctx.Frame(op.Layer(frameOps...))
	}
}
