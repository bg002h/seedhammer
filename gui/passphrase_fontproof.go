package gui

import (
	"image"

	"seedhammer.com/gui/assets"
	"seedhammer.com/gui/op"
	"seedhammer.com/passphrase"
)

// The FONTPROOF! test pattern (spec 9.1 -- the O1 hardware-legibility
// procedure; followup seedhammer-fontproof-test-pattern).
//
// O1 needs one plate carrying every glyph the engraving face can cut. Typing
// all 95 printable ASCII characters in codepoint order costs several hundred
// taps across four keyboard pages, and a single mistype yields both a wrong
// plate and a false legibility reading -- the plate would be evidence about a
// character set nobody chose.
//
// So: typing the literal ppFontProofTrigger into ANY of the passphrase
// program's three fields OFFERS to populate all three at once.
//
// Five properties are load-bearing, each for a stated reason:
//
//  1. It ASKS. It never populates silently, and the prompt says it replaces
//     ALL THREE fields -- triggering from a fingerprint field clobbers a
//     passphrase already typed, so "load the test pattern?" alone would not be
//     honest wording.
//  2. The prompt's NO branch continues with FONTPROOF! exactly as typed. Any
//     string can be somebody's real passphrase, including this one. Silent
//     replacement would make it untypeable with no explanation. (The collision
//     is real only in the passphrase field: FONTPROOF! is not hex, so
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

// ppFontProofTrigger is the literal that offers the pattern. The trailing '!'
// mirrors the NFC debugCommand precedent (FOREVERLAURA!, gui/scan.go).
const ppFontProofTrigger = "FONTPROOF!"

// ppFontProofPassphrase is all 95 printable ASCII, 0x20-0x7E, in codepoint
// order, one each. It begins with a literal space, which is the point: the
// plate must show the substituted space MARK beside a real '_' and beside the
// genuine 0x20 the metadata bands engrave.
//
// TestFontProofPatternIsEvery95PrintableASCII derives this string from the
// codepoint range and compares, so a typo in the literal cannot survive.
const ppFontProofPassphrase = " !\"#$%&'()*+,-./0123456789:;<=>?@ABCDEFGHIJKLMNOPQRSTUVWXYZ[\\]^_`abcdefghijklmnopqrstuvwxyz{|}~"

// The two fingerprints. Both are valid hex, canonicalise unchanged, and render
// grouped as "DEAD BEEF" / "CAFE BABE" -- which is how they supply the last
// alphabet rune the text block cannot: a REAL 0x20. They are also obviously
// synthetic, so the plate labels itself a proof rather than resembling a backup.
const (
	ppFontProofSeedFP = "DEADBEEF"
	ppFontProofCombFP = "CAFEBABE"
)

// ppFontProofFingerprint is the fixed value the pattern puts in field which.
func ppFontProofFingerprint(which ppFingerprintStep) string {
	if which == ppCombinedFP {
		return ppFontProofCombFP
	}
	return ppFontProofSeedFP
}

// The prompt's wording. The three lines are separate constants so a test can
// assert what each one has to say without depending on how they are joined.
//
// ppFontProofReplaces is the honest part and carries the whole warning: it
// names all three fields, says the values replace whatever is in them, and is
// the phrase tests match on to tell the PROMPT from the field behind it (the
// title alone would not do -- the keyboard readout literally reads FONTPROOF!,
// and uiContains strips spaces from its needle, so "Font Proof" matches it).
const (
	ppFontProofTitle = "Test Pattern"

	ppFontProofAsk = "Load the font-proof test pattern?"

	ppFontProofReplaces = "This REPLACES ALL THREE fields, discarding whatever is in them now: " +
		"Passphrase becomes all 95 printable ASCII, " +
		"Seed FP becomes DEAD BEEF, " +
		"Expected Comb FP becomes CAFE BABE."

	ppFontProofKeep = "Back = no: continue with FONTPROOF! exactly as typed. " +
		"Any text can be a real passphrase, including this one."
)

