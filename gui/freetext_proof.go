package gui

import (
	"image"
	"strings"

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

	// ftProofTriggerBoth loads the MIXED-FACE proof: one plate whose top half
	// is cut in font/sh and whose bottom half is cut in font/constant, both at
	// 3.0mm. Named for what it proves rather than for a face, because it proves
	// both.
	//
	// It exists because plates are scarce. A legibility round that has to
	// qualify both faces costs two plates -- TEXTPROOF! and CONSTPROOF! -- and
	// this one costs one. It is not a replacement for either: half a plate
	// holds less than a whole one, so the single-face proofs stay for the
	// rounds where a face needs the deeper read.
	//
	// The three triggers differ from their FIRST character, so none is a prefix
	// of another and a mistyped one matches nothing.
	ftProofTriggerBoth = "BOTHPROOF!"
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

// The four single-face patterns. Head + as much prose as ABOUT NINE TENTHS of
// the plate holds. The tails differ because the plates differ -- font/sh is 44
// columns against font/constant's 39, and a QR takes roughly two thirds of the
// width off every line beside it. The fifth pattern, the mixed-face plate, is
// below and is built from halves rather than from the shared head.
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

// The mixed-face plate, as two halves.
//
// Each half is a proof of its own face and carries the whole payload for it:
// the label, the complete 95-character codepoint sweep and the complete
// confusable table. Those three are what makes a half a proof at all, and
// neither the sweep nor the table may be trimmed to make room -- a face is not
// qualified by most of its glyphs.
//
// What each half does NOT carry is a full page of prose. Two halves do not hold
// what two plates hold; the reading material is split between them and the
// running-text pangrams are what survives, because the halves have to fit ONE
// plate at 3.0mm with margin to spare. See TestProofPatternsFillThePlate: this
// pattern is deliberately well short of capacity, since auto-fit is
// all-or-nothing and a pattern balanced on the limit is refused outright by the
// next glyph edit rather than engraved slightly smaller.
//
// The LABEL is the first line of each half and is cut IN THE FACE IT NAMES, so
// it is a specimen of that face as well as a caption for it. Without it the two
// halves of an engraved plate cannot be told apart -- which half is which is
// exactly the question the plate exists to answer. They are the same strings
// the single-face plates carry as their titles, so the grid they state is
// asserted by the same measurement; see TestProofBlockLabelsNameTheirOwnFace.
const (
	ftProofBothSH = ftProofTitleSH + "\n" + ftProofSweep + "\n" + ftProofConfusables + "\n" +
		ftProofUpperPangram + " " + ftProofLowerPangram

	ftProofBothConst = ftProofTitleConst + "\n" + ftProofSweep + "\n" + ftProofConfusables + "\n" +
		ftProofSeedWords + "\n" + ftProofUpperPangram + " " + ftProofLowerPangram

	ftProofTextBoth = ftProofBothSH + "\n" + ftProofBothConst
)

// ftProofBothSplit is how many of ftProofTextBoth's '\n'-blocks belong to the
// font/sh half. DERIVED from the half itself rather than written down: a
// hand-counted split that fell out of step with the text would cut part of one
// face's proof in the other face, and the label would then name the wrong one.
var ftProofBothSplit = strings.Count(ftProofBothSH, "\n") + 1

// ftPlanBoth is the mixed-face plan: the first ftProofBothSplit blocks in
// font/sh, everything after them in font/constant.
var ftPlanBoth = ftPlan{Runs: []ftFaceRun{
	{Face: ftFaceSH, Blocks: ftProofBothSplit},
	{Face: ftFaceConst},
}}

// ftProofTitleBoth is the mixed plate's own title. It cannot state a grid --
// the plate has two, 44 columns in its top half and 39 in its bottom -- so the
// grids live on the block labels, where each one sits in the face it describes,
// and the title carries what is true of the whole plate: which faces, at which
// size.
//
// It is cut in font/sh, the face of the block it borders; the footer is cut in
// font/constant for the same reason. See FitBlocks.
const ftProofTitleBoth = "SH+CONST 3.0mm"

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

// ftProof is one trigger's worth of proof: which face plan, what the plate says
// it is, and the patterns tuned for that plan.
type ftProof struct {
	Trigger string
	Plan    *ftPlan
	Title   string
	// Text is the pattern with no QR; TextQR the one with a QR beside it.
	//
	// An EMPTY TextQR means this proof needs the whole plate and cannot carry a
	// QR at all -- see NeedsWholePlate.
	Text   string
	TextQR string
}

// NeedsWholePlate reports that this proof has no QR variant.
//
// The mixed-face plate is the case: with a QR the lines beside it are roughly a
// third as wide, and the plate then holds about half as much at 3.0mm -- less
// than ONE half's sweep and confusable table, let alone two. There is no
// reduced mixed pattern worth cutting, so the honest answer is that this plate
// carries no QR, said out loud in the prompt before the operator accepts it.
func (p *ftProof) NeedsWholePlate() bool { return p.TextQR == "" }

// For returns the pattern for the QR choice the operator has ALREADY made.
// Splitting on the QR is not a nicety: with a QR the plate holds roughly half
// as much at 3.0mm, so loading the other pattern would be refused outright.
//
// A proof that needs the whole plate has one pattern and the QR is dropped when
// it loads, so there is nothing to choose between.
func (p *ftProof) For(qr bool) string {
	if qr && !p.NeedsWholePlate() {
		return p.TextQR
	}
	return p.Text
}

// ftProofs is every proof the free-text program offers, one per engraving face.
var ftProofs = []ftProof{
	{
		Trigger: ftProofTriggerSH,
		Plan:    &ftPlanSH,
		Title:   ftProofTitleSH,
		Text:    ftProofTextSH,
		TextQR:  ftProofTextSHQR,
	},
	{
		Trigger: ftProofTriggerConst,
		Plan:    &ftPlanConst,
		Title:   ftProofTitleConst,
		Text:    ftProofTextConst,
		TextQR:  ftProofTextConstQR,
	},
	{
		Trigger: ftProofTriggerBoth,
		Plan:    &ftPlanBoth,
		Title:   ftProofTitleBoth,
		Text:    ftProofTextBoth,
		// No QR variant: the pattern needs the whole plate. See
		// NeedsWholePlate.
		TextQR: "",
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

// ftProofAsk names the face plan in the question itself, because the triggers
// differ ONLY in which faces they prove and the operator has to be able to
// tell, from the screen, which of them they typed.
func ftProofAsk(p *ftProof) string {
	return "Load the " + p.Plan.Name() + " test pattern?"
}

// ftProofReplaces is the prompt's promise, and ftProofLoader is what keeps it.
// "REPLACES ALL THREE" is the load-bearing phrase: the operator is one tap from
// losing a body they may have spent minutes typing.
//
// A proof that needs the whole plate also DROPS THE QR, which is a fourth field
// and the only one the loader ever writes without being asked. It is said here,
// in the sentence the operator reads before accepting, because the QR decides
// what a scanner returns from the plate and changing it silently is exactly the
// substitution this program exists to avoid.
func ftProofReplaces(p *ftProof) string {
	s := "This REPLACES ALL THREE fields, discarding whatever is in them now: " +
		"Text becomes the proof pattern, Title becomes " + p.Title +
		", Footer becomes " + ftProofFooter + ". The plate is cut in " +
		p.Plan.Name() + " at 3.0mm."
	if p.NeedsWholePlate() {
		s += " It also REMOVES THE QR: this pattern needs the whole plate, " +
			"so the plate will not be machine-readable."
	}
	return s
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
// flipping it would change what a scanner returns from the plate -- with the
// single exception of a proof that NEEDS THE WHOLE PLATE, which has no QR
// variant to load and whose prompt says in as many words that accepting removes
// the QR. That is not silent, which is the property that mattered.
//
// The FACE PLAN is written, because it is what the trigger selects. It is
// returned to the flow by writing through the pointer rather than by the return
// value: the return value is the text, and the caller uses it to re-seed the
// keyboard.
func ftProofLoader(text, title, footer *string, plan **ftPlan, useQR *bool) func(*ftProof) string {
	return func(p *ftProof) string {
		if p.NeedsWholePlate() {
			// The one exception, and it is prompted: ftProofReplaces says this
			// will happen before the operator can accept it. Written BEFORE the
			// text so For reads the choice that will actually be in force.
			*useQR = false
		}
		*text = p.For(*useQR)
		*title = p.Title
		*footer = ftProofFooter
		*plan = p.Plan
		return *text
	}
}
