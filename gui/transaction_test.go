package gui

import (
	"strings"
	"testing"

	qr "github.com/seedhammer/kortschak-qr"
	"seedhammer.com/font/sh"
	"seedhammer.com/mt"
	"seedhammer.com/sysw"
)

// The pinned "even" vector (mt-codec corpus): a real signed 222-byte tx,
// 6 chunks, set 0x2dcf2, txid 2dcf2b97… — the same fixture the sysw and mt
// packages pin, so every layer is exercised on one artifact.
var txEven = []string{
	"mt1p9h8jqq9qqqqgqqqqqqqyqherdfykhhpey6z2cvafak8804qd7g0dl6v8ex9wr2cvky023skwkeud2229sax",
	"mt1p9h8jqq9qqphgdqqqqqqqq0mllllupyqj6vqqqqqqqqzcqpfsw7ph2rt5w54kt768636cls8zxg0najlzunp",
	"mt1p9h8jqq9qqzj8yqpnzw4vl2rwffqyqqqqqkqq282yyhc2vavd20hvk94pz39hts3u5s9a0qd8pwskxfl7ju5",
	"mt1p9h8jqq9qqrqfrnq3qzyp77h37cnxzvwutegzmzy5zrrrfvrpykdfsckvk03dcq6rcjtvlsfcglv7zx43yaz",
	"mt1p9h8jqq9qqylgpzqmhcwhuupdvnrc82rncvzzdahpgjsdwgu52jd7vmxsve9x3w5ujeqyssuvddxvwqze4ve",
	"mt1p9h8jqq9qq9qdcc7h75twfxyf340c4sgqzhfdq6xtgt7zhxngpwa049l0z59l6jqcqqqqqq5k5y2ye5nv8yf",
}

const txEvenTxid = "2dcf2b973d52044b1e58c988a5a59d388073ff05598b0a1e93eeb04c72ebf630"

func evenTx(t *testing.T) mt.Tx {
	t.Helper()
	tx, err := mt.Decode(txEven)
	if err != nil {
		t.Fatal(err)
	}
	return tx
}

// A session holding the given public records, loaded and compared, the way
// syswLoadFlow builds one — bypassing only the screens.
func sessionWith(records ...string) *syswSession {
	s := &syswSession{}
	s.load(&sysw.Payload{Public: records}, [32]byte{1}, false, false, true, true)
	return s
}

