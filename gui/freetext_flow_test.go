package gui

import (
	"image"
	"math"
	"slices"
	"strconv"
	"strings"
	"testing"

	qrpkg "github.com/seedhammer/kortschak-qr"
	"seedhammer.com/backup"
	"seedhammer.com/bspline"
	"seedhammer.com/engrave"
	"seedhammer.com/font/sh"
	"seedhammer.com/font/vector"
	"seedhammer.com/gui/text"
)

// Every test in this file drives the flow by PointerEvent, never by a
// synthesized ButtonEvent: SeedHammer II has no directional buttons, so a
// screen wired to ButtonFilter alone is dead on the machine and green in a
// button-driven test.

// ftRun captures a whole free-text program run.
type ftRun struct {
	done bool

	// got is exactly what EngraveFitted was handed, via freetextPlateHook --
	// including Faces, the face each engraved line is cut in, which nothing
	// recoverable from the finished Plate can report.
	got      backup.Fitted
	gotPlate bool
}

func startFT(t *testing.T) (*ppHarness, *ftRun) {
	t.Helper()
	h := newPPHarness(t)
	r := new(ftRun)
	freetextPlateHook = func(f backup.Fitted) {
		r.got, r.gotPlate = f, true
		r.got.Lines = slices.Clone(f.Lines)
		r.got.Faces = slices.Clone(f.Faces)
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

// ftPastQR taps through steps 1 to 3: the QR choice, then the Font and Size
// screens at their DEFAULTS.
//
// The defaults are walked deliberately rather than skipped. Every caller of
// this helper asserts behaviour that predates the two pickers, so taking index
// 0 on both is what keeps those assertions meaningful -- and if either default
// ever moved, all of them would start failing at once, which is the alarm we
// want rather than a silent change of plate.
func ftPastQR(h *ppHarness, add bool) {
	h.t.Helper()
	h.mustReach("QRCode")
	sel := 0
	if add {
		sel = 1
	}
	ftChoose(h, "qr", sel)
	ftPastFaceAndSize(h)
	h.mustReach("lines")
}

// ftPastFaceAndSize takes index 0 on both pickers: font/sh and Auto-fit.
//
// Index 0 is also the ONLY entry when a proof composition is loaded, so this
// works unchanged on the way forward through a ladder.
func ftPastFaceAndSize(h *ppHarness) {
	h.t.Helper()
	h.mustReach("Font")
	ftChoose(h, "face", 0)
	h.mustReach("Size")
	ftChoose(h, "size", 0)
}

// ftBackToQR steps back from the text screen to the QR screen, through the two
// pickers that now sit between them. ChoiceScreen owns its Back button
// privately, so each picker is left by nav slot rather than by widget.
func ftBackToQR(h *ppHarness) {
	h.t.Helper()
	ftBack(h)
	h.mustReach("Size")
	h.tapNav(Button1)
	h.mustReach("Font")
	h.tapNav(Button1)
	h.mustReach("QRCode")
}

func ftOK(h *ppHarness) {
	h.t.Helper()
	h.tapWidget("ok")
}

// ftConfirmPages taps through every page of the confirm screen and returns the
// frames it drew, starting with the one already on screen. It stops when the
// pager wraps back to the first page.
//
// A preview that does not fit the panel is PAGED, not truncated -- the size
// line and the warnings are pinned so they cannot be pushed off the bottom --
// so an assertion about what the operator can read has to walk the pages.
func ftConfirmPages(h *ppHarness) []string {
	h.t.Helper()
	pages := []string{h.content}
	page, ok := h.widget("page").(*Clickable)
	if !ok {
		h.t.Fatal(`widget "page" is not a *Clickable`)
	}
	if _, drawn := h.drawer().TagBounds(page); !drawn {
		return pages // everything fit at once
	}
	for range 64 {
		h.tapAt(h.point(page, "the confirm pager"))
		h.step()
		if h.content == pages[0] {
			return pages
		}
		pages = append(pages, h.content)
	}
	h.t.Fatalf("the confirm pager never wrapped back to its first page after %d taps", len(pages))
	return nil
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

	_, want, _, err := backup.Fit(h.ctx.Platform.EngraverParams(), sh.Font, text, "", "", false)
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
	_, want, _, err := backup.Fit(h.ctx.Platform.EngraverParams(), sh.Font, text, "", "", false)
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
	used, avail, ok := backup.Admissible(h.ctx.Platform.EngraverParams(), sh.Font, huge, "", "", false)
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
	freed := backup.MaxCharsAt(P, sh.Font, smallest, text, false) - backup.MaxCharsAt(P, sh.Font, smallest, text, true)
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
	// Back out of Title lands on the text step directly: Speed no longer sits
	// between them.
	ftBack(h)
	h.mustReach("lines")
	if got := ftKbd(h).Fragment; got != "note" {
		t.Errorf("text = %q after Back, want %q", got, "note")
	}
	ftBackToQR(h)
	cs, ok := h.widget("qr").(*ChoiceScreen)
	if !ok {
		t.Fatal("widget \"qr\" is not a *ChoiceScreen")
	}
	if cs.choice != 1 {
		t.Errorf("the QR opt-in was reset by Back: choice = %d, want 1", cs.choice)
	}
	// Forward again: every field is still there.
	ftChoose(h, "qr", 1)
	ftPastFaceAndSize(h)
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
	wantSize, wantLines, wantQR, err := backup.Fit(P, sh.Font, text, "TO MY HEIR", "2026 COPY 1", true)
	if err != nil {
		t.Fatal(err)
	}
	// Paged: this composition is 8 rows against a QR's tighter budget, so the
	// preview does not fit at once. Every fitted line must be readable on SOME
	// page -- a pager that skipped one would let the operator approve a line
	// they never saw.
	pages := ftConfirmPages(h)
	if len(pages) < 2 {
		t.Fatalf("this test needs a paged confirm screen; got %d page(s)", len(pages))
	}
	all := strings.Join(pages, "\n")
	for i, l := range wantLines {
		if !uiHas(all, strings.ReplaceAll(l, " ", "")) {
			t.Fatalf("fitted line %d (%q) appears on no page of the confirm screen.\npages: %q", i, l, pages)
		}
	}

	ftOK(h) // engrave
	h.step()
	if !r.gotPlate {
		t.Fatal("the flow never built a plate")
	}
	if r.got.SizeMM != wantSize {
		t.Errorf("engraved at %.1fmm, confirmed at %.1fmm", r.got.SizeMM, wantSize)
	}
	if !slices.Equal(r.got.Lines, wantLines) {
		t.Errorf("engraved lines differ from the confirmed ones:\n got %q\nwant %q", r.got.Lines, wantLines)
	}
	if r.got.Title != "TO MY HEIR" || r.got.Footer != "2026 COPY 1" {
		t.Errorf("title/footer engraved as %q/%q, want %q/%q", r.got.Title, r.got.Footer, "TO MY HEIR", "2026 COPY 1")
	}
	if r.got.QR == nil {
		t.Fatal("no QR was engraved")
	}
	// Module level, not "a decoder returns the text": a decoder that ignores
	// trailing data would pass while the modules differed.
	if r.got.QR.Size != wantQR.Size || !slices.Equal(r.got.QR.Bitmap, wantQR.Bitmap) {
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
	if !r.gotPlate || r.got.QR == nil {
		t.Fatal("no QR plate was built")
	}
	want, err := qrpkg.Encode(text, qrpkg.L)
	if err != nil {
		t.Fatal(err)
	}
	if r.got.QR.Size != want.Size || !slices.Equal(r.got.QR.Bitmap, want.Bitmap) {
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
	if r.got.QR != nil {
		t.Errorf("a %d-module QR was engraved after the operator declined one", r.got.QR.Size)
	}
	if r.got.Title != "" || r.got.Footer != "" {
		t.Errorf("skipped title/footer arrived as %q/%q, want empty", r.got.Title, r.got.Footer)
	}
}

// TestFTBuildPlateEncodesOnce is D2a.2's "never encode a second time", checked
// where it can be: ftBuildPlate must hand EngraveFreeText the very code Fit
// returned.
func TestFTBuildPlateEncodesOnce(t *testing.T) {
	const text = "a note that needs a code"
	var got *qrpkg.Code
	freetextPlateHook = func(f backup.Fitted) { got = f.QR }
	t.Cleanup(func() { freetextPlateHook = nil })
	P := newPlatform().EngraverParams()
	if _, err := ftBuildPlate(P, &ftPlanSH, text, "T", "F", true, 0, 0); err != nil {
		t.Fatal(err)
	}
	_, _, want, err := backup.Fit(P, sh.Font, text, "T", "F", true)
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
	f := ftFit{plate: backup.Fitted{
		SizeMM: 3.0,
		Lines:  []string{strings.Repeat("W", 44), strings.Repeat("m", 44), "short"},
	}}
	empty := f
	empty.plate.Lines = nil

	// A budget nothing can page against, so this test measures the layout and
	// not the pager. The paging itself is TestFTConfirmPagesEveryRowExactlyOnce.
	const noPaging = 1 << 20
	body := func(width int, f ftFit, title, footer string) image.Point {
		v := ftConfirmBody(ctx, th, width, noPaging, 0, f, &ftPlanSH, title, footer, "")
		if v.Shown != v.Total {
			t.Fatalf("the %dpx budget still paged: %d of %d rows", noPaging, v.Shown, v.Total)
		}
		return v.Size
	}
	lineBlock := func(width int) int {
		return body(width, f, "", "").Y - body(width, empty, "", "").Y
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
		if got, want := lineBlock(width), len(f.plate.Lines)*row; got != want {
			t.Errorf("at width %d the %d plate lines occupy %dpx, want %dpx (%d rows of %dpx) -- they are being re-wrapped",
				width, len(f.plate.Lines), got, want, len(f.plate.Lines), row)
		}
	}
	// The title and footer are single rows too, in the subtitle style.
	titleRow := rowOf(ctx.Styles.subtitle)
	cap18 := strings.Repeat("W", 18)
	for _, width := range []int{sh2DisplaySize.X, sh2DisplaySize.X / 4} {
		plain := body(width, empty, "", "")
		titled := body(width, empty, cap18, "")
		if got := titled.Y - plain.Y; got != titleRow {
			t.Errorf("at width %d an 18-character title occupies %dpx, want one %dpx row", width, got, titleRow)
		}
		footed := body(width, empty, "", cap18)
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
		got, err := ftBuildPlate(P, &ftPlanSH, text, "TO MY HEIR", "2026 COPY 1", useQR, 0, 0)
		if err != nil {
			t.Fatal(err)
		}
		size, lines, qrc, err := backup.Fit(P, sh.Font, text, "TO MY HEIR", "2026 COPY 1", useQR)
		if err != nil {
			t.Fatal(err)
		}
		want, err := toPlate(backup.EngraveFreeText(P, sh.Font, size, "TO MY HEIR", lines, "2026 COPY 1", qrc), P)
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
		other, err := toPlate(backup.EngraveFreeText(P, sh.Font, backup.FontSizes[len(backup.FontSizes)-1], "TO MY HEIR", lines, "2026 COPY 1", qrc), P)
		if err == nil && slices.Equal(ftSpline(t, got), ftSpline(t, other)) && size != backup.FontSizes[len(backup.FontSizes)-1] {
			t.Errorf("qr=%v: %.1fmm and %.1fmm produce identical geometry; this test cannot see a size change",
				useQR, size, backup.FontSizes[len(backup.FontSizes)-1])
		}
	}
}

// TestFTBuiltPlateIsCutInTheFittedFace: the composition is measured in one face
// and engraved in another only if something is wrong, and NOTHING recoverable
// from the plate says which face it was -- Plate is {Duration, Spline}, stroke
// geometry with no text in it. So the geometry itself is the assertion.
//
// freetextPlateHook cannot cover this on its own: it reports the face
// ftBuildPlate was HANDED, so a builder that reports one face and engraves in
// the other passes every hook assertion in this file.
func TestFTBuiltPlateIsCutInTheFittedFace(t *testing.T) {
	const text = "Dear heir the wallet is in the safe and the PIN is not written down at all"
	P := newPlatform().EngraverParams()
	for _, plan := range []*ftPlan{&ftPlanSH, &ftPlanConst} {
		face := plan.Runs[0].Face
		got, err := ftBuildPlate(P, plan, text, "TO MY HEIR", "2026 COPY 1", false, 0, 0)
		if err != nil {
			t.Fatal(err)
		}
		size, lines, qrc, err := backup.Fit(P, face.Face, text, "TO MY HEIR", "2026 COPY 1", false)
		if err != nil {
			t.Fatal(err)
		}
		want, err := toPlate(backup.EngraveFreeText(P, face.Face, size, "TO MY HEIR", lines, "2026 COPY 1", qrc), P)
		if err != nil {
			t.Fatal(err)
		}
		if g, w := ftSpline(t, got), ftSpline(t, want); !slices.Equal(g, w) {
			t.Errorf("%s: the built plate is not the composition cut in font/%s (%d knots vs %d)",
				face.Name, face.Name, len(g), len(w))
		}
		// And the OTHER face produces different geometry, so the comparison
		// above can actually see a wrong face.
		other := ftFaceConst
		if face == ftFaceConst {
			other = ftFaceSH
		}
		wrong, err := toPlate(backup.EngraveFreeText(P, other.Face, size, "TO MY HEIR", lines, "2026 COPY 1", qrc), P)
		if err == nil && slices.Equal(ftSpline(t, got), ftSpline(t, wrong)) {
			t.Errorf("%s: font/%s and font/%s produce identical geometry; this test cannot see a wrong face",
				face.Name, face.Name, other.Name)
		}
	}
}

// TestFTConfirmCarriesTheSafetyCopy is spec 9. A free-text box is where someone
// will type a seed phrase: it bypasses the wordlist, the checksum and the verify
// flow, and the confirm screen is the only place that says so.
//
// This is the TEXT half only. It cannot see whether the copy landed on the
// panel -- op.Drawer.ExtractText collects the runes of every drawn text op
// wherever it went -- and before the execution review it was the ONLY half,
// which made it a false PASS: measured, a 20-line composition put all three
// warnings at y=537 on a 320-pixel panel and this test stayed green. The
// geometric half is TestFTConfirmAlwaysFitsThePanel; neither is sufficient
// alone.
func TestFTConfirmCarriesTheSafetyCopy(t *testing.T) {
	for _, useQR := range []bool{false, true} {
		name := "without a QR"
		if useQR {
			name = "with a QR"
		}
		t.Run(name, func(t *testing.T) {
			h, _ := startFT(t)
			ftPastQR(h, useQR)
			h.typeString("hi")
			ftOK(h)
			h.mustReach("Title")
			ftOK(h)
			h.mustReach("Footer")
			ftOK(h)
			h.mustReach("Confirm")

			// Nothing here is checked.
			if !uiContains(h.content, "not a validated backup") {
				t.Errorf("the confirm screen does not say this is not a validated backup; frame %q", h.content)
			}
			// Duration leaks content.
			if !uiContains(h.content, "not constant-time") {
				t.Errorf("the confirm screen does not warn that engraving is not constant-time; frame %q", h.content)
			}
			// The QR clause is GATED. The needle is a phrase this screen uses
			// nowhere else -- uiContains strips spaces from its needle, so a
			// vaguer one ("readable") would match "machine-readable" and pass
			// vacuously.
			hasQRWarning := uiContains(h.content, "readable by any camera")
			if hasQRWarning != useQR {
				t.Errorf("QR warning present = %v, want %v; frame %q", hasQRWarning, useQR, h.content)
			}
			// And the QR state itself is stated either way.
			want := "QR: no"
			if useQR {
				want = "QR: yes"
			}
			if !uiContains(h.content, want) {
				t.Errorf("the confirm screen does not state %q; frame %q", want, h.content)
			}
			// The face is stated too. It decides how much fits and what the
			// plate looks like, and the proof triggers change it, so it must
			// never be something the operator has to guess.
			if !uiContains(h.content, "font: "+ftPlanSH.Name()) {
				t.Errorf("the confirm screen does not name the engraving face; frame %q", h.content)
			}
		})
	}
}

// ftWorstCompositions returns the LARGEST composition the flow will admit for
// each face plan / QR / title-and-footer combination -- the confirm screen's
// worst case, found by search rather than assumed.
//
// The mixed plan is searched too. Its texts here carry no '\n', so they collapse
// to one block and fit exactly as the sh plan's do -- but the SUMMARY differs:
// a mixed plan prints the measured row count of every run ("sh 24"), which is a
// longer string than a single-face plan's bare "sh" and could take a line the
// budget did not reserve.
func ftWorstCompositions(t *testing.T, P engrave.Params) []ftFit {
	t.Helper()
	cap18 := strings.Repeat("W", backup.MaxTitleLen)
	var out []ftFit
	for _, plan := range []*ftPlan{&ftPlanSH, &ftPlanConst, &ftPlanBoth} {
		for _, useQR := range []bool{false, true} {
			for _, tf := range [][2]string{{"", ""}, {cap18, cap18}} {
				// Binary search the longest admissible text. Word-free, so the
				// wrap fills every column and the line count is maximal.
				lo, hi, best := 1, 4000, 0
				for lo <= hi {
					mid := (lo + hi) / 2
					f := ftEvaluate(P, plan, strings.Repeat("a", mid), tf[0], tf[1], useQR, 0)
					if f.ok && f.err == nil {
						best, lo = mid, mid+1
					} else {
						hi = mid - 1
					}
				}
				if best == 0 {
					t.Fatalf("no admissible text at all for plan %s qr=%v", plan.Name(), useQR)
				}
				f := ftEvaluate(P, plan, strings.Repeat("a", best), tf[0], tf[1], useQR, 0)
				out = append(out, f)
			}
		}
	}
	return out
}

// ftWidestSettingsNote is the longest suffix ftSettingsNote can produce, over
// every speed rung and pass rung the settings screen offers.
//
// Chosen by MEASURED WIDTH and not by string length, because every speed rung
// formats to the same character count ("8.0mm/s" .. "1.0mm/s") -- which of them
// is widest is a glyph-width question about the digits, not a counting one. It
// is derived from the rung tables rather than written out, so a rung added
// later widens the probe instead of silently escaping it.
func ftWidestSettingsNote(ctx *Context) string {
	P := ctx.Platform.EngraverParams()
	widest, w := "", -1
	for _, s := range ftSpeedRungs {
		for _, p := range ftPassRungs {
			n := ftSettingsNote(P, s, p)
			// ftConfirmSummary draws the note in ctx.Styles.subtitle, appended
			// to the summary line; measure it in the same style.
			if sz := ctx.Styles.subtitle.Measure(math.MaxInt, "%s", n); sz.X > w {
				widest, w = n, sz.X
			}
		}
	}
	return widest
}

// TestFTConfirmAlwaysFitsThePanel is M6, the defect the text-only safety test
// could not see: the confirm screen must show the size line and all three
// warnings for EVERY composition the flow will admit, on the panel the machine
// actually has.
//
// Measured as RECTANGLES against the same budget the layout spends, because it
// cannot be done from ExtractText -- that collects runes from every drawn text
// op regardless of where they landed, so an overflowing screen reads as fully
// present. Before the fix the worst case measured 637px (no QR) and 701px (with
// one) against 270px of area: the size line and every warning were off-panel,
// and the codebase's only scroller is bound to buttons SeedHammer II does not
// have.
//
// Every PAGE is measured, not just the first: a pager that fits page 0 and
// overflows page 3 is the same defect one tap later.
func TestFTConfirmAlwaysFitsThePanel(t *testing.T) {
	p := newPlatform()
	p.display = sh2DisplaySize
	ctx := NewContext(p)
	dims := ctx.Platform.DisplaySize()
	if dims != sh2DisplaySize {
		t.Fatalf("the fit test is running at %v, not the real %v panel", dims, sh2DisplaySize)
	}
	area := ppConfirmArea(dims)
	th := &descriptorTheme
	// The note is APPENDED to the summary line, so it is part of the budget
	// this test exists to guard, and passing "" measured a screen the flow can
	// no longer produce now that the gear can put a speed and a pass count
	// there. Both cases are kept: the noteless screen is not superseded, it is
	// what every ordinary plate still shows.
	//
	// What this actually discriminates, measured rather than assumed. A longer
	// note does NOT overflow the panel by itself -- it grows the summary, the
	// summary takes its room out of the preview's budget, and the preview pages
	// one row sooner. That is correct behaviour, not a defect, and it is why
	// doubling and tripling today's note both still pass. The failure it does
	// catch is the note growing until the FIXED part -- summary plus warnings,
	// the block that cannot be paged away -- no longer fits: at 4x today's note
	// the worst case needs 281px of the 270px area and this test fails.
	//
	// Worst page today: 267px of 270 with the widest note, 269px without it
	// (noteless is tighter, since it spends the slack on another preview row).
	notes := []string{"", ftWidestSettingsNote(ctx)}
	if notes[1] == "" {
		t.Fatal("no non-default settings note exists; the probe would measure the empty case twice")
	}
	worst := ftWorstCompositions(t, ctx.Platform.EngraverParams())
	if len(worst) != 12 {
		t.Fatalf("expected 12 worst cases, got %d", len(worst))
	}
	cap18 := strings.Repeat("W", backup.MaxTitleLen)
	sawPaging := false
	for i, f := range worst {
		for _, plan := range []*ftPlan{&ftPlanSH, &ftPlanConst, &ftPlanBoth} {
			for _, tf := range [][2]string{{"", ""}, {cap18, cap18}} {
				for _, note := range notes {
					useQR := f.plate.QR != nil
					start, guard := 0, 0
					for {
						v := ftConfirmBody(ctx, th, area.Dx(), area.Dy(), start, f, plan, tf[0], tf[1], note)
						if v.Size.Y > area.Dy() {
							t.Fatalf("case %d (%d lines, qr=%v, face=%s, title=%q, note=%q) page from row %d needs %dpx "+
								"of a %dpx area: the size line and the warnings are off the %v panel",
								i, len(f.plate.Lines), useQR, plan.Name(), tf[0], note, start, v.Size.Y, area.Dy(), dims)
						}
						if v.Size.X > area.Dx() {
							t.Fatalf("case %d page from row %d is %dpx wide in a %dpx area", i, start, v.Size.X, area.Dx())
						}
						if v.Shown < 1 && v.Total > 0 {
							t.Fatalf("case %d page from row %d drew no rows: the pager cannot advance", i, start)
						}
						if v.Shown < v.Total {
							sawPaging = true
						}
						guard++
						if guard > 200 {
							t.Fatalf("case %d never finished paging", i)
						}
						start += v.Shown
						if start >= v.Total {
							break
						}
					}
				}
			}
		}
	}
	// The premise: at least one of these compositions really does need paging,
	// or the loop above proved nothing about the paged path.
	if !sawPaging {
		t.Error("no worst-case composition needed a second page; this test cannot see a pager defect")
	}
}

// TestFTConfirmReservesRoomForTheWarnings is the non-vacuity check under
// TestFTConfirmAlwaysFitsThePanel: a body that drew NO plate rows would fit the
// panel trivially. The preview budget has to be at least one row in the
// tightest case the flow can produce -- a QR (which adds a third warning) and a
// title and footer (which add two subtitle rows to the preview).
func TestFTConfirmReservesRoomForTheWarnings(t *testing.T) {
	p := newPlatform()
	p.display = sh2DisplaySize
	ctx := NewContext(p)
	area := ppConfirmArea(ctx.Platform.DisplaySize())
	th := &descriptorTheme
	cap18 := strings.Repeat("W", backup.MaxTitleLen)
	f := ftFit{plate: backup.Fitted{SizeMM: 3.0, QR: &qrpkg.Code{Size: 73}}}
	for i := 0; i < 24; i++ {
		f.plate.Lines = append(f.plate.Lines, strings.Repeat("W", 44))
	}
	v := ftConfirmBody(ctx, th, area.Dx(), area.Dy(), 0, f, &ftPlanConst, cap18, cap18, "")
	if v.Total != 26 {
		t.Fatalf("worst case is %d rows, want 26 (title + 24 lines + footer)", v.Total)
	}
	if v.Shown < 1 {
		t.Fatal("the tightest composition leaves no room for a single plate row")
	}
	// And the warnings themselves are what is being reserved for: the summary
	// must be the taller part of the budget, or nothing was actually reserved.
	_, sum := ftConfirmSummary(ctx, th, area.Dx(), f, &ftPlanConst, ftConfirmPager(0, 1, 26), "")
	if sum.Y <= 0 {
		t.Fatal("the summary block measures nothing; the reservation is vacuous")
	}
	if v.Size.Y > area.Dy() {
		t.Errorf("the tightest composition needs %dpx of a %dpx area", v.Size.Y, area.Dy())
	}
	t.Logf("tightest case: summary %dpx, %d of %d rows shown, total %dpx of %dpx",
		sum.Y, v.Shown, v.Total, v.Size.Y, area.Dy())
}

// TestFTConfirmPagesEveryRowExactlyOnce: the pager advances by what was DRAWN,
// so walking it must visit every plate row once, in order, with no gap and no
// repeat. A fixed-page pager over variable-height rows silently skips rows,
// which on this screen means approving lines that were never displayed.
func TestFTConfirmPagesEveryRowExactlyOnce(t *testing.T) {
	p := newPlatform()
	p.display = sh2DisplaySize
	ctx := NewContext(p)
	area := ppConfirmArea(ctx.Platform.DisplaySize())
	th := &descriptorTheme
	cap18 := strings.Repeat("W", backup.MaxTitleLen)
	f := ftFit{plate: backup.Fitted{SizeMM: 3.0}}
	for i := 0; i < 24; i++ {
		f.plate.Lines = append(f.plate.Lines, strings.Repeat("W", 44))
	}
	rows := ftConfirmRows(f, cap18, cap18)
	seen := 0
	start := 0
	pages := 0
	for {
		v := ftConfirmBody(ctx, th, area.Dx(), area.Dy(), start, f, &ftPlanSH, cap18, cap18, "")
		if v.Total != len(rows) {
			t.Fatalf("page %d reports %d rows, want %d", pages, v.Total, len(rows))
		}
		seen += v.Shown
		pages++
		start += v.Shown
		if start >= v.Total {
			break
		}
		if pages > 100 {
			t.Fatal("the pager never reached the end")
		}
	}
	if seen != len(rows) {
		t.Errorf("walking the pager showed %d rows of %d", seen, len(rows))
	}
	if pages < 2 {
		t.Fatalf("the worst case fit on %d page(s); this test cannot see a paging defect", pages)
	}
	t.Logf("%d rows over %d pages", len(rows), pages)
}

// TestFTQRChoiceLabelsBindToMeaning pins the QR screen's LABELS to the boolean
// they produce. Every other test in this file selects the QR choice by index,
// using the same index convention the flow itself uses, so nothing asserted the
// label<->semantics binding in either direction.
//
// Proven necessary by mutation: swapping only the two strings, leaving
// `sel == 1` alone, left the whole gui suite green. Under that mutation an
// operator who taps the displayed "Add QR" gets NO QR, and one who taps "No QR"
// gets a plate carrying a machine-readable copy of their text -- the
// approved-vs-engraved inversion this feature's review gate exists to catch, on
// the one field spec 9 calls out as a privacy hazard.
//
// This also pins spec 9's "default off, opt-in only": the initial selection is
// index 0, so if the order were reversed, OK-without-moving would opt IN.
func TestFTQRChoiceLabelsBindToMeaning(t *testing.T) {
	for _, tc := range []struct {
		sel   int
		label string
		want  bool
	}{
		{0, "No QR", false},
		{1, "Add QR", true},
	} {
		h := newPPHarness(t)
		var got, ok bool
		// The composition the first pass through this step actually sees: an
		// empty field in the default face plan, which states no rungs.
		blocks := ftPlanSH.Blocks("")
		h.start(func() { got, ok = ftQRChoiceFlow(h.ctx, &descriptorTheme, false, blocks) })
		h.mustReach("QRCode")
		cs, isCS := h.widget("qr").(*ChoiceScreen)
		if !isCS {
			t.Fatal(`widget "qr" is not a *ChoiceScreen`)
		}
		// The default must be the no-QR option, and it must be first.
		if cs.choice != 0 {
			t.Errorf("the QR screen opens on index %d; it must default to no QR", cs.choice)
		}
		if cs.Choices[0] != "No QR" {
			t.Errorf("choice 0 is %q, want %q -- OK-without-moving must decline",
				cs.Choices[0], "No QR")
		}
		if cs.Choices[tc.sel] != tc.label {
			t.Fatalf("choice %d is labelled %q, want %q", tc.sel, cs.Choices[tc.sel], tc.label)
		}
		ftChoose(h, "qr", tc.sel)
		for i := 0; i < 8 && !ok; i++ {
			h.frame()
		}
		if !ok {
			t.Fatalf("the QR screen never returned for %q", tc.label)
		}
		if got != tc.want {
			t.Errorf("tapping the button labelled %q produced useQR=%v, want %v -- "+
				"the operator gets the opposite of what they read", tc.label, got, tc.want)
		}
	}
}

// TestFTTitleAndFooterRejectNewlines guards the single-line rule. Untested
// before: disabling the guard left the gui suite green.
//
// engrave.String treats '\n' as a line break, so a two-line title engraves its
// tail onto the body's first row -- and StringCmd.Measure returns only the LAST
// segment's advance, so the centering is computed from the wrong width.
func TestFTTitleAndFooterRejectNewlines(t *testing.T) {
	for _, what := range []string{"Title", "Footer"} {
		h := newPPHarness(t)
		var out string
		var ok bool
		h.start(func() { out, ok = ftLineEntryFlow(h.ctx, &descriptorTheme, what, "") })
		h.mustReach(what)
		kbd, isKbd := h.widget("kbd").(*PassphraseKeyboard)
		if !isKbd {
			t.Fatal(`widget "kbd" is not a *PassphraseKeyboard`)
		}
		kbd.Fragment = "two\nlines"
		h.next("after entering a newline in the %s", what)
		h.tapWidget("ok")
		if !h.pump(8, "single line") {
			t.Errorf("%s accepted a newline; got %q (out=%q ok=%v)", what, h.content, out, ok)
		}
	}
}

// ---- face plans -------------------------------------------------------------

// TestPlansAreWellFormed: every plan the program ships must be usable. A
// non-final run that covers no blocks is a plan whose second face never
// appears; a plan with no runs at all has no face to cut in.
func TestPlansAreWellFormed(t *testing.T) {
	plans := map[string]*ftPlan{
		"sh": &ftPlanSH, "constant": &ftPlanConst, "both": &ftPlanBoth,
		"sizeproof-front": &ftPlanSizeFront, "sizeproof-back": &ftPlanSizeBack,
	}
	for name, plan := range plans {
		if len(plan.Runs) == 0 {
			t.Errorf("%s: the plan has no runs", name)
			continue
		}
		for i, r := range plan.Runs {
			if r.Face.Face == nil {
				t.Errorf("%s: run %d has no face", name, i)
			}
			if r.Face.Name == "" {
				t.Errorf("%s: run %d has no name; the confirm screen would print nothing", name, i)
			}
			if i < len(plan.Runs)-1 && r.Blocks < 1 {
				t.Errorf("%s: run %d covers %d blocks, so the face after it would take the whole plate",
					name, i, r.Blocks)
			}
		}
		// Distinct faces, or the plan is a single-face plate wearing two names.
		for i := 1; i < len(plan.Runs); i++ {
			if plan.Runs[i].Face == plan.Runs[i-1].Face {
				t.Errorf("%s: runs %d and %d are the same face", name, i-1, i)
			}
		}
		if plan.Name() == "" {
			t.Errorf("%s: the plan has no name", name)
		}
	}
	if ftPlanSH.Name() != "sh" || ftPlanConst.Name() != "constant" {
		t.Errorf("a single-face plan no longer names itself after its face: %q / %q",
			ftPlanSH.Name(), ftPlanConst.Name())
	}
	if len(ftPlanBoth.Runs) != 2 {
		t.Errorf("the mixed plan has %d runs, want 2", len(ftPlanBoth.Runs))
	}
}

// TestPlanBlocksAreLosslessAndTotal: Blocks splits the field's text on '\n' and
// nothing else. Rejoining the blocks must give back exactly what was typed --
// a free-text plate engraves what was entered, so a split that consumed,
// doubled or substituted a character would put something else on the steel and
// something else again in the QR.
//
// And it is TOTAL: a text with fewer blocks than the plan expects collapses to
// ONE block in the first run's face rather than producing an empty one, which
// would engrave a blank row nobody asked for.
func TestPlanBlocksAreLosslessAndTotal(t *testing.T) {
	texts := []string{
		"", "one", "one\ntwo", "one\ntwo\nthree", "a\nb\nc\nd\ne\nf\ng\nh",
		"\nleading", "trailing\n", "double\n\nblank",
		"a\nb\nc\nd\ne\n", // enough blocks to split AND a trailing newline
		ftProofTextBoth,
	}
	for _, plan := range []*ftPlan{&ftPlanSH, &ftPlanConst, &ftPlanBoth} {
		for _, text := range texts {
			blocks := plan.Blocks(text)
			if len(blocks) == 0 {
				t.Fatalf("%s: %.20q produced no blocks", plan.Name(), text)
			}
			if got := backup.CompositionText(blocks); got != text {
				t.Errorf("%s: %.20q rejoins as %.20q", plan.Name(), text, got)
			}
			for i, b := range blocks {
				if b.Face == nil {
					t.Errorf("%s: block %d has no face", plan.Name(), i)
				}
				// An empty block is a defect -- it engraves a blank row nobody
				// asked for -- EXCEPT as the last block of a text that ends in
				// '\n', where the empty final line is real and the single-face
				// wrap engraves it too. See
				// TestBlockSplittingDoesNotChangeTheLayoutWithinAFace.
				trailing := i == len(blocks)-1 && strings.HasSuffix(text, "\n")
				if b.Text == "" && len(blocks) > 1 && !trailing {
					t.Errorf("%s: %.20q produced an empty block %d, which engraves a blank row",
						plan.Name(), text, i)
				}
			}
			// A single-face plan is never split at all: it is the untouched
			// path, and it must stay one block whatever the text contains.
			if len(plan.Runs) == 1 && len(blocks) != 1 {
				t.Errorf("%s: a single-face plan split %.20q into %d blocks", plan.Name(), text, len(blocks))
			}
		}
	}
	// The specific collapse: too few blocks for the mixed plan means all of it
	// in the FIRST face, in one block.
	short := "only\ntwo"
	if n := strings.Count(short, "\n") + 1; n >= ftProofBothSplit {
		t.Fatalf("this case needs fewer than %d blocks; it has %d", ftProofBothSplit, n)
	}
	got := ftPlanBoth.Blocks(short)
	if len(got) != 1 || got[0].Face != ftFaceSH.Face || got[0].Text != short {
		t.Errorf("an edited-down mixed text became %d blocks, want one in font/sh holding all of it", len(got))
	}
	// And the full pattern DOES split, in the declared order.
	full := ftPlanBoth.Blocks(ftProofTextBoth)
	if len(full) != 2 {
		t.Fatalf("the mixed pattern split into %d blocks, want 2", len(full))
	}
	if full[0].Face != ftFaceSH.Face || full[1].Face != ftFaceConst.Face {
		t.Error("the mixed pattern's halves are in the wrong faces, or in the wrong order")
	}
	if full[0].Text != ftProofBothSH || full[1].Text != ftProofBothConst {
		t.Error("the split does not fall between the two authored halves")
	}
}

// TestFaceSummaryReportsTheMeasuredRuns: the confirm screen's "font:" field is
// read from the FITTED face map, so it says what the plate will be rather than
// what was asked for. A mixed plate that collapsed to one face, or whose halves
// came out swapped, must read differently on the screen the operator approves.
func TestFaceSummaryReportsTheMeasuredRuns(t *testing.T) {
	sh, cn := ftFaceSH.Face, ftFaceConst.Face
	mixed := []*vector.Face{sh, sh, sh, cn, cn}
	base := ftFaceSummary(&ftPlanBoth, mixed, nil)
	for _, tc := range []struct {
		name  string
		faces []*vector.Face
	}{
		{"all in the first face", []*vector.Face{sh, sh, sh, sh, sh}},
		{"all in the last face", []*vector.Face{cn, cn, cn, cn, cn}},
		{"the halves swapped", []*vector.Face{cn, cn, cn, sh, sh}},
		{"the boundary moved one row", []*vector.Face{sh, sh, cn, cn, cn}},
	} {
		if got := ftFaceSummary(&ftPlanBoth, tc.faces, nil); got == base {
			t.Errorf("%s reads as %q, the same as the correct plate", tc.name, got)
		}
	}
	if !strings.Contains(base, ftFaceSH.Name) || !strings.Contains(base, ftFaceConst.Name) {
		t.Errorf("the mixed summary %q does not name both faces", base)
	}
	// A single-face plan is untouched: the bare face name, as it always was.
	if got := ftFaceSummary(&ftPlanSH, mixed, nil); got != ftFaceSH.Name {
		t.Errorf("a single-face plan now summarises as %q, want %q", got, ftFaceSH.Name)
	}
}
