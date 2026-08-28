//go:build oraclelive

package gui

// THE CHAIN FIXTURES' FRESHNESS AUDIT: are the committed bytes still what `me`
// emits today?
//
// Behind a build tag rather than skipping, exactly as
// sysw/vendored_vectors_live_test.go is and for the same reason: it needs the
// `me` CLI, which no CI runner has, and a test that answers "I could not tell"
// by reporting success is the default failure mode in this tree. Operator
// directive, 2026-08-15: "Don't skip jobs unless I ask."
//
// What runs everywhere, with no skip path and no toolchain: the whole of
// gui/chain_walk_test.go, against these committed bytes. Those enforce
// AGREEMENT between the container and the device. This one asks the different
// question -- STALENESS -- and it is a maintainer's question asked on a
// maintainer's machine:
//
//	ME=$(command -v me) MT=$(command -v mt) go test -tags oraclelive ./gui -run TestChainFixtures
//
// ABSENCE IS FATAL HERE. You asked for the audit; not having `me` means the
// audit cannot be performed, which is a failure and not a pass.
//
// Type-checked on every push by `go vet -tags oraclelive ./...`.

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

// TestChainFixturesStillMatchWhatMeEmits re-runs each fixture's recorded
// command and compares byte for byte.
//
// `me sysw pack` is DETERMINISTIC for the unsealed variant -- salt and IV are
// only consumed on the sealed path -- which is what makes this a byte
// comparison rather than a structural one. A sealed fixture could not be
// audited this way and there is none here.
func TestChainFixturesStillMatchWhatMeEmits(t *testing.T) {
	me := os.Getenv("ME")
	if me == "" {
		me = "me"
	}
	// RESOLVED AND REPORTED. A bare name can be a shell alias or a stale
	// install, and this test's whole claim is about which binary produced the
	// bytes.
	path, err := exec.LookPath(me)
	if err != nil {
		t.Fatalf("the `me` CLI is not on PATH as %q (%v).\n"+
			"This audit was requested explicitly (-tags oraclelive) and cannot be "+
			"performed without the producer to compare against, so its absence is a "+
			"failure. Set ME=/path/to/me, or run the suite without the tag -- the "+
			"committed bytes are still exercised there by the whole of "+
			"gui/chain_walk_test.go.", me, err)
	}
	ver, err := exec.Command(path, "--version").Output()
	if err != nil {
		t.Fatalf("%s --version: %v", path, err)
	}
	version := strings.TrimSpace(string(ver))
	t.Logf("auditing against %s (%s)", path, version)

	fixture := filepath.Join("testdata", "chain", "chain_payloads.json")
	raw, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatalf("%s: %v", fixture, err)
	}
	var doc struct {
		MeVersion string         `json:"me_version"`
		Payloads  []chainPayload `json:"payloads"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parsing %s: %v", fixture, err)
	}
	if doc.MeVersion != version {
		// NOT fatal on its own: a version bump that changes no bytes is a
		// bookkeeping error, not a conformance one. The byte comparison below
		// is the gate.
		t.Errorf("the fixture was recorded against %q; this is %q. If the bytes "+
			"below still match, re-run ./scripts/gen-chain-fixtures.sh to update "+
			"the recorded version.", doc.MeVersion, version)
	}
	if len(doc.Payloads) == 0 {
		t.Fatal("the fixture holds no payloads")
	}

	for _, p := range doc.Payloads {
		t.Run(p.Name, func(t *testing.T) {
			dir := t.TempDir()
			recs := filepath.Join(dir, "records.txt")
			if err := os.WriteFile(recs, []byte(strings.Join(p.Records, "\n")+"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			out := filepath.Join(dir, p.Name+".bin")
			// The recorded command with its two placeholders resolved. Taking
			// the flags FROM THE FIXTURE rather than restating them here is what
			// makes the audit an audit: a fixture built with an escape hatch
			// (--allow-unsigned-inputs) is re-built with it, and a fixture that
			// quietly gained one is visible in the diff of this file's data.
			var args []string
			for _, a := range p.Command[1:] {
				switch a {
				case "<records>":
					args = append(args, recs)
				default:
					if strings.HasSuffix(a, ".bin") {
						args = append(args, out)
					} else {
						args = append(args, a)
					}
				}
			}
			cmd := exec.Command(path, args...)
			var stderr strings.Builder
			cmd.Stderr = &stderr
			if err := cmd.Run(); err != nil {
				t.Fatalf("%s %s: %v\n%s", path, strings.Join(args, " "), err, stderr.String())
			}
			got, err := os.ReadFile(out)
			if err != nil {
				t.Fatal(err)
			}
			want, err := hex.DecodeString(p.Blob)
			if err != nil {
				t.Fatalf("fixture blob is not hex: %v", err)
			}
			if string(got) != string(want) {
				gs, ws := sha256.Sum256(got), sha256.Sum256(want)
				t.Fatalf("the committed fixture is STALE: `me` now emits %d bytes "+
					"hashing to %s; the fixture holds %d bytes hashing to %s.\n"+
					"Re-record with ./scripts/gen-chain-fixtures.sh, then re-run "+
					"the chain walks -- a changed container may change the digest "+
					"the device shows, which is the number the walks assert.",
					len(got), hex.EncodeToString(gs[:]),
					len(want), hex.EncodeToString(ws[:]))
			}
			// The digest is a SECOND claim about the same bytes, and it is the
			// one the walks assert against a device screen. Re-read it from the
			// CLI rather than recomputing it here: two implementations of one
			// number is what the pin exists to avoid.
			var digest string
			for _, line := range strings.Split(stderr.String(), "\n") {
				if strings.HasPrefix(strings.TrimSpace(line), "digest:") {
					digest = strings.TrimSpace(strings.SplitN(line, ":", 2)[1])
				}
			}
			if digest == "" {
				t.Fatalf("`me sysw pack` printed no digest line; the fixture's "+
					"`digest` field can no longer be audited.\n%s", stderr.String())
			}
			if digest != p.Digest {
				t.Fatalf("bytes are identical but the recorded digest is wrong: "+
					"`me` prints %q, the fixture holds %q", digest, p.Digest)
			}
		})
	}
}
