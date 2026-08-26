package gui

import (
	"strings"
	"testing"

	"seedhammer.com/font/sh"
)

// THE tx: RECORD IS THE WHOLE QR PATH. `me tx` produces it, `me sysw pack`
// carries it, and the device is supposed to turn it into QR plates that any
// ordinary scanner reads back into broadcastable hex. It is the reason the
// feature exists.
//
// MEASURED BEFORE THIS FILE: it did not work at all. payloadTransactions built
// the candidate as `txCandidate{tx: tx, src: srcPayload}` and never set
// `confirmed`, whose ZERO VALUE is false. So a perfectly good signed
// transaction reached a review screen reading
//
//	UNCONFIRMED SET
//	Set 00000, 0 string(s).
//	This does NOT reassemble into a transaction. ...
//	The plate legend WILL be replaced with:
//	QR plates are unavailable: there are no transaction bytes.
//
// -- every sentence of it false -- and then transactionReviewAndEngrave found
// no TEXT (strs is nil for a tx: record) and no QR (!confirmed), so
// len(choices) == 0 and it RETURNED SILENTLY. The operator confirmed the
// review and landed back on the carousel with no screen and no explanation.
//
// Sixteen transaction tests passed throughout, because not one of them drove a
// tx: record past payloadTransactions.

// A signed tx: record is a CONFIRMED candidate: the device parsed it, derived
// its txid, and has its bytes.
func TestATxRecordIsAConfirmedCandidate(t *testing.T) {
	ctx := NewContext(newPlatform())
	ctx.sysw = sessionWith("tx:" + rawHexOf(t, evenTx(t)))
	cands, _ := payloadTransactions(ctx)
	if len(cands) != 1 {
		t.Fatalf("got %d candidates", len(cands))
	}
	c := cands[0]
	if !c.confirmed {
		t.Error("a signed tx: record must be confirmed -- the device parsed it and holds its bytes")
	}
	if c.subst != "" {
		t.Errorf("a confirmed candidate must not carry a legend substitution: %q", c.subst)
	}
	if c.tx.TxidDisplay != txEvenTxid {
		t.Errorf("txid %q", c.tx.TxidDisplay)
	}
}

// The review screen for a tx: record must be the CONFIRMED one -- the txid the
// operator compares, the size, the bearer warning -- and must not claim to be
// a set of strings that did not reassemble.
func TestATxRecordGetsTheConfirmedReviewScreen(t *testing.T) {
	ctx := NewContext(newPlatform())
	ctx.sysw = sessionWith("tx:" + rawHexOf(t, evenTx(t)))
	cands, _ := payloadTransactions(ctx)
	joined := strings.Join(transactionReviewLines(cands[0]), "\n")
	for _, forbidden := range []string{
		"UNCONFIRMED SET",
		"Set 00000",
		"0 string(s)",
		"QR plates are unavailable",
	} {
		if strings.Contains(joined, forbidden) {
			t.Errorf("the review screen for a signed transaction says %q:\n%s", forbidden, joined)
		}
	}
	if !strings.Contains(joined, txEvenTxid[:16]) {
		t.Error("must show the txid the operator compares against the host")
	}
	if !strings.Contains(joined, "BEARER") {
		t.Error("must carry the bearer warning")
	}
}

// CAN THE OPERATOR ACTUALLY DO THE THING? The plate kinds offered for a tx:
// record must include QR -- that is the ONLY thing a tx: record can become,
// and an empty choice list is what made the whole path inert.
func TestATxRecordOffersQRPlates(t *testing.T) {
	ctx := NewContext(newPlatform())
	ctx.sysw = sessionWith("tx:" + rawHexOf(t, evenTx(t)))
	cands, _ := payloadTransactions(ctx)
	got := transactionPlateKinds(cands[0])
	if len(got) == 0 {
		t.Fatal("no plate kind offered: the program returns silently and nothing is engraved")
	}
	if !contains(got, "QR PLATES") {
		t.Errorf("QR is the only form a tx: record can take; offered %v", got)
	}
	if contains(got, "TEXT PLATES") {
		t.Errorf("a tx: record carries no mt1 strings, so TEXT cannot be offered: %v", got)
	}
	// ...and the plan really builds.
	plates, _, _, err := planTransactionQRPlates(newPlatform(), cands[0])
	if err != nil || len(plates) == 0 {
		t.Fatalf("the offered choice must produce plates: %v", err)
	}
}

