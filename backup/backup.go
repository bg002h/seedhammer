// package backup implements the SeedHammer backup scheme.
package backup

import (
	"errors"
	"fmt"
	"image"
	"math"
	"strings"

	qr "github.com/seedhammer/kortschak-qr"
	"seedhammer.com/engrave"
	"seedhammer.com/font/vector"
)

type Seed struct {
	Title             string
	Mnemonic          []string
	ShortestWord      int
	LongestWord       int
	QR                *qr.Code
	MasterFingerprint uint32
	Font              *vector.Face
}

type SeedString struct {
	Title             string
	Seed              string
	MasterFingerprint uint32
	Font              *vector.Face
}

type Text struct {
	Paragraphs []Paragraph
	Font       *vector.Face
	// FontSize is the text size in millimeters. Zero means
	// plateFontSizeUR, which is what every descriptor and mdmk caller
	// constructs, and is why their goldens are unaffected by the
	// free-text plate's size ladder.
	FontSize float32
}

// fontMM resolves FontSize, applying the zero-means-plateFontSizeUR rule in one
// place so no caller can forget it.
func (t Text) fontMM() float32 {
	if t.FontSize == 0 {
		return plateFontSizeUR
	}
	return t.FontSize
}

type Paragraph struct {
	Text    string
	QR      *qr.Code
	QRScale int
}

const MaxTitleLen = 18

const outerMargin = 3
const innerMargin = 10

// plateSize is the width and height of a plate in millimeters.
const plateSize = 85

// FontSizes is the descending ladder of free-text plate sizes in millimeters.
// Auto-fit walks it largest-first and engraves at the first rung the whole
// composition fits, so it MUST stay sorted descending: an out-of-order entry
// does not fail, it silently engraves smaller than necessary.
var FontSizes = []float32{6.0, 5.0, 4.4, 3.8, 3.4, 3.0}

// CharsPerLine returns how many fixed-width characters fit on one unobstructed
// plate line at the given text size. Lines crossing a screw-hole band hold
// fewer; see the widthAt band predicate in fit.go.
func CharsPerLine(params engrave.Params, fnt *vector.Face, fontMM float32) int {
	width := params.F(plateSize) - 2*params.I(outerMargin)
	return width / fixedCharWidth(fnt, params.F(fontMM))
}

// LinesPerPlate returns how many text lines fit a plate at the given text size.
func LinesPerPlate(params engrave.Params, fontMM float32) int {
	height := params.F(plateSize) - 2*params.I(outerMargin)
	return height / params.F(fontMM)
}

// fixedCharWidth is the character advance at fontSize machine units, assuming a
// fixed-width face. Verified by TestFixedCharWidthIsExactForEveryGlyph: every
// font/sh advance is 4000 with Metrics{Ascent:5000, Height:6700}, so 'W' is
// exact for all glyphs.
func fixedCharWidth(fnt *vector.Face, fontSize int) int {
	w, _, ok := fnt.Decode('W')
	if !ok {
		panic("W not in font")
	}
	return int(float32(w*fontSize) / float32(fnt.Metrics().Height))
}

func TitleString(face *vector.Face, s string) string {
	s = strings.ToUpper(s)
	res := ""
	for _, r := range s {
		if _, _, valid := face.Decode(r); valid {
			res += string(r)
		}
		if len(res) == MaxTitleLen {
			break
		}
	}
	return res
}

func EngraveSeed(params engrave.Params, plate Seed) (engrave.Engraving, error) {
	var qrc *engrave.ConstantQRCmd
	if plate.QR != nil {
		var err error
		qrc, err = engrave.ConstantQR(plate.QR)
		if err != nil {
			return nil, err
		}
	}
	side := frontSideSeed(params, plate, qrc)
	return side, nil
}

func EngraveSeedString(params engrave.Params, plate SeedString) (engrave.Engraving, error) {
	seed := strings.ToUpper(plate.Seed)
	qrc, err := qr.Encode(seed, qr.M)
	if err != nil {
		return nil, err
	}
	if qrc.Size > 33 {
		return nil, errors.New("seed too long to engrave QR")
	}
	qrCmd, err := engrave.ConstantQR(qrc)
	if err != nil {
		return nil, err
	}
	side := engraveSeedString(params, plate, qrCmd)
	return side, nil
}

