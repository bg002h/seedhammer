package backup

import (
	"errors"
	"fmt"
	"image"
	"strings"

	qr "github.com/seedhammer/kortschak-qr"
	"seedhammer.com/engrave"
	"seedhammer.com/font/vector"
)

// HashlockForm selects the band text and the body of a hashlock plate (H6 §6.2).
type HashlockForm int

const (
	// HashlockString cuts the ms1 kind-0x03 plate string, text only.
	HashlockString HashlockForm = iota
	// HashlockPhrase cuts the hashlock phrase and its method, optionally with
	// a QR of §8.6's text.
	HashlockPhrase
)

// Hashlock is a hashlock preimage plate: a DEDICATED layout, never a free-text
// plate and never validateMdmkStrings' single-string arm, whose TEXT + QR /
// QR ONLY options would QR-encode the ms1 string itself (decision 1 forbids
// it).
//
// It is not a seed plate and it is not a bundleCard: it does not travel with
// the policy set, and the footer band says NOT A SEED on both forms.
type Hashlock struct {
	// Form selects the band text and the body (§6.2).
	Form HashlockForm
	// MS1 is the kind-0x03 plate string, for HashlockString. Engraved
	// VERBATIM: lowercase and UNGROUPED, which is what the goldens pin. The
	// fork's ms1 SEED plate does the opposite (EngraveSeedString upper-cases
	// and groups in tens) and so does `ms hashlock`'s engraving card, so
	// "verbatim" is a decision here and not an oversight -- grouped in tens the
	// 75 characters take 8 rows at 6.0mm instead of 4 and the plate no longer
	// fits at that rung.
	MS1 string
	// Phrase and Method are for HashlockPhrase. Phrase is engraved VERBATIM,
	// with every space rendered as SpaceMark; Method is §8.6's line.
	Phrase string
	Method string
	// Locator rows, pre-formatted by the caller (§6.3). backup takes no
	// dependency on md or hashlock, exactly as Passphrase takes none.
	Locator []string
	// QR is opt-in and legal only on HashlockPhrase (§6.4). It encodes §8.6's
	// text, which the caller supplies as QRText -- NEVER the ms1 string.
	QR     bool
	QRText string
	Font   *vector.Face
}

const (
	hashlockTitleString = "HASHLOCK PREIMAGE"
	hashlockTitlePhrase = "HASHLOCK PHRASE"
	hashlockFooter      = "NOT A SEED"

	// hashlockQRScale is 2, NOT the passphrase plate's 3. MEASURED: at 53
	// modules scale 3 is 47.70mm and does not fit at any rung (3.0mm gives
	// 79.70mm against a 65mm budget); scale 2 is 31.80mm. The pitch matches the
	// free-text plate's (freeTextQRScale = 2, "0.6mm modules against the 0.9mm
	// every other plate uses") but not its code path: this QR is a spend secret
	// and goes through engrave.ConstantQR, never engrave.QR.
	hashlockQRScale = 2
	// hashlockQREnvelope is the reserved module count -- v9, the §8.6 worst
	// case (a 100-character phrase under the hardened method line is 194 bytes
	// and encodes at ECC-L to 53 modules). The size is VARIABLE with the phrase
	// length, so the layout reserves the worst case and centres the actual code
	// inside it, exactly as passphraseQREnvelope does.
	hashlockQREnvelope = 53
	// hashlockQRGap separates the text block from the QR, in millimetres.
	hashlockQRGap = 2
)

// ErrHashlockTooLarge is EngraveHashlock's refusal: the composition does not
// lay out at any rung of FontSizes.
//
// It REFUSES rather than draws what it cannot lay out, the way EngraveText,
// EngraveSeed and EngraveSeedString do, because the alternative lands on the
// device: a plate is cut from whatever this returns, and there is no camera to
// read it back.
var ErrHashlockTooLarge = errors.New("backup: the hashlock plate does not fit at any font size")

// ErrHashlockQRForm is the refusal of a QR on the string form. Decision 1
// declines a QR of the ms1 string, so the only machine-readable copy this
// plate ever carries is §8.6's text on the phrase form.
var ErrHashlockQRForm = errors.New("backup: a QR is legal only on the phrase form of a hashlock plate")

