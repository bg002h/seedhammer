package gui

import (
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"seedhammer.com/bip39"
)

// TestMultisigFrontDoorRouting drives the new choose-or-supply front-door at the
// top of engraveMultisigFlow: the first screen offers exactly
// ["Supply policy (md1)", "Build policy"]; choosing Supply (index 0) reaches the
// existing T6b gather ("Engrave Bundle" gather title), and choosing Build
// (index 1) reaches the new Build path's first picker ("Template" / script type).
func TestMultisigFrontDoorRouting(t *testing.T) {
	t.Run("supply reaches the existing gather", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			ctx := NewContext(newPlatform())
			frame, quit := runUI(ctx, func() { engraveMultisigFlow(ctx, &descriptorTheme) })
			defer quit()
			// Front-door appears.
			if c, ok := pumpUntil(frame, "Supply policy", 16); !ok {
				t.Fatalf("front-door not shown; got %q", c)
			}
			// Choose index 0 (Supply) -> the existing T6b body runs the bundle gather.
			click(&ctx.Router, Button3) // default selection is index 0
			if c, ok := pumpUntil(frame, "Engrave Bundle", 16); !ok {
				t.Fatalf("Supply did not reach the existing gather; got %q", c)
			}
		})
	})
	t.Run("build reaches the new flow", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			ctx := NewContext(newPlatform())
			frame, quit := runUI(ctx, func() { engraveMultisigFlow(ctx, &descriptorTheme) })
			defer quit()
			if c, ok := pumpUntil(frame, "Supply policy", 16); !ok {
				t.Fatalf("front-door not shown; got %q", c)
			}
			// Down to index 1 (Build), confirm.
			click(&ctx.Router, Down)
			frame()
			click(&ctx.Router, Button3)
			if c, ok := pumpUntil(frame, "Template", 16); !ok {
				t.Fatalf("Build did not reach the template picker; got %q", c)
			}
		})
	})
	t.Run("back from the front-door returns", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			ctx := NewContext(newPlatform())
			done := false
			frame, quit := runUI(ctx, func() {
				engraveMultisigFlow(ctx, &descriptorTheme)
				done = true
			})
			defer quit()
			if c, ok := pumpUntil(frame, "Supply policy", 16); !ok {
				t.Fatalf("front-door not shown; got %q", c)
			}
			click(&ctx.Router, Button1) // Back
			for i := 0; i < 16 && !done; i++ {
				frame()
			}
			if !done {
				t.Fatal("engraveMultisigFlow did not return on Back from the front-door")
			}
		})
	})
}

// TestBuildParamPickBackNav drives buildParamPickFlow's stage loop and asserts
// that Back navigates back ONE stage through the full picker sequence
// (template -> n -> k -> @S -> fp), and that Back from the FIRST stage
// (template) abandons the Build flow (ok==false). This is the NIT-2 fix: Back
// from n used to abandon the whole flow rather than re-show the template.
func TestBuildParamPickBackNav(t *testing.T) {
	t.Run("back from k re-shows n, then template (no abandon)", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			ctx := NewContext(newPlatform())
			done := false
			var gotOK bool
			frame, quit := runUI(ctx, func() {
				_, gotOK = buildParamPickFlow(ctx, &descriptorTheme)
				done = true
			})
			defer quit()
			// Forward: template (wsh, default) -> n (default) -> k.
			if _, ok := pumpUntil(frame, "Template", 16); !ok {
				t.Fatal("template picker not shown")
			}
			click(&ctx.Router, Button3) // template wsh
			frame()
			if _, ok := pumpUntil(frame, "Cosigners", 16); !ok {
				t.Fatal("n picker not shown")
			}
			click(&ctx.Router, Button3) // n=2
			frame()
			if _, ok := pumpUntil(frame, "Threshold", 16); !ok {
				t.Fatal("k picker not shown")
			}
			// Back once: k -> n (re-shown, NOT an abandon).
			click(&ctx.Router, Button1)
			if _, ok := pumpUntil(frame, "Cosigners", 16); !ok {
				t.Fatal("Back from k did not re-show the n picker")
			}
			if done {
				t.Fatal("flow abandoned on Back from k; want re-show n")
			}
			// Back again: n -> template (re-shown, NOT an abandon).
			click(&ctx.Router, Button1)
			if _, ok := pumpUntil(frame, "Template", 16); !ok {
				t.Fatal("Back from n did not re-show the template picker")
			}
			if done {
				t.Fatal("flow abandoned on Back from n; want re-show template")
			}
			// Back from template DOES abandon -> ok==false.
			click(&ctx.Router, Button1)
			for i := 0; i < 16 && !done; i++ {
				frame()
			}
			if !done {
				t.Fatal("flow did not return on Back from the template (first) stage")
			}
			if gotOK {
				t.Fatal("Back from template returned ok==true; want false (abandon)")
			}
		})
	})
}

