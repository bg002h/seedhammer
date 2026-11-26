package md

import "errors"

// ─── Template-engrave shape guard (C4 — NEW refusal code) ────────────────────
//
// The shipped off-device toolkit's template_admissible refuses shapes that do
// not render through rust-miniscript: tr(sortedmulti_a) (the SortedMultiA render
// gap) and sortedmulti nested under a combinator (legacy multi renders fine and
// is ADMITTED — only sortedmulti is the render-gap shape). The fork does NOT port
// rust-miniscript, so these shapes ENCODE/STRIP fine at the wire level — they
// would be silently engraved as an UNRECOVERABLE backup. templateEngraveShapeGuard
// adds the refusal on the TEMPLATE-ENGRAVE path (the default full-policy path is
// unchanged). tr(NUMS, multi_a) is ADMITTED (the toolkit ships it), and so is
// tr(sortedmulti_a) since F-215 — see guardNode. A hardened
// use-site is NOT refused here — its restoration concern is an off-device
// derive/address matter, not the template wire (it strips/encodes fine).

var errTemplateUnsupportedShape = errors.New("md: template-engrave refuses this shape (tr(sortedmulti_a) / sortedmulti-in-combinator: unrecoverable with shipped tooling)")

// templateEngraveShapeGuard refuses the render-gap template shapes. It is called
// on the decoded descriptor at the template-engrave opt-in confirm (Tasks 6/7),
// BEFORE any engrave.
func templateEngraveShapeGuard(d *descriptor) error {
	return guardNode(d.tree, false)
}

// ErrTemplateUnsupportedShape is returned by TemplateEngraveShapeGuardChunks for
// a render-gap shape the shipped toolkit cannot reconstruct.
var ErrTemplateUnsupportedShape = errTemplateUnsupportedShape

// TemplateEngraveShapeGuardChunks is the chunked-md1-input form of the guard: it
// reassembles the wire strings and refuses the render-gap shapes (the GUI
// template-engrave opt-in calls this before engrave). Reassemble errors surface
// unchanged.
func TemplateEngraveShapeGuardChunks(strs []string) error {
	d, err := Reassemble(strs)
	if err != nil {
		return err
	}
	return templateEngraveShapeGuard(d)
}

// guardNode walks the tree. inCombinator becomes true once we descend past the
// canonical wrapper spine (wsh/sh/sh-wsh) into a combinator/script body, so a
// sortedmulti under it is refused (legacy multi is always admitted); a tap-script
// tree refuses sortedmulti_a on any leaf.
func guardNode(root node, inCombinator bool) error {
	// An explicit stack in pre-order, children left to right, so the first
	// refusal is the recursion's (review r3 I-P1: a max-depth card recursed
	// once per level here).
	type item struct {
		n            node
		inCombinator bool
	}
	work := []item{{root, inCombinator}}
	for len(work) > 0 {
		it := work[len(work)-1]
		work = work[:len(work)-1]
		children, childIn, err := guardStep(it.n, it.inCombinator)
		if err != nil {
			return err
		}
		for i := len(children) - 1; i >= 0; i-- {
			work = append(work, item{children[i], childIn})
		}
	}
	return nil
}

// guardStep checks n itself and returns the children guardNode walks next,
// with the context they are walked in.
func guardStep(n node, inCombinator bool) (children []node, childInCombinator bool, err error) {
	switch n.tag {
	case tagSortedMultiA:
		// ADMITTED SINCE 2026-08-21 (F-215). This arm refused on the grounds that
		// `sortedmulti_a` "has no rust-miniscript renderer" — true when written,
		// and false since the S0 pin lift (PR #910 gave upstream
		// `Terminal::SortedMultiA`) and R5.
		//
		// Re-measured on the current binaries before this line changed, because
		// an enumerated safety argument goes stale silently:
		//
		//	md encode  tr(@0,sortedmulti_a(2,@0,@1))  -> 1 chunk
		//	md decode  -> exit 0, the template verbatim
		//	md verify  -> re-encodes to its own template
		//	md address -> bc1p588jmtx4ptv76t9sclt6gt33eyydvsrea4njyayerqj2frw5m5aq5gzycw
		//
		// Fully recoverable with the shipped tooling, which is the only thing
		// this guard was ever asking. Refusing it now blocks a legitimate
		// template engrave (D4) and nothing else. Convergence toward the
		// primary, which admits the shape at every stage.
		return nil, false, nil
	case tagMultiA:
		// multi_a renders (tr(NUMS, multi_a) is shipped) → admit; nothing below.
		return nil, false, nil
	case tagSortedMulti:
		if inCombinator {
			// sortedmulti nested under a combinator → refuse (no rust-miniscript renderer).
			return nil, false, errTemplateUnsupportedShape
		}
		return nil, false, nil
	case tagMulti:
		// Legacy multi renders fine inside combinators — the shipped toolkit ADMITS
		// it (e.g. the §5 degrade2 wallet's wsh(or_i(and_v(...multi(3,...)),...)));
		// only sortedmulti is the render-gap shape. Always admitted (C1, exec-review).
		return nil, false, nil
	case tagWsh:
		// wsh wrapper: its sole child stays on the canonical spine (NOT a
		// combinator) so a direct wsh(sortedmulti) is admitted; deeper wrapping is
		// handled by the child's own arm.
		if b, ok := n.body.(childrenBody); ok {
			return b.children, false, nil
		}
		return nil, false, nil
	case tagSh:
		// sh wrapper: a direct sh(sortedmulti/multi) and sh(wsh(...)) stay on the
		// spine; everything else its child decides.
		if b, ok := n.body.(childrenBody); ok {
			return b.children, false, nil
		}
		return nil, false, nil
	case tagTr:
		// Taproot: a key-path-only tr is fine; a script tree is walked as a
		// COMBINATOR context (its leaves are reached via tap branching) — so a
		// sortedmulti leaf there is refused, and sortedmulti_a is refused by its own
		// arm. multi_a and legacy multi are admitted.
		if b, ok := n.body.(trBody); ok && b.tree != nil {
			return []node{*b.tree}, true, nil
		}
		return nil, false, nil
	case tagTapTree:
		if b, ok := n.body.(childrenBody); ok {
			return b.children, true, nil
		}
		return nil, false, nil
	default:
		// A combinator / miniscript fragment (or_i, and_v, thresh, …): descend with
		// inCombinator=true so any sortedmulti below is refused (legacy multi admitted).
		switch b := n.body.(type) {
		case childrenBody:
			return b.children, true, nil
		case variableBody:
			return b.children, true, nil
		}
		return nil, false, nil
	}
}
