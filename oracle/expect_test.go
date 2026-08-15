package oracle

// THE BYTE-IDENTITY GATE. It compares every gate record's engraved census
// against the expectation committed beside it, needs no toolchain, and HAS NO
// SKIP PATH — absent, empty or unparseable is a fatal INCONCLUSIVE. It runs on
// every machine, including the one whose verdict gates a merge, which is the
// entire repair C-1 through C-4 called for.
//
// # There is no test in this package that decides for itself whether to run
//
// Operator directive, 2026-08-15: "Don't skip jobs unless I ask." So the live
// re-derivation against the pinned md/mk/ms binaries is NOT a test that skips
// when they are absent, and it is not a test gated on an environment variable
// either — an escalation nobody sets and an opt-out everybody sets are the same
// failure. It lives in live_test.go behind the `oraclelive` build tag, so in a
// normal build it DOES NOT EXIST rather than reporting that it declined to run,
// and a human turns it on by name:
//
//	./scripts/oracle-live.sh          # or: go test -tags oraclelive ./...
//
// What makes that safe is that live derivation is enforced where it actually
// matters — at MINT time, unconditionally, in cmd/gaterecord, which refuses to
// write a record whose census is not what it just derived. An expectation
// cannot exist except as the output of a live run, and CheckProvenance below
// binds every one of them to pins.json with no toolchain at all.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ─── LAYER 1: no toolchain, no skip path ────────────────────────────────────

// loadExpectations returns every gate record on disk paired with the
// expectation committed beside it, and FAILS on any record that has none.
//
// This is C-2's fix: the comparison used to be hardwired to the single filename
// "S0-trace-a.record.json", so a later stage could mint a record holding six
// invented strings and no test would ever open it. The loop is over the
// DIRECTORY, and a record without an expectation is a failure rather than a
// record the loop happens not to cover.
func loadExpectations(t *testing.T) []struct {
	Name   string
	Record GateRecord
	Expect ExpectFile
} {
	t.Helper()
	names, err := Records(GateRecordsDir)
	if err != nil {
		t.Fatalf("reading %s: %v", GateRecordsDir, err)
	}
	if len(names) == 0 {
		t.Fatalf("INCONCLUSIVE: %s holds no %s file, so this gate compares nothing",
			GateRecordsDir, RecordSuffix)
	}
	var out []struct {
		Name   string
		Record GateRecord
		Expect ExpectFile
	}
	for _, n := range names {
		rec, err := LoadRecord(filepath.Join(GateRecordsDir, n))
		if err != nil {
			t.Fatalf("loading %s: %v", n, err)
		}
		ep := ExpectPathFor(GateRecordsDir, n)
		ef, err := LoadExpect(ep)
		if err != nil {
			t.Fatalf("gate record %s has no usable committed expectation at %s: %v\n"+
				"A record nothing compares is not evidence. Mint it with cmd/gaterecord, "+
				"which derives the expectation live and refuses to write a record whose "+
				"census disagrees with it.", n, ep, err)
		}
		out = append(out, struct {
			Name   string
			Record GateRecord
			Expect ExpectFile
		}{n, rec, ef})
	}
	return out
}

// THE GATE (F-170 + F-171, C-2). What the inputs REQUIRE, computed by the
// primary toolchain and committed, must equal what the device engraved — byte
// for byte, in order, for EVERY record on disk.
//
// No oracle is invoked here and none can be: that is what makes this arm
// unskippable, and it is the only reason the property is enforced anywhere.
func TestEveryGateRecordCensusMatchesItsCommittedExpectation(t *testing.T) {
	for _, c := range loadExpectations(t) {
		if err := CompareCensus(c.Expect.Artifacts, c.Record.Walk.Census.Strings); err != nil {
			// Name what disagreed with what. CompareCensus's own wrapper error
			// speaks of "the walk beside it", which is the wrong pair here.
			t.Errorf("%s: the engraved census is not what the primary toolchain derived "+
				"(committed in %s):\n%v", c.Name,
				ExpectPathFor(GateRecordsDir, c.Name), err)
			continue
		}
		t.Logf("%s: %d committed artifact(s) matched the engraved census byte for byte",
			c.Name, len(c.Expect.Artifacts))
	}
}

