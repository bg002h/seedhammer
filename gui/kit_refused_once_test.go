package gui

import "testing"

// S1 14e: "Kit Refused" shows ONCE at every load, after Payload Warnings and its
// KEEP: at boot (the menu that follows shows it) and at a load from the
// carousel with nothing loaded (no menu follows, so the load flow shows it).
func TestKitRefusedOnceAtEveryLoad(t *testing.T) {
	const sentence = "Record 2 of this payload is not an md1, mk1 or mr1 string (it is: codex32 secret), so this payload is not a kit."
	for _, boot := range []bool{true, false} {
		p := kitPlatform(plaintextRegion(t, kitWithMs1(t)...))
		ctx := NewContext(p)
		frame, drawer, quit := runUITouch(ctx, func() {
			if boot {
				if syswLoadFlow(ctx, &descriptorTheme, ctx.Platform.SyswReader(), true) {
					syswPayloadMenu(ctx, &descriptorTheme)
				}
				return
			}
			syswPayloadMenu(ctx, &descriptorTheme) // carousel Load Payload, nothing loaded
		})
		if boot {
			pumpUntil(frame, "Load it?", 32)
			click(&ctx.Router, Button3)
		}
		pumpUntil(frame, "Compare this against", 32)
		tapNavSlot(t, ctx, drawer(), Button3)
		if c, ok := pumpUntil(frame, "Payload Warnings", 32); !ok {
			t.Fatalf("boot=%v: no Payload Warnings; got %q", boot, c)
		}
		tapNavSlot(t, ctx, drawer(), Button3)
		if c, ok := pumpUntil(frame, "Keep this payload loaded?", 32); !ok {
			t.Fatalf("boot=%v: no KEEP / UNLOAD; got %q", boot, c)
		}
		click(&ctx.Router, Button3) // KEEP
		c, ok := pumpUntil(frame, "Kit Refused", 32)
		if !ok {
			t.Fatalf("boot=%v: no Kit Refused after KEEP; got %q", boot, c)
		}
		kitSentences(t, c, sentence)
		click(&ctx.Router, Button3) // dismiss
		menu := false
		for i := 0; i < 24; i++ {
			c, more := frame()
			if !more {
				break
			}
			if uiContains(c, "Kit Refused") {
				t.Fatalf("boot=%v: Kit Refused shown a second time at one load", boot)
			}
			if uiContains(c, "It holds") {
				menu = true
				break // boot: today's menu follows, once
			}
		}
		if boot && !menu {
			t.Errorf("boot: today's menu did not follow Kit Refused")
		}
		if !boot && menu {
			t.Errorf("carousel load: a menu followed the load flow")
		}
		quit()
	}
}
