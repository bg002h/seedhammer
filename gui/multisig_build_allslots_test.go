package gui

import (
	"testing"
	"testing/synctest"
)

// ─── I-6: a build that needs ZERO cosigner cards ─────────────────────────────
//
// classifyCosignerSupply refused whenever `state != cosignerSourceLoaded`
// REGARDLESS of `open`, because its first case is a comma-OR:
//
//	case state != cosignerSourceLoaded, have < open:
//
// so a build needing no cosigner cards at all was stopped by a demand for a
// cosigner-card payload, and the refusal contradicted itself in one sentence:
//
//	"No payload is loaded, and this policy needs NO COSIGNER KEY CARDS. This
//	 device has no card reader: pack the cards on the host with `me sysw pack`,
//	 load the payload, then build."
//
// showError is dismiss-only and dismissing returns straight out of
// buildMultisigPolicyFlow, so the whole build was abandoned with no forward
// route on the screen.
//
// PRE-S5 THIS WAS UNREACHABLE, because `open` was always p.N-1 >= 1. S5's
// multi-select @S picker is what made it reachable: pick @0, answer "YES, ONE
// MORE", and at n=2 the last slot auto-adds -- a path
// TestSelfSlotPickerNeverAsksAOneAnswerQuestion already exercises and passes.
// Trace B does not catch it either (3 held of n=4, so open == 1). The premise
// that hid it is the stale comment asserting the @S picker "always sets exactly
// one held slot", false since 4b10319.

// TestZeroDemandBuildIsNotRefusedForAPayloadItDoesNotNeed is the classification
// table's missing cell, across ALL THREE payload states.
//
// The shipped table had only open=2 rows. The three states must AGREE at
// open == 0, and the measured agreement point is cosignerAutoFill: that is what
// `state == cosignerSourceLoaded, open == 0` already yielded, so the other two
// are brought to it rather than the reverse.
func TestZeroDemandBuildIsNotRefusedForAPayloadItDoesNotNeed(t *testing.T) {
	for _, tc := range []struct {
		name  string
		state cosignerSourceState
		have  int
	}{
		{"no payload at all", cosignerSourceNoPayload, 0},
		{"payload loaded but not compared", cosignerSourceUncompared, 0},
		{"payload loaded", cosignerSourceLoaded, 0},
		{"payload loaded and over-supplied", cosignerSourceLoaded, 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyCosignerSupply(tc.state, tc.have, 0)
			if got == cosignerRefuse {
				t.Fatalf("a build needing NO cosigner cards was refused for want of a "+
					"cosigner-card payload (state=%v have=%d open=0).\nThe refusal it "+
					"produces says both things at once:\n%s",
					tc.state, tc.have, buildSupplyRefusal(tc.state, tc.have, 0, false))
			}
			if got != cosignerAutoFill {
				t.Errorf("classified as %v, want cosignerAutoFill: the three payload "+
					"states must agree when the policy demands nothing, and autoFill is "+
					"what the loaded state already yielded", got)
			}
		})
	}

	// NON-VACUITY: the refusal must still fire when cards ARE needed. Without
	// this, `return cosignerAutoFill` unconditionally passes the table above and
	// the under-supply guard is gone.
	for _, tc := range []struct {
		state cosignerSourceState
		have  int
	}{
		{cosignerSourceNoPayload, 0},
		{cosignerSourceUncompared, 0},
		{cosignerSourceLoaded, 1},
	} {
		if got := classifyCosignerSupply(tc.state, tc.have, 2); got != cosignerRefuse {
			t.Errorf("state=%v have=%d open=2 classified as %v; under-supply is still a "+
				"named refusal (SPEC 0.1 clause 1: no authority licenses inventing a "+
				"cosigner key)", tc.state, tc.have, got)
		}
	}
}

