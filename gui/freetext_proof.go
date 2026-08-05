package gui

import (
	"image"

	"seedhammer.com/gui/assets"
	"seedhammer.com/gui/op"
)

// The legibility test patterns for the free-text plate (followup
// seedhammer-textproof; user design 2026-08-04, revised 2026-08-04 after the
// execution review).
//
// O1 asks a question a render cannot answer: are these glyphs legible cut into
// steel? The free-text plate makes that harder than the passphrase plate did,
// because the size is chosen by auto-fit -- so the LENGTH of the pattern
// selects which rung is being tested. Every pattern here is tuned to land at
// **3.0mm**, the smallest rung and the hardest legibility case. If a glyph
// reads at 3.0mm it reads at every rung.
//
// There are FOUR patterns, because two independent things vary:
//
//   - The FACE. The machine ships two engraving faces and both are used on real
//     plates: font/sh cuts the free-text plate, font/constant cuts the seed,
//     descriptor and passphrase plates. A proof cut in one says nothing about
//     the other -- they are different outlines -- so each gets its own trigger.
//     The faces do not even share a grid: at 3.0mm font/sh is 44 columns and
//     font/constant is 39, so each face also needs its own tuned length.
//   - The QR, which competes with the text for the same plate. With a QR the
//     lines beside it are roughly a third as wide, so the same pattern would be
//     refused outright and the operator would get no proof at all.
//
// Each pattern fills its plate to 23 of the 24 available body rows -- as much
// as fits, with one spare row as the margin. See TestProofPatternsFillThePlate.
const (
	// ftProofTriggerSH loads the font/sh proof. Kept from the original feature
	// (the followup, the review and the operator's notes all call it
	// TEXTPROOF!), and it needs no face in its name: font/sh is the face this
	// program engraves in unless told otherwise, so TEXTPROOF! proves the TEXT
	// plate as it normally cuts.
	ftProofTriggerSH = "TEXTPROOF!"

	// ftProofTriggerConst loads the font/constant proof. Named for the face and
	// not for the plate, because that is the whole point of it: it borrows the
	// free-text plate as a rig to qualify the face every OTHER plate is cut in.
	// The two triggers are the same length and differ from the first character,
	// so neither is a prefix of the other and a mistyped one matches nothing.
	ftProofTriggerConst = "CONSTPROOF!"
)

// The building blocks. Shared verbatim by all four patterns so a change to a
// glyph group changes every plate at once.
const (
	// Every printable ASCII character in codepoint order, one each. The space
	// is first and is deliberately a real 0x20: free text engraves spaces as
	// spaces, so it is present as a character rather than only as a word gap.
	ftProofSweep = " !\"#$%&'()*+,-./0123456789:;<=>?@ABCDEFGHIJKLMNOPQRSTUVWXYZ" +
		"[\\]^_`abcdefghijklmnopqrstuvwxyz{|}~"

	// The confusable groups, ADJACENT. Codepoint order separates exactly the
	// characters that get mistaken for each other -- 'r' and 'm' are two cells
	// apart in the sweep above -- and a confusable pair can only be judged side
	// by side.
	//
	// Two construction rules, both load-bearing:
	//
	//  1. No group contains a space. WrapText breaks at spaces, so a group with
	//     one inside CAN be split across two engraved lines, which destroys the
	//     comparison exactly where the plate is tightest. Written as single
	//     tokens, the wrap can only fall on a separator.
	//  2. The separator is " + ". It was " | " until the review, and '|' is
	//     itself a member of two groups here, so the plate read "1lI| | 5S" --
	//     ambiguous at the size under test. '+' belongs to no group.
	//
	// "rnmrn" is the classic single-stroke hazard, with the m flanked by "rn"
	// on both sides: at plate scale they differ mainly by the r's short
	// diagonal.
	ftProofConfusables = "rnmrn + 0OoO + 1lI| + !i| + 5SsS + 2ZzZ + 8B + 9g6 + " +
		"adoq + ceo + uvw + \\/ + ^~ + _- + ,;.: + '`\" + {[(<>)]}"

	// Real BIP-39 words, UPPER CASE, because that is the reading task the
	// machine actually sets: backup.EngraveSeed upper-cases the seed and every
	// mnemonic word before engraving it. Reading ABANDON in steel is a
	// different job from reading it inside the sweep, where every neighbour is
	// a capital and there is no word shape to help.
	ftProofSeedWords = "ABANDON ABILITY ZOO ZONE QUIZ JUNGLE VOYAGE WITNESS ISOLATE MIRROR"

	// One pangram per case. Uppercase running text is what a seed plate is;
	// lowercase running text is what a free-text note is. Both appear in every
	// pattern.
	ftProofUpperPangram = "THE QUICK BROWN FOX JUMPS OVER THE LAZY DOG."
	ftProofLowerPangram = "pack my box with five dozen liquor jugs."
)

