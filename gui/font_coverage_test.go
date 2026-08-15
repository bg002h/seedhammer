package gui

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
	"unicode"

	"seedhammer.com/font/bitmap"
	"seedhammer.com/gui/text"
)

// ─── S3b: the blanking class, guarded by FACE COVERAGE rather than a list ────
//
// THE DEFECT. An unrenderable rune does not fail to draw itself: it blanks the
// ENTIRE BODY of the frame it appears in. Measured on this build, five bodies of
// different lengths through showError all rastered at 2652 px, which is the
// title-only value. And uiContains returns TRUE on that frame, because
// ExtractText walks the op tree and reports what was SUBMITTED
// (gui/raster_test.go:11). So every content assertion in this package is blind
// to it, and the only instruments that see it are source lookup and ink.
//
// THE MECHANISM, and it is much wider than the em-dash everyone chased.
// font/bitmap/bitmap.go:33 sets
//
//	indexLen = unicode.MaxASCII
//
// and glyphFor rejects `int(r) >= indexLen` at :62. EVERY NON-ASCII RUNE IS
// UNRENDERABLE ON EVERY BITMAP FACE. Face choice is therefore immaterial, which
// TestEveryFaceRejectsNonASCII below pins rather than assumes.
//
// WHY THIS IS A LOOKUP AND NOT A BLOCKLIST. S2 shipped `blankingGlyphs`, seven
// runes measured one at a time. Two later inventories built the same way came
// back 27 and 21 and both missed four whole classes, because a list of the runes
// somebody happened to hit is the hand-maintained construct F-163 indicts: it
// cannot catch the NEXT one. Asking the face whether it has the glyph needs no
// list, tracks the font format if it ever widens, and fails on a rune nobody has
// met yet.

// allowedNonPrinting are the control runes a production string may carry: they
// are layout instructions consumed before glyph lookup, not text to be drawn.
// Anything else non-printing is a defect for the same reason a missing glyph is
// — it occupies the string and draws nothing.
var allowedNonPrinting = map[rune]bool{'\n': true, '\t': true}

// runesDrawnAsImages are runes that reach a string literal as a SENTINEL and are
// never looked up in a face at all.
//
// THE EXEMPTION IS NOT SELF-CERTIFYING. Every entry is proven by
// TestSentinelRunesAreImageDrawn, which requires the source to special-case the
// rune to an image asset AND requires the screen that carries it to raster well
// clear of a blank. An exemption justified by a comment is the same
// hand-maintained construct one layer down, so a comment is not accepted here.
var runesDrawnAsImages = map[rune]struct {
	file  string // the production file that special-cases it
	asset string // the image drawn in its place
}{
	// gui.go:1572-1574 compares key.r and blits the asset instead of measuring a
	// glyph, so this rune never reaches GlyphAdvance and cannot blank a frame.
	'⌫': {file: "gui.go", asset: "assets.KeyBackspace"},
}

// productionLiteral is one string literal from a non-test source file.
type productionLiteral struct {
	File  string
	Line  int
	Value string
}

// productionStringLiterals returns every string literal in every non-test .go
// file of this package.
//
// IT PARSES RATHER THAN SCANS, and that is a correctness point rather than a
// tidiness one. S2's hand-rolled scanner walked characters and skipped
// whole-line comments by prefix, which mishandles a trailing comment containing
// a quote, treats a backtick raw string as ordinary text, and cannot report a
// line number. The parser gets comments right STRUCTURALLY (they are not in the
// AST at all), unquotes raw strings and escapes correctly, and gives a position
// — which is what lets a failure name the file and the rune instead of only the
// text.
//
// This also answers F-184, which I filed against cmd/emu/needle_test.go for the
// same root cause: its productionSites counts a needle quoted in a COMMENT as a
// production site. Same defect, same fix; that one is not mine to land here.
func productionStringLiterals(t testing.TB) []productionLiteral {
	t.Helper()
	ents, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("reading the package directory: %v", err)
	}
	fset := token.NewFileSet()
	var out []productionLiteral
	files := 0
	for _, e := range ents {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		files++
		// Comments are NOT requested, so they are absent from the AST and cannot
		// be mistaken for text the device draws.
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", name, err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			v, err := strconv.Unquote(lit.Value)
			if err != nil {
				// A literal the standard unquoter cannot read is not something to
				// wave through: fail loudly rather than skip it silently.
				t.Errorf("%s: cannot unquote %s: %v", name, lit.Value, err)
				return true
			}
			out = append(out, productionLiteral{
				File:  name,
				Line:  fset.Position(lit.Pos()).Line,
				Value: v,
			})
			return true
		})
	}
	// A floor, so a wrong working directory cannot make every guard below pass by
	// finding nothing to check. gui is a large package.
	if files < 30 {
		t.Fatalf("only %d production .go file(s) in the package directory — the path "+
			"is wrong, and every verdict from it is meaningless", files)
	}
	return out
}

