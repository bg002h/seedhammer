package gui

import (
	"image"

	"seedhammer.com/gui/op"
	"seedhammer.com/gui/widget"
)

// nfcFaulter is the OPTIONAL interface a platform implements when it can fail
// to turn its tag reader off (Refugium plan F7 §4.2, R0 round 3 M-7). Only the
// SH2 controller's Refugium build implements it (cmd/controller/nfc_refugium.go):
// the default controller, the emulator and the test platform do not, so
// Platform itself is unchanged.
type nfcFaulter interface {
	// NFCFault is non-nil when the boot-time field-off write failed, so the
	// reader's field state is unknown.
	NFCFault() error
}

// nfcFaultLead is the whole of the fault screen's text. It names no secret
// and asks for nothing: there is nothing the operator can do on this machine
// but switch it off.
const nfcFaultLead = "NFC could not be turned off. Power off."

// platformNFCFault reports a platform's NFC fault, or nil when the platform
// has none to report.
func platformNFCFault(p Platform) error {
	if f, ok := p.(nfcFaulter); ok {
		return f.NFCFault()
	}
	return nil
}

// nfcFaultScreen draws the fault and TAKES NO INPUT: no button, no tap target,
// no way forward. It returns only when the session ends (ctx.Done), and the
// session loop re-enters uiFlow, which draws it again.
func nfcFaultScreen(ctx *Context, th *Colors) {
	for !ctx.Done {
		dims := ctx.Platform.DisplaySize()
		titleOp, _ := layoutTitle(ctx, dims.X, th.Text, "NFC Fault")
		lbl, sz := widget.Labelw(&ctx.B, ctx.Styles.body, dims.X-2*8, th.Text, nfcFaultLead)
		body := lbl.Offset(image.Pt((dims.X-sz.X)/2, leadingSize+8))
		ctx.Frame(op.Layer(titleOp, body, op.Color(&ctx.B, th.Background)))
	}
}
