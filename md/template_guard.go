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
// unchanged). tr(NUMS, multi_a) is ADMITTED (the toolkit ships it). A hardened
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
func guardNode(n node, inCombinator bool) error {
	switch n.tag {
	case tagSortedMultiA:
		// SortedMultiA (tap leaf) has no rust-miniscript renderer → refuse.
		return errTemplateUnsupportedShape
	case tagMultiA:
		// multi_a renders (tr(NUMS, multi_a) is shipped) → admit; nothing below.
		return nil
	case tagSortedMulti:
		if inCombinator {
			// sortedmulti nested under a combinator → refuse (no rust-miniscript renderer).
			return errTemplateUnsupportedShape
		}
		return nil
	case tagMulti:
		// Legacy multi renders fine inside combinators — the shipped toolkit ADMITS
		// it (e.g. the §5 degrade2 wallet's wsh(or_i(and_v(...multi(3,...)),...)));
		// only sortedmulti is the render-gap shape. Always admitted (C1, exec-review).
		return nil
	case tagWsh:
		// wsh wrapper: its sole child stays on the canonical spine (NOT a
		// combinator) so a direct wsh(sortedmulti) is admitted; deeper wrapping is
		// handled by the child's own arm.
		if b, ok := n.body.(childrenBody); ok {
			for _, c := range b.children {
				if err := guardNode(c, false); err != nil {
					return err
				}
			}
		}
		return nil
	case tagSh:
		// sh wrapper: a direct sh(sortedmulti/multi) and sh(wsh(...)) stay on the
		// spine; everything else its child decides.
		if b, ok := n.body.(childrenBody); ok {
			for _, c := range b.children {
				if err := guardNode(c, false); err != nil {
					return err
				}
			}
		}
		return nil
	case tagTr:
		// Taproot: a key-path-only tr is fine; a script tree is walked as a
		// COMBINATOR context (its leaves are reached via tap branching) — so a
		// sortedmulti leaf there is refused, and sortedmulti_a is refused by its own
		// arm. multi_a and legacy multi are admitted.
		if b, ok := n.body.(trBody); ok && b.tree != nil {
			return guardNode(*b.tree, true)
		}
		return nil
	case tagTapTree:
		if b, ok := n.body.(childrenBody); ok {
			for _, c := range b.children {
				if err := guardNode(c, true); err != nil {
					return err
				}
			}
		}
		return nil
	default:
		// A combinator / miniscript fragment (or_i, and_v, thresh, …): descend with
		// inCombinator=true so any sortedmulti below is refused (legacy multi admitted).
		if b, ok := n.body.(childrenBody); ok {
			for _, c := range b.children {
				if err := guardNode(c, true); err != nil {
					return err
				}
			}
		}
		if b, ok := n.body.(variableBody); ok {
			for _, c := range b.children {
				if err := guardNode(c, true); err != nil {
					return err
				}
			}
		}
		return nil
	}
}
