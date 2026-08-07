package bspline

import (
	"iter"
	"math"

	"seedhammer.com/bezier"
)

type segmentBounds struct {
	Knots [4]Knot
}

type Segment struct {
	// The previous segment end point.
	prev bezier.Point
	// Knots is a sliding window of knots.
	Knots [3]Knot
}

// Curve is an iterator over the knots of a b-spline.
type Curve = iter.Seq[Knot]

type Knot struct {
	Ctrl    bezier.Point
	T       uint
	Engrave bool
}

func (s *segmentBounds) Knot(k Knot) (Bounds, bool) {
	c0, c1, c2, c3 := s.Knots[0].Ctrl, s.Knots[1].Ctrl, s.Knots[2].Ctrl, s.Knots[3].Ctrl
	segKnot := s.Knots[2]
	copy(s.Knots[:3], s.Knots[1:])
	s.Knots[3] = k
	return Bounds{
		Min: bezier.Point{
			X: min(c0.X, c1.X, c2.X, c3.X),
			Y: min(c0.Y, c1.Y, c2.Y, c3.Y),
		},
		Max: bezier.Point{
			X: max(c0.X, c1.X, c2.X, c3.X),
			Y: max(c0.Y, c1.Y, c2.Y, c3.Y),
		},
	}, segKnot.Engrave
}

func (s *Segment) Knot(k Knot) (bezier.Cubic, uint, bool) {
	// Extract Bézier curve through Böhm's algorithm.
	// See "An Introduction to B-Spline Curves" by Thomas W. Sederberg.
	dt2, dt3, dt4, dt5 := s.Knots[0].T, s.Knots[1].T, s.Knots[2].T, k.T
	P234, P345, P456 := s.Knots[0].Ctrl, s.Knots[1].Ctrl, s.Knots[2].Ctrl
	engrave := s.Knots[1].Engrave

	// Shift the knot window.
	copy(s.Knots[:2], s.Knots[1:])
	s.Knots[2] = k
	// The start point of this segment is the end point of the previous.
	P333 := s.prev

	if dt3 == 0 {
		s.prev = P345
		// Zero duration curve.
		return bezier.Cubic{
			C0: P234,
			C1: P234,
			C2: P345,
			C3: P345,
		}, 0, engrave
	}
	d1 := int(dt4 + dt3 + dt2)
	P334 := bezier.P64(P234).Mul(int(dt4 + dt3)).Add(bezier.P64(P345).Mul(int(dt2))).Div(d1).Point()
	P344 := bezier.P64(P234).Mul(int(dt4)).Add(bezier.P64(P345).Mul(int(dt3 + dt2))).Div(d1).Point()
	d3 := int(dt5 + dt4 + dt3)
	P445 := bezier.P64(P345).Mul(int(dt5 + dt4)).Add(bezier.P64(P456).Mul(int(dt3))).Div(d3).Point()
	d5 := int(dt4 + dt3)
	P444 := bezier.P64(P344).Mul(int(dt4)).Add(bezier.P64(P445).Mul(int(dt3))).Div(d5).Point()
	s.prev = P444
	return bezier.Cubic{
		C0: P333,
		C1: P334,
		C2: P344,
		C3: P444,
	}, dt3, engrave
}

// Kinematics track the derivative values of a B-Spline.
type Kinematics struct {
	Velocity     bezier.Point
	Acceleration bezier.Point
	Jerk         bezier.Point

	// Sliding window of knots and state.
	knots [3]uint
	p     [2]bezier.Point
	v     bezier.Point
	a     bezier.Point
}

// Knot shifts the spline one knot and updates the kinematics.
func (k *Kinematics) Knot(t uint, ctrl bezier.Point, scale uint) {
	copy(k.knots[:], k.knots[1:])
	k.knots[2] = t
	v := k.derive(k.p[0], k.p[1], 3, scale)
	copy(k.p[:1], k.p[1:])
	k.p[1] = ctrl
	a := k.derive(k.v, v, 2, scale)
	k.v = v
	j := k.derive(k.a, a, 1, scale)
	k.a = a
	k.Velocity = v
	k.Acceleration = a
	k.Jerk = j
}

