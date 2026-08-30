package backup

import (
	"errors"
	"math"
	"strings"
	"testing"

	qrpkg "github.com/seedhammer/kortschak-qr"
	"seedhammer.com/font/constant"
	"seedhammer.com/font/sh"
	"seedhammer.com/font/vector"
)

// TestQRBandAgreesWithTheRowIndexAtEveryRung is spec 7.7(a), and it is the
// whole no-golden-moves argument of this phase written as an assertion.
//
// The band used to be a ROW INDEX counted off the layout's own baseY --
// holeLines <= i < holeLines+qrLines -- and it is now a plate-absolute y range.
// Every existing plate anchors the code at the same y the text starts at, so
// the two must agree on every row of every plate at every rung; where they
// would not, a golden would move. The old predicate is spelled out here rather
// than called, because the point is to reproduce arithmetic that no longer
// exists in the package.
//
// The comparison is not just the predicate but at()'s whole answer, budget and
// inset, since that is what the wrap and the engraver actually read.
func TestQRBandAgreesWithTheRowIndexAtEveryRung(t *testing.T) {
	P := prodParams
	margin, inner := P.I(outerMargin), P.I(innerMargin)
	qrBorder := P.I(2)
	// Codes pinned by MODULE COUNT, and geometry only -- at() reads nothing of
	// a code but its Size. A count is what the band is a function of; a text
	// length is not, because qr.Encode picks its mode from the character set.
	for _, modules := range []int{25, 37, 61, 89, 105} {
		qrc := &qrpkg.Code{Size: modules}
		for _, size := range FontSizes {
			fontSize := P.F(size)
			rows := LinesPerPlate(P, size)
			lay, _ := layAt(size, qrc, freeTextQRScale)

			holeLines := int(math.Ceil(float64(inner-margin) / float64(fontSize)))
			qrsz := modules * P.StrokeWidth * freeTextQRScale
			qrLines := (qrsz + 2*qrBorder + fontSize - 1) / fontSize
			// The narrowed budget, from the code and both its borders. Derived
			// here and not read off lay, because lay is what is under test: a
			// KeepOutX that dropped the border would otherwise agree with
			// itself on every row of every rung.
			width := P.F(plateSize) - 2*margin
			if got, want := lay.charPerQRLine, (width-2*qrBorder-qrsz)/lay.charWidth; got != want {
				t.Errorf("%.1fmm %d modules: charPerQRLine = %d, want %d", size, modules, got, want)
			}
			// at() as it stood before this phase, in full.
			oldAt := func(i int) (n, offx int) {
				n = lay.charPerLine
				isQRLine := holeLines <= i && i < holeLines+qrLines
				if isQRLine {
					n = lay.charPerQRLine
				}
				holeLine := lay.baseY+i*lay.fontSize < lay.innerMargin ||
					lay.baseY+(i+1)*lay.fontSize > lay.plateHeight-lay.innerMargin
				if holeLine {
					if !isQRLine {
						n -= lay.holeChars
					}
					n -= lay.holeChars
					offx = lay.holeChars * lay.charWidth
				}
				if n < 1 {
					n = 1
				}
				return n, offx
			}

			band, plain := 0, 0
			for i := range rows {
				old := holeLines <= i && i < holeLines+qrLines
				y := lay.baseY + i*lay.fontSize
				now := lay.qrTop <= y && y < lay.qrBottom
				if old != now {
					t.Errorf("%.1fmm %d modules row %d: the y band says %v, the row index says %v",
						size, modules, i, now, old)
				}
				if old {
					band++
				} else {
					plain++
				}
				wantN, wantOffx := oldAt(i)
				if gotN, gotOffx := lay.at(i); gotN != wantN || gotOffx != wantOffx {
					t.Errorf("%.1fmm %d modules row %d = (%d chars, inset %d), the row-index layout says (%d, %d)",
						size, modules, i, gotN, gotOffx, wantN, wantOffx)
				}
			}
			// Neither half vacuous. A rung where no row is in the band, or
			// where every row is, agrees for a reason that has nothing to do
			// with the change under test.
			if band == 0 || plain == 0 {
				t.Fatalf("%.1fmm %d modules: %d rows in the band, %d outside it; this rung proves nothing",
					size, modules, band, plain)
			}
		}
	}
	// And a plate with no code has no band: the empty [0, 0) window must not
	// claim a row, which is what keeps every plain plate's golden still.
	for _, size := range FontSizes {
		lay, rows := layAt(size, nil, freeTextQRScale)
		for i := range rows {
			y := lay.baseY + i*lay.fontSize
			if lay.qrTop <= y && y < lay.qrBottom {
				t.Errorf("%.1fmm row %d is a QR row on a plate that carries no code", size, i)
			}
		}
	}
}