// Every expectation must belong to a record, and every record's expectation
// must name it. An ORPHAN expectation makes the directory look better covered
// than it is, and a mislabelled one vouches for a run it did not come from.
func TestEveryCommittedExpectationBelongsToARecord(t *testing.T) {
	exps, err := Expectations(GateRecordsDir)
	if err != nil {
		t.Fatalf("reading %s: %v", GateRecordsDir, err)
	}
	if len(exps) == 0 {
		t.Fatalf("INCONCLUSIVE: %s holds no %s file", GateRecordsDir, ExpectSuffix)
	}
	for _, n := range exps {
		base := strings.TrimSuffix(n, ExpectSuffix)
		if _, err := os.Stat(filepath.Join(GateRecordsDir, base+RecordSuffix)); err != nil {
			t.Errorf("%s is an orphan: no %s beside it (%v)", n, base+RecordSuffix, err)
		}
	}
	for _, c := range loadExpectations(t) {
		base := strings.TrimSuffix(c.Name, RecordSuffix)
		if c.Expect.Record != base+RecordSuffix {
			t.Errorf("%s's expectation names record %q, but it sits beside %q",
				c.Name, c.Expect.Record, c.Name)
		}
		if c.Expect.Stage != c.Record.Stage {
			t.Errorf("%s: the record is stage %q, its expectation says %q",
				c.Name, c.Record.Stage, c.Expect.Stage)
		}

		// RE-REVIEW I-1. Naming a record is not enough: a hand-authored
		// expectation that names a real record, with a provenance block copied
		// from an honest one, passed this whole suite green while logging
		// "6 committed artifact(s) matched the engraved census byte for byte"
		// about six invented strings. The only check that caught it was behind
		// the `oraclelive` tag, which nothing routinely runs.
		//
		// Nothing toolchain-free can make hand-authoring IMPOSSIBLE. What it can
		// do is force a committed expectation to be SELF-DESCRIBING and
		// one-command re-derivable: it must name the inputs it came from, that
		// file must exist and parse, and it must declare a kind this package can
		// actually derive, producing the artifact kind it claims. A fabricator
		// then has to forge an inputs file that the live arm will contradict the
		// moment anyone runs it, instead of forging six strings nothing reads.
		if c.Expect.Inputs == "" {
			t.Errorf("%s's expectation names no inputs file, so nothing can ever "+
				"re-derive it and the committed strings vouch only for themselves",
				c.Name)
			continue
		}
		inf, err := LoadInputsFile(filepath.Join(GateRecordsDir, c.Expect.Inputs))
		if err != nil {
			t.Errorf("%s's expectation names inputs %q, which does not load: %v",
				c.Name, c.Expect.Inputs, err)
			continue
		}
		if inf.Expect == nil {
			t.Errorf("%s: inputs %q carries no expect block, so the expectation "+
				"beside it describes a derivation nobody specified",
				c.Name, c.Expect.Inputs)
			continue
		}
		if !KnownExpectKind(inf.Expect.Kind) {
			t.Errorf("%s: inputs %q declares expectation kind %q, which "+
				"DeriveExpected cannot derive — a committed expectation for a "+
				"kind nothing derives can never be checked against anything",
				c.Name, c.Expect.Inputs, inf.Expect.Kind)
			continue
		}
		// The artifacts must be the kind that kind produces. The fabrication
		// that motivated this set md1 artifacts against a cosigner-cards inputs
		// block, and nothing noticed.
		want := ArtifactKindFor(inf.Expect.Kind)
		for i, a := range c.Expect.Artifacts {
			if a.Kind != want {
				t.Errorf("%s: artifact %d is kind %q, but inputs %q declares %q, "+
					"which produces %q",
					c.Name, i, a.Kind, c.Expect.Inputs, inf.Expect.Kind, want)
			}
		}
	}
}

// THE STALENESS BINDING (ruling item 6), and it needs no toolchain either.
// Every committed expectation's recorded oracle identity must still equal
// pins.json — commit and binary hash. Bump a pin without re-minting and this
// goes red on any machine, including the one that decides a merge.
func TestVendoredExpectationsWereDerivedFromThePinnedToolchain(t *testing.T) {
	pf, err := LoadPins("pins.json")
	if err != nil {
		t.Fatalf("loading pins: %v", err)
	}
	for _, c := range loadExpectations(t) {
		if err := c.Expect.CheckProvenance(pf); err != nil {
			t.Errorf("%s: %v", c.Name, err)
			continue
		}
		var names []string
		for _, o := range c.Expect.Derivation.Oracles {
			names = append(names, o.Name+"@"+o.Commit[:8])
		}
		t.Logf("%s: derived by %s, all still pinned", c.Name, strings.Join(names, " "))
	}
}

