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
	"strconv"
	"strings"
	"testing"
)

// ─── LAYER 1: no toolchain, no skip path ────────────────────────────────────

// expectCase is one gate record on disk with the expectation committed beside
// it. A named type rather than an anonymous struct because several tests need
// to pass one to a helper, and an anonymous struct cannot be named in a
// signature.
type expectCase struct {
	Name   string
	Record GateRecord
	Expect ExpectFile
}

// loadExpectations returns every gate record on disk paired with the
// expectation committed beside it, and FAILS on any record that has none.
//
// This is C-2's fix: the comparison used to be hardwired to the single filename
// "S0-trace-a.record.json", so a later stage could mint a record holding six
// invented strings and no test would ever open it. The loop is over the
// DIRECTORY, and a record without an expectation is a failure rather than a
// record the loop happens not to cover.
func loadExpectations(t *testing.T) []expectCase {
	t.Helper()
	names, err := Records(GateRecordsDir)
	if err != nil {
		t.Fatalf("reading %s: %v", GateRecordsDir, err)
	}
	if len(names) == 0 {
		t.Fatalf("INCONCLUSIVE: %s holds no %s file, so this gate compares nothing",
			GateRecordsDir, RecordSuffix)
	}
	var out []expectCase
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
		out = append(out, expectCase{n, rec, ef})
	}
	return out
}

