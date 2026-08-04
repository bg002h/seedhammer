package gui

import (
	"slices"
	"strconv"
	"strings"
	"testing"

	qrpkg "github.com/seedhammer/kortschak-qr"
	"seedhammer.com/backup"
	"seedhammer.com/bspline"
	"seedhammer.com/gui/text"
)

// Every test in this file drives the flow by PointerEvent, never by a
// synthesized ButtonEvent: SeedHammer II has no directional buttons, so a
// screen wired to ButtonFilter alone is dead on the machine and green in a
// button-driven test.

// ftRun captures a whole free-text program run.
type ftRun struct {
	done bool

	// What EngraveFreeText was handed, via freetextPlateHook.
	gotSize   float32
	gotTitle  string
	gotLines  []string
	gotFooter string
	gotQR     *qrpkg.Code
	gotPlate  bool
}

func startFT(t *testing.T) (*ppHarness, *ftRun) {
	t.Helper()
	h := newPPHarness(t)
	r := new(ftRun)
	freetextPlateHook = func(fontMM float32, title string, lines []string, footer string, qrc *qrpkg.Code) {
		r.gotSize, r.gotTitle, r.gotFooter, r.gotQR, r.gotPlate = fontMM, title, footer, qrc, true
		r.gotLines = slices.Clone(lines)
	}
	t.Cleanup(func() { freetextPlateHook = nil })
	h.start(func() {
		engraveTextFlow(h.ctx, &descriptorTheme)
		r.done = true
	})
	return h, r
}

// ftKbd returns the keyboard the current step registered.
func ftKbd(h *ppHarness) *PassphraseKeyboard {
	h.t.Helper()
	k, ok := h.widget("kbd").(*PassphraseKeyboard)
	if !ok {
		h.t.Fatal("widget \"kbd\" is not a *PassphraseKeyboard")
	}
	return k
}

// ftSetText replaces the field wholesale. Used only where typing thousands of
// characters by touch would take thousands of frames; every step that can be
// driven by real taps is.
func ftSetText(h *ppHarness, s string) {
	h.t.Helper()
	ftKbd(h).Fragment = s
	h.next("after setting a %d-character field", len(s))
}

// ftChoose taps choice i on the ChoiceScreen registered under name.
func ftChoose(h *ppHarness, name string, i int) {
	h.t.Helper()
	cs, ok := h.widget(name).(*ChoiceScreen)
	if !ok {
		h.t.Fatalf("widget %q is not a *ChoiceScreen", name)
	}
	if i >= len(cs.children) {
		h.t.Fatalf("choice %d out of range (%d drawn)", i, len(cs.children))
	}
	h.tapAt(h.point(&cs.children[i].click, "choice "+cs.Choices[i]))
	h.next("after selecting %q", cs.Choices[i])
	// Selecting is not choosing: ChoiceScreen returns on its OWN primary nav
	// button, which it owns privately, so the confirmation is a nav tap.
	h.tapNav(Button3)
}

// ftPastQR taps through step 1.
func ftPastQR(h *ppHarness, add bool) {
	h.t.Helper()
	h.mustReach("QRCode")
	sel := 0
	if add {
		sel = 1
	}
	ftChoose(h, "qr", sel)
	h.mustReach("lines")
}

func ftOK(h *ppHarness) {
	h.t.Helper()
	h.tapWidget("ok")
}

func ftBack(h *ppHarness) {
	h.t.Helper()
	h.tapWidget("back")
}

// TestFTFlowOrderIsQRFirst pins spec 7's ordering. The QR choice must be taken
// before the text is typed, because it is what the admission anchor is fixed
// against.
func TestFTFlowOrderIsQRFirst(t *testing.T) {
	h, _ := startFT(t)
	if !uiContains(h.content, "QR Code") {
		t.Fatalf("the first screen is not the QR choice; got %q", h.content)
	}
	ftPastQR(h, false)
	h.typeString("hi")
	ftOK(h)
	h.mustReach("Title")
	h.typeString("t")
	ftOK(h)
	h.mustReach("Footer")
	h.typeString("f")
	ftOK(h)
	h.mustReach("Confirm")
}

