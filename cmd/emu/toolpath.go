// This file deliberately carries NO build tag.
//
// Everything else in cmd/emu is //go:build js, which means it compiles only for
// the browser and can never be tested on this host. The decoder below is the
// load-bearing half of the toolpath recorder -- it reconstructs where the
// machine's head actually went from the bytes the driver would have DMA'd -- so
// it has to be reachable by `go test`. toolpath_js.go holds the parts that need
// syscall/js; toolpath_test.go round-trips this decoder against the REAL
// stepper.Driver encoder, which is what stops the mirrored bit layout below
// from drifting.

package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

// The wire format, mirrored from stepper/stepper.go. Those constants are
// unexported, so this is a copy -- and a copy of a bit layout is exactly the
// thing that rots silently. TestDecodeMatchesTheRealEncoder is the guard: it
// drives the real stepper.Driver and fails if any of these is wrong.
const (
	pinBits      = 5
	stepsPerWord = 32 / pinBits // 6
)

// Pin offsets from the base pin. Written out rather than derived from iota:
// this is a mirror of another file's layout, and the whole point is that it
// says the same numbers that file says.
const (
	pinDirY   = 0
	pinDirX   = 1
	pinNeedle = 2
	pinStepY  = 3
	pinStepX  = 4
)

// maxVertices bounds the recording so a full plate cannot exhaust the browser
// tab. Hitting it sets Truncated, and Summary reports it: a recorder that
// silently drops the end of the path would read as "the head stopped there",
// which is indistinguishable from the defect this exists to find.
const maxVertices = 200000

const (
	// turnWindow is how many moving steps the trailing direction window spans,
	// and turnTol is how far (in steps of chord-normalised cross product) the
	// window may depart from the run's chord before it counts as a corner.
	// Rasterisation alone stays within about 1; a real turn is far above 8.
	turnWindow = 64
	turnTol    = 8

	// maxRunSteps is a safety valve, not the corner detector. turned() handles
	// arcs, so this only bounds a pathologically long dead-straight run.
	maxRunSteps = 1 << 20
)

// Vertex is a corner of the reconstructed path. Needle is the state of the
// segment ENDING at this vertex, so a polyline of vertices with Needle=true is
// steel actually being cut.
type Vertex struct {
	X, Y   int  `json:"-"`
	Needle bool `json:"n"`
}

// MarshalJSON keeps the dump compact -- a plate has a lot of these.
func (v Vertex) MarshalJSON() ([]byte, error) {
	n := 0
	if v.Needle {
		n = 1
	}
	return []byte(fmt.Sprintf("[%d,%d,%d]", v.X, v.Y, n)), nil
}

// toolpathRecorder is a stepper.Writer that decodes the step stream instead of
// discarding it.
//
// WHY THIS EXISTS. cmd/emu's engraver threw the stream away, so the emulator
// could qualify every screen around engraving and nothing about the motion. That
// left one defect class unreachable without cutting a plate: F-107/F-108 zero
// seed-derived geometry, and zeroing it too EARLY does not leave a residue, it
// drives the head somewhere wrong. SafePointer.history is what the operator's
// hold-to-resume re-reads, so clearing it while a restart is still reachable
// sends the catch-up motion to the origin at T:0 and wrecks the plate.
//
// Decoding the driver's own output is stronger evidence than a plan preview:
// cmd/plateview renders from PlanEngraving, i.e. from what the firmware
// INTENDED, while this reconstructs what the DMA would have clocked out -- after
// the resume, after the catch-up, after any zeroing.
type toolpathRecorder struct {
	mu sync.Mutex

	x, y     int
	started  bool
	needle   bool
	vertices []Vertex

	// The current run: where it began, the per-axis sign it has committed to
	// (0 = that axis has not moved yet), and how long it has gone on.
	runX, runY   int
	runSx, runSy int
	runSteps     int
	// The trailing window, re-anchored every turnWindow moving steps. Its
	// direction is what gets compared against the run's chord.
	winX, winY int
	winSteps   int

	words     int
	steps     int
	cutSteps  int
	truncated bool

	minX, minY, maxX, maxY int
}

func newToolpathRecorder() *toolpathRecorder {
	r := &toolpathRecorder{}
	r.reset()
	return r
}

