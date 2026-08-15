//go:build oraclelive

package oracle

// THE LIVE FRESHNESS CHECKS — behind a build tag, so in a normal build they DO
// NOT EXIST rather than skipping.
//
// # Why a build tag and not a skip, and not an environment variable either
//
// Operator directive, 2026-08-15: "Don't skip jobs unless I ask."
//
// Everything in this file shells out to the pinned md/mk/ms binaries, which
// cannot be installed on the machine that decides whether a merge lands:
// pins.json binds each oracle to the SHA-256 of a binary built on the
// maintainer's machine, so a CI `cargo install` produces a different binary, a
// different hash, and a hard resolution failure. A test that cannot run there
// has exactly three honest shapes, and only one of them is acceptable:
//
//	SKIP when absent          reports `ok` having checked nothing — this is the
//	                          defect C-1..C-4 were filed about, measured on CI
//	                          run 31898063163
//	FAIL when absent          turns the required check permanently red for a
//	                          reason that is not a defect
//	NOT EXIST unless asked    this file
//
// An environment-variable opt-out was tried and rejected: it is a skip with
// extra steps, and this repo already proved the mirror image of it fails — sysw
// shipped SYSW_REQUIRE_VECTORS=1 -> t.Fatalf, the right mechanism, and nothing
// ever set it. Under a build tag the decision is made by a human typing the tag,
// once, on purpose, and there is no runtime branch that could take itself.
//
// # What still enforces the property when this file does not exist
//
// Nothing here is load-bearing. The guarantee is carried by three things that
// have no skip path and need no toolchain:
//
//	cmd/gaterecord              refuses to mint a record whose census is not what
//	                            it JUST derived live — so an expectation cannot
//	                            exist except as the output of a live run
//	TestEveryGateRecordCensus…  compares every record against that expectation
//	TestVendoredExpectations…   binds every expectation's recorded oracle
//	                            identity to pins.json, commit and binary hash
//
// This file is the drift check on top of that: it asks whether the primary has
// changed under a pin that did not move. That is a maintainer's question, asked
// on a maintainer's machine, deliberately.
//
// # Running it
//
//	./scripts/oracle-live.sh
//	go test -tags oraclelive ./oracle/ -v
//
// ABSENCE IS FATAL HERE. You asked for the live checks; not having the oracles
// is a failure, not a reason to report success.
//
// The tagged files are TYPE-CHECKED on every push (`go vet -tags oraclelive
// ./...` in .github/workflows/test.yml) for the reason the workflow already
// states about the GOOS=js emulator build: a gate whose instrument does not
// compile is not a gate, and a build tag is an excellent place for one to rot.

import (
	"os"
	"path/filepath"
	"testing"
)

// resolveBins locates the pinned oracles the way cmd/gaterecord does.
//
// The old version of this function called t.Skipf when a binary was missing,
// which took out the derived-census gate AND all three of its own mutation
// proofs at once, on every machine without a Rust toolchain — permanently
// including the deciding one. Its stated justification was false too: it claimed
// "the gate that makes absence fail is TestS0GateHasARecord", and that test
// checks only that a record FILE exists and never touches CompareCensus.
//
// There is no skip left. Absence is fatal.
func resolveBins(t *testing.T) Bins {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("no home directory, so the pinned oracles cannot be located: %v", err)
	}
	dir := filepath.Join(home, ".cargo", "bin")
	pf, err := LoadPins("pins.json")
	if err != nil {
		t.Fatalf("loading pins: %v", err)
	}
	for _, fp := range pf.Pins {
		if _, err := os.Stat(filepath.Join(dir, fp.Name)); err != nil {
			t.Fatalf("the pinned oracle %q is not installed at %s.\n"+
				"These checks were requested explicitly (-tags oraclelive), so their absence "+
				"is a failure rather than a reason to report success. Install the pinned "+
				"oracles — oracle/pins.json names the repo and commit of each — or run the "+
				"suite without the tag, where the committed expectations are compared byte "+
				"for byte with no toolchain at all.", fp.Name, dir)
		}
	}
	// Resolve for real, so a SUBSTITUTED binary fails here rather than being
	// used to derive an expectation nobody checked.
	if _, err := ResolveAll(pf, func(name string) (string, string) {
		return filepath.Join(dir, name), ""
	}); err != nil {
		t.Fatalf("the pinned oracles do not resolve, so nothing derived from them can be trusted: %v", err)
	}
	return Bins{
		MD: filepath.Join(dir, "md"),
		MK: filepath.Join(dir, "mk"),
		MS: filepath.Join(dir, "ms"),
	}
}

