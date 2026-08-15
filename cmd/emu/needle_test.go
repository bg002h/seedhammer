package main

// UNTAGGED, for the reason engraved.go gives: this reads gui's SOURCE off disk
// rather than referencing any //go:build js symbol, so it runs on the host.

// WHY THIS FILE EXISTS (F-169).
//
// §4.5 makes an emulator walk the closing gate of every stage from S1 on, and
// all five of those stages edit buildMultisigPolicyFlow — which sits behind
// "Engrave Multisig -> Build policy". The walk that existed drove the SIBLING
// choice, "Engrave Bundle", so every one of those gates named a flow no walk
// entered.
//
// A walk written by editing that script's goTo target would have looked
// identical and still proved nothing, because every needle it used is
// AMBIGUOUS. Measured, not assumed:
//
//	"First card from where?"   3 production sites
//	"Which md1?"               2 production sites
//	"Choose policy type"       1
//
// So a walk must anchor on a string that exists in ONE production flow, and
// "one" has to be a machine-checked fact rather than a claim in a comment —
// the counts above drift every time somebody adds a screen. That is what this
// file checks. It is the standing half of the gate; the walk asserts the needle
// appears, this asserts the needle could only have come from one place.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// buildFlowNeedles are the strings a Build-policy walk may anchor on. Each MUST
// have exactly one production site, and that site must be inside the build
// flow's own file — a needle unique to some OTHER flow identifies the wrong
// thing just as badly as an ambiguous one.
//
// Keep this list SHORT and load-bearing. It is not an inventory of the flow's
// screens; it is the set a walk is allowed to trust.
var buildFlowNeedles = []struct {
	text string
	file string // the single production file the needle must live in
}{
	{"Choose policy type", "gui/multisig_build.go"},
	{"How many keys (n)?", "gui/multisig_build.go"},
	{"Which slot is your key?", "gui/multisig_build.go"},
	// S2/D-4. The cosigner gather's own title, which until D-4 did not exist:
	// the screen was titled for a DIFFERENT program by the shared gatherer, so
	// the one screen where the payload's cards become visible could not be
	// anchored on at all. It is the first needle that identifies a screen the
	// build flow does not itself draw — bundleGatherFlow draws it, from a title
	// only this flow passes.
	// (Spelt as a literal because this is package main, not gui; gui's own
	// buildCosignerGatherTitle constant is what production reads.)
	{"Cosigner Keys", "gui/multisig_build.go"},
	// The front door, one level above the build flow. A walk uses this to prove
	// it reached Engrave Multisig at all before choosing "Build policy".
	{"Supply or build a policy?", "gui/multisig.go"},
	// S1's bounded-selection surface, reached ONLY when the payload supplied
	// more cosigner cards than the policy has open slots. It is the first needle
	// that proves something about the CARDS rather than the parameters: a walk
	// seeing it has a payload-fed cosigner set that had to be narrowed, which is
	// the whole `0..n` ruling on screen.
	{"Payload cards", "gui/multisig_build_payload.go"},
	{"Use payload card", "gui/multisig_build_payload.go"},
	// S3. The two needles that let a walk prove it built NESTED SEGWIT, which a
	// tap on the template picker cannot prove for itself: row 1 and row 0 are the
	// same gesture, and nothing downstream said which landed until §0.1a made the
	// origin announcement template-dependent.
	//
	// The NOTE is emitted only by buildOriginAnnouncement's md.MultisigShWsh arm,
	// so it cannot appear on a wsh or legacy-sh build.
	{"BIP-48 assigns m/48h/0h/0h/1h to nested segwit", "gui/multisig_build.go"},
	// The NAME is S3's whole subject: scriptName's nested arm. It reaches the
	// restore doc through desc4Display, and gui/multisig_restore.go deliberately
	// does NOT quote it in a comment, because this counter matches source bytes
	// and a quoted literal would cost the needle its uniqueness.
	{"P2SH-P2WSH", "gui/md1_inspect.go"},
}

