package gui

import (
	"testing"
	"testing/synctest"
	"time"
)

// ═══ G-P3.20 — THE END-TO-END UI WALK ════════════════════════════════════════
//
// `runUITouch`/`runUI` are used in 39 other test files in this package. Not one
// of them was this program's. Every transaction test called a planner, a
// formatter or a predicate directly, so the program's SPINE --
//
//	choice -> review -> plate kind -> plan confirm -> engrave loop -> post-cut
//
// -- was exercised by nothing, and the two Criticals this phase found both
// lived exactly there: a tx: record produced an empty choice list and the flow
// RETURNED SILENTLY, and the review screen described a signed transaction as an
// unconfirmed set of zero strings. Sixteen transaction tests were green
// throughout.
//
// These walks drive the real flow with the real screens, for TEXT and for QR,
// confirmed and substituted, and they finish the engrave -- so a screen that
// nothing reaches fails here rather than passing everywhere else.

// txWalk is the harness: a platform with a real test engraver, a session, and
// the flow under a frame pump.
type txWalk struct {
	t     *testing.T
	ctx   *Context
	eng   *testEngraver
	pl    *engravedAwarePlatform
	frame func() (string, bool)
	quit  func()
	last  string
}

func newTxWalk(t *testing.T, records ...string) *txWalk {
	t.Helper()
	e := newEngraver()
	p := newEngravedAwarePlatform()
	p.engraver = e
	p.display = sh2DisplaySize
	ctx := NewContext(p)
	ctx.sysw = sessionWith(records...)
	w := &txWalk{t: t, ctx: ctx, eng: e, pl: p}
	w.frame, w.quit = runUI(ctx, func() { engraveTransactionFlow(ctx, &descriptorTheme) })
	return w
}

// until pumps frames until `want` appears, and FAILS otherwise. Every step of a
// walk goes through it, so a flow that ends early is reported at the step it
// ended rather than as a confusing assertion three screens later.
func (w *txWalk) until(want string) string {
	w.t.Helper()
	got, ok := pumpUntil(w.frame, want, 64)
	w.last = got
	if !ok {
		w.t.Fatalf("the walk never reached a screen saying %q.\nLast frame: %q", want, got)
	}
	return got
}

func (w *txWalk) confirm() { click(&w.ctx.Router, Button3) }

// paged pumps, and PAGES, until `want` appears -- which is what the operator
// does on a read-only screen. Where a sentence falls relative to a page break
// is layout, not behaviour, so a walk that asserts a page number would fail on
// a font change; what matters is that the operator can reach the sentence
// without leaving the screen.
func (w *txWalk) paged(want string) string {
	w.t.Helper()
	for page := 0; page < 6; page++ {
		got, ok := pumpUntil(w.frame, want, 24)
		w.last = got
		if ok {
			return got
		}
		click(&w.ctx.Router, Button2)
	}
	w.t.Fatalf("%q is on no page the operator can reach.\nLast frame: %q", want, w.last)
	return ""
}

// engraveOnePlate drives one plate from the ENGRAVE choice through the
// engraver to accepted. Copied in shape from
// TestEngraveScreenReportsTheStringItEngraved, which is the only place in the
// tree that takes a real EngraveScreen to completion.
func (w *txWalk) engraveOnePlate() {
	w.t.Helper()
	w.confirm() // "ENGRAVE" on the per-plate choice screen
	// next, next, next, then hold to confirm the connect step
	click(&w.ctx.Router, Button3, Button3, Button3)
	press(&w.ctx.Router, Button3)
	w.frame()
	time.Sleep(confirmDelay)
	for {
		w.frame()
		select {
		case <-w.eng.closes:
			return
		case <-w.pl.wakeups:
		}
	}
}

// ─── the QR path, end to end ────────────────────────────────────────────────

// A `tx:` record is the whole reason the QR path exists: `me tx` makes it,
// `me sysw pack` carries it, and any ordinary phone scanner turns the symbol
// back into broadcastable hex. This is that journey, from a loaded payload to
// the screen that tells the operator to test the plate.
func TestWalkQRPathFromATxRecordToThePostCutScreen(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		w := newTxWalk(t, "tx:"+rawHexOfEven(t))
		defer w.quit()

		// (1) REVIEW. One candidate, so no picker -- and the screen must be
		// the CONFIRMED one, with the txid the operator compares.
		got := w.until("Engrave this transaction?")
		if !uiContains(got, txEvenTxid[:16]) {
			t.Errorf("the review screen must carry the txid: %q", got)
		}
		if !uiContains(got, "BEARER") {
			t.Errorf("...and the bearer warning: %q", got)
		}
		w.confirm()

		// (2) PLATE KIND. QR is the only form a tx: record can take.
		got = w.until("Choose plate kind")
		if !uiContains(got, "QR PLATES") {
			t.Fatalf("QR was not offered: %q", got)
		}
		if uiContains(got, "TEXT PLATES") {
			t.Errorf("a tx: record carries no mt1 strings: %q", got)
		}
		w.confirm()

		// (3) PLAN CONFIRM. Plate count, protection, module size AND time.
		got = w.until("Engrave?")
		for _, want := range []string{"plate(s)", "QR", "ECC", "modules", "cutting"} {
			if !uiContains(got, want) {
				t.Errorf("the plan-confirm screen never says %q: %q", want, got)
			}
		}
		w.confirm()

		// (4) THE ENGRAVE LOOP.
		got = w.until("Engrave this plate")
		if !uiContains(got, "TX 2DCF2B97 1/1") {
			t.Errorf("the plate title must name the transaction and its place: %q", got)
		}
		w.engraveOnePlate()

		// (5) THE POST-CUT SCREEN. This is the step that nothing reached
		// before: deleting the call site broke no test, because every post-cut
		// assertion called the function directly.
		synctest.Wait()
		click(&w.ctx.Router, Button3) // accept the finished plate
		synctest.Wait()
		w.until("TEST THEM NOW")
		w.paged("mt inspect")
		w.paged("no camera")
	})
}

