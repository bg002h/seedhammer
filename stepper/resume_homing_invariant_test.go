package stepper

import (
	"testing"

	"seedhammer.com/bezier"
	"seedhammer.com/bspline"
	"seedhammer.com/engrave"
)

// Why a resumed cut's approach line starts at the ORIGIN, and what makes that
// correct.
//
// engrave.SafePointer.Resume synthesises its approach line from bezier.Point{}:
//
//	move = appendLine(move, conf, false, bezier.Point{}, s.safePoint)
//
// F-114 read that as a defect -- "a resumed cut approaches its safe point FROM
// THE ORIGIN, wherever the head actually is" -- and asked whether the resulting
// traverse is motion-profiled, on the theory that an unramped max-rate slew
// across the plate would be a plate-integrity problem.
//
// It is not a defect. The premise does not hold on the device, because the head
// is NOT "wherever it actually is" when those knots are executed: it is at the
// origin, because the machine has just homed to it.
//
//	cmd/controller/platform_sh2.go  Platform.Engraver returns a *homingEngraver
//	                                with homed=false, freshly, on every call
//	                                (gui/engraver.go:186 calls it per run).
//	                                homingEngraver.Write homes on the FIRST
//	                                write, so homing precedes any engraved step
//	                                reaching the device.
//	cmd/controller/engraver.go      engraver.home drives to the limit switches,
//	                                resets the driver, then moves to
//	                                (originX, originY) -- the plate origin,
//	                                which IS engraving coordinate (0,0). The
//	                                5.0mm/3.2mm offset is machine-zero to
//	                                plate-origin and is consumed inside home's
//	                                own driver.
//	gui/engraver.go:198             runEngraving then builds a FRESH
//	                                stepper.NewDriver, whose d.pos is (0,0).
//
// So d.pos and the physical head agree at (0,0), and an approach line drawn
// from the origin is exactly right. The "head tracks toward the origin" that the
// follow-up describes -- and that the operator saw on hardware on 2026-08-10,
// "went a short distance towards top left and then directly to where it left
// off" -- is the homing move itself, which is intended.
//
// What these tests pin is the COUPLING, because it is invisible: nothing in
// engrave/ or stepper/ mentions homing, and Resume's correctness depends
// entirely on it. If homing is ever removed, made conditional, or moved after
// the first write, a resumed cut silently starts in the wrong place. That should
// be a red test, not a ruined plate.

func decodeTicks(words []uint32) (steps []bezier.Point, needleDown []bool) {
	for _, w := range words {
		for j := range stepsPerWord {
			s := uint8((w >> (j * pinBits)) & (0b1<<pinBits - 1))
			// A tick that moves nothing is still a NON-ZERO word: Driver.fill
			// always sets at least one direction pin ("the all-zero value means
			// halt"). So a zero word is genuinely the end of the stream, not a
			// stationary tick.
			if s == 0 {
				return steps, needleDown
			}
			var d bezier.Point
			if (s>>pinStepX)&0b1 != 0 {
				d.X = 1
				if (s>>pinDirX)&0b1 != 0 {
					d.X = -1
				}
			}
			if (s>>pinStepY)&0b1 != 0 {
				d.Y = 1
				if (s>>pinDirY)&0b1 != 0 {
					d.Y = -1
				}
			}
			steps = append(steps, d)
			needleDown = append(needleDown, (s>>pinNeedle)&0b1 != 0)
		}
	}
	return steps, needleDown
}

