package gui

import (
	"bytes"
	"errors"
	"image"
	"strconv"
	"unicode/utf8"

	"seedhammer.com/backup"
	"seedhammer.com/engrave"
	"seedhammer.com/font/constant"
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
//
// loadProof populates all three of the program's fields with the fixed
// pass-proof test pattern; see ppPassProofOffer.
func passphraseEntryFlow(ctx *Context, th *Colors, dst []byte, n int, loadProof func()) (int, bool) {
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
			// The pass-proof trigger, checked HERE -- on OK, on the whole
			// field, in this program's own handler -- and not in
			// PassphraseKeyboard, which BIP-85 index entry and address
			// verification share. Declining loads nothing and falls through to
			// the validation below, so PASSPROOF! remains a typeable
			// passphrase.
			if ppPassProofOffer(ctx, th, kbd.Fragment, true, loadProof) {
				// Stay on this screen: the operator sees the 95 characters
				// that landed before choosing to advance.
				kbd.Fragment = ppPassProofPassphrase
				continue
			}
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
		// Live counter. Over-length is shown rather than clamped: silently
		// dropping keystrokes at 100 would leave the operator believing a
		// longer passphrase had been entered.
		cntOp, cntsz := widget.Labelf(&ctx.B, ctx.Styles.subtitle, th.Text,
			"%d/%d", utf8.RuneCountInString(kbd.Fragment), passphrase.MaxLen)
		// RESERVE the counter's band before laying out the keyboard. The
		// keyboard block is readout+grid, and the readout's height grows with
		// the passphrase, so an unreserved band let the block rise INTO and
		// above the counter -- and op.Layer draws the keyboard on top, so from
		// ~70 characters the counter was hidden in exactly the revealed state a
		// user proof-reads in. Measured at the real 480x320 panel; the
		// over-length "101/100" signal was obscured too. ExtractText collects
		// runes regardless of occlusion, so no text-based test could see it --
		// TestPassphraseEntryFitsPanel measures rectangles instead.
		counterBand, content := content.CutTop(cntsz.Y)
		cntOp = cntOp.Offset(counterBand.N(cntsz))
		// BOUND the block, do not merely reserve around it. Cutting the band
		// off `content` does not move the keyboard at all -- it is bottom-
		// aligned (Max.Y-size.Y) and CutTop leaves Max.Y unchanged -- so the
		// reservation alone left the readout growing up over the counter from
		// ~70 characters and over the title past ~90, exactly as before.
		// TestPassphraseEntryFitsPanel measures the block against this bound.
		kbd.MaxHeight = content.Dy()
		kbdOp, kbdsz := kbd.Layout(ctx, th)
		kbdOp = kbdOp.Offset(content.S(kbdsz))
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
// prior is the value already entered for this field, if any. Stepping Back
// from a later screen and forward again must NOT silently discard it: the flow
// promises Back walks backwards rather than abandoning, and a cleared field
// looks identical to one that was never filled. (Phase D review M2.)
//
// loadProof populates all three of the program's fields with the fixed
// pass-proof test pattern; see ppPassProofOffer.
func fingerprintEntryFlow(ctx *Context, th *Colors, which ppFingerprintStep, prior string, loadProof func()) (string, bool) {
	kbd := NewAddressKeyboard(ctx)
	// Re-seed from the value already entered, so Back-then-forward preserves it.
	// Grouped for display, exactly as the field renders while being typed.
	kbd.Fragment = passphrase.GroupFingerprint(prior)
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
			// Same trigger as the entry step, in this program's own handler.
			// Declining falls through to ValidateFingerprint, which refuses
			// PASSPROOF! -- it is not hex -- so the "no" branch is honest here
			// too: the operator is told what a fingerprint field wants.
			if ppPassProofOffer(ctx, th, kbd.Fragment, false, loadProof) {
				// Stay on this screen, showing the loaded value grouped
				// exactly as the plate will carry it.
				kbd.Fragment = ppPassProofFragment(which)
				continue
			}
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

// ppSpaceMark is the on-screen stand-in for the plate's visible-space mark.
//
// The plate engraves backup.SpaceMark (0x1F), a glyph authored for the
// engraving face. The GUI's bitmap faces index printable ASCII only
// (cmd/bitmapfont's alphabet, plus whitespace runes whose rasters are empty),
// so 0x1F inks nothing on screen -- it would be exactly as invisible as the
// space it exists to expose. '_' is the closest ASCII stand-in the face
// actually draws.
//
// KNOWN LIMIT: a literal '_' in the passphrase renders identically. The plate
// has no such ambiguity, since it engraves '_' and 0x1F as different glyphs.
// On screen it is resolved by the two things drawn beside the readout -- the
// legend naming the mark, and the space COUNT, which pins how many of the
// marks are spaces. This is exactly why spec 5.1 demands counts as well as
// marks.
const ppSpaceMark = '_'

// ppSpaceLegend names the mark. Without it the mark is a private convention:
// a reader has no way to know whether to type a space, an underscore or
// nothing, and each guess opens a different wallet. It mirrors
// backup.passphraseLegend, which does the same job on the steel.
const ppSpaceLegend = string(ppSpaceMark) + " = SPACE"

// ppMarkSpaces copies s into dst with every 0x20 replaced by ppSpaceMark, and
// returns dst[:len(s)]. dst is the CALLER's buffer so the marked copy of the
// secret is a []byte that can be wiped, not a string that cannot (spec 5.3).
func ppMarkSpaces(dst, s []byte) []byte {
	dst = dst[:len(s)]
	for i, c := range s {
		if c == ' ' {
			c = ppSpaceMark
		}
		dst[i] = c
	}
	return dst
}

func ppPlural(n int, noun string) string {
	if n == 1 {
		return strconv.Itoa(n) + " " + noun
	}
	return strconv.Itoa(n) + " " + noun + "s"
}

// ppPassphraseCounts describes the passphrase in numbers, because a count is
// checkable against intent in a way a wall of characters is not (spec 5.1).
//
// Leading and trailing spaces are named rather than folded into the total: a
// fat-fingered trailing space is invisible on screen and on metal, and
// "hunter2 " is a different wallet from "hunter2". "no spaces" is stated
// affirmatively so that the absence of a space clause cannot be mistaken for a
// screen that forgot to count.
//
// The returned string contains only numbers and fixed words -- never the
// passphrase (spec 5.3).
func ppPassphraseCounts(s []byte) string {
	spaces := 0
	for _, c := range s {
		if c == ' ' {
			spaces++
		}
	}
	leading := 0
	for leading < len(s) && s[leading] == ' ' {
		leading++
	}
	trailing := 0
	for trailing < len(s) && s[len(s)-1-trailing] == ' ' {
		trailing++
	}
	out := ppPlural(len(s), "char")
	if spaces == 0 {
		out += ", no spaces"
	} else {
		out += ", " + ppPlural(spaces, "space")
	}
	if leading > 0 {
		out += ", " + strconv.Itoa(leading) + " leading"
	}
	if trailing > 0 {
		out += ", " + strconv.Itoa(trailing) + " trailing"
	}
	return out
}

// ppQRChoiceFlow is step 4 of spec 5. The QR is OPT-IN and defaults to OFF
// (spec D8): it is a machine-readable copy of the secret itself, so a default
// that quietly added one would be the worst way to get this wrong.
// prior is the choice already made, if the operator is arriving here a second
// time via Back. Resetting it to off would silently undo a deliberate opt-in
// (Phase D review M2).
func ppQRChoiceFlow(ctx *Context, th *Colors, prior bool) (bool, bool) {
	cs := &ChoiceScreen{
		Title:   "QR Code",
		Lead:    "A QR is a machine-readable copy of the passphrase.",
		Choices: []string{"No QR", "Add QR"},
	}
	if prior {
		cs.choice = 1 // preserve a deliberate opt-in across Back
	}
	hookPPWidget("qr", cs)
	// ChoiceScreen.choice starts at 0, which is "No QR" -- the default is a
	// property of this ordering, so do not reorder the choices.
	sel, ok := cs.Choose(ctx, th)
	if !ok {
		return false, false
	}
	return sel == 1, true
}

// ppConfirmArea is the region the confirm screen's body may use: below the
// title, inboard of the nav column, with a small margin. Named so the fit test
// measures the same budget the layout spends.
func ppConfirmArea(dims image.Point) layout.Rectangle {
	const margin = 6
	navW := assets.NavBtnPrimary.Bounds().Dx() + 4
	return layout.Rectangle{
		Min: image.Pt(margin, leadingSize),
		Max: image.Pt(dims.X-navW, dims.Y-margin),
	}
}

func ppYesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

// ppFingerprintClaim renders one fingerprint line. A skipped field says "not
// recorded" rather than vanishing: a missing line is indistinguishable from a
// screen that forgot to draw it.
func ppFingerprintClaim(label, canonical string) string {
	if canonical == "" {
		return label + ": not recorded"
	}
	return label + ": " + passphrase.GroupFingerprint(canonical)
}

// ppConfirmBody lays out the confirm screen's content and returns it with its
// measured size, so a test can assert the worst case fits without scrolling.
//
// marked is the passphrase with spaces already replaced by ppSpaceMark; it is
// passed as a []byte and rendered with a %s verb, which text.Formatter accepts
// directly -- no string copy of the secret is made here.
func ppConfirmBody(ctx *Context, th *Colors, width int, marked []byte, counts, seedFP, combinedFP string, qr, hasSpace bool) (op.Op, image.Point) {
	var rt richText
	// The passphrase itself, revealed: a masked readout cannot be proof-read,
	// and this is the last moment before a permanent plate (spec 5.1).
	rt.Addf(&ctx.B, ctx.Styles.body, width, th.Text, "%s", marked)
	rt.Y += 4
	rt.Add(&ctx.B, ctx.Styles.subtitle, width, th.Text, counts)
	// Gate on a REAL space, never on the substituted buffer. `marked` contains
	// the mark for a literal '_' just as much as for a space, so gating on it
	// printed "_ = SPACE" for a passphrase with no spaces at all -- e.g.
	// "hunter_2" rendered as `hunter_2 / 8 chars, no spaces / _ = SPACE`, which
	// says there are no spaces and simultaneously asserts every '_' is one, and
	// promises a legend the plate will not engrave. backup.passphraseLayoutFor
	// gates on strings.ContainsRune(plate.Passphrase, ' '); match it.
	if hasSpace {
		rt.Add(&ctx.B, ctx.Styles.body, width, th.Text, ppSpaceLegend)
	}
	rt.Y += 4
	rt.Add(&ctx.B, ctx.Styles.body, width, th.Text, ppFingerprintClaim("Seed FP", seedFP))
	rt.Add(&ctx.B, ctx.Styles.body, width, th.Text, ppFingerprintClaim("Expected comb FP", combinedFP))
	rt.Add(&ctx.B, ctx.Styles.body, width, th.Text, "QR: "+ppYesNo(qr))
	rt.Y += 4
	rt.Add(&ctx.B, ctx.Styles.body, width, th.Text, ppConfirmWarning)
	return rt.Content, image.Pt(width, rt.Y)
}

// ppConfirmWarning is spec 5.1's statement that nothing here has been checked.
// The passphrase is the dangerous one: a wrong fingerprint is a wrong label,
// but a wrong passphrase is a different wallet, and it fails silently.
// The fingerprint clause deliberately echoes the plate's own footer,
// backup.passphraseFooter ("FINGERPRINTS TYPED, NOT VERIFIED"), so the screen
// and the steel say the same thing.
const ppConfirmWarning = "Fingerprints are typed, not verified. " +
	"A wrong passphrase does not fail: it opens a DIFFERENT wallet."

// ppConfirmFlow is step 5 of spec 5: the last checkpoint before a permanent
// plate. It returns true to engrave, false to go back.
func ppConfirmFlow(ctx *Context, th *Colors, secret []byte, seedFP, combinedFP string, qr bool) bool {
	backBtn := &Clickable{Button: Button1}
	okBtn := &Clickable{Button: Button3}
	hookPPWidget("back", backBtn)
	hookPPWidget("ok", okBtn)
	// The marked copy is a []byte, wiped on return, so proof-reading the
	// passphrase adds no unwipeable copy of it (spec 5.3).
	marked := make([]byte, len(secret))
	defer wipeBytes(marked)
	ppMarkSpaces(marked, secret)
	counts := ppPassphraseCounts(secret)
	// Derived from the RAW secret, never from `marked` -- see the gate below.
	hasSpace := bytes.IndexByte(secret, ' ') >= 0
	for !ctx.Done {
		if backBtn.Clicked(ctx) {
			return false
		}
		if okBtn.Clicked(ctx) {
			return true
		}
		dims := ctx.Platform.DisplaySize()
		area := ppConfirmArea(dims)
		body, _ := ppConfirmBody(ctx, th, area.Dx(), marked, counts, seedFP, combinedFP, qr, hasSpace)
		body = body.Offset(image.Point(area.Min))
		nav, _ := layoutNavigation(&ctx.B, th, dims, []NavButton{
			{Clickable: backBtn, Style: StyleSecondary, Icon: assets.IconBack},
			{Clickable: okBtn, Style: StylePrimary, Icon: assets.IconCheckmark},
		}...)
		title, _ := layoutTitle(ctx, dims.X, th.Text, "Confirm")
		ctx.Frame(op.Layer(body, nav, title, op.Color(&ctx.B, th.Background)))
	}
	return false
}

// passphraseSecretHook is a test-only seam handing the flow's secret buffer to
// a test, so it can assert the buffer really was wiped once the flow returned
// -- on a completed engrave and on an abort alike (spec 5.3). The test keeps
// the slice header and reads the backing array AFTER the wipe defer has run.
// nil in production; mirrors bip85SeedHook.
var passphraseSecretHook func(secret []byte)

// passphrasePlateHook receives exactly what ppBuildPlate was handed. nil in
// production; mirrors freetextPlateHook. Without it there is no way to bind
// what the confirm screen showed to what was actually engraved: secret here
// is whatever the CALLER passed -- correctly, engravePassphraseFlow passes
// secret[:n] -- but secret's backing array can be up to passphrase.MaxLen
// bytes and, after the PASSPROOF! pattern shrinks n, the bytes past n can
// still hold a printable tail of a longer passphrase typed earlier in the
// same flow. A caller that passed the whole buffer instead of secret[:n]
// would leak that tail onto the plate, and nothing short of observing the
// exact slice handed here catches it -- a unit test of ppBuildPlate cannot,
// because the defect would be in the caller, not this function.
var passphrasePlateHook func(secret []byte, seedFP, combinedFP string, qr bool)

// ppBuildPlate turns the collected fields into an engravable plate.
//
// RESIDUAL COPY (spec 5.3): backup.Passphrase.Passphrase is a Go string, so
// this conversion leaves a copy of the secret on the heap that cannot be
// wiped. It is the single conversion on the engrave path and it is required by
// the Phase C plate type; everything upstream of here -- entry, the confirm
// screen's marked readout, the counts -- works on []byte. Mitigating context,
// not an excuse: RAM is volatile, the device is air-gapped, and it powers down
// between uses.
//
// SeedFP and CombinedFP arrive canonical from fingerprintEntryFlow, which is
// the precondition backup.Passphrase documents but does not check.
func ppBuildPlate(params engrave.Params, secret []byte, seedFP, combinedFP string, qr bool) (Plate, error) {
	if passphrasePlateHook != nil {
		passphrasePlateHook(secret, seedFP, combinedFP, qr)
	}
	desc := backup.Passphrase{
		Passphrase: string(secret),
		SeedFP:     seedFP,
		CombinedFP: combinedFP,
		QR:         qr,
		Font:       constant.Font,
	}
	side, err := backup.EngravePassphrase(params, desc)
	if err != nil {
		return Plate{}, err
	}
	return toPlate(side, params)
}

// The steps of spec 5, in order. Named so the Back transitions read as
// movement through the flow rather than arithmetic.
type ppStep int

const (
	ppStepEntry ppStep = iota
	ppStepSeedFP
	ppStepCombinedFP
	ppStepQR
	ppStepConfirm
	ppStepEngrave
)

// engravePassphraseFlow is the engravePassphrase program (spec 5): enter a
// BIP-39 passphrase, optionally record the two user-typed fingerprints, choose
// whether to include a QR, review, and engrave.
//
// Back steps BACKWARDS through the flow rather than abandoning it, so a
// mis-tap on step 5 does not throw away a 100-character passphrase; only Back
// on the first step leaves the program.
//
// SECRET HANDLING (spec 5.3), stated honestly:
//
//   - The passphrase is accumulated in secret, a []byte wiped by a defer that
//     runs on EVERY exit -- completed engrave, abort at any step, and
//     ctx.Done. The confirm screen's mark-substituted copy is likewise a
//     []byte, wiped when that screen returns.
//   - It is never logged, never written to any store, and never sent over NFC.
//     No error value on this path carries it either: the messages are
//     constants, and the passphrase package's sentinels carry no input.
//   - It CANNOT be fully wiped, and this code does not claim otherwise.
//     PassphraseKeyboard.Fragment is a Go string grown by concatenation, so
//     every keystroke leaves an unreachable heap copy of a prefix. Two further
//     unwipeable copies exist by construction: seeding the keyboard when
//     stepping back into entry, and backup.Passphrase.Passphrase in
//     ppBuildPlate. Both are marked at their site. What is achievable is that
//     the buffers this flow OWNS are zeroed, and that no additional string
//     copy is made for display, counting or proof-reading.
func engravePassphraseFlow(ctx *Context, th *Colors) {
	secret := make([]byte, passphrase.MaxLen)
	// The only scrub defer, and so the last to run: it zeroes the backing
	// array after every other defer, on every return path.
	defer wipeBytes(secret)
	if passphraseSecretHook != nil {
		passphraseSecretHook(secret)
	}
	n := 0
	var seedFP, combinedFP string
	qr := false
	// The pass-proof trigger fires inside ONE field but must populate all
	// three, and no field step owns the other two -- so the writer lives here,
	// where the state does, and each step is handed it. A step wired with nil
	// silently loses the trigger; TestPassProofTriggersFromEveryField drives
	// this production flow from each field in turn, so that mutation fails.
	loadProof := ppPassProofLoader(secret, &n, &seedFP, &combinedFP)
	step := ppStepEntry
	for !ctx.Done {
		switch step {
		case ppStepEntry:
			m, ok := passphraseEntryFlow(ctx, th, secret, n, loadProof)
			if !ok {
				return // Back out of the first step leaves the program.
			}
			n = m
		case ppStepSeedFP:
			fp, ok := fingerprintEntryFlow(ctx, th, ppSeedFP, seedFP, loadProof)
			if !ok {
				step -= 2
				break
			}
			seedFP = fp
		case ppStepCombinedFP:
			fp, ok := fingerprintEntryFlow(ctx, th, ppCombinedFP, combinedFP, loadProof)
			if !ok {
				step -= 2
				break
			}
			combinedFP = fp
		case ppStepQR:
			add, ok := ppQRChoiceFlow(ctx, th, qr)
			if !ok {
				step -= 2
				break
			}
			qr = add
		case ppStepConfirm:
			if !ppConfirmFlow(ctx, th, secret[:n], seedFP, combinedFP, qr) {
				step -= 2
				break
			}
		case ppStepEngrave:
			plate, err := ppBuildPlate(ctx.Platform.EngraverParams(), secret[:n], seedFP, combinedFP, qr)
			if err != nil {
				// The message names no part of the passphrase.
				showError(ctx, th, "Passphrase", "This passphrase does not fit a plate.")
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
