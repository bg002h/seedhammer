package backup

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"

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
//
// A plate may also be cut in more than one face, one run of rows each: see
// Block, FitBlocks and EngraveFitted, of which the single-face functions above
// are the one-block case.
var FreeTextFont *vector.Face = sh.Font

// ErrTooLarge means the composition does not fit one plate even at the smallest
// rung. Text is refused, never split across plates (spec 6, user directive).
var ErrTooLarge = errors.New("backup: text does not fit a plate")

// Block is one run of consecutive body rows cut in a SINGLE engraving face.
//
// A plate is a list of blocks in plate order, top to bottom. One block is the
// ordinary free-text plate; two make a MIXED-FACE plate, which exists so a
// single piece of steel can qualify both shipped faces at 3.0mm instead of
// costing one plate each.
//
// Text may contain '\n' -- it wraps by the same rules as any other block text.
// The composition text is the block texts joined by '\n' (CompositionText), so a
// caller that split a string into blocks gets exactly that string back, and the
// QR the plate carries is a copy of what is engraved rather than of a fragment.
//
// SizeMM is the size this block's rows are cut at. Zero means "the size the
// plate is fitted at", which is what every caller but FitSized passes, so the
// zero value preserves every existing composition and every golden.
//
// SizeMM is data the composition CARRIES; the wrap never reads it. FitSized is
// the one function that resolves it into the per-block sizes the wrap is
// handed, and every other entry point passes its own single rung for every
// block. FitBlocks and FitBlocksAt therefore ignore it outright.
type Block struct {
	Face   *vector.Face
	Text   string
	SizeMM float32
}

// CompositionText is the whole composition as one string: the block texts joined by '\n',
// which is the boundary WrapText already breaks on. This is what the QR encodes
// and what capacity is measured against.
func CompositionText(blocks []Block) string {
	if len(blocks) == 1 {
		return blocks[0].Text
	}
	parts := make([]string, len(blocks))
	for i, b := range blocks {
		parts[i] = b.Text
	}
	return strings.Join(parts, "\n")
}

// Fitted is one composition laid out for one plate, and it is the ONLY output
// of the fit. The confirm screen renders it, the engraver engraves it and the
// readout counts it -- three consumers of ONE result, never three
// re-derivations that ought to agree.
//
// Faces is parallel to Lines: Faces[i] is the face line i is cut in, and with a
// mixed-face plate the per-line CHARACTER BUDGET the line was wrapped to
// depends on it (font/sh is 44 columns at 3.0mm, font/constant 39). Recomputing
// that mapping anywhere downstream is the one way this feature can silently
// engrave lines wide of the grid they were broken to, so nothing downstream
// may: read Faces.
//
// Sizes is parallel to Lines for the same reason and under the same
// prohibition. It is ALWAYS populated, uniform plates included, so the engraver
// has one path and the existing goldens prove that path reproduces the special
// case rather than running beside it.
type Fitted struct {
	// Mixed is true when the plate does NOT cut everything at one size --
	// counting the title and the footer, so !Mixed means literally every glyph
	// on the plate is one size, which is exactly what makes SizeMM printable.
	//
	// Mixed rather than Uniform because the zero value has to be the safe,
	// legacy branch: every hand-built Fitted literal leaves it false, and false
	// must mean "one size everywhere", which is what those literals are.
	Mixed bool
	// SizeMM is valid only when !Mixed. A reader that prints it regardless
	// prints "0.0mm" for a mixed plate, which is a defect and not a fallback.
	SizeMM float32
	Sizes  []float32
	Lines  []string
	Faces  []*vector.Face
	QR     *qr.Code

	// The screw-hole rows. Title takes plate row 0 and Footer the last row,
	// each when non-empty. Their faces are the faces of the blocks they border
	// -- see FitBlocks.
	Title, Footer         string
	TitleFace, FooterFace *vector.Face

	// TitleSizeMM and FooterSizeMM are EXPLICIT, never inherited from the
	// blocks they border: the title of a size-ladder plate sits at the side's
	// SMALLEST rung, not at the first block's. Each is non-zero exactly when
	// its string is non-empty; EngraveFitted panics on any other combination,
	// because LinesPerPlate(params, 0) is a divide by zero and a title at
	// fontSize 0 is one too.
	TitleSizeMM, FooterSizeMM float32

	// qrAt is the code's resolved placement: computed ONCE, by the fit, and
	// never re-derived. It is nil exactly when QR is nil.
	//
	// Unexported because the only Fitted literals outside this package feed the
	// readout and never engrave, and a literal that set QR without a placement
	// and then engraved it must fail loudly rather than draw a code at a y
	// nobody computed.
	qrAt *qrPlacement
}

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

