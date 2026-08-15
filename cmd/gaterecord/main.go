// Command gaterecord emits the gate record for one emulator walk: the resolved
// oracle SOURCE COMMITS, the full input tuple, and the walk's own census and
// per-plate digests, bound to the run that produced them.
//
// It is the last clause of S0's gate (D5). It is deliberately NOT sufficient on
// its own — a command an operator can forget is a gate that passes in silence —
// so it is one of three parts:
//
//	this command      builds a record, and REFUSES without a green walk
//	oracle.VerifyAll  re-checks every record against the walk beside it
//	TestS0GateHasARecord   makes ABSENCE a test failure
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
// It writes oracle/gaterecords/<base>.record.json and <base>.walk.json, and
// prints the record to stdout.
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
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"seedhammer.com/oracle"
)

// inputsFile is what -inputs holds. Seeds accept words OR a pre-computed
// digest, never both, so a caller cannot half-supply one and get a record that
// silently identifies nothing.
type inputsFile struct {
	Payload oracle.Payload `json:"payload"`
	Inputs  struct {
		Template  string   `json:"template"`
		N         int      `json:"n"`
		K         int      `json:"k"`
		SlotOrder []int    `json:"slot_order"`
		FPChoice  string   `json:"fp_choice"`
		Origins   []string `json:"origins"`
		Seeds     []struct {
			Label  string `json:"label"`
			Words  string `json:"words,omitempty"`
			Digest string `json:"digest,omitempty"`
		} `json:"seeds"`
	} `json:"inputs"`
}

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
	)
	flag.Parse()

	if err := run(*stage, *walk, *inputs, *base, *out, *pins, *binDir, *force); err != nil {
		fmt.Fprintf(os.Stderr, "gaterecord: %v\n", err)
		os.Exit(1)
	}
}

func run(stage, walkPath, inputsPath, base, outDir, pinsPath, binDir string, force bool) error {
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

	inf, err := loadInputs(inputsPath)
	if err != nil {
		return err
	}
	tuple, err := toTuple(inf)
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

	rec, err := oracle.NewRecord(stage, resolved, tuple, inf.Payload, base+oracle.WalkSuffix, raw)
	if err != nil {
		return err
	}

	target := filepath.Join(outDir, base+oracle.RecordSuffix)
	if _, err := os.Stat(target); err == nil && !force {
		return fmt.Errorf("%s already exists; pass -force to replace it (a record is evidence, "+
			"so replacing one is a decision, not a default)", target)
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
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
	fmt.Fprintf(os.Stderr, "\nwrote %s\n      %s\n", recPath, walkOut)
	return nil
}

func loadInputs(path string) (inputsFile, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return inputsFile{}, err
	}
	var inf inputsFile
	dec := json.NewDecoder(bytes.NewReader(b))
	// Unknown fields are an error here: an inputs file with a misspelt key
	// would otherwise produce a record missing that input, which is the exact
	// class of silence this deliverable exists to remove.
	dec.DisallowUnknownFields()
	if err := dec.Decode(&inf); err != nil {
		return inputsFile{}, fmt.Errorf("%s: %w", path, err)
	}
	return inf, nil
}

func toTuple(inf inputsFile) (oracle.InputTuple, error) {
	t := oracle.InputTuple{
		Template:  inf.Inputs.Template,
		N:         inf.Inputs.N,
		K:         inf.Inputs.K,
		SlotOrder: inf.Inputs.SlotOrder,
		FPChoice:  inf.Inputs.FPChoice,
		Origins:   inf.Inputs.Origins,
	}
	for _, s := range inf.Inputs.Seeds {
		switch {
		case s.Words != "" && s.Digest != "":
			return oracle.InputTuple{}, fmt.Errorf("seed %q gives both words and a digest; give one", s.Label)
		case s.Words != "":
			t.Seeds = append(t.Seeds, oracle.NewSeedRef(s.Label, s.Words))
		case s.Digest != "":
			t.Seeds = append(t.Seeds, oracle.SeedRef{Label: s.Label, Digest: s.Digest})
		default:
			return oracle.InputTuple{}, fmt.Errorf("seed %q identifies nothing: give words or a digest", s.Label)
		}
	}
	return t, nil
}
