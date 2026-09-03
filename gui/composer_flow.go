package gui

import (
	"fmt"
	"slices"

	"seedhammer.com/codex32"
	"seedhammer.com/md"
)

// composerFlow is "Build a new policy" (SPEC_wallet_policy_composer.md §7),
// from the door to the plate, in §7's order.
//
// THIS FUNCTION IS THE JOIN, and its absence was the defect two R0 lenses
// found independently. Part B built fourteen production functions --
// composerKeySources, composerCardSources, composerSeatFlow, composerShortfall,
// composerMappingReview, composerConsentFlow, composerFormPick,
// composerMintCards, composerCensusLines and the rest -- and nothing called
// any of them, so an operator with four `key:` records in flash read the
// door's "Keys loaded: 4", chose Build, and was never offered a slot. §7e's
// self-check and §8q never executed on a shipped device; §7f's form choice,
// card minting and the census's card-chunk count were unreachable; and §7b's
// live line read `keys available: 0` whatever the payload held. Go does not
// error on an unused package-scope function, and every TestComposer* called
// these directly, so nothing was red. The lesson is recorded in memory as
// "plans list components and omit the call that joins them"; the gate against
// it is composerFlowWalk's flow-level test and the reachability test beside it.
//
// THE SCRUB IS INSTALLED HERE, at the top, before any seed can exist -- the
// same construction gui/multisig_build.go:290-291 uses, so every exit below (a
// Back, a refusal, a ctx.Done unwind, a panic) is covered without an
// implementer remembering to add one to a new return (C14).
func composerFlow(ctx *Context, th *Colors) {
	st := &composerState{reg: &seedRegistry{}, bound: composerBoundFrom(ctx.sysw)}
	defer st.reg.scrub()

	// SOURCES ARE LOADED BEFORE THE SHAPE, so §7b's live line
	// ("slots: N / keys available: M") is right from the first frame. Loading
	// them at seating time would make the line that helps the operator SIZE the
	// policy read zero for the whole of the decision it exists to inform.
	st.sources = append(composerKeySources(ctx), composerCardSources(ctx)...)

	w, ok := composerWrapperPick(ctx, th)
	if !ok {
		return
	}
	st.list.Wrapper = w

	// §7b's step is "Wrapper -> preset or blank -> paths", and this is
	// that middle step (§4d, task A10). Declining the picker is the BLANK
	// route, not an error: composerShapeFlow below is unchanged and opens
	// on an empty path list, which is exactly what it did before presets
	// existed. Accepting seeds the list with the chosen archetype, whose
	// shape is pinned to the Rust primary's own exported vector.
	if list, ok := composerPresetPick(ctx, th, w); ok {
		st.list = list
	}

	var shown []string // the chunk set the stub screen last displayed (§8s)
	for !ctx.Done {
		if !composerShapeFlow(ctx, th, st) {
			// BACK AT THE PATH LIST GOES BACK ONE STEP, to the wrapper, with
			// the list intact -- §7b's "going back should lose nothing". It
			// used to return, dropping the wrapper, every path, every lock and
			// every digest.
			w, ok := composerWrapperPick(ctx, th)
			if !ok {
				return
			}
			st.list.Wrapper = w
			continue
		}
		composerSizeAssignments(st)

		template, err := composerTemplateChunksFor(st)
		if err != nil {
			composerShowRefusal(ctx, th, "Template", err)
			continue
		}
		// §8s's changed-id line is decided by COMPARING CHUNK SETS, not by an
		// "edited" flag. The flag was set on any Back out of the stub screen,
		// the consent or §8l and never reset, so re-reaching the stub screen
		// without touching the shape asserted that the id had changed -- a
		// false statement on the screen whose job is to be copied onto steel,
		// which trains the operator to discount the line that will one day be
		// true.
		changed := shown != nil && !slices.Equal(shown, template)
		if !composerStubFlow(ctx, th, template, nil, changed) {
			shown = template
			continue
		}
		shown = template

		if !composerSeatingStep(ctx, th, st) {
			continue
		}
		template, keyed, err := composerArtifactsFor(st)
		if err != nil {
			composerShowRefusal(ctx, th, "Template", err)
			continue
		}
		if len(keyed) > 0 && !composerStubFlow(ctx, th, template, keyed, false) {
			continue
		}
		consent := template
		if len(keyed) > 0 {
			consent = keyed
		}
		if !composerConsentFlow(ctx, th, st, consent) {
			continue
		}
		if composerEngraveStep(ctx, th, st, template, keyed) {
			return
		}
	}
}

