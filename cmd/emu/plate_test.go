package main

import (
	"bytes"
	"strconv"
	"strings"
	"testing"

	"seedhammer.com/bezier"
	"seedhammer.com/bspline"
	"seedhammer.com/engrave"
	"seedhammer.com/internal/golden"
	"seedhammer.com/internal/sh2"
)

// planSpline is a plan with both cut and travel moves, as a spline.
func planSpline(waypoints []waypoint) bspline.Curve {
	conf := sh2.Params().StepperConfig
	return engrave.PlanEngraving(conf, func(yield func(engrave.Command) bool) {
		for _, w := range waypoints {
			c := engrave.Move(w.p)
			if w.cut {
				c = engrave.Line(w.p)
			}
			if !yield(c) {
				return
			}
		}
	})
}

var overlayPlan = []waypoint{
	{false, bezier.Pt(0, 0)},
	{false, bezier.Pt(10*mm, 5*mm)},
	{true, bezier.Pt(40*mm, 5*mm)},
	{true, bezier.Pt(40*mm, 25*mm)},
	{true, bezier.Pt(10*mm, 25*mm)},
	{true, bezier.Pt(10*mm, 5*mm)},
}

// TestPlanPathMatchesVectorize is the anti-drift guard for the one thing
// planPath duplicates.
//
// cmd/plateview renders a plate through internal/golden.Vectorize, and that is
// the rendering the whole repo trusts -- "what you see is the cut". planPath
// emits the same geometry, because the overlay needs the path DATA to put
// inside a live SVG rather than a finished SVG document, and cmd/plateview's
// way of getting at it is to index into Vectorize's output and splice
// (cmd/plateview/main.go's `anchor`), which is a fragile assumption this
// declines to make a second time in production code.
//
// So the duplication is admitted and pinned here instead: byte-for-byte, on a
// plan with cuts and travels in both directions. If either side changes, this
// fails rather than the overlay quietly drawing a different plate from the one
// cmd/plateview shows.
func TestPlanPathMatchesVectorize(t *testing.T) {
	params := sh2.Params()

	var buf bytes.Buffer
	if err := golden.Vectorize(&buf, params.StrokeWidth,
		bspline.Bounds{Max: bezier.Pt(85*mm, 85*mm)}, planSpline(overlayPlan)); err != nil {
		t.Fatalf("Vectorize: %v", err)
	}
	want := spliceD(t, buf.String())
	got, _ := planPath(planSpline(overlayPlan))

	if want == "" {
		t.Fatal("INCONCLUSIVE: Vectorize emitted an empty path, so this compares nothing")
	}
	if got != want {
		t.Errorf("planPath and Vectorize disagree.\n got %d bytes: %.120s\nwant %d bytes: %.120s",
			len(got), got, len(want), want)
	}
}

// spliceD pulls the d attribute out of Vectorize's SVG. Only the test does
// this; see TestPlanPathMatchesVectorize for why it is not done in the overlay.
func spliceD(t *testing.T, svg string) string {
	t.Helper()
	const anchor = `<path class="spline" d="`
	i := strings.Index(svg, anchor)
	if i < 0 {
		t.Fatal("INCONCLUSIVE: Vectorize's output no longer contains the spline path this " +
			"test splices, so it is comparing against nothing")
	}
	rest := svg[i+len(anchor):]
	j := strings.Index(rest, `"`)
	if j < 0 {
		t.Fatal("INCONCLUSIVE: unterminated d attribute in Vectorize's output")
	}
	return rest[:j]
}

// TestPlanAndRecordingShareOneCoordinateSpace is the property the whole overlay
// rests on, and the only one that cannot be seen by looking at either half.
//
// The plan is drawn from bspline control points in microsteps. The progress is
// drawn from toolpathRecorder, which integrates the driver's step deltas from
// the origin. Those are the same space ONLY because stepper.Driver's position
// starts at the zero bezier.Point and every step is a delta from it -- which is
// also why F-121's missing homing broke registration rather than merely losing
// detail.
//
// If this ever fails, the overlay is drawing a truthful plan and a truthful cut
// in two different frames, which reads as a machine cutting the wrong plate.
func TestPlanAndRecordingShareOneCoordinateSpace(t *testing.T) {
	plan, _ := planPath(planSpline(overlayPlan))
	cuts := cutEndpoints(t, plan)
	if len(cuts) < 4 {
		t.Fatalf("INCONCLUSIVE: the plan yielded %d cut endpoints, too few to locate anything",
			len(cuts))
	}

	recorded := runPlan(t, overlayPlan).Path()
	for _, p := range cuts {
		states, ok := visits(recorded, p)
		if !ok {
			t.Errorf("the plan cuts through %v but the recorded path never goes there -- the "+
				"overlay would draw progress somewhere the plan is not", p)
			continue
		}
		if !states[true] {
			t.Errorf("the plan cuts through %v but the recording only travels there", p)
		}
	}
}

