package gui

import (
	"strings"
	"testing"
	"testing/synctest"
	"time"
)

// ─── S6b P9: three Important findings off the failure-states review ─────────
//
// design/agent-reports/s6b-failure-states-review.md (2026-08-18). F1/F2/F3 are
// this file's three subjects; the fold's own report
// (design/agent-reports/s6b-p9-failure-states-fold.md) records how each was
// reproduced, what the fix is, and what the operator sees on both sides.

// ─── F1: an aborted or rejected passphrase-plate engrave leaves steel that
// nothing warns to destroy ─────────────────────────────────────────────────

// engraveOnePlateThenReject drives a plate engrave to COMPLETION -- the same
// hold-to-confirm sequence engraveOnePlate uses -- and then REJECTS it by
// pressing Back at the accept screen instead of the checkmark. This is F1's
// sequence (b): a COMPLETE plate exists and EngraveScreen.Engrave still
// returns false (gui/gui.go:3111-3141, "Not when the job reaches
// engraveDone: the operator still has to confirm, and can leave with Back
// instead").
func engraveOnePlateThenReject(t *testing.T, ctx *Context, frame func() (string, bool), e *testEngraver) {
	t.Helper()
	click(&ctx.Router, Button3, Button3, Button3)
	press(&ctx.Router, Button3)
	frame()
	time.Sleep(confirmDelay)
	for i := 0; i < 4096; i++ {
		select {
		case <-e.closes:
			click(&ctx.Router, Button1) // Back at the accept screen: REJECT a COMPLETE plate.
			frame()
			return
		default:
		}
		if _, ok := frame(); !ok {
			return
		}
	}
	t.Fatal("the engrave never closed the engraver, so no plate was cut")
}

// TestPassphrasePlateAbortAfterEngraveAttemptWarnsToDestroy is F1's
// reproduction and fix proof. It drives the REAL engravePassphraseFlowPreloaded
// through a COMPLETE cut that is then REJECTED at the accept screen (sequence
// (b) from the review -- (a), an abort mid-cut, latches the identical
// `attempted` bool at the identical call site and is not separately walked),
// then backs all the way out through Confirm -> QR -> Entry -> out of the
// program, and asserts a DESTROY-shaped warning is shown before the function
// returns passphrasePlateNotCut. Pre-fix this modal does not exist anywhere on
// this path -- the operator's abort dead-ends silently and the restore
// document (proven by the pre-existing GATE 6a tests) then says nothing this
// device engraves carries a passphrase.
func TestPassphrasePlateAbortAfterEngraveAttemptWarnsToDestroy(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		e := newEngraver()
		p := newPlatform()
		p.display = sh2DisplaySize
		p.engraver = e
		ctx := NewContext(p)
		var got passphrasePlateResult
		var done bool
		frame, _, _, quit := runUITouchRaster(ctx, func() {
			got = engravePassphraseFlowPreloaded(ctx, &descriptorTheme, []byte("hunter2"),
				"A1B2C3D4", "5E6F7A8B", "1A2B3C4D")
			done = true
		})
		defer quit()

		if c, ok := pumpUntil(frame, "Source:", 32); !ok {
			t.Fatalf("never reached the acceptance screen; got %q", c)
		}
		click(&ctx.Router, Button3) // accept the source
		if c, ok := pumpUntil(frame, "/100", 32); !ok {
			t.Fatalf("never reached the preloaded passphrase-entry screen; got %q", c)
		}
		click(&ctx.Router, Button3) // accept the preloaded passphrase as-is
		if c, ok := pumpUntil(frame, "QR Code", 32); !ok {
			t.Fatalf("did not reach the QR choice; got %q", c)
		}
		click(&ctx.Router, Button3) // accept the QR default (off)
		if c, ok := pumpUntil(frame, "Confirm", 32); !ok {
			t.Fatalf("did not reach the confirm screen; got %q", c)
		}
		click(&ctx.Router, Button3) // accept: advances to the engrave step
		if c, ok := pumpUntil(frame, "Engrave", 32); !ok {
			t.Fatalf("did not reach the passphrase-plate engrave screen; got %q", c)
		}

		// CUT THE PLATE IN FULL, THEN REJECT IT. `attempted` latches the moment
		// this call is made, regardless of what happens inside it.
		engraveOnePlateThenReject(t, ctx, frame, e)

		if c, ok := pumpUntil(frame, "Confirm", 32); !ok {
			t.Fatalf("rejecting the cut plate did not return to the confirm screen; got %q", c)
		}
		click(&ctx.Router, Button1) // Back: Confirm -> QR
		if c, ok := pumpUntil(frame, "QR Code", 32); !ok {
			t.Fatalf("did not return to the QR choice; got %q", c)
		}
		click(&ctx.Router, Button1) // Back: QR -> entry
		if c, ok := pumpUntil(frame, "/100", 32); !ok {
			t.Fatalf("did not return to the passphrase-entry screen; got %q", c)
		}

		// BACK OUT OF THE PROGRAM. This is the exact site F1's fix latches on:
		// `attempted` is true (the engrave was entered above), so a dismissible
		// warning must appear BEFORE the function returns passphrasePlateNotCut.
		click(&ctx.Router, Button1)

		warned, ok := pumpUntil(frame, "DESTROY", 64)
		if !ok {
			t.Fatalf("a COMPLETE passphrase plate was cut and then rejected, and backing "+
				"out of the program afterwards shows no destroy warning at all -- "+
				"F1: secret steel now exists that nothing accounts for and nothing tells "+
				"the operator to destroy. Last frame: %q", warned)
		}
		if !uiContains(warned, "passphrase") {
			t.Errorf("the warning does not say what it is warning about: %q", warned)
		}
		click(&ctx.Router, Button3) // dismiss
		for i := 0; i < 64 && !done; i++ {
			frame()
		}
		if !done {
			t.Fatal("engravePassphraseFlowPreloaded never returned after the warning was dismissed")
		}
		if got != passphrasePlateNotCut {
			t.Errorf("got %v, want passphrasePlateNotCut -- a rejected plate is still not a "+
				"cut one, GATE 6a is unchanged by this fix", got)
		}
	})
}