// TestTwoBlockQRPlateWrapsBlockTwoAtTheCodesBudget is spec 7.7(b): a TWO-block
// plate carrying a code, where block 2 spans the band.
//
// It is a REGRESSION pin, and it passes for free today -- every block still
// lays out at baseY = outerMargin, so a row index and a plate row are the same
// number and block 2's window is right whatever the layout does. It is written
// now because it is cheap now, and because the phase that gives each block its
// own running baseY is the phase in which reaching for a block-relative row
// index re-creates the measured defect: block 2's rows beside the code wrapped
// at the full width and engraved straight across a machine-readable copy of the
// text. No shipped golden is a multi-block QR plate, so nothing else sees it.
//
// The code is INJECTED into fitBlocksAt. An 89-module code is not producible BY
// a composition that fits at 3.0mm -- the plate holds a few hundred characters
// beside such a code and the code itself needs some 645 bytes -- so the
// exported path cannot pose this plate.
func TestTwoBlockQRPlateWrapsBlockTwoAtTheCodesBudget(t *testing.T) {
	P := prodParams
	const size = 3.0
	// Pinned by MODULE COUNT, never by text length: qr.Encode picks its mode
	// from the character SET, so "700 characters" names a different code the
	// day this fixture's case changes.
	const modules = 89
	qrc := QR(t, strings.Repeat("Aa", 350))
	if qrc.Size != modules {
		t.Fatalf("the fixture text encodes to %d modules, want %d; this plate's band was measured at %d",
			qrc.Size, modules, modules)
	}
	qrp := qrPlaceFor(size, qrc, freeTextQRScale)
	fontSize := P.F(size)
	margin := P.I(outerMargin)
	rows := LinesPerPlate(P, size)
	start, end := bodyRows(rows, "", "")
	bandRow := (qrp.Top - margin) / fontSize

	// Block 1 takes TWO rows and ends ABOVE the band's first row. A block 1 of
	// no rows would start block 2 on the plate's own first row, where a
	// block-relative index and a plate row are the same number and the defect
	// is invisible; a block 1 reaching past the band would leave block 2 with
	// no rows beside the code at all.
	const block1Rows = 2
	if start+block1Rows > bandRow {
		t.Fatalf("block 1 ends at row %d, at or past the band's first row %d",
			start+block1Rows, bandRow)
	}

	// Spaceless runs sized to the LIVE budget of every row they land on, so a
	// wrong budget produces INK rather than merely room for it. Short words
	// would leave the surplus columns empty and the fixture would pass under
	// the very defect it exists for.
	fill := func(c string, fnt *vector.Face, from, to int) string {
		lay := textLayout(P, fnt, fontSize, margin, qrp)
		total := 0
		for r := from; r < to; r++ {
			n, _ := lay.at(r)
			total += n
		}
		return strings.Repeat(c, total)
	}
	blocks := []Block{
		{Face: sh.Font, Text: fill("a", sh.Font, start, start+block1Rows)},
		{Face: constant.Font, Text: fill("b", constant.Font, start+block1Rows, end)},
	}
	f, err := fitBlocksAt(P, blocks, "", "", qrc, size)
	if err != nil {
		t.Fatalf("the fixture composition does not fit at %.1fmm: %v", size, err)
	}
	if f.qrAt == nil {
		t.Fatal("the fit produced no placement for a plate that carries a code")
	}
	if len(f.Lines) != end-start {
		t.Fatalf("the composition wrapped to %d rows, want the whole body's %d; "+
			"the fill is not sized to the budgets it is measured against", len(f.Lines), end-start)
	}
	if f.Faces[bandRow-start] != constant.Font {
		t.Fatalf("the band's first row %d is cut in block 1's face; block 2 must span the code", bandRow)
	}

	// (i) Every row is wrapped at the budget of the PLATE ROW it lands on, in
	// its own face. This is the sharp half: a band shifted by even one row
	// gives some row of block 2 the unobstructed budget instead of the code's.
	layAtRow := func(i int) lineLayout {
		return textLayout(P, f.Faces[i], fontSize, margin, f.qrAt)
	}
	for i, l := range f.Lines {
		row := start + i
		n, _ := layAtRow(i).at(row)
		if len(l) != n {
			t.Errorf("row %d wrapped %d characters; the budget of plate row %d in its own face is %d",
				row, len(l), row, n)
		}
	}

	// (ii) No body ink enters the code box. An intersection with the code's
	// RECTANGLE, [X, X+Size) x [Y, Y+Size) -- never a plate-wide max-x bound:
	// measured, rows below the band legitimately ink the full width, so a
	// max-x assertion fails on a CORRECT plate and the natural repair is to
	// weaken it into something the defect also satisfies.
	bx0, by0 := f.qrAt.X, f.qrAt.Y
	bx1, by1 := bx0+f.qrAt.Size, by0+f.qrAt.Size
	beside, pastLeft := 0, 0
	for i, l := range f.Lines {
		row := start + i
		lay := layAtRow(i)
		_, offx := lay.at(row)
		// The ADVANCE box, which is wider than the ink, so this is the
		// conservative question.
		x0 := margin + offx
		x1 := x0 + len(l)*lay.charWidth
		y0 := margin + row*lay.fontSize
		y1 := y0 + lay.fontSize
		if y1 > by0 && y0 < by1 {
			beside++
		}
		if x1 > bx0 {
			pastLeft++
		}
		if x1 > bx0 && x0 < bx1 && y1 > by0 && y0 < by1 {
			t.Errorf("row %d (%d characters) spans x[%d,%d] y[%d,%d], inside the code's box x[%d,%d] y[%d,%d]",
				row, len(l), x0, x1, y0, y1, bx0, bx1, by0, by1)
		}
	}
	// Non-vacuity, both ways. Some row must run BESIDE the code, or the box
	// assertion is about rows that could never reach it. And some row must ink
	// PAST the code's left edge -- from outside its y range -- which is the
	// measurement that makes the rectangle form necessary rather than fussy.
	if beside == 0 {
		t.Error("no row of this plate lies beside the code; the box assertion tests nothing")
	}
	if pastLeft == 0 {
		t.Error("no row inks past the code's left edge; a plate-wide max-x bound would have " +
			"passed here, so this fixture is not the plate the rectangle form was written for")
	}
}

