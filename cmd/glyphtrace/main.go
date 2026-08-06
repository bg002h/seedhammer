// Command glyphtrace renders the ENGRAVING TRACE of individual glyphs into one
// image, for judging a glyph before it is cut into steel.
//
// It is a picture of the TOOLPATH, not of constant.svg: the geometry comes from
// engrave.PlanEngraving, the same planner the device runs, so what is drawn is
// what the machine would move. Each cell carries three things a font editor
// does not show:
//
//   - the INK, stroked at the real 0.3mm width against the glyph's real size at
//     the chosen rung. Ink that closes a counter here closes it on the plate.
//   - the CENTERLINE, so the path is visible inside a stroke wide enough to
//     hide it.
//   - the TRAVEL moves, dashed, between one stroke and the next. Their count is
//     k-1 for a k-stroke glyph, which is the quantity the disclosure bound
//     T_row = rowLen + n_row is stated over -- so a glyph that quietly became
//     three strokes is visible here as two dashes.
//
// Usage:
//
//	go run ./cmd/glyphtrace -o /tmp/glyphs.png
//	go run ./cmd/glyphtrace -glyphs 'aeso' -size 5.0 -face sh -o /tmp/x.png
package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"seedhammer.com/bezier"
	"seedhammer.com/bspline"
	"seedhammer.com/engrave"
	"seedhammer.com/font/constant"
	"seedhammer.com/font/sh"
	"seedhammer.com/font/vector"
	"seedhammer.com/internal/sh2"
)

// problemGlyphs is the set under review (operator, 2026-08-05): the closed
// counters that fill in, the strokes that lose their identity, and the six
// brackets that have to stay distinguishable from each other.
const problemGlyphs = "aeszOo8@*&<>(){}"

// glyphNames disambiguates a caption where the character alone does not.
// 'O' against 'o' and '(' against '{' are exactly the pairs the sheet is being
// read for, and at caption size the two are no easier to tell apart than the
// engraved glyphs are.
//
// The character itself always leads, so the caption for '<' contains a literal
// '<'. That is deliberate: it is what keeps esc below on the live path rather
// than defensive, and three of the sixteen glyphs under review are the three
// characters XML reserves.
var glyphNames = map[rune]string{
	'O': "cap", 'o': "low", '@': "at", '*': "star", '&': "amp",
	'<': "lt", '>': "gt",
	'(': "lparen", ')': "rparen", '{': "lbrace", '}': "rbrace",
}

func main() {
	var (
		glyphs   = flag.String("glyphs", problemGlyphs, "the glyphs to draw, as a string")
		faceArg  = flag.String("face", "const", "engraving face: const or sh")
		sizeMM   = flag.Float64("size", 3.0, "the rung to draw at, in mm; 3.0 is the ladder's smallest")
		cols     = flag.Int("cols", 4, "cells per row")
		px       = flag.Int("px", 2000, "PNG width in pixels")
		out      = flag.String("o", "glyphs.png", "output file; .png converts via rsvg-convert")
		counters = flag.Bool("counters", false, "print the counter table for the whole face and exit")
		word     = flag.String("word", "", "render this string as one engraved line instead of a glyph grid")
		label    = flag.String("label", "", "banner drawn across the top, e.g. \"OPTION 2 - bottom bar angled down\"")
	)
	flag.Parse()

	face, faceName, err := faceFor(*faceArg)
	if err != nil {
		fail(err)
	}
	if *counters {
		counterTable(os.Stdout, face, faceName, float32(*sizeMM))
		return
	}
	var svg []byte
	if *word != "" {
		svg, err = renderWord(face, faceName, *word, float32(*sizeMM))
	} else {
		svg, err = render(face, faceName, []rune(*glyphs), float32(*sizeMM), *cols)
	}
	if err != nil {
		fail(err)
	}
	svg = banner(svg, *label)
	if strings.EqualFold(filepath.Ext(*out), ".png") {
		if err := writePNG(*out, svg, *px); err != nil {
			fail(err)
		}
	} else if err := os.WriteFile(*out, svg, 0o644); err != nil {
		fail(err)
	}
	fmt.Fprintf(os.Stderr, "glyphtrace: %d glyphs of font/%s at %.1fmm -> %s\n",
		len([]rune(*glyphs)), faceName, *sizeMM, *out)
}

