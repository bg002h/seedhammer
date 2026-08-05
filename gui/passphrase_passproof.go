package gui

import (
	"image"

	"seedhammer.com/gui/assets"
	"seedhammer.com/gui/op"
	"seedhammer.com/passphrase"
)

// The PASSPROOF! test pattern (spec 9.1 -- the O1 hardware-legibility
// procedure; followup seedhammer-passproof-test-pattern).
//
// O1 needs one plate carrying every glyph the engraving face can cut. Typing
// all 95 printable ASCII characters in codepoint order costs several hundred
// taps across four keyboard pages, and a single mistype yields both a wrong
// plate and a false legibility reading -- the plate would be evidence about a
// character set nobody chose.
//
// So: typing the literal ppPassProofTrigger into ANY of the passphrase
// program's three fields OFFERS to populate all three at once.
//
// Five properties are load-bearing, each for a stated reason:
//
//  1. It ASKS. It never populates silently, and the prompt says it replaces
//     ALL THREE fields -- triggering from a fingerprint field clobbers a
//     passphrase already typed, so "load the test pattern?" alone would not be
//     honest wording.
//  2. The prompt's NO branch continues with PASSPROOF! exactly as typed. Any
//     string can be somebody's real passphrase, including this one. Silent
//     replacement would make it untypeable with no explanation. (The collision
//     is real only in the passphrase field: PASSPROOF! is not hex, so
//     passphrase.ValidateFingerprint already refuses it in the other two. The
//     prompt is REQUIRED in the passphrase field and kept in the other two for
//     consistency.)
//  3. It is checked on OK/advance, never per keystroke, and only when the
//     constant is the ENTIRE field -- so a passphrase that merely contains the
//     word is untouched.
//  4. Accepting stays on the current screen, so the operator sees what was
//     loaded before advancing.
//  5. It is scoped to these three fields in THIS program. The fingerprint
//     fields use NewAddressKeyboard, which is shared with BIP-85 child-index
//     entry (gui/bip85.go) and typed-address verification
//     (gui/verify_address.go); the check therefore lives in the passphrase
//     flow's own field handlers and NEVER in PassphraseKeyboard, which would
//     leak it into both.
//
// All three values below are FIXED FIRMWARE CONSTANTS. Nothing here is read
// from NFC, from storage, or from any other external source.

// ppPassProofTrigger is the literal that offers the pattern. The trailing '!'
// mirrors the NFC debugCommand precedent (FOREVERLAURA!, gui/scan.go).
//
// Renamed from FONTPROOF! (2026-08-05). The old root named no axis and was
// distinguished from the free-text program's TEXTPROOF! only by living in
// another program -- "font" and "text" being near-synonyms, while FONTPROOF!
// cut in font/constant, the very face CONSTPROOF! proves. That is not
// theoretical: the operator called the free-text proof "FONTPROOF!" repeatedly,
// and typing it opened the passphrase program instead. PASSPROOF! names its
// program and differs from every other root at the first character. See
// mnemonic-engrave design/LEXICON_proof_triggers.md.
const ppPassProofTrigger = "PASSPROOF!"

// ppPassProofPassphrase is all 95 printable ASCII, 0x20-0x7E, in codepoint
// order, one each -- followed by ppPassProofConfusables. It begins with a
// literal space, which is the point: the plate must show the substituted space
// MARK beside a real '_' and beside the genuine 0x20 the metadata bands
// engrave.
//
// TestPassProofPatternIsEvery95PrintableASCII derives the first 95 runes from
// the codepoint range and compares, so a typo in the literal cannot survive.
const ppPassProofPassphrase = " !\"#$%&'()*+,-./0123456789:;<=>?@ABCDEFGHIJKLMNOPQRSTUVWXYZ[\\]^_`abcdefghijklmnopqrstuvwxyz{|}~" +
	ppPassProofConfusables

