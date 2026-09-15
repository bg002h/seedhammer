package hashlock

import (
	"encoding/json"
	"os"
	"seedhammer.com/md"
	"strings"
	"testing"

	qr "github.com/seedhammer/kortschak-qr"
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
			Kind   string `json:"kind"`
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
		kind, ok := md.HashKindFromToken(row.Kind)
		if !ok {
			t.Fatalf("row %s: corpus kind %q is not a §6 token", row.Name, row.Kind)
		}
		if got := QRText(hardened, kind, row.Phrase); got != row.Text {
			t.Errorf("row %s: QRText = %q, want the corpus row %q", row.Name, got, row.Text)
		} else if len(got) != row.Bytes {
			t.Errorf("row %s: QRText is %d bytes, the corpus says %d", row.Name, len(got), row.Bytes)
		}
		// The method line is the corpus row's SECOND line, so MethodLine is
		// pinned by the same rows rather than by a transcription of them.
		// FOUR lines since SPEC_hashlock_kinds §13.1 added the `hash:` line
		// between `method:` and `phrase:`. MethodLine is still the SECOND, so
		// it stays pinned by the rows rather than by a transcription of them.
		lines := strings.Split(row.Text, "\n")
		if len(lines) != 4 {
			t.Fatalf("row %s: the corpus text is %d LF-separated lines, want 4", row.Name, len(lines))
		}
		// ...and the third line names the kind, which is the whole point of
		// the row: a plate read years later says which hash it commits to.
		if got, want := lines[2], "hash: "+row.Kind; got != want {
			t.Errorf("row %s: third line %q, want %q", row.Name, got, want)
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

// TestQRStaysAtFiftyThreeModules pins the invariant SPEC_hashlock_kinds §13.1
// rests on and ACCEPTANCE item 8 (the operator's ~43-minute QR-scan gate)
// depends on: adding the `hash:` line does NOT move the QR version.
//
// §13.1 measured it and nothing pinned it. A version bump would change the
// plate's reserved envelope and silently invalidate a scan gate an operator
// spends most of an hour on, so it is a gate here rather than a note in a
// document.
//
// The worst case is hardened + ripemd160: the longest method line and the
// longest kind token.
func TestQRStaysAtFiftyThreeModules(t *testing.T) {
	worst := strings.Repeat("0", 100) // the phrase cap
	for _, k := range []md.HashKind{
		md.KindSha256, md.KindHash256, md.KindRipemd160, md.KindHash160,
	} {
		txt := QRText(true, k, worst)
		c, err := qr.Encode(txt, qr.L)
		if err != nil {
			t.Fatalf("%s: %v", k.Token(), err)
		}
		if c.Size != 53 {
			t.Errorf("%s: %d bytes encodes to %d modules, want 53 -- the plate's "+
				"reserved envelope and ACCEPTANCE item 8's scan gate assume it",
				k.Token(), len(txt), c.Size)
		}
	}
}
