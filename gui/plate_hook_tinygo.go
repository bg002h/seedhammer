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
// accident.
//
// WHAT IT COSTS, MEASURED -- because the first version of this comment claimed
// "an empty call the compiler removes" and that is FALSE. Built at the
// production settings with VERSION pinned so the stamp could not differ:
//
//	97e38c1 (before the hook)                     sha256 0a379302...
//	71f1d42 (with the hook)                       sha256 8c4380c4...
//	71f1d42 with ONLY the call in runEngraving deleted   sha256 0a379302...
//
// The third is byte-identical to the first, so the entire image delta -- 486,697
// bytes across 3,480 of 5,146 UF2 blocks, at unchanged total size, which is code
// shifting downstream rather than code being added -- is that one call. TinyGo
// does not eliminate it. The build is byte-reproducible across paths, checked by
// building the same SHA twice in different directories, so the difference is
// real and not build noise.
//
// It is still the right trade: the cost is one call to an empty function per
// ENGRAVING JOB -- not per knot, not per DMA write -- against an interface that
// cannot reach the image at all. But it is a cost, not a free abstraction, and
// the previous sentence was a claim about a compiler that nobody had asked the
// compiler about.
func notifyPlate(Engraver, bspline.Curve, engrave.StepperConfig, bool) {}
