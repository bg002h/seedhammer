package gui

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"seedhammer.com/md"
)

// F-449 stage 4: SPEC §7's print sites, the self-check, the one compose site,
// §6's refusal on the composer's mint path, and §7a.3's engrave half.

// TestComposerSelfCheckSeesTheKeyPathChoice: the self-check compares the
// operator's choice with the decoded card in both directions.
//
// Mutation: deleting the biconditional in composerSelfCheck fails both rows.
func TestComposerSelfCheckSeesTheKeyPathChoice(t *testing.T) {
	for _, tc := range []struct {
		chose, built md.UnspendableKind
	}{{md.UnspendableLiana, md.UnspendableNums}, {md.UnspendableNums, md.UnspendableLiana}} {
		st := composerStateFor(composerTrPreset(t, "kofn-recovery"), tc.chose)
		if err := composerSelfCheck(st, lianaKofnChunks(t, tc.built)); err == nil ||
			!strings.Contains(err.Error(), "key path") {
			t.Errorf("chose %v, built %v: self-check said %v", tc.chose, tc.built, err)
		}
		st.unspendable = tc.built
		if err := composerSelfCheck(st, lianaKofnChunks(t, tc.built)); err != nil {
			t.Errorf("an honest %v composition failed the self-check: %v", tc.built, err)
		}
	}
}

// TestComposerComposesOnlyThroughOneSite: every production lowering of the
// operator's path list goes through composerCompose, so the key-path choice
// reaches every artifact. A second call site that went straight to
// md.ComposeWith would build the NUMS wallet under a Liana choice, and the
// cards minted from it would carry the other wallet's stub.
//
// EXEMPT BY ENCLOSING FUNCTION, NOT BY FILE (R0 m4): a file exemption let a
// second compose inside composer_unspendable.go through unseen. Exactly three
// functions may compose: composerCompose (the site), composerUnspendableFires
// (composes kind 1 for itself, to evaluate §0b's predicate; it builds no
// artifact), and composerShapeSignature (md.Compose; reads only the slot map,
// which the kind does not move, SPEC §5).
//
// Mutations: composerArtifactsFor calling md.ComposeWith(st.list, declared)
// again fails; so does a second md.ComposeWithUnspendable added to
// composerUnspendableStep.
func TestComposerComposesOnlyThroughOneSite(t *testing.T) {
	allowed := map[string]string{
		"ComposeWith":            "composerCompose",
		"ComposeWithUnspendable": "composerCompose|composerUnspendableFires",
		"Compose":                "composerShapeSignature",
	}
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]int{}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		fset := token.NewFileSet()
		af, err := parser.ParseFile(fset, f, src, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range af.Decls {
			encl := "<package scope>"
			if fd, ok := decl.(*ast.FuncDecl); ok {
				encl = fd.Name.Name
			}
			ast.Inspect(decl, func(n ast.Node) bool {
				sel, ok := n.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				if x, ok := sel.X.(*ast.Ident); !ok || x.Name != "md" {
					return true
				}
				want, watched := allowed[sel.Sel.Name]
				if !watched {
					return true
				}
				seen[encl]++
				ok = false
				for _, a := range strings.Split(want, "|") {
					ok = ok || a == encl
				}
				if !ok {
					t.Errorf("%s: md.%s in %s; only %s may compose", fset.Position(sel.Pos()), sel.Sel.Name, encl, want)
				}
				return true
			})
		}
	}
	// The allowed sites must still exist, or the rule is guarding a name.
	for _, fn := range []string{"composerCompose", "composerUnspendableFires", "composerShapeSignature"} {
		if seen[fn] != 1 {
			t.Errorf("%s composes %d times, want exactly 1", fn, seen[fn])
		}
	}
}

// TestComposerComposeRefusesAnUnmetLianaRequest is SPEC §6 row 3 on the
// composer's mint path (R0 m1): a Liana choice over a shape whose first bare
// single key became the internal key is refused before any chunk exists. The
// reset keeps the flow out of this state, so it is constructed.
//
// Mutation: deleting the UnspendableRequestUnmet check in composerCompose fails.
func TestComposerComposeRefusesAnUnmetLianaRequest(t *testing.T) {
	realKey := md.PathList{Wrapper: md.ComposeTr, Paths: []md.SpendPath{
		{Keys: &md.KeySet{K: 1, N: 1}},
		{Keys: &md.KeySet{K: 1, N: 1}, Lock: &md.Lock{Kind: md.LockOlderBlocks, Value: 26280}},
	}}
	st := composerStateFor(realKey, md.UnspendableLiana)
	if _, err := composerTemplateChunksFor(st); err == nil || err.Error() != composerCopyLianaUnmet() {
		t.Fatalf("an unmet Liana request composed: err %v", err)
	}
	st.unspendable = md.UnspendableNums
	if _, err := composerTemplateChunksFor(st); err != nil {
		t.Fatalf("the same shape at NUMS refused: %v", err)
	}
}

// TestComposerComposeRefusesSpecSix: SPEC §6 on the composer's mint path
// (F-654). The predicate keeps a Liana choice off plain-multisig, so the state
// is set directly: the refusal is the belt for the day the predicate changes.
//
// Mutation: deleting the ValidateUnspendableShape call in composerCompose fails.
func TestComposerComposeRefusesSpecSix(t *testing.T) {
	st := composerStateFor(composerTrPreset(t, "plain-multisig"), md.UnspendableLiana)
	if _, err := composerTemplateChunksFor(st); !errors.Is(err, md.ErrUnspendableSortedMultiA) {
		t.Fatalf("a Liana key over sortedmulti_a composed: err %v", err)
	}
	st.unspendable = md.UnspendableNums
	if _, err := composerTemplateChunksFor(st); err != nil {
		t.Fatalf("the NUMS plain-multisig refused: %v", err)
	}
}
