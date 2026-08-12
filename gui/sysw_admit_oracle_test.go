package gui

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"

	"seedhammer.com/sysw"
)

// §13 D7's MECHANISM, as a test.
//
// The ruling: §3.3.2's table is the normative ORACLE, and enforcement is
// per-site — each consumption site hard-codes its one admitted class, and a
// structural test reconciles every site against the table. `admits()` has zero
// production callers and wired in it could never return false at any existing
// site (a check that cannot fail), while a wrong FUTURE site could simply omit
// the call. So the reconciliation is here, and `admits()` is its oracle.
//
// This walks every non-test `syswOffer(...)` / `.take(...)` call site by AST,
// maps site -> program through the table below, and asserts each named class
// against `admitted`. A NEW consumption site fails until it appears here with an
// admitted class, which is the property that makes this worth its weight.

// syswConsumer is one production consumption site: the function that names a
// class, and the programs its callers belong to.
type syswConsumer struct {
	file string
	fn   string
	// progs is every program this site can run inside. More than one where a
	// shared helper serves several — the class must be admitted to ALL of them,
	// because the site cannot tell which caller it is under.
	progs []syswProgram
	why   string
}

var syswConsumers = []syswConsumer{
	{"gui.go", "newInputFlow", []syswProgram{progBackupWallet},
		"Backup Wallet's typed-menu entry; its only non-test caller is the backupWallet arm"},
	{"bundle_flow.go", "bundleFlow", []syswProgram{progBundle},
		"Engrave Bundle's first card"},
	{"freetext_flow.go", "engraveTextFlowFrom", []syswProgram{progText},
		"Engrave Text"},
	{"passphrase_flow.go", "engravePassphraseFlowFrom", []syswProgram{progPassword},
		"BIP-39 Password"},
	{"derive_xpub.go", "syswSeedPicker", []syswProgram{
		progXpub, progSingleSig, progMultisig, progBip85},
		"§3.1's shared seam: seedEntryFlow's four programs. NOT the two verify " +
			"re-entries, which call seedEntryFlowTypedOnly (test 16)"},
	{"sysw_source.go", "syswPassphraseFlow", []syswProgram{
		progXpub, progSingleSig, progMultisig, progBip85},
		"the four seam programs' optional-passphrase step (plan stage 13b). NOT " +
			"Backup Wallet, which refuses ClassPassphrase, and NOT the two verify " +
			"flows (§7.4) — which is why this is a wrapper and not an edit inside " +
			"passphraseFlow"},
	{"multisig.go", "supplyMultisigPolicyFlow", []syswProgram{progMultisig},
		"the supplied-md1 path (plan stage 13c)"},
	{"multisig_build.go", "buildMultisigPolicyFlow", []syswProgram{progMultisig},
		"the cosigner-card gather (plan stage 13c)"},
}

// classNames maps the sysw.Class identifiers a call site can name to their
// values, so the assertion is against the same table `admits` reads.
var classNames = map[string]sysw.Class{
	"ClassMnemonic":      sysw.ClassMnemonic,
	"ClassCodex32Secret": sysw.ClassCodex32Secret,
	"ClassPassphrase":    sysw.ClassPassphrase,
	"ClassFreeText":      sysw.ClassFreeText,
	"ClassDescriptor":    sysw.ClassDescriptor,
	"ClassMDMK":          sysw.ClassMDMK,
	"ClassAddress":       sysw.ClassAddress,
	"ClassUnknown":       sysw.ClassUnknown,
}