// composerSizeAssignments sizes st.assigned to the shape's slot count AT FLOW
// LEVEL, not at seating entry.
//
// Sizing it only inside composerSeatFlow left a composition that never entered
// seating -- C26's key-less template, §12 item 3 -- failing composerSelfCheck
// on slot count alone. An unseated slot is src == -1, never a zero value: zero
// would read as "seated from source 0".
func composerSizeAssignments(st *composerState) {
	n := composerSlotCount(st.list)
	if len(st.assigned) == n {
		return
	}
	st.assigned = make([]composerAssignment, n)
	for i := range st.assigned {
		st.assigned[i].src = -1
	}
}

// composerDeclaredOrigins projects the seating onto md.ComposeWith's input.
// An unseated slot declares nothing and the codec assigns §4f's lowest free
// account for it.
func composerDeclaredOrigins(st *composerState) []*md.SlotOrigin {
	out := make([]*md.SlotOrigin, len(st.assigned))
	for i, a := range st.assigned {
		if a.src < 0 {
			continue
		}
		out[i] = &md.SlotOrigin{Origin: a.origin, Fingerprint: a.fingerprint, FpPresent: a.fpPresent}
	}
	return out
}

// composerTemplateChunksFor emits the keyless template for the current shape
// and seating.
func composerTemplateChunksFor(st *composerState) ([]string, error) {
	c, err := md.ComposeWith(st.list, composerDeclaredOrigins(st))
	if err != nil {
		return nil, err
	}
	return c.Chunks()
}

// composerArtifactsFor emits the template AND, when every slot is seated, the
// keyed policy.
//
// COMPOSED TWICE, NOT COPIED: md.Composed's own doc says a copy shares the
// underlying descriptor, so Bind on one keys them both -- and a "template"
// that had been keyed by its own policy is not a template.
func composerArtifactsFor(st *composerState) (template, keyed []string, err error) {
	declared := composerDeclaredOrigins(st)
	ct, err := md.ComposeWith(st.list, declared)
	if err != nil {
		return nil, nil, err
	}
	if template, err = ct.Chunks(); err != nil {
		return nil, nil, err
	}
	if !composerSeatingComplete(st) {
		return template, nil, nil
	}
	pub := map[uint8][65]byte{}
	fps := map[uint8][4]byte{}
	for i, a := range st.assigned {
		cc, pk, _, derr := decodeXpubBytes(a.xpub)
		if derr != nil {
			return nil, nil, derr
		}
		var b [65]byte
		copy(b[0:32], cc[:])
		copy(b[32:65], pk[:])
		pub[uint8(i)] = b
		if a.fpPresent {
			fps[uint8(i)] = a.fingerprint
		}
	}
	ck, err := md.ComposeWith(st.list, declared)
	if err != nil {
		return nil, nil, err
	}
	if err := ck.Bind(pub, fps); err != nil {
		return nil, nil, err
	}
	if keyed, err = ck.Chunks(); err != nil {
		return nil, nil, err
	}
	return template, keyed, nil
}

// composerSeatingStep is §7d: seat every slot, or take §8p's exit.
//
// Offered only when the payload holds keys or seeds, or the operator asks to
// type one -- §7d's own condition. With nothing to seat from, a key-less
// template is the C26 answer and the flow goes straight on.
func composerSeatingStep(ctx *Context, th *Colors, st *composerState) bool {
	if len(st.sources) == 0 {
		cs := &ChoiceScreen{
			Title:   "Keys",
			Lead:    "Seat keys into this template?",
			Choices: []string{"Engrave a key-less template", "Type a seed"},
		}
		sel, ok := cs.Choose(ctx, th)
		if !ok {
			return false
		}
		if sel == 0 {
			return true
		}
		src, ok := composerSeedSource(ctx, th, st)
		if !ok {
			return false
		}
		st.sources = append(st.sources, src)
	}
	if !composerSeatFlow(ctx, th, st) {
		return false
	}
	if !composerSeatingComplete(st) && !composerShortfall(ctx, th, st) {
		return false
	}
	return composerMappingReview(ctx, th, st)
}

