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
//	"Cards from where?"        3 production sites
//	"Which md1?"               2 production sites
//	"Choose policy type"       1
//
// So a walk must anchor on a string that exists in ONE production flow, and
// "one" has to be a machine-checked fact rather than a claim in a comment —
// the counts above drift every time somebody adds a screen. That is what this
// file checks. It is the standing half of the gate; the walk asserts the needle
// appears, this asserts the needle could only have come from one place.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
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
	// S5's MULTI-SELECT @S picker, and these two are the only screens in the
	// firmware that can prove a walk built a policy the operator holds SEVERAL
	// slots of. Before S5 the picker was single-select and neither existed; the
	// three earlier drivers tap "NO, THAT IS ALL" on the first of them and so
	// never reach the second at all.
	{"Do you hold another slot?", "gui/multisig_build.go"},
	{"Which other slot is yours?", "gui/multisig_build.go"},
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
	// so it cannot appear on a wsh or legacy-sh build. S5 rewrote that sentence
	// (§0.1a made the nested default 1h rather than a warning about 2h), so the
	// needle moved with it.
	{"BIP-48 for nested segwit (script type 1h)", "gui/multisig_build.go"},
	// S4. The three screens that make the slot-assignment model and its gate
	// observable from outside, each measured single-site.
	//
	// The slot-source QUESTION. Spelt as the fragment after the slot number,
	// because the production string is a format ("Is your @%d key on a card?")
	// and a needle has to be a substring that literally occurs in the source.
	{"key on a card?", "gui/multisig_build_slots.go"},
	// Its PLURAL arm, which only a multi-slot build can reach. The two arms are
	// deliberately different substrings — buildSelfSourceLead says so in its own
	// comment — so "keys on cards?" adds no second site to the singular needle and
	// each is pinned on its own.
	{"keys on cards?", "gui/multisig_build_slots.go"},
	// The pre-assembly REVIEW's opening line. It proves the operator was shown
	// where every key came from before anything was assembled.
	{"Where each key comes from:", "gui/multisig_build_slots.go"},
	// The gate's FAIL screen title. This is the needle that separates a walk
	// that DROVE the gate from one that merely passed through the flow: the
	// title exists nowhere else, and no honest build can draw it.
	{"Key does not match seed", "gui/multisig_build.go"},
	// The plate census, before the tail. The TITLE, not the body: "This
	// engraves" also occurs in gui/bip85.go (measured, 2 sites), which is
	// exactly the ambiguity this list exists to catch.
	{"Plate Count", "gui/multisig_build.go"},
}

// composerFlowNeedles are the composer walk's anchors, on buildFlowNeedles'
// pattern and checked the same way but by a LITERAL counter (review N-4).
//
// WHY A SECOND LIST RATHER THAN TWO MORE ENTRIES ABOVE. productionSites is a
// raw text scan -- deliberately, since it is what lets decoyNeedles pin counts
// for strings built by concatenation -- and "Build a new policy" occurs in FIVE
// gui files: one rendered site and four COMMENTS (gui/composer_flow.go:11,
// gui/multisig_build.go:24 and :29, gui/sysw_admit.go:54, gui/gui.go:193, all
// of which name the screen while describing something else). Putting it in
// buildFlowNeedles fails on the tip for a reason that is not a defect, and
// widening productionSites would move every existing pin and decoy count with
// it. So the needle is pinned by the site that matters -- a string literal in
// code -- using the AST walk embed_confinement_test.go already uses for the
// same "a mention is not a reference" reason.
//
// WHY THESE TWO. The composer walk's engrave tail terminates on the DOOR's own
// row (cmd/emu/shots_composer.js's DOOR_ROW), so a second screen that reused
// that copy would end the tail EARLY and compareEngraved would then run against
// a PARTIAL census -- a wrong answer rather than a timeout, which is the one
// failure shape a census must not have. "Which script?" is what proves a walk
// entered the composer's own shape step rather than some other program's.
var composerFlowNeedles = []struct {
	text string
	file string // the single production file whose CODE spells the needle
}{
	{"Build a new policy", "gui/composer_door.go"},
	{"Which script?", "gui/composer_shape.go"},
}