// wrapBlocks is THE wrap, and the only place a block's face is turned into a
// per-line character budget.
//
// Each block is wrapped in its OWN face, starting at the plate row the previous
// block ended on, so the face boundary falls exactly on the content boundary
// and the budget of every line is the budget of the face that line is cut in.
// The returned faces slice is parallel to lines, and is the mapping every
// consumer of the fit reads instead of recomputing.
//
// With a single block this is exactly the arithmetic Fit has always done: one
// textLayout, one WrapText over [start, end).
//
// qrp is the plate's ONE resolved placement, shared by every block: the code is
// one object on one plate and does not belong to a block, so a caller resolves
// it once and every block narrows against the same band.
func wrapBlocks(params engrave.Params, blocks []Block, fontMM float32, qrp *qrPlacement, start, end int) (lines []string, faces []*vector.Face, ok bool) {
	row := start
	for _, b := range blocks {
		lay := textLayout(params, b.Face, params.F(fontMM), params.I(outerMargin), qrp)
		l, fits := WrapText(b.Text, widthFor(lay, row), end-row)
		if !fits {
			return nil, nil, false
		}
		lines = append(lines, l...)
		for range l {
			faces = append(faces, b.Face)
		}
		row += len(l)
	}
	return lines, faces, true
}

// FitBlocks is the largest rung whose layout holds the whole composition, with
// each block cut in its own face.
//
// The title takes the face of the FIRST block and the footer the face of the
// LAST. Both sit on screw-hole rows at the very top and the very bottom of the
// plate, so this makes the plate read as contiguous runs -- each screw-hole row
// is cut in the face of the half it borders -- and with one block it is simply
// "the plate's face", unchanged.
//
// It returns the QR CODE ITSELF, not just its size, so the artifact engraved is
// the very object the fit measured: there is exactly one encode per
// composition, and no caller can re-encode with different parameters and
// disagree.
func FitBlocks(params engrave.Params, blocks []Block, title, footer string, useQR bool) (Fitted, error) {
	if len(blocks) == 0 {
		return Fitted{}, ErrTooLarge
	}
	qrc, err := qrFor(CompositionText(blocks), useQR)
	if err != nil {
		return Fitted{}, err
	}
	for _, size := range FontSizes {
		f, err := fitBlocksAt(params, blocks, title, footer, qrc, size)
		if err != nil {
			continue
		}
		return f, nil
	}
	return Fitted{}, ErrTooLarge
}

// FitBlocksAt is FitBlocks at ONE rung: it engraves at size or refuses, and
// never quietly picks a different one.
//
// Auto-fit answers "as large as possible", which is the right question for a
// plate whose content is fixed. It is the wrong question when the SIZE is what
// the operator chose and the content is what gives way -- a proof pattern
// trimmed to reach 4.4mm also fits at 5.0mm, so FitBlocks would engrave the
// trimmed pattern at 5.0 and the operator would get neither the size they asked
// for nor the content they gave up.
//
// size must be a rung in FontSizes: those are the sizes every capacity number
// in this package is measured at, and an arbitrary one would lay out fine while
// matching nothing any test pins.
func FitBlocksAt(params engrave.Params, blocks []Block, title, footer string, useQR bool, size float32) (Fitted, error) {
	if len(blocks) == 0 {
		return Fitted{}, ErrTooLarge
	}
	if !slices.Contains(FontSizes, size) {
		return Fitted{}, fmt.Errorf("backup: %.1fmm is not a rung in FontSizes %v", size, FontSizes)
	}
	qrc, err := qrFor(CompositionText(blocks), useQR)
	if err != nil {
		return Fitted{}, err
	}
	return fitBlocksAt(params, blocks, title, footer, qrc, size)
}

