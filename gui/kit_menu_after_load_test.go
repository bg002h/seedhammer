package gui

import "testing"

// Brian 2026-11-22 (s1 I-2 option (b)): after EVERY load of a payload that
// offers a kit the payload menu opens: the "Engrave kit now" lead for an
// eligible kit, "Kit Refused" once and then today's menu for a refused one.
// Routes: the carousel with nothing loaded, and LOAD AGAIN.

func TestKitMenuOpensAfterCarouselLoadOfEligibleKit(t *testing.T) {
	ctx := NewContext(kitPlatform(plaintextRegion(t, kitVectorRecords(t, "kit-single")...)))
	frame, drawer, quit := runUITouch(ctx, func() { syswPayloadMenu(ctx, &descriptorTheme) })
	defer quit()
	if c, ok := pumpUntil(frame, "Compare this against", 32); !ok {
		t.Fatalf("no digest screen; got %q", c)
	}
	tapNavSlot(t, ctx, drawer(), Button3)
	c, ok := pumpUntil(frame, "ENGRAVE KIT", 32)
	if !ok {
		t.Fatalf("no kit menu after a carousel load; got %q", c)
	}
	kitSentences(t, c, "Engrave kit now (1 card, 1 plate)?", "LOAD AGAIN", "UNLOAD")
}

func TestKitMenuOpensAfterLoadAgainOfEligibleKit(t *testing.T) {
	ctx := NewContext(kitPlatform(plaintextRegion(t, kitVectorRecords(t, "kit-single")...)))
	ctx.sysw = sessionWith(kitVectorRecords(t, "kit-single")...)
	frame, drawer, quit := runUITouch(ctx, func() { syswPayloadMenu(ctx, &descriptorTheme) })
	defer quit()
	if c, ok := pumpUntil(frame, "ENGRAVE KIT", 32); !ok {
		t.Fatalf("no kit menu to start; got %q", c)
	}
	click(&ctx.Router, Down, Button3) // LOAD AGAIN
	if c, ok := pumpUntil(frame, "Compare this against", 32); !ok {
		t.Fatalf("LOAD AGAIN: no digest screen; got %q", c)
	}
	tapNavSlot(t, ctx, drawer(), Button3)
	c, ok := pumpUntil(frame, "ENGRAVE KIT", 32)
	if !ok {
		t.Fatalf("no kit menu after LOAD AGAIN; got %q", c)
	}
	kitSentences(t, c, "Engrave kit now (1 card, 1 plate)?")
}

func TestKitRefusedOnceThenMenuAfterLoadAgain(t *testing.T) {
	skipUnderRefugium(t, refugiumSkipPlaintextSecret)
	ctx := NewContext(kitPlatform(plaintextRegion(t, kitWithMs1(t)...)))
	ctx.sysw = sessionWith(kitWithMs1(t)...)
	frame, drawer, quit := runUITouch(ctx, func() { syswPayloadMenu(ctx, &descriptorTheme) })
	defer quit()
	if c, ok := pumpUntil(frame, "Kit Refused", 32); !ok {
		t.Fatalf("no Kit Refused on opening; got %q", c)
	}
	click(&ctx.Router, Button3)
	if c, ok := pumpUntil(frame, "It holds", 32); !ok {
		t.Fatalf("no menu; got %q", c)
	}
	click(&ctx.Router, Button3) // LOAD AGAIN is first: no transaction in this payload
	if c, ok := pumpUntil(frame, "Compare this against", 32); !ok {
		t.Fatalf("LOAD AGAIN: no digest screen; got %q", c)
	}
	tapNavSlot(t, ctx, drawer(), Button3)
	if c, ok := pumpUntil(frame, "Payload Warnings", 32); !ok {
		t.Fatalf("no Payload Warnings; got %q", c)
	}
	tapNavSlot(t, ctx, drawer(), Button3)
	if c, ok := pumpUntil(frame, "Keep this payload loaded?", 32); !ok {
		t.Fatalf("no KEEP; got %q", c)
	}
	click(&ctx.Router, Button3)
	if c, ok := pumpUntil(frame, "Kit Refused", 32); !ok {
		t.Fatalf("no Kit Refused after the reload; got %q", c)
	}
	click(&ctx.Router, Button3)
	for i := 0; i < 24; i++ {
		c, more := frame()
		if !more {
			t.Fatal("flow ended without a menu after the reload")
		}
		if uiContains(c, "Kit Refused") {
			t.Fatalf("Kit Refused shown twice for one load: %q", c)
		}
		if uiContains(c, "It holds") {
			return
		}
	}
	t.Error("no menu after Kit Refused on LOAD AGAIN")
}
