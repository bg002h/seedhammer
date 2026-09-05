package gui

import (
	"image"
	"testing"

	"seedhammer.com/gui/layout"
	"seedhammer.com/gui/op"
	"seedhammer.com/gui/widget"
)

// ─── H5 §3 (F-484): the phrase screen's lead stays inside the page band ──────
//
// W-3 is the whole argument, one screen further on. composerPageLines wrapped at
// `dims.X - 2*8` and centred on the WHOLE panel while the navigation column sits
// at `dims.X - NavBtnPrimary.width`, so text was drawn under a button and the
// operator lost its tail. hashlockPhraseFlow's lead was still laid out that way:
// measured 152 px of its ink inside the Back button's rectangle. Nothing was
// LOST -- the ink fell in the button's empty margin, not on its chip or glyph --
// which is precisely why no text assertion and no screenshot review found it.
//
// A GEOMETRY TEST, for W-3's own reason: op.Drawer.ExtractText collects a
// glyph's rune wherever it lands, under a button included, so every existing
// assertion about this screen passes either way.

// hashlockPhraseLeadTop is the y the production flow lays the lead at: the panel
// less the title band (hashlockPhraseFlow's `screen.CutTop(leadingSize)`).
func hashlockPhraseLeadTop(dims image.Point) int {
	screen := layout.Rectangle{Max: dims}
	_, content := screen.CutTop(leadingSize)
	return content.Min.Y
}

// TestHashlockPhraseLeadIsDrawnInsideTheBand is §3.2 (a) and (c).
//
// MUTATION: restore the panel-wide layout in hashlockPhraseLead
// (`widget.Labelw(..., dims.X-2*8, ...)` centred on the panel) -> (a) fails,
// naming the Back button and the pixel it was hit at.
// MUTATION: composerTextBand returning `dims.X` for the width -> (a) fails.
func TestHashlockPhraseLeadIsDrawnInsideTheBand(t *testing.T) {
	p := newPlatform()
	p.display = sh2DisplaySize
	ctx := NewContext(p)
	dims := sh2DisplaySize

	leadOp, leadSz := hashlockPhraseLead(ctx, &descriptorTheme, dims, hashlockPhraseLeadTop(dims))

	// (a) no lead ink inside any navigation button rectangle. Only the lead is
	// rasterised, so any ink found there is lead ink by construction -- the
	// buttons are not drawn into this buffer.
	if nav, at, hit := inkUnderNavOps(t, dims, []op.Op{leadOp}); hit {
		t.Errorf("the phrase screen's lead is drawn UNDER a navigation button.\n"+
			"  button %v received ink at %v\n"+
			"The operator cannot read what a button covers, and ExtractText collects "+
			"the runes anyway -- which is why every text assertion on this screen "+
			"passed while 152 px of the lead sat inside Back (F-484, W-3).", nav, at)
	}

	// (c) at most two lines. The single-line height is MEASURED at the same
	// style and band width rather than hardcoded, so this stays true if the
	// face or the band changes.
	left, width := composerTextBand(dims)
	_, one := widget.Labelw(&ctx.B, ctx.Styles.lead, width, descriptorTheme.Text, "X")
	if one.Y <= 0 {
		t.Fatalf("a one-line label measured %v; this test cannot count lines", one)
	}
	lines := (leadSz.Y + one.Y - 1) / one.Y
	t.Logf("band left=%d width=%d; lead %v = %d line(s) of %d px", left, width, leadSz, lines, one.Y)
	if lines > 2 {
		t.Errorf("the lead wraps to %d lines in the %d px band, over §3.2(c)'s two. "+
			"§3.3's fallback copy applies: \"This screen does the hashing. Use a phrase "+
			"you have never used anywhere else.\", with H2 §4.2 folded to it.", lines, width)
	}
}

