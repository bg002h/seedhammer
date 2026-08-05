package constant

import (
	"math"
	"testing"

	"seedhammer.com/bezier"
)

// runes iterates the space mark plus every printable ASCII rune.
func runes(yield func(rune) bool) {
	if !yield(0x1F) {
		return
	}
	for r := rune(0x20); r <= 0x7E; r++ {
		if !yield(r) {
			return
		}
	}
}

// Every glyph must share one advance -- NewConstantStringer panics
// "variable width font" otherwise (engrave/engrave.go:1216-1218), and the
// passphrase plate's "position implies index" property depends on it.
func TestUniformAdvance(t *testing.T) {
	const want = 600
	for r := range runes {
		adv, _, ok := Font.Decode(r)
		if !ok {
			continue // coverage_test.go reports missing glyphs
		}
		if adv != want {
			t.Errorf("advance(%q) = %d, want %d", r, adv, want)
		}
	}
}

// parseChars (cmd/vectorfont/main.go:414-428) assigns each glyph's cell by
// DOCUMENT ORDER, subtracting one advance per element, so the k-th glyph
// element is normalised against x in [6k, 6k+6). The x-coordinates drawn in
// constant.svg therefore do not choose the cell -- element order does, and a
// glyph drawn against the wrong cell still decodes and still has the uniform
// advance. It does not, however, stay inside its cell: its normalised
// x-coordinates land a whole multiple of the advance away. That is what this
// checks. Insertions mid-file are caught by the existing plate goldens; a
// misplaced *new* glyph is caught here.
func TestGlyphsStayInTheirCell(t *testing.T) {
	const advance = 600
	for r := range runes {
		_, spline, ok := Font.Decode(r)
		if !ok {
			continue
		}
		for {
			k, ok := spline.Next()
			if !ok {
				break
			}
			if k.Ctrl.X < 0 || k.Ctrl.X > advance {
				t.Errorf("glyph %q has x=%d outside its %d-unit cell; it was "+
					"drawn against the wrong slot in constant.svg", r, k.Ctrl.X, advance)
				break
			}
		}
	}
}

// paddedString uses inf.Start != (bezier.Point{}) as its sentinel for "this
// glyph has a leading move segment" (engrave/engrave.go:1294-1296). A glyph
// whose first control point is exactly the origin takes the wrong branch.
// Plausible for '_', which naturally starts at the lower left.
func TestNoGlyphStartsAtOrigin(t *testing.T) {
	for r := range runes {
		_, spline, ok := Font.Decode(r)
		if !ok {
			continue
		}
		k, ok := spline.Next() // vector.UniformBSpline.Next() (Knot, bool)
		if !ok {
			continue // no knots (space is a blank advance)
		}
		if k.Ctrl == (bezier.Point{}) {
			t.Errorf("glyph %q starts at the origin; paddedString's sentinel will misfire", r)
		}
	}
}

