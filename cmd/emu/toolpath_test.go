package main

import (
	"testing"

	"seedhammer.com/bezier"
	"seedhammer.com/engrave"
	"seedhammer.com/internal/sh2"
	"seedhammer.com/stepper"
)

// mm matches engrave/engrave_test.go:113 and stepper/stepper_test.go:115.
const mm = 6400

// runPlan drives the REAL stepper.Driver over a plan and returns what the
// recorder decoded from the words it emitted.
func runPlan(t *testing.T, waypoints []waypoint) *toolpathRecorder {
	t.Helper()
	return runPlanInto(t, newToolpathRecorder(), waypoints)
}

// runPlanInto is runPlan against a recorder that may already hold a recording,
// which is how the homing tests below put two jobs through one recorder -- the
// arrangement cmd/emu/platform.go deliberately has.
func runPlanInto(t *testing.T, rec *toolpathRecorder, waypoints []waypoint) *toolpathRecorder {
	t.Helper()
	conf := sh2.Params().StepperConfig
	plan := func(yield func(engrave.Command) bool) {
		for _, w := range waypoints {
			c := engrave.Move(w.p)
			if w.cut {
				c = engrave.Line(w.p)
			}
			if !yield(c) {
				return
			}
		}
	}
	// A FRESH driver per job, as runEngraving makes one per pass. Its position
	// starts at (0,0) -- which is the whole of F-121: on the machine the head
	// really is there, because homingEngraver drove it there.
	drv := stepper.NewDriver(rec)
	for k := range engrave.PlanEngraving(conf, plan) {
		if _, err := drv.Knot(k); err != nil {
			t.Fatalf("Knot: %v", err)
		}
	}
	if err := drv.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	return rec
}

type waypoint struct {
	cut bool
	p   bezier.Point
}

// visits reports every needle state the path has while passing through p.
//
// It returns a SET, not the first match, because a closed figure passes through
// its own start twice -- once travelling in, once closing the cut. The first
// version returned the first hit and reported "needle false, want true" for a
// corner the path genuinely cuts.
//
// Membership is ANALYTIC. The version before that stepped along each segment
// using the sign of its delta, which never terminates on a run that is neither
// axis-aligned nor exactly diagonal: (0,0)->(100,3) walks the diagonal forever,
// matching neither p nor the endpoint. It hung the suite until the timeout.
func visits(path []Vertex, p bezier.Point) (states map[bool]bool, ok bool) {
	// tol is in steps, and one step is 1/6400 mm. Corners land a few steps
	// past the ideal because a turn is only detected once the trailing window
	// has swung away from the chord -- measured at 9 to 17 steps, i.e. under
	// 0.003mm against a 0.3mm stroke. 128 steps (0.02mm) is loose enough to
	// admit that and far too tight to hide a misread pin, which would move the
	// head by the whole length of a run.
	const tol = 128
	states = map[bool]bool{}
	for i := 1; i < len(path); i++ {
		a, b := path[i-1], path[i]
		abx, aby := float64(b.X-a.X), float64(b.Y-a.Y)
		apx, apy := float64(p.X-a.X), float64(p.Y-a.Y)
		l2 := abx*abx + aby*aby
		t := 0.0
		if l2 > 0 {
			t = max(0, min(1, (apx*abx+apy*aby)/l2))
		}
		dx, dy := apx-t*abx, apy-t*aby
		if dx*dx+dy*dy <= tol*tol {
			states[b.Needle] = true
			ok = true
		}
	}
	return states, ok
}