// kindOf resolves the ExpectKind an on-disk case declares, from the INPUTS file
// rather than from the expectation.
//
// Deliberately from the inputs: the expectation is the derivation's OUTPUT, and
// an output that declared its own kind could not contradict itself. The inputs
// file is what a human wrote, so it is the side that can be wrong — which is the
// side worth checking against.
func kindOf(t *testing.T, c expectCase) (ExpectKind, bool) {
	t.Helper()
	if c.Expect.Inputs == "" {
		return "", false
	}
	inf, err := LoadInputsFile(filepath.Join(GateRecordsDir, c.Expect.Inputs))
	if err != nil {
		t.Errorf("%s: loading inputs %q: %v", c.Name, c.Expect.Inputs, err)
		return "", false
	}
	if inf.Expect == nil {
		return "", false
	}
	return inf.Expect.Kind, true
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
		// The artifacts must have the SHAPE that kind produces — every kind it
		// declares present, in the declared engrave order, and nothing else.
		// The fabrication that motivated this set md1 artifacts against a
		// cosigner-cards inputs block, and nothing noticed. Extended for the
		// multi-kind answer: a single-kind equality check cannot see a
		// full-mode expectation that lost its ms1 plates, nor one whose secret
		// plate migrated to the end of the set.
		if err := CheckArtifactShape(inf.Expect.Kind, c.Expect.Artifacts); err != nil {
			t.Errorf("%s: the committed artifacts do not have the shape inputs %q declares "+
				"(kind %q -> %v):\n%v",
				c.Name, c.Expect.Inputs, inf.Expect.Kind,
				ArtifactKindsFor(inf.Expect.Kind), err)
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
//
// SCOPED BY KIND, per the S0b fold re-review's M-3. The rule used to be "every
// artifact carries a fingerprint, and at least two are distinct", which is right
// for cosigner cards and wrong twice over for a built policy: an md1 chunk
// belongs to no single master and has no fingerprint to carry, and an ordinary
// single-self-slot build honestly has ONE master. Both would have failed a
// correct S5 record at its first artifact. CheckFingerprintScope holds the
// per-kind rule so the same statement is testable directly, in both directions,
// before any built-policy record exists on disk to carry it.
func TestCommittedFingerprintsAreRealAndDistinct(t *testing.T) {
	for _, c := range loadExpectations(t) {
		kind, ok := kindOf(t, c)
		if !ok {
			t.Errorf("%s: no inputs kind, so no fingerprint rule can be chosen for it "+
				"(TestEveryCommittedExpectationBelongsToARecord says why that is fatal)", c.Name)
			continue
		}
		if err := CheckFingerprintScope(kind, c.Expect.Artifacts); err != nil {
			t.Errorf("%s (kind %q): %v", c.Name, kind, err)
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
//
// IT ASSERTS ON THE MESSAGE, and that is a mutation finding rather than a
// stylistic choice. There are now TWO refusals in this path — KnownExpectKind at
// the top, and the `default` arm of DeriveExpected's switch, which exists so a
// kind added to ArtifactKindsFor and not to the switch cannot derive nothing.
// Measured: deleting the FIRST one left this test green, because the second
// caught the same input. Belt and braces is right; a test that cannot tell which
// one fired proves only that one of them exists, and would stay green while the
// first was removed and the second later "simplified" away.
func TestDeriveRefusesAnUnknownKind(t *testing.T) {
	_, err := DeriveExpected(Expect{Kind: "no-such-kind", PolicyIDStub: "5b48af35"},
		InputTuple{Origins: []string{"m/48h/0h/0h/2h"}}, []Seed{{Label: "x", Words: "y"}}, Bins{})
	if err == nil {
		t.Fatal("an unknown expectation kind was accepted")
	}
	if !strings.Contains(err.Error(), "unknown expectation kind") {
		t.Fatalf("the refusal did not come from the KnownExpectKind check at the top of "+
			"DeriveExpected; got: %v", err)
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

// ─── S5.0: the BUILT-POLICY kind, and the traps it had to be built around ────
//
// Everything below runs UNTAGGED, with no toolchain. That is deliberate and it
// is the whole reason the md/ms stdout parsers are separate functions rather
// than loops inside the exec calls: a refusal that can only be demonstrated on a
// machine with three Rust binaries installed is a refusal the deciding machine
// has never seen fail.
//
// The captured stdout below is REAL — printed by the pinned md 0.13.0 and
// ms 0.16.0 on 2026-08-15 for the three BIP-39 published-vector masters at
// m/48'/0'/0'/2', 2-of-3 wsh — not invented to make a parser pass.

// mdRealStdout is exactly what `md encode … --group-size 0 --force-chunked
// --policy-id-fingerprint` printed for Trace A's policy. Note the header line
// FIRST and the policy-id line LAST: the artifacts are in the middle, which is
// why "skip the first line" would have been a wrong fix.
const mdRealStdout = `chunk-set-id: 0x30d86
md1fxrvxzspqjtvyyy4qqxppcgsc27rchwsv0jskp2rsal4egz4ep5859p875x67p5s5tk09nzz08lv4
md1fxrvxzsg3wem7sgluxl3d2a3syx3m7halwd7s7d5e8l2xm3y3xzfmadfj6e20urgnf9h0eap9ujpc
md1fxrvxzshanz7jwkzae8efd8x2rk7av9gmew82jq5zap9302ynhp37ggd6z5u4emcwp309kt3p7qjm
md1fxrvxzsag0zr8gh9upnjugr26jfvunvs35jvgdjkm3kghwnt0qqymzc0utyzxyhs535vcd9tq33dm
md1fxrvxz3ry9pu8uyfgtl737tj9hrdzaved93u3a2nawefnqlk8ycusz0quhfev0as4ure80muhz6hz
md1fxrvxz3fhqdghjmksz3ry92d3gv4ejtmu9f0zxf3clxvtlnnv86xy4qee32ay5yaf3qfqktd9h9
policy-id-fingerprint: 0x06215ac0
`

// TestArtifactKindsForNamesTheFlowsEngraveOrder pins the kind table itself.
// ArtifactKindsFor's ORDER is load-bearing — CheckArtifactShape enforces it and
// CompareCensus is order-sensitive — so a future edit that "tidied" the slice
// would silently redefine what a correct census is.
func TestArtifactKindsForNamesTheFlowsEngraveOrder(t *testing.T) {
	for _, tc := range []struct {
		kind ExpectKind
		want []string
	}{
		{KindCosignerCards, []string{"mk1"}},
		// gui/multisig_engrave.go:11-35 — ms1 secret card FIRST in full mode.
		{KindBuiltPolicyFull, []string{"ms1", "mk1", "md1"}},
		{KindBuiltPolicyWatch, []string{"mk1", "md1"}},
	} {
		got := ArtifactKindsFor(tc.kind)
		if len(got) != len(tc.want) {
			t.Errorf("%s -> %v, want %v", tc.kind, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("%s -> %v, want %v (order is part of the answer)", tc.kind, got, tc.want)
				break
			}
		}
		if !KnownExpectKind(tc.kind) {
			t.Errorf("%s produces %v but KnownExpectKind says it is not derivable", tc.kind, got)
		}
	}
	if got := ArtifactKindsFor("no-such-kind"); got != nil {
		t.Errorf("an unknown kind produced %v; it must produce nothing so callers refuse", got)
	}
	if KnownExpectKind("no-such-kind") {
		t.Error("an unknown kind was reported derivable")
	}
}

// arts is a tiny builder for shape/fingerprint tables: "kind:fingerprint".
func arts(spec ...string) []Artifact {
	out := make([]Artifact, len(spec))
	for i, s := range spec {
		kind, fp, _ := strings.Cut(s, ":")
		out[i] = Artifact{Kind: kind, Label: "a" + strconv.Itoa(i), String: "x", Fingerprint: fp}
	}
	return out
}

// TestCheckArtifactShapeRefusesTheWrongShape — including THE ORDER MUTATION.
// The reordered case is the one that matters most: CompareCensus refuses a
// reordered census, but only if the committed expectation was in the right order
// to begin with, and nothing before this checked that.
func TestCheckArtifactShapeRefusesTheWrongShape(t *testing.T) {
	for _, tc := range []struct {
		name string
		kind ExpectKind
		arts []Artifact
		ok   bool
	}{
		{"cosigner cards", KindCosignerCards, arts("mk1:a", "mk1:b"), true},
		{"full, one held slot", KindBuiltPolicyFull, arts("ms1:a", "mk1:a", "md1:", "md1:"), true},
		{"full, two masters", KindBuiltPolicyFull,
			arts("ms1:a", "ms1:b", "mk1:a", "mk1:b", "md1:", "md1:"), true},
		{"watch-only", KindBuiltPolicyWatch, arts("mk1:a", "md1:", "md1:"), true},

		// THE ORDER MUTATION: the secret plate moved to the end of the set,
		// which is exactly the "public plates first, secret last" reordering
		// the plan DEFERRED. It must be caught here rather than shipped as an
		// accident.
		{"full, ms1 last", KindBuiltPolicyFull, arts("mk1:a", "md1:", "ms1:a"), false},
		{"full, md1 before mk1", KindBuiltPolicyFull, arts("ms1:a", "md1:", "mk1:a"), false},
		// A MISSING CLASS — the C2 shape: a "Full" backup with no seed in it.
		{"full, no ms1 at all", KindBuiltPolicyFull, arts("mk1:a", "md1:"), false},
		{"full, no md1 at all", KindBuiltPolicyFull, arts("ms1:a", "mk1:a"), false},
		// A CLASS THAT KIND DOES NOT ENGRAVE.
		{"watch-only carrying an ms1", KindBuiltPolicyWatch, arts("ms1:a", "mk1:a", "md1:"), false},
		{"cosigner cards carrying an md1", KindCosignerCards, arts("mk1:a", "md1:"), false},
		// A TRAILING RUN after the last declared kind.
		{"full, mk1 after the md1s", KindBuiltPolicyFull,
			arts("ms1:a", "mk1:a", "md1:", "mk1:b"), false},
		{"empty", KindBuiltPolicyFull, nil, false},
		{"unknown kind", "no-such-kind", arts("mk1:a"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := CheckArtifactShape(tc.kind, tc.arts)
			if tc.ok && err != nil {
				t.Fatalf("an honest %s set was refused: %v", tc.kind, err)
			}
			if !tc.ok && err == nil {
				t.Fatalf("a wrong-shape %s set was accepted", tc.kind)
			}
		})
	}
}

// TestCheckFingerprintScopeHoldsInBothDirections — mk1/ms1 must carry one, md1
// must NOT, and the >=2-distinct rule belongs to cosigner-cards alone.
func TestCheckFingerprintScopeHoldsInBothDirections(t *testing.T) {
	for _, tc := range []struct {
		name string
		kind ExpectKind
		arts []Artifact
		ok   bool
	}{
		{"cosigner cards, two masters", KindCosignerCards, arts("mk1:a", "mk1:b"), true},
		// DIRECTION 1: a key card with no fingerprint binds no seed to a plate.
		{"cosigner card with no fingerprint", KindCosignerCards, arts("mk1:a", "mk1:"), false},
		// The >=2-distinct rule, still enforced where it belongs.
		{"cosigner cards, one master", KindCosignerCards, arts("mk1:a", "mk1:a"), false},

		{"built policy, fingerprints on ms1+mk1 only", KindBuiltPolicyFull,
			arts("ms1:a", "mk1:a", "md1:", "md1:"), true},
		// THE SCOPING DIFFERENCE, proven: one held master is honest for a built
		// policy and would have been a failure under the old blanket rule.
		{"built policy, one master only", KindBuiltPolicyFull,
			arts("ms1:a", "mk1:a", "md1:"), true},
		// DIRECTION 2: an md1 chunk carrying a fingerprint is a value no oracle
		// produced. "Not demanded" would have let this through.
		{"built policy, md1 carrying a fingerprint", KindBuiltPolicyFull,
			arts("ms1:a", "mk1:a", "md1:a"), false},
		{"built policy, ms1 with no fingerprint", KindBuiltPolicyFull,
			arts("ms1:", "mk1:a", "md1:"), false},
		{"built policy, mk1 with no fingerprint", KindBuiltPolicyWatch,
			arts("mk1:", "md1:"), false},
		{"unknown kind", "no-such-kind", arts("mk1:a"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := CheckFingerprintScope(tc.kind, tc.arts)
			if tc.ok && err != nil {
				t.Fatalf("an honest %s set was refused: %v", tc.kind, err)
			}
			if !tc.ok && err == nil {
				t.Fatalf("a %s set with wrong fingerprint scope was accepted", tc.kind)
			}
		})
	}
}

// TestParseMdStdoutRefusesTheHeaderAsAnArtifact — the trap mkEncode's comment
// predicted before any md step existed.
func TestParseMdStdoutRefusesTheHeaderAsAnArtifact(t *testing.T) {
	chunks, stub, err := parseMdStdout([]byte(mdRealStdout))
	if err != nil {
		t.Fatalf("the pinned md's REAL stdout was refused: %v", err)
	}
	if len(chunks) != 6 {
		t.Errorf("parsed %d chunk(s) from md's real output, want 6", len(chunks))
	}
	// THE ADOPTION DIRECTION, asserted rather than assumed: neither non-artifact
	// line may appear anywhere in the artifact set.
	for i, c := range chunks {
		if !strings.HasPrefix(c, "md1") {
			t.Errorf("chunk %d is %q, which is not an md1 string — a non-artifact line was adopted", i, c)
		}
		if strings.Contains(c, "chunk-set-id") || strings.Contains(c, "policy-id-fingerprint") {
			t.Errorf("chunk %d is %q — md's own header/trailer was collected as an expected "+
				"engraved string", i, c)
		}
	}
	if stub != "06215ac0" {
		t.Errorf("policy id parsed as %q, want 06215ac0 (the value the S2 golden's own live test "+
			"logs for this policy)", stub)
	}

	for _, tc := range []struct{ name, body string }{
		// A THIRD header line md does not print today. This is the case the
		// prefix-matching design exists for: an unrecognised line must refuse,
		// not be adopted and not be silently skipped.
		{"an unknown header line", "surprise: 0x1\n" + mdRealStdout},
		{"an unknown trailer line", mdRealStdout + "note: rebuild me\n"},
		{"no md1 lines at all", "chunk-set-id: 0x30d86\npolicy-id-fingerprint: 0x06215ac0\n"},
		{"no policy-id line", strings.ReplaceAll(mdRealStdout, "policy-id-fingerprint: 0x06215ac0\n", "")},
		{"a policy id that is not a stub", strings.ReplaceAll(mdRealStdout, "0x06215ac0", "0xZZ")},
		{"two policy-id lines", mdRealStdout + "policy-id-fingerprint: 0x06215ac1\n"},
		{"empty", ""},
		// THE SEPARATOR TRAP at the bytes: --group-size was not passed.
		{"a separated md1", "chunk-set-id: 0x1\nmd1fx rvxzs pqjtv\npolicy-id-fingerprint: 0x06215ac0\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, err := parseMdStdout([]byte(tc.body)); err == nil {
				t.Fatal("accepted stdout this gate must refuse")
			}
		})
	}
}

// TestParseMsStdoutRefusesASeparatedString — the --group-size 0 trap, caught at
// the bytes rather than trusted to the flag.
func TestParseMsStdoutRefusesASeparatedString(t *testing.T) {
	const unbroken = "ms10entrsqqqqqqqqqqqqqqqqqqqqqqqqqqqqcj9sxraq34v7f"
	got, err := parseMsStdout([]byte(unbroken + "\n"))
	if err != nil {
		t.Fatalf("the pinned ms's REAL --group-size 0 output was refused: %v", err)
	}
	if got != unbroken {
		t.Errorf("parsed %q, want %q", got, unbroken)
	}
	for _, tc := range []struct{ name, body string }{
		// Exactly what ms 0.16.0 prints WITHOUT --group-size 0. Measured.
		{"the default group-size 5 form", "ms10e ntrsq qqqqq qqqqq qqqqq qqqqq qqqqq qqcj9 sxraq 34v7f\n"},
		{"a hyphen separator", "ms10e-ntrsq-qqqqq\n"},
		{"a header line", "engraving card:\n" + unbroken + "\n"},
		{"two ms1 strings", unbroken + "\n" + unbroken + "\n"},
		{"empty", "\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := parseMsStdout([]byte(tc.body)); err == nil {
				t.Fatal("accepted stdout this gate must refuse")
			}
		})
	}
}

// TestPolicyTemplateMatchesTheDevicesOwnS2Template binds the generated template
// to the one the device is already compared against.
//
// The literal is s2WantTemplate from gui/multisig_build_oracle_test.go:87 — the
// string S2's committed md1 golden was minted with. Restating it here would be a
// second source of truth, so this test exists to make the two provably equal:
// if either side changes, the gate compares two different wallets, and this goes
// red instead.
func TestPolicyTemplateMatchesTheDevicesOwnS2Template(t *testing.T) {
	const s2WantTemplate = "wsh(sortedmulti(2,@0/<0;1>/*,@1/<0;1>/*,@2/<0;1>/*))"
	got, err := policyTemplate("bip48-p2wsh", 2, 3)
	if err != nil {
		t.Fatal(err)
	}
	if got != s2WantTemplate {
		t.Errorf("policyTemplate(bip48-p2wsh, 2, 3) = %q\nS2 encodes                       %q\n"+
			"These must be identical or the oracle derives a different wallet than the device", got, s2WantTemplate)
	}
	sh, err := policyTemplate("bip48-p2sh-p2wsh", 2, 3)
	if err != nil {
		t.Fatal(err)
	}
	if sh != "sh(wsh(sortedmulti(2,@0/<0;1>/*,@1/<0;1>/*,@2/<0;1>/*)))" {
		t.Errorf("bip48-p2sh-p2wsh -> %q", sh)
	}
	if _, err := policyTemplate("bip84-native-segwit", 2, 3); err == nil {
		t.Error("a template with no registered md root was accepted")
	}
}

// TestBuiltPolicyRefusalsAreReachableWithoutAToolchain drives every refusal the
// built-policy kind can reach BEFORE it shells out to anything.
//
// Bins{} is passed on purpose: with no binary resolved, any case that got as far
// as an oracle invocation would fail with "no ms binary resolved" instead. Each
// case below asserts on the refusal's own wording, so a case that started
// failing for the wrong reason is visible rather than merely still red.
func TestBuiltPolicyRefusalsAreReachableWithoutAToolchain(t *testing.T) {
	shared := []string{"m/48h/0h/0h/2h", "m/48h/0h/0h/2h", "m/48h/0h/0h/2h"}
	threeSeeds := []Seed{{Label: "A", Words: "a"}, {Label: "B", Words: "b"}, {Label: "C", Words: "c"}}
	base := InputTuple{Template: "t", N: 3, K: 2, Origins: shared}

	for _, tc := range []struct {
		name  string
		e     Expect
		in    InputTuple
		seeds []Seed
		want  string
	}{
		{
			name: "held_slots empty",
			e:    Expect{Kind: KindBuiltPolicyFull},
			in:   base, seeds: threeSeeds,
			want: "held_slots names none",
		}, {
			name: "held_slots out of range",
			e:    Expect{Kind: KindBuiltPolicyFull, HeldSlots: []int{0, 3}},
			in:   base, seeds: threeSeeds,
			want: "but the tuple has 3 slot(s)",
		}, {
			name: "held_slots repeated",
			e:    Expect{Kind: KindBuiltPolicyFull, HeldSlots: []int{1, 1}},
			in:   base, seeds: threeSeeds,
			want: "strictly ascending and distinct",
		}, {
			name: "held_slots descending",
			e:    Expect{Kind: KindBuiltPolicyFull, HeldSlots: []int{2, 0}},
			in:   base, seeds: threeSeeds,
			want: "strictly ascending and distinct",
		}, {
			name:  "n disagrees with the slots supplied",
			e:     Expect{Kind: KindBuiltPolicyFull, HeldSlots: []int{0}},
			in:    InputTuple{Template: "t", N: 4, K: 2, Origins: shared},
			seeds: threeSeeds,
			want:  "the tuple says n=4 but supplies 3 slot(s)",
		}, {
			name:  "k unset",
			e:     Expect{Kind: KindBuiltPolicyFull, HeldSlots: []int{0}},
			in:    InputTuple{Template: "t", N: 3, K: 0, Origins: shared},
			seeds: threeSeeds,
			want:  "k=0 over 3 slot(s)",
		}, {
			name:  "k larger than n",
			e:     Expect{Kind: KindBuiltPolicyFull, HeldSlots: []int{0}},
			in:    InputTuple{Template: "t", N: 3, K: 4, Origins: shared},
			seeds: threeSeeds,
			want:  "k=4 over 3 slot(s)",
		}, {
			// THE DIVERGENT-ORIGIN REFUSAL. Measured: the pinned md has no
			// invocation that encodes a per-slot origin, so deriving anything
			// here would derive a DIFFERENT wallet's md1.
			name: "origins diverge",
			e:    Expect{Kind: KindBuiltPolicyFull, HeldSlots: []int{0}},
			in: InputTuple{Template: "t", N: 3, K: 2, Origins: []string{
				"m/48h/0h/0h/2h", "m/48h/0h/1h/2h", "m/48h/0h/0h/2h"}},
			seeds: threeSeeds,
			want:  "the origins DIVERGE",
		}, {
			name: "script types mixed",
			e:    Expect{Kind: KindBuiltPolicyFull, HeldSlots: []int{0}},
			in: InputTuple{Template: "t", N: 3, K: 2, Origins: []string{
				"m/48h/0h/0h/2h", "m/48h/0h/0h/1h", "m/48h/0h/0h/2h"}},
			seeds: threeSeeds,
			want:  "one md1 encodes ONE script type",
		}, {
			name: "networks mixed",
			e:    Expect{Kind: KindBuiltPolicyFull, HeldSlots: []int{0}},
			in: InputTuple{Template: "t", N: 3, K: 2, Origins: []string{
				"m/48h/0h/0h/2h", "m/48h/1h/0h/2h", "m/48h/0h/0h/2h"}},
			seeds: threeSeeds,
			want:  "one policy is on one network",
		}, {
			// Cosigner-cards must not silently accept a field it has no use for.
			name: "cosigner cards given held_slots",
			e:    Expect{Kind: KindCosignerCards, PolicyIDStub: "5b48af35", HeldSlots: []int{0}},
			in:   base, seeds: threeSeeds,
			want: "engraves GATHERED cards verbatim",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := DeriveExpected(tc.e, tc.in, tc.seeds, Bins{})
			if err == nil {
				t.Fatal("accepted an expectation this gate must refuse")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("refused for the wrong reason.\n  want a message containing %q\n  got %v",
					tc.want, err)
			}
		})
	}
}

// TestBuiltPolicyReachesTheOracleOnHonestInputs is the CONTROL for the table
// above: a well-formed built-policy expectation must get past every structural
// refusal and fail only at the missing binary. Without it, a validation bug that
// refused everything would leave that whole table green.
func TestBuiltPolicyReachesTheOracleOnHonestInputs(t *testing.T) {
	_, err := DeriveExpected(
		Expect{Kind: KindBuiltPolicyFull, HeldSlots: []int{0}},
		InputTuple{Template: "t", N: 3, K: 2, Origins: []string{
			"m/48h/0h/0h/2h", "m/48h/0h/0h/2h", "m/48h/0h/0h/2h"}},
		[]Seed{{Label: "A", Words: "a"}, {Label: "B", Words: "b"}, {Label: "C", Words: "c"}},
		Bins{})
	if err == nil {
		t.Fatal("derived a census with no oracle binaries at all")
	}
	if !strings.Contains(err.Error(), "no ms binary resolved") {
		t.Fatalf("honest inputs were refused before reaching the oracle, so the refusal table "+
			"above proves nothing: %v", err)
	}
}

// TestCompareCensusCatchesAMultiKindReorder — CompareCensus is order-sensitive
// and the built-policy census is the first one where "order" means plates of
// DIFFERENT kinds. A secret plate cut where a public one was expected is a
// different engrave, and this proves the comparison sees it.
func TestCompareCensusCatchesAMultiKindReorder(t *testing.T) {
	want := []Artifact{
		{Kind: "ms1", Label: "A", String: "ms1aaa"},
		{Kind: "mk1", Label: "A", String: "mk1bbb"},
		{Kind: "md1", Label: "p", String: "md1ccc"},
	}
	got := []string{"ms1aaa", "mk1bbb", "md1ccc"}
	if err := CompareCensus(want, got); err != nil {
		t.Fatalf("control failed before any mutation: %v", err)
	}
	got[0], got[2] = got[2], got[0]
	err := CompareCensus(want, got)
	if err == nil {
		t.Fatal("a census with the secret plate and the descriptor swapped was accepted")
	}
	if !strings.Contains(err.Error(), "plate 0") || !strings.Contains(err.Error(), "plate 2") {
		t.Errorf("the refusal must name both plates that differ; got: %v", err)
	}
}