// literalSites returns the gui files whose CODE contains `text` in a string
// literal. Comments are excluded by construction: the parser keeps them out of
// the AST unless asked for them, and this asks for the code.
func literalSites(t *testing.T, text string) []string {
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
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, 0) // 0 == no comments
		if err != nil {
			t.Fatalf("parsing %s: %v", name, err)
		}
		found := false
		ast.Inspect(f, func(n ast.Node) bool {
			bl, ok := n.(*ast.BasicLit)
			if !ok || bl.Kind != token.STRING {
				return true
			}
			v, err := strconv.Unquote(bl.Value)
			if err == nil && strings.Contains(v, text) {
				found = true
			}
			return true
		})
		if found {
			out = append(out, "gui/"+name)
		}
	}
	// The same floor productionSites carries, for the same reason: a misrooted
	// path must not make every needle look unique by finding nothing at all.
	if checked < 30 {
		t.Fatalf("only %d production .go file(s) under %s — the path is wrong, "+
			"and every count from it is meaningless", checked, dir)
	}
	return out
}

// TestComposerFlowNeedlesHaveExactlyOneLiteralSite is N-4's gate.
//
// It FAILS when a second gui file spells one of these literals in code, which
// is the mutation that would silently shorten the composer walk's engrave tail.
func TestComposerFlowNeedlesHaveExactlyOneLiteralSite(t *testing.T) {
	for _, n := range composerFlowNeedles {
		sites := literalSites(t, n.text)
		if len(sites) != 1 {
			t.Errorf("composer needle %q is spelt in %d production file(s) in CODE, want exactly 1:\n  %s\n"+
				"the composer walk anchors on this; a second site makes it name the wrong screen, "+
				"and for the door's row that ends the engrave tail early against a PARTIAL census",
				n.text, len(sites), strings.Join(sites, "\n  "))
			continue
		}
		if got := sites[0]; got != n.file {
			t.Errorf("composer needle %q is unique but lives in %s, want %s — "+
				"it identifies a different screen than the walk thinks", n.text, got, n.file)
		}
		t.Logf("%-24q -> %s", n.text, sites[0])
	}
}

// TestLiteralSiteCounterIgnoresComments is the mutation proof for the counter
// itself, on TestNeedleSiteCounterCanCount's pattern.
//
// Without it a literalSites that returned one file for everything would make
// every composer needle look unique -- the false-PASS shape this file exists to
// remove. "Build a new policy" is the case that proves the point: the raw
// counter finds five files, this one finds the single rendered site.
func TestLiteralSiteCounterIgnoresComments(t *testing.T) {
	raw := productionSites(t, "Build a new policy")
	lit := literalSites(t, "Build a new policy")
	if len(raw) <= len(lit) {
		t.Fatalf("the two counters agree on %q (raw %v, literal %v) — this test is pinning "+
			"nothing, and the comment sites it exists to discount are gone",
			"Build a new policy", raw, lit)
	}
	if len(lit) != 1 {
		t.Errorf("the literal counter finds %d site(s) for %q, want 1: %v",
			len(lit), "Build a new policy", lit)
	}
	// A string that is spelt in NO gui file must come back empty from both, or
	// the counter is matching something other than what it was asked for.
	if got := literalSites(t, "a string no gui file spells"); len(got) != 0 {
		t.Errorf("the literal counter found %v for a string no file spells", got)
	}
	t.Logf("raw counter %d file(s) %v; literal counter %d %v", len(raw), raw, len(lit), lit)
}

