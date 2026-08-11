package poppins

import (
	"testing"

	"seedhammer.com/image/alpha4"
)

// TestBoldprogress45HasPercentGlyph is F-86's regression guard: unlockDerive
// (gui/unlock_kdf.go) formats the KDF progress readout as "%d%%" in
// ctx.Styles.progress, which is Boldprogress45 -- the engrave timer's face.
// Boldprogress45 was generated with -alphabet "0123456789:", which has no
// "%", so the percent sign rendered as zero pixels for the whole ~31s KDF
// wait (the machine's longest single screen).
//
// Asserted on the RASTER, not on measured label width -- a rasterising check
// is exactly what F-78's own text says this suite is missing, and it is a
// stronger property than width: a glyph absent from the generation alphabet
// still has an INDEX entry (bitmap.Face.glyphFor returns ok=true for any
// rune < indexLen, populated or not), just with a zero-size Rect. Width
// alone can't distinguish "no glyph" from "1px glyph"; the raster can.
func TestBoldprogress45HasPercentGlyph(t *testing.T) {
	img, _, ok := Boldprogress45.Glyph('%')
	if !ok {
		t.Fatal("Boldprogress45.Glyph('%') ok=false -- '%' is outside this face's index range")
	}
	if !rasterHasInk(img) {
		t.Error("Boldprogress45.Glyph('%') renders a zero-pixel raster -- the KDF progress " +
			"screen shows a bare number for its whole ~31s wait (F-86)")
	}

	// Positive control: 'x' is genuinely absent from this face's restricted
	// alphabet ("0123456789:%") and was never in it. The same check must
	// report it empty, proving rasterHasInk can actually catch the F-86
	// defect shape rather than being vacuously true for any glyph.
	xImg, _, _ := Boldprogress45.Glyph('x')
	if rasterHasInk(xImg) {
		t.Fatal("setup invalid: 'x' unexpectedly has ink in Boldprogress45 -- the positive " +
			"control no longer proves what it claims")
	}
}

// rasterHasInk reports whether img has a non-empty bounding box AND at least
// one non-transparent pixel. A glyph that is in the index but was never
// populated at generation time (rune outside the -alphabet flag) decodes to
// a zero-size Rect, which this treats as "no ink" -- the same shape as the
// zero-pixel-glyph defect this test guards.
func rasterHasInk(img alpha4.Image) bool {
	b := img.Bounds()
	if b.Dx() <= 0 || b.Dy() <= 0 {
		return false
	}
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if img.AlphaAt(x, y).A != 0 {
				return true
			}
		}
	}
	return false
}
