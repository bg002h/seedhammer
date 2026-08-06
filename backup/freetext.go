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
// operator never approved. Row 0 and the last row are both screw-hole rows, and
// what keeps their ink clear of the holes is the 18-character CAP, measured at
// every rung by TestTitleCapFitsAtEveryRung: 6.0mm is the tight one, where an
// 18-character title clears by 0.620mm and 20 characters do not clear at all.
//
// centerInset centres them in the inset span rather than on the full plate
// width, and that term states an intent rather than enforcing one. Centring is
// symmetric about the plate's midline either way, so the inset moves a title by
// at most one device unit and by none at all when the gap is even. MEASURED at
// spec 7.19: zeroing it leaves this repository's whole suite green and every
// golden byte for byte identical. An earlier version of this comment claimed
// full-width centring put a 20-character 6.0mm title across both screw-hole
// bands; it puts it in exactly the same place, and the cap is what refuses it.
//
// f must be the fit's output for the same composition. Its lines are engraved
// down a running y in DEVICE units, each row advancing by its OWN size, which
// is what a plate that mixes sizes needs and what a plate that does not gets
// for free: with every entry of f.Sizes equal, the running sum and
// margin + row*fontSize are exactly equal. Extra lines are NOT dropped: they
// run past the plate and toPlate refuses the result, which is a visible failure
// rather than a plate missing the end of what the operator wrote.
//
// f.Faces and f.Sizes are READ, never re-derived. Each line is cut in the face
// it was WRAPPED to and at the size it was MEASURED at, and the left inset of a
// screw-hole row is that face's own character width at that size. Engraving a
// line in any other face re-flows nothing -- the line is already broken, so it
// simply runs wide or short of the grid it was measured against, which no
// assertion on the size, the lines or the code can see.
//
// f.SizeMM is deliberately not read here: it is valid only when !Mixed, and a
// mixed plate's zero would put LinesPerPlate and the width division through a
// divide by zero, mid-flow, with a plate clamped in the machine.
func EngraveFitted(params engrave.Params, f Fitted) engrave.Engraving {
	// The ORDER of these two guards is not load-bearing, and an earlier version
	// of this comment claimed it was -- it said a Sizes guard placed ahead of
	// the Faces one would answer for the short-face-map fixture and stop it
	// testing the face map. It would not: that fixture hands in a COMPLETE size
	// map and its partner a complete face map, so exactly one guard can fire in
	// each. MEASURED at spec 7.19: swapping them leaves both fixtures green.
	// What tells the two apart is that each fixture asserts WHICH panic it
	// recovered; recover() != nil is not an assertion.
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
		margin := params.I(outerMargin)
		plateW := params.F(plateSize)
		// The SAME window the fit wrapped against. limit is the footer's own
		// top y, so the row the body was refused above and the row the footer
		// is cut on cannot be two different rows; a bottom anchor of the
		// engraver's own differs from it by the LinesPerPlate remainder.
		start, limit := yBudget(params, f.Title, f.Footer, f.TitleSizeMM, f.FooterSizeMM)

		// centerInset engraves s centred on the plate, in its own face AND at
		// its own size: the title of a size-ladder plate is not cut at the
		// body's rung. The inset is a whole number of THAT face's characters at
		// THAT size; see the doc comment above for what it does and does not do.
		centerInset := func(s string, fnt *vector.Face, sizeMM float32, y int) {
			if s == "" {
				return
			}
			fontSize := params.F(sizeMM)
			lay := textLayout(params, fnt, fontSize, y, f.qrAt)
			cmd := engrave.String(fnt, fontSize, s)
			w, _ := cmd.Measure()
			inset := lay.holeChars * lay.charWidth
			t.Offset(margin+inset+(plateW-2*margin-2*inset-w)/2, y)
			cmd.Engrave(t.Yield)
		}

		centerInset(f.Title, f.TitleFace, f.TitleSizeMM, margin)
		if f.Footer != "" {
			// Guarded on the STRING and not on centerInset's own empty check,
			// because limit is only the footer's row when there IS a footer:
			// without one it is the bottom margin, and spec 5's ladder plates
			// carry no footer at all.
			centerInset(f.Footer, f.FooterFace, f.FooterSizeMM, limit)
		}

		// The running y, in DEVICE units. Accumulating float32 millimetres and
		// converting once at the end drifts -- 20 additions of 3.8 can land on
		// 486399 instead of 486400 -- and moves every golden. For a uniform
		// plate this sum and margin + row*fontSize are exactly equal, because
		// every rung converts exactly at MM = 6400.
		y := start
		for i, l := range f.Lines {
			fnt := f.Faces[i]
			fontSize := params.F(f.Sizes[i])
			// Row i's own layout at row i's own baseY, read at 0. Keeping a
			// plate-absolute row index here would agree on a uniform plate and
			// diverge on a mixed one: the fit computed this inset
			// block-relative and the engraver would compute it plate-relative,
			// a drift no assertion on the size, the lines or the code can see.
			_, offx := textLayout(params, fnt, fontSize, y, f.qrAt).at(0)
			t.Offset(offx+margin, y)
			engrave.String(fnt, fontSize, l).Engrave(t.Yield)
			y += fontSize
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
