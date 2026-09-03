package gui

import (
	"strings"
	"testing"
	"testing/synctest"
)

// TestComposerDigitPadTypesOnlyDigits is the widget's whole contract: the pad
// offers digits and a backspace, so an operand can never carry a stray rune
// the parser would then have to reject.
func TestComposerDigitPadTypesOnlyDigits(t *testing.T) {
	for _, r := range composerDigitKeys {
		if r == '\n' {
			continue
		}
		if r < '0' || r > '9' {
			t.Errorf("the digit pad's alphabet carries %q; §6b says the operator never "+
				"types a raw operand and never types a separator", r)
		}
	}
	if !strings.Contains(composerDigitKeys, "0") || !strings.Contains(composerDigitKeys, "9") {
		t.Error("the digit pad is missing a digit")
	}
}

// TestComposerDigitPadDrawsItsEchoAndGatesTheConfirm drives the real screen:
// the echo the caller returns is drawn, and the confirm icon appears only
// once the fragment is acceptable.
func TestComposerDigitPadDrawsItsEchoAndGatesTheConfirm(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p := newPlatform()
		p.display = sh2DisplaySize
		ctx := NewContext(p)
		frame, _, ink, quit := runUITouchRaster(ctx, func() {
			composerDigitEntry(ctx, &descriptorTheme, "Blocks", "How many blocks?", 5,
				func(frag string) (string, bool) {
					if frag == "" {
						return "type a number", false
					}
					return "echo for " + frag, true
				})
		})
		defer quit()
		content, ok := frame()
		if !ok {
			t.Fatal("the digit pad never drew")
		}
		assertFrameHasBody(t, ink(), "the composer digit pad")
		if !uiContains(content, "type a number") {
			t.Errorf("the pad does not draw the caller's feedback for an empty fragment.\nFrame: %q", content)
		}
		if !uiContains(content, "How many blocks?") {
			t.Errorf("the pad does not draw its lead.\nFrame: %q", content)
		}
	})
}

// TestComposerDigitPadBackLeavesWithNothing: Back is a decline everywhere on
// this device, and an entry screen that returned its partial fragment on Back
// would hand a half-typed operand to a lock.
func TestComposerDigitPadBackLeavesWithNothing(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p := newPlatform()
		p.display = sh2DisplaySize
		ctx := NewContext(p)
		var got string
		var ok bool
		frame, quit := runUI(ctx, func() {
			got, ok = composerDigitEntry(ctx, &descriptorTheme, "Blocks", "How many?", 5,
				func(frag string) (string, bool) { return "", true })
		})
		defer quit()
		if _, drew := frame(); !drew {
			t.Fatal("no frame")
		}
		click(&ctx.Router, Button1)
		for i := 0; i < 8; i++ {
			if _, more := frame(); !more {
				break
			}
		}
		if ok || got != "" {
			t.Errorf("Back returned (%q, %v), want (\"\", false)", got, ok)
		}
	})
}