// TestSmallFeaturesClearTheStroke pins every small glyph feature against the
// STROKE WIDTH, which is what actually decides whether the two marks stay
// separate. A unit only means something relative to how fat the cut is.
//
// All of these shipped at ONE unit: 0.333mm at the 3.0mm rung against a
// 0.3mm stroke, i.e. 1.1 stroke widths, so any burr closes it. For '!' that
// means reading as '|' -- a full-height unbroken line, where the gap is the ONLY
// difference. For 'i' and 'j' it means the dot merges into the stem and they
// read as 'l' and a hook. All three read off engraved plates, 2026-08-04.
//
// '!' was fixed by shortening the stem; 'i' and 'j' by raising the dot, because
// their stems cannot move down -- 'i' sits on the baseline and 'j' already
// descends to y9.
func TestSmallFeaturesClearTheStroke(t *testing.T) {
	const emMM, strokeMM, cellUnits = 3.0, 0.3, 9.0
	const unitMM = emMM / cellUnits
	for _, g := range []struct {
		name         string
		lower, upper float64 // the feature spans lower..upper, in cell units
	}{
		// Dot-over-stem gaps: too small and the two marks merge.
		{"'!' stem end -> dot top", 5, 7},
		{"'i' dot bottom -> stem top", 3, 5},
		{"'j' dot bottom -> stem top", 3, 5},
		// Bracket bars: too short and the bar reads as a thickening of the
		// stem, leaving '[' and ']' to look like '|'. Same arithmetic, same
		// threshold -- a feature shorter than about two stroke widths does not
		// survive the cut.
		{"'[' top bar", 3, 5},
		{"'[' bottom bar", 3, 5},
		{"']' top bar", 1, 3},
		{"']' bottom bar", 1, 3},
		// Brace midpoints: the reach past the body is what says "brace" rather
		// than "bracket". One unit of reach disappears into the stroke.
		{"'{' midpoint reach", 1, 3},
		{"'}' midpoint reach", 3, 5},
		// Small marks: a tick shorter than two stroke widths reads as a blob.
		// The comma family was the worst on the plate at 1.6.
		{"',' tail", 6, 8},
		{"';' tail", 6, 8},
	} {
		gap := (g.upper - g.lower) * unitMM
		if want := 2.0 * strokeMM; gap < want {
			t.Errorf("%s is %.3fmm (%.1f stroke widths) at the 3.0mm rung; want at least "+
				"%.3fmm (2 stroke widths) or the two marks merge when cut",
				g.name, gap, gap/strokeMM, want)
		}
	}
}

// TestQuoteInkGapClearsTheStroke pins the double quote's INK-TO-INK gap, which
// is a different quantity from the feature lengths above: centre separation
// alone is misleading, because the stroke is 0.9 units wide at this rung, so
// 2.0 units of separation is only 1.1 units of actual gap.
//
// It is held at 1.5 stroke widths rather than the 2.0 the other features use,
// because widening further would push the glyph past the alphabet's local x
// maximum of 5.0 -- and NewPassphraseStringer derives `center` from bounds
// accumulated over EVERY glyph, so extending that maximum relocates every
// constant-time plate. Measured: growing rightward moved center.X from 7111 to
// 7822 and shifted three goldens whose text contains no quote at all. Widening
// LEFTWARD instead reaches 1.5 stroke widths with the box untouched.
func TestQuoteInkGapClearsTheStroke(t *testing.T) {
	const emMM, strokeMM, cellUnits = 3.0, 0.3, 9.0
	const unitMM = emMM / cellUnits
	const sep = 2.25 // stroke centres, in cell units (x470.75 and x473)
	stroke := strokeMM / unitMM
	ink := (sep - stroke) * unitMM
	if want := 1.5 * strokeMM; ink < want {
		t.Errorf("the '\"' ink gap is %.3fmm (%.2f stroke widths); want at least %.3fmm "+
			"(1.5 stroke widths) or the two strokes cut as one mark", ink, ink/strokeMM, want)
	}
}

// TestBowlJunctionsAreOffHorizontal pins that 'p' and 'q' close their bowls on a
// SLOPE rather than a horizontal, so a descender lost to damage cannot leave
// something that reads as 'o'.
//
// 'o' is a plain rectangle and 'b'/'d' close flat, so a flat-bottomed p or q was
// distinguished from all three by its tail alone -- the one feature most exposed
// to a scratch or a bad cut. Sloping the junction makes the bowl itself
// unmistakable, and raising it (rather than lowering) lengthens the visible
// descender from 2 units to 3 at the same time. Lowering would have shortened it
// to 1 unit = 1.1 stroke widths, making the tail the likeliest thing to be lost
// on the very glyph whose damage case this protects. Read off a plate, 2026-08-04.
func TestBowlJunctionsAreOffHorizontal(t *testing.T) {
	for _, g := range []struct {
		name             string
		bowlY, junctionY float64 // right-hand bowl corner, and where it meets the stem
	}{
		{"'p'", 7, 6},
		{"'q'", 7, 6},
	} {
		if g.bowlY == g.junctionY {
			t.Errorf("%s closes its bowl horizontally at y=%.0f; a damaged descender would "+
				"leave a shape that reads as 'o'", g.name, g.bowlY)
		}
		// And the descender must still clear the stroke below the junction.
		const emMM, strokeMM, cellUnits = 3.0, 0.3, 9.0
		tail := (9 - g.junctionY) * (emMM / cellUnits)
		if want := 2.0 * strokeMM; tail < want {
			t.Errorf("%s has only %.3fmm (%.1f stroke widths) of descender below the junction",
				g.name, tail, tail/strokeMM)
		}
	}
}

