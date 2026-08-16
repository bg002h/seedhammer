package main

// UNTAGGED, for needle_test.go's reason: this reads gui's SOURCE off disk rather
// than referencing any //go:build js symbol.

// ─── F-190 (and F-184): the counter measures SCREENS, not source bytes ────────
//
// needle_test.go proves a needle is single-site by blunt substring match over
// gui's source. That answers the wrong question in two ways, and both were found
// by tripping over them rather than by reading:
//
//	F-184  it counts a literal quoted in a COMMENT as a production site, so
//	       explaining why a screen says what it says costs that string its
//	       uniqueness proof. S3 hit it twice in one day.
//	F-190  a string emitted by a SHARED HELPER appears once in source and on
//	       EVERY caller's screen. The count says "unique" while the needle
//	       identifies nothing. F-188 hit it directly: reusing the build path's
//	       plate-census title on the supply path made the build walk's anchor
//	       two-site, and the implementer had to differ an OPERATOR-FACING TITLE
//	       ("Plates To Cut" vs "Plate Count") purely to keep a test honest. That
//	       is the tail wagging the dog.
//
// The question a needle actually answers is "if this string is on screen, WHICH
// FLOW am I in". So this file answers that one: it parses gui, finds the
// functions whose STRING LITERALS carry the needle, and walks UP the call graph
// to the flow that owns each of them.
//
//	COMMENTS DISAPPEAR FOR FREE. go/parser without ParseComments yields an AST
//	with no comment text in it, and only *ast.BasicLit of kind STRING is
//	inspected, so a needle quoted in a comment is not a site at all. F-184's
//	workaround ("do not quote a needle literal in gui/ source comments") stops
//	being a rule anybody has to remember.
//
//	SHARED HELPERS ARE ATTRIBUTED TO THEIR CALL SITES. A literal inside
//	buildSlotSourceLines belongs to buildSlotSourceReviewFlow; a literal inside
//	confirmReviewScreen would belong to EVERY flow that calls it, which is the
//	true answer and the one the substring counter cannot give.
//
// A literal passed as an ARGUMENT into a shared helper already lives in the
// caller, so it is attributed there with no special case -- which is why
// "Cosigner Keys" is the build flow's even though bundleGatherFlow draws it.
//
// WHAT THIS DOES NOT COVER, stated because a gate that hides its blind spot is
// worse than none:
//
//   - Functions are keyed by NAME, so a method and a function sharing a name are
//     one node. gui has no such collision today and the floors below would fire
//     if the graph collapsed.
//   - A string built by concatenation across several literals is seen only if one
//     of the pieces contains the whole needle. needle_test.go's blunt substring
//     pass still runs and is strictly wider, so the two are kept side by side
//     rather than one replacing the other.
//   - Dynamic dispatch through a function value is invisible. gui dispatches
//     programs through a switch on a name (gui/gui.go), which IS a static call.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// guiGraph is gui's package-level call graph plus the string literals each
// function carries.
type guiGraph struct {
	file    map[string]string   // func name -> file it is declared in
	lits    map[string][]string // func name -> string literals in its body
	calls   map[string][]string // func name -> names it calls
	callers map[string][]string // func name -> names that call it
	// consts maps a package-level const/var name to its string value, and refs
	// maps a func name to the package-level names it mentions. A needle declared
	// as a const belongs to the functions that USE it, not to the file that spells
	// it -- gui.buildCosignerGatherTitle is exactly that shape.
	consts map[string]string
	refs   map[string][]string
	funcs  []string
}

func loadGUIGraph(t *testing.T) *guiGraph {
	t.Helper()
	dir := filepath.Join("..", "..", "gui")
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading %s: %v", dir, err)
	}
	g := &guiGraph{
		file:    map[string]string{},
		lits:    map[string][]string{},
		calls:   map[string][]string{},
		callers: map[string][]string{},
		consts:  map[string]string{},
		refs:    map[string][]string{},
	}
	fset := token.NewFileSet()
	parsed := 0
	for _, e := range ents {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		// Deliberately NOT parser.ParseComments: an AST with no comments in it is
		// how F-184 stops being a rule to remember.
		f, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, 0)
		if err != nil {
			t.Fatalf("parsing gui/%s: %v", name, err)
		}
		parsed++
		g.collectFile("gui/"+name, f)
	}
	// FLOORS, so a misrooted path or a parser that silently produced nothing
	// cannot make every needle look unique by finding no sites at all.
	if parsed < 30 {
		t.Fatalf("parsed only %d production .go file(s) under %s — the path is wrong, "+
			"and every answer from it is meaningless", parsed, dir)
	}
	if len(g.funcs) < 300 {
		t.Fatalf("the graph holds %d function(s); gui is far larger, so the collector is "+
			"not reading it", len(g.funcs))
	}
	for caller, callees := range g.calls {
		for _, c := range callees {
			if !slices.Contains(g.callers[c], caller) {
				g.callers[c] = append(g.callers[c], caller)
			}
		}
	}
	return g
}