// TestEngraveFittedDrawsTheQRAtTheStoredPlacement is the free-text half of spec
// 7.7(c). The code's y is the fit's answer, carried on Fitted, and the engraver
// READS it -- it does not derive a second one from SizeMM and the module count,
// which is the derivation that could drift from the band the lines were broken
// around.
func TestEngraveFittedDrawsTheQRAtTheStoredPlacement(t *testing.T) {
	P := prodParams
	const size = 3.8
	f, err := FitBlocksAt(P, []Block{{Face: sh.Font, Text: "a note with a code"}}, "", "", true, size)
	if err != nil {
		t.Fatal(err)
	}
	if f.qrAt == nil {
		t.Fatal("the fit produced no placement for a plate that carries a code")
	}
	// Blank the body, so the ink on this plate IS the code's.
	blank := f
	blank.Lines = make([]string, len(f.Lines))

	b := inkBounds(t, P, EngraveFitted(P, blank))
	if got, want := b.Min.X, f.qrAt.X+P.StrokeWidth/2; got != want {
		t.Errorf("the code inks from x=%d, want %d (half a stroke in from the placement's %d)",
			got, want, f.qrAt.X)
	}
	if got, want := b.Min.Y, f.qrAt.Y+P.StrokeWidth/2; got != want {
		t.Errorf("the code inks from y=%d, want %d (half a stroke in from the placement's %d)",
			got, want, f.qrAt.Y)
	}

	// And it is READ, not recomputed. Displace the stored placement -- leaving
	// Bottom alone, so the band still ends inside the plate and 2.1.1's
	// defensive guard has nothing to say -- and the code must follow. An
	// engraver deriving its own y would draw in the same place as before and
	// never notice.
	moved := *f.qrAt
	moved.X -= P.I(5)
	moved.Y += P.I(4)
	g := blank
	g.qrAt = &moved
	m := inkBounds(t, P, EngraveFitted(P, g))
	if got, want := m.Min.X, moved.X+P.StrokeWidth/2; got != want {
		t.Errorf("with the placement moved to x=%d the code inks from %d, want %d; "+
			"the engraver is not reading qrAt.X", moved.X, got, want)
	}
	if got, want := m.Min.Y, moved.Y+P.StrokeWidth/2; got != want {
		t.Errorf("with the placement moved to y=%d the code inks from %d, want %d; "+
			"the engraver is not reading qrAt.Y", moved.Y, got, want)
	}
}

