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

	// Passes is how many times each glyph is engraved in place. 0 and 1 mean
	// once. Only the FREE-TEXT path constructs a Fitted, which is what keeps a
	// pass count away from seed and passphrase plates.
	Passes int
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

// widthFor turns a plate layout into the widthAt closure WrapText wants:
// indexed by OUTPUT line, with the plate-row offset applied inside.
func widthFor(lay lineLayout, startRow int) func(int) int {
	return func(i int) int {
		n, _ := lay.at(startRow + i)
		return n
	}
}

// footerRowY is the footer's OWN top y, in device units.
//
// It is a function rather than an expression because the body's budget is READ
// OFF IT (yBudget) and the engraver cuts the footer AT it (EngraveFitted): one
// expression with two readers, so the row the body is refused above and the row
// the footer is engraved on cannot be two different rows. A naive bottom anchor
// differs from it by the LinesPerPlate remainder -- 1.000mm at 3.0mm, 3.000mm
// at 3.8mm -- and moves every existing golden.
//
// It divides by footerSizeMM, so every caller must have established that there
// IS a footer first; see yBudget.
func footerRowY(params engrave.Params, footerSizeMM float32) int {
	return params.I(outerMargin) + (LinesPerPlate(params, footerSizeMM)-1)*params.F(footerSizeMM)
}

// yBudget is the vertical window the body may use, in DEVICE units: the y its
// first row starts at, and the y no row's bottom may pass.
//
// The limit is read off the footer rather than computed from the bottom margin.
// plateHeight - margin - F(footerSizeMM) sits BELOW the footer's own ink and
// lays the last body row over it -- measured, by 3.000mm at 3.8mm and 4.200mm
// at 4.4mm. Uniform plates escape it only because the old row counting never
// consulted either formula, so no golden would have moved.
//
// BOTH branches test the STRING, never the size: spec 5's mandated no-footer
// plate carries a footerSizeMM of 0, and LinesPerPlate would divide by it.
func yBudget(params engrave.Params, title, footer string, titleSizeMM, footerSizeMM float32) (start, limit int) {
	start = params.I(outerMargin)
	if title != "" {
		start += params.F(titleSizeMM)
	}
	limit = params.F(plateSize) - params.I(outerMargin)
	if footer != "" {
		limit = footerRowY(params, footerSizeMM)
	}
	return start, limit
}

// wrapBlocks is THE wrap, and the only place a block's face is turned into a
// per-line character budget.
//
// Each block is wrapped in its OWN face at its OWN size, starting at the y the
// previous block ended on, so the face boundary falls exactly on the content
// boundary and the budget of every line is the budget of the face that line is
// cut in. The returned faces and rowSizes slices are parallel to lines, and are
// the mapping every consumer of the fit reads instead of recomputing.
//
// sizes is parallel to BLOCKS and is the only channel a per-block size travels
// on: Block.SizeMM is data the composition carries and the wrap never reads it.
//
// start and limit are device-unit y, NOT row indices. Each block is laid out at
// the running y, and its rows are indexed from 0 within the block -- so a row
// index never has to mean the same thing to two blocks that start at different
// heights. A row is admitted iff its BOTTOM is at or above limit; math.MaxInt is
// the unbounded value the untruncated callers pass.
//
// qrp is the plate's ONE resolved placement, shared by every block: the code is
// one object on one plate and does not belong to a block, so a caller resolves
// it once and every block narrows against the same band. That band is
// plate-absolute, which is what lets a block be laid out at its own baseY
// without moving the code.
func wrapBlocks(params engrave.Params, blocks []Block, sizes []float32, qrp *qrPlacement, start, limit int) (lines []string, faces []*vector.Face, rowSizes []float32, ok bool) {
	y := start
	for i, b := range blocks {
		fontSize := params.F(sizes[i])
		lay := textLayout(params, b.Face, fontSize, y, qrp)
		// Row j of this block occupies [y+j*fontSize, y+(j+1)*fontSize), so the
		// rows that fit are those with j+1 <= (limit-y)/fontSize. On a uniform
		// plate that is exactly the end-row count this replaced, because
		// (limit - y) is a whole number of rows either way.
		avail := 0
		if limit > y {
			avail = (limit - y) / fontSize
		}
		l, fits := WrapText(b.Text, widthFor(lay, 0), avail)
		if !fits {
			return nil, nil, nil, false
		}
		lines = append(lines, l...)
		for range l {
			faces = append(faces, b.Face)
			rowSizes = append(rowSizes, sizes[i])
		}
		y += len(l) * fontSize
	}
	return lines, faces, rowSizes, true
}

