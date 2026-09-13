package main

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// vectorChunks reads the md1 strings out of a vendored corpus vector.
func vectorChunks(t *testing.T, name string) []string {
	t.Helper()
	path := filepath.Join("..", "..", "md", "testdata", "vectors", name+".phrase.txt")
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer f.Close()
	var out []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.ReplaceAll(strings.TrimSpace(sc.Text()), " ", "")
		if strings.HasPrefix(line, "md1") {
			out = append(out, line)
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if len(out) == 0 {
		t.Fatalf("%s carries no md1 strings", path)
	}
	return out
}

// TestSingleStringCardIsNotACodecFailure pins the second repair the corpus smoke
// test forced.
//
// md.Reassemble takes a CHUNK SET. Handed one md1 string it reads the first four
// bits as a wire version, finds something else, and reports "md: wire version
// mismatch" -- a true statement about a header the card never had, and a
// thoroughly false impression of the card, which the device reads without
// complaint. Thirteen of the 67 vendored vectors are single-string cards, and
// every one was being counted as a CODEC failure by a harness whose entire
// purpose is to report what the device does.
//
// The distinction this test protects is between "the device cannot read this
// card" and "the device reads this card and has nothing to derive from it".
// Collapsing those two is how a harness invents a decoder bug.
//
// MUTATION: delete the singleStringTemplate fallback in probe() and the first
// subtest fails with stage "expand" and the wire-version sentence.
func TestSingleStringCardIsNotACodecFailure(t *testing.T) {
	t.Run("keyless single-string card is a source refusal", func(t *testing.T) {
		chunks := vectorChunks(t, "pkh_basic")
		if len(chunks) != 1 {
			t.Fatalf("pkh_basic is no longer a single-string card (%d chunks): pick another fixture", len(chunks))
		}
		res := probe(caseIn{ID: "t", Chunks: chunks, Indices: []uint32{0}})
		if res.OK {
			t.Fatal("a keyless template claimed derived addresses")
		}
		if res.Stage != stageSource {
			t.Fatalf("stage = %q, want %q (error: %s)", res.Stage, stageSource, res.Error)
		}
		if strings.Contains(res.Error, "wire version") {
			t.Fatalf("a readable card reported as a codec failure: %s", res.Error)
		}
		if res.Keys == nil || *res.Keys != 0 {
			t.Fatalf("keys = %v, want 0 (a keyless template seats none)", res.Keys)
		}
	})

	t.Run("an unreadable single-string card keeps its own error", func(t *testing.T) {
		// sh_wpkh is refused by md.Decode as errMissingExplicitOrigin, a
		// documented refusal (md/testdata_test.go:35). The chunk-set error must
		// not stand in for it, or a reader goes hunting for a codec bug.
		res := probe(caseIn{ID: "t", Chunks: vectorChunks(t, "sh_wpkh"), Indices: []uint32{0}})
		if res.OK {
			t.Fatal("sh_wpkh derived addresses: fixture no longer refused")
		}
		if res.Stage != stageExpand {
			t.Fatalf("stage = %q, want %q", res.Stage, stageExpand)
		}
		if !strings.Contains(res.Error, "explicit origin") {
			t.Fatalf("error = %q, want the origin refusal, not the chunk-set one", res.Error)
		}
	})

	t.Run("a broken chunk SET keeps the chunk-set error", func(t *testing.T) {
		// The fallback must never re-label a genuine multi-chunk failure: "the
		// operator is missing a card" is the finding, not a single-card error
		// about whichever chunk happened to be first.
		full := vectorChunks(t, "keyed_compose_wsh_timelock_hashlock")
		if len(full) < 3 {
			t.Fatalf("fixture has %d chunks, need a real set", len(full))
		}
		res := probe(caseIn{ID: "t", Chunks: full[:2], Indices: []uint32{0}})
		if res.OK {
			t.Fatal("a truncated chunk set derived addresses")
		}
		if res.Stage != stageExpand {
			t.Fatalf("stage = %q, want %q (error: %s)", res.Stage, stageExpand, res.Error)
		}
		t.Logf("truncated set: %s", res.Error)
	})
}
