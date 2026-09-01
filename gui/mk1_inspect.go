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
	// chunked is Contract 2's explicit discriminator (device-csid-warning
	// R0-I3): whether the header that PRIMED this gatherer was itself
	// Header.Chunked. Set once, at prime time, from the header directly --
	// NEVER inferred from setID == 0 (a real mis-stamp value: `mk encode
	// --chunk-set-id 00000` on a genuinely chunked card) or total == 1 (a
	// chunked header can legally declare TotalChunks == 1: mk/mk.go's
	// parseHeaderSyms only guards total > maxChunks || index >= total). Both
	// proxies are wrong; this field is not a proxy.
	chunked bool
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
		g.chunked = h.Chunked
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

// isPrimed is chunkSink's half of F-76's payload priming; see md1Gatherer's.
func (g *mk1Gatherer) isPrimed() bool { return g.primed }

// collected returns the gathered chunk strings in ascending ChunkIndex order
// (0..total-1), deterministically — NEVER Go's randomized map-iteration order.
//
// F-162: this ranged the map until 2026-08-14, so a card's chunks came back in
// a different order on different runs. bundlePlatePlan walks these to build the
// engrave plan, so the plates of one card were cut in an arbitrary sequence and
// the "Plate 1 of 2" label — a claim about WHICH chunk is on the plate — was
// wrong about half the time. Caught by running the emulator walk three times
// and getting three different plate orders; md1 had the identical defect fixed
// at 3a23dbb and this line was missed.
//
// NOT a decode hazard, which is why it survived: mk.Decode reassembles by
// ChunkIndex into slots (mk/mk.go, reassemble) and tolerates any input order,
// as does the primary Rust mk-codec. Nothing compares mk1 chunk strings
// positionally either — bundle/verify.go's equalStrings is used on MD1 only, and
// both MK1 sides go through mk.Decode. So this was an ordering and labelling
// defect, never a wrong-key one.
//
// collected() is only ever called after complete() (mk1_inspect.go:155,175;
// bundle.go:194), which requires len(set) == total; and offer() cannot store an
// out-of-range index because mk.ParseHeader rejects ChunkIndex >= TotalChunks
// (mk/mk.go, parseHeaderSyms). Every index 0..total-1 is therefore populated and
// no lookup here can yield a "" gap.
func (g *mk1Gatherer) collected() []string {
	out := make([]string, 0, g.total)
	for i := 0; i < g.total; i++ {
		out = append(out, g.set[i])
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

// mk1GatherFlow collects a complete mk1 chunk set — from the LOADED PAYLOAD
// first, then via NFC — starting from the first scanned chunk, then decodes and
// returns the Card. It owns its own scanner goroutine (StartScreen.Flow has
// already closed its reader before engraveObjectFlow runs). Returns (Card,
// true) on a complete valid set, or (zero, false) on Back / decode error.
//
// F-76, as in md1GatherFlow: a payload-derived key card has no tags to tap, so
// the payload is offered to the gatherer before the reader is opened.
func mk1GatherFlow(ctx *Context, th *Colors, first string) (mk.Card, bool) {
	g := &mk1Gatherer{}
	g.offer(first) // first came from a ValidMK mdmkText; primes the set.
	syswPrimeCard(ctx, g)
	if g.complete() {
		return decodeGathered(ctx, th, g)
	}
	// One loop, one shape, one backoff -- see startScanner (F-126). A nil
	// reader is handled there and yields a channel that never delivers.
	scans, stopScanner := startScanner(ctx, ctx.Platform.NFCReader())
	defer stopScanner()
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
					msg = "Different key - rescan the right card."
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
	// device-csid-warning Contract 2: gated on the EXPLICIT g.chunked field
	// (R0-I3), never on g.setID == 0 or g.total == 1 (both real, representable
	// non-mismatch values). A NON-BLOCKING notice, shown BEFORE the card
	// display below returns to its caller (mk1DisplayFlow): every modal
	// answers BACK, and proceeding continues to the card (R1 — warning,
	// never refusal). A DerivedChunkSetID error is a defensive no-op, same
	// reasoning as offerChunkedMK1's (gui/bundle.go).
	if g.chunked {
		if derived, derr := mk.DerivedChunkSetID(card); derr == nil && derived != g.setID {
			showNotice(ctx, th, "Inspect key", csidMismatchWarningText(g.setID, derived))
		}
	}
	return card, true
}
