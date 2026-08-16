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
	"encoding/json"
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

// ─── S5.0: divergent origins, through md's own encoder ──────────────────────

// mdPathDecl is the part of `md decode --json` this file reads back.
//
// `data` is json.RawMessage because md's path_decl is a TAGGED UNION whose
// payload changes SHAPE with the tag: Shared carries a string, Divergent carries
// an array of strings. A struct declaring either concrete type would fail to
// unmarshal the other, and — worse — a `data string` would leave the Divergent
// case as an empty string that an "is it what I expected" check could read as a
// benign zero value.
type mdPathDecl struct {
	Descriptor struct {
		PathDecl struct {
			Tag  string          `json:"tag"`
			Data json.RawMessage `json:"data"`
		} `json:"path_decl"`
	} `json:"descriptor"`
}

// mdDecodePathDecl asks the PINNED md what origins are actually in a chunk set.
//
// This is the assertion that cannot be fooled by the code under test: everything
// else about the mapping is a claim about the string policyTemplate built, and
// this reads the origins back out of the ENCODED BYTES, in md's own order. If
// the template builder and md disagreed about which placeholder a path belongs
// to, only this would see it.
//
// The `note:` lines md prints are on STDERR — measured on md 0.13.0, both for
// encode ("stdout is watch-only") and decode ("stdout is a keyless descriptor
// template") — so .Output() yields pure JSON. That is the same trap
// parseMdStdout exists for, checked rather than assumed.
func mdDecodePathDecl(t *testing.T, bin string, chunks []string) (tag string, paths []string) {
	t.Helper()
	out, err := exec.Command(bin, append(append([]string{"decode"}, chunks...), "--json")...).Output()
	if err != nil {
		t.Fatalf("md decode: %v", runErr(err))
	}
	var d mdPathDecl
	if err := json.Unmarshal(out, &d); err != nil {
		t.Fatalf("md decode --json emitted output this test cannot read (%v):\n%s", err, out)
	}
	tag = d.Descriptor.PathDecl.Tag
	switch tag {
	case "Divergent":
		if err := json.Unmarshal(d.Descriptor.PathDecl.Data, &paths); err != nil {
			t.Fatalf("path_decl is Divergent but its data is not a list of paths: %v", err)
		}
	case "Shared":
		var one string
		if err := json.Unmarshal(d.Descriptor.PathDecl.Data, &one); err != nil {
			t.Fatalf("path_decl is Shared but its data is not a path: %v", err)
		}
		paths = []string{one}
	default:
		t.Fatalf("md decode reported path_decl tag %q, which this test does not know how to read; "+
			"do NOT widen it to accept an unknown tag — the tag is what says whether the origins "+
			"are per-slot at all", tag)
	}
	return tag, paths
}

