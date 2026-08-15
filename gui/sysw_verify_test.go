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
			// The two payload-offering entry points, named exactly. S4 added
			// seedEntryFlowTitled so a per-seed entry can name its slot, and it
			// reaches the payload exactly as far as seedEntryFlow does; leaving it
			// off this list would have reopened the door §7.4 closes. The
			// ...TypedOnly variants are the SAFE ones and must not be matched,
			// which is why this is an identifier set and never a substring (R1-N1).
			if ok && (id.Name == "seedEntryFlow" || id.Name == "seedEntryFlowTitled" ||
				id.Name == "syswSeedPicker" || id.Name == "syswSeedPickerTitled") {
				t.Errorf("%s calls %s, which offers the PAYLOAD as a source. "+
					"A verify that accepts the engrave's own secret compares it against "+
					"itself and passes unconditionally. Use seedEntryFlowTypedOnly.",
					filepath.Base(name), id.Name)
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
	// The FIELD, not the whole literal: S4 added titlePrefix to the same struct
	// so a per-seed entry can name its slot on the word screen, and a test keyed
	// to the exact literal would report the gate missing because a sibling field
	// appeared beside it.
	if !strings.Contains(string(src), "wordEntryOpts{checksumGate: true") {
		t.Error("seed entry must keep the checksum gate: it catches a mistyped last word " +
			"before anything is derived")
	}
}
