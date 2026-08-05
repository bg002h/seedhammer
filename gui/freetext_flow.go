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
	"seedhammer.com/font/constant"
	"seedhammer.com/font/sh"
	"seedhammer.com/font/vector"
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

// ftFace is the engraving face the composition is cut in, carried with the NAME
// the confirm screen shows. A bare *vector.Face has nothing in it a screen can
// print, and the face is not a detail the operator may be left to guess: it
// decides how much text fits (44 columns at 3.0mm in font/sh, 39 in
// font/constant) and what the plate looks like.
type ftFace struct {
	Name string
	Face *vector.Face
}

var (
	// ftFaceSH is the free-text plate's own face and the program's default.
	ftFaceSH = ftFace{"sh", sh.Font}
	// ftFaceConst is the face every seed, descriptor and passphrase plate is
	// cut in. The free-text program can engrave in it so that face can be
	// PROVEN at 3.0mm; see freetext_proof.go.
	ftFaceConst = ftFace{"constant", constant.Font}
)

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
func ftEvaluate(params engrave.Params, face ftFace, text, title, footer string, useQR bool) ftFit {
	var f ftFit
	f.linesUsed, f.linesAvail, f.ok = backup.Admissible(params, face.Face, text, title, footer, useQR)
	f.sizeMM, f.lines, f.qrc, f.err = backup.Fit(params, face.Face, text, title, footer, useQR)
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
func ftRefuse(ctx *Context, th *Colors, params engrave.Params, face ftFace, f ftFit, text string, useQR bool) bool {
	if !useQR {
		showError(ctx, th, "Text", fmt.Sprintf(
			"The text needs %d lines and a plate holds %d, at the smallest size. Shorten the Text field.",
			f.linesUsed, f.linesAvail))
		return false
	}
	freed := backup.MaxCharsAt(params, face.Face, backup.FontSizes[len(backup.FontSizes)-1], text, false) -
		backup.MaxCharsAt(params, face.Face, backup.FontSizes[len(backup.FontSizes)-1], text, true)
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
// loadProof, when non-nil, is called if the operator types one of the proof
// triggers and accepts the prompt. It writes the other fields and the face,
// which is why it takes pointers, and RETURNS the text it wrote so this screen
// re-seeds from the value that was actually stored rather than recomputing it.
func ftTextEntryFlow(ctx *Context, th *Colors, params engrave.Params, prior string, title, footer *string, face *ftFace, useQR *bool, loadProof func(*ftProof) string) (string, bool) {
	kbd := NewTextKeyboard(ctx)
	kbd.Fragment = prior
	backBtn := &Clickable{Button: Button1}
	okBtn := &Clickable{Button: Button3}
	hookPPWidget("kbd", kbd)
	hookPPWidget("back", backBtn)
	hookPPWidget("ok", okBtn)

	// The evaluation is cached on (text, qr, face) because it encodes a QR, and
	// the screen redraws every frame while the text changes only on a
	// keystroke. The FACE is part of the key: loading a proof changes it, and a
	// cache that ignored it would keep reporting the old face's line count and
	// fitted size for the new plate.
	var cache ftFit
	var cacheText string
	var cacheQR bool
	var cacheFace ftFace
	cacheValid := false
	evaluate := func() ftFit {
		if !cacheValid || cacheText != kbd.Fragment || cacheQR != *useQR || cacheFace != *face {
			cache = ftEvaluate(params, *face, kbd.Fragment, *title, *footer, *useQR)
			cacheText, cacheQR, cacheFace, cacheValid = kbd.Fragment, *useQR, *face, true
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
			// The trigger check runs BEFORE this field's own validation, and
			// before the fit evaluation: the pattern is chosen for the CURRENT
			// QR choice and face, so evaluating the literal "TEXTPROOF!" first
			// would tell the operator nothing useful.
			if loaded, ok := ftProofOffer(ctx, th, kbd.Fragment, loadProof); ok {
				// Stay on this screen (continue, do NOT fall through to the
				// return) so the operator sees what landed, and re-seed the
				// field from the text the loader actually wrote.
				kbd.Fragment = loaded
				continue
			}
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
				if ftRefuse(ctx, th, params, *face, f, kbd.Fragment, *useQR) {
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

// ftConfirmRow is one row of the plate as it will be cut: the title, one
// wrapped body line, or the footer. Kept as a list so the preview can be paged
// a whole row at a time and the pager's arithmetic is over PLATE ROWS, not
// pixels.
type ftConfirmRow struct {
	text string
	// head is true for the title and footer rows, which engrave on the
	// screw-hole rows and are drawn in the subtitle style to say so.
	head bool
}

func ftConfirmRows(f ftFit, title, footer string) []ftConfirmRow {
	rows := make([]ftConfirmRow, 0, len(f.lines)+2)
	if title != "" {
		rows = append(rows, ftConfirmRow{title, true})
	}
	for _, l := range f.lines {
		rows = append(rows, ftConfirmRow{l, false})
	}
	if footer != "" {
		rows = append(rows, ftConfirmRow{footer, true})
	}
	return rows
}

// ftConfirmSummary is the block that MUST be on the panel whatever the text
// says: the fitted size, the row count, the QR state, the face -- and the three
// safety warnings. It is measured at the panel's width and drawn BELOW the
// preview, but the preview's budget is what is left after it, so it can never
// be pushed off the bottom.
//
// Before the execution review this block simply followed the lines, and a
// 20-line composition put the size line at y=510 and all three warnings at
// y=537 on a panel 320 pixels tall: 136% overflow, entirely invisible, with a
// text-only test reporting them present because ExtractText ignores occlusion.
// That was a defect in the free-text feature at large -- any long text reached
// it -- not merely in the proof.
func ftConfirmSummary(ctx *Context, th *Colors, width int, f ftFit, face ftFace, useQR bool, pager string) (op.Op, image.Point) {
	var rt richText
	rt.Add(&ctx.B, ctx.Styles.subtitle, width, th.Text, fmt.Sprintf(
		"%.1fmm  %d lines  QR: %s  font: %s",
		f.sizeMM, len(f.lines), ppYesNo(useQR), face.Name))
	if pager != "" {
		rt.Add(&ctx.B, ctx.Styles.subtitle, width, th.Text, pager)
	}
	rt.Y += 4
	rt.Add(&ctx.B, ctx.Styles.body, width, th.Text, ftWarnNotBackup)
	rt.Add(&ctx.B, ctx.Styles.body, width, th.Text, ftWarnTiming)
	if useQR {
		rt.Add(&ctx.B, ctx.Styles.body, width, th.Text, ftWarnQR)
	}
	return rt.Content, image.Pt(width, rt.Y)
}

// ftConfirmView is one rendered page of the confirm screen.
type ftConfirmView struct {
	Content op.Op
	Size    image.Point
	// Shown is how many plate rows this page drew, Total how many there are.
	// The pager advances by Shown, so pages never skip or repeat a row.
	Shown int
	Total int
}

// ftConfirmBody renders the confirm screen's page starting at plate row start,
// inside height pixels.
//
// Each plate row is its OWN UNWRAPPED label. A width-bounded label would
// re-wrap in the proportional screen face and break the single-wrap-function
// invariant this screen exists to demonstrate: the operator would approve lines
// the machine will not cut.
//
// The summary's height is subtracted from the budget FIRST, and one pager row
// is reserved whether or not it ends up drawn, so the returned Size is <=
// height for every composition the flow will ever hand it. A page always draws
// at least one row, so the pager cannot stall; TestFTConfirmAlwaysFitsThePanel
// pins that the budget on the real panel is at least one row even in the
// tightest case.
func ftConfirmBody(ctx *Context, th *Colors, width, height, start int, f ftFit, face ftFace, title, footer string, useQR bool) ftConfirmView {
	rows := ftConfirmRows(f, title, footer)
	if start < 0 || start >= len(rows) {
		start = 0
	}
	// Measured with a pager string of the same shape as the real one, so the
	// reservation is exact rather than approximately right.
	_, probe := ftConfirmSummary(ctx, th, width, f, face, useQR, ftConfirmPager(0, 0, 0))
	budget := height - probe.Y

	var rt richText
	shown := 0
	for i := start; i < len(rows); i++ {
		st := ctx.Styles.body
		if rows[i].head {
			st = ctx.Styles.subtitle
		}
		m := st.Face.Metrics()
		next := rt.Y + m.Ascent.Ceil() + m.Descent.Ceil()
		if shown > 0 && next > budget {
			break
		}
		// math.MaxInt: never re-wrap. See the doc comment.
		rt.Add(&ctx.B, st, math.MaxInt, th.Text, rows[i].text)
		shown++
	}
	rt.Y += 4
	pager := ""
	if start > 0 || shown < len(rows) {
		pager = ftConfirmPager(start, shown, len(rows))
	}
	sum, sumSz := ftConfirmSummary(ctx, th, width, f, face, useQR, pager)
	return ftConfirmView{
		Content: op.Layer(rt.Content, sum.Offset(image.Pt(0, rt.Y))),
		Size:    image.Pt(width, rt.Y+sumSz.Y),
		Shown:   shown,
		Total:   len(rows),
	}
}

// ftConfirmPager names which plate rows this page is showing. Without it a
// paged preview is indistinguishable from a truncated one, and the operator
// would approve a plate having read only its first rows believing they were all
// of it.
func ftConfirmPager(start, shown, total int) string {
	return fmt.Sprintf("rows %d-%d of %d  >", start+1, start+shown, total)
}

// ftConfirmFlow is step 5: the last checkpoint before a permanent plate.
//
// Three buttons, not two: Back, "next page" and OK. The page button appears
// only when the preview does not fit at once, so a short text -- which is
// almost every real one -- still shows the whole plate and two buttons.
func ftConfirmFlow(ctx *Context, th *Colors, f ftFit, face ftFace, title, footer string, useQR bool) bool {
	backBtn := &Clickable{Button: Button1}
	pageBtn := &Clickable{Button: Button2}
	okBtn := &Clickable{Button: Button3}
	hookPPWidget("back", backBtn)
	hookPPWidget("page", pageBtn)
	hookPPWidget("ok", okBtn)
	start := 0
	view := ftConfirmView{}
	for !ctx.Done {
		if backBtn.Clicked(ctx) {
			return false
		}
		if okBtn.Clicked(ctx) {
			return true
		}
		if pageBtn.Clicked(ctx) {
			// Advance by what was actually drawn, so no row is skipped, and
			// wrap to the top rather than sticking at the end.
			if start+view.Shown < view.Total {
				start += view.Shown
			} else {
				start = 0
			}
		}
		dims := ctx.Platform.DisplaySize()
		area := ppConfirmArea(dims)
		view = ftConfirmBody(ctx, th, area.Dx(), area.Dy(), start, f, face, title, footer, useQR)
		body := view.Content.Offset(image.Point(area.Min))
		btns := []NavButton{
			{Clickable: backBtn, Style: StyleSecondary, Icon: assets.IconBack},
			{Clickable: okBtn, Style: StylePrimary, Icon: assets.IconCheckmark},
		}
		if view.Shown < view.Total {
			btns = append(btns, NavButton{Clickable: pageBtn, Style: StyleSecondary, Icon: assets.IconRight})
		}
		nav, _ := layoutNavigation(&ctx.B, th, dims, btns...)
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
//
// The FACE is handed over too. It is not recoverable from the plate either, and
// engraving a composition in a face other than the one it was fitted in puts
// lines wide of the grid they were wrapped to -- a defect no assertion on the
// size, the lines or the code can see.
var freetextPlateHook func(fnt *vector.Face, fontMM float32, title string, lines []string, footer string, qrc *qr.Code)

// ftBuildPlate turns the fitted composition into an engravable plate.
//
// ONE call to Fit yields the size, the lines and the code, and all three go
// straight to EngraveFreeText. It never encodes a second time: Fit's code IS
// the artifact, so the fit path and the build path cannot disagree about what a
// scanner will return.
func ftBuildPlate(params engrave.Params, face ftFace, text, title, footer string, useQR bool) (Plate, error) {
	fontMM, lines, qrc, err := backup.Fit(params, face.Face, text, title, footer, useQR)
	if err != nil {
		return Plate{}, err
	}
	if freetextPlateHook != nil {
		freetextPlateHook(face.Face, fontMM, title, lines, footer, qrc)
	}
	return toPlate(backup.EngraveFreeText(params, face.Face, fontMM, title, lines, footer, qrc), params)
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
	// The free-text plate's own face, unless a proof trigger asks for another
	// one. Held here rather than in ftTextEntryFlow so it survives Back exactly
	// as the other four fields do.
	face := ftFaceSH
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
			s, ok := ftTextEntryFlow(ctx, th, params, text, &title, &footer, &face, &useQR,
				ftProofLoader(&text, &title, &footer, &face, &useQR))
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
			f := ftEvaluate(params, face, text, title, footer, useQR)
			if f.err != nil || !f.ok {
				// A title or footer entered after the text can only ever make
				// the composition SMALLER on the plate, never inadmissible --
				// admission reserves both rows unconditionally. So this is a
				// genuine surprise, and it goes back rather than engraving.
				showError(ctx, th, "Text", "This text does not fit a plate.")
				step -= 2
				break
			}
			if !ftConfirmFlow(ctx, th, f, face, title, footer, useQR) {
				step -= 2
				break
			}
		case ftStepEngrave:
			plate, err := ftBuildPlate(params, face, text, title, footer, useQR)
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