// Write consumes one DMA buffer. It must COPY nothing and retain nothing: the
// caller reuses its buffer across flushes (stepper.Driver.flush does
// `copy(d.buf, d.buf[n:])`), so holding the slice would alias a live buffer.
// Decoding immediately sidesteps that entirely.
func (r *toolpathRecorder) Write(steps []uint32) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, w := range steps {
		r.words++
		for i := 0; i < stepsPerWord; i++ {
			pins := uint8(w>>(i*pinBits)) & 0x1f
			// All-zero means halt. The encoder always sets at least one
			// direction pin for a real step, so a zero slot is padding in a
			// partially filled word -- never a movement worth plotting.
			if pins == 0 {
				continue
			}
			r.step(pins)
		}
	}
	return len(steps), nil
}

// step applies one 5-bit pin group.
//
// The direction pins are set when the movement is -1 OR 0, so a direction pin
// only means "negative" in company with its step pin. Reading dir alone would
// invent a movement on every idle axis.
func (r *toolpathRecorder) step(pins uint8) {
	dx, dy := 0, 0
	if pins&(1<<pinStepX) != 0 {
		if pins&(1<<pinDirX) != 0 {
			dx = -1
		} else {
			dx = 1
		}
	}
	if pins&(1<<pinStepY) != 0 {
		if pins&(1<<pinDirY) != 0 {
			dy = -1
		} else {
			dy = 1
		}
	}
	needle := pins&(1<<pinNeedle) != 0

	r.steps++
	if needle {
		r.cutSteps++
	}
	if !r.started {
		r.started = true
		r.startRun(r.x, r.y, needle)
	}

	// WHERE A VERTEX GOES. Two rules, and the history of both is worth keeping
	// because each wrong version produced a confident, wrong picture.
	//
	// 1. A change of NEEDLE always ends a run.
	// 2. A run continues while each axis keeps its sign. Motion is rasterised,
	//    so a straight line of any slope steps (1,0),(1,1),(0,1)... -- monotone
	//    in both axes -- while a corner reverses one of them. This detects
	//    corners exactly and costs two comparisons.
	//
	// The rejected version tested the perpendicular deviation of the PREVIOUS
	// point from the run's chord. That point is one step behind the chord's own
	// end, so it is always collinear with it: measured, a 40mm box decoded as a
	// smooth diagonal with the entire cut missing. Testing deviation properly
	// needs the max-deviation point of the whole run, which is not O(1).
	//
	// A monotone ARC therefore collapses to its chord, which is why the run is
	// also re-anchored every maxRunSteps: 1024 steps is 0.16mm, well inside the
	// 0.3mm stroke width, so a curve stays a curve in the SVG. This is a motion
	// comparator, not a renderer -- it is exact at corners and chord-accurate
	// between them.
	sx, sy := sign(dx), sign(dy)
	switch {
	case needle != r.needle:
		r.emit(r.x, r.y, r.needle)
		r.startRun(r.x, r.y, needle)
	case (sx != 0 && r.runSx != 0 && sx != r.runSx) ||
		(sy != 0 && r.runSy != 0 && sy != r.runSy),
		r.turned(),
		r.runSteps >= maxRunSteps:
		r.emit(r.x, r.y, r.needle)
		r.startRun(r.x, r.y, needle)
	}
	if sx != 0 && r.runSx == 0 {
		r.runSx = sx
	}
	if sy != 0 && r.runSy == 0 {
		r.runSy = sy
	}
	// DISTANCE, not ticks. A pin group can be nonzero with both step bits
	// clear -- the encoder always sets a direction pin, so a dwell tick during
	// acceleration looks like a step and is not one. Counting ticks re-anchored
	// the run every 1024 dwells and produced 5,010 vertices clustered in the
	// first few hundred microns of travel.
	if sx != 0 || sy != 0 {
		r.runSteps++
		r.winSteps++
		if r.winSteps >= turnWindow {
			r.winX, r.winY = r.x, r.y
			r.winSteps = 0
		}
	}

	r.x += dx
	r.y += dy
	r.minX, r.maxX = min(r.minX, r.x), max(r.maxX, r.x)
	r.minY, r.maxY = min(r.minY, r.y), max(r.maxY, r.y)
}

