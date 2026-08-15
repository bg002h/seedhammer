// Command gaterecord emits the gate record for one emulator walk: the resolved
// oracle SOURCE COMMITS, the full input tuple, and the walk's own census and
// per-plate digests, bound to the run that produced them.
//
// It is the last clause of S0's gate (D5). It is deliberately NOT sufficient on
// its own — a command an operator can forget is a gate that passes in silence —
// so it is one of four parts:
//
//	this command      builds a record, REFUSES without a green walk, and
//	                  REFUSES a census that is not what the primary toolchain
//	                  just derived — writing that derivation out beside the
//	                  record as <base>.expect.json
//	oracle.VerifyAll  re-checks every record against the walk beside it
//	TestS0GateHasARecord   makes ABSENCE a test failure
//	TestEveryGateRecordCensusMatchesItsCommittedExpectation
//	                  compares every record's census against its committed
//	                  expectation, with no toolchain and no skip path
//
// # Why minting is where the live derivation is MANDATORY (C-2)
//
// Before this, the comparison lived only in a test hardwired to ONE filename, so
// a later stage could mint a record holding six invented strings and the whole
// suite stayed green — measured: a fabricated S9 record "verified" and passed.
// Now a record cannot come into existence except as something the primary
// toolchain agreed with, and the agreement is written down beside it.
//
// THERE IS NO WAY TO TURN THIS OFF, by flag or by environment. Minting is the
// one operation that has no meaning without the primary toolchain, and the
// safety of every toolchain-free check downstream rests on this refusal being
// unconditional: it is what makes "a committed expectation" mean "something the
// primary produced" rather than "something somebody wrote down".
//
// # Running it
//
// Build and serve cmd/emu on a fresh port (the browser caches emu.wasm), open
// index.html, and drive the walk fire-and-forget so a long run does not die in
// the driver's idle timeout:
//
//	const w = await import("./walk_trace_a.js");
//	w.run({ perPlateDigest: true }).then(r => { window.__walk = r });
//	// poll window.__walk, then save JSON.stringify(window.__walk) to a file
//
// perPlateDigest is REQUIRED here: without it the walk returns no plate digests
// and this command refuses, because a record with no digests is bound to
// nothing. Then:
//
//	go run ./cmd/gaterecord \
//	  -stage S0 \
//	  -walk /tmp/walk.json \
//	  -inputs oracle/gaterecords/S0-trace-a.inputs.json \
//	  -base S0-trace-a
//
// It writes oracle/gaterecords/<base>.record.json, <base>.walk.json and
// <base>.expect.json, and prints the record to stdout. The inputs file must
// already live in that same directory: the expectation is re-derived from it,
// so a record whose inputs sit somewhere else is a record nothing can re-check.
//
// # The inputs file
//
// A gate record must say what went IN, or two runs cannot be compared. The
// inputs file is authored by the operator and committed beside the record:
//
//	{
//	  "payload": {"name": "cmd/emu/sysw_cards_payload.bin", "digest": "2527 …"},
//	  "inputs": {
//	    "template": "bundle-engrave", "n": 3, "k": 0,
//	    "slot_order": [0,1,2], "fp_choice": "default",
//	    "origins": ["m/48h/0h/0h/2h", …],
//	    "seeds": [{"label": "payload:masterA", "words": "abandon … about"}]
//	  }
//	}
//
// The payload digest is stated by the OPERATOR and checked against the one the
// walk actually loaded, so it is a claim that can fail rather than a copy.
//
// Seed WORDS are digested here and never written to the record — a gate record
// that carried seed words would be key material, in a repo, in CI logs. Supply
// "digest" instead of "words" for a seed whose words must not sit on disk at
// all; the inputs file below is committed only because its three masters are
// BIP-39's own published vectors.
//
// Scratch tool for producing a gate artifact; not part of the firmware.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"seedhammer.com/oracle"
)

