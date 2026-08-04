package gui

import (
	"errors"
	"fmt"
	"image"
	"math"
	"strings"

	qr "github.com/seedhammer/kortschak-qr"
	"seedhammer.com/backup"
	"seedhammer.com/engrave"
	"seedhammer.com/gui/assets"
	"seedhammer.com/gui/layout"
	"seedhammer.com/gui/op"
	"seedhammer.com/gui/widget"
)

// ftMaxLineLen is the title and footer cap, unconditional at every rung (spec
// 2). A rung-relative cap is unsound in both directions: anchored at 3.0mm a
// short text auto-fits at 6.0mm carrying a title that row cannot hold, and
// toPlate does not catch it; anchored at the CURRENT rung, deleting text raises
// the rung and retroactively invalidates a title already entered.
const ftMaxLineLen = backup.MaxTitleLen

// ftFit is one live evaluation of the composition: what the operator is told
// while typing, and what the plate will be.
type ftFit struct {
	sizeMM     float32
	lines      []string
	qrc        *qr.Code
	linesUsed  int
	linesAvail int
	ok         bool
	err        error
}

// ftEvaluate answers every live question at once, from ONE encode. Splitting it
// would let the readout, the refusal figure and the engraving disagree about
// the same text.
func ftEvaluate(params engrave.Params, text, title, footer string, useQR bool) ftFit {
	var f ftFit
	f.linesUsed, f.linesAvail, f.ok = backup.Admissible(params, text, title, footer, useQR)
	f.sizeMM, f.lines, f.qrc, f.err = backup.Fit(params, text, title, footer, useQR)
	return f
}

// ftSizeLabel is the readout: the fitted size and "lines used / lines
// available".
//
// NEVER "characters remaining". Under word wrap no scalar character count is
// correct -- appending "x" to the last word can cost a whole line while
// appending " x" does not -- so a character budget would be wrong in the one
// direction that matters, telling the operator there is room when there is not.
func ftSizeLabel(f ftFit) string {
	size := "--"
	if f.err == nil {
		size = fmt.Sprintf("%.1fmm", f.sizeMM)
	}
	return fmt.Sprintf("%s  %d/%d lines", size, f.linesUsed, f.linesAvail)
}

// ftQRChoiceFlow is step 1. It comes FIRST so the admission anchor is fixed
// before any text is typed: choosing a QR afterwards would shrink the capacity
// under text already accepted.
func ftQRChoiceFlow(ctx *Context, th *Colors, prior bool) (bool, bool) {
	cs := &ChoiceScreen{
		Title: "QR Code",
		Lead: "A QR is a machine-readable copy of the text. " +
			"Anyone who photographs the plate can read it.",
		Choices: []string{"No QR", "Add QR"},
	}
	if prior {
		cs.choice = 1 // preserve a deliberate opt-in across Back
	}
	hookPPWidget("qr", cs)
	// choice starts at 0, which is "No QR": the default is a property of this
	// ordering, so do not reorder the choices.
	sel, ok := cs.Choose(ctx, th)
	if !ok {
		return false, false
	}
	return sel == 1, true
}

// ftRefuse explains an over-capacity text and, when a QR is present, offers
// dropping it as an EXPLICIT choice with the live figure.
//
// The QR is never dropped automatically: it changes what a scanner returns from
// the plate, and doing that on the operator's behalf to make room is exactly
// the silent substitution this program exists to avoid.
func ftRefuse(ctx *Context, th *Colors, params engrave.Params, f ftFit, text string, useQR bool) bool {
	if !useQR {
		showError(ctx, th, "Text", fmt.Sprintf(
			"The text needs %d lines and a plate holds %d, at the smallest size. Shorten the Text field.",
			f.linesUsed, f.linesAvail))
		return false
	}
	freed := backup.MaxCharsAt(params, backup.FontSizes[len(backup.FontSizes)-1], text, false) -
		backup.MaxCharsAt(params, backup.FontSizes[len(backup.FontSizes)-1], text, true)
	cs := &ChoiceScreen{
		Title: "Too Long",
		Lead: fmt.Sprintf(
			"The Text field needs %d lines and a plate holds %d, at the smallest size. "+
				"Removing the QR frees about %d characters, and the plate stops being machine-readable.",
			f.linesUsed, f.linesAvail, freed),
		Choices: []string{"Keep the QR", "Remove the QR"},
	}
	hookPPWidget("refusal", cs)
	sel, ok := cs.Choose(ctx, th)
	return ok && sel == 1
}