// TestDecodeMatchesTheRealEncoder is the anti-drift guard for the pin layout
// mirrored in toolpath.go.
//
// Those constants are unexported in stepper/stepper.go, so cmd/emu keeps a
// copy, and a copied bit layout rots silently: nothing about a wrong pinNeedle
// offset would fail to compile, and the emulator would draw a confident,
// wrong plate. This drives the REAL stepper.Driver and checks the decoded path
// against the geometry that produced it.
//
// Mutations this pins: any swap or off-by-one among pinDirX/pinDirY/
// pinNeedle/pinStepX/pinStepY; reading a direction pin without its step pin;
// a wrong pinBits or stepsPerWord.
func TestDecodeMatchesTheRealEncoder(t *testing.T) {
	// A closed figure with both travel and cut moves, so every pin is
	// exercised in both states and in both directions on both axes.
	waypoints := []waypoint{
		{false, bezier.Pt(0, 0)},
		{false, bezier.Pt(10*mm, 5*mm)},  // travel out (+x, +y)
		{true, bezier.Pt(40*mm, 5*mm)},   // cut +x
		{true, bezier.Pt(40*mm, 25*mm)},  // cut +y
		{true, bezier.Pt(10*mm, 25*mm)},  // cut -x
		{true, bezier.Pt(10*mm, 5*mm)},   // cut -y, closing the box
		{false, bezier.Pt(30*mm, 15*mm)}, // travel back inside
	}

	rec := runPlan(t, waypoints)
	path := rec.Path()
	s := rec.Summarize(0)

	if s.Steps == 0 {
		t.Fatal("INCONCLUSIVE: the driver emitted no steps, so nothing was decoded")
	}
	if s.CutSteps == 0 {
		t.Fatal("INCONCLUSIVE: no needle-down steps decoded -- the plan had four cut moves, " +
			"so either pinNeedle is wrong or the plan did not engrave")
	}
	if s.CutSteps == s.Steps {
		t.Fatal("INCONCLUSIVE: EVERY step decoded as needle-down; the plan opens and closes " +
			"with travel moves, so pinNeedle is reading a pin that is always set")
	}
	if s.Truncated {
		t.Fatalf("INCONCLUSIVE: recording truncated at %d vertices", maxVertices)
	}

	// Each corner must be on the path, with the needle state its move implies.
	// The first waypoint is the origin the head already sits at.
	for _, w := range waypoints[1:] {
		states, ok := visits(path, w.p)
		if !ok {
			t.Errorf("the decoded path never reaches %v -- the head is not going where the "+
				"plan says, so a step or direction pin is misread", w.p)
			continue
		}
		if !states[w.cut] {
			t.Errorf("the path reaches %v but never with the needle %v -- pinNeedle is "+
				"reading the wrong bit", w.p, w.cut)
		}
	}

	// The bounding box is a whole-run cross-check that no waypoint assertion
	// can give: it fails if the path wanders anywhere the plan never goes.
	if got, want := s.Bounds, [4]int{0, 0, 40 * mm, 25 * mm}; got != want {
		t.Errorf("decoded bounds %v, want %v -- the path covers a different region than "+
			"the plan describes", got, want)
	}
	if s.EndX != 30*mm || s.EndY != 15*mm {
		t.Errorf("decoded end (%d,%d), want (%d,%d)", s.EndX, s.EndY, 30*mm, 15*mm)
	}
}

// TestDigestSeparatesDifferentMotion is what the abort->resume comparison
// rests on: two runs that move differently must not digest the same.
//
// Without this the reading "resumed run matches uninterrupted run" could pass
// on a digest that ignores position, which is precisely the failure it is
// supposed to catch.
func TestDigestSeparatesDifferentMotion(t *testing.T) {
	base := []waypoint{
		{false, bezier.Pt(0, 0)},
		{false, bezier.Pt(10*mm, 5*mm)},
		{true, bezier.Pt(40*mm, 5*mm)},
		{true, bezier.Pt(40*mm, 25*mm)},
	}
	same := rerun(t, base)
	if a, b := runPlan(t, base).Summarize(0).Digest, same; a != b {
		t.Errorf("the same plan digested differently across two runs: %s != %s -- the "+
			"digest is not a function of the motion alone, so it cannot compare runs", a, b)
	}

	// One corner moved. Same command count, same needle states, same step
	// count order of magnitude -- only the geometry differs.
	moved := append([]waypoint(nil), base...)
	moved[3] = waypoint{true, bezier.Pt(40*mm, 24*mm)}
	if a, b := runPlan(t, base).Summarize(0).Digest, runPlan(t, moved).Summarize(0).Digest; a == b {
		t.Errorf("moving a cut corner by 1mm did not change the digest (%s) -- it would not "+
			"detect a resumed run cutting the wrong path", a)
	}
}

func rerun(t *testing.T, w []waypoint) string {
	t.Helper()
	return runPlan(t, w).Summarize(0).Digest
}

