package gui

import (
	"strings"
	"testing"
)

// THE PICKER PANICKED. Found by walking the flow (G-P3.20), not by reading it.
//
//	panic: runtime error: slice bounds out of range [:8] with length 0
//
// engraveTransactionFlowSeeded built one choice row per candidate, and every
// row read `c.tx.TxidDisplay[:8]`. An UNCONFIRMED candidate has the ZERO-VALUE
// mt.Tx -- it never reassembled, so there is no txid -- and the rows are built
// for ALL candidates before `len(choices) > 1` decides whether to show the
// screen. So a payload holding a single incomplete mt1 set crashed the Engrave
// Transaction program outright.
//
// It has been live since the ruling-2026-08-25 fold made unconfirmed sets
// engraveable (verified at the phase's base commit, 82fad4a), and the
// acceptance sheet recorded the picker row as MET, "keyed on the 5-hex csid --
// honest, since no txid exists". The intent was right; the code never got
// there.
//
// The payload that triggers it is the ordinary one the ruling exists FOR: an
// operator who packed the strings they had.

// The direct assertion. Row-building is a pure function now, so the crashing
// case is a value rather than a screen.
func TestThePickerRowSurvivesEveryCandidateShape(t *testing.T) {
	ctx := NewContext(newPlatform())

	// (1) An incomplete set: no txid, no bytes.
	ctx.sysw = sessionWith(txEven[:3]...)
	incomplete, _ := payloadTransactions(ctx)
	if len(incomplete) != 1 || incomplete[0].confirmed {
		t.Fatalf("premise: %+v", incomplete)
	}
	row := transactionChoiceRow(incomplete[0])
	if !strings.Contains(row, "2DCF2") {
		t.Errorf("an unconfirmed candidate is keyed on its SET id: %q", row)
	}
	if strings.Contains(row, "TX ") {
		t.Errorf("it must not be labelled TX -- there is no txid: %q", row)
	}
	if !strings.Contains(row, "3 strings") {
		t.Errorf("it must say what the operator would be cutting: %q", row)
	}

	// (2) A confirmed set.
	ctx.sysw = sessionWith(txEven...)
	confirmed, _ := payloadTransactions(ctx)
	row = transactionChoiceRow(confirmed[0])
	if !strings.Contains(row, "TX 2DCF2B97") || !strings.Contains(row, "6 strings") {
		t.Errorf("%q", row)
	}

	// (3) A signed tx: record: bytes, no strings.
	ctx.sysw = sessionWith("tx:" + rawHexOf(t, evenTx(t)))
	rec, _ := payloadTransactions(ctx)
	row = transactionChoiceRow(rec[0])
	if !strings.Contains(row, "TX 2DCF2B97") || !strings.Contains(row, "raw bytes") {
		t.Errorf("%q", row)
	}

	// (4) An UNSIGNED tx: record: bytes and a real txid, and it must not be
	// offered under the same label as a spendable one.
	ctx.sysw = sessionWith("tx:" + txStrippedHex)
	unsigned, _ := payloadTransactions(ctx)
	row = transactionChoiceRow(unsigned[0])
	if !strings.Contains(row, "UNSIGNED") {
		t.Errorf("an unsigned transaction must be labelled in the picker, where "+
			"the operator chooses: %q", row)
	}
}

// AND THE PROGRAM SURVIVES ENTERING IT. The rows are built for every candidate
// before the screen is shown, so a single unconfirmed candidate crashed a
// program that was never going to display a picker at all.
func TestTheProgramDoesNotCrashOnAPayloadOfIncompleteStrings(t *testing.T) {
	w := newTxWalk(t, txEven[:3]...)
	defer w.quit()
	w.until("UNCONFIRMED SET")
}

// ...and with a picker actually SHOWN, over a mixture with no txid in it at
// all. This is the shape that crashed: rows are built for every candidate
// before the screen is decided on, so nothing about it depended on the picker
// being displayed.
//
// Two different chunk_set_ids: an incomplete set (0x2dcf2) and a complete
// 1-chunk set of entropy that does not decode. Neither has a txid.
func TestThePickerShowsTwoUnconfirmedSetsWithoutATxidBetweenThem(t *testing.T) {
	w := newTxWalk(t, txEven[0], mtSmuggled)
	defer w.quit()
	got := w.until("Which transaction?")
	if !uiContains(got, "SET") {
		t.Errorf("both rows are sets: %q", got)
	}
	if uiContains(got, "TX ") {
		t.Errorf("neither candidate has a txid: %q", got)
	}
}
