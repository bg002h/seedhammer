package oracle

// TIER 2 (§4.6): these shell out to the pinned md/mk/ms binaries. Keep them out
// of the inner loop; they are the gate, not a unit test.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// resolveBins locates the pinned oracles the way cmd/gaterecord does, and SKIPS
// (rather than fails) when they are not installed — a contributor without the
// Rust toolchain should not see a red suite they cannot fix. The gate that
// makes absence fail is TestS0GateHasARecord, which does not need the binaries.
func resolveBins(t *testing.T) Bins {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("no home directory: %v", err)
	}
	dir := filepath.Join(home, ".cargo", "bin")
	pf, err := LoadPins("pins.json")
	if err != nil {
		t.Fatalf("loading pins: %v", err)
	}
	for _, fp := range pf.Pins {
		if _, err := os.Stat(filepath.Join(dir, fp.Name)); err != nil {
			t.Skipf("oracle %q is not installed at %s; skipping the derived-census gate", fp.Name, dir)
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

func loadS0(t *testing.T) (GateRecord, InputsFile, []Artifact, Bins) {
	t.Helper()
	bins := resolveBins(t)
	rec, err := LoadRecord(filepath.Join(GateRecordsDir, "S0-trace-a.record.json"))
	if err != nil {
		t.Fatalf("loading S0's record: %v", err)
	}
	inf, err := LoadInputsFile(filepath.Join(GateRecordsDir, "S0-trace-a.inputs.json"))
	if err != nil {
		t.Fatalf("loading S0's inputs: %v", err)
	}
	if inf.Expect == nil {
		t.Fatal("S0's inputs file carries no expect block, so no census can be derived from it")
	}
	seeds, err := inf.SeedWords()
	if err != nil {
		t.Fatalf("S0's seeds: %v", err)
	}
	want, err := DeriveExpected(*inf.Expect, rec.Inputs, seeds, bins)
	if err != nil {
		t.Fatalf("deriving S0's expected artifacts: %v", err)
	}
	return rec, inf, want, bins
}

// THE GATE (F-170 + F-171). What the inputs REQUIRE, computed by the primary
// toolchain, must equal what the device engraved — byte for byte, in order.
func TestS0CensusMatchesTheDerivedExpectation(t *testing.T) {
	rec, _, want, _ := loadS0(t)
	if err := CompareCensus(want, rec.Walk.Census.Strings); err != nil {
		t.Fatal(err)
	}
	t.Logf("derived %d artifact(s) from the recorded inputs; all matched the engraved census", len(want))
}

// The COUNT is derived, not a literal. This is what replaces `plates = 6`: the
// number six now comes from three seeds x two chunks, computed by mk.
func TestS0PlateCountIsDerivedFromTheInputs(t *testing.T) {
	rec, _, want, _ := loadS0(t)
	if len(want) != len(rec.Walk.Census.Strings) {
		t.Fatalf("the inputs require %d plate(s); the record holds %d", len(want), len(rec.Walk.Census.Strings))
	}
	if len(want) != len(rec.Walk.Digests) {
		t.Fatalf("%d derived artifact(s) but %d plate digest(s)", len(want), len(rec.Walk.Digests))
	}
}

// The oracle vouches for the SEED->KEY step independently of the xpub: every
// derived card's master fingerprint must be the one ms computes from that
// seed's words. Without this the comparison would prove only that mk encodes
// consistently, not that the right key was engraved.
func TestS0FingerprintsComeFromTheRecordedSeeds(t *testing.T) {
	_, _, want, _ := loadS0(t)
	seen := map[string]bool{}
	for _, a := range want {
		if a.Fingerprint == "" {
			t.Fatalf("artifact %q carries no fingerprint", a.Label)
		}
		seen[a.Fingerprint] = true
	}
	if len(seen) < 2 {
		t.Fatalf("only %d distinct fingerprint(s) across the whole census — the cards are not "+
			"from distinct seeds, so this gate is weaker than it looks", len(seen))
	}
}

// MUTATION 1 — one expected string. Required by the stage's gate: a mechanism
// that has not been seen to fail does not leave this stage.
func TestCompareCensusCatchesAMutatedString(t *testing.T) {
	_, _, want, _ := loadS0(t)
	got := make([]string, len(want))
	for i, a := range want {
		got[i] = a.String
	}
	if err := CompareCensus(want, got); err != nil {
		t.Fatalf("control failed before any mutation: %v", err)
	}
	// Flip ONE character of ONE plate — the smallest defect the gate must catch.
	orig := got[2]
	b := []byte(orig)
	if b[len(b)-1] == 'q' {
		b[len(b)-1] = 'p'
	} else {
		b[len(b)-1] = 'q'
	}
	got[2] = string(b)
	err := CompareCensus(want, got)
	if err == nil {
		t.Fatal("a one-character mutation in plate 2 was accepted; the byte comparison is not comparing")
	}
	if !strings.Contains(err.Error(), "plate 2") {
		t.Errorf("the refusal must name the plate that differs; got: %v", err)
	}
	// And both strings in full — a truncated message renders them identical.
	if !strings.Contains(err.Error(), orig) || !strings.Contains(err.Error(), got[2]) {
		t.Errorf("the refusal must print BOTH strings in full; got: %v", err)
	}
}

// MUTATION 2 — a dropped plate. A partial walk may never satisfy a total gate.
func TestCompareCensusCatchesAShortCensus(t *testing.T) {
	_, _, want, _ := loadS0(t)
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
	_, _, want, _ := loadS0(t)
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