// TestCutsThroughOriginIgnoresALegitimateResumeApproach pins the anomaly detector against
// the shape it exists for: a needle-down move back through (0,0) part way
// into a plate.
//
// That is what a zeroed SafePointer.history produces -- Resume feeds appendLine
// from a cleared safePoint, so the catch-up drives to the origin at T:0. On a
// plate it is a ruined plate; here it is a boolean.
func TestCutsThroughOriginIgnoresALegitimateResumeApproach(t *testing.T) {
	// The shape SafePointer.Resume actually produces: a needle-UP run back
	// through the origin, then out to the safe point. Measured on hardware --
	// every healthy resume does this, and the first version of the flag called
	// it a wrecked plate.
	resume := []waypoint{
		{false, bezier.Pt(0, 0)},
		{false, bezier.Pt(10*mm, 5*mm)},
		{true, bezier.Pt(40*mm, 5*mm)},
		{false, bezier.Pt(0, 0)}, // <- the synthesised approach
		{false, bezier.Pt(30*mm, 15*mm)},
		{true, bezier.Pt(40*mm, 25*mm)},
	}
	if s := runPlan(t, resume).Summarize(0); s.CutsThroughOrigin {
		t.Error("a needle-UP return through the origin was flagged -- that is what EVERY " +
			"legitimate resume does, so the flag would condemn every healthy plate")
	}

	clean := []waypoint{
		{false, bezier.Pt(0, 0)},
		{false, bezier.Pt(10*mm, 5*mm)},
		{true, bezier.Pt(40*mm, 5*mm)},
		{true, bezier.Pt(40*mm, 25*mm)},
		{false, bezier.Pt(30*mm, 15*mm)},
	}
	if s := runPlan(t, clean).Summarize(0); s.CutsThroughOrigin {
		t.Errorf("a plan that never returns to the origin was flagged as doing so -- "+
			"the detector would cry wolf on every plate (digest %s)", s.Digest)
	}

	wrecked := []waypoint{
		{false, bezier.Pt(0, 0)},
		{false, bezier.Pt(10*mm, 5*mm)},
		{true, bezier.Pt(40*mm, 5*mm)},
		{true, bezier.Pt(0, 0)}, // the catch-up going home mid-cut
		{true, bezier.Pt(40*mm, 25*mm)},
	}
	s := runPlan(t, wrecked).Summarize(0)
	if !s.CutsThroughOrigin {
		t.Error("a needle-down move through the origin mid-plate was NOT flagged -- this is " +
			"the exact signature of resume state zeroed while a restart was reachable")
	}
	if s.LongCuts == 0 {
		t.Error("the origin dive was not counted as a long cut either, so neither anomaly " +
			"would surface it")
	}
}

// TestHomeReturnsTheHeadToTheOriginBetweenJobs is F-121, as a test.
//
// cmd/controller wraps its engraver in a homingEngraver that homes on the first
// write of every job, so the machine's head is at the plate origin whenever a
// pass begins -- resumes included. stepper.Driver relies on it: Driver.pos is a
// zero bezier.Point and every step it emits is a delta from there, so the step
// stream MEANS "from the origin".
//
// The emulator had no such wrapper, and one recorder spans every job. So a
// second pass integrated an origin-relative stream from wherever the previous
// pass stopped, and recorded a plate the machine would never cut -- measured on
// the seed plate, an offset of (68.1mm, 34.4mm) on an 85mm plate.
//
// Mutations this pins: drop the Home call from emuEngraver.Write; make Home
// clear the position without recording the travel; home on the first job only.
func TestHomeReturnsTheHeadToTheOriginBetweenJobs(t *testing.T) {
	plan := []waypoint{
		{false, bezier.Pt(0, 0)},
		{false, bezier.Pt(10*mm, 5*mm)},
		{true, bezier.Pt(40*mm, 5*mm)},
		{true, bezier.Pt(40*mm, 25*mm)},
	}

	rec := newToolpathRecorder()
	runPlanInto(t, rec, plan)
	first := rec.Summarize(0)
	if first.EndX == 0 && first.EndY == 0 {
		t.Fatal("INCONCLUSIVE: the plan ended at the origin, so a missing home would be " +
			"invisible and this test could not fail")
	}

	rec.Home()
	if s := rec.Summarize(0); s.EndX != 0 || s.EndY != 0 {
		t.Fatalf("after Home the head is at (%d,%d), want the origin -- the next job's step "+
			"stream is a delta from (0,0) and would be recorded from here instead",
			s.EndX, s.EndY)
	}

	// The same plan again, through a fresh driver, as a second pass is.
	runPlanInto(t, rec, plan)
	second := rec.Summarize(0)
	if second.EndX != first.EndX || second.EndY != first.EndY {
		t.Errorf("the second pass over the same plan ended at (%d,%d), want (%d,%d) -- "+
			"it is being recorded offset by the previous pass, which is the plate the "+
			"machine does not cut", second.EndX, second.EndY, first.EndX, first.EndY)
	}
	if second.Bounds != first.Bounds {
		t.Errorf("the second pass covered %v, want %v -- an overlay would draw it off the "+
			"plan by the whole offset", second.Bounds, first.Bounds)
	}
}

