package gui

import (
	"testing"
	"testing/synctest"

	"seedhammer.com/bip39"
	"seedhammer.com/gui/op"
)

// §10.2.4 rows 4 and 5 -- F-105: arming the wipe during passphrase entry
// (Task 9). Settled by `agent-reports/…-task9-r0.md`: the seam is
// unlockPassphraseFlow's OWN bracket, and it must close BEFORE
// unlockAttemptOnce (and therefore unlockDerive) ever runs -- arming across
// the KDF is unsurvivable (C1), not merely a UX wrinkle. These four tests are
// lifted from the R0 report's §5 checks, which already carry positive
// controls and (for the two zeroing checks) mutation evidence; see that
// report for the arithmetic behind "unsurvivable".
//
// NOT tested here, per the report's own finding: a "complete but
// unsubmitted" park. §1 of the report traces every frame-free stretch between
// the twelfth word and the KDF and finds none -- a wipe can only be raised
// from inside ctx.Frame, and there is no ctx.Frame call in that stretch.
// Fabricating that state would pin a park the product cannot reach.

// TestUnlockPassphraseWipeDuringPartialEntryZeroesTheWordBuffer -- 9.3(1).
//
// This is a REGRESSION check, not a Task 9 discriminator: the clear(m) on
// unlockPassphraseFlow's partial-entry exit predates this task and is
// unconditional on ctx.wipe. What Task 9 must not do is disturb it while
// adding the bracket around the same function.
func TestUnlockPassphraseWipeDuringPartialEntryZeroesTheWordBuffer(t *testing.T) {
	var typed []bip39.Mnemonic
	unlockPassphraseWordsHook = func(m bip39.Mnemonic) { typed = append(typed, m) }
	t.Cleanup(func() { unlockPassphraseWordsHook = nil })

	v := sealVector(t, "D")
	h := newUnlockHarness(t, payloadReaderFrom(t, v.blob(t)))
	h.toPassphrase(true)
	words := vectorPassphrase(t, v)
	for _, w := range words[:6] {
		h.typeWord(w)
	}
	if len(typed) != 1 {
		t.Fatalf("the word buffer was handed over %d times, want exactly 1", len(typed))
	}
	nonZero := 0
	for _, w := range typed[0] {
		if w > 0 {
			nonZero++
		}
	}
	// The positive control: a buffer that was never typed into reads all -1,
	// so this must have carried real words before the unwind.
	if nonZero == 0 {
		t.Fatal("positive control failed: no words were typed before the unwind, " +
			"so this test asserted nothing")
	}
	// The §11.2 exit: yield returns false, so ctx.Done is set from inside
	// ctx.Frame -- the same unwind §10.2.4's wipe uses. inputWordsFlow returns
	// on it and unlockPassphraseFlow's `if !isMnemonicComplete(m) { clear(m); ...}`
	// branch fires.
	h.quit()
	if !allZeroWords(typed[0]) {
		t.Errorf("the PARTIAL typed passphrase word buffer SURVIVED the unwind: %v\n"+
			"§10.2.4 row 4 requires it zeroed on every exit, and a walk-away mid-entry "+
			"is the ordinary shape of that exit", typed[0])
	}
}

// TestUnlockPassphraseBracketExcludesTheKDF -- 9.3(2), the C1 guard.
//
// The one property the R0 review's Critical turned on: ctx.wipe must read
// nil for the derivation's ENTIRE run, not just at its start. Sampled on
// EVERY drawn KDF frame, not once -- a single sample could not tell "closes
// before the KDF starts" from "reopens partway through".
func TestUnlockPassphraseBracketExcludesTheKDF(t *testing.T) {
	v := sealVector(t, "D")
	h := newUnlockHarness(t, payloadReaderFrom(t, v.blob(t)))
	h.toPassphrase(true)
	h.typePassphrase(vectorPassphrase(t, v))

	sampled := 0
	reachedPlateList := false
	for i := 0; i < 512; i++ {
		c, ok := h.frame()
		if !ok {
			t.Fatalf("the flow produced no frame; last frame %q", h.content)
		}
		h.content = c
		if uiContains(c, "Unlocking") {
			sampled++
			if h.ctx.wipe != nil {
				t.Fatalf("ctx.wipe is non-nil on a KDF progress frame (sample %d) -- "+
					"the passphrase bracket must close BEFORE unlockAttemptOnce runs "+
					"(§10.2.4 row 5; arming here would freeze the derivation under the "+
					"warning and make it unopenable): %q", sampled, c)
			}
		}
		if uiContains(c, "SECRET seed material") {
			reachedPlateList = true
			break
		}
	}
	// Positive controls: the KDF must actually have run, and the vector's own
	// passphrase must actually have unlocked it -- otherwise "nil throughout"
	// is vacuously true because there was no derivation to sample.
	if sampled == 0 {
		t.Fatal("positive control failed: no KDF progress frame was ever observed, " +
			"so this test asserted nothing")
	}
	if !reachedPlateList {
		t.Fatalf("the vector's own passphrase did not unlock the payload; last frame %q", h.content)
	}
}

