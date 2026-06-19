package gui

import (
	"errors"
	"fmt"
	"image"
	"io"
	"log"
	"time"

	"seedhammer.com/address"
	"seedhammer.com/bip380"
	"seedhammer.com/gui/assets"
	"seedhammer.com/gui/layout"
	"seedhammer.com/gui/op"
	"seedhammer.com/gui/widget"
)

// verifyAddressFlow lets the user supply a candidate address — by NFC scan or by
// typing it — then verifies it against the descriptor. Read-only: no
// engrave/NFC-write/mutation.
func verifyAddressFlow(ctx *Context, th *Colors, desc *bip380.Descriptor) {
	cs := &ChoiceScreen{Title: "Verify address", Lead: "Input method", Choices: []string{"Scan", "Type"}}
	choice, ok := cs.Choose(ctx, th)
	if !ok {
		return
	}
	var candidate string
	if choice == 0 {
		candidate, ok = scanAddressFlow(ctx, th)
	} else {
		candidate, ok = typeAddressFlow(ctx, th)
	}
	if !ok {
		return
	}
	runVerify(ctx, th, desc, candidate)
}

// typeAddressFlow lets the user type a candidate address on an UNMASKED keyboard
// (a public address is not a secret). The fragment is case-preserving (no
// ToUpper). Returns (address, true) on OK, or ("", false) on Back. Validity is
// checked downstream by address.Find.
func typeAddressFlow(ctx *Context, th *Colors) (string, bool) {
	kbd := NewAddressKeyboard(ctx)
	backBtn := &Clickable{Button: Button1}
	okBtn := &Clickable{Button: Button3}
	for !ctx.Done {
		for kbd.Update(ctx) {
		}
		if backBtn.Clicked(ctx) {
			return "", false
		}
		if okBtn.Clicked(ctx) {
			return kbd.Fragment, true
		}
		dims := ctx.Platform.DisplaySize()
		screen := layout.Rectangle{Max: dims}
		_, content := screen.CutTop(leadingSize)
		content, _ = content.CutBottom(8)
		kbdOp, kbdsz := kbd.Layout(ctx, th)
		kbdOp = kbdOp.Offset(content.S(kbdsz))
		nav, _ := layoutNavigation(&ctx.B, th, dims, []NavButton{
			{Clickable: backBtn, Style: StyleSecondary, Icon: assets.IconBack},
			{Clickable: okBtn, Style: StylePrimary, Icon: assets.IconCheckmark},
		}...)
		title, _ := layoutTitle(ctx, dims.X, th.Text, "Enter address")
		ctx.Frame(op.Layer(kbdOp, nav, title, op.Color(&ctx.B, th.Background)))
	}
	return "", false
}

// scanAddressFlow collects a candidate address via NFC, returning the first
// scanned addressText. Mirrors mk1GatherFlow's scanner-shell idiom (own scanner
// goroutine; testPlatform.NFCReader()==nil → no goroutine, Back-only). Returns
// (address, true) on a scan, or ("", false) on Back.
func scanAddressFlow(ctx *Context, th *Colors) (string, bool) {
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
			return "", false
		}
		select {
		case scan := <-scans:
			if a, ok := scan.Object.(addressText); ok {
				return string(a), true
			}
			switch scan.Status {
			case scanUnknownFormat:
				msg = "Not a recognized address."
			case scanFailed:
				msg = "Scan failed — try again."
			}
		default:
		}
		lines := []string{"Scan the address QR."}
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
		titleOp, _ := layoutTitle(ctx, dims.X, th.Text, "Scan address")
		nav, _ := layoutNavigation(&ctx.B, th, dims, []NavButton{
			{Clickable: backBtn, Style: StyleSecondary, Icon: assets.IconBack},
		}...)
		frameOps := append([]op.Op{nav, titleOp}, body...)
		frameOps = append(frameOps, op.Color(&ctx.B, th.Background))
		ctx.Frame(op.Layer(frameOps...))
	}
	return "", false
}

// runVerify shows a ONE-SHOT non-blocking "Verifying…" frame, runs address.Find
// ONCE (outside any loop — a multisig gap scan can block for seconds on RP2350,
// R0-M3), then displays the result in a Back-able screen. Read-only: no
// engrave/NFC/mutation. The "Verifying…" frame is a single ctx.Frame (NOT a
// blocking showError-style modal — R0-C1), so Find actually runs after it.
func runVerify(ctx *Context, th *Colors, desc *bip380.Descriptor, candidate string) {
	dims := ctx.Platform.DisplaySize()
	{ // one-shot progress frame, then compute
		title, _ := layoutTitle(ctx, dims.X, th.Text, "Verifying…")
		ctx.Frame(op.Layer(title, op.Color(&ctx.B, th.Background)))
	}
	chain, index, found, err := address.Find(desc, candidate, 20)
	var body string
	switch {
	case errors.Is(err, address.ErrAddrUnparseable):
		body = "Invalid address."
	case errors.Is(err, address.ErrAddrWrongNetwork):
		body = "Address is for a different network."
	case err != nil:
		body = "Can't verify this address."
	case !found:
		body = "Not found in the first 20 receive or change addresses."
	default:
		chainName := "receive"
		if chain == 1 {
			chainName = "change"
		}
		// Degenerate range/wildcard-less descriptor reports (0,0); phrase plainly.
		body = fmt.Sprintf("Match: %s address #%d. Controlled by this descriptor.", chainName, index)
	}
	// Back-able result screen (mirror descriptorAddressFlow's nav/render).
	backBtn := &Clickable{Button: Button1}
	lineWidth := dims.X - 2*8
	for !ctx.Done {
		if backBtn.Clicked(ctx) {
			return
		}
		lbl, sz := widget.Labelw(&ctx.B, ctx.Styles.body, lineWidth, th.Text, body)
		bodyOp := lbl.Offset(image.Pt((dims.X-sz.X)/2, leadingSize+16))
		title, _ := layoutTitle(ctx, dims.X, th.Text, "Verify address")
		nav, _ := layoutNavigation(&ctx.B, th, dims, []NavButton{
			{Clickable: backBtn, Style: StyleSecondary, Icon: assets.IconBack},
		}...)
		ctx.Frame(op.Layer(nav, title, bodyOp, op.Color(&ctx.B, th.Background)))
	}
}