// cutEndpoints reads the on-curve end of every cubic in an SVG path, which is
// where the plan says the needle is down.
func cutEndpoints(t *testing.T, d string) []bezier.Point {
	t.Helper()
	f := strings.Fields(strings.ReplaceAll(d, ",", " "))
	num := func(i int) int {
		t.Helper()
		v, err := strconv.Atoi(f[i])
		if err != nil {
			t.Fatalf("token %d of the plan path is %q, not a number", i, f[i])
		}
		return v
	}
	var out []bezier.Point
	for i := 0; i < len(f); {
		switch f[i] {
		case "C":
			if i+6 >= len(f) {
				t.Fatalf("truncated cubic at token %d", i)
			}
			out = append(out, bezier.Pt(num(i+5), num(i+6)))
			i += 7
		case "M":
			if i+2 >= len(f) {
				t.Fatalf("truncated move at token %d", i)
			}
			i += 3
		default:
			t.Fatalf("unexpected token %q at %d in the plan path", f[i], i)
		}
	}
	return out
}

// TestPlatePlanHoldsThePlanAcrossAResume pins the resume rule.
//
// A hold-to-resume is a new engraving job over the SAME spline, so it arrives
// through the same hook as a fresh plate. Re-rendering would be harmless; what
// is not harmless is bumping the sequence, because the page treats a new
// sequence as a new plate and clears the progress it has drawn. The operator
// would see a plate they are half way through restart itself.
func TestPlatePlanHoldsThePlanAcrossAResume(t *testing.T) {
	params := sh2.Params()
	var p platePlan

	if fresh := p.Set(planSpline(overlayPlan), params, false); !fresh {
		t.Fatal("the first plate did not report itself as new, so the page would never draw it")
	}
	svg, seq := p.Snapshot()
	if seq == 0 {
		t.Error("a plate was planned but the sequence stayed at its zero value, which the " +
			"page cannot distinguish from no plate at all")
	}
	if !strings.Contains(svg, "<svg") || !strings.Contains(svg, cutPathID) {
		t.Fatalf("the rendered plan is not an SVG carrying the live-cut element: %.120s", svg)
	}

	if fresh := p.Set(planSpline(overlayPlan), params, true); fresh {
		t.Error("a RESUMED job reported a new plate -- the page would clear the progress " +
			"drawn so far and redraw the plate from empty, mid-cut")
	}
	if svg2, seq2 := p.Snapshot(); seq2 != seq || svg2 != svg {
		t.Errorf("a resume changed the plan (seq %d -> %d), so the page would redraw", seq, seq2)
	}

	// A genuinely different plate does bump it.
	if fresh := p.Set(planSpline(overlayPlan[:3]), params, false); !fresh {
		t.Error("a new plate did not report itself as new")
	}
	if _, seq3 := p.Snapshot(); seq3 == seq {
		t.Errorf("a new plate kept sequence %d, so the page would keep drawing the old plan", seq)
	}
}

// TestPlanSVGCoversTheWholePlate keeps the overlay's viewBox tied to the plate
// the firmware plans against rather than to the ink on it.
//
// A viewBox fitted to the engraving would rescale between plates -- a short
// passphrase plate and a full seed plate would draw their strokes at different
// apparent widths, and the operator would have no way to see that one plate
// uses more of the steel than another.
func TestPlanSVGCoversTheWholePlate(t *testing.T) {
	params := sh2.Params()
	svg := planSVG(planSpline(overlayPlan), params)

	want := "viewBox=\"0 0 " + strconv.Itoa(85*mm) + " " + strconv.Itoa(85*mm) + "\""
	if !strings.Contains(svg, want) {
		t.Errorf("the plan SVG does not carry %s -- its frame is not the plate", want)
	}
	for _, id := range []string{cutPathID, headMarkerID} {
		if !strings.Contains(svg, `id="`+id+`"`) {
			t.Errorf("the plan SVG has no %q element, so the page has nothing to draw "+
				"progress into", id)
		}
	}
}

