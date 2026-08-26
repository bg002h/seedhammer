package gui

import (
	"strings"
	"testing"

	qr "github.com/seedhammer/kortschak-qr"
)

// ─── G-P3.17(a) — the per-plate instruction is about THAT plate ─────────────

// §4.3a. The legend is cut into ONE plate, and which plate that is changes what
// the instruction should say. The LEGEND-ONLY layout -- chosen whenever
// P-1-symbols-plus-a-legend-plate beats P symbols inline -- put "scan all qr,
// any order" onto a plate with NO SYMBOL ON IT: an instruction about plates
// that are not this one, cut into the only plate there is nothing to scan on.
func TestTheLegendPlateDoesNotTellYouToScanItself(t *testing.T) {
	c := txCandidate{tx: evenTx(t), confirmed: true}
	alone := transactionLegend(c, 3, qr.M, false)
	inline := transactionLegend(c, 3, qr.M, true)

	if alone == inline {
		t.Fatal("a legend-only plate and a legend-beside-a-symbol plate say the same thing")
	}
	if !strings.Contains(alone, "on the other plates") {
		t.Errorf("the legend plate must say where the symbols are:\n%s", alone)
	}
	if !strings.Contains(alone, "scan them all") {
		t.Errorf("...and what to do with them:\n%s", alone)
	}
	if strings.Contains(inline, "on the other plates") {
		t.Errorf("a plate that carries a symbol must not disown it:\n%s", inline)
	}
	// Both still carry the identifying facts.
	for _, l := range []string{alone, inline} {
		if !strings.Contains(l, "txid "+txEvenTxid) {
			t.Errorf("every legend carries the txid:\n%s", l)
		}
	}
}

// A SINGLE-symbol plate must not say "all qr, any order" either: there is one,
// and order is not a question the operator has.
func TestASingleSymbolPlateSaysScanThisOne(t *testing.T) {
	c := txCandidate{tx: evenTx(t), confirmed: true}
	one := transactionLegend(c, 1, qr.H, true)
	if strings.Contains(one, "any order") {
		t.Errorf("one symbol has no order:\n%s", one)
	}
	if !strings.Contains(one, "scan this qr") {
		t.Errorf("%s", one)
	}
}

