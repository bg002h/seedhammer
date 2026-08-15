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
// What this file adds on top of that is TWO different questions, and until
// 2026-08-15 this comment claimed the second one while nothing here asked it:
//
//	FRESHNESS   TestLiveDerivationReproducesEveryCommittedExpectation and
//	            TestRealPinsResolveTheInstalledOracles. Do the INSTALLED binaries
//	            still reproduce the committed bytes, and are they still the
//	            binaries pins.json names? Both are about what is on this disk.
//	DRIFT       TestPinsAreCurrentWithTheirPrimaries. Has the PRIMARY moved under
//	            a pin that did not? Nothing on this machine can answer that —
//	            resolution only ever compares an installed binary's hash to a
//	            recorded one, and both sides of that stay equal forever while the
//	            upstream repo cuts release after release.
//
// The old wording said "this file is the drift check … it asks whether the
// primary has changed under a pin that did not move", and no code in it did.
// That is the comments-outlive-their-conditions class: a claim that reads as
// coverage, is load-bearing for whoever decides not to write the check, and
// costs nothing to be wrong. The claim is true now because the test exists;
// if the test is ever deleted, delete this paragraph with it.
//
// Both are maintainer's questions, asked on a maintainer's machine, deliberately.
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
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

// ─── DRIFT: has the PRIMARY moved under a pin that did not? ─────────────────

// TestPinsAreCurrentWithTheirPrimaries compares every pins.json commit to the
// newest RELEASE TAG in that oracle's sibling checkout.
//
// # Why nothing else can ask this
//
// Every other check in this repo compares a pin to something derived from the
// pin: Resolve hashes the installed binary against the recorded hash;
// CheckProvenance compares an expectation's recorded identity to pins.json;
// the live derivation re-runs the installed binaries. All of them stay green
// forever while descriptor-mnemonic, mnemonic-key and mnemonic-secret cut
// release after release, because none of them ever looks at the primary. The
// pin can be perfectly HONEST and years stale at the same time, and that is
// exactly the state F-177 found ms in.
//
// # Why it is behind the tag and must NOT be untagged
//
// It reads sibling git checkouts, which the machine that decides a merge does
// not have. An untagged version would therefore need a skip, and the operator
// directive is "Don't skip jobs unless I ask" — so it lives here, where asking
// for it is a human typing the tag, and where ABSENCE IS FATAL rather than a
// reason to report success.
//
// # The three-way answer, and why "pin == newest tag" is the wrong question
//
// Measured today: the mk pin is at a38a908, which is two commits AHEAD of
// mk-cli-v0.12.1 (4ac7ab4) — the pinned binary reports "mk 0.13.0", a version
// that has not been tagged yet. A test demanding equality would be red on
// arrival for a pin that is not stale at all, and a check that is red for a
// non-defect gets weakened rather than fixed. So the question asked is whether a
// RELEASE EXISTS THAT THE PIN DOES NOT NAME:
//
//	tag == pin              current. Nothing to decide.
//	tag is an ANCESTOR      the pin is ahead of the newest release — an
//	                        unreleased commit, deliberately pinned. Logged, not
//	                        failed; the hygiene item is to cut the tag.
//	anything else           a release the pin does not name. FAIL: this is a
//	                        DECISION for a human (bump and re-anchor, or record
//	                        why the delta is inert), and it is not a reason to
//	                        weaken this check.
func TestPinsAreCurrentWithTheirPrimaries(t *testing.T) {
	pf, err := LoadPins("pins.json")
	if err != nil {
		t.Fatalf("loading pins: %v", err)
	}
	for _, fp := range pf.Pins {
		if fp.Repo == "" {
			t.Errorf("pin %q names no repo, so nothing says which primary it tracks", fp.Name)
			continue
		}
		// The sibling convention this repo already uses: sysw's vendored-vector
		// live check resolves ../../mnemonic-engrave the same way.
		dir := filepath.Join("..", "..", fp.Repo)
		if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
			t.Fatalf("the primary checkout for %q is not at %s (%v).\n"+
				"These checks were requested explicitly (-tags oraclelive), so a missing checkout "+
				"is a failure rather than a reason to report success — without it nothing can ask "+
				"whether %s has released anything since the pin was recorded. Clone %s beside this "+
				"repo, or run the suite without the tag.", fp.Repo, dir, err, fp.Repo, fp.Repo)
		}

		tag, err := newestReleaseTag(dir, fp.Name)
		if err != nil {
			t.Errorf("%s: %v", fp.Name, err)
			continue
		}
		target, err := gitOut(dir, "rev-parse", tag+"^{}")
		if err != nil {
			t.Errorf("%s: resolving tag %s: %v", fp.Name, tag, err)
			continue
		}
		// In FULL, both sides, always: a commit rendered truncated makes two
		// different values read as one.
		switch {
		case strings.EqualFold(target, strings.TrimSpace(fp.Commit)):
			t.Logf("%s: pin is the newest release %s (%s)", fp.Name, tag, target)
		case isAncestor(dir, target, fp.Commit):
			t.Logf("NOTE: %s is pinned AHEAD of its newest release. pin %s, newest tag %s -> %s. "+
				"Deliberate while a version is unreleased (the binary reports %q); the hygiene "+
				"item is to cut the tag so the pin names a release.",
				fp.Name, fp.Commit, tag, target, fp.Version)
		default:
			t.Errorf("%s HAS DRIFTED: %s names %s, which the pin does not.\n"+
				"  pinned      %s\n  newest tag  %s -> %s\n"+
				"The pin is still HONEST — the installed binary matches its hash — and that is "+
				"the point: nothing else in this repo can see a primary move, because every other "+
				"check compares the pin to something derived from the pin. This is a DECISION: "+
				"rebuild at the tag and re-anchor every affected record and expectation in one "+
				"commit (see F-177 and the `gaterecord -force` route), or record why the delta is "+
				"byte-inert for every invocation this gate makes. Do not weaken this check.",
				fp.Name, fp.Repo, tag, fp.Commit, tag, target)
		}
	}
}

