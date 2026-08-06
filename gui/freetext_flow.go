package gui

import (
	"errors"
	"fmt"
	"image"
	"math"
	"slices"
	"strings"

	"seedhammer.com/backup"
	"seedhammer.com/engrave"
	"seedhammer.com/font/constant"
	"seedhammer.com/font/sh"
	"seedhammer.com/font/vector"
	"seedhammer.com/gui/assets"
	"seedhammer.com/gui/layout"
	"seedhammer.com/gui/op"
	"seedhammer.com/gui/widget"
)

// ftMaxLineLen is the title and footer cap, unconditional at every rung (spec
// 2). A rung-relative cap is unsound in both directions: anchored at 3.0mm a
// short text auto-fits at 6.0mm carrying a title that row cannot hold, and
// toPlate does not catch it; anchored at the CURRENT rung, deleting text raises
// the rung and retroactively invalidates a title already entered.
const ftMaxLineLen = backup.MaxTitleLen

// ftFace is the engraving face the composition is cut in, carried with the NAME
// the confirm screen shows. A bare *vector.Face has nothing in it a screen can
// print, and the face is not a detail the operator may be left to guess: it
// decides how much text fits (44 columns at 3.0mm in font/sh, 39 in
// font/constant) and what the plate looks like.
type ftFace struct {
	Name string
	Face *vector.Face
}

var (
	// ftFaceSH is the free-text plate's own face and the program's default.
	ftFaceSH = ftFace{"sh", sh.Font}
	// ftFaceConst is the face every seed, descriptor and passphrase plate is
	// cut in. The free-text program can engrave in it so that face can be
	// PROVEN at 3.0mm; see freetext_proof.go.
	ftFaceConst = ftFace{"constant", constant.Font}
)

// ftFaceRun is one run of the composition: a face, and how many of the Text
// field's '\n'-separated blocks are cut in it.
//
// Blocks is a count of BLOCKS, not of rows or of characters, because '\n' is
// the one boundary the operator can see in the field, WrapText already breaks
// on it, and nothing is consumed or substituted to mark it -- a free-text plate
// engraves what was typed, so a face plan may not be smuggled into the text as
// an escape the operator cannot see. The LAST run in a plan takes whatever is
// left over, whatever its Blocks says.
//
// SizeMM is the rung this run's blocks are cut at, or 0 for "the size the plate
// is fitted at" -- which is what every plan that shipped before the size ladder
// says, so the zero value is the legacy auto-fit plate. See ftPlan.Blocks: the
// sizes are stamped onto the blocks ALL OR NOTHING, against the part count.
type ftFaceRun struct {
	Face   ftFace
	Blocks int
	SizeMM float32
}

// ftPlan is the composition's FACE PLAN: which engraving face each part of the
// text is cut in, top of the plate to bottom.
//
// One run is the ordinary free-text plate. Two make a MIXED-FACE plate, which
// exists so one piece of steel can qualify both shipped faces at 3.0mm rather
// than costing a plate each; see ftProofTriggerBoth.
//
// Held by POINTER everywhere so plans compare by identity: the text-entry
// screen caches its evaluation on the plan, and two plans that happened to be
// equal are still two different plates as far as anything downstream is
// concerned.
type ftPlan struct {
	Runs []ftFaceRun
}

var (
	// ftPlanSH is the free-text plate's own face and the program's default.
	ftPlanSH = ftPlan{Runs: []ftFaceRun{{Face: ftFaceSH}}}
	// ftPlanConst cuts the whole plate in the face every seed, descriptor and
	// passphrase plate uses.
	ftPlanConst = ftPlan{Runs: []ftFaceRun{{Face: ftFaceConst}}}
)

// Name is what the prompt and the confirm screen call this plan: the face for a
// single-face plate, and the faces joined by '+' for a mixed one.
func (p *ftPlan) Name() string {
	if len(p.Runs) == 1 {
		return p.Runs[0].Face.Name
	}
	names := make([]string, len(p.Runs))
	for i, r := range p.Runs {
		names[i] = r.Face.Name
	}
	return strings.Join(names, "+")
}

// Sized reports that this plan states a rung per run, so the plate is a SIZE
// LADDER cut at several sizes rather than a composition auto-fitted at one.
func (p *ftPlan) Sized() bool {
	for _, r := range p.Runs {
		if r.SizeMM != 0 {
			return true
		}
	}
	return false
}

// Rungs is the plan's sizes, deduplicated and in PLATE ORDER -- top of the
// plate to bottom, which is the order the ladder titles name them in. Empty for
// a plan that states no sizes.
func (p *ftPlan) Rungs() []float32 {
	var out []float32
	for _, r := range p.Runs {
		if r.SizeMM != 0 && !slices.Contains(out, r.SizeMM) {
			out = append(out, r.SizeMM)
		}
	}
	return out
}

// declaredParts is how many '\n'-separated parts this plan describes: the SUM
// of every run's Blocks, which is the quantity Blocks' predicate matches the
// text against.
//
// It is a sum rather than len(Runs) because a run may declare more than one
// part, and it is not the BLOCK count because the final run absorbs the
// remainder -- see Blocks.
func (p *ftPlan) declaredParts() int {
	n := 0
	for _, r := range p.Runs {
		n += r.Blocks
	}
	return n
}

