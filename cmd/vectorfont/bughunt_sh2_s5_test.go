package main

import (
	"encoding/xml"
	"errors"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"seedhammer.com/bezier"
)

// parsePathD runs a single <path d="..."/> through the real parseSegments.
func parsePathD(t *testing.T, d string) []segment {
	t.Helper()
	dec := xml.NewDecoder(strings.NewReader(`<path d="` + d + `"/>`))
	tok, err := dec.Token()
	if err != nil {
		t.Fatal(err)
	}
	segs, err := parseSegments(nil, dec, tok.(xml.StartElement), 0, 0, nil)
	if err != nil {
		t.Fatalf("parseSegments(%q): %v", d, err)
	}
	return segs
}

// TestBughuntSh230aSpliceFirstLine: bug-hunt sh2 #30 (first bug). With -splice,
// a glyph whose first segment is a line indexes segs[-1].
func TestBughuntSh230aSpliceFirstLine(t *testing.T) {
	old := *splice
	*splice = true
	t.Cleanup(func() { *splice = old })
	// A <path> that does not start with a moveto yields a leading LineTo.
	segs := optimizeSegments(parsePathD(t, "L10,10 20,20"))
	if len(segs) == 0 || segs[0].Op != SegmentOpLineTo {
		t.Fatalf("precondition: want a leading LineTo, got %+v", segs)
	}
	defer func() {
		if p := recover(); p != nil {
			t.Errorf("segmentsToBSpline panicked on a path starting with a line: %v", p)
		}
	}()
	segmentsToBSpline(segs, 4)
}

// TestBughuntSh230bRunMissingInput: bug-hunt sh2 #30 (second bug). run ignores
// the os.ReadFile error, so the operator is told "EOF" instead of why.
func TestBughuntSh230bRunMissingInput(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	err := run(filepath.Join(dir, "nope.svg"), "out")
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("run(missing file) = %v, want an error wrapping fs.ErrNotExist", err)
	}
}

// TestBughuntSh230cRepeatedS: bug-hunt sh2 #30 (third bug). SVG 1.1 paths.html:
// "The first control point is assumed to be the reflection of the second
// control point on the previous command relative to the current point", and
// "The command letter can be eliminated on subsequent commands if the same
// command is used multiple times in a row". So "S a b c d" is "S a b S c d".
func TestBughuntSh230cRepeatedS(t *testing.T) {
	// First curve: pen (0,0), p2 (1,1), p3 (2,2). Second curve: pen (2,2), the
	// previous second control point (1,1), so p1 = 2*(2,2)-(1,1) = (3,3).
	segs := parsePathD(t, "M0,0 S1,1 2,2 3,3 4,4")
	if len(segs) != 3 {
		t.Fatalf("want 3 segments (M, C, C), got %d: %+v", len(segs), segs)
	}
	if got, want := segs[2].Args[0], bezier.Pt(3, 3); got != want {
		t.Errorf("second curve's first control point = %v, want %v", got, want)
	}
}

// TestBughuntSh230dRepeatedRelative: bug-hunt sh2 #30 (fourth bug). Relative
// coordinates are relative to the current point at the start of each
// (implicitly repeated) command (SVG 1.1 paths.html, as above).
func TestBughuntSh230dRepeatedRelative(t *testing.T) {
	tests := []struct {
		d    string
		want bezier.Point
	}{
		{"m0,0 l1,0 1,0", bezier.Pt(2, 0)},
		{"m0,0 h1 1", bezier.Pt(2, 0)},
		{"m0,0 v1 1", bezier.Pt(0, 2)},
	}
	for _, tc := range tests {
		t.Run(tc.d, func(t *testing.T) {
			segs := parsePathD(t, tc.d)
			if len(segs) != 3 {
				t.Fatalf("want 3 segments, got %d: %+v", len(segs), segs)
			}
			if got := segs[len(segs)-1].Args[0]; got != tc.want {
				t.Errorf("%q ends at %v, want %v", tc.d, got, tc.want)
			}
		})
	}
}

// TestBughuntSh249OptimizeStalePen: bug-hunt sh2 #49. After merging a colinear
// segment, optimizeSegments leaves the pen at the old end, so a backtrack to
// that old end is dropped as "p1 == p0".
func TestBughuntSh249OptimizeStalePen(t *testing.T) {
	line := func(x, y int) segment {
		return segment{Op: SegmentOpLineTo, Args: [4]bezier.Point{bezier.Pt(x, y)}}
	}
	in := []segment{
		{Op: SegmentOpMoveTo, Args: [4]bezier.Point{bezier.Pt(5, 5)}},
		line(6, 5), line(7, 5), line(6, 5), line(6, 9),
	}
	var got []bezier.Point
	for _, s := range optimizeSegments(in) {
		got = append(got, s.Args[0])
	}
	want := []bezier.Point{bezier.Pt(5, 5), bezier.Pt(7, 5), bezier.Pt(6, 5), bezier.Pt(6, 9)}
	if len(got) != len(want) {
		t.Fatalf("optimized path %v, want %v (the backtrack to (6,5) must survive)", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("optimized path %v, want %v", got, want)
		}
	}
}