// newestReleaseTag returns the highest `<name>-cli-v*` tag in dir.
//
// --sort=v:refname is REQUIRED and is not a nicety: git's default tag order is
// lexicographic, under which v0.9.0 sorts after v0.16.0 and this check would
// compare the pin against a year-old release and report everything current.
// Measured on mnemonic-secret, where the default order ends at ms-cli-v0.9.0
// while the newest release is ms-cli-v0.16.0.
func newestReleaseTag(dir, name string) (string, error) {
	out, err := gitOut(dir, "tag", "-l", name+"-cli-v*", "--sort=v:refname")
	if err != nil {
		return "", err
	}
	tags := strings.Fields(out)
	if len(tags) == 0 {
		return "", fmt.Errorf("no %s-cli-v* tag in %s, so there is no release to compare the pin "+
			"against; this check cannot answer for %s", name, dir, name)
	}
	return tags[len(tags)-1], nil
}

// isAncestor reports whether a is an ancestor of b in dir.
func isAncestor(dir, a, b string) bool {
	return exec.Command("git", "-C", dir, "merge-base", "--is-ancestor", a, b).Run() == nil
}

func gitOut(dir string, args ...string) (string, error) {
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) && len(ee.Stderr) > 0 {
			return "", fmt.Errorf("git %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(string(ee.Stderr)))
		}
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(out)), nil
}

// ─── S5.0: the built-policy kind, run against the real toolchain ────────────