// ftTextEntryFlow is step 2. Keystrokes are always accepted; the readout shows
// the over-capacity state and OK refuses, naming the field. Silently dropping
// keystrokes would leave the operator believing a longer text had been entered
// (gui/passphrase_flow.go:113-118's reviewed decision).
func ftTextEntryFlow(ctx *Context, th *Colors, params engrave.Params, prior, title, footer string, useQR *bool) (string, bool) {
	kbd := NewTextKeyboard(ctx)
	kbd.Fragment = prior
	backBtn := &Clickable{Button: Button1}
	okBtn := &Clickable{Button: Button3}
	hookPPWidget("kbd", kbd)
	hookPPWidget("back", backBtn)
	hookPPWidget("ok", okBtn)

	// The evaluation is cached on (text, qr) because it encodes a QR, and the
	// screen redraws every frame while the text changes only on a keystroke.
	var cache ftFit
	var cacheText string
	var cacheQR bool
	cacheValid := false
	evaluate := func() ftFit {
		if !cacheValid || cacheText != kbd.Fragment || cacheQR != *useQR {
			cache = ftEvaluate(params, kbd.Fragment, title, footer, *useQR)
			cacheText, cacheQR, cacheValid = kbd.Fragment, *useQR, true
		}
		return cache
	}

	for !ctx.Done {
		for kbd.Update(ctx) {
		}
		if backBtn.Clicked(ctx) {
			return "", false
		}
		if okBtn.Clicked(ctx) {
			if kbd.Fragment == "" {
				showError(ctx, th, "Text", "The Text field is required.")
				continue
			}
			f := evaluate()
			// ErrTooLarge is a GEOMETRY refusal and has a remedy the operator
			// can act on. Any other error is the encoder giving up -- qr.Encode
			// fails at 2954 bytes, which the uncapped Text field can reach --
			// and Fit returns a nil code either way, so the two cases have to
			// be told apart by the error and not by the code being nil.
			if f.err != nil && !errors.Is(f.err, backup.ErrTooLarge) {
				showError(ctx, th, "Text", "The text is too long to encode as a QR.")
				continue
			}
			if !f.ok || f.err != nil {
				if ftRefuse(ctx, th, params, f, kbd.Fragment, *useQR) {
					*useQR = false
				}
				continue
			}
			return kbd.Fragment, true
		}
		f := evaluate()
		dims := ctx.Platform.DisplaySize()
		screen := layout.Rectangle{Max: dims}
		_, content := screen.CutTop(leadingSize)
		content, _ = content.CutBottom(8)
		// Reserve the readout's band BEFORE the keyboard, and BOUND the
		// keyboard block: its readout grows with the text, and op.Layer draws
		// the keyboard on top, so an unreserved band lets the block cover the
		// very readout that says the text no longer fits.
		cntOp, cntsz := widget.Labelf(&ctx.B, ctx.Styles.subtitle, th.Text, "%s", ftSizeLabel(f))
		counterBand, content := content.CutTop(cntsz.Y)
		cntOp = cntOp.Offset(counterBand.N(cntsz))
		kbd.MaxHeight = content.Dy()
		kbdOp, kbdsz := kbd.Layout(ctx, th)
		kbdOp = kbdOp.Offset(content.S(kbdsz))
		nav, _ := layoutNavigation(&ctx.B, th, dims, []NavButton{
			{Clickable: backBtn, Style: StyleSecondary, Icon: assets.IconBack},
			{Clickable: okBtn, Style: StylePrimary, Icon: assets.IconCheckmark},
		}...)
		titleOp, _ := layoutTitle(ctx, dims.X, th.Text, "Text")
		ctx.Frame(op.Layer(kbdOp, cntOp, nav, titleOp, op.Color(&ctx.B, th.Background)))
	}
	return "", false
}