func fail(err error) {
	fmt.Fprintf(os.Stderr, "glyphtrace: %v\n", err)
	os.Exit(1)
}

// minFeature is the rule the counter table is graded against: a feature must
// clear TWO stroke widths centre to centre, which leaves one stroke width of
// bare metal at the tightest point. It is the house rule already applied by
// hand in font/constant's glyph_rules_test.go; here it is applied to every
// glyph instead of to a written list.
func minFeature(sw int) int { return 2 * sw }

// counterTable grades every glyph the face can cut: the enclosed counters it
// holds, and the narrowest CHANNEL between two of its strokes.
//
// Two different hazards, and the second is the one the first is blind to. A
// counter is enclosed and a flood fill cannot reach it; a channel is the gap
// between two roughly parallel strokes and is usually open at one end -- the
// space between an 'e's top bar and its bottom bar, which no counter metric
// ever sees. Below some floor separation a channel stops reading as a gap and
// the two strokes read as one thick line (operator's principle, 2026-08-05).
//
// Each glyph is also measured at 6.0mm, because the failure that matters is not
// a gap that shrank -- they all shrink, the stroke is 0.30mm at every rung while
// the glyph scales -- but one that stopped existing.
func counterTable(w io.Writer, face *vector.Face, faceName string, sizeMM float32) {
	P := sh2.Params()
	sw := P.StrokeWidth
	swMM := float64(sw) / float64(P.Millimeter)
	// The floor, pending a verdict from steel: two stroke widths of bare metal.
	// It is the same number the house minimum-feature rule uses, applied to the
	// gap between strokes rather than to a feature's own size.
	floorMM := 2 * swMM

	type row struct {
		g        glyph
		lost     int
		tight    float64
		hasTight bool
		ch       channel
		hasCh    bool
	}
	var rows []row
	for r := rune(0); r < 0x2000; r++ {
		if _, _, ok := face.Decode(r); !ok {
			continue
		}
		g := trace(face, r, P.F(sizeMM), P.StepperConfig)
		if !g.hasInk {
			continue
		}
		ras := rasterize(g.runs, sw, P.Millimeter)
		g.counters = ras.findCounters(P.Millimeter)
		ch, hasCh := worstChannel(ras.findChannels(), floorMM)

		ref := trace(face, r, P.F(6.0), P.StepperConfig)
		refc := rasterize(ref.runs, sw, P.Millimeter).findCounters(P.Millimeter)
		t, ok := g.tightest()
		rows = append(rows, row{
			g: g, lost: max(len(refc)-len(g.counters), 0),
			tight: t, hasTight: ok, ch: ch, hasCh: hasCh,
		})
	}
	// Longest run under the floor first: that is the ranking the opening-up
	// work follows, because it is how far the glyph reads as one thick line.
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].hasCh != rows[j].hasCh {
			return rows[i].hasCh
		}
		return rows[i].ch.SustainedMax(sustainMM) < rows[j].ch.SustainedMax(sustainMM)
	})

	fmt.Fprintf(w, "font/%s at %.1fmm, stroke %.2fmm.\n", faceName, sizeMM, swMM)
	fmt.Fprintf(w, "channel = the narrowest white gap between two strokes, and how far it runs at that width.\n")
	fmt.Fprintf(w, "counter = enclosed bare metal; 'lost' counts counters present at 6.0mm and gone here.\n\n")
	fmt.Fprintf(w, "the FLOOR is %.2fmm (%.0f strokes): under it, two parallel strokes read as one thick line.\n\n",
		floorMM, floorMM/swMM)
	fmt.Fprintf(w, "widest = the widest the gap gets and HOLDS; the eye takes that and projects it across.\n\n")
	fmt.Fprintf(w, "%-9s %-3s %-8s %-9s %-9s %-9s %-9s %s\n",
		"glyph", "k", "counters", "counter", "widest", "median", "runs", "verdict")

	for _, rw := range rows {
		g := rw.g
		tight, wide, med, runs := "    -   ", "    -   ", "    -   ", "    -   "
		if rw.hasTight {
			tight = fmt.Sprintf("%6.3fmm", rw.tight)
		}
		widest := rw.ch.SustainedMax(sustainMM)
		if rw.hasCh {
			wide = fmt.Sprintf("%6.3fmm", widest)
			med = fmt.Sprintf("%6.3fmm", rw.ch.Median())
			runs = fmt.Sprintf("%6.2fmm", rw.ch.RunMM)
		}
		verdict := "ok"
		if rw.hasCh {
			switch {
			case widest < floorMM/2:
				verdict = "MERGES: never opens past half the floor"
			case widest < floorMM:
				verdict = "tight: never reaches the floor"
			}
		}
		if rw.lost > 0 {
			verdict = fmt.Sprintf("CLOSED: %d counter(s) filled in", rw.lost)
		}
		fmt.Fprintf(w, "%-9s %-3d %-8d %-9s %-9s %-9s %-9s %s\n",
			caption(g.r), g.strokes, len(g.counters), tight, wide, med, runs, verdict)
	}
}