// Blocks cuts text into the engraving blocks this plan describes.
//
// Splitting on '\n' and rejoining with '\n' is lossless, so the composition the
// blocks describe is character-for-character the text that was typed -- which
// is what makes the QR a copy of the plate rather than of a fragment.
//
// Each NON-FINAL run takes min(Blocks, remaining) parts and the walk stops as
// soon as the parts run out; the FINAL run takes whatever is left, whatever its
// own Blocks says. So len(out) == len(p.Runs) only once every non-final run's
// declared share has been satisfied -- and it then stays equal for every LARGER
// part count too, because the final run absorbs the remainder. A text with too
// few parts to fill the first run collapses to a single block in that run's
// face rather than producing an empty one: an empty block would engrave a blank
// row nobody asked for, and there is no half of a plate to cut in the next face.
//
// The comment this replaced said the text "collapses to a single block in the
// first run's face", full stop. That is true of the two-run plans that shipped
// when it was written -- ftPlanBoth's first run declares every part of its half,
// so it swallows everything at any part count at or below that -- and FALSE for
// the four- and six-run ladders, where five parts of a six-part plan produce
// FIVE blocks and no collapse at all. It misled a spec draft; the rule above is
// read off the walk rather than generalised from the plans in front of it.
//
// SIZES ARE ALL OR NOTHING. Every emitted block is stamped with its run's
// SizeMM -- unless the text's PART count differs from declaredParts, in which
// case every block's size is CLEARED and the plate reverts to ordinary uniform
// auto-fit.
//
// The predicate is on the part count and NOT on the block count, because the
// final run absorbs the remainder and so pins len(out) at len(p.Runs) for every
// part count at or above the declared one. A block-count predicate reports
// "exact shape" for a text with an extra newline in it, and what gets cut is a
// full ladder with every band one run late, in faces the title does not name --
// which the confirm screen cannot expose, because the expected number of rungs
// is present and correct. The part count is the quantity that actually has to
// match, and it is the one the operator can see in the field.
//
// Reverting on a mismatch rather than cutting the partial ladder is spec 3.1's
// decision: on a deleted newline the rung that goes missing is always the LAST
// run, which on both ladder sides is the smallest and the one the proof exists
// to answer. A refusal or a plain plate is visible; a size change is not.
func (p *ftPlan) Blocks(text string) []backup.Block {
	first := p.Runs[0].Face.Face
	// Resolved once, over the WHOLE text, so every block agrees about it.
	exact := len(strings.Split(text, "\n")) == p.declaredParts()
	sizeOf := func(r ftFaceRun) float32 {
		if !exact {
			return 0
		}
		return r.SizeMM
	}
	if len(p.Runs) == 1 {
		return []backup.Block{{Face: first, Text: text, SizeMM: sizeOf(p.Runs[0])}}
	}
	parts := strings.Split(text, "\n")
	out := make([]backup.Block, 0, len(p.Runs))
	for i, r := range p.Runs {
		n := r.Blocks
		if i == len(p.Runs)-1 || n > len(parts) {
			n = len(parts)
		}
		if n <= 0 {
			// A non-final run that covers nothing is a malformed plan; see
			// TestPlansAreWellFormed. Treat it as absent rather than emitting
			// an empty block, which would engrave a blank row.
			continue
		}
		out = append(out, backup.Block{
			Face:   r.Face.Face,
			Text:   strings.Join(parts[:n], "\n"),
			SizeMM: sizeOf(r),
		})
		parts = parts[n:]
		if len(parts) == 0 {
			break
		}
	}
	if len(out) == 0 {
		// The collapse carries no size: it is one block standing in for a plan
		// the text no longer matches.
		return []backup.Block{{Face: first, Text: text}}
	}
	return out
}

// ftFaceSummary is what the confirm screen prints for "font:". It is read from
// the FITTED FACE MAP -- the faces the lines were actually wrapped and will
// actually be cut in -- and never from the plan, so a plate that collapsed to
// one face, or whose halves came out swapped, says so on the screen the
// operator approves.
//
// A single-face plate is named plainly, exactly as it always was. A mixed one
// carries the row count of each run, because how the plate divides is the one
// thing about it that cannot be seen from the size or the line total.
//
// Runs are grouped by (FACE, SIZE), never by face alone: the ladder cuts font/sh
// at 5.0mm and again at 3.8mm, and grouping by face reports two runs where the
// plate has four -- with row counts that belong to neither.
//
// The size is PRINTED only when the plate mixes them. On a uniform plate the
// size is already the first thing on the summary line, so repeating it against
// every run is noise on a panel whose budget this block competes for; on a
// ladder it is the only thing that maps a rung to the rows it is cut on. sizes
// is read from Fitted.Sizes and may be nil, which is the uniform case.
func ftFaceSummary(plan *ftPlan, faces []*vector.Face, sizes []float32) string {
	if len(plan.Runs) == 1 {
		return plan.Runs[0].Face.Name
	}
	name := func(f *vector.Face) string {
		for _, r := range plan.Runs {
			if r.Face.Face == f {
				return r.Face.Name
			}
		}
		return "?"
	}
	// sizes is parallel to faces, or it is not consulted at all: a short slice
	// is a caller that has not got one, not a plate whose tail has no size.
	if len(sizes) != len(faces) {
		sizes = nil
	}
	mixed := false
	for _, s := range sizes {
		if s != sizes[0] {
			mixed = true
		}
	}
	same := func(a, b int) bool {
		return faces[a] == faces[b] && (sizes == nil || sizes[a] == sizes[b])
	}
	var parts []string
	for i := 0; i < len(faces); {
		j := i
		for j < len(faces) && same(i, j) {
			j++
		}
		if mixed {
			parts = append(parts, fmt.Sprintf("%s %.1f %d", name(faces[i]), sizes[i], j-i))
		} else {
			parts = append(parts, fmt.Sprintf("%s %d", name(faces[i]), j-i))
		}
		i = j
	}
	if len(parts) == 0 {
		return plan.Name()
	}
	return strings.Join(parts, " + ")
}

