package gui

import (
	"fmt"
	"image"
	"image/color"
	"strings"
	"testing"

	"seedhammer.com/gui/assets"
	"seedhammer.com/gui/op"
)

// ─── F-208 / R-I: the scroll arrows ────────────────────────────────────────
//
// SPEC_s6b_pre_flash_cycle.md §5, §5.1; REQUIREMENTS_s6b_pre_flash_cycle.md
// R-E, R-I. Three gates: 5.1 (the visibility predicate, MUST be green), 5.1b
// (R-E's maxScroll divergence probe, EXPECTED TO FAIL -- see its own doc
// comment), 5.3 (a pixel-level chip/text-row overlap check).
//
// Measured geometry this file assumes and does not re-derive: panel 480x320;
// bodyClip = (6,44)-(423,314), 417 wide (Warning.Layout, gui.go); leadingSize
// = 44 (theme.go:43); scrollFadeDist = 16 (gui.go:761); assets.ArrowUp /
// ArrowDown = 15x9.

// TestBodyClipWidthStaysAt417 is the standalone regression pin
// IMPLEMENTATION_PLAN_s6b.md's P5 boundary asks for by name: "must not change
// body width from 417" -- R-I decoupled F-192's modal-fit sweep from the
// arrows precisely because the arrows float OVER the body rather than
// narrowing it, and that decoupling is void the moment this number moves
// (P6's fit measurements would need to be retaken).
func TestBodyClipWidthStaysAt417(t *testing.T) {
	if got := warningBodyClip(image.Pt(480, 320)).Dx(); got != 417 {
		t.Errorf("warningBodyClip(480,320).Dx() = %d, want 417 -- this voids R-I's "+
			"decoupling of F-192 from the scroll arrows; P6's fit measurements would need "+
			"to be retaken", got)
	}
}

// ─── GATE 5.1 -- the visibility predicate, unit-level ──────────────────────
//
// Pins scrollArrowsVisible's exact formula against both predicates the spec
// names and rejects: `maxScroll > 0` (false positive -- fires while content is
// still entirely on the panel) and `bodysz.Y > bodyClip.Dy()` (false negative
// -- fires 10px late, hiding content with no arrow, "F-185's own harm").
func TestGate51VisibilityPredicateFormula(t *testing.T) {
	dims := image.Pt(480, 320)
	bodyClip := warningBodyClip(dims)
	if got := bodyClip.Dx(); got != 417 {
		t.Fatalf("INCONCLUSIVE: warningBodyClip(480,320).Dx() is %d, not the 417 the spec "+
			"measures -- the fixture below no longer matches gui.go", got)
	}
	if bodyClip != (image.Rectangle{Min: image.Pt(6, 44), Max: image.Pt(423, 314)}) {
		t.Fatalf("INCONCLUSIVE: warningBodyClip(480,320) is %v, not the (6,44)-(423,314) "+
			"the spec measures", bodyClip)
	}

	cases := []struct {
		name    string
		bodyszY int
		want    bool
	}{
		// The exact boundary the spec derives: 44+16+bodysz.Y > 320 iff
		// bodysz.Y > 260.
		{"260 -- exactly fits, no arrow", 260, false},
		{"261 -- one pixel off panel, arrow", 261, true},
		{"far under", 100, false},
		{"far over", 400, true},
		// bodysz.Y > bodyClip.Dy() (270) is the REJECTED false-negative
		// predicate. At 270 the new predicate is ALREADY true (since 270>260)
		// -- proving the new predicate does not share that predicate's blind
		// spot.
		{"270 -- the rejected bodysz.Y>bodyClip.Dy() boundary", 270, true},
		// maxScroll>0 (bodysz.Y>238) is the REJECTED false-positive predicate
		// R-E/GATE-5.1b names. At 239 the new predicate is still false --
		// proving the new predicate does not share THAT blind spot either.
		{"239 -- the rejected maxScroll>0 boundary", 239, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := scrollArrowsVisible(bodyClip, image.Pt(0, c.bodyszY), dims)
			if got != c.want {
				t.Errorf("scrollArrowsVisible(bodysz.Y=%d) = %v, want %v", c.bodyszY, got, c.want)
			}
		})
	}
}

