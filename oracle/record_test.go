package oracle

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Everything above TestS0GateHasARecord is hermetic: fake walks in t.TempDir(),
// fake pins, no binaries, no browser. The two tests at the bottom are the ones
// that read the REAL records directory, and they are the deliverable — the
// hermetic ones only prove the machinery they lean on can actually fail.

// greenWalk is the shape cmd/emu/walk_trace_a.js run() returns on a completed
// six-plate Trace A. Written as a map so a test can knock one field out and see
// what happens, which is the only way to know a refusal is real.
func greenWalk() map[string]any {
	return map[string]any{
		"pace":           4096,
		"paceOverridden": false,
		"elapsedSec":     165,
		"payloadDigest":  "2527 1e58 3f3e aa03 ae18 f359 c72b 76e3",
		"census": map[string]any{
			"strings":      []string{"mk1aaa", "mk1bbb", "mk1ccc"},
			"announced":    6,
			"unattributed": 0,
		},
		"digests": []string{"d0", "d1", "d2"},
		"acts":    []any{},
		"screen":  "Engraving completed successfully",
		"ok":      true,
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func testInputs() InputTuple {
	return InputTuple{
		Template:  "bundle-engrave",
		N:         3,
		SlotOrder: []int{0, 1, 2},
		FPChoice:  "default",
		Origins:   []string{"m/48h/0h/0h/2h"},
		Seeds:     []SeedRef{NewSeedRef("payload:masterA", "abandon abandon about")},
	}
}

func testPayload() Payload {
	return Payload{Name: "sysw_cards_payload.bin", Digest: "2527 1e58 3f3e aa03 ae18 f359 c72b 76e3"}
}

func testOracles() []Resolved {
	return []Resolved{
		{Name: "md", Commit: "5a0a4f41017d71d47f70684c145702d4ca0c3aa9", Method: ByBinaryHash},
		{Name: "mk", Commit: "018aca00b4a2da50323e4c83c82d019febf17e14", Method: ByBinaryHash},
	}
}

func testPins() PinFile {
	return PinFile{Pins: []FilePin{
		{Pin: Pin{Name: "md", Commit: "5a0a4f41017d71d47f70684c145702d4ca0c3aa9"}},
		{Pin: Pin{Name: "mk", Commit: "018aca00b4a2da50323e4c83c82d019febf17e14"}},
	}}
}

// writePair emits a valid record + walk pair into a fresh dir and returns both.
func writePair(t *testing.T) (dir, recordName, walkName string) {
	t.Helper()
	dir = t.TempDir()
	const base = "S0-test"
	raw := mustJSON(t, greenWalk())
	g, err := NewRecord("S0", testOracles(), testInputs(), testPayload(), base+WalkSuffix, raw)
	if err != nil {
		t.Fatalf("a green walk was refused: %v", err)
	}
	if _, _, err := g.Write(dir, base, raw); err != nil {
		t.Fatal(err)
	}
	return dir, base + RecordSuffix, base + WalkSuffix
}

// TestNewRecordRefusesWithoutAGreenWalk is property 1 — "no run, no record" —
// and it is written as a table because the interesting failure is not "it
// refuses" but "it refuses for each of the ways a walk can be not-a-walk".
//
// The last four rows matter most: each is a walk that DID run and still must
// not anchor a gate. A record built from a walk that cut something it cannot
// name, or that never recorded a plate digest, would be a gate passing on
// evidence it did not collect.
func TestNewRecordRefusesWithoutAGreenWalk(t *testing.T) {
	for _, tc := range []struct {
		name string
		walk []byte
		want error
	}{
		{"nothing at all", nil, ErrNoWalk},
		{"whitespace", []byte("   \n"), ErrNoWalk},
		{"not JSON", []byte("the walk went fine, honest"), ErrNoWalk},
		{"an empty object", []byte("{}"), ErrWalkNotGreen},
		{"ok=false", mutateWalk(t, func(w map[string]any) { w["ok"] = false }), ErrWalkNotGreen},
		{"empty census", mutateWalk(t, func(w map[string]any) {
			w["census"].(map[string]any)["strings"] = []string{}
		}), ErrWalkNotGreen},
		{"something unnamed was cut", mutateWalk(t, func(w map[string]any) {
			w["census"].(map[string]any)["unattributed"] = 1
		}), ErrWalkNotGreen},
		{"the census hook is not wired", mutateWalk(t, func(w map[string]any) {
			w["census"].(map[string]any)["announced"] = 0
		}), ErrWalkNotGreen},
		{"no per-plate digests", mutateWalk(t, func(w map[string]any) { w["digests"] = []string{} }), ErrWalkNotGreen},
		{"an empty per-plate digest", mutateWalk(t, func(w map[string]any) {
			w["digests"] = []string{"d0", "", "d2"}
		}), ErrWalkNotGreen},
		{"no payload digest", mutateWalk(t, func(w map[string]any) { w["payloadDigest"] = "" }), ErrWalkNotGreen},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewRecord("S0", testOracles(), testInputs(), testPayload(), "x"+WalkSuffix, tc.walk)
			if !errors.Is(err, tc.want) {
				t.Fatalf("built a record from %s: err = %v, want %v", tc.name, err, tc.want)
			}
		})
	}
}

