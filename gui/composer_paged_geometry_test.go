package gui

import (
	"image"
	"image/color"
	"slices"
	"testing"

	"seedhammer.com/gui/assets"
	"seedhammer.com/gui/op"
	"seedhammer.com/gui/widget"
	"seedhammer.com/image/rgb565"
	"seedhammer.com/md"
)

// ─── W-3: no glyph may be drawn under a navigation button ────────────────────
//
// composerPageLines wrapped every line at `dims.X - 2*8` and centred it across
// the WHOLE panel, while layoutNavigation places Back / page / take in a column
// at `dims.X - NavBtnPrimary.width`. So any line whose measured width came near
// the wrap bound reached under a button, and the operator lost its tail. Found
// on the S4 emulator shots (c06-stub-p0.png, c10-stub2-p0.png, k02-stub-p0.png):
// `Template-ID: 531ab9e1777f018ae53694387dd0d128` lost its 32nd hex digit under
// Back, and the key-less arm's `mk encode` lines lost their tails under the
// pager.
//
// THIS IS A GEOMETRY TEST, AND IT HAS TO BE. The capture walked these screens
// and passed, because op.Drawer.ExtractText collects a glyph's rune wherever it
// lands -- under a button included. A text-presence assertion therefore reports
// a screen as complete while the operator cannot read it. So this rasterises the
// body ops the widget actually produced and looks for INK inside the button
// rectangles: the same thing an eye does, and the only check the shots'
// symptom could not slip past.
//
// The nav buttons are NOT drawn into the test frame. Only the body is, so any
// ink inside a button's rectangle is body ink by construction -- there is
// nothing else in the buffer it could be.

// navButtonRects returns the three navigation button rectangles, computed the
// way layoutNavigation computes them (gui/gui.go): a column at the right edge,
// at leadingSize, vertically centred, and one button-height above the bottom
// leadingSize band.
func navButtonRects(dims image.Point) []image.Rectangle {
	btn := assets.NavBtnPrimary.Bounds().Size()
	ys := []int{leadingSize, (dims.Y - btn.Y) / 2, dims.Y - leadingSize - btn.Y}
	var out []image.Rectangle
	for _, y := range ys {
		min := image.Pt(dims.X-btn.X, y)
		out = append(out, image.Rectangle{Min: min, Max: min.Add(btn)})
	}
	return out
}

// inkUnderNav rasterises one page's body ops and returns the button rectangles
// that received ink, with a sample point for the message.
//
// It draws into an rgb565 buffer exactly as op.Drawer.ExtractText does, so this
// sees what the panel is handed rather than what the layout intended.
func inkUnderNav(t *testing.T, ctx *Context, dims image.Point, lines []string, start int) (image.Rectangle, image.Point, bool) {
	t.Helper()
	body, shown, _ := composerPageLines(ctx, &descriptorTheme, dims, lines, start, -1)
	if shown == 0 {
		t.Fatalf("composerPageLines drew no row from index %d of %d", start, len(lines))
	}
	return inkUnderNavOps(t, dims, body)
}

// inkUnderNavOps is the scanner itself, taking ops rather than lines.
//
// SPLIT OUT SO THE PROBE'S OWN PROOF DOES NOT DEPEND ON THE THING UNDER TEST.
// It first went through composerPageLines with a 200-character unbroken token,
// and once the fix landed that token no longer reached a button -- so the proof
// started passing for the same reason the gate did, and proved nothing about the
// scanner. Handed an op placed under a button directly, it stays a proof.
func inkUnderNavOps(t *testing.T, dims image.Point, body []op.Op) (image.Rectangle, image.Point, bool) {
	t.Helper()
	r := image.Rectangle{Max: dims}
	fb := rgb565.New(r)
	maskfb := image.NewAlpha(r)
	d := new(op.Drawer)
	d.Draw(fb, maskfb, op.Layer(body...))

	// The buffer starts zeroed, so "not the zero pixel" is ink. Sampling the
	// corner rather than assuming a colour keeps this true if the buffer's zero
	// value ever changes.
	blank := fb.At(0, 0)
	for _, nav := range navButtonRects(dims) {
		for y := nav.Min.Y; y < nav.Max.Y && y < dims.Y; y++ {
			for x := nav.Min.X; x < nav.Max.X && x < dims.X; x++ {
				if !sameColor(fb.At(x, y), blank) {
					return nav, image.Pt(x, y), true
				}
			}
		}
	}
	return image.Rectangle{}, image.Point{}, false
}

// rasterInk draws `o` into an rgb565 buffer, as op.Drawer.ExtractText does, and
// returns which pixels carry ink. Shared with composer_digitpad_layout_test.go:
// both gates ask what the PANEL is handed rather than what a layout intended.
func rasterInk(dims image.Point, o op.Op) [][]bool {
	r := image.Rectangle{Max: dims}
	fb := rgb565.New(r)
	maskfb := image.NewAlpha(r)
	d := new(op.Drawer)
	d.Draw(fb, maskfb, o)
	blank := fb.At(0, 0)
	ink := make([][]bool, dims.Y)
	for y := 0; y < dims.Y; y++ {
		ink[y] = make([]bool, dims.X)
		for x := 0; x < dims.X; x++ {
			ink[y][x] = !sameColor(fb.At(x, y), blank)
		}
	}
	return ink
}

