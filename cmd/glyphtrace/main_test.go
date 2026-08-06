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