// turned reports whether the recent direction of travel has departed from the
// run's own chord.
//
// The sign rule alone cannot see a 90 degree corner: going +x and then +y never
// REVERSES an axis, it only stops one, so a box decoded as four chords across
// its own corners. This compares the trailing window's direction against the
// chord from the run's start, which separates the two cases cleanly --
// rasterisation noise leaves the cross product within about one step of the
// chord length, while a real turn is tens of times that.
func (r *toolpathRecorder) turned() bool {
	cx, cy := r.x-r.runX, r.y-r.runY
	wx, wy := r.x-r.winX, r.y-r.winY
	if (wx == 0 && wy == 0) || (cx == 0 && cy == 0) {
		return false
	}
	return abs(cx*wy-cy*wx) > turnTol*max(abs(cx), abs(cy))
}

func (r *toolpathRecorder) startRun(x, y int, needle bool) {
	r.runX, r.runY = x, y
	r.runSx, r.runSy = 0, 0
	r.runSteps = 0
	r.winX, r.winY = x, y
	r.winSteps = 0
	r.needle = needle
}

func sign(v int) int {
	switch {
	case v > 0:
		return 1
	case v < 0:
		return -1
	}
	return 0
}

// emit closes the current run at (x, y), which becomes a vertex of the path.
func (r *toolpathRecorder) emit(x, y int, needle bool) {
	if len(r.vertices) >= maxVertices {
		r.truncated = true
		return
	}
	// The needle is part of the identity. Dropping a same-coordinate vertex
	// unconditionally deleted the ENTIRE cut of a closed box: the figure ends
	// where it began, so the cut->travel vertex matched the travel->cut one
	// and was discarded, leaving a path that never engraved anything.
	if v := r.vertices[len(r.vertices)-1]; v.X == x && v.Y == y && v.Needle == needle {
		return // a zero-length run of the same kind is not a corner
	}
	r.vertices = append(r.vertices, Vertex{X: x, Y: y, Needle: needle})
}

func (r *toolpathRecorder) reset() {
	r.x, r.y = 0, 0
	r.started = false
	r.needle = false
	r.runX, r.runY = 0, 0
	r.runSx, r.runSy = 0, 0
	r.runSteps = 0
	r.vertices = r.vertices[:0]
	r.words, r.steps, r.cutSteps = 0, 0, 0
	r.truncated = false
	r.minX, r.minY, r.maxX, r.maxY = 0, 0, 0, 0
	// The path starts at the driver's origin, which is where bezier.Point's
	// zero value puts it.
	r.vertices = append(r.vertices, Vertex{})
}

// Reset clears the recording. Call it between the two runs of a comparison.
func (r *toolpathRecorder) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reset()
}

// Home records the origin-seeking move the machine performs at the start and
// the end of every job, and it is not cosmetic: it is what makes a second pass
// mean the same thing here as it does on the machine. F-121.
//
// cmd/controller wraps its engraver in a homingEngraver
// (cmd/controller/platform_sh2.go:589) that homes on the first write and again
// on Close, so the head is physically at the plate origin whenever a pass
// begins. stepper.Driver depends on that: Driver.pos starts as the zero
// bezier.Point and every step it emits is a delta from there, so a step stream
// MEANS "starting from the origin".
//
// This recorder deliberately spans every job (platform.go:185), so without
// this the second pass integrated an origin-relative stream from wherever the
// first one stopped. On the seed plate that is an offset of (68.1mm, 34.4mm)
// on an 85mm plate -- not a fidelity nit but a recording of a plate no machine
// cuts, which is the reverse of what the recorder is for.
//
// The travel is needle UP. It has to be: a needle-DOWN pass through the origin
// is the F-108 signature Summary.CutsThroughOrigin exists to catch, and homing
// that forged it would make the flag useless exactly on interrupted plates.
func (r *toolpathRecorder) Home() {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.x == 0 && r.y == 0 {
		// The device homes here too -- it cannot know it is already home -- but
		// a zero-length travel is not motion, and this runs twice per job on a
		// recording that spans all of them.
		//
		// The open run still has to be closed. path() appends the live position
		// as a trailing vertex while started is set, so clearing started without
		// emitting first DELETES the arrival: measured on a plan that closes at
		// the origin, homing removed the vertex where the cut got there.
		if r.started {
			r.emit(r.x, r.y, r.needle)
		}
		r.startRun(0, 0, false)
		r.started = false
		return
	}
	// Close the open run where the head stands, then the travel home. BOTH
	// vertices are needed: without the first, the polyline would leave from
	// wherever the last corner was rather than from the head; without the
	// second, the next pass's first segment would cut the corner and never
	// pass through the origin at all.
	r.emit(r.x, r.y, r.needle)
	r.x, r.y = 0, 0
	r.needle = false
	r.emit(0, 0, false)
	r.startRun(0, 0, false)
	// started=false leaves exactly the shape reset() leaves: the origin is in
	// the vertex list already, so path() must not append it a second time. The
	// next real step sets it again.
	r.started = false
}

