//go:build !tinygo

// The composition-state seam, and the FOURTH build-tagged pair in this package.
// plate_hook.go states the general argument for the split, frame_hook.go the
// measurement discipline around it, and engraved_hook.go what a hook may
// announce; read those first, because everything they say applies here.
//
// WHAT IT IS FOR (H5 §4, F-485). cmd/emu's walk of the hashlock phrase route
// asserted the tokens the SCREEN drew and nothing about what the composition
// STORED, so two defects passed it: a hash assigned before the hold-to-confirm,
// and a stored digest that differs from the displayed one. Both are caught by
// the gui tests in CI; neither was caught by the gate the stage closes on, and a
// walk that cannot see the difference between "the screen says d" and "the
// policy holds d" is asserting the weaker of the two claims at the moment funds
// depend on the stronger.
//
// composerState is a LOCAL of composerFlow (gui/composer_flow.go:34) with no
// path out of this package, which is why a hook is needed at all: there is no
// accessor to add, no field to export, and giving the state a package-level home
// to make it readable would be a far larger change than the seam.
//
// WHAT A WALK MAY DO WITH IT, stated once and normatively: READ, to assert that
// what the screen shows equals what is stored. It never DRIVES through this
// hook. The driving primitives are window.shTap and its siblings (cmd/emu/walk_js.go),
// which inject the events a finger would; anything that let a walk reach past a
// screen would make the walk prove less than the operator's own hands do, which
// is the opposite of the point.
//
// WHY `!tinygo` AND NOT AN EXPORTED ACCESSOR. The same rule frame_hook.go
// applies: the consumer is JavaScript on a page, outside anything Go can wipe,
// so the firmware must not merely decline to use this, it must not carry it.
// What travels here is a set of 32-byte digests -- public values, and by H2 §4's
// design the preimage never leaves the stack -- but the rule is structural
// rather than a judgement about this payload, and a structural rule that is
// relaxed once for a value that seemed harmless is not a rule.
//
// WHAT IT COSTS, MEASURED: see composer_state_hook_tinygo.go.
package gui

// composerStateHook reports each spend path's hash for the composition that is
// running NOW, in path order, nil where a path carries none.
//
// nil except while composerFlow is running: it is installed at the top of the
// flow and cleared when the flow returns, so a consumer that calls it from the
// start screen gets nil rather than the last composition's digests. A stale
// answer is worse than no answer here -- a walk asserting "path 1 holds no hash
// yet" would pass on a previous run's cleared state.
var composerStateHook func() []*[32]byte

// setComposerStateHook installs read access to st for the composition's
// lifetime. Paired with clearComposerStateHook by composerFlow's defer, on the
// same construction the seed scrub uses there, so every exit -- a Back, a
// refusal, a ctx.Done unwind, a panic -- clears it.
//
// The closure COPIES each digest rather than handing out st's pointers: the
// caller is JavaScript, the state is live, and a *[32]byte into an md.SpendPath
// would let a consumer write the policy this hook exists to observe.
func setComposerStateHook(st *composerState) {
	composerStateHook = func() []*[32]byte {
		out := make([]*[32]byte, len(st.list.Paths))
		for i, p := range st.list.Paths {
			if p.Hash == nil {
				continue
			}
			d := *p.Hash
			out[i] = &d
		}
		return out
	}
}

func clearComposerStateHook() {
	composerStateHook = nil
}

// ComposerPathHashes is the consumer's entry point: each path's hash for the
// running composition, or nil when none is running.
//
// Exported because cmd/emu is a different package; it exists only in this
// build-tagged file, so the firmware has nothing to export.
func ComposerPathHashes() []*[32]byte {
	if composerStateHook == nil {
		return nil
	}
	return composerStateHook()
}