const plateFontSize = 4.1
const plateFontSizeUR = 3.8
const plateSmallFontSize = 3.

const groupLen = 10

func engraveSeedString(params engrave.Params, plate SeedString, qrc *engrave.ConstantQRCmd) engrave.Engraving {
	pfs := params.F(plateFontSize)
	constant := engrave.NewConstantStringer(plate.Font, params, pfs)
	return func(yield func(engrave.Command) bool) {
		plateDims := image.Point{
			X: params.F(85),
			Y: params.F(85),
		}
		t := engrave.NewTransform(yield)

		const (
			maxCol1 = 16
			maxCol2 = 4
			qrScale = 3
		)
		seed := strings.ToUpper(plate.Seed)
		ngroups := (len(seed) + groupLen - 1) / groupLen
		endCol1 := min(ngroups, maxCol1)
		qrsz := qrc.Size * params.StrokeWidth * qrScale
		col1Height := max(qrsz, pfs*endCol1)

		// Engrave version, mfp and page.
		innerMargin := params.I(innerMargin)
		metaMargin := params.I(4)
		if plate.MasterFingerprint != 0 {
			mfp := fmt.Sprintf("%.8X", plate.MasterFingerprint)
			offy := (plateDims.Y-col1Height)/2 - metaMargin
			mfpStr := engrave.String(plate.Font, params.F(plateSmallFontSize), mfp)
			mfpszX, mfpszY := mfpStr.Measure()
			t.Offset((plateDims.X-mfpszX)/2, offy-mfpszY)
			mfpStr.Engrave(t.Yield)
		}

		// Engrave column 1.
		off := t.Offset(innerMargin, (plateDims.Y-col1Height)/2)
		stringColumn(off, constant, plate.Font, pfs, seed, groupLen, 0, endCol1)

		// Engrave (top of) column 2.
		endCol2 := min(ngroups, endCol1+maxCol2)
		off = t.Offset(params.I(44), (plateDims.Y-col1Height)/2)
		stringColumn(off, constant, plate.Font, pfs, seed, groupLen, endCol1, endCol2)

		// Engrave seed QR.
		qrCmd := qrc.Engrave(params.StepperConfig, params.StrokeWidth, qrScale)
		t.Offset(params.I(60)-qrsz/2, (plateDims.Y-qrsz)/2)
		qrCmd(t.Yield)

		{
			// Engrave bottom of column 2.
			height := (ngroups - endCol2) * pfs
			off := t.Offset(params.I(44), (plateDims.Y+col1Height)/2-height)
			stringColumn(off, constant, plate.Font, pfs, seed, groupLen, endCol2, ngroups)
		}

		// Engrave title.
		title := strings.ToUpper(plate.Title)
		{
			offy := (plateDims.Y+col1Height)/2 + metaMargin
			title := engrave.String(plate.Font, params.F(plateSmallFontSize), title)
			titleWidth, _ := title.Measure()
			t.Offset((plateDims.X-titleWidth)/2, offy)
			title.Engrave(t.Yield)
		}
	}
}

