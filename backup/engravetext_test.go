package backup

import (
	"math"
	"slices"
	"strings"
	"testing"

	"seedhammer.com/bezier"
	"seedhammer.com/bspline"
	"seedhammer.com/engrave"
	"seedhammer.com/font/sh"
)

// The two golden inputs of TestText that carry text. Duplicated deliberately:
// TestText owns them as function-local constants and must not be touched, and
// the standing guard that they are still the strings being engraved is the
// goldens themselves.
const (
	goldenURText   = "UR:CRYPTO-OUTPUT/1-2/LPADAOCFADFXCYDAPRLRMSHDOETAADMHTAADMETAADMSOEADAOAOLSTAADDLOXAXHDCLAOLBAOTTVYCXLRCXFLATSAKBMUVWLUOTOSRDOTRSHYZMJNADIELPTBCSPMAOFZPABNAAHDCXHTRDDAOYRYSGUYHLIDHGDMAAGEKIRFRTJZLOFSSRONUYIOJTKOMKTLSBCMIALBTIAMTAADDYOEADLOCSDYYKAEYKAEYKADYKAOCYCFWYAAPAAYCYWYAYDRTBTAADDLOXAXHDCLAXSKURKKMDRFRNIYSFLRDAAYJOOXCKKNEESNEETEHSOYMYECENKGRHJYMYJPINCPAOAAHDCXUTWNSFKGIHUY"
	goldenMultisig = "wsh(sortedmulti(2,[dc567276/48h/0h/0h/2h]xpub6DiYrfRwNnjeX4vHsWMajJVFKrbEEnu8gAW9vDuQzgTWEsEHE16sGWeXXUV1LBWQE1yCTmeprSNcqZ3W74hqVdgDbtYHUv3eM4W2TEUhpan/<0;1>/1/*,[f245ae38/48h/0h/0h/2h]xpub6DnT4E1fT8VxuAZW29avMjr5i99aYTHBp9d7fiLnpL5t4JEprQqPMbTw7k7rh5tZZ2F5g8PJpssqrZoebzBChaiJrmEvWwUTEMAbHsY39Ge/<0;1>/0h/*,[c5d87297/48h/0h/0h/2h]xpub6DjrnfAyuonMaboEb3ZQZzhQ2ZEgaKV2r64BFmqymZqJqviLTe1JzMr2X2RfQF892RH7MyYUbcy77R7pPu1P71xoj8cDUMNhAMGYzKR4noZ/<0;1>/*h))#qjs07xve"
)

// TestDescriptorTextHasNoSpacesOrNewlines: the refactor of EngraveText onto
// WrapText is only safe because descriptor text contains neither. A space would
// change the greedy fill, and a '\n' would split into blocks under spec 5.2
// rule 1, whereas today engrave.String handles '\n' inside a sliced chunk.
func TestDescriptorTextHasNoSpacesOrNewlines(t *testing.T) {
	for _, s := range []string{goldenURText, goldenMultisig} {
		if i := strings.IndexAny(s, " \n"); i >= 0 {
			t.Errorf("golden input contains %q at index %d; the wrap refactor is not equivalent for it", s[i], i)
		}
	}
}

// oldSliceLines is EngraveText's pre-refactor line producer, kept as a
// reference so the equivalence claim can be tested rather than argued.
func oldSliceLines(txt string, widthAt func(int) int) []string {
	var lines []string
	for lineno := 0; len(txt) > 0; lineno++ {
		n := widthAt(lineno)
		if n < 1 {
			n = 1
		}
		if l := len(txt); n > l {
			n = l
		}
		lines = append(lines, txt[:n])
		txt = txt[n:]
	}
	return lines
}

// TestWrapEqualsCharacterSliceWithoutSpaces is the reason the text-* goldens
// survive: on text with no spaces and no newlines, greedy fill plus
// break-at-exactly-widthAt(i) IS the txt[:n] slice, for every width schedule.
func TestWrapEqualsCharacterSliceWithoutSpaces(t *testing.T) {
	schedules := []func(int) int{
		func(int) int { return 34 },
		func(i int) int { return 3 + i%9 },
		// The real shape: narrow on the screw-hole rows, wide between.
		func(i int) int {
			if i < 2 || i > 17 {
				return 26
			}
			return 34
		},
		func(int) int { return 1 },
	}
	inputs := []string{goldenURText, goldenMultisig, "x", strings.Repeat("Z", 999)}
	for si, widthAt := range schedules {
		for ii, in := range inputs {
			want := oldSliceLines(in, widthAt)
			got, ok := WrapText(in, widthAt, math.MaxInt)
			if !ok {
				t.Fatalf("schedule %d input %d: WrapText refused on an unbounded call", si, ii)
			}
			if !slices.Equal(got, want) {
				t.Errorf("schedule %d input %d: wrap and slice disagree\n got %d lines %q\nwant %d lines %q",
					si, ii, len(got), first(got), len(want), first(want))
			}
		}
	}
}

