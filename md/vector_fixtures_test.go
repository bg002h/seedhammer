package md

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// ─── The vector fixtures boundary (F-630 D5a/D5b) ────────────────────────────
//
// THE RULE, and it is about PAIRS: a conformance RECORD and the md1 CARD it is
// compared against must come from the same tier. Two loaders exist —
// vectorChunksFor and vectorRecordFor prefer md/testdata/forkbuilt/, while
// loadPhraseChunks reads the vendored corpus only — and mixing them across one
// test pairs a PINNED artifact with a RE-VENDORED one, which is two different
// policies compared against each other.
//
// Both halves of that mistake are real and both were measured at the F-630
// T2+T3 state, in opposite directions:
//
//	pinned CARD + vendored RECORD   gui/policy_address_test.go,
//	                                gui/taproot_script_path_test.go,
//	                                gui/wsh_script_emit_test.go
//	                                -> addresses differ, e.g. keyed_tr_multi_a
//	                                   chain 0 index 0 go bc1pf4auj… vs rust
//	                                   bc1pgrupj…
//	pinned RECORD + vendored CARD   md/conformance_keyed_test.go
//	                                -> ids differ, keyed_tr_multi_a
//	                                   wallet_policy_id go fe4d264c… (re-vendored
//	                                   card) vs rust 1f26f9b7… (pinned record)
//
// So: A FILE THAT READS A RECORD TAKES ITS CARD FROM THE PIN-PREFERRING LOADER
// TOO. There is no scanner enforcing this — three drafts of one were each found
// defective and it changed zero verdicts against ten corpus mutations — and it
// does not need one: the hazard is LOUD, which is how all four sites above were
// found. What this file buys is that the pairing is made in ONE place rather
// than at seven call sites.

// vectorRecordFor reads a vector's `.conformance.json`, PREFERRING the
// fork-side pin in testdata/forkbuilt/.
//
// Returns raw bytes rather than a decoded type on purpose: the call sites
// unmarshal into four different anonymous shapes, and a shared type would
// rewrite every one of them for no gain.
func vectorRecordFor(t *testing.T, name string) []byte {
	t.Helper()
	if raw, err := os.ReadFile(filepath.Join("testdata", "forkbuilt", name+".conformance.json")); err == nil {
		return raw
	}
	raw, err := os.ReadFile(vectorPath(name, "conformance.json"))
	if err != nil {
		t.Fatalf("read %s.conformance.json: %v", name, err)
	}
	return raw
}

// eachKeyedVector returns the vendored keyed vector names matching pattern
// (e.g. "keyed_*", "keyed_tr_*"), sorted. The glob is over the VENDORED
// directory because that is what defines corpus membership; which tier each
// record is then read from is vectorRecordFor's business.
// eachKeyedVector enumerates the UNION of the vendored tier and the fork-side
// record pins.
//
// THE UNION IS THE WHOLE POINT, and globbing only `testdata/vectors/` was a
// defect the whole-diff review reproduced. A pin exists for the day F-529
// describes -- the day a re-vendor DELETES the vector it preserves -- and a
// vendored-only glob examines the pin only while the thing it replaces is
// still there. Delete `testdata/vectors/keyed_tr_multi_a.*` and the gate
// passed at "45 of 45 ... 0 fail" with the pin untouched on disk and still
// named in forkbuiltRecordPins: coverage that evaporates exactly when it is
// first needed.
func eachKeyedVector(t *testing.T, pattern string) []string {
	t.Helper()
	seen := map[string]bool{}
	for _, dir := range []string{"vectors", "forkbuilt"} {
		paths, err := filepath.Glob(filepath.Join("testdata", dir, pattern+".conformance.json"))
		if err != nil {
			t.Fatalf("glob %s in %s: %v", pattern, dir, err)
		}
		for _, p := range paths {
			seen[strings.TrimSuffix(filepath.Base(p), ".conformance.json")] = true
		}
	}
	if len(seen) == 0 {
		t.Fatalf("no %s.conformance.json in either tier — a gate over them is checking NOTHING", pattern)
	}
	out := make([]string, 0, len(seen))
	for n := range seen {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// vectorChunksFor loads a corpus vector, PREFERRING a fork-side pin.
//
// The pin comes first because F-529 says a re-vendor would delete the three
// key-reuse vectors, and two of them are the only witnesses F-533's refusal
// has (f533_pinned_vectors_test.go). While both copies exist they are asserted
// byte-identical there, so reading the pin changes nothing today and keeps
// every caller working the day the vendored file goes.
func vectorChunksFor(t *testing.T, name string) []string {
	t.Helper()
	if pin, ok := pinnedChunks(name); ok {
		if len(pin) == 0 {
			t.Fatalf("the pin for %s carries no md1 strings", name)
		}
		return pin
	}
	raw, err := os.ReadFile(filepath.Join("testdata", "vectors", name+".phrase.txt"))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	out := md1Lines(string(raw))
	if len(out) == 0 {
		t.Fatalf("%s carries no md1 strings", name)
	}
	return out
}

// loadPhraseChunks reads a VENDORED `.phrase.txt` and only a vendored one,
// dropping the `chunk-set-id:` header a chunked card carries and stripping the
// display separators an operator's re-typed card would not have.
//
// It is the loader a file that reads a conformance record MUST NOT use: see
// the pairing rule at the top of this file. It remains correct for tests that
// read no record and want the primary's current card regardless of any pin
// (md/compose_shape_test.go, md/compose_test.go's chunk-parity golden).
func loadPhraseChunks(t *testing.T, name string) []string {
	t.Helper()
	raw, err := os.ReadFile(vectorPath(name, "phrase.txt"))
	if err != nil {
		t.Fatalf("read %s.phrase.txt: %v", name, err)
	}
	var out []string
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.ReplaceAll(strings.TrimSpace(line), " ", "")
		if strings.HasPrefix(line, "md1") {
			out = append(out, line)
		}
	}
	return out
}