func frontSideSeed(params engrave.Params, plate Seed, qrc *engrave.ConstantQRCmd) engrave.Engraving {
	return func(yield func(engrave.Command) bool) {
		plateDims := image.Point{
			X: params.F(85),
			Y: params.F(85),
		}
		t := engrave.NewTransform(yield)

		const (
			maxCol1 = 16
			maxCol2 = 4
			// largeN is the inclusive upper bound of the legacy
			// 16+4+4 two-block column-2 layout. Word counts above
			// largeN use the rebalanced single-block layout.
			largeN = 24
		)
		n := len(plate.Mnemonic)
		// pfs is the plate font size, endCol1 the number of rows in
		// column 1, and col1Height the height of column 1. For N<=24
		// these are the legacy values; for N>24 column 1 is rebalanced
		// to ceil(N/2) rows and the font is shrunk just enough to keep
		// those rows within the legacy column envelope (16 rows at the
		// full font).
		pfs := params.F(plateFontSize)
		endCol1 := min(maxCol1, n)
		if n > largeN {
			endCol1 = (n + 1) / 2 // ceil(N/2)
			pfs = min(pfs, maxCol1*params.F(plateFontSize)/endCol1)
		}
		col1Height := pfs * endCol1
		constant := engrave.NewConstantStringer(plate.Font, params, pfs)

		// Engrave master fingerprint.
		innerMargin := params.I(innerMargin)
		metaMargin := params.I(4)
		if plate.MasterFingerprint != 0 {
			mfp := fmt.Sprintf("%.8X", plate.MasterFingerprint)
			offy := (plateDims.Y-col1Height)/2 - metaMargin
			mfpStr := engrave.String(plate.Font, params.F(plateSmallFontSize), mfp)
			mfpszX, mfpszY := mfpStr.Measure()
			t.Offset((plateDims.X-mfpszX)/2, offy-mfpszY)
			mfpStr.Engrave(t.Yield)
		}

		// Engrave column 1.
		off := t.Offset(innerMargin, (plateDims.Y-col1Height)/2)
		wordColumn(off, constant, plate.Font, pfs, plate.Mnemonic, plate.ShortestWord, plate.LongestWord, 0, endCol1)

		if n > largeN {
			// Column 2 is a single contiguous block (rows endCol1..N)
			// anchored at the shared top, eliminating the legacy
			// two-block collision. The large-N path is SLIP-39 only
			// (qrc==nil), so no QR is engraved here.
			off := t.Offset(params.I(44), (plateDims.Y-col1Height)/2)
			wordColumn(off, constant, plate.Font, pfs, plate.Mnemonic, plate.ShortestWord, plate.LongestWord, endCol1, n)
		} else {
			// Engrave (top of) column 2.
			endCol2 := min(endCol1+maxCol2, n)
			off := t.Offset(params.I(44), (plateDims.Y-col1Height)/2)
			wordColumn(off, constant, plate.Font, pfs, plate.Mnemonic, plate.ShortestWord, plate.LongestWord, endCol1, endCol2)

			// Engrave seed QR.
			if qrc != nil {
				const qrScale = 3
				qrCmd := qrc.Engrave(params.StepperConfig, params.StrokeWidth, qrScale)
				qrsz := qrc.Size * params.StrokeWidth * qrScale
				t.Offset(params.I(60)-qrsz/2, (plateDims.Y-qrsz)/2)
				qrCmd(t.Yield)
			}

			// Engrave bottom of column 2.
			height := (n - endCol2) * pfs
			off = t.Offset(params.I(44), (plateDims.Y+col1Height)/2-height)
			wordColumn(off, constant, plate.Font, pfs, plate.Mnemonic, plate.ShortestWord, plate.LongestWord, endCol2, n)
		}

		// Engrave title.
		title := strings.ToUpper(plate.Title)
		{
			offy := (plateDims.Y+col1Height)/2 + metaMargin
			title := engrave.String(plate.Font, params.F(plateSmallFontSize), title)
			titleWidth, _ := title.Measure()
			t.Offset((plateDims.X-titleWidth)/2, offy)
			title.Engrave(t.Yield)
		}
	}
}

func wordColumn(t engrave.Transform, constant *engrave.ConstantStringer, font *vector.Face, fontSize int, mnemonic []string, shortest, longest, start, end int) {
	y := 0
	for i := start; i < end; i++ {
		num := engrave.String(font, fontSize, fmt.Sprintf("%2d ", i+1))
		width, _ := num.Measure()
		w := mnemonic[i]
		word := strings.ToUpper(w)
		t.Offset(0, y)
		num.Engrave(t.Yield)
		t.Offset(width, y)
		constant.PaddedString(t.Yield, word, shortest, longest)
		y += fontSize
	}
}

// stringColumn engraves rows [start, end) of s, rowLen characters per row, one
// constant-time String call per row. The passphrase plate reuses it with
// rowLen != groupLen, which is why the group size is a parameter.
func stringColumn(t engrave.Transform, constant *engrave.ConstantStringer, font *vector.Face, fontSize int, s string, rowLen, start, end int) {
	y := 0
	for i := start; i < end; i++ {
		word := s[i*rowLen:]
		word = word[:min(len(word), rowLen)]
		constant.String(t.Offset(0, y).Yield, word)
		y += fontSize
	}
}