func first(s []string) string {
	if len(s) == 0 {
		return "<none>"
	}
	return s[0]
}

// inkBounds is where the tool actually CUTS, via bspline.Measure, which unions
// only engraved segments. A bounding box over every knot would include the
// travel moves and answer a different question.
func inkBounds(t testing.TB, p engrave.Params, e engrave.Engraving) bspline.Bounds {
	t.Helper()
	b := bspline.Measure(engrave.PlanEngraving(p.StepperConfig, e)).Bounds
	if b.Empty() {
		t.Fatal("engraving cut nothing")
	}
	return b
}

// TestQROnlyParagraphCentersQR guards spec 5.2's empty-block rule from leaking
// into the descriptor path. WrapText("") returns ONE empty line, so keying the
// centering test to len(lines) instead of to the original text would displace
// the QR-ONLY plate -- measured, by (6.450, 2.300)mm at production stroke, with
// 45281/45282 knot mismatches against text-2-shards-1.bin.
func TestQROnlyParagraphCentersQR(t *testing.T) {
	qrc := QR(t, goldenMultisig)
	plate := Text{
		Paragraphs: []Paragraph{{QR: qrc, QRScale: 3}},
		Font:       sh.Font,
	}
	b := inkBounds(t, params, EngraveText(params, plate))
	plate85 := params.F(plateSize)
	// Centered means the left gap equals the right gap, and likewise
	// vertically. Half a stroke of slack, no more.
	slack := params.StrokeWidth / 2
	if d := (b.Min.X + b.Max.X) - plate85; d < -slack || d > slack {
		t.Errorf("QR-only plate is not horizontally centered: x[%d,%d] on a %d plate, off by %.3fmm",
			b.Min.X, b.Max.X, plate85, float64(d)/2/mm)
	}
	if d := (b.Min.Y + b.Max.Y) - plate85; d < -slack || d > slack {
		t.Errorf("QR-only plate is not vertically centered: y[%d,%d] on a %d plate, off by %.3fmm",
			b.Min.Y, b.Max.Y, plate85, float64(d)/2/mm)
	}
}

// TestEmptyTextEngravesZeroLines: both existing callers build a QR-ONLY variant
// with Text: "" (gui/gui.go:443, :1959). Spec 5.2's one-empty-line rule serves
// the free-text plate only; here zero characters must mean zero engraved rows.
func TestEmptyTextEngravesZeroLines(t *testing.T) {
	qrc := QR(t, goldenMultisig)
	withText := Text{
		Paragraphs: []Paragraph{{Text: " ", QR: qrc, QRScale: 3}},
		Font:       sh.Font,
	}
	empty := Text{
		Paragraphs: []Paragraph{{QR: qrc, QRScale: 3}},
		Font:       sh.Font,
	}
	bEmpty := inkBounds(t, params, EngraveText(params, empty))
	bText := inkBounds(t, params, EngraveText(params, withText))
	// A single blank line still reserves a row, so the QR of the one-line
	// variant sits where the text path puts it -- NOT centered. If empty text
	// took the same path, these two would agree.
	if bEmpty == bText {
		t.Error("empty text and one blank line produced the same layout; the len(p.Text)==0 branch is not being taken")
	}
}

