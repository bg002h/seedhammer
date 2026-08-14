// This file carries NO build tag, for the reason engraved.go and screen.go
// give: the JS glue is //go:build js and can never run on this host, while the
// half that DECIDES something can, and is (pace_test.go). What is decided here
// is how often the emulator's engraver yields to the browser.
//
// WHY A KNOB. stepPace exists so a human can watch: engrave.go's comment is
// right that returning immediately would flash the screen past and starve the
// wasm scheduler. But §4.5 makes an automated walk the closing gate of every
// stage, and at one yield per Write a single plate takes about twelve minutes
// -- measured, on the first walk that ever reached a completed engrave. A
// six-plate bundle is then over an hour, per stage, and a gate nobody can
// afford to run is not a gate. So the pace stays for the human and gets out of
// the way for the walk.
//
// FREQUENCY, NOT DURATION -- and this is the part worth not re-deriving. The
// obvious lever is a shorter sleep, and it does not work: the browser's timer
// granularity puts a floor under how short a sleep can be, so lowering stepPace
// buys little. Yielding LESS OFTEN is independent of that floor.
//
// Measured on one plate, raising the pace mid-cut. It is NOT linear -- past a
// few dozen the sleeping stops being the bottleneck and the spline and recorder
// become it, so the curve saturates:
//
//	pace     steps/second   speedup
//	   1          162,385      1.0x
//	   8          977,161      6.0x
//	  64        3,010,289     18.5x
//	 512        6,455,772     39.8x
//
// So 512 turns a twelve-minute plate into about eighteen seconds, and there is
// little point going higher. Yields are still frequent in absolute terms: at
// 512 the engrave goroutine parks about 75 times a second.
//
// WHAT IT DOES NOT CHANGE, which is the whole reason it is safe: every knot
// still passes through the driver and the recorder still decodes all of it. The
// step stream is byte-identical at any pace -- the toolpath digest is the proof,
// and it is pace-independent by construction because nothing here touches the
// stream. Skipping the stream instead would be a different thing entirely: it
// would re-open the defect class engraver.go says it exists to catch, where
// geometry zeroed too early sends the head somewhere wrong and leaves no
// residue for anything above it to notice.

package main

import "sync/atomic"

// writesPerYield is how many Writes the engraver performs between yields to the
// browser. 1 is the human pace and the default; a walk raises it.
//
// Atomic because the two ends are different goroutines: Write runs on the
// engrave goroutine while shPace is set from a JS callback on the main one.
var writesPerYield atomic.Int64

// setWritesPerYield clamps and stores the pace, returning what was stored.
//
// Clamped at the bottom to 1 -- zero or negative would mean "never yield", which
// is precisely the frozen tab stepPace exists to prevent, and a walk that wedges
// the browser reports no result at all rather than a fast one.
func setWritesPerYield(n int64) int64 {
	if n < 1 {
		n = 1
	}
	writesPerYield.Store(n)
	return n
}

// pacer counts Writes and says when one should yield.
//
// Per-engraver rather than global state: an engraver's life is one pass over one
// plate, so the count resets with the plate and a pace change mid-bundle takes
// effect on the next plate without leaving a stale remainder behind.
type pacer struct {
	n int64
}

// yield reports whether this Write should sleep.
//
// Reads the pace on every call rather than caching it at construction, so a
// walk can raise the pace after a cut has already started -- which is the case
// that matters, since the operator only learns a plate is slow once it is
// underway.
func (p *pacer) yield() bool {
	every := writesPerYield.Load()
	if every < 1 {
		every = 1
	}
	p.n++
	if p.n < every {
		return false
	}
	p.n = 0
	return true
}
