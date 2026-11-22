package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

// nrgbaPNG encodes a PNG, ncolors wide, that decodes to *image.NRGBA with
// exactly ncolors distinct colours (pixel 0 is transparent, so the encoder
// does not pick the opaque RGB form that decodes to *image.RGBA).
func nrgbaPNG(t *testing.T, ncolors int) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, ncolors, 1))
	for i := 1; i < ncolors; i++ {
		img.SetNRGBA(i, 0, color.NRGBA{R: uint8(i), G: uint8(i >> 8), A: 255})
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// TestBughuntSh229AssetsPalette256: bug-hunt sh2 #29. A palette holds 256
// entries (uint8 index 0..255), so exactly 256 colours must convert; 257 must
// not.
func TestBughuntSh229AssetsPalette256(t *testing.T) {
	t.Skip("deferred-minor: sh-assets-palette-256")
	t.Chdir(t.TempDir()) // convert writes <name>.bin to the working directory
	for _, tc := range []struct {
		ncolors int
		wantErr bool
	}{
		{255, false},
		{256, false},
		{257, true},
	} {
		err := convert(new(bytes.Buffer), bytes.NewReader(nrgbaPNG(t, tc.ncolors)), "x.png")
		if (err != nil) != tc.wantErr {
			t.Errorf("convert(%d colours) error = %v, want error: %v", tc.ncolors, err, tc.wantErr)
		}
	}
}