// decoyNeedles are strings a stage author reaches for FIRST and must not use.
// Pinned with their measured counts so this test fails loudly if one ever
// becomes unique — at which point it is promoted deliberately, not by accident.
var decoyNeedles = []struct {
	text string
	want int
}{
	// Two sites: the build flow's wallet-policy form picker and singlesig's.
	{"Which md1?", 2},
	// TWO sites since S1: bundleFlow and supplyMultisigPolicyFlow. It was three
	// — buildMultisigPolicyFlow had the same picker — and S1 removed that one,
	// because the Build path now takes the WHOLE cosigner set from the payload
	// and a source picker with one answer is a tap that teaches nothing. Still a
	// decoy, and still the reason "the walk reached a card gather" proves
	// nothing: two flows can draw it.
	{"First card from where?", 2},
	// CORRECTED BY S2/D-4. This used to read "the gather's title comes from the
	// SHARED gatherer, so it reads 'Engrave Bundle' even when the operator
	// arrived via Build policy" — true when it was written, false since D-4 made
	// the title the caller's. The string stays a DECOY for the reason that
	// outlives the fix: four flows still pass it, and gui.go's carousel draws it
	// as a program name, so it identifies no flow. What changed is that the Build
	// path is no longer one of the four; its anchor is "Cosigner Keys" above.
	{"Engrave Bundle", 0}, // 0 == "at least one, count not pinned"; see the test
}

func TestBuildFlowNeedlesHaveExactlyOneProductionSite(t *testing.T) {
	for _, n := range buildFlowNeedles {
		sites := productionSites(t, n.text)
		if len(sites) != 1 {
			t.Errorf("needle %q has %d production site(s), want exactly 1:\n  %s\n"+
				"a walk anchoring on this cannot prove which flow it is in",
				n.text, len(sites), strings.Join(sites, "\n  "))
			continue
		}
		if got := sites[0]; got != n.file {
			t.Errorf("needle %q is unique but lives in %s, want %s — "+
				"it identifies a different flow than the walk thinks",
				n.text, got, n.file)
		}
	}
}

func TestDecoyNeedlesAreStillAmbiguous(t *testing.T) {
	for _, d := range decoyNeedles {
		sites := productionSites(t, d.text)
		switch {
		case d.want == 0:
			if len(sites) == 0 {
				t.Errorf("decoy %q has no production site at all — it was renamed, "+
					"so this guard now protects nothing", d.text)
			}
		case len(sites) != d.want:
			t.Errorf("decoy %q now has %d production site(s), pinned at %d:\n  %s\n"+
				"if it became UNIQUE, promote it to buildFlowNeedles deliberately; "+
				"if it grew, update the pin",
				d.text, len(sites), d.want, strings.Join(sites, "\n  "))
		}
	}
}

// TestNeedleSiteCounterCanCount is the mutation proof for the counter itself.
//
// Without it, a productionSites that silently returned nothing would make
// EVERY decoy look unique and every needle look absent, and the two tests above
// would report whatever that bug implied — the false-PASS shape this whole
// stage exists to remove. So the counter is exercised against strings whose
// answers are known independently of gui's contents.
func TestNeedleSiteCounterCanCount(t *testing.T) {
	// A string that cannot occur in gui's source.
	if got := productionSites(t, "zzz-this-string-is-not-in-gui-zzz"); len(got) != 0 {
		t.Errorf("counter found %d site(s) for an impossible string: %v", len(got), got)
	}
	// A string that certainly occurs many times.
	if got := productionSites(t, "func "); len(got) < 5 {
		t.Errorf("counter found %d file(s) containing %q; gui has far more, "+
			"so the counter is not reading the tree", len(got), "func ")
	}
}

// ─── I-2: the two halves of the needle gate, joined by a CHECK ──────────────
//
// The design is explicit that neither half is a gate alone: needle_test.go pins
// the needles, the walk asserts each one appeared. The JS even says "keep in
// sync with cmd/emu/needle_test.go's buildFlowNeedles, which is what proves
// 'unique'". Nothing enforced that — measured, no Go file read the driver at
// all — so a stage author could point a NEEDLE_* constant at an AMBIGUOUS string
// ("First card from where?", 2 sites; "Which md1?", 2 sites, both already
// waitFor'd elsewhere in the same walk), and the pinned list would still pass
// because it validates its own untouched copy. The walk's central claim — this
// is the Build-policy flow and not Engrave Bundle — would then be false while
// green, which is F-169 recurring through the one seam its fix did not close.
//
// GLOBBED, not named. The finding asked for walk_build_policy.js specifically;
// the check covers every walk_*.js in this directory, so a walk added by a later
// stage is bound the moment it lands rather than the moment somebody remembers
// to extend this list.
//
// WHAT THIS DOES NOT COVER, stated because a gate hiding its blind spot is worse
// than no gate: it binds `export const NEEDLE_* = "..."` declarations only. A
// walk that calls waitFor("some bare literal") without routing it through a
// NEEDLE_ constant is invisible here — walk_trace_a.js does exactly that today,
// legitimately, because it drives the Engrave Bundle flow and anchors on strings
// that are not build-flow needles. The convention IS the coverage: a walk that
// wants a needle bound must declare it.
var needleDeclRe = regexp.MustCompile(`(?m)^export\s+const\s+(NEEDLE_\w+)\s*=\s*"([^"\n]*)"`)