func contains(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}

// ─── the signature predicate, on the tx: class, ON THE DEVICE ────────────────

// The 113-byte SIGNATURE-STRIPPED "even" transaction. Its txid is BYTE-FOR-BYTE
// the honest 222-byte one's, because stripping the witness is precisely the
// operation the txid is defined to ignore. Every other check on the device
// passes it: it is lowercase hex, it parses, sysw.Classify calls it ClassTx.
const txStrippedHex = "02000000017c8da925af70e49a12b0cea7b639df5037c87b7fa61f262b86ac32c47aa3ba1a0000000000fdffffff02404b4c0000000000160014c1de0dd435d1d4ad97ed1f51d63f91c800cc4eab3ea1b92901000000160014751097c299d6354fbb2c5a84512dd708f2902f5e60000000"

// G-P3.1's OTHER HALF. mt.Decode gained the predicate for the CHUNK class, and
// the tx: RECORD class was left with none: sysw.Classify requires only a
// structural parse, so an unsigned tx: record is ClassTx, reaches
// payloadTransactions, and became a candidate with no flag of any kind.
//
// It gets there only through `me sysw pack --allow-unsigned-inputs`, which is
// a deliberate operator act -- so it is not refused. It is FLAGGED, with the
// legend substituted, which is ruling 2026-08-25b applied to the class the
// ruling did not name.
func TestAnUnsignedTxRecordIsFlaggedAndItsLegendReplaced(t *testing.T) {
	ctx := NewContext(newPlatform())
	ctx.sysw = sessionWith("tx:" + txStrippedHex)
	cands, _ := payloadTransactions(ctx)
	if len(cands) != 1 {
		t.Fatalf("got %d candidates -- it must not be DROPPED either", len(cands))
	}
	c := cands[0]
	// The premise, asserted rather than assumed: nothing else can tell.
	if c.tx.TxidDisplay != txEvenTxid {
		t.Fatalf("premise broken: the stripped txid is %q, not the honest one -- "+
			"if they differed, the txid alone would catch this", c.tx.TxidDisplay)
	}
	if c.confirmed {
		t.Error("an unsigned transaction must not be confirmed: it cannot be broadcast")
	}
	if c.subst == "" {
		t.Error("ruling 2026-08-25b: the operator's legend is REPLACED, un-overridably")
	}
	if len(c.unsigned) != 1 || c.unsigned[0] != 0 {
		t.Errorf("must name which input: %v", c.unsigned)
	}
	// AND IT IS STILL ENGRAVEABLE -- the override exists for honest empty
	// inputs, so refusing here would make it useless.
	if !contains(transactionPlateKinds(c), "QR PLATES") {
		t.Error("an unsigned tx: record has all its bytes; QR must still be offered")
	}
}

// The review screen for it is its OWN screen -- not the confirmed one (which
// would vouch for a transaction that cannot be broadcast) and not the
// unconfirmed-SET one (which would say "0 string(s)" about a record).
func TestTheUnsignedReviewScreenSaysWhatIsWrongAndWhichInput(t *testing.T) {
	ctx := NewContext(newPlatform())
	ctx.sysw = sessionWith("tx:" + txStrippedHex)
	cands, _ := payloadTransactions(ctx)
	joined := strings.Join(transactionReviewLines(cands[0]), "\n")
	for _, want := range []string{
		"UNSIGNED TRANSACTION",
		"Input 0",
		"CANNOT be",
		"same one",       // the txid is the same one a signed version would have
		cands[0].subst,   // the legend that will replace the operator's
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("the screen never says %q:\n%s", want, joined)
		}
	}
	if strings.Contains(joined, "0 string(s)") {
		t.Errorf("a tx: record is not a set of strings:\n%s", joined)
	}
	// The txid IS shown -- it is the value the operator compares -- but it must
	// never be shown without the sentence that says it is worth nothing here.
	if !strings.Contains(joined, txEvenTxid[:16]) {
		t.Error("the txid must be shown: it is what the operator compares")
	}
}

