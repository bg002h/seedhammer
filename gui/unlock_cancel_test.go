package gui

import (
	"strconv"
	"testing"
)

// §10.2 steps 7-9's CANCEL route -- the one unlockSealedFlow's own doc calls out
// and that nothing tested (B2a-ii whole-diff review, lens 2 I1).
//
// TestUnlockCancelNeverReachesThePlateList cancels at WORD ENTRY, which returns
// through unlockPassphraseFlow's `ok == false` and never produces
// errUnlockCancelled at all. The ONLY producer of that error is unlockDerive
// returning !ok -- Back on the ~31 s progress screen, or ctx.Done -- and until
// this file nothing in the suite tapped Button1 on that screen.

// TestUnlockCancelDuringTheKDFNeverReachesThePlateList closes both halves of
// that gap at once:
//
//	G21  case errors.Is(err, errUnlockCancelled): return false  ->  return true
//	H1   if backBtn.Clicked(ctx) { ... }  ->  if backBtn.Clicked(ctx) && false
//
// Under G21 the operator taps Back, p.Secret is never populated, and the flow
// walks on through an empty secret session into unlockPlateListFlow -- which for
// vector D is five legitimate mk1/md1 plates. They engrave the PUBLIC half of a
// sealed payload, see a complete-looking plate list, and store an incomplete
// backup believing it complete. That is §6.4's incomplete-backup-believed-
// complete, the worst available outcome, and it is exactly what
// unlockSealedFlow's contract says a false return exists to prevent.
//
// Under H1 the only escape from a ~31 s wait silently does nothing.
//
// So this test asserts BOTH directions of the same tap: the flow must END, and
// no plate label may ever be drawn on the way out.
func TestUnlockCancelDuringTheKDFNeverReachesThePlateList(t *testing.T) {
	v := sealVector(t, "D")
	h := newUnlockHarness(t, payloadReaderFrom(t, v.blob(t)))
	h.toPassphrase(true)
	h.typePassphrase(vectorPassphrase(t, v))

	// The derivation is ON SCREEN and NOT finished. Without this the test could
	// tap Back at a moment the flow was never in unlockDerive, and would then
	// pass for the wrong reason.
	m := progressPct.FindStringSubmatch(h.content)
	if m == nil {
		t.Fatalf("the derivation never reached the progress screen; last frame %q", h.content)
	}
	pct, err := strconv.Atoi(m[1])
	if err != nil {
		t.Fatalf("unreadable progress reading %q", m[1])
	}
	if pct >= 50 {
		t.Fatalf("the derivation is already %d%% done at the first frame; this test needs "+
			"to cancel MID-derivation, not after it", pct)
	}

	// Back, the only escape from the wait.
	h.tapNav(Button1)

	labels := plateRecords(t, "D")
	if len(labels) == 0 {
		t.Fatal("premise broken: vector D must carry public records for the plate list to show")
	}
	// The budget covers a whole 100,000-iteration derivation (199 frames) plus
	// the secret session and plate list that would follow it, so a mutant that
	// merely IGNORES the tap runs out of frames here rather than being missed.
	for i := 0; i < 512 && !*h.done; i++ {
		c, ok := h.frame()
		if !ok {
			break
		}
		for j := range labels {
			if uiContains(c, plateLabel(labels[j], j)) {
				t.Fatalf("cancelling the derivation reached the PLATE LIST: %q\n"+
					"a false return from unlockSealedFlow must not fall through: p.Public "+
					"on a sealed payload is a legitimate record set, and engraving it "+
					"while dropping the encrypted half is §6.4's incomplete-backup-"+
					"believed-complete", c)
			}
		}
		// Nor may the secret session run: the operator cancelled before any
		// record was ever decrypted.
		if uiContains(c, "SECRET seed material") {
			t.Fatalf("cancelling the derivation reached §10.2.2's secret session: %q", c)
		}
	}
	if !*h.done {
		t.Fatalf("Back during the derivation did not end the flow; last frame %q\n"+
			"§10.2 step 7's progress screen has exactly one escape from a ~31 s wait, "+
			"and it must work", h.content)
	}
}
