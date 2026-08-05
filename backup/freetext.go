package backup

import (
	qr "github.com/seedhammer/kortschak-qr"
	"seedhammer.com/engrave"
	"seedhammer.com/font/vector"
)

// EngraveFreeText lays out the free-text plate per spec 8.
//
// title and footer are engraved VERBATIM -- never through TitleString, which
// upper-cases and truncates at 18, so it would engrave something the operator
// never approved -- and are centered in the INSET SPAN, not the full plate
// width. Row 0 and the last row are both screw-hole rows: centering a
// 20-character title at 6.0mm on the full width inks x[7.127, 77.962]mm, which
// crosses both screw-hole bands while every check passes.
//
// lines must be Fit's output for the same composition, fontMM AND FACE. They
// are engraved at plate rows start, start+1, ... where start is 1 when a title
// occupies row 0. Extra lines are NOT dropped: they run past the plate and
// toPlate refuses the result, which is a visible failure rather than a plate
// missing the end of what the operator wrote.
//
// fnt is the face the whole plate is cut in -- body, title and footer alike.
// Passing a face other than the one Fit measured re-flows nothing: the lines
// are already broken, so they simply run wide or short of the grid they were
// wrapped to.
func EngraveFreeText(params engrave.Params, fnt *vector.Face, fontMM float32,
	title string, lines []string, footer string, qrc *qr.Code) engrave.Engraving {
	return func(yield func(engrave.Command) bool) {
		t := engrave.NewTransform(yield)
		fontSize := params.F(fontMM)
		margin := params.I(outerMargin)
		plateW := params.F(plateSize)
		lay := textLayout(params, fnt, fontSize, margin, qrc, freeTextQRScale)
		rows := LinesPerPlate(params, fontMM)
		start, _ := bodyRows(rows, title, footer)

		rowY := func(row int) int { return margin + row*fontSize }

		// centerInset engraves s centered between the screw-hole bands.
		centerInset := func(s string, row int) {
			if s == "" {
				return
			}
			cmd := engrave.String(fnt, fontSize, s)
			w, _ := cmd.Measure()
			inset := lay.holeChars * lay.charWidth
			t.Offset(margin+inset+(plateW-2*margin-2*inset-w)/2, rowY(row))
			cmd.Engrave(t.Yield)
		}

		centerInset(title, 0)
		centerInset(footer, rows-1)

		for i, l := range lines {
			row := start + i
			_, offx := lay.at(row)
			t.Offset(offx+margin, rowY(row))
			engrave.String(fnt, fontSize, l).Engrave(t.Yield)
		}

		if qrc != nil {
			qrsz := qrc.Size * params.StrokeWidth * freeTextQRScale
			qrBorder := params.I(2)
			t.Offset(plateW-qrsz-margin-qrBorder,
				margin+lay.holeLines*fontSize+(lay.qrLines*fontSize-qrsz)/2)
			engrave.QR(params.StrokeWidth, freeTextQRScale, qrc)(t.Yield)
		}
	}
}
