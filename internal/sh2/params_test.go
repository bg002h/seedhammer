package sh2

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/parser"
	"go/token"
	"testing"
)

// machineParams is the source of platform_sh2.go, the ORIGINAL of every
// constant in this package. It cannot be imported -- that file is
// `tinygo && rp` -- so it is parsed instead.
const machineParams = "../../cmd/controller/platform_sh2.go"

// TestParamsMatchTheMachine evaluates the machine's own declarations and
// compares the NUMBERS to this package's.
//
// Comparing numbers rather than expression text matters in both directions: a
// machine that changed mmPerRevolution would double every derived value while
// every expression string stayed identical, and a typo HERE is invisible to
// any test that only reads the other file. The first version of this test read
// only platform_sh2.go, and would have passed with any values at all on this
// side.
func TestParamsMatchTheMachine(t *testing.T) {
	env := map[string]constant.Value{
		// params.go imports tmc2209.Microsteps for real; giving the evaluator
		// the same number keeps mm's formula checkable rather than assumed.
		"tmc2209.Microsteps": constant.MakeInt64(256),
	}
	exprs := constExprs(t, machineParams)

	// Resolve in dependency order, so mm is defined before the speeds use it.
	for _, name := range []string{"fullStepsPerRevolution", "mmPerRevolution", "mm"} {
		e, ok := exprs[name]
		if !ok {
			t.Fatalf("%s no longer declares %s; internal/sh2 is unanchored", machineParams, name)
		}
		v, err := eval(e, env)
		if err != nil {
			t.Fatalf("evaluating %s: %v", name, err)
		}
		env[name] = v
	}

	want := map[string]int64{
		"fullStepsPerRevolution": FullStepsPerRevolution,
		"mmPerRevolution":        MMPerRevolution,
		"mm":                     MM,
		"strokeWidth":            StrokeWidth,
		"topSpeed":               TopSpeed,
		"engravingSpeed":         EngravingSpeed,
		"acceleration":           Acceleration,
		"jerk":                   Jerk,
		"lcdWidth":               DisplayWidth,
		"lcdHeight":              DisplayHeight,
	}
	for name, ours := range want {
		e, ok := exprs[name]
		if !ok {
			t.Errorf("%s no longer declares %s; internal/sh2 is unanchored", machineParams, name)
			continue
		}
		v, err := eval(e, env)
		if err != nil {
			t.Errorf("evaluating %s: %v", name, err)
			continue
		}
		theirs, exact := constant.Int64Val(constant.ToInt(v))
		if !exact {
			t.Errorf("%s is not an integer in %s", name, machineParams)
			continue
		}
		if theirs != ours {
			t.Errorf("%s = %d in %s, but internal/sh2 says %d -- "+
				"the machine is the original; update params.go",
				name, theirs, machineParams, ours)
		}
	}
}

// eval does NOT execute code. It folds a Go constant expression that has
// already been parsed into an AST, over a fixed environment, using go/constant
// arithmetic -- the same evaluation the compiler does for untyped constants.
// Every node type it accepts is listed below and anything else is an error, so
// there is no path from file contents to execution.
//
// It computes a constant expression over env. It covers exactly the forms
// platform_sh2.go uses for these constants -- literals, identifiers, selectors,
// parentheses and + - * / -- and returns an error rather than a guess for
// anything else, so an expression that grows a form this does not model fails
// the test instead of silently comparing the wrong thing.
func eval(e ast.Expr, env map[string]constant.Value) (constant.Value, error) {
	switch e := e.(type) {
	case *ast.BasicLit:
		v := constant.MakeFromLiteral(e.Value, e.Kind, 0)
		if v.Kind() == constant.Unknown {
			return nil, fmt.Errorf("bad literal %s", e.Value)
		}
		return v, nil
	case *ast.Ident:
		v, ok := env[e.Name]
		if !ok {
			return nil, fmt.Errorf("unknown identifier %s", e.Name)
		}
		return v, nil
	case *ast.SelectorExpr:
		pkg, ok := e.X.(*ast.Ident)
		if !ok {
			return nil, fmt.Errorf("unsupported selector")
		}
		name := pkg.Name + "." + e.Sel.Name
		v, ok := env[name]
		if !ok {
			return nil, fmt.Errorf("unknown identifier %s", name)
		}
		return v, nil
	case *ast.ParenExpr:
		return eval(e.X, env)
	case *ast.BinaryExpr:
		x, err := eval(e.X, env)
		if err != nil {
			return nil, err
		}
		y, err := eval(e.Y, env)
		if err != nil {
			return nil, err
		}
		switch e.Op {
		case token.MUL, token.ADD, token.SUB, token.QUO:
			return constant.BinaryOp(x, e.Op, y), nil
		}
		return nil, fmt.Errorf("unsupported operator %s", e.Op)
	}
	return nil, fmt.Errorf("unsupported expression %T", e)
}

// constExprs returns every top-level constant in a file as name -> expression.
// Build tags are irrelevant to the parser, which is what lets a host test read
// a TinyGo-only file.
func constExprs(t *testing.T, path string) map[string]ast.Expr {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}
	out := map[string]ast.Expr{}
	for _, d := range f.Decls {
		gd, ok := d.(*ast.GenDecl)
		if !ok || gd.Tok != token.CONST {
			continue
		}
		for _, s := range gd.Specs {
			vs := s.(*ast.ValueSpec)
			for i, n := range vs.Names {
				if i < len(vs.Values) {
					out[n.Name] = vs.Values[i]
				}
			}
		}
	}
	if len(out) == 0 {
		t.Fatalf("no constants found in %s", path)
	}
	return out
}