// packageFaces returns every face the package can draw text with.
//
// THE LIST IS EXPLICIT BUT ITS COMPLETENESS IS ENFORCED, not promised. Styles'
// fields are unexported, so reading them by reflection would need unsafe; naming
// them is legal and clear from inside the package. What a bare list cannot do is
// notice a style added later, so the count is checked against the struct by
// reflection. Add a style without adding it here and this fails, which is the
// property the list would otherwise lose.
func packageFaces(t testing.TB) map[string]*bitmap.Face {
	t.Helper()
	s := NewStyles()
	faces := map[string]*bitmap.Face{
		"title":    s.title.Face,
		"subtitle": s.subtitle.Face,
		"body":     s.body.Face,
		"lead":     s.lead.Face,
		"button":   s.button.Face,
		"word":     s.word.Face,
		"keyboard": s.keyboard.Face,
		"warning":  s.warning.Face,
		"debug":    s.debug.Face,
		"progress": s.progress.Face,
	}
	styleFields := 0
	st := reflect.TypeOf(Styles{})
	styleType := reflect.TypeOf(text.Style{})
	for i := 0; i < st.NumField(); i++ {
		if st.Field(i).Type == styleType {
			styleFields++
		}
	}
	if len(faces) != styleFields {
		t.Fatalf("Styles has %d text.Style field(s) but this test checks %d face(s); "+
			"a style was added without being covered, and the guard would silently "+
			"stop checking whatever it draws", styleFields, len(faces))
	}
	for name, f := range faces {
		if f == nil {
			t.Fatalf("style %s has a nil Face; a coverage check against nil passes "+
				"everything", name)
		}
	}
	return faces
}

// TestEveryFaceRejectsNonASCII pins the mechanism the guard rests on, instead of
// trusting the comment above it: font/bitmap indexes glyphs by rune up to
// unicode.MaxASCII, so the answer is the same for every face and a per-face
// argument about "the body face" is beside the point.
func TestEveryFaceRejectsNonASCII(t *testing.T) {
	faces := packageFaces(t)
	t.Logf("checked %d face(s): %s", len(faces), strings.Join(sortedKeys(faces), ", "))
	for name, face := range faces {
		if _, ok := face.GlyphAdvance('A'); !ok {
			t.Errorf("face %s has no glyph for 'A' — the lookup is not working", name)
		}
		for _, r := range []rune{'—', '·', '✓', '…', '→', '⌫', rune(unicode.MaxASCII)} {
			if _, ok := face.GlyphAdvance(r); ok {
				t.Errorf("face %s reports a glyph for %q (U+%04X); the guard's premise "+
					"that non-ASCII is unrenderable no longer holds and the floor "+
					"assumptions in this package need re-measuring", name, r, r)
			}
		}
	}
}

// TestProductionStringsAreDrawable is the guard: every rune of every production
// string literal in gui/*.go must be one the faces can actually draw.
func TestProductionStringsAreDrawable(t *testing.T) {
	lits := productionStringLiterals(t)
	if len(lits) == 0 {
		t.Fatal("found zero production string literals, so this guard checks nothing")
	}
	faces := packageFaces(t)

	bad := 0
	for _, lit := range lits {
		for _, r := range lit.Value {
			if allowedNonPrinting[r] {
				continue
			}
			if _, exempt := runesDrawnAsImages[r]; exempt {
				continue
			}
			var missing []string
			for name, face := range faces {
				if _, ok := face.GlyphAdvance(r); !ok {
					missing = append(missing, name)
				}
			}
			if len(missing) == 0 && unicode.IsPrint(r) {
				continue
			}
			bad++
			why := "no face has a glyph for it"
			if len(missing) == 0 {
				why = "it is a non-printing rune with no layout meaning"
			}
			t.Errorf("%s:%d carries %q (U+%04X): %s, so the frame drawing this string "+
				"renders its title and NOTHING else. uiContains still reports the text "+
				"as present. Literal: %q",
				lit.File, lit.Line, r, r, why, lit.Value)
		}
	}
	t.Logf("scanned %d production string literal(s) across gui/*.go; %d undrawable rune site(s)",
		len(lits), bad)
}

