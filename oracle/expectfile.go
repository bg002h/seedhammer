package oracle

// The VENDORED expectation: the primary toolchain's own output, committed into
// this fork with the provenance that says which toolchain produced it.
//
// # Why a file, when the whole package exists to avoid trusting files (C-1..C-4)
//
// The derived-census comparison worked. It just never ran where it mattered:
// every gate that checks this Go firmware against the Rust primary needed three
// Rust binaries installed at ~/.cargo/bin, and the machine whose verdict gates a
// merge has no Rust toolchain. Installing the oracles in CI is not the fix
// either — pins.json binds each oracle to the SHA-256 of a binary built on the
// maintainer's machine, so a CI `cargo install` produces a different binary, a
// different hash, and a hard resolution failure.
//
// So the comparison is split in two:
//
//	the GATE        this file: the primary's output, committed, compared with
//	                NO toolchain and NO skip path, everywhere, every push
//	the FRESHNESS   live re-derivation, wherever the pinned oracles exist,
//	                required to reproduce the committed bytes exactly
//
// # This is NOT what CheckDataSource refuses, and the difference is the point
//
// CheckDataSource (oracle.go) refuses fork-vendored `testdata` as a comparison
// source, because "vendored vectors are an older and smaller sample of
// OURSELVES; agreeing with them proves nothing about the primary". An
// ExpectFile is the opposite object: every byte in Artifacts came OUT of a
// pinned primary binary, and none of it was authored here. It is a cached
// primary output, not a fork-authored vector.
//
// That distinction is only worth anything if it is mechanically maintained, so
// it is, three ways:
//
//  1. NOTHING CAN MINT ONE BUT A LIVE DERIVATION. cmd/gaterecord refuses to
//     write a record whose census does not equal what it just derived from
//     oracles that resolved against pins.json, and writes the .expect.json as
//     part of minting. `go test ./gui -update` does the same for the S2 md1
//     golden. An expectation cannot exist except as the output of a live run.
//  2. STALENESS IS RED, NOT SILENT. CheckProvenance requires every oracle
//     identity recorded here to equal pins.json — commit AND binary hash. Bump
//     a pin without regenerating and CI goes red, with no toolchain involved.
//  3. THE LIVE ARM STILL RUNS on every machine that could mint, which is
//     exactly the population that could mint a wrong one.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// ExpectSchema is bumped when the on-disk shape changes. LoadExpect refuses a
// schema it does not understand rather than reading half of it.
const ExpectSchema = 1

// ExpectSuffix is the third file of a gate record's triple, beside
// <base>.record.json and <base>.walk.json.
const ExpectSuffix = ".expect.json"

var (
	// ErrNoExpectation is the fail-closed clause for the vendored layer:
	// a record with no committed expectation is a record nothing compares.
	ErrNoExpectation = errors.New("oracle: no committed expectation for this record")
	// ErrProvenance refuses an expectation minted against a different toolchain
	// than pins.json now names.
	ErrProvenance = errors.New("oracle: the committed expectation was derived from a toolchain pins.json no longer names")
)

// Derivation is the provenance block. It answers "which toolchain produced
// these bytes, and how", in enough detail to redo it by hand.
type Derivation struct {
	// Oracles is the identity of every oracle the derivation ACTUALLY invoked,
	// copied from pins.json at mint time: name, source commit, binary SHA-256.
	//
	// Collected by the derivation itself (DeriveExpected returns the names it
	// used) rather than declared in a table beside it, so this block cannot
	// claim a toolchain the derivation never touched.
	Oracles []Pin `json:"oracles"`
	// Args records the invocation shape, one entry per distinct oracle call
	// form. Prose for a human re-running it; nothing parses it.
	Args []string `json:"args"`
	// DerivedAt is when the mint ran, UTC.
	DerivedAt string `json:"derived_at"`
}

// ExpectFile is one committed expectation.
type ExpectFile struct {
	Schema int    `json:"schema"`
	Stage  string `json:"stage"`
	// Record is the basename of the gate record this expectation covers.
	// Empty for a vendored golden that is not tied to a walk (the S2 md1
	// golden, which is derived from a unit-test fixture, not from a run).
	Record string `json:"record,omitempty"`
	// Inputs is the basename of the inputs file it was derived from, if any.
	Inputs string `json:"inputs,omitempty"`
	// Note is free prose for a reader. Never parsed, never compared.
	Note       string     `json:"note,omitempty"`
	Derivation Derivation `json:"derivation"`
	// Artifacts is what the primary produced, in engrave order. This is the
	// whole point of the file; everything above says where it came from.
	Artifacts []Artifact `json:"artifacts"`
}

// NewExpectFile builds an expectation from a derivation that just ran.
//
// used names the oracles the derivation reported invoking; each must be pinned,
// because an artifact produced by an unpinned binary has no identity at all.
func NewExpectFile(stage, record, inputs, note string, pf PinFile, used, args []string, arts []Artifact) (ExpectFile, error) {
	if strings.TrimSpace(stage) == "" {
		return ExpectFile{}, fmt.Errorf("%w: no stage named", ErrBadRecord)
	}
	if len(arts) == 0 {
		return ExpectFile{}, fmt.Errorf("%w: refusing to write an expectation holding no "+
			"artifacts — an empty expectation compares nothing and reports a match", ErrBadRecord)
	}
	if len(used) == 0 {
		return ExpectFile{}, fmt.Errorf("%w: the derivation reported invoking no oracles, so "+
			"nothing identifies where these bytes came from", ErrBadRecord)
	}
	var oracles []Pin
	for _, name := range used {
		i := slices.IndexFunc(pf.Pins, func(fp FilePin) bool { return fp.Name == name })
		if i < 0 {
			return ExpectFile{}, fmt.Errorf("%w: the derivation invoked oracle %q, which is not "+
				"pinned — its output cannot be attributed to any commit", ErrIncompletePin, name)
		}
		oracles = append(oracles, pf.Pins[i].Pin)
	}
	for i, a := range arts {
		if strings.TrimSpace(a.String) == "" {
			return ExpectFile{}, fmt.Errorf("%w: artifact %d (%s) is the empty string; an "+
				"expectation of empty strings matches an engraving of empty strings",
				ErrBadRecord, i, a.Label)
		}
	}
	return ExpectFile{
		Schema: ExpectSchema,
		Stage:  stage,
		Record: record,
		Inputs: inputs,
		Note:   note,
		Derivation: Derivation{
			Oracles:   oracles,
			Args:      args,
			DerivedAt: time.Now().UTC().Format(time.RFC3339),
		},
		Artifacts: arts,
	}, nil
}