func faceFor(name string) (*vector.Face, string, error) {
	switch name {
	case "const", "constant":
		return constant.Font, "constant", nil
	case "sh":
		return sh.Font, "sh", nil
	}
	return nil, "", fmt.Errorf("unknown face %q (want const or sh)", name)
}

// glyph is one cell's worth of measured geometry.
type glyph struct {
	r        rune
	ink      string // the engraved path, as SVG path data
	travel   string // the lifts between strokes
	ctrl     []bezier.Point
	starts   []bezier.Point // where each run begins, so stroke order is readable
	strokes  int            // k: how many separate runs the tool cuts
	advance  int            // the glyph's cell width in device units
	bounds   bspline.Bounds
	hasInk   bool
	unmapped bool

	// runs is the flattened centreline, kept so the ink can be rasterised and
	// its counters measured without re-planning. See counters.go.
	runs [][]bezier.Point
	// counters is the enclosed bare metal, widest first. Empty means the glyph
	// encloses nothing at this rung -- which is correct for 'l' and a finding
	// for 'o'.
	counters []counter
}

// tightest is the narrowest counter the glyph still has, in mm, and whether it
// has one at all.
func (g glyph) tightest() (float64, bool) {
	if len(g.counters) == 0 {
		return 0, false
	}
	return g.counters[len(g.counters)-1].WidthMM, true
}

// trace plans one glyph exactly as the device would and splits the result into
// the segments that CUT and the moves that do not.
//
// The split is the whole point: bspline.Segment.Knot reports `line` false for a
// travel, and Vectorize throws those away because a golden only cares about
// ink. Here they are drawn, because how many times the tool lifts inside one
// glyph is a property worth seeing.
func trace(face *vector.Face, r rune, em int, conf engrave.StepperConfig) glyph {
	return traceString(face, string(r), em, conf, r)
}