func main() {
	var (
		stage  = flag.String("stage", "", "which stage's gate this record anchors, e.g. S0")
		walk   = flag.String("walk", "", "path to the saved run() return value from walk_trace_a.js")
		inputs = flag.String("inputs", "", "path to the input-tuple JSON (see this command's doc comment)")
		base   = flag.String("base", "", "basename for the record/walk pair, e.g. S0-trace-a")
		out    = flag.String("out", filepath.Join("oracle", oracle.GateRecordsDir), "directory to write the pair into")
		pins   = flag.String("pins", filepath.Join("oracle", "pins.json"), "the oracle pin file")
		binDir = flag.String("oracle-bin-dir", "", "where the md/mk/ms binaries live (default ~/.cargo/bin)")
		force  = flag.Bool("force", false, "overwrite an existing record for -base")
		// -expect-only exists because a record is EVIDENCE and an expectation is
		// a DERIVATION, and the two go stale for different reasons. Moving a pin
		// makes every committed expectation stale; it does not make the walk that
		// produced the record any less true. Re-minting the record to refresh an
		// expectation would rewrite the evidence's recorded_at to a moment when no
		// walk ran, which is exactly the kind of quiet fiction this whole
		// deliverable exists to remove. So: re-derive, re-compare against BOTH the
		// walk and the record already on disk, and write only the expectation.
		expectOnly = flag.Bool("expect-only", false,
			"re-derive and write ONLY <base>.expect.json for a record already on disk")
	)
	flag.Parse()

	if err := run(*stage, *walk, *inputs, *base, *out, *pins, *binDir, *force, *expectOnly); err != nil {
		fmt.Fprintf(os.Stderr, "gaterecord: %v\n", err)
		os.Exit(1)
	}
}

