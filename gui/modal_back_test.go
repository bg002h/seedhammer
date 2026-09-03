package gui

import (
	"testing"
)

// ═══ F-440: a dismiss-only modal that ignores BACK ═══════════════════════════
//
// FROM THE BENCH, 2026-08-29. The operator, holding the pathological vault's
// payload, walked Wallet Policy to the plate screen, backed out, and reported:
// "I am hung on bundle incomplete screen." The device was alive and redrawing.
// The button they were pressing was wired to nothing.
//
// ErrorScreen.Layout bound `s.ok.Button = Button3` and NOTHING else, and
// showModal loops on it until that one button fires. Measured in the headless
// sim before the fix: THIRTY Button1 presses left the screen unchanged, and one
// Button3 press dismissed it and returned control. EventRouter.Reset discards a
// head event no filter matched, so the presses were not even queued -- they were
// dropped, frame after frame, in silence.
//
// The class is every dismiss-only modal in the firmware: 143 showError /
// showNotice call sites at the time of the fix, all reached through this one
// Layout.
//
// WHY BACK-DISMISS IS SAFE AT ALL OF THEM, checked before it was written:
// ErrorScreen.Layout returns (op, dismissed bool) -- ONE boolean, carrying no
// "which button", and every one of its five caller loops is `if dismissed {
// return/break }`. There is exactly one exit and it leads to exactly one place,
// so BACK cannot skip anything: there is nothing to skip. Every screen in the
// firmware that must force an acknowledgment is a ConfirmWarningScreen instead
// -- a different type, with a cancel/confirm pair and a hold-to-confirm delay --
// and this change does not touch it.

// The generic contract: BACK and OK both dismiss; nothing else does.
func TestErrorScreenDismissesOnBackAndOnOK(t *testing.T) {
	for _, tc := range []struct {
		name string
		btn  Button
		want bool
	}{
		{"Button3 (the checkmark) dismisses", Button3, true},
		{"Button1 (back) dismisses", Button1, true},
		{"Button2 does not dismiss", Button2, false},
		{"Center does not dismiss", Center, false},
		{"Up does not dismiss", Up, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := NewContext(newPlatform())
			s := &ErrorScreen{Title: "Bundle Incomplete", Body: "Stopped at card 1 of 12."}
			dims := ctx.Platform.DisplaySize()
			// One frame first, so the screen has registered its filters and the
			// Clickables have been polled once -- a click is a RELEASE seen by a
			// Clickable that saw the press, so a modal cannot be dismissed by
			// the release of the press that opened it.
			if _, dismissed := s.Layout(ctx, &descriptorTheme, dims); dismissed {
				t.Fatal("dismissed before any button was pressed")
			}
			ctx.Reset()

			click(&ctx.Router, tc.btn)
			_, dismissed := s.Layout(ctx, &descriptorTheme, dims)
			if dismissed != tc.want {
				t.Errorf("%v dismissed the modal = %v, want %v", tc.btn, dismissed, tc.want)
			}
		})
	}
}

// A modal must not be dismissed by a stray RELEASE it never saw the press for:
// that is how a click meant for the screen underneath skips the screen that
// replaced it.
func TestErrorScreenIgnoresAReleaseItNeverSawPressed(t *testing.T) {
	for _, b := range []Button{Button1, Button3} {
		ctx := NewContext(newPlatform())
		s := &ErrorScreen{Title: "Bundle Incomplete", Body: "body"}
		ctx.Router.Events(nil, ButtonEvent{Button: b, Pressed: false}.Event())
		if _, dismissed := s.Layout(ctx, &descriptorTheme, ctx.Platform.DisplaySize()); dismissed {
			t.Errorf("a bare %v release dismissed a modal that never saw the press", b)
		}
	}
}