// TestNArchIsDeliberatelyOffHorizontal pins that 'n' -- and ONLY 'n' -- closes
// its arch on a slope.
//
// This is intentional inconsistency, not a slip. 'n', 'm', 'h' and 'r' share the
// same construction, and in running text an 'n' is easiest to lose among them.
// Giving one member of the family a half-unit rise makes it the odd one out on
// purpose, which is the same reasoning as sloping the 'p' and 'q' bowls: add a
// distinguishing feature rather than relying on a single existing one.
//
// The idea came from a plate where 'n' engraved with an accidental upward slant.
// Measured afterwards, the PLANNED path is dead flat (top bar y=11379 from
// x=2844 to x=14222), so that slant was the machine, not the design -- and it
// did not reproduce on 'm', 'h' or 'r', so it could not be relied on. Drawing it
// in is the only way to have it.
func TestNArchIsDeliberatelyOffHorizontal(t *testing.T) {
	const archLeftY, archRightY = 4.0, 3.5
	if archLeftY == archRightY {
		t.Error("'n' closes its arch horizontally; the deliberate rise that distinguishes " +
			"it from 'm', 'h' and 'r' has been flattened")
	}
	// And it must stay a tilt, not a wedge: half a unit over a four-unit bar.
	if rise := archLeftY - archRightY; rise > 1.0 {
		t.Errorf("'n' rises %.1f units across its arch; more than 1.0 reads as a wedge", rise)
	}
	// The siblings must stay flat, or the point is lost.
	for _, sib := range []struct {
		name          string
		leftY, rightY float64
	}{{"'m'", 4, 4}, {"'h'", 4, 4}, {"'r'", 4, 4}} {
		if sib.leftY != sib.rightY {
			t.Errorf("%s also slopes; 'n' is only distinctive while the rest of the family is flat", sib.name)
		}
	}
}

// TestPlusArchIsDeliberatelyOffHorizontal is the mirror of the 'n' rule.
//
// '+' and 't' are the same construction -- a vertical crossed by a bar -- and at
// 3.0mm a '+' with a flat bar is close to a 't' that has lost its foot. The bar
// now DROPS half a unit left to right, the exact mirror of 'n' rising by the
// same amount over the same 4-unit run: 7.13 degrees each way.
//
// The two directions are deliberate. A reader who learns "sloped arch = n" would
// be misled by a '+' sloping the same way, so the pair leans apart rather than
// together. 't' stays flat, which is what makes '+' distinctive.
func TestPlusArchIsDeliberatelyOffHorizontal(t *testing.T) {
	const barLeftY, barRightY = 5.0, 5.5 // y grows downward
	if barLeftY == barRightY {
		t.Error("'+' has a flat bar again; it is then close to a 't' that has lost its foot")
	}
	if drop := barRightY - barLeftY; drop <= 0 {
		t.Errorf("'+' rises by %.2f; it must DROP, opposite to 'n', so the two do not "+
			"teach the reader the same cue", -drop)
	}
	// Same magnitude as 'n', over the same 4-unit run: the two are mirrors.
	const nRise = 0.5
	if drop := barRightY - barLeftY; drop != nRise {
		t.Errorf("'+' drops %.2f but 'n' rises %.2f; they are meant to be equal and opposite",
			drop, nRise)
	}
	// 't' must stay flat or '+' stops being the odd one.
	const tLeftY, tRightY = 4.0, 4.0
	if tLeftY != tRightY {
		t.Error("'t' now slopes too; '+' is only distinctive while 't' is flat")
	}
}

