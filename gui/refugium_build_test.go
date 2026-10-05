package gui

import (
	"go/ast"
	"go/build/constraint"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// The Refugium build's SOURCE-LEVEL gates (Refugium plan F7 §4.1 and §4.5).
// UNTAGGED, so every `go test` runs them -- the default suite and the tagged
// one alike -- and each has its positive control in the same function.
//
// Why source and not bytes (R0 I-1): gc lowers a `switch` against short
// constant strings into integer compares, so `FOREVERLAURA!` never appears
// contiguously in a binary. A byte scan's positive control fails on today's
// tree and an absence check would pass vacuously. The device ELF check
// (scripts/refugium-elf-check.sh) covers what does survive into the binary.

// refugiumForbiddenLiterals are the string literals no file in a Refugium
// build may contain, whole or as a substring.
var refugiumForbiddenLiterals = []string{
	"FOREVERLAURA!",
	"lock-boot",
	"PASSPROOF!",
	"TEXTPROOF!",
	"CONSTPROOF!",
	"BOTHPROOF!",
	"SIZEPROOF!FRONT",
	"SIZEPROOF!BACK",
	"https://seedhammer.com/doc/?d=SHII",
}

// refugiumForbiddenCalls are the OTP writers no Refugium file may call: the
// lock-boot writer and every driver/otp function that writes.
var refugiumForbiddenCalls = []string{
	"writeOTPValues",
	"AddBootKey",
	"EnableSecureBoot",
	"WriteWhiteLabelAddr",
	"WriteWhiteLabelString",
	"WriteBootKey",
}

// goTool is the go command of the toolchain running this test.
func goTool() string {
	return filepath.Join(runtime.GOROOT(), "bin", "go")
}

// goListFiles returns the absolute paths of a package's GoFiles under tags.
// GOFLAGS is cleared so the tagged suite's GOFLAGS=-tags=refugium cannot leak
// into a listing that names its own tags.
func goListFiles(t *testing.T, tags, pkg string) []string {
	t.Helper()
	cmd := exec.Command(goTool(), "list", "-e", "-tags", tags,
		"-f", "{{$d := .Dir}}{{range .GoFiles}}{{$d}}/{{.}}\n{{end}}", pkg)
	cmd.Dir = ".."
	cmd.Env = append(os.Environ(), "GOFLAGS=")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list -tags %q %s: %v", tags, pkg, err)
	}
	var files []string
	for _, l := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if l != "" {
			files = append(files, l)
		}
	}
	if len(files) == 0 {
		t.Fatalf("INCONCLUSIVE: go list -tags %q %s listed no files", tags, pkg)
	}
	return files
}

// fileSet is the union of the listings, deduplicated and sorted.
func fileSet(t *testing.T, pkg string, tagSets ...string) []string {
	t.Helper()
	var all []string
	for _, tags := range tagSets {
		all = append(all, goListFiles(t, tags, pkg)...)
	}
	slices.Sort(all)
	return slices.Compact(all)
}

// sourceFacts is what a file set says, read from its AST: every string
// literal's value, and every called function's name (the last selector).
// Comments are not in either.
type sourceFacts struct {
	literals map[string][]string // value -> files
	calls    map[string][]string // name -> files
}

func readSourceFacts(t *testing.T, files []string) sourceFacts {
	t.Helper()
	f := sourceFacts{literals: map[string][]string{}, calls: map[string][]string{}}
	fset := token.NewFileSet()
	for _, path := range files {
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", path, err)
		}
		name := filepath.Base(path)
		ast.Inspect(file, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.BasicLit:
				if n.Kind == token.STRING {
					if v, err := strconv.Unquote(n.Value); err == nil {
						f.literals[v] = append(f.literals[v], name)
					}
				}
			case *ast.CallExpr:
				switch fn := n.Fun.(type) {
				case *ast.Ident:
					f.calls[fn.Name] = append(f.calls[fn.Name], name)
				case *ast.SelectorExpr:
					f.calls[fn.Sel.Name] = append(f.calls[fn.Sel.Name], name)
				}
			}
			return true
		})
	}
	return f
}