// ftFit is one live evaluation of the composition: what the operator is told
// while typing, and what the plate will be.
//
// plate is FitBlocks' whole result, carried as ONE value: the size, the wrapped
// lines, the face each line is cut in and the QR. The readout, the confirm
// screen and the engraver all read it, and none of them recomputes any part of
// it -- with a mixed-face plate the per-line character budget depends on the
// face of that row, so a second derivation of that mapping is a second answer.
type ftFit struct {
	plate      backup.Fitted
	linesUsed  int
	linesAvail int
	ok         bool
	err        error
}

// ftEvaluate answers every live question at once, from ONE encode. Splitting it
// would let the readout, the refusal figure and the engraving disagree about
// the same text.
//
// ADMISSION MUST AGREE WITH THE ROUTER about the QR. ftFitAt sends a sized
// composition to FitSized, which has no QR parameter at all (spec 2.7) and so
// ignores useQR entirely -- while AdmissibleBlocks reserves a whole QR band
// whenever the flag is set. Handing it the raw flag therefore narrows the band
// for a plate that can never carry a code: the operator loads a ladder (which
// clears the flag), goes Back to the QR screen and turns it on again -- spec
// 3.2's reachable path -- and comes back to a plate that FitSized lays out
// perfectly being refused, over a code it cannot hold. Measured before the fix:
// SIZEPROOF!BACK went from (18, 24, ok) to (30, 24, REFUSED) on the flag alone,
// and the front sat four lines under the same cliff.
//
// The stale flag is dropped HERE and not in AdmissibleBlocks. Spec 6 pins that
// function at the uniform 3.0mm anchor -- it never reads Block.SizeMM -- and
// TestAdmissibleBlocksVerdictDoesNotMove holds its verdict to measured cliff
// values. Counting a sized composition at its OWN rungs is the better answer
// and would also make the readout's line count describe the rows actually cut
// (spec 6 accepts that divergence); it changes that function's contract, so it
// is left to the phase that next touches admission rather than made at a gate.
//
// Dropping the flag is not the WHOLE answer either: spec 3.0 requires the QR
// step to stop offering a choice it will not honour, rather than accepting one
// and discarding it downstream. That is the QR step's own change, and its own
// follow-up.
func ftEvaluate(params engrave.Params, plan *ftPlan, text, title, footer string, useQR bool, size float32) ftFit {
	var f ftFit
	blocks := plan.Blocks(text)
	admitQR := useQR && !ftSizedBlocks(blocks)
	f.linesUsed, f.linesAvail, f.ok = backup.AdmissibleBlocks(params, blocks, title, footer, admitQR)
	f.plate, f.err = ftFitAt(params, blocks, title, footer, useQR, size)
	return f
}

// ftSizedBlocks reports that EVERY block states its own rung, which is the
// condition that routes a composition to FitSized.
//
// Every block, not any: a composition with one unsized block is not a ladder,
// and handing it to FitSized would refuse it as "sized 0mm" rather than laying
// it out at a single rung the way an ordinary edited plate is.
func ftSizedBlocks(blocks []backup.Block) bool {
	if len(blocks) == 0 {
		return false
	}
	for _, b := range blocks {
		if b.SizeMM == 0 {
			return false
		}
	}
	return true
}

// ftScrewHoleRung is the size the title and footer rows of a sized composition
// are cut at: the SMALLEST rung on the plate.
//
// Not the first block's, which is spec 2.3's explicit rule and R0's I1: the
// back's first block is 4.4mm, and a title cut there costs 1.400mm of the
// 2.400mm this side has to spare.
func ftScrewHoleRung(blocks []backup.Block) float32 {
	rung := blocks[0].SizeMM
	for _, b := range blocks {
		if b.SizeMM < rung {
			rung = b.SizeMM
		}
	}
	return rung
}

