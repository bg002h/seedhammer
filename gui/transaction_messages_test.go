package gui

import (
	"strings"
	"testing"

	qr "github.com/seedhammer/kortschak-qr"
	"seedhammer.com/sysw"
	"seedhammer.com/txqr"
)

// ─── G-P3.11 — R11' has THREE branches, not two ─────────────────────────────

// The table in TestNoPayloadAndNoTransactionAreDifferentMessages covered two.
// The third exists because a payload nobody has authenticated is NOT a payload
// with nothing in it, and the fix is different: compare the digest. Saying
// "holds no transaction" there would be a claim about contents this session is
// not allowed to read.
func TestR11HasThreeDistinctMessages(t *testing.T) {
	noPayload := txNothingToEngrave(nil, 0)

	// LOADED BUT NOT COMPARED. syswLoadFlow drops such a session today
	// (ctx.sysw = nil when the digest is declined), so this branch is
	// DEFENSIVE rather than reachable from the load flow -- which is exactly
	// why it had no test: nobody could get to it to notice it was wrong. The
	// state is one field away for any future caller of syswSession.load, and a
	// branch that would say the wrong thing when it fires is worth pinning
	// while it costs three lines.
	uncompared := &syswSession{}
	uncompared.load(&sysw.Payload{Public: []string{"text:6869"}}, [32]byte{1},
		false, false, false /* compared */, true)
	notCompared := txNothingToEngrave(uncompared, 0)

	compared := sessionWith("text:6869")
	noTx := txNothingToEngrave(compared, 0)

	if !strings.Contains(noPayload, "No payload is loaded") {
		t.Errorf("(a) %q", noPayload)
	}
	if !strings.Contains(notCompared, "not been checked") {
		t.Errorf("(b) %q", notCompared)
	}
	if strings.Contains(notCompared, "holds no transaction") {
		t.Errorf("(b) must not speak about contents it has not authenticated: %q", notCompared)
	}
	if !strings.Contains(notCompared, "digest") {
		t.Errorf("(b) must name the fix -- compare the digest: %q", notCompared)
	}
	if !strings.Contains(noTx, "holds no transaction") {
		t.Errorf("(c) %q", noTx)
	}
	for i, a := range []string{noPayload, notCompared, noTx} {
		for j, b := range []string{noPayload, notCompared, noTx} {
			if i < j && a == b {
				t.Errorf("branches %d and %d are the same message", i, j)
			}
		}
	}
}

// The ORPHAN suffix is the fourth thing this function does, and it composes
// with (c) rather than replacing it: the payload is still loaded and still
// holds things, so the operator needs the inventory either way.
func TestOrphanStringsAreASuffixNotAMessage(t *testing.T) {
	s := sessionWith("text:6869")
	plain := txNothingToEngrave(s, 0)
	withOrphans := txNothingToEngrave(s, 3)
	if !strings.HasPrefix(withOrphans, plain) {
		t.Errorf("the orphan count must be a SUFFIX:\n  plain: %q\n  with:  %q", plain, withOrphans)
	}
	if !strings.Contains(withOrphans, "3 mt1 string") {
		t.Errorf("must count the orphans: %q", withOrphans)
	}
}

// ─── G-P3.12 — R16 names the module size ────────────────────────────────────

// §4.1a requires the refusal to name the MODULE SIZE, not just the byte count.
// "1700 bytes is too large" tells the operator nothing about how much too
// large, and the ceiling is not a property of the transaction at all -- it is a
// property of the smallest module this machine will cut.
func TestTheQRRefusalNamesTheModuleSizeAndTheCeiling(t *testing.T) {
	pl := newPlatform()
	// Past the MEASURED 16-symbol capacity at 0.6mm on an 85mm plate (17,968
	// bytes), which is the only way to reach this refusal at all.
	tx := syntheticTx(t, 20000)
	_, _, _, err := planTransactionQRPlates(pl, txCandidate{tx: tx, confirmed: true})
	if err == nil {
		t.Fatal("a 12KB transaction must not fit 16 symbols on one 85mm plate face")
	}
	msg := err.Error()
	for _, want := range []string{"0.6mm", "ECC M", "16 Structured Append", "TEXT plates"} {
		if !strings.Contains(msg, want) {
			t.Errorf("the refusal never says %q:\n%s", want, msg)
		}
	}
	ceiling := qrCeilingBytes(pl.EngraverParams(), qr.M, 2)
	if ceiling <= 0 {
		t.Fatalf("the computed ceiling is %d", ceiling)
	}
	if !strings.Contains(msg, "at most") {
		t.Errorf("the refusal must state the ceiling: %s", msg)
	}
}

