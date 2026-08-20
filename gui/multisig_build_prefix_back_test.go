package gui

import (
	"testing"
)

// buildPrefixWalk drives buildMultisigPolicyFlow's PREFIX -- the param pickers,
// the self-source question, and the cosigner gather -- and stops at the screen
// named by `until`, without answering it.
//
// The step list is s5DriveBuildToEngravePicker's (multisig_engrave_tail_walk_test.go),
// truncated: this file is about what Back does on those screens, not about the
// engrave tail beyond them.
func buildPrefixWalk(t *testing.T, ctx *Context, frame func() (string, bool), until string) {
	t.Helper()
	for _, s := range []buildWalkStep{
		{needle: "Choose policy type", downs: 0},            // wsh
		{needle: "How many keys (n)?", downs: 1},            // n = 3
		{needle: "Required signatures (k of 3)?", downs: 1}, // k = 2
		{needle: "Which slot is your key?", downs: 0},       // self @0
		{needle: "Do you hold another slot?", downs: 0},     // NO, THAT IS ALL
		{needle: "Include key fingerprints?", downs: 0},     // omit
		{needle: "key on a card?", downs: 0},                // NO, JUST MY SEED
		{needle: buildCosignerGatherTitle, downs: 0},        // Done adding cards
	} {
		c, ok := pumpUntil(frame, s.needle, 96)
		if !ok {
			t.Fatalf("the build never reached %q; screen reads %q", s.needle, c)
		}
		if s.needle == until {
			return
		}
		for i := 0; i < s.downs; i++ {
			click(&ctx.Router, Down)
			frame()
		}
		click(&ctx.Router, Button3)
		frame()
	}
	t.Fatalf("the walk never stopped at %q", until)
}

func buildPrefixCtx(t *testing.T) (*Context, func() (string, bool), *bool, func()) {
	t.Helper()
	records := cosignerCardRecords(t, 4) // A@0, B@0, C@0, A@1
	p := newPlatform()
	p.display = sh2DisplaySize
	ctx := NewContext(p)
	ctx.sysw = sessionHolding(records...)
	done := new(bool)
	frame, quit := runUI(ctx, func() {
		buildMultisigPolicyFlow(ctx, &descriptorTheme)
		*done = true
	})
	return ctx, frame, done, quit
}

// TestBuildBackAtSelfSourceReturnsToTheParams is the first of the two prefix
// Backs left over from the seed-loop conversion (2026-08-19 directive, "going
// back should lose nothing").
//
// Both of them used a bare `return`, so a Back on either screen abandoned the
// whole build -- every parameter pick with it -- rather than stepping back one
// screen.
func TestBuildBackAtSelfSourceReturnsToTheParams(t *testing.T) {
	ctx, frame, done, quit := buildPrefixCtx(t)
	defer quit()
	buildPrefixWalk(t, ctx, frame, "key on a card?")

	click(&ctx.Router, Button1) // Back at the self-source question
	c, ok := pumpUntil(frame, "Include key fingerprints?", 96)
	if *done {
		t.Fatal("Back at the self-source question ABANDONED the build, discarding " +
			"every parameter the operator had already picked")
	}
	if !ok {
		t.Fatalf("Back at the self-source question did not return to the last "+
			"parameter screen; got %q", c)
	}
}

// TestBuildBackAtTheGatherReturnsToSelfSource is the second one. The gather's
// predecessor is the self-source question, so that is where Back belongs --
// this build reaches it, since the payload supplies enough cards to make the
// question askable.
func TestBuildBackAtTheGatherReturnsToSelfSource(t *testing.T) {
	ctx, frame, done, quit := buildPrefixCtx(t)
	defer quit()
	buildPrefixWalk(t, ctx, frame, buildCosignerGatherTitle)

	click(&ctx.Router, Button1) // Back at the cosigner gather
	c, ok := pumpUntil(frame, "key on a card?", 96)
	if *done {
		t.Fatal("Back at the cosigner gather ABANDONED the build rather than " +
			"returning to the question before it")
	}
	if !ok {
		t.Fatalf("Back at the cosigner gather did not return to the self-source "+
			"question; got %q", c)
	}
}
