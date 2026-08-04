package gui

import (
	"errors"
	"unicode/utf8"

	"seedhammer.com/gui/assets"
	"seedhammer.com/gui/layout"
	"seedhammer.com/gui/op"
	"seedhammer.com/gui/widget"
	"seedhammer.com/passphrase"
)

// passphraseWidgetHook is a test-only seam. Every interactive widget this flow
// constructs is handed to it under a stable name, so a touch test can aim taps
// at the hit areas that were actually drawn (op.Drawer.TagBounds) instead of at
// hardcoded pixels that drift silently with the layout. nil in production;
// mirrors bip85SeedHook, the sanctioned in-file test seam.
var passphraseWidgetHook func(name string, w any)

func hookPPWidget(name string, w any) {
	if passphraseWidgetHook != nil {
		passphraseWidgetHook(name, w)
	}
}

// ppEntryError maps a validation failure to an operator-facing message.
//
// SECRET HYGIENE (spec 5.3): the returned string is a CONSTANT. The passphrase
// package's sentinel errors carry no input either -- that is a documented
// property of ValidatePassphrase -- so there is no path from the typed
// passphrase into a message, a log line, or an error value.
func ppEntryError(err error) string {
	switch {
	case errors.Is(err, passphrase.ErrEmpty):
		return "Enter a passphrase before continuing."
	case errors.Is(err, passphrase.ErrTooLong):
		return "Too long. At most 100 characters fit on one plate."
	case errors.Is(err, passphrase.ErrNonASCII):
		return "This device can only engrave printable ASCII."
	default:
		return "That passphrase cannot be engraved."
	}
}

// passphraseEntryFlow is step 1 of spec 5: the REQUIRED passphrase, typed on
// the masked PassphraseKeyboard with a reveal toggle and a live n/100 counter.
//
// The accepted passphrase is copied into dst (which must hold
// passphrase.MaxLen bytes) and its length returned -- it is NOT returned as a
// string, because the caller owns a []byte it can wipe and cannot wipe a
// string (spec 5.3). See engravePassphraseFlow for what is and is not
// achievable there.
//
// The step refuses to advance until ValidatePassphrase accepts, so an empty
// passphrase cannot pass; the refusal explains itself without echoing what was
// typed.
func passphraseEntryFlow(ctx *Context, th *Colors, dst []byte) (int, bool) {
	kbd := NewPassphraseKeyboard(ctx)
	backBtn := &Clickable{Button: Button1}
	okBtn := &Clickable{Button: Button3}
	hookPPWidget("kbd", kbd)
	hookPPWidget("back", backBtn)
	hookPPWidget("ok", okBtn)
	for !ctx.Done {
		for kbd.Update(ctx) {
		}
		if backBtn.Clicked(ctx) {
			return 0, false
		}
		if okBtn.Clicked(ctx) {
			if err := passphrase.ValidatePassphrase(kbd.Fragment); err != nil {
				showError(ctx, th, "Passphrase", ppEntryError(err))
				continue
			}
			// copy, not append: dst is the caller's wipeable buffer and must
			// stay the same backing array.
			return copy(dst, kbd.Fragment), true
		}
		dims := ctx.Platform.DisplaySize()
		screen := layout.Rectangle{Max: dims}
		_, content := screen.CutTop(leadingSize)
		content, _ = content.CutBottom(8)
		kbdOp, kbdsz := kbd.Layout(ctx, th)
		kbdOp = kbdOp.Offset(content.S(kbdsz))
		// Live counter. Over-length is shown rather than clamped: silently
		// dropping keystrokes at 100 would leave the operator believing a
		// longer passphrase had been entered.
		cntOp, cntsz := widget.Labelf(&ctx.B, ctx.Styles.subtitle, th.Text,
			"%d/%d", utf8.RuneCountInString(kbd.Fragment), passphrase.MaxLen)
		cntOp = cntOp.Offset(content.N(cntsz))
		nav, _ := layoutNavigation(&ctx.B, th, dims, []NavButton{
			{Clickable: backBtn, Style: StyleSecondary, Icon: assets.IconBack},
			{Clickable: okBtn, Style: StylePrimary, Icon: assets.IconCheckmark},
		}...)
		title, _ := layoutTitle(ctx, dims.X, th.Text, "Passphrase")
		ctx.Frame(op.Layer(kbdOp, cntOp, nav, title, op.Color(&ctx.B, th.Background)))
	}
	return 0, false
}

// engravePassphraseFlow is the engravePassphrase program (spec 5): enter a
// BIP-39 passphrase, optionally record the two user-typed fingerprints, choose
// whether to include a QR, review, and engrave.
//
// Phase D Task 1 wires the menu entry; the remaining steps land in Tasks 3-5.
func engravePassphraseFlow(ctx *Context, th *Colors) {
	secret := make([]byte, passphrase.MaxLen)
	defer wipeBytes(secret)
	if _, ok := passphraseEntryFlow(ctx, th, secret); !ok {
		return
	}
}
