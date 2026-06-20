package gui

import (
	"errors"
	"fmt"
	"image"
	"io"
	"log"
	"time"

	"seedhammer.com/gui/assets"
	"seedhammer.com/gui/op"
	"seedhammer.com/gui/widget"
	"seedhammer.com/md"
)

// md1Gatherer accumulates md1 chunk strings toward a complete set. A near-clone
// of mk1Gatherer (gui/mk1_inspect.go:48-83): the only difference is the parser
// (md.ParseChunkHeader vs mk.ParseHeader) and the single/chunked discriminator
// (md.ParseChunkHeader returns Chunked=false for a single md1, which the
// !h.Chunked guard treats as foreign once primed). Pure (no GUI/NFC) so it is
// unit-tested directly.
type md1Gatherer struct {
	set    map[int]string
	total  int
	setID  uint32
	primed bool
}

func (g *md1Gatherer) offer(s string) gatherStatus {
	h, err := md.ParseChunkHeader(s)
	if err != nil {
		return gatherIgnored
	}
	if !g.primed {
		// A single (non-chunked) md1 cannot prime a chunk set — there is nothing
		// to gather. Treat it as not-an-md1-chunk so the gather flow ignores it.
		if !h.Chunked {
			return gatherIgnored
		}
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

func (g *md1Gatherer) complete() bool { return g.primed && len(g.set) == g.total }

// collected returns the gathered chunk strings in ascending ChunkIndex order
// (0..total-1), deterministically — NEVER Go's randomized map-iteration order.
// The deterministic comparator (bundle.Verify) compares md1 positionally against
// the index-ordered derived side (md.split emits index order), so a random
// readback order would FALSE-FAIL a correct backup. collected() is only ever
// called after complete() (md1_gather.go:76,140; bundle.go:234), which requires
// every index 0..total-1 present, so each lookup is populated (no "" gaps).
func (g *md1Gatherer) collected() []string {
	out := make([]string, 0, len(g.set))
	for i := 0; i < g.total; i++ {
		out = append(out, g.set[i])
	}
	return out
}

// md1GatherFlow collects a complete md1 chunk set via NFC, starting from the
// first scanned chunk, then reassembles + expands + displays it. It owns its own
// scanner goroutine (a near-clone of mk1GatherFlow, gui/mk1_inspect.go:156-256;
// the completion action differs). On completion it hands off to
// gatheredDescriptorFlow. Returns true if a complete set was processed, false on
// Back. testPlatform.NFCReader()==nil → no goroutine, Back-only (multi-chunk
// sets can't complete without a reader).
func md1GatherFlow(ctx *Context, th *Colors, first string) bool {
	g := &md1Gatherer{}
	g.offer(first) // first came from a chunked md1 mdmkText; primes the set.
	if g.complete() {
		gatheredDescriptorFlow(ctx, th, g.collected())
		return true
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
			return false
		}
		select {
		case scan := <-scans:
			if s, ok := scan.Object.(mdmkText); ok {
				switch g.offer(string(s)) {
				case gatherAdded:
					msg = ""
					if g.complete() {
						gatheredDescriptorFlow(ctx, th, g.collected())
						return true
					}
				case gatherForeign:
					msg = "Different descriptor — rescan the right chunks."
				case gatherDup:
					msg = "Already captured that chunk."
				case gatherIgnored:
					msg = "Not an md1 descriptor chunk."
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
		titleOp, _ := layoutTitle(ctx, dims.X, th.Text, "Inspect descriptor")
		nav, _ := layoutNavigation(&ctx.B, th, dims, []NavButton{
			{Clickable: backBtn, Style: StyleSecondary, Icon: assets.IconBack},
		}...)
		frameOps := append([]op.Op{nav, titleOp}, body...)
		frameOps = append(frameOps, op.Color(&ctx.B, th.Background))
		ctx.Frame(op.Layer(frameOps...))
	}
	return false
}

// gatheredDescriptorFlow is the gather-completion handler (D6). ALL work that
// could allocate — reassemble, integrity-check, expand, build the
// *bip380.Descriptor — happens HERE, before entering the alloc-gated
// DescriptorScreen. It never runs per-frame.
//
// Routing:
//   - csid mismatch (errors.Is ErrChunkSetIDMismatch, R0-C1) → a distinct
//     "chunks don't match" error.
//   - any other decode/reassemble error → a generic decode error.
//   - expandOK → the full descriptor display + address-verify (descriptorFlow).
//   - expandTemplateOnly (D3, no xpubs) → read-only template display.
//   - expandUnsupported (D2/D5, non-bip380 shape) → a clear "complex policy —
//     display only" notice, then the read-only template display.
func gatheredDescriptorFlow(ctx *Context, th *Colors, collected []string) {
	tpl, keys, err := md.ExpandWalletPolicyChunks(collected)
	if err != nil {
		switch {
		case errors.Is(err, md.ErrChunkSetIDMismatch):
			showError(ctx, th, "Inspect descriptor", "Chunks don't match — mixed or tampered set.")
		default:
			showError(ctx, th, "Inspect descriptor", "Can't decode this descriptor set.")
		}
		return
	}
	desc, status := expandedToDescriptor(tpl, keys)
	switch status {
	case expandOK:
		descriptorFlow(ctx, th, desc)
	case expandTemplateOnly:
		md1DisplayFlow(ctx, th, tpl)
	default: // expandUnsupported
		showError(ctx, th, "Inspect descriptor", "Complex policy — display only.")
		md1DisplayFlow(ctx, th, tpl)
	}
}
