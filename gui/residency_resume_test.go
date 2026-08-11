package gui

import (
	"errors"
	"testing"

	"seedhammer.com/bezier"
	"seedhammer.com/bspline"
)

var errFakeDriver = errors.New("fake driver failure")

// countingKnotter is a driver stub: it records how many knots it was fed so a
// test can prove the fast-forward loop actually ran.
type countingKnotter struct {
	n   int
	err error
}

func (k *countingKnotter) Knot(bspline.Knot) (uint, error) {
	k.n++
	return 1, k.err
}

// TestSplineResumerZeroesTheCatchupArray is F-108 item (2b).
//
// The catch-up array is a FRESH copy of the history knots, made per restart by
// SafePointer.Resume. Once splineResumer.Knot sets s.catchup = nil -- which it
// does on the very first resumed knot -- nothing outside that block can reach
// the array, so a job-level sweep would zero a nil slice while every restart
// left another intact copy behind. That is exactly the placement R0 round 2
// rejected.
//
// This test holds its OWN reference to the array and reads the memory back,
// rather than asking the resumer whether it thinks it cleared anything.
//
// Mutation this pins: delete `defer clear(c)` from splineResumer.Knot.
func TestSplineResumerZeroesTheCatchupArray(t *testing.T) {
	catchup := []bspline.Knot{
		{Ctrl: bezier.Pt(11, 22), T: 3},
		{Ctrl: bezier.Pt(33, 44), T: 4},
		{Ctrl: bezier.Pt(55, 66), T: 5},
	}
	mine := catchup // our own reference to the same array

	drv := &countingKnotter{}
	res := newSplineResumer(drv, catchup)
	if _, err := res.Knot(bspline.Knot{Ctrl: bezier.Pt(77, 88), T: 6}); err != nil {
		t.Fatalf("Knot: %v", err)
	}

	if drv.n < len(mine) {
		t.Fatalf("INCONCLUSIVE: the fast-forward loop fed %d knots, want at least %d -- "+
			"the catch-up path did not run, so this test cannot witness the clear",
			drv.n, len(mine))
	}
	for i, k := range mine {
		if k != (bspline.Knot{}) {
			t.Fatalf("catch-up knot %d survives as %+v -- it is a per-restart copy of the "+
				"plate geometry and nothing else can reach it", i, k)
		}
	}
}

// TestSplineResumerZeroesTheCatchupArrayOnDriverError pins the error path.
// The fast-forward loop returns early on a driver error, so a clear placed
// after the loop would miss it -- which is why the implementation uses a defer.
//
// Mutation this pins: move `defer clear(c)` to a plain clear after the loop.
func TestSplineResumerZeroesTheCatchupArrayOnDriverError(t *testing.T) {
	catchup := []bspline.Knot{
		{Ctrl: bezier.Pt(11, 22), T: 3},
		{Ctrl: bezier.Pt(33, 44), T: 4},
	}
	mine := catchup

	drv := &countingKnotter{err: errFakeDriver}
	res := newSplineResumer(drv, catchup)
	if _, err := res.Knot(bspline.Knot{Ctrl: bezier.Pt(77, 88), T: 6}); err == nil {
		t.Fatal("INCONCLUSIVE: the stub driver did not report an error, so the early-return path never ran")
	}
	for i, k := range mine {
		if k != (bspline.Knot{}) {
			t.Fatalf("catch-up knot %d survives as %+v after a driver error -- "+
				"the clear does not cover the early return", i, k)
		}
	}
}

// TestReleaseResumeStateOnlyClearsAnAbandonedJob is F-108 item (2)'s gui half,
// and it pins BOTH directions -- which is the whole point.
//
// Zeroing too little leaves the plate geometry resident. Zeroing too EARLY is
// worse: SafePointer.history is what catchup() re-reads on the operator's
// hold-to-resume (gui/gui.go:2747), so clearing it while a restart is still
// reachable drives the catch-up motion to the origin at T:0 and wrecks the
// plate. That was R0 round 1's Critical.
//
// Mutations this pins: (a) empty releaseResumeState -- caught by the terminal
// cases; (b) delete the terminal-state switch so it always clears -- caught by
// the running/stopping cases.
func TestReleaseResumeStateOnlyClearsAnAbandonedJob(t *testing.T) {
	for _, tc := range []struct {
		name      string
		state     engraveState
		wantClear bool
	}{
		{"done -- abandoned", engraveDone, true},
		{"stopped -- abandoned", engraveStopped, true},
		{"failed -- abandoned", engraveFailed, true},
		{"running -- a restart is reachable", engraveRunning, false},
		{"stopping -- the goroutine is still winding down", engraveStopping, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := &engraveJob{}
			e.status.State = tc.state
			p := bezier.Pt(1234, 5678)
			for i := 0; i < 8; i++ {
				e.safePoint.Knot(bspline.Knot{Ctrl: p, T: 5})
			}

			e.releaseResumeState()

			cleared := e.safePoint.HistoryLen() == 0
			if cleared != tc.wantClear {
				if tc.wantClear {
					t.Errorf("resume state survives releaseResumeState in state %v -- "+
						"the job is abandoned and the knots are plate geometry", tc.state)
				} else {
					t.Errorf("releaseResumeState CLEARED resume state in state %v -- "+
						"a restart is still reachable, and a resumed cut would drive the head "+
						"to the origin at T:0 and wreck the plate", tc.state)
				}
			}
		})
	}
}