func rawHexOfEven(t *testing.T) string {
	t.Helper()
	return rawHexOf(t, evenTx(t))
}

// THE BEARER WARNING IS ON THE FIRST PAGE, and this is its own test because it
// is a POSITION, not a wording. Every assertion in the tree checked that the
// sentence exists; none could see that confirmReviewScreen paged it off the
// screen the operator confirms from.
func TestTheBearerWarningIsOnTheFirstPageOfTheReview(t *testing.T) {
	w := newTxWalk(t, "tx:"+rawHexOfEven(t))
	defer w.quit()
	got := w.until("Engrave this transaction?")
	if !uiContains(got, "BEARER") {
		t.Errorf("the operator can press Continue from this page without ever "+
			"seeing the bearer warning:\n%q", got)
	}
	if !uiContains(got, "broadcast") {
		t.Errorf("...and the sentence must be whole, not cut in half: %q", got)
	}
}

// ─── the TEXT path, end to end ──────────────────────────────────────────────

// A CONFIRMED mt1 set: six strings, one plate, engraved verbatim. This is the
// delivery a transaction takes when it is too large for one record, i.e. the
// usual one.
func TestWalkTextPathFromAConfirmedSetToThePostCutScreen(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		w := newTxWalk(t, txEven...)
		defer w.quit()

		got := w.until("Engrave this transaction?")
		if !uiContains(got, "BEARER") {
			t.Errorf("the bearer warning must be on the first page here too: %q", got)
		}
		w.confirm()

		got = w.until("Choose plate kind")
		if !uiContains(got, "TEXT PLATES") || !uiContains(got, "QR PLATES") {
			t.Fatalf("a confirmed SET can be engraved either way: %q", got)
		}
		w.confirm() // TEXT PLATES is choice 0

		got = w.until("Engrave?")
		if !uiContains(got, "6 string(s)") {
			t.Errorf("the plan must state how many strings are being cut: %q", got)
		}
		if !uiContains(got, "cutting") {
			t.Errorf("...and the time: %q", got)
		}
		w.confirm()

		got = w.until("Engrave this plate")
		if !uiContains(got, "TX 2DCF2B97") {
			t.Errorf("plate title: %q", got)
		}
		w.engraveOnePlate()
		synctest.Wait()
		click(&w.ctx.Router, Button3)
		synctest.Wait()

		w.until("TEST THEM NOW")
		got = w.paged("mt verify")
		if uiContains(got, "mt inspect") {
			t.Errorf("a text plate is verified string by string, not as raw bytes: %q", got)
		}
	})
}

// ─── the legend-substitution walk ───────────────────────────────────────────

// RULING 2026-08-25b, walked. An unconfirmed set is engraveable and the
// operator's legend is replaced un-overridably. The screens have to SAY so
// before the cut, and the post-cut screen must not send the operator off to
// check a txid this set never produced.
func TestWalkAnUnconfirmedSetIsEngravedUnderASubstitutedLegend(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		w := newTxWalk(t, txEven[:3]...)
		defer w.quit()

		got := w.until("UNCONFIRMED SET")
		if !uiContains(got, "Set 2dcf2") {
			t.Errorf("the set id is the only identifier an unconfirmed set has: %q", got)
		}
		// The substitution is stated BEFORE the cut, which is the whole point:
		// the operator is told what the plate will say instead of their legend.
		got = w.paged("MISSING STRINGS")
		if !uiContains(got, "RE-ENCODE PAYLOAD") {
			t.Errorf("the substituted legend must be shown in full: %q", got)
		}
		w.confirm()

		got = w.until("Choose plate kind")
		if uiContains(got, "QR PLATES") {
			t.Errorf("an unconfirmed set has no transaction bytes to encode: %q", got)
		}
		if !uiContains(got, "TEXT PLATES") {
			t.Fatalf("the strings the operator HAS are still worth cutting: %q", got)
		}
		w.confirm()

		got = w.until("Engrave?")
		if !uiContains(got, "3 string(s)") {
			t.Errorf("%q", got)
		}
		w.confirm()

		got = w.until("Engrave this plate")
		if !uiContains(got, "SET 2DCF2") {
			t.Errorf("an unconfirmed set's plates are titled by SET, not by TX: %q", got)
		}
		w.engraveOnePlate()
		synctest.Wait()
		click(&w.ctx.Router, Button3)
		synctest.Wait()

		w.until("TEST THEM NOW")
		got = w.paged("did NOT confirm")
		if uiContains(got, "2DCF2B97") {
			t.Errorf("it must not name a txid this set never produced: %q", got)
		}
	})
}