func sameColor(a, b color.Color) bool {
	ar, ag, ab, aa := a.RGBA()
	br, bg, bb, ba := b.RGBA()
	return ar == br && ag == bg && ab == bb && aa == ba
}

// whichLineIntersects names the offending line, so the failure says WHAT is
// unreadable rather than only that something is. It re-lays each line alone,
// which is exactly what composerPageLines does for it in the real page.
func whichLineIntersects(t *testing.T, ctx *Context, dims image.Point, lines []string) []string {
	t.Helper()
	var bad []string
	for _, l := range lines {
		if l == "" {
			continue
		}
		if _, _, hit := inkUnderNav(t, ctx, dims, []string{l}, 0); hit {
			bad = append(bad, l)
		}
	}
	return bad
}

// composerPagedScreens returns the paged bodies this test covers, named.
func composerPagedScreens(t *testing.T) map[string][]string {
	t.Helper()
	out := map[string][]string{}

	// The KEYED stub screen, both stubs present, built by the production
	// function from the S4 fixture's own shape: wsh, 2-of-2 plus a 1-key path.
	keyed := md.PathList{Wrapper: md.ComposeWsh}
	keyed.Paths = append(keyed.Paths, md.SpendPath{Keys: &md.KeySet{K: 2, N: 2}})
	keyed.Paths = append(keyed.Paths, md.SpendPath{Keys: &md.KeySet{K: 1, N: 1}})
	c, err := md.Compose(keyed)
	if err != nil {
		t.Fatal(err)
	}
	chunks, err := c.Chunks()
	if err != nil {
		t.Fatal(err)
	}
	lines, err := composerStubLines(chunks, nil, composerStubIdMoved)
	if err != nil {
		t.Fatal(err)
	}
	out["keyed stub"] = lines

	// The KEY-LESS arm's stub screen: tr, one 2-of-3 path -- §12 item 3's shape
	// and the one the operator cuts.
	keyless := md.PathList{Wrapper: md.ComposeTr}
	keyless.Paths = append(keyless.Paths, md.SpendPath{Keys: &md.KeySet{K: 2, N: 3}})
	c2, err := md.Compose(keyless)
	if err != nil {
		t.Fatal(err)
	}
	chunks2, err := c2.Chunks()
	if err != nil {
		t.Fatal(err)
	}
	lines2, err := composerStubLines(chunks2, nil, composerStubUnchanged)
	if err != nil {
		t.Fatal(err)
	}
	out["keyless stub"] = lines2

	// The pick list at a payload size the composer plausibly meets, whose rows
	// are the longest the seating screen draws.
	rows := make([]string, 0, 34)
	for i := 0; i < 8; i++ {
		rows = append(rows, composerKeyLabel([4]byte{0x73, 0xc5, 0xda, byte(i)}, composerTestPath(i)))
	}
	rows = append(rows, "Type a seed", "Leave unseated")
	out["pick list"] = append([]string{composerCopySeatPrompt(2, 1, 2, 3), ""}, rows...)

	// H6 §8.3: the Done census, with every one of its four preimage states at
	// once -- an accepted plate, a declined one, one on no path, and the
	// apart-storage line. It joined this map when composerEngraveStep stopped
	// drawing the census through confirmReviewScreen's panel-wide wrap; five of
	// its rows are over the 374 px at which a panel-centred row reaches under
	// the navigation column.
	st := composerH6PlateState(t, "anchor a", "anchor b")
	hd, hm := composerH6Material("anchor d", true)
	composerHoldHashlockMaterial(st, hd, hm)
	plates := composerPreimagePlates(st)
	plates[0].choice = hashlockPlatePhraseQR
	plates[1].choice = hashlockPlateDecline
	out["done census"] = composerCensusLines(newPlatform().EngraverParams(),
		[]bundleCard{{kind: cardMD1, label: "md1 template", strings: []string{"md1abc"},
			summary: "key-less wallet policy"}}, plates)

	// And the pick screen step (A) draws, lead and rows, as composerPickScreen
	// composes them.
	_, pm := composerH6Material("anchor a", true)
	prows, _ := composerPreimagePlateRows(pm)
	out["preimage plate pick"] = append([]string{
		composerCopyPreimagePlateLead("b867db87..edbc96cb", 2, 100, "hardened", md.KindRipemd160), ""}, prows...)

	return out
}

