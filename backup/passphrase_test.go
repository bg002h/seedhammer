package backup

import (
	"slices"
	"strings"
	"testing"

	"seedhammer.com/bezier"
	"seedhammer.com/bspline"
	"seedhammer.com/engrave"
	"seedhammer.com/font/constant"
	"seedhammer.com/passphrase"
)

// plateKnot is one commanded point of an engraving.
type plateKnot struct {
	P bezier.Point
	// Engrave is true for a cutting move, false for a repositioning move.
	Engrave bool
}

func passphraseEngraving(t *testing.T, plate Passphrase) engrave.Engraving {
	t.Helper()
	eng, err := EngravePassphrase(params, plate)
	if err != nil {
		t.Fatal(err)
	}
	return eng
}

// passphrasePoints returns the points the machine is commanded to visit. It
// reads the command stream rather than PlanEngraving's spline, because the
// planner prepends two clamping knots at the origin that are not plate
// geometry; every planned knot is inside the convex hull of these points.
func passphrasePoints(t *testing.T, plate Passphrase) []plateKnot {
	t.Helper()
	var out []plateKnot
	for c := range passphraseEngraving(t, plate) {
		k, ok := c.AsKnot()
		if !ok {
			continue
		}
		out = append(out, plateKnot{P: k.Knot, Engrave: k.Engrave})
	}
	return out
}

// knotBounds is the bounding box of pts.
func knotBounds(pts []plateKnot) bspline.Bounds {
	if len(pts) == 0 {
		return bspline.Bounds{}
	}
	b := bspline.Bounds{Min: pts[0].P, Max: pts[0].P}
	for _, k := range pts[1:] {
		b.Min.X = min(b.Min.X, k.P.X)
		b.Min.Y = min(b.Min.Y, k.P.Y)
		b.Max.X = max(b.Max.X, k.P.X)
		b.Max.Y = max(b.Max.Y, k.P.Y)
	}
	return b
}

// inRect reports whether p is inside [lo, hi] on both axes.
func inRect(p bezier.Point, lo, hi bezier.Point) bool {
	return lo.X <= p.X && p.X <= hi.X && lo.Y <= p.Y && p.Y <= hi.Y
}

// The passphrase must be engraved verbatim. SeedString uppercases its content;
// doing that here would silently destroy the secret.
func TestPassphrasePreservesCase(t *testing.T) {
	got := passphraseGlyphs("Hunter2")
	if got != "Hunter2" {
		t.Errorf("case not preserved: %q", got)
	}
}