// TestBuiltPolicyDerivesDivergentOrigins is the live half of the 2026-08-15
// divergent-origin repair, and it asserts FOUR things that are separate
// failures.
//
// # Why it exists
//
// Until this change the built-policy kind refused any policy whose slots sat at
// different accounts, on a premise that was measured in md's DESCRIPTOR syntax
// (origin in brackets BEFORE the placeholder — genuinely rejected) and wrongly
// generalised to its TEMPLATE syntax (origin AFTER the placeholder — accepted,
// and the way a Divergent path declaration is expressed). S5's own md1 is a
// divergent policy: Trace B is one master at two accounts plus other masters. So
// the refusal was not conservative, it made S5's byte-identity gate underivable.
//
// # The four assertions, and what each one alone would MISS
//
// Each was checked against the mutation it is supposed to catch, and two of them
// were rewritten because the first version did not catch it.
//
//  1. IT DERIVES. A divergent-origin expectation produces a full artifact set.
//     Alone this passes on a silent flatten, which is why 2 and 3 exist.
//  2. IT DIFFERS, WITH THE XPUBS HELD CONSTANT. The obvious control — the same
//     masters at UNIFORM origins — is NOT sufficient, and that was measured
//     rather than reasoned: under a flatten mutation the two derivations still
//     differ, because each slot's xpub is derived at its own account and carries
//     it whether the template does or not. So the real control encodes the SAME
//     three xpubs under the divergent template and under a flattened one and
//     requires those to differ. That isolates the template.
//  3. THE MAPPING HOLDS THROUGH THE ENCODER. md decode reports path_decl
//     Divergent with slot i's origin at index i, read back out of the encoded
//     bytes rather than off the string this package built. This is the assertion
//     the adversarial review of md's own support found missing: mutating the
//     origin vector to `(0..n).rev()` left an entire suite green, because every
//     check asked whether both origins were PRESENT. It needs a NON-PALINDROMIC
//     fixture to mean anything — see the origins below.
//  4. S5's OWN SHAPE derives: one master at two accounts plus another master,
//     which is the only case where two slots share a fingerprint and differ in
//     path, and the case the distinct-master ms1 rule exists for.
func TestBuiltPolicyDerivesDivergentOrigins(t *testing.T) {
	bins := resolveBins(t)

	// BIP-39's own published vectors, the same three the S0 inputs file commits.
	seeds := []Seed{
		{Label: "masterA", Words: "abandon abandon abandon abandon abandon " +
			"abandon abandon abandon abandon abandon abandon about"},
		{Label: "masterB", Words: "legal winner thank year wave sausage " +
			"worth useful legal winner thank yellow"},
		{Label: "masterC", Words: "letter advice cage absurd amount doctor " +
			"acoustic avoid letter advice cage above"},
	}
	// THREE DISTINCT ACCOUNTS, and the distinctness is load-bearing rather than
	// decorative. Measured: with the palindromic vector this fixture first used
	// — (0,1,0), which is S5's Trace B shape — the mutation that reverses the
	// origin-to-placeholder mapping left this whole test GREEN, because a
	// palindrome is its own reverse and md's path_decl came back identical. A
	// fixture that cannot distinguish a permutation cannot assert a mapping.
	//
	// Trace B's real (0,1,0) shape is still covered, in section 4 below, where
	// what is being asserted is the same-master-two-accounts property rather than
	// the ordering.
	uniformOrigins := []string{"m/48h/0h/0h/2h", "m/48h/0h/0h/2h", "m/48h/0h/0h/2h"}
	divergentOrigins := []string{"m/48h/0h/0h/2h", "m/48h/0h/1h/2h", "m/48h/0h/2h/2h"}

	tuple := func(origins []string) InputTuple {
		return InputTuple{
			Template:  "built-policy: 2-of-3 wsh",
			N:         3,
			K:         2,
			SlotOrder: []int{0, 1, 2},
			Origins:   origins,
		}
	}
	md1Of := func(set DerivedSet) []string {
		var out []string
		for _, a := range set.Artifacts {
			if a.Kind == "md1" {
				out = append(out, a.String)
			}
		}
		return out
	}

	// ── 1. IT DERIVES ───────────────────────────────────────────────────────
	div, err := DeriveExpected(
		Expect{Kind: KindBuiltPolicyFull, HeldSlots: []int{0}}, tuple(divergentOrigins), seeds, bins)
	if err != nil {
		t.Fatalf("a divergent-origin policy did not derive. This is the defect the 2026-08-15 "+
			"repair exists to remove, and S5's md1 needs it:\n%v", err)
	}
	if err := CheckArtifactShape(KindBuiltPolicyFull, div.Artifacts); err != nil {
		t.Errorf("the divergent derivation does not satisfy its own shape rule: %v", err)
	}
	if err := CheckFingerprintScope(KindBuiltPolicyFull, div.Artifacts); err != nil {
		t.Errorf("the divergent derivation does not satisfy its own fingerprint rule: %v", err)
	}
	divMD := md1Of(div)
	if len(divMD) == 0 {
		t.Fatal("the divergent derivation produced no md1 chunk at all")
	}

	uni, err := DeriveExpected(
		Expect{Kind: KindBuiltPolicyFull, HeldSlots: []int{0}}, tuple(uniformOrigins), seeds, bins)
	if err != nil {
		t.Fatalf("the UNIFORM control did not derive, so nothing below can be compared: %v", err)
	}
	uniMD := md1Of(uni)

	sameChunks := func(a, b []string) bool {
		if len(a) != len(b) {
			return false
		}
		for i := range a {
			if a[i] != b[i] {
				return false
			}
		}
		return true
	}

	// ── 2. IT DIFFERS — AND THE COMPARISON THAT PROVES IT IS NOT THE OBVIOUS ONE
	//
	// The obvious control is the same masters at UNIFORM origins, and it is NOT
	// SUFFICIENT. Measured, not assumed: under a mutation that flattens every
	// slot's origin to slot 0's in policyTemplate, the two derivations STILL
	// produce different md1, because each slot's XPUB is derived at its own
	// account and therefore carries that account whether the template does or
	// not. A difference here proves the two TUPLES are different wallets; it does
	// not prove the origins reached md.
	//
	// So the control that isolates the TEMPLATE holds the xpubs FIXED and varies
	// only the origins written into it: the same three account xpubs, encoded
	// once under the divergent template and once under the flattened one. Those
	// may not be equal. Under the flatten mutation they are, and this is the
	// assertion that says so.
	xpubs := make([]string, len(divergentOrigins))
	for i, o := range divergentOrigins {
		acct, msTmpl, net, err := templateForOrigin(o)
		if err != nil {
			t.Fatalf("slot %d: %v", i, err)
		}
		_, xpub, _, err := msDerive(bins.MS, seeds[i].Words, msTmpl, acct, net)
		if err != nil {
			t.Fatalf("slot %d: %v", i, err)
		}
		xpubs[i] = xpub
	}
	flatOrigins := []string{divergentOrigins[0], divergentOrigins[0], divergentOrigins[0]}
	flatTemplate, err := policyTemplate("bip48-p2wsh", 2, flatOrigins)
	if err != nil {
		t.Fatalf("building the flattened control template: %v", err)
	}
	flatMD, _, err := mdEncode(bins.MD, flatTemplate, xpubs, "mainnet")
	if err != nil {
		t.Fatalf("encoding the flattened control: %v", err)
	}
	if sameChunks(divMD, flatMD) {
		t.Errorf("THE ORIGINS WERE FLATTENED. Encoding the SAME three xpubs under the divergent "+
			"template and under a template with every slot at %q produced byte-identical md1, so "+
			"the per-slot accounts never reached md and this expectation describes a different "+
			"wallet than its own tuple.\n  divergent origins %v\n  flattened to      %v\n"+
			"  flattened template %q\n  chunks             %q",
			divergentOrigins[0], divergentOrigins, flatOrigins, flatTemplate, divMD)
	} else {
		t.Logf("divergent md1 differs from the SAME xpubs under a flattened template — the " +
			"per-slot origins reached md")
	}

	// The weaker statement, kept because it is still true and cheap: a different
	// tuple is a different wallet. Named for what it proves, so nobody mistakes
	// it for the anti-flatten check above.
	if sameChunks(divMD, uniMD) {
		t.Errorf("the divergent policy's md1 is byte-identical to the same masters at uniform "+
			"origins %v, which are different wallets", uniformOrigins)
	}
	t.Logf("divergent md1 (%d chunk(s)); uniform control (%d chunk(s))", len(divMD), len(uniMD))
	for i, c := range divMD {
		t.Logf("  divergent chunk %d: %s", i, c)
	}

	// ── 3. THE MAPPING, READ BACK OUT OF THE ENCODED BYTES ──────────────────
	//
	// THE FIXTURE'S OWN GUARD FIRST. A vector that is its own reverse cannot
	// detect a permutation, and this test's first version used one: with
	// (acct0, acct1, acct0) the `(0..n).rev()` mutation left every assertion
	// below green, because md's path_decl came back byte-identical. If somebody
	// later "simplifies" these origins back to a palindrome, everything below
	// still passes and this fails, naming the reason.
	palindrome := true
	for i := range divergentOrigins {
		if !samePath(divergentOrigins[i], divergentOrigins[len(divergentOrigins)-1-i]) {
			palindrome = false
			break
		}
	}
	if palindrome {
		t.Errorf("the divergent fixture %v is its own reverse, so nothing below can distinguish "+
			"slot i's origin landing on @i from the whole vector being permuted. Measured: with a "+
			"palindromic vector, reversing the origin-to-placeholder mapping left this entire test "+
			"GREEN. Use distinct accounts.", divergentOrigins)
	}

	tag, paths := mdDecodePathDecl(t, bins.MD, divMD)
	if tag != "Divergent" {
		t.Fatalf("md decode reports path_decl tag %q for a policy whose slots sit at different "+
			"accounts, want \"Divergent\". A Shared declaration here means the per-slot origins "+
			"were collapsed to one path.", tag)
	}
	if len(paths) != len(divergentOrigins) {
		t.Fatalf("md decode reports %d origin(s) for a %d-slot policy: %v",
			len(paths), len(divergentOrigins), paths)
	}
	for i, want := range divergentOrigins {
		if !samePath(paths[i], want) {
			t.Errorf("SLOT %d's origin did not land on @%d IN THE ENCODED BYTES.\n"+
				"  md decode reports path_decl.data[%d] = %q\n"+
				"  the tuple records slot %d at        %q\n"+
				"A policy whose origins are permuted is a DIFFERENT WALLET in the same funds "+
				"class. Every assertion above would still pass — this is the only one that sees it.",
				i, i, i, paths[i], i, want)
		}
	}

	// THE CONTROL FOR ASSERTION 3, so a comparison that could not fail is
	// visible: the uniform tuple must decode as SHARED. If mdDecodePathDecl or
	// samePath were vacuous, this would report Divergent-or-anything and pass.
	uniTag, uniPaths := mdDecodePathDecl(t, bins.MD, uniMD)
	if uniTag != "Shared" {
		t.Errorf("the uniform control decodes as path_decl %q, want \"Shared\" — the tag check "+
			"above cannot distinguish anything if every policy is Divergent", uniTag)
	}
	if len(uniPaths) != 1 || !samePath(uniPaths[0], uniformOrigins[0]) {
		t.Errorf("the uniform control's shared path decodes as %v, want %q", uniPaths, uniformOrigins[0])
	}

	t.Logf("divergent path_decl: %s %v", tag, paths)
	t.Logf("uniform   path_decl: %s %v", uniTag, uniPaths)
	for _, a := range div.Args {
		t.Logf("  arg form: %s", a)
	}

	// ── 4. S5's ACTUAL SHAPE: ONE MASTER AT TWO ACCOUNTS ────────────────────
	//
	// The three assertions above vary ONE account across three DISTINCT masters,
	// which is the cleanest isolation of the origin but is not the tuple S5
	// engraves. Trace B is one master at two accounts plus other masters, and
	// that shape has a property none of the above reaches: two slots sharing a
	// FINGERPRINT while sitting at different paths. In full mode it is also the
	// case the distinct-master rule exists for — two held slots, one seed, and
	// engraving that seed twice would be a plate of pure risk.
	traceB := []Seed{seeds[0], seeds[0], seeds[1]} // masterA, masterA, masterB
	traceBOrigins := []string{"m/48h/0h/0h/2h", "m/48h/0h/1h/2h", "m/48h/0h/0h/2h"}
	tb, err := DeriveExpected(
		Expect{Kind: KindBuiltPolicyFull, HeldSlots: []int{0, 1}}, tuple(traceBOrigins), traceB, bins)
	if err != nil {
		t.Fatalf("S5's own shape — one master at two accounts plus another master — did not "+
			"derive. This is the tuple the removed refusal made underivable:\n%v", err)
	}
	if err := CheckArtifactShape(KindBuiltPolicyFull, tb.Artifacts); err != nil {
		t.Errorf("the Trace B derivation does not satisfy its own shape rule: %v", err)
	}
	if err := CheckFingerprintScope(KindBuiltPolicyFull, tb.Artifacts); err != nil {
		t.Errorf("the Trace B derivation does not satisfy its own fingerprint rule: %v", err)
	}

	var ms1s, mk1s []Artifact
	for _, a := range tb.Artifacts {
		switch a.Kind {
		case "ms1":
			ms1s = append(ms1s, a)
		case "mk1":
			mk1s = append(mk1s, a)
		}
	}
	// TWO held slots, ONE distinct master, so exactly ONE ms1. Keyed on the
	// WORDS in deriveBuiltPolicy; this is the live proof that divergent origins
	// did not defeat that dedup by making the two slots look distinct.
	if len(ms1s) != 1 {
		t.Errorf("full mode derived %d ms1(s) for 2 held slots that are the SAME master at two "+
			"accounts; want 1 — a second secret plate is a seed on steel for no gain", len(ms1s))
	}
	if len(mk1s) < 2 {
		t.Errorf("derived %d mk1 artifact(s) for 2 held slots; each held slot engraves a key card",
			len(mk1s))
	}
	// THE PROPERTY ONLY THIS SHAPE HAS: same fingerprint, different origin. If
	// the origins had been flattened, both cards would read the same path and
	// this would be two identical cards for one wallet leg.
	if len(mk1s) >= 2 {
		if mk1s[0].Fingerprint != mk1s[len(mk1s)-1].Fingerprint {
			t.Errorf("the two held slots are the same master, so their key cards must carry one "+
				"fingerprint; got %q and %q", mk1s[0].Fingerprint, mk1s[len(mk1s)-1].Fingerprint)
		}
		if samePath(mk1s[0].Origin, mk1s[len(mk1s)-1].Origin) {
			t.Errorf("the two held slots sit at DIFFERENT accounts (%q and %q) but their key cards "+
				"both record %q — the per-slot origins were flattened",
				traceBOrigins[0], traceBOrigins[1], mk1s[0].Origin)
		}
	}

	tbTag, tbPaths := mdDecodePathDecl(t, bins.MD, md1Of(tb))
	if tbTag != "Divergent" {
		t.Errorf("Trace B's md1 decodes as path_decl %q, want \"Divergent\"", tbTag)
	}
	for i, want := range traceBOrigins {
		if i < len(tbPaths) && !samePath(tbPaths[i], want) {
			t.Errorf("Trace B slot %d: md1 encodes %q, tuple records %q", i, tbPaths[i], want)
		}
	}
	t.Logf("Trace B (one master at two accounts + another master): %d ms1 + %d mk1 + %d md1; "+
		"path_decl %s %v", len(ms1s), len(mk1s), len(md1Of(tb)), tbTag, tbPaths)
	t.Logf("  slot 0 card: fp %s at %s", mk1s[0].Fingerprint, mk1s[0].Origin)
	t.Logf("  slot 1 card: fp %s at %s", mk1s[len(mk1s)-1].Fingerprint, mk1s[len(mk1s)-1].Origin)
}