// TestConfirmLinesEqualWrapText is spec 5's invariant, on-screen half: the
// lines the operator approves must be WrapText's, character for character.
//
// Each line is rendered as its OWN unwrapped label. A width-bounded
// widget.Labelw would re-wrap in the proportional screen face and break exactly
// this.
func TestConfirmLinesEqualWrapText(t *testing.T) {
	const text = "Dear heir the hardware wallet is in the safe and the PIN is not written down anywhere at all"
	h, _ := startFT(t)
	ftPastQR(h, false)
	ftSetText(h, text)
	ftOK(h)
	h.mustReach("Title")
	ftOK(h) // skip
	h.mustReach("Footer")
	ftOK(h) // skip
	h.mustReach("Confirm")

	_, want, _, err := backup.Fit(h.ctx.Platform.EngraverParams(), text, "", "", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(want) < 3 {
		t.Fatalf("this test needs a text that wraps; got %d lines", len(want))
	}
	// Rendered text carries no spaces: a space advances the pen and inks
	// nothing, so ExtractText never sees one.
	var sb strings.Builder
	for _, l := range want {
		sb.WriteString(strings.ReplaceAll(l, " ", ""))
	}
	if !uiHas(h.content, sb.String()) {
		t.Errorf("the confirm screen does not show WrapText's lines in order.\nwant substring %q\n         frame %q", sb.String(), h.content)
	}
}

// TestConfirmLinesAreNotRewrapped: the same text at a width the screen face
// would break differently must still show the plate's lines. A single
// concatenated blob would pass the test above by accident, so this one asserts
// the line COUNT the screen reports.
func TestConfirmLinesAreNotRewrapped(t *testing.T) {
	const text = "aaaa bbbb cccc dddd eeee ffff gggg hhhh iiii jjjj kkkk llll mmmm nnnn oooo pppp"
	h, _ := startFT(t)
	ftPastQR(h, false)
	ftSetText(h, text)
	ftOK(h)
	h.mustReach("Title")
	ftOK(h)
	h.mustReach("Footer")
	ftOK(h)
	h.mustReach("Confirm")
	_, want, _, err := backup.Fit(h.ctx.Platform.EngraverParams(), text, "", "", false)
	if err != nil {
		t.Fatal(err)
	}
	if !uiContains(h.content, strconv.Itoa(len(want))+" lines") {
		t.Errorf("confirm screen does not report %d lines; frame %q", len(want), h.content)
	}
}

// TestFTReadoutIsLinesUsedOverAvailable is spec 6. Never "characters
// remaining": under word wrap no scalar character count is correct, since
// appending "x" to the last word can cost a whole line while appending " x"
// does not.
func TestFTReadoutIsLinesUsedOverAvailable(t *testing.T) {
	h, _ := startFT(t)
	ftPastQR(h, false)
	rows := backup.LinesPerPlate(h.ctx.Platform.EngraverParams(), backup.FontSizes[len(backup.FontSizes)-1])
	avail := rows - 2

	h.typeString("hi")
	if !uiHas(h.content, "1/"+strconv.Itoa(avail)+"lines") {
		t.Errorf("readout does not show lines used/available; frame %q", h.content)
	}
	// The fitted size is shown and drops as the text grows.
	if !uiHas(h.content, "6.0mm") {
		t.Errorf("readout does not show the fitted size; frame %q", h.content)
	}
	ftSetText(h, strings.Repeat("a", 900))
	if !uiHas(h.content, "3.0mm") {
		t.Errorf("the fitted size did not drop to 3.0mm at 900 characters; frame %q", h.content)
	}
	// "remaining" must appear nowhere. The needle is one word, and it cannot
	// occur in any label this screen draws.
	if uiContains(h.content, "remaining") {
		t.Errorf("the readout offers a character budget; frame %q", h.content)
	}
}

// TestFTOverCapacityIsShownNotDropped is spec 6 and
// gui/passphrase_flow.go:113-118's reviewed decision: keystrokes are accepted,
// the readout shows the over-capacity state, and OK refuses.
func TestFTOverCapacityIsShownNotDropped(t *testing.T) {
	h, _ := startFT(t)
	ftPastQR(h, false)
	huge := strings.Repeat("a", 2000)
	ftSetText(h, huge)
	// Not clamped.
	if got := ftKbd(h).Fragment; got != huge {
		t.Errorf("keystrokes were dropped: field holds %d characters, %d were entered", len(got), len(huge))
	}
	// The readout says so: used exceeds available.
	rows := backup.LinesPerPlate(h.ctx.Platform.EngraverParams(), backup.FontSizes[len(backup.FontSizes)-1])
	used, avail, ok := backup.Admissible(h.ctx.Platform.EngraverParams(), huge, "", "", false)
	if ok || avail != rows-2 || used <= avail {
		t.Fatalf("this test needs an over-capacity text; got %d/%d ok=%v", used, avail, ok)
	}
	if !uiHas(h.content, strconv.Itoa(used)+"/"+strconv.Itoa(avail)+"lines") {
		t.Errorf("the readout does not show the over-capacity state; frame %q", h.content)
	}
	// And OK refuses, naming the field.
	ftOK(h)
	if !uiContains(h.content, "Text") || !uiContains(h.content, "smallest size") {
		t.Errorf("OK did not refuse naming the field; frame %q", h.content)
	}
	if uiContains(h.content, "Title") {
		t.Errorf("OK advanced to the Title step with an over-capacity text; frame %q", h.content)
	}
}

// TestFTRefusalOffersTheQRRatherThanDroppingIt is spec 6 and decision 12.3.
// Dropping the QR silently changes what a scanner returns from the plate, so it
// is offered as an explicit choice -- with the figure computed from a LIVE
// encode, not from spec 4's geometry column.
func TestFTRefusalOffersTheQRRatherThanDroppingIt(t *testing.T) {
	h, _ := startFT(t)
	ftPastQR(h, true)
	text := strings.Repeat("a", 700)
	ftSetText(h, text)
	ftOK(h)
	h.mustReach("TooLong")

	P := h.ctx.Platform.EngraverParams()
	smallest := backup.FontSizes[len(backup.FontSizes)-1]
	freed := backup.MaxCharsAt(P, smallest, text, false) - backup.MaxCharsAt(P, smallest, text, true)
	if freed != 640 {
		t.Fatalf("the live figure is %d, not the measured 640; the test's premise has moved", freed)
	}
	if !uiHas(h.content, strconv.Itoa(freed)) {
		t.Errorf("the refusal does not name the live figure %d; frame %q", freed, h.content)
	}
	// 135 is what spec 4's geometry column would suggest (1104 -> 969).
	if uiHas(h.content, "135characters") {
		t.Errorf("the refusal quotes the geometry column, not a live encode; frame %q", h.content)
	}
	// Declining leaves the QR on and the text unchanged.
	ftChoose(h, "refusal", 0)
	h.mustReach("lines")
	if got := ftKbd(h).Fragment; got != text {
		t.Errorf("declining the refusal changed the text")
	}
	ftOK(h)
	h.mustReach("TooLong")
	// Accepting drops the QR, and only then does the text fit.
	ftChoose(h, "refusal", 1)
	h.mustReach("lines")
	ftOK(h)
	h.mustReach("Title")
}

// TestFTTitleAndFooterCap is spec 2's unconditional 18-character cap.
func TestFTTitleAndFooterCap(t *testing.T) {
	for _, step := range []string{"Title", "Footer"} {
		t.Run(step, func(t *testing.T) {
			h, _ := startFT(t)
			ftPastQR(h, false)
			h.typeString("hi")
			ftOK(h)
			h.mustReach("Title")
			if step == "Footer" {
				ftOK(h) // skip the title
				h.mustReach("Footer")
			}
			ftSetText(h, strings.Repeat("W", backup.MaxTitleLen+1))
			ftOK(h)
			if !uiContains(h.content, "screw-hole row") {
				t.Errorf("%s did not refuse %d characters; frame %q", step, backup.MaxTitleLen+1, h.content)
			}
			h.tapNav(Button3) // dismiss the modal
			h.mustReach("optional")
			// Exactly the cap is accepted.
			ftSetText(h, strings.Repeat("W", backup.MaxTitleLen))
			ftOK(h)
			if uiContains(h.content, "screw-hole row") {
				t.Errorf("%s refused exactly %d characters; frame %q", step, backup.MaxTitleLen, h.content)
			}
		})
	}
}

// TestFTBackPreservesEveryValue is spec 7. Driven through the REAL flow: a test
// that sets a field and then asserts the field would pass on a flow that
// forgets everything.
func TestFTBackPreservesEveryValue(t *testing.T) {
	h, _ := startFT(t)
	ftPastQR(h, true)
	h.typeString("note")
	ftOK(h)
	h.mustReach("Title")
	h.typeString("TT")
	ftOK(h)
	h.mustReach("Footer")
	h.typeString("FF")
	ftOK(h)
	h.mustReach("Confirm")

	// Back all the way to the QR choice, checking each field survived.
	ftBack(h)
	h.mustReach("Footer")
	if got := ftKbd(h).Fragment; got != "FF" {
		t.Errorf("footer = %q after Back, want %q", got, "FF")
	}
	ftBack(h)
	h.mustReach("Title")
	if got := ftKbd(h).Fragment; got != "TT" {
		t.Errorf("title = %q after Back, want %q", got, "TT")
	}
	ftBack(h)
	h.mustReach("lines")
	if got := ftKbd(h).Fragment; got != "note" {
		t.Errorf("text = %q after Back, want %q", got, "note")
	}
	ftBack(h)
	h.mustReach("QRCode")
	cs, ok := h.widget("qr").(*ChoiceScreen)
	if !ok {
		t.Fatal("widget \"qr\" is not a *ChoiceScreen")
	}
	if cs.choice != 1 {
		t.Errorf("the QR opt-in was reset by Back: choice = %d, want 1", cs.choice)
	}
	// Forward again: every field is still there.
	ftChoose(h, "qr", 1)
	h.mustReach("lines")
	if got := ftKbd(h).Fragment; got != "note" {
		t.Errorf("text = %q after going forward again, want %q", got, "note")
	}
}

// TestFTPlateIsWhatWasApproved is the end-to-end half of spec 5's invariant.
// D2.1 covers only the on-screen half; this binds the layout the operator
// APPROVED to the one EngraveFreeText was handed. Plate is {Duration, Spline}
// -- stroke geometry with no text in it -- so nothing can be recovered from the
// plate itself, hence the hook.
func TestFTPlateIsWhatWasApproved(t *testing.T) {
	const text = "Dear heir the hardware wallet is in the safe and the PIN is not written down"
	h, r := startFT(t)
	ftPastQR(h, true)
	ftSetText(h, text)
	ftOK(h)
	h.mustReach("Title")
	ftSetText(h, "TO MY HEIR")
	ftOK(h)
	h.mustReach("Footer")
	ftSetText(h, "2026 COPY 1")
	ftOK(h)
	h.mustReach("Confirm")

	// What the screen displayed.
	P := h.ctx.Platform.EngraverParams()
	wantSize, wantLines, wantQR, err := backup.Fit(P, text, "TO MY HEIR", "2026 COPY 1", true)
	if err != nil {
		t.Fatal(err)
	}
	var sb strings.Builder
	for _, l := range wantLines {
		sb.WriteString(strings.ReplaceAll(l, " ", ""))
	}
	if !uiHas(h.content, sb.String()) {
		t.Fatalf("the confirm screen does not show the fitted lines; frame %q", h.content)
	}

	ftOK(h) // engrave
	h.step()
	if !r.gotPlate {
		t.Fatal("the flow never built a plate")
	}
	if r.gotSize != wantSize {
		t.Errorf("engraved at %.1fmm, confirmed at %.1fmm", r.gotSize, wantSize)
	}
	if !slices.Equal(r.gotLines, wantLines) {
		t.Errorf("engraved lines differ from the confirmed ones:\n got %q\nwant %q", r.gotLines, wantLines)
	}
	if r.gotTitle != "TO MY HEIR" || r.gotFooter != "2026 COPY 1" {
		t.Errorf("title/footer engraved as %q/%q, want %q/%q", r.gotTitle, r.gotFooter, "TO MY HEIR", "2026 COPY 1")
	}
	if r.gotQR == nil {
		t.Fatal("no QR was engraved")
	}
	// Module level, not "a decoder returns the text": a decoder that ignores
	// trailing data would pass while the modules differed.
	if r.gotQR.Size != wantQR.Size || !slices.Equal(r.gotQR.Bitmap, wantQR.Bitmap) {
		t.Error("the engraved QR is not the code Fit measured")
	}
}

// TestFTQREncodesTheTextOnly is spec 2: the QR carries the Text field and
// nothing else. Asserted at MODULE level -- a decoder ignoring trailing data
// would pass while the modules differed.
func TestFTQREncodesTheTextOnly(t *testing.T) {
	const text = "the note"
	h, r := startFT(t)
	ftPastQR(h, true)
	ftSetText(h, text)
	ftOK(h)
	h.mustReach("Title")
	ftSetText(h, "A TITLE")
	ftOK(h)
	h.mustReach("Footer")
	ftSetText(h, "A FOOTER")
	ftOK(h)
	h.mustReach("Confirm")
	ftOK(h)
	h.step()
	if !r.gotPlate || r.gotQR == nil {
		t.Fatal("no QR plate was built")
	}
	want, err := qrpkg.Encode(text, qrpkg.L)
	if err != nil {
		t.Fatal(err)
	}
	if r.gotQR.Size != want.Size || !slices.Equal(r.gotQR.Bitmap, want.Bitmap) {
		t.Error("the engraved QR is not qr.Encode(Text); a field other than Text reached it")
	}
	// And a code over the concatenation would be a DIFFERENT code, so the
	// assertion above is not vacuous.
	other, err := qrpkg.Encode(text+"A TITLE"+"A FOOTER", qrpkg.L)
	if err != nil {
		t.Fatal(err)
	}
	if slices.Equal(other.Bitmap, want.Bitmap) {
		t.Fatal("text and text+title+footer encode identically; this test proves nothing")
	}
}

// TestFTNoQRMeansNoCode: opting out must reach EngraveFreeText as a nil code,
// not an empty one.
func TestFTNoQRMeansNoCode(t *testing.T) {
	h, r := startFT(t)
	ftPastQR(h, false)
	h.typeString("hi")
	ftOK(h)
	h.mustReach("Title")
	ftOK(h)
	h.mustReach("Footer")
	ftOK(h)
	h.mustReach("Confirm")
	ftOK(h)
	h.step()
	if !r.gotPlate {
		t.Fatal("the flow never built a plate")
	}
	if r.gotQR != nil {
		t.Errorf("a %d-module QR was engraved after the operator declined one", r.gotQR.Size)
	}
	if r.gotTitle != "" || r.gotFooter != "" {
		t.Errorf("skipped title/footer arrived as %q/%q, want empty", r.gotTitle, r.gotFooter)
	}
}

// TestFTBuildPlateEncodesOnce is D2a.2's "never encode a second time", checked
// where it can be: ftBuildPlate must hand EngraveFreeText the very code Fit
// returned.
func TestFTBuildPlateEncodesOnce(t *testing.T) {
	const text = "a note that needs a code"
	var got *qrpkg.Code
	freetextPlateHook = func(_ float32, _ string, _ []string, _ string, qrc *qrpkg.Code) { got = qrc }
	t.Cleanup(func() { freetextPlateHook = nil })
	P := newPlatform().EngraverParams()
	if _, err := ftBuildPlate(P, text, "T", "F", true); err != nil {
		t.Fatal(err)
	}
	_, _, want, err := backup.Fit(P, text, "T", "F", true)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("no code reached EngraveFreeText")
	}
	if got.Size != want.Size || !slices.Equal(got.Bitmap, want.Bitmap) {
		t.Error("ftBuildPlate engraved a code Fit did not measure")
	}
}

