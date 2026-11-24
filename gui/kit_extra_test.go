package gui

import (
	"fmt"
	"strings"
	"testing"
	"testing/synctest"

	"seedhammer.com/gui/op"
)

// kitListAt opens the kit list of kit-2of3 through Engrave Bundle.
func kitListAt(t *testing.T) (ctx *Context, frame func() (string, bool), drawerOf func() *op.Drawer, e *testEngraver, quit func()) {
	t.Helper()
	p, eng := kitRunPlatform()
	ctx = NewContext(p)
	ctx.sysw = sessionWith(kitVectorRecords(t, "kit-2of3")...)
	f, d, q := runUITouch(ctx, func() { bundleFlow(ctx, &descriptorTheme) })
	if c, ok := pumpUntil(f, "Start at:", 32); !ok {
		t.Fatalf("no list; got %q", c)
	}
	return ctx, f, d, eng, q
}

// The first card of a kit is an mr1 card, and its plate is marked like any
// other: CARD 1/6 over PLATE 1/1, TEXT ONLY alone. Cutting it marks " (cut)"
// on its list entry, and on no other.
func TestKitMr1CardPlateMarkedAndListShowsCut(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		calls := kitSeams(t, false, false)
		ctx, frame, _, e, quit := kitListAt(t)
		defer quit()
		click(&ctx.Router, Button3) // OK on card 1
		c, ok := pumpUntil(frame, "Card 1 of 6 | Plate 1 of 1", 64)
		if !ok {
			t.Fatalf("no choice screen for card 1; got %q", c)
		}
		kitSentences(t, c, "Choose engraving", "TEXT ONLY")
		if uiContains(c, "QR") {
			t.Errorf("the mr1 plate offers a QR choice: %q", c)
		}
		if len(*calls) == 0 || (*calls)[0].title != "CARD 1/6" || (*calls)[0].footer != "PLATE 1/1" {
			t.Errorf("card 1 plate marked %+v, want CARD 1/6 / PLATE 1/1", *calls)
		}
		kitCut(t, ctx, frame, e)
		if c, ok = pumpUntil(frame, "Card 2 of 6 | Plate 1 of 1", 64); !ok {
			t.Fatalf("no card 2; got %q", c)
		}
		click(&ctx.Router, Button1) // Back at plate 1: the list again
		c, ok = pumpUntil(frame, "Start at:", 32)
		if !ok {
			t.Fatalf("no list; got %q", c)
		}
		kitSentences(t, c, "Card 1 of 6: mr1, begins mr1lg2c0, ends 06uf (cut)")
		if uiContains(c, "ends 0uvz (cut)") {
			t.Errorf("card 2 marked cut though only card 1 was cut: %q", c)
		}
	})
}

// A run that reaches its last card returns to a list whose highlight is on card
// 1; starting at the last card says "Card 6 of this kit is cut."
func TestKitRunFromTheLastCardAndHighlightAfterTheEnd(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		kitSeams(t, false, false)
		ctx, frame, drawer, e, quit := kitListAt(t)
		defer quit()
		for i := 0; i < 6; i++ {
			if c, _ := frame(); uiContains(c, "Card 6 of 6:") {
				break
			}
			click(&ctx.Router, Button2)
			frame()
		}
		pts := plateHitPoints(ctx, drawer())
		tap(&ctx.Router, drawer(), pts[len(pts)-1])
		frame()
		click(&ctx.Router, Button3)
		if c, ok := pumpUntil(frame, "Card 6 of 6 | Plate 1 of 1", 64); !ok {
			t.Fatalf("not at card 6; got %q", c)
		}
		kitCut(t, ctx, frame, e)
		c, ok := pumpUntil(frame, "of this kit", 64)
		if !ok || !uiContains(c, "Card 6 of this kit is cut.") {
			t.Fatalf("end screen for the last card: %q", c)
		}
		click(&ctx.Router, Button3)
		if c, ok = pumpUntil(frame, "Start at:", 32); !ok {
			t.Fatalf("no list; got %q", c)
		}
		click(&ctx.Router, Button3) // OK: the highlight is on card 1 after a run's end
		if c, ok = pumpUntil(frame, "Card 1 of 6 | Plate 1 of 1", 64); !ok {
			t.Fatalf("the highlight is not on card 1 after the run ended; got %q", c)
		}
	})
}

