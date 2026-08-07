package assets

import "testing"

// TestIconGearIsDrawn: an icon that generated to an empty rect draws nothing and
// leaves an invisible tap target, which a synthetic touch test still passes.
func TestIconGearIsDrawn(t *testing.T) {
	b := IconGear.Bounds()
	if b.Dx() < 8 || b.Dy() < 8 {
		t.Errorf("IconGear bounds are %dx%d; too small to find with a finger", b.Dx(), b.Dy())
	}
	if len(IconGearData) == 0 {
		t.Error("IconGear has no pixel data")
	}
}
