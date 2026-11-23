package oracle

// FIX-sh-oracle-live-discovery (bughunt-1119 bh-sh-I11): the runner
// scripts/oracle-live.sh must find and run every //go:build oraclelive test in
// the module, in its own package, and fail when one of them fails or never
// runs. These tests run the real script with a stub `go` first on PATH: the
// stub records which package was asked for which test names and prints the
// `=== RUN` lines a real `go test -v` would, so the script's discovery and its
// vacuity check are exercised without the pinned Rust oracles.
//
// This file is UNTAGGED on purpose: it runs in every `go test ./...`, so the
// runner's discovery is checked on every push, not only when a maintainer
// runs the live checks by hand.

import (
	"go/ast"
	"go/build/constraint"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// goStub stands in for `go test -tags oraclelive -count=1 -v -run '^(A|B)$'
// <pkg>... [args]`. For every package argument it appends "<pkg>\t<name>" per
// requested name to $STUB_LOG, prints a RUN/PASS pair per name unless
// "<pkg>:<name>" is in $STUB_SKIP, and fails the package if "<pkg>" is in
// $STUB_FAIL.
const goStub = `#!/usr/bin/env bash
run=; pkgs=()
while [ $# -gt 0 ]; do
  case "$1" in
    -run) run=$2; shift 2 ;;
    ./*) pkgs+=("$1"); shift ;;
    *) shift ;;
  esac
done
names=${run#^(}; names=${names%')$'}
IFS='|' read -ra ns <<<"$names"
rc=0
for p in "${pkgs[@]}"; do
  for n in "${ns[@]}"; do
    printf '%s\t%s\n' "$p" "$n" >>"$STUB_LOG"
    case " ${STUB_SKIP:-} " in *" $p:$n "*) continue ;; esac
    echo "=== RUN   $n"
    echo "--- PASS: $n (0.00s)"
  done
  case " ${STUB_FAIL:-} " in
    *" $p "*) echo "--- FAIL: synthetic"; echo "FAIL	$p"; rc=1 ;;
    *) echo "ok  	$p	0.001s" ;;
  esac
done
exit $rc
`

type runnerResult struct {
	exit  int
	out   string
	pairs []string // "<pkg> <Test>", sorted, one per (package, name) the stub was asked to run
}

// runRunner runs the oracle-live.sh found at <root>/scripts/ with the stub `go`.
func runRunner(t *testing.T, root, skip, fail string) runnerResult {
	t.Helper()
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Fatalf("bash is required to test scripts/oracle-live.sh: %v", err)
	}
	stubDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(stubDir, "go"), []byte(goStub), 0o755); err != nil {
		t.Fatal(err)
	}
	log := filepath.Join(stubDir, "stub.log")
	cmd := exec.Command(bash, filepath.Join(root, "scripts", "oracle-live.sh"))
	cmd.Env = append(os.Environ(),
		"PATH="+stubDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"STUB_LOG="+log, "STUB_SKIP="+skip, "STUB_FAIL="+fail)
	out, err := cmd.CombinedOutput()
	res := runnerResult{out: string(out)}
	if ee, ok := err.(*exec.ExitError); ok {
		res.exit = ee.ExitCode()
	} else if err != nil {
		t.Fatalf("running oracle-live.sh: %v", err)
	}
	raw, err := os.ReadFile(log)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	for _, l := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		if l != "" {
			res.pairs = append(res.pairs, strings.Replace(l, "\t", " ", 1))
		}
	}
	sort.Strings(res.pairs)
	return res
}