// Every space becomes the visible mark; nothing else changes.
func TestPassphraseSpaceSubstitution(t *testing.T) {
	tests := []struct{ in, want string }{
		{"ab", "ab"},
		{"a b", "a\x1fb"},
		{"a  b", "a\x1f\x1fb"},
		{" ab", "\x1fab"},
		{"ab ", "ab\x1f"},
		{"  ", "\x1f\x1f"},
	}
	for _, tc := range tests {
		if got := passphraseGlyphs(tc.in); got != tc.want {
			t.Errorf("%q -> %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestPassphraseGlyphsLeavesNoSpace checks the SUBSTITUTION HELPER only.
//
// Named honestly after a Phase C review finding: an earlier version of this test
// was called TestPassphraseEngravesNoLiteralSpace and its comment claimed to
// guard the engrave path. It did not -- it only called passphraseGlyphs, so
// engraving plate.Passphrase raw would have left it passing. The engrave path is
// guarded by TestPassphraseEngravesMarkNotSpace and TestSpaceFidelity; this is
// the helper-level check, and its name now says so.
func TestPassphraseGlyphsLeavesNoSpace(t *testing.T) {
	if strings.ContainsRune(passphraseGlyphs("a b c"), ' ') {
		t.Error("passphraseGlyphs left a literal 0x20")
	}
}

// The 100-character worst case must stay inside the usable area: the plate is
// 85x85mm and the 10mm margin bands are reserved for metadata.
func TestPassphraseLayoutFitsNoQR(t *testing.T) {
	p := Passphrase{Passphrase: strings.Repeat("a", 100), Font: constant.Font}
	pts := passphrasePoints(t, p)
	if len(pts) == 0 {
		t.Fatal("no knots engraved")
	}
	lo := bezier.Pt(params.F(innerMargin), params.F(innerMargin))
	hi := bezier.Pt(params.F(85-innerMargin), params.F(85-innerMargin))
	for _, k := range pts {
		if !inRect(k.P, lo, hi) {
			t.Fatalf("knot %v outside the usable area [%v, %v]", k.P, lo, hi)
		}
	}
	b := knotBounds(pts)
	// The block is the full 10 rows x 10 characters at the pinned em, not a
	// length-scaled one: 40mm wide, 60mm tall.
	if got, want := b.Max.X-b.Min.X, params.F(40); got > want {
		t.Errorf("block width %d > %d", got, want)
	}
	if got, want := b.Max.Y-b.Min.Y, params.F(60); got > want {
		t.Errorf("block height %d > %d", got, want)
	}
}

// The engraver must receive the visible mark, never a real space: 0x20 has an
// advance but an empty spline, so a passphrase of spaces would engrave nothing
// at all.
func TestPassphraseEngravesMarkNotSpace(t *testing.T) {
	if ink := passphraseTextInk(t, Passphrase{Passphrase: "   ", Font: constant.Font}); len(ink) == 0 {
		t.Error("a passphrase of spaces engraved no ink: 0x20 reached the stringer")
	}
	// Engraving " " must be indistinguishable from engraving the mark itself.
	// passphraseGlyphs is the identity on a string that already holds the mark,
	// so equality here can only mean the space was translated. Only the text
	// block is compared: the space-containing plate also carries the legend.
	viaSpace := passphraseTextInk(t, Passphrase{Passphrase: "a b c", Font: constant.Font})
	viaMark := passphraseTextInk(t, Passphrase{
		Passphrase: "a" + string(SpaceMark) + "b" + string(SpaceMark) + "c",
		Font:       constant.Font,
	})
	if !slices.Equal(viaSpace, viaMark) {
		t.Errorf("space and mark engrave differently: %d vs %d ink points", len(viaSpace), len(viaMark))
	}
}

// passphraseTextInk returns the points engraved inside the text block, in cut
// order, relative to the block's top-left corner. Everything outside the block
// -- the metadata bands and the QR -- is excluded; TestPassphrasePlateWorstCase
// is what proves nothing escapes a region in the first place.
func passphraseTextInk(t *testing.T, plate Passphrase) []bezier.Point {
	t.Helper()
	dim := 0
	if plate.QR {
		dim = passphraseDim(t, plate)
	}
	l := passphraseLayoutFor(params, plate, dim)
	lo := bezier.Pt(l.textX, l.textY)
	hi := bezier.Pt(l.textX+l.blockW, l.textY+l.blockH)
	var out []bezier.Point
	for _, k := range passphrasePoints(t, plate) {
		if k.Engrave && inRect(k.P, lo, hi) {
			out = append(out, k.P.Sub(lo))
		}
	}
	return out
}

func allPrintableASCII() string {
	var b strings.Builder
	for r := rune(0x20); r <= 0x7E; r++ {
		b.WriteRune(r)
	}
	return b.String()
}

// qrModuleIndex maps an offset from the module-(0,0) centre to a module index.
// A module's engraved outline reaches exactly one stroke width from its centre
// and the module pitch is three stroke widths, so a point further than tol off
// a centre is not on the grid at all.
func qrModuleIndex(d, pitch, tol int) (int, bool) {
	m := (d + pitch/2) / pitch
	if d < 0 {
		m = -((-d + pitch/2) / pitch)
	}
	r := d - m*pitch
	if r < 0 {
		r = -r
	}
	return m, r <= tol
}

// passphraseQRGrid replays the plate's engraving and recovers the QR module
// grid from the geometry that will actually be cut. ConstantQR engraves every
// black module and only black modules, so a module is black exactly when
// something lands on it.
func passphraseQRGrid(t *testing.T, plate Passphrase) qrGrid {
	t.Helper()
	code, err := passphraseQRCode(plate)
	if err != nil {
		t.Fatal(err)
	}
	dim := code.Size
	l := passphraseLayoutFor(params, plate, dim)
	if l.qrDim != dim {
		t.Fatalf("layout reserved a %d-module code, plate encodes %d", l.qrDim, dim)
	}
	sw := params.StrokeWidth
	pitch := sw * passphraseQRScale
	// See engrave.centerOf: module (0,0) is centred one stroke width plus a
	// radius in from the QR's top-left corner.
	originX, originY := l.qrX+sw+sw/2, l.qrY+sw+sw/2
	g := make(qrGrid, dim)
	for i := range g {
		g[i] = make([]bool, dim)
	}
	lo := bezier.Pt(l.qrX, l.qrY)
	hi := bezier.Pt(l.qrX+l.qrSize, l.qrY+l.qrSize)
	engraved := 0
	for _, k := range passphrasePoints(t, plate) {
		if !k.Engrave || !inRect(k.P, lo, hi) {
			continue
		}
		x, okx := qrModuleIndex(k.P.X-originX, pitch, sw)
		y, oky := qrModuleIndex(k.P.Y-originY, pitch, sw)
		if !okx || !oky || x < 0 || x >= dim || y < 0 || y >= dim {
			t.Fatalf("engraved knot %v inside the QR box is not on a module centre", k.P)
		}
		g[y][x] = true
		engraved++
	}
	if engraved == 0 {
		t.Fatal("nothing engraved inside the QR box")
	}
	return g
}

// Two variants of the secret are in flight: the RAW string (QR, confirm screen)
// and the MARK-TRANSLATED one (engraver). Swapping them either engraves
// invisible real spaces or QR-encodes the control codepoint, which a scanner
// hands to a wallet as different bytes. Silent either way.
func TestPassphraseQRIsByteExact(t *testing.T) {
	cases := []struct{ name, in string }{
		{"simple", "hunter2"},
		{"words", "correct horse battery staple"},
		{"leading", " leading"},
		{"trailing", "trailing "},
		{"double", "double  space"},
		{"only-spaces", "   "},
		{"all-printable", allPrintableASCII()},
		{"max-length", strings.Repeat("a", 100)},
		{"max-length-spaces", strings.Repeat("a b", 33) + "a"},
		// Alphanumeric and numeric mode: qr.Encode picks the smallest
		// encoding, so these take different code paths to the same bytes.
		{"alphanumeric", "CORRECT HORSE BATTERY STAPLE"},
		{"alphanumeric-odd", "HUNTER2"},
		{"numeric", "31415926535897932384626433832795"},
		{"numeric-odd", "3141592653589793238462643383279"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			plate := Passphrase{Passphrase: tc.in, QR: true, Font: constant.Font}
			out := decodeQR(t, passphraseQRGrid(t, plate))
			if out != tc.in {
				t.Errorf("QR round-trip changed the passphrase:\n in: %q\nout: %q", tc.in, out)
			}
			if strings.ContainsRune(out, SpaceMark) {
				t.Errorf("QR encoded the visible-space mark instead of a real space: %q", out)
			}
		})
	}
}

// passphraseDim is the module count of the plate's QR.
func passphraseDim(t *testing.T, plate Passphrase) int {
	t.Helper()
	code, err := passphraseQRCode(plate)
	if err != nil {
		t.Fatal(err)
	}
	return code.Size
}

func TestPassphraseQRLayoutFits(t *testing.T) {
	p := Passphrase{Passphrase: strings.Repeat("a", 100), QR: true, Font: constant.Font}
	pts := passphrasePoints(t, p)
	lo := bezier.Pt(params.F(innerMargin), params.F(innerMargin))
	hi := bezier.Pt(params.F(85-innerMargin), params.F(85-innerMargin))
	for _, k := range pts {
		if !inRect(k.P, lo, hi) {
			t.Fatalf("knot %v outside the usable area [%v, %v]", k.P, lo, hi)
		}
	}
	l := passphraseLayoutFor(params, p, passphraseDim(t, p))
	if l.rows != 5 || l.rowLen != passphraseRowLenQR {
		t.Errorf("QR layout is %d rows x %d, want 5 x %d", l.rows, l.rowLen, passphraseRowLenQR)
	}
	if l.em != params.F(passphraseFontSizeQR) {
		t.Errorf("QR layout em = %d, want %d", l.em, params.F(passphraseFontSizeQR))
	}
	// The text block must end before the QR envelope begins.
	if l.textY+l.blockH > l.envY {
		t.Errorf("text block (ends %d) overlaps the QR envelope (starts %d)", l.textY+l.blockH, l.envY)
	}
}

// The QR size is variable and the layout must not assume the worst case: the
// envelope is reserved for 37 modules, and a smaller code is centred in it
// without moving anything else.
func TestPassphraseQRSizeVariable(t *testing.T) {
	// Same length, different encodings. qr.Encode picks the smallest: the
	// alphanumeric subset packs 100 characters into version 4 (33 modules),
	// byte mode needs version 5 (37).
	cases := []struct {
		name    string
		in      string
		wantDim int
	}{
		{"alphanumeric-33", strings.Repeat("A", 100), 33},
		{"byte-37", strings.Repeat("a", 100), 37},
		{"short-21", "hunter2", 21},
	}
	var prevText bezier.Point
	for i, tc := range cases {
		plate := Passphrase{Passphrase: tc.in, QR: true, Font: constant.Font}
		dim := passphraseDim(t, plate)
		if dim != tc.wantDim {
			t.Errorf("%s: QR is %d modules, want %d", tc.name, dim, tc.wantDim)
		}
		l := passphraseLayoutFor(params, plate, dim)
		if l.envSize != passphraseQREnvelope*params.StrokeWidth*passphraseQRScale {
			t.Errorf("%s: envelope %d is not the reserved %d modules", tc.name, l.envSize, passphraseQREnvelope)
		}
		// The code sits inside the reserved envelope, centred.
		if l.qrX < l.envX || l.qrX+l.qrSize > l.envX+l.envSize ||
			l.qrY < l.envY || l.qrY+l.qrSize > l.envY+l.envSize {
			t.Errorf("%s: %d-module code at (%d,%d)+%d escapes its envelope (%d,%d)+%d",
				tc.name, dim, l.qrX, l.qrY, l.qrSize, l.envX, l.envY, l.envSize)
		}
		if got, want := l.qrX-l.envX, (l.envSize-l.qrSize)/2; got != want {
			t.Errorf("%s: code is %d from the envelope's left edge, want %d (centred)", tc.name, got, want)
		}
		// The text block's left edge does not move with the QR size.
		if i > 0 && l.textX != prevText.X {
			t.Errorf("%s: text block moved to x=%d from x=%d when the QR resized", tc.name, l.textX, prevText.X)
		}
		prevText = bezier.Pt(l.textX, l.textY)

		// And the whole plate still lays out inside the usable area.
		for _, k := range passphrasePoints(t, plate) {
			lo := bezier.Pt(params.F(innerMargin), params.F(innerMargin))
			hi := bezier.Pt(params.F(85-innerMargin), params.F(85-innerMargin))
			if !inRect(k.P, lo, hi) {
				t.Fatalf("%s: knot %v outside the usable area", tc.name, k.P)
			}
		}
	}
}

// The QR must be engraved by ConstantQR, whose plan depends only on the module
// count. engrave.QR's plan follows the modules themselves, so its timing would
// disclose the secret (spec 3.5.2). These two passphrases have identical length
// and identical per-row character multisets, so the text contributes the same
// timing; only the QR's content differs.
func TestPassphraseQRTimingIsContentIndependent(t *testing.T) {
	a := Passphrase{Passphrase: strings.Repeat("ab", 50), QR: true, Font: constant.Font}
	b := Passphrase{Passphrase: strings.Repeat("ba", 50), QR: true, Font: constant.Font}
	if passphraseDim(t, a) != passphraseDim(t, b) {
		t.Fatal("test is void: the two QRs are different sizes")
	}
	pa := engrave.ProfileSpline(engrave.PlanEngraving(conf, passphraseEngraving(t, a)))
	pb := engrave.ProfileSpline(engrave.PlanEngraving(conf, passphraseEngraving(t, b)))
	if !pa.Equal(pb) {
		t.Errorf("QR timing depends on content:\n%+v\n%+v", pa, pb)
	}
}

// passphraseRegion is one rectangle of the plate: a metadata band, the text
// block or the QR envelope. Every engraved point must land in exactly one.
type passphraseRegion struct {
	name     string
	lo, hi   bezier.Point
	required bool
}

// passphraseRegions derives the plate's regions from the layout the engraver
// itself uses, so a test cannot assert geometry the engraver does not honour.
func passphraseRegions(t *testing.T, plate Passphrase) []passphraseRegion {
	t.Helper()
	dim := 0
	if plate.QR {
		dim = passphraseDim(t, plate)
	}
	l := passphraseLayoutFor(params, plate, dim)
	plateX := params.F(85)
	var rs []passphraseRegion
	if n := len(l.topLines); n > 0 {
		rs = append(rs, passphraseRegion{
			name: "top band",
			lo:   bezier.Pt(0, l.topY), hi: bezier.Pt(plateX, l.topY+n*l.smallEm),
			required: true,
		})
	}
	rs = append(rs, passphraseRegion{
		name: "text block",
		lo:   bezier.Pt(l.textX, l.textY), hi: bezier.Pt(l.textX+l.blockW, l.textY+l.blockH),
		required: true,
	})
	if l.qrDim > 0 {
		rs = append(rs, passphraseRegion{
			name: "qr envelope",
			lo:   bezier.Pt(l.envX, l.envY), hi: bezier.Pt(l.envX+l.envSize, l.envY+l.envSize),
			required: true,
		})
	}
	if n := len(l.bottomLines); n > 0 {
		rs = append(rs, passphraseRegion{
			name: "bottom band",
			lo:   bezier.Pt(0, l.bottomY), hi: bezier.Pt(plateX, l.bottomY+n*l.smallEm),
			required: true,
		})
	}
	return rs
}

// The partial case -- text only, no metadata -- is what let the metadata
// overflow go unnoticed twice during specification, so this is the full one:
// 100 characters with spaces, both fingerprints, the QR, the legend and the
// footer, all at once.
func TestPassphrasePlateWorstCase(t *testing.T) {
	p := Passphrase{
		Passphrase: strings.Repeat("a b", 33) + "a", // 100 chars, spaces -> legend required
		SeedFP:     "A1B2C3D4",
		CombinedFP: "5E6F7A8B",
		QR:         true,
		Font:       constant.Font,
	}
	if got := len(p.Passphrase); got != 100 {
		t.Fatalf("test is void: passphrase is %d characters, want 100", got)
	}
	dim := passphraseDim(t, p)
	l := passphraseLayoutFor(params, p, dim)

	// Spec 4.3: at most two lines per 10mm band. A band offers innerMargin 10
	// minus outerMargin 3 = 7mm; three 3mm lines need 9 and run off the plate.
	if n := len(l.topLines); n != 2 {
		t.Errorf("top band has %d lines, want 2 in the worst case: %q", n, l.topLines)
	}
	if n := len(l.bottomLines); n != 2 {
		t.Errorf("bottom band has %d lines, want 2 in the worst case: %q", n, l.bottomLines)
	}

	regions := passphraseRegions(t, p)
	// The regions must be disjoint on Y and in top-to-bottom order: that is
	// what "blocks do not overlap" means on this plate.
	for i := 1; i < len(regions); i++ {
		if regions[i-1].hi.Y > regions[i].lo.Y {
			t.Errorf("%s (ends %d) overlaps %s (starts %d)",
				regions[i-1].name, regions[i-1].hi.Y, regions[i].name, regions[i].lo.Y)
		}
	}
	// The bands live in the 10mm margins; the text and QR live between them.
	band, content := params.F(outerMargin), params.F(innerMargin)
	for _, r := range regions {
		switch r.name {
		case "top band":
			if r.lo.Y < band || r.hi.Y > content {
				t.Errorf("top band [%d,%d] escapes the margin [%d,%d]", r.lo.Y, r.hi.Y, band, content)
			}
		case "bottom band":
			if r.lo.Y < params.F(85-innerMargin) || r.hi.Y > params.F(85-outerMargin) {
				t.Errorf("bottom band [%d,%d] escapes the margin [%d,%d]",
					r.lo.Y, r.hi.Y, params.F(85-innerMargin), params.F(85-outerMargin))
			}
		default:
			if r.lo.Y < content || r.hi.Y > params.F(85-innerMargin) {
				t.Errorf("%s [%d,%d] escapes the usable area [%d,%d]",
					r.name, r.lo.Y, r.hi.Y, content, params.F(85-innerMargin))
			}
		}
	}

	// Every engraved point lands in exactly one region, and never in a corner
	// screw-hole band.
	seen := make(map[string]int)
	for _, k := range passphrasePoints(t, p) {
		hits := 0
		for _, r := range regions {
			if inRect(k.P, r.lo, r.hi) {
				hits++
				seen[r.name]++
			}
		}
		if hits != 1 {
			t.Fatalf("knot %v lands in %d regions, want exactly 1", k.P, hits)
		}
		inXBand := k.P.X < content || k.P.X > params.F(85-innerMargin)
		inYBand := k.P.Y < content || k.P.Y > params.F(85-innerMargin)
		if inXBand && inYBand {
			t.Fatalf("knot %v lands in a corner screw-hole band", k.P)
		}
	}
	for _, r := range regions {
		if r.required && seen[r.name] == 0 {
			t.Errorf("nothing engraved in the %s", r.name)
		}
	}
}

// Spec 4.3: no metadata line may exceed 64mm. The longest, FINGERPRINTS TYPED,
// NOT VERIFIED, is 32 characters x the 2.0mm advance at the 3mm em, and clears
// the 10mm corner screw-hole bands by 0.5mm on each side.
func TestPassphraseBandBudget(t *testing.T) {
	plates := []Passphrase{
		{Passphrase: "hunter2", Font: constant.Font},
		{Passphrase: "a b", Font: constant.Font},
		{Passphrase: "a b", SeedFP: "A1B2C3D4", Font: constant.Font},
		{Passphrase: "a b", CombinedFP: "5E6F7A8B", Font: constant.Font},
		{Passphrase: strings.Repeat("a b", 33) + "a", SeedFP: "FFFFFFFF", CombinedFP: "00000000",
			QR: true, Font: constant.Font},
		// S6b GATE 2.3: the preloaded (derived+policy-id) footer, 25
		// characters (the P3 measured correction -- see
		// passphraseFooterDerivedSuffix's doc in passphrase.go; R-H's
		// literal 36-character form does NOT clear this budget, which is
		// the finding that forced the correction) -- must clear the same
		// 64mm/2-line budget as the typed one.
		{Passphrase: strings.Repeat("a b", 33) + "a", SeedFP: "FFFFFFFF", CombinedFP: "00000000",
			PolicyID: "1A2B3C4D", Derived: true, QR: true, Font: constant.Font},
	}
	for _, p := range plates {
		dim := 0
		if p.QR {
			dim = passphraseDim(t, p)
		}
		l := passphraseLayoutFor(params, p, dim)
		for _, band := range [][]string{l.topLines, l.bottomLines} {
			if len(band) > 2 {
				t.Errorf("%q: band has %d lines, want at most 2: %q", p.Passphrase, len(band), band)
			}
			for _, line := range band {
				w, _ := engrave.String(p.Font, l.smallEm, line).Measure()
				if w > params.F(64) {
					t.Errorf("metadata line %q is %d wide, over the %d budget", line, w, params.F(64))
				}
			}
		}
	}
}

// The legend is required whenever the passphrase contains a space, and must not
// appear otherwise: without it the mark is a private convention documented
// nowhere the reader will be.
func TestLegendPresence(t *testing.T) {
	cases := []struct{ in string }{
		{"hunter2"}, {"a b"}, {" leading"}, {"trailing "}, {"a  b"}, {"   "},
		{"no_spaces_at_all"}, {"correct horse battery staple"},
	}
	legend := string(SpaceMark) + " = SPACE"
	for _, tc := range cases {
		l := passphraseLayoutFor(params, Passphrase{Passphrase: tc.in, Font: constant.Font}, 0)
		got := slices.Contains(l.bottomLines, legend)
		want := strings.ContainsRune(tc.in, ' ')
		if got != want {
			t.Errorf("%q: legend present = %v, want %v (lines %q)", tc.in, got, want, l.bottomLines)
		}
	}
}

// Fingerprints are engraved grouped 4-and-4, are omitted entirely when blank,
// and the separator is a plain space, NEVER the visible mark: the mark means "a
// literal space in the passphrase".
func TestPassphraseFingerprintLines(t *testing.T) {
	footer := "FINGERPRINTS TYPED, NOT VERIFIED"
	cases := []struct {
		seed, comb string
		wantTop    []string
		wantFooter bool
	}{
		{"", "", nil, false},
		{"A1B2C3D4", "", []string{"SEED FP: A1B2 C3D4"}, true},
		{"", "5E6F7A8B", []string{"EXPECTED COMB FP: 5E6F 7A8B"}, true},
		{"A1B2C3D4", "5E6F7A8B", []string{"SEED FP: A1B2 C3D4", "EXPECTED COMB FP: 5E6F 7A8B"}, true},
	}
	for _, tc := range cases {
		p := Passphrase{Passphrase: "hunter2", SeedFP: tc.seed, CombinedFP: tc.comb, Font: constant.Font}
		l := passphraseLayoutFor(params, p, 0)
		if !slices.Equal(l.topLines, tc.wantTop) {
			t.Errorf("seed=%q comb=%q: top band %q, want %q", tc.seed, tc.comb, l.topLines, tc.wantTop)
		}
		for _, line := range l.topLines {
			if strings.ContainsRune(line, SpaceMark) {
				t.Errorf("fingerprint line %q uses the visible-space mark as a separator", line)
			}
			if !strings.Contains(line, " ") {
				t.Errorf("fingerprint line %q has no 4-and-4 grouping", line)
			}
		}
		if got := slices.Contains(l.bottomLines, footer); got != tc.wantFooter {
			t.Errorf("seed=%q comb=%q: footer present = %v, want %v", tc.seed, tc.comb, got, tc.wantFooter)
		}
	}
}

// glyphInk is one glyph's engraved outline relative to its slot, produced by
// the real plate path.
func glyphInk(t *testing.T, r rune) []bezier.Point {
	t.Helper()
	ink := passphraseTextInk(t, Passphrase{Passphrase: string(r), Font: constant.Font})
	if len(ink) == 0 {
		t.Fatalf("glyph %#x engraves nothing", r)
	}
	return ink
}

// Leading, trailing, interior and repeated spaces each produce the
// corresponding count of visible marks. The expectation is rebuilt from the RAW
// passphrase here -- "space becomes the mark, everything else is verbatim" --
// rather than from passphraseGlyphs, so the test does not assume the
// substitution it is checking.
func TestSpaceFidelity(t *testing.T) {
	cases := []struct {
		in    string
		marks int
	}{
		{"a b", 1},
		{" leading", 1},
		{"trailing ", 1},
		{"a  b", 2},
		{"   ", 3},
		{"a b  c   d", 6},
		{"Mixed Case Kept", 2},
		{strings.Repeat("a b", 33) + "a", 33},
	}
	cache := map[rune][]bezier.Point{}
	ink := func(r rune) []bezier.Point {
		if v, ok := cache[r]; ok {
			return v
		}
		cache[r] = glyphInk(t, r)
		return cache[r]
	}
	for _, tc := range cases {
		plate := Passphrase{Passphrase: tc.in, Font: constant.Font}
		l := passphraseLayoutFor(params, plate, 0)
		adv := passphraseAdvance(constant.Font, l.em)
		var want []bezier.Point
		marks := 0
		for i, c := range []rune(tc.in) {
			if c == ' ' {
				c = SpaceMark
				marks++
			}
			off := bezier.Pt((i%l.rowLen)*adv, (i/l.rowLen)*l.em)
			for _, p := range ink(c) {
				want = append(want, p.Add(off))
			}
		}
		if marks != tc.marks {
			t.Fatalf("test is void: %q has %d spaces, expected %d", tc.in, marks, tc.marks)
		}
		got := passphraseTextInk(t, plate)
		if !slices.Equal(got, want) {
			t.Errorf("%q: the engraved text block is not the mark-substituted glyph sequence (%d vs %d ink points)",
				tc.in, len(got), len(want))
		}
	}
}

func TestPassphraseGolden(t *testing.T) {
	const words = "correct horse battery staple"
	tests := []struct {
		name  string
		plate Passphrase
	}{
		{"0-plain", Passphrase{
			Passphrase: words,
			SeedFP:     "A1B2C3D4",
			CombinedFP: "5E6F7A8B",
			Font:       constant.Font,
		}},
		{"1-qr", Passphrase{
			Passphrase: words,
			SeedFP:     "A1B2C3D4",
			CombinedFP: "5E6F7A8B",
			QR:         true,
			Font:       constant.Font,
		}},
		{"2-no-metadata", Passphrase{
			// No space, no fingerprints: neither band is engraved.
			Passphrase: "Hunter2!",
			Font:       constant.Font,
		}},
		{"3-max-qr", Passphrase{
			// The worst case the plate must survive.
			Passphrase: strings.Repeat("a b", 33) + "a",
			SeedFP:     "FFFFFFFF",
			CombinedFP: "00000000",
			QR:         true,
			Font:       constant.Font,
		}},
		// S6b (GATE 2.3/2.3c/R-H, with the P3 measured correction -- see
		// passphraseFooterDerivedSuffix): the PRELOADED path. Both
		// fingerprints were DERIVED by the device, not typed, so the
		// footer carries the policy id instead of the typed-provenance
		// string -- "POLICY 1A2B 3C4D  DERIVED", not "FINGERPRINTS TYPED,
		// NOT VERIFIED". This is a NEW golden (R-G: "marked states get new
		// golden files"), not a rewrite of 0-plain/1-qr/3-max-qr above --
		// those three keep Derived at its zero value (false) and stay
		// byte-identical, pinning that the untouched typed/standalone path
		// (R-H: "standalone path ... unchanged") is still reachable and
		// still renders the old 32-character string.
		{"4-preloaded", Passphrase{
			Passphrase: words,
			SeedFP:     "A1B2C3D4",
			CombinedFP: "5E6F7A8B",
			PolicyID:   "1A2B3C4D",
			Derived:    true,
			QR:         true,
			Font:       constant.Font,
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			compareGolden(t, "passphrase-"+test.name, passphraseEngraving(t, test.plate))
		})
	}
}

// Every string ValidatePassphrase accepts must lay out through the REAL entry
// point without panicking -- not through engrave.String in isolation, which is
// what made an earlier draft's guarantee hollow. This exercises the passphrase
// ConstantStringer and, with QR on, ConstantQR.
func TestPassphraseNoPanicOverCharset(t *testing.T) {
	var cases []string
	for r := rune(0x20); r <= 0x7E; r++ {
		// Each character on its own, and padded out to a full plate so every
		// row length from 1 to rowLen is exercised.
		cases = append(cases, string(r), strings.Repeat(string(r), 100))
	}
	cases = append(cases, allPrintableASCII(), strings.Repeat(allPrintableASCII(), 2)[:100])
	for _, in := range cases {
		if err := passphrase.ValidatePassphrase(in); err != nil {
			t.Fatalf("test is void: %q is not a valid passphrase: %v", in, err)
		}
		for _, withQR := range []bool{false, true} {
			plate := Passphrase{Passphrase: in, SeedFP: "A1B2C3D4", CombinedFP: "5E6F7A8B",
				QR: withQR, Font: constant.Font}
			eng, err := EngravePassphrase(params, plate)
			if err != nil {
				t.Fatalf("%q (qr=%v): %v", in, withQR, err)
			}
			// Drain through the PLANNER, not the raw command stream.
			// The panic class spec 3.4 checks 7-8 names as the dangerous one --
			// unaligned delay, scale already in effect, delay during spline --
			// fires inside PlanEngraving, so iterating `eng` alone stops one
			// layer ABOVE the panics this test exists to catch. Proven by the
			// whole-feature review: a plan-time defect injected for '=' left
			// this test, the whole backup suite AND the whole gui suite green.
			for range engrave.PlanEngraving(params.StepperConfig, eng) {
			}
		}
	}
}

// A passphrase too long for a constant-time QR must return an error, never
// panic and never fall back to the content-timed engraver.
func TestPassphraseQRTooLong(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("EngravePassphrase panicked instead of returning an error: %v", r)
		}
	}()
	plate := Passphrase{Passphrase: strings.Repeat("a", 200), QR: true, Font: constant.Font}
	if _, err := EngravePassphrase(params, plate); err == nil {
		t.Error("want an error for a QR beyond ConstantQR's reach, got nil")
	}
	// Without the QR the same passphrase still lays out: the limit is the QR's,
	// not the plate's.
	if _, err := EngravePassphrase(params, Passphrase{Passphrase: plate.Passphrase, Font: constant.Font}); err != nil {
		t.Errorf("no-QR plate returned %v", err)
	}
}

// TestPassphraseQRIgnoresFingerprints pins the rule that the QR carries the
// passphrase and NOTHING else. A QR that swept in the fingerprints would put
// two fields the user was warned are unverified -- and which they typed from
// memory -- into a machine-readable blob that any scanner will trust, on a
// plate whose whole point is that the passphrase is the secret.
//
// This is asserted at MODULE level rather than by decoding, because a decoder
// that ignored trailing data would pass while the modules differed. Identical
// module grids is the strongest available statement: the fingerprints changed
// nothing about what gets engraved.
// TestPassphraseQRIgnoresPolicyID is GATE 2.5's S6b extension:
// TestPassphraseQRIgnoresFingerprints already pins that the QR carries the
// passphrase and nothing else; this proves the same of the two fields P3
// adds, PolicyID and Derived. Same module-level comparison, for the same
// reason: a decoder that ignored trailing data would pass while the
// modules differed.
func TestPassphraseQRIgnoresPolicyID(t *testing.T) {
	const secret = "correct horse battery staple"
	base, err := passphraseQRCode(Passphrase{Passphrase: secret})
	if err != nil {
		t.Fatal(err)
	}
	got, err := passphraseQRCode(Passphrase{
		Passphrase: secret, SeedFP: "DEADBEEF", CombinedFP: "CAFEBABE",
		PolicyID: "1A2B3C4D", Derived: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Size != base.Size {
		t.Fatalf("QR size changed from %d to %d -- the policy id or provenance is being encoded", base.Size, got.Size)
	}
	diff := 0
	for y := 0; y < base.Size; y++ {
		for x := 0; x < base.Size; x++ {
			if base.Black(x, y) != got.Black(x, y) {
				diff++
			}
		}
	}
	if diff != 0 {
		t.Errorf("%d of %d QR modules differ from the passphrase-only code -- "+
			"the policy id/provenance is reaching the QR, which must carry the passphrase alone",
			diff, base.Size*base.Size)
	}
}

// TestPassphraseFooterProvenance is S6b GATES 2.3, 2.3b, 2.3c and 2.3d.
//
// R-D/spec 2.3d: the footer TEXT is selected by the recorded PROVENANCE
// (Derived), never by whether PolicyID is set. "typed, policy id present"
// is the case that would fail if the code were keyed on PolicyID != ""
// instead: a future path preloading fingerprints without a descriptor must
// not print "DERIVED" over values it did not derive, and conversely a
// caller that happens to hold a policy id without having derived anything
// must not have it silently annexed onto the typed footer (which is also
// R-H's "never co-occur" requirement -- the two forms are asserted mutually
// exclusive below, in every case).
//
// 2.3c: "derived, policy id present" proves the policy id actually RENDERS
// -- the footer line is gated on a non-empty fingerprint
// (SeedFP/CombinedFP), an ORTHOGONAL condition that could vanish it
// silently if this case were only asserted negatively.
//
// 2.3b: "typed, no policy id" is the byte-for-byte pre-S6b behaviour --
// fingerprintEntryFlow's own path -- and must still say "TYPED, NOT
// VERIFIED", never "DERIVED".
func TestPassphraseFooterProvenance(t *testing.T) {
	const (
		seedFP = "A1B2C3D4"
		combFP = "5E6F7A8B"
		policy = "1A2B3C4D"
	)
	const (
		typedFooter = "FINGERPRINTS TYPED, NOT VERIFIED"
		// 25 chars, NOT R-H's literal 36-char "...DERIVED, NOT TYPED": see
		// the P3 measured-correction comment on passphraseFooterDerivedSuffix
		// in passphrase.go. R-H's string measures 460800 units against this
		// same band's pre-existing 409600-unit (64mm) cap -- TestPassphraseBandBudget
		// below is the gate that caught it.
		derivedFooter = "POLICY 1A2B 3C4D  DERIVED"
	)
	if got := len(derivedFooter); got != 25 {
		t.Fatalf("test is void: the derived footer literal is %d chars, want 25", got)
	}
	if got := len(typedFooter); got != 32 {
		t.Fatalf("test is void: the typed footer literal is %d chars, want 32", got)
	}

	cases := []struct {
		name       string
		derived    bool
		policyID   string
		wantFooter string
	}{
		{"typed, no policy id", false, "", typedFooter},
		{"typed, policy id present (must be ignored, 2.3d)", false, policy, typedFooter},
		{"derived, policy id present (must render, 2.3c)", true, policy, derivedFooter},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := Passphrase{
				Passphrase: "hunter2",
				SeedFP:     seedFP,
				CombinedFP: combFP,
				PolicyID:   tc.policyID,
				Derived:    tc.derived,
				Font:       constant.Font,
			}
			l := passphraseLayoutFor(params, p, 0)
			if !slices.Contains(l.bottomLines, tc.wantFooter) {
				t.Errorf("bottom band %q does not contain %q", l.bottomLines, tc.wantFooter)
			}
			other := typedFooter
			if tc.wantFooter == typedFooter {
				other = derivedFooter
			}
			if slices.Contains(l.bottomLines, other) {
				t.Errorf("bottom band %q ALSO contains %q -- the typed and derived forms must never co-occur (R-H)", l.bottomLines, other)
			}
		})
	}
}

func TestPassphraseQRIgnoresFingerprints(t *testing.T) {
	const secret = "correct horse battery staple"
	base, err := passphraseQRCode(Passphrase{Passphrase: secret})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name               string
		seedFP, combinedFP string
	}{
		{"seed only", "DEADBEEF", ""},
		{"combined only", "", "CAFEBABE"},
		{"both", "DEADBEEF", "CAFEBABE"},
		{"both, different values", "01234567", "89ABCDEF"},
	} {
		got, err := passphraseQRCode(Passphrase{
			Passphrase: secret, SeedFP: tc.seedFP, CombinedFP: tc.combinedFP,
		})
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if got.Size != base.Size {
			t.Fatalf("%s: QR size changed from %d to %d -- the fingerprints are "+
				"being encoded", tc.name, base.Size, got.Size)
		}
		diff := 0
		for y := 0; y < base.Size; y++ {
			for x := 0; x < base.Size; x++ {
				if base.Black(x, y) != got.Black(x, y) {
					diff++
				}
			}
		}
		if diff != 0 {
			t.Errorf("%s: %d of %d QR modules differ from the passphrase-only code -- "+
				"the fingerprints are reaching the QR, which must carry the passphrase alone",
				tc.name, diff, base.Size*base.Size)
		}
	}
}
