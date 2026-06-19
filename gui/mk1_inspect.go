package gui

import (
	"fmt"
	"image"
	"strings"

	"seedhammer.com/gui/assets"
	"seedhammer.com/gui/layout"
	"seedhammer.com/gui/op"
	"seedhammer.com/gui/widget"
	"seedhammer.com/mk"
)

func hasMKPrefix(s string) bool {
	return strings.HasPrefix(s, "mk1") || strings.HasPrefix(s, "MK1")
}

// chunkString splits s into substrings of at most n runes (ASCII here), so the
// long base58 xpub renders as short non-wrapping display lines.
func chunkString(s string, n int) []string {
	var out []string
	for len(s) > n {
		out = append(out, s[:n])
		s = s[n:]
	}
	if len(s) > 0 {
		out = append(out, s)
	}
	return out
}

type gatherStatus int

const (
	gatherIgnored gatherStatus = iota // not an mk1 chunk / parse failed
	gatherForeign                     // valid mk1 but a different chunk set
	gatherDup                         // chunk index already captured
	gatherAdded                       // new chunk added
)

// mk1Gatherer accumulates mk1 chunk strings toward a complete set. Pure (no
// GUI/NFC) so it is unit-tested directly; mk1GatherFlow is a thin NFC shell.
type mk1Gatherer struct {
	set    map[int]string
	total  int
	setID  uint32
	primed bool
}

func (g *mk1Gatherer) offer(s string) gatherStatus {
	h, err := mk.ParseHeader(s)
	if err != nil {
		return gatherIgnored
	}
	if !g.primed {
		g.set = map[int]string{}
		g.total = h.TotalChunks
		g.setID = h.ChunkSetID
		g.primed = true
	} else if !h.Chunked || h.ChunkSetID != g.setID || h.TotalChunks != g.total {
		return gatherForeign
	}
	if _, ok := g.set[h.ChunkIndex]; ok {
		return gatherDup
	}
	g.set[h.ChunkIndex] = s
	return gatherAdded
}

func (g *mk1Gatherer) complete() bool { return g.primed && len(g.set) == g.total }

func (g *mk1Gatherer) collected() []string {
	out := make([]string, 0, len(g.set))
	for _, s := range g.set {
		out = append(out, s)
	}
	return out
}

// mk1DisplayFlow shows the decoded mk1 account metadata for verification. Read-
// only: no engrave, no NFC, no mutation. Measure-and-advance paging (the T1
// lesson): the long base58 xpub is chunked into short non-wrapping lines and
// paged gap-free so the tail is always reachable (spec invariant 2.10).
func mk1DisplayFlow(ctx *Context, th *Colors, card mk.Card) {
	fp := card.Fingerprint
	if fp == "" {
		fp = "none"
	}
	lines := []string{
		"Network: " + card.Network,
		"Path: " + card.Path,
		"Fingerprint: " + fp,
		fmt.Sprintf("Policy stubs: %d", len(card.Stubs)),
		"Account xpub:",
	}
	lines = append(lines, chunkString(card.Xpub, 20)...)

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
		titleOp, _ := layoutTitle(ctx, dims.X, th.Text, "mk1 key")
		nav, _ := layoutNavigation(&ctx.B, th, dims, []NavButton{
			{Clickable: backBtn, Style: StyleSecondary, Icon: assets.IconBack},
			{Clickable: pageBtn, Style: StylePrimary, Icon: assets.IconRight},
		}...)
		frameOps := append([]op.Op{nav, titleOp}, body...)
		frameOps = append(frameOps, op.Color(&ctx.B, th.Background))
		ctx.Frame(op.Layer(frameOps...))
	}
}