// ftFitAt is the ONE place the rung choice is applied. With no rung named the
// plate auto-fits, as every free-text plate always has; with one, the plate is
// cut at THAT rung or refused. Splitting this decision across the evaluate and
// the build paths would let the readout promise a size the engraver does not use.
//
// The PER-BLOCK-SIZE test comes first, and the order is load-bearing.
// FitBlocksAt ignores Block.SizeMM (spec 2.2), and this function is shared with
// the BOTHPROOF!<rung> path, which does set a non-zero rung -- so a FitSized
// branch appended second would engrave a size ladder at one uniform rung the
// moment both were set, which is R0's C6 in a third hat.
//
// A non-zero size together with sized blocks is therefore an ERROR rather than
// a silent uniform fit. The un-edited ladder never reaches it: the ladder proofs
// are not Sizeable, so the trigger resolves rung 0 and the loader writes 0.
//
// There is no QR on a sized plate. FitSized has no parameter for one (spec 2.7):
// the code's keep-out band is quantised by a single fontSize and a plate that
// mixes sizes has none. The operator's choice is dropped at the LOAD, by the
// prompt the ladder proofs put up (spec 7.16), and the confirm screen reads the
// QR state off the fitted plate rather than off the flag, so it cannot claim a
// code the plate does not carry.
func ftFitAt(params engrave.Params, blocks []backup.Block, title, footer string, useQR bool, size float32) (backup.Fitted, error) {
	if ftSizedBlocks(blocks) {
		if size != 0 {
			return backup.Fitted{}, fmt.Errorf(
				"gui: the composition states its own rungs and cannot also be fitted at %.1fmm", size)
		}
		var titleSize, footerSize float32
		rung := ftScrewHoleRung(blocks)
		// Both branches test the STRING, never the size: spec 2.3's invariant
		// is that each size is non-zero exactly when its string is non-empty.
		if title != "" {
			titleSize = rung
		}
		if footer != "" {
			footerSize = rung
		}
		return backup.FitSized(params, blocks, title, footer, titleSize, footerSize)
	}
	if size != 0 {
		return backup.FitBlocksAt(params, blocks, title, footer, useQR, size)
	}
	return backup.FitBlocks(params, blocks, title, footer, useQR)
}

// ftPlateRungs is the size field of the confirm screen and of the prompt: the
// one rung of a uniform plate, or the ladder's distinct rungs in PLATE ORDER,
// joined the way the ladder titles join them.
//
// A plate that mixes sizes has no valid SizeMM -- it is 0 by spec 2.3 -- so a
// reader that prints it regardless prints "0.0mm" on the one screen the
// operator approves. That is a defect, not a fallback.
func ftPlateRungs(f backup.Fitted) string {
	if !f.Mixed {
		return fmt.Sprintf("%.1fmm", f.SizeMM)
	}
	var rungs []float32
	for _, s := range f.Sizes {
		if !slices.Contains(rungs, s) {
			rungs = append(rungs, s)
		}
	}
	if len(rungs) == 0 {
		return "--"
	}
	parts := make([]string, len(rungs))
	for i, r := range rungs {
		parts[i] = fmt.Sprintf("%.1f", r)
	}
	return strings.Join(parts, "+") + "mm"
}

// ftPlateSizeSpan is the same answer in the space the live readout has: the
// largest and the smallest size on the plate. The readout sits beside the line
// count on the text-entry screen and grows with the text, so it takes the range
// where the confirm screen takes the list.
func ftPlateSizeSpan(f backup.Fitted) string {
	if !f.Mixed {
		return fmt.Sprintf("%.1fmm", f.SizeMM)
	}
	if len(f.Sizes) == 0 {
		return "--"
	}
	return fmt.Sprintf("%.1f-%.1fmm", slices.Max(f.Sizes), slices.Min(f.Sizes))
}

// ftSizeLabel is the readout: the fitted size and "lines used / lines
// available".
//
// NEVER "characters remaining". Under word wrap no scalar character count is
// correct -- appending "x" to the last word can cost a whole line while
// appending " x" does not -- so a character budget would be wrong in the one
// direction that matters, telling the operator there is room when there is not.
func ftSizeLabel(f ftFit) string {
	size := "--"
	if f.err == nil {
		size = ftPlateSizeSpan(f.plate)
	}
	return fmt.Sprintf("%s  %d/%d lines", size, f.linesUsed, f.linesAvail)
}

// ftQRChoiceFlow is step 1. It comes FIRST so the admission anchor is fixed
// before any text is typed: choosing a QR afterwards would shrink the capacity
// under text already accepted.
func ftQRChoiceFlow(ctx *Context, th *Colors, prior bool) (bool, bool) {
	cs := &ChoiceScreen{
		Title: "QR Code",
		Lead: "A QR is a machine-readable copy of the text. " +
			"Anyone who photographs the plate can read it.",
		Choices: []string{"No QR", "Add QR"},
	}
	if prior {
		cs.choice = 1 // preserve a deliberate opt-in across Back
	}
	hookPPWidget("qr", cs)
	// choice starts at 0, which is "No QR": the default is a property of this
	// ordering, so do not reorder the choices.
	sel, ok := cs.Choose(ctx, th)
	if !ok {
		return false, false
	}
	return sel == 1, true
}

// ftRefuse explains an over-capacity text and, when a QR is present, offers
// dropping it as an EXPLICIT choice with the live figure.
//
// The QR is never dropped automatically: it changes what a scanner returns from
// the plate, and doing that on the operator's behalf to make room is exactly
// the silent substitution this program exists to avoid.
//
// A SIZED composition is never offered that remedy, whatever the flag says. It
// is laid out by FitSized, which has no QR (spec 2.7), so removing a code the
// plate does not carry frees nothing -- and an edited ladder that overflows its
// own rungs is refused by the fit, not by the QR band. Shortening the text is
// the only remedy there is, so it is the only one offered.
func ftRefuse(ctx *Context, th *Colors, params engrave.Params, plan *ftPlan, f ftFit, text string, useQR bool) bool {
	blocks := plan.Blocks(text)
	if !useQR || ftSizedBlocks(blocks) {
		showError(ctx, th, "Text", fmt.Sprintf(
			"The text needs %d lines and a plate holds %d, at the smallest size. Shorten the Text field.",
			f.linesUsed, f.linesAvail))
		return false
	}
	smallest := backup.FontSizes[len(backup.FontSizes)-1]
	freed := backup.MaxCharsAtBlocks(params, blocks, smallest, false) -
		backup.MaxCharsAtBlocks(params, blocks, smallest, true)
	cs := &ChoiceScreen{
		Title: "Too Long",
		Lead: fmt.Sprintf(
			"The Text field needs %d lines and a plate holds %d, at the smallest size. "+
				"Removing the QR frees about %d characters, and the plate stops being machine-readable.",
			f.linesUsed, f.linesAvail, freed),
		Choices: []string{"Keep the QR", "Remove the QR"},
	}
	hookPPWidget("refusal", cs)
	sel, ok := cs.Choose(ctx, th)
	return ok && sel == 1
}

