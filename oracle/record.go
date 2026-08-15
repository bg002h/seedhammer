package oracle

// The gate record on disk: what a walk proved, bound to the run that proved it.
//
// # Why this file exists, and why it is not just a command
//
// S0's gate requires the harness to print resolved oracle commits and the full
// input tuple INTO EVERY GATE RECORD. The obvious build — a command the
// operator runs beside a walk — was rejected on 2026-08-14 against the stated
// criterion "we don't want to be surprised later by code that doesn't work as
// expected". A plain command fails it twice:
//
//   - it is OPTIONAL, so a gate passes with no record at all and the absence is
//     silent; and
//   - it is UNBOUND, so a record emitted from run A can sit beside run B's
//     artifacts and nothing notices.
//
// Three properties answer that, and all three live here rather than in the
// command, so they are testable without shelling out:
//
//  1. NewRecord REFUSES to build a record without a green walk result. No run,
//     no record. ParseWalk is the refusal, and it checks the walk's own
//     invariants rather than trusting its `ok` flag alone.
//  2. The record EMBEDS the walk's census and per-plate digests, and the raw
//     run() return value is written beside it and hashed into the record. A
//     record and a walk from different runs cannot be paired.
//  3. VerifyAll re-checks every record on disk — the walk file still hashes to
//     what the record says, its census and digests still match, and every
//     oracle commit still equals the pin file's. TestS0GateHasARecord makes
//     ABSENCE a failure rather than a silence.
//
// Residual risk, stated rather than hidden: the operator still has to run the
// walk. Property 3 is what converts forgetting from "never noticed" into "fails
// at gate time".

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// RecordSchema is bumped when the on-disk shape changes. Verify refuses a
// record it does not understand rather than reading half of it: a gate record
// silently half-parsed is worse than none.
const RecordSchema = 1

// RecordSuffix and WalkSuffix name the two halves of a record on disk. They are
// a pair by construction — the record names its walk file and hashes it.
const (
	RecordSuffix = ".record.json"
	WalkSuffix   = ".walk.json"
)

// GateRecordsDir is where records live, relative to this package's directory.
// Deliberately not `testdata`: CheckDataSource refuses any path with a
// `testdata` element, and gate records are evidence, not vectors.
const GateRecordsDir = "gaterecords"

var (
	// ErrNoWalk is property 1: a gate record cannot be built out of nothing.
	ErrNoWalk = errors.New("oracle: a gate record needs a walk result; no run, no record")
	// ErrWalkNotGreen refuses a walk that did not finish, or finished with
	// something this census cannot name.
	ErrWalkNotGreen = errors.New("oracle: the walk did not finish green, so it cannot anchor a gate record")
	// ErrRecordMismatch is property 3's refusal: the record and the walk beside
	// it disagree, so at least one of them has been edited.
	ErrRecordMismatch = errors.New("oracle: the record does not match the walk beside it")
	// ErrRecordMissing is the fail-closed clause. Absence is a failure.
	ErrRecordMissing = errors.New("oracle: no gate record on disk")
	// ErrIncompleteInputs refuses a record whose input tuple could not identify
	// a second run of the same thing.
	ErrIncompleteInputs = errors.New("oracle: the input tuple is incomplete")
	// ErrBadRecord refuses a structurally unusable record.
	ErrBadRecord = errors.New("oracle: malformed gate record")
)

// Census is cmd/emu's engraved census, exactly as shToolpath.strings() returns
// it. Announced and Unattributed are carried because they are what tells an
// EMPTY census apart from a BROKEN one — see cmd/emu/engraved.go.
type Census struct {
	Strings      []string `json:"strings"`
	Announced    int      `json:"announced"`
	Unattributed int      `json:"unattributed"`
}

// WalkResult is the part of cmd/emu/walk_trace_a.js run()'s return value that a
// record binds to. The raw value is kept verbatim beside the record, so the
// fields dropped here (acts, gathered, screen) are not lost — they are simply
// not load-bearing for the binding.
type WalkResult struct {
	Pace          int      `json:"pace"`
	ElapsedSec    int      `json:"elapsedSec"`
	Census        Census   `json:"census"`
	Digests       []string `json:"digests"`
	PayloadDigest string   `json:"payloadDigest"`
	OK            bool     `json:"ok"`
}

// Payload names the systemwide payload the walk ran against. Digest is what the
// device displayed and the walk asserted, so a record made against the wrong
// blob is visible without opening the blob.
type Payload struct {
	Name   string `json:"name"`
	Digest string `json:"digest"`
}

