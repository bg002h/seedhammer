package backup

import (
	"image"
	"strings"

	"seedhammer.com/engrave"
	"seedhammer.com/font/vector"
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

// Layout constants, pinned per spec 4.1. The em is fixed per layout mode and
// does NOT scale with passphrase length: a 20-character passphrase occupies 2
// rows at the same em as a 100-character one.
const (
	// passphraseFontSize gives a lowercase x-height of ~9 stroke widths,
	// comparable to what uppercase gets at today's 4.1mm.
	passphraseFontSize = 6.0
	// passphraseRowLen is one 10-character group per row: position implies
	// index, and there are no intra-row gaps to confuse with the space mark.
	passphraseRowLen = 10
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

func EngravePassphrase(params engrave.Params, plate Passphrase) (engrave.Engraving, error) {
	return engravePassphrase(params, plate), nil
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

func passphraseLayoutFor(params engrave.Params, font *vector.Face, glyphs string) passphraseLayout {
	plateDims := image.Point{X: params.F(85), Y: params.F(85)}
	l := passphraseLayout{
		rowLen: passphraseRowLen,
		em:     params.F(passphraseFontSize),
	}
	l.rows = (len(glyphs) + l.rowLen - 1) / l.rowLen
	// blockW is the width of a FULL row, not of the longest row: every row is
	// left-aligned on the same edge, so a short final row must not shift the
	// block.
	l.blockW = l.rowLen * passphraseAdvance(font, l.em)
	l.blockH = l.rows * l.em
	l.textX = (plateDims.X - l.blockW) / 2
	l.textY = (plateDims.Y - l.blockH) / 2
	return l
}

func engravePassphrase(params engrave.Params, plate Passphrase) engrave.Engraving {
	// NewPassphraseStringer, never NewConstantStringer: the shared alphabet is
	// 36 characters and panics on lowercase (spec 3.5.1).
	glyphs := passphraseGlyphs(plate.Passphrase)
	l := passphraseLayoutFor(params, plate.Font, glyphs)
	constant := engrave.NewPassphraseStringer(plate.Font, params, l.em)
	return func(yield func(engrave.Command) bool) {
		t := engrave.NewTransform(yield)
		off := t.Offset(l.textX, l.textY)
		stringColumn(off, constant, plate.Font, l.em, glyphs, l.rowLen, 0, l.rows)
	}
}
