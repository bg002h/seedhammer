package gui

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestNoVerifyFlowCanReachAPayloadSecret is spec test 16, and it is STRUCTURAL
// rather than behavioural on purpose.
//
// §7.4: a verify that accepted the same secret the engrave used would compare
// the engrave source against itself and pass unconditionally — certifying a
// WRONG PLATE as good, silently. R0-C1 showed a behavioural test could be
// satisfied at the session layer while the UI still offered the option, so the
// guarantee is instead that a verify flow has no way to NAME the payload source.
//
// The match is on the AST IDENTIFIER, not a substring: seedEntryFlowTypedOnly
// CONTAINS seedEntryFlow, so strings.Contains fails on a correct implementation
// (R1-N1).
func TestNoVerifyFlowCanReachAPayloadSecret(t *testing.T) {
	ents, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("reading gui: %v", err)
	}
	fset := token.NewFileSet()
	var checked int
	for _, e := range ents {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, "_verify.go") {
			continue
		}
		checked++
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", name, err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			id, ok := n.(*ast.Ident)
			if ok && id.Name == "seedEntryFlow" {
				t.Errorf("%s calls seedEntryFlow, which offers the PAYLOAD as a source. "+
					"A verify that accepts the engrave's own secret compares it against "+
					"itself and passes unconditionally. Use seedEntryFlowTypedOnly.",
					filepath.Base(name))
			}
			return true
		})
	}
	if checked < 2 {
		t.Fatalf("INCONCLUSIVE: only %d *_verify.go files scanned; singlesig_verify.go "+
			"and multisig_verify.go must both be present or this test guards nothing", checked)
	}
	t.Logf("%d verify flows scanned, none can name the payload source", checked)
}

// The gate is per-invocation, and every EXISTING caller wants it on: they are
// all seed entry, where it catches a mistyped last word.
func TestTheChecksumGateIsOnForSeedEntry(t *testing.T) {
	src, err := os.ReadFile("derive_xpub.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(src), "wordEntryOpts{checksumGate: true}") {
		t.Error("seed entry must keep the checksum gate: it catches a mistyped last word " +
			"before anything is derived")
	}
}