// ftTextEntryFlow is step 2. Keystrokes are always accepted; the readout shows
// the over-capacity state and OK refuses, naming the field. Silently dropping
// keystrokes would leave the operator believing a longer text had been entered
// (gui/passphrase_flow.go:113-118's reviewed decision).
// loadProof, when non-nil, is called if the operator types one of the proof
// triggers and accepts the prompt. It writes the other fields and the face
// plan, which is why it takes pointers, and RETURNS the text it wrote so this
// screen re-seeds from the value that was actually stored rather than
// recomputing it.
func ftTextEntryFlow(ctx *Context, th *Colors, params engrave.Params, prior string, title, footer *string, plan **ftPlan, useQR *bool, size *float32, loadProof func(*ftProof, float32) string) (string, bool) {
	kbd := NewTextKeyboard(ctx)
	kbd.Fragment = prior
	backBtn := &Clickable{Button: Button1}
	okBtn := &Clickable{Button: Button3}
	hookPPWidget("kbd", kbd)
	hookPPWidget("back", backBtn)
	hookPPWidget("ok", okBtn)

	// The evaluation is cached on (text, qr, plan) because it encodes a QR, and
	// the screen redraws every frame while the text changes only on a
	// keystroke. The PLAN is part of the key: loading a proof changes it, and a
	// cache that ignored it would keep reporting the old face's line count and
	// fitted size for the new plate.
	var cache ftFit
	var cacheText string
	var cacheQR bool
	var cachePlan *ftPlan
	// The rung is part of the cache KEY. Loading a proof at a named size changes
	// the plate without necessarily changing the text, and a stale entry would
	// report the auto-fit size while the engraver used the chosen one.
	var cacheSize float32
	cacheValid := false
	evaluate := func() ftFit {
		if !cacheValid || cacheText != kbd.Fragment || cacheQR != *useQR ||
			cachePlan != *plan || cacheSize != *size {
			cache = ftEvaluate(params, *plan, kbd.Fragment, *title, *footer, *useQR, *size)
			cacheText, cacheQR, cachePlan, cacheSize, cacheValid =
				kbd.Fragment, *useQR, *plan, *size, true
		}
		return cache
	}

	for !ctx.Done {
		for kbd.Update(ctx) {
		}
		if backBtn.Clicked(ctx) {
			return "", false
		}
		if okBtn.Clicked(ctx) {
			// The trigger check runs BEFORE this field's own validation, and
			// before the fit evaluation: the pattern is chosen for the CURRENT
			// QR choice and face, so evaluating the literal "TEXTPROOF!" first
			// would tell the operator nothing useful.
			if loaded, ok := ftProofOffer(ctx, th, kbd.Fragment, *useQR, loadProof); ok {
				// Stay on this screen (continue, do NOT fall through to the
				// return) so the operator sees what landed, and re-seed the
				// field from the text the loader actually wrote.
				kbd.Fragment = loaded
				continue
			}
			if kbd.Fragment == "" {
				showError(ctx, th, "Text", "The Text field is required.")
				continue
			}
			f := evaluate()
			// ErrTooLarge is a GEOMETRY refusal and has a remedy the operator
			// can act on. Any other error is the encoder giving up -- qr.Encode
			// fails at 2954 bytes, which the uncapped Text field can reach --
			// and Fit returns a nil code either way, so the two cases have to
			// be told apart by the error and not by the code being nil.
			if f.err != nil && !errors.Is(f.err, backup.ErrTooLarge) {
				showError(ctx, th, "Text", "The text is too long to encode as a QR.")
				continue
			}
			if !f.ok || f.err != nil {
				if ftRefuse(ctx, th, params, *plan, f, kbd.Fragment, *useQR) {
					*useQR = false
				}
				continue
			}
			return kbd.Fragment, true
		}
		f := evaluate()
		dims := ctx.Platform.DisplaySize()
		screen := layout.Rectangle{Max: dims}
		_, content := screen.CutTop(leadingSize)
		content, _ = content.CutBottom(8)
		// Reserve the readout's band BEFORE the keyboard, and BOUND the
		// keyboard block: its readout grows with the text, and op.Layer draws
		// the keyboard on top, so an unreserved band lets the block cover the
		// very readout that says the text no longer fits.
		cntOp, cntsz := widget.Labelf(&ctx.B, ctx.Styles.subtitle, th.Text, "%s", ftSizeLabel(f))
		counterBand, content := content.CutTop(cntsz.Y)
		cntOp = cntOp.Offset(counterBand.N(cntsz))
		kbd.MaxHeight = content.Dy()
		kbdOp, kbdsz := kbd.Layout(ctx, th)
		kbdOp = kbdOp.Offset(content.S(kbdsz))
		nav, _ := layoutNavigation(&ctx.B, th, dims, []NavButton{
			{Clickable: backBtn, Style: StyleSecondary, Icon: assets.IconBack},
			{Clickable: okBtn, Style: StylePrimary, Icon: assets.IconCheckmark},
		}...)
		titleOp, _ := layoutTitle(ctx, dims.X, th.Text, "Text")
		ctx.Frame(op.Layer(kbdOp, cntOp, nav, titleOp, op.Color(&ctx.B, th.Background)))
	}
	return "", false
}

