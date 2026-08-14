package main

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

const (
	testMD1 = "md1yqpqqxqq8xtwhw4xwn4qh"
	testMK1 = "mk1yqpqqxqq8xtwhw4xwn4qh"
)

// TestEngravedRecorderCountsOnlyWhatWasEngraved is the census property the
// §4.5 gate rests on.
//
// Announcing a string is not engraving it, and the difference is the whole
// reason the seam is two events. A walk that validated three variants and cut
// ONE must report one string -- not three, and not zero.
func TestEngravedRecorderCountsOnlyWhatWasEngraved(t *testing.T) {
	r := newEngravedRecorder()

	// One string, three variants: TEXT+QR, TEXT ONLY, QR ONLY.
	r.PlateText([]uint64{1, 2, 3}, testMD1)
	if got := r.Strings(); len(got) != 0 {
		t.Fatalf("announcing recorded %q -- the operator has not chosen a variant yet", got)
	}

	// The operator picks TEXT ONLY and accepts it.
	r.PlateEngraved(2)

	got := r.Strings()
	if !slices.Equal(got, []string{testMD1}) {
		t.Errorf("recorded %q, want exactly one %q -- three variants were offered and one was cut",
			got, testMD1)
	}
}

// TestEngravedRecorderKeepsEngraveOrder pins that the census is a SEQUENCE.
//
// A bundle is cut card by card and plate by plate, and a set that arrives in
// the wrong order describes a different restore than the walk asked for. A
// recorder backed by a map would pass every other test in this file and fail
// this one at random.
func TestEngravedRecorderKeepsEngraveOrder(t *testing.T) {
	r := newEngravedRecorder()
	r.PlateText([]uint64{1}, testMD1)
	r.PlateText([]uint64{2}, testMK1)

	r.PlateEngraved(2)
	r.PlateEngraved(1)

	if got, want := r.Strings(), []string{testMK1, testMD1}; !slices.Equal(got, want) {
		t.Errorf("recorded %q, want %q -- the census is reporting a different cut order than "+
			"the one that happened", got, want)
	}
}

// TestEngravedRecorderIgnoresUnannouncedPlates is the anti-inflation property.
//
// Seed, passphrase and free-text plates never pass through validateMdmk, so
// they arrive with id 0. A recorder that guessed -- attributing a finished
// plate to whichever string it heard about last -- would let "validate an md1,
// back out, later cut a seed" report the md1 as engraved. A gate that reports a
// plate nobody cut is worse than no gate.
func TestEngravedRecorderIgnoresUnannouncedPlates(t *testing.T) {
	r := newEngravedRecorder()
	r.PlateText([]uint64{7}, testMD1)

	r.PlateEngraved(0)   // a seed plate
	r.PlateEngraved(999) // an id from nowhere

	if got := r.Strings(); len(got) != 0 {
		t.Errorf("unannounced plates entered the census as %q", got)
	}

	// The positive control. Without it, a recorder that dropped EVERYTHING
	// would pass the assertion above and look like it was enforcing a rule.
	r.PlateEngraved(7)
	if got := r.Strings(); !slices.Equal(got, []string{testMD1}) {
		t.Fatalf("an ANNOUNCED plate was dropped too (%q) -- nothing is being recorded at all, "+
			"so the check above proves nothing", got)
	}
}

// TestEngravedCensusJSONDistinguishesEmptyFromBroken is why the JSON carries
// counts rather than only the strings.
//
// An empty census has two very different causes: nothing was cut, or the hook
// is not wired up. A walk that cannot tell them apart reports a clean gate for
// a harness that was never connected -- the false-PASS shape this project has
// been bitten by before.
func TestEngravedCensusJSONDistinguishesEmptyFromBroken(t *testing.T) {
	decode := func(t *testing.T, s string) map[string]any {
		t.Helper()
		var m map[string]any
		if err := json.Unmarshal([]byte(s), &m); err != nil {
			t.Fatalf("census is not JSON: %v (%q)", err, s)
		}
		return m
	}

	// Nothing wired: no announcements, nothing engraved.
	broken := decode(t, newEngravedRecorder().StringsJSON())
	if n := broken["announced"].(float64); n != 0 {
		t.Errorf("a recorder that saw nothing reports announced=%v, want 0", n)
	}

	// Wired, and a walk that legitimately cut nothing yet.
	r := newEngravedRecorder()
	r.PlateText([]uint64{1, 2, 3}, testMD1)
	r.PlateEngraved(0) // a seed plate went past
	live := decode(t, r.StringsJSON())
	if n := live["announced"].(float64); n != 3 {
		t.Errorf("announced=%v, want 3 -- this is what tells a walk the hook is connected", n)
	}
	if n := live["unattributed"].(float64); n != 1 {
		t.Errorf("unattributed=%v, want 1 -- a seed plate was cut and must not vanish silently", n)
	}
	if got := live["strings"].([]any); len(got) != 0 {
		t.Errorf("strings=%v, want empty", got)
	}

	// An empty census must serialise as [] and not null, or a walk doing
	// `for (const s of census.strings)` throws instead of reporting zero.
	if !strings.Contains(r.StringsJSON(), `"strings":[]`) {
		t.Errorf("empty census serialised as %q, want a literal empty array", r.StringsJSON())
	}
}