// ─── GATE 5.1 -- wired up, integration-level ───────────────────────────────
//
// Proves the FORMULA above is what actually gates the DRAWN pixels, not just
// an isolated fact. Renders ConfirmWarningScreen (the Warning.Layout shape
// behind every hold-to-confirm modal) and looks for the arrow ICON's own
// colour (th.Text) inside the expected top-chip rectangle -- present only
// when an arrow was actually drawn there.
func scrollArrowsDrawnFor(t *testing.T, body string) bool {
	t.Helper()
	dst := gate53Frame(t, "Modal Fit", body)
	bodyClip := image.Rectangle{Min: image.Pt(6, 44), Max: image.Pt(423, 314)}
	centerX := bodyClip.Min.X + bodyClip.Dx()/2
	topChip := image.Rectangle{
		Min: image.Pt(centerX-arrowChipWidth/2, bodyClip.Min.Y),
		Max: image.Pt(centerX-arrowChipWidth/2+arrowChipWidth, bodyClip.Min.Y+scrollFadeDist),
	}
	return rectHasColor(dst, topChip, descriptorTheme.Text)
}

func TestGate51ArrowsDrawnOnlyWhenContentOverflowsThePanel(t *testing.T) {
	short := "Short body."
	if drawn := scrollArrowsDrawnFor(t, short); drawn {
		t.Errorf("the scroll arrow drew for a short body that fits entirely on the panel")
	}
	long := modalFiller(700)
	if drawn := scrollArrowsDrawnFor(t, long); !drawn {
		t.Errorf("the scroll arrow did NOT draw for a body that overflows the panel " +
			"(modalFiller(700), well past the 260-char/px threshold)")
	}
}

// TestGate51ArrowActuallyScrolls is the "can a user do the thing" check: the
// arrow is not just drawn, tapping it moves w.scroll. Driven through the
// BUTTON path (click helper, event_test.go), which is the same event shape a
// tap on the arrow's op.Input region resolves to inside Clickable.Next
// (gui/widget.go).
func TestGate51ArrowActuallyScrolls(t *testing.T) {
	p := newPlatform()
	p.display = sh2DisplaySize
	ctx := NewContext(p)
	w := &Warning{}
	dims := ctx.Platform.DisplaySize()
	long := modalFiller(700)

	w.Layout(ctx, &descriptorTheme, dims, "Modal Fit", long)
	if w.scroll != 0 {
		t.Fatalf("INCONCLUSIVE: scroll is %d before any input", w.scroll)
	}
	click(&ctx.Router, Down)
	w.Layout(ctx, &descriptorTheme, dims, "Modal Fit", long)
	if w.scroll <= 0 {
		t.Errorf("clicking the down arrow's button did not advance w.scroll (got %d)", w.scroll)
	}
	before := w.scroll
	click(&ctx.Router, Up)
	w.Layout(ctx, &descriptorTheme, dims, "Modal Fit", long)
	if w.scroll >= before {
		t.Errorf("clicking the up arrow's button did not retreat w.scroll (got %d, was %d)", w.scroll, before)
	}
}

// ─── GATE 5.1b -- R-E's maxScroll divergence probe ─────────────────────────
//
// EXPECTED TO FAIL, per plan (IMPLEMENTATION_PLAN_s6b.md, "GATE 5.1b is
// expected to FAIL and does not gate") and per spec
// (SPEC_s6b_pre_flash_cycle.md §7 gate table: "R-E's maxScroll divergence
// probe -- failures expected, files findings, does not gate"). DO NOT loosen
// this assertion to make it pass -- a green result here before fadeClip's
// real clip mask is restored would be masking the exact gap R-E documents,
// not closing it.
//
// `maxScroll` (gui.go:409) reserves 2*scrollFadeDist=32px of margin that
// fadeClip (a stubbed no-op, R-E) never actually renders as fade. So
// `maxScroll > 0` can be TRUE while GATE 5.1's predicate -- what the PANEL
// actually shows -- is FALSE: a false positive in the direction R-E's own
// text predicts ("content can satisfy maxScroll > 0 while being entirely
// visible"). This test is the cheapest guard against a future edit wiring an
// arrow to maxScroll by mistake (REQUIREMENTS_s6b_pre_flash_cycle.md R-E: "S6b
// owes a test that the two agree"): it fails loudly, right here, instead of
// silently showing an arrow with nothing below the fold.
//
// It is expected to go green on its own the day fadeClip's real mask is
// restored (filed as the honest-geometry work, after F-192 -- R-E) and the two
// predicates stop diverging.
func TestGate51bMaxScrollAgreesWithVisibility(t *testing.T) {
	dims := image.Pt(480, 320)
	bodyClip := image.Rectangle{Min: image.Pt(6, 44), Max: image.Pt(423, 314)}

	var lines []string
	diverged := 0
	const lo, hi = 0, 320
	for y := lo; y <= hi; y++ {
		maxScroll := y - (bodyClip.Dy() - 2*scrollFadeDist)
		oldPredicate := maxScroll > 0
		newPredicate := scrollArrowsVisible(bodyClip, image.Pt(0, y), dims)
		if oldPredicate != newPredicate {
			diverged++
			lines = append(lines, fmt.Sprintf("bodysz.Y=%d: maxScroll=%d (>0=%v) vs GATE-5.1=%v",
				y, maxScroll, oldPredicate, newPredicate))
		}
	}
	t.Logf("R-E divergence probe over bodysz.Y in [%d,%d] (%d values): %d diverge",
		lo, hi, hi-lo+1, diverged)
	if len(lines) > 0 {
		t.Logf("diverging range:\n%s", strings.Join(lines, "\n"))
	}
	if diverged > 0 {
		t.Errorf("maxScroll>0 disagrees with GATE 5.1's predicate on %d of %d bodysz.Y "+
			"values in [%d,%d] -- see the log above for the exact range. EXPECTED (R-E): "+
			"fadeClip is a stubbed no-op, so maxScroll's reserved fade margin is never "+
			"actually rendered as fade. This is a FINDING against the deferred "+
			"honest-geometry work that restores fadeClip (R-E), not a defect in this phase.",
			diverged, hi-lo+1, lo, hi)
	}
}

