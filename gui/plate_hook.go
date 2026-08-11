//go:build !tinygo

// The plate-plan seam, and the reason it is a build-tagged pair.
//
// cmd/emu wants to draw a plate's FINAL layout at the moment the engrave
// begins, and mark on it what is being cut right now. The second half it
// already has: cmd/emu's toolpathRecorder decodes the driver's real step
// stream. The first half exists nowhere below this line -- the plan is held by
// engraveJob and never offered to the platform, which sees only the steps that
// come out the far end of it.
//
// WHY `!tinygo` AND NOT A PLAIN TYPE ASSERTION. A spline is seed-derived
// geometry: it is a picture of the operator's words, at the scale they will be
// cut, and it is the same material F-107/F-108 are about. The emulator's
// consumer for it is JavaScript on a page -- outside anything Go can wipe. The
// firmware must therefore not merely decline to use this hook, it must not
// carry it, so that the copy inventory of seed-derived material stays true for
// the image by CONSTRUCTION rather than by argument. gui/preview.go keeps the
// plate previews out of the image the same way, for a related reason.
//
// The cost of the split is one empty function in the TinyGo build. The cost of
// getting it wrong is a hole in an inventory nobody re-derives.
package gui

import (
	"seedhammer.com/bspline"
	"seedhammer.com/engrave"
)

// PlateAware is an OPTIONAL interface an [Engraver] may implement to be handed
// the plan it is about to be given, before the first Write.
//
// It is on the Engraver rather than the Platform because the ENGRAVER's
// lifetime is the job's: Platform.Engraver is called once per runEngraving, so
// "a new engraver" and "a new pass over the plate" are the same event, and a
// hook hung there cannot drift from the thing it describes.
//
// spline is safe to range: [bspline.Curve] is an iterator that can be walked
// more than once (engrave.PlanEngraving allocates its own knot buffer), and the
// job ranges it again immediately afterwards. Implementations must not retain
// it past the job -- it is the operator's seed, drawn.
//
// resumed distinguishes a hold-to-resume from a fresh plate. Both call this,
// because both are a new engraver over the same spline; only the caller knows
// which, and a consumer that guesses gets it wrong exactly when a plate is
// interrupted.
type PlateAware interface {
	Plate(spline bspline.Curve, conf engrave.StepperConfig, resumed bool)
}

// notifyPlate offers the plan to d, if d wants it.
func notifyPlate(d Engraver, spline bspline.Curve, conf engrave.StepperConfig, resumed bool) {
	if pa, ok := d.(PlateAware); ok {
		pa.Plate(spline, conf, resumed)
	}
}
