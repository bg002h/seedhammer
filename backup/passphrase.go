package backup

import (
	"image"
	"strings"

	qr "github.com/seedhammer/kortschak-qr"
	"seedhammer.com/engrave"
	"seedhammer.com/font/vector"
	"seedhammer.com/passphrase"
)

// SpaceMark is the codepoint of the visible-space glyph. A space is invisible
// on metal -- one space and two look identical, and leading or trailing spaces
// cannot be seen at all -- while "hunter2 " is a different wallet from
// "hunter2". Every space is therefore engraved as this mark (spec 3.3).
//
// It lives at a control codepoint, NOT at 0x20: TitleString and EngraveText
// paragraphs already engrave literal spaces, so remapping 0x20 would silently
// change existing plate types.
const SpaceMark = '\x1f'

type Passphrase struct {
	// Passphrase is engraved VERBATIM. Never uppercase it.
	Passphrase string
	// SeedFP and CombinedFP are canonical 8-hex-digit fingerprints, or empty.
	// Both are user-typed and unverified (spec D1).
	SeedFP     string
	CombinedFP string
	// QR includes a machine-readable copy of the passphrase. Opt-in (spec D8).
	QR   bool
	Font *vector.Face
}

// Layout constants, pinned per spec 4.1 and 4.2. The em is fixed per layout
// mode and does NOT scale with passphrase length: a 20-character passphrase
// occupies 2 rows at the same em as a 100-character one.
const (
	// passphraseFontSize gives a lowercase x-height of ~9 stroke widths,
	// comparable to what uppercase gets at today's 4.1mm.
	passphraseFontSize = 6.0
	// passphraseRowLen is one 10-character group per row: position implies
	// index, and there are no intra-row gaps to confuse with the space mark.
	passphraseRowLen = 10

	// With a QR the text reflows smaller: text and QR cannot both be full
	// size within the ~65mm of usable height.
	passphraseFontSizeQR = 4.5
	passphraseRowLenQR   = 20
	// passphraseQRScale matches the other plate types.
	passphraseQRScale = 3
	// passphraseQREnvelope is the reserved module count. The QR size is
	// VARIABLE -- 33 or 37 modules at ECC-L within the 100-character cap, and
	// smaller for short passphrases -- so the layout reserves the worst case
	// and centres the actual code inside it rather than assuming a size.
	passphraseQREnvelope = 37
	// passphraseQRGap separates the text block from the QR, in millimetres.
	passphraseQRGap = 2
)

// passphraseGlyphs maps the passphrase to the glyph sequence that gets
// engraved: every space becomes SpaceMark, everything else is verbatim.
func passphraseGlyphs(s string) string {
	if !strings.ContainsRune(s, ' ') {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r == ' ' {
			r = SpaceMark
		}
		b.WriteRune(r)
	}
	return b.String()
}

// passphraseQRCode encodes the passphrase EXACTLY as entered: the same bytes,
// the same case, real 0x20 spaces and never SpaceMark. The mark is a rendering
// device for the text block only; a scanner that saw it would hand a wallet
// different bytes, silently opening a different wallet (spec 4.2).
//
// ECC-L is pinned, not M: the engraved text is the authoritative copy and the
// QR is convenience, so four fewer modules is the better trade.
func passphraseQRCode(plate Passphrase) (*qr.Code, error) {
	return qr.Encode(plate.Passphrase, qr.L)
}

func EngravePassphrase(params engrave.Params, plate Passphrase) (engrave.Engraving, error) {
	var qrc *engrave.ConstantQRCmd
	if plate.QR {
		code, err := passphraseQRCode(plate)
		if err != nil {
			return nil, err
		}
		// ConstantQR, never engrave.QR: the latter engraves in a
		// content-dependent pattern and would leak the secret through timing
		// (spec 3.5.2).
		qrc, err = engrave.ConstantQR(code)
		if err != nil {
			return nil, err
		}
	}
	return engravePassphrase(params, plate, qrc), nil
}

// passphraseLayout is the geometry of one passphrase plate, in machine units.
// It is computed once and shared by the engraver and by the layout tests, so a
// test asserting "the QR is here" cannot drift from where it is engraved.
type passphraseLayout struct {
	// rowLen is the number of characters per row and em the row height.
	rowLen int
	em     int
	// rows is ceil(len(glyphs)/rowLen).
	rows int
	// textX, textY is the top-left of the text block, blockW x blockH its size.
	textX, textY   int
	blockW, blockH int
	// qrDim is the QR's module count, or 0 when no QR is engraved. qrX, qrY
	// and qrSize describe the code itself; envX, envY and envSize describe the
	// fixed 37-module envelope it is centred in.
	qrDim               int
	qrX, qrY, qrSize    int
	envX, envY, envSize int

	// glyphs is the mark-substituted passphrase, one row per rowLen glyphs.
	glyphs string
	// The metadata bands. smallEm is their line height; topY and bottomY are
	// the top edge of each band's first line. Fingerprint lines are omitted
	// entirely when blank -- an empty label is worse than no label.
	smallEm       int
	topY, bottomY int
	topLines      []string
	bottomLines   []string
}