// ftLineEntryFlow is steps 3 and 4: one optional line, capped at ftMaxLineLen.
// Skippable -- OK on an empty field means "no title".
func ftLineEntryFlow(ctx *Context, th *Colors, what, prior string) (string, bool) {
	kbd := NewTextKeyboard(ctx)
	kbd.Fragment = prior
	backBtn := &Clickable{Button: Button1}
	okBtn := &Clickable{Button: Button3}
	hookPPWidget("kbd", kbd)
	hookPPWidget("back", backBtn)
	hookPPWidget("ok", okBtn)
	for !ctx.Done {
		for kbd.Update(ctx) {
		}
		if backBtn.Clicked(ctx) {
			return "", false
		}
		if okBtn.Clicked(ctx) {
			if strings.ContainsRune(kbd.Fragment, '\n') {
				showError(ctx, th, what, "The "+strings.ToLower(what)+" is a single line.")
				continue
			}
			if len(kbd.Fragment) > ftMaxLineLen {
				showError(ctx, th, what, fmt.Sprintf(
					"The %s holds %d characters and %d were entered. It sits on a screw-hole row at every size.",
					strings.ToLower(what), ftMaxLineLen, len(kbd.Fragment)))
				continue
			}
			return kbd.Fragment, true
		}
		dims := ctx.Platform.DisplaySize()
		screen := layout.Rectangle{Max: dims}
		_, content := screen.CutTop(leadingSize)
		content, _ = content.CutBottom(8)
		cntOp, cntsz := widget.Labelf(&ctx.B, ctx.Styles.subtitle, th.Text,
			"%d/%d  optional", len(kbd.Fragment), ftMaxLineLen)
		counterBand, content := content.CutTop(cntsz.Y)
		cntOp = cntOp.Offset(counterBand.N(cntsz))
		kbd.MaxHeight = content.Dy()
		kbdOp, kbdsz := kbd.Layout(ctx, th)
		kbdOp = kbdOp.Offset(content.S(kbdsz))
		nav, _ := layoutNavigation(&ctx.B, th, dims, []NavButton{
			{Clickable: backBtn, Style: StyleSecondary, Icon: assets.IconBack},
			{Clickable: okBtn, Style: StylePrimary, Icon: assets.IconCheckmark},
		}...)
		titleOp, _ := layoutTitle(ctx, dims.X, th.Text, what)
		ctx.Frame(op.Layer(kbdOp, cntOp, nav, titleOp, op.Color(&ctx.B, th.Background)))
	}
	return "", false
}

// ftConfirmWarnings is spec 9. Nothing on this plate has been validated, the
// machine's duration leaks the content, and a QR makes it readable from a
// photograph.
const (
	ftWarnNotBackup = "Nothing here is checked. This is not a validated backup: " +
		"no wordlist, no checksum, no verify step."
	ftWarnTiming = "Engraving is not constant-time. How long the machine runs " +
		"depends on what it is cutting, so anyone watching or timing it learns about the text."
	ftWarnQR = "The QR makes the text readable by any camera. A photograph of " +
		"the plate is a copy of the text."
)

// ftConfirmRow is one row of the plate as it will be cut: the title, one
// wrapped body line, or the footer. Kept as a list so the preview can be paged
// a whole row at a time and the pager's arithmetic is over PLATE ROWS, not
// pixels.
type ftConfirmRow struct {
	text string
	// head is true for the title and footer rows, which engrave on the
	// screw-hole rows and are drawn in the subtitle style to say so.
	head bool
}

func ftConfirmRows(f ftFit, title, footer string) []ftConfirmRow {
	rows := make([]ftConfirmRow, 0, len(f.plate.Lines)+2)
	if title != "" {
		rows = append(rows, ftConfirmRow{title, true})
	}
	for _, l := range f.plate.Lines {
		rows = append(rows, ftConfirmRow{l, false})
	}
	if footer != "" {
		rows = append(rows, ftConfirmRow{footer, true})
	}
	return rows
}

