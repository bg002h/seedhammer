package main

import "sort"

// Channels: the white gap between two roughly parallel strokes.
//
// This is a DIFFERENT hazard from the counter measured in counters.go, and the
// counter metric is blind to it. A counter is enclosed, so a flood fill from
// outside cannot reach it. The gap between the top bar and the bottom bar of an
// 'e' is open at the free end, the fill walks straight in, and the region is
// never counted -- while being exactly the place two parallel lines merge into
// one thick one as the rung drops.
//
// Operator's principle, 2026-08-05: "parallel lines below a certain threshold
// separation may look like a single thick line; finding a floor separation
// distance is needed." That floor is a property of this measurement, not of the
// counter one.
//
// Measured by SCAN LINE, which is what makes open channels fall out for free. A
// vertical channel is a run of white in one column with ink above it and ink
// below it; its width is the run's length. Sweeping every column gives the
// channel's width along its whole extent, and sweeping every row does the same
// for horizontal channels. Nothing has to decide what is "enclosed", and the
// free end of a bar simply stops contributing columns.

// channel is one white gap between two strokes, with its width profile along
// its whole extent.
//
// The PROFILE is the point, and the first draft got this wrong in a way that
// mattered: it reported the narrowest place in the channel, so every glyph came
// back at one pixel. Two strokes meeting at an angle pinch to nothing just
// before they join, and 'N', 'K', 'v', 'w', '>' and twenty more scored 0.025mm
// on a junction while their actual gap ran half a millimetre.
//
// Worse, that metric condemns the very shape the opening-up work aims at: a
// WEDGE is narrow at the fixed end by construction. Judging it on its narrowest
// point would score a diverging pair identically to a parallel one, which is
// the opposite of the truth.
//
// What reads as one thick line is a gap that stays narrow FOR A DISTANCE. So
// the profile is kept and asked that question directly -- see RunBelow.
type channel struct {
	Widths []float64 // the gap at each scan line along the channel, in mm
	RunMM  float64   // how far the channel extends
	Vert   bool      // true: ink above and below. false: ink left and right.
}

// RunBelow is the longest continuous distance over which the gap stays under w.
//
// This is the merge hazard, stated the way the operator stated it: two parallel
// lines closer than the floor, for long enough to read as one line. A wedge
// crosses the floor once and its run below is short; a parallel pair is under
// the floor for its whole length.
func (c channel) RunBelow(w float64) float64 {
	pxMM := 1 / counterRes
	best, cur := 0, 0
	for _, x := range c.Widths {
		if x < w {
			cur++
			best = max(best, cur)
		} else {
			cur = 0
		}
	}
	return float64(best) * pxMM
}

// Median is the channel's typical gap, which is what the eye averages over.
func (c channel) Median() float64 {
	if len(c.Widths) == 0 {
		return 0
	}
	s := append([]float64(nil), c.Widths...)
	sort.Float64s(s)
	return s[len(s)/2]
}

// minChannelRunMM is how far a gap must persist before it counts.
//
// A gap that exists in three columns is a corner, not a channel: two strokes
// meeting at an angle are arbitrarily close just before they join, and reading
// that as "these lines merge" would condemn every junction in the face. A tenth
// of a millimetre is a third of the stroke and about four pixels at the raster
// resolution.
const minChannelRunMM = 0.1

// findChannels returns every gap between strokes, narrowest first.
func (r *inkRaster) findChannels() []channel {
	if r == nil {
		return nil
	}
	pxMM := 1 / counterRes
	minRun := int(minChannelRunMM * counterRes)

	// widths[axis] holds, per scan line, the white runs bracketed by ink.
	type gap struct {
		line  int // which column (vertical) or row (horizontal)
		start int // where the white run begins along that line
		width int
	}
	var vgaps, hgaps []gap

	// Vertical: walk each column, and record every white run that has ink both
	// above and below it. The bracketing is what makes it a gap between two
	// strokes rather than the open field beside the glyph.
	for x := range r.w {
		y := 0
		seenInk := false
		for y < r.h {
			if r.on[y*r.w+x] {
				seenInk = true
				y++
				continue
			}
			start := y
			for y < r.h && !r.on[y*r.w+x] {
				y++
			}
			if seenInk && y < r.h {
				vgaps = append(vgaps, gap{x, start, y - start})
			}
		}
	}
	for y := range r.h {
		x := 0
		seenInk := false
		for x < r.w {
			if r.on[y*r.w+x] {
				seenInk = true
				x++
				continue
			}
			start := x
			for x < r.w && !r.on[y*r.w+x] {
				x++
			}
			if seenInk && x < r.w {
				hgaps = append(hgaps, gap{y, start, x - start})
			}
		}
	}

	group := func(gaps []gap, vert bool) []channel {
		// Gaps in adjacent scan lines that overlap belong to the same channel.
		// Grouping them is what turns a per-column width into a channel with an
		// extent, so a three-column pinch at a junction can be told from a bar
		// running the width of the glyph.
		used := make([]bool, len(gaps))
		byLine := map[int][]int{}
		for i, g := range gaps {
			byLine[g.line] = append(byLine[g.line], i)
		}
		var out []channel
		for i := range gaps {
			if used[i] {
				continue
			}
			// Walk forward through adjacent lines while a gap overlaps.
			used[i] = true
			cur := gaps[i]
			widths := []float64{float64(cur.width) * pxMM}
			run := 1
			for line := cur.line + 1; ; line++ {
				next := -1
				for _, j := range byLine[line] {
					if used[j] {
						continue
					}
					g := gaps[j]
					// Overlap in the direction across the scan.
					if g.start < cur.start+cur.width && cur.start < g.start+g.width {
						next = j
						break
					}
				}
				if next < 0 {
					break
				}
				used[next] = true
				cur = gaps[next]
				widths = append(widths, float64(cur.width)*pxMM)
				run++
			}
			if run < minRun {
				continue
			}
			out = append(out, channel{
				Widths: widths,
				RunMM:  float64(run) * pxMM,
				Vert:   vert,
			})
		}
		return out
	}

	return append(group(vgaps, true), group(hgaps, false)...)
}

// worstChannel is the gap that spends the longest distance under the floor:
// the place in the glyph most likely to read as one thick line.
func worstChannel(cs []channel, floor float64) (channel, bool) {
	best, found := channel{}, false
	for _, c := range cs {
		if !found || c.RunBelow(floor) > best.RunBelow(floor) {
			best, found = c, true
		}
	}
	return best, found
}
