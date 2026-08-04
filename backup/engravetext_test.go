package backup

import (
	"math"
	"slices"
	"strings"
	"testing"

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
	lay := lineLayout{
		charPerLine:   34,
		charPerQRLine: 2,
		holeChars:     4,
		holeLines:     2,
		qrLines:       19,
		charWidth:     14520,
		fontSize:      24320,
		baseY:         19200,
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
	lay3.qrLines, lay3.charPerQRLine = 0, 0
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