// The third legend has to fit BESIDE A QR SYMBOL on plate 1, which the other
// two never had to do -- an unconfirmed SET has no QR at all. A legend that
// does not fit fails the plan and the operator gets an error instead of a
// plate.
func TestTheUnsignedLegendStillFitsAQRPlate(t *testing.T) {
	ctx := NewContext(newPlatform())
	ctx.sysw = sessionWith("tx:" + txStrippedHex)
	cands, _ := payloadTransactions(ctx)
	plates, titles, note, err := planTransactionQRPlates(newPlatform(), cands[0])
	if err != nil {
		t.Fatalf("an unsigned tx: record must still plan QR plates: %v", err)
	}
	if len(plates) == 0 || len(titles) == 0 {
		t.Fatalf("no plates: %s", note)
	}
	if len(legendUnsigned) > 48 {
		t.Errorf("the substituted legend is %d chars; it shares plate 1 with a "+
			"QR symbol and the other two are 47", len(legendUnsigned))
	}
}

// THE PROMISE THE REVIEW SCREEN MAKES MUST BE KEPT IN STEEL. The review says
// "The plate legend WILL be replaced with: <subst>". Before this test, the QR
// legend was built from mt.Tx alone and could not know -- so an unsigned
// transaction was cut under
//
//	txid 2dcf2b97...
//	raw signed bitcoin tx, 113 bytes, 1 qr, ecc H
//	scan, then broadcast
//
// which asserts in permanent steel that a transaction that can NEVER be
// broadcast is signed and ready to be. The device has no camera, so nothing
// can ever read that plate back and correct it.
func TestTheUnsignedQRPlateCarriesTheSubstitutedLegendNotTheSignedOne(t *testing.T) {
	ctx := NewContext(newPlatform())
	ctx.sysw = sessionWith("tx:" + txStrippedHex)
	cands, _ := payloadTransactions(ctx)
	c := cands[0]

	legend := transactionLegend(c, 1, 0, true)
	if !strings.Contains(legend, c.subst) {
		t.Errorf("the engraved legend must carry the substitution:\n%s", legend)
	}
	if strings.Contains(legend, "signed") {
		t.Errorf("the legend claims the transaction is signed:\n%s", legend)
	}
	if strings.Contains(legend, "broadcast") && !strings.Contains(legend, "NOT BROADCASTABLE") {
		t.Errorf("the legend tells the operator to broadcast it:\n%s", legend)
	}
	// The txid still appears: it is how the plate is identified.
	if !strings.Contains(legend, txEvenTxid) {
		t.Errorf("the legend must still carry the txid:\n%s", legend)
	}

	// And a SIGNED one is unaffected -- the substitution must not leak into
	// the ordinary case.
	ctx2 := NewContext(newPlatform())
	ctx2.sysw = sessionWith("tx:" + rawHexOf(t, evenTx(t)))
	ok, _ := payloadTransactions(ctx2)
	good := transactionLegend(ok[0], 1, 0, true)
	if !strings.Contains(good, "raw signed bitcoin tx") || !strings.Contains(good, "then broadcast") {
		t.Errorf("a signed transaction lost its ordinary legend:\n%s", good)
	}
	if strings.Contains(good, "CANNOT") {
		t.Errorf("the warning leaked onto a signed plate:\n%s", good)
	}
}

// Every character of the substituted legend has a glyph in the engraving face.
// A rune with no glyph is not a compile error and not a plan error -- it is a
// gap in steel.
func TestTheSubstitutedLegendsAreEngraveable(t *testing.T) {
	ctx := NewContext(newPlatform())
	ctx.sysw = sessionWith("tx:" + txStrippedHex)
	cands, _ := payloadTransactions(ctx)
	texts := []string{
		transactionLegend(cands[0], 3, 0, true),
		legendUnsigned,
		legendSubstitution(true),
		legendSubstitution(false),
	}
	for _, txt := range texts {
		for _, r := range txt {
			if r == '\n' {
				continue
			}
			if _, _, ok := sh.Font.Decode(r); !ok {
				t.Errorf("rune %q of %q has no glyph in font/sh", r, txt)
			}
		}
	}
}