// ─── GATE 5.3 -- pixel-level chip/text-row overlap ─────────────────────────
//
// bodyDrawnFully (modal_fits_test.go) proves a body was SUBMITTED; it cannot
// prove a submitted glyph was not subsequently painted over by an opaque
// chip, because ExtractText walks the op tree and a glyph under a mask is
// still "in" it. This is the pixel-level check the spec requires instead
// (§5 point 3): render an actual raster and look at what is really on the
// glass.

// gate53Frame renders ConfirmWarningScreen's FIRST frame (no pumping -- the
// screen draws synchronously on entry, matching modal_fits_test.go's
// firstModalFrame rationale: ConfirmDelay.Progress is 0 until Start is
// called, which needs a press event this call never generates) and returns
// the raster.
func gate53Frame(t *testing.T, title, body string) *image.RGBA {
	t.Helper()
	p := newPlatform()
	p.display = sh2DisplaySize
	ctx := NewContext(p)
	w := &ConfirmWarningScreen{Title: title, Body: body, Icon: assets.IconHammer}
	dims := ctx.Platform.DisplaySize()
	o, _ := w.Layout(ctx, &descriptorTheme, dims)
	r := image.Rectangle{Max: dims}
	dst := image.NewRGBA(r)
	mask := image.NewRGBA(r)
	d := new(op.Drawer)
	d.Draw(dst, mask, o)
	return dst
}

// rectHasColor reports whether any pixel in r (clipped to dst's bounds)
// equals col.
func rectHasColor(dst *image.RGBA, r image.Rectangle, col color.RGBA) bool {
	r = r.Intersect(dst.Bounds())
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			if dst.RGBAAt(x, y) == col {
				return true
			}
		}
	}
	return false
}

// rowInkExtent scans rows [yLo,yHi) x [xLo,xHi) for pixels matching col and
// returns the [minY,maxY) range where any were found. ok is false if none
// were found at all.
func rowInkExtent(dst *image.RGBA, xLo, xHi, yLo, yHi int, col color.RGBA) (minY, maxY int, ok bool) {
	minY, maxY = yHi, yLo
	for y := yLo; y < yHi; y++ {
		for x := xLo; x < xHi; x++ {
			if dst.RGBAAt(x, y) == col {
				if y < minY {
					minY = y
				}
				if y+1 > maxY {
					maxY = y + 1
				}
				ok = true
			}
		}
	}
	return minY, maxY, ok
}

