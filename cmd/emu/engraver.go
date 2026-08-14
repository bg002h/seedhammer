//go:build js

package main

import (
	"time"

	"seedhammer.com/bspline"
	"seedhammer.com/engrave"
	"seedhammer.com/gui"
)

// emuEngraver accepts a step stream and throws it away, at a pace.
//
// The pace is the point. Returning immediately would be wrong twice: the
// engrave screen would flash past faster than it can be read, and -- because
// the GUI writes the whole plan in a tight loop -- the wasm scheduler would
// never yield to the browser, freezing the tab until the plate finished.
// Sleeping parks the goroutine, which is what lets JS run.
//
// It does NOT model the machine: no stalls, no load, no failure. What the
// emulator qualifies is the UI around engraving, not engraving.
//
// It does, however, DECODE the stream on the way past (toolpath.go). Throwing
// it away left one defect class reachable only by cutting metal: zeroing
// seed-derived geometry too early does not leave a residue, it sends the head
// somewhere wrong, and nothing above this line would notice.
// It DOES home, though, which is the one part of the machine it models on
// purpose. See toolpathRecorder.Home and F-121: the recorder spans every job
// while each job's step stream is a delta from the origin, so an emulator that
// skips the homing the device always performs records a resumed plate offset
// from the plan by wherever the last one stopped.
type emuEngraver struct {
	// job carries the homing and the decoding. Both live in toolpath.go,
	// which has no build tag, so both are reachable by `go test` -- this file
	// is js-only and nothing in it ever is.
	job jobRecorder
	// plan is shared with the platform: the page draws one plate at a time,
	// and an engraver's life is one pass over it.
	plan   *platePlan
	params engrave.Params
	// pace counts Writes between yields. Zero value is correct: it reads the
	// pace fresh on every call, so a new engraver starts at whatever the page
	// last set rather than at a stale copy.
	pace pacer
}

// Plate implements gui.PlateAware: it is handed the plan before the first
// Write, which is what lets the page show the whole plate at the moment the
// cut starts rather than watching it appear stroke by stroke.
//
// The interface exists only in non-TinyGo builds, so the firmware neither calls
// this nor contains the type -- see gui/plate_hook.go for why a spline is not
// something to hand out casually.
func (e *emuEngraver) Plate(spline bspline.Curve, conf engrave.StepperConfig, resumed bool) {
	beginPlate(e.plan, e.job.rec, spline, e.params, resumed)
}

// stepPace is how long each Write pretends to take. Small enough that a plate
// does not take its real quarter of an hour, large enough that the browser
// gets a turn between writes.
//
// How OFTEN a Write sleeps is the walk's business, not this constant's -- see
// pace.go, and window.shPace. The duration stays fixed because shortening it
// buys nothing against the browser's timer granularity.
const stepPace = time.Millisecond

func (e *emuEngraver) Write(steps []uint32) (int, error) {
	if _, err := e.job.Write(steps); err != nil {
		return 0, err
	}
	// The recorder above ran unconditionally: the pace decides when to yield,
	// never what to record, so the step stream is identical at any pace.
	if e.pace.yield() {
		time.Sleep(stepPace)
	}
	return len(steps), nil
}

// Close homes, as homingEngraver.Close does. Unconditionally, where the device
// skips it after a write error: nothing in this engraver can fail a write, so
// there is no error state to respect and inventing one would be modelling a
// machine this file says plainly it does not model.
func (e *emuEngraver) Close() error {
	e.job.Close()
	return nil
}

// Stats reports a healthy machine: zero stalls, zero load. A stall the
// operator never provoked would be an invented failure.
func (e *emuEngraver) Stats() gui.EngraverStats {
	return gui.EngraverStats{}
}
