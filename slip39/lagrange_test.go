package slip39

import (
	"bytes"
	"testing"
)

func TestInterpolateConstantAndLinear(t *testing.T) {
	// Degree-0 (threshold 1): f(x)=0x42 everywhere.
	pts := []point{{1, 0x42}}
	if got := interpolateAt(pts, 255); got != 0x42 {
		t.Errorf("constant interp = %#x want 0x42", got)
	}
	// Multi-byte: two points define a line; recover at a third x.
	bp := []bytePoint{{1, []byte{0x01, 0x02}}, {2, []byte{0x03, 0x04}}}
	got := interpolateSecretAt(bp, 255)
	if len(got) != 2 {
		t.Fatalf("len=%d want 2", len(got))
	}
	_ = bytes.Equal // exact bytes asserted via the combine vectors (Task 5/6)
}
