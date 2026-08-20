package gui

import (
	"reflect"
	"testing"
)

// script drives gatherSlotSeeds without a GUI: each entry is what the operator
// does at the NEXT prompt, in order. A negative value means Back.
type seedStep int

const back seedStep = -1

func scriptedAsk(t *testing.T, steps []seedStep) (func(int) (int, bool), *[]int) {
	t.Helper()
	n := 0
	var visited []int
	return func(i int) (int, bool) {
		if n >= len(steps) {
			t.Fatalf("the flow asked for slot %d after the script ran out "+
				"(%d steps); it is looping", i, len(steps))
		}
		s := steps[n]
		n++
		visited = append(visited, i)
		if s == back {
			return 0, false
		}
		return int(s), true
	}, &visited
}

// TestGatherSlotSeedsBackStepsBackOneSlot is the directive's core case: Back at
// the second slot must return to the first, not abandon the build.
func TestGatherSlotSeedsBackStepsBackOneSlot(t *testing.T) {
	ask, visited := scriptedAsk(t, []seedStep{10, back, 11, 12})
	ids, ok := gatherSlotSeeds(2, ask)
	if !ok {
		t.Fatal("Back at the SECOND slot abandoned the whole build -- the " +
			"pre-2026-08-19 behaviour, which also discarded the first slot's seed")
	}
	if want := []int{0, 1, 0, 1}; !reflect.DeepEqual(*visited, want) {
		t.Fatalf("prompt order was %v, want %v (back at slot 1 returns to slot 0)",
			*visited, want)
	}
	if want := []int{11, 12}; !reflect.DeepEqual(ids, want) {
		t.Fatalf("ids = %v, want %v", ids, want)
	}
}

// TestGatherSlotSeedsTruncatesOnStepBack is the funds-relevant half, and the
// one a walk-and-leave test cannot reach.
//
// buildSlotSources reads seedIDs[hi], where hi indexes SelfSlots -- so the slice
// is POSITIONAL. If stepping back appends instead of truncating, the re-entered
// seed lands at the wrong index: slot 0 keeps the seed the operator just
// REPLACED, and slot 1 gets the replacement. Both slots would then be engraved
// with a key the operator did not choose for them.
func TestGatherSlotSeedsTruncatesOnStepBack(t *testing.T) {
	// Enter 10 for slot 0, Back, replace it with 99, then 12 for slot 1.
	ask, _ := scriptedAsk(t, []seedStep{10, back, 99, 12})
	ids, ok := gatherSlotSeeds(2, ask)
	if !ok {
		t.Fatal("flow abandoned")
	}
	if len(ids) != 2 {
		t.Fatalf("ids has %d entries for 2 slots (%v): a step back GREW the "+
			"slice instead of truncating it, so every later slot's seed is "+
			"shifted by one", len(ids), ids)
	}
	if ids[0] != 99 {
		t.Errorf("slot 0 bound to seed %d, want the REPLACEMENT 99; the "+
			"discarded seed is still bound to the slot the operator re-entered",
			ids[0])
	}
	if ids[1] != 12 {
		t.Errorf("slot 1 bound to seed %d, want 12; it inherited another "+
			"slot's seed", ids[1])
	}
}

// TestGatherSlotSeedsBackOnFirstSlotLeaves pins the boundary the directive
// keeps: the first step's Back is still an exit.
func TestGatherSlotSeedsBackOnFirstSlotLeaves(t *testing.T) {
	ask, _ := scriptedAsk(t, []seedStep{back})
	ids, ok := gatherSlotSeeds(2, ask)
	if ok {
		t.Fatal("Back on the FIRST slot should leave the build")
	}
	if ids != nil {
		t.Errorf("a leaving flow returned ids %v; they must not be usable", ids)
	}
}

// TestGatherSlotSeedsWalksBackToTheStart covers repeated Backs: stepping back
// past the first slot leaves, and does not underflow the slice.
func TestGatherSlotSeedsWalksBackToTheStart(t *testing.T) {
	ask, visited := scriptedAsk(t, []seedStep{10, 11, back, back, back})
	if _, ok := gatherSlotSeeds(3, ask); ok {
		t.Fatal("backing out of every slot should leave the build")
	}
	if want := []int{0, 1, 2, 1, 0}; !reflect.DeepEqual(*visited, want) {
		t.Fatalf("prompt order was %v, want %v", *visited, want)
	}
}

// TestGatherSlotSeedsSingleSlot is the degenerate case -- one held slot means
// Back has nowhere to step back to.
func TestGatherSlotSeedsSingleSlot(t *testing.T) {
	ask, _ := scriptedAsk(t, []seedStep{7})
	ids, ok := gatherSlotSeeds(1, ask)
	if !ok || !reflect.DeepEqual(ids, []int{7}) {
		t.Fatalf("gatherSlotSeeds(1) = %v, %v; want [7], true", ids, ok)
	}
}
