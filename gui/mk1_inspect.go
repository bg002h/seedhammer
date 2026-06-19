package gui

import (
	"errors"
	"fmt"
	"image"
	"io"
	"log"
	"strings"
	"time"

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

// mk1GatherFlow collects a complete mk1 chunk set via NFC, starting from the
// first scanned chunk, then decodes and returns the Card. It owns its own
// scanner goroutine (StartScreen.Flow has already closed its reader before
// engraveObjectFlow runs). Returns (Card, true) on a complete valid set, or
// (zero, false) on Back / decode error.
func mk1GatherFlow(ctx *Context, th *Colors, first string) (mk.Card, bool) {
	g := &mk1Gatherer{}
	g.offer(first) // first came from a ValidMK mdmkText; primes the set.
	if g.complete() {
		return decodeGathered(ctx, th, g)
	}
	scans := make(chan scanResult, 1)
	if r := ctx.Platform.NFCReader(); r != nil {
		closer := make(chan struct{})
		closed := make(chan struct{})
		defer func() {
			close(closer)
			r.Close()
			<-closed
		}()
		wakeup := ctx.Platform.Wakeup
		go func() {
			s := new(scanner)
			for {
				select {
				case <-closer:
					close(closed)
					return
				default:
				}
				obj, err := s.Scan(r)
				scan := scanResult{Object: obj}
				switch {
				case errors.Is(err, errScanInProgress):
					scan.Status = scanStarted
				case errors.Is(err, errScanUnknownFormat):
					scan.Status = scanUnknownFormat
				case err == nil || err == io.EOF:
				default:
					scan.Status = scanFailed
					log.Printf("nfc scan: %v", err)
				}
				select {
				case old := <-scans:
					if scan.Object == nil {
						scan.Object = old.Object
					}
					scan.Status = max(scan.Status, old.Status)
				default:
				}
				scans <- scan
				wakeup()
				if scan.Status == scanFailed {
					time.Sleep(1 * time.Second)
				}
			}
		}()
	}
	backBtn := &Clickable{Button: Button1}
	dims := ctx.Platform.DisplaySize()
	msg := ""
	for !ctx.Done {
		if backBtn.Clicked(ctx) {
			return mk.Card{}, false
		}
		select {
		case scan := <-scans:
			if s, ok := scan.Object.(mdmkText); ok {
				switch g.offer(string(s)) {
				case gatherAdded:
					msg = ""
					if g.complete() {
						return decodeGathered(ctx, th, g)
					}
				case gatherForeign:
					msg = "Different key — rescan the right card."
				case gatherDup:
					msg = "Already captured that chunk."
				case gatherIgnored:
					msg = "Not an mk1 key chunk."
				}
			}
		default:
		}
		lines := []string{fmt.Sprintf("Captured %d of %d.", len(g.set), g.total), "Scan the next chunk."}
		if msg != "" {
			lines = append(lines, msg)
		}
		lineWidth := dims.X - 2*8
		y := leadingSize + 8
		body := make([]op.Op, 0, len(lines))
		for _, ln := range lines {
			lbl, sz := widget.Labelw(&ctx.B, ctx.Styles.body, lineWidth, th.Text, ln)
			body = append(body, lbl.Offset(image.Pt((dims.X-sz.X)/2, y)))
			y += sz.Y + 6
		}
		titleOp, _ := layoutTitle(ctx, dims.X, th.Text, "Inspect key")
		nav, _ := layoutNavigation(&ctx.B, th, dims, []NavButton{
			{Clickable: backBtn, Style: StyleSecondary, Icon: assets.IconBack},
		}...)
		frameOps := append([]op.Op{nav, titleOp}, body...)
		frameOps = append(frameOps, op.Color(&ctx.B, th.Background))
		ctx.Frame(op.Layer(frameOps...))
	}
	return mk.Card{}, false
}

func decodeGathered(ctx *Context, th *Colors, g *mk1Gatherer) (mk.Card, bool) {
	card, err := mk.Decode(g.collected())
	if err != nil {
		showError(ctx, th, "Inspect key", "Can't decode this key set.")
		return mk.Card{}, false
	}
	return card, true
}
