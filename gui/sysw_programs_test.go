package gui

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
)

// The four programs that do NOT share seedEntryFlow must each reach the payload
// individually — measured in the plan, they share no helper at all. This asserts
// each one is actually wired, because "I wired four flows" is exactly the claim
// that goes stale when a fifth is added.
func TestEveryNonSeamProgramReachesThePayload(t *testing.T) {
	want := map[string]string{
		"gui.go":             "Backup Wallet (newInputFlow)",
		"passphrase_flow.go": "BIP-39 Password",
		"freetext_flow.go":   "Engrave Text",
		"bundle_flow.go":     "Engrave Bundle",
	}
	fset := token.NewFileSet()
	for file, prog := range want {
		f, err := parser.ParseFile(fset, file, nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", file, err)
		}
		var calls bool
		ast.Inspect(f, func(n ast.Node) bool {
			if id, ok := n.(*ast.Ident); ok && id.Name == "syswOffer" {
				calls = true
			}
			return !calls
		})
		if !calls {
			t.Errorf("%s (%s) never calls syswOffer, so the payload cannot reach it",
				file, prog)
		}
	}
}

// A dangling write is the defect this catches: syswBundleSeed was set and never
// read for one commit, which would have taken a card from the session and
// silently dropped it.
func TestTheBundleSeedIsBothWrittenAndRead(t *testing.T) {
	src, err := os.ReadFile("bundle_flow.go")
	if err != nil {
		t.Fatal(err)
	}
	s := string(src)
	if !strings.Contains(s, "ctx.syswBundleSeed = body") {
		t.Error("nothing writes the bundle seed")
	}
	if !strings.Contains(s, "seed := ctx.syswBundleSeed") {
		t.Error("nothing READS the bundle seed — the card would be taken and dropped")
	}
	// The SEED specifically must reach offer(). An earlier version of this
	// assertion looked for "scr.g.offer(" anywhere in the file — which is
	// present regardless, so replacing the seed's call with `_ = seed` left the
	// test GREEN. Found by mutation; the guard was passing for the wrong reason.
	if !strings.Contains(s, "scr.g.offer(mdmkText(seed))") {
		t.Error("the payload card must enter through offer() ITSELF, the same path a " +
			"scanned card takes, or only one of the two gets the dedup, chunk " +
			"assembly and validation")
	}
}

// Engrave Text must PRE-FILL, not bypass: a payload source that skipped the
// title, footer, size and confirm screens would engrave a plate nobody saw.
//
// ASSERTED OVER THE AST, not over a fixed window of characters. This test used
// to read the 400 bytes following the syswOffer call and forbid the word
// "return" anywhere in them. That window is not the offer branch: stage 10d put
// the F3 acceptance screen -- whose decline path is a legitimate `return` -- a
// few lines below the branch, and the test failed on code that had not bypassed
// anything. The property is about the BRANCH, so the branch is what is parsed.
func TestEngraveTextPreFillsRatherThanBypassing(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "freetext_flow.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var offer *ast.IfStmt
	ast.Inspect(f, func(n ast.Node) bool {
		is, ok := n.(*ast.IfStmt)
		if !ok || is.Init == nil {
			return true
		}
		var names bool
		ast.Inspect(is.Init, func(n ast.Node) bool {
			if id, ok := n.(*ast.Ident); ok &&
				(id.Name == "syswOffer" || id.Name == "ClassFreeText") {
				names = true
			}
			return true
		})
		if names {
			offer = is
		}
		return true
	})
	if offer == nil {
		t.Fatal("Engrave Text does not offer the payload")
	}
	var returns, prefills bool
	ast.Inspect(offer.Body, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.ReturnStmt:
			returns = true
		case *ast.AssignStmt:
			// text = string(raw)
			if len(n.Lhs) == 1 {
				if id, ok := n.Lhs[0].(*ast.Ident); ok && id.Name == "text" {
					prefills = true
				}
			}
		}
		return true
	})
	if returns {
		t.Error("the payload branch returns early, skipping the screens the operator " +
			"must still walk")
	}
	if !prefills {
		t.Error("the payload branch must pre-fill text")
	}
}