func (k *Kinematics) Max() (v, a, j uint) {
	vv, aa, jj := k.Velocity, k.Acceleration, k.Jerk
	return uint(max(vv.X, -vv.X, vv.Y, -vv.Y)),
		uint(max(aa.X, -aa.X, aa.Y, -aa.Y)),
		uint(max(jj.X, -jj.X, jj.Y, -jj.Y))
}

func (k *Kinematics) derive(p0, p1 bezier.Point, degree int, scale uint) bezier.Point {
	t := uint(0)
	knots := k.knots[:degree]
	for _, k := range knots {
		t += k
	}
	if t == 0 {
		return bezier.Point{}
	}
	return bezier.P64(p1.Sub(p0)).Mul(degree * int(scale)).Div(int(t)).Point()
}

// ComputeKinematics compute the absolute kinematic maximums of the
// clamped B-spline.
func ComputeKinematics(spline []Knot, scale uint) (v, a, j uint) {
	var deriv Kinematics
	var maxv, maxa, maxj uint
	for _, p := range spline {
		deriv.Knot(p.T, p.Ctrl, scale)
		v, a, j := deriv.Max()
		maxv = max(maxv, v)
		maxa = max(maxa, a)
		maxj = max(maxj, j)
	}
	return maxv, maxa, maxj
}

type Attributes struct {
	Bounds Bounds
	// Duration is the whole spline's tick count, and it is uint64 rather than
	// uint BECAUSE THE FIRMWARE TARGET IS 32-BIT. Knot.T is a per-knot count
	// and stays uint; the SUM is what overflows.
	//
	// Measured on the real proof patterns at 1.0mm/s with 8 passes:
	// CONSTPROOF! plans 5,221,685,814 ticks and TEXTPROOF! 5,143,579,063 --
	// both past MaxUint32 (4,294,967,295) by about 1.2x. On RP2350 a uint
	// accumulator wrapped CONSTPROOF! to ~926.7M, which the engrave screen
	// then displayed as a 80-minute countdown for a seven-and-a-half-hour job
	// before underflowing partway through. The cut itself was never affected
	// -- completion is signalled by the job goroutine's error channel, not by
	// this count -- but the number the operator plans around was off by 5x.
	Duration uint64
}

// Bounds is like [image.Rectangle] with its upper
// bound inclusive.
type Bounds struct {
	Min, Max bezier.Point
}

func (b Bounds) In(b2 Bounds) bool {
	return inBounds(b.Min, b2) && inBounds(b.Max, b2)
}

func (b Bounds) Union(b2 Bounds) Bounds {
	return Bounds{
		Min: bezier.Point{
			X: min(b.Min.X, b2.Min.X),
			Y: min(b.Min.Y, b2.Min.Y),
		},
		Max: bezier.Point{
			X: max(b.Max.X, b2.Max.X),
			Y: max(b.Max.Y, b2.Max.Y),
		},
	}
}

func (b Bounds) Empty() bool {
	return b.Max.X < b.Min.X || b.Max.Y < b.Min.Y
}

func (b Bounds) Dx() int {
	return int(b.Max.X - b.Min.X)
}

func (b Bounds) Dy() int {
	return int(b.Max.Y - b.Min.Y)
}

func inBounds(p bezier.Point, b Bounds) bool {
	return b.Min.X <= p.X && p.X <= b.Max.X &&
		b.Min.Y <= p.Y && p.Y <= b.Max.Y
}

func Measure(spline Curve) Attributes {
	inf := Bounds{
		Min: bezier.Point{X: math.MaxInt32, Y: math.MaxInt32},
		Max: bezier.Point{X: math.MinInt32, Y: math.MinInt32},
	}
	bounds := inf
	// uint64, not uint: see Attributes.Duration. The widest real plate is
	// ~1.2x MaxUint32, so this accumulator wraps on the 32-bit firmware target
	// and nowhere else -- which is exactly why no host test caught it.
	var t uint64
	var bsb segmentBounds
	for c := range spline {
		t += uint64(c.T)
		if bb, engrave := bsb.Knot(c); engrave {
			bounds = bounds.Union(bb)
		}
	}
	return Attributes{
		Bounds:   bounds,
		Duration: t,
	}
}