func mutateWalk(t *testing.T, f func(map[string]any)) []byte {
	t.Helper()
	w := greenWalk()
	f(w)
	return mustJSON(t, w)
}

// The record's own fields have to be there too. A record with no oracles is the
// exact artifact D5 replaces.
func TestNewRecordRefusesAnIncompleteRecord(t *testing.T) {
	raw := mustJSON(t, greenWalk())
	for _, tc := range []struct {
		name string
		call func() (GateRecord, error)
		want error
	}{
		{"no stage", func() (GateRecord, error) {
			return NewRecord("", testOracles(), testInputs(), testPayload(), "x"+WalkSuffix, raw)
		}, ErrBadRecord},
		{"no oracles", func() (GateRecord, error) {
			return NewRecord("S0", nil, testInputs(), testPayload(), "x"+WalkSuffix, raw)
		}, ErrBadRecord},
		{"a walk path instead of a basename", func() (GateRecord, error) {
			return NewRecord("S0", testOracles(), testInputs(), testPayload(), "../elsewhere"+WalkSuffix, raw)
		}, ErrBadRecord},
		{"no template", func() (GateRecord, error) {
			in := testInputs()
			in.Template = ""
			return NewRecord("S0", testOracles(), in, testPayload(), "x"+WalkSuffix, raw)
		}, ErrIncompleteInputs},
		{"no seeds", func() (GateRecord, error) {
			in := testInputs()
			in.Seeds = nil
			return NewRecord("S0", testOracles(), in, testPayload(), "x"+WalkSuffix, raw)
		}, ErrIncompleteInputs},
		{"a seed with no digest", func() (GateRecord, error) {
			in := testInputs()
			in.Seeds = []SeedRef{{Label: "payload:masterA"}}
			return NewRecord("S0", testOracles(), in, testPayload(), "x"+WalkSuffix, raw)
		}, ErrIncompleteInputs},
		{"the record names a payload the walk did not load", func() (GateRecord, error) {
			p := testPayload()
			p.Digest = "55ad b800 0000 0000 0000 0000 0000 0000"
			return NewRecord("S0", testOracles(), testInputs(), p, "x"+WalkSuffix, raw)
		}, ErrRecordMismatch},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := tc.call(); !errors.Is(err, tc.want) {
				t.Fatalf("accepted %s: err = %v, want %v", tc.name, err, tc.want)
			}
		})
	}
}

// Property 2: the record carries the walk's census and per-plate digests, not a
// summary of them. Seed WORDS must still never appear.
func TestRecordEmbedsTheWalksCensusAndDigests(t *testing.T) {
	raw := mustJSON(t, greenWalk())
	g, err := NewRecord("S0", testOracles(), testInputs(), testPayload(), "x"+WalkSuffix, raw)
	if err != nil {
		t.Fatal(err)
	}
	b, err := g.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, want := range []string{
		"mk1aaa", "mk1bbb", "mk1ccc", // the census, verbatim
		`"d0"`, `"d1"`, `"d2"`, // the per-plate digests
		"5a0a4f41017d71d47f70684c145702d4ca0c3aa9", // an oracle COMMIT
		"slot_order", "origins", "plate_digests",
		g.Walk.SHA256, // the binding to the raw walk file
	} {
		if !strings.Contains(s, want) {
			t.Errorf("the record is missing %q:\n%s", want, s)
		}
	}
	if strings.Contains(s, "abandon") {
		t.Errorf("the record carries seed words:\n%s", s)
	}
	if len(g.Walk.SHA256) != 64 {
		t.Errorf("walk sha256 is %q, want a full 64-char hex digest", g.Walk.SHA256)
	}
}

