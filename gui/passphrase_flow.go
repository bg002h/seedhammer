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
// n is the length already held in dst, so stepping BACK into this screen
// restores what was typed instead of discarding a 100-character passphrase.
// RESIDUAL COPY (spec 5.3): seeding the keyboard converts dst[:n] to a string,
// which cannot be wiped. It is the same unwipeable copy the keyboard's own
// Fragment already holds, not an additional class of exposure.
//
// The step refuses to advance until ValidatePassphrase accepts, so an empty
// passphrase cannot pass; the refusal explains itself without echoing what was
// typed.
func passphraseEntryFlow(ctx *Context, th *Colors, dst []byte, n int) (int, bool) {
	kbd := NewPassphraseKeyboard(ctx)
	if n > 0 {
		kbd.Fragment = string(dst[:n])
	}
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

// ppFingerprintStep names which of the two optional fingerprint fields is
// being entered. They differ in exactly one thing that matters: how badly a
// wrong value can mislead (spec D1/D2, 5.1).
type ppFingerprintStep int

const (
	ppSeedFP ppFingerprintStep = iota
	ppCombinedFP
)

// The step titles mirror the plate's own band labels ("SEED FP: ",
// "EXPECTED COMB FP: ", backup/passphrase.go), so the screen the operator
// proof-reads and the steel they end up holding are labelled the same way.
func ppFingerprintTitle(which ppFingerprintStep) string {
	if which == ppCombinedFP {
		return "Expected Comb FP"
	}
	return "Seed FP"
}

// ppFingerprintHeading is the title once a complete fingerprint has been
// typed: the label and the CANONICAL value grouped 4-and-4, exactly as the
// plate carries it. The heading, not the keyboard's raw readout, is what the
// operator proof-reads -- the readout still shows whatever was typed, case and
// stray spaces included, which is not what gets engraved.
func ppFingerprintHeading(which ppFingerprintStep, typed string) string {
	title := ppFingerprintTitle(which)
	if grouped := ppFingerprintPreview(typed); grouped != "" {
		return title + ": " + grouped
	}
	return title
}

// The leads carry spec 5.1's warning. Neither value is checked by anything on
// this device: both are claims (spec D1). The combined fingerprint's is the
// stronger warning, because an incorrect passphrase does not fail -- it
// silently opens a different wallet.
const (
	ppSeedFPLead     = "Optional. Typed, never verified by this device."
	ppCombinedFPLead = "Optional. Typed, never verified. A wrong passphrase does not fail: it opens a DIFFERENT wallet."
)

func ppFingerprintLead(which ppFingerprintStep) string {
	if which == ppCombinedFP {
		return ppCombinedFPLead
	}
	return ppSeedFPLead
}

// Layout margins shared by the flow's keyboard screens, named so the fit tests
// measure the same budget the layout spends.
const (
	ppBottomMargin = 8
	ppSideMargin   = 8
)

// ppFingerprintPreview is the exact string the fingerprint screens echo for a
// typed fragment: the canonical value grouped 4-and-4, matching the plate
// (spec 4.3). It returns "" while the fragment is not yet a complete
// fingerprint, so a half-typed value is never shown as if it were accepted.
//
// The separator is a plain 0x20 and NEVER backup.SpaceMark: the mark means "a
// literal space in the passphrase", and hex is 0-9A-F, so a gap cannot be
// misread as a digit.
func ppFingerprintPreview(typed string) string {
	canon, err := passphrase.ValidateFingerprint(typed)
	if err != nil || canon == "" {
		return ""
	}
	return passphrase.GroupFingerprint(canon)
}

// fingerprintEntryFlow is step 2 or 3 of spec 5: an OPTIONAL, user-typed
// fingerprint. It returns the CANONICAL form -- whitespace stripped,
// uppercased -- or "" when the operator skips the field by accepting it empty.
//
// Returning the canonical form is load-bearing, not tidiness.
// backup.Passphrase documents SeedFP/CombinedFP as "canonical 8-hex-digit or
// empty" but nothing on the plate path enforces it, and
// passphrase.GroupFingerprint fails OPEN: anything that is not 8 characters
// comes back unchanged. A raw typed string therefore reaches the steel
// verbatim -- a 32-hex-digit value renders an 82mm metadata line, over spec
// 4.3's 64mm cap and through both corner screw-hole bands, with no error and
// no panic. This function is where that precondition is established.
//
// The keyboard is the CLEARTEXT one: a fingerprint is public, and masking it
// would only stop the operator proof-reading it.
func fingerprintEntryFlow(ctx *Context, th *Colors, which ppFingerprintStep) (string, bool) {
	kbd := NewAddressKeyboard(ctx)
	backBtn := &Clickable{Button: Button1}
	okBtn := &Clickable{Button: Button3}
	hookPPWidget("kbd", kbd)
	hookPPWidget("back", backBtn)
	hookPPWidget("ok", okBtn)
	title := ppFingerprintTitle(which)
	for !ctx.Done {
		for kbd.Update(ctx) {
		}
		if backBtn.Clicked(ctx) {
			return "", false
		}
		if okBtn.Clicked(ctx) {
			canon, err := passphrase.ValidateFingerprint(kbd.Fragment)
			if err != nil {
				showError(ctx, th, title, "Enter 8 hex digits (0-9, A-F), or leave it blank to skip.")
				continue
			}
			return canon, true
		}
		dims := ctx.Platform.DisplaySize()
		screen := layout.Rectangle{Max: dims}
		_, content := screen.CutTop(leadingSize)
		content, _ = content.CutBottom(ppBottomMargin)
		kbdOp, kbdsz := kbd.Layout(ctx, th)
		kbdOp = kbdOp.Offset(content.S(kbdsz))
		info, _ := content.CutBottom(kbdsz.Y)
		leadOp, leadsz := widget.Labelw(&ctx.B, ctx.Styles.lead, dims.X-2*ppSideMargin, th.Text, ppFingerprintLead(which))
		leadOp = leadOp.Offset(info.N(leadsz))
		nav, _ := layoutNavigation(&ctx.B, th, dims, []NavButton{
			{Clickable: backBtn, Style: StyleSecondary, Icon: assets.IconBack},
			{Clickable: okBtn, Style: StylePrimary, Icon: assets.IconCheckmark},
		}...)
		titleOp, _ := layoutTitle(ctx, dims.X, th.Text, ppFingerprintHeading(which, kbd.Fragment))
		ctx.Frame(op.Layer(kbdOp, leadOp, nav, titleOp, op.Color(&ctx.B, th.Background)))
	}
	return "", false
}

// engravePassphraseFlow is the engravePassphrase program (spec 5): enter a
// BIP-39 passphrase, optionally record the two user-typed fingerprints, choose
// whether to include a QR, review, and engrave.
//
// Back steps BACKWARDS through the flow rather than abandoning it, so a
// mis-tap on step 3 does not throw away a 100-character passphrase; only Back
// on the first step leaves the program.
//
// Phase D Tasks 1-3 wire the menu entry and the first three steps; the QR
// choice, confirm and engrave land in Tasks 4-5.
func engravePassphraseFlow(ctx *Context, th *Colors) {
	secret := make([]byte, passphrase.MaxLen)
	defer wipeBytes(secret)
	n := 0
	var seedFP, combinedFP string
	step := 0
	for !ctx.Done {
		switch step {
		case 0:
			m, ok := passphraseEntryFlow(ctx, th, secret, n)
			if !ok {
				return
			}
			n = m
		case 1:
			fp, ok := fingerprintEntryFlow(ctx, th, ppSeedFP)
			if !ok {
				step -= 2
				break
			}
			seedFP = fp
		case 2:
			fp, ok := fingerprintEntryFlow(ctx, th, ppCombinedFP)
			if !ok {
				step -= 2
				break
			}
			combinedFP = fp
		default:
			_, _ = seedFP, combinedFP
			return
		}
		step++
	}
}
