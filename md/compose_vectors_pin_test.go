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
	"keyed_compose_wsh_timelock_hashlock",
	"keyed_compose_wsh_three_paths", "keyed_compose_wsh_two_path_distinct_fingerprints",
	"keyed_compose_wsh_two_path_or_d", "keyed_compose_wsh_unsorted_sole",
	// The six archetype presets, exported by S0b (F-453) and vendored by
	// scripts/vendor-compose-vectors.sh. 26 -> 32.
	"keyed_compose_preset_plain_multisig", "keyed_compose_preset_simple_timelocked_inheritance",
	"keyed_compose_preset_kofn_recovery", "keyed_compose_preset_tiered_recovery",
	"keyed_compose_preset_hashlock_gated", "keyed_compose_preset_decaying_multisig",
	// ONE PER HASH KIND (SPEC_hashlock_kinds §10). The corpus was pinned to a
	// primary commit that PREDATED these three, so the pin matched its own
	// stale corpus and passed -- while covering nothing at all for the three
	// kinds this cycle added. A green pin over a corpus missing the feature
	// under test is the exact shape of "a stale pin hides the next defect".
	"keyed_compose_preset_hashlock_gated_hash256",
	"keyed_compose_preset_hashlock_gated_ripemd160",
	"keyed_compose_preset_hashlock_gated_hash160",
	// THE REST OF THE KEYED TIER, 36 -> 50 (F-630 D4). These 14 had no
	// provenance pin of any kind: the selector was `^(keyed_)?compose_`, so
	// files nobody could date sat beside 32 that were pinned, and 9 of them
	// carried descriptors the md-codec 0.44.0 header rewrite had corrected
	// upstream months earlier.
	"keyed_tr_depth2",
	"keyed_tr_depth2_rightspine",
	"keyed_tr_keyonly",
	"keyed_tr_multi_a",
	"keyed_tr_pathological",
	"keyed_tr_sortedmulti_a",
	"keyed_tr_with_leaf",
	"keyed_wpkh",
	"keyed_wsh_multi_2of3",
	"keyed_wsh_or_b",
	"keyed_wsh_or_d_degrading",
	"keyed_wsh_sortedmulti_2of3",
	"keyed_wsh_thresh",
	"keyed_wsh_timelock_hashlock",
	// WIRE KIND 1, 50 -> 53 (F-449 stage 3). The primary's first vectors whose
	// taproot internal key is Liana's unspendable xpub: two keyed (conformance
	// records, so the keyed gates enrol them) and one keyless single-string
	// twin of nums_taproot (the only version-8 SINGLE-string card).
	"keyed_tr_liana_kofn_recovery",
	"keyed_tr_liana_nested_two_recoveries",
	"liana_taproot",
}

// isComposeVectorFile: the pinned corpus's file names, and nothing else in the
// shared vectors directory (the MANIFEST's other vectors live beside them).
//
// THE keyed_ PREFIX IS PART OF THE PREDICATE, not just keyed_compose_ (F-630
// D4). The pin now covers the whole keyed tier, and this function drives the
// DIRECTORY scan rather than the sha256 scan -- so leaving it at
// keyed_compose_ would give the 14 newly-pinned vectors hash coverage and no
// directory coverage at all. Measured by smuggling an unpinned
// keyed_tr_smuggled.conformance.json into testdata/vectors: under the old
// predicate it passes unflagged while a keyed_compose_smuggled one is caught.
//
// compose_refusal_* IS EXCLUDED, and it is excluded BY ITS OWN PIN rather
// than by a literal name here: composeRefusalPinnedNames reads
// compose_refusal_vectors.provenance.json, so a refusal vector that reached
// the tree without an entry in THAT pin still fails this directory scan. A
// hardcoded prefix would have let any compose_refusal_*.json in unchecked,
// which is the hole this scan exists to close.
func isComposeVectorFile(name string) bool {
	if !strings.HasPrefix(name, "compose_") && !strings.HasPrefix(name, "keyed_") &&
		!strings.HasPrefix(name, "liana_") {
		return false
	}
	return !composeRefusalPinnedNames()[name]
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
	// 48 keyed vectors carry five files, 5 unkeyed carry four: 260
	// (48x5 + 5x4 = 260, and 50 + 3 = 53 names; F-449 stage 3 added two keyed
	// kind-1 vectors and the keyless liana_taproot).
	// The 29th keyed_compose is keyed_compose_wsh_timelock_hashlock, the composer's own
	// three-path wsh policy with both timelock kinds and a hashlock; 30-32 are
	// the per-kind hashlock presets added by SPEC_hashlock_kinds §10.
	if len(p.Files) != 260 {
		t.Fatalf("pin lists %d files, want 260", len(p.Files))
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
