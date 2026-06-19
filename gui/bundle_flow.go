package gui

// bundleFlow is the engraveBundle program: gather a bundle of PUBLIC md1/mk1
// cards over NFC (Phase 1), review/confirm them (Phase 2), then engrave all
// cards' plates verbatim with cross-card guidance (Phase 3). The full
// implementation lands in Tasks 3 and 4; this entry point is wired into the
// program dispatch (gui.go) by the Task-2 lockstep.
func bundleFlow(ctx *Context, th *Colors) {
	// Phase 1+2+3 — implemented in Tasks 3 and 4.
}