// TestBeginPlateResetsOnlyForANewPlate pins the lifetime rule that ties the two
// halves of the overlay together.
//
// cmd/emu/platform.go keeps ONE recorder across every job on purpose, "so a
// plate that is aborted and resumed records as ONE motion -- which is the thing
// being compared". A per-plate overlay needs the opposite for a genuinely new
// plate: without a reset it would draw the previous plate's cut as progress on
// this one's plan. Resetting at the right moment gets both, which is why the
// recorder's lifetime is left alone and the RECORDING is what restarts.
func TestBeginPlateResetsOnlyForANewPlate(t *testing.T) {
	params := sh2.Params()
	var p platePlan

	rec := runPlan(t, overlayPlan)
	cut := rec.Summarize(0)
	if cut.CutSteps == 0 {
		t.Fatal("INCONCLUSIVE: nothing was cut, so a reset would be undetectable")
	}

	// A resume must keep every step of it.
	beginPlate(&p, rec, planSpline(overlayPlan), params, true)
	if s := rec.Summarize(0); s.CutSteps != cut.CutSteps {
		t.Errorf("a resume cleared the recording (%d cut steps -> %d) -- the operator would "+
			"watch the plate they are half way through start over", cut.CutSteps, s.CutSteps)
	}

	// A new plate must start from nothing.
	beginPlate(&p, rec, planSpline(overlayPlan), params, false)
	s := rec.Summarize(0)
	if s.CutSteps != 0 || s.Steps != 0 {
		t.Errorf("a new plate kept %d steps (%d cut) of the previous one, which the overlay "+
			"would draw as progress already made on a plate not yet touched",
			s.Steps, s.CutSteps)
	}
	if s.EndX != 0 || s.EndY != 0 {
		t.Errorf("a new plate began with the head at (%d,%d), not the origin", s.EndX, s.EndY)
	}
}

// TestPlanPathIsBounded stops the overlay from hanging the tab on a plan that
// never ends.
//
// gui/qa.go's qaPlan is exactly that: `for { ... }` around a rect, a diagonal
// and a circle, yielding until the consumer stops. Nothing about a plate makes
// a plan finite -- the engrave loop simply stops asking -- but rendering the
// LAYOUT means ranging the whole spline before the first step, so an unbounded
// plan is an unbounded render.
//
// qaProgram is unreachable in cmd/emu today, because it is triggered by an NFC
// debug command and Platform.NFCReader returns nil. That is a property of the
// emulator's wiring, not of this code, and it is one feature away from being
// false -- which is the shape of hazard this repo has been bitten by before.
func TestPlanPathIsBounded(t *testing.T) {
	// The generator stops well past the cap rather than never, so a planPath
	// that failed to bound itself fails this test instead of hanging the suite.
	const limit = maxPlanKnots * 2
	yielded := 0
	endless := func(yield func(bspline.Knot) bool) {
		for yielded = 0; yielded < limit; yielded++ {
			k := bspline.Knot{
				Ctrl:    bezier.Pt((yielded%64)*mm/8, (yielded%37)*mm/8),
				T:       1,
				Engrave: true,
			}
			if !yield(k) {
				return
			}
		}
	}

	d, truncated := planPath(endless)
	if yielded >= limit {
		t.Fatalf("planPath consumed all %d knots on offer -- an unbounded plan renders "+
			"unboundedly, and the tab stops responding before the first step is cut", limit)
	}
	if yielded > maxPlanKnots+1 {
		t.Errorf("planPath consumed %d knots, want no more than %d", yielded, maxPlanKnots+1)
	}
	if !truncated {
		t.Error("a plan that hit the cap was not reported as truncated -- the page would " +
			"present a partial layout as the whole plate")
	}
	if d == "" {
		t.Error("nothing was rendered before the cap, so the bound is not a bound but a refusal")
	}

	// And a real plate is nowhere near it: the widest measured plate is
	// constproof at 27,062 knots.
	if _, truncated := planPath(planSpline(overlayPlan)); truncated {
		t.Error("an ordinary plan was truncated, so the cap is too tight to draw real plates")
	}
}