// hashlockRow is one PHYSICAL row of the body: the text as it will be
// engraved, plus whether it carries secret material.
//
// The split decides the ENGRAVER, not the wrap. A secret row goes through the
// constant-time passphrase stringer, exactly as the passphrase plate's own body
// does; the method line and the locator carry a digest, a stub and a parameter
// set, none of which is secret, and they go through the ordinary variable-time
// engrave.String the passphrase plate's metadata bands already use.
type hashlockRow struct {
	text   string
	secret bool
}

type hashlockLayout struct {
	fontMM float32
	em     int
	// rows is the wrapped body, in engraving order.
	rows []hashlockRow
	// textX, textY is the top-left of the body; blockH its height.
	textX, textY int
	blockH       int
	// qrDim is the code's module count, or 0. envSize is the reserved
	// hashlockQREnvelope-module square it is centred in.
	qrDim               int
	qrX, qrY, qrSize    int
	envX, envY, envSize int
	// total is blockH + gap + envSize, the height that must fit budget.
	total, budget int

	smallEm       int
	topY, bottomY int
	topLines      []string
	bottomLines   []string
}

// hashlockWrap splits s into rows of at most n CHARACTERS. It never breaks on
// word boundaries, and that is forced rather than chosen: a 100-character
// phrase and a 75-character ms1 string are SINGLE TOKENS, so a word wrapper has
// nothing to break them on and would either fail or overflow. The visible
// consequence is that an 8-hex stub splits mid-token at the narrow rungs, which
// is accepted.
func hashlockWrap(s string, n int) []string {
	if n <= 0 {
		return nil
	}
	if s == "" {
		return []string{""}
	}
	var out []string
	for len(s) > n {
		out = append(out, s[:n])
		s = s[n:]
	}
	return append(out, s)
}

// hashlockSegments is the body in LOGICAL rows, before wrapping (§6.2, §6.3).
//
// The method line is the FIRST body row of the phrase form, directly beneath
// the title band; the locator follows it; a blank separates each block from the
// next; the secret is LAST, so a reader knows where it ends.
func hashlockSegments(plate Hashlock) []hashlockRow {
	var segs []hashlockRow
	if plate.Form == HashlockPhrase && plate.Method != "" {
		segs = append(segs, hashlockRow{text: plate.Method})
		segs = append(segs, hashlockRow{text: ""})
	}
	for _, l := range plate.Locator {
		segs = append(segs, hashlockRow{text: l})
	}
	if len(segs) > 0 {
		segs = append(segs, hashlockRow{text: ""})
	}
	switch plate.Form {
	case HashlockPhrase:
		segs = append(segs, hashlockRow{text: passphraseGlyphs(plate.Phrase), secret: true})
	default:
		segs = append(segs, hashlockRow{text: plate.MS1, secret: true})
	}
	return segs
}

func hashlockLayoutFor(params engrave.Params, plate Hashlock, fontMM float32, qrDim int) hashlockLayout {
	plateDims := image.Point{X: params.F(plateSize), Y: params.F(plateSize)}
	l := hashlockLayout{
		fontMM:  fontMM,
		em:      params.F(fontMM),
		qrDim:   qrDim,
		smallEm: params.F(plateSmallFontSize),
		topY:    params.F(outerMargin),
		bottomY: params.F(plateSize - innerMargin),
		// The vertical budget for the centred group: the plate less the two
		// innerMargin bands, which hold the title and the footer.
		budget: params.F(plateSize) - 2*params.I(innerMargin),
	}
	switch plate.Form {
	case HashlockPhrase:
		l.topLines = []string{hashlockTitlePhrase}
	default:
		l.topLines = []string{hashlockTitleString}
	}
	if plate.Form == HashlockPhrase && strings.ContainsRune(plate.Phrase, ' ') {
		// The SAME legend the passphrase plate draws, with the same reason:
		// one space and two look identical on steel, and "Correct Horse" and
		// "correct horse" derive different preimages.
		l.bottomLines = append(l.bottomLines, passphraseLegend)
	}
	l.bottomLines = append(l.bottomLines, hashlockFooter)

	perLine := CharsPerLine(params, plate.Font, fontMM)
	for _, seg := range hashlockSegments(plate) {
		for _, r := range hashlockWrap(seg.text, perLine) {
			l.rows = append(l.rows, hashlockRow{text: r, secret: seg.secret})
		}
	}
	l.blockH = len(l.rows) * l.em

	gap := 0
	if qrDim > 0 {
		l.envSize = hashlockQREnvelope * params.StrokeWidth * hashlockQRScale
		l.qrSize = qrDim * params.StrokeWidth * hashlockQRScale
		gap = params.I(hashlockQRGap)
	}
	l.total = l.blockH + gap + l.envSize

	// Centre body, gap and QR envelope as one group on the plate. innerMargin
	// is symmetric, so centring on the plate is centring in the usable area.
	l.textX = params.I(outerMargin)
	l.textY = (plateDims.Y - l.total) / 2
	if qrDim > 0 {
		l.envX = (plateDims.X - l.envSize) / 2
		l.envY = l.textY + l.blockH + gap
		l.qrX = l.envX + (l.envSize-l.qrSize)/2
		l.qrY = l.envY + (l.envSize-l.qrSize)/2
	}
	return l
}