// traceString plans any string -- one glyph, or a whole word for judging a
// change in context. A cell shows what a glyph IS; a word shows whether it
// sits on the baseline with its neighbours, which is the question a bar tilted
// below the baseline actually raises.
func traceString(face *vector.Face, txt string, em int, conf engrave.StepperConfig, r rune) glyph {
	g := glyph{r: r}
	for _, c := range txt {
		if _, _, ok := face.Decode(c); !ok {
			g.unmapped = true
			return g
		}
	}
	for _, c := range txt {
		adv, _, _ := face.Decode(c)
		g.advance += adv * em / face.Metrics().Height
	}

	plan := func(yield func(engrave.Command) bool) {
		engrave.String(face, em, txt).Engrave(yield)
	}
	spline := engrave.PlanEngraving(conf, plan)

	var ink, travel strings.Builder
	var seg bspline.Segment
	var last bezier.Point
	inRun := false
	for k := range spline {
		c, dt, line := seg.Knot(k)
		if dt == 0 {
			continue
		}
		if line {
			// EVERY run opens with an M. A path beginning with a C is invalid
			// and rsvg-convert drops the whole element without a word, which
			// renders as a glyph that cuts nothing -- the exact reading this
			// tool exists to give, arrived at by a bug rather than by the font.
			if !inRun {
				fmt.Fprintf(&ink, " M %d %d", c.C0.X, c.C0.Y)
				g.starts = append(g.starts, c.C0)
				g.ctrl = append(g.ctrl, c.C0)
				g.strokes++
				inRun = true
			}
			fmt.Fprintf(&ink, " C %d %d, %d %d, %d %d",
				c.C1.X, c.C1.Y, c.C2.X, c.C2.Y, c.C3.X, c.C3.Y)
			// The knot's own control point, which is the thing edited in
			// constant.svg -- NOT c.C3, the curve's endpoint. On a B-spline
			// those are different points, and it is the control point that
			// moves when a `points="..."` coordinate is changed.
			g.ctrl = append(g.ctrl, k.Ctrl)
			g.hasInk = true
		} else {
			// A lift BETWEEN runs, drawn from where the tool was. The approach
			// move that precedes the first run is not one of these: it comes
			// from wherever the previous glyph ended, so on a single-glyph
			// drawing it is an artifact of the crop rather than the glyph's.
			if inRun {
				fmt.Fprintf(&travel, " M %d %d L %d %d", last.X, last.Y, c.C3.X, c.C3.Y)
			}
			inRun = false
		}
		last = c.C3
	}
	g.ink, g.travel = ink.String(), travel.String()
	if g.hasInk {
		g.bounds = bspline.Measure(engrave.PlanEngraving(conf, plan)).Bounds
		g.runs = flatten(engrave.PlanEngraving(conf, plan))
	}
	return g
}

// flattenSteps is how finely each cubic is sampled before distances are taken.
// The glyphs are polylines in practice -- C1 and C2 sit on the line -- so this
// is mostly about not missing the inside of a corner.
const flattenSteps = 16

// flatten samples the ENGRAVED segments into polylines, one per run. Travels
// are excluded: the tool is off the work, so the distance across a lift is not
// a gap in the glyph.
func flatten(spline bspline.Curve) [][]bezier.Point {
	var runs [][]bezier.Point
	var cur []bezier.Point
	var seg bspline.Segment
	for k := range spline {
		c, dt, line := seg.Knot(k)
		if dt == 0 {
			continue
		}
		if !line {
			if len(cur) > 1 {
				runs = append(runs, cur)
			}
			cur = nil
			continue
		}
		if len(cur) == 0 {
			cur = append(cur, c.C0)
		}
		for i := 1; i <= flattenSteps; i++ {
			t := float64(i) / flattenSteps
			cur = append(cur, cubicAt(c, t))
		}
	}
	if len(cur) > 1 {
		runs = append(runs, cur)
	}
	return runs
}

func cubicAt(c bezier.Cubic, t float64) bezier.Point {
	u := 1 - t
	a, b, d, e := u*u*u, 3*u*u*t, 3*u*t*t, t*t*t
	return bezier.Point{
		X: int(a*float64(c.C0.X) + b*float64(c.C1.X) + d*float64(c.C2.X) + e*float64(c.C3.X)),
		Y: int(a*float64(c.C0.Y) + b*float64(c.C1.Y) + d*float64(c.C2.Y) + e*float64(c.C3.Y)),
	}
}