// passphraseAdvance is the per-character advance of the (fixed-width)
// engraving face at em.
func passphraseAdvance(font *vector.Face, em int) int {
	adv, _, ok := font.Decode('W')
	if !ok {
		panic("W not in font")
	}
	return adv * em / font.Metrics().Height
}

// passphraseLegend labels the visible-space mark. Without it the mark is a
// private convention documented nowhere the reader will be: someone inheriting
// a plate has no way to know whether to type a space, a hyphen or nothing, and
// each guess silently opens a different wallet. It is engraved with the real
// mark glyph so the reader matches shapes, not descriptions.
const passphraseLegend = string(SpaceMark) + " = SPACE"

// passphraseFooter is engraved whenever either fingerprint is present. Both are
// user-typed claims (spec D1); nothing on this device has checked them.
const passphraseFooter = "FINGERPRINTS TYPED, NOT VERIFIED"

// passphraseLayoutFor lays out plate, reserving room for a qrDim-module QR.
// qrDim of 0 means no QR.
func passphraseLayoutFor(params engrave.Params, plate Passphrase, qrDim int) passphraseLayout {
	plateDims := image.Point{X: params.F(85), Y: params.F(85)}
	font := plate.Font
	l := passphraseLayout{
		rowLen:  passphraseRowLen,
		em:      params.F(passphraseFontSize),
		qrDim:   qrDim,
		glyphs:  passphraseGlyphs(plate.Passphrase),
		smallEm: params.F(plateSmallFontSize),
		topY:    params.F(outerMargin),
	}
	glyphs := l.glyphs
	// The bands are the 10mm margins, matching existing practice: the master
	// fingerprint and title already sit in exactly those bands. At most two
	// lines fit -- a band offers innerMargin 10 - outerMargin 3 = 7mm, and
	// three 3mm lines need 9mm and run off the plate edge (spec 4.3).
	l.bottomY = params.F(85 - innerMargin)
	if plate.SeedFP != "" {
		l.topLines = append(l.topLines, "SEED FP: "+passphrase.GroupFingerprint(plate.SeedFP))
	}
	if plate.CombinedFP != "" {
		l.topLines = append(l.topLines, "EXPECTED COMB FP: "+passphrase.GroupFingerprint(plate.CombinedFP))
	}
	if strings.ContainsRune(plate.Passphrase, ' ') {
		l.bottomLines = append(l.bottomLines, passphraseLegend)
	}
	if plate.SeedFP != "" || plate.CombinedFP != "" {
		l.bottomLines = append(l.bottomLines, passphraseFooter)
	}
	gap := 0
	if qrDim > 0 {
		l.rowLen = passphraseRowLenQR
		l.em = params.F(passphraseFontSizeQR)
		l.envSize = passphraseQREnvelope * params.StrokeWidth * passphraseQRScale
		l.qrSize = qrDim * params.StrokeWidth * passphraseQRScale
		gap = params.I(passphraseQRGap)
	}
	l.rows = (len(glyphs) + l.rowLen - 1) / l.rowLen
	// blockW is the width of a FULL row, not of the longest row: every row is
	// left-aligned on the same edge, so a short final row must not shift the
	// block.
	l.blockW = l.rowLen * passphraseAdvance(font, l.em)
	l.blockH = l.rows * l.em

	// Centre text block, gap and QR envelope as one group. innerMargin is
	// symmetric, so centring on the plate is centring in the usable area.
	total := l.blockH + gap + l.envSize
	l.textX = (plateDims.X - l.blockW) / 2
	l.textY = (plateDims.Y - total) / 2
	if qrDim > 0 {
		l.envX = (plateDims.X - l.envSize) / 2
		l.envY = l.textY + l.blockH + gap
		l.qrX = l.envX + (l.envSize-l.qrSize)/2
		l.qrY = l.envY + (l.envSize-l.qrSize)/2
	}
	return l
}

func engravePassphrase(params engrave.Params, plate Passphrase, qrc *engrave.ConstantQRCmd) engrave.Engraving {
	qrDim := 0
	if qrc != nil {
		qrDim = qrc.Size
	}
	l := passphraseLayoutFor(params, plate, qrDim)
	// NewPassphraseStringer, never NewConstantStringer: the shared alphabet is
	// 36 characters and panics on lowercase (spec 3.5.1).
	constant := engrave.NewPassphraseStringer(plate.Font, params, l.em)
	plateX := params.F(85)
	// band engraves centred metadata lines downwards from y.
	band := func(t engrave.Transform, y int, lines []string) {
		for i, line := range lines {
			s := engrave.String(plate.Font, l.smallEm, line)
			w, _ := s.Measure()
			t.Offset((plateX-w)/2, y+i*l.smallEm)
			s.Engrave(t.Yield)
		}
	}
	return func(yield func(engrave.Command) bool) {
		t := engrave.NewTransform(yield)
		band(t, l.topY, l.topLines)

		off := t.Offset(l.textX, l.textY)
		stringColumn(off, constant, plate.Font, l.em, l.glyphs, l.rowLen, 0, l.rows)

		if qrc != nil {
			qrCmd := qrc.Engrave(params.StepperConfig, params.StrokeWidth, passphraseQRScale)
			t.Offset(l.qrX, l.qrY)
			qrCmd(t.Yield)
		}

		band(t, l.bottomY, l.bottomLines)
	}
}
