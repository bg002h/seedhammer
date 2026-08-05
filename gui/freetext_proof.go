package gui

import (
	"image"

	"seedhammer.com/gui/assets"
	"seedhammer.com/gui/op"
)

// The TEXTPROOF! test pattern for the free-text plate (followup
// seedhammer-textproof; user design 2026-08-04).
//
// O1 asks a question a render cannot answer: are these glyphs legible cut into
// steel? The free-text plate makes that harder than the passphrase plate did,
// because the size is chosen by auto-fit -- so the LENGTH of the proof selects
// which rung is being tested. Measured at production stroke with the title and
// footer below:
//
//	63 chars -> 6.0mm      282 chars -> 5.0mm      758 chars -> 3.0mm
//	95 chars -> 6.0mm      405 chars -> 3.8mm
//
// Both patterns here are sized to land at **3.0mm**, the smallest rung and the
// hardest legibility case. If a glyph reads at 3.0mm it reads at every rung.
//
// Two lengths exist because the QR competes for the same plate. Measured, the
// largest text that fits at 3.0mm is 816 characters without a QR and 516 with
// one -- so a single pattern cannot do both. ftProofFor picks by the QR choice,
// which the flow has already made by the time the text field is reached.
const ftProofTrigger = "TEXTPROOF!"

// The building blocks, kept separate so the two patterns share them verbatim.
const (
	// Every printable ASCII character in codepoint order, one each. The space
	// is first and is deliberately a real 0x20 -- unlike the passphrase plate,
	// free text engraves spaces as spaces, so this is also the only place the
	// plate shows one.
	ftProofSweep = " !\"#$%&'()*+,-./0123456789:;<=>?@ABCDEFGHIJKLMNOPQRSTUVWXYZ" +
		"[\\]^_`abcdefghijklmnopqrstuvwxyz{|}~"

	// The confusable groups, ADJACENT. Codepoint order separates exactly the
	// characters that get mistaken for each other -- 'r' and 'm' are two cells
	// apart in the sweep above -- and a confusable pair can only be judged side
	// by side. "rn" against "m" is the classic single-stroke hazard: at plate
	// scale they differ mainly by the r's short diagonal.
	ftProofConfusables = "rn m rnm | 0O oO | 1lI| | 5S sS | 2Z zZ | 8B | 9g6 | " +
		"adoq | ceo | uvw | ,;.: | '`\" | {[(<>)]}"

	// Prose, to exercise WORD WRAP -- ragged line ends, varying word lengths,
	// and the reading experience the sweep and the confusable table cannot
	// show. Pangrams first so every letter appears in running text too.
	ftProofPangrams = "The quick brown fox jumps over the lazy dog. " +
		"Pack my box with five dozen liquor jugs. " +
		"How vexingly quick daft zebras jump."
	ftProofLorem1 = "Lorem ipsum dolor sit amet, consectetur adipiscing elit, " +
		"sed do eiusmod tempor incididunt ut labore et dolore magna aliqua."
	ftProofLorem2 = "Ut enim ad minim veniam, quis nostrud exercitation ullamco " +
		"laboris nisi ut aliquip ex ea commodo consequat."
	ftProofLorem3 = "Duis aute irure dolor in reprehenderit in voluptate velit " +
		"esse cillum dolore eu fugiat nulla pariatur."
	ftProofLorem4 = "Excepteur sint occaecat cupidatat non proident, sunt in " +
		"culpa qui officia deserunt mollit anim id est laborum."
)

// ftProofFull is the no-QR pattern: 758 characters, measured to land at 3.0mm
// in 20 lines against an 816-character limit.
const ftProofFull = ftProofSweep + "\n" + ftProofConfusables + "\n" +
	ftProofPangrams + " " + ftProofLorem1 + " " + ftProofLorem2 + " " +
	ftProofLorem3 + " " + ftProofLorem4

// ftProofWithQR is the pattern used when a QR was chosen: 436 characters,
// measured to land at 3.0mm in 21 lines with a 73-module QR beside it, against
// a 516-character limit. It keeps the sweep and the confusable table whole --
// only the prose is shorter, because that is the part a smaller sample still
// represents.
const ftProofWithQR = ftProofSweep + "\n" + ftProofConfusables + "\n" +
	ftProofPangrams + " " + ftProofLorem1

// Title and footer are both exactly 18 characters -- the cap -- so the plate
// also proves the cap's geometry. Both land on screw-hole rows, so this is the
// only place the inset centring is exercised at its limit. The footer carries
// descenders and confusables deliberately: it is the row where a glyph is most
// likely to collide with a screw hole.
const (
	ftProofTitle  = "TEXTPROOF 3.0mm 44" // 18
	ftProofFooter = "gjpqy 0O 1lI| rn m" // 18
)

// ftProofFor returns the pattern for the QR choice already made. Splitting on
// the QR is not a nicety: with a QR the plate holds 516 characters at 3.0mm
// rather than 816, so loading ftProofFull would be refused outright and the
// operator would get no proof at all.
func ftProofFor(qr bool) string {
	if qr {
		return ftProofWithQR
	}
	return ftProofFull
}

