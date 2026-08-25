package gui

import (
	"os"
	"testing"

	"seedhammer.com/mt"
	"seedhammer.com/sysw"
)

// The cross-language artifact: gui/testdata/sysw_mt_payload.bin was packed by
// the Rust host tool itself —
//
//	me sysw pack --no-passphrase --in records.txt --out sysw_mt_payload.bin
//
// over the pinned "even" vector's six mt1 strings plus its tx: record
// (me-cli @ exp/tx-brief-driven). Every other test in this feature exercises
// one implementation against fixtures; this one exercises the actual seam —
// bytes the host wrote, read by the device code — which is the class of test
// that caught F-212 (two sides computing different ids while 887 single-repo
// tests passed either way).
func TestHostPackedMtPayloadLoadsAndConfirms(t *testing.T) {
	blob, err := os.ReadFile("testdata/sysw_mt_payload.bin")
	if err != nil {
		t.Fatalf("INCONCLUSIVE: the host-packed fixture is unreadable: %v", err)
	}
	h, err := sysw.ParseHeader(blob)
	if err != nil {
		t.Fatalf("the device cannot parse the host's container: %v", err)
	}
	if h.Sealed() {
		t.Fatal("fixture should be the plaintext variant")
	}
	p, err := sysw.Open(blob[:h.TotalLen()], "")
	if err != nil {
		t.Fatalf("the device cannot open the host's container: %v", err)
	}
	if len(p.Public) != 7 {
		t.Fatalf("expected 6 mt1 records + 1 tx: record, got %d", len(p.Public))
	}

	// Through the same session machinery the flow uses.
	s := &syswSession{}
	s.load(p, sysw.Identity(blob[:h.TotalLen()]), false, false, true, true)
	for i, r := range s.records {
		if r.unconfirmed {
			t.Errorf("record %d unconfirmed — the complete set must confirm", i)
		}
	}
	ctx := NewContext(newPlatform())
	ctx.sysw = s
	cands, incomplete := payloadTransactions(ctx)
	if len(cands) != 1 || incomplete != 0 {
		t.Fatalf("got %d candidates, %d incomplete", len(cands), incomplete)
	}
	c := cands[0]
	if c.tx.TxidDisplay != txEvenTxid {
		t.Errorf("txid %s, want %s", c.tx.TxidDisplay, txEvenTxid)
	}
	if len(c.strs) != 6 {
		t.Errorf("the merged candidate carries %d strings, want 6", len(c.strs))
	}

	// ...and both plate kinds actually plan, so the operator holding this
	// payload can reach steel.
	if _, _, err := planTransactionTextPlates(ctx.Platform, c.tx, c.strs); err != nil {
		t.Errorf("text plates: %v", err)
	}
	if _, _, _, err := planTransactionQRPlates(ctx.Platform, c.tx); err != nil {
		t.Errorf("qr plates: %v", err)
	}

	// The decoded bytes are the broadcastable transaction, byte for byte.
	tx, err := mt.Decode(c.strs)
	if err != nil {
		t.Fatal(err)
	}
	if tx.TxidDisplay != txEvenTxid {
		t.Error("re-decode diverged")
	}
}
