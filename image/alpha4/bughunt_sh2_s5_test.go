package alpha4

import (
	"image"
	"testing"
)

// TestBughuntSh225Alpha4WideSpan: bug-hunt sh2 #25. A Rectangle holds int8
// coordinates, so a span of 128..255 is representable (Rect() round-trips it),
// but New and PixOffset subtract in int8 and wrap.
//
// Open product question: whether a span over 127 should be supported (assumed
// here) or refused by Rect with a clear error.
func TestBughuntSh225Alpha4WideSpan(t *testing.T) {
	r := Rectangle{MinX: -100, MinY: 0, MaxX: 100, MaxY: 10} // 200 x 10
	if got, want := r.Rect(), image.Rect(-100, 0, 100, 10); got != want {
		t.Fatalf("precondition: Rect() = %v, want %v", got, want)
	}
	var img *Image
	func() {
		defer func() {
			if p := recover(); p != nil {
				t.Errorf("New(%+v) panicked: %v", r, p)
			}
		}()
		img = New(r)
	}()
	if img != nil && len(img.Pix) != 1000 {
		t.Errorf("len(New(%+v).Pix) = %d, want 1000 (200*10 pixels, 2 per byte)", r, len(img.Pix))
	}
	// PixOffset does not depend on New.
	if got, want := (&Image{Rect: r}).PixOffset(50, 5), 5*200+150; got != want {
		t.Errorf("PixOffset(50, 5) = %d, want %d", got, want)
	}
	// Far corner round trip.
	if img != nil && len(img.Pix) == 1000 {
		img.SetAlpha4(99, 9, 0b1010)
		if a := img.AlphaAt(99, 9).A; a != 0b1010_1010 {
			t.Errorf("AlphaAt(99, 9) = %#x after SetAlpha4(…, 0b1010), want 0xaa", a)
		}
	}
}