func dist(a, b bezier.Point) float64 {
	dx, dy := float64(a.X-b.X), float64(a.Y-b.Y)
	return math.Sqrt(dx*dx + dy*dy)
}

func render(face *vector.Face, faceName string, runes []rune, sizeMM float32, cols int) ([]byte, error) {
	P := sh2.Params()
	em := P.F(sizeMM)
	sw := P.StrokeWidth

	glyphs := make([]glyph, 0, len(runes))
	for _, r := range runes {
		glyphs = append(glyphs, trace(face, r, em, P.StepperConfig))
	}

	// The cell is sized from the EM, so every glyph is drawn at the same scale
	// and the picture answers "which of these is denser" rather than only
	// "what shape is this".
	const (
		padF   = 0.45 // cell padding, in ems
		labelF = 0.95 // room under the glyph for its two caption lines, in ems
		capF   = 0.19 // caption text size, in ems
		hdF    = 0.30 // header text size, in ems
	)
	pad := int(padF * float64(em))
	label := int(labelF * float64(em))
	capPx := int(capF * float64(em))
	cellW, cellH := em*2+2*pad, em+label+2*pad
	rows := (len(glyphs) + cols - 1) / cols
	head := int(1.6 * float64(em))
	w, h := cols*cellW, rows*cellH+head

	// The header is SIZED TO FIT rather than set to a fixed fraction of the em.
	// At 0.30em the legend ran off the right edge and rsvg-convert clipped it
	// without complaint -- a caption that silently loses its second half is
	// worse on a reference image than no caption.
	const legendChars = 104 // the longer of the two lines below, rounded up
	hdPx := min(int(hdF*float64(em)), (w-2*pad)*10/(legendChars*6))

	var b bytes.Buffer
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" width="%d" height="%d">`,
		w, h, w, h)
	fmt.Fprintf(&b, `<rect width="%d" height="%d" fill="#fff"/>`, w, h)
	// The ink is the metal REMOVED, so it is drawn as a pale slab and the
	// skeleton rides on top of it. Drawn the other way round -- black ink over
	// a thin line -- a 0.3mm stroke at 3.0mm swallows the path completely,
	// which is the whole difficulty of judging one of these glyphs.
	fmt.Fprintf(&b, `<style>
	.ink    { fill:none; stroke:#d9d9d9; stroke-width:%d; stroke-linejoin:round; stroke-linecap:round; }
	.mid    { fill:none; stroke:#0984e3; stroke-width:%d; stroke-linecap:round; }
	.travel { fill:none; stroke:#9aa0a6; stroke-width:%d; stroke-dasharray:%d %d; }
	.box    { fill:none; stroke:#d0d0d0; stroke-width:%d; }
	.cap    { font-family:monospace; font-size:%dpx; fill:#222; }
	.sub    { font-family:monospace; font-size:%dpx; fill:#777; }
	.hd     { font-family:sans-serif; font-size:%dpx; fill:#000; }
	.warn   { fill:#c00000; }
</style>`, sw, max(em/70, 1), max(em/110, 1), em/22, em/22, max(sw/12, 1),
		capPx, capPx, hdPx)

	l1 := fmt.Sprintf("font/%s at %.1fmm — grey slab = ink at the real %.1fmm stroke, blue = centreline",
		faceName, sizeMM, float64(sw)/float64(P.Millimeter))
	l2 := "red dots = the control points you edit in the SVG · green ring = where a stroke starts · " +
		"k = strokes · thin box = the advance cell"
	fmt.Fprintf(&b, `<text class="hd" x="%d" y="%d">%s</text>`, pad, int(0.55*float64(em)), esc(l1))
	fmt.Fprintf(&b, `<text class="hd" x="%d" y="%d">%s</text>`, pad, int(1.05*float64(em)), esc(l2))

	for i, g := range glyphs {
		cx, cy := (i%cols)*cellW, head+(i/cols)*cellH
		fmt.Fprintf(&b, `<g transform="translate(%d,%d)">`, cx, cy)

		// The advance cell, so the glyph is seen inside the box it must not
		// leave -- and so its density against its neighbours is visible.
		fmt.Fprintf(&b, `<rect class="box" x="%d" y="%d" width="%d" height="%d"/>`,
			pad, pad, g.advance, em)

		capY := pad + em + int(0.42*float64(em))
		if g.unmapped {
			fmt.Fprintf(&b, `<text class="cap warn" x="%d" y="%d">%s: NOT IN FACE</text>`,
				pad, capY, esc(caption(g.r)))
			fmt.Fprint(&b, `</g>`)
			continue
		}
		// engrave.String lays the glyph out from the origin, so the spline is
		// already in the cell's own coordinates and only the padding shifts it.
		fmt.Fprintf(&b, `<g transform="translate(%d,%d)">`, pad, pad)
		if g.ink != "" {
			fmt.Fprintf(&b, `<path class="ink" d="%s"/>`, g.ink)
			fmt.Fprintf(&b, `<path class="mid" d="%s"/>`, g.ink)
		}
		if g.travel != "" {
			fmt.Fprintf(&b, `<path class="travel" d="%s"/>`, g.travel)
		}
		dot := max(em/55, 1)
		for _, p := range g.ctrl {
			fmt.Fprintf(&b, `<circle cx="%d" cy="%d" r="%d" fill="#d81b1b"/>`, p.X, p.Y, dot)
		}
		for _, p := range g.starts {
			fmt.Fprintf(&b, `<circle cx="%d" cy="%d" r="%d" fill="none" stroke="#0a8f3c" stroke-width="%d"/>`,
				p.X, p.Y, 2*dot, max(dot/2, 1))
		}
		fmt.Fprint(&b, `</g>`)

		// Two lines, because one does not fit the cell: the glyph and its
		// stroke count, then the ink's own extent. Those are the numbers a
		// legibility judgement is actually made on -- a counter's height in mm
		// against the 0.3mm the tool lays down on each side of it.
		fmt.Fprintf(&b, `<text class="cap" x="%d" y="%d">%s   k=%d</text>`,
			pad, capY, esc(caption(g.r)), g.strokes)
		sub := "no ink"
		if g.hasInk {
			sub = fmt.Sprintf("%.2f x %.2f mm",
				float64(g.bounds.Dx())/float64(P.Millimeter),
				float64(g.bounds.Dy())/float64(P.Millimeter))
		}
		fmt.Fprintf(&b, `<text class="sub" x="%d" y="%d">%s</text>`,
			pad, capY+int(0.30*float64(em)), sub)
		fmt.Fprint(&b, `</g>`)
	}
	fmt.Fprint(&b, `</svg>`)
	return b.Bytes(), nil
}