// literalHits returns the files whose string literals contain needle.
func (f sourceFacts) literalHits(needle string) []string {
	var hits []string
	for v, files := range f.literals {
		if strings.Contains(v, needle) {
			hits = append(hits, files...)
		}
	}
	slices.Sort(hits)
	return slices.Compact(hits)
}

// TestRefugiumFileSetsCarryNoForbiddenLiteralOrCall is §4.5's source-level
// gate. The Refugium file sets are the union of the host and device listings
// for gui (round 3 M-4: the device set adds the *_tinygo.go hooks and drops
// preview.go) and the device listing for cmd/controller (round 2 M-2). The
// positive control lists the DEFAULT sets the same way and requires each
// literal and each call there, so a listing that silently matched nothing --
// or a check that stopped reading -- fails instead of passing.
func TestRefugiumFileSetsCarryNoForbiddenLiteralOrCall(t *testing.T) {
	refugium := readSourceFacts(t, append(
		fileSet(t, "./gui", "refugium", "tinygo,rp,refugium"),
		fileSet(t, "./cmd/controller", "tinygo,rp,refugium")...))
	def := readSourceFacts(t, append(
		fileSet(t, "./gui", "", "tinygo,rp"),
		fileSet(t, "./cmd/controller", "tinygo,rp")...))

	for _, lit := range refugiumForbiddenLiterals {
		if hits := def.literalHits(lit); len(hits) == 0 {
			t.Errorf("POSITIVE CONTROL: no default-build file has a string literal containing %q, "+
				"so its absence from the Refugium build proves nothing", lit)
		}
		if hits := refugium.literalHits(lit); len(hits) > 0 {
			t.Errorf("the Refugium build carries a string literal containing %q, in %v", lit, hits)
		}
	}
	for _, call := range refugiumForbiddenCalls {
		// WriteBootKey has no caller in either build today; it is listed so a
		// Refugium caller is caught, and is exempt from the control only.
		if call != "WriteBootKey" && len(def.calls[call]) == 0 {
			t.Errorf("POSITIVE CONTROL: no default-build file calls %s, "+
				"so its absence from the Refugium build proves nothing", call)
		}
		if files := refugium.calls[call]; len(files) > 0 {
			t.Errorf("the Refugium build calls %s, in %v", call, files)
		}
	}
}

// refugiumPairPackages are the packages carrying *_refugium.go files, and the
// constraint every such file conjoins with `refugium` (X in §4.1's rule).
var refugiumPairPackages = []struct {
	dir string
	x   string
}{
	{"../gui", ""},
	{"../cmd/controller", "tinygo && rp"},
	{"../cmd/emu", "js"},
}

func fileConstraint(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(b), "\n") {
		if constraint.IsGoBuild(line) {
			e, err := constraint.Parse(line)
			if err != nil {
				t.Fatalf("%s: %v", path, err)
			}
			return e.String()
		}
		if strings.HasPrefix(line, "package ") {
			break
		}
	}
	return ""
}

