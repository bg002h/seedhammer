package md

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The compose corpus is VENDORED from the Rust primary (descriptor-mnemonic,
// crates/md-codec/tests/vectors/, the MANIFEST's `compose_*` and
// `keyed_compose_*` entries) and pinned here, the same way sysw/testdata/
// sysw_vectors.json is: no sibling checkout, no network, no skip path. A copy
// with no pin is a file nobody can date.
const composeVectorProvenance = "testdata/compose_vectors.provenance.json"

type composeVectorPin struct {
	Comment []string `json:"_comment"`
	Repo    string   `json:"repo"`
	Remote  string   `json:"remote"`
	Commit  string   `json:"commit"`
	Path    string   `json:"path"`
	Files   []struct {
		Name   string `json:"name"`
		SHA256 string `json:"sha256"`
	} `json:"files"`
	Vectors    int    `json:"vectors"`
	RecordedAt string `json:"recorded_at"`
}

// composeVectorNames is the primary's compose corpus at the pinned commit.
// Hand-maintained like singleStringVectorNames, and checked both against the
// pin and against the DIRECTORY below, so a file copied in without a name
// here, a name with no file, or a compose-named file the pin does not list
// all fail rather than silently asserting nothing.
var composeVectorNames = []string{
	"compose_tr_seven_leaves", "compose_tr_thirty_two_slots",
	"compose_wsh_eight_paths", "compose_wsh_thirty_two_slots",
	"keyed_compose_sh_sole", "keyed_compose_sh_two_of_four",
	"keyed_compose_sh_wsh_one_of_two", "keyed_compose_sh_wsh_sole",
	"keyed_compose_tr_extracted_first", "keyed_compose_tr_extracted_later_four_paths",
	"keyed_compose_tr_hash_leaf", "keyed_compose_tr_key_path_only",
	"keyed_compose_tr_nums_three_leaves", "keyed_compose_tr_sole_sortedmulti_a",
	"keyed_compose_tr_three_paths_extracted_later", "keyed_compose_tr_two_path_distinct_fingerprints",
	"keyed_compose_tr_two_path_nums", "keyed_compose_tr_unsorted_sole_leaf",
	"keyed_compose_wsh_hash_and_time", "keyed_compose_wsh_locked_head_or_i",
	"keyed_compose_wsh_single_head_or_i", "keyed_compose_wsh_sole_sortedmulti",
	"keyed_compose_wsh_three_paths", "keyed_compose_wsh_two_path_distinct_fingerprints",
	"keyed_compose_wsh_two_path_or_d", "keyed_compose_wsh_unsorted_sole",
}

// isComposeVectorFile: the corpus's file names, and nothing else in the
// shared vectors directory (the MANIFEST's other vectors live beside them).
func isComposeVectorFile(name string) bool {
	return strings.HasPrefix(name, "compose_") || strings.HasPrefix(name, "keyed_compose_")
}

func loadComposeVectorPin(t *testing.T) composeVectorPin {
	t.Helper()
	raw, err := os.ReadFile(composeVectorProvenance)
	if err != nil {
		t.Fatalf("INCONCLUSIVE: no provenance pin at %s: %v", composeVectorProvenance, err)
	}
	var p composeVectorPin
	if err := json.Unmarshal(raw, &p); err != nil {
		t.Fatalf("parsing %s: %v", composeVectorProvenance, err)
	}
	if strings.TrimSpace(p.Commit) == "" || strings.TrimSpace(p.Path) == "" {
		t.Fatalf("INCONCLUSIVE: %s names no primary commit and path", composeVectorProvenance)
	}
	return p
}

func TestComposeVectorsMatchTheirProvenancePin(t *testing.T) {
	p := loadComposeVectorPin(t)
	if p.Vectors != len(composeVectorNames) {
		t.Fatalf("pin says %d vectors, this test knows %d", p.Vectors, len(composeVectorNames))
	}
	// 22 keyed vectors carry five files, 4 unkeyed carry four: 126.
	if len(p.Files) != 126 {
		t.Fatalf("pin lists %d files, want 126", len(p.Files))
	}
	pinned := map[string]bool{}
	seen := map[string]bool{}
	for _, f := range p.Files {
		raw, err := os.ReadFile(filepath.Join("testdata", "vectors", f.Name))
		if err != nil {
			t.Fatalf("pinned file missing: %v", err)
		}
		sum := sha256.Sum256(raw)
		if got := hex.EncodeToString(sum[:]); got != f.SHA256 {
			t.Errorf("%s: sha256 %s, pin says %s", f.Name, got, f.SHA256)
		}
		pinned[f.Name] = true
		seen[strings.SplitN(f.Name, ".", 2)[0]] = true
	}
	for _, name := range composeVectorNames {
		if !seen[name] {
			t.Errorf("%s: named here but no file of it is pinned", name)
		}
		delete(seen, name)
	}
	for stray := range seen {
		t.Errorf("%s: pinned file whose vector is not named here", stray)
	}
	// The DIRECTORY, not just the pin: a compose-named file that reached the
	// tree without an entry in the pin is the "copied in without a name" case
	// (tests-lens C-1), and the pin alone cannot see it.
	entries, err := os.ReadDir(filepath.Join("testdata", "vectors"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if isComposeVectorFile(e.Name()) && !pinned[e.Name()] {
			t.Errorf("%s: in testdata/vectors but not in the provenance pin -- re-run scripts/vendor-compose-vectors.sh or remove it", e.Name())
		}
	}
}

// Every keyed compose vector must be a MEMBER of the keyed conformance gate
// (md/conformance_keyed_test.go globs keyed_*.conformance.json): the ids the
// composer's consent screen shows are exactly what that gate checks.
func TestEveryKeyedComposeVectorHasAConformanceRecord(t *testing.T) {
	for _, name := range composeVectorNames {
		if !strings.HasPrefix(name, "keyed_") {
			continue
		}
		if _, err := os.Stat(vectorPath(name, "conformance.json")); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}
