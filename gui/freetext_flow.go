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
// It now agrees about the ROWS as well, and on the SAME predicate ftFitAt routes
// on: a sized composition goes to AdmissibleSized, which counts it at its own
// rungs, where AdmissibleBlocks laid it out uniformly at the 3.0mm anchor and so
// described a different plate. Measured before: the front reported 12 of 24 used
// while FitSized cuts 16 rows, the back 18 of 24 against 20, and an edited ladder
// that overflowed its own rungs was refused by the fit while admission still said
// "ok" with room to spare.
//
// AdmissibleBlocks itself is untouched. Spec 6 pins it at the uniform 3.0mm
// anchor -- it never reads Block.SizeMM -- and TestAdmissibleBlocksVerdictDoesNotMove
// holds its verdict to measured cliff values for every ordinary plate.
//
// admitQR is kept and is now BELT AND BRACES. The router sends a sized
// composition to a function that has no QR parameter at all, so this expression
// can no longer be the thing that drops the flag; it stays so that a future
// re-route cannot silently re-open the defect it closed.
func ftEvaluate(params engrave.Params, plan *ftPlan, text, title, footer string, useQR bool, size float32) ftFit {
	var f ftFit
	blocks := plan.Blocks(text)
	admitQR := useQR && !ftSizedBlocks(blocks)
	if ftSizedBlocks(blocks) {
		f.linesUsed, f.linesAvail, f.ok = backup.AdmissibleSized(params, blocks, title, footer)
	} else {
		f.linesUsed, f.linesAvail, f.ok = backup.AdmissibleBlocks(params, blocks, title, footer, admitQR)
	}
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
	return ftRungsLabel(rungs)
}

