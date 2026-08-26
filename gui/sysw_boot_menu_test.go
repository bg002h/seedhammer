package gui

import (
	"testing"
)

// THE SECOND HALF OF G-P3.15, and the half a gate can pass without.
//
// SPEC §3.3 ruled the payload menu "appears immediately after a successful
// load". The menu itself existing satisfies a gate reading "the payload menu
// exists and lists what the payload holds" -- while the ruled behaviour stays
// untrue, because the boot path called syswLoadFlow directly and returned to
// the carousel. So this test drives the REAL uiFlow, from power-on, over a real
// region, and asserts the menu is what the operator sees.
func TestTheBootLoadEndsAtThePayloadMenu(t *testing.T) {
	p := newPlatform()
	p.display = sh2DisplaySize
	p.sysw = syswRegionFor(t, "S-A")
	ctx := NewContext(p)

	frame, drawer, quit := runUITouch(ctx, func() { uiFlow(ctx, "test") })
	defer quit()

	content, ok := pumpUntil(frame, "Load it?", 64)
	if !ok {
		t.Fatalf("no boot offer; got %q", content)
	}
	tapNavSlot(t, ctx, drawer(), Button3) // LOAD

	content, ok = pumpUntil(frame, "Payload Digest", 64)
	if !ok {
		t.Fatalf("no digest screen after LOAD; got %q", content)
	}
	tapNavSlot(t, ctx, drawer(), Button3) // the digest matches

	// PAST THE WARNINGS, if this vector raises any: they are dismissible and
	// are not what this test is about. The menu is behind them.
	for i := 0; i < 6; i++ {
		content, ok = frame()
		if !ok {
			t.Fatal("the flow ended before the payload menu")
		}
		if uiContains(content, "It holds") {
			return
		}
		tapNavSlot(t, ctx, drawer(), Button3)
	}
	t.Errorf("the boot load returned to the carousel without showing the payload "+
		"menu; last frame was %q", content)
}
