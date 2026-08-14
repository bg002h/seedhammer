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
//	shToolpath.strings()        JSON census of what was ENGRAVED (see below)
//
// strings() is the §4.5 census: {"strings":[...], "announced":N,
// "unattributed":N}. strings holds the strings whose plates were cut AND
// accepted, in cut order -- those that passed through validateMdmk. An ms1 cut
// through the standalone codex32 flows is NOT among them and appears only in
// `unattributed`; see cmd/emu/engraved.go. Treat unattributed > 0 as "something
// was cut that this census cannot name".
//
// The two counts exist so an EMPTY census can be told apart from a BROKEN one -- announced=0 on a walk that reached an engrave
// screen means the hook is not wired, which otherwise reads exactly like "no
// plates were cut" and would pass a gate that tested nothing. It is cumulative
// for the session and has no reset: reload the page. See engraved.go for why
// hanging one on reset() would be a trap.
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
func installToolpathAPI(r *toolpathRecorder, p *platePlan, e *engravedRecorder) {
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
		"strings": js.FuncOf(func(js.Value, []js.Value) any {
			return e.StringsJSON()
		}),
	}
	js.Global().Set("shToolpath", js.ValueOf(api))
}
