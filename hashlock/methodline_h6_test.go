package hashlock

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// TestH6MethodLineAndQRTextMatchTheMSCorpus pins MethodLine and QRText against
// the VENDORED `qr_text` rows -- H6 §8.6 rule 4a's "DOWNSTREAM of the Rust
// primary and pinned against the vendored qr_text rows (§11.2), NOT against
// themselves".
//
// A LITERAL IN THIS FILE IS NOT A PIN, and the first version of this test was
// one. R0 round 0 (fidelity I-4) ran the event the pin exists for: reorder
// `salt=` and `iterations=` in the Rust primary with no change to any
// derivation constant -- the same 73 characters -- and re-vendor the corpus and
// its sha256. `go test ./hashlock/` stayed GREEN against a corpus it now
// disagreed with; the only failure was in `backup`, which named ITS OWN
// test-local literal, and updating that one literal turned the whole tree green
// while every phrase plate cut from that firmware carried a method line and a
// QR text that disagreed with `ms hashlock`. The corpus is the primary's own
// output, so it is what these two functions are compared to.
//
// MUTATION: reorder `salt=` and `iterations=` in MethodLine and re-vendor ->
// every hardened row fails here, with the corpus text beside the built one.
// MUTATION: swap MethodLine's two arms -> both classes fail on the method line.
func TestH6MethodLineAndQRTextMatchTheMSCorpus(t *testing.T) {
	raw, err := os.ReadFile("testdata/hashlock-v0.8.json")
	if err != nil {
		t.Fatalf("INCONCLUSIVE: no vendored hashlock corpus: %v", err)
	}
	var corpus struct {
		QRText []struct {
			Name   string `json:"name"`
			Method string `json:"method"`
			Phrase string `json:"phrase"`
			Text   string `json:"qr_text"`
			Bytes  int    `json:"bytes"`
		} `json:"qr_text"`
	}
	if err := json.Unmarshal(raw, &corpus); err != nil {
		t.Fatalf("parsing the vendored corpus: %v", err)
	}
	if len(corpus.QRText) < 7 {
		t.Fatalf("the corpus carries %d qr_text rows; H6 §11.2 pins seven", len(corpus.QRText))
	}
	seen := map[string]int{}
	for _, row := range corpus.QRText {
		hardened := row.Method == "hardened"
		seen[row.Method]++
		if got := QRText(hardened, row.Phrase); got != row.Text {
			t.Errorf("row %s: QRText = %q, want the corpus row %q", row.Name, got, row.Text)
		} else if len(got) != row.Bytes {
			t.Errorf("row %s: QRText is %d bytes, the corpus says %d", row.Name, len(got), row.Bytes)
		}
		// The method line is the corpus row's SECOND line, so MethodLine is
		// pinned by the same rows rather than by a transcription of them.
		lines := strings.Split(row.Text, "\n")
		if len(lines) != 3 {
			t.Fatalf("row %s: the corpus text is %d LF-separated lines, want 3", row.Name, len(lines))
		}
		if got := MethodLine(hardened); got != lines[1] {
			t.Errorf("row %s: MethodLine(%v) = %q, want the corpus line %q",
				row.Name, hardened, got, lines[1])
		}
	}
	// NON-VACUITY: a corpus that lost every row of one method would otherwise
	// pin only the other one, and MethodLine has exactly two arms.
	for _, m := range []string{"hardened", "sha256"} {
		if seen[m] == 0 {
			t.Errorf("the corpus carries no %s row, so that arm is pinned by nothing", m)
		}
	}
	// §6.5's worst case rests on this length: a 79-character method line puts
	// the QR'd phrase plate over the 416000-unit budget at 11 rows.
	if n := len(MethodLine(true)); n != 73 {
		t.Errorf("the hardened method line is %d characters; H6 §6.5 pins the plate's worst case on 73", n)
	}
}