// TestRefugiumPairsAreWellFormed is §4.1's guard, the refugium counterpart of
// tinygo_split_test.go (which finds only _tinygo.go pairs, R0 M-2): every
// *_refugium.go file is constrained `X && refugium` (just `refugium` in gui)
// and has a *_default.go twin constrained `X && !refugium`; and no
// *_refugium.go file carries a forbidden literal or call.
func TestRefugiumPairsAreWellFormed(t *testing.T) {
	pairs := 0
	for _, pkg := range refugiumPairPackages {
		files, err := filepath.Glob(filepath.Join(pkg.dir, "*_refugium.go"))
		if err != nil {
			t.Fatal(err)
		}
		join := func(tag string) string {
			if pkg.x == "" {
				return tag
			}
			return pkg.x + " && " + tag
		}
		for _, f := range files {
			pairs++
			if got, want := fileConstraint(t, f), join("refugium"); got != want {
				t.Errorf("%s is constrained %q, want %q", f, got, want)
			}
			twin := strings.TrimSuffix(f, "_refugium.go") + "_default.go"
			if _, err := os.Stat(twin); err != nil {
				t.Errorf("%s has no twin %s", f, filepath.Base(twin))
				continue
			}
			if got, want := fileConstraint(t, twin), join("!refugium"); got != want {
				t.Errorf("%s is constrained %q, want %q", twin, got, want)
			}
			facts := readSourceFacts(t, []string{f})
			for _, lit := range refugiumForbiddenLiterals {
				if hits := facts.literalHits(lit); len(hits) > 0 {
					t.Errorf("%s carries a string literal containing %q", f, lit)
				}
			}
			for _, call := range refugiumForbiddenCalls {
				if len(facts.calls[call]) > 0 {
					t.Errorf("%s calls %s", f, call)
				}
			}
		}
	}
	// The pairs this phase adds. A glob that found fewer is a guard checking
	// less than it claims.
	if pairs < 6 {
		t.Fatalf("INCONCLUSIVE: found %d *_refugium.go files across %v, want at least 6 (gui: profile, debugcmd, prooftriggers; controller: lockboot, nfc; emu: platform_nfc)", pairs, refugiumPairPackages)
	}
}

// productionFiles parses gui's own non-test sources (every build variant:
// the gates below are about names, and a name in any variant counts).
func productionFiles(t *testing.T) (*token.FileSet, map[string]*ast.File) {
	t.Helper()
	paths, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	out := map[string]*ast.File{}
	for _, p := range paths {
		if strings.HasSuffix(p, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, p, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		out[p] = f
	}
	if len(out) < 100 {
		t.Fatalf("INCONCLUSIVE: parsed %d gui files; this test is not running from gui/", len(out))
	}
	return fset, out
}

// enclosingFunc names the function declaration containing pos, or "" at
// package level.
func enclosingFunc(f *ast.File, pos token.Pos) string {
	for _, d := range f.Decls {
		if fd, ok := d.(*ast.FuncDecl); ok && fd.Pos() <= pos && pos < fd.End() {
			return fd.Name.Name
		}
	}
	return ""
}

// TestNFCIsAskedForInOnePlace is §4.2's source gate (R0 I-5, round 3's
// identifier form): in gui's production code the identifier FeatureNFC appears
// only in its declaration and inside nfcAvailable, and NFCReader is called only
// inside nfcReader. So every scan offer is keyed on nfcAvailable and every
// poll loop takes its reader from nfcReader -- the two places the Refugium
// profile answers "no reader".
func TestNFCIsAskedForInOnePlace(t *testing.T) {
	fset, files := productionFiles(t)
	var featureUses, readerCalls []string
	for name, f := range files {
		ast.Inspect(f, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.Ident:
				if n.Name == "FeatureNFC" {
					featureUses = append(featureUses, name+":"+enclosingFunc(f, n.Pos())+
						"@"+strconv.Itoa(fset.Position(n.Pos()).Line))
				}
			case *ast.CallExpr:
				if sel, ok := n.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "NFCReader" {
					readerCalls = append(readerCalls, name+":"+enclosingFunc(f, n.Pos()))
				}
			}
			return true
		})
	}
	var inHelper, decl int
	for _, u := range featureUses {
		switch {
		case strings.HasPrefix(u, "nfc_scan.go:nfcAvailable@"):
			inHelper++
		case strings.HasPrefix(u, "gui.go:@"):
			decl++ // the const block declaring it
		default:
			t.Errorf("FeatureNFC is named outside nfcAvailable: %s", u)
		}
	}
	if inHelper != 1 || decl != 1 {
		t.Errorf("POSITIVE CONTROL: want FeatureNFC once in nfcAvailable and once in its declaration, "+
			"got %d and %d (all uses: %v)", inHelper, decl, featureUses)
	}
	var inReader int
	for _, c := range readerCalls {
		if c == "nfc_scan.go:nfcReader" {
			inReader++
			continue
		}
		t.Errorf("NFCReader() is called outside nfcReader: %s", c)
	}
	if inReader != 1 {
		t.Errorf("POSITIVE CONTROL: want exactly one NFCReader() call, in nfcReader; got %d (%v)", inReader, readerCalls)
	}
}