// TestLineLayoutClampsBudgetToOne is a test the plan expected the goldens to
// provide and they do not: measured, changing the n < 1 clamp to n < 2, or
// removing it, leaves all three text-* goldens byte-identical, because no
// descriptor plate carries a QR wide enough to exhaust a line's budget. The
// clamp still matters -- without it the budget goes non-positive and WrapText's
// widthAt >= 1 assertion turns a cramped plate into a panic.
func TestLineLayoutClampsBudgetToOne(t *testing.T) {
	// A layout whose QR leaves less room than the screw-hole band takes.
	//
	// The band was written as holeLines: 2, qrLines: 19 -- rows 2 through 20
	// counted off this layout's own baseY. It is the SAME band expressed in
	// absolute y, with those two numbers still on the page, because the rows
	// this test names (18 is both a QR row and a bottom-band row; 5 and 0 are
	// neither) are what every assertion below turns on.
	const (
		llBaseY     = 19200
		llFontSize  = 24320
		llHoleLines = 2
		llQRLines   = 19
	)
	lay := lineLayout{
		charPerLine:   34,
		charPerQRLine: 2,
		holeChars:     4,
		qrTop:         llBaseY + llHoleLines*llFontSize,
		qrBottom:      llBaseY + (llHoleLines+llQRLines)*llFontSize,
		charWidth:     14520,
		fontSize:      llFontSize,
		baseY:         llBaseY,
		plateHeight:   544000,
		innerMargin:   64000,
	}
	// Line 18 is both a QR line and a bottom-band line: 2 - 4 = -2.
	n, offx := lay.at(18)
	if n != 1 {
		t.Errorf("lay.at(18) budget = %d, want 1 (clamped)", n)
	}
	if want := lay.holeChars * lay.charWidth; offx != want {
		t.Errorf("lay.at(18) offx = %d, want %d", offx, want)
	}
	// A line that legitimately allows exactly one character must not be
	// rounded up to two: that would push a character into the screw-hole band.
	lay2 := lay
	lay2.charPerQRLine = 5
	if n, _ := lay2.at(18); n != 1 {
		t.Errorf("lay.at(18) with a 5-char QR budget = %d, want exactly 1", n)
	}
	// And an unobstructed line is untouched by the clamp: no QR, no band.
	lay3 := lay
	lay3.qrTop, lay3.qrBottom, lay3.charPerQRLine = 0, 0, 0
	if n, offx := lay3.at(5); n != 34 || offx != 0 {
		t.Errorf("unobstructed line = (%d, %d), want (34, 0)", n, offx)
	}
	// Both bands are deducted on an unobstructed-width line inside a band.
	if n, offx := lay3.at(0); n != 34-2*lay.holeChars || offx != lay.holeChars*lay.charWidth {
		t.Errorf("top-band line = (%d, %d), want (%d, %d)", n, offx, 34-2*lay.holeChars, lay.holeChars*lay.charWidth)
	}
}

// TestEngraveTextSurvivesAQRWiderThanTheLine drives the clamp through the real
// entry point: without it this panics inside WrapText rather than engraving.
func TestEngraveTextSurvivesAQRWiderThanTheLine(t *testing.T) {
	// Mixed case forces byte mode; 400 alphanumeric characters would only be
	// 61 modules, which is not wide enough to exhaust a line budget.
	qrc := QR(t, strings.Repeat("Aa", 200))
	if qrc.Size < 66 {
		t.Fatalf("QR is %d modules; this test needs >= 66 to exhaust a line budget", qrc.Size)
	}
	plate := Text{
		Paragraphs: []Paragraph{{Text: strings.Repeat("x", 200), QR: qrc, QRScale: 3}},
		Font:       sh.Font,
	}
	b := inkBounds(t, params, EngraveText(params, plate))
	if b.Empty() {
		t.Fatal("nothing engraved")
	}
}

// TestEmptyParagraphAdvancesNoRow: EngraveText must not route empty text
// through WrapText, whose spec 5.2 empty-block rule would return one empty
// line. Nothing inks either way, so only a following paragraph can see the
// difference -- which is exactly why it needs its own test rather than a
// golden.
func TestEmptyParagraphAdvancesNoRow(t *testing.T) {
	after := func(firstText string) int {
		plate := Text{
			Paragraphs: []Paragraph{{Text: firstText}, {Text: "X"}},
			Font:       sh.Font,
		}
		return inkBounds(t, params, EngraveText(params, plate)).Min.Y
	}
	empty, oneBlank := after(""), after(" ")
	if empty == oneBlank {
		t.Errorf("an empty paragraph and a one-blank-line paragraph put the next paragraph at the same y (%d); empty text is being wrapped", empty)
	}
	if fontSize := params.F(plateFontSizeUR); oneBlank-empty != fontSize {
		t.Errorf("a blank line advanced the next paragraph by %d, want one full row of %d", oneBlank-empty, fontSize)
	}
}

