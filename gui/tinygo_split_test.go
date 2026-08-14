package gui

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// The guard over every optional hook this package keeps OUT of the firmware
// image.
//
// WHY THE PROPERTY MATTERS. gui/plate_hook.go hands out a spline -- the
// operator's words as geometry -- and gui/frame_hook.go hands out a frame's
// content op -- the operator's words as words. Both are the material F-107/F-108
// are about, both have exactly one consumer, and that consumer is JavaScript on
// a page, outside anything Go can wipe. So the firmware must not merely decline
// to use them, it must not CONTAIN them: `//go:build !tinygo` puts each
// interface and the type assertion that reaches it outside the image, the way
// gui/preview.go already puts the plate previews outside it. The copy inventory
// of seed-derived material is then true by CONSTRUCTION rather than by
// argument.
//
// WHY IT IS A TEST AND NOT THE COMPILER. CI does build the device image
// (.github/workflows/test.yml's tinygo-device-build), so the loud mutations --
// delete a stub, drop a constraint, call the hook from shared code -- fail
// there. What builds CLEANLY on both targets is the quiet one: move the
// interface into an untagged file. Nothing then fails to compile, the image
// carries it, and the only thing that notices is this.
//
// WHY IT IS DERIVED FROM THE TREE. Its predecessor,
// TestPlateHookIsAbsentFromTheFirmwareBuild, hard-coded "plate_hook.go" and
// "PlateAware". That is a hand-maintained list of one, and adding the SECOND
// hook is exactly the moment such a list starts going stale -- the same defect
// commit 3009f22 removed from cmd/emu's embed confinement, which used to be
// keyed to a names list and now discovers every //go:embed under the tree. This
// discovers every //go:build pair instead, so hook number three is covered by
// existing without anybody remembering to add it here.
//
// WHAT IT DOES NOT COVER, said plainly rather than left for a reviewer -- and
// this paragraph is shorter than it was, because review found that two of the
// things it did not cover were things it CLAIMED to:
//
//   - It finds pairs by the `_tinygo.go` FILENAME SUFFIX. A split written as
//     `foo_host.go`/`foo_device.go` would carry the same risk and be invisible.
//     The suffix is the convention both existing pairs use and the one Go
//     programmers reach for; nothing enforces it.
//   - It says nothing about a `!tinygo` file with NO stub at all --
//     gui/preview.go is one, is legitimate, and is excluded wholesale.
//   - It cannot see outside this module's `gui/` tree. Within it the walk is
//     recursive (it was not, and that was an Important finding: a pair moved
//     into gui/op or gui/widget was invisible while the doc here claimed the
//     walk "discovers every //go:build pair").
//
// Its own history is the argument for keeping this list honest. The first
// version checked only the HOST file of each pair for interfaces and never
// parsed the stub, so an interface declared inside the firmware-side file --
// the one place it must never be -- passed cleanly. That was a Critical, found
// by an independent review and proven with a real tinygo build, against a guard
// whose entire purpose was that property.
func TestBuildTaggedHooksAreAbsentFromTheFirmwareImage(t *testing.T) {
	// RECURSIVE, and it was not. os.ReadDir(".") saw only this directory, so a
	// hook pair moved into gui/op, gui/widget or any other subpackage was
	// invisible to every check below -- and all of them compile into the
	// firmware exactly as this package does. Found by review; the doc above
	// claimed "discovers every //go:build pair" while the walk could not see
	// most of the tree.
	var files []string
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		files = append(files, filepath.ToSlash(path))
		return nil
	})
	if err != nil {
		t.Fatalf("walking gui: %v", err)
	}
	if len(files) < 20 {
		t.Fatalf("INCONCLUSIVE: only %d non-test .go files found under gui, which is too few to be "+
			"this package -- the walk is looking in the wrong place", len(files))
	}

	// The pairs, discovered from the stub side: X_tinygo.go implies X.go.
	type pair struct{ host, stub string }
	var pairs []pair
	for _, stub := range files {
		if !strings.HasSuffix(stub, "_tinygo.go") {
			continue
		}
		host := strings.TrimSuffix(stub, "_tinygo.go") + ".go" // same directory, by construction
		if !slices.Contains(files, host) {
			t.Errorf("%s has no %s beside it -- a firmware stub with no host counterpart means "+
				"the host build has no implementation at all", stub, host)
			continue
		}
		pairs = append(pairs, pair{host: host, stub: stub})
	}
	if len(pairs) < 2 {
		t.Fatalf("INCONCLUSIVE: %d //go:build pair(s) found in gui, want at least the plate and "+
			"frame hooks -- either a stub was deleted or this walk is not finding them", len(pairs))
	}

	for _, p := range pairs {
		for _, f := range []struct{ name, want string }{
			{p.host, "!tinygo"},
			{p.stub, "tinygo"},
		} {
			b, err := os.ReadFile(f.name)
			if err != nil {
				t.Fatalf("reading %s: %v -- the build split is what keeps seed-derived material "+
					"out of the firmware image", f.name, err)
			}
			if got := buildConstraint(string(b)); got != "//go:build "+f.want {
				t.Errorf("%s carries %q, want %q", f.name, got, "//go:build "+f.want)
			}
		}
	}

	// NOTHING EXPORTED-INTERFACE-SHAPED MAY LIVE IN A STUB. This is the check
	// the guard was missing, and its absence was the exact hole the guard
	// exists to close: the stub was read once, for its build constraint, and
	// never parsed. So an interface DECLARED AND USED entirely inside the
	// //go:build tinygo file -- with the host carrying a decoy interface to
	// satisfy the per-pair check below -- passed cleanly while the firmware
	// image carried it. Proven by review with a real tinygo build at CI's
	// flags, which linked a reachable call site into a 1,342,308-byte image
	// with this test green.
	//
	// The rule is stated directly rather than inferred: a tinygo-tagged file is
	// firmware, and a hook interface in the firmware is the thing being
	// forbidden, so the stub may declare no exported interface at all.
	for _, p := range pairs {
		f, err := parser.ParseFile(token.NewFileSet(), p.stub, nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", p.stub, err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			ts, ok := n.(*ast.TypeSpec)
			if !ok {
				return true
			}
			if _, isIface := ts.Type.(*ast.InterfaceType); isIface && ts.Name.IsExported() {
				t.Errorf("%s declares exported interface %s -- that file IS the firmware, so an "+
					"interface in it is in the image, which is the one thing this split exists "+
					"to prevent", p.stub, ts.Name.Name)
			}
			return true
		})
	}

	// Every exported interface declared in a host file, and the file that owns
	// it. These are the hooks: the shapes cmd/emu implements and the firmware
	// must not know the names of.
	//
	// The scan is over the AST rather than the bytes, so a file that MENTIONS a
	// hook in a comment pointing at the split is not a violation -- a check
	// that forbids naming the rule is a check that gets the explanation deleted
	// to make it pass. (An earlier comment here credited parser mode 0 with
	// that. It does not: comments are not Idents, so ast.Inspect never visits
	// them whatever the mode. Verified -- the mode makes no difference. The
	// behaviour was right and the stated mechanism was wrong.)
	fset := token.NewFileSet()
	owner := make(map[string]string)
	for _, p := range pairs {
		f, err := parser.ParseFile(fset, p.host, nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", p.host, err)
		}
		found := 0
		ast.Inspect(f, func(n ast.Node) bool {
			ts, ok := n.(*ast.TypeSpec)
			if !ok {
				return true
			}
			if _, isIface := ts.Type.(*ast.InterfaceType); isIface && ts.Name.IsExported() {
				owner[ts.Name.Name] = p.host
				found++
			}
			return true
		})
		// PER PAIR, not merely in total. A total-only floor has a hole that
		// grows as hooks are added: with three pairs and a floor of two,
		// MOVING one interface into an untagged file leaves the count
		// satisfied and the scan below simply stops looking for the one that
		// moved -- which is the quiet mutation this whole test exists for, and
		// it would pass. Measured: with the floor alone, that mutation was
		// killed by a compile error rather than by this guard.
		//
		// If a //go:build pair is ever legitimately not an interface hook, this
		// is a deliberate edit and not a stale list: the check has to be told,
		// out loud, that the pair carries no interface to protect.
		if found == 0 {
			t.Errorf("%s declares no exported interface, so nothing about it is checked below -- "+
				"if this pair is not an interface hook, say so here rather than leaving the "+
				"scan silently vacuous", p.host)
		}
	}
	if len(owner) < 2 {
		t.Fatalf("INCONCLUSIVE: %d exported hook interface(s) found across %d pair(s) -- with "+
			"nothing to look for, the scan below passes without checking anything",
			len(owner), len(pairs))
	}

	for _, name := range files {
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", name, err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			id, ok := n.(*ast.Ident)
			if !ok {
				return true
			}
			if host, isHook := owner[id.Name]; isHook && host != name {
				t.Errorf("%s uses %s in code but is not %s -- the interface and the type "+
					"assertion that reaches it belong in the one file the firmware build drops",
					name, id.Name, host)
			}
			return true
		})
	}
}

// buildConstraint returns the //go:build line preceding the package clause, or
// "" if the file carries none.
func buildConstraint(src string) string {
	for line := range strings.SplitSeq(src, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "package ") {
			return ""
		}
		if strings.HasPrefix(line, "//go:build ") {
			return line
		}
	}
	return ""
}