// uniformSizes is one size per BLOCK, for the entry points that fit a whole
// composition at a single rung. FitSized is the one function that resolves
// per-block sizes; everything else passes its own rung for every block, which
// is what keeps spec 6's admission anchor true.
func uniformSizes(blocks []Block, size float32) []float32 {
	sizes := make([]float32, len(blocks))
	for i := range sizes {
		sizes[i] = size
	}
	return sizes
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
	var titleSize, footerSize float32
	if title != "" {
		titleSize = size
	}
	if footer != "" {
		footerSize = size
	}
	start, limit := yBudget(params, title, footer, titleSize, footerSize)
	// The placement is resolved BEFORE the wrap, because the wrap narrows
	// against it -- but its bound is checked AFTER, so ErrTooLarge still wins
	// when both would fire and every reachable case returns exactly what it
	// returned before this change.
	qrAt := qrPlacementFor(params, qrc, params.F(size))
	lines, faces, sizes, ok := wrapBlocks(params, blocks, uniformSizes(blocks, size), qrAt, start, limit)
	if !ok {
		return Fitted{}, ErrTooLarge
	}
	if err := qrFitsPlate(params, qrAt); err != nil {
		return Fitted{}, err
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

// FitSized lays out a composition whose every block states its OWN size, and it
// is the one function that resolves Block.SizeMM (spec 2.2). FitBlocks and
// FitBlocksAt are untouched and go on ignoring it: they fit a whole composition
// at ONE rung, which is what keeps spec 6's admission anchor true.
//
// There is no useQR parameter and no code. The QR band is quantised by a single
// fontSize (spec 2.1), and a plate that mixes sizes has none -- so Fitted.QR and
// its placement are both left nil, which makes spec 2.1.1's "QR != nil implies
// !Mixed" STRUCTURALLY true rather than checked.
//
// ErrQRTooTall, the third of spec 2.1.1's invariants, is therefore never
// returned from here, and that AGREES with fitBlocksAt rather than diverging
// from it. fitBlocksAt resolves the placement before the wrap and checks its
// bottom bound AFTER, so ErrTooLarge wins when both would fire and every
// reachable case returns byte for byte what it returned before this change.
// Here only one of the two refusals can ever fire -- qrFitsPlate(params, nil)
// is unconditionally nil -- so there is no ordering to match and calling it
// would be a call with one possible answer. Spec 7.7(d)'s FitSized half is
// vacuous for the same reason. A future FitSized that took a code would owe
// fitBlocksAt's order: the refusal that is a property of the whole
// composition, ErrTooLarge, first.
//
// Every refusal below comes back as an ERROR and never as a panic. EngraveFitted
// is reached only AFTER the confirm screen, so the same refusal arriving from
// the engraver arrives mid-flow with a plate clamped in the machine.
func FitSized(params engrave.Params, blocks []Block, title, footer string,
	titleSizeMM, footerSizeMM float32) (Fitted, error) {
	if len(blocks) == 0 {
		return Fitted{}, ErrTooLarge
	}
	sizes := make([]float32, len(blocks))
	for i, b := range blocks {
		// The zero that means "the size the plate is fitted at" for every other
		// entry point has no meaning here, and a zero reaching Fitted.Sizes is
		// spec 2.3's per-entry invariant -- which EngraveFitted enforces as a
		// panic, on nothing today. This is the matching ERROR, worded as that
		// panic is and kept separate from the rung check below so a block that
		// simply forgot its size is not reported as one that named a size off
		// the ladder.
		if b.SizeMM == 0 {
			return Fitted{}, fmt.Errorf("backup: block %d is sized 0mm", i)
		}
		if !slices.Contains(FontSizes, b.SizeMM) {
			return Fitted{}, fmt.Errorf("backup: block %d at %.1fmm is not a rung in FontSizes %v",
				i, b.SizeMM, FontSizes)
		}
		sizes[i] = b.SizeMM
	}
	// Spec 2.3's size/string invariant, refused here rather than panicked on in
	// the engraver. A zero size with a non-empty string puts a whole row through
	// the layout at fontSize 0, where LinesPerPlate divides by it; a non-zero
	// size with an empty string is the same bug read the other way round.
	if (title != "") != (titleSizeMM != 0) {
		return Fitted{}, fmt.Errorf("backup: title %q at %.1fmm", title, titleSizeMM)
	}
	if (footer != "") != (footerSizeMM != 0) {
		return Fitted{}, fmt.Errorf("backup: footer %q at %.1fmm", footer, footerSizeMM)
	}
	start, limit := yBudget(params, title, footer, titleSizeMM, footerSizeMM)
	lines, faces, rowSizes, ok := wrapBlocks(params, blocks, sizes, nil, start, limit)
	if !ok {
		return Fitted{}, ErrTooLarge
	}
	// Mixed is MEASURED, never assumed. Every ladder composition genuinely
	// mixes, so hardcoding true here passes every fixture this change ships --
	// and then reports "0.0mm" on the confirm screen for the all-one-rung
	// composition this entry point is public for, which spec 3 calls a defect
	// rather than a fallback.
	//
	// The candidate is the FIRST BLOCK's size rather than rowSizes[0] only so
	// this cannot index an empty slice; the two are the same value, since a
	// successful wrap emits at least one row for every block and block 0's rows
	// come first.
	mixed := false
	common := sizes[0]
	for _, s := range rowSizes {
		if s != common {
			mixed = true
		}
	}
	// Counting the title and the footer is what makes !Mixed mean that literally
	// every glyph on the plate is one size, which is exactly what makes SizeMM
	// printable. Each is consulted only when its string is non-empty -- the
	// invariant above has already established that its size is 0 when it is not.
	if title != "" && titleSizeMM != common {
		mixed = true
	}
	if footer != "" && footerSizeMM != common {
		mixed = true
	}
	sizeMM := common
	if mixed {
		// SizeMM is valid only when !Mixed, and 0 is how it says so.
		sizeMM = 0
	}
	return Fitted{
		Mixed:        mixed,
		SizeMM:       sizeMM,
		Sizes:        rowSizes,
		Lines:        lines,
		Faces:        faces,
		Title:        title,
		Footer:       footer,
		TitleFace:    blocks[0].Face,
		FooterFace:   blocks[len(blocks)-1].Face,
		TitleSizeMM:  titleSizeMM,
		FooterSizeMM: footerSizeMM,
	}, nil
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
	// The y a block starts at still decides its budget, so the blocks are
	// counted in plate order from the body's first row exactly as they are laid
	// out.
	//
	// start is a device-unit y now, and the row index 1 this passed before the
	// change is type-correct as one -- one device unit below the plate's top
	// edge, which moves FOUR rows into the top screw-hole band instead of two
	// and over-reports every line count the readout shows. It is the title row
	// this reserves unconditionally, so the translation is margin + one row.
	l, _, _, _ := wrapBlocks(params, blocks, uniformSizes(blocks, size), qrAt,
		params.I(outerMargin)+params.F(size), math.MaxInt)
	return len(l), linesAvail, len(l) <= linesAvail
}

// AdmissibleSized is AdmissibleBlocks' sibling for a composition that states its
// OWN rungs -- the shape FitSized lays out -- counted where FitSized counts it.
//
// AdmissibleBlocks lays every composition out uniformly at FontSizes' smallest
// rung, which is spec 6's anchor and stays exactly as it is. For a size ladder
// that anchor describes a DIFFERENT PLATE: measured, it reports 12 rows for the
// front while FitSized cuts 16, and 18 for the back against 20, so the readout's
// line count and the refusal's figures were about neither the plate nor a bound
// on it -- and an edited ladder that overflowed its own rungs was refused by the
// fit while admission still said "ok" with room to spare, the refusal's own
// numbers contradicting the refusal.
//
// It is a SIBLING and not a widening because AdmissibleBlocks' verdict is held
// to measured cliff values by TestAdmissibleBlocksVerdictDoesNotMove: that
// verdict gates OK on the text and confirm steps for every ordinary
// QR-carrying plate, the path seeds and passphrases take.
//
// THE TITLE ROW IS RESERVED UNCONDITIONALLY, at the plate's smallest rung, which
// is the size ftFitAt cuts a sized composition's title and footer at. Neither
// the title string nor the footer string is READ -- that is what makes admission
// monotone, so entering either after the text can never retroactively invalidate
// text already accepted.
//
// THE FOOTER ROW IS NOT RESERVED, and that is a measurement rather than an
// omission. Spec 5 measured a footer refusing the front ladder by 3.200mm and
// the back by 1.600mm: the front's whole spare is 3.600mm against a 3.800mm
// smallest rung, and the back's is 2.400mm against 3.000mm. So the smallest row
// this plate can reserve is larger than the room either side has, and reserving
// one would refuse both plates the machine cuts today -- it would move the
// verdict, which is precisely what this change must not do. AdmissibleBlocks'
// "-2" is not a bound for a sized composition either way: at 3.0mm it reserves
// two rows of a plate that is not the one being cut.
//
// The consequence is stated rather than hidden: "ok" here does not survive a
// FOOTER being entered, and the FIT is the authority that refuses it, at the
// confirm step. That was already true before this change -- admission said 12/24
// ok for a front that FitSized refuses the moment it carries a footer -- so
// nothing that held has been given up.
//
// There is no useQR parameter. FitSized has no code (spec 2.7), so a flag cannot
// reach the layout, and leaving the parameter off is what makes that structural
// rather than remembered.
func AdmissibleSized(params engrave.Params, blocks []Block, title, footer string) (linesUsed, linesAvail int, ok bool) {
	if len(blocks) == 0 {
		return 0, 0, false
	}
	sizes := make([]float32, len(blocks))
	rung := blocks[0].SizeMM
	for i, b := range blocks {
		if b.SizeMM <= 0 {
			// Not a sized composition: this is the shape FitSized refuses as
			// "sized 0mm", and laying it out here would put a whole block through
			// the wrap at fontSize 0, where the rows-that-fit division is by zero.
			return 0, 0, false
		}
		sizes[i] = b.SizeMM
		if b.SizeMM < rung {
			rung = b.SizeMM
		}
	}
	// The screw-hole rung, and the same one ftFitAt hands FitSized: the plate's
	// SMALLEST, never the first block's. yBudget then starts the body exactly
	// here for a titled composition.
	start := params.I(outerMargin) + params.F(rung)
	limit := params.F(plateSize) - params.I(outerMargin)
	// Unbounded on purpose, for AdmissibleBlocks' reason: a refusal reporting
	// "20 lines and the plate holds 20" for a composition needing forty says
	// nothing about how much to cut.
	lines, _, rowSizes, _ := wrapBlocks(params, blocks, sizes, nil, start, math.MaxInt)
	// linesAvail is the composition's OWN rows that fit the window, and then rows
	// at the LAST block's rung -- the block that would grow into them, which is
	// rowFaces' rule for the face read in the currency of size. A row count is
	// the only currency the readout has, and on a plate whose rows are different
	// heights there is no composition-independent number: 24 rows is a fact about
	// a 3.0mm plate, not about this one.
	y, avail := start, 0
	whole := true
	for _, s := range rowSizes {
		h := params.F(s)
		if y+h > limit {
			whole = false
			break
		}
		y, avail = y+h, avail+1
	}
	if whole {
		// A rung that quantises to zero device units would spin here forever;
		// FontSizes holds no such rung, and this is what keeps that a fact about
		// the data rather than an assumption about it.
		if last := params.F(blocks[len(blocks)-1].SizeMM); last > 0 {
			for y+last <= limit {
				y, avail = y+last, avail+1
			}
		}
	}
	return len(lines), avail, len(lines) <= avail
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
		// start is a device-unit y: the plate's TOP MARGIN, which is where the
		// row index 0 this passed before the change put the first row. The
		// literal 0 carried over is the plate's top EDGE, three millimetres
		// higher, which moves the screw-hole bands by a row and with them the
		// face boundary.
		_, faces, _, _ = wrapBlocks(params, blocks, uniformSizes(blocks, fontMM), qrp,
			params.I(outerMargin), math.MaxInt)
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
	total := 0
	for row := range rows {
		// One layout per row. The per-face cache this replaced hardcoded
		// baseY = outerMargin and keyed on the face alone, so on a plate that
		// cuts one face at two sizes it handed the second run the first run's
		// grid -- and re-keying it on (face, size, baseY) would never hit,
		// because 2.5 gives every row its own baseY. This is a value struct and
		// one Face.Decode('W') per row, at most 26 rows.
		lay := textLayout(params, faces[row], params.F(fontMM), params.I(outerMargin), qrAt)
		n, _ := lay.at(row)
		total += n
	}
	return total
}

// MaxCharsAt is MaxCharsAtBlocks for a composition cut entirely in one face.
func MaxCharsAt(params engrave.Params, fnt *vector.Face, fontMM float32, text string, qr bool) int {
	return MaxCharsAtBlocks(params, []Block{{Face: fnt, Text: text}}, fontMM, qr)
}