// TestComposerPagedLinesNeverDrawUnderTheNavButtons is W-3's gate.
//
// EVERY PAGE, not only the first: paging changes which lines are laid out and
// the longest one may be on any of them.
func TestComposerPagedLinesNeverDrawUnderTheNavButtons(t *testing.T) {
	p := newPlatform()
	p.display = sh2DisplaySize
	ctx := NewContext(p)
	dims := sh2DisplaySize

	for name, lines := range composerPagedScreens(t) {
		start := 0
		for page := 0; start < len(lines); page++ {
			_, shown, _ := composerPageLines(ctx, &descriptorTheme, dims, lines, start, -1)
			if shown == 0 {
				t.Fatalf("%s: page %d drew no row, so this walk cannot terminate", name, page)
			}
			if nav, at, hit := inkUnderNav(t, ctx, dims, lines, start); hit {
				bad := whichLineIntersects(t, ctx, dims, lines[start:start+shown])
				t.Errorf("%s page %d: a line is drawn UNDER a navigation button.\n"+
					"  button %v received ink at %v\n"+
					"  the line(s) that reach under it: %q\n"+
					"The operator cannot read what a button covers, and ExtractText "+
					"collects the runes anyway -- which is why the emulator capture "+
					"passed while the screenshot showed the last hex digit missing (W-3).",
					name, page, nav, at, bad)
			}
			start += shown
		}
	}
}

// TestComposerPagedGeometryProbeCanSeeInk is the mutation proof for the probe.
//
// Without it an inkUnderNav that always returned false would make the gate above
// pass by looking at nothing -- the false-PASS shape this tree removes on sight.
// So the scanner is handed a label PLACED under the first button, which is a
// fact about the buffer and not about composerPageLines: it stays a proof after
// the fix, when no line the widget lays out reaches a button any more.
//
// It also asserts the negative: the same label at the left margin is NOT
// reported, so the scanner is not simply returning true.
func TestComposerPagedGeometryProbeCanSeeInk(t *testing.T) {
	p := newPlatform()
	p.display = sh2DisplaySize
	ctx := NewContext(p)
	dims := sh2DisplaySize
	navs := navButtonRects(dims)

	lbl, sz := widget.Label(&ctx.B, ctx.Styles.body, descriptorTheme.Text, "XXXX")
	under := navs[0].Min.Add(image.Pt(6, 6))
	nav, at, hit := inkUnderNavOps(t, dims, []op.Op{lbl.Offset(under)})
	if !hit {
		t.Fatalf("the scanner found no ink for a %v label drawn at %v, inside button %v -- "+
			"so it is not looking at the frame at all", sz, under, navs[0])
	}
	t.Logf("scanner sees ink: button %v at %v", nav, at)

	// The negative control: the same label, at the left margin, must NOT be
	// reported. A scanner that returned true unconditionally would pass the
	// half above and fail here.
	if _, _, hit := inkUnderNavOps(t, dims, []op.Op{lbl.Offset(image.Pt(8, 120))}); hit {
		t.Errorf("the scanner reports ink under a button for a label drawn at the left " +
			"margin, so it is not reading the button rectangles")
	}
}

// TestComposerPickerRowsShareALeftEdge pins the picker's row alignment: every
// option row on a page starts at the same x. On the §0b key-path screen the
// one-line NUMS row was centred on its own while the wrapped Liana row ran
// flush left, so the two options did not line up (operator report,
// 2026-09-23). Measured on the panel raster, not on the layout's intent.
func TestComposerPickerRowsShareALeftEdge(t *testing.T) {
	p := newPlatform()
	p.display = sh2DisplaySize
	ctx := NewContext(p)
	dims := sh2DisplaySize
	lead, rows := composerCopyUnspendableLead(), composerUnspendableRows()
	page := append([]string{lead, ""}, rows...)
	const rowBase = composerPickRowBase

	// alignFrom >= 0 measures the picker's real layout; -1 is the old
	// per-row centring, kept only as the control below.
	leftEdges := func(alignFrom int) []int {
		var body []op.Op
		var shown int
		if alignFrom >= 0 {
			body, shown, _ = composerPickPageLayout(ctx, &descriptorTheme, dims, lead, rows, -1)
		} else {
			body, shown, _ = composerPageLinesAligned(ctx, &descriptorTheme, dims, page, 0, -1, -1)
		}
		if shown < len(page) {
			t.Fatalf("the key-path page drew %d of %d lines; this test needs both rows on one page", shown, len(page))
		}
		var edges []int
		for i := rowBase; i < len(page); i++ {
			ink := rasterInk(dims, body[i])
			left := -1
			for y := range ink {
				for x, on := range ink[y] {
					if on && (left < 0 || x < left) {
						left = x
					}
				}
			}
			if left < 0 {
				t.Fatalf("row %d drew no ink", i-rowBase)
			}
			edges = append(edges, left)
		}
		return edges
	}
	// Glyph side bearings differ by a pixel or two between first letters.
	const slack = 3
	spread := func(e []int) int { return slices.Max(e) - slices.Min(e) }

	if got := leftEdges(rowBase); spread(got) > slack {
		t.Errorf("the key-path rows start at x=%v; they must share one left edge (within %dpx)", got, slack)
	}
	// The control: centring each row on its own is what the operator saw, and
	// this test must see it too, or it proves nothing.
	if got := leftEdges(-1); spread(got) <= slack {
		t.Errorf("control: per-row centring gave edges %v, the same edge; the test cannot tell the two layouts apart", got)
	}
}
