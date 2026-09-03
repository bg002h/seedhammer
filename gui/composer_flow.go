package gui

import (
	"slices"

	"seedhammer.com/md"
)

// composerFlow is "Build a new policy" (SPEC_wallet_policy_composer.md §7),
// from the door to the plate.
//
// PART A's VERSION, AND IT BUILDS WITH PART B ABSENT. That is the whole point
// of the split: the plan's architecture prose and its Global Constraints both
// say "Part A ships alone", and a Task A11 whose own fence called
// composerSeatFlow, composerMappingReview, composerConsentFlow, composerFormPick
// and composerMintCards would make that false of the artifact meant to deliver
// it -- an implementer following the plan task by task could not compile the
// milestone the plan promises here. Task B11 REPLACES this file wholesale to
// insert seating between the stub screen and consent; nothing here reaches
// forward into it.
//
// WHAT PART A ALONE IS HONEST FOR: the C26 no-payload journey, §12 item 3.
// With a payload loaded the door states "Keys loaded: N" over a Build that
// cannot yet use them, which is why Part B follows immediately. §7e's
// self-check arrives with it (Task B6) -- Part A's consent is the decoded
// md1's own lines plus §8l, and the plan says so rather than implying the
// check is present.
//
// THE SCRUB IS INSTALLED HERE, at the top, before any seed can exist -- the
// same construction gui/multisig_build.go:290-291 uses and for the reason
// stated there: every exit below (a Back, a refusal, a ctx.Done unwind, a
// panic) is covered without an implementer remembering to add one to a new
// return. C14 asks for Multisig Build's treatment and this is it.
func composerFlow(ctx *Context, th *Colors) {
	st := &composerState{reg: &seedRegistry{}, bound: composerBoundFrom(ctx.sysw)}
	defer st.reg.scrub()

	w, ok := composerWrapperPick(ctx, th)
	if !ok {
		return
	}
	st.list.Wrapper = w

	var shown []string // the chunk set the stub screen last displayed (§8s)
	for !ctx.Done {
		if !composerShapeFlow(ctx, th, st) {
			// BACK AT THE PATH LIST GOES BACK ONE STEP, to the wrapper, with
			// the list intact -- §7b's "going back should lose nothing".
			w, ok := composerWrapperPick(ctx, th)
			if !ok {
				return
			}
			st.list.Wrapper = w
			continue
		}
		c, err := md.Compose(st.list)
		if err != nil {
			composerShowRefusal(ctx, th, "Spend paths", err)
			continue
		}
		chunks, err := c.Chunks()
		if err != nil {
			showError(ctx, th, "Template", "Couldn't build the template from this shape.")
			continue
		}
		// §8s's changed-id line is decided by COMPARING CHUNK SETS, not by an
		// "edited" flag that any Back would set and nothing would reset. A
		// false "this id changed" on the screen whose job is to be copied onto
		// steel trains the operator to discount the line that will one day be
		// true.
		changed := shown != nil && !slices.Equal(shown, chunks)
		if !composerStubFlow(ctx, th, chunks, nil, changed) {
			shown = chunks
			continue
		}
		shown = chunks

		listed, keyPathNo := composerListedPaths(st.list)
		lines, err := composerConsentLinesFor(chunks, listed, keyPathNo)
		if err != nil {
			showError(ctx, th, "Review", composerCopySelfCheckFailed())
			return
		}
		if !composerReadScreen(ctx, th, "Review", lines) {
			continue
		}
		// §8l, unskippable, immediately before the first thing that cuts.
		if !composerConfirmScreen(ctx, th, "Before you fund it",
			composerConfirmBody(composerCopyNothingChecked())) {
			continue
		}
		composerEngraveTemplate(ctx, th, chunks)
		return
	}
}

// composerEngraveTemplate cuts the keyless template through the SHIPPED
// bundle machinery, so the plate planning, the census and the engrave screen
// are the ones every other md1 goes through -- the composer contributes the
// strings and nothing else (the I-VERBATIM rule gui/multisig_build.go:30-32
// states for its own md1).
//
// §7f's form choice COLLAPSES here and says so: with no slot seated there is
// no concrete policy and there are no cards, so "template only" is the whole
// of the offer. Task B11's composerEngraveStep replaces this with the full
// §7f choice once seating exists.
func composerEngraveTemplate(ctx *Context, th *Colors, chunks []string) bool {
	showError(ctx, th, "What to engrave",
		"No slot is seated, so there is a template and nothing else.")
	cards := []bundleCard{{
		kind:    cardMD1,
		label:   "md1 template",
		strings: chunks,
		summary: "key-less wallet policy",
	}}
	// buildPlateCensusLines, not composerCensusLines: the latter is Task B9's
	// and adds §7f's recovery-error line, which arrives with the cards it is
	// about. Part A cuts one md1 and counts it through the same
	// bundlePlatePlan every other plate goes through.
	if !confirmReviewScreen(ctx, th, "Plate Count",
		buildPlateCensusLines(ctx.Platform.EngraverParams(), cards)) {
		return false
	}
	return bundleEngrave(ctx, th, "Wallet Policy", cards, "", "") == bundleEngraveDone
}
