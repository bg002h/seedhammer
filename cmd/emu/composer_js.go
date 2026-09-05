//go:build js

package main

import (
	"encoding/hex"
	"syscall/js"

	"seedhammer.com/gui"
)

// installComposerAPI exposes the running composition's stored path hashes to the
// page as window.shComposerPathHashes.
//
//	shComposerPathHashes()   [ "<64 hex>" | null, ... ]  one entry per spend
//	                         path, in path order, null where a path carries no
//	                         hash; null (not an array) when no composition is
//	                         running.
//
// WHY A WALK NEEDS IT (H5 §4, F-485). Every other reading primitive here reports
// what was DRAWN. That is the right default -- a walk is evidence about the
// screen an operator sees -- but the hashlock phrase route ends by writing a
// digest into the policy, and "the confirm modal displayed b867db87..edbc96cb"
// and "the policy now holds b867db87..." are different claims. Two defects lived
// in the gap: a hash assigned BEFORE the hold-to-confirm (so Back after reading
// the digest left it set), and a stored digest that differs from the displayed
// one. Both are caught by CI's gui tests; the walk that closes the stage saw
// neither.
//
// READING ONLY, AND THAT IS NOT A CONVENTION. gui.ComposerPathHashes hands back
// COPIES of the digests (gui/composer_state_hook.go), so there is nothing here
// to write through. Driving stays with shTap and its siblings, which inject the
// events a finger would.
//
// FULL 64 HEX, not the first8..last8 the screens draw: the point of the call is
// to compare what is stored against what was shown, and comparing an
// abbreviation against an abbreviation would accept 2^192 wrong digests.
func installComposerAPI() {
	js.Global().Set("shComposerPathHashes", js.FuncOf(func(js.Value, []js.Value) any {
		hashes := gui.ComposerPathHashes()
		if hashes == nil {
			// No composition is running. Distinguishable from a composition
			// with no paths, which is an empty ARRAY -- a walk that could not
			// tell those apart would read "the flow is not running" as "no path
			// holds a hash" and pass on a screen it never reached.
			return nil
		}
		out := make([]any, 0, len(hashes))
		for _, h := range hashes {
			if h == nil {
				out = append(out, nil)
				continue
			}
			out = append(out, hex.EncodeToString(h[:]))
		}
		return out
	}))
}
