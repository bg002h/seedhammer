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

	// Title and Footer are OPTIONAL screw-hole rows (S6b spec 1.1): Title is
	// plate row 0, Footer the last plate row. Both are rendered through the
	// SAME layout helpers the paragraph body uses -- textLayout and
	// qrPlaceAt, in wrap.go -- so the hole-band inset a title/footer centers
	// against is computed the one place every row on this plate computes it.
	//
	// EMPTY IS NORMATIVE, NOT AN OPTIMISATION (R-F, R-G). An empty Title or
	// Footer renders no row and consumes no vertical budget: EngraveText must
	// produce byte-identical output to before these fields existed, which is
	// what backup/testdata/text-{0,1,2}-shards-1.bin pin and what lets the
	// caller -- never validateMdmk -- decide whether a plate is marked.
	Title, Footer string
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

// seedQRLevel and seedQRMaxSize are the two inputs -- and the ONLY two -- that
// decide seal.MaxEngraveableCodex32Len, §10.2.1a's admission limit. They are
// named rather than written inline so the test that re-derives that limit can
// read them instead of restating them: a duplicated `qr.M` in the test would
// keep computing 90 while this function had moved to another level, which is
// the exact silent drift §10.2.1a says must not be possible. Behaviour is
// unchanged -- these are the values that were already here.
//
// Measured: at qr.Q the limit drops from 90 to 67, below EncodeMS1's ordinary
// output, so that change would both reopen F-113 and reject ordinary seeds.
// Raising seedQRMaxSize is F-117/F-118 and deliberately not done here.
//
// qrScale is NOT one of them: the version is decided by qr.Encode before
// qrScale is ever read. It changes how big the QR is cut, not which version the
// string needs.
const (
	seedQRLevel   = qr.M
	seedQRMaxSize = 33
)