// WalkBinding is property 2: the record's half of the pairing.
type WalkBinding struct {
	// File is the BASENAME of the raw run() return value, written beside the
	// record. A basename, never a path — a record must not be able to point
	// verification at a file outside its own directory.
	File string `json:"file"`
	// SHA256 is over that file's bytes, in full. Never truncate it in a
	// mismatch message: a hash differing at character 16 rendered truncated
	// reads as two identical strings.
	SHA256     string   `json:"sha256"`
	Pace       int      `json:"pace"`
	ElapsedSec int      `json:"elapsed_sec"`
	Census     Census   `json:"census"`
	Digests    []string `json:"plate_digests"`
}

// GateRecord is what a gate emits: which oracles it trusted, established by
// commit, the exact inputs, and the walk that produced the artifacts.
type GateRecord struct {
	Schema     int    `json:"schema"`
	Stage      string `json:"stage"`
	RecordedAt string `json:"recorded_at"`

	Oracles []Resolved  `json:"oracles"`
	Inputs  InputTuple  `json:"inputs"`
	Payload Payload     `json:"payload"`
	Walk    WalkBinding `json:"walk"`
}

// Marshal renders the gate record as indented JSON.
func (g GateRecord) Marshal() ([]byte, error) { return json.MarshalIndent(g, "", "  ") }

// ParseWalk decodes a raw run() return value and refuses anything that is not a
// completed, green walk.
//
// It deliberately re-derives the walk's invariants instead of trusting `ok`.
// `ok` is computed in JavaScript by the same script that produced the numbers,
// so a change to its definition would silently widen what a gate accepts; the
// checks below say, in this package, what a record-worthy walk is.
func ParseWalk(b []byte) (WalkResult, error) {
	if len(bytes.TrimSpace(b)) == 0 {
		return WalkResult{}, ErrNoWalk
	}
	var w WalkResult
	if err := json.Unmarshal(b, &w); err != nil {
		return WalkResult{}, fmt.Errorf("%w: %v", ErrNoWalk, err)
	}
	switch {
	case !w.OK:
		return WalkResult{}, fmt.Errorf("%w: run() reported ok=false", ErrWalkNotGreen)
	case len(w.Census.Strings) == 0:
		return WalkResult{}, fmt.Errorf("%w: the census is empty, so nothing was engraved to anchor to", ErrWalkNotGreen)
	case w.Census.Unattributed != 0:
		return WalkResult{}, fmt.Errorf("%w: %d unattributed plate(s) — something was cut that this census cannot name",
			ErrWalkNotGreen, w.Census.Unattributed)
	case w.Census.Announced < len(w.Census.Strings):
		return WalkResult{}, fmt.Errorf("%w: announced %d but engraved %d — the census hook is not wired",
			ErrWalkNotGreen, w.Census.Announced, len(w.Census.Strings))
	case len(w.Digests) != len(w.Census.Strings):
		return WalkResult{}, fmt.Errorf("%w: %d plate digest(s) for %d engraved string(s) — re-run with perPlateDigest: true",
			ErrWalkNotGreen, len(w.Digests), len(w.Census.Strings))
	case strings.TrimSpace(w.PayloadDigest) == "":
		return WalkResult{}, fmt.Errorf("%w: the walk recorded no payload digest, so the record cannot name what it ran against",
			ErrWalkNotGreen)
	}
	for i, d := range w.Digests {
		if strings.TrimSpace(d) == "" {
			return WalkResult{}, fmt.Errorf("%w: plate %d has an empty digest", ErrWalkNotGreen, i)
		}
	}
	return w, nil
}