// Marshal renders the expectation as indented JSON with a trailing newline.
//
// HTML escaping is OFF: this file is committed EVIDENCE that a human reads in a
// diff, and the recorded invocation forms carry <angle-bracket> placeholders
// that json.Marshal would render as < — unreadable exactly where a reader
// is checking what was run.
func (e ExpectFile) Marshal() ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(e); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Write commits the expectation to path.
func (e ExpectFile) Write(path string) error {
	b, err := e.Marshal()
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

// LoadExpect reads and structurally checks one committed expectation.
//
// Every failure mode here is a REFUSAL, never an empty success: absent,
// unparseable, wrong schema and empty-artifact-list all return an error, so a
// caller cannot end up comparing nothing and calling it a match.
func LoadExpect(path string) (ExpectFile, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return ExpectFile{}, fmt.Errorf("%w: %v", ErrNoExpectation, err)
	}
	var e ExpectFile
	dec := json.NewDecoder(bytes.NewReader(b))
	// Unknown fields are an ERROR: a misspelt key in a provenance block would
	// otherwise silently drop the thing that makes the file checkable.
	dec.DisallowUnknownFields()
	if err := dec.Decode(&e); err != nil {
		return ExpectFile{}, fmt.Errorf("%s: %w: %v", path, ErrBadRecord, err)
	}
	if e.Schema != ExpectSchema {
		return ExpectFile{}, fmt.Errorf("%s: %w: schema %d, this build understands %d",
			path, ErrBadRecord, e.Schema, ExpectSchema)
	}
	if len(e.Artifacts) == 0 {
		return ExpectFile{}, fmt.Errorf("%s: %w: the expectation holds no artifacts, so anything "+
			"compared against it would pass by comparing nothing", path, ErrBadRecord)
	}
	for i, a := range e.Artifacts {
		if strings.TrimSpace(a.String) == "" {
			return ExpectFile{}, fmt.Errorf("%s: %w: artifact %d is the empty string",
				path, ErrBadRecord, i)
		}
	}
	if len(e.Derivation.Oracles) == 0 {
		return ExpectFile{}, fmt.Errorf("%s: %w: the expectation names no oracle, so nothing "+
			"says which toolchain produced it", path, ErrProvenance)
	}
	return e, nil
}

// CheckProvenance requires every oracle identity recorded in the expectation to
// still equal pins.json — commit AND binary hash.
//
// THIS IS THE ALWAYS-ON BINDING between the vendored layer and the primary, and
// it needs no toolchain: bump a pin without regenerating the expectations and it
// goes red on any machine.
//
// It deliberately does NOT require the expectation to name every pin. A
// cosigner-card derivation invokes ms and mk and never touches md, and a
// provenance block that listed md would be claiming a toolchain it did not use.
// The pins an expectation does not name are covered elsewhere: VerifyRecord
// requires EVERY pinned oracle to appear in the gate record at the pinned
// commit, and the S2 md1 golden names md.
func (e ExpectFile) CheckProvenance(pf PinFile) error {
	if len(e.Derivation.Oracles) == 0 {
		return fmt.Errorf("%w: no oracle recorded", ErrProvenance)
	}
	for _, o := range e.Derivation.Oracles {
		i := slices.IndexFunc(pf.Pins, func(fp FilePin) bool { return fp.Name == o.Name })
		if i < 0 {
			return fmt.Errorf("%w: it names oracle %q, which pins.json does not pin at all",
				ErrProvenance, o.Name)
		}
		p := pf.Pins[i].Pin
		// In FULL, both sides, always: a commit or a hash rendered truncated
		// makes two different values read as one.
		if !strings.EqualFold(strings.TrimSpace(o.Commit), strings.TrimSpace(p.Commit)) {
			return fmt.Errorf("%w: oracle %s was derived at commit %s, pins.json now says %s — "+
				"re-mint the expectation, do not edit it", ErrProvenance, o.Name, o.Commit, p.Commit)
		}
		if !strings.EqualFold(strings.TrimSpace(o.SHA256), strings.TrimSpace(p.SHA256)) {
			return fmt.Errorf("%w: oracle %s was derived from binary %s, pins.json now says %s — "+
				"re-mint the expectation, do not edit it", ErrProvenance, o.Name, o.SHA256, p.SHA256)
		}
	}
	return nil
}

// ExpectPathFor names the expectation file belonging to a record file.
func ExpectPathFor(dir, recordName string) string {
	return filepath.Join(dir, strings.TrimSuffix(recordName, RecordSuffix)+ExpectSuffix)
}

// Expectations lists the expectation files in dir, sorted. Used to catch an
// ORPHAN — an expectation whose record was deleted still vouches for nothing,
// and left alone it would make the directory look better covered than it is.
func Expectations(dir string) ([]string, error) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range ents {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ExpectSuffix) {
			out = append(out, e.Name())
		}
	}
	slices.Sort(out)
	return out, nil
}
