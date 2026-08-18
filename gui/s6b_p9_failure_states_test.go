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

// ─── F2: the single-sig verify tail dead-ends on both adverse arms ──────────

// TestSingleSigEngraveReOffersTheVerify is the mechanical wiring proof
// (mirrors TestBothEngraveFlowsReOfferTheVerify, gui/multisig_verify_report_
// test.go): engraveSingleSigFlow's source must dispatch through the
// singleSigVerifyFn seam, READ its retryable result as the loop condition,
// and draw multisigVerifyRetryLead. This is the class check that killed two
// independent mutations on the multisig side (a one-iteration loop, and
// swapped row labels); it does not re-prove the generic ChoiceScreen
// loop/label mechanics here, because that mechanism is byte-identical to the
// already mutation-tested multisig shape (same `for`, same two-choice
// re-offer, same shared constant) -- see this file's own header comment in
// the fold report for why a third full click-through walk was not added.
func TestSingleSigEngraveReOffersTheVerify(t *testing.T) {
	body := funcBody(t, "singlesig.go", "func engraveSingleSigFlow(")
	call := `singleSigVerifyFn(ctx, th, full, template, passphrase != "", &rec)`
	if !strings.Contains(body, call) {
		t.Fatalf("engraveSingleSigFlow does not hand the engrave-time passphrase fact to "+
			"the verify through the test seam; got body:\n%s", body)
	}
	if !strings.Contains(body, "if !"+call) {
		t.Errorf("the verify's retryable result is not read as the loop's own condition")
	}
	if !strings.Contains(body, "multisigVerifyRetryLead") {
		t.Errorf("engraveSingleSigFlow does not draw the retry offer")
	}
	if !strings.Contains(body, "for {") {
		t.Errorf("the verify offer is not wrapped in a loop -- a one-shot `if` dead-ends on " +
			"both adverse arms (S6b P9 F2, the class F-199 fixed on multisig in the same diff)")
	}
}

