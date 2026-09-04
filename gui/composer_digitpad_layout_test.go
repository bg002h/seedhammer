package gui

import (
	"image"
	"iter"
	"testing"

	"seedhammer.com/gui/assets"
	"seedhammer.com/gui/layout"
	"seedhammer.com/gui/op"
	"seedhammer.com/gui/widget"
)

// ─── W-4: the digit pad's prompt and its echo must not be drawn over each other ─
//
// composerDigitEntry clamped EACH info line to `top.Max.Y - sz.Y` on its own, so
// a second line that did not fit was pushed UP onto the first. The operator met
// it on the device on the blocks pad (decaying-multisig -> Path 1 -> Time lock ->
// After a wait -> Blocks): "How many blocks?" and "1 to 65535 blocks" in one
// illegible band. Measured on this harness before the fix, every one of the four
// pads drew both lines inside a single 17-20 px band, and the three CEILING
// messages -- which wrap to two lines -- merged with the entry box as well, one
// blob from y74 to y129.
//
// A TEXT-PRESENCE ASSERTION CANNOT SEE THIS, which is the same lesson W-3 taught
// one screen over: op.Drawer.ExtractText collects a glyph's rune wherever it
// lands, so the pad reported both strings while showing neither. So this
// rasterises the frame and measures INK.
//
// HOW OVERLAP IS DETECTED, without counting bands (which wrapping makes
// unstable): each info string is rendered ALONE, at the same style and wrap
// width production uses, and its ink rows are counted. The frame's own info ink
// rows must be at least that sum. Two lines drawn over each other occupy FEWER
// rows than the two of them need -- that is what overlapping means -- so the
// count falls and the test fails. It is exact, and it does not care how many
// visual lines a message wraps to.

// padInk is one rendered frame: the ink map, the extracted text, and the drawer
// (so a test can type through the real key geometry).
type padInk struct {
	ink [][]bool
	txt string
	d   *op.Drawer
}

// runPadFrames renders composerDigitEntry frame by frame, rasterising each into
// an ink map INSIDE the frame callback -- the op is only valid for the length of
// that call, exactly as screen.go says.
func runPadFrames(ctx *Context, ui func()) (func() (padInk, bool), func()) {
	next, quit := iter.Pull(func(yield func(padInk) bool) {
		ctx.FrameCallback = func(o op.Op) {
			dims := ctx.Platform.DisplaySize()
			r := image.Rectangle{Max: dims}
			d := new(op.Drawer)
			txt := d.ExtractText(r, o)
			ink := rasterInk(dims, o)
			ctx.Reset()
			ctx.Done = ctx.Done || !yield(padInk{ink, txt, d})
		}
		ui()
	})
	return next, quit
}

// inkBands returns the maximal runs of consecutive rows carrying ink inside r,
// as rectangles. A blank row separates two bands; two things drawn over each
// other are ONE band, which is the whole point.
func inkBands(ink [][]bool, r image.Rectangle) []image.Rectangle {
	var out []image.Rectangle
	inBand, y0, minX, maxX := false, 0, 1<<30, -1
	for y := r.Min.Y; y < r.Max.Y && y < len(ink); y++ {
		any, lo, hi := false, 1<<30, -1
		for x := r.Min.X; x < r.Max.X && x < len(ink[y]); x++ {
			if ink[y][x] {
				any = true
				if x < lo {
					lo = x
				}
				if x > hi {
					hi = x
				}
			}
		}
		switch {
		case any && !inBand:
			inBand, y0, minX, maxX = true, y, lo, hi
		case any:
			minX, maxX = min(minX, lo), max(maxX, hi)
		case inBand:
			out = append(out, image.Rect(minX, y0, maxX+1, y))
			inBand, minX, maxX = false, 1<<30, -1
		}
	}
	if inBand {
		out = append(out, image.Rect(minX, y0, maxX+1, r.Max.Y))
	}
	return out
}

func inkRows(ink [][]bool, r image.Rectangle) int {
	n := 0
	for y := r.Min.Y; y < r.Max.Y && y < len(ink); y++ {
		for x := r.Min.X; x < r.Max.X && x < len(ink[y]); x++ {
			if ink[y][x] {
				n++
				break
			}
		}
	}
	return n
}

