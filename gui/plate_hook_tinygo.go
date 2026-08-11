//go:build tinygo

package gui

import (
	"seedhammer.com/bspline"
	"seedhammer.com/engrave"
)

// notifyPlate does nothing on the machine, and PlateAware does not exist here.
//
// See plate_hook.go for why the firmware carries neither: the spline it would
// pass is seed-derived geometry, and the only consumer for it lives in a
// browser. An interface the image does not contain cannot be implemented by
// accident, so this costs the device an empty call the compiler removes and
// buys a claim that needs no re-checking.
func notifyPlate(Engraver, bspline.Curve, engrave.StepperConfig, bool) {}