// TestConfirmLinesAreOwnUnwrappedLabels is the geometric half of spec 5's
// on-screen invariant, and it exists because the textual half CANNOT see the
// defect it guards: op.Drawer.ExtractText concatenates runes in draw order, so
// a line re-wrapped into two screen rows extracts to exactly the same string.
// Two mutations -- bounding each line at the panel width, and joining the lines
// into one wrapped blob -- both survived a text-only assertion.
//
// So the height of the line block is measured instead. Rendered as its own
// UNWRAPPED label, each plate line costs exactly one screen row, whatever the
// panel width.
func TestConfirmLinesAreOwnUnwrappedLabels(t *testing.T) {
	ctx := NewContext(newPlatform())
	th := &descriptorTheme
	// Lines far wider than any panel this screen is laid out on.
	f := ftFit{
		sizeMM: 3.0,
		lines:  []string{strings.Repeat("W", 44), strings.Repeat("m", 44), "short"},
	}
	empty := f
	empty.lines = nil

	lineBlock := func(width int) int {
		_, with := ftConfirmBody(ctx, th, width, f, "", "", false)
		_, without := ftConfirmBody(ctx, th, width, empty, "", "", false)
		return with.Y - without.Y
	}
	// richText.Addf advances Y by ascent+descent for an unwrapped label, and by
	// a further LineHeight for every break the layout inserts. So one row per
	// plate line, at every width, is exactly the claim.
	rowOf := func(st text.Style) int {
		m := st.Face.Metrics()
		return m.Ascent.Ceil() + m.Descent.Ceil()
	}
	row := rowOf(ctx.Styles.body)
	for _, width := range []int{sh2DisplaySize.X, sh2DisplaySize.X / 2, sh2DisplaySize.X / 4} {
		if got, want := lineBlock(width), len(f.lines)*row; got != want {
			t.Errorf("at width %d the %d plate lines occupy %dpx, want %dpx (%d rows of %dpx) -- they are being re-wrapped",
				width, len(f.lines), got, want, len(f.lines), row)
		}
	}
	// The title and footer are single rows too, in the subtitle style.
	titleRow := rowOf(ctx.Styles.subtitle)
	cap18 := strings.Repeat("W", 18)
	for _, width := range []int{sh2DisplaySize.X, sh2DisplaySize.X / 4} {
		_, plain := ftConfirmBody(ctx, th, width, empty, "", "", false)
		_, titled := ftConfirmBody(ctx, th, width, empty, cap18, "", false)
		if got := titled.Y - plain.Y; got != titleRow {
			t.Errorf("at width %d an 18-character title occupies %dpx, want one %dpx row", width, got, titleRow)
		}
		_, footed := ftConfirmBody(ctx, th, width, empty, "", cap18, false)
		if got := footed.Y - plain.Y; got != titleRow {
			t.Errorf("at width %d an 18-character footer occupies %dpx, want one %dpx row", width, got, titleRow)
		}
	}
	// The narrow width has to be one the title WOULD wrap at, or the check
	// above proves nothing.
	if ctx.Styles.subtitle.Measure(1<<30, "%s", cap18).X <= sh2DisplaySize.X/4 {
		t.Fatalf("an 18-character title is %dpx and fits the %dpx test width; widen the composition",
			ctx.Styles.subtitle.Measure(1<<30, "%s", cap18).X, sh2DisplaySize.X/4)
	}
}