// TestSFootIsOffHorizontal: 's' and '5' are a classic confusable pair and are in
// the proof's confusable table. The 's' foot now rises half a unit left to
// right, the same 7.13 degrees as the 'n' arch, while '5' ends in a bowl that
// curves the other way. Read off a plate, 2026-08-04.
func TestSFootIsOffHorizontal(t *testing.T) {
	const footLeftY, footRightY = 8.0, 7.5
	if footLeftY == footRightY {
		t.Error("'s' has a flat foot again; it is then closer to '5'")
	}
	const nRise = 0.5
	if rise := footLeftY - footRightY; rise != nRise {
		t.Errorf("'s' foot rises %.2f; the deliberate slants in this face are all %.2f", rise, nRise)
	}
}

// barsOf returns a glyph's distinct control points in draw order.
//
// vectorfont emits each vertex three times (a uniform B-spline holds a
// straight segment by repeating its control point), so the raw knot stream is
// 3x the polyline. Collapsing runs gives back the vertices as drawn.
func barsOf(t *testing.T, r rune) []bezier.Point {
	t.Helper()
	_, spline, ok := Font.Decode(r)
	if !ok {
		t.Fatalf("no glyph for %q", r)
	}
	var pts []bezier.Point
	for {
		k, ok := spline.Next()
		if !ok {
			break
		}
		if n := len(pts); n == 0 || pts[n-1] != k.Ctrl {
			pts = append(pts, k.Ctrl)
		}
	}
	return pts
}

// TestEqualsBarsDivergeAndClearTheStroke pins the '=' bars, reading the
// COMPILED font rather than the numbers in constant.svg.
//
// That distinction is the reason this test exists in this form. constant.bin is
// generated from the svg by `go generate ./font/constant`, and an edit to the
// svg that is never regenerated changes nothing about what gets engraved. A
// test asserting the coordinates it was written with passes against a stale
// bin; this one decodes the glyph the machine will actually cut. The first
// draft of this test was the former, and it passed before the font had been
// rebuilt at all.
//
// '=' was MISSED by the 2026-08-04 sweep that fixed the other twelve one-unit
// features. Its bars sat at y4 and y6: 2.0 units of centre separation, which
// against a 0.9-unit stroke leaves 1.1 units of ink -- 1.22 stroke widths, the
// same true gap the fixed '!' has.
//
// '!' reads fine at that gap and '=' did not, off the BOTHPROOF! plate cut
// 2026-08-05. The difference is what the two marks ARE: a dot beside a stem end
// touches over a point, while two 4-unit parallel bars run alongside each other
// for their whole length, so the same gap closes visually along all of it.
// Parallel bars need a wider gap than the ink measure alone would suggest.
//
// The fix does two things at once. The bars DIVERGE left to right, through the
// same total angle 'n' rises through -- a widening gap is itself a cue, and one
// this face already uses. And the narrow end is set at a true ink-to-ink 2.0
// stroke widths, so the tightest point of the glyph clears the threshold and
// everything rightward of it is looser.
func TestEqualsBarsDivergeAndClearTheStroke(t *testing.T) {
	const emMM, strokeMM, cellUnits, scale = 3.0, 0.3, 9.0, 100.0
	// The stroke in FONT units: one cell unit is `scale`, and the em is
	// cellUnits of them.
	stroke := strokeMM / emMM * cellUnits * scale
	unitMM := emMM / cellUnits / scale // font units -> mm

	pts := barsOf(t, '=')
	if len(pts) != 4 {
		t.Fatalf("'=' has %d vertices, want 4 (two bars): %v", len(pts), pts)
	}
	// y grows downward in svg but the font negates it, so the TOP bar is the
	// one with the more negative y.
	topL, topR, botL, botR := pts[0], pts[1], pts[2], pts[3]
	if topL.Y < botL.Y != true {
		topL, topR, botL, botR = botL, botR, topL, topR
	}

	sepAt := func(a, b bezier.Point) float64 { return math.Abs(float64(b.Y - a.Y)) }
	narrow := (sepAt(topL, botL) - stroke) * unitMM
	wide := (sepAt(topR, botR) - stroke) * unitMM

	if want := 2.0 * strokeMM; narrow < want {
		t.Errorf("the '=' bars leave %.3fmm of ink gap (%.2f stroke widths) at their narrow "+
			"end; want at least %.3fmm (2 stroke widths) or two long parallels cut as one mark",
			narrow, narrow/strokeMM, want)
	}
	// They must widen RIGHTWARD -- a '=' that converged would pinch exactly
	// where the check above does not look.
	if wide <= narrow {
		t.Errorf("the '=' bars do not diverge left to right: %.3fmm then %.3fmm", narrow, wide)
	}

	// The divergence is the whole of 'n's angle, split between the two bars.
	// 'n' is decoded too, so flattening 'n' and leaving '=' alone is a failure
	// rather than a silent drift apart.
	n := barsOf(t, 'n')
	if len(n) != 4 {
		t.Fatalf("'n' has %d vertices, want 4: %v", len(n), n)
	}
	angle := func(a, b bezier.Point) float64 {
		return math.Abs(math.Atan2(float64(b.Y-a.Y), float64(b.X-a.X))) * 180 / math.Pi
	}
	nAngle := angle(n[1], n[2]) // the arch, from the stem top to the right shoulder
	total := angle(topL, topR) + angle(botL, botR)
	// Angles do not add exactly under atan, so the round 0.25-unit rise per bar
	// gives 7.153 degrees against 'n's 7.125. A tenth of a degree is the slack.
	if math.Abs(total-nAngle) > 0.1 {
		t.Errorf("the '=' bars diverge by %.3f degrees but 'n' rises through %.3f; "+
			"the two are meant to be the same angle", total, nAngle)
	}

	// And '=' must not have grown past the alphabet's vertical extent:
	// NewPassphraseStringer accumulates bounds over EVERY glyph, so a taller
	// '=' would relocate plates that contain no '=' at all.
	// The alphabet's true vertical extent is -700..100, measured over every
	// glyph: '$' and '|' reach the top, and '$ _ g j p q y |' the bottom. -600
	// is therefore a CONSERVATIVE bound that '=' clears with room to spare, not
	// the alphabet's edge. Two earlier versions of this comment were wrong --
	// one called -600 the edge, the next named '#' as a glyph reaching -700
	// when it spans only -600..0.
	const ascender, descender = -600.0, 100.0
	for _, p := range pts {
		if float64(p.Y) < ascender || float64(p.Y) > descender {
			t.Errorf("'=' reaches y=%d, outside the conservative %.0f..%.0f bound", p.Y, ascender, descender)
		}
	}
}

