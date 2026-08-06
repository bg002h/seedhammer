// Package sh2 holds the SeedHammer II's machine parameters for HOST tools --
// cmd/plateview and cmd/emu -- which need to plan a toolpath exactly as the
// device does but cannot import the device's own definition of it.
//
// cmd/controller/platform_sh2.go is `tinygo && rp`. It is the ORIGINAL; this
// is a copy, and a copy that drifts renders a plate no machine cuts, quietly.
// TestParamsMatchTheMachine parses that file and fails on any divergence, so
// do not edit these without editing them there.
//
// They are not decoration: PlanEngraving is jerk- and acceleration-limited, so
// these numbers shape the toolpath and set the time estimate.
package sh2

import (
	"seedhammer.com/driver/tmc2209"
	"seedhammer.com/engrave"
)

const (
	FullStepsPerRevolution = 200
	MMPerRevolution        = 8
	// MM is the number of (micro-)steps per millimeter: 6400.
	MM = FullStepsPerRevolution / MMPerRevolution * tmc2209.Microsteps

	StrokeWidth = 0.3 * MM
	TopSpeed    = 30 * MM
	// EngravingSpeed is HALF the upstream 8mm/s; see the rationale on
	// cmd/controller/platform_sh2.go's engravingSpeed, which this copies.
	EngravingSpeed = 4 * MM
	Acceleration   = 250 * MM
	Jerk           = 2600 * MM
)

// DisplayWidth and DisplayHeight are the panel the machine ships with,
// cmd/controller/platform_sh2.go's lcdWidth x lcdHeight. A host tool that
// lays the UI out at any other size draws widgets the device never draws.
const (
	DisplayWidth  = 480
	DisplayHeight = 320
)

// Params is what the engraver plans with.
func Params() engrave.Params {
	return engrave.Params{
		StrokeWidth: StrokeWidth,
		Millimeter:  MM,
		StepperConfig: engrave.StepperConfig{
			TicksPerSecond: TopSpeed,
			Speed:          TopSpeed,
			EngravingSpeed: EngravingSpeed,
			Acceleration:   Acceleration,
			Jerk:           Jerk,
		},
	}
}