// TestPassphraseAbortWarningTextFits is the F-185-class fit check for F1's new
// modal body (see gui/modal_fits_test.go's own doc comment for why this
// matters on a device with no scroll buttons).
func TestPassphraseAbortWarningTextFits(t *testing.T) {
	assertModalBodyFits(t, "passphraseAbortWarningText", errorScreenBody, passphraseAbortWarningText)
}

// TestPassphraseAbortWarningTextIsHedged pins the property the fix's own
// comment argues for: the wording must be true whether nothing was cut yet, a
// partial cut was stopped, or a complete plate was cut and rejected --
// EngraveScreen.Engrave's bool return cannot tell those apart, so the text may
// not assert unconditionally that a plate WAS cut.
func TestPassphraseAbortWarningTextIsHedged(t *testing.T) {
	if strings.Contains(passphraseAbortWarningText, "This passphrase plate has been cut") ||
		strings.Contains(passphraseAbortWarningText, "The passphrase plate was cut") {
		t.Errorf("the warning asserts a cut unconditionally, which is false when the "+
			"operator backed out before the hold-to-confirm ever started: %q",
			passphraseAbortWarningText)
	}
	if !strings.Contains(passphraseAbortWarningText, "If any") {
		t.Errorf("the warning is not conditioned on whether anything was actually cut: %q",
			passphraseAbortWarningText)
	}
	if !strings.Contains(passphraseAbortWarningText, "DESTROY") {
		t.Errorf("the warning does not say DESTROY: %q", passphraseAbortWarningText)
	}
	if strings.ContainsAny(passphraseAbortWarningText, "—–·‘’“”…") {
		t.Errorf("the warning carries a glyph the body face lacks (F-78/F-151), so it does "+
			"not draw: %q", passphraseAbortWarningText)
	}
}