// With the one-string-per-plate seam the lead and the list header count PLATES,
// not cards: kit-2of3 has 6 cards and 8 plates under that seam.
func TestKitLeadAndHeaderCountPlatesNotCards(t *testing.T) {
	kitSeams(t, true, false)
	ctx := NewContext(kitPlatform(nil))
	ctx.sysw = sessionWith(kitVectorRecords(t, "kit-2of3")...)
	v := kitCards(ctx.sysw)
	plan := kitPlatePlan(ctx.Platform.EngraverParams(), v.cards)
	if len(plan) == len(v.cards) {
		t.Fatalf("the seam did not split any card: %d plates for %d cards", len(plan), len(v.cards))
	}
	frame, _, quit := runUITouch(ctx, func() { syswPayloadMenu(ctx, &descriptorTheme) })
	defer quit()
	want := fmt.Sprintf("Engrave kit now (6 cards, %d plates)?", len(plan))
	if c, ok := pumpUntil(frame, "Engrave kit now", 32); !ok || !uiContains(c, want) {
		t.Fatalf("lead: want %q; got %q", want, c)
	}
	click(&ctx.Router, Button3)
	want = fmt.Sprintf("6 cards, %d plates. Start at:", len(plan))
	if c, ok := pumpUntil(frame, "Start at:", 32); !ok || !uiContains(c, want) {
		t.Fatalf("header: want %q; got %q", want, c)
	}
}

// An engrave that does not complete (Back on the engrave screen) shows the same
// choice screen again, and the plate is not marked cut.
func TestKitEngraveBackShowsTheChoiceAgain(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		kitSeams(t, false, false)
		ctx, frame, _, _, quit := kitListAt(t)
		defer quit()
		click(&ctx.Router, Button3)
		if c, ok := pumpUntil(frame, "Card 1 of 6 | Plate 1 of 1", 64); !ok {
			t.Fatalf("no choice screen; got %q", c)
		}
		click(&ctx.Router, Button3) // TEXT ONLY: opens the engrave screen
		frame()
		click(&ctx.Router, Button1) // Back out of the engrave screen
		c, ok := pumpUntil(frame, "Card 1 of 6 | Plate 1 of 1", 64)
		if !ok || !uiContains(c, "TEXT ONLY") {
			t.Fatalf("the choice screen did not come back; got %q", c)
		}
		if len(ctx.sysw.kitCut) != 0 {
			t.Errorf("an engrave that did not complete was recorded as cut: %v", ctx.sysw.kitCut)
		}
	})
}