// TestMultisigBuildExperimentalWarningAbort: Back (Button1) at the EXPERIMENTAL
// warning drives ConfirmWarningScreen.Layout -> ConfirmNo, so the warning
// returns false (abort). Mirrors TestChildSeedWarningAbort (NON-vacuous: the
// goroutine actually reaches + dismisses the warning).
func TestMultisigBuildExperimentalWarningAbort(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := NewContext(newPlatform())
		var got bool
		done := false
		frame, quit := runUI(ctx, func() {
			got = multisigBuildExperimentalWarning(ctx, &descriptorTheme)
			done = true
		})
		defer quit()
		if c, ok := pumpUntil(frame, "EXPERIMENTAL", 16); !ok {
			t.Fatalf("experimental warning not shown; got %q", c)
		}
		click(&ctx.Router, Button1) // Back -> ConfirmNo
		for i := 0; i < 16 && !done; i++ {
			frame()
		}
		if !done {
			t.Fatal("warning did not return after Back")
		}
		if got {
			t.Fatal("warning returned true after Back; want false (abort, no engrave)")
		}
	})
}

// TestMultisigBuildExperimentalWarningConfirm: holding Button3 confirms
// (ConfirmYes -> true), the only route past the warning.
func TestMultisigBuildExperimentalWarningConfirm(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := NewContext(newPlatform())
		var got bool
		done := false
		frame, quit := runUI(ctx, func() {
			got = multisigBuildExperimentalWarning(ctx, &descriptorTheme)
			done = true
		})
		defer quit()
		if c, ok := pumpUntil(frame, "EXPERIMENTAL", 16); !ok {
			t.Fatalf("experimental warning not shown; got %q", c)
		}
		press(&ctx.Router, Button3) // hold to confirm
		frame()
		time.Sleep(confirmDelay)
		for i := 0; i < 16 && !done; i++ {
			frame()
		}
		if !done {
			t.Fatal("warning did not return after hold-confirm")
		}
		if !got {
			t.Fatal("warning returned false after hold-confirm; want true")
		}
	})
}

// buildWalkParamPickers accepts every buildParamPickFlow default in turn
// (template wsh -> n=2 -> k=1 -> @0 -> "no further slots" -> fp Omit), leaving
// the flow at whatever comes after the pickers. Each stage is asserted on the
// way past, because tapping through a picker that did not appear silently
// answers the NEXT one.
//
// S5's multi-select @S picker added the "Do you hold another slot?" stage, whose
// default row is "NO, THAT IS ALL". So the all-defaults walk still produces the
// one-element set {@0} that every caller of this helper was written against --
// the extra tap is a screen, not a change of outcome.
//
// The new stage is matched on its LEAD rather than its title: the title is
// "Your slots", which contains "Your slot" as a substring, so a title match
// would find the wrong screen.
func buildWalkParamPickers(t *testing.T, ctx *Context, frame func() (string, bool)) {
	t.Helper()
	for _, stage := range []string{"Template", "Cosigners", "Threshold", "Your slot",
		"Do you hold another slot?", "Fingerprints"} {
		if _, ok := pumpUntil(frame, stage, 16); !ok {
			t.Fatalf("%s picker not shown", stage)
		}
		click(&ctx.Router, Button3)
		frame()
	}
}

