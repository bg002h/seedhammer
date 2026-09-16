package main

import (
	"encoding/json"
	"os"
	"regexp"
	"sort"
	"testing"
)

// F-545: shots_composer.js could not be RUN without a host-derived expectation
// set. Every `must` in it compares against `expect.*`, so with none supplied it
// threw on the first comparison — and a walk nobody can run goes stale in
// silence. It did: it carried two assertions this cycle had retired
// (`hash 1`, `hash abababab..abababab`) and nobody noticed until a review READ
// the file.
//
// expect_composer.json is now committed beside the walk and is its default, so
// the walk runs in one line. This gate is what stops the fixture and the walk
// drifting apart in the OTHER direction: the required field list is derived
// from the walk's own source, not written down here, so adding an `expect.X`
// to the walk without adding X to the fixture fails.
//
// The fixture's VALUES are kept honest separately, by
// `capture_composer.py --check-expect`, which re-derives them from the host
// artifacts and exits non-zero on drift. This test is about SHAPE; that one is
// about truth.
//
// MUTATION: delete any field from expect_composer.json -> the arm that needs it
// fails by name. MUTATION: add `expect.somethingNew` to the walk -> every arm
// that lacks it fails.
func TestShotsComposerExpectFixtureCoversEveryFieldTheWalkReads(t *testing.T) {
	src, err := os.ReadFile("shots_composer.js")
	if err != nil {
		t.Fatal(err)
	}
	// Derived from the walk, so the two cannot disagree about what it needs.
	re := regexp.MustCompile(`expect\.([A-Za-z]+)`)
	seen := map[string]bool{}
	for _, m := range re.FindAllStringSubmatch(string(src), -1) {
		seen[m[1]] = true
	}
	if len(seen) == 0 {
		t.Fatal("no expect.* references found in shots_composer.js: this gate is " +
			"reading the wrong file and would pass on an empty fixture")
	}
	var fields []string
	for f := range seen {
		fields = append(fields, f)
	}
	sort.Strings(fields)
	t.Logf("the walk reads %d expect fields: %v", len(fields), fields)

	blob, err := os.ReadFile("expect_composer.json")
	if err != nil {
		t.Fatalf("no committed expectation set: %v\nRegenerate with "+
			"design/journeys/capture_composer.py --emit-expect", err)
	}
	var all struct {
		Keyed   map[string]map[string]json.RawMessage `json:"keyed"`
		Keyless map[string]json.RawMessage            `json:"keyless"`
	}
	if err := json.Unmarshal(blob, &all); err != nil {
		t.Fatalf("expect_composer.json does not parse: %v", err)
	}

	// The keyed arm drives both forms and reads EVERY field.
	for _, form := range []string{"A", "B"} {
		got, ok := all.Keyed[form]
		if !ok {
			t.Errorf("expect_composer.json has no keyed form %q", form)
			continue
		}
		for _, f := range fields {
			if _, ok := got[f]; !ok {
				t.Errorf("keyed form %s is missing %q, which shots_composer.js reads", form, f)
			}
		}
	}

	// The key-less arm runs before any policy is composed, so it legitimately
	// carries only the four fields its own leg compares. Naming them here is
	// what keeps "fewer fields" from silently becoming "no fields".
	for _, f := range []string{"templateId", "templateStub", "entries", "strings"} {
		if _, ok := all.Keyless[f]; !ok {
			t.Errorf("the key-less arm is missing %q", f)
		}
	}
	if len(all.Keyless) == 0 {
		t.Error("the key-less arm is empty")
	}
}
