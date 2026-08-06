package main

import (
	"math"

	"seedhammer.com/bezier"
)

// Counters, measured on the INK rather than on the path.
//
// The obvious method -- the smallest distance between two parts of the
// centreline that are far apart along it -- was tried first and is wrong here,
// for a reason specific to this face. font/constant glyphs are SINGLE STROKE:
// the tool cannot lift, so it RETRACES. 'T' draws its crossbar, comes back
// along it, and goes down the stem; 'E' and '+' and 't' do the same. The path
// therefore lies exactly on top of itself at points a long way apart along it,
// and a centreline metric reports a gap of zero for half the alphabet. Measured
// 2026-08-05: 85 of 95 glyphs "failed", including the period.
//
// Retraced metal is metal that was already removed. What actually decides
// whether a counter survives is the shape of the BARE METAL left between the
// strokes, so that is what is measured: stamp the ink at its true width, find
// the white regions it encloses, and report the largest disc that fits in each.
// Retracing, seams and corners all stop being special cases, because ink is ink.

// counterRes is the raster resolution in pixels per millimetre. At 40, one
// pixel is 0.025mm and the 0.30mm stroke is 12 pixels across -- fine enough
// that the inscribed-disc figures below are good to about a pixel.
const counterRes = 40.0

// counter is one enclosed region of bare metal.
type counter struct {
	// WidthMM is the diameter of the largest disc that fits inside it: the
	// narrowest dimension a reader has to see. Area alone does not decide
	// legibility -- a long thin slot has plenty of it and still reads as a
	// solid line once the tool wanders.
	WidthMM float64
	AreaMM2 float64
}

// ink is a rasterised glyph plus the geometry needed to talk about it in mm.
type inkRaster struct {
	w, h   int
	on     []bool
	ox, oy float64 // device-unit coordinate of pixel (0,0)
	scale  float64 // pixels per device unit
}

// rasterize stamps the stroked centreline into a bitmap, exactly as the tool
// removes metal: a disc of the stroke's diameter dragged along the path.
func rasterize(runs [][]bezier.Point, sw int, mmUnits int) *inkRaster {
	if len(runs) == 0 {
		return nil
	}
	minX, minY := math.Inf(1), math.Inf(1)
	maxX, maxY := math.Inf(-1), math.Inf(-1)
	for _, run := range runs {
		for _, p := range run {
			minX, minY = math.Min(minX, float64(p.X)), math.Min(minY, float64(p.Y))
			maxX, maxY = math.Max(maxX, float64(p.X)), math.Max(maxY, float64(p.Y))
		}
	}
	scale := counterRes / float64(mmUnits) // px per device unit
	// A margin of a full stroke plus two pixels, so the ink never touches the
	// border. The flood fill below starts at the border and has to be able to
	// reach all the way round the outside of the glyph.
	margin := float64(sw)/2 + 3/scale
	minX, minY = minX-margin, minY-margin
	maxX, maxY = maxX+margin, maxY+margin

	r := &inkRaster{
		w:     int((maxX-minX)*scale) + 1,
		h:     int((maxY-minY)*scale) + 1,
		ox:    minX,
		oy:    minY,
		scale: scale,
	}
	r.on = make([]bool, r.w*r.h)

	rad := float64(sw) / 2 * scale // the disc, in pixels
	for _, run := range runs {
		for i := 1; i < len(run); i++ {
			r.stampSegment(run[i-1], run[i], rad)
		}
		if len(run) == 1 {
			r.stampDisc(r.px(run[0]))(rad)
		}
	}
	return r
}

func (r *inkRaster) px(p bezier.Point) (float64, float64) {
	return (float64(p.X) - r.ox) * r.scale, (float64(p.Y) - r.oy) * r.scale
}

// stampSegment drags the disc from a to b. Round caps and joins come free: the
// same disc is stamped at every step including both ends, which is what
// stroke-linecap:round and stroke-linejoin:round mean.
func (r *inkRaster) stampSegment(a, b bezier.Point, rad float64) {
	ax, ay := r.px(a)
	bx, by := r.px(b)
	n := int(math.Hypot(bx-ax, by-ay)) + 1
	for i := 0; i <= n; i++ {
		t := float64(i) / float64(n)
		r.stampDisc(ax+(bx-ax)*t, ay+(by-ay)*t)(rad)
	}
}