func run(stage, walkPath, inputsPath, base, outDir, pinsPath, binDir string, force, expectOnly bool) error {
	for _, r := range []struct{ name, v string }{
		{"-stage", stage}, {"-walk", walkPath}, {"-inputs", inputsPath}, {"-base", base},
	} {
		if r.v == "" {
			return fmt.Errorf("%s is required", r.name)
		}
	}

	// Property 1, at the outermost layer: no walk file, no record. The read
	// failing IS the refusal — there is no fallback that emits something.
	raw, err := os.ReadFile(walkPath)
	if err != nil {
		return fmt.Errorf("%w: %v", oracle.ErrNoWalk, err)
	}

	// The inputs-file shape lives in package oracle: the expectation deriver
	// reads the same file for the seed WORDS and the expect block, and two
	// declarations of one on-disk shape drift.
	inf, err := oracle.LoadInputsFile(inputsPath)
	if err != nil {
		return err
	}
	tuple, err := inf.Tuple()
	if err != nil {
		return err
	}

	pf, err := oracle.LoadPins(pinsPath)
	if err != nil {
		return err
	}
	if binDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("no home directory, so no default oracle bin dir: %w", err)
		}
		binDir = filepath.Join(home, ".cargo", "bin")
	}
	// Binary-hash mode: these are installed binaries, not built inside a
	// checkout in this run, so pairing them with a source tree would be refused
	// by design (oracle.ErrBinaryOutsideCheckout).
	resolved, err := oracle.ResolveAll(pf, func(name string) (string, string) {
		return filepath.Join(binDir, name), ""
	})
	if err != nil {
		return fmt.Errorf("the oracles do not resolve, so this walk has nothing to be compared against: %w", err)
	}

	// THE MINT-TIME COMPARISON (C-2). Derive what these inputs REQUIRE from the
	// oracles just resolved, and refuse the whole record if the walk engraved
	// anything else. This is what makes a committed expectation trustworthy
	// downstream: it cannot exist except as the output of a derivation that
	// agreed with the run it is filed beside.
	if inf.Expect == nil {
		return fmt.Errorf("%s carries no `expect` block, so nothing says what these inputs "+
			"REQUIRE to have been engraved — and a record whose census nothing derives is a "+
			"record no gate compares. Add one (see oracle.Expect / oracle.ExpectKind)", inputsPath)
	}
	if got, want := filepath.Clean(filepath.Dir(inputsPath)), filepath.Clean(outDir); got != want {
		return fmt.Errorf("the inputs file must live beside the record it describes: %s is in "+
			"%s, the record goes in %s. The committed expectation is re-derived from the inputs "+
			"file by basename, so a record whose inputs sit elsewhere cannot be re-checked",
			inputsPath, got, want)
	}
	seeds, err := inf.SeedWords()
	if err != nil {
		return err
	}
	w, err := oracle.ParseWalk(raw)
	if err != nil {
		return err
	}
	derived, err := oracle.DeriveExpected(*inf.Expect, tuple, seeds, oracle.Bins{
		MD: filepath.Join(binDir, "md"),
		MK: filepath.Join(binDir, "mk"),
		MS: filepath.Join(binDir, "ms"),
	})
	if err != nil {
		return fmt.Errorf("deriving what these inputs require: %w", err)
	}
	if err := oracle.CompareCensus(derived.Artifacts, w.Census.Strings); err != nil {
		return fmt.Errorf("REFUSING to mint this record: the walk's engraved census is not what "+
			"the primary toolchain derives from these inputs.\n%w", err)
	}
	if expectOnly {
		// The record on disk is the thing the expectation will be filed against,
		// so it — not just the walk — is what must agree with the derivation.
		rec, err := oracle.LoadRecord(filepath.Join(outDir, base+oracle.RecordSuffix))
		if err != nil {
			return fmt.Errorf("-expect-only needs the record it describes to be on disk already: %w", err)
		}
		if rec.Stage != stage {
			return fmt.Errorf("-stage %s, but %s%s on disk is stage %s",
				stage, base, oracle.RecordSuffix, rec.Stage)
		}
		if err := oracle.CompareCensus(derived.Artifacts, rec.Walk.Census.Strings); err != nil {
			return fmt.Errorf("REFUSING to write an expectation: the RECORD's census is not what "+
				"the primary toolchain derives from these inputs.\n%w", err)
		}
	}

	rec, err := oracle.NewRecord(stage, resolved, tuple, inf.Payload, base+oracle.WalkSuffix, raw)
	if err != nil {
		return err
	}
	note := "Derived live by cmd/gaterecord and compared against the walk's census before the " +
		"record was written."
	if expectOnly {
		note = "Re-derived live by `cmd/gaterecord -expect-only` and compared against both the " +
			"walk and the record already on disk; neither was rewritten."
	}
	note += " Committed so the comparison runs where no Rust toolchain exists — which is every " +
		"machine whose verdict gates a merge."
	exp, err := oracle.NewExpectFile(stage, base+oracle.RecordSuffix, filepath.Base(inputsPath),
		note, pf, derived.Oracles, derived.Args, derived.Artifacts)
	if err != nil {
		return err
	}

	expPath := filepath.Join(outDir, base+oracle.ExpectSuffix)
	guarded := []string{expPath}
	if !expectOnly {
		guarded = append(guarded, filepath.Join(outDir, base+oracle.RecordSuffix))
	}
	for _, target := range guarded {
		if _, err := os.Stat(target); err == nil && !force {
			return fmt.Errorf("%s already exists; pass -force to replace it (a record is evidence, "+
				"so replacing one is a decision, not a default)", target)
		} else if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}

	if err := exp.Write(expPath); err != nil {
		return err
	}
	if expectOnly {
		fmt.Fprintf(os.Stderr, "%d artifact(s) derived live by %v and matched both the walk and "+
			"the record on disk\nwrote %s\n", len(derived.Artifacts), derived.Oracles, expPath)
		return nil
	}

	recPath, walkOut, err := rec.Write(outDir, base, raw)
	if err != nil {
		return err
	}

	// "Prints resolved oracle commits and the full input tuple", literally.
	b, err := rec.Marshal()
	if err != nil {
		return err
	}
	fmt.Println(string(b))
	fmt.Fprintf(os.Stderr, "\n%d artifact(s) derived live by %v and matched the walk's census\n",
		len(derived.Artifacts), derived.Oracles)
	fmt.Fprintf(os.Stderr, "wrote %s\n      %s\n      %s\n", recPath, walkOut, expPath)
	return nil
}