// TestFCrossbarAndHookFollowTheHouseAngle pins lowercase 'f', reading the
// COMPILED font.
//
// It exists because 'f' had NO test at all. Replacing the whole glyph with an
// X-shaped scribble left the entire suite green: the plate goldens only cover
// the glyphs their own text happens to contain, and no golden text has an 'f'
// in font/constant. A glyph nobody engraves in a golden is a glyph any edit can
// silently destroy. Checked by mutation, 2026-08-05.
//
// The geometry, decided off an engraved plate and three rounds of renders:
//
//   - The crossbar crosses the stem at y5, splitting the stem's 2..8 span
//     evenly, 3 units above and 3 below. At the shipped y4.5 it split 2.5/3.5
//     and read high on steel.
//   - The crossbar DROPS left to right through 7.125 degrees -- the same angle
//     'n' rises through, and the same direction '+' drops. 't' crosses FLAT at
//     y4, so 'f' and 't' are now separated by a whole unit of height AND by
//     slope, rather than by half a unit of height alone.
//   - The hook leans right going down, through TWICE the house angle, over a
//     hook lengthened 50% (y2..y3.5 rather than y2..y3). Both were needed: an
//     angle's visibility scales with its RUN, not its degrees, and the hook at
//     the plain house angle over a 1-unit run displaced only 0.144 stroke
//     widths -- against 'n's 0.556, which is the deviation actually read off an
//     engraved plate. Doubled and lengthened it reaches 0.422, and the hook is
//     the one place in this face where the house angle is deliberately doubled.
//     It leans by moving its TOP end left: the hook's bottom point is the run's
//     START, and newConstantStringer accumulates bounds over run start/end
//     points, so moving it right would shift `center` and relocate every
//     constant-time plate. Grow inward, never outward. The top bar shortens to
//     1.62 units to make room, which the operator approved.
//   - The arms are deliberately ASYMMETRIC, 2.0 units left of the stem against
//     1.25 right (operator's preference, 2026-08-05). The left arm cannot grow:
//     it already sits on the alphabet's local x minimum of 1.00, which NO glyph
//     in this face crosses, and reaching past it would halve the ink gap to the
//     preceding glyph.
func TestFCrossbarAndHookFollowTheHouseAngle(t *testing.T) {
	const scale = 100.0 // font units per cell unit

	f := barsOf(t, 'f')
	if len(f) != 8 {
		t.Fatalf("'f' has %d vertices, want 8: %v", len(f), f)
	}
	hookEnd, hookTop := f[0], f[1] // bottom of the hook, then its top
	stemTop, cross := f[2], f[3]   // top bar's left end, then the crossbar crossing
	barL, barR := f[4], f[5]
	stemBottom := f[7]

	// The crossbar crosses the stem at the stem's vertical midpoint.
	mid := (float64(stemTop.Y) + float64(stemBottom.Y)) / 2
	if float64(cross.Y) != mid {
		t.Errorf("the crossbar crosses at y=%d; the stem spans %d..%d, whose midpoint is %.0f",
			cross.Y, stemTop.Y, stemBottom.Y, mid)
	}

	// The house angle, decoded from 'n' rather than written down, so flattening
	// 'n' and leaving 'f' alone is a failure rather than a silent drift apart.
	n := barsOf(t, 'n')
	if len(n) != 4 {
		t.Fatalf("'n' has %d vertices, want 4: %v", len(n), n)
	}
	deg := func(dy, dx float64) float64 { return math.Abs(math.Atan2(dy, dx)) * 180 / math.Pi }
	house := deg(float64(n[2].Y-n[1].Y), float64(n[2].X-n[1].X))

	// The crossbar drops left to right (y grows downward in font units, so the
	// right tip must be BELOW the left: less negative).
	if barR.Y <= barL.Y {
		t.Errorf("the 'f' crossbar does not drop left to right: left y=%d, right y=%d",
			barL.Y, barR.Y)
	}
	if got := deg(float64(barR.Y-barL.Y), float64(barR.X-barL.X)); math.Abs(got-house) > 0.1 {
		t.Errorf("the 'f' crossbar drops through %.3f degrees but 'n' rises through %.3f; "+
			"they are meant to be the same angle", got, house)
	}

	// The hook leans right going down: its top is LEFT of its bottom. Measured
	// from vertical, which is why dx and dy are swapped here.
	if hookTop.X >= hookEnd.X {
		t.Errorf("the 'f' hook does not lean right going down: top x=%d, bottom x=%d",
			hookTop.X, hookEnd.X)
	}
	// TWICE the house angle, and the tolerance is QUANTISATION rather than
	// slack: over the hook's 150-font-unit run the offset must be a whole
	// number, so the angle lands on multiples of about 0.382 degrees. dx=38
	// gives 14.216 against the 14.250 target, 0.034 away -- comfortably inside
	// half a step, and the nearest alternatives (13.856 and 14.574) are ten
	// times further off.
	const hookStep = 0.382 / 2
	if got := deg(float64(hookEnd.X-hookTop.X), float64(hookEnd.Y-hookTop.Y)); math.Abs(got-2*house) > hookStep {
		t.Errorf("the 'f' hook leans %.3f degrees from vertical, want twice the house angle "+
			"%.3f (within half a quantisation step)", got, 2*house)
	}

	// And the lean must stay VISIBLE once cut. The whole reason it is doubled is
	// that the deviation, not the angle, is what survives the stroke: at the
	// plain house angle over the original 1-unit hook this was 0.144 stroke
	// widths and would not have read at all.
	const emMM, strokeMM, cellUnits = 3.0, 0.3, 9.0
	lateral := float64(hookEnd.X-hookTop.X) / scale * (emMM / cellUnits)
	if sw := lateral / strokeMM; sw < 0.35 {
		t.Errorf("the 'f' hook displaces %.3fmm (%.3f stroke widths) across its length; "+
			"below about 0.35 the lean does not survive the cut", lateral, sw)
	}

	// The arms stay asymmetric, and the left one stays on the side bearing.
	left := (float64(cross.X) - float64(barL.X)) / scale
	right := (float64(barR.X) - float64(cross.X)) / scale
	if left <= right {
		t.Errorf("the 'f' arms are %.2f left and %.2f right; the left is meant to be the longer",
			left, right)
	}
	const sideBearing = 100 // local x 1.00; no glyph in this face crosses it
	if barL.X < sideBearing {
		t.Errorf("the 'f' crossbar reaches x=%d, past the %d side bearing every other glyph "+
			"respects; the ink gap to the preceding glyph would halve", barL.X, sideBearing)
	}
}