func (l hashlockLayout) fits() bool { return l.total <= l.budget }

// HashlockQRCode encodes §8.6's text at ECC-L -- the text, never the ms1
// string, and never the SpaceMark-substituted glyphs (a scanner that saw the
// mark would hand a reader different bytes).
//
// ECC-L is pinned, not M: the engraved text is the authoritative copy and the
// QR is convenience, so four fewer modules is the better trade -- and at the
// 194-byte worst case ECC-L is already at v9, the ceiling ConstantQR was raised
// to.
func HashlockQRCode(plate Hashlock) (*qr.Code, error) {
	return qr.Encode(plate.QRText, qr.L)
}

// EngraveHashlock lays out a hashlock preimage plate, AUTO-FITTING down
// FontSizes and refusing at the bottom rung rather than drawing what it cannot
// lay out.
func EngraveHashlock(params engrave.Params, plate Hashlock) (engrave.Engraving, error) {
	if plate.QR && plate.Form != HashlockPhrase {
		return nil, ErrHashlockQRForm
	}
	var qrc *engrave.ConstantQRCmd
	qrDim := 0
	if plate.QR {
		code, err := HashlockQRCode(plate)
		if err != nil {
			return nil, err
		}
		// ConstantQR, never engrave.QR: the latter engraves in a
		// content-dependent pattern and would leak the phrase through timing.
		qrc, err = engrave.ConstantQR(code)
		if err != nil {
			return nil, err
		}
		qrDim = qrc.Size
	}
	l, ok := hashlockFit(params, plate, qrDim)
	if !ok {
		return nil, fmt.Errorf("%w: %d rows at %.1fmm need %d units against a budget of %d",
			ErrHashlockTooLarge, len(l.rows), l.fontMM, l.total, l.budget)
	}
	return engraveHashlock(params, plate, l, qrc), nil
}

// hashlockFit walks FontSizes largest-first and returns the layout at the first
// rung the whole composition fits. When none fits it returns the BOTTOM rung's
// layout and false, so the refusal can name the measured ceiling.
func hashlockFit(params engrave.Params, plate Hashlock, qrDim int) (hashlockLayout, bool) {
	var l hashlockLayout
	for _, size := range FontSizes {
		l = hashlockLayoutFor(params, plate, size, qrDim)
		if l.fits() {
			return l, true
		}
	}
	return l, false
}

func engraveHashlock(params engrave.Params, plate Hashlock, l hashlockLayout, qrc *engrave.ConstantQRCmd) engrave.Engraving {
	// NewPassphraseStringer, never NewConstantStringer: the shared alphabet is
	// 36 characters and panics on lowercase, and both a phrase and a bech32
	// ms1 string are lowercase.
	constant := engrave.NewPassphraseStringer(plate.Font, params, l.em)
	plateX := params.F(plateSize)
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
		// The bands are engraved VERBATIM, never through TitleString, which
		// upper-cases and truncates at MaxTitleLen -- the three literals are
		// already inside that cap and already upper-case, and running them
		// through it would let a future longer literal be silently cut.
		band(t, l.topY, l.topLines)

		y := l.textY
		for _, r := range l.rows {
			if r.text != "" {
				off := t.Offset(l.textX, y)
				if r.secret {
					constant.String(off.Yield, r.text)
				} else {
					engrave.String(plate.Font, l.em, r.text).Engrave(off.Yield)
				}
			}
			y += l.em
		}

		if qrc != nil {
			qrCmd := qrc.Engrave(params.StepperConfig, params.StrokeWidth, hashlockQRScale)
			t.Offset(l.qrX, l.qrY)
			qrCmd(t.Yield)
		}

		band(t, l.bottomY, l.bottomLines)
	}
}
