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
)

// bundleFlow is the engraveBundle program: gather a bundle of PUBLIC md1/mk1
// cards over NFC (Phase 1), review/confirm them (Phase 2), then engrave all
// cards' plates verbatim with cross-card guidance + a set-level abort (Phase 3,
// Task 4). The device analogue of host `me bundle`.
//
// SECURITY SPINE: md1/mk1 are PUBLIC → NFC-gathered. An ms1 (codex32 secret) is
// REFUSED in this channel (hand-typed only); a single mk1 is refused (malformed,
// no integrity). No secret is ever gathered, displayed, or engraved.
func bundleFlow(ctx *Context, th *Colors) {
	for {
		cards, ok := bundleGatherFlow(ctx, th)
		if !ok {
			return // Back / empty bundle.
		}
		if !bundleReviewFlow(ctx, th, cards) {
			// Back from review → resume adding cards. The gather flow starts a
			// fresh accumulator; the operator re-scans (mirrors single-card flows,
			// which also don't persist a half-built set across Back).
			continue
		}
		bundleEngrave(ctx, th, cards)
		return
	}
}

// ─── Phase 1: gather ─────────────────────────────────────────────────────────

// bundleGatherScreen holds the gather UI state: the accumulator and the last
// per-scan feedback message. Its rendering + status-mapping are factored out
// (pure) so they are unit-tested without an NFC reader.
type bundleGatherScreen struct {
	g   *bundleGatherer
	msg string
}

// feedback maps a per-offer status to the operator message (R0-C1/C2). The ms1
// and single-mk1 refusals are explicit, never silent.
func (s *bundleGatherScreen) feedback(status bundleOfferStatus) string {
	switch status {
	case bundleRefusedMs1:
		return "Type the ms1 share on-device — never over NFC."
	case bundleRefusedSingleMK1:
		return "Incomplete key card — scan all its chunks."
	case bundleCardComplete:
		return "Card added."
	case bundleAddedSingleMD1:
		return "Descriptor added."
	case bundleDuplicate:
		return "Already captured that card."
	case bundleDropped:
		return "Not an md1/mk1 card."
	default: // bundleChunkProgress — shown via the tally, no message.
		return ""
	}
}

// tally returns the running on-screen tally of verified cards by type.
func (s *bundleGatherScreen) tally() []string {
	var nMD, nMK int
	for _, c := range s.g.cards {
		switch c.kind {
		case cardMD1:
			nMD++
		case cardMK1:
			nMK++
		}
	}
	return []string{
		fmt.Sprintf("md1 descriptors: %d", nMD),
		fmt.Sprintf("mk1 keys: %d", nMK),
		"Scan a card, or Done.",
	}
}

// bundleGatherFlow accumulates distinct verified cards via NFC, returning them
// on "Done adding cards" (Button3) or (nil,false) on Back / an empty bundle. It
// owns its own scanner goroutine (clone of mk1GatherFlow's shell). With
// testPlatform.NFCReader()==nil the goroutine doesn't run; the gatherer +
// review flow are driven directly in tests.
func bundleGatherFlow(ctx *Context, th *Colors) ([]bundleCard, bool) {
	scr := &bundleGatherScreen{g: &bundleGatherer{}}
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
	doneBtn := &Clickable{Button: Button3, AltButton: Center}
	dims := ctx.Platform.DisplaySize()
	for !ctx.Done {
		if backBtn.Clicked(ctx) {
			return nil, false
		}
		if doneBtn.Clicked(ctx) {
			switch bundleDoneDecision(scr.g) {
			case bundleDoneEmpty:
				showError(ctx, th, "Engrave Bundle", "No complete cards yet — scan a card's chunks first.")
			case bundleDonePending:
				// A card is mid-chunk-set: warn it's incomplete and drop it so the
				// operator never engraves a partial. Then proceed with the complete
				// cards (if any) — or fall back to the gather screen if none.
				scr.g.dropPending()
				showError(ctx, th, "Engrave Bundle", "Dropped an incomplete card — scan all its chunks to include it.")
				if len(scr.g.cards) > 0 {
					return scr.g.cards, true
				}
			case bundleDoneProceed:
				return scr.g.cards, true
			}
		}
		select {
		case scan := <-scans:
			if scan.Object != nil {
				scr.msg = scr.feedback(scr.g.offer(scan.Object))
			}
		default:
		}
		lines := scr.tally()
		if scr.msg != "" {
			lines = append(lines, scr.msg)
		}
		lineWidth := dims.X - 2*8
		y := leadingSize + 8
		body := make([]op.Op, 0, len(lines))
		for _, ln := range lines {
			lbl, sz := widget.Labelw(&ctx.B, ctx.Styles.body, lineWidth, th.Text, ln)
			body = append(body, lbl.Offset(image.Pt((dims.X-sz.X)/2, y)))
			y += sz.Y + 6
		}
		titleOp, _ := layoutTitle(ctx, dims.X, th.Text, "Engrave Bundle")
		nav, _ := layoutNavigation(&ctx.B, th, dims, []NavButton{
			{Clickable: backBtn, Style: StyleSecondary, Icon: assets.IconBack},
			{Clickable: doneBtn, Style: StylePrimary, Icon: assets.IconCheckmark},
		}...)
		frameOps := append([]op.Op{nav, titleOp}, body...)
		frameOps = append(frameOps, op.Color(&ctx.B, th.Background))
		ctx.Frame(op.Layer(frameOps...))
	}
	return nil, false
}

