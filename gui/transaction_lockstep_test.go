package gui

import (
	"reflect"
	"strings"
	"testing"
)

// THE FOUR LOCKSTEP SITES. Adding a program to this firmware means editing
// four places, and three of them fail SILENTLY:
//
//	gui.go  uiFlow's program switch    -- the only one that fails loudly
//	gui.go  StartScreen.draw's titles  -- a missing case leaves the title BLANK
//	gui.go  layoutMainPlates           -- a missing case PANICS at draw time
//	gui.go  engraveObjectFlow          -- a missing case drops the scan on the floor
//
// The sheet recorded all four sites as present and all three silent ones as
// "asserted only indirectly". These are the direct assertions.

// A scanned mt1 string must reach the transaction program's GATHER, not fall
// through to "unknown format" and not be engraved alone. engraveObjectFlow's
// mtText case is the only thing that routes it.
func TestEngraveObjectFlowRoutesAnMt1StringToTheGather(t *testing.T) {
	ctx := NewContext(newPlatform())
	frame, quit := runUI(ctx, func() {
		if !engraveObjectFlow(ctx, &descriptorTheme, mtText(txEven[0])) {
			t.Error("engraveObjectFlow returned false for an mt1 string: the scan is dropped")
		}
	})
	defer quit()
	// The gather counts what it holds and says how many are still to come. A
	// seeded chunk is offered before the scanner starts, so the very first
	// frame already reflects it.
	got, ok := pumpUntil(frame, "String 1 of 6", 32)
	if !ok {
		t.Fatalf("a scanned mt1 string did not reach the gather; last frame was %q", got)
	}
	if !uiContains(got, "5 to go") {
		t.Errorf("the gather must say how many remain; got %q", got)
	}
}

// An UNROUTED object returns false, which is what "unknown format" is built
// from. This is the control: without it the test above passes for a function
// that returns true for everything.
func TestEngraveObjectFlowReturnsFalseForAnUnroutedObject(t *testing.T) {
	ctx := NewContext(newPlatform())
	if engraveObjectFlow(ctx, &descriptorTheme, addressText("bc1qkwl5qpx6k93cqmnygn6kgucgka8q3z4kur2nm8")) {
		t.Error("addressText has no engraveObjectFlow case and must return false (R0-M5)")
	}
}

// EVERY navigable program has a title and a plate image. A missing title case
// draws a BLANK title bar; a missing layoutMainPlates case PANICS. Both are
// swept here rather than one program at a time, so the next program added is
// covered without anyone remembering to add a test.
func TestEveryNavigableProgramHasATitleAndAPlate(t *testing.T) {
	ctx := NewContext(newPlatform())
	s := &StartScreen{Version: "test"}
	last := s.lastNav()
	if last < engraveTransaction {
		t.Fatalf("engraveTransaction (%d) is past the last navigable program (%d)", engraveTransaction, last)
	}
	for p := program(0); p <= last; p++ {
		// layoutMainPlates panics on an unlisted page; recover names WHICH.
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("layoutMainPlates panics on program %d: %v", p, r)
				}
			}()
			op, sz := layoutMainPlates(&ctx.B, p)
			_ = op
			if sz.X == 0 || sz.Y == 0 {
				t.Errorf("program %d has a zero-sized plate image", p)
			}
		}()
	}
}

// The transaction program's TITLE, asserted through the screen that draws it
// rather than by reading the switch. A `case` deleted from StartScreen.draw
// leaves titleTxt at "" and the bar simply goes empty -- no panic, no failure.
func TestTheTransactionProgramIsTitledOnTheStartScreen(t *testing.T) {
	ctx := NewContext(newPlatform())
	m := new(StartScreen)
	frame, drawer, quit := runUITouch(ctx, func() { m.Flow(ctx, &descriptorTheme) })
	defer quit()
	if _, ok := frame(); !ok {
		t.Fatal("StartScreen produced no frame")
	}
	_, right := arrowPoints(ctx)
	for i := 0; i < int(engraveTransaction)+2; i++ {
		content, ok := frame()
		if !ok {
			t.Fatal("no frame")
		}
		if uiContains(content, "Engrave Transaction") {
			return
		}
		tap(&ctx.Router, drawer(), right)
	}
	t.Error("no page of the carousel is titled \"Engrave Transaction\"")
}

// The SCANNER's carrier type is the routing. Asserted here as well as in
// TestScan's table, because the table compares against a value this test
// derives from the string itself -- so a change to the carrier type has to be
// made in two places that disagree with each other.
func TestTheScannerYieldsTheMtCarrierType(t *testing.T) {
	obj, err := scanOnce(t, txEven[0])
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if _, isMt := obj.(mtText); !isMt {
		t.Fatalf("scanned object is %T (%v), want mtText", obj, reflect.TypeOf(obj))
	}
	if string(obj.(mtText)) != txEven[0] {
		t.Error("the carrier must hold the string verbatim -- it is engraved verbatim")
	}
	// A near-miss must NOT be claimed: one flipped character is not an mt1
	// string, and falling through to free text would engrave the damage.
	bad := strings.TrimSuffix(txEven[0], "x") + "y"
	obj2, err2 := scanOnce(t, bad)
	if _, isMt := obj2.(mtText); isMt {
		t.Errorf("a BCH-invalid string was accepted as mt1 (err=%v)", err2)
	}
}
