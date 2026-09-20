package gui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ─── The vector fixtures boundary (F-630 D5a/D5b) ────────────────────────────
//
// THE RULE, and it is about PAIRS: a conformance RECORD and the md1 CARD it is
// compared against must come from the same tier. Both loaders below prefer the
// fork-side pin in md/testdata/forkbuilt/, so using them together keeps a test
// on one policy; taking the card from loadVectorChunks and the record from a
// plain os.ReadFile of md/testdata/vectors/ pairs a PINNED policy with a
// RE-VENDORED one and compares one against the other.
//
// That is not hypothetical. Three sites in this package did exactly it, and all
// three red once the F-630 re-vendor lands, because descriptor-mnemonic
// b2c5d693 replaced the three reuse-bearing F-529 policies with reuse-free ones:
//
//	gui/policy_address_test.go:125        all three F-529 vectors
//	gui/taproot_script_path_test.go:30    keyed_tr_multi_a, keyed_tr_sortedmulti_a
//	gui/wsh_script_emit_test.go:31        keyed_wsh_timelock_hashlock
//
//	keyed_tr_multi_a chain 0 index 0:
//	  go:   bc1pf4aujydl48hah9qxvk4j0dcce737pl9svne7rmzcprrh7y92znsstul4rt
//	  rust: bc1pgrupj0fjv79xtj05mzthds4dqzvdhcptx2gzpgzt90uzc86qzfes2a0yhh
//
// THREE OCCURRENCES OF ONE SHAPE IS A WRONG SHAPE, so the fix is this boundary
// rather than a fourth call-site edit. There is no scanner enforcing it —
// three drafts of one were each found defective and it changed zero verdicts
// against ten corpus mutations — and it does not need one: a fourth site
// pairing a pinned card with a vendored record reds loudly on all three pinned
// vectors, which is how these three were found.

// loadVectorRecord reads a vector's `.conformance.json`, PREFERRING the
// fork-side pin in md/testdata/forkbuilt/.
//
// Returns raw bytes rather than a decoded type on purpose: the call sites
// unmarshal into four different anonymous shapes, and a shared type would
// rewrite every one of them for no gain.
func loadVectorRecord(t *testing.T, name string) []byte {
	t.Helper()
	if raw, err := os.ReadFile(filepath.Join("..", "md", "testdata", "forkbuilt", name+".conformance.json")); err == nil {
		return raw
	}
	path := filepath.Join("..", "md", "testdata", "vectors", name+".conformance.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return raw
}

// loadVectorChunks loads a corpus vector's md1 card, PREFERRING a fork-side pin
// in md/testdata/forkbuilt/.
//
// Same reason as md's vectorChunksFor: the pinned vectors are the only fork-side
// witnesses F-533's refusal and F-514's duplicate warning have, and the primary
// has since replaced those policies. md/f533_pinned_vectors_test.go asserts each
// pin is either byte-identical to the vendored card or that the vendored card is
// exactly the policy the primary is recorded as having moved to.
func loadVectorChunks(t *testing.T, name string) []string {
	t.Helper()
	path := filepath.Join("..", "md", "testdata", "forkbuilt", name+".md1.txt")
	raw, err := os.ReadFile(path)
	if err != nil {
		path = filepath.Join("..", "md", "testdata", "vectors", name+".phrase.txt")
		raw, err = os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
	}
	var out []string
	for _, l := range strings.Split(string(raw), "\n") {
		l = strings.ReplaceAll(strings.TrimSpace(l), " ", "")
		if strings.HasPrefix(l, "md1") {
			out = append(out, l)
		}
	}
	return out
}

// eachKeyedVector returns the vendored keyed vector names matching pattern
// (e.g. "keyed_*", "keyed_tr_*"), sorted. The glob is over the VENDORED
// directory because that is what defines corpus membership; which tier each
// record is then read from is loadVectorRecord's business.
func eachKeyedVector(t *testing.T, pattern string) []string {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join("..", "md", "testdata", "vectors", pattern+".conformance.json"))
	if err != nil {
		t.Fatalf("glob %s: %v", pattern, err)
	}
	if len(paths) == 0 {
		t.Fatalf("no %s.conformance.json vendored — a gate over them is checking NOTHING", pattern)
	}
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		out = append(out, strings.TrimSuffix(filepath.Base(p), ".conformance.json"))
	}
	return out
}