// ftSpline collects a plate's knots so two plates can be compared as the
// geometry they are.
func ftSpline(t *testing.T, p Plate) []bspline.Knot {
	t.Helper()
	return slices.Collect(p.Spline)
}

// TestFTBuiltPlateIsTheFittedComposition compares the plate ftBuildPlate
// produces against one built independently from Fit's own answers.
//
// The hook alone cannot catch this: a builder that reports fontMM to the hook
// and engraves at another size passes every hook assertion. Measured -- that
// exact mutation survived until this test existed. Plate is stroke geometry,
// and stroke geometry is precisely what a wrong size changes.
func TestFTBuiltPlateIsTheFittedComposition(t *testing.T) {
	const text = "Dear heir the wallet is in the safe and the PIN is not written down at all"
	P := newPlatform().EngraverParams()
	for _, useQR := range []bool{false, true} {
		got, err := ftBuildPlate(P, text, "TO MY HEIR", "2026 COPY 1", useQR)
		if err != nil {
			t.Fatal(err)
		}
		size, lines, qrc, err := backup.Fit(P, text, "TO MY HEIR", "2026 COPY 1", useQR)
		if err != nil {
			t.Fatal(err)
		}
		want, err := toPlate(backup.EngraveFreeText(P, size, "TO MY HEIR", lines, "2026 COPY 1", qrc), P)
		if err != nil {
			t.Fatal(err)
		}
		if got.Duration != want.Duration {
			t.Errorf("qr=%v: built plate runs %d ticks, the fitted composition %d", useQR, got.Duration, want.Duration)
		}
		if g, w := ftSpline(t, got), ftSpline(t, want); !slices.Equal(g, w) {
			t.Errorf("qr=%v: the built plate is not the fitted composition (%d knots vs %d)", useQR, len(g), len(w))
		}
		// And it is NOT the same plate at another size, so the comparison is
		// not vacuous.
		other, err := toPlate(backup.EngraveFreeText(P, backup.FontSizes[len(backup.FontSizes)-1], "TO MY HEIR", lines, "2026 COPY 1", qrc), P)
		if err == nil && slices.Equal(ftSpline(t, got), ftSpline(t, other)) && size != backup.FontSizes[len(backup.FontSizes)-1] {
			t.Errorf("qr=%v: %.1fmm and %.1fmm produce identical geometry; this test cannot see a size change",
				useQR, size, backup.FontSizes[len(backup.FontSizes)-1])
		}
	}
}