// The COUNT is derived, not a literal. This is what replaces `plates = 6`: the
// number six comes from three seeds x two chunks, computed by mk at mint time
// and committed. Runs with no toolchain, so it is enforced where it matters.
func TestPlateCountIsDerivedFromTheInputs(t *testing.T) {
	for _, c := range loadExpectations(t) {
		n := len(c.Expect.Artifacts)
		if n != len(c.Record.Walk.Census.Strings) {
			t.Errorf("%s: the inputs require %d plate(s); the record holds %d",
				c.Name, n, len(c.Record.Walk.Census.Strings))
		}
		if n != len(c.Record.Walk.Digests) {
			t.Errorf("%s: %d derived artifact(s) but %d plate digest(s)",
				c.Name, n, len(c.Record.Walk.Digests))
		}
	}
}

// The oracle vouches for the SEED->KEY step independently of the xpub: every
// derived card carries the master fingerprint ms computed from that seed's
// words. Without this the comparison would prove only that mk encodes
// consistently, not that the right key was engraved.
//
// The committed fingerprints are the ones ms produced (nothing else can mint
// them — see expectfile.go); this arm asserts they are PRESENT and DISTINCT, and
// the live arm below asserts they are still what ms derives today.
func TestCommittedFingerprintsAreRealAndDistinct(t *testing.T) {
	for _, c := range loadExpectations(t) {
		seen := map[string]bool{}
		for _, a := range c.Expect.Artifacts {
			if a.Fingerprint == "" {
				t.Errorf("%s: artifact %q carries no fingerprint", c.Name, a.Label)
				continue
			}
			seen[a.Fingerprint] = true
		}
		if len(seen) < 2 {
			t.Errorf("%s: only %d distinct fingerprint(s) across the whole census — the cards "+
				"are not from distinct seeds, so this gate is weaker than it looks", c.Name, len(seen))
		}
	}
}

// ─── The three mutation proofs, which now run WITHOUT the toolchain ─────────
//
// C-1's sharpest consequence was that the skip took out the gate AND all three
// of its own mutation proofs at once — the mechanism was never seen to fail on
// the machine that decides. They are driven off the committed expectation now,
// so they execute everywhere the gate does.

// MUTATION 1 — one expected string. Required by the stage's gate: a mechanism
// that has not been seen to fail does not leave this stage.
func TestCompareCensusCatchesAMutatedString(t *testing.T) {
	want := loadExpectations(t)[0].Expect.Artifacts
	got := make([]string, len(want))
	for i, a := range want {
		got[i] = a.String
	}
	if err := CompareCensus(want, got); err != nil {
		t.Fatalf("control failed before any mutation: %v", err)
	}
	// Flip ONE character of ONE plate — the smallest defect the gate must catch.
	const plate = 2
	if len(got) <= plate {
		t.Fatalf("INCONCLUSIVE: the expectation holds %d plate(s), so plate %d cannot be mutated",
			len(got), plate)
	}
	orig := got[plate]
	b := []byte(orig)
	if b[len(b)-1] == 'q' {
		b[len(b)-1] = 'p'
	} else {
		b[len(b)-1] = 'q'
	}
	got[plate] = string(b)
	err := CompareCensus(want, got)
	if err == nil {
		t.Fatal("a one-character mutation in plate 2 was accepted; the byte comparison is not comparing")
	}
	if !strings.Contains(err.Error(), "plate 2") {
		t.Errorf("the refusal must name the plate that differs; got: %v", err)
	}
	// And both strings in full — a truncated message renders them identical.
	if !strings.Contains(err.Error(), orig) || !strings.Contains(err.Error(), got[plate]) {
		t.Errorf("the refusal must print BOTH strings in full; got: %v", err)
	}
}

// MUTATION 2 — a dropped plate. A partial walk may never satisfy a total gate.
func TestCompareCensusCatchesAShortCensus(t *testing.T) {
	want := loadExpectations(t)[0].Expect.Artifacts
	got := make([]string, 0, len(want))
	for _, a := range want {
		got = append(got, a.String)
	}
	if err := CompareCensus(want, got[:len(got)-1]); err == nil {
		t.Fatal("a census one plate short was accepted")
	}
}

// MUTATION 3 — reordered plates. Order is identity-bearing: a set that arrives
// in a different order is a different restore.
func TestCompareCensusCatchesReorderedPlates(t *testing.T) {
	want := loadExpectations(t)[0].Expect.Artifacts
	got := make([]string, len(want))
	for i, a := range want {
		got[i] = a.String
	}
	got[0], got[2] = got[2], got[0]
	if err := CompareCensus(want, got); err == nil {
		t.Fatal("a reordered census was accepted; the comparison is order-blind")
	}
}