// THE BRIEF'S OWN REQUIREMENT, measured: as many mt1 strings per plate as
// fit, so the total plate count is minimal. Six 87-char strings at 3.0mm must
// share plates — anything close to one-string-per-plate means the packing is
// not happening.
func TestTextPlatesPackMultipleStringsPerPlate(t *testing.T) {
	pl := newPlatform()
	plates, titles, err := planTransactionTextPlates(pl, txCandidate{tx: evenTx(t), strs: txEven, confirmed: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(plates) != len(titles) {
		t.Fatalf("%d plates, %d titles", len(plates), len(titles))
	}
	if len(plates) >= len(txEven) {
		t.Errorf("6 strings took %d plates — nothing was packed", len(plates))
	}
	if len(plates) != 1 {
		// 6 strings x (2-3 rows + gap) at 3.0mm against ~26 rows: one plate.
		// If the layout ever legitimately changes this may become 2; what it
		// must never be is 6.
		t.Logf("note: packing produced %d plates for 6 strings", len(plates))
	}
	for i, title := range titles {
		if !strings.Contains(title, "TX 2DCF2B97") {
			t.Errorf("plate %d title %q does not name the transaction", i, title)
		}
	}
}

// Packing must not reorder: chunk i's string precedes chunk i+1's, whatever
// order the payload delivered them in.
func TestTextPlatesKeepIndexOrder(t *testing.T) {
	shuffled := []string{txEven[3], txEven[0], txEven[5], txEven[2], txEven[4], txEven[1]}
	got := orderByIndex(shuffled)
	for i, s := range got {
		if s != txEven[i] {
			t.Fatalf("position %d holds the wrong string", i)
		}
	}
}

// The QR plan follows the measured objective: this 222-byte transaction fits
// ONE plate as ONE symbol, and the leftover capacity is spent on ECC ABOVE
// the M floor.
func TestQRPlanSmallTransactionIsOnePlateAboveTheFloor(t *testing.T) {
	pl := newPlatform()
	plates, titles, note, err := planTransactionQRPlates(pl, evenTx(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(plates) != 1 {
		t.Fatalf("222 B took %d QR plates, want 1: %s", len(plates), note)
	}
	if titles[0] != "TX 2DCF2B97 1/1" {
		t.Errorf("title %q", titles[0])
	}
	// One symbol, and the leftover capacity spent ABOVE the M floor — the
	// findings' step 3. 222 B in one v13-ish symbol reaches Q or H.
	if !strings.Contains(note, "1 QR") {
		t.Errorf("note %q", note)
	}
	if strings.Contains(note, "ECC M") || strings.Contains(note, "ECC L") {
		t.Errorf("222 B should reach above the ECC floor, got %q", note)
	}
}

// The legend states what the findings say an operator must be told: what the
// QR carries, and — for a multi-symbol set — that scan order is irrelevant.
func TestTransactionLegendNamesTheFacts(t *testing.T) {
	tx := evenTx(t)
	one := transactionLegend(tx, 1, qr.M)
	if !strings.Contains(one, "txid "+txEvenTxid) {
		t.Error("legend must carry the full txid")
	}
	if !strings.Contains(one, "raw signed bitcoin tx") {
		t.Error("legend must say what the QR carries — mt1/codex32 wording would be wrong here")
	}
	many := transactionLegend(tx, 3, qr.M)
	if !strings.Contains(many, "any order") {
		t.Error("a multi-symbol legend must say scan order is irrelevant")
	}
}

// Payload candidates: a complete set confirms; adding its tx: twin does not
// duplicate the candidate; an incomplete set is counted, not offered.
func TestPayloadTransactionsConfirmsAndMerges(t *testing.T) {
	ctx := NewContext(newPlatform())
	ctx.sysw = sessionWith(txEven...)
	cands, incomplete := payloadTransactions(ctx)
	if len(cands) != 1 || incomplete != 0 {
		t.Fatalf("got %d candidates, %d incomplete", len(cands), incomplete)
	}
	if cands[0].tx.TxidDisplay != txEvenTxid || len(cands[0].strs) != 6 {
		t.Errorf("candidate %+v", cands[0])
	}
	if cands[0].src != srcPayload {
		t.Error("source must be the payload (F3 names it on the review screen)")
	}

	// The same transaction as a tx: record too: merged, still one candidate.
	rec := "tx:" + rawHexOf(t, cands[0].tx)
	ctx.sysw = sessionWith(append(append([]string{}, txEven...), rec)...)
	cands, _ = payloadTransactions(ctx)
	if len(cands) != 1 {
		t.Fatalf("tx: twin duplicated the candidate: %d", len(cands))
	}
	if len(cands[0].strs) != 6 {
		t.Error("the merged candidate lost its text strings")
	}

	// A tx: record ALONE is a candidate with no text strings.
	ctx.sysw = sessionWith(rec)
	cands, _ = payloadTransactions(ctx)
	if len(cands) != 1 || cands[0].strs != nil {
		t.Fatalf("tx: alone: %+v", cands)
	}

	// RULING 2026-08-25a: an incomplete set IS offered -- reported loudly and
	// engraveable -- not dropped. This assertion previously required 0
	// candidates, which was the pre-ruling behaviour: the device did `continue`
	// and the operator lost a signing ceremony to a set missing one string.
	ctx.sysw = sessionWith(txEven[:4]...)
	cands, incomplete = payloadTransactions(ctx)
	if len(cands) != 1 || incomplete != 4 {
		t.Fatalf("incomplete set: %d candidates, %d counted", len(cands), incomplete)
	}
	if cands[0].confirmed {
		t.Error("an incomplete set must not be marked confirmed")
	}
	if cands[0].subst == "" {
		t.Error("an unconfirmed candidate must carry the legend that replaces the operator's")
	}
	if len(cands[0].strs) != 4 {
		t.Errorf("the four strings the operator HAS must still be engraveable, got %d", len(cands[0].strs))
	}
}

func rawHexOf(t *testing.T, tx mt.Tx) string {
	t.Helper()
	const hexdigits = "0123456789abcdef"
	var b strings.Builder
	for _, x := range tx.Raw {
		b.WriteByte(hexdigits[x>>4])
		b.WriteByte(hexdigits[x&0xf])
	}
	return b.String()
}

// The session marks every record of an incomplete mt set unconfirmed — the
// flag path that makes F1 fire on it in a plaintext payload.
func TestSessionMarksIncompleteMtSetsUnconfirmed(t *testing.T) {
	s := sessionWith(txEven[:2]...)
	for i, r := range s.records {
		if r.class != sysw.ClassMt {
			t.Fatalf("record %d classified %v", i, r.class)
		}
		if !r.unconfirmed {
			t.Errorf("record %d of an incomplete set is not marked unconfirmed", i)
		}
	}
	s = sessionWith(txEven...)
	for i, r := range s.records {
		if r.unconfirmed {
			t.Errorf("record %d of the complete set is marked unconfirmed", i)
		}
	}
}

// The review screen shows the FULL txid — the value the host printed and the
// only thing the operator can compare — plus the bearer warning.
func TestTransactionReviewLines(t *testing.T) {
	// confirmed: true is REQUIRED -- the zero value is UNCONFIRMED, so a
	// candidate that nothing confirmed fails closed (rulings 2026-08-25).
	c := txCandidate{tx: evenTx(t), strs: txEven, src: srcPayload, confirmed: true}
	joined := strings.Join(transactionReviewLines(c), "\n")
	if !strings.Contains(joined, txEvenTxid[:16]) ||
		!strings.Contains(joined, txEvenTxid[48:]) {
		t.Error("review must carry the whole txid")
	}
	if !strings.Contains(joined, "222 bytes") {
		t.Error("review must carry the size")
	}
	if !strings.Contains(joined, "BEARER") {
		t.Error("review must carry the bearer warning")
	}
}

// Every character the transaction plates engrave must have a glyph in the
// engraving face: titles, legend, and the mt1 charset itself.
func TestTransactionPlateTextIsEngraveable(t *testing.T) {
	tx := evenTx(t)
	texts := []string{
		transactionPlateTitle(tx, 16, 16),
		transactionLegend(tx, 3, qr.M),
		transactionLegend(tx, 1, qr.H),
		txEven[0],
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

// syntheticTx builds a parse-valid legacy transaction of roughly n bytes, so
// the QR planner can be exercised at sizes the pinned vector does not reach.
func syntheticTx(t *testing.T, scriptLen int) mt.Tx {
	t.Helper()
	b := []byte{0x01, 0x00, 0x00, 0x00, 0x01}
	b = append(b, make([]byte, 36)...) // outpoint
	// scriptSig, varint length
	switch {
	case scriptLen < 0xFD:
		b = append(b, byte(scriptLen))
	default:
		b = append(b, 0xFD, byte(scriptLen), byte(scriptLen>>8))
	}
	b = append(b, make([]byte, scriptLen)...)
	b = append(b, 0xFF, 0xFF, 0xFF, 0xFF) // sequence
	b = append(b, 0x01)                   // one output
	b = append(b, make([]byte, 8)...)     // value
	b = append(b, 0x02, 0x51, 0x51)       // script
	b = append(b, make([]byte, 4)...)     // locktime
	tx, err := mt.ParseTx(b)
	if err != nil {
		t.Fatal(err)
	}
	return tx
}

// The planner at the sizes the findings measured: a mid-size transaction
// takes a small number of plates, symbols never exceed the Structured Append
// cap, and the count grows with size rather than jumping to one-per-40-bytes.
func TestQRPlanScalesWithTransactionSize(t *testing.T) {
	pl := newPlatform()
	for _, tc := range []struct {
		bytes     int
		maxPlates int
	}{
		{600, 2},  // near the one-plate boundary at 0.6mm with a legend
		{1700, 4}, // the findings' 2in/2out neighbourhood
		{4000, 7}, // the largest a tx: record can deliver (section cap)
	} {
		tx := syntheticTx(t, tc.bytes-64)
		plates, _, note, err := planTransactionQRPlates(pl, tx)
		if err != nil {
			t.Fatalf("%d B: %v", tc.bytes, err)
		}
		if len(plates) > tc.maxPlates {
			t.Errorf("%d B took %d plates (%s), want <= %d",
				tc.bytes, len(plates), note, tc.maxPlates)
		}
		if strings.Contains(note, "ECC L") {
			t.Errorf("%d B: below the ECC floor: %s", tc.bytes, note)
		}
	}
}

// RULING 2026-08-25: an unconfirmed set is ENGRAVEABLE, and the operator's
// legend is replaced un-overridably. Before this, the device DROPPED such a set
// -- the payload path did `continue` and the NFC path said "Dropped."
func TestUnconfirmedSetIsEngraveableWithASubstitutedLegend(t *testing.T) {
	c := txCandidate{
		strs: txEven[:3], src: srcPayload, csid: 0x2dcf2,
		confirmed: false, subst: legendSubstitution(false),
	}
	lines := strings.Join(transactionReviewLines(c), "\n")
	if !strings.Contains(lines, "UNCONFIRMED SET") {
		t.Error("the review screen must say the set is unconfirmed")
	}
	if !strings.Contains(lines, c.subst) {
		t.Error("the review screen must show the legend that will REPLACE the operator's")
	}
	if !strings.Contains(lines, "QR plates are unavailable") {
		t.Error("QR needs transaction bytes an unconfirmed set does not have")
	}

	pl := newPlatform()
	plates, _, err := planTransactionTextPlates(pl, c)
	if err != nil {
		t.Fatalf("an unconfirmed set must still ENGRAVE: %v", err)
	}
	if len(plates) == 0 {
		t.Fatal("no plates produced for an unconfirmed set")
	}
}
