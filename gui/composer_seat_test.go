package gui

import (
	"strings"
	"testing"
)

// TestComposerAssignableSlotsCountsSeatsNotSources is §7d's counting rule,
// and getting it wrong in either direction is a refusal the operator cannot
// act on: too low refuses a payload that would have worked, too high walks
// them through seating and refuses at the end.
func TestComposerAssignableSlotsCountsSeatsNotSources(t *testing.T) {
	st := &composerState{list: composerTwoPathList()} // 4 slots
	st.sources = []composerSource{{kind: composerSourceKey}, {kind: composerSourceKey}}
	if got := composerAssignableSlots(st); got != 2 {
		t.Errorf("two key records fill %d slots, want 2", got)
	}
	st.sources = append(st.sources, composerSource{kind: composerSourceSeed, seedID: 0})
	if got := composerAssignableSlots(st); got != composerSlotCount(st.list) {
		t.Errorf("with a seed present %d slots are assignable, want all %d: a seed fills "+
			"any number of slots (C12, §4f)", got, composerSlotCount(st.list))
	}
	st.sources = st.sources[:2]
	st.assigned = []composerAssignment{{src: 0}, {src: 1}, {src: -1}, {src: -1}}
	if got := composerUnfilledSlots(st); len(got) != 2 || got[0] != 2 || got[1] != 3 {
		t.Errorf("unfilled slots = %v, want [2 3]", got)
	}
	assertModalBodyFits(t, "the §8p shortfall refusal", errorScreenBody,
		composerCopyShortfall(4, 3, []uint8{3}))
	// The §8p body NAMES no cause, and that is the rule rather than an
	// omission (§7d): the C5 lesson is taught at the shape step by §8k.
	body := composerCopyShortfall(4, 2, []uint8{2, 3})
	for _, forbidden := range []string{"because", "seed", "card"} {
		if strings.Contains(strings.ToLower(body), forbidden) {
			t.Errorf("the shortfall body guesses a cause (%q):\n%s", forbidden, body)
		}
	}
}

// TestComposerSeatingCompleteTracksEverySlot is the predicate the transition
// out of seating turns on: all-or-nothing (§7d), so "complete" must mean every
// slot and not most of them.
func TestComposerSeatingCompleteTracksEverySlot(t *testing.T) {
	st := &composerState{list: composerTwoPathList()}
	st.assigned = make([]composerAssignment, composerSlotCount(st.list))
	for i := range st.assigned {
		st.assigned[i].src = -1
	}
	if composerSeatingComplete(st) {
		t.Fatal("an all-unseated template reports complete seating")
	}
	for i := range st.assigned {
		st.assigned[i].src = i
	}
	if !composerSeatingComplete(st) {
		t.Fatal("a fully seated template does not report complete seating")
	}
	// ONE slot short is NOT complete, which is the case the all-or-nothing
	// rule exists for: a partial seating is exactly the state that produces a
	// plausible wrong address.
	st.assigned[len(st.assigned)-1].src = -1
	if composerSeatingComplete(st) {
		t.Errorf("a template one slot short of seated reports complete; §7d refuses at " +
			"the transition instead, with §8p's counts")
	}
	if got := composerUnfilledSlots(st); len(got) != 1 {
		t.Errorf("one slot short leaves %d unfilled, want 1", len(got))
	}
}