// ─── S6b GATE 1.2b: Title is plate row 0, Footer the last plate row ─────────

// textRowBand is rowBand's Text-mechanism twin (freetext_test.go): Text has
// ONE fixed size, plateFontSizeUR, not a ladder, so unlike the free-text
// plate's rowBand this takes no size parameter.
func textRowBand(row int) (top, bottom int) {
	y := prodParams.I(outerMargin) + row*prodParams.F(plateFontSizeUR)
	return y, y + prodParams.F(plateFontSizeUR)
}

// TestTextTitleFooterAreAbsoluteRows pins spec 1.1.2/1.2b: the title is plate
// row 0, the footer is plate row LinesPerPlate-1 -- absolute, not "after the
// text" -- and the body drops exactly one row when, and only when, a title is
// present. The mk1/md1 twin of freetext_test.go's TestFreeTextRowsAreAbsolute,
// for the OTHER title/footer mechanism this cycle adds (backup.Text, not
// Fitted).
func TestTextTitleFooterAreAbsoluteRows(t *testing.T) {
	rows := LinesPerPlate(prodParams, plateFontSizeUR)

	title := inkBounds(t, prodParams, EngraveText(prodParams, Text{Title: "TITLE", Font: sh.Font}))
	top, bottom := textRowBand(0)
	if title.Min.Y < top || title.Max.Y > bottom {
		t.Errorf("title inks y[%d,%d], outside row 0's band [%d,%d]", title.Min.Y, title.Max.Y, top, bottom)
	}

	footer := inkBounds(t, prodParams, EngraveText(prodParams, Text{Footer: "FOOTER", Font: sh.Font}))
	top, bottom = textRowBand(rows - 1)
	if footer.Min.Y < top || footer.Max.Y > bottom {
		t.Errorf("footer inks y[%d,%d], outside row %d's band [%d,%d]", footer.Min.Y, footer.Max.Y, rows-1, top, bottom)
	}

	// The body drops a row when -- and only when -- a title is present.
	noTitle := inkBounds(t, prodParams, EngraveText(prodParams, Text{Paragraphs: []Paragraph{{Text: "X"}}, Font: sh.Font}))
	withTitle := inkBounds(t, prodParams, EngraveText(prodParams, Text{Title: "T", Paragraphs: []Paragraph{{Text: "X"}}, Font: sh.Font}))
	if got, want := withTitle.Max.Y-noTitle.Max.Y, prodParams.F(plateFontSizeUR); got != want {
		t.Errorf("a title moved the body by %d, want exactly one row of %d", got, want)
	}
	// A footer must not move the body at all: its row is absolute, anchored
	// from the plate's bottom rather than "after the text".
	withFooter := inkBounds(t, prodParams, EngraveText(prodParams, Text{Footer: "F", Paragraphs: []Paragraph{{Text: "X"}}, Font: sh.Font}))
	if withFooter.Min.Y != noTitle.Min.Y {
		t.Errorf("a footer moved the body's top from %d to %d", noTitle.Min.Y, withFooter.Min.Y)
	}
}

// ─── S6b GATE 1.2a: the Title/Footer budget, layout-based ───────────────────