// fitBlocksAt is the single rung attempt both entry points share, so the
// laddered and the fixed-size paths cannot lay a plate out differently.
//
// It fills Sizes with one entry per line, all at size, and sets TitleSizeMM and
// FooterSizeMM to size exactly when the corresponding string is non-empty. That
// is what makes this the ONE-SIZE CASE of the general path rather than a second
// path beside it: Mixed stays false and every golden that runs through here is
// engraved by the same code a mixed plate will be.
func fitBlocksAt(params engrave.Params, blocks []Block, title, footer string, qrc *qr.Code, size float32) (Fitted, error) {
	rows := LinesPerPlate(params, size)
	start, end := bodyRows(rows, title, footer)
	// The placement is resolved BEFORE the wrap, because the wrap narrows
	// against it -- but its bound is checked AFTER, so ErrTooLarge still wins
	// when both would fire and every reachable case returns exactly what it
	// returned before this change.
	qrAt := qrPlacementFor(params, qrc, params.F(size))
	lines, faces, ok := wrapBlocks(params, blocks, size, qrAt, start, end)
	if !ok {
		return Fitted{}, ErrTooLarge
	}
	if err := qrFitsPlate(params, qrAt); err != nil {
		return Fitted{}, err
	}
	sizes := make([]float32, len(lines))
	for i := range sizes {
		sizes[i] = size
	}
	var titleSize, footerSize float32
	if title != "" {
		titleSize = size
	}
	if footer != "" {
		footerSize = size
	}
	return Fitted{
		SizeMM:       size,
		Sizes:        sizes,
		Lines:        lines,
		Faces:        faces,
		QR:           qrc,
		qrAt:         qrAt,
		Title:        title,
		Footer:       footer,
		TitleFace:    blocks[0].Face,
		FooterFace:   blocks[len(blocks)-1].Face,
		TitleSizeMM:  titleSize,
		FooterSizeMM: footerSize,
	}, nil
}

// ErrQRTooTall means the code's keep-out band runs past the bottom margin, so
// there is no room below it for the rows the plate is supposed to carry.
//
// It is a property of the COMPOSITION, which is why it comes back from the fit
// as an error rather than out of the engraver as a panic: EngraveFitted is
// reached only AFTER the confirm screen, so a check that belongs at the fit but
// lives in the engraver crashes mid-flow with a plate clamped in the machine.
var ErrQRTooTall = errors.New("backup: the QR band runs past the bottom margin")

// qrPlacementFor resolves the free-text plate's ONE code placement, anchored at
// the plate's top margin -- one code on one plate, whatever block a row belongs
// to.
//
// Returns nil when there is no code, which is the nil half of the "qrAt is nil
// exactly when QR is nil" invariant, satisfied at the constructor.
//
// The bottom-margin bound is qrFitsPlate's, not this function's, because the
// wrap needs the placement whether or not it fits and the ordering of the two
// refusals is the caller's to choose; see fitBlocksAt.
func qrPlacementFor(params engrave.Params, qrc *qr.Code, fontSize int) *qrPlacement {
	if qrc == nil {
		return nil
	}
	p := qrPlaceAt(params, qrc, freeTextQRScale, fontSize, params.I(outerMargin))
	return &p
}

// qrFitsPlate is ErrQRTooTall's one test: the band must end above the bottom
// margin, or there is no room below it for the rows the plate carries.
func qrFitsPlate(params engrave.Params, qrp *qrPlacement) error {
	if qrp != nil && qrp.Bottom > params.F(plateSize)-params.I(outerMargin) {
		return ErrQRTooTall
	}
	return nil
}

// Fit is FitBlocks for a composition cut entirely in one face. The same text
// fits differently in different faces, so fnt must be the face the composition
// will be ENGRAVED in; see FreeTextFont.
func Fit(params engrave.Params, fnt *vector.Face, text, title, footer string, qr bool) (fontMM float32, lines []string, qrc *qr.Code, err error) {
	f, err := FitBlocks(params, []Block{{Face: fnt, Text: text}}, title, footer, qr)
	if err != nil {
		return 0, nil, nil, err
	}
	return f.SizeMM, f.Lines, f.QR, nil
}

// AdmissibleBlocks is spec 6's anchor: 3.0mm, the QR as chosen, and BOTH the
// title and footer rows reserved whether or not they are used.
//
// Reserving unconditionally is what makes admission monotone -- title and
// footer are deliberately NOT read, so entering a title after the text can
// never retroactively invalidate text already accepted. linesAvail is defined
// even when ok is false, so the readout can show "lines used / lines available"
// over capacity.
func AdmissibleBlocks(params engrave.Params, blocks []Block, title, footer string, useQR bool) (linesUsed, linesAvail int, ok bool) {
	size := FontSizes[len(FontSizes)-1]
	rows := LinesPerPlate(params, size)
	linesAvail = rows - 2
	if linesAvail < 0 {
		linesAvail = 0
	}
	qrc, err := qrFor(CompositionText(blocks), useQR)
	if err != nil {
		// An unencodable text is inadmissible, but the readout keeps working:
		// linesAvail is already meaningful, and lines used is unknowable
		// without a code to lay out around.
		return 0, linesAvail, false
	}
	// The code is anchored at the plate's TOP MARGIN, not at the row the body
	// starts on. This function reserves a title row unconditionally, which
	// moves the first row of TEXT; it does not move the CODE. Anchoring at the
	// body's start row instead shifts the whole band down one row, swapping
	// charPerLine for charPerQRLine at both band edges, and the verdict this
	// returns gates OK on the text and confirm steps for every ordinary
	// QR-carrying plate.
	qrAt := qrPlacementFor(params, qrc, params.F(size))
	// Unbounded on purpose: a refusal that reported "26 / 26" for a text
	// needing 300 lines would tell the operator nothing about how much to cut.
	// The row a block starts on still decides its budget, so the blocks are
	// counted in plate order from row 1 exactly as they are laid out.
	l, _, _ := wrapBlocks(params, blocks, size, qrAt, 1, math.MaxInt)
	return len(l), linesAvail, len(l) <= linesAvail
}

