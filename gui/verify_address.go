package gui

import (
	"errors"
	"fmt"
	"image"

	"seedhammer.com/address"
	"seedhammer.com/bip380"
	"seedhammer.com/gui/assets"
	"seedhammer.com/gui/op"
	"seedhammer.com/gui/widget"
)

// verifyAddressFlow is a temporary stub so Task 2 compiles; Task 3 replaces it
// with the real Scan/Type input flow.
func verifyAddressFlow(ctx *Context, th *Colors, desc *bip380.Descriptor) {
	runVerify(ctx, th, desc, "")
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