// TestHashlockPhraseLeadGeometryProbeCanSeeInk is the mutation proof for the
// scanner, at THIS screen's band.
//
// composer_paged_geometry_test.go proves inkUnderNavOps can see ink; this proves
// the same for the y the lead is actually drawn at, which is the row the Back
// button occupies. Without it, a gate above that found nothing would be
// indistinguishable from a gate looking at the wrong band.
func TestHashlockPhraseLeadGeometryProbeCanSeeInk(t *testing.T) {
	p := newPlatform()
	p.display = sh2DisplaySize
	ctx := NewContext(p)
	dims := sh2DisplaySize
	navs := navButtonRects(dims)

	lbl, sz := widget.Label(&ctx.B, ctx.Styles.lead, descriptorTheme.Text, "XXXX")
	under := navs[0].Min.Add(image.Pt(6, 6))
	if _, at, hit := inkUnderNavOps(t, dims, []op.Op{lbl.Offset(under)}); !hit {
		t.Fatalf("the scanner found no ink for a %v label drawn at %v, inside button %v -- "+
			"so §3.2(a) above is looking at nothing", sz, under, navs[0])
	} else {
		t.Logf("scanner sees ink at %v", at)
	}
	// The lead's own top y, at the LEFT margin, must not be reported: this is
	// the negative control that says the scanner reads the button rectangles
	// rather than the whole row the lead sits on.
	if _, _, hit := inkUnderNavOps(t, dims, []op.Op{lbl.Offset(image.Pt(8, hashlockPhraseLeadTop(dims)))}); hit {
		t.Error("the scanner reports ink under a button for a label at the left margin")
	}
}

// TestHashlockPhraseScreenKeepsTheReadoutBudget is §3.2 (b): F-481 must not
// regress.
//
// The lead's height decides how much of the panel is left for the keyboard, and
// the keyboard's readout is what is clamped away when that runs short -- an 8 px
// CutBottom once left the budget at 11 px, one line needs 19, and every typed
// character vanished while the `show` key stayed live. A NARROWER band can make
// the lead taller, so the screen that fixes F-484 is exactly the screen that
// could re-break F-481.
//
// Measured on the LIVE screen: kbd.MaxHeight is what hashlockPhraseFlow set on
// the frame, and kbd.size[page] is the grid the keyboard built, so this is the
// budget PassphraseKeyboard.Layout actually divided by, not a re-derivation.
//
// MUTATION: restore `content, _ = content.CutBottom(8)` in hashlockPhraseFlow ->
// the budget drops below one line and this fails.
func TestHashlockPhraseScreenKeepsTheReadoutBudget(t *testing.T) {
	st := composerStateWithPaths(t, 1)
	var ret bool
	h := runComposerHashEdit(t, st, composerSessionWith(nil, nil), 0, &ret)
	h.mustReach("Type a hashlock phrase")
	h.tapRow(0, 3)
	h.mustReach("32-byte value")
	h.tapNav(Button3)
	h.mustReach("Hashlock phrase")
	typeOnPassphraseKeyboard(t, h, "abc")
	h.mustReach("3/100")

	kbd, ok := hashlockKbdFor[h]
	if !ok {
		t.Fatal("no *PassphraseKeyboard was registered: this test measured nothing")
	}
	if kbd.MaxHeight <= 0 {
		t.Fatalf("the phrase screen left the keyboard UNBOUNDED (MaxHeight=%d); the "+
			"readout is free to grow over the counter and the title", kbd.MaxHeight)
	}
	grid := kbd.size[kbd.page]
	budget := kbd.MaxHeight - grid.Y - ppReadoutGap
	_, one := widget.Labelw(&h.ctx.B, h.ctx.Styles.word, grid.X, descriptorTheme.Text, "*")
	t.Logf("MaxHeight=%d grid=%v gap=%d -> readout budget %d px; one line is %d px",
		kbd.MaxHeight, grid, ppReadoutGap, budget, one.Y)
	if budget < one.Y {
		t.Errorf("the readout budget is %d px and one line needs %d: PassphraseKeyboard.Layout "+
			"clamps every rune away, so nothing is masked, nothing is revealed, and the "+
			"`show` key is a dead control (F-481)", budget, one.Y)
	}
}
