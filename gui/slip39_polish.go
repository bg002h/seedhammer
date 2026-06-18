package gui

import (
	"fmt"
	"image"

	"seedhammer.com/backup"
	"seedhammer.com/font/constant"
	"seedhammer.com/gui/assets"
	"seedhammer.com/gui/layout"
	"seedhammer.com/gui/op"
	"seedhammer.com/gui/widget"
	slip39words "seedhammer.com/slip39"
)

// showError displays a dismissible error modal (Button3 dismisses) over a blank
// background; returns when dismissed or ctx.Done. (Generalizes showCodex32Error
// with a title parameter.)
func showError(ctx *Context, th *Colors, title, msg string) {
	errScr := &ErrorScreen{Title: title, Body: msg}
	for !ctx.Done {
		dims := ctx.Platform.DisplaySize()
		d, dismissed := errScr.Layout(ctx, th, dims)
		if dismissed {
			return
		}
		ctx.Frame(op.Layer(d, op.Color(&ctx.B, th.Background)))
	}
}

// confirmSLIP39Flow shows a pre-engrave review of a parsed SLIP-39 share.
// Back (Button1) → false; Engrave (Button3) → true.
func confirmSLIP39Flow(ctx *Context, th *Colors, s slip39words.Share) bool {
	lines := []string{
		fmt.Sprintf("id %d", s.Identifier),
		fmt.Sprintf("member %d of %d", s.MemberIndex+1, s.MemberThreshold),
	}
	if s.GroupCount > 1 {
		lines = append(lines, fmt.Sprintf("group %d of %d", s.GroupIndex+1, s.GroupCount))
	}
	lines = append(lines, fmt.Sprintf("%d words", len(s.Mnemonic)))

	backBtn := &Clickable{Button: Button1}
	engraveBtn := &Clickable{Button: Button3, AltButton: Center}
	for !ctx.Done {
		if backBtn.Clicked(ctx) {
			return false
		}
		if engraveBtn.Clicked(ctx) {
			return true
		}
		dims := ctx.Platform.DisplaySize()
		nav, _ := layoutNavigation(&ctx.B, th, dims, []NavButton{
			{Clickable: backBtn, Style: StyleSecondary, Icon: assets.IconBack},
			{Clickable: engraveBtn, Style: StylePrimary, Icon: assets.IconHammer},
		}...)
		titleOp, _ := layoutTitle(ctx, dims.X, th.Text, "Confirm SLIP-39 Share")

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
	return false
}

// engraveSLIP39 confirms a SLIP-39 share and engraves it verbatim. Always returns
// true (recognized/handled) — Back, a fit failure, and engrave-complete all
// return true, never falling to the caller's scanUnknownFormat ("Unknown format").
func engraveSLIP39(ctx *Context, th *Colors, scan slip39words.Share) bool {
	if !confirmSLIP39Flow(ctx, th, scan) {
		return true
	}
	seedDesc := backup.Seed{
		Mnemonic:     scan.Mnemonic, // canonical uppercase words; verbatim
		ShortestWord: slip39words.ShortestWord,
		LongestWord:  slip39words.LongestWord,
		Title:        fmt.Sprintf("%d #%d/%d", scan.Identifier, scan.MemberIndex+1, scan.MemberThreshold), // max "32767 #16/16" = 12 <= MaxTitleLen 18
		Font:         constant.Font,
	}
	params := ctx.Platform.EngraverParams()
	seedSide, err := backup.EngraveSeed(params, seedDesc)
	if err != nil {
		showError(ctx, th, "Too large", "Share doesn't fit a plate.")
		return true
	}
	plate, err := toPlate(seedSide, params)
	if err != nil {
		showError(ctx, th, "Too large", "Share doesn't fit a plate.")
		return true
	}
	for {
		if NewEngraveScreen(ctx, plate).Engrave(ctx, &engraveTheme) {
			return true
		}
	}
}
