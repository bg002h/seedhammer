//go:build oraclelive

package sysw

// The vendored vectors' FRESHNESS audit: are we still current with the primary?
//
// Behind a build tag rather than skipping. It needs a checkout of
// mnemonic-engrave beside this repo, which no CI runner has, and a test that
// answers "I could not tell" by reporting success is the exact defect C-4 was
// filed about. Operator directive, 2026-08-15: "Don't skip jobs unless I ask."
//
// What runs without the tag, everywhere, with no skip path:
// TestVendoredVectorsMatchTheirProvenancePin (are the bytes what the pin says?)
// plus the whole conformance suite against those bytes. Those enforce AGREEMENT.
// This one asks a different question — STALENESS — and it is a maintainer's
// question, asked on a maintainer's machine, deliberately:
//
//	./scripts/oracle-live.sh
//
// ABSENCE IS FATAL HERE. You asked for the audit; not having the primary
// checkout means the audit cannot be performed, which is a failure and not a
// pass.
//
// Type-checked on every push by `go vet -tags oraclelive ./...`.

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestVendoredVectorsAreInSyncWithThePrimary(t *testing.T) {
	p := loadVectorPin(t)
	sibling := filepath.Join("..", "..", "mnemonic-engrave", filepath.FromSlash(p.Path))
	upstream, err := os.ReadFile(sibling)
	if err != nil {
		abs, _ := filepath.Abs(sibling)
		t.Fatalf("no primary checkout at %s (%v).\n"+
			"This audit was requested explicitly (-tags oraclelive) and cannot be performed "+
			"without the primary to compare against, so its absence is a failure. Check out "+
			"%s beside this repo, or run the suite without the tag — the vendored bytes are "+
			"still gated there by TestVendoredVectorsMatchTheirProvenancePin and the whole "+
			"conformance suite.", abs, err, p.Repo)
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
	// could be wrong. Best-effort by design: a wrong commit with identical bytes
	// is a bookkeeping error, not a conformance one.
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
	t.Logf("vendored copy is byte-identical to %s/%s, file_commit %s confirmed",
		p.Repo, p.Path, p.FileCommit[:8])
}