// walkNeedleLiterals extracts every NEEDLE_* declaration from a walk source.
func walkNeedleLiterals(t *testing.T, src string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, m := range needleDeclRe.FindAllStringSubmatch(src, -1) {
		if strings.Contains(m[2], `\`) {
			t.Fatalf("needle %s carries a backslash escape (%q), which this extractor does "+
				"not decode — it would compare the wrong string. Write the needle plainly.",
				m[1], m[2])
		}
		out[m[1]] = m[2]
	}
	return out
}

func TestWalkNeedleLiteralsAreAllPinned(t *testing.T) {
	files, err := filepath.Glob("walk_*.js")
	if err != nil {
		t.Fatalf("globbing walk scripts: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("INCONCLUSIVE: no walk_*.js in this directory, so this test binds nothing — " +
			"the walk drivers moved, and the glob is now wrong rather than empty")
	}
	pinned := map[string]bool{}
	for _, n := range buildFlowNeedles {
		pinned[n.text] = true
	}

	total := 0
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("reading %s: %v", f, err)
		}
		lits := walkNeedleLiterals(t, string(b))
		total += len(lits)
		bad := 0
		for name, text := range lits {
			if !pinned[text] {
				bad++
				t.Errorf("%s declares %s = %q, which is not in buildFlowNeedles.\n"+
					"Nothing has ever measured how many production sites that string has, so a "+
					"walk anchoring on it cannot prove which flow it is in. Pin it here (and "+
					"TestBuildFlowNeedlesHaveExactlyOneProductionSite will say whether it is "+
					"usable at all), or stop calling it a needle.", f, name, text)
			}
		}
		t.Logf("%s: %d NEEDLE_* declaration(s), %d unpinned", f, len(lits), bad)
	}
	if total == 0 {
		t.Fatalf("INCONCLUSIVE: %d walk script(s) and not one NEEDLE_* declaration between "+
			"them — either the convention was abandoned or this extractor stopped matching, "+
			"and both make this test pass by checking nothing", len(files))
	}
}

// TestWalkNeedleExtractorCanExtract is the mutation proof for the extractor
// itself. Without it, a regex that silently matched nothing would make every
// walk look clean — the false-PASS shape this whole file exists to remove. So it
// is exercised against sources whose answers are known independently.
func TestWalkNeedleExtractorCanExtract(t *testing.T) {
	got := walkNeedleLiterals(t, `
// a comment mentioning NEEDLE_FAKE = "not a declaration"
export const NEEDLE_ONE = "Choose policy type"; // gui/multisig_build.go
export const NEEDLE_TWO   =   "Cosigner Keys";
const GATHER_NOT_A_NEEDLE = "Scan a card, or Done";
export const CARDS_DIGEST = "25271e58";
  export const NEEDLE_INDENTED = "indented, so not a declaration at line start";
`)
	want := map[string]string{
		"NEEDLE_ONE": "Choose policy type",
		"NEEDLE_TWO": "Cosigner Keys",
	}
	if len(got) != len(want) {
		t.Fatalf("extracted %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}
	// And the negative: a source with no declarations must yield none, not
	// everything.
	if n := len(walkNeedleLiterals(t, "export function run() { await waitFor(\"Cosigner Keys\"); }")); n != 0 {
		t.Errorf("extracted %d declaration(s) from a source that has none", n)
	}
	// The real driver must be non-empty, or the two tests above are shadow
	// boxing. Measured here rather than assumed, because the count changes.
	b, err := os.ReadFile("walk_build_policy.js")
	if err != nil {
		t.Fatalf("reading walk_build_policy.js: %v", err)
	}
	if n := len(walkNeedleLiterals(t, string(b))); n < 5 {
		t.Errorf("the Build-policy driver yields %d needle declaration(s); it has always had "+
			"at least the five flow anchors, so the extractor is not reading it", n)
	}
}

// ─── I-1: `ok` may not contain a term the DRIVER supplied ───────────────────
//
// F-170 was "a walk asserted a plate COUNT — census.strings.length === plates,
// with `plates` a parameter defaulting to 6", so a run that engraved six WRONG
// strings was green. It was fixed on the Go side, where the count falls out of
// len(DeriveExpected(...)), and left standing verbatim in BOTH walk scripts —
// and then reproduced in the new Build-policy driver with a hand-derived 9.
//
// Removing the term is a deletion, and a deletion has no gate. This is that
// gate: `plates` survives as runEngraveTail's loop bound, one identifier away
// from being put back into `ok` by a stage author who wants a stricter-looking
// green. The rule is not "no plate count anywhere"; it is that `ok` may contain
// only terms the emulator was OBSERVED to produce.
//
// Blind spot, stated: this reads the `ok:` expression textually. A driver that
// computed `ok` into a variable first, or that shipped a helper named something
// else, would slip past. It costs one grep and catches the shape that has now
// occurred twice.
var okExprRe = regexp.MustCompile(`(?ms)^\s*ok:.*?\n  \};`)

func TestWalkOkContainsNoDriverSuppliedPlateCount(t *testing.T) {
	files, err := filepath.Glob("walk_*.js")
	if err != nil {
		t.Fatalf("globbing walk scripts: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("INCONCLUSIVE: no walk_*.js in this directory")
	}
	checked := 0
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("reading %s: %v", f, err)
		}
		expr := okExprRe.FindString(string(b))
		if expr == "" {
			t.Errorf("INCONCLUSIVE: %s has no `ok:` property this test can read, so nothing "+
				"was checked for it — the walk's return shape changed and this guard did not", f)
			continue
		}
		// A floor, so a regex that grabbed the wrong span cannot pass by reading
		// something harmless. Every walk's verdict is about the census.
		if !strings.Contains(expr, "census") {
			t.Errorf("INCONCLUSIVE: the `ok:` expression extracted from %s does not mention "+
				"the census, so the wrong span was read:\n%s", f, expr)
			continue
		}
		checked++
		if strings.Contains(expr, "plates") {
			t.Errorf("%s's `ok` contains `plates`, which the CALLER supplies (I-1/F-170):\n%s\n"+
				"A walk cannot derive, so a caller-supplied count in `ok` is content the walk "+
				"never observed — a run that cut N WRONG strings is green. Report the count as "+
				"data; let the derived expectation committed beside the gate record "+
				"(oracle/gaterecords/*.expect.json) say what the strings should BE.", f, expr)
		}
	}
	if checked == 0 {
		t.Fatal("INCONCLUSIVE: no walk's `ok` expression could be read, so this test " +
			"passed by checking nothing")
	}
	t.Logf("%d walk script(s) checked; no driver-supplied plate count in any `ok`", checked)
}

// productionSites returns the gui/*.go files containing text, excluding tests.
// Returns one entry per FILE, deduplicated — two hits in one file still mean
// one place a walk could be.
//
// Deliberately blunt substring matching over source bytes, the same thing
// `git grep -F` does, because the alternative (parsing Go and reading string
// literals) would quietly stop counting a needle built by concatenation or
// fmt.Sprintf — and a needle a walk can SEE on screen is one this must count.
func productionSites(t *testing.T, text string) []string {
	t.Helper()
	dir := filepath.Join("..", "..", "gui")
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading %s: %v", dir, err)
	}
	var out []string
	checked := 0
	for _, e := range ents {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		checked++
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("reading %s: %v", name, err)
		}
		if strings.Contains(string(b), text) {
			out = append(out, "gui/"+name)
		}
	}
	// A floor, so a misrooted path cannot make every needle look unique by
	// finding nothing at all. gui is a large package; if this ever legitimately
	// drops below the floor the number is wrong, not the guard.
	if checked < 30 {
		t.Fatalf("only %d production .go file(s) under %s — the path is wrong, "+
			"and every count from it is meaningless", checked, dir)
	}
	return out
}
