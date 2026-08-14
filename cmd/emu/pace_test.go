package main

import "testing"

// The pace knob's whole value is that a walk can raise it, so these pin the two
// things a wrong implementation would get wrong: that the default still yields
// on every Write (the human pace, unchanged), and that a raised pace yields
// exactly as often as it claims rather than approximately.

func TestDefaultPaceYieldsOnEveryWrite(t *testing.T) {
	setWritesPerYield(1)
	var p pacer
	for i := 0; i < 5; i++ {
		if !p.yield() {
			t.Fatalf("write %d did not yield at the default pace; the human pace is gone", i)
		}
	}
}

func TestARaisedPaceYieldsExactlyOncePerBatch(t *testing.T) {
	const every = 64
	setWritesPerYield(every)
	defer setWritesPerYield(1)

	var p pacer
	var yields, writes int
	for writes = 0; writes < every*4; writes++ {
		if p.yield() {
			yields++
			// The yield must land on the batch boundary, not merely the right
			// number of times: a pacer that yielded four times in a row and
			// then slept would starve the browser for a whole batch.
			if (writes+1)%every != 0 {
				t.Fatalf("yielded at write %d, which is not a multiple of %d", writes+1, every)
			}
		}
	}
	if want := writes / every; yields != want {
		t.Errorf("yielded %d times over %d writes, want %d", yields, writes, want)
	}
}

// Zero or negative would mean "never yield", which freezes the tab -- the exact
// failure stepPace exists to prevent. A walk that wedges the browser reports no
// result at all, which is worse than a slow one.
func TestPaceIsClampedSoAWalkCannotWedgeTheBrowser(t *testing.T) {
	defer setWritesPerYield(1)
	for _, n := range []int64{0, -1, -1000} {
		if got := setWritesPerYield(n); got != 1 {
			t.Errorf("setWritesPerYield(%d) = %d, want 1", n, got)
		}
		var p pacer
		if !p.yield() {
			t.Errorf("pace %d did not yield; the browser would never get a turn", n)
		}
	}
}

// The same failure from the other direction. Nothing above 2048 measured any
// faster, so an unbounded knob offers only ways to starve the browser.
func TestPaceIsCappedAtTheTop(t *testing.T) {
	defer setWritesPerYield(1)
	for _, n := range []int64{maxPace + 1, 100000, 1 << 40} {
		if got := setWritesPerYield(n); got != maxPace {
			t.Errorf("setWritesPerYield(%d) = %d, want the %d cap", n, got, maxPace)
		}
	}
	// The cap must be an actual bound on yielding, not just on the stored
	// number: at maxPace a yield still has to arrive on the batch boundary.
	setWritesPerYield(1 << 40)
	var p pacer
	for i := int64(0); i < maxPace-1; i++ {
		if p.yield() {
			t.Fatalf("yielded at write %d, before the %d cap", i+1, maxPace)
		}
	}
	if !p.yield() {
		t.Errorf("never yielded within %d writes; the browser would be starved", maxPace)
	}
}

// The pace the walk actually ships with must be reachable -- a cap set below it
// would silently slow every walk.
func TestTheWalksDefaultPaceIsUnderTheCap(t *testing.T) {
	defer setWritesPerYield(1)
	const walkDefault = 2048 // walk_trace_a.js run({pace})
	if got := setWritesPerYield(walkDefault); got != walkDefault {
		t.Errorf("the walk's default pace %d clamps to %d", walkDefault, got)
	}
}

// A pace change must reach a cut that is already running, because that is when
// the operator finds out the plate is slow.
func TestARaisedPaceTakesEffectMidPlate(t *testing.T) {
	setWritesPerYield(1)
	defer setWritesPerYield(1)

	var p pacer
	if !p.yield() {
		t.Fatal("expected a yield at the default pace")
	}
	setWritesPerYield(8)
	for i := 0; i < 7; i++ {
		if p.yield() {
			t.Fatalf("yielded at write %d after the pace was raised to 8", i)
		}
	}
	if !p.yield() {
		t.Error("did not yield on the 8th write after the pace was raised")
	}
}
