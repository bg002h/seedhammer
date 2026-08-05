package backup

import (
	"errors"
	"math"

	qr "github.com/seedhammer/kortschak-qr"
	"seedhammer.com/engrave"
	"seedhammer.com/font/sh"
	"seedhammer.com/font/vector"
)

// freeTextQRScale is normative (spec 8): 0.6mm modules against the 0.9mm every
// other plate uses. Spec 4's capacity columns are computed at this scale, so
// changing it changes what fits on a plate.
const freeTextQRScale = 2

// FreeTextFont is the engraving face the free-text plate uses unless a caller
// asks for another one. Every capacity figure in spec 4 is measured in THIS
// face: the ladder's character grid is a property of the face (font/sh is 44
// columns at 3.0mm, font/constant is 39), so a composition fitted in one face
// must never be engraved in the other.
//
// The face is a parameter rather than a package constant because the plate has
// to be able to prove BOTH shipped engraving faces at the smallest rung -- the
// seed, descriptor and passphrase plates engrave in font/constant, and a
// legibility proof cut in font/sh says nothing about those. Fit, Admissible,
// MaxCharsAt and EngraveFreeText therefore all take the face, and a caller that
// passes different faces to the fit and to the engraving gets a plate that does
// not match what was measured.
var FreeTextFont *vector.Face = sh.Font

// ErrTooLarge means the composition does not fit one plate even at the smallest
// rung. Text is refused, never split across plates (spec 6, user directive).
var ErrTooLarge = errors.New("backup: text does not fit a plate")

// qrFor returns the code the plate will carry, or nil when want is false.
// qr.Encode fails at 2954 bytes and above, and the Text field is deliberately
// uncapped, so that input is REACHABLE on a live per-keystroke path. Every
// caller must handle err; none may dereference a nil *qr.Code.
func qrFor(text string, want bool) (*qr.Code, error) {
	if !want {
		return nil, nil
	}
	return qr.Encode(text, qr.L)
}

// bodyRows is the half-open range of plate rows the body may use. A title takes
// row 0 and a footer the last row -- but only when they exist, which is what
// makes spec 4's "Plain" column the full plate. Admission is the one place that
// reserves both unconditionally; see Admissible.
func bodyRows(rows int, title, footer string) (start, end int) {
	start, end = 0, rows
	if title != "" {
		start++
	}
	if footer != "" {
		end--
	}
	if end < start {
		end = start
	}
	return start, end
}

// widthFor turns a plate layout into the widthAt closure WrapText wants:
// indexed by OUTPUT line, with the plate-row offset applied inside.
func widthFor(lay lineLayout, startRow int) func(int) int {
	return func(i int) int {
		n, _ := lay.at(startRow + i)
		return n
	}
}

// Fit is the largest rung whose layout holds the whole composition, in the
// face fnt. The same text fits differently in different faces, so fnt must be
// the face the composition will be ENGRAVED in; see FreeTextFont.
//
// It returns the QR CODE ITSELF, not just its size, so the artifact engraved is
// the very object the fit measured: there is exactly one encode per
// composition, and no caller can re-encode with different parameters and
// disagree.
func Fit(params engrave.Params, fnt *vector.Face, text, title, footer string, qr bool) (fontMM float32, lines []string, qrc *qr.Code, err error) {
	qrc, err = qrFor(text, qr)
	if err != nil {
		return 0, nil, nil, err
	}
	for _, size := range FontSizes {
		rows := LinesPerPlate(params, size)
		start, end := bodyRows(rows, title, footer)
		lay := textLayout(params, fnt, params.F(size), params.I(outerMargin), qrc, freeTextQRScale)
		l, ok := WrapText(text, widthFor(lay, start), end-start)
		if ok {
			return size, l, qrc, nil
		}
	}
	return 0, nil, nil, ErrTooLarge
}

// Admissible is spec 6's anchor: 3.0mm, the QR as chosen, and BOTH the title
// and footer rows reserved whether or not they are used.
//
// Reserving unconditionally is what makes admission monotone -- title and
// footer are deliberately NOT read, so entering a title after the text can
// never retroactively invalidate text already accepted. linesAvail is defined
// even when ok is false, so the readout can show "lines used / lines available"
// over capacity.
func Admissible(params engrave.Params, fnt *vector.Face, text, title, footer string, qr bool) (linesUsed, linesAvail int, ok bool) {
	size := FontSizes[len(FontSizes)-1]
	rows := LinesPerPlate(params, size)
	linesAvail = rows - 2
	if linesAvail < 0 {
		linesAvail = 0
	}
	qrc, err := qrFor(text, qr)
	if err != nil {
		// An unencodable text is inadmissible, but the readout keeps working:
		// linesAvail is already meaningful, and lines used is unknowable
		// without a code to lay out around.
		return 0, linesAvail, false
	}
	lay := textLayout(params, fnt, params.F(size), params.I(outerMargin), qrc, freeTextQRScale)
	// Unbounded on purpose: a refusal that reported "26 / 26" for a text
	// needing 300 lines would tell the operator nothing about how much to cut.
	l, _ := WrapText(text, widthFor(lay, 1), math.MaxInt)
	return len(l), linesAvail, len(l) <= linesAvail
}

// MaxCharsAt is the capacity solver behind the refusal message's live figure:
// how many characters fit at fontMM given the QR that THIS text produces.
//
// The QR's size comes from encoding the text, never from a length table, which
// is the whole point: at 3.0mm with a 700-character text, spec 4's geometry
// column suggests dropping the QR frees about 135 characters and the true
// figure is several times that. Returns 0 if the text cannot be encoded.
func MaxCharsAt(params engrave.Params, fnt *vector.Face, fontMM float32, text string, qr bool) int {
	qrc, err := qrFor(text, qr)
	if err != nil {
		return 0
	}
	lay := textLayout(params, fnt, params.F(fontMM), params.I(outerMargin), qrc, freeTextQRScale)
	rows := LinesPerPlate(params, fontMM)
	total := 0
	for row := range rows {
		n, _ := lay.at(row)
		total += n
	}
	return total
}