// TestTextTitleFooterBudget pins the LAYOUT-BASED budget for backup.Text's
// Title/Footer at plateFontSizeUR, driven through the REAL layout
// (textLayout, via EngraveText) rather than raw string width. The mk1/md1
// twin of freetext_test.go's TestTitleCapFitsAtEveryRung, but for backup.Text's
// ONE rung (plateFontSizeUR is fixed, not a ladder).
//
// THE MEASURED BUDGET IS 28, NOT SPIKE_s6b_q2_results.md §3c's 25. The spike
// computed 25 from raw string width against the inset span and flagged its own
// method caveat -- "raw width UNDER-reports... 25 at 3.8mm is conservative
// rather than optimistic... the implementation's gate must be the
// layout-based form" -- and that is exactly what happened: bisecting through
// EngraveText (not raw width) finds 28 'W's fit and 29 do not. This is GATE
// 1.2a working as specified, not a defect: every string this cycle
// introduces (<=18 chars) clears either number with room to spare, so the
// discrepancy changes no pass/fail outcome. Reported as a finding rather than
// silently reconciled with the spike.
//
// ON THE BUDGET, NOT TODAY'S STRINGS (GATE 1.2a's own wording): the
// 'W'-repeat cap pins the (measured) budget itself; the second half checks
// every title/footer this cycle introduces is within it, by length -- exact
// for a fixed-pitch face (fixedCharWidth's doc comment: every font/sh advance
// is equal, so any N-character string inks the same width as N 'W's).
func TestTextTitleFooterBudget(t *testing.T) {
	lo := prodParams.I(innerMargin)
	hi := prodParams.F(plateSize) - prodParams.I(innerMargin)
	const budget = 28 // measured; see doc comment -- SPIKE §3c's 25 was raw-width, not layout-based
	capStr := strings.Repeat("W", budget)
	b := inkBounds(t, prodParams, EngraveText(prodParams, Text{Title: capStr, Footer: capStr, Font: sh.Font}))
	if b.Min.X < lo || b.Max.X > hi {
		t.Errorf("%d-character title/footer inks x[%.3f,%.3f]mm, outside the screw-hole-free span [%.1f,%.1f]mm",
			budget, float64(b.Min.X)/mm, float64(b.Max.X)/mm, float64(lo)/mm, float64(hi)/mm)
	}
	// budget+1 must NOT fit -- pinning the budget rather than merely a safe
	// value.
	over := strings.Repeat("W", budget+1)
	if bOver := inkBounds(t, prodParams, EngraveText(prodParams, Text{Title: over, Font: sh.Font})); bOver.Min.X >= lo && bOver.Max.X <= hi {
		t.Errorf("a %d-character title fits the screw-hole-free span; the %d-character budget has stopped binding", budget+1, budget)
	}
	// Every title/footer S6b introduces (spec 1.2), on the budget:
	for _, s := range []string{
		"PASSWORD REQUIRED",  // 17
		"COMB FP: FC60 C6DF", // 18
		"SEED FP: 73C5 DA0A", // 18
	} {
		if len(s) > budget {
			t.Errorf("%q is %d characters, over the %d-character budget", s, len(s), budget)
		}
	}
}

// GRAFT 4 — THE PLATE'S CLAIM ABOUT ITSELF IS CUT LAST.
//
// > AN UNSIGNED PLATE IS AN UNFINISHED PLATE.
//
// The title row is the plate's claim about itself -- "TX 2DCF2B97 1/2". Cut
// FIRST, a plate abandoned at minute 20 already carries that claim and LOOKS
// FINISHED, so an operator taught to sort by it puts a half-cut plate in the
// good stack. The device has no camera and the operator is the only inspector,
// so a claim that outruns the artifact has nothing downstream to catch it.
//
// THE TEST MUST ASSERT THE ORDER OF EMITTED OPERATIONS, NOT THE IMAGE: a
// FINISHED plate looks identical either way, so every bounds/golden test in
// this file passes under both orders and none of them can see this.
//
// `Engraving` is `iter.Seq[Command]` -- an ordered sequence executed in
// emission order -- and plate POSITION comes from the y offset rather than
// from that order, which is why legend-last is a reordering of yields and not
// a layout change. TestTextTitleFooterAreAbsoluteRows still pins the rows.
func TestTheTitleAndFooterAreEmittedLast(t *testing.T) {
	plate := Text{
		Title:      "TX 2DCF2B97 1/2",
		Footer:     "FOOTERROW",
		Paragraphs: []Paragraph{{Text: "mt1p9h8jqq9qqqqgqqqqqqqyqherdfykhhpey6z2"}},
		Font:       sh.Font,
		FontSize:   3.0,
	}
	fontSize := params.F(plate.fontMM())
	titleBottom := params.I(outerMargin) + fontSize
	footerTop := footerRowY(params, plate.fontMM())

	// Bucket every knot by which ROW it lands in, and record when it was
	// emitted. The three bands do not overlap: measured on this fixture the
	// title occupies y[22133,34961] against a titleBottom of 38400, the body
	// y[41505,75654], and the footer y[501987,513614] against a footerTop of
	// 499200.
	firstClaim, lastBody, nTitle, nFooter, nBody := -1, -1, 0, 0, 0
	i := 0
	for c := range EngraveText(params, plate) {
		k, ok := c.AsKnot()
		if !ok {
			continue
		}
		switch {
		case k.Knot.Y < titleBottom:
			nTitle++
			if firstClaim < 0 || i < firstClaim {
				firstClaim = i
			}
		case k.Knot.Y >= footerTop:
			nFooter++
			if firstClaim < 0 || i < firstClaim {
				firstClaim = i
			}
		default:
			nBody++
			lastBody = i
		}
		i++
	}
	if nTitle == 0 || nFooter == 0 || nBody == 0 {
		t.Fatalf("the fixture must cut all three: title=%d body=%d footer=%d "+
			"(a band that never fires makes this test vacuous)", nTitle, nBody, nFooter)
	}
	if firstClaim < lastBody {
		t.Errorf("the plate's claim about itself is cut at knot %d, before the body "+
			"finishes at %d: a plate abandoned mid-cut would already read as finished",
			firstClaim, lastBody)
	}
}

