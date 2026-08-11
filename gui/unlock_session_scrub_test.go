package gui

import "testing"

// TestSecretSessionScrubsOnANormalExit is F-107's original filing, and the half
// that stayed unbuilt through three review rounds while its Critical (the
// outgrown-array class) was being fixed.
//
// ctx.B.Scrub() had exactly ONE call site, gui/run_flow.go:245, inside the
// §10.2.4 wipe path. Buffer.Reset runs per frame and only TRUNCATES args, and
// op.Glyph encodes every drawn rune into args as a uint32 -- so on a normal exit
// the rendered words come back verbatim and in order from the backing array.
//
// The exposed path is the COMMON one: read your words, press back. The protected
// path -- walk away for 3:30 -- is the rare one.
//
// Assertion point (round 0 M4, round 2 N-b): the Context is NOT abandoned on
// this path -- §F-107 proves a normal exit keeps the same Context and the same
// Buffer. Only the harness ends the run, by bounding the flow at the session
// return. So the assertion is taken after *h.done, which is after the session's
// defer has run and before anything reuses the buffer.
//
// Mutation this pins: delete ctx.B.Scrub() from unlockSecretSession's defer.
func TestSecretSessionScrubsOnANormalExit(t *testing.T) {
	p := unlockedPayload(t, "F")

	h := runSecretSession(t, p)
	// Offer every secret and decline each, so the session ends of its own
	// accord: a NORMAL exit, with no wipe anywhere in it.
	for i := 0; i < 8 && !*h.done; i++ {
		c, ok := h.frame()
		if !ok {
			break
		}
		h.content = c
		if uiContains(c, "SECRET seed material") {
			h.choose(1) // Skip
		}
	}
	for i := 0; i < 64 && !*h.done; i++ {
		if _, ok := h.frame(); !ok {
			break
		}
	}
	if !*h.done {
		t.Fatal("INCONCLUSIVE: the session never returned, so no normal exit happened")
	}

	args, refs := h.ctx.B.Residue()
	if args != 0 || refs != 0 {
		t.Errorf("after a NORMAL session exit the frame buffer still holds %d non-zero args "+
			"and %d non-nil refs -- op.Glyph wrote every rendered rune into args, and only "+
			"the wipe path scrubbed", args, refs)
	}
}