// TestEngraveTextDrawsTheQRAtItsParagraphsPlacement is the descriptor half of
// spec 7.7(c). That code belongs to a PARAGRAPH and moves with it, so its
// placement is anchored at the paragraph's own offy -- not at the plate margin,
// which is the anchor the free-text plate uses and the one an engraver reaching
// for the nearest constant would pick.
//
// THE DISTINGUISHER IS A TITLE ROW, not a preceding paragraph. This test used to
// put the code on paragraph 2 of 2, which since F-434 is a refused plate
// (ErrMultiParagraphQR) -- so the only arrangement left in which a paragraph
// starts anywhere but the plate margin is a plate whose row 0 is spent on a
// title. The property is the same one and the non-vacuity check below is what
// keeps it honest: an anchor equal to the margin cannot tell the two apart.
func TestEngraveTextDrawsTheQRAtItsParagraphsPlacement(t *testing.T) {
	P := prodParams
	const scale = 2
	qrc := QR(t, "a code under a title row")
	// Short text, so the only ink at the plate's right edge and at its bottom is
	// the code's. The control below proves that rather than assuming it.
	plate := Text{
		Font:  sh.Font,
		Title: "T",
		Paragraphs: []Paragraph{
			{Text: "x", QR: qrc, QRScale: scale},
		},
	}
	fontSize := P.F(plate.fontMM())
	// offy as EngraveText advances it: row 0 is spoken for by the title, so the
	// body -- and the code anchored at its top -- starts on row 1.
	offy := P.I(outerMargin) + fontSize
	if offy == P.I(outerMargin) {
		t.Fatal("the code's paragraph starts at the plate margin; this test cannot tell the two anchors apart")
	}
	want := qrPlaceAt(P, qrc, scale, fontSize, offy)

	b := inkBounds(t, P, mustEngraveText(t, P, plate))
	if got, w := b.Max.X, want.X+want.Size-P.StrokeWidth/2; got != w {
		t.Errorf("the plate inks to x=%d, want the code's right edge %d", got, w)
	}
	if got, w := b.Max.Y, want.Y+want.Size-P.StrokeWidth/2; got != w {
		t.Errorf("the plate inks to y=%d, want the code's bottom edge %d (anchored at the paragraph's %d, "+
			"not at the plate margin %d)", got, w, offy, P.I(outerMargin))
	}

	// The control. With the code gone, nothing on this plate reaches either
	// edge, so the two assertions above are measuring the code and not the
	// title or the text that happen to share the bounds with it.
	plain := plate
	plain.Paragraphs = []Paragraph{{Text: "x"}}
	c := inkBounds(t, P, mustEngraveText(t, P, plain))
	if c.Max.X >= want.X || c.Max.Y >= want.Y {
		t.Fatalf("the text alone inks to (%d, %d), reaching the code's box at (%d, %d); "+
			"the assertions above are not measuring the code", c.Max.X, c.Max.Y, want.X, want.Y)
	}
}

// TestFitBlocksAtRefusesATooTallQRBandAsAnError is spec 7.7(d)'s fitBlocksAt
// half. A band running past the bottom margin is a property of the COMPOSITION,
// so it is refusable before the operator approves anything -- and it must come
// back as an ERROR. EngraveFitted is reached only after the confirm screen, so
// the same refusal arriving as a panic arrives with a plate clamped in the
// machine, which is the failure 2.1.1 exists to avoid.
//
// This fixture deliberately does NOT recover: a panic here IS the failure. A
// test that accepted one would pass against exactly that crash.
//
// (Spec 7.7(d)'s FitSized half is vacuous -- that constructor leaves qrAt nil
// by construction -- and belongs to no phase.)
func TestFitBlocksAtRefusesATooTallQRBandAsAnError(t *testing.T) {
	P := prodParams
	const size = 3.0
	// Pinned by MODULE COUNT and by the band's own bottom. The byte count
	// selects the encoder's MODE, not its version, so it is not the thing this
	// fixture rests on.
	const modules = 113
	qrc := QR(t, strings.Repeat("Aa", 550))
	if qrc.Size != modules {
		t.Fatalf("the fixture text encodes to %d modules, want %d", qrc.Size, modules)
	}
	limit := P.F(plateSize) - P.I(outerMargin)
	if got := qrPlaceFor(size, qrc, freeTextQRScale).Bottom; got <= limit {
		t.Fatalf("the fixture's band ends at %d, inside the %d bottom margin; it tests nothing", got, limit)
	}
	// One character, so the composition WRAPS and ErrTooLarge cannot answer
	// for the bound. The code is injected: 2.1.1 measures this case
	// unreachable from any shipped fit, which is why it has no golden.
	f, err := fitBlocksAt(P, []Block{{Face: sh.Font, Text: "x"}}, "", "", qrc, size)
	if !errors.Is(err, ErrQRTooTall) {
		t.Fatalf("fitBlocksAt returned err = %v, want %v", err, ErrQRTooTall)
	}
	if f.QR != nil || f.qrAt != nil || f.Lines != nil {
		t.Errorf("a refused fit returned a plate: %d lines, QR %v, placement %v",
			len(f.Lines), f.QR != nil, f.qrAt != nil)
	}
}
