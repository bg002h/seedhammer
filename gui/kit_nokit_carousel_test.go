package gui

import "testing"

// A payload that offers no kit (no mr1 candidate), loaded from the carousel with
// nothing loaded, returns to the carousel: no payload menu follows (14e, 14d).
func TestNoKitPayloadFromCarouselOpensNoMenu(t *testing.T) {
	ctx := NewContext(kitPlatform(plaintextRegion(t, chainPayloadNamed(t, "chain-codex32").Records...)))
	frame, drawer, quit := runUITouch(ctx, func() { syswPayloadMenu(ctx, &descriptorTheme) })
	defer quit()
	if c, ok := pumpUntil(frame, "Compare this against", 32); !ok {
		t.Fatalf("no digest screen; got %q", c)
	}
	tapNavSlot(t, ctx, drawer(), Button3)
	if c, ok := pumpUntil(frame, "Payload Warnings", 32); !ok {
		t.Fatalf("no Payload Warnings; got %q", c)
	}
	tapNavSlot(t, ctx, drawer(), Button3)
	if c, ok := pumpUntil(frame, "Keep this payload loaded?", 32); !ok {
		t.Fatalf("no KEEP; got %q", c)
	}
	click(&ctx.Router, Button3) // KEEP
	for i := 0; i < 24; i++ {
		c, more := frame()
		if !more {
			return // flow returned to the carousel
		}
		if uiContains(c, "It holds") || uiContains(c, "ENGRAVE KIT") {
			t.Fatalf("a payload that offers no kit opened the payload menu after the load: %q", c)
		}
	}
}
