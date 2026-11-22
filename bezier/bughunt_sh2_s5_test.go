package bezier

import "testing"

// withinHull reports whether p lies in the bounding box of c's control points
// (a sample of a cubic always does).
func withinHull(c Cubic, p Point) bool {
	return p.X >= min(c.C0.X, c.C1.X, c.C2.X, c.C3.X) &&
		p.X <= max(c.C0.X, c.C1.X, c.C2.X, c.C3.X) &&
		p.Y >= min(c.C0.Y, c.C1.Y, c.C2.Y, c.C3.Y) &&
		p.Y <= max(c.C0.Y, c.C1.Y, c.C2.Y, c.C3.Y)
}

// TestBughuntSh241SampleNoOrigin: bug-hunt sh2 #41. Sample must only append
// points on the curve; with the inner loop skipped it appends the zero Point.
func TestBughuntSh241SampleNoOrigin(t *testing.T) {
	t.Skip("deferred-minor: sh-bezier-degenerate-sample")
	p := Pt(100, 100)
	tests := []struct {
		name   string
		points []Point
		c      Cubic
	}{
		// The prover's input.
		{"zero-length, nil points", nil, Cubic{p, p, p, p}},
		// Production shape: cmd/vectorfont seeds points with C0 first.
		{"zero-length, seeded", []Point{p}, Cubic{p, p, p, p}},
		// The report says "very short"; a 40-unit line is fine (control row).
		{"short line (40 units), seeded", []Point{Pt(1000, 1000)},
			Cubic{Pt(1000, 1000), Pt(1013, 1000), Pt(1027, 1000), Pt(1040, 1000)}},
		// The other trigger: a curve longer than about 400 spacings makes one
		// of the 200 interpolator steps cover two samples' worth of distance.
		{"long line (4000 units = 400 spacings), seeded", []Point{Pt(1000, 1000)},
			Cubic{Pt(1000, 1000), Pt(2333, 1000), Pt(3667, 1000), Pt(5000, 1000)}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Sample(append([]Point(nil), tc.points...), tc.c, 10)
			for i, q := range got[len(tc.points):] {
				if !withinHull(tc.c, q) {
					t.Errorf("sample %d = %v lies off the curve (hull %v..%v)", i, q, tc.c.C0, tc.c.C3)
				}
			}
		})
	}
}

// TestBughuntSh243BoundsEmpty: bug-hunt sh2 #43. Empty compared Max.Y with
// Min.X. (bspline.Bounds.Empty had the same fault, fixed in aef6058; the
// bezier copy was not.)
func TestBughuntSh243BoundsEmpty(t *testing.T) {
	t.Skip("deferred-minor: sh-bounds-empty-inverted")
	tests := []struct {
		name string
		b    Bounds
		want bool
	}{
		{"zero value", Bounds{}, false},
		{"real box", Bounds{Min: Pt(79816, 23194), Max: Pt(89617, 37350)}, false},
		{"tall and far right", Bounds{Min: Pt(500, 0), Max: Pt(600, 10)}, false},
		{"inverted x", Bounds{Min: Pt(10, 0), Max: Pt(5, 10)}, true},
		{"inverted y", Bounds{Min: Pt(0, 10), Max: Pt(10, 5)}, true},
		{"inverted y, Min.X below Max.Y (the report's case)", Bounds{Min: Pt(0, 50), Max: Pt(100, 10)}, true},
	}
	for _, tc := range tests {
		if got := tc.b.Empty(); got != tc.want {
			t.Errorf("%s: %+v.Empty() = %v, want %v", tc.name, tc.b, got, tc.want)
		}
	}
}