func caption(r rune) string {
	if n, ok := glyphNames[r]; ok {
		return string(r) + " " + n
	}
	return string(r)
}

// esc is the minimum XML text escaping. '<', '>' and '&' are three of the
// glyphs under review, so a caption that skipped this would produce an SVG that
// does not parse -- for exactly the characters most in need of looking at.
func esc(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s)
}

func writePNG(out string, svg []byte, px int) error {
	if _, err := exec.LookPath("rsvg-convert"); err != nil {
		return fmt.Errorf("PNG output needs rsvg-convert on PATH (%v); write .svg instead", err)
	}
	cmd := exec.Command("rsvg-convert", "-w", fmt.Sprint(px), "-b", "white", "-o", out)
	cmd.Stdin = bytes.NewReader(svg)
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// renderWord draws a string as the machine would cut it, with the BASELINE
// drawn in.
//
// A glyph cell cannot answer the question a tilted bottom bar raises. Dropping
// the free end of an 'e' below the baseline opens its channel and also makes
// the glyph a third of a millimetre taller -- and whether that reads as a
// deliberate slant or as a letter sinking out of the line is not visible until
// the letter has neighbours to sit beside.
func renderWord(face *vector.Face, faceName, txt string, sizeMM float32) ([]byte, error) {
	P := sh2.Params()
	em, sw := P.F(sizeMM), P.StrokeWidth
	g := traceString(face, txt, em, P.StepperConfig, ' ')
	if g.unmapped {
		return nil, fmt.Errorf("the face cannot cut every character of %q", txt)
	}
	// The baseline, from the face's OWN metrics rather than from the ink: a
	// word whose letters have all sunk would put the line wherever they sank
	// to, and draw a baseline that agrees with the defect.
	m := face.Metrics()
	base := em * m.Ascent / m.Height
	pad := em
	w, h := g.advance+2*pad, em*2+2*pad

	var b bytes.Buffer
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" width="%d" height="%d">`, w, h, w, h)
	fmt.Fprintf(&b, `<rect width="%d" height="%d" fill="#fff"/>`, w, h)
	fmt.Fprintf(&b, `<style>
	.ink  { fill:none; stroke:#d9d9d9; stroke-width:%d; stroke-linejoin:round; stroke-linecap:round; }
	.mid  { fill:none; stroke:#0984e3; stroke-width:%d; stroke-linecap:round; }
	.base { stroke:#e05a5a; stroke-width:%d; stroke-dasharray:%d %d; }
	.hd   { font-family:sans-serif; font-size:%dpx; fill:#000; }
</style>`, sw, max(em/90, 1), max(em/120, 1), em/14, em/14, int(0.30*float64(em)))
	fmt.Fprintf(&b, `<text class="hd" x="%d" y="%d">font/%s at %.1fmm &#183; red dashes = the baseline</text>`,
		pad, int(0.5*float64(em)), faceName, sizeMM)
	fmt.Fprintf(&b, `<g transform="translate(%d,%d)">`, pad, pad)
	fmt.Fprintf(&b, `<line class="base" x1="%d" y1="%d" x2="%d" y2="%d"/>`, -pad/2, base, g.advance+pad/2, base)
	fmt.Fprintf(&b, `<path class="ink" d="%s"/><path class="mid" d="%s"/>`, g.ink, g.ink)
	fmt.Fprintf(&b, `</g></svg>`)
	return b.Bytes(), nil
}

// banner stamps a heading across the top of a rendered sheet, so a set of
// variants can be talked about by NUMBER rather than by describing the picture.
//
// Applied to the finished SVG rather than threaded through both renderers: the
// heading is not part of what is being measured, and a variant sheet that
// disagreed with its own caption would be worse than no caption.
func banner(svg []byte, label string) []byte {
	if label == "" {
		return svg
	}
	// The viewBox is the sheet's own coordinate system; the banner is sized
	// from its width so it reads the same whatever rung was drawn.
	var vx, vy, vw, vh int
	if _, err := fmt.Sscanf(string(svg), `<svg xmlns="http://www.w3.org/2000/svg" viewBox="%d %d %d %d"`,
		&vx, &vy, &vw, &vh); err != nil {
		return svg
	}
	head := vw / 14
	open := fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="%d %d %d %d" width="%d" height="%d">`,
		vx, vy-head, vw, vh+head, vw, vh+head)
	rest := string(svg)
	if i := strings.Index(rest, ">"); i >= 0 {
		rest = rest[i+1:]
	}
	text := fmt.Sprintf(`<rect x="%d" y="%d" width="%d" height="%d" fill="#f2f2f2"/>`+
		`<text x="%d" y="%d" font-family="sans-serif" font-size="%d" font-weight="bold" fill="#111">%s</text>`,
		vx, vy-head, vw, head,
		vx+head/4, vy-head/4, head*55/100, esc(label))
	return []byte(open + text + rest)
}