// ftLineEntryFlow is steps 3 and 4: one optional line, capped at ftMaxLineLen.
// Skippable -- OK on an empty field means "no title".
func ftLineEntryFlow(ctx *Context, th *Colors, what, prior string) (string, bool) {
	kbd := NewTextKeyboard(ctx)
	kbd.Fragment = prior
	backBtn := &Clickable{Button: Button1}
	okBtn := &Clickable{Button: Button3}
	hookPPWidget("kbd", kbd)
	hookPPWidget("back", backBtn)
	hookPPWidget("ok", okBtn)
	for !ctx.Done {
		for kbd.Update(ctx) {
		}
		if backBtn.Clicked(ctx) {
			return "", false
		}
		if okBtn.Clicked(ctx) {
			if strings.ContainsRune(kbd.Fragment, '\n') {
				showError(ctx, th, what, "The "+strings.ToLower(what)+" is a single line.")
				continue
			}
			if len(kbd.Fragment) > ftMaxLineLen {
				showError(ctx, th, what, fmt.Sprintf(
					"The %s holds %d characters and %d were entered. It sits on a screw-hole row at every size.",
					strings.ToLower(what), ftMaxLineLen, len(kbd.Fragment)))
				continue
			}
			return kbd.Fragment, true
		}
		dims := ctx.Platform.DisplaySize()
		screen := layout.Rectangle{Max: dims}
		_, content := screen.CutTop(leadingSize)
		content, _ = content.CutBottom(8)
		cntOp, cntsz := widget.Labelf(&ctx.B, ctx.Styles.subtitle, th.Text,
			"%d/%d  optional", len(kbd.Fragment), ftMaxLineLen)
		counterBand, content := content.CutTop(cntsz.Y)
		cntOp = cntOp.Offset(counterBand.N(cntsz))
		kbd.MaxHeight = content.Dy()
		kbdOp, kbdsz := kbd.Layout(ctx, th)
		kbdOp = kbdOp.Offset(content.S(kbdsz))
		nav, _ := layoutNavigation(&ctx.B, th, dims, []NavButton{
			{Clickable: backBtn, Style: StyleSecondary, Icon: assets.IconBack},
			{Clickable: okBtn, Style: StylePrimary, Icon: assets.IconCheckmark},
		}...)
		titleOp, _ := layoutTitle(ctx, dims.X, th.Text, what)
		ctx.Frame(op.Layer(kbdOp, cntOp, nav, titleOp, op.Color(&ctx.B, th.Background)))
	}
	return "", false
}

// ftConfirmWarnings is spec 9. Nothing on this plate has been validated, the
// machine's duration leaks the content, and a QR makes it readable from a
// photograph.
const (
	ftWarnNotBackup = "Nothing here is checked. This is not a validated backup: " +
		"no wordlist, no checksum, no verify step."
	ftWarnTiming = "Engraving is not constant-time. How long the machine runs " +
		"depends on what it is cutting, so anyone watching or timing it learns about the text."
	ftWarnQR = "The QR makes the text readable by any camera. A photograph of " +
		"the plate is a copy of the text."
)

// ftConfirmBody renders the confirm screen.
//
// Each wrapped line is its OWN UNWRAPPED label. A width-bounded label would
// re-wrap in the proportional screen face and break the single-wrap-function
// invariant this screen exists to demonstrate: the operator would approve lines
// the machine will not cut.
func ftConfirmBody(ctx *Context, th *Colors, width int, f ftFit, title, footer string, useQR bool) (op.Op, image.Point) {
	var rt richText
	if title != "" {
		rt.Add(&ctx.B, ctx.Styles.subtitle, math.MaxInt, th.Text, title)
	}
	for _, l := range f.lines {
		rt.Add(&ctx.B, ctx.Styles.body, math.MaxInt, th.Text, l)
	}
	if footer != "" {
		rt.Add(&ctx.B, ctx.Styles.subtitle, math.MaxInt, th.Text, footer)
	}
	rt.Y += 4
	rt.Add(&ctx.B, ctx.Styles.subtitle, width, th.Text,
		fmt.Sprintf("%.1fmm  %d lines  QR: %s", f.sizeMM, len(f.lines), ppYesNo(useQR)))
	rt.Y += 4
	rt.Add(&ctx.B, ctx.Styles.body, width, th.Text, ftWarnNotBackup)
	rt.Add(&ctx.B, ctx.Styles.body, width, th.Text, ftWarnTiming)
	if useQR {
		rt.Add(&ctx.B, ctx.Styles.body, width, th.Text, ftWarnQR)
	}
	return rt.Content, image.Pt(width, rt.Y)
}