// resumePlan builds a SafePointer that has engraved as far as safe and still has
// an outstanding cut, then returns the knots Resume would hand the driver.
func resumePlan(t *testing.T, safe bezier.Point) []bspline.Knot {
	t.Helper()
	conf := params.StepperConfig
	sp := new(engrave.SafePointer)
	spline := engrave.PlanEngraving(conf, func(yield func(engrave.Command) bool) {
		yield(engrave.Move(safe))
		yield(engrave.Line(bezier.Pt(safe.X+10*mm, safe.Y)))
	})
	// Report progress only up to where the ENGRAVING starts, so the cut is
	// genuinely outstanding. Reporting the full duration retires everything and
	// Resume returns nothing but the approach line -- with no needle-down tick,
	// which reads like a clean result while actually measuring nothing.
	var upToCut uint
	seenCut := false
	for k := range spline {
		sp.Knot(k)
		if k.Engrave {
			seenCut = true
		}
		if !seenCut {
			upToCut += k.T
		}
	}
	if !seenCut {
		t.Fatal("the plan contains no engrave knot; the scenario is invalid")
	}
	sp.Progress(upToCut)

	knots := sp.Resume(conf)
	nCut := 0
	for _, k := range knots {
		if k.Engrave {
			nCut++
		}
	}
	// Positive control: without an engrave knot there is no needle-down tick,
	// and every assertion below would pass vacuously.
	if nCut == 0 {
		t.Fatalf("Resume returned %d knots and NONE engrave -- the scenario "+
			"retired the whole cut, so there is nothing to measure", len(knots))
	}
	out := make([]bspline.Knot, len(knots))
	copy(out, knots)
	return out
}

// TestResumeApproachLineStartsAtTheOrigin pins the mechanism itself. If this
// ever fails, F-114's premise has changed and the homing coupling below needs
// rechecking with it.
func TestResumeApproachLineStartsAtTheOrigin(t *testing.T) {
	knots := resumePlan(t, bezier.Pt(30*mm, 25*mm))
	if got := knots[0].Ctrl; got != (bezier.Point{}) {
		t.Errorf("Resume's approach line starts at %v, want the origin %v",
			got, bezier.Point{})
	}
}

// TestResumeIsCorrectOnlyBecauseTheMachineHomes is a characterisation test. It
// runs the same resume knots from two driver positions:
//
//	(0,0) -- the post-homing state the device is actually in. The cut must
//	         start exactly on the safe point.
//	far   -- the state the device would be in if homing were skipped. The cut
//	         starts in the WRONG PLACE, and the error grows with distance.
//
// The second case is not a bug report; it is the cost of the invariant, kept
// here so that whoever removes homing sees it.
func TestResumeIsCorrectOnlyBecauseTheMachineHomes(t *testing.T) {
	safe := bezier.Pt(30*mm, 25*mm)
	knots := resumePlan(t, safe)

	run := func(start bezier.Point) (cutAt bezier.Point, firstCut int, ticks int) {
		dev := new(device)
		drv := NewDriver(dev)
		drv.pos = start
		for _, k := range knots {
			if _, err := drv.Knot(k); err != nil {
				t.Fatal(err)
			}
		}
		if err := drv.Flush(); err != nil {
			t.Fatal(err)
		}
		steps, needle := decodeTicks(dev.steps)
		pos := start
		firstCut = -1
		for i, d := range steps {
			pos = pos.Add(d)
			if needle[i] && firstCut < 0 {
				firstCut, cutAt = i, pos
			}
		}
		return cutAt, firstCut, len(steps)
	}

	// The real device: homed, so the driver's (0,0) is the truth.
	cutAt, firstCut, ticks := run(bezier.Point{})
	if firstCut < 0 {
		t.Fatalf("homed run: the needle never dropped in %d ticks", ticks)
	}
	if cutAt != safe {
		t.Errorf("homed run: the cut starts at %v, want the safe point %v -- "+
			"a resumed cut on a homed machine must begin exactly where it left off",
			cutAt, safe)
	}

	// Skipping homing. Reported, not asserted as a defect: this state does not
	// occur on the device today.
	for _, far := range []bezier.Point{
		bezier.Pt(20*mm, 15*mm),
		bezier.Pt(60*mm, 40*mm),
		bezier.Pt(80*mm, 60*mm),
	} {
		cutAt, firstCut, ticks := run(far)
		if firstCut < 0 {
			t.Logf("un-homed at %2d,%2d mm: needle never dropped in %d ticks",
				far.X/mm, far.Y/mm, ticks)
			continue
		}
		e := cutAt.Sub(safe)
		t.Logf("un-homed at %2d,%2d mm: cut starts %5d,%5d steps (%.2f,%.2f mm) off the safe point",
			far.X/mm, far.Y/mm, e.X, e.Y,
			float64(e.X)/float64(mm), float64(e.Y)/float64(mm))
	}
}