// AND THE REAL PLANNER USES IT. The unit test above could pass while
// buildQRPlates went on passing `true` for every plate.
func TestTheLegendPlateInARealPlanIsAboutItself(t *testing.T) {
	pl := newPlatform()
	// Big enough to need several symbols, so the legend-alone layout is tried.
	tx := syntheticTx(t, 2400)
	plates, titles, note, err := planTransactionQRPlates(pl, txCandidate{tx: tx, confirmed: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(plates) < 2 {
		t.Skipf("this size planned onto %d plate(s) (%s); the legend-alone "+
			"layout is not exercised", len(plates), note)
	}
	if len(titles) != len(plates) {
		t.Fatalf("%d plates, %d titles", len(plates), len(titles))
	}
}

// ─── G-P3.18 — the confirm screen states TIME, not just plate count ─────────

// The code claimed "plate count and ECC are the two numbers the operator
// budgeted blanks and TIME by" while no time appeared anywhere. At ~21 minutes
// a plate a four-plate job is most of an afternoon, and stopping mid-set costs
// a blank (G-P3.13), so the estimate is not a nicety.
func TestTheJobTimeComesFromThePlanAndTheSameClockTheRunUses(t *testing.T) {
	pl := newPlatform()
	plates, _, _, err := planTransactionQRPlates(pl, txCandidate{tx: evenTx(t), confirmed: true})
	if err != nil {
		t.Fatal(err)
	}
	tps := pl.EngraverParams().TicksPerSecond
	got := transactionJobTime(plates, tps)
	if !strings.Contains(got, "cutting") {
		t.Errorf("%q", got)
	}
	// It is DERIVED, not a constant: doubling the plan doubles the time.
	one := transactionJobTime(plates, tps)
	many := transactionJobTime(append(append([]Plate{}, plates...), plates...), tps)
	if one == many {
		t.Errorf("twice the plates reported the same time: %q", one)
	}
	// A zero-tick machine says so rather than dividing by zero on a confirm
	// screen -- the same posture engraveRemaining takes.
	if got := transactionJobTime(plates, 0); !strings.Contains(got, "unknown") {
		t.Errorf("a zero-tick machine must not divide by zero: %q", got)
	}
}

// The confirm screen the operator actually sees carries both numbers.
func TestThePlanConfirmScreenStatesPlatesAndTime(t *testing.T) {
	ctx := NewContext(newPlatform())
	ctx.sysw = sessionWith("tx:" + rawHexOf(t, evenTx(t)))
	cands, _ := payloadTransactions(ctx)

	frame, quit := runUI(ctx, func() {
		transactionReviewAndEngrave(ctx, &descriptorTheme, cands[0])
	})
	defer quit()
	if _, ok := pumpUntil(frame, "Engrave this transaction?", 32); !ok {
		t.Fatal("no review screen")
	}
	click(&ctx.Router, Button3) // confirm the review
	if _, ok := pumpUntil(frame, "Choose plate kind", 32); !ok {
		t.Fatal("no plate-kind choice")
	}
	click(&ctx.Router, Button3) // QR PLATES (the only choice for a tx: record)
	got, ok := pumpUntil(frame, "Engrave?", 64)
	if !ok {
		t.Fatalf("no plan-confirm screen; last frame %q", got)
	}
	if !uiContains(got, "plate(s)") {
		t.Errorf("the confirm screen must state the plate count: %q", got)
	}
	if !uiContains(got, "cutting") {
		t.Errorf("the confirm screen must state the cut TIME: %q", got)
	}
}

// ─── G-P3.17(b) — one per-JOB instruction, after the last plate ─────────────

// The device says "test the plate" and never tests one, because it CANNOT: the
// SH2 has no camera, so nothing on this machine can read a plate back. If the
// operator does not check the artifact now, nobody checks it until the day it
// is needed.
func TestThePostCutScreenNamesOneCommandAndSaysOrderDoesNotMatter(t *testing.T) {
	ctx := NewContext(newPlatform())
	ctx.sysw = sessionWith("tx:" + rawHexOf(t, evenTx(t)))
	cands, _ := payloadTransactions(ctx)
	got := strings.Join(transactionPostCutLines(cands[0], true, 3), " ")
	for _, want := range []string{"mt inspect", "Order does not matter", "no camera", "3 plate(s)", "TEST THEM NOW"} {
		if !strings.Contains(got, want) {
			t.Errorf("the post-cut screen never says %q: %q", want, got)
		}
	}
	if !strings.Contains(got, "2DCF2B97") {
		t.Errorf("it must name the txid to check against: %q", got)
	}
	// ONCE, and only here: `mt inspect` must not also be on the plate.
	legend := transactionLegend(cands[0], 3, qr.M, true)
	if strings.Contains(legend, "mt inspect") {
		t.Errorf("the per-JOB instruction leaked onto a plate: %s", legend)
	}
}

// TEXT plates get a DIFFERENT command, because a chunk string is not a QR and
// `mt inspect` is not what reads one.
func TestThePostCutScreenNamesTheRightCommandForTextPlates(t *testing.T) {
	ctx := NewContext(newPlatform())
	ctx.sysw = sessionWith(txEven...)
	cands, _ := payloadTransactions(ctx)
	got := strings.Join(transactionPostCutLines(cands[0], false, 1), " ")
	if !strings.Contains(got, "mt verify") {
		t.Errorf("a text plate is verified string by string: %q", got)
	}
	if strings.Contains(got, "mt inspect") {
		t.Errorf("`mt inspect` reads raw bytes, not chunk strings: %q", got)
	}
}

// AN UNCONFIRMED SET MUST NOT BE SENT OFF TO CHECK A TXID IT DOES NOT HAVE.
// Telling the operator to compare a txid that was never derived sends them to
// a host tool to be told the same thing again -- and invites them to read the
// failure as a problem with the plate.
func TestThePostCutScreenDoesNotPromiseATxidAnUnconfirmedSetLacks(t *testing.T) {
	ctx := NewContext(newPlatform())
	ctx.sysw = sessionWith(txEven[:3]...)
	cands, _ := payloadTransactions(ctx)
	if cands[0].confirmed {
		t.Fatal("premise broken: this set confirms")
	}
	got := strings.Join(transactionPostCutLines(cands[0], false, 1), " ")
	if !strings.Contains(got, "did NOT confirm") {
		t.Errorf("it must say the set did not confirm here: %q", got)
	}
	if strings.Contains(got, "2DCF2B97") {
		t.Errorf("it must not name a txid this set never produced: %q", got)
	}
}

// EVERY LINE IS REACHABLE ON THE PANEL. The first draft of this screen was a
// showNotice modal, and ErrorScreen does not page: the body stopped
// mid-sentence at "Order does not matter - it is inside", so the operator
// never reached what to check the txid against, or that this machine can never
// read a plate back. Three assertions on the wording passed while the words
// were unreachable.
//
// So this drives the real screen and PAGES it, asserting every line arrives.
// A test that reads the strings out of the builder cannot see this class at
// all -- which is why the two tests above do exactly that and this one does
// not.
func TestEveryLineOfThePostCutScreenIsReachableOnThePanel(t *testing.T) {
	pl := newPlatform()
	pl.display = sh2DisplaySize
	ctx := NewContext(pl)
	ctx.sysw = sessionWith("tx:" + rawHexOf(t, evenTx(t)))
	cands, _ := payloadTransactions(ctx)
	want := transactionPostCutLines(cands[0], true, 3)

	frame, quit := runUI(ctx, func() {
		transactionPostCutFlow(ctx, &descriptorTheme, cands[0], true, 3)
	})
	defer quit()

	seen := map[int]bool{}
	for page := 0; page < 8; page++ {
		content, ok := frame()
		if !ok {
			break
		}
		for i, l := range want {
			if l == "" || uiContains(content, l) {
				seen[i] = true
			}
		}
		click(&ctx.Router, Button2) // page
	}
	for i, l := range want {
		if !seen[i] {
			t.Errorf("line %d never appeared on any page: %q", i, l)
		}
	}
}

// THE CONTROL for the test above: it must be able to fail. A line the screen
// does not contain has to be reported missing, or the paging walk is proving
// nothing about the lines it does find.
func TestThePostCutReachabilityWalkCanFail(t *testing.T) {
	pl := newPlatform()
	pl.display = sh2DisplaySize
	ctx := NewContext(pl)
	ctx.sysw = sessionWith("tx:" + rawHexOf(t, evenTx(t)))
	cands, _ := payloadTransactions(ctx)

	frame, quit := runUI(ctx, func() {
		transactionPostCutFlow(ctx, &descriptorTheme, cands[0], true, 3)
	})
	defer quit()
	const absent = "THIS SENTENCE IS ON NO PAGE"
	for page := 0; page < 8; page++ {
		content, ok := frame()
		if !ok {
			break
		}
		if uiContains(content, absent) {
			t.Fatal("the walk found a line the screen does not carry")
		}
		click(&ctx.Router, Button2)
	}
}
