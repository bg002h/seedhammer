package gui

import (
	"testing"
	"testing/synctest"
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
