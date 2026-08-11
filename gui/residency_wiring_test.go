package gui

import (
	"testing"
	"testing/synctest"
	"time"

	"seedhammer.com/bezier"
	"seedhammer.com/bspline"
)

// These two tests exist because the whole-diff review found that F-107's and
// F-108's CALL SITES were pinned by nothing. The functions were mutation-tested;
// the wiring was not, and in both cases removing the wiring is a ONE-LINE revert
// to the immediately preceding commit's text with the whole suite still green.
//
// That is the false-PASS shape this project keeps rediscovering: a test that
// exercises a helper directly proves the helper works and says nothing about
// whether anything calls it.

// TestPassphraseFlowScrubsOnTheGiveUpRoute pins ctx.B.Scrub() in
// unlockPassphraseFlow's OWN defer -- not unlockSecretSession's.
//
// §8's twelve-word passphrase is rendered by this flow, OUTSIDE the secret
// session's bracket, and it is what opens the sealed payload. op.Glyph encodes
// every drawn rune into ctx.B.args, and Buffer.Reset only truncates, so without
// the scrub the words come back from the backing array on the ordinary give-up
// route: type a word, press Back. Measured by the reviewer at 906 non-zero args
// with the scrub deleted.
//
// Mutation this pins: revert unlockPassphraseFlow's defer to
// `defer func() { ctx.wipe = prev }()`.
func TestPassphraseFlowScrubsOnTheGiveUpRoute(t *testing.T) {
	v := sealVector(t, "D")
	h := newUnlockHarness(t, payloadReaderFrom(t, v.blob(t)))
	h.toPassphrase(true)
	// A part-typed passphrase then Back is the ORDINARY shape of "the operator
	// left", not an error case (gui/gui.go:704).
	h.typeWord("beef")
	h.tapNav(Button1)
	for i := 0; i < 64 && !*h.done; i++ {
		if _, ok := h.frame(); !ok {
			break
		}
	}
	if !*h.done {
		t.Fatal("INCONCLUSIVE: backing out of word entry did not return, so no give-up exit happened")
	}

	args, refs := h.ctx.B.Residue()
	if args != 0 || refs != 0 {
		t.Errorf("after the passphrase give-up route the frame buffer still holds %d non-zero "+
			"args and %d non-nil refs -- the typed passphrase derives the key that opens "+
			"everything, and unlockPassphraseFlow's own bracket is the only thing that scrubs it",
			args, refs)
	}
}

// TestEngraveScreenReleasesResumeStateOnReturn pins
// s.job.releaseResumeState() in EngraveScreen.Engrave's defer.
//
// TestReleaseResumeStateOnlyClearsAnAbandonedJob builds an engraveJob directly
// and calls the method, so it pins the LOGIC -- which states clear and which do
// not -- and never that Engrave calls it at all.
//
// Mutation this pins: revert Engrave's defer to `defer s.job.Stop()`.
func TestEngraveScreenReleasesResumeStateOnReturn(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		e := newEngraver()
		p := newPlatform()
		p.engraver = e
		ctx := NewContext(p)

		// A cut that COMPLETES. The double-Back abort returns in
		// engraveStopping, which is deliberately non-terminal (the goroutine is
		// still winding down), so it is not the scenario this pins -- see
		// F-110. "The plate finished and the operator walked away" is.
		var scr *EngraveScreen
		frame, quit := runUI(ctx, func() {
			scr = NewEngraveScreen(ctx, Plate{
				Spline: func(yield func(bspline.Knot) bool) {
					for i := 0; i < 8; i++ {
						if !yield(bspline.Knot{Ctrl: bezier.Pt(1234, 5678), T: 5}) {
							return
						}
					}
				},
				// Omitting Conf leaves the resume path with Jerk=0, which
				// divides by zero inside SafePointer.Resume.
				Conf: p.EngraverParams().StepperConfig,
			})
			scr.Engrave(ctx, &engraveTheme)
		})
		defer quit()

		// Start: click to focus, then HOLD to confirm.
		click(&ctx.Router, Button3, Button3, Button3)
		press(&ctx.Router, Button3)
		if _, ok := frame(); !ok {
			t.Fatal("INCONCLUSIVE: the engrave screen exited before the job started")
		}
		time.Sleep(confirmDelay)
		for i := 0; i < 32; i++ {
			if _, ok := frame(); !ok {
				break
			}
			if scr.job.Status().State == engraveDone {
				break
			}
		}
		if scr.job.Status().State != engraveDone {
			t.Fatalf("INCONCLUSIVE: the job never completed (state %v), so Engrave cannot "+
				"return in a terminal state", scr.job.Status().State)
		}

		// The goroutine has sent on e.errs, so nothing else writes safePoint now.
		if scr.job.safePoint.HistoryLen() == 0 {
			for i := 0; i < 8; i++ {
				scr.job.safePoint.Knot(bspline.Knot{Ctrl: bezier.Pt(1234, 5678), T: 5})
			}
		}
		if scr.job.safePoint.HistoryLen() == 0 {
			t.Fatal("INCONCLUSIVE: no resume state to clear")
		}

		// Back on a finished plate: state is not engraveRunning, so Engrave
		// returns. No restart is reachable once it does.
		click(&ctx.Router, Button1, Button1, Button1)
		if _, ok := frame(); ok {
			t.Fatal("INCONCLUSIVE: the engrave screen did not exit on Back")
		}

		if n := scr.job.safePoint.HistoryLen(); n != 0 {
			t.Errorf("%d resume knots survive after EngraveScreen.Engrave returned in a "+
				"terminal state -- the job is abandoned and the knots are the seed rendered "+
				"as geometry; releaseResumeState is not wired into the defer", n)
		}
	})
}