// aloneInkRows renders one info string by itself, at the style and wrap width
// composerDigitEntry uses, and counts the rows its glyphs ink.
func aloneInkRows(ctx *Context, dims image.Point, s string) int {
	lbl, _ := widget.Labelw(&ctx.B, ctx.Styles.body, dims.X-2*8, descriptorTheme.Text, s)
	ink := rasterInk(dims, lbl)
	return inkRows(ink, image.Rectangle{Max: dims})
}

// padGeometry recomputes the band above the keyboard and the keyboard's own
// column range, the way composerDigitEntry lays them out.
func padGeometry(ctx *Context, dims image.Point) (band image.Rectangle, kbdMinX, kbdMaxX int) {
	kbd := NewKeyboard(ctx, composerDigitKeys)
	_, ksz := kbd.Layout(ctx, &descriptorTheme)
	screen := layout.Rectangle{Max: dims}
	_, content := screen.CutTop(leadingSize)
	content, _ = content.CutBottom(8)
	top, _ := content.CutBottom(ksz.Y)
	origin := content.S(ksz)
	return image.Rect(top.Min.X, top.Min.Y, top.Max.X, top.Max.Y), origin.X, origin.X + ksz.X
}

// The four pads the composer uses, with their REAL validators from
// composer_lock.go -- a copy in a test would be a second answer to a question
// that must have one.
type padCase struct {
	name, lead, typed string
	echo              func(string) (string, bool)
	maxDigits         int
}

func composerPadCases() []padCase {
	return []padCase{
		{"blocks/empty", "How many blocks?", "", composerBlocksBandEcho, 5},
		{"blocks/12960", "How many blocks?", "12960", composerBlocksBandEcho, 5},
		{"blocks/99999-ceiling", "How many blocks?", "99999", composerBlocksBandEcho, 5},
		{"days/empty", "How many days?", "", composerDaysBandEcho, 3},
		{"days/90", "How many days?", "90", composerDaysBandEcho, 3},
		{"days/999-ceiling", "How many days?", "999", composerDaysBandEcho, 3},
		{"date/empty", "Date as YYYYMMDD", "", composerDateBandEcho, 8},
		{"date/20260901", "Date as YYYYMMDD", "20260901", composerDateBandEcho, 8},
		{"date/20990101-ceiling", "Date as YYYYMMDD", "20990101", composerDateBandEcho, 8},
		{"date/20270231-nodate", "Date as YYYYMMDD", "20270231", composerDateBandEcho, 8},
		{"height/empty", "Block height", "", composerHeightBandEcho, 9},
		{"height/905000", "Block height", "905000", composerHeightBandEcho, 9},
	}
}