// THE CEILING IS TRUE, not a decoration. A transaction AT the ceiling plans;
// one just past it does not. A number in a refusal that nobody checks is how a
// message goes stale silently.
func TestTheStatedCeilingIsTheRealCeiling(t *testing.T) {
	pl := newPlatform()
	params := pl.EngraverParams()
	ceiling := qrCeilingBytes(params, qr.M, 2)
	usable := params.F(85) - 2*params.I(3) - 2*params.I(2)

	fits := func(n int) bool {
		set, err := txqr.EncodeSet(make([]byte, n), txqr.MaxSymbols, qr.M)
		if err != nil {
			return false
		}
		for _, c := range set {
			if c.Size*params.StrokeWidth*2 > usable {
				return false
			}
		}
		return true
	}
	if !fits(ceiling) {
		t.Errorf("the stated ceiling %d does not actually fit", ceiling)
	}
	if fits(ceiling + 1) {
		t.Errorf("%d fits too, so the stated ceiling %d is not the ceiling", ceiling+1, ceiling)
	}
}

// The module label is ONE table. The plan note says "0.6mm modules" and the
// refusal says "0.6mm modules"; two operators comparing them must be reading
// the same units.
func TestTheModuleLabelIsOneTable(t *testing.T) {
	if moduleLabel(3) != "0.9mm" || moduleLabel(2) != "0.6mm" {
		t.Fatalf("module labels: %q %q", moduleLabel(3), moduleLabel(2))
	}
	pl := newPlatform()
	_, _, note, err := planTransactionQRPlates(pl, txCandidate{tx: evenTx(t), confirmed: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(note, moduleLabel(3)) && !strings.Contains(note, moduleLabel(2)) {
		t.Errorf("the plan note uses units the refusal does not: %q", note)
	}
}

// ─── G-P3.13 — the device says DISCARD the plate ────────────────────────────

// §4.4: "legend cut last; incomplete plates discarded; no resume". The device
// said a re-run starts at plate 1 and left the operator to infer the rest --
// and the inference most people make is the wrong one: keep the half-cut blank
// and finish it later. Nothing finishes it. A re-run mints byte-identical
// plates FROM PLATE 1, so a kept partial plate becomes a second, wrong copy of
// plate n/m in the same drawer, and the device has no camera to tell them
// apart afterwards.
func TestStoppingMidSetSaysToDiscardThePlate(t *testing.T) {
	ctx := NewContext(newPlatform())
	ctx.sysw = sessionWith(txEven...)
	cands, _ := payloadTransactions(ctx)
	plates, titles, err := planTransactionTextPlates(ctx.Platform, cands[0])
	if err != nil {
		t.Fatal(err)
	}
	// Drive the engrave loop and BACK out of the first plate's choice screen.
	c2 := NewContext(newPlatform())
	frame, quit := runUI(c2, func() { engraveTransactionPlates(c2, &descriptorTheme, plates, titles) })
	defer quit()
	if _, ok := frame(); !ok {
		t.Fatal("no frame")
	}
	click(&c2.Router, Button1) // Back out of "Engrave this plate"
	got, ok := pumpUntil(frame, "DISCARD", 32)
	if !ok {
		t.Fatalf("the stop screen never says to discard the plate; last frame was %q", got)
	}
	if !uiContains(got, "half cut") {
		t.Errorf("it must say WHY -- nothing will finish it: %q", got)
	}
	if !uiContains(got, "plate 1") {
		t.Errorf("it must say a re-run starts at plate 1: %q", got)
	}
}

// THE REFUSAL IS UNREACHABLE THROUGH THE CONTAINER, and that is worth an
// assertion rather than a shrug.
//
// Measured on this platform: 16 Structured Append symbols at ECC M and 0.6mm
// modules hold 17,968 bytes. The largest `tx:` record a systemwide container
// can carry is (32,734 - len("tx:")) / 2 = 16,365 bytes of transaction,
// because the body is hex. So `me sysw pack` refuses at exit 4 -- naming the
// section cap -- before any payload can reach R16.
//
// R16 stays as defence in depth for the NFC gather and for any future move in
// either bound. What it must not be is a refusal whose message nobody has
// read: this test is why its ceiling is computed rather than written down.
func TestTheQRCeilingIsAboveWhatTheContainerCanDeliver(t *testing.T) {
	pl := newPlatform()
	ceiling := qrCeilingBytes(pl.EngraverParams(), qr.M, 2)
	const maxSection = 32734 // sysw.MaxSectionLen
	deliverable := (maxSection - len("tx:")) / 2
	if ceiling <= deliverable {
		t.Fatalf("the QR ceiling (%d B) is at or below what one container section "+
			"can deliver (%d B), so R16 IS reachable through the payload path and "+
			"the sheet's refusal table needs a row saying so", ceiling, deliverable)
	}
	t.Logf("QR ceiling %d B > container-deliverable %d B: R16 is unreachable "+
		"through the payload path", ceiling, deliverable)
}