// TestBuildFlow_GatherBeforeSeed asserts the gather-before-seed ordering: no
// secret exists while the cosigner set is being resolved, mirroring T6b's
// posture. The seed-hook must never fire on any route that leaves the flow
// before assembly.
//
// S1 CHANGED THE NO-PAYLOAD ARM and this test changed with it. It used to drive
// a reader-less machine into the gather, press Done on zero cards, and read the
// gather's own "No complete cards yet — scan a card's chunks first". That is
// exactly the refusal S1 exists to remove: this phase's data entry is the
// payload and the keyboard, so "scan a card's chunks first" prescribed a route
// the flow does not take and dead-ended the operator. (The SH2 does HAVE a
// reader — a soldered ST25R3916 — so the defect was never absent hardware; it
// was a prompt pointing away from the only route the flow offers. Operator,
// 2026-08-15.) The Build path now refuses BEFORE the gather and names the host
// route. The property under
// test is unchanged — no seed is typed — so both arms assert it.
func TestBuildFlow_GatherBeforeSeed(t *testing.T) {
	t.Run("no payload refuses before the gather, naming the host route", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			seedTyped := false
			buildMultisigSeedHook = func(bip39.Mnemonic) { seedTyped = true }
			defer func() { buildMultisigSeedHook = nil }()
			ctx := NewContext(newPlatform())
			done := false
			frame, quit := runUI(ctx, func() {
				buildMultisigPolicyFlow(ctx, &descriptorTheme)
				done = true
			})
			defer quit()
			buildWalkParamPickers(t, ctx, frame)
			content, ok := pumpUntil(frame, "me sysw pack", 16)
			if !ok {
				t.Fatalf("a reader-less machine with no payload did not name the host "+
					"route; got %q", content)
			}
			if strings.Contains(content, "scan") || strings.Contains(content, "Scan") {
				t.Errorf("the refusal still tells the operator to scan, which phase 1 "+
					"removed with NFC: %q", content)
			}
			click(&ctx.Router, Button3) // dismiss the modal -> the flow returns
			for i := 0; i < 32 && !done; i++ {
				frame()
			}
			if !done {
				t.Fatal("flow did not return after the refusal")
			}
			if seedTyped {
				t.Fatal("a seed was typed on a flow that never had a cosigner set")
			}
		})
	})

	t.Run("with a payload the gather runs, and Back leaves before any seed", func(t *testing.T) {
		records := cosignerCardRecords(t, 1)
		synctest.Test(t, func(t *testing.T) {
			seedTyped := false
			buildMultisigSeedHook = func(bip39.Mnemonic) { seedTyped = true }
			defer func() { buildMultisigSeedHook = nil }()
			ctx := NewContext(newPlatform())
			ctx.sysw = sessionHolding(records...)
			done := false
			frame, quit := runUI(ctx, func() {
				buildMultisigPolicyFlow(ctx, &descriptorTheme)
				done = true
			})
			defer quit()
			buildWalkParamPickers(t, ctx, frame)
			if _, ok := pumpUntil(frame, "mk1 keys: 1", 16); !ok {
				t.Fatal("the payload's card never reached the gather tally")
			}
			// Back LEAVES the gather -> bundleGatherFlow returns ok=false, so the
			// flow returns without ever typing a seed.
			click(&ctx.Router, Button1)
			for i := 0; i < 32 && !done; i++ {
				frame()
			}
			if !done {
				t.Fatal("flow did not return after gather Back")
			}
			if seedTyped {
				t.Fatal("seed was typed BEFORE the cosigner gather; gather must precede seed entry")
			}
		})
	})
}