// TestComposerDigitPadLinesNeverOverlap is W-4's gate.
func TestComposerDigitPadLinesNeverOverlap(t *testing.T) {
	pitch, rows := loadWalkDigitPad(t)
	dims := sh2DisplaySize

	for _, pc := range composerPadCases() {
		t.Run(pc.name, func(t *testing.T) {
			p := newPlatform()
			p.display = sh2DisplaySize
			ctx := NewContext(p)
			frame, quit := runPadFrames(ctx, func() {
				composerDigitEntry(ctx, &descriptorTheme, "Path 2 lock", pc.lead, pc.maxDigits, pc.echo)
			})
			defer quit()

			f, ok := frame()
			if !ok {
				t.Fatal("the pad drew no frame")
			}
			for i := 0; i < len(pc.typed); i++ {
				pt, ok := digitPoint(pitch, rows, pc.typed[i])
				if !ok {
					t.Fatalf("the walk has no key for %q", pc.typed[i])
				}
				tap(&ctx.Router, f.d, pt)
				if f, ok = frame(); !ok {
					t.Fatalf("the pad returned while typing %q", pc.typed)
				}
			}

			echoLine, _ := pc.echo(pc.typed)
			band, kbdMinX, kbdMaxX := padGeometry(ctx, dims)
			navLeft := dims.X - assets.NavBtnPrimary.Bounds().Size().X

			// (c) PRESENCE. Both must be on the screen at all; if one is
			// missing the geometry below would pass by having less to place.
			if !uiContains(f.txt, pc.lead) {
				t.Fatalf("the prompt %q is not on the pad at all.\nFrame: %q", pc.lead, f.txt)
			}
			if !uiContains(f.txt, echoLine) {
				t.Fatalf("the echo %q is not on the pad at all.\nFrame: %q", echoLine, f.txt)
			}

			// (a) NO TWO TEXT RECTANGLES INTERSECT. The bands inside the
			// keyboard-free band are the entry box first, then the info lines;
			// the info ink must occupy at least as many rows as the two strings
			// need when each is rendered alone. Overlap costs rows, and nothing
			// else can.
			scan := image.Rect(0, leadingSize, navLeft, band.Max.Y)
			bands := inkBands(f.ink, scan)
			if len(bands) < 2 {
				t.Fatalf("the band above the keyboard holds %d ink band(s); the entry box and "+
					"the info lines cannot all be there.\nbands=%v", len(bands), bands)
			}
			info := image.Rect(scan.Min.X, bands[0].Max.Y, scan.Max.X, scan.Max.Y)
			got := inkRows(f.ink, info)
			want := aloneInkRows(ctx, dims, pc.lead) + aloneInkRows(ctx, dims, echoLine)
			if got < want {
				t.Errorf("the prompt and the echo are drawn OVER EACH OTHER.\n"+
					"  below the entry box the frame inks %d row(s); rendered alone the two "+
					"lines need %d\n"+
					"  prompt %q, echo %q\n"+
					"  ink bands in the band above the keyboard: %v\n"+
					"Two lines on top of one another occupy fewer rows than the two of them "+
					"need -- and ExtractText reports both regardless, which is why the walk "+
					"passed while the operator could read neither (W-4).",
					got, want, pc.lead, echoLine, bands)
			}

			// (b) EVERYTHING INSIDE THE BAND. Below it the keyboard owns
			// x[kbdMinX,kbdMaxX); ink anywhere else down there is a line that
			// was pushed out of the band.
			for y := band.Max.Y; y < dims.Y; y++ {
				for x := 0; x < navLeft; x++ {
					if x >= kbdMinX && x < kbdMaxX {
						continue
					}
					if f.ink[y][x] {
						t.Errorf("text ink at (%d,%d) is BELOW the band above the keyboard "+
							"(which ends at y=%d) and outside the keyboard's own columns "+
							"[%d,%d) -- a line was pushed out of the band rather than fitted "+
							"into it", x, y, band.Max.Y, kbdMinX, kbdMaxX)
						return
					}
				}
			}
			if bands[0].Min.Y < band.Min.Y {
				t.Errorf("the entry box starts at y=%d, above the band (y=%d)",
					bands[0].Min.Y, band.Min.Y)
			}
			t.Logf("band=%v kbd_cols=[%d,%d) bands=%v info_rows=%d/%d",
				band, kbdMinX, kbdMaxX, bands, got, want)
		})
	}
}

// TestComposerDigitPadGeometryProbeCanSeeOverlap is the mutation proof for the
// row-counting probe, on composer_paged_geometry_test.go's pattern.
//
// Without it a probe that always reported enough rows would make the gate pass
// by measuring nothing. So it renders two info strings at the SAME offset -- the
// defect, constructed -- and requires the count to come out short, then the same
// two stacked and requires it not to.
func TestComposerDigitPadGeometryProbeCanSeeOverlap(t *testing.T) {
	p := newPlatform()
	p.display = sh2DisplaySize
	ctx := NewContext(p)
	dims := sh2DisplaySize
	const a, b = "How many blocks?", "1 to 65535 blocks"

	la, sa := widget.Labelw(&ctx.B, ctx.Styles.body, dims.X-2*8, descriptorTheme.Text, a)
	lb, _ := widget.Labelw(&ctx.B, ctx.Styles.body, dims.X-2*8, descriptorTheme.Text, b)
	want := aloneInkRows(ctx, dims, a) + aloneInkRows(ctx, dims, b)

	over := rasterInk(dims, op.Layer(la.Offset(image.Pt(20, 100)), lb.Offset(image.Pt(20, 100))))
	if got := inkRows(over, image.Rectangle{Max: dims}); got >= want {
		t.Fatalf("two lines drawn at the SAME offset ink %d row(s) and the probe wants %d -- "+
			"it cannot see an overlap, so the gate is measuring nothing", got, want)
	}
	apart := rasterInk(dims, op.Layer(la.Offset(image.Pt(20, 100)), lb.Offset(image.Pt(20, 100+sa.Y+4))))
	if got := inkRows(apart, image.Rectangle{Max: dims}); got < want {
		t.Errorf("two lines drawn APART ink %d row(s), fewer than the %d the probe wants -- "+
			"it would report every healthy pad as overlapping", got, want)
	}
}