// TestSingleSigVerifyRetryProducesAnHonestStatusVerifiedOnRetryLine is F2's
// behavioural proof, and it is the trace the review's own brief demanded:
// "trace what buildVerifyStatusLine renders for a retried single-sig pass,
// and confirm it is true." It drives the REAL singleSigVerifyFlow TWICE
// against the SAME sticky verifyRecord -- first with a seed that is NOT what
// the plates were engraved with (guaranteed comparator FAIL on plates that
// are actually fine), then with the correct seed (a clean pass) -- exactly
// what engraveSingleSigFlow's new retry loop does when the operator presses
// "VERIFY AGAIN". No test seam is substituted for singleSigVerifyFlow itself:
// this is the real function, the real re-derivation, and the real NFC-gather
// simulation, off a bench built once by s6aSingleSigBundle (watch-only, so no
// ms1 hand-typing is needed, keeping this cheap -- no real engrave, no
// synctest raster harness).
func TestSingleSigVerifyRetryProducesAnHonestStatusVerifiedOnRetryLine(t *testing.T) {
	opts := s6aSingleSigOpts{watchOnly: true}
	b := s6aSingleSigBundle(t, opts)

	var rec verifyRecord
	var ret1, ret2, done bool
	p := newPlatform()
	p.display = sh2DisplaySize
	ctx := NewContext(p)
	ctx.syswBundleSeeds = append(append([]string(nil), b.MK1...), b.MD1...)
	frame, quit := runUI(ctx, func() {
		ret1 = singleSigVerifyFlow(ctx, &descriptorTheme, false /* watch-only */, false, false, &rec)
		ret2 = singleSigVerifyFlow(ctx, &descriptorTheme, false /* watch-only */, false, false, &rec)
		done = true
	})
	defer quit()

	// FIRST ATTEMPT: a DIFFERENT seed than what was engraved. The plates on
	// the bench are fine; only the re-type is wrong.
	if c, ok := pumpUntil(frame, "Choose number of words", 96); !ok {
		t.Fatalf("the first attempt did not ask for the seed; got %q", c)
	}
	click(&ctx.Router, Button3) // 12 WORDS
	frame()
	driveWords(&ctx.Router, fixtureMasterB)
	if c, ok := pumpUntil(frame, "Wallet Type", 240); !ok {
		t.Fatalf("the first attempt did not reach the wallet-type picker; got %q", c)
	}
	click(&ctx.Router, Button3) // BIP-84 default
	if c, ok := pumpUntil(frame, "Add a BIP-39 passphrase?", 96); !ok {
		t.Fatalf("the first attempt did not reach the passphrase prompt; got %q", c)
	}
	click(&ctx.Router, Button3) // Skip
	frame()
	if c, ok := pumpUntil(frame, "mk1 keys:", 96); !ok {
		t.Fatalf("the first attempt's readback never reached the gatherer's tally; got %q", c)
	}
	click(&ctx.Router, Button3) // Done adding cards
	frame()
	if c, ok := pumpUntil(frame, "Verify Failed", 96); !ok {
		t.Fatalf("the wrong seed did not FAIL the first attempt; got %q", c)
	}
	click(&ctx.Router, Button3) // dismiss
	frame()

	// bundleGatherFlow CONSUMES ctx.syswBundleSeeds on entry
	// (gui/bundle_flow.go:171-177, "ctx.syswBundleSeeds = nil"): it is a
	// one-shot payload, and the first attempt above already drained it. The
	// SAME plates are still on the operator's bench for the retry -- this
	// re-presents them, the way a second NFC tap would.
	ctx.syswBundleSeeds = append(append([]string(nil), b.MK1...), b.MD1...)

	// THE RETRY: the seed the plates were ACTUALLY engraved with.
	if c, ok := pumpUntil(frame, "Choose number of words", 96); !ok {
		t.Fatalf("the retry did not ask for the seed; got %q", c)
	}
	click(&ctx.Router, Button3) // 12 WORDS
	frame()
	driveWords(&ctx.Router, abandonAboutPhrase())
	if c, ok := pumpUntil(frame, "Wallet Type", 240); !ok {
		t.Fatalf("the retry did not reach the wallet-type picker; got %q", c)
	}
	click(&ctx.Router, Button3) // BIP-84 default
	if c, ok := pumpUntil(frame, "Add a BIP-39 passphrase?", 96); !ok {
		t.Fatalf("the retry did not reach the passphrase prompt; got %q", c)
	}
	click(&ctx.Router, Button3) // Skip
	frame()
	if c, ok := pumpUntil(frame, "mk1 keys:", 96); !ok {
		t.Fatalf("the retry's readback never reached the gatherer's tally; got %q", c)
	}
	click(&ctx.Router, Button3) // Done adding cards
	frame()
	if c, ok := pumpUntil(frame, "Verify OK", 96); !ok {
		t.Fatalf("the correct seed did not PASS the retry; got %q", c)
	}
	click(&ctx.Router, Button3) // dismiss

	for i := 0; i < 64 && !done; i++ {
		frame()
	}
	if !done {
		t.Fatal("the two-attempt sequence never completed")
	}

	if !ret1 {
		t.Error("the first (failed) attempt must report retryable=true, or the caller's " +
			"loop has nothing to key its re-offer on")
	}
	if ret2 {
		t.Error("the second (passed) attempt must report retryable=false")
	}
	if !rec.adverse {
		t.Error("the sticky adverse bit was lost across the retry -- a clean second pass " +
			"would then render as a bare statusVerified rather than statusVerifiedOnRetry")
	}
	if rec.pass == nil {
		t.Fatal("the retry's pass was never recorded")
	}

	got := buildVerifyStatusLine(rec)
	if verifyStatusFor(rec) != statusVerifiedOnRetry {
		t.Fatalf("this record does not land in the statusVerifiedOnRetry cell (adverse=%v, "+
			"pass=%v) -- the trace this test exists to run never happens", rec.adverse, rec.pass != nil)
	}
	want := buildVerifyPassLine(*rec.pass) + " " + verifyStatusRetryClause
	if got != want {
		t.Errorf("buildVerifyStatusLine's own construction does not match what it returned:\n"+
			"got  %q\nwant %q", got, want)
	}
	// THE LINE'S CONTENT IS TRUE OF WHAT THIS RUN ACTUALLY DID: one key plate
	// (mk1) was read back and matched on the retry -- singleSigVerifyFlow's
	// own construction always writes legs:1 -- no ms1 was compared
	// (watch-only), and the retry clause names an earlier failure this run's
	// own first attempt produced.
	for _, want := range []string{
		"1 key plate was read back and matched what this run engraved.",
		verifyStatusNoMS1Clause,
		verifyStatusRetryClause,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the retried status line is missing %q:\ngot %q", want, got)
		}
	}
	if strings.Contains(got, verifyStatusCosignerClause) {
		t.Errorf("a single-sig retry line must not carry the multisig cosigner clause "+
			"(single-sig has no policy cosigners): %q", got)
	}
	if strings.Contains(got, verifyStatusMS1Clause) {
		t.Errorf("a WATCH-ONLY retry line must not claim an ms1 comparison that never ran: %q", got)
	}
	t.Logf("statusVerifiedOnRetry line for a retried single-sig pass:\n%s", got)
}