// TestHomeTravelsWithTheNeedleUp pins the one way homing could manufacture the
// exact anomaly the recorder exists to find.
//
// CutsThroughOrigin is the signature of resume state zeroed while a restart was
// reachable (F-108): a needle-DOWN pass through (0,0) part way into a plate. A
// homing move that recorded the needle's last state instead of lifting it would
// stamp that signature onto every healthy interrupted plate -- and the flag
// would then be worthless in the one place it is read.
func TestHomeTravelsWithTheNeedleUp(t *testing.T) {
	// Ends mid-cut, needle DOWN and far from the origin.
	rec := runPlan(t, []waypoint{
		{false, bezier.Pt(0, 0)},
		{false, bezier.Pt(10*mm, 5*mm)},
		{true, bezier.Pt(40*mm, 25*mm)},
	})
	if !rec.Path()[len(rec.Path())-1].Needle {
		t.Fatal("INCONCLUSIVE: the plan did not end with the needle down, so lifting it " +
			"cannot be what this test observes")
	}

	rec.Home()

	last := rec.Path()[len(rec.Path())-1]
	if last.X != 0 || last.Y != 0 {
		t.Fatalf("the homing move ended at (%d,%d), want the origin", last.X, last.Y)
	}
	if last.Needle {
		t.Error("the homing move was recorded as a CUT to the origin -- that is the F-108 " +
			"wrecked-plate signature, and homing would forge it on every healthy plate")
	}
	if s := rec.Summarize(0); s.CutsThroughOrigin {
		t.Error("homing set CutsThroughOrigin, so the anomaly that detects a zeroed " +
			"SafePointer.history now fires on every interrupted plate")
	}
}

// TestHomeAtTheOriginRecordsNothing keeps the common case free of debris: the
// device homes at the start of every job whether or not it needs to, but a
// zero-length travel is not motion, and a vertex per job would accumulate in a
// recording that deliberately spans all of them.
func TestHomeAtTheOriginRecordsNothing(t *testing.T) {
	rec := newToolpathRecorder()
	before := len(rec.Path())
	rec.Home()
	rec.Home()
	if got := len(rec.Path()); got != before {
		t.Errorf("two homes at the origin added %d vertices, want 0", got-before)
	}

	// And after a plan that happens to end where it started.
	rec = runPlan(t, []waypoint{
		{false, bezier.Pt(0, 0)},
		{true, bezier.Pt(20*mm, 0)},
		{true, bezier.Pt(0, 0)},
	})
	s := rec.Summarize(0)
	if s.EndX != 0 || s.EndY != 0 {
		t.Fatalf("INCONCLUSIVE: the closing plan ended at (%d,%d), not the origin", s.EndX, s.EndY)
	}
	n := len(rec.Path())
	rec.Home()
	if got := len(rec.Path()); got != n {
		t.Errorf("homing from the origin added %d vertices, want 0", got-n)
	}
}

// TestJobRecorderHomesOncePerJob pins the wiring, not the motion.
//
// It exists because the wiring used to live in engraver.go, which is
// //go:build js and therefore cannot be tested at all. "Homes on the first
// write and on Close, and not in between" is a three-state machine, and the
// two ways to get it wrong -- homing on every write, homing on none -- are
// both invisible until a plate is walked in a browser.
//
// Mutations this pins: home on every Write; never home on Write; drop the home
// from Close; share homed across jobs.
func TestJobRecorderHomesOncePerJob(t *testing.T) {
	plan := []waypoint{
		{false, bezier.Pt(0, 0)},
		{false, bezier.Pt(10*mm, 5*mm)},
		{true, bezier.Pt(40*mm, 25*mm)},
	}
	atOrigin := func(t *testing.T, rec *toolpathRecorder, what string) {
		t.Helper()
		if s := rec.Summarize(0); s.EndX != 0 || s.EndY != 0 {
			t.Fatalf("%s left the head at (%d,%d), want the origin", what, s.EndX, s.EndY)
		}
	}

	rec := runPlan(t, plan)
	if s := rec.Summarize(0); s.EndX == 0 && s.EndY == 0 {
		t.Fatal("INCONCLUSIVE: the plan ended at the origin, so homing cannot be observed")
	}

	// An EMPTY write, deliberately: homingEngraver homes before it forwards
	// anything, so what triggers it is the write happening, not its content.
	j := &jobRecorder{rec: rec}
	if _, err := j.Write(nil); err != nil {
		t.Fatalf("Write: %v", err)
	}
	atOrigin(t, rec, "the first Write of a job")

	// Now drive the head away again and write a second time. That must NOT
	// home: the machine homes once per job, and a home mid-plate would drag
	// the recording back to the origin between two halves of one cut.
	runPlanInto(t, rec, plan)
	before := rec.Summarize(0)
	if _, err := j.Write(nil); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if after := rec.Summarize(0); after.EndX != before.EndX || after.EndY != before.EndY {
		t.Errorf("a later Write moved the head to (%d,%d) from (%d,%d) -- it homed again, "+
			"mid-job", after.EndX, after.EndY, before.EndX, before.EndY)
	}

	j.Close()
	atOrigin(t, rec, "Close")

	// And a NEW job homes again, because Platform.Engraver hands out a new
	// engraver per pass.
	runPlanInto(t, rec, plan)
	j2 := &jobRecorder{rec: rec}
	if _, err := j2.Write(nil); err != nil {
		t.Fatalf("Write: %v", err)
	}
	atOrigin(t, rec, "the first Write of the NEXT job")
}