func EngraveText(params engrave.Params, plate Text) engrave.Engraving {
	return func(yield func(engrave.Command) bool) {
		t := engrave.NewTransform(yield)
		fontSize := params.F(plate.fontMM())
		fnt := plate.Font

		charWidth := fixedCharWidth(fnt, fontSize)
		margin := params.I(outerMargin)
		innerMargin := params.I(innerMargin)
		holeChars := int(math.Ceil(float64(innerMargin-margin) / float64(charWidth)))
		holeLines := int(math.Ceil(float64(innerMargin-margin) / float64(fontSize)))
		plateDims := image.Point{
			X: params.F(85),
			Y: params.F(85),
		}
		width := plateDims.X - 2*margin
		charPerLine := int(width / charWidth)
		offy := params.I(outerMargin)
		for i, p := range plate.Paragraphs {
			qrLines := 0
			charPerQRLine := 0
			qrsz := 0
			qrBorder := params.I(2)
			var qr engrave.Engraving
			if p.QR != nil {
				qrScale := p.QRScale
				if qrScale == 0 {
					qrScale = 2
				}
				qr = engrave.QR(params.StrokeWidth, qrScale, p.QR)
				qrsz = p.QR.Size * params.StrokeWidth * qrScale
				charPerQRLine = (width - 2*qrBorder - qrsz) / charWidth
				qrLines = (qrsz + 2*qrBorder + fontSize - 1) / fontSize
			}
			// baseY is this paragraph's top edge in DEVICE units. widthAt is
			// indexed by output line, so the plate-row offset has to live
			// inside the layout -- and for the descriptor path that offset is
			// not row-aligned, because paragraphs after the first advance offy
			// by lineno*fontSize + 1mm.
			lay := lineLayout{
				charPerLine:   charPerLine,
				charPerQRLine: charPerQRLine,
				holeChars:     holeChars,
				holeLines:     holeLines,
				qrLines:       qrLines,
				charWidth:     charWidth,
				fontSize:      fontSize,
				baseY:         offy,
				plateHeight:   plateDims.Y,
				innerMargin:   innerMargin,
			}
			var lines []string
			if len(p.Text) > 0 {
				// The descriptor and mdmk callers keep an UNBOUNDED path:
				// their TEXT+QR -> TEXT-ONLY -> QR-ONLY fallback depends on
				// toPlate rejecting overflow, so a maxLines refusal here would
				// silently change which variants they offer.
				//
				// Empty text is special-cased rather than wrapped, because
				// spec 5.2's empty-block rule returns ONE empty line and that
				// rule serves the free-text plate only. Here zero characters
				// must mean zero rows, which is what the QR-ONLY variant --
				// and text-2-shards-1.bin -- depends on.
				lines, _ = WrapText(p.Text, func(i int) int {
					n, _ := lay.at(i)
					return n
				}, math.MaxInt)
			}
			for lineno, s := range lines {
				_, offx := lay.at(lineno)
				t.Offset(offx+margin, lay.baseY+lineno*fontSize)
				engrave.String(fnt, fontSize, s).Engrave(t.Yield)
			}
			lineno := len(lines)
			if qr != nil {
				qrx := plateDims.X - qrsz - margin - qrBorder
				qry := lay.baseY + holeLines*fontSize + (qrLines*fontSize-qrsz)/2
				// Keyed to the ORIGINAL text, never to len(lines): under
				// spec 5.2 an empty string wraps to one empty line, and
				// keying this to the line count displaces the QR-ONLY plate
				// by (6.450, 2.300)mm at production stroke.
				if len(p.Text) == 0 {
					// Center QR.
					qrx, qry = (plateDims.X-qrsz)/2, (plateDims.Y-qrsz)/2
				}
				t.Offset(qrx, qry)
				qr(t.Yield)
			}
			offy += lineno * fontSize
			if i != len(plate.Paragraphs)-1 {
				// Space UR sections.
				offy += params.I(1)
			}
		}
	}
}