// ftConfirmFlow is step 5: the last checkpoint before a permanent plate.
func ftConfirmFlow(ctx *Context, th *Colors, f ftFit, title, footer string, useQR bool) bool {
	backBtn := &Clickable{Button: Button1}
	okBtn := &Clickable{Button: Button3}
	hookPPWidget("back", backBtn)
	hookPPWidget("ok", okBtn)
	for !ctx.Done {
		if backBtn.Clicked(ctx) {
			return false
		}
		if okBtn.Clicked(ctx) {
			return true
		}
		dims := ctx.Platform.DisplaySize()
		area := ppConfirmArea(dims)
		body, _ := ftConfirmBody(ctx, th, area.Dx(), f, title, footer, useQR)
		body = body.Offset(image.Point(area.Min))
		nav, _ := layoutNavigation(&ctx.B, th, dims, []NavButton{
			{Clickable: backBtn, Style: StyleSecondary, Icon: assets.IconBack},
			{Clickable: okBtn, Style: StylePrimary, Icon: assets.IconCheckmark},
		}...)
		titleOp, _ := layoutTitle(ctx, dims.X, th.Text, "Confirm")
		ctx.Frame(op.Layer(body, nav, titleOp, op.Color(&ctx.B, th.Background)))
	}
	return false
}

// freetextPlateHook receives exactly what EngraveFreeText was handed. nil in
// production. Without it there is no way to bind the layout the operator
// APPROVED to the one that was ENGRAVED: the confirm screen is inspectable via
// op.Drawer.ExtractText, a bspline.Curve is not -- Plate is {Duration, Spline},
// stroke geometry carrying no text at all.
var freetextPlateHook func(fontMM float32, title string, lines []string, footer string, qrc *qr.Code)

// ftBuildPlate turns the fitted composition into an engravable plate.
//
// ONE call to Fit yields the size, the lines and the code, and all three go
// straight to EngraveFreeText. It never encodes a second time: Fit's code IS
// the artifact, so the fit path and the build path cannot disagree about what a
// scanner will return.
func ftBuildPlate(params engrave.Params, text, title, footer string, useQR bool) (Plate, error) {
	fontMM, lines, qrc, err := backup.Fit(params, text, title, footer, useQR)
	if err != nil {
		return Plate{}, err
	}
	if freetextPlateHook != nil {
		freetextPlateHook(fontMM, title, lines, footer, qrc)
	}
	return toPlate(backup.EngraveFreeText(params, fontMM, title, lines, footer, qrc), params)
}

// The steps of spec 7, in order. The QR choice is first so the admission anchor
// is fixed before typing.
type ftStep int

const (
	ftStepQR ftStep = iota
	ftStepText
	ftStepTitle
	ftStepFooter
	ftStepConfirm
	ftStepEngrave
)

// engraveTextFlow is the engraveText program (spec 7). Back from any step
// preserves every entered value.
func engraveTextFlow(ctx *Context, th *Colors) {
	params := ctx.Platform.EngraverParams()
	var text, title, footer string
	useQR := false
	step := ftStepQR
	for !ctx.Done {
		switch step {
		case ftStepQR:
			add, ok := ftQRChoiceFlow(ctx, th, useQR)
			if !ok {
				return // Back out of the first step leaves the program.
			}
			useQR = add
		case ftStepText:
			s, ok := ftTextEntryFlow(ctx, th, params, text, title, footer, &useQR)
			if !ok {
				step -= 2
				break
			}
			text = s
		case ftStepTitle:
			s, ok := ftLineEntryFlow(ctx, th, "Title", title)
			if !ok {
				step -= 2
				break
			}
			title = s
		case ftStepFooter:
			s, ok := ftLineEntryFlow(ctx, th, "Footer", footer)
			if !ok {
				step -= 2
				break
			}
			footer = s
		case ftStepConfirm:
			f := ftEvaluate(params, text, title, footer, useQR)
			if f.err != nil || !f.ok {
				// A title or footer entered after the text can only ever make
				// the composition SMALLER on the plate, never inadmissible --
				// admission reserves both rows unconditionally. So this is a
				// genuine surprise, and it goes back rather than engraving.
				showError(ctx, th, "Text", "This text does not fit a plate.")
				step -= 2
				break
			}
			if !ftConfirmFlow(ctx, th, f, title, footer, useQR) {
				step -= 2
				break
			}
		case ftStepEngrave:
			plate, err := ftBuildPlate(params, text, title, footer, useQR)
			if err != nil {
				// The message quotes no field content.
				showError(ctx, th, "Text", "This text does not fit a plate.")
				step -= 2
				break
			}
			if NewEngraveScreen(ctx, plate).Engrave(ctx, &engraveTheme) {
				return
			}
			// Backed out of the engrave: return to the confirm screen.
			step -= 2
		}
		step++
	}
}