// THE BENCH REPORT, as a walk. Wallet Policy from a payload, forward to the
// plate screen, back out to "Bundle Incomplete" -- then ONE press of BACK must
// dismiss it and return control, where before the fix thirty did nothing.
func TestF440BundleIncompleteModalDismissesOnBack(t *testing.T) {
	ctx := NewContext(f76Platform())
	ctx.sysw = f76Session(t, f76Md1CardPayload, f76Md1CardSHA256, wshSortedmultiChunks)

	frame, quit := runUI(ctx, func() { walletPolicyFlow(ctx, &descriptorTheme) })
	defer quit()

	// (0) THE COMPOSER'S DOOR, which is now the first screen in every
	// state (SPEC_wallet_policy_composer §7a). "Scan cards" is index 0, so
	// one Down selects "From payload", which is the route this walk takes.
	if _, ok := pumpUntil(frame, "Build a new policy", 16); !ok {
		t.Fatal("the composer door never drew")
	}
	click(&ctx.Router, Down)
	click(&ctx.Router, Button3)
	if got, ok := pumpUntil(frame, "Cards from where?", 24); !ok {
		t.Fatalf("the door never drew.\nLast frame: %q", got)
	}
	click(&ctx.Router, Button3) // FROM PAYLOAD
	if got, ok := pumpUntil(frame, "md1 descriptors: 1", 64); !ok {
		t.Fatalf("the card did not assemble.\nLast frame: %q", got)
	}
	// Done -> the consent surface (the wallet id, its shape, its addresses).
	click(&ctx.Router, Button3)
	if got, ok := pumpUntil(frame, "-ID:", 64); !ok {
		t.Fatalf("Done did not reach the consent screen.\nLast frame: %q", got)
	}
	// Consent -> bundle review -> engrave picker -> the plate screen.
	for _, want := range []string{"cards verified", "Choose engraving", "blank plate"} {
		click(&ctx.Router, Button3)
		if got, ok := pumpUntil(frame, want, 64); !ok {
			t.Fatalf("never reached %q.\nLast frame: %q", want, got)
		}
	}
	// Back out: the plate screen, then the picker, then the abort modal.
	click(&ctx.Router, Button1)
	if got, ok := pumpUntil(frame, "Choose engraving", 32); !ok {
		t.Fatalf("Back at the plate screen did not reach the picker.\nLast frame: %q", got)
	}
	click(&ctx.Router, Button1)
	got, ok := pumpUntil(frame, "Bundle Incomplete", 32)
	if !ok {
		t.Fatalf("Back at the picker did not reach the abort modal.\nLast frame: %q", got)
	}

	// ONE press of BACK. Before F-440 thirty of these changed nothing.
	click(&ctx.Router, Button1)
	for i := 0; i < 12; i++ {
		if _, more := frame(); !more {
			return // walletPolicyFlow returned: control is back with the operator.
		}
	}
	t.Errorf("BACK did not dismiss the Bundle Incomplete modal; the flow is still "+
		"drawing it after 12 frames. Last frame: %q", got)
}

// A dismissal consumes exactly ONE click, and the press queued behind it
// belongs to whoever comes next.
//
// The router is a single queue whose head is taken by the first matching
// filter, so a Layout that polled `ok` and then `back` unconditionally
// swallowed a Button1 press queued BEHIND the dismissing Button3 -- a press
// meant for the screen underneath. That is not hypothetical: it hung
// TestRecoverRejectsNonCodex32 forever, whose script is exactly
// `Button3, Button3, Button1` (OK the entry, dismiss the modal, Back out).
// This pins the property directly, so the next person who finds the
// short-circuit untidy learns why it is there from a failure rather than from
// a comment.
func TestErrorScreenDismissalLeavesTheNextClickAlone(t *testing.T) {
	ctx := NewContext(newPlatform())
	s := &ErrorScreen{Title: "Bundle Incomplete", Body: "body"}
	dims := ctx.Platform.DisplaySize()
	if _, dismissed := s.Layout(ctx, &descriptorTheme, dims); dismissed {
		t.Fatal("dismissed before any button was pressed")
	}
	ctx.Reset()

	// The dismissing click, and behind it a Back meant for the screen below.
	click(&ctx.Router, Button3, Button1)
	if _, dismissed := s.Layout(ctx, &descriptorTheme, dims); !dismissed {
		t.Fatal("Button3 did not dismiss")
	}
	// The Button1 must still be there. A fresh Clickable models the screen
	// underneath picking up where the modal left off.
	under := &Clickable{Button: Button1}
	if !under.Clicked(ctx) {
		t.Error("the modal swallowed the Back queued behind the dismissal; the " +
			"screen underneath never receives it and a flow waiting on it hangs")
	}
}