func TestEverySyswConsumptionSiteNamesAnAdmittedClass(t *testing.T) {
	index := map[string]syswConsumer{}
	for _, c := range syswConsumers {
		index[c.file+":"+c.fn] = c
	}

	ents, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("reading gui: %v", err)
	}
	fset := token.NewFileSet()
	var sites int
	for _, e := range ents {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", name, err)
		}
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			// sysw_session.go declares syswOffer and take THEMSELVES. They are
			// the helpers, not consumption sites: the class they handle is the
			// caller's argument, and checking them would be checking a
			// parameter name.
			if name == "sysw_session.go" {
				continue
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				var isOffer, isTake bool
				switch fun := call.Fun.(type) {
				case *ast.Ident:
					isOffer = fun.Name == "syswOffer"
				case *ast.SelectorExpr:
					isTake = fun.Sel.Name == "take"
				}
				if !isOffer && !isTake {
					return true
				}
				// The class is whichever argument names a sysw.Class constant.
				var class string
				for _, a := range call.Args {
					sel, ok := a.(*ast.SelectorExpr)
					if !ok {
						continue
					}
					if pkg, ok := sel.X.(*ast.Ident); ok && pkg.Name == "sysw" {
						if _, known := classNames[sel.Sel.Name]; known {
							class = sel.Sel.Name
						}
					}
				}
				if class == "" {
					t.Errorf("%s:%s consumes from the payload without naming a "+
						"sysw.Class constant — §13 D7's enforcement is that each site "+
						"HARD-CODES its one admitted class, and a site that computes "+
						"the class cannot be reconciled against §3.3.2 at all",
						name, fn.Name.Name)
					return true
				}
				sites++
				key := name + ":" + fn.Name.Name
				c, mapped := index[key]
				if !mapped {
					t.Errorf("NEW consumption site %s (class %s) is not in "+
						"syswConsumers — add it with the programs it runs inside, so "+
						"§3.3.2's table can be checked against it. This is the whole "+
						"point of this test: a site nobody mapped is a site nobody "+
						"checked", key, class)
					return true
				}
				for _, p := range c.progs {
					if !admits(p, classNames[class]) {
						t.Errorf("%s names %s, which §3.3.2 REFUSES to program %d (%s)",
							key, class, p, c.why)
					}
				}
				return true
			})
		}
	}
	if sites < len(syswConsumers) {
		t.Errorf("INCONCLUSIVE: only %d consumption sites found for %d mapped "+
			"entries — a mapped site has vanished and this test now guards less "+
			"than it claims", sites, len(syswConsumers))
	}
	t.Logf("%d consumption sites reconciled against §3.3.2", sites)
}

// The other direction, and the one an oracle test cannot give on its own: the
// helper that reaches the payload for the four SEAM programs must not be
// reachable from a program whose row refuses the class.
//
// syswPassphraseFlow is one site serving four programs, so the test above checks
// it against those four and would say nothing if backupWalletFlow started
// calling it — which is exactly the defect plan stage 13b's literal wording
// would have introduced, because passphraseFlow has NINE call sites (measured;
// the plan says four and an earlier correction said ten) and three of
// them must never see a payload passphrase.
func TestTheSeamPassphraseOfferReachesOnlyProgramsThatAdmitIt(t *testing.T) {
	// The five sites, in the four programs §3.3.2 admits ClassPassphrase to.
	want := map[string]int{
		"derive_xpub.go":    1, // Account Xpub
		"bip85.go":          1, // BIP-85 Child Seed
		"singlesig.go":      1, // Engrave Single-Sig
		"multisig.go":       1, // Engrave Multisig, supplied policy
		"multisig_build.go": 1, // Engrave Multisig, built policy
		"sysw_source.go":    1, // the declaration itself
	}
	// And the callers that must keep the plain keyboard. Named individually,
	// with the rule each one would break.
	forbidden := map[string]string{
		"gui.go":              "Backup Wallet REFUSES ClassPassphrase (§3.3.2): it engraves the mnemonic itself and the passphrase is never engraved and never in the QR",
		"singlesig_verify.go": "§7.4: a verify must not take its re-derivation input from the session",
		"multisig_verify.go":  "§7.4: a verify must not take its re-derivation input from the session",
		"slip39_polish.go":    "that is a SLIP-39 passphrase, not a BIP-39 one — a different secret entirely",
	}

	ents, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("reading gui: %v", err)
	}
	fset := token.NewFileSet()
	got := map[string]int{}
	for _, e := range ents {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", name, err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			if id, ok := n.(*ast.Ident); ok && id.Name == "syswPassphraseFlow" {
				got[name]++
			}
			return true
		})
	}
	for file, why := range forbidden {
		if got[file] > 0 {
			t.Errorf("%s reaches the payload's passphrase — %s", file, why)
		}
	}
	for file, n := range want {
		if got[file] < n {
			t.Errorf("%s no longer offers the payload's passphrase (%d of %d) — "+
				"§3.3.2 admits ClassPassphrase to it, and stage 13b is what served "+
				"the cell", file, got[file], n)
		}
	}
	if len(got) == 0 {
		t.Fatal("INCONCLUSIVE: syswPassphraseFlow is named nowhere, so this test " +
			"guards nothing")
	}
}