// TestProductionLiteralScannerCanSee is the vacuity proof for the scanner, for
// TestStringLiteralScannerCanSee's reason: a scanner that silently returned
// nothing would make the guard above pass on any tree at all, which is the exact
// false-PASS shape this class exists to remove.
func TestProductionLiteralScannerCanSee(t *testing.T) {
	lits := productionStringLiterals(t)
	if len(lits) < 200 {
		t.Fatalf("scanner found only %d literal(s) in gui/*.go; the package has far "+
			"more, so it is not reading the tree", len(lits))
	}
	// It must find a string that certainly exists, at a real position.
	found := false
	for _, l := range lits {
		if l.Value == "Choose policy type" && l.File == "multisig_build.go" && l.Line > 0 {
			found = true
		}
	}
	if !found {
		t.Error(`scanner did not find "Choose policy type" in multisig_build.go with a line number`)
	}
	// And it must NOT report comment prose. Every non-test file's comments are
	// full of em-dashes; if any literal equals a known comment fragment the
	// parser is being bypassed.
	for _, l := range lits {
		if strings.Contains(l.Value, "F-151") || strings.Contains(l.Value, "MEASURED") {
			t.Errorf("%s:%d looks like comment prose reported as a literal: %q",
				l.File, l.Line, l.Value)
		}
	}
	t.Logf("scanner sees %d production string literal(s)", len(lits))
}

// TestSentinelRunesAreImageDrawn is what makes runesDrawnAsImages an EXEMPTION
// rather than a hole.
//
// The guard skips these runes, so something has to establish that skipping them
// is safe, and a comment saying "this one is an image" is the same
// hand-maintained construct the guard replaced — one layer down, and now load-
// bearing for the guard's own soundness. Two independent things are required of
// every entry:
//
//	STRUCTURAL  the file names the rune as a CHAR literal (so it is COMPARED,
//	            not drawn) and references the image asset. Read from the AST, so
//	            a comment mentioning either cannot satisfy it.
//	INK         the screen that carries the rune rasters clear of a body-less
//	            frame. If the sentinel ever did reach a face, the keyboard would
//	            blank and this fails.
func TestSentinelRunesAreImageDrawn(t *testing.T) {
	if len(runesDrawnAsImages) == 0 {
		t.Skip("no sentinel exemptions to prove")
	}

	fset := token.NewFileSet()
	for r, spec := range runesDrawnAsImages {
		f, err := parser.ParseFile(fset, spec.file, nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", spec.file, err)
		}
		comparedAsRune, referencesAsset := false, false
		wantAsset := spec.asset[strings.LastIndex(spec.asset, ".")+1:]
		ast.Inspect(f, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.BasicLit:
				if x.Kind == token.CHAR {
					if v, err := strconv.Unquote(x.Value); err == nil && v == string(r) {
						comparedAsRune = true
					}
				}
			case *ast.SelectorExpr:
				if x.Sel != nil && x.Sel.Name == wantAsset {
					referencesAsset = true
				}
			}
			return true
		})
		if !comparedAsRune {
			t.Errorf("%s never compares %q (U+%04X) as a rune literal, so the claim "+
				"that it is a sentinel rather than drawn text is unsupported",
				spec.file, r, r)
		}
		if !referencesAsset {
			t.Errorf("%s does not reference %s, so %q (U+%04X) has no image to be "+
				"drawn as", spec.file, spec.asset, r, r)
		}
	}

	// The ink half. The keyboard's alphabet carries the sentinel; if it were ever
	// looked up in a face, this screen is the one that would blank.
	p := newPlatform()
	p.display = sh2DisplaySize
	ctx := NewContext(p)
	m := emptyBIP39Mnemonic(12)
	frame, _, ink, quit := runUITouchRaster(ctx, func() {
		inputWordsFlow(ctx, &descriptorTheme, m, 0, "", wordEntryOpts{checksumGate: true})
	})
	defer quit()
	if _, ok := frame(); !ok {
		t.Fatal("the keyboard never rendered a frame")
	}
	blank := titleOnlyInk(t)
	got := ink()
	t.Logf("keyboard (carries the sentinel) ink = %d px, worst body-less frame %d", got, blank)
	if got < blank+bodyInkMargin {
		t.Errorf("the keyboard drew %d ink pixels against a body-less frame of %d: "+
			"the sentinel is reaching a face after all, and the exemption is a hole", got, blank)
	}
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
