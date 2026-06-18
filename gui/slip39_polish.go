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

// slip39LengthPick asks how many words are on the user's physical share and
// returns the chosen word count ∈ {20,23,27,30,33}. The 20- and 33-word counts
// (the only ones mainstream wallets emit) are presented prominently; the three
// intermediate 160/192/224-bit counts follow. Returns 0 if the user backs out.
// (inputSLIP39Flow fills a pre-sized slice, so the length must be known at
// allocation; the user can simply count the words on their share — SPEC §5.2.)
func slip39LengthPick(ctx *Context, th *Colors) int {
	counts := []int{20, 33, 23, 27, 30}
	cs := &ChoiceScreen{
		Title: "SLIP-39 Share",
		Lead:  "Words on your share?",
		Choices: []string{
			"20 (128-bit)",
			"33 (256-bit)",
			"23 (160-bit)",
			"27 (192-bit)",
			"30 (224-bit)",
		},
	}
	sel, ok := cs.Choose(ctx, th)
	if !ok {
		return 0
	}
	return counts[sel]
}

// slip39ConfirmAction is the result of the pre-engrave SLIP-39 confirm screen.
type slip39ConfirmAction int

const (
	slip39Back    slip39ConfirmAction = iota // Button1
	slip39Engrave                            // Button3 / Center
	slip39Recover                            // Button2 — only when part of a multi-share set
)

// recoverable reports whether a share can be combined with siblings: it must be
// part of a set with a threshold ≥ 2 layer (member or group), so the Recover
// path always exercises a digest gate. A lone 1-of-1 share takes the verbatim
// Engrave path only.
func recoverableSLIP39(s slip39words.Share) bool {
	return s.MemberThreshold > 1 || s.GroupThreshold > 1
}

// confirmSLIP39Flow shows a pre-engrave review of a parsed SLIP-39 share. For a
// lone 1-of-1 share it offers Back/Engrave; for a share in a multi-share set it
// also offers Recover (reconstruct the seed from enough shares). Button2 is
// ALWAYS drained every frame — even when Recover is not offered — so an
// unconsumed event cannot block the router queue head in a direct-call
// (non-runUI) context. (Cycle-B R0-C1 EventRouter footgun.)
func confirmSLIP39Flow(ctx *Context, th *Colors, s slip39words.Share) slip39ConfirmAction {
	recover := recoverableSLIP39(s)
	lines := []string{
		fmt.Sprintf("id %d", s.Identifier),
		fmt.Sprintf("member %d of %d", s.MemberIndex+1, s.MemberThreshold),
	}
	if s.GroupCount > 1 {
		lines = append(lines, fmt.Sprintf("group %d of %d", s.GroupIndex+1, s.GroupCount))
	}
	lines = append(lines, fmt.Sprintf("%d words", len(s.Mnemonic)))
	if recover {
		lines = append(lines, "Engrave this share, or Recover the seed")
	}

	backBtn := &Clickable{Button: Button1}
	recoverBtn := &Clickable{Button: Button2}
	engraveBtn := &Clickable{Button: Button3, AltButton: Center}
	for !ctx.Done {
		if backBtn.Clicked(ctx) {
			return slip39Back
		}
		// Always drain Button2 — even for a lone share, where Recover is not
		// offered — so an unconsumed event cannot block the router queue head
		// in a direct-call (non-runUI) context. Act on it only when offered.
		recoverClicked := recoverBtn.Clicked(ctx)
		if recover && recoverClicked {
			return slip39Recover
		}
		if engraveBtn.Clicked(ctx) {
			return slip39Engrave
		}
		dims := ctx.Platform.DisplaySize()
		navBtns := []NavButton{
			{Clickable: backBtn, Style: StyleSecondary, Icon: assets.IconBack},
		}
		if recover {
			navBtns = append(navBtns, NavButton{Clickable: recoverBtn, Style: StyleSecondary, Icon: assets.IconRight})
		}
		navBtns = append(navBtns, NavButton{Clickable: engraveBtn, Style: StylePrimary, Icon: assets.IconHammer})
		nav, _ := layoutNavigation(&ctx.B, th, dims, navBtns...)
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
	return slip39Back
}

// engraveSLIP39 confirms a SLIP-39 share. A lone share is engraved verbatim; a
// share in a multi-share set may instead be Recovered into the master seed,
// which is then engraved as the native BIP-39 plate via backupWalletFlow.
// Always returns true (recognized/handled) — Back, a fit failure, and
// engrave-complete all return true, never falling to the caller's
// scanUnknownFormat ("Unknown format").
func engraveSLIP39(ctx *Context, th *Colors, scan slip39words.Share) bool {
	for {
		switch confirmSLIP39Flow(ctx, th, scan) {
		case slip39Back:
			return true
		case slip39Engrave:
			engraveSLIP39Verbatim(ctx, th, scan)
			return true
		case slip39Recover:
			// Wired in Task 4 (recover → acknowledgement → fingerprint →
			// backupWalletFlow). Placeholder: re-confirm.
			continue
		}
	}
}

// engraveSLIP39Verbatim engraves a single share's words verbatim onto a plate.
func engraveSLIP39Verbatim(ctx *Context, th *Colors, scan slip39words.Share) {
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
		return
	}
	plate, err := toPlate(seedSide, params)
	if err != nil {
		showError(ctx, th, "Too large", "Share doesn't fit a plate.")
		return
	}
	for {
		if NewEngraveScreen(ctx, plate).Engrave(ctx, &engraveTheme) {
			return
		}
	}
}
