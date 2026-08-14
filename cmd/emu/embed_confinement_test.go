package main

// Structural confinement for EVERY embedded test payload, present and future.
//
// WHY THIS REPLACES A NAME LIST. The two existing guards each match a literal:
//
//	names := []string{"syswTestPayload", "syswTestDigest", "sysw_test_payload.bin"}
//
// That protects the blobs somebody remembered to add. It does not protect the
// next one, and the plan this test belongs to requires a SECOND systemwide
// payload carrying cosigner cards — which the name-keyed guard would not see at
// all. A hand-maintained list is the same construct as the "four TYPED-ONLY
// comments" that turned out to be nine.
//
// So this derives its protected set from the tree: find every `//go:embed`
// under cmd/emu, take the file it embeds and the identifier declared beneath
// it, and require both to be invisible outside files that cannot reach a
// shipped firmware binary.
//
// THE PROPERTY, stated once: a shipped SeedHammer II must never boot carrying a
// payload somebody else packed. A file satisfies that if it is `//go:build js`
// (cmd/emu only, never cmd/controller) or if it is a `_test.go` file (never
// linked into any binary). Anything else naming a payload token is the failure.
//
// WHAT THIS DOES NOT COVER. It reasons about Go source only. A payload reaching
// the firmware by some route that is not a Go identifier — a linker flag, a
// generated file, a build script — is invisible here. It also cannot tell you
// the blob's CONTENTS are safe, only that they stay in the browser build.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// embedToken is one thing that must not escape: either an embedded filename or
// the Go identifier bound to it.
type embedToken struct {
	text     string
	declFile string
}

// findEmbedTokens parses every Go file in dir and returns the tokens each
// //go:embed introduces, plus the count of files it parsed.
func findEmbedTokens(t *testing.T, dir string) ([]embedToken, int) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading %s: %v", dir, err)
	}
	fset := token.NewFileSet()
	var out []embedToken
	parsed := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		f, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			t.Fatalf("parsing %s: %v", path, err)
		}
		parsed++
		// A file carrying ANY //go:embed is a payload file, and everything it
		// declares is payload material -- not just the identifier the directive
		// binds to. `syswTestDigest` is the case that forces this: it is a const
		// beside the embed, not bound to it, and it is the value an operator
		// compares across the air gap. Binding-only discovery would have silently
		// stopped protecting it when the name-keyed guard was removed.
		var patterns, decls []string
		hasEmbed := false
		for _, decl := range f.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok {
				continue
			}
			if gd.Doc != nil {
				for _, c := range gd.Doc.List {
					if strings.HasPrefix(c.Text, "//go:embed ") {
						hasEmbed = true
						patterns = append(patterns,
							strings.Fields(strings.TrimPrefix(c.Text, "//go:embed "))...)
					}
				}
			}
			if gd.Tok != token.VAR && gd.Tok != token.CONST && gd.Tok != token.TYPE {
				continue
			}
			for _, spec := range gd.Specs {
				switch sp := spec.(type) {
				case *ast.ValueSpec:
					for _, n := range sp.Names {
						if n.Name != "_" {
							decls = append(decls, n.Name)
						}
					}
				case *ast.TypeSpec:
					decls = append(decls, sp.Name.Name)
				}
			}
		}
		if !hasEmbed {
			continue
		}
		for _, tok := range append(patterns, decls...) {
			out = append(out, embedToken{tok, e.Name()})
		}
	}
	return out, parsed
}

// hasJSBuildTag is confinement_test.go's, reused rather than reimplemented:
// its version correctly excludes `!js`, which a second copy of mine did not.

// referencedNames returns every identifier and string literal a Go file uses in
// CODE. Comments are excluded by construction: the parser keeps them out of the
// AST unless asked for them, and this asks for the code.
func referencedNames(t *testing.T, path string) map[string]bool {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, 0) // 0 == no comments
	if err != nil {
		// A file that does not parse cannot reference anything; skipping it is
		// safe here and the build would fail anyway.
		return nil
	}
	out := map[string]bool{}
	ast.Inspect(f, func(n ast.Node) bool {
		switch v := n.(type) {
		case *ast.Ident:
			out[v.Name] = true
		case *ast.BasicLit:
			if v.Kind == token.STRING {
				out[strings.Trim(v.Value, "`\"")] = true
			}
		}
		return true
	})
	return out
}

// TestEveryEmbeddedPayloadIsStructurallyConfined is the guard. It is deliberately
// one test over ALL embeds rather than one per blob: a per-blob test is a name
// list wearing a different hat.
func TestEveryEmbeddedPayloadIsStructurallyConfined(t *testing.T) {
	root := repoRoot(t)
	emuDir := filepath.Join(root, "cmd", "emu")

	tokens, parsed := findEmbedTokens(t, emuDir)
	if parsed < 5 {
		t.Fatalf("INCONCLUSIVE: parsed only %d Go files in cmd/emu, so this test is "+
			"not looking at the package it thinks it is", parsed)
	}
	if len(tokens) == 0 {
		t.Fatalf("INCONCLUSIVE: found no //go:embed under cmd/emu. Either the " +
			"payloads moved — in which case this guard must follow them — or the " +
			"discovery is broken and this test now protects nothing")
	}

	// Every embed must live in a js-only file. This is the first half of the
	// property: an embed in an untagged file is compiled into every build.
	for _, tok := range tokens {
		src, err := os.ReadFile(filepath.Join(emuDir, tok.declFile))
		if err != nil {
			t.Fatalf("reading %s: %v", tok.declFile, err)
		}
		if !hasJSBuildTag(string(src)) {
			t.Errorf("cmd/emu/%s carries a //go:embed but is not //go:build js, so its "+
				"payload compiles into every build of this package", tok.declFile)
		}
	}

	// Second half: the tokens may appear only where they cannot ship — a
	// js-tagged file, or a test file, which is never linked into a binary.
	checked := 0
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if info.Name() == ".git" || info.Name() == "third_party" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		checked++
		rel, _ := filepath.Rel(root, path)
		if strings.HasSuffix(path, "_test.go") || hasJSBuildTag(string(src)) {
			return nil // cannot reach a shipped binary
		}
		// CODE ONLY, never comments. A doc comment that mentions a payload
		// filename is documentation; a reference is what ships. The first
		// version matched raw text and flagged this repo's own generator for
		// naming the blob it generates -- the same mention-vs-reference
		// confusion that made the NAME-KEYED guard flag the file replacing it.
		used := referencedNames(t, path)
		for _, tok := range tokens {
			if used[tok.text] {
				t.Errorf("%s names %q, which is bound to a //go:embed in cmd/emu/%s. "+
					"That file is neither //go:build js nor a test, so it can reach a "+
					"shipped firmware binary — and a shipped SeedHammer II must never "+
					"boot carrying a payload somebody else packed",
					rel, tok.text, tok.declFile)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking the module: %v", err)
	}
	if checked < 50 {
		t.Fatalf("INCONCLUSIVE: only %d .go files scanned, so this test is not "+
			"looking at the module it thinks it is", checked)
	}
	t.Logf("confined %d embed token(s) across %d scanned files", len(tokens), checked)
}
