// This file deliberately carries NO build tag, for the reason toolpath.go
// gives: everything else that draws the overlay is //go:build js and can never
// be tested on this host. What is here is the half that can be -- the plan's
// geometry and the small amount of state around it -- so plate_test.go can
// check that the plan and the recording land in the same coordinate space,
// which is the one property the overlay cannot be looked at to verify.
//
// WHAT THE OVERLAY IS. cmd/emu could show every screen around engraving and
// nothing about the plate: the operator watched a countdown and found out what
// the steel said when it was finished. The plan is available in full before the
// first step is emitted -- gui's engraveJob has held it all along -- so the
// whole layout can be drawn at the moment the cut begins, with the toolpath
// recorder's decoded motion marking progress on top of it.
//
// The two halves register EXACTLY, and not by arrangement: the plan's control
// points and the recorder's integrated step deltas are both in microsteps from
// the machine origin. See TestPlanAndRecordingShareOneCoordinateSpace.

package main

import (
	"fmt"
	"strings"
	"sync"

	"seedhammer.com/bspline"
	"seedhammer.com/engrave"
	"seedhammer.com/gui"
)

// The elements the page fills in. They are ids rather than classes because the
// page addresses exactly one of each, and it sets attributes on them rather
// than rebuilding the SVG -- redrawing a 600KB path four times a second to
// extend a polyline would spend the whole frame budget on the part that did
// not change.
const (
	cutPathID    = "sh-cut"
	headMarkerID = "sh-head"
)

// maxPlanKnots bounds the layout render, for the reason maxVertices bounds the
// recording: a plan is an ITERATOR, and nothing about it is obliged to end.
// gui/qa.go's qaPlan is literally `for { ... }` -- the engrave loop just stops
// asking -- so a renderer that ranges to completion before the first step would
// never hand the tab back.
//
// The widest real plate measured through gui.BuildPreview is constproof at
// 27,062 knots, so this is under 4x headroom on anything a plate can be. A plan
// that reaches it is reported truncated rather than presented as whole.
const maxPlanKnots = 100000

// planPath renders a plan as the d attribute of an SVG path, in raw microstep
// coordinates.
//
// It emits a cubic per engraving segment and a move per travel, so what it
// draws is the CUT and not the head's wandering -- the same choice
// internal/golden.Vectorize makes, whose output this matches byte for byte
// (TestPlanPathMatchesVectorize). It is not simply called, because Vectorize
// writes a whole SVG document and the overlay needs the path data to place
// inside a live one; cmd/plateview gets at it by indexing into that document
// and splicing, and one copy of that assumption is enough.
func planPath(spline bspline.Curve) (d string, truncated bool) {
	var b strings.Builder
	var seg bspline.Segment
	knots := 0
	for k := range spline {
		if knots >= maxPlanKnots {
			truncated = true
			break
		}
		knots++
		c, dt, line := seg.Knot(k)
		// A zero-duration knot is the spline's own padding, not a segment.
		if dt == 0 {
			continue
		}
		if line {
			fmt.Fprintf(&b, " C %d %d, %d %d, %d %d",
				c.C1.X, c.C1.Y, c.C2.X, c.C2.Y, c.C3.X, c.C3.Y)
		} else {
			fmt.Fprintf(&b, " M %d %d", c.C3.X, c.C3.Y)
		}
	}
	return b.String(), truncated
}

// planSVG draws the plate and the plan on it, with empty elements for the page
// to fill with progress.
//
// The viewBox is the PLATE, never the engraving's bounds. Fitting it to the ink
// would rescale between plates, so a passphrase plate and a full seed plate
// would show strokes at different apparent widths and nothing would reveal that
// one uses more of the steel than the other. The stroke is the production cut
// width, so a glyph that looks thin here is thin on the metal.
//
// No screw-hole keep-out bands: they are backup's innerMargin, which is
// unexported, and cmd/plateview already keeps one copy of that number. A third
// would be a third thing to notice when it changes.
func planSVG(spline bspline.Curve, params engrave.Params) string {
	dims := gui.SquarePlate.Dims(params.Millimeter)
	sw := params.StrokeWidth
	d, truncated := planPath(spline)

	// data-truncated is how the page learns the layout is partial. It is an
	// attribute rather than drawn text because the page already owns the
	// caption, and a plan that renders a warning INTO the plate would put a
	// mark on the steel that is not being cut.
	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" `+
		`preserveAspectRatio="xMidYMid meet" data-truncated="%t">`, dims.X, dims.Y, truncated)
	fmt.Fprintf(&b, `<rect x="0" y="0" width="%d" height="%d" rx="%d" fill="#c9c9c3"/>`,
		dims.X, dims.Y, 2*params.Millimeter)
	// The plan, muted: it is what WILL be cut, and it must not compete with
	// what has been.
	fmt.Fprintf(&b, `<path d="%s" fill="none" stroke="#9c9c96" stroke-width="%d" `+
		`stroke-linejoin="round" stroke-linecap="round"/>`, d, sw)
	// What has been cut, and where the needle is. Both empty until the page
	// fills them; an overlay that pre-drew progress would be showing a plate
	// that has not been cut yet.
	fmt.Fprintf(&b, `<path id="%s" d="" fill="none" stroke="#16161a" stroke-width="%d" `+
		`stroke-linejoin="round" stroke-linecap="round"/>`, cutPathID, sw)
	fmt.Fprintf(&b, `<circle id="%s" cx="0" cy="0" r="%d" fill="none" stroke="#e2593f" `+
		`stroke-width="%d" visibility="hidden"/>`, headMarkerID, 8*sw, 2*sw)
	b.WriteString(`</svg>`)
	return b.String()
}

// platePlan is the plan the page is currently drawing, and a sequence number
// that changes only when the plate does.
type platePlan struct {
	mu  sync.Mutex
	svg string
	seq int
}

// Set renders spline as the current plate and reports whether it is a NEW one.
//
// A hold-to-resume is a fresh engraving job over the same spline, so it arrives
// here exactly as a new plate does and only the caller can tell them apart.
// Re-rendering a resume would merely be wasted work; bumping the sequence would
// not, because the page reads a new sequence as a new plate and clears the
// progress it has drawn -- so a plate the operator is half way through would
// appear to restart itself at the moment they resumed it.
func (p *platePlan) Set(spline bspline.Curve, params engrave.Params, resumed bool) bool {
	if resumed {
		return false
	}
	svg := planSVG(spline, params)

	p.mu.Lock()
	defer p.mu.Unlock()
	p.svg = svg
	p.seq++
	return true
}

// Snapshot returns the current plan and its sequence.
func (p *platePlan) Snapshot() (string, int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.svg, p.seq
}

// beginPlate installs the plan for a job and prepares the recording for it.
//
// The RECORDING restarts for a new plate; the recorder does not. platform.go
// keeps one recorder across every job deliberately, so that an aborted and
// resumed plate records as one motion, and re-scoping it per plate would take
// that away. Clearing the recording at the start of a genuinely new plate
// leaves that intact and stops the previous plate's cut from being drawn as
// progress on this one's plan.
func beginPlate(p *platePlan, rec *toolpathRecorder, spline bspline.Curve, params engrave.Params, resumed bool) {
	if !p.Set(spline, params, resumed) {
		return
	}
	if rec != nil {
		rec.Reset()
	}
}