// Prose, to exercise WORD WRAP -- ragged line ends, varying word lengths, and
// the reading experience the sweep and the confusable table cannot show. Each
// pattern takes as many of these as its plate holds; the long paragraphs are
// the coarse fill and the short pangrams are the fine adjustment.
const (
	ftProofLorem1 = "Lorem ipsum dolor sit amet, consectetur adipiscing elit, " +
		"sed do eiusmod tempor incididunt ut labore et dolore magna aliqua."
	ftProofLorem2 = "Ut enim ad minim veniam, quis nostrud exercitation ullamco " +
		"laboris nisi ut aliquip ex ea commodo consequat."
	ftProofLorem3 = "Duis aute irure dolor in reprehenderit in voluptate velit " +
		"esse cillum dolore eu fugiat nulla pariatur."
	ftProofLorem4 = "Excepteur sint occaecat cupidatat non proident, sunt in " +
		"culpa qui officia deserunt mollit anim id est laborum."
	ftProofPangram1 = "How vexingly quick daft zebras jump."
	ftProofPangram2 = "Sphinx of black quartz, judge my vow."
)

// ftProofHead is what every pattern opens with, in every face and with or
// without a QR: the sweep, the confusable table, the upper-case seed words and
// the two pangrams. The '\n's are hard breaks, so the sweep and the table each
// start a line of their own rather than running into the prose.
const ftProofHead = ftProofSweep + "\n" + ftProofConfusables + "\n" +
	ftProofSeedWords + "\n" + ftProofUpperPangram + " " + ftProofLowerPangram

// The four patterns. Head + as much prose as ABOUT NINE TENTHS of the plate
// holds. The tails differ because the plates differ -- font/sh is 44 columns
// against font/constant's 39, and a QR takes roughly two thirds of the width
// off every line beside it.
//
// Deliberately NOT tuned to the last character (user directive 2026-08-04).
// Auto-fit is all-or-nothing: a pattern sitting at 99% of the 3.0mm capacity is
// pushed over by any later edit that lengthens a glyph path, and Fit then
// REFUSES it outright -- the operator gets no proof at all rather than a
// slightly smaller one. The last few percent of characters buy nothing. What
// makes the plate informative is that every glyph is present, the confusables
// are adjacent, there is enough prose to judge wrap and word shapes, and it
// lands at 3.0mm. Measured, each of these uses 21 or 22 of the 24 body rows and
// still fits with 5% more text; see TestProofPatternsFillThePlate.
const (
	ftProofTextSH = ftProofHead + " " + ftProofLorem1 + " " + ftProofLorem2 + " " +
		ftProofLorem3 + " " + ftProofLorem4 + " " + ftProofPangram1
	ftProofTextSHQR  = ftProofHead + " " + ftProofPangram1 + " " + ftProofPangram2
	ftProofTextConst = ftProofHead + " " + ftProofLorem1 + " " + ftProofLorem2 + " " +
		ftProofLorem3 + " " + ftProofPangram1 + " " + ftProofPangram2
	ftProofTextConstQR = ftProofHead + " " + ftProofPangram1
)