// Path returns the vertices recorded so far, closed with the current position.
func (r *toolpathRecorder) Path() []Vertex {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.path()
}

func (r *toolpathRecorder) path() []Vertex {
	out := make([]Vertex, len(r.vertices), len(r.vertices)+1)
	copy(out, r.vertices)
	if r.started {
		out = append(out, Vertex{X: r.x, Y: r.y, Needle: r.needle})
	}
	return out
}

// jobRecorder is one engraving pass over a recorder: it performs the homing
// the machine performs, once per job.
//
// It lives HERE rather than in engraver.go because engraver.go is
// //go:build js and cannot be tested on this host at all. "Homes on the first
// write and on Close, and nowhere in between" is a small state machine whose
// two failure modes -- homing on every write, homing on none -- both look like
// a plausible plate until someone walks one in a browser.
type jobRecorder struct {
	rec *toolpathRecorder
	// homed is per-JOB. Platform.Engraver hands out a fresh engraver per pass,
	// exactly as cmd/controller does, so this arms once per pass the way
	// homingEngraver.homed does.
	homed bool
}

// Write homes before forwarding the first buffer of the job, as
// homingEngraver.Write does. What triggers it is the write happening, not what
// the write carries.
func (j *jobRecorder) Write(steps []uint32) (int, error) {
	if !j.homed {
		j.homed = true
		j.home()
	}
	if j.rec == nil {
		return len(steps), nil
	}
	return j.rec.Write(steps)
}

// Close homes, as homingEngraver.Close does.
func (j *jobRecorder) Close() { j.home() }

func (j *jobRecorder) home() {
	if j.rec != nil {
		j.rec.Home()
	}
}

// Summary is the machine-checkable digest of a run. It is what a comparison
// diffs, and what an assertion reads.
type Summary struct {
	Words     int  `json:"words"`
	Steps     int  `json:"steps"`
	CutSteps  int  `json:"cutSteps"`
	Vertices  int  `json:"vertices"`
	Truncated bool `json:"truncated"`

	EndX int `json:"endX"`
	EndY int `json:"endY"`

	Bounds [4]int `json:"bounds"` // minX, minY, maxX, maxY

	// Digest is order-sensitive over the whole vertex list. Two runs of the
	// same plate that agree here agree on the motion, which is the comparison
	// the abort->resume reading needs.
	Digest string `json:"digest"`

	// CutsThroughOrigin is a needle-DOWN pass through the machine origin.
	//
	// It is deliberately NOT "the path reaches the origin". Measured on
	// hardware 2026-08-10: EVERY legitimate resume tracks toward the origin,
	// because SafePointer.Resume synthesises its approach from there --
	// appendLine(move, conf, false, bezier.Point{}, s.safePoint)
	// (engrave/engrave.go:1664) interpolates in absolute coordinates from
	// (0,0). The first version of this flag fired on healthy runs.
	//
	// That approach is needle UP. Cutting through the origin is not something
	// any plate does.
	CutsThroughOrigin bool `json:"cutsThroughOrigin"`
	LongCuts          int  `json:"longCuts"`
	LongCutLimit      int  `json:"longCutLimit"`
}