func EngraveSeedString(params engrave.Params, plate SeedString) (engrave.Engraving, error) {
	seed := strings.ToUpper(plate.Seed)
	qrc, err := qr.Encode(seed, seedQRLevel)
	if err != nil {
		return nil, err
	}
	if qrc.Size > seedQRMaxSize {
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

// ErrMultiParagraphQR is EngraveText's refusal of a QR on a plate that holds
// more than one paragraph (F-434).
//
// A paragraph advances the running y by its TEXT lines only, while a code
// occupies twelve rows from two rows below its paragraph's top -- so paragraph
// n's code is drawn ACROSS paragraphs n+1 onward, and a text-less paragraph's
// code is centred on the PLATE, stacking every code in a packed plate on one
// spot. Both lay out INSIDE the plate, so gui.toPlate reports a FIT: overlapping
// ink on steel, announced as a fit.
//
// The real fix is to advance by max(textLines, qrLines), which moves every
// golden; until then this turns a silently wrong plate into an immediate
// refusal. Nothing in the tree constructs the arrangement -- every QR-carrying
// Paragraph is alone on its plate -- so the refusal costs no caller anything and
// disarms the trap for the next one.
var ErrMultiParagraphQR = errors.New("backup: a QR belongs to a plate of its own; this plate has more than one paragraph")

// EngraveText lays out a plate of paragraphs, per S6b spec 1.1/1.2.
//
// It REFUSES rather than draws what it cannot lay out, the way its siblings
// EngraveSeed and EngraveSeedString do, because the alternative lands on the
// device: a plate is cut from whatever this returns, and there is no camera to
// read it back. Two impossibilities are refused, both of which gui.toPlate calls
// a fit -- a QR on a multi-paragraph plate (ErrMultiParagraphQR) and a body that
// runs past its vertical budget (ErrTooLarge).
func EngraveText(params engrave.Params, plate Text) (engrave.Engraving, error) {
	if len(plate.Paragraphs) > 1 {
		for i, p := range plate.Paragraphs {
			if p.QR != nil {
				return nil, fmt.Errorf("%w (paragraph %d of %d carries one)",
					ErrMultiParagraphQR, i+1, len(plate.Paragraphs))
			}
		}
	}

	fontSize := params.F(plate.fontMM())
	fnt := plate.Font
	margin := params.I(outerMargin)
	plateDims := image.Point{
		X: params.F(plateSize),
		Y: params.F(plateSize),
	}

	// THE SAME WINDOW THE FREE-TEXT PATH WRAPS AGAINST -- fit.go's yBudget, at
	// this plate's one face and size -- and it answers both of the questions the
	// body's geometry used to answer separately here.
	//
	// start is where the body begins: row 0 when the plate is unmarked, row 1
	// when a title has spoken for it (spec 1.2b). limit is the y no row's bottom
	// may pass: the FOOTER'S OWN ROW when there is a footer, the bottom margin
	// when there is not. Reading both off yBudget is what makes the row a body
	// is refused above and the row the footer is cut on one expression -- see
	// footerRowY, which exists for exactly that reason.
	titleRow := margin
	start, limit := yBudget(params, plate.Title, plate.Footer, plate.fontMM(), plate.fontMM())

	// THE BODY IS LAID OUT BEFORE ANY OF IT IS DRAWN (F-435). The paragraph path
	// had no budget at all: a body could be cut straight over a non-empty
	// footer and still be In() the safety margin, so gui.toPlate reported a FIT
	// -- the footer sits INSIDE that margin, so nothing downstream could see the
	// collision. The packer carried a second check of its own to cover it; this
	// makes the collision impossible instead, for every caller rather than the
	// one that remembered.
	//
	// Each paragraph's layout, wrapped lines and code placement are resolved
	// ONCE, here, and READ by the closure below. Laying out twice -- once to
	// measure, once to draw -- would let the rows the budget admitted and the
	// rows the engraver cuts drift apart, which is the drift textLayout's own
	// comment says must not be possible.
	//
	// IT BUDGETS TEXT ROWS, exactly as yBudget does on the free-text side: a
	// code's band is not part of this window there either (it is refused
	// separately, against the bottom margin, as ErrQRTooTall) and a QR-ONLY
	// paragraph's code is centred on the PLATE rather than laid on rows at all.
	type paragraphPlan struct {
		lay lineLayout
		// lines is the wrap; emptyText records that the ORIGINAL text was empty,
		// which is a different question -- see the centering override below.
		lines     []string
		emptyText bool
		qrp       *qrPlacement
		qr        engrave.Engraving
	}
	plans := make([]paragraphPlan, len(plate.Paragraphs))
	offy := start
	for i, p := range plate.Paragraphs {
		qrScale := p.QRScale
		if qrScale == 0 {
			qrScale = 2
		}
		// The descriptor's code belongs to its PARAGRAPH and moves with it,
		// so the placement is anchored at offy -- this paragraph's top --
		// and resolved once, here. The layout below narrows the lines
		// against it and the offset below draws the code from it: one
		// object, one y, read twice and derived never.
		pp := paragraphPlan{emptyText: len(p.Text) == 0}
		if p.QR != nil {
			at := qrPlaceAt(params, p.QR, qrScale, fontSize, offy)
			pp.qrp = &at
			pp.qr = engrave.QR(params.StrokeWidth, qrScale, p.QR)
		}
		// baseY is this paragraph's top edge in DEVICE units. widthAt is
		// indexed by output line, so the plate-row offset has to live
		// inside the layout -- and for the descriptor path that offset is
		// not row-aligned, because paragraphs after the first advance offy
		// by lineno*fontSize + 1mm.
		pp.lay = textLayout(params, fnt, fontSize, offy, pp.qrp)
		if !pp.emptyText {
			// The descriptor and mdmk callers (validateDescriptor,
			// validateMdmk) keep an UNBOUNDED path here: they offer
			// whichever of TEXT+QR / TEXT-ONLY / QR-ONLY fit, which
			// depends on toPlate rejecting overflow, so a maxLines
			// refusal here would silently change which variants they
			// offer.
			//
			// These three are NOT a TEXT+QR -> TEXT-ONLY -> QR-ONLY
			// fallback chain, despite an earlier version of this comment
			// claiming that order (F-119). Measured by growing the input
			// string through validateMdmk and watching each variant drop
			// out: TEXT+QR fails first (works through 268 chars, fails at
			// 269), then QR-ONLY (641, fails at 642), and TEXT-ONLY fails
			// LAST (645, fails at 646). QR-ONLY is the LEAST robust of
			// the two single-mode variants, not the most: a QR code's
			// capacity is a hard ceiling, while wrapped text keeps
			// fitting a few characters longer at the same plate size. The
			// variants themselves are correct -- validateMdmk/
			// validateDescriptor try all three and offer whichever fit,
			// not a first-match chain -- only the comment's stated order
			// was wrong.
			//
			// Empty text is special-cased rather than wrapped, because
			// spec 5.2's empty-block rule returns ONE empty line and that
			// rule serves the free-text plate only. Here zero characters
			// must mean zero rows, which is what the QR-ONLY variant --
			// and text-2-shards-1.bin -- depends on.
			pp.lines, _ = WrapText(p.Text, func(i int) int {
				n, _ := pp.lay.at(i)
				return n
			}, math.MaxInt)
		}
		plans[i] = pp
		offy += len(pp.lines) * fontSize
		if i != len(plate.Paragraphs)-1 {
			// Space UR sections.
			offy += params.I(1)
		}
	}
	// THE BUDGET BINDS WHERE SOMETHING IS CUT AT THE LIMIT, which is the footer
	// and only the footer.
	//
	// Without a footer, limit is the bottom margin -- and that y is ALREADY
	// enforced, better, by gui.toPlate, which measures the INK rather than the
	// nominal row. The difference is the last row's descender space, and it is
	// real capacity: MEASURED, a nominal refusal at the bottom margin rejects
	// backup/testdata/text-0-shards-1.bin (body ends at 529920, margin 524800,
	// ink 5120 short of it) -- a golden that engraves correctly today. It would
	// also silently change which variants validateDescriptor and validateMdmk
	// offer, which the unbounded wrap above exists to avoid.
	//
	// The footer is different in kind: it is INSIDE the safety margin, so ink
	// laid on top of it is still In() the plate and toPlate cannot see the
	// collision at all. There is no downstream check to defer to, which is why
	// this one is here.
	if plate.Footer != "" && offy > limit {
		// ErrTooLarge, wrapped: this is the paragraph path's answer to the
		// question fit.go answers with the same value on the free-text side, and
		// every caller already treats "does not fit a plate" as the signal to
		// drop this variant or split this plate.
		return nil, fmt.Errorf("%w: the body ends at %d, past the footer row %d",
			ErrTooLarge, offy, limit)
	}

	return func(yield func(engrave.Command) bool) {
		t := engrave.NewTransform(yield)

		// centerRow engraves s, verbatim, centered in the screw-hole-free
		// inset span of one plate row at y -- the same arithmetic
		// EngraveFitted's title/footer use (freetext.go's centerInset):
		// textLayout's holeChars*charWidth, at THIS plate's one face and
		// size. s=="" is a no-op: no Yield call, so an unmarked plate is
		// untouched by this closure existing at all (R-F, R-G).
		centerRow := func(s string, y int) {
			if s == "" {
				return
			}
			lay := textLayout(params, fnt, fontSize, y, nil)
			cmd := engrave.String(fnt, fontSize, s)
			w, _ := cmd.Measure()
			inset := lay.holeChars * lay.charWidth
			t.Offset(margin+inset+(plateDims.X-2*margin-2*inset-w)/2, y)
			cmd.Engrave(t.Yield)
		}

		for i := range plans {
			pp := &plans[i]
			for lineno, s := range pp.lines {
				_, offx := pp.lay.at(lineno)
				t.Offset(offx+margin, pp.lay.baseY+lineno*fontSize)
				engrave.String(fnt, fontSize, s).Engrave(t.Yield)
			}
			if pp.qr != nil {
				qrx, qry := pp.qrp.X, pp.qrp.Y
				// Keyed to the ORIGINAL text, never to len(lines): under
				// spec 5.2 an empty string wraps to one empty line, and
				// keying this to the line count displaces the QR-ONLY plate
				// by (6.450, 2.300)mm at production stroke.
				if pp.emptyText {
					// Center QR. A placement OVERRIDE, not a band question:
					// the paragraph has no rows for a band to narrow.
					qrx, qry = (plateDims.X-pp.qrp.Size)/2, (plateDims.Y-pp.qrp.Size)/2
				}
				t.Offset(qrx, qry)
				pp.qr(t.Yield)
			}
		}

		// AN UNSIGNED PLATE IS AN UNFINISHED PLATE.
		//
		// The title row is the plate's CLAIM ABOUT ITSELF -- "TX 2DCF2B97
		// 1/2", "PLATE 2 OF 3". Cut first, as it was, a plate abandoned at
		// minute 20 already carried that claim and LOOKED FINISHED, so an
		// operator taught to sort by it would put a half-cut plate in the good
		// stack. This device has NO CAMERA and cannot read a plate back, so
		// the operator is the only inspector there will ever be and a claim
		// that outruns the artifact has nothing downstream to catch it.
		//
		// It is a reordering of YIELDS and nothing else: both rows' positions
		// are the y offsets computed above, so the finished plate is
		// byte-identical to what this emitted before. That is also why no
		// bounds or golden test in this package can see the change, and why
		// TestTheTitleAndFooterAreEmittedLast asserts the ORDER of emitted
		// operations instead.
		//
		// There is no resume for an abandoned cut, for a mechanical reason:
		// re-clamping cannot guarantee the plate returns to the same origin,
		// and this machine has already produced a misregistration artefact
		// traced to Y-axis play from a loose screw. A resumed cut would be
		// offset and would still look finished.
		centerRow(plate.Title, titleRow)
		if plate.Footer != "" {
			// Guarded on the STRING and not on centerRow's own empty check,
			// because limit is only the footer's row when there IS a footer:
			// without one it is the bottom margin, which is not a row anything
			// is cut on.
			centerRow(plate.Footer, limit)
		}
	}, nil
}