// The plate's own titles. On permanent steel the title is the only record of
// WHAT was tested, so it carries the three things that would otherwise be
// guesswork a year later: the face, the rung, and the character grid at that
// rung (columns x rows). Every number in them is asserted against the live
// measurement by TestProofTitlesStateTheMeasuredGrid -- a title claiming a size
// or a grid the plate does not have is worse than no title.
//
// Both sit under backup.MaxTitleLen; the FOOTER is the row pinned at exactly
// the cap.
const (
	ftProofTitleSH    = "SH 3.0mm 44x26"
	ftProofTitleConst = "CONST 3.0mm 39x26"
)

// ftProofFooter is exactly backup.MaxTitleLen characters, so the plate also
// exercises the cap. It carries descenders and confusables deliberately: the
// footer sits on the last plate row, which is a screw-hole row, and that is
// where a glyph is most likely to collide with a hole.
const ftProofFooter = "gjpqy 0O 1lI| rn m" // 18

// ftProof is one trigger's worth of proof: which face, what the plate says it
// is, and the two patterns tuned for that face.
type ftProof struct {
	Trigger string
	Face    ftFace
	Title   string
	// Text is the pattern with no QR; TextQR the one with a QR beside it.
	Text   string
	TextQR string
}

// For returns the pattern for the QR choice the operator has ALREADY made.
// Splitting on the QR is not a nicety: with a QR the plate holds roughly half
// as much at 3.0mm, so loading the other pattern would be refused outright.
func (p *ftProof) For(qr bool) string {
	if qr {
		return p.TextQR
	}
	return p.Text
}

// ftProofs is every proof the free-text program offers, one per engraving face.
var ftProofs = []ftProof{
	{
		Trigger: ftProofTriggerSH,
		Face:    ftFaceSH,
		Title:   ftProofTitleSH,
		Text:    ftProofTextSH,
		TextQR:  ftProofTextSHQR,
	},
	{
		Trigger: ftProofTriggerConst,
		Face:    ftFaceConst,
		Title:   ftProofTitleConst,
		Text:    ftProofTextConst,
		TextQR:  ftProofTextConstQR,
	},
}

// ftProofForTrigger is the trigger lookup, and the only place it lives.
//
// typed is the field's ENTIRE contents: an equality test, not a substring
// search, so "see TEXTPROOF! for details" is just text.
func ftProofForTrigger(typed string) (*ftProof, bool) {
	for i := range ftProofs {
		if ftProofs[i].Trigger == typed {
			return &ftProofs[i], true
		}
	}
	return nil, false
}

// Prompt copy. Titled "Test Pattern" and NOT "Text Proof": uiContains
// lowercases and strips spaces from its needle, so a screen titled "Text Proof"
// would be matched by a readout containing the literal TEXTPROOF!.
//
// That reasoning is why the title reads as it does, but it does NOT make a
// "the prompt did not appear" assertion safe on its own, and the original
// comment here claimed it did. Measured, both halves of that claim were wrong:
// uiContains(content, "Test Pattern") is true even with the title not drawn at
// all, because the body's own "test pattern?" survives the space-stripping; and
// the trap it named is unreachable anyway, because NewTextKeyboard starts
// unrevealed and the readout extracts as asterisks, never as TEXTPROOF!. Tests
// that need to know whether the prompt went up watch for a FRAME, or for copy
// only this screen carries -- see TestProofNeedsTheWholeField.
const ftProofPromptTitle = "Test Pattern"

// ftProofAsk names the face in the question itself, because the two triggers
// differ ONLY in which face they prove and the operator has to be able to tell,
// from the screen, which of the two they typed.
func ftProofAsk(p *ftProof) string {
	return "Load the " + p.Face.Name + " test pattern?"
}

// ftProofReplaces is the prompt's promise, and ftProofLoader is what keeps it.
// "REPLACES ALL THREE" is the load-bearing phrase: the operator is one tap from
// losing a body they may have spent minutes typing.
func ftProofReplaces(p *ftProof) string {
	return "This REPLACES ALL THREE fields, discarding whatever is in them now: " +
		"Text becomes the proof pattern, Title becomes " + p.Title +
		", Footer becomes " + ftProofFooter + ". The plate is cut in " +
		p.Face.Name + " at 3.0mm."
}

