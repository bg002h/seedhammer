package gui

// engraveSingleSigFlow is the engraveSingleSig program orchestrator: a typed
// BIP-39 seed (SECRET) → wallet-type pick → derive ms1+mk1+md1 (policy-bound) →
// engrave (full or watch-only) → verify-bundle → watch-only restore doc.
//
// The full orchestrator is assembled in Task 7; Task 1 wires only the program +
// lockstep, so this is a minimal entry point.
func engraveSingleSigFlow(ctx *Context, th *Colors) {
	// Implemented in Task 7.
}