// ppFontProofBody lays out the prompt's text and returns it with its measured
// size, so TestFontProofPromptFitsPanel can assert it fits the real 480x320
// panel by MEASURING RECTANGLES. Asserting fit from ExtractText is impossible:
// it collects the runes of every drawn text op regardless of occlusion, so a
// label drawn under the keyboard, off the panel, or past the bottom edge reads
// as present.
func ppFontProofBody(ctx *Context, th *Colors, width int) (op.Op, image.Point) {
	var rt richText
	rt.Add(&ctx.B, ctx.Styles.body, width, th.Text, ppFontProofAsk)
	rt.Y += 4
	rt.Add(&ctx.B, ctx.Styles.body, width, th.Text, ppFontProofReplaces)
	rt.Y += 4
	rt.Add(&ctx.B, ctx.Styles.body, width, th.Text, ppFontProofKeep)
	return rt.Content, image.Pt(width, rt.Y)
}

// ppFontProofPrompt asks. It returns true only if the operator explicitly
// accepted with the checkmark; Back, and a ctx that shuts down mid-prompt, both
// mean NO, which is the answer that changes nothing.
//
// It borrows ppConfirmArea -- below the title, inboard of the nav column -- so
// the prompt and the confirm screen spend the same measured budget.
func ppFontProofPrompt(ctx *Context, th *Colors) bool {
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
		body, _ := ppFontProofBody(ctx, th, area.Dx())
		body = body.Offset(image.Point(area.Min))
		nav, _ := layoutNavigation(&ctx.B, th, dims, []NavButton{
			{Clickable: noBtn, Style: StyleSecondary, Icon: assets.IconBack},
			{Clickable: yesBtn, Style: StylePrimary, Icon: assets.IconCheckmark},
		}...)
		title, _ := layoutTitle(ctx, dims.X, th.Text, ppFontProofTitle)
		ctx.Frame(op.Layer(body, nav, title, op.Color(&ctx.B, th.Background)))
	}
	return false
}

// ppFontProofOffer is the trigger check for ONE field, and the only place it
// lives. Call it from a field step's OK handler, BEFORE that step's own
// validation.
//
// typed is the field's ENTIRE contents: an equality test, not a substring
// search, so "myFONTPROOF!pass" is just a passphrase.
//
// load writes the three fixed values into the flow's state; it is supplied by
// engravePassphraseFlow, because the trigger fires in one field but must
// populate all three, and no single step owns the other two. A nil load
// disables the trigger rather than crashing the device --
// TestFontProofTriggersFromEveryField drives the PRODUCTION flow, so a step
// wired with nil fails there.
//
// It returns true when the pattern was loaded, which the caller must treat as
// "stay on this screen": the operator has to see what landed before advancing.
// False means proceed exactly as if the constant were any other string.
func ppFontProofOffer(ctx *Context, th *Colors, typed string, load func()) bool {
	if typed != ppFontProofTrigger || load == nil {
		return false
	}
	if !ppFontProofPrompt(ctx, th) {
		return false
	}
	load()
	return true
}

// ppFontProofLoader returns the load function for a flow whose passphrase lives
// in dst/n and whose fingerprints live in seedFP/combinedFP. Every value it
// writes is a constant of this package.
//
// It is a named function rather than a closure literal at the call site so the
// three writes are in one place: a loader that populated two fields would defeat
// the whole point of the prompt's "all three" promise.
func ppFontProofLoader(dst []byte, n *int, seedFP, combinedFP *string) func() {
	return func() {
		*n = copy(dst, ppFontProofPassphrase)
		*seedFP = ppFontProofSeedFP
		*combinedFP = ppFontProofCombFP
	}
}

// ppFontProofFragment is what a fingerprint field shows after the pattern
// loads: the value grouped exactly as the field renders it while being typed,
// and as the plate carries it.
func ppFontProofFragment(which ppFingerprintStep) string {
	return passphrase.GroupFingerprint(ppFontProofFingerprint(which))
}
