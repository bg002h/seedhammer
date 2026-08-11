//go:build js

package main

import "syscall/js"

// installToolpathAPI exposes the recorder and the current plate to the page as
// window.shToolpath.
//
// The recording half is a JS API rather than a UI because the reading it serves
// is a COMPARISON: cut once uninterrupted, cut again with an abort and a
// hold-to-resume, and check the two runs describe the same motion. A human
// cannot eyeball that; a digest settles it.
//
// The plate half IS drawn, by index.html, because the reading THERE is the one
// a human does better than a digest: is the plate laid out the way it should
// be, and how far along is it.
//
//	shToolpath.reset()          start a fresh recording
//	shToolpath.summary()        JSON digest + anomalies (see Summary)
//	shToolpath.path()           JSON [[x,y,needle],...]
//	shToolpath.svg()            an SVG of the decoded motion
//	shToolpath.planSeq()        which plate is loaded; 0 before the first
//	shToolpath.plan()           that plate's layout, as an SVG document
//
// planSeq is separate from plan so the page can poll cheaply. A full seed plate
// renders to about 640KB of SVG and changes once per plate, while progress
// changes continuously -- handing back the plan on every poll would spend the
// frame budget re-serialising the half that did not move.
//
// Typical session, in the console:
//
//	shToolpath.reset(); /* cut the plate straight through */
//	a = JSON.parse(shToolpath.summary())
//	shToolpath.reset(); /* cut again, Back mid-cut, hold to resume */
//	b = JSON.parse(shToolpath.summary())
//	a.digest === b.digest   // the resumed plate follows the same path
//	b.cutsThroughOrigin     // must be false: a dive home is the F-108 failure
func installToolpathAPI(r *toolpathRecorder, p *platePlan) {
	api := map[string]any{
		"reset": js.FuncOf(func(js.Value, []js.Value) any {
			r.Reset()
			return nil
		}),
		"summary": js.FuncOf(func(_ js.Value, args []js.Value) any {
			frac := 0.0
			if len(args) > 0 && args[0].Type() == js.TypeNumber {
				frac = args[0].Float()
			}
			return r.SummaryJSON(frac)
		}),
		"path": js.FuncOf(func(js.Value, []js.Value) any {
			return r.PathJSON()
		}),
		"svg": js.FuncOf(func(js.Value, []js.Value) any {
			return r.SVG()
		}),
		"planSeq": js.FuncOf(func(js.Value, []js.Value) any {
			_, seq := p.Snapshot()
			return seq
		}),
		"plan": js.FuncOf(func(js.Value, []js.Value) any {
			svg, _ := p.Snapshot()
			return svg
		}),
	}
	js.Global().Set("shToolpath", js.ValueOf(api))
}