// ftConfirmSummary is the block that MUST be on the panel whatever the text
// says: the fitted size, the row count, the QR state, the face -- and the three
// safety warnings. It is measured at the panel's width and drawn BELOW the
// preview, but the preview's budget is what is left after it, so it can never
// be pushed off the bottom.
//
// Before the execution review this block simply followed the lines, and a
// 20-line composition put the size line at y=510 and all three warnings at
// y=537 on a panel 320 pixels tall: 136% overflow, entirely invisible, with a
// text-only test reporting them present because ExtractText ignores occlusion.
// That was a defect in the free-text feature at large -- any long text reached
// it -- not merely in the proof.
// The QR state and the SIZE are both read off the FITTED PLATE rather than off
// the flow's own fields. The size because a plate that mixes them has no valid
// SizeMM and would print "0.0mm"; the QR because a sized composition carries no
// code whatever the operator chose one step earlier (spec 2.7), and a screen
// that reported the flag would promise a machine-readable plate that the
// engraver does not cut.
func ftConfirmSummary(ctx *Context, th *Colors, width int, f ftFit, plan *ftPlan, pager string) (op.Op, image.Point) {
	useQR := f.plate.QR != nil
	var rt richText
	rt.Add(&ctx.B, ctx.Styles.subtitle, width, th.Text, fmt.Sprintf(
		"%s  %d lines  QR: %s  font: %s",
		ftPlateRungs(f.plate), len(f.plate.Lines), ppYesNo(useQR),
		ftFaceSummary(plan, f.plate.Faces, f.plate.Sizes)))
	if pager != "" {
		rt.Add(&ctx.B, ctx.Styles.subtitle, width, th.Text, pager)
	}
	rt.Y += 4
	rt.Add(&ctx.B, ctx.Styles.body, width, th.Text, ftWarnNotBackup)
	rt.Add(&ctx.B, ctx.Styles.body, width, th.Text, ftWarnTiming)
	if useQR {
		rt.Add(&ctx.B, ctx.Styles.body, width, th.Text, ftWarnQR)
	}
	return rt.Content, image.Pt(width, rt.Y)
}

// ftConfirmView is one rendered page of the confirm screen.
type ftConfirmView struct {
	Content op.Op
	Size    image.Point
	// Shown is how many plate rows this page drew, Total how many there are.
	// The pager advances by Shown, so pages never skip or repeat a row.
	Shown int
	Total int
}

// ftConfirmBody renders the confirm screen's page starting at plate row start,
// inside height pixels.
//
// Each plate row is its OWN UNWRAPPED label. A width-bounded label would
// re-wrap in the proportional screen face and break the single-wrap-function
// invariant this screen exists to demonstrate: the operator would approve lines
// the machine will not cut.
//
// The summary's height is subtracted from the budget FIRST, and one pager row
// is reserved whether or not it ends up drawn, so the returned Size is <=
// height for every composition the flow will ever hand it. A page always draws
// at least one row, so the pager cannot stall; TestFTConfirmAlwaysFitsThePanel
// pins that the budget on the real panel is at least one row even in the
// tightest case.
func ftConfirmBody(ctx *Context, th *Colors, width, height, start int, f ftFit, plan *ftPlan, title, footer string) ftConfirmView {
	rows := ftConfirmRows(f, title, footer)
	if start < 0 || start >= len(rows) {
		start = 0
	}
	// Measured with a pager string of the same shape as the real one, so the
	// reservation is exact rather than approximately right.
	_, probe := ftConfirmSummary(ctx, th, width, f, plan, ftConfirmPager(0, 0, 0))
	budget := height - probe.Y

	var rt richText
	shown := 0
	for i := start; i < len(rows); i++ {
		st := ctx.Styles.body
		if rows[i].head {
			st = ctx.Styles.subtitle
		}
		m := st.Face.Metrics()
		next := rt.Y + m.Ascent.Ceil() + m.Descent.Ceil()
		if shown > 0 && next > budget {
			break
		}
		// math.MaxInt: never re-wrap. See the doc comment.
		rt.Add(&ctx.B, st, math.MaxInt, th.Text, rows[i].text)
		shown++
	}
	rt.Y += 4
	pager := ""
	if start > 0 || shown < len(rows) {
		pager = ftConfirmPager(start, shown, len(rows))
	}
	sum, sumSz := ftConfirmSummary(ctx, th, width, f, plan, pager)
	return ftConfirmView{
		Content: op.Layer(rt.Content, sum.Offset(image.Pt(0, rt.Y))),
		Size:    image.Pt(width, rt.Y+sumSz.Y),
		Shown:   shown,
		Total:   len(rows),
	}
}

// ftConfirmPager names which plate rows this page is showing. Without it a
// paged preview is indistinguishable from a truncated one, and the operator
// would approve a plate having read only its first rows believing they were all
// of it.
func ftConfirmPager(start, shown, total int) string {
	return fmt.Sprintf("rows %d-%d of %d  >", start+1, start+shown, total)
}

// ftConfirmFlow is step 5: the last checkpoint before a permanent plate.
//
// Three buttons, not two: Back, "next page" and OK. The page button appears
// only when the preview does not fit at once, so a short text -- which is
// almost every real one -- still shows the whole plate and two buttons.
func ftConfirmFlow(ctx *Context, th *Colors, f ftFit, plan *ftPlan, title, footer string) bool {
	backBtn := &Clickable{Button: Button1}
	pageBtn := &Clickable{Button: Button2}
	okBtn := &Clickable{Button: Button3}
	hookPPWidget("back", backBtn)
	hookPPWidget("page", pageBtn)
	hookPPWidget("ok", okBtn)
	start := 0
	view := ftConfirmView{}
	for !ctx.Done {
		if backBtn.Clicked(ctx) {
			return false
		}
		if okBtn.Clicked(ctx) {
			return true
		}
		if pageBtn.Clicked(ctx) {
			// Advance by what was actually drawn, so no row is skipped, and
			// wrap to the top rather than sticking at the end.
			if start+view.Shown < view.Total {
				start += view.Shown
			} else {
				start = 0
			}
		}
		dims := ctx.Platform.DisplaySize()
		area := ppConfirmArea(dims)
		view = ftConfirmBody(ctx, th, area.Dx(), area.Dy(), start, f, plan, title, footer)
		body := view.Content.Offset(image.Point(area.Min))
		btns := []NavButton{
			{Clickable: backBtn, Style: StyleSecondary, Icon: assets.IconBack},
			{Clickable: okBtn, Style: StylePrimary, Icon: assets.IconCheckmark},
		}
		if view.Shown < view.Total {
			btns = append(btns, NavButton{Clickable: pageBtn, Style: StyleSecondary, Icon: assets.IconRight})
		}
		nav, _ := layoutNavigation(&ctx.B, th, dims, btns...)
		titleOp, _ := layoutTitle(ctx, dims.X, th.Text, "Confirm")
		ctx.Frame(op.Layer(body, nav, titleOp, op.Color(&ctx.B, th.Background)))
	}
	return false
}