// contentNeedles identify WHAT WAS BUILT, never WHICH FLOW built it.
//
// RECLASSIFIED BY F-190's COUNTER, not by taste. "P2SH-P2WSH" is spelt in
// exactly one production file, so the substring counter above calls it unique
// and it sat in buildFlowNeedles for that reason. The flow counter
// (needle_flow_test.go) says otherwise: it is drawn by scriptName, which reaches
// the restore doc, and the restore doc is shown by the BUILD path and the SUPPLY
// path alike. So it proves a nested-segwit policy was built and proves nothing at
// all about which flow the walk is in.
//
// That is not a demotion of its value. S3's walk needs exactly this string,
// because a tap on template row 1 is indistinguishable from a tap on row 0 unless
// something downstream says which landed. The rule is that a content needle may
// only ever be asserted ALONGSIDE a flow needle, never instead of one — which is
// what walk_s3_nested.js already does.
//
// TestTheTwoCountersDisagreeOnlyWhereRecorded holds both halves: one source site,
// several drawing flows. If either ever stops being true, the classification is
// wrong and should move rather than be edited around.
var contentNeedles = []struct {
	text string
}{
	{"P2SH-P2WSH"},
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
	// THREE sites since the Wallet Policy program: bundleFlow,
	// supplyMultisigPolicyFlow and walletPolicyFlow. It was two — S1 had removed
	// buildMultisigPolicyFlow's copy, because the Build path takes the WHOLE
	// cosigner set from the payload and a source picker with one answer is a tap
	// that teaches nothing. Wallet Policy put a third back, deliberately: it
	// offers the payload through the SAME offer() a scanned card enters by, and
	// a second insertion path would be a second way for a card to join a set
	// with only one of them checked. Still a decoy, and MORE of one now — "the
	// walk reached a card gather" distinguishes nothing among three flows.
	//
	// SPELT "Cards from where?" SINCE F-76 (review r1, M1). It read "First card
	// from where?" while the door handed the gatherer one record; the door now
	// hands over every md1/mk1 card the payload holds, so the lead was reworded
	// to what it does. The COUNT is the load-bearing part and is unchanged at
	// three — a rename that quietly took it to one would make this string a
	// needle by accident, which is the promotion this list exists to keep
	// deliberate.
	{"Cards from where?", 3},
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
// ("Cards from where?", 3 sites; "Which md1?", 2 sites, both already
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
	// Content needles are pinned too — a walk may declare one, it just may not
	// stand alone as proof of WHICH FLOW. What must never happen is a NEEDLE_*
	// pointing at a string nobody has measured at all.
	for _, n := range contentNeedles {
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
// Blind spot, stated: this reads the `ok` expression textually. A driver that
// computed `ok` into a variable first, or that shipped a helper named something
// else, would slip past. It costs one grep and catches the shape that has now
// occurred twice.
//
// TWO SHAPES, BECAUSE THIS TEST WAS RED AT FORK MAIN b9a9a30 AND HAD BEEN SINCE
// H0 (found by the H5 plan's build gate, 2026-09-05). Only the object-literal
// property `ok: <expr>,` was readable, so every walk that instead ASSIGNS --
// `out.ok = <expr>;`, which walk_h0_preimage.js has done since 45f3d4c and
// walk_hashlock_phrase.js since e1bf137 -- reported INCONCLUSIVE, and
// INCONCLUSIVE here is a t.Errorf. CI runs `go test ./...`, so the package has
// been failing for two stages while the guard's own doc claimed it covered
// "BOTH walk scripts".
//
// EVERY ASSIGNMENT IS READ, NOT THE FIRST (r0 fidelity I-2). A walk's verdict is
// its LAST `ok` assignment, and the first draft of this branch took
// FindStringSubmatch -- the first match -- so a walk that opened
// `out.ok = false;` and closed `out.ok = out.plates === 3;` cleared the guard,
// was counted as checked, and was LOGGED as restating nothing. That is worse
// than the INCONCLUSIVE it replaced, because INCONCLUSIVE said so. The `plates`
// check now runs on every right-hand side, so the position of the offending one
// does not matter; walkOkAssignments below is separated out for exactly one
// reason, that TestWalkOkGuardReadsEveryAssignment can feed it that shape.
//
// The assignment regex captures the right-hand side EXACTLY, anchored on the
// `.ok =` it is looking for, so unlike the property span it cannot grab a
// neighbouring literal -- which is why the census/verdict floor below is
// required of the property shape and not of this one.
var (
	okPropRe   = regexp.MustCompile(`(?ms)^\s*ok:.*?\n  \};`)
	okAssignRe = regexp.MustCompile(`(?ms)^\s*\w+\.ok\s*=\s*(.*?);\s*$`)
	// A bare boolean right-hand side: `out.ok = true;`. This is the STRONGEST
	// form of the property under test, not an exemption from it -- an `ok` that
	// is SET after the last assertion contains no term at all, so it cannot
	// contain one the driver supplied, and there is nothing left for the
	// `plates` check to find. H5 §4.4 requires exactly this of the hashlock
	// walk, and a guard that called the strongest shape INCONCLUSIVE would push
	// the next author back to a recomputation.
	okSetRe = regexp.MustCompile(`^(true|false)$`)
)

// walkOkAssignments returns the right-hand side of EVERY `x.ok = <rhs>;` in src,
// in source order, trimmed.
//
// Separated from the test so the guard's own blind spot has a test: see
// TestWalkOkGuardReadsEveryAssignment.
func walkOkAssignments(src string) []string {
	ms := okAssignRe.FindAllStringSubmatch(src, -1)
	out := make([]string, 0, len(ms))
	for _, m := range ms {
		out = append(out, strings.TrimSpace(m[1]))
	}
	return out
}

// walkOkDriverSupplied returns the right-hand sides of assignments that name a
// term the CALLER supplies (I-1/F-170), and whether every assignment was a bare
// boolean constant.
func walkOkDriverSupplied(rhs []string) (bad []string, allConst bool) {
	allConst = true
	for _, r := range rhs {
		if okSetRe.MatchString(r) {
			continue
		}
		allConst = false
		if strings.Contains(r, "plates") {
			bad = append(bad, r)
		}
	}
	return bad, allConst
}

// TestWalkOkGuardReadsEveryAssignment is the guard's own gate (r0 fidelity I-2).
//
// The shape that matters is row 3: a walk whose LAST assignment is the defect
// and whose FIRST is innocent. Reading one match passes it.
//
// MUTATION: make walkOkAssignments use FindStringSubmatch (the first match only)
// -> the row "the verdict is the last assignment" fails on both counts, measured:
// `walkOkDriverSupplied found 0 caller-supplied term(s) [] in ["false"], want 1`
// and `allConst = true over ["false"], want false`. The row after it survives that
// mutation BY CONSTRUCTION -- its offender IS the first assignment -- which is why
// both rows are here: one pins the position, the other pins that position is not
// what the check depends on.
func TestWalkOkGuardReadsEveryAssignment(t *testing.T) {
	for _, tc := range []struct {
		name     string
		src      string
		wantBad  int
		wantAllC bool
	}{
		{"set after the last assertion", "  out.ok = false;\n  must(x);\n  out.ok = true;\n", 0, true},
		{"the verdict is the last assignment",
			"  const out = { plates: null, ok: false };\n  out.ok = false;\n" +
				"  out.plates = window.shPlates();\n  out.ok = out.plates === 3;\n", 1, false},
		{"an early offender with a bare verdict after it",
			"  out.ok = out.plates === 3;\n  out.ok = true;\n", 1, false},
		{"a derived verdict that names nothing the caller supplies",
			"  out.ok = census.length > 0;\n", 0, false},
		{"no assignment at all", "  const out = { ok: false };\n", 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rhs := walkOkAssignments(tc.src)
			bad, allConst := walkOkDriverSupplied(rhs)
			if len(bad) != tc.wantBad {
				t.Errorf("walkOkDriverSupplied found %d caller-supplied term(s) %q in %q, want %d",
					len(bad), bad, rhs, tc.wantBad)
			}
			if allConst != tc.wantAllC {
				t.Errorf("allConst = %v over %q, want %v", allConst, rhs, tc.wantAllC)
			}
		})
	}
}

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
		src := string(b)
		// The ASSIGNMENT shape first: its span is exact, so it needs no floor.
		if rhs := walkOkAssignments(src); len(rhs) > 0 {
			checked++
			bad, allConst := walkOkDriverSupplied(rhs)
			for _, r := range bad {
				t.Errorf("%s's `ok` contains `plates`, which the CALLER supplies (I-1/F-170):\n%s\n"+
					"A walk cannot derive, so a caller-supplied count in `ok` is content the walk "+
					"never observed — a run that cut N WRONG strings is green.", f, r)
			}
			if allConst {
				// SAYS ONLY WHAT IS CHECKED (r0 journey N-1). Nothing here
				// measures where the assignment sits relative to the last
				// assertion; what is measured is that every right-hand side is a
				// constant, which is what makes it restate nothing.
				t.Logf("%s assigns `ok` nothing but the constant(s) %s, so it restates no assertion "+
					"(H5 §4.4)", f, strings.Join(rhs, ", "))
			}
			continue
		}
		expr := okPropRe.FindString(src)
		if expr == "" {
			t.Errorf("INCONCLUSIVE: %s has neither an `ok:` property nor an `x.ok =` assignment "+
				"this test can read, so nothing was checked for it — the walk's return shape "+
				"changed and this guard did not", f)
			continue
		}
		// A floor, so a regex that grabbed the wrong span cannot pass by reading
		// something harmless: the span has to name what the walk is ABOUT.
		//
		// For an engraving walk that is the census. NOT EVERY WALK ENGRAVES,
		// though -- walk_verify drives bundle.Verify and cuts nothing, so it has
		// no census to name and its subject is the verdict. Requiring "census"
		// of it was demanding a count from a walk that has none, which is a guard
		// asserting its own premise rather than the walk's honesty.
		//
		// The check that matters runs on BOTH kinds and is unchanged: `ok` must
		// not contain a caller-supplied plate count.
		// THE EXACT KEY, not a substring like "erdict". The loose form was
		// satisfied by `verifyTrail: [...new Set(verdict)]` -- an incidental
		// mention of a local VARIABLE -- so a walk could rename its verdict key
		// away and still clear the floor on a word it happened to use elsewhere.
		// A floor that any nearby identifier satisfies is not a floor.
		if !strings.Contains(expr, "census") && !strings.Contains(expr, "verifyVerdict") {
			t.Errorf("INCONCLUSIVE: the `ok:` expression extracted from %s reports neither a "+
				"`census` nor a `verifyVerdict`, so the wrong span was read:\n%s", f, expr)
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