// composerEngraveStep is §7f: the form choice, then what it implies.
func composerEngraveStep(ctx *Context, th *Colors, st *composerState, template, keyed []string) bool {
	form, ok := composerFormPick(ctx, th, st)
	if !ok {
		return false
	}
	var cards []bundleCard
	switch form {
	case composerFormConcrete:
		// FORM A IS THE KEYED md1, and only that, this cycle. §7f also names
		// "plain-text plates" and "QR plates" of the concrete descriptor, and
		// md DELIBERATELY EMITS NO TEXT: "a rendering that cannot be re-parsed
		// is the defect this package's invariant exists to prevent"
		// (md/compose.go's header). Adding a descriptor renderer is normative
		// and lands in Rust first, so it is filed (F-457) rather than invented
		// here. composerDescriptorCeilingChars stays as §13 item 1's
		// measurement, which is what that number was asked for.
		cards = []bundleCard{{
			kind: cardMD1, label: "md1 policy", strings: keyed,
			summary: "the wallet policy, with its keys",
		}}
	default:
		cards = []bundleCard{{
			kind: cardMD1, label: "md1 template", strings: template,
			summary: "key-less wallet policy",
		}}
		minted, err := composerMintCards(st, template, keyed)
		if err != nil {
			showError(ctx, th, "Key cards", "Couldn't mint a key card for a seated slot.")
			return false
		}
		cards = append(cards, minted...)
	}

	// FULL vs WATCH-ONLY, and only where a seed is actually held: the question
	// is meaningless when no slot came from one.
	if st.reg.count() > 0 {
		full, ok := composerEngraveModePick(ctx, th, st)
		if !ok {
			return false
		}
		if full {
			secrets, err := composerSecretCards(st)
			if err != nil {
				showError(ctx, th, "Secret plates", "Couldn't encode a seed as ms1.")
				return false
			}
			cards = append(secrets, cards...)
		}
	}

	// The census carries the SUPPLY paths' title (gui/multisig.go,
	// gui/singlesig.go), not Multisig Build's: the build title is a registered
	// walk anchor (cmd/emu/needle_test.go) whose proof is that exactly one flow
	// draws it, and the composer is a supply-shaped flow, not a build.
	if !confirmReviewScreen(ctx, th, "Plates To Cut",
		composerCensusLines(ctx.Platform.EngraverParams(), cards)) {
		return false
	}
	return bundleEngrave(ctx, th, "Wallet Policy", cards, "", "") == bundleEngraveDone
}

// composerSecretCards is §7f's "a seed that filled several slots is cut ONCE".
//
// THE DEDUP IS BY REGISTERED SEED, not by slot: one seed at three slots is one
// secret, and cutting it three times would triple the bearer plates in the set
// for no recovery value. The form is ms1, which is what the bundle machinery
// carries (cardMS1, gui/multisig_engrave.go:36) and what Multisig Build's own
// Full mode cuts. The words-plus-SeedQR plate is a backup.Seed, not a bundle
// card, and needs its own plate pass; it is filed with F-455 rather than
// offered by a picker with no builder behind it.
func composerSecretCards(st *composerState) ([]bundleCard, error) {
	seen := map[int]bool{}
	var out []bundleCard
	for _, a := range st.assigned {
		if a.src < 0 || a.src >= len(st.sources) {
			continue
		}
		src := st.sources[a.src]
		if src.kind != composerSourceSeed || seen[src.seedID] {
			continue
		}
		seen[src.seedID] = true
		seed, ok := st.reg.at(src.seedID)
		if !ok {
			continue
		}
		entropy := seed.Mnemonic.Entropy()
		ms1, err := codex32.EncodeMS1(entropy)
		wipeBytes(entropy)
		if err != nil {
			return nil, err
		}
		out = append(out, bundleCard{
			kind:    cardMS1,
			label:   fmt.Sprintf("ms1 secret share %d", len(out)+1),
			strings: []string{ms1},
			summary: "secret seed backup",
		})
	}
	return out, nil
}