// TestSeedStringPlatesGoThroughTheMS1Gate pins §4.4's codex32 producers at the
// source: gui names backup.EngraveSeedString only inside engraveSeedStringPlate,
// which substitutes the text-only plate for an ms1 string under the profile.
// A new caller of the QR plate would bypass that gate, and fails here.
func TestSeedStringPlatesGoThroughTheMS1Gate(t *testing.T) {
	_, files := productionFiles(t)
	var sites []string
	for name, f := range files {
		ast.Inspect(f, func(n ast.Node) bool {
			if sel, ok := n.(*ast.SelectorExpr); ok && sel.Sel.Name == "EngraveSeedString" {
				if x, ok := sel.X.(*ast.Ident); ok && x.Name == "backup" {
					sites = append(sites, name+":"+enclosingFunc(f, n.Pos()))
				}
			}
			return true
		})
	}
	if len(sites) != 1 || sites[0] != "ms1_qr_gate.go:engraveSeedStringPlate" {
		t.Fatalf("backup.EngraveSeedString is named at %v; want only ms1_qr_gate.go:engraveSeedStringPlate", sites)
	}
}

// TestEveryBIP39PassphrasePromptIsAsked pins §4.3's call sites at the source:
// every ChoiceScreen whose Lead is "Add a BIP-39 passphrase?" is put to the
// operator through askBIP39Passphrase -- which is the notice under the profile
// -- and never through its own Choose. The count is the eight sites §4.3 lists,
// so a ninth prompt fails here until it is routed the same way.
func TestEveryBIP39PassphrasePromptIsAsked(t *testing.T) {
	const lead = "Add a BIP-39 passphrase?"
	_, files := productionFiles(t)
	var prompts, asked []string
	for name, f := range files {
		for _, d := range f.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			vars := map[string]bool{}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				as, ok := n.(*ast.AssignStmt)
				if !ok || len(as.Lhs) != 1 || len(as.Rhs) != 1 {
					return true
				}
				u, ok := as.Rhs[0].(*ast.UnaryExpr)
				if !ok {
					return true
				}
				cl, ok := u.X.(*ast.CompositeLit)
				if !ok {
					return true
				}
				if id, ok := cl.Type.(*ast.Ident); !ok || id.Name != "ChoiceScreen" {
					return true
				}
				for _, e := range cl.Elts {
					kv, ok := e.(*ast.KeyValueExpr)
					if !ok {
						continue
					}
					k, _ := kv.Key.(*ast.Ident)
					v, _ := kv.Value.(*ast.BasicLit)
					if k != nil && k.Name == "Lead" && v != nil && v.Value == strconv.Quote(lead) {
						if id, ok := as.Lhs[0].(*ast.Ident); ok {
							vars[id.Name] = true
							prompts = append(prompts, name+":"+fn.Name.Name)
						}
					}
				}
				return true
			})
			if len(vars) == 0 {
				continue
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Choose" {
					if x, ok := sel.X.(*ast.Ident); ok && vars[x.Name] {
						t.Errorf("%s:%s puts the passphrase prompt %s with Choose, bypassing askBIP39Passphrase",
							name, fn.Name.Name, x.Name)
					}
				}
				if id, ok := call.Fun.(*ast.Ident); ok && id.Name == "askBIP39Passphrase" && len(call.Args) == 3 {
					if x, ok := call.Args[2].(*ast.Ident); ok && vars[x.Name] {
						asked = append(asked, name+":"+fn.Name.Name)
					}
				}
				return true
			})
		}
	}
	if len(prompts) != 8 {
		t.Errorf("found %d BIP-39 passphrase prompts, want the 8 §4.3 lists: %v", len(prompts), prompts)
	}
	if len(asked) != len(prompts) {
		t.Errorf("%d prompts but %d askBIP39Passphrase calls on them:\nprompts %v\nasked   %v",
			len(prompts), len(asked), prompts, asked)
	}
}
