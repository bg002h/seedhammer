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
// Yields stay frequent in absolute terms: at 512 the engrave goroutine parks
// about 75 times a second.
//
// WHERE TO SET IT, measured end to end rather than extrapolated from the curve
// above -- the same six-plate Trace A bundle, whole walk, wall clock:
//
//	pace     bundle      vs 512
//	  64       398s        0.59x
//	 512       236s        1.00x
//	2048       186s        1.27x   <- walk_trace_a.js's default
//	8192       183s        1.02x
//
// So 2048 is the last real gain and 8192 buys three seconds. Past roughly 2048
// the walk is dominated by the DRIVER's fixed cost -- the hold threshold, the
// stall-detect poll, the settle sleeps -- and not by cutting, so a bundle big
// enough to hurt (25 plates is about 12 minutes at 2048) gets faster by trimming
// those, not by raising this.
//
// WHICH IS WHY THERE IS A CEILING. Since nothing above 2048 pays, an unbounded
// knob offers only ways to lose: at a large enough value the engrave goroutine
// effectively never yields, and a wedged tab reports no result at all rather
// than a slow one -- the precise failure stepPace exists to prevent. maxPace is
// 4096, double the useful setting and comfortably below anything that starves
// the browser. The 8192 row above stays because it is the evidence the ceiling
// costs nothing, not because it is reachable.
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

// defaultPace is what the emulator runs at unless something says otherwise.
//
// It is the WALK pace, not 1, and that is deliberate. A walk that has to
// remember to opt into the fast path is a walk that will one day forget, and
// every later stage's gate is a walk (§4.5). Making the fast pace the default
// means a future walk script, a bare console session, and this one all get it
// without asking.
//
// Nothing is lost by it. The emulator does not model the machine -- engraver.go
// says so plainly -- so the twenty minutes were never fidelity, only sleeping.
// The original argument for a slow pace was that returning IMMEDIATELY would
// flash the screen past and freeze the tab; 2048 does neither, measured: the
// screen still refreshes 1.38 times a second and a plate takes about twenty
// seconds, which is more watchable than twenty minutes, not less.
//
// An operator who wants the real-time countdown back has shPace(1).
const defaultPace = 2048

// writesPerYield is how many Writes the engraver performs between yields to the
// browser. 1 is the human pace; defaultPace is where it starts.
//
// Atomic because the two ends are different goroutines: Write runs on the
// engrave goroutine while shPace is set from a JS callback on the main one.
var writesPerYield atomic.Int64

func init() { writesPerYield.Store(defaultPace) }

// maxPace is the highest pace a walk may ask for: double the 2048 that measured
// as the last real gain, so the useful range keeps headroom while the useless
// range stays out of reach.
const maxPace = 4096

// setWritesPerYield clamps and stores the pace, returning what was stored.
//
// Clamped at BOTH ends, and returning the stored value rather than nothing so a
// caller can see it was clamped -- window.shPace hands this straight back.
//
// Bottom: zero or negative would mean "never yield", the frozen tab stepPace
// exists to prevent. Top: see maxPace. Both ends are the same failure, reached
// from opposite directions -- a walk that wedges the browser reports no result
// at all, which is worse than a slow one.
func setWritesPerYield(n int64) int64 {
	if n < 1 {
		n = 1
	}
	if n > maxPace {
		n = maxPace
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
