//go:build js

package main

import "syscall/js"

// installToolpathAPI exposes the recorder to the page as window.shToolpath.
//
// It is a JS API rather than a UI because the reading it serves is a
// COMPARISON: cut once uninterrupted, cut again with an abort and a
// hold-to-resume, and check the two runs describe the same motion. A human
// cannot eyeball that; a digest settles it.
//
//	shToolpath.reset()          start a fresh recording
//	shToolpath.summary()        JSON digest + anomalies (see Summary)
//	shToolpath.path()           JSON [[x,y,needle],...]
//	shToolpath.svg()            an SVG of the decoded motion
//
// Typical session, in the console:
//
//	shToolpath.reset(); /* cut the plate straight through */
//	a = JSON.parse(shToolpath.summary())
//	shToolpath.reset(); /* cut again, Back mid-cut, hold to resume */
//	b = JSON.parse(shToolpath.summary())
//	a.digest === b.digest   // the resumed plate follows the same path
//	b.returnsToOrigin       // must be false: a dive home is the F-108 failure
func installToolpathAPI(r *toolpathRecorder) {
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
	}
	js.Global().Set("shToolpath", js.ValueOf(api))
}