// Kit Refused shows once at boot and once after LOAD AGAIN from the menu, and
// the menu follows each (Brian 2026-11-22, s1 I-2 option (b): adapted from the
// reviewer's row, which pinned "a load no menu follows").
func TestKitRefusedAfterLoadAgainAndOnceAtBoot(t *testing.T) {
	skipUnderRefugium(t, refugiumSkipPlaintextSecret)
	count := func(frames []string) int {
		return strings.Count(strings.Join(frames, "\n"), "Nothing from this payload")
	}
	_ = count
	// boot: syswLoadFlow(atBoot) then the menu; Kit Refused exactly once.
	p := kitPlatform(plaintextRegion(t, kitWithMs1(t)...))
	ctx := NewContext(p)
	frame, drawer, quit := runUITouch(ctx, func() {
		if syswLoadFlow(ctx, &descriptorTheme, ctx.Platform.SyswReader(), true) {
			syswPayloadMenu(ctx, &descriptorTheme)
		}
	})
	defer quit()
	pumpUntil(frame, "Load it?", 32)
	click(&ctx.Router, Button3)
	pumpUntil(frame, "Compare this against", 32)
	tapNavSlot(t, ctx, drawer(), Button3)
	pumpUntil(frame, "Payload Warnings", 32)
	tapNavSlot(t, ctx, drawer(), Button3)
	pumpUntil(frame, "Keep this payload loaded?", 32)
	click(&ctx.Router, Button3) // KEEP
	shown := 0
	for i := 0; i < 40; i++ {
		c, _ := frame()
		if uiContains(c, "Kit Refused") {
			shown++
			click(&ctx.Router, Button3)
		}
		if uiContains(c, "It holds") {
			break
		}
	}
	if shown != 1 {
		t.Errorf("Kit Refused shown %d times at boot, want 1", shown)
	}
	// LOAD AGAIN from the menu: Load it? -> compare -> warnings -> KEEP -> Kit Refused.
	click(&ctx.Router, Button3)
	if c, ok := pumpUntil(frame, "Compare this against", 32); !ok {
		t.Fatalf("LOAD AGAIN: no digest; got %q", c)
	}
	tapNavSlot(t, ctx, drawer(), Button3)
	pumpUntil(frame, "Payload Warnings", 32)
	tapNavSlot(t, ctx, drawer(), Button3)
	pumpUntil(frame, "Keep this payload loaded?", 32)
	click(&ctx.Router, Button3)
	if c, ok := pumpUntil(frame, "Kit Refused", 32); !ok {
		t.Fatalf("LOAD AGAIN of a refused kit did not show Kit Refused; got %q", c)
	}
	click(&ctx.Router, Button3)
	again := 0
	menu := false
	for i := 0; i < 24; i++ {
		c, more := frame()
		if !more {
			break
		}
		if uiContains(c, "Kit Refused") {
			again++
		}
		if uiContains(c, "It holds") {
			menu = true
			break
		}
	}
	if again != 0 || !menu {
		t.Errorf("after LOAD AGAIN: Kit Refused repeated %d times, menu followed %v; want 0 and true", again, menu)
	}
}

// kitTextOnlyLayout offers TEXT ONLY alone for a one-string plate, where the
// ordinary layout of the same string offers the QR variants too (the guard: the
// plain layout must offer more than one, or this proves nothing).
func TestKitTextOnlyLayoutDropsQR(t *testing.T) {
	p := newPlatform()
	p.display = sh2DisplaySize
	rec := kitVectorRecords(t, "kit-single")[0]
	plain, _, err := validateMdmkStrings(p, []string{rec}, "CARD 1/1", "PLATE 1/1")
	if err != nil || len(plain) < 2 {
		t.Fatalf("guard: the ordinary layout of one string offers %v (err %v), want TEXT + QR and more", plain, err)
	}
	labels, plates, err := kitTextOnlyLayout(p, []string{rec}, "CARD 1/1", "PLATE 1/1")
	if err != nil || len(labels) != 1 || labels[0] != "TEXT ONLY" || len(plates) != 1 {
		t.Errorf("kit layout offers %v (err %v), want TEXT ONLY alone", labels, err)
	}
}

// Back at plate 2 of a two-plate card leaves it NOT cut, though plate 1 was.
func TestKitCardIsCutOnlyWhenAllItsPlatesAre(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		kitSeams(t, true, false)
		ctx, frame, drawer, e, quit := kitListAt(t)
		defer quit()
		kitSelectCard(t, ctx, frame, drawer, 2)
		click(&ctx.Router, Button3)
		if c, ok := pumpUntil(frame, "Card 2 of 6 | Plate 1 of 2", 64); !ok {
			t.Fatalf("no plate 1; got %q", c)
		}
		kitCut(t, ctx, frame, e)
		if c, ok := pumpUntil(frame, "Card 2 of 6 | Plate 2 of 2", 64); !ok {
			t.Fatalf("no plate 2; got %q", c)
		}
		click(&ctx.Router, Button1) // stop before plate 2
		pumpUntil(frame, "Stopped before", 32)
		click(&ctx.Router, Button3)
		c, ok := pumpUntil(frame, "Start at:", 32)
		if !ok {
			t.Fatalf("no list; got %q", c)
		}
		if uiContains(c, "(cut)") {
			t.Errorf("a card with one of two plates cut is marked cut: %q", c)
		}
	})
}
