package engrave

import (
	"testing"

	"seedhammer.com/bezier"
	"seedhammer.com/bspline"
)

// squarePlan is a small closed figure -- enough commands to drive planEngraving
// through several clamped segments and leave real knots in the buffer, without
// depending on any font or plate fixture.
func squarePlan(yield func(Command) bool) {
	if !yield(Move(bezier.Pt(0, 0))) {
		return
	}
	for _, p := range []bezier.Point{
		bezier.Pt(40*mm, 0),
		bezier.Pt(40*mm, 40*mm),
		bezier.Pt(0, 40*mm),
		bezier.Pt(0, 0),
	} {
		if !yield(Line(p)) {
			return
		}
	}
}

func nonZeroKnots(ks []bspline.Knot) int {
	n := 0
	for _, k := range ks {
		if k != (bspline.Knot{}) {
			n++
		}
	}
	return n
}

// TestPlanEngravingZeroesTheKnotBuffer is F-108 item (1).
//
// The knots ARE the seed rendered as geometry. planEngraving's caller supplies
// the buffer, so this test can hold it and read the bytes back -- which is the
// only way to witness the property: a zeroed buffer and a buffer that was never
// written look identical from outside, so the test first asserts the range
// actually WROTE knots, and fails as inconclusive otherwise.
//
// Mutation this pins: delete the `defer` from planEngraving's closure.
func TestPlanEngravingZeroesTheKnotBuffer(t *testing.T) {
	for _, tc := range []struct {
		name  string
		drain func(bspline.Curve)
	}{
		{"ranged to completion", func(c bspline.Curve) {
			for range c {
			}
		}},
		{"broken out of early (!yield)", func(c bspline.Curve) {
			n := 0
			for range c {
				n++
				if n == 3 {
					break
				}
			}
		}},
		{"bspline.Measure only -- never cut", func(c bspline.Curve) {
			bspline.Measure(c)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			buf := make([]bspline.Knot, 0, 100)
			// A canary in the whole array: "zeroed" must be distinguishable
			// from "never written".
			full := buf[:cap(buf)]
			for i := range full {
				full[i] = bspline.Knot{Ctrl: bezier.Point{X: 7, Y: 7}, T: 7}
			}

			tc.drain(planEngraving(buf, conf, squarePlan))

			if got := nonZeroKnots(full); got != 0 {
				t.Errorf("%d of %d knots survive in the caller's buffer after the iterator finished; "+
					"planEngraving did not zero the geometry it built", got, len(full))
			}
		})
	}
}

// TestPlanEngravingRematerialisesAfterZeroing pins that the zeroing is SAFE,
// not merely done. runEngraving re-ranges e.spline on the operator's
// hold-to-resume, so a buffer zeroed by the previous range must be rebuilt --
// not read back empty. This is the property that makes item (1) legal at all.
func TestPlanEngravingRematerialisesAfterZeroing(t *testing.T) {
	buf := make([]bspline.Knot, 0, 100)
	c := planEngraving(buf, conf, squarePlan)

	var first, second []bspline.Knot
	for k := range c {
		first = append(first, k)
	}
	for k := range c {
		second = append(second, k)
	}

	if len(first) == 0 {
		t.Fatal("INCONCLUSIVE: the plan yielded no knots")
	}
	if len(first) != len(second) {
		t.Fatalf("re-range yielded %d knots, first range yielded %d -- zeroing broke the restart path",
			len(second), len(first))
	}
	for i := range first {
		if first[i] != second[i] {
			t.Fatalf("re-range diverged at knot %d: %+v != %+v", i, second[i], first[i])
		}
	}
	t.Logf("re-range reproduced all %d knots after the buffer was zeroed", len(first))
}

// TestSafePointerTrimZeroesTheTail is F-108 item (3).
//
// Resume's trim compacts live knots down with copy() and reslices, so
// [rem:cap] holds knots that are dead but present. The design calls this piece
// "free, and always safe"; nothing pinned it until now.
//
// Mutation this pins: delete the clear at the trim in SafePointer.Resume.
func TestSafePointerTrimZeroesTheTail(t *testing.T) {
	var sp SafePointer
	// Feed a clamped run so the trim fires: three identical control points in a
	// row is what marks a safe point.
	p := bezier.Point{X: 11 * mm, Y: 13 * mm}
	for i := 0; i < 8; i++ {
		sp.Knot(bspline.Knot{Ctrl: p, T: 5})
	}
	sp.Progress(40)
	sp.Resume(conf)

	tail := sp.history[len(sp.history):cap(sp.history)]
	if len(tail) == 0 {
		t.Skip("INCONCLUSIVE: no tail past len -- the trim did not shorten the history")
	}
	if got := nonZeroKnots(tail); got != 0 {
		t.Errorf("%d of %d knots survive in history[len:cap] after the trim; "+
			"reslicing is not clearing", got, len(tail))
	}
}

// TestClearHistoryZeroesEverySeedDerivedField is F-108 item (2)'s engrave half.
//
// ClearHistory's first version zeroed history and completed but left safePoint
// -- the last known-good COORDINATE, which Resume feeds straight to appendLine.
// One bezier.Point is a small residue next to the knots, which is exactly why
// it went unnoticed through two review rounds.
//
// Mutation this pins: drop `s.safePoint = bezier.Point{}` from ClearHistory.
func TestClearHistoryZeroesEverySeedDerivedField(t *testing.T) {
	var sp SafePointer
	p := bezier.Point{X: 1234, Y: 5678}
	for i := 0; i < 8; i++ {
		sp.Knot(bspline.Knot{Ctrl: p, T: 5})
	}
	sp.Progress(40)
	sp.Resume(conf) // establishes safePoint

	if sp.safePoint == (bezier.Point{}) {
		t.Fatal("INCONCLUSIVE: safePoint was never set, so clearing it cannot be witnessed")
	}

	sp.ClearHistory()

	if sp.safePoint != (bezier.Point{}) {
		t.Errorf("safePoint survives ClearHistory: %+v -- it is the last plate coordinate, "+
			"seed-derived geometry that Resume feeds to appendLine", sp.safePoint)
	}
	if sp.progress != 0 {
		t.Errorf("progress survives ClearHistory: %d", sp.progress)
	}
	if sp.completed != 0 {
		t.Errorf("completed survives ClearHistory: %d", sp.completed)
	}
	if got := nonZeroKnots(sp.history[:cap(sp.history)]); got != 0 {
		t.Errorf("%d knots survive in history[:cap] after ClearHistory", got)
	}
}