// Summarize digests the recording. longCutFrac is the fraction of the bounding
// box's larger side above which a single needle-DOWN run is treated as a smear;
// pass 0 for the default of one quarter.
func (r *toolpathRecorder) Summarize(longCutFrac float64) Summary {
	r.mu.Lock()
	defer r.mu.Unlock()

	if longCutFrac <= 0 {
		longCutFrac = 0.25
	}
	vs := r.path()
	span := max(r.maxX-r.minX, r.maxY-r.minY)
	limit := int(float64(span) * longCutFrac)

	s := Summary{
		Words:     r.words,
		Steps:     r.steps,
		CutSteps:  r.cutSteps,
		Vertices:  len(vs),
		Truncated: r.truncated,
		EndX:      r.x,
		EndY:      r.y,
		Bounds:    [4]int{r.minX, r.minY, r.maxX, r.maxY},

		LongCutLimit: limit,
	}

	h := uint64(1469598103934665603) // FNV-1a 64 offset basis
	mix := func(v int) {
		for i := 0; i < 8; i++ {
			h ^= uint64(byte(v >> (i * 8)))
			h *= 1099511628211
		}
	}
	// A plate STARTS at the origin, so "is any vertex at (0,0)" would flag
	// every clean run. The head has to have gone somewhere first -- and the
	// pass has to be needle-down, since a resume legitimately travels back
	// through the origin with the needle up.
	away := max(span/10, 1)
	// The path is rasterised, so a vertex lands within a step or so of the
	// ideal coordinate. Exact equality against (0,0) misses the dive it is
	// looking for.
	const originTol = 4
	hasLeft := false
	for i, v := range vs {
		mix(v.X)
		mix(v.Y)
		if v.Needle {
			mix(1)
		} else {
			mix(0)
		}
		if max(abs(v.X), abs(v.Y)) > away {
			hasLeft = true
		}
		if i == 0 {
			continue
		}
		p := vs[i-1]
		if hasLeft && v.Needle && abs(v.X) <= originTol && abs(v.Y) <= originTol {
			s.CutsThroughOrigin = true
		}
		if v.Needle && limit > 0 {
			d := max(abs(v.X-p.X), abs(v.Y-p.Y))
			if d > limit {
				s.LongCuts++
			}
		}
	}
	s.Digest = fmt.Sprintf("%016x", h)
	return s
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// SummaryJSON is what the JS binding hands back.
func (r *toolpathRecorder) SummaryJSON(longCutFrac float64) string {
	b, err := json.Marshal(r.Summarize(longCutFrac))
	if err != nil {
		return `{"error":` + fmt.Sprintf("%q", err.Error()) + `}`
	}
	return string(b)
}

// PathJSON dumps every vertex as [x,y,needle], for diffing two runs offline.
func (r *toolpathRecorder) PathJSON() string {
	b, err := json.Marshal(r.Path())
	if err != nil {
		return `[]`
	}
	return string(b)
}

// SVG renders the reconstruction: travel moves thin and grey, cuts black at the
// production stroke width. It is a picture of the DECODED stream, so a wrong
// move is visible as a wrong line rather than inferred from a number.
func (r *toolpathRecorder) SVG() string {
	vs := r.Path()
	r.mu.Lock()
	minX, minY, maxX, maxY := r.minX, r.minY, r.maxX, r.maxY
	r.mu.Unlock()

	// Always include the origin: a stray move to (0,0) is the failure this is
	// looking for, and a viewBox fitted to the cut would crop it out.
	minX, minY = min(minX, 0), min(minY, 0)
	maxX, maxY = max(maxX, 0), max(maxY, 0)
	w, h := max(maxX-minX, 1), max(maxY-minY, 1)
	pad := max(w, h)/20 + 1

	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="%d %d %d %d" width="800">`,
		minX-pad, minY-pad, w+2*pad, h+2*pad)
	fmt.Fprintf(&b, `<rect x="%d" y="%d" width="%d" height="%d" fill="white"/>`,
		minX-pad, minY-pad, w+2*pad, h+2*pad)

	sw := max(w, h)/400 + 1
	var cuts, travels strings.Builder
	for i := 1; i < len(vs); i++ {
		p, v := vs[i-1], vs[i]
		seg := &travels
		if v.Needle {
			seg = &cuts
		}
		fmt.Fprintf(seg, "M%d %dL%d %d", p.X, p.Y, v.X, v.Y)
	}
	fmt.Fprintf(&b, `<path d="%s" fill="none" stroke="#bbb" stroke-width="%d"/>`, travels.String(), sw)
	fmt.Fprintf(&b, `<path d="%s" fill="none" stroke="#000" stroke-width="%d" stroke-linecap="round"/>`, cuts.String(), sw*3)
	// The origin, marked, for the same reason it is kept in the viewBox.
	fmt.Fprintf(&b, `<circle cx="0" cy="0" r="%d" fill="none" stroke="red" stroke-width="%d"/>`, sw*4, sw)
	b.WriteString(`</svg>`)
	return b.String()
}
