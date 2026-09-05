package gui

// Task 3 stub. Task 4 Step 3 REPLACES this whole file; the type and BOTH
// constants are declared here because composerHashEdit's switch names
// hashlockAssigned, so a stub that declared only the function would not compile.
type hashlockOutcome int

const (
	hashlockAssigned hashlockOutcome = iota
	hashlockBackToWhichHash
)

func hashlockPhraseRoute(ctx *Context, th *Colors, st *composerState, idx int, payload [][32]byte) hashlockOutcome {
	return hashlockBackToWhichHash
}
