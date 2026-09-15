package gui

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"seedhammer.com/md"
)

// H6 §2.2: retention of phrase, method and preimage for the composition's
// lifetime, keyed by digest, scrubbed by the flow-exit defer.

// composerH6ZeroState builds composerState EXACTLY as composerFlow does -- a
// struct literal setting two fields and leaving every other one zero -- so
// hashlockHeld arrives NIL.
//
// THE NIL IS THE POINT. An assignment into a nil map panics, and this is the
// one production construction site, so a helper that pre-allocated would hide
// the defect on the machine at the moment the operator holds to confirm a hash
// that gates funds.
func composerH6ZeroState(t *testing.T, paths int) *composerState {
	t.Helper()
	st := &composerState{reg: &seedRegistry{}, bound: composerBoundFrom(nil)}
	if st.hashlockHeld != nil {
		t.Fatal("this helper exists to reproduce the NIL map composerFlow leaves; it is not nil")
	}
	st.list = md.PathList{Wrapper: md.ComposeWsh, Paths: make([]md.SpendPath, paths)}
	return st
}

// TestComposerHoldsHashlockMaterialOnTheZeroValueState.
//
// MUTATION: assign straight into st.hashlockHeld without the nil check ->
// `panic: assignment to entry in nil map`.
func TestComposerHoldsHashlockMaterialOnTheZeroValueState(t *testing.T) {
	st := composerH6ZeroState(t, 1)
	var raw [32]byte
	raw[0], raw[31] = 0xb8, 0xcb
	h := composerTestLock(raw)
	composerHoldHashlockMaterial(st, h, hashlockMaterial{
		phrase:     []byte("correct horse battery staple"),
		method:     hashlockSHA256,
		preimage:   [32]byte{1, 2, 3},
		provenance: hashlockFromPhrase,
	})
	m, ok := st.hashlockHeld[h.MapKey()]
	if !ok {
		t.Fatalf("the digest is not held (%d entries)", len(st.hashlockHeld))
	}
	if string(m.phrase) != "correct horse battery staple" || m.provenance != hashlockFromPhrase {
		t.Errorf("held material is %q/%v", m.phrase, m.provenance)
	}
}

// TestComposerFlowExitScrubsHeldHashlockMaterial is §2.2 item 3, driven through
// composerFlowExit -- the FUNCTION composerFlow defers -- rather than through
// composerScrubHashlockHeld directly, so a scrub that existed but was never
// wired into the defer would still red here.
//
// MUTATION: remove the composerScrubHashlockHeld call from composerFlowExit ->
// the phrase is still readable after exit and both assertions below fail.
// MUTATION: range over the map and wipe `m` without writing it back -> the
// PREIMAGE assertion fails (the byte array is a value in the map), while the
// phrase one still passes because a []byte's backing store is shared -- which
// is why both are asserted.
func TestComposerFlowExitScrubsHeldHashlockMaterial(t *testing.T) {
	st := composerH6ZeroState(t, 1)
	h := composerTestLock([32]byte{0x7f})
	phrase := []byte("correct horse battery staple")
	composerHoldHashlockMaterial(st, h, hashlockMaterial{
		phrase:     phrase,
		method:     hashlockHardened,
		preimage:   [32]byte{0xab, 0xcd, 0xef},
		provenance: hashlockFromPayload,
	})

	composerFlowExit(st)

	if !bytes.Equal(phrase, make([]byte, len(phrase))) {
		t.Errorf("the phrase survived the flow-exit defer: %q", phrase)
	}
	m := st.hashlockHeld[h.MapKey()]
	if m.preimage != ([32]byte{}) {
		t.Errorf("the preimage survived the flow-exit defer: %x", m.preimage)
	}
	if m.phrase != nil {
		t.Errorf("the phrase slice was not released: %v", m.phrase)
	}
}

// TestComposerScrubIsInTheExistingDefer pins §2.2 item 3's OTHER half: the
// scrub rides composerFlowExit rather than a second `defer`, which measured
// +96 B of flash because TinyGo removes the empty stub's CALL and not the
// defer bookkeeping around it.
//
// It is asserted on the AST because there is nothing at runtime to observe:
// two defers and one defer behave identically and differ only in flash.
//
// MUTATION: add `defer composerScrubHashlockHeld(st)` to composerFlow -> the
// count below is 2 and this fails with "composerFlow has 2 defers".
func TestComposerScrubIsInTheExistingDefer(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "composer_flow.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	defers, callsScrub := 0, false
	for _, d := range f.Decls {
		fn, ok := d.(*ast.FuncDecl)
		if !ok {
			continue
		}
		switch fn.Name.Name {
		case "composerFlow":
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				if _, ok := n.(*ast.DeferStmt); ok {
					defers++
				}
				return true
			})
		case "composerFlowExit":
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				if id, ok := call.Fun.(*ast.Ident); ok && id.Name == "composerScrubHashlockHeld" {
					callsScrub = true
				}
				return true
			})
		}
	}
	if defers != 1 {
		t.Errorf("composerFlow has %d defers; H6 §2.2 item 3 and H5's 96 B measurement "+
			"both require exactly one", defers)
	}
	if !callsScrub {
		t.Error("composerFlowExit does not call composerScrubHashlockHeld")
	}
}
