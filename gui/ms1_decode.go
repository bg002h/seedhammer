package gui

import (
	"encoding/hex"
	"fmt"
	"image"

	"seedhammer.com/bip39"
	"seedhammer.com/codex32"
	"seedhammer.com/gui/assets"
	"seedhammer.com/gui/layout"
	"seedhammer.com/gui/op"
	"seedhammer.com/gui/widget"
)

// ms1DecodeFlow decodes and DISPLAYS the unshared ms1 secret: the BIP-39 words
// (language 0 = English) or the language name + entropy hex (non-English, since
// the fork ships only the English wordlist). Display-only: no engrave, no NFC,
// no mutation. SECRET — the entropy buffer is scrubbed on return (the displayed
// strings are immutable Go strings and live until GC, as with SeedScreen).
func ms1DecodeFlow(ctx *Context, th *Colors, scan codex32.String) {
	_, language, entropy, err := codex32.DecodeMS1(scan)
	if err != nil {
		// Unreachable in practice: callers gate this flow on DecodeMS1 already
		// succeeding. Keep a clean generic message; never surface the raw error.
		showError(ctx, th, "Secret", "Can't decode this secret.")
		return
	}
	defer wipeBytes(entropy)

	var lines []string
	if language == 0 { // English (entr or mnem-English) → the words
		m := bip39.New(entropy)
		for i, w := range m {
			lines = append(lines, fmt.Sprintf("%d %s", i+1, bip39.LabelFor(w)))
		}
	} else { // non-English mnem → name + hex + warning, never English words
		name := codex32.MSLanguageNames[language]
		lines = []string{
			"Language: " + name,
			"entropy: " + hex.EncodeToString(entropy),
			"Words not shown on this device.",
			"Restore with a " + name + " BIP-39 wallet.",
		}
	}

	backBtn := &Clickable{Button: Button1}
	pageBtn := &Clickable{Button: Button3}
	dims := ctx.Platform.DisplaySize()
	lineWidth := dims.X - 2*8
	screen := layout.Rectangle{Max: dims}
	_, content := screen.CutTop(leadingSize)
	content, _ = content.CutBottom(leadingSize)
	contentTop := content.Min.Y + 8
	contentBottom := content.Max.Y
	start := 0
	for !ctx.Done {
		if backBtn.Clicked(ctx) {
			return
		}
		// Measure-and-advance: render only the lines that fit; page forward by
		// the count shown (gap-free; the T1 lesson — never fixed-page wrapping text).
		shown := 0
		y := contentTop
		body := make([]op.Op, 0, len(lines))
		for i := start; i < len(lines); i++ {
			lbl, sz := widget.Labelw(&ctx.B, ctx.Styles.body, lineWidth, th.Text, lines[i])
			if i > start && y+sz.Y > contentBottom {
				break
			}
			body = append(body, lbl.Offset(image.Pt((dims.X-sz.X)/2, y)))
			y += sz.Y + 6
			shown++
			if y > contentBottom {
				break
			}
		}
		if pageBtn.Clicked(ctx) {
			if start+shown < len(lines) {
				start += shown
			} else {
				start = 0 // wrap to the top
			}
			continue
		}
		titleOp, _ := layoutTitle(ctx, dims.X, th.Text, "Secret")
		nav, _ := layoutNavigation(&ctx.B, th, dims, []NavButton{
			{Clickable: backBtn, Style: StyleSecondary, Icon: assets.IconBack},
			{Clickable: pageBtn, Style: StylePrimary, Icon: assets.IconRight},
		}...)
		frameOps := append([]op.Op{nav, titleOp}, body...)
		frameOps = append(frameOps, op.Color(&ctx.B, th.Background))
		ctx.Frame(op.Layer(frameOps...))
	}
}