// freetextPlateHook receives exactly what EngraveFitted was handed. nil in
// production. Without it there is no way to bind the layout the operator
// APPROVED to the one that was ENGRAVED: the confirm screen is inspectable via
// op.Drawer.ExtractText, a bspline.Curve is not -- Plate is {Duration, Spline},
// stroke geometry carrying no text at all.
//
// It takes the WHOLE backup.Fitted, and the FACE MAP is the reason. None of it
// is recoverable from the plate, and engraving a line in a face other than the
// one it was fitted in puts it wide of the grid it was wrapped to -- a defect
// no assertion on the size, the lines or the code can see. On a mixed-face
// plate that includes engraving both halves in one face, or in each other's.
var freetextPlateHook func(f backup.Fitted)

// ftBuildPlate turns the fitted composition into an engravable plate.
//
// ONE call to FitBlocks yields the size, the lines, the FACE OF EVERY LINE and
// the code, and that one value goes straight to EngraveFitted. It never encodes
// a second time and never re-derives the face map: the fit's code IS the
// artifact and the fit's faces ARE the faces the lines were measured against,
// so the fit path and the build path cannot disagree about what a scanner will
// return or about which face any row is cut in.
func ftBuildPlate(params engrave.Params, plan *ftPlan, text, title, footer string, useQR bool, size float32) (Plate, error) {
	fitted, err := ftFitAt(params, plan.Blocks(text), title, footer, useQR, size)
	if err != nil {
		return Plate{}, err
	}
	if freetextPlateHook != nil {
		freetextPlateHook(fitted)
	}
	return toPlate(backup.EngraveFitted(params, fitted), params)
}

// The steps of spec 7, in order. The QR choice is first so the admission anchor
// is fixed before typing.
type ftStep int

const (
	ftStepQR ftStep = iota
	ftStepText
	ftStepTitle
	ftStepFooter
	ftStepConfirm
	ftStepEngrave
)

// engraveTextFlow is the engraveText program (spec 7). Back from any step
// preserves every entered value.
func engraveTextFlow(ctx *Context, th *Colors) {
	params := ctx.Platform.EngraverParams()
	var text, title, footer string
	useQR := false
	// The rung the operator named on a proof trigger, or 0 for auto-fit. Held
	// here beside the other fields so it survives Back exactly as they do.
	var size float32
	// The free-text plate's own face, unless a proof trigger asks for another
	// plan. Held here rather than in ftTextEntryFlow so it survives Back exactly
	// as the other four fields do.
	plan := &ftPlanSH
	step := ftStepQR
	for !ctx.Done {
		switch step {
		case ftStepQR:
			add, ok := ftQRChoiceFlow(ctx, th, useQR)
			if !ok {
				return // Back out of the first step leaves the program.
			}
			useQR = add
		case ftStepText:
			s, ok := ftTextEntryFlow(ctx, th, params, text, &title, &footer, &plan, &useQR, &size,
				ftProofLoader(params, &text, &title, &footer, &plan, &useQR, &size))
			if !ok {
				step -= 2
				break
			}
			text = s
		case ftStepTitle:
			s, ok := ftLineEntryFlow(ctx, th, "Title", title)
			if !ok {
				step -= 2
				break
			}
			title = s
		case ftStepFooter:
			s, ok := ftLineEntryFlow(ctx, th, "Footer", footer)
			if !ok {
				step -= 2
				break
			}
			footer = s
		case ftStepConfirm:
			f := ftEvaluate(params, plan, text, title, footer, useQR, size)
			if f.err != nil || !f.ok {
				// A title or footer entered after the text can only ever make
				// the composition SMALLER on the plate, never inadmissible --
				// admission reserves both rows unconditionally. So this is a
				// genuine surprise, and it goes back rather than engraving.
				showError(ctx, th, "Text", "This text does not fit a plate.")
				step -= 2
				break
			}
			if !ftConfirmFlow(ctx, th, f, plan, title, footer) {
				step -= 2
				break
			}
		case ftStepEngrave:
			plate, err := ftBuildPlate(params, plan, text, title, footer, useQR, size)
			if err != nil {
				// The message quotes no field content.
				showError(ctx, th, "Text", "This text does not fit a plate.")
				step -= 2
				break
			}
			if NewEngraveScreen(ctx, plate).Engrave(ctx, &engraveTheme) {
				return
			}
			// Backed out of the engrave: return to the confirm screen.
			step -= 2
		}
		step++
	}
}
