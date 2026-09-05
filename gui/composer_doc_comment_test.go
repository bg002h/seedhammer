package gui

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// ─── r0 fidelity I-3: an inserted helper must not steal a doc comment ────────
//
// Go attaches a comment block to the declaration that follows it, so a new
// function whose own comment is written directly beneath an existing doc
// comment -- with no blank line between them -- MERGES the two: the new
// function is documented with the old one's text and the old one is left with
// nothing. H5 inserted two helpers exactly that way, in two different files,
// and took `composerFlow`'s comment (the written record of the "plans list
// components and omit the call that joins them" Critical) and
// `composerPageLines`'s (the "ONE MEASURE SITE" record behind every SPEC §13
// capacity number) with them.
//
// NOTHING ELSE IN THE SUITE CAN SEE IT. gofmt is clean either way, go vet says
// nothing, every test passes, and scripts/h5-plan-blocks-vs-tree.sh matches
// fragments by substring, which is satisfied whichever side of the blank line
// the block sits on. A merged block is not a syntax error; it is a silent
// change of owner.
//
// This is deliberately a NAMED LIST rather than a whole-package rule. Plenty of
// doc comments in this package legitimately open with something other than the
// symbol's name (composerCopyHashlockReconcile opens "§4.5's reconciliation
// screen"), so a package-wide "must start with the name" check would be noise.
// What is asserted here is narrow and exact: these symbols have a doc comment,
// and it is THEIR doc comment.
var composerDocOwners = map[string]string{
	"composerFlow":      "composer_flow.go",
	"composerFlowExit":  "composer_flow.go",
	"composerPageLines": "composer_paged.go",
	"composerTextBand":  "composer_paged.go",
}

// TestComposerHelpersDidNotStealADocComment is the gate for the above.
//
// MUTATION: move composerFlowExit's comment and func back BETWEEN composerFlow's
// doc comment and composerFlow (the arrangement fidelity I-3 found) ->
// "composerFlow has NO doc comment in composer_flow.go" and "composerFlowExit's
// doc comment ... opens \"composerFlow is ...\"". Same for composerTextBand and
// composerPageLines.
//
// NOT a mutation, MEASURED: deleting the blank line between composerFlowExit's
// closing brace and composerFlow's doc comment does NOT break the attachment and
// this test stays green -- go/ast binds a comment group to the declaration on the
// line after it, whatever precedes the group. The defect is the ORDER, not the
// whitespace, which is why the fix moves the helper above the doc block rather
// than inserting a blank line inside the sandwich.
func TestComposerHelpersDidNotStealADocComment(t *testing.T) {
	fset := token.NewFileSet()
	seen := map[string]bool{}
	for name, file := range composerDocOwners {
		f, err := parser.ParseFile(fset, file, nil, parser.ParseComments)
		if err != nil {
			t.Fatalf("parsing %s: %v", file, err)
		}
		var fn *ast.FuncDecl
		for _, d := range f.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if ok && fd.Recv == nil && fd.Name.Name == name {
				fn = fd
				break
			}
		}
		if fn == nil {
			t.Errorf("%s declares no func %s; this list is stale", file, name)
			continue
		}
		seen[name] = true
		if fn.Doc == nil {
			t.Errorf("%s has NO doc comment in %s -- a block inserted beneath it with no "+
				"blank line between takes it, and the record it carried goes with it "+
				"(r0 fidelity I-3)", name, file)
			continue
		}
		first := strings.TrimSpace(strings.TrimPrefix(fn.Doc.List[0].Text, "//"))
		if !strings.HasPrefix(first, name+" ") {
			t.Errorf("%s's doc comment in %s opens %q -- it is documenting %s with another "+
				"symbol's text", name, file, first, name)
		}
	}
	if len(seen) != len(composerDocOwners) {
		t.Fatalf("checked %d of %d symbols", len(seen), len(composerDocOwners))
	}
}
