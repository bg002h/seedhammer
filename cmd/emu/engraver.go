//go:build js

package main

import (
	"time"

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
type emuEngraver struct{}

// stepPace is how long each Write pretends to take. Small enough that a plate
// does not take its real quarter of an hour, large enough that the browser
// gets a turn between writes.
const stepPace = time.Millisecond

func (e *emuEngraver) Write(steps []uint32) (int, error) {
	time.Sleep(stepPace)
	return len(steps), nil
}

func (e *emuEngraver) Close() error { return nil }

// Stats reports a healthy machine: zero stalls, zero load. A stall the
// operator never provoked would be an invented failure.
func (e *emuEngraver) Stats() gui.EngraverStats {
	return gui.EngraverStats{}
}
