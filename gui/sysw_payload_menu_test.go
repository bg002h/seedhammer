package gui

import (
	"strings"
	"testing"

	"seedhammer.com/sysw"
)

// ─── G-P3.15 — the payload menu, and the moment it appears ──────────────────
//
// SPEC §3.3 ruled: "it appears immediately after a successful load."
//
//	boot -> "payload present, load it?" -> LOAD -> compare digest
//	     -> THE PAYLOAD MENU ("this payload holds: 1 transaction, 2 seeds")
//	     -> BACK exits to the carousel
//
// The boot path called syswLoadFlow DIRECTLY and returned to the carousel, so
// syswPayloadMenu -- documented in its own file as "the Load Payload carousel
// entry" -- was reachable only by navigating there afterwards. Note the shape:
// a gate reading "the payload menu exists and lists what the payload holds" is
// satisfiable by the menu alone while the ruled behaviour stays untrue.

// THE MENU LISTS WHAT THE PAYLOAD HOLDS. Without it the operator cannot tell a
// payload with the WRONG contents from the right one, and those have different
// fixes -- re-pack, or carry on.
func TestThePayloadMenuNamesWhatThePayloadHolds(t *testing.T) {
	ctx := NewContext(newPlatform())
	ctx.sysw = sessionWith(append(append([]string{}, txEven...), "text:6869")...)
	frame, quit := runUI(ctx, func() { syswPayloadMenu(ctx, &descriptorTheme) })
	defer quit()
	got, ok := pumpUntil(frame, "It holds", 32)
	if !ok {
		t.Fatalf("the menu never lists the contents; last frame was %q", got)
	}
	if !uiContains(got, "6 mt1 chunk") {
		t.Errorf("the inventory must count the transaction strings: %q", got)
	}
	if !uiContains(got, "free text") {
		t.Errorf("the inventory must name every class present: %q", got)
	}
}

// THE CONTENT-DERIVED ENTRY. It is offered only when the payload holds
// something the transaction program can consume, so it can never be a button
// that leads to "this payload holds no transaction".
func TestTheMenuOffersTheTransactionProgramOnlyWhenThereIsOne(t *testing.T) {
	with := NewContext(newPlatform())
	with.sysw = sessionWith(txEven...)
	if !syswPayloadHasTransaction(with.sysw) {
		t.Error("a payload of mt1 chunks holds a transaction")
	}
	without := NewContext(newPlatform())
	without.sysw = sessionWith("text:6869")
	if syswPayloadHasTransaction(without.sysw) {
		t.Error("a payload of free text does not")
	}
	// A tx: record counts too -- it is the other admitted class.
	rec := NewContext(newPlatform())
	rec.sysw = sessionWith("tx:" + rawHexOf(t, evenTx(t)))
	if !syswPayloadHasTransaction(rec.sysw) {
		t.Error("a tx: record holds a transaction")
	}
	// An UNCOMPARED session holds nothing that may be taken.
	unc := &syswSession{}
	unc.load(&sysw.Payload{Public: txEven}, [32]byte{1}, false, false, false, true)
	if syswPayloadHasTransaction(unc) {
		t.Error("nothing may be taken from a payload nobody authenticated")
	}

	// ...and the entry really appears on the screen.
	frame, quit := runUI(with, func() { syswPayloadMenu(with, &descriptorTheme) })
	defer quit()
	got, _ := pumpUntil(frame, "ENGRAVE TRANSACTION", 32)
	if !uiContains(got, "ENGRAVE TRANSACTION") {
		t.Errorf("no content-derived entry on a payload holding a transaction: %q", got)
	}

	frame2, quit2 := runUI(without, func() { syswPayloadMenu(without, &descriptorTheme) })
	defer quit2()
	got2, _ := frame2()
	if uiContains(got2, "ENGRAVE TRANSACTION") {
		t.Errorf("a free-text payload was offered the transaction program: %q", got2)
	}
	if !uiContains(got2, "UNLOAD") {
		t.Errorf("the ordinary entries must survive: %q", got2)
	}
}

// BACK IS THE EXIT AND MUST BE (§3.3). The menu appears unbidden at boot now,
// so a screen that costs anything to leave is a screen that interrupts every
// power-on.
func TestTheMenuLeavesOnBack(t *testing.T) {
	ctx := NewContext(newPlatform())
	ctx.sysw = sessionWith(txEven...)
	done := make(chan struct{})
	frame, quit := runUI(ctx, func() { syswPayloadMenu(ctx, &descriptorTheme); close(done) })
	defer quit()
	if _, ok := frame(); !ok {
		t.Fatal("no frame")
	}
	click(&ctx.Router, Button1)
	for i := 0; i < 8; i++ {
		select {
		case <-done:
			return
		default:
		}
		if _, ok := frame(); !ok {
			break
		}
	}
	select {
	case <-done:
	default:
		t.Error("Back did not leave the payload menu")
	}
}

// A menu on a payload holding NOTHING loaded is the load flow, unchanged. The
// entry keeps its one meaning on a machine that has not loaded anything, which
// is every machine at boot.
func TestTheMenuWithNothingLoadedIsStillTheLoadFlow(t *testing.T) {
	ctx := NewContext(newPlatform())
	frame, quit := runUI(ctx, func() { syswPayloadMenu(ctx, &descriptorTheme) })
	defer quit()
	got, ok := pumpUntil(frame, "No payload found", 32)
	if !ok {
		t.Fatalf("with nothing loaded the menu must BE the load flow; got %q", got)
	}
	if strings.Contains(got, "It holds") {
		t.Errorf("it must not claim contents for a payload that is not there: %q", got)
	}
}

// ─── G-P3.16 — the compare screen names the READ path ───────────────────────

// SPEC §3.2. It said "Compare this against what `me sysw pack` printed" -- the
// WRITE path. Re-running pack means re-supplying every record and re-running
// the ceremony, and on the sealed path it mints a FRESH PASSPHRASE. The
// operator standing at the machine has the file, not the records.
//
// `me sysw show <file>` reads what they have, and `me sysw pack` now prints the
// same pointer, so the two sides of the air gap name one command.
func TestTheCompareScreenNamesTheReadPath(t *testing.T) {
	p := newPlatform()
	p.display = sh2DisplaySize
	p.sysw = syswRegionFor(t, "S-A")
	ctx := NewContext(p)

	frame, drawer, quit := runUITouch(ctx, func() {
		syswLoadFlow(ctx, &descriptorTheme, ctx.Platform.SyswReader(), true)
	})
	defer quit()
	content, ok := pumpUntil(frame, "Load it?", 64)
	if !ok {
		t.Fatalf("no boot offer; got %q", content)
	}
	tapNavSlot(t, ctx, drawer(), Button3)
	content, ok = pumpUntil(frame, "Payload Digest", 64)
	if !ok {
		t.Fatalf("no digest screen; got %q", content)
	}
	if !uiContains(content, "me sysw show") {
		t.Errorf("the compare screen must name the READ path: %q", content)
	}
	if uiContains(content, "me sysw pack") {
		t.Errorf("it must no longer send the operator to the WRITE path: %q", content)
	}
	if !uiContains(content, "<file>") {
		t.Errorf("it must show the command takes the file they hold: %q", content)
	}
}