// ftRungsLabel joins rungs the way the ladder titles join them. ONE joiner, so
// the Size screen and the confirm screen cannot disagree about what a given
// ladder is called.
func ftRungsLabel(rungs []float32) string {
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

// The QR step's two leads. The first is the ordinary one: the choice is real,
// and what it costs is that a photograph of the plate is a copy of the text.
//
// The second is what a SIZED composition gets. It states the plate carries no
// code and, in the same breath, WHY -- an option that is merely missing teaches
// the operator nothing, and this is the one screen where they can learn that the
// pattern they just loaded needs the whole plate. Both are two lines at the panel
// width, which is the height ChoiceScreen's lead band is measured for.
const (
	ftQRLead = "A QR is a machine-readable copy of the text. " +
		"Anyone who photographs the plate can read it."
	ftQRLeadSized = "This pattern is cut at several sizes and needs the whole plate. " +
		"It carries no QR and is not machine-readable."
)

// ftQRChoiceFlow is step 1. It comes FIRST so the admission anchor is fixed
// before any text is typed: choosing a QR afterwards would shrink the capacity
// under text already accepted.
//
// blocks is the composition CURRENTLY in the text field, which on the first pass
// is empty and on any later pass is whatever Back was pressed over. When every
// block states its own rung the plate is a size ladder, FitSized has no parameter
// for a code (spec 2.7), and this screen states that instead of offering "Add QR"
// -- spec 3.0. Accepting an answer and discarding it three functions downstream
// is the silent substitution this program exists to avoid, and it was reachable:
// the ladder's loader clears the flag under a prompt that says so, and Back
// returned to a screen that knew nothing about what was loaded and re-seeded
// itself with the very opt-in that had just been cleared.
//
// Scoped to SIZED compositions and nothing else. BOTHPROOF! keeps both answers:
// a code is possible there in principle and merely does not fit, which the
// prompted drop and the capacity refusal already handle out loud.
func ftQRChoiceFlow(ctx *Context, th *Colors, prior bool, blocks []backup.Block) (bool, bool) {
	cs := &ChoiceScreen{Title: "QR Code"}
	sized := ftSizedBlocks(blocks)
	if sized {
		cs.Lead = ftQRLeadSized
		// ONE answer, and it is the state rather than a decision. The prior
		// opt-in is deliberately NOT carried in here: it is the thing this
		// screen exists to stop carrying.
		cs.Choices = []string{"No QR"}
	} else {
		cs.Lead = ftQRLead
		cs.Choices = []string{"No QR", "Add QR"}
		if prior {
			cs.choice = 1 // preserve a deliberate opt-in across Back
		}
	}
	hookPPWidget("qr", cs)
	// choice starts at 0, which is "No QR": the default is a property of this
	// ordering, so do not reorder the choices.
	sel, ok := cs.Choose(ctx, th)
	if !ok {
		return false, false
	}
	// Structurally false in the sized case -- index 1 is not on the screen -- so
	// the flag cannot diverge from a plate that has no code to carry.
	return sel == 1, true
}

// ftSizeAutoFit is the Size screen's first entry and the program's default: the
// fit takes the largest rung the composition holds at, which is what free text
// did before this screen existed.
const ftSizeAutoFit = "Auto-fit"

const (
	ftFaceLead = "The face the whole plate is cut in. font/sh is this " +
		"program's own; font/constant is the face every seed, descriptor and " +
		"passphrase plate uses."
	ftSizeLead = "The size the whole plate is cut at. Auto-fit takes the " +
		"largest that holds the text."
	// A proof composition states its own faces and rungs, so both screens say
	// what it is rather than offering to change it.
	ftFaceLeadFixed = "This pattern states its own faces. They are not a choice here."
	ftSizeLeadFixed = "This pattern states its own sizes. They are not a choice here."
	ftSpeedLead     = "How fast the needle travels while cutting. Slower spaces " +
		"the hammer's dots more closely."
	// Off a proof composition the feed is fixed, so seed, descriptor and
	// passphrase plates can never carry a non-standard one.
	ftSpeedLeadFixed = "The engraving speed is adjustable on test patterns only."
	ftSettingsLead   = "Engraving parameters for this plate. They are not saved."
	ftPassLead       = "How many times each character is cut, without moving. " +
		"More passes cut deeper and take proportionally longer."
	// Off a proof composition the pass count is fixed, for the same reason the
	// feed is: neither may vary on a seed, descriptor or passphrase plate.
	ftPassLeadFixed = "Passes are adjustable on test patterns only."
)

// ftFaceOptions is the Font screen's content: the labels, and the plan each one
// selects.
//
// A PROOF composition is deliberately not in the list. It is STATE rather than
// a decision, so it yields ONE entry naming itself and taking that entry
// changes nothing -- exactly what ftQRChoiceFlow does for a sized composition,
// and for the same reason: a pinned proof plate must not be half-edited into a
// composition nothing measures.
//
// The labels are READ FROM THE FACES and never spelled again here, so this
// screen and ftFaceSummary cannot drift apart.
func ftFaceOptions(cur *ftPlan) ([]string, []*ftPlan) {
	if ftPlanIsProof(cur) {
		return []string{cur.Name()}, []*ftPlan{cur}
	}
	return []string{ftFaceSH.Name, ftFaceConst.Name}, []*ftPlan{&ftPlanSH, &ftPlanConst}
}

// ftPlanIsProof reports a composition that came from a proof trigger rather
// than from the Font screen.
//
// The predicate is deliberately NOT plan.Sized(). ftPlanBoth states no rungs
// and so is not Sized, but its faces and its CONTENT were built together --
// ftProofOutcomeFor reaches a named rung by calling ftBothAt, which rebuilds
// and trims the mixed pattern to fit it. Editing the rung from the Size screen
// cannot do that, so it would leave the pattern's text as it was and cut a
// plate no trigger ever produces. Neither field is editable here for the same
// reason: the proof's parts are only correct together.
func ftPlanIsProof(p *ftPlan) bool {
	return p != &ftPlanSH && p != &ftPlanConst
}

// ftSizeOptions is the Size screen's content: the labels and the rung each one
// selects, 0 meaning auto-fit. cur is handed straight back for a plan that
// states its own rungs, so taking the only entry cannot move the size.
//
// Built by RANGING OVER backup.FontSizes rather than a list written out here.
// That set is the only one every capacity number in backup is measured against,
// so a rung that is offered is a rung that is pinned; a hand-written list is
// how an unpinned size gets onto the screen.
func ftSizeOptions(plan *ftPlan, cur float32) ([]string, []float32) {
	if ftPlanIsProof(plan) {
		// A ladder names its own rungs; any other proof names the size it is
		// currently pinned at, which is auto-fit unless a trigger set one.
		switch {
		case plan.Sized():
			return []string{ftRungsLabel(plan.Rungs())}, []float32{cur}
		case cur == 0:
			return []string{ftSizeAutoFit}, []float32{cur}
		default:
			return []string{fmt.Sprintf("%.1fmm", cur)}, []float32{cur}
		}
	}
	labels := make([]string, 0, len(backup.FontSizes)+1)
	sizes := make([]float32, 0, len(backup.FontSizes)+1)
	labels = append(labels, ftSizeAutoFit)
	sizes = append(sizes, 0)
	for _, s := range backup.FontSizes {
		labels = append(labels, fmt.Sprintf("%.1fmm", s))
		sizes = append(sizes, s)
	}
	return labels, sizes
}

// ftSpeedRungs is the engraving feeds the Speed screen offers, in mm/s.
//
// FIVE FIXED VALUES, and deliberately not a numeric box. The machine has no
// motion safety envelope at all -- no StepperConfig.Validate, no bounds
// constant, no planner guard -- and the degenerate cases were reproduced by
// driving the real planner on 2026-08-06:
//
//	EngravingSpeed = 0   PANICS at engrave.go:1117 (timeScaler.Scale)
//	Jerk           = 0   PANICS at engrave.go:1155 via bezier.go:300
//	Acceleration   = 0   does not panic; plans at 3x its own velocity limit
//
// A list of non-zero values cannot reach any of those. The argument is not that
// typing is hard, it is that the layer which would catch a typo does not exist.
//
// The list spans both sides of the shipped 4mm/s so 4-against-8 can be cut, and
// stays inside ChoiceScreen's silent-overflow budget (it does not scroll, and
// op.Layer draws content OVER the title past roughly seven entries).
var ftSpeedRungs = []float32{8.0, 6.0, 4.0, 2.0, 1.0}

// ftSpeedCeilingMM is the highest feed that may be offered, in mm/s.
//
// PHYSICAL, not a preference, for two independent reasons. It is what upstream
// shipped and validated; and above it engraving crosses INTO the StallGuard
// window -- minimumStallVelocity is 8*mm, giving TCOOLTHRS 234 against an
// engraving TSTEP of 234.4 at exactly 8mm/s -- so a faster feed would start
// reading the hammer's own strikes as stalls. See the worked table at
// cmd/controller/platform_sh2.go's minimumStallVelocity.
const ftSpeedCeilingMM = 8.0

// ftDefaultSpeedMM is the machine's own engraving feed, in mm/s.
func ftDefaultSpeedMM(params engrave.Params) float32 {
	return float32(params.EngravingSpeed) / float32(params.Millimeter)
}

// ftParamsAtSpeed returns params with ONLY EngravingSpeed replaced. A zero
// mmPerSec leaves them untouched.
//
// Speed and TicksPerSecond are deliberately not derived from it. Speed above
// TicksPerSecond is silently rate-limited rather than rejected
// (stepper.go:49-53 clamps to +-1 microstep per tick; fill() has no return value
// and Driver.Knot() inspects no error), and the loss is permanent rather than
// deferred, so every later stroke on the plate would be offset with no warning.
func ftParamsAtSpeed(params engrave.Params, mmPerSec float32) engrave.Params {
	if mmPerSec <= 0 {
		return params
	}
	params.EngravingSpeed = uint(mmPerSec*float32(params.Millimeter) + 0.5)
	return params
}

// ftSpeedNote is the confirm screen's suffix for a non-default feed, empty when
// the machine default is in force.
//
// Nothing on the finished steel records the feed it was cut at, so the operator
// must not be able to approve a non-default one without seeing it. Equally, an
// ordinary plate must not grow a line that never varies.
func ftSpeedNote(params engrave.Params, mmPerSec float32) string {
	if mmPerSec <= 0 || mmPerSec == ftDefaultSpeedMM(params) {
		return ""
	}
	return fmt.Sprintf("  speed: %.1fmm/s", mmPerSec)
}

// ftSpeedOptions is the Speed screen's content: the labels and the feed each one
// selects, 0 meaning "leave the machine default alone".
//
// UNTIL A PROOF PATTERN IS LOADED IT IS ONE ENTRY, naming the default -- state,
// not a decision, the same idiom Font and Size use. That gate is what keeps the
// feed away from seed, descriptor and passphrase plates, and it is load-bearing
// safety rather than a convenience: nothing on a finished plate records the feed
// it was cut at, so an ordinary backup must not be able to carry a strange one.
//
// The gate is proofLoaded and NOT the plan, because TEXTPROOF! and CONSTPROOF!
// resolve to plans the Font screen can produce on its own -- see ftProofLoader.
func ftSpeedOptions(params engrave.Params, proofLoaded bool, cur float32) ([]string, []float32) {
	def := ftDefaultSpeedMM(params)
	if !proofLoaded {
		return []string{fmt.Sprintf("%.1fmm/s (default)", def)}, []float32{cur}
	}
	labels := make([]string, 0, len(ftSpeedRungs))
	speeds := make([]float32, 0, len(ftSpeedRungs))
	for _, s := range ftSpeedRungs {
		l := fmt.Sprintf("%.1fmm/s", s)
		if s == def {
			l += " (default)"
		}
		labels = append(labels, l)
		speeds = append(speeds, s)
	}
	return labels, speeds
}

// ftSpeedChoiceFlow is step 5, and unlike Font and Size it sits AFTER the text.
//
// Those two precede the text because they change plate CAPACITY. The feed
// changes no geometry at all -- only the tick counts on an already-decided
// toolpath -- so it has no reason to come first, and coming last means the flow
// already knows whether a proof keyword was used, since a proof is loaded ON the
// text screen. A picker before Text would have needed a Back to see it.
func ftSpeedChoiceFlow(ctx *Context, th *Colors, params engrave.Params, proofLoaded bool, prior float32) (float32, bool) {
	labels, speeds := ftSpeedOptions(params, proofLoaded, prior)
	cs := &ChoiceScreen{Title: "Speed", Lead: ftSpeedLead, Choices: labels}
	if len(speeds) == 1 {
		cs.Lead = ftSpeedLeadFixed
	}
	// Preserve a deliberate choice across Back; otherwise start on the machine
	// default so the common path is still one checkmark.
	want := prior
	if want <= 0 {
		want = ftDefaultSpeedMM(params)
	}
	if i := slices.Index(speeds, want); i > 0 {
		cs.choice = i
	}
	hookPPWidget("speed", cs)
	sel, ok := cs.Choose(ctx, th)
	if !ok {
		return prior, false
	}
	return speeds[sel], true
}

// ftPassRungs is how many times each glyph may be engraved IN PLACE.
//
// Time is LINEAR in passes: a full proof plate at 4mm/s goes from about 15
// minutes at 1 to about two hours at 8, so the ceiling is a practical one
// rather than a safety one. Six entries, inside ChoiceScreen's budget.
var ftPassRungs = []int{1, 2, 3, 4, 5, 8}

// ftPassOptions is the Passes screen's content: the labels and the pass count
// each one selects.
//
// UNTIL A PROOF PATTERN IS LOADED IT IS ONE ENTRY, handing cur straight back --
// state, not a decision, the same idiom ftSpeedOptions uses. The gate is
// proofLoaded and NOT the plan, for the same reason ftSpeedOptions' is: TEXTPROOF!
// and CONSTPROOF! resolve to plans the Font screen can produce on its own.
func ftPassOptions(proofLoaded bool, cur int) ([]string, []int) {
	if !proofLoaded {
		return []string{"1 (default)"}, []int{cur}
	}
	labels := make([]string, 0, len(ftPassRungs))
	out := make([]int, 0, len(ftPassRungs))
	for _, n := range ftPassRungs {
		l := fmt.Sprintf("%d", n)
		if n == 1 {
			l += " (default)"
		}
		labels, out = append(labels, l), append(out, n)
	}
	return labels, out
}

// ftPassChoiceFlow is the Passes screen, reached from the settings gear rather
// than from the main step sequence.
func ftPassChoiceFlow(ctx *Context, th *Colors, proofLoaded bool, prior int) (int, bool) {
	labels, passes := ftPassOptions(proofLoaded, prior)
	cs := &ChoiceScreen{Title: "Passes", Lead: ftPassLead, Choices: labels}
	if len(passes) == 1 {
		cs.Lead = ftPassLeadFixed
	}
	want := prior
	if want <= 0 {
		want = 1
	}
	if i := slices.Index(passes, want); i > 0 {
		cs.choice = i
	}
	hookPPWidget("passes", cs)
	sel, ok := cs.Choose(ctx, th)
	if !ok {
		return prior, false
	}
	return passes[sel], true
}

// ftSettingsFlow is the gear's first level: pick a parameter, then its value.
//
// TWO LEVELS rather than one flat list, because ChoiceScreen does not scroll and
// op.Layer draws content over its own title past roughly seven entries -- so a
// flat list cannot hold this family once acceleration and jerk join it.
//
// Nothing calls this yet: the gear key that reaches it is a later task. It is
// exercised directly by this package's tests in the meantime.
func ftSettingsFlow(ctx *Context, th *Colors, params engrave.Params, proofLoaded bool, speed *float32, passes *int) {
	for !ctx.Done {
		cs := &ChoiceScreen{
			Title: "Engraving",
			Lead:  ftSettingsLead,
			Choices: []string{
				fmt.Sprintf("Speed: %s", ftSpeedLabel(params, *speed)),
				fmt.Sprintf("Passes: %d", max(1, *passes)),
			},
		}
		hookPPWidget("settings", cs)
		sel, ok := cs.Choose(ctx, th)
		if !ok {
			return // Back leaves settings and returns to the keyboard.
		}
		switch sel {
		case 0:
			if v, ok := ftSpeedChoiceFlow(ctx, th, params, proofLoaded, *speed); ok {
				*speed = v
			}
		case 1:
			if v, ok := ftPassChoiceFlow(ctx, th, proofLoaded, *passes); ok {
				*passes = v
			}
		}
	}
}

// ftSpeedLabel is the settings row's value text: the chosen feed, or the
// machine's own when untouched.
func ftSpeedLabel(params engrave.Params, mmPerSec float32) string {
	if mmPerSec <= 0 {
		return fmt.Sprintf("%.1fmm/s", ftDefaultSpeedMM(params))
	}
	return fmt.Sprintf("%.1fmm/s", mmPerSec)
}

// ftFaceChoiceFlow is step 2. It sits BEFORE the text screen because the face
// changes plate CAPACITY -- 44 columns at 3.0mm in font/sh against 39 in
// font/constant -- and letting the operator type against a capacity that then
// changes underneath them is exactly what the QR step is already placed early
// to avoid.
func ftFaceChoiceFlow(ctx *Context, th *Colors, prior *ftPlan) (*ftPlan, bool) {
	labels, plans := ftFaceOptions(prior)
	cs := &ChoiceScreen{Title: "Font", Lead: ftFaceLead, Choices: labels}
	if len(plans) == 1 {
		cs.Lead = ftFaceLeadFixed
	}
	// Preserve the live face across Back, as the QR step preserves a deliberate
	// opt-in. choice starts at 0, which is sh: the default is a property of
	// ftFaceOptions' ordering, so do not reorder it.
	if i := slices.Index(plans, prior); i > 0 {
		cs.choice = i
	}
	hookPPWidget("face", cs)
	sel, ok := cs.Choose(ctx, th)
	if !ok {
		return prior, false
	}
	return plans[sel], true
}

// ftSizeChoiceFlow is step 3, and it follows the face for the reason the face
// follows the QR: each one narrows what the next screen can hold.
func ftSizeChoiceFlow(ctx *Context, th *Colors, plan *ftPlan, prior float32) (float32, bool) {
	labels, sizes := ftSizeOptions(plan, prior)
	cs := &ChoiceScreen{Title: "Size", Lead: ftSizeLead, Choices: labels}
	if len(sizes) == 1 {
		cs.Lead = ftSizeLeadFixed
	}
	if i := slices.Index(sizes, prior); i > 0 {
		cs.choice = i
	}
	hookPPWidget("size", cs)
	sel, ok := cs.Choose(ctx, th)
	if !ok {
		return prior, false
	}
	return sizes[sel], true
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
//
// It does not get the OTHER message either. "A plate holds N, at the smallest
// size" is a true sentence about a plate cut at one rung and a false one about a
// ladder, whose rows are several sizes and whose capacity is a property of the
// pattern rather than of the plate. The figures are now the ladder's own
// (AdmissibleSized), so the sentence has to be too, or the refusal quotes
// numbers under a sentence that disowns them.
func ftRefuse(ctx *Context, th *Colors, params engrave.Params, plan *ftPlan, f ftFit, text string, useQR bool) bool {
	blocks := plan.Blocks(text)
	if ftSizedBlocks(blocks) {
		showError(ctx, th, "Text", fmt.Sprintf(
			"The text needs %d lines and this pattern's own sizes hold %d. Shorten the Text field.",
			f.linesUsed, f.linesAvail))
		return false
	}
	if !useQR {
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
	// Button2, the middle nav slot, is free on this screen. layoutNavigation
	// indexes a FIXED [3]int by Button-Button1, so a fourth affordance would
	// panic rather than lay out badly -- Back, Clear and OK is the whole budget.
	clearBtn := &Clickable{Button: Button2}
	hookPPWidget("clear", clearBtn)

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
		if clearBtn.Clicked(ctx) {
			// Guarded on the field, not on the button being drawn: a click can
			// be delivered in the same frame the last character is deleted.
			if kbd.Fragment != "" && ftClearPrompt(ctx, th, len(kbd.Fragment)) {
				kbd.Fragment = ""
			}
			continue
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
		// Clear appears only when there IS something to clear, so the screen
		// never offers an action that would do nothing.
		navs := []NavButton{{Clickable: backBtn, Style: StyleSecondary, Icon: assets.IconBack}}
		if kbd.Fragment != "" {
			navs = append(navs, NavButton{Clickable: clearBtn, Style: StyleSecondary, Icon: assets.IconDiscard})
		}
		navs = append(navs, NavButton{Clickable: okBtn, Style: StylePrimary, Icon: assets.IconCheckmark})
		nav, _ := layoutNavigation(&ctx.B, th, dims, navs...)
		titleOp, _ := layoutTitle(ctx, dims.X, th.Text, "Text")
		ctx.Frame(op.Layer(kbdOp, cntOp, nav, titleOp, op.Color(&ctx.B, th.Background)))
	}
	return "", false
}

// ftClearPrompt asks before discarding the whole Text field, and returns true
// only on an explicit yes.
//
// It exists because the field is uncapped -- a loaded proof pattern runs to
// several hundred characters, and clearing one by backspace is hundreds of taps.
// It is PROMPTED because the same gesture would otherwise destroy a long
// composition with no undo: the field is the only copy until the plate is built.
//
// "Keep the text" is index 0, so the default answer is the harmless one. That is
// a property of this ordering and nothing else states it -- do not reorder.
func ftClearPrompt(ctx *Context, th *Colors, n int) bool {
	cs := &ChoiceScreen{
		Title: "Clear Text",
		Lead: fmt.Sprintf(
			"Clear all %d characters? The text cannot be recovered afterwards.", n),
		Choices: []string{"Keep the text", "Clear it"},
	}
	hookPPWidget("clearPrompt", cs)
	sel, ok := cs.Choose(ctx, th)
	return ok && sel == 1
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
// speedNote is appended only for a NON-DEFAULT engraving feed, so an ordinary
// plate does not grow a line that never varies and a test pattern cannot be
// approved without its feed on screen. See ftSpeedNote.
func ftConfirmSummary(ctx *Context, th *Colors, width int, f ftFit, plan *ftPlan, pager, speedNote string) (op.Op, image.Point) {
	useQR := f.plate.QR != nil
	var rt richText
	rt.Add(&ctx.B, ctx.Styles.subtitle, width, th.Text, fmt.Sprintf(
		"%s  %d lines  QR: %s  font: %s%s",
		ftPlateRungs(f.plate), len(f.plate.Lines), ppYesNo(useQR),
		ftFaceSummary(plan, f.plate.Faces, f.plate.Sizes), speedNote))
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
func ftConfirmBody(ctx *Context, th *Colors, width, height, start int, f ftFit, plan *ftPlan, title, footer, speedNote string) ftConfirmView {
	rows := ftConfirmRows(f, title, footer)
	if start < 0 || start >= len(rows) {
		start = 0
	}
	// Measured with a pager string of the same shape as the real one, so the
	// reservation is exact rather than approximately right.
	_, probe := ftConfirmSummary(ctx, th, width, f, plan, ftConfirmPager(0, 0, 0), speedNote)
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
	sum, sumSz := ftConfirmSummary(ctx, th, width, f, plan, pager, speedNote)
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
func ftConfirmFlow(ctx *Context, th *Colors, f ftFit, plan *ftPlan, title, footer, speedNote string) bool {
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
		view = ftConfirmBody(ctx, th, area.Dx(), area.Dy(), start, f, plan, title, footer, speedNote)
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

// freetextEngraveHook receives the finished Plate at the moment the flow hands
// it to the engraver. nil in production; mirrors freetextPlateHook and exists
// for the same reason -- a defect in the CALLER cannot be caught by any unit
// test of the callee.
//
// freetextPlateHook cannot cover this: it reports the backup.Fitted, which
// carries the layout but not the motion config. A flow that computed the
// operator's chosen feed and then handed ftBuildPlate the undecorated params
// would pass every layout assertion in this package while engraving at the
// wrong speed. That is not hypothetical -- mutation testing on 2026-08-06
// dropped ftParamsAtSpeed from the engrave step and the whole suite stayed
// green until this hook existed.
var freetextEngraveHook func(p Plate)

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
	// Face and size come BEFORE the text: both change plate capacity, and the
	// QR step is already placed first for that reason. See ftFaceChoiceFlow.
	ftStepFace
	ftStepSize
	ftStepText
	// Speed comes AFTER the text, unlike face and size: it changes no geometry,
	// and by here the flow knows whether a proof keyword was used. See
	// ftSpeedChoiceFlow.
	ftStepSpeed
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
	// The engraving feed in mm/s, or 0 for the machine's own. Held here beside
	// the other fields so it survives Back exactly as they do, and discarded
	// when the program returns -- there is no persistence, by design.
	var speed float32
	// Set by any accepted proof trigger; see ftProofLoader. This and not the
	// plan is what unlocks the Speed screen.
	var proofLoaded bool
	step := ftStepQR
	for !ctx.Done {
		switch step {
		case ftStepQR:
			// The composition as it stands NOW: empty on the first pass, and on a
			// Back into this step whatever is in the field. A size ladder there
			// takes the whole plate, so the step states that rather than offering
			// a code it would drop (spec 3.0).
			add, ok := ftQRChoiceFlow(ctx, th, useQR, plan.Blocks(text))
			if !ok {
				return // Back out of the first step leaves the program.
			}
			useQR = add
		case ftStepFace:
			p, ok := ftFaceChoiceFlow(ctx, th, plan)
			if !ok {
				step -= 2
				break
			}
			plan = p
		case ftStepSize:
			// Handed the LIVE plan, not the prior one: a face chosen on the
			// previous screen decides whether this one offers rungs at all.
			s, ok := ftSizeChoiceFlow(ctx, th, plan, size)
			if !ok {
				step -= 2
				break
			}
			size = s
		case ftStepText:
			s, ok := ftTextEntryFlow(ctx, th, params, text, &title, &footer, &plan, &useQR, &size,
				ftProofLoader(params, &text, &title, &footer, &plan, &useQR, &size, &proofLoaded))
			if !ok {
				step -= 2
				break
			}
			text = s
		case ftStepSpeed:
			s, ok := ftSpeedChoiceFlow(ctx, th, params, proofLoaded, speed)
			if !ok {
				step -= 2
				break
			}
			speed = s
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
			f := ftEvaluate(ftParamsAtSpeed(params, speed), plan, text, title, footer, useQR, size)
			if f.err != nil || !f.ok {
				// A title or footer entered after the text can only ever make
				// the composition SMALLER on the plate, never inadmissible --
				// admission reserves both rows unconditionally. So this is a
				// genuine surprise, and it goes back rather than engraving.
				showError(ctx, th, "Text", "This text does not fit a plate.")
				step -= 2
				break
			}
			if !ftConfirmFlow(ctx, th, f, plan, title, footer, ftSpeedNote(params, speed)) {
				step -= 2
				break
			}
		case ftStepEngrave:
			plate, err := ftBuildPlate(ftParamsAtSpeed(params, speed), plan, text, title, footer, useQR, size)
			if err != nil {
				// The message quotes no field content.
				showError(ctx, th, "Text", "This text does not fit a plate.")
				step -= 2
				break
			}
			if freetextEngraveHook != nil {
				freetextEngraveHook(plate)
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