// TestVerifyRecordCatchesATamperedWalk is the "run A's record beside run B's
// artifacts" case the plain-command design could not detect.
func TestVerifyRecordCatchesATamperedWalk(t *testing.T) {
	dir, rec, walk := writePair(t)
	if err := VerifyRecord(dir, rec, testPins()); err != nil {
		t.Fatalf("a freshly written pair does not verify: %v", err)
	}

	other := greenWalk()
	other["census"].(map[string]any)["strings"] = []string{"mk1zzz", "mk1yyy", "mk1xxx"}
	if err := os.WriteFile(filepath.Join(dir, walk), mustJSON(t, other), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := VerifyRecord(dir, rec, testPins()); !errors.Is(err, ErrRecordMismatch) {
		t.Fatalf("a swapped walk file verified: %v", err)
	}
}

// And the other direction: the SHA alone is not the binding. Editing the
// record's embedded census leaves the walk file's hash intact, so this is what
// proves the census and digests are compared field by field.
func TestVerifyRecordCatchesATamperedRecord(t *testing.T) {
	dir, rec, _ := writePair(t)
	for _, tc := range []struct {
		name string
		edit func(*GateRecord)
	}{
		{"an edited census string", func(g *GateRecord) { g.Walk.Census.Strings[0] = "mk1forged" }},
		{"an edited plate digest", func(g *GateRecord) { g.Walk.Digests[1] = "d9" }},
		{"an inflated announced count", func(g *GateRecord) { g.Walk.Census.Announced = 99 }},
		{"a re-labelled payload", func(g *GateRecord) { g.Payload.Digest = "0000 0000 0000 0000" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g, err := LoadRecord(filepath.Join(dir, rec))
			if err != nil {
				t.Fatal(err)
			}
			tc.edit(&g)
			b, err := g.Marshal()
			if err != nil {
				t.Fatal(err)
			}
			p := filepath.Join(t.TempDir(), rec)
			if err := os.WriteFile(p, b, 0o644); err != nil {
				t.Fatal(err)
			}
			// The walk file has to travel with it, or this would pass for the
			// wrong reason — a missing walk file, not an edited record.
			raw, err := os.ReadFile(filepath.Join(dir, g.Walk.File))
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(filepath.Dir(p), g.Walk.File), raw, 0o644); err != nil {
				t.Fatal(err)
			}
			if err := VerifyRecord(filepath.Dir(p), rec, testPins()); !errors.Is(err, ErrRecordMismatch) {
				t.Fatalf("%s verified: %v", tc.name, err)
			}
		})
	}
}

func TestVerifyRecordRefusesAHalfPair(t *testing.T) {
	dir, rec, walk := writePair(t)
	if err := os.Remove(filepath.Join(dir, walk)); err != nil {
		t.Fatal(err)
	}
	if err := VerifyRecord(dir, rec, testPins()); !errors.Is(err, ErrRecordMismatch) {
		t.Fatalf("a record with no walk beside it verified: %v", err)
	}
}

