package sysw

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// The vendoring pin (C-4).
//
// TestVendoredVectorsMatchTheirProvenancePin below is a GATE: no sibling
// checkout, no network, no toolchain, no skip path. It runs everywhere.
//
// Its counterpart — is the vendored copy STALE relative to the primary? — needs
// a checkout of mnemonic-engrave that no CI runner has, so it is not a test that
// decides for itself whether to run. It lives in vendored_vectors_live_test.go
// behind the `oraclelive` build tag, and in a normal build it does not exist.
// Operator directive, 2026-08-15: "Don't skip jobs unless I ask."
//
// Saying which is which matters more than either one: the reason the old
// arrangement failed is that a freshness audit was doing a gate's job, and
// nobody noticed it had stopped running.

type vectorPin struct {
	Comment               []string `json:"_comment"`
	Repo                  string   `json:"repo"`
	Remote                string   `json:"remote"`
	Path                  string   `json:"path"`
	Commit                string   `json:"commit"`
	FileCommit            string   `json:"file_commit"`
	RepoCleanWhenRecorded bool     `json:"repo_clean_when_recorded"`
	SHA256                string   `json:"sha256"`
	Bytes                 int      `json:"bytes"`
	Vectors               int      `json:"vectors"`
	RecordedAt            string   `json:"recorded_at"`
}

func loadVectorPin(t *testing.T) vectorPin {
	t.Helper()
	raw, err := os.ReadFile(vectorProvenance)
	if err != nil {
		t.Fatalf("INCONCLUSIVE: the vendored vectors have no provenance pin at %s: %v\n"+
			"A vendored copy with no pin is a file nobody can date; that is the whole "+
			"objection to vendoring, and the pin is the answer to it.", vectorProvenance, err)
	}
	var p vectorPin
	if err := json.Unmarshal(raw, &p); err != nil {
		t.Fatalf("parsing %s: %v", vectorProvenance, err)
	}
	switch {
	case strings.TrimSpace(p.SHA256) == "":
		t.Fatalf("INCONCLUSIVE: %s records no sha256", vectorProvenance)
	case strings.TrimSpace(p.Commit) == "" || strings.TrimSpace(p.Path) == "":
		t.Fatalf("INCONCLUSIVE: %s does not name a primary commit and path", vectorProvenance)
	}
	return p
}

// THE GATE. The vendored bytes must be the bytes the pin describes.
//
// This is what stops the vendored copy from drifting SILENTLY: a hand-edit to
// make an awkward vector pass would change the hash, and re-recording the hash
// is a visible line in a diff rather than an invisible non-event.
func TestVendoredVectorsMatchTheirProvenancePin(t *testing.T) {
	p := loadVectorPin(t)
	raw, err := os.ReadFile(defaultVectors)
	if err != nil {
		t.Fatalf("INCONCLUSIVE: %s is unreadable: %v", defaultVectors, err)
	}
	sum := sha256.Sum256(raw)
	// In FULL, both of them. A hash differing at character 16 rendered
	// truncated reads as two identical values.
	if got := hex.EncodeToString(sum[:]); !strings.EqualFold(got, p.SHA256) {
		t.Fatalf("the vendored vectors are not the file the pin describes:\n"+
			"  on disk %s\n  pinned  %s\n"+
			"Re-sync from %s@%s and update %s; do not edit the vectors here.",
			got, p.SHA256, p.Repo, p.Commit, vectorProvenance)
	}
	if p.Bytes != 0 && len(raw) != p.Bytes {
		t.Errorf("the vendored vectors are %d bytes, the pin says %d", len(raw), p.Bytes)
	}
	var vs []vector
	if err := json.Unmarshal(raw, &vs); err != nil {
		t.Fatalf("the vendored vectors do not parse: %v", err)
	}
	if p.Vectors != 0 && len(vs) != p.Vectors {
		t.Errorf("the vendored file holds %d vector(s), the pin says %d", len(vs), p.Vectors)
	}
	t.Logf("%d vector(s), %d bytes, %s — from %s @ %s (file last changed in %s)",
		len(vs), len(raw), p.SHA256[:16], p.Repo, p.Commit[:8], p.FileCommit[:8])
}