// NewRecord builds a gate record from a green walk, or refuses.
//
// walkFile is the basename the raw walk JSON will be written under, beside the
// record; walkJSON is its exact bytes, which are hashed into the binding.
func NewRecord(stage string, oracles []Resolved, inputs InputTuple, payload Payload,
	walkFile string, walkJSON []byte) (GateRecord, error) {

	w, err := ParseWalk(walkJSON)
	if err != nil {
		return GateRecord{}, err
	}
	if strings.TrimSpace(stage) == "" {
		return GateRecord{}, fmt.Errorf("%w: no stage named", ErrBadRecord)
	}
	if len(oracles) == 0 {
		return GateRecord{}, fmt.Errorf("%w: no resolved oracles, which is the whole point of the record", ErrBadRecord)
	}
	if walkFile == "" || filepath.Base(walkFile) != walkFile {
		return GateRecord{}, fmt.Errorf("%w: walk file %q must be a basename", ErrBadRecord, walkFile)
	}
	if err := checkInputs(inputs); err != nil {
		return GateRecord{}, err
	}
	if strings.TrimSpace(payload.Name) == "" || strings.TrimSpace(payload.Digest) == "" {
		return GateRecord{}, fmt.Errorf("%w: the payload must be named and digested", ErrBadRecord)
	}
	if !digestEqual(payload.Digest, w.PayloadDigest) {
		return GateRecord{}, fmt.Errorf("%w: payload pinned %q but the walk loaded %q",
			ErrRecordMismatch, payload.Digest, w.PayloadDigest)
	}
	return GateRecord{
		Schema:     RecordSchema,
		Stage:      stage,
		RecordedAt: time.Now().UTC().Format(time.RFC3339),
		Oracles:    oracles,
		Inputs:     inputs,
		Payload:    payload,
		Walk: WalkBinding{
			File:       walkFile,
			SHA256:     sha256Bytes(walkJSON),
			Pace:       w.Pace,
			ElapsedSec: w.ElapsedSec,
			Census:     w.Census,
			Digests:    w.Digests,
		},
	}, nil
}

// checkInputs refuses a tuple that could not identify a second run of the same
// thing. Template, origins and seeds are required; N/K are not, because S0's
// own walk engraves a gathered bundle and chooses no threshold, and inventing a
// k there would be a fiction in the one artifact that exists to stop fictions.
func checkInputs(in InputTuple) error {
	switch {
	case strings.TrimSpace(in.Template) == "":
		return fmt.Errorf("%w: no template", ErrIncompleteInputs)
	case len(in.Origins) == 0:
		return fmt.Errorf("%w: no origins", ErrIncompleteInputs)
	case len(in.Seeds) == 0:
		return fmt.Errorf("%w: no seeds", ErrIncompleteInputs)
	}
	for _, s := range in.Seeds {
		if strings.TrimSpace(s.Label) == "" || strings.TrimSpace(s.Digest) == "" {
			return fmt.Errorf("%w: a seed reference needs both a label and a digest, got %+v", ErrIncompleteInputs, s)
		}
	}
	return nil
}

// Write emits the record and its walk file into dir, as one pair. If the record
// cannot be written the walk copy is removed again, so a half-pair never
// survives to be verified.
func (g GateRecord) Write(dir, base string, walkJSON []byte) (recordPath, walkPath string, err error) {
	if base == "" || filepath.Base(base) != base {
		return "", "", fmt.Errorf("%w: record base %q must be a basename", ErrBadRecord, base)
	}
	if g.Walk.File != base+WalkSuffix {
		return "", "", fmt.Errorf("%w: record names walk file %q, writing %q",
			ErrRecordMismatch, g.Walk.File, base+WalkSuffix)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", "", err
	}
	walkPath = filepath.Join(dir, base+WalkSuffix)
	recordPath = filepath.Join(dir, base+RecordSuffix)
	if err := os.WriteFile(walkPath, walkJSON, 0o644); err != nil {
		return "", "", err
	}
	b, err := g.Marshal()
	if err != nil {
		os.Remove(walkPath)
		return "", "", err
	}
	if err := os.WriteFile(recordPath, append(b, '\n'), 0o644); err != nil {
		os.Remove(walkPath)
		return "", "", err
	}
	return recordPath, walkPath, nil
}

// LoadRecord reads and structurally checks one record.
func LoadRecord(path string) (GateRecord, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return GateRecord{}, err
	}
	var g GateRecord
	if err := json.Unmarshal(b, &g); err != nil {
		return GateRecord{}, fmt.Errorf("%s: %w: %v", path, ErrBadRecord, err)
	}
	if g.Schema != RecordSchema {
		return GateRecord{}, fmt.Errorf("%s: %w: schema %d, this build understands %d",
			path, ErrBadRecord, g.Schema, RecordSchema)
	}
	return g, nil
}