// TestLiveDerivationReproducesEveryCommittedExpectation re-derives every
// committed expectation and requires it to reproduce byte for byte — string,
// origin and fingerprint.
//
// This is what stops a committed expectation from becoming a cached wrong
// answer, and it runs on every machine that could MINT one, which is exactly the
// population that could mint a wrong one.
func TestLiveDerivationReproducesEveryCommittedExpectation(t *testing.T) {
	bins := resolveBins(t)
	for _, c := range loadExpectations(t) {
		if c.Expect.Inputs == "" {
			t.Errorf("%s: the expectation names no inputs file, so it cannot be re-derived", c.Name)
			continue
		}
		inf, err := LoadInputsFile(filepath.Join(GateRecordsDir, c.Expect.Inputs))
		if err != nil {
			t.Errorf("%s: loading %s: %v", c.Name, c.Expect.Inputs, err)
			continue
		}
		if inf.Expect == nil {
			t.Errorf("%s: %s carries no expect block, so no census can be derived from it",
				c.Name, c.Expect.Inputs)
			continue
		}
		seeds, err := inf.SeedWords()
		if err != nil {
			t.Errorf("%s: %v", c.Name, err)
			continue
		}
		got, err := DeriveExpected(*inf.Expect, c.Record.Inputs, seeds, bins)
		if err != nil {
			t.Errorf("%s: deriving: %v", c.Name, err)
			continue
		}
		if len(got.Artifacts) != len(c.Expect.Artifacts) {
			t.Errorf("%s: the live toolchain derives %d artifact(s); the committed expectation "+
				"holds %d — re-mint it", c.Name, len(got.Artifacts), len(c.Expect.Artifacts))
			continue
		}
		for i := range got.Artifacts {
			a, b := got.Artifacts[i], c.Expect.Artifacts[i]
			// In full, both sides, always.
			if a.String != b.String {
				t.Errorf("%s: artifact %d (%s) drifted:\n  live      %s\n  committed %s",
					c.Name, i, b.Label, a.String, b.String)
			}
			if a.Fingerprint != b.Fingerprint {
				t.Errorf("%s: artifact %d (%s) fingerprint drifted: live %s, committed %s",
					c.Name, i, b.Label, a.Fingerprint, b.Fingerprint)
			}
			if a.Origin != b.Origin {
				t.Errorf("%s: artifact %d (%s) origin drifted: live %s, committed %s",
					c.Name, i, b.Label, a.Origin, b.Origin)
			}
		}
		t.Logf("%s: %d artifact(s) re-derived live and identical to the committed expectation "+
			"(oracles invoked: %v)", c.Name, len(got.Artifacts), got.Oracles)
	}
}

// TestRealPinsResolveTheInstalledOracles resolves the REAL md/mk/ms against the
// committed pin file. It is the test that would have caught a stale pin file,
// which is the way this deliverable most plausibly rots: the pins are recorded
// by hand and the binaries get rebuilt.
//
// Moved here from oracle_test.go, where it skipped whenever a binary was
// missing — so the backstop against a stale pin file sat behind the same door as
// everything it was backing up, and had never run on the machine that decides a
// merge. Its -short skip is gone too: -short means "keep tier 2 out of the inner
// loop", and asking for -tags oraclelive IS asking for tier 2.
func TestRealPinsResolveTheInstalledOracles(t *testing.T) {
	pf, err := LoadPins("pins.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(pf.Pins) != 3 {
		t.Fatalf("pins.json has %d pins, want md/mk/ms", len(pf.Pins))
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("no home dir: %v", err)
	}
	dir := filepath.Join(home, ".cargo", "bin")
	for _, fp := range pf.Pins {
		if _, err := os.Stat(filepath.Join(dir, fp.Name)); err != nil {
			t.Fatalf("%s is not installed at %s: %v", fp.Name, dir, err)
		}
	}
	locate := func(name string) (string, string) {
		// Binary-hash mode: these are installed binaries, NOT built inside a
		// checkout in this run, so pairing them with a source tree would be
		// refused by design (see ErrBinaryOutsideCheckout).
		return filepath.Join(dir, name), ""
	}
	got, err := ResolveAll(pf, locate)
	if err != nil {
		t.Fatalf("the committed pins do not match the installed oracles — "+
			"re-record them, do not weaken the check: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("resolved %d oracles, want 3", len(got))
	}
	for _, r := range got {
		if r.Method != ByBinaryHash {
			t.Errorf("%s resolved via %s, want %s", r.Name, r.Method, ByBinaryHash)
		}
		if len(r.Commit) != 40 {
			t.Errorf("%s resolved to %q, which is not a full commit id", r.Name, r.Commit)
		}
		if !r.VersionMatchesPin {
			t.Logf("NOTE: %s reports %q, pin says otherwise — not a failure, "+
				"but the pin's version field is stale", r.Name, r.ReportedVersion)
		}
	}
}