// bundleDoneOutcome is the result of pressing "Done adding cards".
type bundleDoneOutcome int

const (
	bundleDoneEmpty   bundleDoneOutcome = iota // 0 complete cards — nothing to engrave
	bundleDonePending                          // a card is mid-chunk-set — warn + drop
	bundleDoneProceed                          // >=1 complete card, nothing pending
)

// bundleDoneDecision classifies the "Done" gate (Option A). A pending
// (half-scanned) card takes precedence so the operator is always warned before
// a partial card is stranded (risk #2), even when complete cards exist.
func bundleDoneDecision(g *bundleGatherer) bundleDoneOutcome {
	if g.pending() {
		return bundleDonePending
	}
	if len(g.cards) == 0 {
		return bundleDoneEmpty
	}
	return bundleDoneProceed
}

// ─── Phase 2: review / confirm ───────────────────────────────────────────────

// bundleReviewFlow shows the accumulated bundle (count + per-card type +
// verified summary) and lets the operator Confirm (Button3 → true) or Back
// (Button1 → false, resume adding). Read-only; every listed card is already
// integrity-verified (I-1).
func bundleReviewFlow(ctx *Context, th *Colors, cards []bundleCard) bool {
	lines := []string{fmt.Sprintf("%d cards verified:", len(cards))}
	for i, c := range cards {
		lines = append(lines, fmt.Sprintf("%d. %s ✓", i+1, c.label))
		if c.summary != "" {
			lines = append(lines, chunkString(c.summary, 24)...)
		}
	}

	backBtn := &Clickable{Button: Button1}
	contBtn := &Clickable{Button: Button3, AltButton: Center}
	pageBtn := &Clickable{Button: Button2}
	dims := ctx.Platform.DisplaySize()
	lineWidth := dims.X - 2*8
	contentTop := leadingSize + 8
	contentBottom := dims.Y - leadingSize
	start := 0
	for !ctx.Done {
		if backBtn.Clicked(ctx) {
			return false
		}
		if contBtn.Clicked(ctx) {
			return true
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
		titleOp, _ := layoutTitle(ctx, dims.X, th.Text, "Bundle")
		nav, _ := layoutNavigation(&ctx.B, th, dims, []NavButton{
			{Clickable: backBtn, Style: StyleSecondary, Icon: assets.IconBack},
			{Clickable: pageBtn, Style: StyleSecondary, Icon: assets.IconRight},
			{Clickable: contBtn, Style: StylePrimary, Icon: assets.IconCheckmark},
		}...)
		frameOps := append([]op.Op{nav, titleOp}, body...)
		frameOps = append(frameOps, op.Color(&ctx.B, th.Background))
		ctx.Frame(op.Layer(frameOps...))
	}
	return false
}

// bundleEngrave is the Phase-3 guided verbatim engrave — implemented in Task 4.
func bundleEngrave(ctx *Context, th *Colors, cards []bundleCard) {
	// Phase 3 — implemented in Task 4.
}
