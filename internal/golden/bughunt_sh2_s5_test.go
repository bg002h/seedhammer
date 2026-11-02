package golden

import (
	"bytes"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"testing"

	"seedhammer.com/bezier"
	"seedhammer.com/bspline"
)

// TestBughuntSh227aRoundTrip: bug-hunt sh2 #27 (first bug). Every spline must
// survive encodeBSpline -> decodeBSpline, including one whose first knot is
// engraved and the empty one (the encoder emits a leading zero-count header
// for both).
func TestBughuntSh227aRoundTrip(t *testing.T) {
	k := func(x, y int, tt uint, engrave bool) bspline.Knot {
		return bspline.Knot{Ctrl: bezier.Pt(x, y), T: tt, Engrave: engrave}
	}
	tests := []struct {
		name string
		s    []bspline.Knot
	}{
		{"control: move first, then engrave", []bspline.Knot{k(1, 2, 3, false), k(5, 2, 3, true), k(9, -4, 1, true)}},
		{"engrave first", []bspline.Knot{k(1, 2, 3, true), k(5, 2, 3, true)}},
		{"empty", nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			enc := encodeBSpline(slices.Values(tc.s))
			got, err := decodeBSpline(enc)
			if err != nil {
				t.Fatalf("decode(%v): %v", enc, err)
			}
			if !slices.Equal(got, tc.s) {
				t.Errorf("enc %v decoded to %v, want %v", enc, got, tc.s)
			}
		})
	}
}

// TestBughuntSh227aCompareBSplineUpdateThenCheck is the same fault through the
// public API: a golden written with update=true must then compare clean.
func TestBughuntSh227aCompareBSplineUpdateThenCheck(t *testing.T) {
	spline := []bspline.Knot{
		{Ctrl: bezier.Pt(1, 2), T: 3, Engrave: true},
		{Ctrl: bezier.Pt(5, 2), T: 3, Engrave: true},
	}
	dir := t.TempDir()
	p := filepath.Join(dir, "g.bin")
	bounds := bspline.Bounds{Max: bezier.Pt(1000, 1000)}
	if err := CompareBSpline(p, true, dir, 1, bounds, slices.Values(spline)); err != nil {
		t.Fatalf("update: %v", err)
	}
	if err := CompareBSpline(p, false, dir, 1, bounds, slices.Values(spline)); err != nil {
		t.Errorf("a golden just written does not compare equal to its own spline: %v", err)
	}
}

// TestBughuntSh227bZeroCountHeader: bug-hunt sh2 #27 (second bug). A header
// byte with a zero count must not be read as 255 knots. Whether it is an error
// or is skipped is open; either is accepted here.
func TestBughuntSh227bZeroCountHeader(t *testing.T) {
	// 0x00 = zero-count header; 0x01 = one knot, not engraved; varints
	// x=1 (0x02), y=2 (0x04), t=3 (0x06).
	oneKnot := bspline.Knot{Ctrl: bezier.Pt(1, 2), T: 3}
	tests := []struct {
		name string
		enc  []byte
		ok   []bspline.Knot // the accepted non-error result
	}{
		{"lone zero header", []byte{0x00}, nil},
		{"zero header, then a one-knot header", []byte{0x00, 0x01, 0x02, 0x04, 0x06}, []bspline.Knot{oneKnot}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := decodeBSpline(tc.enc)
			if err == nil && !slices.Equal(got, tc.ok) {
				t.Errorf("decode(%v) = %v, nil; want an error or %v", tc.enc, got, tc.ok)
			}
		})
	}
}

var svgWidth = regexp.MustCompile(`width="(\d+)" height="600"`)

// TestBughuntSh227cVectorizeAspect: bug-hunt sh2 #27 (third bug). The SVG
// width is 600*aspect; the aspect is an integer division, so a tall plate
// gets width 0.
func TestBughuntSh227cVectorizeAspect(t *testing.T) {
	// 100 x 300 plus a 20-unit margin on each side: 140 x 340.
	bounds := bspline.Bounds{Max: bezier.Pt(100, 300)}
	var buf bytes.Buffer
	if err := Vectorize(&buf, 1, bounds, slices.Values([]bspline.Knot(nil))); err != nil {
		t.Fatal(err)
	}
	m := svgWidth.FindStringSubmatch(buf.String())
	if m == nil {
		t.Fatalf("no width/height attributes in %q", buf.String())
	}
	w, _ := strconv.Atoi(m[1])
	// 600*140/340 = 247.05
	if w < 246 || w > 248 {
		t.Errorf("SVG width = %d, want about 247 (600 * 140/340)", w)
	}
}
