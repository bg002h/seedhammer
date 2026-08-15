//go:build oraclelive

package gui

// S2's LIVE freshness check, and the only thing that can mint its golden.
//
// Behind a build tag rather than skipping: see oracle/live_test.go for the whole
// argument. In short — this shells out to the pinned `md`, which cannot be
// installed on the machine that decides a merge (pins.json binds a
// maintainer-built binary's SHA-256, which a CI build cannot reproduce), so the
// only honest shapes are FAIL-there, SKIP-there, or NOT-EXIST-unless-asked. The
// first turns the required check red for a non-defect; the second is the defect
// C-3 was filed about; this is the third.
//
// What still enforces S2's byte-identity without it:
// TestAssembledMd1MatchesTheCommittedGolden, which compares the device's
// assembly against the primary's committed output and checks that output's
// provenance against oracle/pins.json — no toolchain, no skip path, every push.
//
// Run it:
//
//	./scripts/oracle-live.sh              # check
//	./scripts/oracle-live.sh -update      # re-mint the golden
//
// Type-checked on every push by `go vet -tags oraclelive ./...`, so this file
// cannot rot uncompiled behind its own tag.

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"seedhammer.com/oracle"
)

// s2OracleMD resolves the pinned md binary the way cmd/gaterecord does.
//
// ABSENCE IS FATAL. The old version skipped, with the rationale "a contributor
// without the Rust toolchain should not see a red suite they cannot fix" — a
// real concern and the wrong remedy, because the machine that decides a merge is
// permanently such a contributor. That concern is served properly now: a
// toolchain-free contributor gets the real byte-identity comparison from the
// committed golden and never builds this file at all. Someone who typed the tag
// asked for the live check, so not having the oracle is a failure.
func s2OracleMD(t *testing.T) string {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("no home directory, so the pinned oracle cannot be located: %v", err)
	}
	dir := filepath.Join(home, ".cargo", "bin")
	pf, err := oracle.LoadPins(filepath.Join("..", "oracle", "pins.json"))
	if err != nil {
		t.Fatalf("loading pins: %v", err)
	}
	bin := filepath.Join(dir, "md")
	if _, err := os.Stat(bin); err != nil {
		t.Fatalf("the pinned md oracle is not installed at %s.\n"+
			"These checks were requested explicitly (-tags oraclelive), so their absence is a "+
			"failure rather than a reason to report success. Install it (oracle/pins.json "+
			"names descriptor-mnemonic and the commit), or run the suite without the tag, "+
			"where the committed golden is compared byte for byte with no toolchain.", bin)
	}
	var pin oracle.Pin
	for _, fp := range pf.Pins {
		if fp.Name == "md" {
			pin = fp.Pin
		}
	}
	if pin.Name == "" {
		t.Fatal("pins.json has no entry for md, so nothing identifies the oracle")
	}
	res, err := oracle.Resolve(oracle.Request{Pin: pin, Bin: bin})
	if err != nil {
		t.Fatalf("the pinned md oracle does not resolve, so nothing compared against it "+
			"can be trusted: %v", err)
	}
	t.Logf("md oracle resolved: commit %s by %s (reports %q, matches pin: %v)",
		res.Commit, res.Method, res.ReportedVersion, res.VersionMatchesPin)
	return bin
}

// TestAssembledMd1MatchesThePrimaryByteForByte re-derives Trace A's md1 from the
// live pinned oracle and requires the device AND the committed golden to equal
// it. This is what stops the golden from becoming a cached wrong answer: it runs
// on every machine that could MINT one.
//
// `-update` re-mints the golden. There is no other code path that writes one, so
// a golden can only ever be the output of a live derivation — which is what
// makes it safe for the untagged suite to trust it.
func TestAssembledMd1MatchesThePrimaryByteForByte(t *testing.T) {
	bin := s2OracleMD(t)
	got, xpubs, stub := s2TraceAPolicy(t)

	args := s2MDArgs(xpubs)
	out, err := exec.Command(bin, args...).Output()
	if err != nil {
		msg := ""
		if ee, ok := err.(*exec.ExitError); ok {
			msg = string(ee.Stderr)
		}
		t.Fatalf("md encode failed: %v\n%s", err, msg)
	}

	// REFUSE any stdout line that is not an md1 string rather than collecting it
	// as one: md encode prints "chunk-set-id: 0x…" ahead of the strings, and a
	// line-splitter that trusts every line would adopt that header as a chunk and
	// then report a length mismatch instead of a content one.
	var want []string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "md1") {
			want = append(want, line)
		}
	}
	if len(want) == 0 {
		t.Fatalf("md encode produced no md1 strings; stdout was:\n%s", out)
	}

	if len(got) != len(want) {
		t.Fatalf("the device assembled %d chunk(s); the primary produced %d for the "+
			"same inputs\ndevice:  %v\nprimary: %v", len(got), len(want), got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			// In full, both of them, always. A truncated string makes two
			// different values read as one.
			t.Fatalf("chunk %d differs:\n  primary %s\n  device  %s", i, want[i], got[i])
		}
	}

	arts := make([]oracle.Artifact, len(want))
	for i, s := range want {
		arts[i] = oracle.Artifact{
			Kind:   "md1",
			Label:  "Trace A 2-of-3 wsh policy, chunk " + string(rune('0'+i)),
			String: s,
		}
	}
	pf, err := oracle.LoadPins(filepath.Join("..", "oracle", "pins.json"))
	if err != nil {
		t.Fatalf("loading pins: %v", err)
	}

	if *update {
		ef, err := oracle.NewExpectFile("S2", "", "", "Trace A's 2-of-3 wsh policy as the "+
			"PINNED PRIMARY encodes it: self = masterA at the shared origin, cosigners B@0 and "+
			"C@0, self slot @0, fingerprints omitted. Minted only by "+
			"`./scripts/oracle-live.sh -update`, which cannot run without the pinned md "+
			"binary. The device's own assembly is compared against this on every machine, "+
			"with no toolchain, by TestAssembledMd1MatchesTheCommittedGolden.",
			pf, []string{"md"}, []string{"md " + strings.Join(args, " ")}, arts)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Dir(s2GoldenPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := ef.Write(s2GoldenPath); err != nil {
			t.Fatal(err)
		}
		t.Logf("wrote %s (%d chunk(s))", s2GoldenPath, len(arts))
		return
	}

	// THE FRESHNESS ASSERTION. The golden must be what the live oracle produces
	// today, not merely what the device produces today.
	golden, err := oracle.LoadExpect(s2GoldenPath)
	if err != nil {
		t.Fatalf("INCONCLUSIVE: no committed golden to check for freshness at %s: %v",
			s2GoldenPath, err)
	}
	if err := golden.CheckProvenance(pf); err != nil {
		t.Fatalf("%s: %v", s2GoldenPath, err)
	}
	if err := oracle.CompareCensus(golden.Artifacts, want); err != nil {
		t.Fatalf("the committed golden is not what the pinned primary produces today — "+
			"re-mint it with `./scripts/oracle-live.sh -update`, do not edit it.\n%v", err)
	}
	t.Logf("%d md1 chunk(s) byte-identical to the primary and to the committed golden; "+
		"policy stub %x", len(got), stub)
}