// ppPassProofConfusables is appended to the codepoint sweep so the letters sit
// ADJACENT on the plate. Codepoint order separates them -- 'm' and 'n' are
// neighbours but 'r' is two cells away -- and a confusable pair can only be
// judged side by side. "rn" against "m" is the classic single-stroke hazard:
// at plate scale they differ mainly by the r's short diagonal.
//
// Anything added here must keep the pattern within passphrase.MaxLen and must
// not change what the sweep itself covers.
const ppPassProofConfusables = "rnm"

// The two fingerprints. Both are valid hex, canonicalise unchanged, and render
// grouped as "DEAD BEEF" / "CAFE BABE" -- which is how they supply the last
// alphabet rune the text block cannot: a REAL 0x20. They are also obviously
// synthetic, so the plate labels itself a proof rather than resembling a backup.
const (
	ppPassProofSeedFP = "DEADBEEF"
	ppPassProofCombFP = "CAFEBABE"
)

// ppPassProofFingerprint is the fixed value the pattern puts in field which.
func ppPassProofFingerprint(which ppFingerprintStep) string {
	if which == ppCombinedFP {
		return ppPassProofCombFP
	}
	return ppPassProofSeedFP
}

// The prompt's wording. The three lines are separate constants so a test can
// assert what each one has to say without depending on how they are joined.
//
// ppPassProofReplaces is the honest part and carries the whole warning: it
// names all three fields, says the values replace whatever is in them, and is
// the phrase tests match on to tell the PROMPT from the field behind it (the
// title alone would not do -- the keyboard readout literally reads PASSPROOF!,
// and uiContains strips spaces from its needle, so "Pass Proof" matches it).
const (
	ppPassProofTitle = "Test Pattern"

	ppPassProofAsk = "Load the passphrase test pattern?"

	ppPassProofReplaces = "This REPLACES ALL THREE fields, discarding whatever is in them now: " +
		"Passphrase becomes all 95 printable ASCII plus rnm, " +
		"Seed FP becomes DEAD BEEF, " +
		"Expected Comb FP becomes CAFE BABE."

	// The declining branch differs by FIELD, and saying otherwise makes the one
	// prompt in this feature whose entire purpose is honesty tell a small lie.
	// In the passphrase field, "no" really does continue with PASSPROOF! as the
	// passphrase. In either fingerprint field it CANNOT: ValidateFingerprint
	// refuses a non-hex value, so "no" returns to the field with the text still
	// there. Pinned by TestPassProofNoBranchInFingerprintFieldRefuses, which
	// asserts the refusal, and by TestPassProofKeepLineMatchesTheField.
	ppPassProofKeepPassphrase = "Back = no: continue with PASSPROOF! exactly as typed. " +
		"Any text can be a real passphrase, including this one."

	ppPassProofKeepFingerprint = "Back = no: keep PASSPROOF! in this field. " +
		"It is not hex, so this field will ask again for 8 hex digits."
)

// ppPassProofKeep is the declining-branch sentence for a field.
func ppPassProofKeep(isPassphrase bool) string {
	if isPassphrase {
		return ppPassProofKeepPassphrase
	}
	return ppPassProofKeepFingerprint
}

// ppPassProofBody lays out the prompt's text and returns it with its measured
// size, so TestPassProofPromptFitsPanel can assert it fits the real 480x320
// panel by MEASURING RECTANGLES. Asserting fit from ExtractText is impossible:
// it collects the runes of every drawn text op regardless of occlusion, so a
// label drawn under the keyboard, off the panel, or past the bottom edge reads
// as present.
func ppPassProofBody(ctx *Context, th *Colors, width int, isPassphrase bool) (op.Op, image.Point) {
	var rt richText
	rt.Add(&ctx.B, ctx.Styles.body, width, th.Text, ppPassProofAsk)
	rt.Y += 4
	rt.Add(&ctx.B, ctx.Styles.body, width, th.Text, ppPassProofReplaces)
	rt.Y += 4
	rt.Add(&ctx.B, ctx.Styles.body, width, th.Text, ppPassProofKeep(isPassphrase))
	return rt.Content, image.Pt(width, rt.Y)
}