// TestGate53ChipDoesNotOverlapDrawnTextRows renders one representative long
// modal with the arrow showing (the ConfirmWarningScreen shape, same as
// GATE 5.1's integration test) and checks three things pixel-by-pixel:
//
//  1. The topmost body text row's ink never reaches above bodyClip.Min.Y +
//     scrollFadeDist (60) -- the top chip's own lower edge -- so it can never
//     be covered by the top chip, whatever the chip's exact footprint.
//  2. The bottommost row that is ENTIRELY inside the readable window
//     [60,298) -- the one after which the fade band (now the bottom chip's
//     footprint) begins -- has ink that stays above bodyClip.Max.Y-
//     scrollFadeDist (298), i.e. it clears the bottom chip too.
//  3. The chip itself is genuinely OPAQUE where overflowing text is known to
//     be drawn underneath it: sampling the exact centre pixel of each chip
//     returns only background or icon colour, never a third (blended) colour
//     that would mean a glyph is bleeding through.
func TestGate53ChipDoesNotOverlapDrawnTextRows(t *testing.T) {
	body := modalFiller(700)
	dst := gate53Frame(t, "Modal Fit", body)

	bodyClip := image.Rectangle{Min: image.Pt(6, 44), Max: image.Pt(423, 314)}
	textCol := descriptorTheme.Text
	bgCol := descriptorTheme.Background

	readTop := bodyClip.Min.Y + scrollFadeDist    // 60
	readBottom := bodyClip.Max.Y - scrollFadeDist // 298

	// (1) nothing above the readable window's top edge, ANYWHERE THE CHIP
	// ITSELF DOES NOT ALREADY COVER: scanning the chip's own X-span would
	// just find the arrow icon's own th.Text-coloured ink, which is expected
	// and is not the thing being checked. What must never happen is BODY TEXT
	// creeping into the fade band on either side of the chip, where nothing
	// is there to catch it.
	topChip := image.Rectangle{
		Min: image.Pt(bodyClip.Min.X+bodyClip.Dx()/2-arrowChipWidth/2, bodyClip.Min.Y),
		Max: image.Pt(bodyClip.Min.X+bodyClip.Dx()/2-arrowChipWidth/2+arrowChipWidth, readTop),
	}
	for _, seg := range [2][2]int{{bodyClip.Min.X, topChip.Min.X}, {topChip.Max.X, bodyClip.Max.X}} {
		if _, top, ok := rowInkExtent(dst, seg[0], seg[1], bodyClip.Min.Y, readTop, textCol); ok {
			t.Errorf("body text ink found at x in [%d,%d), y<%d (up to y=%d), inside the top "+
				"chip's own band but OUTSIDE the chip's footprint -- the first drawn row "+
				"does not clear the top arrow", seg[0], seg[1], readTop, top)
		}
	}

	// (2) the last row entirely inside the readable window: find it via the
	// SAME font metrics Warning.Layout itself uses (Poppins Regular16,
	// LineHeightScale 0.75 -- theme.go), independently re-derived here rather
	// than assumed, then confirm its actual drawn ink stays inside [60,298).
	lineHeight := NewStyles().body.LineHeight()
	if lineHeight <= 0 {
		t.Fatalf("INCONCLUSIVE: body line height is %d", lineHeight)
	}
	rows := (readBottom - readTop) / lineHeight
	if rows < 1 {
		t.Fatalf("INCONCLUSIVE: readable window %d..%d fits fewer than one full text row "+
			"at line height %d", readTop, readBottom, lineHeight)
	}
	lastRowTop := readTop + (rows-1)*lineHeight
	lastRowBottom := lastRowTop + lineHeight
	if lastRowBottom > readBottom {
		t.Fatalf("INCONCLUSIVE: computed last full row %d..%d does not fit in the readable "+
			"window %d..%d", lastRowTop, lastRowBottom, readTop, readBottom)
	}
	minY, maxY, ok := rowInkExtent(dst, bodyClip.Min.X, bodyClip.Max.X, lastRowTop, lastRowBottom+4, textCol)
	if !ok {
		t.Fatalf("no ink found in the expected last full row band %d..%d -- INCONCLUSIVE, "+
			"not proof of anything: the row itself may not be where this test computed it",
			lastRowTop, lastRowBottom+4)
	}
	if maxY > readBottom {
		t.Errorf("the last row inside the readable window draws ink up to y=%d, past the "+
			"bottom chip's own edge at y=%d -- it can be covered by the bottom chip "+
			"(rows: minY=%d maxY=%d)", maxY, readBottom, minY, maxY)
	}

	// (3) opacity: sample the geometric centre of each chip. Content is
	// known to overflow well past the bottom chip's band (that is why the
	// arrow is showing at all), so if the chip were not fully opaque -- wrong
	// z-order, accidental alpha -- a blended third colour would appear here.
	centerX := bodyClip.Min.X + bodyClip.Dx()/2
	topCenter := image.Pt(centerX, bodyClip.Min.Y+scrollFadeDist/2)
	bottomCenter := image.Pt(centerX, bodyClip.Max.Y-scrollFadeDist/2)
	for _, p := range []struct {
		name string
		pt   image.Point
	}{{"top chip centre", topCenter}, {"bottom chip centre", bottomCenter}} {
		got := dst.RGBAAt(p.pt.X, p.pt.Y)
		if got != textCol && got != bgCol {
			t.Errorf("%s at %v is %v, neither the icon colour %v nor the background %v -- "+
				"the chip is not opaque, a glyph may be bleeding through",
				p.name, p.pt, got, textCol, bgCol)
		}
	}
}
