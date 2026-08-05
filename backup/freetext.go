package backup

import (
	"fmt"

	qr "github.com/seedhammer/kortschak-qr"
	"seedhammer.com/engrave"
	"seedhammer.com/font/vector"
)

// EngraveFitted lays out the free-text plate per spec 8, from the ONE object
// the fit produced.
//
// f.Title and f.Footer are engraved VERBATIM -- never through TitleString,
// which upper-cases and truncates at 18, so it would engrave something the
// operator never approved -- and are centered in the INSET SPAN, not the full
// plate width. Row 0 and the last row are both screw-hole rows: centering a
// 20-character title at 6.0mm on the full width inks x[7.127, 77.962]mm, which
// crosses both screw-hole bands while every check passes.
//
// f must be FitBlocks' output for the same composition. Its lines are engraved
// at plate rows start, start+1, ... where start is 1 when a title occupies row
// 0. Extra lines are NOT dropped: they run past the plate and toPlate refuses
// the result, which is a visible failure rather than a plate missing the end of
// what the operator wrote.
//
// f.Faces is READ, never re-derived. Each line is cut in the face it was
// WRAPPED to, and the left inset of a screw-hole row is that face's own
// character width. Engraving a line in any other face re-flows nothing -- the
// line is already broken, so it simply runs wide or short of the grid it was
// measured against, which no assertion on the size, the lines or the code can
// see.
func EngraveFitted(params engrave.Params, f Fitted) engrave.Engraving {
	if len(f.Faces) != len(f.Lines) {
		// A face map that does not cover every line is a caller bug, and the
		// alternative to failing here is engraving some rows in whatever face
		// happened to be at hand.
		panic(fmt.Errorf("backup: %d lines but %d faces", len(f.Lines), len(f.Faces)))
	}
	return func(yield func(engrave.Command) bool) {
		t := engrave.NewTransform(yield)
		fontSize := params.F(f.SizeMM)
		margin := params.I(outerMargin)
		plateW := params.F(plateSize)
		rows := LinesPerPlate(params, f.SizeMM)
		start, _ := bodyRows(rows, f.Title, f.Footer)
		var lays faceLayouts

		rowY := func(row int) int { return margin + row*fontSize }

		// centerInset engraves s centered between the screw-hole bands, in its
		// own face: the inset is a whole number of THAT face's characters.
		centerInset := func(s string, fnt *vector.Face, row int) {
			if s == "" {
				return
			}
			lay := lays.at(params, fnt, fontSize, f.QR)
			cmd := engrave.String(fnt, fontSize, s)
			w, _ := cmd.Measure()
			inset := lay.holeChars * lay.charWidth
			t.Offset(margin+inset+(plateW-2*margin-2*inset-w)/2, rowY(row))
			cmd.Engrave(t.Yield)
		}

		centerInset(f.Title, f.TitleFace, 0)
		centerInset(f.Footer, f.FooterFace, rows-1)

		for i, l := range f.Lines {
			fnt := f.Faces[i]
			row := start + i
			_, offx := lays.at(params, fnt, fontSize, f.QR).at(row)
			t.Offset(offx+margin, rowY(row))
			engrave.String(fnt, fontSize, l).Engrave(t.Yield)
		}

		if f.QR != nil {
			// holeLines and qrLines are plate geometry -- rows of a given
			// height above and beside the code -- so they carry no face, and a
			// mixed-face plate puts the QR exactly where a single-face one
			// does.
			lay := lays.at(params, f.TitleFace, fontSize, f.QR)
			qrsz := f.QR.Size * params.StrokeWidth * freeTextQRScale
			qrBorder := params.I(2)
			t.Offset(plateW-qrsz-margin-qrBorder,
				margin+lay.holeLines*fontSize+(lay.qrLines*fontSize-qrsz)/2)
			engrave.QR(params.StrokeWidth, freeTextQRScale, f.QR)(t.Yield)
		}
	}
}

// EngraveFreeText is EngraveFitted for a plate cut entirely in one face --
// body, title and footer alike.
//
// lines must be Fit's output for the same composition, fontMM AND FACE.
// Passing a face other than the one Fit measured re-flows nothing; see
// EngraveFitted.
func EngraveFreeText(params engrave.Params, fnt *vector.Face, fontMM float32,
	title string, lines []string, footer string, qrc *qr.Code) engrave.Engraving {
	faces := make([]*vector.Face, len(lines))
	for i := range faces {
		faces[i] = fnt
	}
	return EngraveFitted(params, Fitted{
		SizeMM:     fontMM,
		Lines:      lines,
		Faces:      faces,
		QR:         qrc,
		Title:      title,
		Footer:     footer,
		TitleFace:  fnt,
		FooterFace: fnt,
	})
}