// VerifyRecord re-checks one record against the walk beside it and against the
// pin file. It is property 3, and it is the reason a record cannot quietly
// become decoration.
//
// It does NOT shell out to the oracle binaries: the check here is that the
// record's commits still equal the PINS, which is what rots when the pin file
// is re-recorded. Re-resolving the binaries themselves is tier 2 and lives in
// TestRealPinsResolveTheInstalledOracles.
func VerifyRecord(dir, name string, pins PinFile) error {
	g, err := LoadRecord(filepath.Join(dir, name))
	if err != nil {
		return err
	}
	where := filepath.Join(dir, name)

	if g.Walk.File == "" || filepath.Base(g.Walk.File) != g.Walk.File {
		return fmt.Errorf("%s: %w: walk file %q is not a basename", where, ErrBadRecord, g.Walk.File)
	}
	raw, err := os.ReadFile(filepath.Join(dir, g.Walk.File))
	if err != nil {
		return fmt.Errorf("%s: %w: the walk it names is not beside it: %v", where, ErrRecordMismatch, err)
	}
	if got := sha256Bytes(raw); !strings.EqualFold(got, g.Walk.SHA256) {
		// In full, both of them. Truncating here is how two different hashes
		// once rendered as one.
		return fmt.Errorf("%s: %w: %s hashes to %s, record says %s",
			where, ErrRecordMismatch, g.Walk.File, got, g.Walk.SHA256)
	}

	w, err := ParseWalk(raw)
	if err != nil {
		return fmt.Errorf("%s: %w", where, err)
	}
	if !slices.Equal(w.Census.Strings, g.Walk.Census.Strings) ||
		w.Census.Announced != g.Walk.Census.Announced ||
		w.Census.Unattributed != g.Walk.Census.Unattributed {
		return fmt.Errorf("%s: %w: the embedded census is not the walk's:\n embedded %+v\n walk     %+v",
			where, ErrRecordMismatch, g.Walk.Census, w.Census)
	}
	if !slices.Equal(w.Digests, g.Walk.Digests) {
		return fmt.Errorf("%s: %w: the embedded plate digests are not the walk's:\n embedded %v\n walk     %v",
			where, ErrRecordMismatch, g.Walk.Digests, w.Digests)
	}
	if !digestEqual(g.Payload.Digest, w.PayloadDigest) {
		return fmt.Errorf("%s: %w: the record names payload %q, the walk loaded %q",
			where, ErrRecordMismatch, g.Payload.Digest, w.PayloadDigest)
	}
	if err := checkInputs(g.Inputs); err != nil {
		return fmt.Errorf("%s: %w", where, err)
	}

	// Every pinned oracle must appear in the record, at the pinned commit. A
	// record naming two of three oracles is not a lesser record; it is a gate
	// that never established the third.
	for _, fp := range pins.Pins {
		i := slices.IndexFunc(g.Oracles, func(r Resolved) bool { return r.Name == fp.Name })
		if i < 0 {
			return fmt.Errorf("%s: %w: no oracle %q in the record, but pins.json pins one",
				where, ErrRecordMismatch, fp.Name)
		}
		if !commitEqual(g.Oracles[i].Commit, fp.Commit) {
			return fmt.Errorf("%s: %w: oracle %s recorded at %s, pins.json now says %s — "+
				"the record was made against a different toolchain; re-walk, do not edit it",
				where, ErrCommitMismatch, fp.Name, g.Oracles[i].Commit, fp.Commit)
		}
	}
	return nil
}

// Records lists the record files in dir, sorted.
func Records(dir string) ([]string, error) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range ents {
		if !e.IsDir() && strings.HasSuffix(e.Name(), RecordSuffix) {
			out = append(out, e.Name())
		}
	}
	slices.Sort(out)
	return out, nil
}

// VerifyAll verifies every record in dir and returns the names it verified.
// An empty directory is ErrRecordMissing, never an empty success: the whole
// point of this deliverable is that absence fails rather than passes.
func VerifyAll(dir string, pins PinFile) ([]string, error) {
	names, err := Records(dir)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrRecordMissing, err)
	}
	if len(names) == 0 {
		return nil, fmt.Errorf("%w: %s holds no %s file", ErrRecordMissing, dir, RecordSuffix)
	}
	var errs []error
	for _, n := range names {
		if err := VerifyRecord(dir, n, pins); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return names, nil
}

// StagesRecorded reports which stages have at least one verified-shape record.
func StagesRecorded(dir string) (map[string][]string, error) {
	names, err := Records(dir)
	if err != nil {
		return nil, err
	}
	out := map[string][]string{}
	for _, n := range names {
		g, err := LoadRecord(filepath.Join(dir, n))
		if err != nil {
			return nil, err
		}
		out[g.Stage] = append(out[g.Stage], n)
	}
	return out, nil
}

// sha256Bytes hashes bytes in memory, next to fileSHA256 which hashes a path.
func sha256Bytes(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// digestEqual compares payload digests written either grouped
// ("2527 1e58 ...", as the device shows them) or bare.
func digestEqual(a, b string) bool {
	strip := func(s string) string {
		return strings.ToLower(strings.Join(strings.Fields(s), ""))
	}
	return strip(a) != "" && strip(a) == strip(b)
}
