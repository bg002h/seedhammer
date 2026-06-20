package gui

import (
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

// TestBuildFlow_GatherBeforeSeed asserts the gather-before-seed ordering (no
// secret exists during gather, mirroring T6b's posture): with no NFC reader the
// gather yields zero cards, so a Build flow at n=2 returns on gather Back WITHOUT
// typing a seed (the seed-hook never fires). R0 M-b: Done-with-zero shows an
// in-gather error and STAYS; Back exits the gather -> the flow returns ok=false.
func TestBuildFlow_GatherBeforeSeed(t *testing.T) {
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
		// Pick template (wsh, default), n=2 (default), k (default), @S (default),
		// fp Omit (default) by confirming each picker.
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
		click(&ctx.Router, Button3) // k=1
		frame()
		if _, ok := pumpUntil(frame, "Your slot", 16); !ok {
			t.Fatal("self-slot picker not shown")
		}
		click(&ctx.Router, Button3) // @0
		frame()
		if _, ok := pumpUntil(frame, "Fingerprints", 16); !ok {
			t.Fatal("fp picker not shown")
		}
		click(&ctx.Router, Button3) // Omit
		// Now the gather runs; with no NFC reader, press Done -> zero cards -> the
		// gather shows its own "No complete cards yet" error and STAYS (R0 M-b).
		if _, ok := pumpUntil(frame, "Engrave Bundle", 16); !ok {
			t.Fatal("gather screen not shown")
		}
		click(&ctx.Router, Button3) // Done (zero cards) -> in-gather showError, stays
		// The empty-Done showError is a dismiss-only ErrorScreen (Button3 dismisses);
		// dismiss it, then we are back on the gather screen.
		if _, ok := pumpUntil(frame, "No complete cards", 16); !ok {
			t.Fatal("empty-Done error not shown")
		}
		click(&ctx.Router, Button3) // dismiss the error modal -> back at the gather
		if _, ok := pumpUntil(frame, "Engrave Bundle", 16); !ok {
			t.Fatal("did not return to gather after dismissing the empty-Done error")
		}
		// Drive Back to LEAVE the gather -> bundleGatherFlow returns ok=false, so
		// the flow returns without ever typing a seed.
		click(&ctx.Router, Button1) // Back from gather -> flow returns (ok=false)
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
}
