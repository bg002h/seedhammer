//go:build !tinygo

package gui

import (
	"slices"
	"testing"
)

// TestPreviewMS1SeedQR: cmd/plateview can render the ms1 + Standard SeedQR
// plate (F-702 F5) through the same backup function a flow will call.
func TestPreviewMS1SeedQR(t *testing.T) {
	if !slices.Contains(PreviewPlates(), "ms1seedqr") {
		t.Fatalf("PreviewPlates() = %v, want an ms1seedqr entry", PreviewPlates())
	}
	pv, err := BuildPreview(newPlatform().EngraverParams(), "ms1seedqr", PreviewOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if !pv.HasQR {
		t.Error("the ms1 + SeedQR plate reports no QR")
	}
	if pv.Plate.Duration == 0 {
		t.Error("the preview plate is empty")
	}
}
