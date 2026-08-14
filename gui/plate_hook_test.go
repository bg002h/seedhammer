package gui

import (
	"sync"
	"testing"
	"time"

	"seedhammer.com/bspline"
	"seedhammer.com/engrave"
)

// plateAwarePlatform hands out an engraver that accepts the plan.
//
// It overrides testPlatform rather than widening it: testPlatform.engraver is
// concrete, and every other gui test reaches into that field. Making it an
// interface to serve one test would put a PlateAware method on the engraver
// the whole package shares.
type plateAwarePlatform struct {
	*testPlatform
	e Engraver
}

func (p *plateAwarePlatform) Engraver(stall bool) (Engraver, error) { return p.e, nil }

// plateAwareEngraver is a testEngraver that also accepts the plan.
type plateAwareEngraver struct {
	*testEngraver

	mu    sync.Mutex
	calls []plateCall
}

type plateCall struct {
	knots   int
	resumed bool
}

// Plate counts the knots it is handed, which is how the test checks that the
// spline arrives WHOLE. runEngraving is about to range the same value, so a
// hook that consumed it would leave the job with nothing to cut -- and that
// failure would look like an empty plate, not like a broken hook.
func (e *plateAwareEngraver) Plate(spline bspline.Curve, conf engrave.StepperConfig, resumed bool) {
	n := 0
	for range spline {
		n++
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.calls = append(e.calls, plateCall{knots: n, resumed: resumed})
}

func (e *plateAwareEngraver) seen() []plateCall {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]plateCall(nil), e.calls...)
}

// waitIdle drives Status() until the job leaves engraveRunning, as the engrave
// screen's own loop does.
func waitIdle(t *testing.T, job *engraveJob) engraveState {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		st := job.Status()
		if st.State != engraveRunning {
			return st.State
		}
		if time.Now().After(deadline) {
			t.Fatal("the engrave job never left engraveRunning")
		}
		time.Sleep(time.Millisecond)
	}
}

// TestPlateHookFiresOncePerJobWithTheWholeSpline pins the seam cmd/emu's plate
// overlay hangs on: an Engraver that implements PlateAware is handed the plan
// BEFORE the first Write, once per job, and told whether this job is a resume.
//
// The resumed flag is what lets the emulator keep ONE recording across an
// abort and a hold-to-resume -- the property cmd/emu/platform.go's single
// recorder exists for. Get it backwards and every resume restarts the overlay,
// which is indistinguishable from the plate itself restarting.
func TestPlateHookFiresOncePerJobWithTheWholeSpline(t *testing.T) {
	const knots = 3

	e := &plateAwareEngraver{testEngraver: newEngraver()}
	p := &plateAwarePlatform{testPlatform: newPlatform(), e: e}

	spline := func(yield func(bspline.Knot) bool) {
		for range knots {
			if !yield(bspline.Knot{}) {
				return
			}
		}
	}
	job := newEngraverJob(p, spline, p.EngraverParams().StepperConfig, suppressStalls)

	job.Start()
	if st := waitIdle(t, job); st != engraveDone {
		t.Fatalf("first run ended in state %v, want engraveDone", st)
	}

	got := e.seen()
	if len(got) != 1 {
		t.Fatalf("PlateAware.Plate called %d times for one job, want 1", len(got))
	}
	if got[0].resumed {
		t.Error("the FIRST run of a job reported resumed=true -- a fresh plate would be " +
			"drawn as a continuation of whatever the recorder still held")
	}
	if got[0].knots != knots {
		t.Errorf("the hook saw %d knots, want %d -- the spline handed to the hook is not "+
			"the one the job cuts", got[0].knots, knots)
	}

	// A resume is a SECOND runEngraving over the same job, with nknots already
	// past zero -- the path EngraveScreen takes on hold-to-resume.
	job.Start()
	if st := waitIdle(t, job); st != engraveDone {
		t.Fatalf("resumed run ended in state %v, want engraveDone", st)
	}

	got = e.seen()
	if len(got) != 2 {
		t.Fatalf("PlateAware.Plate called %d times across two runs, want 2", len(got))
	}
	if !got[1].resumed {
		t.Error("the resumed run reported resumed=false -- the overlay would reset its " +
			"recording mid-plate, which is the one thing the single recorder exists to avoid")
	}
}

// The structural half -- that PlateAware is absent from the firmware image --
// now lives in tinygo_split_test.go, which checks the property for EVERY
// //go:build pair in this package rather than for this one by name. It moved
// when frame_hook.go became the second such hook: a guard keyed to one file and
// one identifier is a hand-maintained list, and the version there discovers its
// subjects from the tree instead.
