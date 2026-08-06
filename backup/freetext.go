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
	// The Faces guard is evaluated FIRST, and deliberately: the test that pins
	// it hands in a short face map with a correct size map, so a Sizes guard
	// ahead of it would answer for it and the face map would stop being tested.
	if len(f.Faces) != len(f.Lines) {
		// A face map that does not cover every line is a caller bug, and the
		// alternative to failing here is engraving some rows in whatever face
		// happened to be at hand.
		panic(fmt.Errorf("backup: %d lines but %d faces", len(f.Lines), len(f.Faces)))
	}
	if len(f.Sizes) != len(f.Lines) {
		// Same shape, same reason: a size map that does not cover every line
		// leaves the uncovered rows to be cut at whatever size was at hand.
		panic(fmt.Errorf("backup: %d lines but %d sizes", len(f.Lines), len(f.Sizes)))
	}
	for i, s := range f.Sizes {
		if s == 0 {
			panic(fmt.Errorf("backup: line %d is sized 0mm", i))
		}
	}
	// The size/string invariant. A zero size with a non-empty string puts a
	// whole row through the layout at fontSize 0 -- LinesPerPlate divides by it
	// and fixedCharWidth returns 0, so the width division is a second divide by
	// zero. A non-zero size with an empty string is the same bug read the other
	// way round: a size was resolved for a row that does not exist.
	if (f.Title != "") != (f.TitleSizeMM != 0) {
		panic(fmt.Errorf("backup: title %q at %.1fmm", f.Title, f.TitleSizeMM))
	}
	if (f.Footer != "") != (f.FooterSizeMM != 0) {
		panic(fmt.Errorf("backup: footer %q at %.1fmm", f.Footer, f.FooterSizeMM))
	}
	// The QR guards. Each is a caller bug with no operator-facing meaning:
	// there is no correct plate to fall back to, so there is nothing to return.
	if (f.QR == nil) != (f.qrAt == nil) {
		panic(fmt.Errorf("backup: QR %v but placement %v", f.QR != nil, f.qrAt != nil))
	}
	if f.QR != nil && f.Mixed {
		// The band is quantised by a SINGLE fontSize via qrLines, so a plate
		// that mixes sizes has no single band to quantise it to. FitSized makes
		// this structurally true by having no QR parameter at all; the check is
		// here for the constructor that has not been written yet.
		panic(fmt.Errorf("backup: a mixed-size plate carries a QR"))
	}
	if f.qrAt != nil && f.qrAt.Bottom > params.F(plateSize)-params.I(outerMargin) {
		// Defensive: the fit already refused this as ErrQRTooTall. Reaching it
		// here means a Fitted was built by something other than the fit.
		panic(fmt.Errorf("backup: the QR band ends at %d, past the %d bottom margin",
			f.qrAt.Bottom, params.F(plateSize)-params.I(outerMargin)))
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
			lay := lays.at(params, fnt, fontSize, f.qrAt)
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
			_, offx := lays.at(params, fnt, fontSize, f.qrAt).at(row)
			t.Offset(offx+margin, rowY(row))
			engrave.String(fnt, fontSize, l).Engrave(t.Yield)
		}

		if f.QR != nil {
			// READ, never re-derived. The fit resolved this placement and the
			// same placement narrowed the lines above, so the code cannot be
			// drawn anywhere but the hole the text was broken around. It also
			// carries no face: the code is plate geometry, so a mixed-face
			// plate puts it exactly where a single-face one does.
			t.Offset(f.qrAt.X, f.qrAt.Y)
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
	sizes := make([]float32, len(lines))
	for i := range faces {
		faces[i] = fnt
		sizes[i] = fontMM
	}
	var titleSize, footerSize float32
	if title != "" {
		titleSize = fontMM
	}
	if footer != "" {
		footerSize = fontMM
	}
	// The placement is resolved HERE, at the constructor, which is what makes
	// this the one-size case of the general path: Mixed stays false, every
	// entry of Sizes is the same value, and the code has exactly one y, which
	// nothing downstream re-derives.
	var qrAt *qrPlacement
	if qrc != nil {
		p := qrPlaceAt(params, qrc, freeTextQRScale, params.F(fontMM), params.I(outerMargin))
		qrAt = &p
	}
	return EngraveFitted(params, Fitted{
		SizeMM:       fontMM,
		Sizes:        sizes,
		Lines:        lines,
		Faces:        faces,
		QR:           qrc,
		qrAt:         qrAt,
		Title:        title,
		Footer:       footer,
		TitleFace:    fnt,
		FooterFace:   fnt,
		TitleSizeMM:  titleSize,
		FooterSizeMM: footerSize,
	})
}