// ftProofKeep is the honest description of the OTHER answer. Declining is not a
// cancel: the typed trigger is a perfectly good free text and stays exactly as
// entered.
func ftProofKeep(p *ftProof) string {
	return "Back = no: continue with " + p.Trigger + " exactly as typed. " +
		"Any text can be a real plate, including this one."
}

// ftProofOffer is the trigger check and the prompt, and the only place either
// lives. Call it from the text field's OK handler, BEFORE that field's own
// validation.
//
// Scoped to the free-text program's TEXT field alone. Deliberately NOT offered
// from Title or Footer: those are 18 characters and the triggers are 10 and 11,
// so they would fit -- but firing there would clobber a body the operator had
// already typed, and the free-text body is the expensive field to lose.
//
// load writes the fields and RETURNS the text it wrote, so the caller re-seeds
// the keyboard from the one value that was actually stored. A caller that
// recomputed the pattern itself would be a second source of truth for which
// pattern is loaded, and the two could disagree about the face or the QR.
func ftProofOffer(ctx *Context, th *Colors, typed string, load func(*ftProof) string) (string, bool) {
	p, ok := ftProofForTrigger(typed)
	if !ok || load == nil {
		return "", false
	}
	if !ftProofPrompt(ctx, th, p) {
		return "", false
	}
	return load(p), true
}

// ftProofBody lays out the prompt, returned with its measured size so a test
// can assert it fits the real panel by MEASURING RECTANGLES. Asserting fit from
// ExtractText is impossible: it collects the runes of every drawn text op
// regardless of occlusion, so a label drawn off the panel reads as present.
func ftProofBody(ctx *Context, th *Colors, width int, p *ftProof) (op.Op, image.Point) {
	var rt richText
	rt.Add(&ctx.B, ctx.Styles.body, width, th.Text, ftProofAsk(p))
	rt.Y += 4
	rt.Add(&ctx.B, ctx.Styles.body, width, th.Text, ftProofReplaces(p))
	rt.Y += 4
	rt.Add(&ctx.B, ctx.Styles.body, width, th.Text, ftProofKeep(p))
	return rt.Content, image.Pt(width, rt.Y)
}

// ftProofPrompt asks. It returns true only if the operator explicitly accepted
// with the checkmark; Back, and a ctx that shuts down mid-prompt, both mean NO
// -- the answer that changes nothing.
func ftProofPrompt(ctx *Context, th *Colors, p *ftProof) bool {
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
		body, _ := ftProofBody(ctx, th, area.Dx(), p)
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
// operator has only the glyph -- see TestProofNavIconsMeanWhatTheyShow, which
// exists because swapping these two lines left the whole suite green.
func ftProofNav(noBtn, yesBtn *Clickable) []NavButton {
	return []NavButton{
		{Clickable: noBtn, Style: StyleSecondary, Icon: assets.IconBack},
		{Clickable: yesBtn, Style: StylePrimary, Icon: assets.IconCheckmark},
	}
}

// ftProofLoader returns the load function for a flow whose fields live at the
// given pointers. It is a named function rather than a closure literal at the
// call site so the four writes stay in one place: a loader that populated three
// of them would defeat the prompt's "all three fields, in this face" promise.
//
// The text is chosen for the CURRENT QR choice. The QR choice itself is never
// modified -- the operator made it deliberately one step earlier, and silently
// flipping it would change what a scanner returns from the plate.
//
// The FACE is written, because it is what the trigger selects. It is returned
// to the flow by writing through the pointer rather than by the return value:
// the return value is the text, and the caller uses it to re-seed the keyboard.
func ftProofLoader(text, title, footer *string, face *ftFace, useQR *bool) func(*ftProof) string {
	return func(p *ftProof) string {
		*text = p.For(*useQR)
		*title = p.Title
		*footer = ftProofFooter
		*face = p.Face
		return *text
	}
}
