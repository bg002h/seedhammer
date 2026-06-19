package gui

import (
	"fmt"
	"image"
	"strings"

	"seedhammer.com/gui/assets"
	"seedhammer.com/gui/layout"
	"seedhammer.com/gui/op"
	"seedhammer.com/gui/widget"
	"seedhammer.com/md"
)

func hasMDPrefix(s string) bool {
	return strings.HasPrefix(s, "md1") || strings.HasPrefix(s, "MD1")
}

// scriptName maps a ScriptKind to its descriptor-script display name.
func scriptName(k md.ScriptKind) string {
	switch k {
	case md.ScriptPkh:
		return "P2PKH"
	case md.ScriptSh:
		return "P2SH"
	case md.ScriptWsh:
		return "P2WSH"
	case md.ScriptTr:
		return "P2TR"
	default:
		return "P2WPKH"
	}
}

// policyLine summarizes the spending policy in one line.
func policyLine(tpl md.Template) string {
	switch tpl.Policy {
	case md.PolicySingle:
		return "single-key"
	case md.PolicyMulti:
		return fmt.Sprintf("%d-of-%d multisig", tpl.K, tpl.M)
	case md.PolicySortedMulti:
		return fmt.Sprintf("%d-of-%d multisig (sorted)", tpl.K, tpl.M)
	case md.PolicyMultiA:
		return fmt.Sprintf("%d-of-%d multisig (tapscript)", tpl.K, tpl.M)
	case md.PolicySortedMultiA:
		return fmt.Sprintf("%d-of-%d multisig (sorted tapscript)", tpl.K, tpl.M)
	default:
		return "complex"
	}
}

// md1Summary builds the display lines for a decoded md1 Template. A
// non-renderable (complex) policy is refused rather than rendered partially.
func md1Summary(tpl md.Template) []string {
	var lines []string
	if tpl.Renderable {
		lines = append(lines, "Type: "+scriptName(tpl.Root)+" "+policyLine(tpl))
	} else {
		lines = append(lines, "Complex policy — cannot display safely.", fmt.Sprintf("Keys: %d", tpl.N))
	}
	for _, k := range tpl.Keys {
		fp := k.Fingerprint
		if fp == "" {
			fp = "—"
		}
		lines = append(lines, fmt.Sprintf("@%d %s %s %s", k.Index, fp, k.OriginPath, k.UseSite))
	}
	return lines
}

// md1DisplayFlow shows the decoded md1 descriptor template for verification.
// Read-only: no engrave, no NFC, no mutation. Measure-and-advance paging (the
// T1 lesson): long lines are chunked into short non-wrapping segments and paged
// gap-free so the tail is always reachable (spec invariant 2.10). Mirrors
// mk1DisplayFlow.
func md1DisplayFlow(ctx *Context, th *Colors, tpl md.Template) {
	var lines []string
	for _, ln := range md1Summary(tpl) {
		if len(ln) > 20 {
			lines = append(lines, chunkString(ln, 20)...)
		} else {
			lines = append(lines, ln)
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
				start = 0
			}
			continue
		}
		titleOp, _ := layoutTitle(ctx, dims.X, th.Text, "md1 descriptor")
		nav, _ := layoutNavigation(&ctx.B, th, dims, []NavButton{
			{Clickable: backBtn, Style: StyleSecondary, Icon: assets.IconBack},
			{Clickable: pageBtn, Style: StylePrimary, Icon: assets.IconRight},
		}...)
		frameOps := append([]op.Op{nav, titleOp}, body...)
		frameOps = append(frameOps, op.Color(&ctx.B, th.Background))
		ctx.Frame(op.Layer(frameOps...))
	}
}