// The UNSIGNED tx: record: the third substitution, and the one that reaches a
// QR plate. It is here rather than in a unit test because the promise the
// review screen makes ("the legend WILL be replaced") is kept by a different
// function three calls away.
func TestWalkAnUnsignedTxRecordIsEngravedUnderTheUnsignedLegend(t *testing.T) {
	w := newTxWalk(t, "tx:"+txStrippedHex)
	defer w.quit()

	w.until("UNSIGNED TRANSACTION")
	got := w.paged("Input 0")
	if !uiContains(got, "Input 0") {
		t.Errorf("it must name which input: %q", got)
	}
	got = w.paged("CANNOT BE BROADCAST")
	w.confirm()

	got = w.until("Choose plate kind")
	if !uiContains(got, "QR PLATES") {
		t.Fatalf("the bytes are all there; QR is still offered: %q", got)
	}
	w.confirm()
	w.until("Engrave?")
}

// ─── the picker ─────────────────────────────────────────────────────────────

// TWO candidates means a CHOICE screen, and it is skipped when there is one.
// The picker is keyed on the txid for a confirmed candidate and on the set id
// for an unconfirmed one -- honest, since no txid exists for the latter.
func TestWalkThePickerAppearsOnlyWhenThereIsAChoice(t *testing.T) {
	// One candidate: straight to the review.
	one := newTxWalk(t, "tx:"+rawHexOfEven(t))
	defer one.quit()
	got := one.until("Engrave this transaction?")
	if uiContains(got, "Which transaction?") {
		t.Errorf("a single candidate must not be picked from a list of one: %q", got)
	}

	// Two: a confirmed set, and a DIFFERENT transaction as a tx: record.
	//
	// It has to be a different TXID, not merely different bytes -- the merge is
	// keyed on TxidDisplay, so the 113-byte stripped form of this very
	// transaction is silently absorbed into the 6-string set. See
	// TestTheMergeIsKeyedOnTheTxidNotOnTheBytes, which measures that and is why
	// this walk does not use it.
	other := "tx:" + rawHexOf(t, syntheticTx(t, 100))
	two := newTxWalk(t, append(append([]string{}, txEven...), other)...)
	defer two.quit()
	got = two.until("Which transaction?")
	if !uiContains(got, "2DCF2B97") {
		t.Errorf("the picker must name what it is picking between: %q", got)
	}
	if !uiContains(got, "6 strings") {
		t.Errorf("...and how each one arrived: %q", got)
	}
	if !uiContains(got, "raw bytes") {
		t.Errorf("...including the one that arrived as a record: %q", got)
	}
}

// R10 / G-P3.10, MEASURED rather than assumed -- and the acceptance sheet's
// premise is wrong.
//
// The sheet records R10 as MET-DIFF because "duplicate candidates merge on
// BYTES, not on the txid (gui/transaction.go:263-327), so identical twins
// collapse safely and different ones stay two candidates", and files the
// residual as G-P3.10: "two byte-different transactions sharing a derived txid
// present as two identical picker rows".
//
// THEY DO NOT PRESENT AS TWO ROWS. The merge reads `c.tx.TxidDisplay`, so a
// byte-different transaction sharing a txid is DROPPED, not duplicated -- and
// the pair that does it is not exotic: a transaction and its own
// signature-stripped form have the same txid by construction.
//
// This test does not change the behaviour. G-P3.10 needs an operator ruling
// (merge on a content digest? refuse? show both with sizes?), and that ruling
// should start from what the code does rather than from what the sheet says it
// does. Recorded here so the walk has a fact.
func TestTheMergeIsKeyedOnTheTxidNotOnTheBytes(t *testing.T) {
	ctx := NewContext(newPlatform())
	// A confirmed 6-string set, plus the SAME transaction with its witnesses
	// stripped: 222 bytes versus 113, one txid.
	ctx.sysw = sessionWith(append(append([]string{}, txEven...), "tx:"+txStrippedHex)...)
	cands, _ := payloadTransactions(ctx)
	if len(cands) != 1 {
		t.Fatalf("MEASURED BEHAVIOUR CHANGED: %d candidates. If the merge is now "+
			"on bytes, the sheet's R10 row is finally true and G-P3.10's premise "+
			"needs rewriting the other way.", len(cands))
	}
	if len(cands[0].tx.Raw) != 222 {
		t.Errorf("the surviving candidate is %d bytes", len(cands[0].tx.Raw))
	}
	t.Log("the 113-byte stripped form was DROPPED, not offered as a second row -- " +
		"the sheet says two rows; there is one")
}