// TestBuiltPolicyDerivationMatchesTheS2Golden derives a built policy from the
// three published-vector masters and requires its md1 chunks to equal the S2
// golden BYTE FOR BYTE.
//
// # Why this is worth more than a self-consistency check
//
// S5.0 builds an instrument before the stage it judges exists, so the ordinary
// proof — "the mint agreed with the walk" — is not available yet, and the
// ruling is explicit that S5.0 does not pretend to discharge it. What IS
// available is an independent artifact describing the SAME wallet: S2's
// committed golden is Trace A's 2-of-3 wsh policy (self = masterA at the shared
// origin, cosigners B@0 and C@0), minted through a completely different code
// path — the device assembles it, gui's live test invokes md with a template
// written by hand there, and only `./scripts/oracle-live.sh -update` can write
// it.
//
// So if policyTemplate generated the wrong template, or mdEncode passed the
// wrong flags, or parseMdStdout dropped or adopted a line, this derivation would
// produce a different wallet's md1 and this test would say so — before any S5
// code exists to be blamed for it. A derivation that only agreed with itself
// could not.
//
// # This is NOT what CheckDataSource refuses
//
// The golden's path contains `testdata`, which CheckDataSource refuses as a
// COMPARISON SOURCE. The argument is in expectfile.go and it holds here: every
// byte of that file came out of the pinned md, none of it was authored in this
// fork, and CheckProvenance binds it to pins.json below before a single byte is
// compared. It is a cached primary output, not a fork-vendored vector.
func TestBuiltPolicyDerivationMatchesTheS2Golden(t *testing.T) {
	bins := resolveBins(t)

	// Trace A's three masters, which are BIP-39's own published vectors — the
	// same three the S0 inputs file commits, and the reason it may.
	seeds := []Seed{
		{Label: "payload:masterA (card A@0)", Words: "abandon abandon abandon abandon abandon " +
			"abandon abandon abandon abandon abandon abandon about"},
		{Label: "payload:masterB (card B@0)", Words: "legal winner thank year wave sausage " +
			"worth useful legal winner thank yellow"},
		{Label: "payload:masterC (card C@0)", Words: "letter advice cage absurd amount doctor " +
			"acoustic avoid letter advice cage above"},
	}
	in := InputTuple{
		Template:  "built-policy: 2-of-3 wsh, self at slot 0, shared origin",
		N:         3,
		K:         2,
		SlotOrder: []int{0, 1, 2},
		Origins:   []string{"m/48h/0h/0h/2h", "m/48h/0h/0h/2h", "m/48h/0h/0h/2h"},
	}

	golden, err := LoadExpect(filepath.Join("..", "gui", "testdata", "s2_md1_golden.expect.json"))
	if err != nil {
		t.Fatalf("INCONCLUSIVE: S2's committed golden is what this derivation is checked against, "+
			"and it did not load: %v", err)
	}
	pf, err := LoadPins("pins.json")
	if err != nil {
		t.Fatalf("loading pins: %v", err)
	}
	if err := golden.CheckProvenance(pf); err != nil {
		t.Fatalf("the S2 golden's provenance no longer matches pins.json, so it cannot vouch for "+
			"anything: %v", err)
	}

	full, err := DeriveExpected(
		Expect{Kind: KindBuiltPolicyFull, HeldSlots: []int{0}}, in, seeds, bins)
	if err != nil {
		t.Fatalf("deriving the full-mode built policy: %v", err)
	}
	watch, err := DeriveExpected(
		Expect{Kind: KindBuiltPolicyWatch, HeldSlots: []int{0}}, in, seeds, bins)
	if err != nil {
		t.Fatalf("deriving the watch-only built policy: %v", err)
	}

	// The structural rules the untagged suite will apply to a committed
	// expectation must hold for what the derivation actually produces. Without
	// this the two could disagree and only S5's mint would find out.
	for _, c := range []struct {
		kind ExpectKind
		set  DerivedSet
	}{{KindBuiltPolicyFull, full}, {KindBuiltPolicyWatch, watch}} {
		if err := CheckArtifactShape(c.kind, c.set.Artifacts); err != nil {
			t.Errorf("%s: the live derivation does not satisfy its own shape rule: %v", c.kind, err)
		}
		if err := CheckFingerprintScope(c.kind, c.set.Artifacts); err != nil {
			t.Errorf("%s: the live derivation does not satisfy its own fingerprint rule: %v", c.kind, err)
		}
	}

	byKind := func(set DerivedSet, kind string) []Artifact {
		var out []Artifact
		for _, a := range set.Artifacts {
			if a.Kind == kind {
				out = append(out, a)
			}
		}
		return out
	}

	// THE CROSS-CHECK. Same wallet, different code path, byte for byte.
	gotMD := byKind(full, "md1")
	want := make([]string, len(gotMD))
	for i, a := range gotMD {
		want[i] = a.String
	}
	if err := CompareCensus(golden.Artifacts, want); err != nil {
		t.Fatalf("the built-policy derivation's md1 chunks are not S2's committed golden, so this "+
			"kind encodes a DIFFERENT wallet than the device does for the same inputs:\n%v", err)
	}

	// THE MODE DISTINCTION, in both directions: full carries exactly one ms1
	// (one distinct held master), watch-only carries none, and the public
	// artifacts are identical between them.
	if n := len(byKind(full, "ms1")); n != 1 {
		t.Errorf("full mode derived %d ms1(s) for 1 distinct held master", n)
	}
	if n := len(byKind(watch, "ms1")); n != 0 {
		t.Errorf("watch-only derived %d ms1(s); watch-only engraves none, and an ms1 nobody asked "+
			"for is a seed on steel", n)
	}
	for _, kind := range []string{"mk1", "md1"} {
		f, w := byKind(full, kind), byKind(watch, kind)
		if len(f) != len(w) {
			t.Errorf("%s count differs between modes: full %d, watch %d — the mode decides the ms1 "+
				"only", kind, len(f), len(w))
			continue
		}
		for i := range f {
			if f[i].String != w[i].String {
				t.Errorf("%s %d differs between modes:\n  full  %s\n  watch %s", kind, i, f[i].String, w[i].String)
			}
		}
	}

	t.Logf("built-policy-full: %d ms1 + %d mk1 + %d md1 = %d artifact(s); md1 byte-identical to "+
		"S2's committed golden; oracles invoked %v",
		len(byKind(full, "ms1")), len(byKind(full, "mk1")), len(gotMD), len(full.Artifacts), full.Oracles)
	t.Logf("built-policy-watch: %d artifact(s), no ms1", len(watch.Artifacts))
	for _, a := range full.Args {
		t.Logf("  arg form: %s", a)
	}
}