// Admissible is AdmissibleBlocks for a composition cut entirely in one face.
func Admissible(params engrave.Params, fnt *vector.Face, text, title, footer string, qr bool) (linesUsed, linesAvail int, ok bool) {
	return AdmissibleBlocks(params, []Block{{Face: fnt, Text: text}}, title, footer, qr)
}

// rowFaces is which face each of the plate's rows would be cut in, for a
// composition laid out from row 0 with no bound. Rows past the end of the text
// take the LAST block's face, because that is the block that would grow into
// them.
//
// A one-block composition is answered without wrapping anything: every row is
// that block's face, which is what the single-face capacity solver has always
// assumed.
func rowFaces(params engrave.Params, blocks []Block, fontMM float32, qrp *qrPlacement, rows int) []*vector.Face {
	out := make([]*vector.Face, rows)
	last := blocks[len(blocks)-1].Face
	var faces []*vector.Face
	if len(blocks) > 1 {
		_, faces, _ = wrapBlocks(params, blocks, fontMM, qrp, 0, math.MaxInt)
	}
	for r := range rows {
		if r < len(faces) {
			out[r] = faces[r]
		} else {
			out[r] = last
		}
	}
	return out
}

// faceLayouts is the per-face lineLayout cache. A composition has one or two
// faces, so a linear scan beats a map and allocates nothing.
type faceLayouts struct {
	faces []*vector.Face
	lays  []lineLayout
}

func (c *faceLayouts) at(params engrave.Params, fnt *vector.Face, fontSize int, qrp *qrPlacement) lineLayout {
	for i, f := range c.faces {
		if f == fnt {
			return c.lays[i]
		}
	}
	lay := textLayout(params, fnt, fontSize, params.I(outerMargin), qrp)
	c.faces = append(c.faces, fnt)
	c.lays = append(c.lays, lay)
	return lay
}

// MaxCharsAtBlocks is the capacity solver behind the refusal message's live
// figure: how many characters fit at fontMM given the QR that THIS composition
// produces.
//
// The QR's size comes from encoding the text, never from a length table, which
// is the whole point: at 3.0mm with a 700-character text, spec 4's geometry
// column suggests dropping the QR frees about 135 characters and the true
// figure is several times that. Returns 0 if the text cannot be encoded.
//
// A mixed-face plate has no single column count, so each row is counted in the
// face that row would be cut in. Returns 0 if the text cannot be encoded.
func MaxCharsAtBlocks(params engrave.Params, blocks []Block, fontMM float32, useQR bool) int {
	if len(blocks) == 0 {
		return 0
	}
	qrc, err := qrFor(CompositionText(blocks), useQR)
	if err != nil {
		return 0
	}
	rows := LinesPerPlate(params, fontMM)
	// One placement for the plate, anchored at the top margin, shared by the
	// wrap inside rowFaces and by every row counted below -- the same code, at
	// the same y, for both halves of this answer.
	qrAt := qrPlacementFor(params, qrc, params.F(fontMM))
	faces := rowFaces(params, blocks, fontMM, qrAt, rows)
	var lays faceLayouts
	total := 0
	for row := range rows {
		n, _ := lays.at(params, faces[row], params.F(fontMM), qrAt).at(row)
		total += n
	}
	return total
}

// MaxCharsAt is MaxCharsAtBlocks for a composition cut entirely in one face.
func MaxCharsAt(params engrave.Params, fnt *vector.Face, fontMM float32, text string, qr bool) int {
	return MaxCharsAtBlocks(params, []Block{{Face: fnt, Text: text}}, fontMM, qr)
}