// ppPassProofPrompt asks. It returns true only if the operator explicitly
// accepted with the checkmark; Back, and a ctx that shuts down mid-prompt, both
// mean NO, which is the answer that changes nothing.
//
// It borrows ppConfirmArea -- below the title, inboard of the nav column -- so
// the prompt and the confirm screen spend the same measured budget.
func ppPassProofPrompt(ctx *Context, th *Colors, isPassphrase bool) bool {
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
		body, _ := ppPassProofBody(ctx, th, area.Dx(), isPassphrase)
		body = body.Offset(image.Point(area.Min))
		nav, _ := layoutNavigation(&ctx.B, th, dims, ppPassProofNav(noBtn, yesBtn)...)
		title, _ := layoutTitle(ctx, dims.X, th.Text, ppPassProofTitle)
		ctx.Frame(op.Layer(body, nav, title, op.Color(&ctx.B, th.Background)))
	}
	return false
}

// ppPassProofNav is the prompt's two answers. It is a named function purely so
// TestPassProofPromptIconsNotSwapped can assert WHICH ICON sits on WHICH answer:
// layoutNavigation positions a button by its Clickable.Button and draws whatever
// Icon it was handed, so the two are independent, and every test taps by
// registered tag rather than by glyph. The operator has only the glyph.
func ppPassProofNav(noBtn, yesBtn *Clickable) []NavButton {
	return []NavButton{
		{Clickable: noBtn, Style: StyleSecondary, Icon: assets.IconBack},
		{Clickable: yesBtn, Style: StylePrimary, Icon: assets.IconCheckmark},
	}
}

// ppPassProofOffer is the trigger check for ONE field, and the only place it
// lives. Call it from a field step's OK handler, BEFORE that step's own
// validation.
//
// typed is the field's ENTIRE contents: an equality test, not a substring
// search, so "myPASSPROOF!pass" is just a passphrase.
//
// load writes the three fixed values into the flow's state; it is supplied by
// engravePassphraseFlow, because the trigger fires in one field but must
// populate all three, and no single step owns the other two. A nil load
// disables the trigger rather than crashing the device --
// TestPassProofTriggersFromEveryField drives the PRODUCTION flow, so a step
// wired with nil fails there.
//
// It returns true when the pattern was loaded, which the caller must treat as
// "stay on this screen": the operator has to see what landed before advancing.
// False means proceed exactly as if the constant were any other string.
func ppPassProofOffer(ctx *Context, th *Colors, typed string, isPassphrase bool, load func()) bool {
	if typed != ppPassProofTrigger || load == nil {
		return false
	}
	if !ppPassProofPrompt(ctx, th, isPassphrase) {
		return false
	}
	load()
	return true
}

// ppPassProofLoader returns the load function for a flow whose passphrase lives
// in dst/n and whose fingerprints live in seedFP/combinedFP. Every value it
// writes is a constant of this package.
//
// It is a named function rather than a closure literal at the call site so the
// three writes are in one place: a loader that populated two fields would defeat
// the whole point of the prompt's "all three" promise.
func ppPassProofLoader(dst []byte, n *int, seedFP, combinedFP *string) func() {
	return func() {
		*n = copy(dst, ppPassProofPassphrase)
		*seedFP = ppPassProofSeedFP
		*combinedFP = ppPassProofCombFP
	}
}

// ppPassProofFragment is what a fingerprint field shows after the pattern
// loads: the value grouped exactly as the field renders it while being typed,
// and as the plate carries it.
func ppPassProofFragment(which ppFingerprintStep) string {
	return passphrase.GroupFingerprint(ppPassProofFingerprint(which))
}
