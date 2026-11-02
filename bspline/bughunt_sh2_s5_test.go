package bspline

import (
	"testing"

	"seedhammer.com/bezier"
)

// TestBughuntSh226InterpolateNil: bug-hunt sh2 #26. The doc of
// InterpolatePoints says "a zero-length input B-spline is returned as is".
// Zero input points must not panic.
func TestBughuntSh226InterpolateNil(t *testing.T) {
	for name, in := range map[string][]bezier.Point{
		"nil":   nil,
		"empty": {},
	} {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if p := recover(); p != nil {
					t.Errorf("InterpolatePoints(%s) panicked: %v", name, p)
				}
			}()
			got, err := InterpolatePoints(in)
			if err != nil {
				t.Errorf("InterpolatePoints(%s) error: %v", name, err)
			}
			if len(got) != 0 {
				t.Errorf("InterpolatePoints(%s) = %v, want the empty input back", name, got)
			}
		})
	}
}
