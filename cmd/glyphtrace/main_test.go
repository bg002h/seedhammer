package main

import (
	"bytes"
	"encoding/xml"
	"io"
	"strings"
	"testing"

	"seedhammer.com/font/constant"
	"seedhammer.com/internal/sh2"
)

// Both tests here pin a SILENT failure, which is the only kind this tool has.
// rsvg-convert does not report a malformed path or an unparseable document by
// failing -- it drops what it cannot read and writes the PNG anyway. A glyph
// that vanished and a glyph that cuts nothing look identical in the output, and
// "cuts nothing" is a reading this sheet exists to give, so a rendering fault
// arrives disguised as a finding about the font.
//
// Both were live defects, not hypotheses. The first draft emitted ink paths
// beginning with a C, and every one of the sixteen rendered blank.

// svgPaths parses the document and returns every path's d attribute. Parsing is
// half the assertion: a caption carrying an unescaped '<' produces a document
// that is not XML, and three of the glyphs under review are '<', '>' and '&'.
func svgPaths(t *testing.T, svg []byte) []string {
	t.Helper()
	var ds []string
	dec := xml.NewDecoder(bytes.NewReader(svg))
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("the rendered SVG does not parse, so rsvg-convert would drop part of it: %v", err)
		}
		se, ok := tok.(xml.StartElement)
		if !ok || se.Name.Local != "path" {
			continue
		}
		for _, a := range se.Attr {
			if a.Name.Local == "d" {
				ds = append(ds, a.Value)
			}
		}
	}
	return ds
}

func renderProblemGlyphs(t *testing.T) []byte {
	t.Helper()
	svg, err := render(constant.Font, "constant", []rune(problemGlyphs), 3.0, 4)
	if err != nil {
		t.Fatal(err)
	}
	return svg
}

// TestRenderedSVGParsesAndEveryPathOpensWithAMoveTo is the whole guard.
//
// A path whose data begins with a C rather than an M is invalid, and rsvg-convert
// discards the element without a word -- so the cell draws its advance box, its
// caption and its measurements, and no glyph. Nothing about that output says a
// rendering step failed.
func TestRenderedSVGParsesAndEveryPathOpensWithAMoveTo(t *testing.T) {
	paths := svgPaths(t, renderProblemGlyphs(t))
	// Ink and centreline are the same data twice, plus a travel path for any
	// multi-run glyph: at least two per glyph, and never zero.
	if want := 2 * len([]rune(problemGlyphs)); len(paths) < want {
		t.Fatalf("the sheet carries %d paths for %d glyphs, want at least %d",
			len(paths), len([]rune(problemGlyphs)), want)
	}
	for i, d := range paths {
		if d = strings.TrimSpace(d); !strings.HasPrefix(d, "M") {
			head := d
			if len(head) > 60 {
				head = head[:60]
			}
			t.Errorf("path %d opens with %q, not a moveto; rsvg-convert drops it silently: %s",
				i, firstRune(d), head)
		}
	}
}

// TestEveryProblemGlyphCuts: the sixteen are the reason the tool exists, so a
// blank cell among them must be a fact about the face and never about the
// renderer. This asks the trace directly, upstream of any SVG.
func TestEveryProblemGlyphCuts(t *testing.T) {
	P := sh2.Params()
	em := P.F(3.0)
	for _, r := range problemGlyphs {
		g := trace(constant.Font, r, em, P.StepperConfig)
		switch {
		case g.unmapped:
			t.Errorf("%q is not in font/constant", r)
		case !g.hasInk:
			t.Errorf("%q cuts nothing", r)
		case g.strokes < 1:
			t.Errorf("%q reports %d strokes", r, g.strokes)
		case len(g.ctrl) == 0:
			t.Errorf("%q has no control points to draw", r)
		case len(g.starts) != g.strokes:
			t.Errorf("%q marks %d stroke starts against k=%d", r, len(g.starts), g.strokes)
		}
	}
}

// TestCaptionsCarryTheCharacterItself keeps esc on the live path. If the
// captions were names only -- "lt" rather than "< lt" -- no default render would
// contain a reserved character, the escaping would be unreachable, and the parse
// assertion above would pass however broken esc became.
func TestCaptionsCarryTheCharacterItself(t *testing.T) {
	for _, r := range []rune{'<', '>', '&'} {
		if got := caption(r); !strings.ContainsRune(got, r) {
			t.Errorf("caption(%q) = %q, which does not contain the character; esc is then dead code", r, got)
		}
	}
}

func firstRune(s string) string {
	for _, r := range s {
		return string(r)
	}
	return ""
}

// TestCounterOfOMatchesArithmetic anchors the raster against a number that can
// be worked out by hand. 'o' is a square: its centrelines ink 1.33mm apart and
// the tool lays 0.30mm down the middle of them, so 1.03mm of bare metal is left
// and the largest disc that fits is that wide.
//
// It is the only glyph on the sheet whose answer is checkable without the
// raster, which is exactly why it is the one pinned. A resolution or margin
// change that shifted every figure would still agree with itself.
func TestCounterOfOMatchesArithmetic(t *testing.T) {
	P := sh2.Params()
	g := trace(constant.Font, 'o', P.F(3.0), P.StepperConfig)
	cs := rasterize(g.runs, P.StrokeWidth, P.Millimeter).findCounters(P.Millimeter)
	if len(cs) != 1 {
		t.Fatalf("'o' encloses %d regions, want exactly 1", len(cs))
	}
	const want = 1.03
	if got := cs[0].WidthMM; got < want-0.04 || got > want+0.04 {
		t.Errorf("'o' counter is %.3fmm wide, want %.2fmm "+
			"(1.33mm between centrelines less the 0.30mm stroke)", got, want)
	}
}

// TestCountersCloseAsTheGlyphShrinks is the measurement's own falsifiability.
//
// The stroke is 0.30mm at every rung while the glyph scales, so counters must
// survive at 6.0mm and die somewhere below. A metric that returned the same
// count at both would be measuring nothing, and would have reported the
// reassuring "0 lost" that the 3.0mm sweep reports for real.
func TestCountersCloseAsTheGlyphShrinks(t *testing.T) {
	P := sh2.Params()
	count := func(r rune, mm float32) int {
		g := trace(constant.Font, r, P.F(mm), P.StepperConfig)
		return len(rasterize(g.runs, P.StrokeWidth, P.Millimeter).findCounters(P.Millimeter))
	}
	// 'a' and 'e' hold one counter each at the rungs that ship, and lose it
	// well below them.
	for _, r := range []rune{'a', 'e'} {
		big, ship, tiny := count(r, 6.0), count(r, 3.0), count(r, 1.0)
		if big != 1 || ship != 1 {
			t.Errorf("%q encloses %d at 6.0mm and %d at 3.0mm, want 1 at both", r, big, ship)
		}
		if tiny != 0 {
			t.Errorf("%q still encloses %d regions at 1.0mm; the measurement cannot detect a closure", r, tiny)
		}
	}
}