func (r *inkRaster) stampDisc(cx, cy float64) func(float64) {
	return func(rad float64) {
		x0, x1 := int(cx-rad)-1, int(cx+rad)+1
		y0, y1 := int(cy-rad)-1, int(cy+rad)+1
		rr := rad * rad
		for y := max(y0, 0); y <= min(y1, r.h-1); y++ {
			for x := max(x0, 0); x <= min(x1, r.w-1); x++ {
				dx, dy := float64(x)-cx, float64(y)-cy
				if dx*dx+dy*dy <= rr {
					r.on[y*r.w+x] = true
				}
			}
		}
	}
}

// findCounters returns the enclosed bare-metal regions, widest disc first.
//
// "Enclosed" is decided by a flood fill from the border: whatever the outside
// can reach is background, and every white pixel it cannot reach is inside the
// glyph. A counter that the ink has closed is simply not in the result, which
// is the answer the sheet needs -- it did not get smaller, it stopped existing.
func (r *inkRaster) findCounters(mmUnits int) []counter {
	if r == nil {
		return nil
	}
	outside := make([]bool, r.w*r.h)
	var stack []int
	push := func(i int) {
		if !r.on[i] && !outside[i] {
			outside[i] = true
			stack = append(stack, i)
		}
	}
	for x := range r.w {
		push(x)
		push((r.h-1)*r.w + x)
	}
	for y := range r.h {
		push(y * r.w)
		push(y*r.w + r.w - 1)
	}
	for len(stack) > 0 {
		i := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		x, y := i%r.w, i/r.w
		if x > 0 {
			push(i - 1)
		}
		if x < r.w-1 {
			push(i + 1)
		}
		if y > 0 {
			push(i - r.w)
		}
		if y < r.h-1 {
			push(i + r.w)
		}
	}

	dt := r.distanceToInk()
	pxMM := 1 / counterRes

	seen := make([]bool, r.w*r.h)
	var out []counter
	for i := range r.on {
		if r.on[i] || outside[i] || seen[i] {
			continue
		}
		// One enclosed region: its area, and the deepest point in it, which is
		// the centre of the largest disc that fits.
		area, deepest := 0, 0.0
		stack = append(stack[:0], i)
		seen[i] = true
		for len(stack) > 0 {
			j := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			area++
			deepest = math.Max(deepest, dt[j])
			x := j % r.w
			for _, n := range [4]int{j - 1, j + 1, j - r.w, j + r.w} {
				if x == 0 && n == j-1 || x == r.w-1 && n == j+1 || n < 0 || n >= len(r.on) {
					continue
				}
				if !r.on[n] && !outside[n] && !seen[n] {
					seen[n] = true
					stack = append(stack, n)
				}
			}
		}
		out = append(out, counter{
			WidthMM: 2 * deepest * pxMM,
			AreaMM2: float64(area) * pxMM * pxMM,
		})
	}
	// Widest first: the tightest counter is the last one, and it is the one
	// that decides the glyph.
	for i := range out {
		for j := i + 1; j < len(out); j++ {
			if out[j].WidthMM > out[i].WidthMM {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}

// distanceToInk is a two-pass chamfer transform: for every pixel, roughly how
// far it is from the nearest ink, in pixels. Chamfer 3-4 is within about 6% of
// true Euclidean, which at 40px/mm is a couple of hundredths of a millimetre --
// well inside what a 0.3mm tool and a steel plate will honour.
func (r *inkRaster) distanceToInk() []float64 {
	const big = 1 << 20
	d := make([]float64, r.w*r.h)
	for i := range d {
		if r.on[i] {
			d[i] = 0
		} else {
			d[i] = big
		}
	}
	at := func(x, y int) float64 {
		if x < 0 || y < 0 || x >= r.w || y >= r.h {
			return big
		}
		return d[y*r.w+x]
	}
	for y := range r.h {
		for x := range r.w {
			i := y*r.w + x
			d[i] = math.Min(d[i], math.Min(
				math.Min(at(x-1, y)+3, at(x, y-1)+3),
				math.Min(at(x-1, y-1)+4, at(x+1, y-1)+4)))
		}
	}
	for y := r.h - 1; y >= 0; y-- {
		for x := r.w - 1; x >= 0; x-- {
			i := y*r.w + x
			d[i] = math.Min(d[i], math.Min(
				math.Min(at(x+1, y)+3, at(x, y+1)+3),
				math.Min(at(x+1, y+1)+4, at(x-1, y+1)+4)))
		}
	}
	for i := range d {
		d[i] /= 3
	}
	return d
}