func (g *guiGraph) collectFile(path string, f *ast.File) {
	// Package-level string consts and vars.
	for _, d := range f.Decls {
		gd, ok := d.(*ast.GenDecl)
		if !ok || (gd.Tok != token.CONST && gd.Tok != token.VAR) {
			continue
		}
		for _, s := range gd.Specs {
			vs, ok := s.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for i, n := range vs.Names {
				if i >= len(vs.Values) {
					continue
				}
				if bl, ok := vs.Values[i].(*ast.BasicLit); ok && bl.Kind == token.STRING {
					if v, err := strconv.Unquote(bl.Value); err == nil {
						g.consts[n.Name] = v
					}
				}
			}
		}
	}
	for _, d := range f.Decls {
		fd, ok := d.(*ast.FuncDecl)
		if !ok || fd.Body == nil {
			continue
		}
		name := fd.Name.Name
		if _, dup := g.file[name]; !dup {
			g.funcs = append(g.funcs, name)
		}
		g.file[name] = path
		ast.Inspect(fd.Body, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.BasicLit:
				if x.Kind == token.STRING {
					if v, err := strconv.Unquote(x.Value); err == nil {
						g.lits[name] = append(g.lits[name], v)
					}
				}
			case *ast.Ident:
				if _, isConst := g.consts[x.Name]; isConst {
					g.refs[name] = append(g.refs[name], x.Name)
				}
			case *ast.CallExpr:
				switch fn := x.Fun.(type) {
				case *ast.Ident:
					g.calls[name] = appendUnique(g.calls[name], fn.Name)
				case *ast.SelectorExpr:
					g.calls[name] = appendUnique(g.calls[name], fn.Sel.Name)
				}
			}
			return true
		})
	}
}

func appendUnique(xs []string, s string) []string {
	if slices.Contains(xs, s) {
		return xs
	}
	return append(xs, s)
}

// isFlow reports whether a function is a FLOW: a screen sequence an operator can
// be inside. The suffix is gui's own convention and it is used by 60+ functions,
// so this is reading the package's vocabulary rather than inventing one.
func isFlow(name string) bool { return strings.HasSuffix(name, "Flow") }

// sites returns the functions whose own string literals (or referenced string
// consts) contain the needle.
func (g *guiGraph) sites(needle string) []string {
	var out []string
	for _, fn := range g.funcs {
		hit := false
		for _, l := range g.lits[fn] {
			if strings.Contains(l, needle) {
				hit = true
				break
			}
		}
		if !hit {
			for _, r := range g.refs[fn] {
				if strings.Contains(g.consts[r], needle) {
					hit = true
					break
				}
			}
		}
		if hit {
			out = append(out, fn)
		}
	}
	sort.Strings(out)
	return out
}

// owners walks up from a site to the flows that can draw it.
//
// THE DISCRIMINATOR IS THE CALLER COUNT, NOT FLOW-NESS, and getting that wrong
// is what the first version of this function did: it stopped at the first
// `...Flow` ancestor, so "Scan a card, or Done" resolved to bundleGatherFlow and
// looked UNIQUE — while bundleGatherFlow is called by three different programs
// and that body is the very screen F-169 measured as character-for-character
// identical across two of them. The control test caught it, which is what the
// control is for.
//
// So the rule is:
//
//	no callers          -> this function owns it (a root, or a dead screen)
//	several callers     -> every caller's owners, unioned: the string is SHARED
//	exactly one caller  -> the containment is unique, so ascend. If everything
//	                       above is unambiguous too AND this is a flow, this flow
//	                       is the most specific honest answer; otherwise hand back
//	                       whatever the fork above produced.
//
// The last clause is why a flow whose own single caller is shared does NOT get to
// claim the string: being inside it says nothing about which program you came
// through, which is the only question a walk needle is asked.
func (g *guiGraph) owners(fn string, seen map[string]bool) []string {
	if seen[fn] {
		return nil // a cycle: this path adds nothing
	}
	seen[fn] = true
	cs := g.callers[fn]
	if len(cs) == 0 {
		return []string{fn}
	}
	if len(cs) > 1 {
		var out []string
		for _, c := range cs {
			for _, o := range g.owners(c, seen) {
				out = appendUnique(out, o)
			}
		}
		if len(out) == 0 {
			return []string{fn}
		}
		return out
	}
	up := g.owners(cs[0], seen)
	if len(up) == 0 {
		return []string{fn}
	}
	if isFlow(fn) && len(up) == 1 {
		return []string{fn}
	}
	return up
}