// An EMPTY comparison must fail, not pass vacuously. This is the false-PASS
// shape the whole stage exists to remove.
func TestCompareCensusRefusesToPassOnNothing(t *testing.T) {
	if err := CompareCensus(nil, nil); err == nil {
		t.Fatal("comparing nothing to nothing reported success")
	}
}

// And the loader's own vacuity floor: an expectation holding no artifacts, or
// one holding empty strings, must be REFUSED at load rather than compared.
// Without this the vendored layer would inherit exactly the disease it replaced.
func TestLoadExpectRefusesAnEmptyExpectation(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ name, body string }{
		{"no artifacts", `{"schema":1,"stage":"S0","derivation":{"oracles":[{"name":"mk","commit":"a38a908e143c2c4bd6405997d62385b3df01615f","sha256":"x"}],"args":[],"derived_at":"now"},"artifacts":[]}`},
		{"empty string", `{"schema":1,"stage":"S0","derivation":{"oracles":[{"name":"mk","commit":"a38a908e143c2c4bd6405997d62385b3df01615f","sha256":"x"}],"args":[],"derived_at":"now"},"artifacts":[{"kind":"mk1","label":"l","string":""}]}`},
		{"no oracle", `{"schema":1,"stage":"S0","derivation":{"oracles":[],"args":[],"derived_at":"now"},"artifacts":[{"kind":"mk1","label":"l","string":"mk1x"}]}`},
		{"wrong schema", `{"schema":99,"stage":"S0","derivation":{"oracles":[],"args":[],"derived_at":"now"},"artifacts":[{"kind":"mk1","label":"l","string":"mk1x"}]}`},
		{"unknown field", `{"schema":1,"stage":"S0","surprise":1,"derivation":{"oracles":[],"args":[],"derived_at":"now"},"artifacts":[{"kind":"mk1","label":"l","string":"mk1x"}]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := filepath.Join(dir, "x"+ExpectSuffix)
			if err := os.WriteFile(p, []byte(tc.body), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := LoadExpect(p); err == nil {
				t.Fatal("accepted an expectation that compares nothing")
			}
		})
	}
	// And an ABSENT one, which is the case CI hits if a record ever lands
	// without its expectation.
	if _, err := LoadExpect(filepath.Join(dir, "not-there"+ExpectSuffix)); err == nil {
		t.Fatal("a missing expectation was accepted")
	}
}

// An unknown expectation kind must REFUSE rather than derive an empty set that
// then "matches" an empty census.
func TestDeriveRefusesAnUnknownKind(t *testing.T) {
	_, err := DeriveExpected(Expect{Kind: "no-such-kind", PolicyIDStub: "5b48af35"},
		InputTuple{Origins: []string{"m/48h/0h/0h/2h"}}, []Seed{{Label: "x", Words: "y"}}, Bins{})
	if err == nil {
		t.Fatal("an unknown expectation kind was accepted")
	}
}

// The recorded origin is CHECKED against what the template derives. A record
// whose origins drifted from its key material must not pass.
func TestDeriveRefusesAnOriginItCannotName(t *testing.T) {
	for _, bad := range []string{
		"m/84h/0h/0h",    // single-sig purpose: not a cosigner card
		"m/48h/0h/0h",    // BIP-48 with no script_type level
		"m/48h/0h/0h/3h", // script_type BIP-48 does not register
		"m/48h/7h/0h/2h", // coin_type with no oracle
		"not-a-path",     //
	} {
		if _, _, _, err := templateForOrigin(bad); err == nil {
			t.Errorf("origin %q was accepted", bad)
		}
	}
	// And the two registered script types map to the two templates.
	for path, wantTmpl := range map[string]string{
		"m/48h/0h/0h/2h": "bip48-p2wsh",
		"m/48'/0'/0'/1'": "bip48-p2sh-p2wsh",
	} {
		acct, tmpl, net, err := templateForOrigin(path)
		if err != nil {
			t.Errorf("origin %q: %v", path, err)
			continue
		}
		if tmpl != wantTmpl || acct != 0 || net != "mainnet" {
			t.Errorf("origin %q -> (%d, %q, %q), want (0, %q, mainnet)", path, acct, tmpl, net, wantTmpl)
		}
	}
}