// A record must not be able to point verification at a file outside its own
// directory — the record is data, and data that names a path is a traversal
// waiting to happen.
func TestVerifyRecordRefusesAWalkFileOutsideItsDirectory(t *testing.T) {
	dir, rec, _ := writePair(t)
	g, err := LoadRecord(filepath.Join(dir, rec))
	if err != nil {
		t.Fatal(err)
	}
	g.Walk.File = "../" + g.Walk.File
	b, _ := g.Marshal()
	if err := os.WriteFile(filepath.Join(dir, rec), b, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := VerifyRecord(dir, rec, testPins()); !errors.Is(err, ErrBadRecord) {
		t.Fatalf("a record naming a walk outside its directory verified: %v", err)
	}
}

// The record's commits are checked against the CURRENT pin file, so a record
// made against a toolchain that has since been re-pinned stops verifying. It
// must be re-walked, not re-typed.
func TestVerifyRecordRefusesARecordFromAnotherToolchain(t *testing.T) {
	dir, rec, _ := writePair(t)
	pins := testPins()
	pins.Pins[0].Commit = "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef"
	if err := VerifyRecord(dir, rec, pins); !errors.Is(err, ErrCommitMismatch) {
		t.Fatalf("a record from a different toolchain verified: %v", err)
	}
}

// A record naming two of three oracles is not a lesser record; it is a gate
// that never established the third.
func TestVerifyRecordRefusesAMissingOracle(t *testing.T) {
	dir, rec, _ := writePair(t)
	pins := testPins()
	pins.Pins = append(pins.Pins, FilePin{Pin: Pin{Name: "ms", Commit: "bf77f89aca8e28601921fca974089d278060cab0"}})
	if err := VerifyRecord(dir, rec, pins); !errors.Is(err, ErrRecordMismatch) {
		t.Fatalf("a record missing a pinned oracle verified: %v", err)
	}
}

// TestVerifyAllRefusesAnEmptyDirectory is the fail-closed clause in miniature.
// An empty sweep returning success is the failure mode this whole deliverable
// exists to prevent, so it is asserted directly rather than inferred from the
// real directory happening to be non-empty.
func TestVerifyAllRefusesAnEmptyDirectory(t *testing.T) {
	if _, err := VerifyAll(t.TempDir(), testPins()); !errors.Is(err, ErrRecordMissing) {
		t.Fatalf("an empty records directory verified: %v", err)
	}
	if _, err := VerifyAll(filepath.Join(t.TempDir(), "not-there"), testPins()); !errors.Is(err, ErrRecordMissing) {
		t.Fatalf("a missing records directory verified: %v", err)
	}
}

// ---------------------------------------------------------------------------
// The two tests below read the real gaterecords/ directory. They are the gate.
// ---------------------------------------------------------------------------

// TestS0GateHasARecord is the last clause of S0's gate: the harness must print
// resolved oracle commits and the input tuple into a gate record, and ABSENCE
// MUST BE A FAILURE RATHER THAN A SILENCE.
//
// It never skips. Not under -short, not when the oracle binaries are missing,
// not in CI. There is nothing here that needs either — the record is a committed
// file, and the question is whether it is there.
func TestS0GateHasARecord(t *testing.T) {
	const how = "\nProduce one: build and serve cmd/emu, run\n" +
		"    const w = await import(\"./walk_trace_a.js\");\n" +
		"    run(...).then(r => { window.__walk = r })   // fire and forget; poll window.__walk\n" +
		"then\n" +
		"    go run ./cmd/gaterecord -stage S0 -walk <saved __walk>.json \\\n" +
		"        -inputs oracle/gaterecords/S0-trace-a.inputs.json -base S0-trace-a"

	stages, err := StagesRecorded(GateRecordsDir)
	if err != nil {
		t.Fatalf("no gate records directory at %s: %v%s", GateRecordsDir, err, how)
	}
	if len(stages["S0"]) == 0 {
		t.Fatalf("S0 has no gate record in %s (stages present: %v).%s", GateRecordsDir, keys(stages), how)
	}
	t.Logf("S0 gate records: %v", stages["S0"])
}

// TestEveryGateRecordOnDiskVerifies re-checks the pairing on every record: the
// walk file still hashes to what the record says, its census and plate digests
// are still the ones embedded, and every pinned oracle appears at the pinned
// commit.
//
// It does not shell out to the oracle binaries — that is
// TestRealPinsResolveTheInstalledOracles, which is tier 2. What it catches is
// the drift that needs no binary at all: an edited record, a swapped walk, a
// re-recorded pin file.
func TestEveryGateRecordOnDiskVerifies(t *testing.T) {
	pins, err := LoadPins("pins.json")
	if err != nil {
		t.Fatal(err)
	}
	names, err := VerifyAll(GateRecordsDir, pins)
	if err != nil {
		t.Fatalf("gate records do not verify: %v", err)
	}
	t.Logf("verified %d gate record(s): %v", len(names), names)
}

func keys(m map[string][]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