// FlowOwners is the answer: which flows can put this string on a screen.
func (g *guiGraph) FlowOwners(needle string) []string {
	var out []string
	for _, s := range g.sites(needle) {
		for _, o := range g.owners(s, map[string]bool{}) {
			out = appendUnique(out, o)
		}
	}
	sort.Strings(out)
	return out
}

// TestBuildFlowNeedlesAreDrawnByExactlyOneFlow is F-190's gate.
//
// It is stricter than the substring counter in the direction that matters (a
// shared helper's string is no longer "unique") and looser in the direction that
// only ever cost authorship (a comment is not a site).
func TestBuildFlowNeedlesAreDrawnByExactlyOneFlow(t *testing.T) {
	g := loadGUIGraph(t)
	for _, n := range buildFlowNeedles {
		owners := g.FlowOwners(n.text)
		switch {
		case len(owners) == 0:
			t.Errorf("needle %q is drawn by NO flow this graph can name — it was renamed, or it "+
				"is built by concatenation and this analysis cannot see it", n.text)
		case len(owners) > 1:
			var where []string
			for _, o := range owners {
				where = append(where, o+" ("+g.file[o]+")")
			}
			t.Errorf("needle %q reaches the screen from %d flows:\n  %s\n"+
				"A string one shared helper emits appears on every caller's screen, so a walk "+
				"anchoring on it cannot prove which flow it is in (F-190).",
				n.text, len(owners), strings.Join(where, "\n  "))
		default:
			if got := g.file[owners[0]]; got != n.file {
				t.Errorf("needle %q is drawn only by %s, which lives in %s; the walk pins it to %s",
					n.text, owners[0], got, n.file)
			}
			t.Logf("%-45q -> %s (%s)", n.text, owners[0], g.file[owners[0]])
		}
	}
}

// TestSharedHelperStringsAreReportedAsShared is the mutation proof for the
// analysis, and it is the half that makes the test above mean anything.
//
// Without it, an `owners` that returned one name for everything would report
// every needle unique — the false-PASS this file exists to remove. So it is
// exercised against strings whose answers are known independently of any pin:
// the gather's BODY is emitted by the SHARED gatherer and must come back with
// several owners, while a needle the build flow passes IN must come back with
// one.
func TestSharedHelperStringsAreReportedAsShared(t *testing.T) {
	g := loadGUIGraph(t)

	// The gather's body, drawn by bundleGatherFlow for every caller. Every walk
	// script names it as NOT a needle; this is that comment made checkable.
	if owners := g.FlowOwners("Scan a card, or Done"); len(owners) < 2 {
		t.Errorf("the shared gatherer's body resolves to %v; it is drawn for every caller of "+
			"bundleGatherFlow, so an analysis reporting one owner is not attributing shared "+
			"helpers to their call sites at all", owners)
	} else {
		t.Logf("shared gather body -> %d flows: %v", len(owners), owners)
	}

	// And the counter must be able to say NOTHING. A string that is in no literal
	// anywhere has no owner.
	if owners := g.FlowOwners("zzz-this-string-is-not-in-gui-zzz"); len(owners) != 0 {
		t.Errorf("an impossible string resolved to %v", owners)
	}

	// The positive control: a needle the build flow passes into shared code is
	// still the build flow's, because the LITERAL lives in the caller.
	if owners := g.FlowOwners("Cosigner Keys"); len(owners) != 1 {
		t.Errorf("the build flow's own gather title resolves to %v; a literal that lives in the "+
			"caller must be attributed there", owners)
	}
}

// TestTheOldSubstringCounterAndTheFlowCounterDisagreeWhereF190Says.
//
// The two counters are kept side by side deliberately — one is wider (it sees
// concatenated strings), the other is truer (it sees screens) — so this pins the
// one place they are KNOWN to differ, which is the finding F-190 records. A
// future edit that quietly collapsed them would erase it.
func TestTheTwoCountersDisagreeOnlyWhereRecorded(t *testing.T) {
	g := loadGUIGraph(t)
	// contentNeedles are strings that identify WHAT WAS BUILT rather than WHICH
	// FLOW built it. The substring counter calls them unique because they are
	// spelt in one file; the flow counter correctly reports several owners,
	// because the screen that carries them is shared.
	for _, n := range contentNeedles {
		owners := g.FlowOwners(n.text)
		if len(owners) < 2 {
			t.Errorf("content needle %q resolves to %v — if it really is drawn by ONE flow it "+
				"belongs in buildFlowNeedles, deliberately, rather than sitting in the list of "+
				"strings a walk may only pair WITH a flow needle", n.text, owners)
			continue
		}
		if got := productionSites(t, n.text); len(got) != 1 {
			t.Errorf("content needle %q now has %d production site(s), pinned at 1: %v",
				n.text, len(got), got)
		}
		t.Logf("%-14q: 1 source site, %d drawing flow(s): %v", n.text, len(owners), owners)
	}
}