// Prompt copy. Titled "Test Pattern" and NOT "Text Proof": uiContains
// lowercases and strips spaces from its needle, so a screen titled "Text Proof"
// is matched by a readout containing the literal TEXTPROOF!, which would make
// every "the prompt did not appear" assertion vacuously green. That trap has
// already produced false passes three times on this project.
const (
	ftProofPromptTitle = "Test Pattern"

	ftProofAsk = "Load the text-proof test pattern?"

	ftProofReplaces = "This REPLACES ALL THREE fields, discarding whatever is in " +
		"them now: Text becomes the proof pattern, Title becomes " +
		ftProofTitle + ", Footer becomes " + ftProofFooter + "."

	ftProofKeep = "Back = no: continue with " + ftProofTrigger + " exactly as typed. " +
		"Any text can be a real plate, including this one."
)

// ftProofOffer is the trigger check, and the only place it lives. Call it from
// the text field's OK handler, BEFORE that field's own validation.
//
// typed is the field's ENTIRE contents: an equality test, not a substring
// search, so "see TEXTPROOF! for details" is just text.
//
// Scoped to the free-text program's TEXT field alone. Deliberately NOT offered
// from Title or Footer: those are 18 characters and the trigger is 10, so it
// would fit -- but firing there would clobber a body the operator had already
// typed, and the free-text body is the expensive field to lose.
func ftProofOffer(ctx *Context, th *Colors, typed string, load func()) bool {
	if typed != ftProofTrigger || load == nil {
		return false
	}
	if !ftProofPrompt(ctx, th) {
		return false
	}
	load()
	return true
}

// ftProofBody lays out the prompt, returned with its measured size so a test
// can assert it fits the real panel by MEASURING RECTANGLES. Asserting fit from
// ExtractText is impossible: it collects the runes of every drawn text op
// regardless of occlusion, so a label drawn off the panel reads as present.
func ftProofBody(ctx *Context, th *Colors, width int) (op.Op, image.Point) {
	var rt richText
	rt.Add(&ctx.B, ctx.Styles.body, width, th.Text, ftProofAsk)
	rt.Y += 4
	rt.Add(&ctx.B, ctx.Styles.body, width, th.Text, ftProofReplaces)
	rt.Y += 4
	rt.Add(&ctx.B, ctx.Styles.body, width, th.Text, ftProofKeep)
	return rt.Content, image.Pt(width, rt.Y)
}

// ftProofPrompt asks. It returns true only if the operator explicitly accepted
// with the checkmark; Back, and a ctx that shuts down mid-prompt, both mean NO
// -- the answer that changes nothing.
func ftProofPrompt(ctx *Context, th *Colors) bool {
	noBtn := &Clickable{Button: Button1}
	yesBtn := &Clickable{Button: Button3}
	hookPPWidget("proofNo", noBtn)
	hookPPWidget("proofYes", yesBtn)
	for !ctx.Done {
		if noBtn.Clicked(ctx) {
			return false
		}
		if yesBtn.Clicked(ctx) {
			return true
		}
		dims := ctx.Platform.DisplaySize()
		area := ppConfirmArea(dims)
		body, _ := ftProofBody(ctx, th, area.Dx())
		body = body.Offset(image.Point(area.Min))
		nav, _ := layoutNavigation(&ctx.B, th, dims, ftProofNav(noBtn, yesBtn)...)
		title, _ := layoutTitle(ctx, dims.X, th.Text, ftProofPromptTitle)
		ctx.Frame(op.Layer(body, nav, title, op.Color(&ctx.B, th.Background)))
	}
	return false
}

// ftProofNav is the prompt's two answers. Named so a test can assert WHICH ICON
// sits on WHICH answer: layoutNavigation positions a button by its
// Clickable.Button and draws whatever Icon it was handed, so the two are
// independent, and every test taps by registered tag rather than by glyph. The
// operator has only the glyph.
func ftProofNav(noBtn, yesBtn *Clickable) []NavButton {
	return []NavButton{
		{Clickable: noBtn, Style: StyleSecondary, Icon: assets.IconBack},
		{Clickable: yesBtn, Style: StylePrimary, Icon: assets.IconCheckmark},
	}
}

// ftProofLoader returns the load function for a flow whose three fields live at
// the given pointers. It is a named function rather than a closure literal at
// the call site so the three writes stay in one place: a loader that populated
// two fields would defeat the prompt's "all three" promise.
//
// The text is chosen for the CURRENT QR choice (ftProofFor). The QR choice
// itself is never modified -- the operator made it deliberately one step
// earlier, and silently flipping it would change what a scanner returns from
// the plate.
func ftProofLoader(text, title, footer *string, useQR *bool) func() {
	return func() {
		*text = ftProofFor(*useQR)
		*title = ftProofTitle
		*footer = ftProofFooter
	}
}