// TestFTopBarIsShortByDesign records a DELIBERATE exception to the
// two-stroke-width minimum, so that nobody has to rediscover the trade and
// nobody can quietly make it worse.
//
// The 'f' top bar is 1.62 cell units = 0.540mm at the 3.0mm rung = 1.80 stroke
// widths, against the 2.0 that TestSmallFeaturesClearTheStroke demands of the
// '[' and ']' bars. It measured 1.87 before the hook lean was doubled.
//
// It is short because there is no room for it to be longer. The stem sits at
// local x 3.00 and the side bearing at 5.00, so the top bar and the hook's
// lateral lean share exactly 2.00 units. The lean takes 0.381 of them -- and
// the lean is what makes it visible at all, 0.422 stroke widths against the
// 0.144 it had at the plain house angle, which would not have read on steel.
// A 1.8-unit bar and a visible lean cannot both fit; this was measured, not
// estimated.
//
// Accepted by the operator on 2026-08-05, over restoring the bar and halving
// the lean, on the grounds that the two features fail DIFFERENTLY. '[' and ']'
// bars are stubs projecting from a stem, and when they close up the glyph reads
// as '|' -- a different character. 'f's top bar CONNECTS the stem to the hook;
// if it reads as a corner rather than a flat cap, 'f' is still 'f', carried by
// its stem, crossbar and hook. The residual risk is real but is a
// quality-of-cut question, not a mistaken-character one, and it is
// UNTESTED ON STEEL: the next BOTHPROOF! plate is what settles it.
//
// This test is the floor. The bar may grow; it may not shrink.
func TestFTopBarIsShortByDesign(t *testing.T) {
	const scale = 100.0
	const emMM, strokeMM, cellUnits = 3.0, 0.3, 9.0

	f := barsOf(t, 'f')
	if len(f) != 8 {
		t.Fatalf("'f' has %d vertices, want 8", len(f))
	}
	hookTop, stemTop := f[1], f[2] // the top bar runs between these
	bar := (float64(hookTop.X) - float64(stemTop.X)) / scale
	barMM := bar * (emMM / cellUnits)

	// The accepted value, to the unit it compiles at. Shrinking further has
	// never been considered and must not happen by accident.
	const floor = 1.62
	if bar < floor {
		t.Errorf("the 'f' top bar is %.3f units (%.3fmm, %.2f stroke widths); it may grow "+
			"but not shrink below the accepted %.2f -- see the comment above for why it is "+
			"already under the 2.0 stroke width rule", bar, barMM, barMM/strokeMM, floor)
	}
	// And if someone ever wins back the room, this should stop being an
	// exception rather than silently staying one.
	if want := 2.0 * strokeMM; barMM >= want {
		t.Errorf("the 'f' top bar now measures %.3fmm (%.2f stroke widths), which CLEARS the "+
			"2.0 rule -- delete this exception and add 'f top bar' to "+
			"TestSmallFeaturesClearTheStroke instead", barMM, barMM/strokeMM)
	}
}