// TestFooterRowIsWhereTheFooterIsCut is Text.FooterRow's non-vacuity check: the
// number it returns is the row the engraver actually puts a footer on, measured
// off the ink, not off the formula it forwards to.
//
// A one-glyph body and a one-glyph footer are engraved separately, so the
// footer's own ink can be located: the footer plate's ink runs from the body
// glyph's top down to the footer glyph's bottom, and its Max.Y is one font size
// below the row FooterRow names.
func TestFooterRowIsWhereTheFooterIsCut(t *testing.T) {
	plate := Text{Paragraphs: []Paragraph{{Text: "X"}}, Font: sh.Font}
	withFooter := plate
	withFooter.Footer = "F"

	row := plate.FooterRow(prodParams)
	if row != withFooter.FooterRow(prodParams) {
		t.Errorf("FooterRow depends on whether a Footer is set: %d vs %d",
			row, withFooter.FooterRow(prodParams))
	}
	bare := inkBounds(t, prodParams, EngraveText(prodParams, plate))
	footed := inkBounds(t, prodParams, EngraveText(prodParams, withFooter))
	if footed.Max.Y <= bare.Max.Y {
		t.Fatalf("adding a footer did not move the ink down: bare %d, footed %d",
			bare.Max.Y, footed.Max.Y)
	}
	// The footer glyph's ink begins at the row and is at most one font size tall.
	fontSize := prodParams.F(plate.fontMM())
	if footed.Max.Y <= row || footed.Max.Y > row+fontSize {
		t.Errorf("FooterRow says %d but the footer's ink ends at %d, outside [%d, %d]",
			row, footed.Max.Y, row, row+fontSize)
	}
}

// TestAPackedBodyCanCoverTheFooterRow is the REASON FooterRow is exported, and
// it is a demonstration rather than a claim: a plate whose paragraphs run past
// the footer row is still In() the engravable area, so the bounds check every
// paragraph caller uses (gui.toPlate) reports a FIT over ink that lands on top
// of the footer.
//
// If EngraveText ever gains a body budget of its own, this test fails and the
// packer's extra check (gui.bundlePlateTextFits) can go.
func TestAPackedBodyCanCoverTheFooterRow(t *testing.T) {
	const chunk = "md1f9k2szspqjtvyyy4qqxppcgsc97v95zqyudm486mm4xav6hqptc0rd7sr9mfc8yrzcx7sju0ra3jh8llnx"
	var paras []Paragraph
	for i := 0; i < 6; i++ {
		paras = append(paras, Paragraph{Text: chunk})
	}
	plate := Text{Paragraphs: paras, Font: sh.Font, Title: "T", Footer: "F"}
	row := plate.FooterRow(prodParams)

	body := plate
	body.Footer = ""
	bodyInk := inkBounds(t, prodParams, EngraveText(prodParams, body))
	if bodyInk.Max.Y <= row {
		t.Fatalf("six chunks no longer reach the footer row (%d vs %d); pick a "+
			"longer body or this test proves nothing", bodyInk.Max.Y, row)
	}
	// ...and the whole plate is nevertheless inside the engravable area, which is
	// the half that makes it dangerous.
	const safetyMargin = 3 // gui.safetyMargin, mm
	sz := prodParams.F(plateSize)
	margin := prodParams.I(safetyMargin)
	all := inkBounds(t, prodParams, EngraveText(prodParams, plate))
	if !all.In(bspline.Bounds{
		Min: bezier.Pt(margin, margin),
		Max: bezier.Pt(sz-margin, sz-margin),
	}) {
		t.Fatal("the overlapping plate is out of bounds, so toPlate would have " +
			"caught it and FooterRow would not be needed")
	}
}