// TestBuildHoldingEVERYSlotReachesTheSeed is the FLOW-level arm, and it is what
// the classification table cannot see.
//
// An operator builds a 2-of-2 they hold entirely and enters both seeds on the
// keyboard. No payload is needed for anything -- seedEntryFlow offers the
// keyboard -- and yet the flow demanded one and dead-ended.
//
// IT ASSERTS THE SEED ENTRY IS REACHED, not merely that the refusal is gone. The
// review's minimal fix (make classifyCosignerSupply return autoFill at open==0)
// is NOT sufficient on its own: it lets the flow through to bundleGatherFlow,
// which needs at least one complete card to proceed and answers Done on zero
// cards with "No complete cards. Pack them on the host with `me sysw pack` and
// load the payload again." That is the SAME dead end one screen later. So the
// flow skips the payload machinery entirely when it demands nothing, and this
// test is what tells the two fixes apart.
func TestBuildHoldingEverySlotReachesTheSeed(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p := newPlatform()
		p.display = sh2DisplaySize
		ctx := NewContext(p)
		// NO PAYLOAD AT ALL: ctx.sysw stays nil, which is the state the shipped
		// refusal named.
		done := false
		frame, quit := runUI(ctx, func() {
			buildMultisigPolicyFlow(ctx, &descriptorTheme)
			done = true
		})
		defer quit()

		pick := func(stage string, downs int) {
			t.Helper()
			if c, ok := pumpUntil(frame, stage, 32); !ok {
				t.Fatalf("the %q picker was not shown; the flow is on %q", stage, c)
			}
			for d := 0; d < downs; d++ {
				click(&ctx.Router, Down)
				frame()
			}
			click(&ctx.Router, Button3)
			frame()
		}
		pick("Template", 0)
		pick("Cosigners", 0)                 // n = 2
		pick("Threshold", 1)                 // k = 2
		pick("Which slot is your key?", 0)   // @0
		pick("Do you hold another slot?", 1) // YES, ONE MORE -> @1 auto-adds (n=2)
		pick("Fingerprints", 0)

		// THE FLOW MUST REACH THE SEED. Anything else is the dead end: the
		// self-contradictory refusal, or the gather's own empty-set refusal.
		c, ok := pumpUntil(frame, "Choose number of words", 64)
		if !ok {
			t.Fatalf("a build holding EVERY slot never reached seed entry.\nScreen "+
				"reads: %q\n"+
				"Both dead ends read the same way to an operator: the flow demands a "+
				"payload of cosigner cards for a policy that needs none, and the only "+
				"button dismisses out of the build entirely", c)
		}
		if uiContains(c, "me sysw pack") {
			t.Fatalf("the flow reached a host-route refusal for a policy that needs no "+
				"cosigner cards: %q", c)
		}

		// And it really is the two-held-slot build: the seed prompt names @0, and a
		// SECOND one follows for @1. A flow that quietly reverted to one held slot
		// would satisfy everything above.
		if !uiContains(c, "@0") {
			t.Errorf("the first seed prompt does not name the slot it is for: %q", c)
		}
		click(&ctx.Router, Button3) // 12 WORDS
		frame()
		driveWords(&ctx.Router, fixtureMasterA)
		if c, ok := pumpUntil(frame, "passphrase", 240); !ok {
			t.Fatalf("the passphrase prompt for @0 was not reached; got %q", c)
		}
		click(&ctx.Router, Button3) // Skip
		frame()
		c, ok = pumpUntil(frame, "Choose number of words", 96)
		if !ok {
			t.Fatalf("the SECOND held slot's seed was never asked for; got %q.\n"+
				"open==0 means both slots are the operator's, so the flow owes two seed "+
				"entries", c)
		}
		if !uiContains(c, "@1") {
			t.Errorf("the second seed prompt does not name @1: %q", c)
		}

		// BACK NOW STEPS BACK (2026-08-19 operator directive: "going back should
		// lose nothing"). Back at the SECOND held slot returns to the FIRST
		// slot's seed entry instead of leaving — losing one slot's work, not
		// both. This test leaves by backing out TWICE: once to @0, once more
		// from the first seed entry, which is still an exit.
		click(&ctx.Router, Button1)
		c, ok = pumpUntil(frame, "@0", 128)
		if !ok {
			t.Fatalf("Back at the second seed entry did not return to @0's; got %q", c)
		}
		if done {
			t.Fatal("Back at the SECOND seed entry LEFT the flow — the " +
				"pre-2026-08-19 behaviour, which discarded @0's seed too")
		}
		click(&ctx.Router, Button1)
		for i := 0; i < 192 && !done; i++ {
			frame()
		}
		if !done {
			t.Fatal("Back at the FIRST seed entry did not leave the flow")
		}
	})
}
