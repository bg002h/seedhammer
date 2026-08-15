package sysw

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The two halves of the vendoring pin (C-4), and they are deliberately NOT the
// same kind of check.
//
//	TestVendoredVectorsMatchTheirProvenancePin   a GATE. No sibling checkout, no
//	                                             network, no skip path.
//	TestVendoredVectorsAreInSyncWithThePrimary   an AUDIT of freshness. It needs
//	                                             the sibling repo and may skip.
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

// THE FRESHNESS AUDIT, and it may skip — say so plainly rather than letting a
// reader mistake it for the gate.
//
// The primary is where these vectors are generated, and this fork is strictly
// downstream: a vector added there must reach here. Nothing in this repository
// can observe that on its own, so this compares the vendored copy against a
// sibling checkout WHEN ONE IS PRESENT. On a machine without the sibling repo
// there is no fact to check, and a skip is the honest report of that — which is
// only acceptable because the property this test audits (are we current?) is a
// different property from the one the suite ENFORCES (do we agree with what we
// vendored?), and the latter is checked above with no skip path at all.
func TestVendoredVectorsAreInSyncWithThePrimary(t *testing.T) {
	p := loadVectorPin(t)
	sibling := filepath.Join("..", "..", "mnemonic-engrave", filepath.FromSlash(p.Path))
	upstream, err := os.ReadFile(sibling)
	if err != nil {
		abs, _ := filepath.Abs(sibling)
		t.Skipf("AUDIT ONLY, and it has no input here: no primary checkout at %s (%v). "+
			"The gate — TestVendoredVectorsMatchTheirProvenancePin and the whole "+
			"conformance suite — ran regardless; this test only asks whether the vendored "+
			"copy is STALE, which needs the primary to compare against.", abs, err)
	}
	vendored, err := os.ReadFile(defaultVectors)
	if err != nil {
		t.Fatalf("%s: %v", defaultVectors, err)
	}
	if string(vendored) != string(upstream) {
		us := sha256.Sum256(upstream)
		vs := sha256.Sum256(vendored)
		t.Errorf("the vendored vectors are STALE: the primary's copy hashes to %s, ours to %s.\n"+
			"Re-sync it — the Go port is strictly downstream, so a vector added in the primary "+
			"is a case this port is currently untested against. From the REPO ROOT:\n"+
			"  cp ../%s/%s sysw/testdata/sysw_vectors.json\n"+
			"then update sysw/%s (commit, file_commit, sha256, bytes, vectors).",
			hex.EncodeToString(us[:]), hex.EncodeToString(vs[:]), p.Repo, p.Path, vectorProvenance)
		return
	}
	// The copy is identical, so the recorded commit is the last claim left that
	// could be wrong. Check it where git is available; this is best-effort by
	// design — a wrong commit with identical bytes is a bookkeeping error, not a
	// conformance one.
	repo := filepath.Join("..", "..", "mnemonic-engrave")
	out, err := exec.Command("git", "-C", repo, "log", "-1", "--format=%H", "--",
		filepath.ToSlash(p.Path)).Output()
	if err != nil {
		t.Logf("could not read the primary's history (%v); bytes are identical, so the "+
			"vectors are current even if the recorded commit cannot be confirmed", err)
		return
	}
	if got := strings.TrimSpace(string(out)); got != "" && got != p.FileCommit {
		t.Errorf("bytes are identical but the pin's file_commit is wrong: the primary says "+
			"%s last changed in %s, %s records %s", p.Path, got, vectorProvenance, p.FileCommit)
	}
}