// liveTaggedTests is the independent oracle: every top-level Test function in
// a file that the Go build constraint includes only under the oraclelive tag,
// in every directory `./...` would visit (Go's own rules: skip testdata and
// names starting with "." or "_"; skip vendor).
func liveTaggedTests(t *testing.T, root string) []string {
	t.Helper()
	var pairs []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if d.IsDir() {
			if path != root && (name == "testdata" || name == "vendor" ||
				strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_")) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(name, ".go") {
			return nil
		}
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			return err
		}
		tagOnly := false
		for _, cg := range f.Comments {
			if cg.Pos() >= f.Package {
				break
			}
			for _, c := range cg.List {
				if !constraint.IsGoBuild(c.Text) {
					continue
				}
				expr, err := constraint.Parse(c.Text)
				if err != nil {
					return err
				}
				with := expr.Eval(func(tag string) bool { return tag == "oraclelive" || tag == "linux" || tag == "amd64" })
				without := expr.Eval(func(tag string) bool { return tag == "linux" || tag == "amd64" })
				tagOnly = with && !without
			}
		}
		if !tagOnly {
			return nil
		}
		rel, err := filepath.Rel(root, filepath.Dir(path))
		if err != nil {
			return err
		}
		pkg := "./" + filepath.ToSlash(rel) + "/"
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if ok && fn.Recv == nil && strings.HasPrefix(fn.Name.Name, "Test") && fn.Name.Name != "TestMain" {
				pairs = append(pairs, pkg+" "+fn.Name.Name)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(pairs)
	return pairs
}

// syntheticTree builds a module-shaped tree holding a copy of the real script
// and the given files (path -> content).
func syntheticTree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	script, err := os.ReadFile(filepath.Join("..", "scripts", "oracle-live.sh"))
	if err != nil {
		t.Fatal(err)
	}
	files["scripts/oracle-live.sh"] = string(script)
	for p, c := range files {
		full := filepath.Join(root, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(c), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func taggedFile(pkg string, tests ...string) string {
	s := "//go:build oraclelive\n\npackage " + pkg + "\n\nimport \"testing\"\n"
	for _, n := range tests {
		s += "\nfunc " + n + "(t *testing.T) {}\n"
	}
	return s
}

func diffPairs(got, want []string) (missing, extra []string) {
	g := map[string]bool{}
	for _, p := range got {
		g[p] = true
	}
	w := map[string]bool{}
	for _, p := range want {
		w[p] = true
		if !g[p] {
			missing = append(missing, p)
		}
	}
	for _, p := range got {
		if !w[p] {
			extra = append(extra, p)
		}
	}
	return
}

// On the real tree, the runner asks for exactly the tagged tests Go would
// build, each in its own package, and the cmd/emu digest test is among them.
func TestFixShOracleLiveRunsEveryTaggedTestInTheTree(t *testing.T) {
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	want := liveTaggedTests(t, root)
	const emu = "./cmd/emu/ TestSyswComposerPayloadDigestAgreesWithMe"
	found := false
	for _, p := range want {
		found = found || p == emu
	}
	if !found {
		t.Fatalf("independent discovery lost %q; it found %q", emu, want)
	}
	res := runRunner(t, root, "", "")
	missing, extra := diffPairs(res.pairs, want)
	if res.exit != 0 || len(missing) != 0 || len(extra) != 0 {
		t.Fatalf("oracle-live.sh exit %d\nnever asked for: %q\nasked for, not tagged in that package: %q\noutput:\n%s",
			res.exit, missing, extra, res.out)
	}
}

// A tagged test in a package the script has never heard of is run, and when
// it fails the runner fails.
func TestFixShOracleLiveFailsOnAFailingNewPackage(t *testing.T) {
	root := syntheticTree(t, map[string]string{
		"oracle/live_test.go": taggedFile("oracle", "TestO"),
		"gui/live_test.go":    taggedFile("gui", "TestG"),
		"sysw/live_test.go":   taggedFile("sysw", "TestS"),
		"newpkg/live_test.go": taggedFile("newpkg", "TestN"),
	})
	res := runRunner(t, root, "", "./newpkg/")
	asked := false
	for _, p := range res.pairs {
		asked = asked || p == "./newpkg/ TestN"
	}
	if res.exit == 0 || !asked || !strings.Contains(res.out, "live checks: FAIL (exit 1)") {
		t.Fatalf("a failing tagged test in ./newpkg/: exit %d, asked for it: %v; want a nonzero exit, the test asked for, and \"live checks: FAIL (exit 1)\"\noutput:\n%s",
			res.exit, asked, res.out)
	}
}

// The vacuity check is per package: a test name that ran in one package does
// not pay for the same name that never ran in another.
func TestFixShOracleLiveReconcilesByPackage(t *testing.T) {
	root := syntheticTree(t, map[string]string{
		"oracle/live_test.go": taggedFile("oracle", "TestSame"),
		"gui/live_test.go":    taggedFile("gui", "TestSame"),
		"sysw/live_test.go":   taggedFile("sysw", "TestS"),
	})
	res := runRunner(t, root, "./gui/:TestSame", "")
	const line = "\n         ./gui/ TestSame\n"
	if res.exit != 1 || !strings.Contains(res.out, line) {
		t.Fatalf("./gui/ TestSame never ran: exit %d; want exit 1 and the line %q\noutput:\n%s", res.exit, line, res.out)
	}
}

// Directories `./...` never builds are not searched.
func TestFixShOracleLiveSkipsDirsGoIgnores(t *testing.T) {
	root := syntheticTree(t, map[string]string{
		"oracle/live_test.go":          taggedFile("oracle", "TestO"),
		"oracle/testdata/live_test.go": taggedFile("x", "TestTestdata"),
		".hidden/live_test.go":         taggedFile("x", "TestHidden"),
		"_skip/live_test.go":           taggedFile("x", "TestUnderscore"),
		"vendor/v/live_test.go":        taggedFile("v", "TestVendor"),
	})
	res := runRunner(t, root, "", "")
	const line = "   discovered 1 tagged test(s) in 1 package(s) from source\n"
	want := []string{"./oracle/ TestO"}
	if res.exit != 0 || !strings.Contains(res.out, line) || strings.Join(res.pairs, ",") != strings.Join(want, ",") {
		t.Fatalf("exit %d, asked for %q; want exit 0, %q and the line %q\noutput:\n%s", res.exit, res.pairs, want, line, res.out)
	}
}

// TestFixShOracleLiveRefusesAPathWithWhitespace: a tagged file whose path holds
// whitespace is refused (go cannot load it), never dropped with a PASS.
func TestFixShOracleLiveRefusesAPathWithWhitespace(t *testing.T) {
	root := syntheticTree(t, map[string]string{
		"oracle/live_test.go": taggedFile("oracle", "TestO"),
		"a b/live_test.go":    taggedFile("ab", "TestSpace"),
	})
	res := runRunner(t, root, "", "")
	const line = "::error::a tagged file's path holds whitespace; go cannot load it and this script cannot run it:\n         ./a b/live_test.go\n"
	if res.exit != 1 || !strings.Contains(res.out, line) {
		t.Fatalf("exit %d; want exit 1 and the line %q\noutput:\n%s", res.exit, line, res.out)
	}
}

// TestFixShOracleLiveRefusesAPathWithANewline: a newline is whitespace too, but
// grep -l prints one name per line, so a name holding one splits in two and the
// per-line whitespace check never sees it; the tagged file was dropped and the
// run printed PASS.
func TestFixShOracleLiveRefusesAPathWithANewline(t *testing.T) {
	root := syntheticTree(t, map[string]string{
		"oracle/live_test.go": taggedFile("oracle", "TestO"),
		"a\nb/live_test.go":   taggedFile("ab", "TestNewline"),
	})
	res := runRunner(t, root, "", "")
	const line = "::error::a tagged file's path holds whitespace; go cannot load it and this script cannot run it:\n         ./a b/live_test.go\n"
	if res.exit != 1 || !strings.Contains(res.out, line) {
		t.Fatalf("exit %d; want exit 1 and the line %q\noutput:\n%s", res.exit, line, res.out)
	}
}

// TestFixShOracleLiveDoesNotGlobAPath: the path list is split unquoted, so a
// bracket in a directory name is a glob; `./x[ab]/live_test.go` expanded to
// `./xa/live_test.go`, the bracket package was dropped and the run printed PASS.
// Go cannot load `./x[ab]/` (malformed import path), which the stub stands in
// for by failing that package.
func TestFixShOracleLiveDoesNotGlobAPath(t *testing.T) {
	root := syntheticTree(t, map[string]string{
		"x[ab]/live_test.go": taggedFile("xab", "TestBracket"),
		"xa/live_test.go":    taggedFile("xa", "TestXA"),
	})
	res := runRunner(t, root, "", "./x[ab]/")
	const line = "\n         ./x[ab]/ TestBracket\n"
	if res.exit != 1 || !strings.Contains(res.out, line) {
		t.Fatalf("exit %d; want exit 1 and the never-run line %q\noutput:\n%s", res.exit, line, res.out)
	}
}