// TestUnlockPassphraseWarningIsNotArmedOnTheHashScreens -- 9.3(3).
//
// Two screens carry the §6.6 public-data hash the operator is meant to
// compare: the notice shown BEFORE any word is typed (unlockPayloadFlow,
// outside unlockSealedFlow entirely) and the retry screen shown AFTER a wrong
// passphrase (unlockRetryBody, inside unlockSealedFlow but outside
// unlockPassphraseFlow's bracket). Arming either would wipe mid-comparison
// for no benefit: nothing typed is resident on the first, and clear(m) has
// already run before the second is ever shown.
func TestUnlockPassphraseWarningIsNotArmedOnTheHashScreens(t *testing.T) {
	v := sealVector(t, "D")
	h := newUnlockHarness(t, payloadReaderFrom(t, v.blob(t)))

	h.mustReach("Public data hash")
	if h.ctx.wipe != nil {
		t.Error("ctx.wipe is non-nil on the public-data-hash notice, before any word " +
			"has been typed -- nothing here is wipeable and arming it protects nothing")
	}

	h.toPassphrase(true)
	// A checksum-VALID but WRONG passphrase, so the retry screen is reached
	// via the tag failure, not the checksum gate -- same shape as
	// TestUnlockRetryKeepsTheHashOnScreen.
	wrong := []string{"abandon", "abandon", "abandon", "abandon", "abandon", "abandon",
		"abandon", "abandon", "abandon", "abandon", "abandon", "about"}
	if !mnemonicOf(t, wrong...).Valid() {
		t.Fatal("premise broken: the wrong passphrase must still be checksum-valid")
	}
	h.typePassphrase(wrong)
	content := h.mustReach("has been altered")
	if !uiContains(content, *v.PubhashSealed) {
		t.Fatalf("premise broken: the retry screen does not carry the §6.6 hash: %q", content)
	}
	if h.ctx.wipe != nil {
		t.Error("ctx.wipe is non-nil on the retry screen -- wiping mid-comparison destroys " +
			"the one signal the operator is there to check, and clear(m) has already run " +
			"so there is nothing left to protect")
	}
}

// TestUnlockPassphraseWarningShowsTheRow4Subject -- 9.3(4). Drives the real
// §10.2.4 timer (via Task 1's Run-level harness) through unlockPassphraseFlow
// parked at word entry with nothing typed, past idleTimeout and the 30 s
// warning, and reads the text Run actually draws.
func TestUnlockPassphraseWarningShowsTheRow4Subject(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p := newDeadlinePlatform()
		sessions := 0
		flow := func(ctx *Context, version string) {
			sessions++
			if sessions > 1 {
				// The wipe already fired in session 1; end the test cleanly
				// rather than re-arming and looping.
				ctx.Done = true
				return
			}
			unlockPassphraseFlow(ctx, &descriptorTheme)
		}
		var sawRow4, sawRow1 bool
		onDraw := func(o op.Op, text string) {
			if uiContains(text, "partly typed passphrase") {
				sawRow4 = true
			}
			if uiContains(text, "decrypted seed material") {
				sawRow1 = true
			}
		}
		drawn := mustFinish(t, p, flow, onDraw)
		if sessions != 2 {
			t.Fatalf("expected the wipe to restart the session (session 2), got %d sessions; drawn=%q",
				sessions, drawn)
		}
		if !sawRow4 {
			t.Error("the passphrase-entry warning never showed row 4's text " +
				"(\"partly typed passphrase\")")
		}
		if sawRow1 {
			t.Error("the passphrase-entry warning showed row 1's text (\"decrypted seed " +
				"material\"), which is FALSE at passphrase entry -- nothing has been decrypted yet")
		}
	})
}
