package main

import (
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"strings"
	"testing"
)

// machineParams is the source of platform_sh2.go, which is the ORIGINAL of the
// constants this command copies. It cannot be imported -- that file is
// `tinygo && rp` -- so it is parsed instead.
const machineParams = "../controller/platform_sh2.go"

// TestParamsMatchTheMachine pins every constant plateview duplicates against
// the machine's own declaration.
//
// It compares the EXPRESSION SOURCE, not a number: `topSpeed = 30 * mm` and
// `topSpeed = 30 * mm` agree even though neither side evaluates mm the same
// way here. That is sufficient because mm's own inputs are pinned too, so
// there is no path by which a machine value changes and every string still
// matches.
//
// Without this the copy is a comment claiming to be a fact. A drifted
// acceleration or jerk does not fail anything -- PlanEngraving happily plans a
// toolpath for the wrong machine and the preview looks entirely plausible.
func TestParamsMatchTheMachine(t *testing.T) {
	want := map[string]string{
		"fullStepsPerRevolution": "200",
		"mmPerRevolution":        "8",
		"mm":                     "fullStepsPerRevolution / mmPerRevolution * tmc2209.Microsteps",
		"strokeWidth":            "0.3 * mm",
		"topSpeed":               "30 * mm",
		"engravingSpeed":         "8 * mm",
		"acceleration":           "250 * mm",
		"jerk":                   "2600 * mm",
	}

	got := constExprs(t, machineParams)
	for name, exp := range want {
		switch have, ok := got[name]; {
		case !ok:
			t.Errorf("%s no longer declares %s; plateview's copy is now unanchored",
				machineParams, name)
		case have != exp:
			t.Errorf("%s = %s in %s, but plateview was written against %s -- "+
				"update cmd/plateview/main.go and this table together",
				name, have, machineParams, exp)
		}
	}
}

// constExprs returns every top-level constant in a file as name -> expression
// source. Build tags are irrelevant to the parser, which is what lets a host
// test read a TinyGo-only file.
func constExprs(t *testing.T, path string) map[string]string {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}
	out := map[string]string{}
	for _, d := range f.Decls {
		gd, ok := d.(*ast.GenDecl)
		if !ok || gd.Tok != token.CONST {
			continue
		}
		for _, s := range gd.Specs {
			vs := s.(*ast.ValueSpec)
			for i, n := range vs.Names {
				if i >= len(vs.Values) {
					continue
				}
				var b strings.Builder
				if err := printer.Fprint(&b, fset, vs.Values[i]); err != nil {
					t.Fatalf("printing %s: %v", n.Name, err)
				}
				out[n.Name] = b.String()
			}
		}
	}
	if len(out) == 0 {
		// A parse that finds nothing would pass every lookup above as
		// "missing" only if the loop ran -- but an empty map with a renamed
		// file would be silent. Fail loudly instead.
		t.Fatalf("no constants found in %s", path)
	}
	return out
}
