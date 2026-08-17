package gui

import (
	"reflect"
	"strings"
	"testing"
	"testing/synctest"

	"github.com/btcsuite/btcd/chaincfg/v2"
	"seedhammer.com/bip39"
	"seedhammer.com/bundle"
	"seedhammer.com/md"
)

// ─── S6a: the verification status line, tested as a pure function ────────────
//
// These are the step-2 rows of the S6a test plan -- T21, T22 and T26. All three
// run against buildVerifyStatusLine and the two records that feed it, with NO
// flow and NO rendered document, because step 2 deliberately has no callers yet.
// The rows that need a rendered document (T20, T23, T24, T25, T27) land at step
// 7; written here they would pass without ever reaching the sequence they name.
//
// EVERY STATUS ASSERTION COMPARES THE WHOLE STRING (S6a §5). A substring
// assertion cannot tell statusVerified from statusVerifiedOnRetry -- the retry
// line is the pass line plus one sentence -- so a substring test's own named
// mutation can pass. The `Contains` calls below are only ever used to say a
// clause is ABSENT, or as a second, narrower statement about a line whose whole
// text has already been pinned.

// The four §4.7c lines, transcribed once here so a mutation to the production
// text has to be made twice to go unnoticed. The two pass lines are GENERATED,
// so they are assembled from clauses rather than pasted.
const (
	t21ZeroCellLine = "These plates were not fully checked. Confirm they restore " +
		"this wallet (master fingerprint below) before relying on this backup."
	t21DidNotPassLine = "A verification check ran and did not pass: a comparison did " +
		"not match, or a plate could not be read or accounted for. Do NOT rely on this " +
		"backup until a full check passes. Check again with every plate this run " +
		"engraved; if this repeats, engrave a fresh set."
	t21RetrySuffix = "An earlier check did not pass; a later full check passed."

	t22OneKeyPlateClause = "1 key plate was read back and matched what this run engraved."
	t22MS1Clause         = "The ms1 secret you typed matched this seed."
	t22NoMS1Clause       = "No secret seed share was read back or compared."
	t22CosignerClause    = "Other cosigners' keys are taken as supplied."
)

// T21 -- THE ZERO CELL IS THE DEFAULT.
//
// §4.7a's switch is a 2x2 over two RECORDED booleans, and its `default:` arm is
// the cell an observation reaches when it matched no recorded bit: a skip, a
// benign refusal, an abort, or a return path somebody adds next year and
// classifies not at all. That is what makes monotonicity STRUCTURAL rather than
// promised -- a fact that is not recorded cannot set a bit, and an unset bit can
// only move the cell toward statusNotFullyChecked.
//
// The mutation this row exists for is "make any other status the `default:`
// arm". The zero-value record below is what drives that arm, so any other
// status there is reported by the first sub-test.
func TestVerifyStatusZeroCellIsTheDefault(t *testing.T) {
	// (a) THE ZERO VALUE OF THE TYPE. statusNotFullyChecked is iota 0, so a
	// verifyStatus that nothing ever assigned is already the safe one. This is
	// the half of the property no switch can break.
	var unassigned verifyStatus
	if unassigned != statusNotFullyChecked {
		t.Errorf("the zero value of verifyStatus is %v, not statusNotFullyChecked. "+
			"An unassigned status must be the conservative cell, or an omission "+
			"anywhere upstream STRENGTHENS a claim", unassigned)
	}

	// (b) A RECORD NO SITE WROTE. This is the "return path added with no
	// classification at all" case: neither boolean set, no pass record.
	if got := verifyStatusFor(verifyRecord{}); got != statusNotFullyChecked {
		t.Errorf("an unwritten verifyRecord derived %v, want statusNotFullyChecked. "+
			"The default arm is the zero cell; a path nobody classified must land "+
			"there", got)
	}
	if got := buildVerifyStatusLine(verifyRecord{}); got != t21ZeroCellLine {
		t.Errorf("unwritten record rendered\n  got  %q\n  want %q", got, t21ZeroCellLine)
	}

	// (c) THE WHOLE 2x2, so "make another status the default" cannot be repaired
	// by moving the zero cell to a named arm. Exactly one cell -- neither bit --
	// is statusNotFullyChecked, and the other three are each their own status.
	pass := &passRecord{full: true, legs: 1, suppliedCosigners: 0}
	cells := []struct {
		name string
		rec  verifyRecord
		want verifyStatus
	}{
		{"no pass, no adverse (the zero cell)", verifyRecord{}, statusNotFullyChecked},
		{"no pass, adverse", verifyRecord{adverse: true}, statusCheckDidNotPass},
		{"pass, no adverse", verifyRecord{pass: pass}, statusVerified},
		{"pass, adverse", verifyRecord{pass: pass, adverse: true}, statusVerifiedOnRetry},
	}
	seen := map[verifyStatus]string{}
	for _, c := range cells {
		got := verifyStatusFor(c.rec)
		if got != c.want {
			t.Errorf("cell %q derived %v, want %v", c.name, got, c.want)
		}
		if prev, dup := seen[got]; dup {
			t.Errorf("cell %q derived %v, which cell %q already derived. Four cells "+
				"must be four states, or the default arm has swallowed a named one",
				c.name, got, prev)
		}
		seen[got] = c.name
	}

	// (d) AND THE ZERO CELL'S LINE IS NOT SHARED. If the default arm were folded
	// into another status the two would render the same text, which is the
	// failure mode a status-count assertion alone would miss.
	if buildVerifyStatusLine(verifyRecord{}) == buildVerifyStatusLine(verifyRecord{adverse: true}) {
		t.Errorf("the zero cell and the adverse cell render the same line; an "+
			"unclassified path is being reported as a check that ran and failed:\n  %q",
			buildVerifyStatusLine(verifyRecord{}))
	}
	if got := buildVerifyStatusLine(verifyRecord{adverse: true}); got != t21DidNotPassLine {
		t.Errorf("adverse-only rendered\n  got  %q\n  want %q", got, t21DidNotPassLine)
	}
}

// T22 -- THE PASS LINE IS GENERATED PER MODE (R9 C-1).
//
// buildVerifyStatusLine takes the RECORD, whose pass field carries `full`
// captured at the success return. On a full run the line names the typed-ms1
// comparison; on watch-only that clause is ABSENT, because the record does not
// contain it and no ms1 was read back or compared.
//
// The mutation is "use a mode-blind literal" -- the exact bug
// multisigVerifyOKMessage had already found and fixed in all four of its arms
// (gui/multisig_verify.go). A mode-blind pass line claims "the ms1 you typed
// matched this seed" on a run where no ms1 is typed, which is a G2 violation:
// it vouches for a check the device never performed.
func TestVerifyPassLineIsGeneratedPerMode(t *testing.T) {
	full := buildVerifyStatusLine(verifyRecord{
		pass: &passRecord{full: true, legs: 1, suppliedCosigners: 0},
	})
	watchOnly := buildVerifyStatusLine(verifyRecord{
		pass: &passRecord{full: false, legs: 1, suppliedCosigners: 0},
	})

	wantFull := t22OneKeyPlateClause + " " + t22MS1Clause
	wantWatchOnly := t22OneKeyPlateClause + " " + t22NoMS1Clause

	if full != wantFull {
		t.Errorf("full-mode pass line\n  got  %q\n  want %q", full, wantFull)
	}
	if watchOnly != wantWatchOnly {
		t.Errorf("watch-only pass line\n  got  %q\n  want %q", watchOnly, wantWatchOnly)
	}

	// The narrow statement the mutation trips, said separately so the failure
	// names the defect rather than a whole-string diff.
	if !strings.Contains(full, t22MS1Clause) {
		t.Errorf("the full-mode pass line does not name the ms1 comparison that ran:\n  %q", full)
	}
	if strings.Contains(watchOnly, t22MS1Clause) {
		t.Errorf("the watch-only pass line claims an ms1 comparison. No ms1 was "+
			"engraved, typed or compared on this run, and the pass record does not "+
			"contain one:\n  %q", watchOnly)
	}
	if full == watchOnly {
		t.Errorf("both modes render the same pass line, so the line is mode-blind:\n  %q", full)
	}

	// AND THE MODE IS NOT THE ONLY THING A LITERAL WOULD LOSE. A single-sig pass
	// record carries suppliedCosigners == 0, so the cosigner clause must be
	// absent from both modes; T27 drives the present half through a multisig
	// flow at step 7.
	for _, line := range []string{full, watchOnly} {
		if strings.Contains(line, t22CosignerClause) {
			t.Errorf("a pass line with suppliedCosigners == 0 carries the cosigner "+
				"clause. There are no cosigners on this record for it to describe:\n  %q",
				line)
		}
	}
}

// T26 -- P6: EVERY POSITIVE CLAIM IS NAMED PER MODE.
//
// For each clause of each pass line, in each mode, a RECORDED observation is
// named. A clause with none is deleted. The table below IS that audit, made
// executable: every clause names the passRecord field that entitles it, the
// field is checked to exist by reflection rather than by eye, and the rendered
// line is compared whole against exactly the clauses the record entitles.
//
// The mutation is "add an unbacked clause to a pass line". Any sentence the
// production line prints that this table does not entitle changes the whole
// string and is reported with its own text.
//
// Guards are ENTITLEMENT, NEVER INFERENCE (§4.7g): each `entitled` closure reads
// one recorded field and nothing else. None reads a verdict, and there is no
// verdict here to read.
func TestVerifyPassLineClausesAreEachBackedByARecord(t *testing.T) {
	type clause struct {
		text     string
		backedBy string // the passRecord field that entitles this clause
		entitled func(p passRecord) bool
	}
	// In render order. Two clauses may share a backing field when they are the
	// two arms of one recorded fact -- that is still one observation naming one
	// clause per mode, which is what P6 asks.
	clauses := []clause{
		{t22OneKeyPlateClause, "legs", func(p passRecord) bool { return p.legs == 1 }},
		{"2 key plates were read back and matched what this run engraved.", "legs",
			func(p passRecord) bool { return p.legs == 2 }},
		{t22MS1Clause, "full", func(p passRecord) bool { return p.full }},
		{t22NoMS1Clause, "full", func(p passRecord) bool { return !p.full }},
		{t22CosignerClause, "suppliedCosigners", func(p passRecord) bool {
			return p.suppliedCosigners > 0
		}},
	}

	// (a) EVERY CLAUSE NAMES A FIELD THAT EXISTS. A renamed or deleted field
	// turns a "recorded observation" into a claim nothing records, and the
	// prose form of this audit would not notice.
	recType := reflect.TypeOf(passRecord{})
	for _, c := range clauses {
		if c.backedBy == "" {
			t.Errorf("clause %q names no recorded observation. A claim with none is "+
				"deleted, not shipped", c.text)
			continue
		}
		if _, ok := recType.FieldByName(c.backedBy); !ok {
			t.Errorf("clause %q says it is backed by passRecord.%s, which does not "+
				"exist", c.text, c.backedBy)
		}
		// Operator strings only draw in the body face, which lacks these glyphs
		// (F-78/F-151): a line carrying one does not draw at all.
		if strings.ContainsAny(c.text, "—–·‘’“”…") {
			t.Errorf("clause %q carries a glyph the body face lacks, so its line does "+
				"not draw", c.text)
		}
	}

	// (b) EACH MODE RENDERS EXACTLY THE CLAUSES ITS RECORD ENTITLES -- no more,
	// which is the half the mutation trips, and no fewer, which is the half that
	// keeps a mode from silently going quiet.
	modes := []struct {
		name string
		rec  passRecord
	}{
		{"single-sig full", passRecord{full: true, legs: 1, suppliedCosigners: 0}},
		{"single-sig watch-only", passRecord{full: false, legs: 1, suppliedCosigners: 0}},
		{"multisig full, one leg, cosigners supplied",
			passRecord{full: true, legs: 1, suppliedCosigners: 2}},
		{"multisig watch-only, two legs, cosigners supplied",
			passRecord{full: false, legs: 2, suppliedCosigners: 1}},
	}
	for _, m := range modes {
		var want []string
		var backing []string
		for _, c := range clauses {
			if c.entitled(m.rec) {
				want = append(want, c.text)
				backing = append(backing, c.backedBy)
			}
		}
		wantLine := strings.Join(want, " ")
		got := buildVerifyStatusLine(verifyRecord{pass: &m.rec})
		if got != wantLine {
			t.Errorf("%s pass line carries a clause no record entitles, or has lost "+
				"one that is entitled\n  got  %q\n  want %q\n  backed by %v",
				m.name, got, wantLine, backing)
		}

		// (c) AND THE RETRY CELL IS THE SAME LINE PLUS ONE SENTENCE, so an
		// unbacked clause cannot hide in the arm the pass cell does not render.
		gotRetry := buildVerifyStatusLine(verifyRecord{pass: &m.rec, adverse: true})
		wantRetry := wantLine + " " + t21RetrySuffix
		if gotRetry != wantRetry {
			t.Errorf("%s retry line\n  got  %q\n  want %q", m.name, gotRetry, wantRetry)
		}
	}
}

// ─── S6a: the single-sig path says what it does not contain ──────────────────
//
// The restore document is read years later, alone, by someone who was not the
// operator, holding a pile of steel and asking ONE question: is this everything?
// Before S6a the document answered neither half of it on the single-sig path --
// it never said whether a seed is on these plates at all, and the shared
// seed-handling ruling it inherited described a registry that path does not have.
//
// These are the step-3 rows of the plan's test table (T4, T7). Both assert
// through buildPlateInventoryLines, which is the function every restore document
// is built from, rather than on the arm-picking helpers underneath it: a helper
// that returns the right string and is never reached is the shape that let the
// multisig instance of this defect ship.

// TestRestoreDocSaysWhetherTheSetContainsASeed is T4.
//
// WATCH-ONLY IS THE ARM THAT DID NOT EXIST. A watch-only set engraves no ms1 at
// all, and every line the document carried was phrased as though one were on the
// bench. The absence line is the one the stranger needs: it says the words must
// come from somewhere else, so a reader holding a COMPLETE watch-only backup does
// not conclude that a seed plate was lost and give up on a recovery that would
// have worked.
//
// "YOUR seed", NOT "THE seed". The definite article, sitting directly under "If
// any of them is missing, this backup is incomplete.", answers "is this
// everything?" with YES -- which is false on a 2-of-3 and costs the recovery.
//
// THE SINGULAR AND PLURAL ARMS ARE NOT COSMETIC. "Each plate marked 'ms1 secret
// share'" over a set holding ONE reads, to a reader counting plates, as though a
// plate were missing -- numberedLabel leaves a one-leg build UNNUMBERED -- so the
// arm is chosen by the ms1 CARD COUNT.
//
// The single-sig fixtures come from singleSigEngraveCards rather than from
// hand-built literals, so the arms are selected by the card shapes that flow
// actually cuts.
func TestRestoreDocSaysWhetherTheSetContainsASeed(t *testing.T) {
	const (
		absence = "Seed: this set contains NO seed. It is watch-only: it records " +
			"the wallet, but it can never spend. If funds must be recovered, the " +
			"seed words must come from somewhere else -- no plate in this set holds " +
			"them."
		presence = "Seed: this set contains YOUR seed, on the plate marked 'ms1 " +
			"secret share'. Treat that plate as the secret itself."
		several = "Seed: this set contains YOUR seeds, on the plates marked 'ms1 " +
			"secret share'. Treat each of those plates as the secret itself."
	)
	b := bundle.Bundle{
		MS1: "ms1secretshare",
		MK1: []string{"mk1a", "mk1b"},
		MD1: []string{"md1a"},
	}
	doc := func(cards []bundleCard, capacity seedCapacity) string {
		return strings.Join(
			buildPlateInventoryLines(cards, oneSeedPassphraseFact(false), capacity), "\n")
	}

	watch := doc(singleSigEngraveCards(b, false), seedCapacityOne)
	if !strings.Contains(watch, absence) {
		t.Errorf("the watch-only single-sig document does not say the set contains NO "+
			"seed. Silence is what a reader mistakes for a lost plate:\nwant %q\ngot:\n%s",
			absence, watch)
	}
	if strings.Contains(watch, presence) || strings.Contains(watch, several) {
		t.Errorf("the watch-only single-sig document claims a seed is on these plates. "+
			"No ms1 is engraved in watch-only mode:\n%s", watch)
	}

	full := doc(singleSigEngraveCards(b, true), seedCapacityOne)
	if !strings.Contains(full, presence) {
		t.Errorf("the full single-sig document does not say which plate carries the "+
			"seed:\nwant %q\ngot:\n%s", presence, full)
	}
	if strings.Contains(full, absence) {
		t.Errorf("the full single-sig document says the set contains NO seed, over an "+
			"engraved ms1 plate:\n%s", full)
	}
	if strings.Contains(full, several) {
		t.Errorf("a set carrying ONE ms1 plate is described in the plural, which reads "+
			"to a plate-counting stranger as an incomplete set:\n%s", full)
	}

	pair := doc([]bundleCard{
		{kind: cardMS1, label: "ms1 secret share 1 of 2", summary: "seed", strings: []string{"ms1a"}},
		{kind: cardMS1, label: "ms1 secret share 2 of 2", summary: "seed", strings: []string{"ms1b"}},
		{kind: cardMK1, label: "mk1 key", summary: "key", strings: []string{"mk1a"}},
	}, seedCapacityMany)
	if !strings.Contains(pair, several) {
		t.Errorf("a set carrying two ms1 plates does not name them in the plural, so "+
			"the document points at one plate while the set is two:\nwant %q\ngot:\n%s",
			several, pair)
	}
	if strings.Contains(pair, presence) {
		t.Errorf("a two-ms1 set is described as though it held a single seed plate:\n%s",
			pair)
	}
}

// TestSeedHandlingRulingIsKeyedOnCapacityAndOnThePlates is T7.
//
// THE RULING HAS TWO INDEPENDENT AXES. The subject is a property of the PATH --
// how many seeds it can hold -- because a build that happened to take one seed
// can still hold several, and two otherwise identical builds must not print
// different documents because of runtime happenstance. The "plates are the
// secret" pair is a property of THIS RUN, and it is false on every watch-only run
// of every path: the document would otherwise assert "no plate in this set holds
// them" and, a few lines later, "the plates are the secret", contradicting itself
// about the one thing it exists to settle.
//
// The multi-seed, seed-bearing arm is asserted BYTE-EXACT against the shipped
// sentence, because the multisig BUILD path's full-mode document must not churn:
// that text is already reviewed, and a whole-string comparison makes a rewording
// a deliberate test update rather than a silent pass.
func TestSeedHandlingRulingIsKeyedOnCapacityAndOnThePlates(t *testing.T) {
	const shipped = "Seed handling: this build does not time out. Every seed you " +
		"entered -- this build can hold several -- stays in device memory until the " +
		"build ends, and on a full build the words are also on the plates as they " +
		"are cut. Do not leave a mid-build machine unattended: the plates are the " +
		"secret. Power the device off when you are done."

	if got := buildSeedHandlingRuling(seedCapacityMany, true); got != shipped {
		t.Errorf("the multi-seed, seed-bearing ruling is no longer byte-identical to "+
			"the S5-reviewed sentence, so the multisig build path's document churns "+
			"for no gain in truth:\nwant %q\ngot  %q", shipped, got)
	}

	one := buildSeedHandlingRuling(seedCapacityOne, true)
	if !strings.Contains(one, "The seed you entered") {
		t.Errorf("a one-seed path's ruling does not name the ONE seed it holds:\n%s", one)
	}
	if strings.Contains(one, "Every seed") {
		t.Errorf("a one-seed path's ruling claims the machine holds every seed entered, "+
			"over a flow with a single seed seam:\n%s", one)
	}
	many := buildSeedHandlingRuling(seedCapacityMany, true)
	if !strings.Contains(many, "Every seed") {
		t.Errorf("the build path's ruling no longer says the machine holds EVERY seed "+
			"entered:\n%s", many)
	}
	if strings.Contains(many, "The seed you entered") {
		t.Errorf("the build path's ruling describes a registry that holds one seed, "+
			"which S5 falsified:\n%s", many)
	}

	// THE SEEDLESS ARM IS COVERED BY NOTHING ELSE IN THE TREE: three existing
	// tests run the glyph guard over an inventory, and all three build it over
	// ms1-BEARING cards, so this fixture is built on purpose.
	watchOnly := []bundleCard{
		{kind: cardMK1, label: "mk1 key", summary: "key", strings: []string{"mk1a"}},
		{kind: cardMD1, label: "md1 descriptor", summary: "policy", strings: []string{"md1a"}},
	}
	doc := strings.Join(buildPlateInventoryLines(
		watchOnly, oneSeedPassphraseFact(false), seedCapacityOne), "\n")
	if strings.Contains(doc, "the plates are the secret") {
		t.Errorf("a watch-only document says the plates are the secret, on a set whose "+
			"own inventory says no plate in it holds the seed:\n%s", doc)
	}
	if strings.Contains(doc, "the words are also on the plates") {
		t.Errorf("a watch-only document claims the words are on the plates. No ms1 is "+
			"engraved on this run:\n%s", doc)
	}
	// The device DOES hold seed material in memory in watch-only mode -- it
	// derives from a mnemonic either way -- so the walk-away warning stays. Only
	// the half about the steel goes.
	for _, want := range []string{"does not time out", "unattended", "still holding seed material"} {
		if !strings.Contains(doc, want) {
			t.Errorf("the watch-only ruling dropped %q. The machine is still holding "+
				"seed material when the operator walks away, and that is the half of "+
				"the exposure no scrub and no mode changes:\n%s", want, doc)
		}
	}

	// EVERY OPERATOR STRING THIS STEP EMITS MUST DRAW. The body face lacks these
	// glyphs, so a line carrying one does not render AT ALL -- on a page whose
	// entire job is to say what the backup does and does not contain. Swept over
	// the whole cross-product, because the arms this step adds are the ones no
	// shipped test reaches.
	oneMS1 := []bundleCard{
		{kind: cardMS1, label: "ms1 secret share", summary: "seed", strings: []string{"ms1a"}},
		{kind: cardMK1, label: "mk1 key", summary: "key", strings: []string{"mk1a"}},
	}
	twoMS1 := []bundleCard{
		{kind: cardMS1, label: "ms1 secret share 1 of 2", summary: "seed", strings: []string{"ms1a"}},
		{kind: cardMS1, label: "ms1 secret share 2 of 2", summary: "seed", strings: []string{"ms1b"}},
	}
	for _, cards := range [][]bundleCard{watchOnly, oneMS1, twoMS1} {
		for _, capacity := range []seedCapacity{seedCapacityOne, seedCapacityMany} {
			for _, uses := range []bool{false, true} {
				for _, line := range buildPlateInventoryLines(
					cards, oneSeedPassphraseFact(uses), capacity) {
					if strings.ContainsAny(line, "—–·‘’“”…") {
						t.Errorf("an inventory line carries a glyph the body face lacks, "+
							"so it does not draw:\n%q", line)
					}
				}
			}
		}
	}
}

// ─── S6a step 4: the status reaches slice index 0, the inventory the tail ────
//
// Step 4 is a SIGNATURE CHANGE and nothing else. Both restore-doc flows gain
// `status` and `extra`; WHAT flows through them arrives at steps 5 and 7. So
// there are exactly two things this step can get wrong, and both are below:
// POSITION, and the placeholder the four call sites pass while the verify is
// still unwired.
//
// THESE ARE NOT T11 AND NOT T20, and writing either here would be writing a test
// that cannot observe what it names. Both assert on a rendered document carrying
// a REAL verification status -- T11 through a production flow, T20 across all
// three of them -- and no flow records a verify bit until step 7, so every
// document in the tree today carries the same zero-cell line. What IS observable
// now is the seam itself, which is why only the seam is asserted.

// TestRestoreDocPutsTheStatusFirstAndTheInventoryLast is step 4's own row: the
// `status` parameter lands at slice index 0 of what restoreDocScreen is given,
// and `extra` lands at the tail, on BOTH restore-doc flows.
//
// THE PAGER IS THE WHOLE REASON THERE ARE TWO PARAMETERS. An earlier round of
// the design specified only a trailing `extra` and had the status ride in it;
// append(lines, extra...) cannot place anything at index 0, so the status would
// have arrived after the descriptor and both addresses -- several pages into a
// screen whose Page button the reader has to know to press. That is the mutation
// this test exists for, and it is a shape the compiler is perfectly happy with.
//
// THE FIXTURE STATUSES ARE SYNTHETIC ON PURPOSE. Position is the claim here, so
// the strings only need to be short and distinguishable: the real status line
// wraps to most of page 1 on this pager, and "did it come first" then cannot be
// read off a single frame at all. The REAL line's placement on a REAL document is
// T11's, at step 7.
func TestRestoreDocPutsTheStatusFirstAndTheInventoryLast(t *testing.T) {
	const (
		statusNeedle = "ZZSTATUSZZ"
		extraNeedle  = "ZZINVENTORYZZ"
	)
	extra := []string{extraNeedle}

	// page1 renders a restore-doc flow and returns its FIRST frame. This screen
	// never presses on its own -- pumpUntil only pumps -- so the first frame is
	// page 1, which is the page the operator is looking at.
	page1 := func(t *testing.T, ui func(ctx *Context)) string {
		t.Helper()
		ctx := NewContext(newPlatform())
		frame, quit := runUI(ctx, func() { ui(ctx) })
		defer quit()
		content, ok := frame()
		if !ok {
			t.Fatal("the restore doc produced no frame")
		}
		return content
	}

	// uiIndex is uiContains' answer to WHERE, and it has to normalise the same
	// way: the space glyph inks nothing, so op.Drawer.ExtractText never sees one
	// and a needle carrying a space matches nothing at all.
	uiIndex := func(content, needle string) int {
		return strings.Index(strings.ToLower(content),
			strings.ReplaceAll(strings.ToLower(needle), " ", ""))
	}

	// firstDocLine is the first line the DOCUMENT itself contributes, so "the
	// status is at index 0" is checkable as "the status is drawn before it".
	check := func(t *testing.T, name, content, firstDocLine string) {
		t.Helper()
		si := uiIndex(content, statusNeedle)
		if si < 0 {
			t.Errorf("%s: page 1 of the restore document does not carry the status at "+
				"all. A trailing parameter cannot reach slice index 0, and a status the "+
				"reader has to page to is one the reader does not have\n  page 1 %q",
				name, content)
			return
		}
		di := uiIndex(content, firstDocLine)
		if di < 0 {
			t.Fatalf("%s: page 1 does not carry %q, so this test cannot say what the "+
				"status precedes\n  page 1 %q", name, firstDocLine, content)
		}
		if si > di {
			t.Errorf("%s: the status is on page 1 but is drawn BELOW %q, so it is not at "+
				"slice index 0\n  page 1 %q", name, firstDocLine, content)
		}
		if uiIndex(content, extraNeedle) >= 0 {
			t.Errorf("%s: the set inventory is on PAGE 1. It is appended at the tail, "+
				"after the descriptor and both addresses, so a first page carrying it is "+
				"a document whose two ends have been swapped\n  page 1 %q", name, content)
		}
	}

	t.Run("single-sig", func(t *testing.T) {
		_, _, pfp, err := decodeXpubBytes(knownAccountXpub84)
		if err != nil {
			t.Fatalf("decodeXpubBytes: %v", err)
		}
		content := page1(t, func(ctx *Context) {
			restoreDocFlow(ctx, &descriptorTheme, knownAccountXpub84, knownMasterFP, pfp,
				md.ScriptWpkh, singleSigPath(84), statusNeedle, extra)
		})
		check(t, "single-sig", content, "Master fp:")
	})

	t.Run("multisig", func(t *testing.T) {
		tpl, keys, err := md.ExpandWalletPolicyChunks(buildAssembledMd1(t, md.MultisigWsh))
		if err != nil {
			t.Fatalf("ExpandWalletPolicyChunks(assembled): %v", err)
		}
		content := page1(t, func(ctx *Context) {
			multisigRestoreDocFlow(ctx, &descriptorTheme, tpl, keys, statusNeedle, extra)
		})
		// "Type:" is multisigRestoreLines' own first line on the expandOK branch,
		// which is the branch every assembled full policy lands on.
		check(t, "multisig", content, "Type:")
	})
}

// s6aCodeOf is readGuiFile with the COMMENTS STRIPPED.
//
// THE SEARCH HAS TO BE OVER CODE, AND THAT IS NOT A REFINEMENT. Every call site
// this file asserts on carries a comment explaining, by name, the very thing
// being asserted, so a site gutted to "" would leave its own justification
// behind and a raw source search would keep passing over it. Measured at step 4
// by running that row's own mutation, which is the only reason it is written
// this way.
func s6aCodeOf(t *testing.T, file string) string {
	t.Helper()
	var code []string
	for _, line := range strings.Split(readGuiFile(t, file), "\n") {
		if i := strings.Index(line, "//"); i >= 0 {
			line = line[:i]
		}
		code = append(code, line)
	}
	return strings.Join(code, "\n")
}

// s6aDocumentFlows are the THREE production flows that reach a restore document,
// each named by the file its call site lives in. Not two: "wired into two of the
// three and forgotten on the third" is not a hazard this cycle imagined, it is
// the shape that let the multisig instance of the defect ship while single-sig
// went without.
var s6aDocumentFlows = []string{"singlesig.go", "multisig.go", "multisig_build.go"}

// TestRestoreDocStatusIsBuiltFromTheRecordOnEveryFlow pins WHAT the production
// call sites pass as the document's first line.
//
// IT WAS TestRestoreDocStatusPlaceholderCannotStrengthenTheDocument UNTIL STEP
// 7, and it is renamed rather than deleted because half of it is unchanged and
// still load-bearing. Step 4 landed the signature ahead of its values and every
// document in the tree carried verifyStatusNotFullyCheckedLine as a placeholder;
// step 7 replaces those literals with buildVerifyStatusLine(rec). Part (a) is
// what made that substitution safe to make and is what keeps it safe: on a run
// that recorded nothing the two must be BYTE-IDENTICAL, so wiring the record in
// can only ever strengthen a document that earned it. Part (b) moves from "every
// site names the placeholder" to "every site builds from the record", which is
// the same guard one step along.
//
// THE FAILURE THIS GUARDS IS "" AND IT IS NOT A HYPOTHETICAL. An empty status
// still occupies index 0 and still compiles; it renders as SILENCE, and silence
// about a verification is precisely what reads as a pass to a stranger years
// later. That is an omission that STRENGTHENS a claim, and it is the one
// direction the design does not allow a mistake in.
func TestRestoreDocStatusIsBuiltFromTheRecordOnEveryFlow(t *testing.T) {
	// (a) THE ZERO CELL IS WHAT AN UNRECORDED VERIFY RENDERS, exactly. Not
	// "similar to": a Skip leaves the record at its zero value on all three
	// flows, and the line those documents carry must be the weakest of the four.
	if got := buildVerifyStatusLine(verifyRecord{}); got != verifyStatusNotFullyCheckedLine {
		t.Errorf("an unrecorded verify does not render the weakest line, so a skipped "+
			"verify produces a document claiming more than the device observed\n"+
			"  zero cell renders %q\n  weakest line      %q",
			got, verifyStatusNotFullyCheckedLine)
	}

	// (b) AND EVERY PRODUCTION CALL SITE BUILDS FROM THE RECORD. A site left on a
	// literal -- the step-4 placeholder, or "" -- is invisible to the compiler and
	// to every rendering test that only ever drives a skipped verify, because a
	// skipped verify renders that same literal. That is precisely the state a
	// regression would leave behind.
	for _, file := range s6aDocumentFlows {
		src := s6aCodeOf(t, file)
		if !strings.Contains(src, "buildVerifyStatusLine(rec)") {
			t.Errorf("gui/%s reaches a restore document without building its status from "+
				"the verify record. A hardcoded status is a claim about a run nobody "+
				"observed, and on a skipped verify it renders identically to the correct "+
				"one -- so nothing that drives only the skip path can see it", file)
		}
		if strings.Contains(src, "verifyStatusNotFullyCheckedLine") {
			t.Errorf("gui/%s still passes the step-4 placeholder literal. The record is "+
				"wired now, and a literal there reports the zero cell on a run that "+
				"verified", file)
		}
	}
}

// ─── S6a step 5: the single-sig path, through the real screens ───────────────
//
// THE DEFECT THESE ROWS EXIST FOR IS VERIFIED BY BYTES, NOT BY READING THE CALL
// GRAPH. On the same twelve words, with and without a BIP-39 passphrase:
//
//	ms1          ms10entrsqqq...cj9sxraq34v7f   IDENTICAL   -- the WORDS only
//	master fp    73c5da0a  vs  fc60c6df         DIFFERS
//
// So a passphrase run engraves plates that restore a DIFFERENT wallet from the
// one the operator just funded, silently, with no error anywhere -- and until
// this step the flow labelled that set "Full (seed + keys)" and printed a
// restore document that never mentioned a passphrase at all. Permanently
// unspendable, and the paperwork vouched for it.
//
// EVERY ROW BELOW DRIVES THE PRODUCTION SCREENS. buildFullModeLabel(true) and
// buildPassphraseInventoryLines already returned the correct sentences before
// this step and were simply unreachable from this flow, so a helper-level
// assertion would have been green on the broken tree. That is the shape that let
// the multisig instance of this defect ship.

// s6aSingleSigOpts picks which single-sig run a walk drives.
type s6aSingleSigOpts struct {
	// passphrase takes the payload-borne BIP-39 passphrase at the prompt. It is
	// a LIVE derivation input all the way down to deriveSingleSigBundle, which is
	// the whole subject of F-198.
	passphrase bool
	// watchOnly picks row 1 of the engrave-mode picker: mk1 + md1, no ms1 on
	// steel.
	watchOnly bool
	// verify answers the post-engrave offer with "Verify now" and drives the REAL
	// singleSigVerifyFlow to its success exit, instead of skipping it (S6a step
	// 7). It is the only way a single-sig document gets a status this run earned:
	// that path has no test seam, and substituting one would leave the production
	// wiring -- the record's declaration, the flow's out-parameter, the pass write
	// at the fall-through -- unexecuted by anything.
	verify bool
}

// s6aSingleSigRun is what a walk observed, in the operator's own order.
type s6aSingleSigRun struct {
	mode   string // the engrave-mode picker's frame -- what is read BEFORE pressing
	census string // the pre-engrave "Plates To Cut" frame
	doc    string // EVERY page of the restore document
	plates int
}

// s6aPumpCollecting is pumpUntil that keeps every frame rather than the last
// one, so a row can say what was drawn BEFORE a screen and not merely that the
// screen was eventually reached.
func s6aPumpCollecting(frame func() (string, bool), want string, maxFrames int) ([]string, bool) {
	var seen []string
	for i := 0; i < maxFrames; i++ {
		c, ok := frame()
		if !ok {
			break
		}
		seen = append(seen, c)
		if uiContains(c, want) {
			return seen, true
		}
	}
	return seen, false
}

// s6aDriveSingleSigToPolicyForm drives engraveSingleSigFlow from its first
// screen to just past the wallet-policy form picker (answered "Full policy
// md1"), and returns the ENGRAVE-MODE frame the operator read on the way.
//
// It stops there because the next screen is the census, and two rows below need
// to observe the run's arrival at that screen differently: T6 wants everything
// drawn before it, and T5 wants to press through it.
func s6aDriveSingleSigToPolicyForm(t *testing.T, ctx *Context, frame func() (string, bool),
	opts s6aSingleSigOpts,
) string {
	t.Helper()
	frame()
	click(&ctx.Router, Button3) // 12 WORDS
	frame()
	driveWords(&ctx.Router, abandonAboutPhrase())
	if c, ok := pumpUntil(frame, "Wallet Type", 160); !ok {
		t.Fatalf("did not reach the wallet-type picker; got %q", c)
	}
	click(&ctx.Router, Button3) // BIP-84 default
	if c, ok := pumpUntil(frame, "Add a BIP-39 passphrase?", 64); !ok {
		t.Fatalf("did not reach the passphrase prompt; got %q", c)
	}
	if opts.passphrase {
		// THE PASSPHRASE COMES FROM THE PAYLOAD, not the keyboard: SYSW 3.3.2
		// admits ClassPassphrase to this program, and the session below holds one.
		click(&ctx.Router, Down) // "Add passphrase"
		frame()
		click(&ctx.Router, Button3)
		frame()
		if c, ok := pumpUntil(frame, "Password from where?", 64); !ok {
			t.Fatalf("the payload's passphrase was not offered; got %q.\nWithout it this "+
				"walk derives the EMPTY-passphrase wallet and asserts nothing about a "+
				"passphrase run", c)
		}
		click(&ctx.Router, Button3) // FROM PAYLOAD (index 0)
		frame()
	} else {
		click(&ctx.Router, Button3) // Skip
	}
	mode, ok := pumpUntil(frame, "What to engrave?", 96)
	if !ok {
		t.Fatalf("did not reach the engrave-mode choice; got %q", mode)
	}
	if opts.watchOnly {
		click(&ctx.Router, Down) // row 1
		frame()
	}
	click(&ctx.Router, Button3)
	if c, ok := pumpUntil(frame, "Engrave wallet policy", 64); !ok {
		t.Fatalf("did not reach the wallet-policy form choice; got %q", c)
	}
	click(&ctx.Router, Button3) // Full policy md1 (row 0, the default)
	return mode
}

// s6aSingleSigBundle re-derives the bundle a walk with these options engraves,
// so a test can present that run's own plates back to its verify.
//
// It uses the SAME inputs the walk presses: abandonAboutPhrase() typed at the
// keyboard, the wallet picker's one-tap default, and the full policy md1. A
// passphrase run is REFUSED here rather than guessed -- the walk takes its
// passphrase from the payload, and a bundle derived without it is a different
// wallet, so the verify would fail for a reason that has nothing to do with what
// is under test and the failure would look like a defect in this step.
func s6aSingleSigBundle(t *testing.T, opts s6aSingleSigOpts) bundle.Bundle {
	t.Helper()
	if opts.passphrase {
		t.Fatal("s6aSingleSigBundle does not model a passphrase run: the readback it " +
			"builds would be a different wallet's")
	}
	m, err := bip39.ParseMnemonic(abandonAboutPhrase())
	if err != nil {
		t.Fatalf("ParseMnemonic: %v", err)
	}
	b, _, _, _, err := deriveSingleSigBundle(m, "", &chaincfg.MainNetParams,
		singleSigPath(singleSigDefault.purpose), singleSigDefault.script)
	if err != nil {
		t.Fatalf("deriveSingleSigBundle: %v", err)
	}
	return b
}

// s6aDriveSingleSigVerifyOK drives the REAL singleSigVerifyFlow from the offer to
// its SUCCESS EXIT -- the fall-through at the closing brace, which is the only
// exit that writes a pass record and the one an implementer told to "write the
// record at each return site" never reaches at all.
//
// The screens are pressed in the flow's own order: the re-typed seed, the
// wallet-type picker, the passphrase prompt, the NFC readback, and -- in full
// mode only -- the hand-typed ms1. Every one is asserted on the way past, for
// s5DriveVerify's reason: tapping through a screen that did not appear silently
// answers the next one, and a walk that does that reports a status for a verify
// nobody ran.
func s6aDriveSingleSigVerifyOK(t *testing.T, ctx *Context, frame func() (string, bool),
	opts s6aSingleSigOpts,
) {
	t.Helper()
	if c, ok := pumpUntil(frame, "Choose number of words", 96); !ok {
		t.Fatalf("the verify did not ask for the seed again; got %q", c)
	}
	click(&ctx.Router, Button3) // 12 WORDS
	frame()
	driveWords(&ctx.Router, abandonAboutPhrase())
	if c, ok := pumpUntil(frame, "Wallet Type", 240); !ok {
		t.Fatalf("the verify did not reach the wallet-type picker; got %q", c)
	}
	click(&ctx.Router, Button3) // the one-tap default, which is what the engrave took
	if c, ok := pumpUntil(frame, "Add a BIP-39 passphrase?", 96); !ok {
		t.Fatalf("the verify did not reach the passphrase prompt; got %q", c)
	}
	click(&ctx.Router, Button3) // Skip
	frame()
	if c, ok := pumpUntil(frame, "mk1 keys:", 96); !ok {
		t.Fatalf("the readback never reached the gatherer's tally; got %q", c)
	}
	click(&ctx.Router, Button3) // Done adding cards
	frame()
	if !opts.watchOnly {
		if c, ok := pumpUntil(frame, "Type ms1", 96); !ok {
			t.Fatalf("full mode did not ask for the ms1; got %q", c)
		}
		runes(&ctx.Router, strings.ToLower(s6aSingleSigBundle(t, opts).MS1))
		click(&ctx.Router, Button3)
		frame()
	}
	if c, ok := pumpUntil(frame, "Verify OK", 128); !ok {
		t.Fatalf("the single-sig verify did not pass over this run's OWN plates; the "+
			"screen reads %q.\nAny other landing means the flow recorded something other "+
			"than a pass, and the document below would then say so -- truthfully, and "+
			"about a walk that proves nothing", c)
	}
	click(&ctx.Router, Button3) // dismiss the Verify OK notice
	frame()
}

// s6aSingleSigWalk drives engraveSingleSigFlow end to end, THROUGH EVERY PLATE,
// to the restore document.
//
// IT CUTS THE STEEL because both surfaces this step is about are post-decision:
// the mode label is what the operator reads before pressing, and the restore
// document is drawn after the last plate. A walk that stopped at the engrave
// picker would reach neither.
func s6aSingleSigWalk(t *testing.T, opts s6aSingleSigOpts) s6aSingleSigRun {
	t.Helper()
	var run s6aSingleSigRun
	synctest.Test(t, func(t *testing.T) {
		e := newEngraver()
		p := newPlatform()
		p.display = sh2DisplaySize
		p.engraver = e
		ctx := NewContext(p)
		if opts.passphrase {
			ctx.sysw = sessionHolding(s5PassphraseRecord)
		}
		if opts.verify {
			// THE PLATES THIS RUN IS ABOUT TO CUT, waiting on the gatherer's payload
			// seam for the readback. They are DERIVED, not scraped off the engraver:
			// the verify compares the cards it reads against a bundle it re-derives
			// from the re-typed seed, so a readback built the same way as the engrave
			// is the honest one to present. Every card enters through the same offer()
			// a scanned card takes.
			b := s6aSingleSigBundle(t, opts)
			ctx.syswBundleSeeds = append(append([]string(nil), b.MK1...), b.MD1...)
		}
		done := false
		frame, quit := runUI(ctx, func() {
			engraveSingleSigFlow(ctx, &descriptorTheme)
			done = true
		})
		defer quit()

		run.mode = s6aDriveSingleSigToPolicyForm(t, ctx, frame, opts)
		c, ok := pumpUntil(frame, "Plates To Cut", 96)
		if !ok {
			t.Fatalf("the plate census was not shown before the engrave; got %q", c)
		}
		run.census = c
		click(&ctx.Router, Button3)
		frame()

		for {
			if _, ok := pumpUntil(frame, "Choose engraving", 96); !ok {
				break
			}
			click(&ctx.Router, Button3) // first variant
			frame()
			s5EngraveOnePlate(t, ctx, frame, e)
			run.plates++
			if run.plates > 24 {
				t.Fatal("the engrave loop did not terminate")
			}
		}
		if run.plates == 0 {
			t.Fatal("no plate was cut, so this walk never reached the post-engrave surfaces")
		}

		// A WATCH-ONLY SET ENDS ON THE ms1 HAND-ENGRAVE REMINDER, because no ms1
		// was cut here; a full set suppresses it (bundleShowMs1Reminder).
		if _, ok := pumpUntil(frame, "Verify the engraved plates?", 32); !ok {
			click(&ctx.Router, Button3) // dismiss the reminder
			frame()
			if c, ok := pumpUntil(frame, "Verify the engraved plates?", 96); !ok {
				t.Fatalf("the verify offer was not reached after %d plate(s); got %q",
					run.plates, c)
			}
		}
		if opts.verify {
			click(&ctx.Router, Button3) // Verify now (row 0)
			frame()
			s6aDriveSingleSigVerifyOK(t, ctx, frame, opts)
		} else {
			click(&ctx.Router, Down) // Skip
			frame()
			click(&ctx.Router, Button3)
			frame()
		}

		// THE DOCUMENT IS A PAGER and the inventory is appended at the TAIL, after
		// the descriptor chunks and both addresses, so a single-frame assertion
		// misses every line this step adds. The document is found by its TITLE
		// rather than by a body line: a long status line pushes "Descriptor:" off
		// page 1, and two of the four are long.
		run.doc = s6aRestoreDocPages(t, ctx, frame)
		click(&ctx.Router, Button3) // done with the restore doc
		for i := 0; i < 256 && !done; i++ {
			if _, ok := frame(); !ok {
				break
			}
		}
		if !done {
			t.Fatal("the flow did not return after the restore doc")
		}
	})
	return run
}

// TestSingleSigPassphraseRunTellsTheOperatorWhatIsMissing is T1 and T2, and both
// halves are funds-bearing.
//
// T1 is the label the operator reads BEFORE pressing. T2 is the artifact that
// outlives them: a stranger holding this steel in five years has no other way to
// learn that a third spending factor was ever in play, and no plate in the set
// can be made to yield it.
func TestSingleSigPassphraseRunTellsTheOperatorWhatIsMissing(t *testing.T) {
	run := s6aSingleSigWalk(t, s6aSingleSigOpts{passphrase: true})
	t.Logf("the passphrased single-sig run cut %d plate(s)", run.plates)
	t.Logf("engrave-mode screen: %q", run.mode)
	t.Logf("restore doc: %q", run.doc)

	// T1 -- THE LABEL. "Full (seed + keys)" over a passphrase build tells the
	// operator a partial backup is a complete one, at the one moment they can
	// still choose otherwise.
	if !uiContains(run.mode, "NOT passphrase") {
		t.Errorf("the single-sig engrave-mode picker calls a PASSPHRASE build "+
			"\"Full (seed + keys)\":\n%q\nms1 encodes the WORDS ONLY -- measured, it is "+
			"byte-identical with and without a passphrase, while the master fingerprint "+
			"is not -- so this row promises a backup that restores a DIFFERENT wallet. "+
			"buildFullModeLabel(true) already returns the correct string; it was simply "+
			"not called here", run.mode)
	}

	// T2 -- THE DOCUMENT. Two claims, and the second is what F-198 understated:
	// this document had no inventory AT ALL, so it stated no plate count, no
	// completeness claim and no passphrase fact.
	if !uiContains(run.doc, "BIP-39 passphrase WAS used") {
		t.Errorf("the single-sig restore document never mentions the passphrase:\n%q\n"+
			"A reader holding this pile of steel has no way to learn a third spending "+
			"factor exists. The words alone restore a different wallet, with no error", run.doc)
	}
	if !uiContains(run.doc, "This backup is") {
		t.Errorf("the single-sig restore document carries no plate inventory at all:\n%q\n"+
			"The one fact that tells a reader whether they hold ALL of it is how many "+
			"plates there are", run.doc)
	}
	// AND THE CONSEQUENCE, not just the fact. "A passphrase was used" without
	// "these plates do not reach the money" leaves a reader free to assume the
	// steel is sufficient.
	if !uiContains(run.doc, "do not reach the money") {
		t.Errorf("the document names the passphrase but not what its absence costs:\n%q",
			run.doc)
	}
	// The seed statement lands on the same document (step 3's text, step 5's
	// wiring): a full run says WHICH plate is the secret.
	if !uiContains(run.doc, "this set contains YOUR seed") {
		t.Errorf("the full single-sig document does not say which plate carries the "+
			"seed:\n%q", run.doc)
	}
}

// TestSingleSigBareRunDoesNotCryWolf is T3, the NON-VACUITY arm.
//
// Without it, "always print the passphrase warning" satisfies T1 and T2 -- and a
// picker that cries DEFAULT when the operator chose is a picker whose warnings
// get ignored. The bare run must read exactly as it always did, and its document
// must ANSWER the reader's question rather than go silent: silence leaves them
// unable to tell a complete backup from one whose operator forgot to write the
// passphrase down.
//
// IT DRIVES THE MODE PICKER RATHER THAN buildFullModeLabel, because the named
// mutation ("make buildFullModeLabel always return the passphrase arm") is not
// the only way this half goes wrong: a call site passing `true` unconditionally
// is invisible to a helper-level assertion. The DOCUMENT half is asserted on
// buildPassphraseInventoryLines directly, as its prior art does -- this walk
// cuts no plates and so never reaches a restore document.
func TestSingleSigBareRunDoesNotCryWolf(t *testing.T) {
	var mode string
	synctest.Test(t, func(t *testing.T) {
		p := newPlatform()
		// THE REAL PANEL, and this is load-bearing rather than tidy. On
		// newPlatform's default (smaller) display the full row draws as
		// "seed + keys, NOT passph" -- truncated mid-word, because widget.Label
		// does not wrap -- and the negative assertion below then cannot see the
		// clause it is written to forbid. Measured by running this row's own
		// mutation, which failed on the OTHER assertion and left this one vacuous.
		// sh2DisplaySize is the machine, and is the same panel assertChoiceLabelFits
		// budgets the label against.
		p.display = sh2DisplaySize
		ctx := NewContext(p)
		frame, quit := runUI(ctx, func() {
			engraveSingleSigFlow(ctx, &descriptorTheme)
		})
		defer quit()
		mode = s6aDriveSingleSigToPolicyForm(t, ctx, frame, s6aSingleSigOpts{})
	})
	t.Logf("bare engrave-mode screen: %q", mode)

	if uiContains(mode, "NOT passphrase") {
		t.Errorf("a single-sig build with NO passphrase is labelled as though a factor "+
			"were missing:\n%q\nA warning that fires when the operator chose plainly is a "+
			"warning that gets ignored on the run where it matters", mode)
	}
	if !uiContains(mode, "Full (seed + keys)") {
		t.Errorf("the bare run's engrave-mode picker no longer offers the full row at "+
			"all:\n%q", mode)
	}

	// THE DOCUMENT HALF. A bare run must SAY so, not go quiet.
	bare := strings.Join(buildPassphraseInventoryLines(oneSeedPassphraseFact(false)), " ")
	if !strings.Contains(bare, "No BIP-39 passphrase was used") {
		t.Errorf("the bare arm of the inventory does not answer the reader's question, so "+
			"a complete backup is indistinguishable from one missing a factor:\n%s", bare)
	}
	if strings.Contains(bare, "BIP-39 passphrase WAS used") {
		t.Errorf("the bare arm warns about a passphrase nobody used:\n%s", bare)
	}
}

// TestSingleSigAbortIsTheLastScreenOfTheProgram is T5, and it is F-197 driven
// through the real screens.
//
// Everything past the engrave vouches for a COMPLETE set: the verify offer runs
// over plates that were never all cut -- the md1 is emitted LAST, so the readback
// dies reading as "your plates are unreadable" -- and the restore document is
// headed "This backup is N plates ... If any of them is missing, this backup is
// incomplete." The abort modal must be the operator's last screen.
//
// IT ASSERTS IT SAW THE ABORT FIRST. If a future change moves the abort route,
// this row would otherwise start passing by never reaching an abort at all,
// which is the vacuity its multisig twins are written against.
//
// NO ENGRAVER IS NEEDED: Back at the FIRST plate's style picker is
// bundleEngrave's set-level abort, and nothing has been cut at that point.
func TestSingleSigAbortIsTheLastScreenOfTheProgram(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := NewContext(newPlatform())
		done := false
		frame, quit := runUI(ctx, func() {
			engraveSingleSigFlow(ctx, &descriptorTheme)
			done = true
		})
		defer quit()

		s6aDriveSingleSigToPolicyForm(t, ctx, frame, s6aSingleSigOpts{})
		if c, ok := pumpUntil(frame, "Plates To Cut", 96); !ok {
			t.Fatalf("the plate census was not shown; got %q", c)
		}
		click(&ctx.Router, Button3)
		frame()
		if c, ok := pumpUntil(frame, "Choose engraving", 96); !ok {
			t.Fatalf("the engrave-style picker was not reached; got %q", c)
		}

		// THE ABORT.
		click(&ctx.Router, Button1)
		if c, ok := pumpUntil(frame, "Bundle Incomplete", 96); !ok {
			t.Fatalf("Back at the engrave picker did not produce the abort warning; got "+
				"%q.\nWithout it this row asserts nothing about what follows an abort", c)
		}
		click(&ctx.Router, Button3) // dismiss the abort modal

		var after []string
		for i := 0; i < 256 && !done; i++ {
			c, ok := frame()
			if !ok {
				break
			}
			after = append(after, c)
		}
		joined := strings.Join(after, " || ")
		if !done {
			t.Fatalf("the program did not end after the abort; it drew:\n%q", joined)
		}
		for _, banned := range []string{"Verify the engraved plates?", "This backup is", "Descriptor:"} {
			if uiContains(joined, banned) {
				t.Errorf("after \"This set is not a usable backup yet\", the single-sig flow "+
					"still drew %q.\nThe verify cannot succeed over a set whose md1 was never "+
					"cut, and the restore document describes a partial set as a backup.\nDrawn "+
					"after the abort:\n%q", banned, joined)
			}
		}
	})
}

// TestSingleSigShowsThePlateCensusBeforeTheEngrave is T6.
//
// The operator commits to a 2- or 3-plate cut, minutes per plate, and until this
// step no screen on this path stated the count. Back at the census aborts before
// anything is cut, which is the last moment that is free.
//
// THE CLAIM IS ORDERING, NOT PRESENCE. A census drawn after the first plate is
// no census at all, so this row keeps every frame and reports what was drawn
// first -- which is also what makes the "remove the census call" mutation fail
// with the engrave picker's own text rather than with a bare timeout.
func TestSingleSigShowsThePlateCensusBeforeTheEngrave(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := NewContext(newPlatform())
		frame, quit := runUI(ctx, func() {
			engraveSingleSigFlow(ctx, &descriptorTheme)
		})
		defer quit()

		s6aDriveSingleSigToPolicyForm(t, ctx, frame, s6aSingleSigOpts{})
		seen, ok := s6aPumpCollecting(frame, "Plates To Cut", 96)
		joined := strings.Join(seen, " || ")
		if !ok {
			t.Fatalf("the single-sig flow never drew a plate census. It drew:\n%q\n"+
				"The operator commits to minutes per plate with no count, and the last "+
				"free moment to abort is before the first cut", joined)
		}
		for _, banned := range []string{"Choose engraving", "Card 1 of"} {
			if uiContains(strings.Join(seen[:len(seen)-1], " || "), banned) {
				t.Errorf("the engrave picker (%q) was drawn BEFORE the plate census, so the "+
					"count arrives after the operator has already committed:\n%q",
					banned, joined)
			}
		}
		// AND THE COUNT IS ON IT. A census screen with no number is a screen that
		// answers nothing; the count is derived through bundlePlatePlan, the same
		// function bundleEngrave loops, so it cannot drift from what is cut. A FULL
		// single-sig run is ms1(1) + mk1(2) + md1(3) = 6, and every one of those
		// terms comes from the plan rather than from this comment.
		census := seen[len(seen)-1]
		if !uiContains(census, "This engraves 6 plates") {
			t.Errorf("the census screen does not state the plate count this run actually "+
				"cuts:\n%q", census)
		}
		// THE SCREEN IS A PAGER, so the instruction the count exists FOR is not on
		// page 1 at all -- measured, not assumed: page 1 ends mid-inventory.
		pages, ok := s5PageForNeedle(t, ctx, frame, "have that many blanks ready", 8)
		if !ok {
			t.Errorf("the census never tells the operator to have that many blanks ready "+
				"before starting, on any page:\n%q", pages)
		}
	})
}

// TestEveryFlowsRestoreDocumentSaysWhatItCheckedAndWhatItHolds is T7c, and from
// step 7 also T20, T11, T23 and T24.
//
// IT WAS TestSeedHandlingRulingMatchesEachPathsCapacity, and the rename is the
// honest half of a consolidation forced by arithmetic. This function already
// walked all three production flows to a rendered restore document -- 129s of
// the gui package's DEFAULT 10-MINUTE test timeout, against 401s already on the
// clock. Step 7's status rows need exactly the same three walks. Given their
// own, the package died at 600s mid-engrave with every assertion passing, so
// each walk now carries every row that can observe it, and each row's block
// below names itself. No row lost an assertion; each still fails against its own
// mutation. Leaving the old name on it would have made the name the false
// comment.
//
// T7c is the row that covers the one thing no compiler and no other test can
// see: WHICH capacity each of the three call sites hands over.
//
// buildPlateInventoryLines takes a seedCapacity, and T7 asserts it produces the
// right text WHEN HANDED a given one. That says nothing about whether any call
// site hands it the right one -- and a swapped argument compiles, renders, and
// looks entirely healthy. Only the build path holds a registry that can carry a
// seed per held slot; the supply path and single-sig each have ONE seed seam by
// construction, so "Every seed you entered -- this build can hold several" is
// false on both of them.
//
// EVERY ARM DRIVES ITS FLOW TO A RENDERED DOCUMENT. A unit assertion on the
// helper is exactly what let the multisig instance of this cycle's defect ship,
// and the capacity argument is not visible from the helper at all.
//
// THE ARMS ARE WHOLE-CLAUSE AND MUTUALLY EXCLUSIVE: each asserts its own subject
// is present AND the other absent, so swapping either call site's argument fails
// on both halves rather than on a needle that happens to survive.
func TestEveryFlowsRestoreDocumentSaysWhatItCheckedAndWhatItHolds(t *testing.T) {
	const (
		oneSubject  = "The seed you entered"
		manySubject = "Every seed you entered"
	)
	check := func(t *testing.T, path, doc, want, notWant string) {
		t.Helper()
		if !uiContains(doc, want) {
			t.Errorf("the %s restore document's seed-handling ruling does not carry %q, "+
				"which is the subject that path's seed capacity entitles:\n%q",
				path, want, doc)
		}
		if uiContains(doc, notWant) {
			t.Errorf("the %s restore document's seed-handling ruling says %q. That "+
				"describes a different machine from the one the operator was standing at, "+
				"on the artifact read years later:\n%q", path, notWant, doc)
		}
	}

	// SINGLE-SIG: one seed seam (seedEntryFlow, and nothing else reads a secret).
	// Driven WATCH-ONLY, which is also the cheaper set -- two cards rather than
	// three -- and which exercises the arm that drops the "plates are the secret"
	// pair, so the ruling under test is the one this path most often prints.
	//
	// AND IT DRIVES A REAL VERIFY (T20, flow 1 of 3; T11's scope-absent arm). This
	// path has NO test seam, so its status reaches its document by a real verify or
	// not at all -- and every other single-sig walk in the tree skips the verify,
	// which renders the same literal the step-4 placeholder did. Nothing that
	// drives only the skip path can tell a wired document from an unwired one.
	//
	// WATCH-ONLY ALSO PUTS THE MODE-GENERATED HALF OF THE PASS LINE UNDER TEST: a
	// watch-only pass line must say NO SECRET WAS COMPARED rather than merely leave
	// the ms1 clause off. T27 drives the full-mode single-sig pass line.
	t.Run("single-sig", func(t *testing.T) {
		run := s6aSingleSigWalk(t, s6aSingleSigOpts{watchOnly: true, verify: true})
		t.Logf("the verified watch-only single-sig run cut %d plate(s)", run.plates)
		check(t, "single-sig", run.doc, oneSubject, manySubject)

		// T20 (flow 1 of 3) and T11.
		pass := buildVerifyPassLine(passRecord{full: false, legs: 1, suppliedCosigners: 0})
		s6aAssertOneStatusLine(t, "single-sig", run.doc, "statusVerified", pass,
			s6aStatusCells(pass))
		s6aAssertStatusFirstAndScope(t, "single-sig", run.doc, pass, "Master fp:", false)
		// The watch-only document must not contradict itself about the one thing
		// it exists to settle: it says no plate holds the seed, so it may not also
		// say the plates ARE the secret.
		if uiContains(run.doc, "the plates are the secret") {
			t.Errorf("the watch-only single-sig document says the plates are the secret, "+
				"over a set whose own inventory says no plate in it holds the seed:\n%q",
				run.doc)
		}
		if !uiContains(run.doc, "this set contains NO seed") {
			t.Errorf("the watch-only single-sig document never says the set holds no seed. "+
				"Silence is what a reader mistakes for a lost plate:\n%q", run.doc)
		}
	})

	// MULTISIG SUPPLY: also one seed seam, and this document CHANGES -- it said
	// "this build can hold several" today, which S5 wired to a path it does not
	// fit.
	//
	// AND IT CARRIES T20 (flow 2 of 3), T11's scope-PRESENT arm, and T23.
	//
	// T23 -- STICKINESS -- IS A REGRESSION TEST AGAINST THE PLAN ITSELF. Its
	// mutation is not hypothetical: "implement the status as last-wins" is what an
	// earlier fold specified in writing, and it silently downgrades a check that
	// FAILED to "these plates were not fully checked" -- the weaker line, on the
	// run where the device has evidence against the steel. Concretely: clear the
	// record before each attempt in gui/multisig.go and the second attempt starts
	// clean, erasing what the first observed. It compiles, and nothing else in the
	// tree notices.
	//
	// Attempt 1 finds something adverse and reports INCOMPLETE, so the loop
	// re-offers. Attempt 2 is a clean ABANDON -- the operator walked out of the
	// gather -- which records nothing at all. Sticky means attempt 1 still decides.
	t.Run("multisig-supply", func(t *testing.T) {
		doc := s6aSupplyDocWalk(t,
			s6aVerifyStep{res: verifyIncomplete, adverse: true},
			s6aVerifyStep{res: verifyAbandoned},
		)
		check(t, "multisig supply", doc, oneSubject, manySubject)

		cells := s6aStatusCells(buildVerifyPassLine(passRecord{full: true, legs: 1}))
		s6aAssertOneStatusLine(t, "multisig supply", doc,
			"statusCheckDidNotPass", t21DidNotPassLine, cells)
		// "Type:" is multisigRestoreLines' own first line on the expandOK branch,
		// which every assembled full policy lands on.
		s6aAssertStatusFirstAndScope(t, "multisig supply", doc, t21DidNotPassLine, "Type:", true)
		if uiContains(doc, t21ZeroCellLine) {
			t.Errorf("the document reports \"these plates were not fully checked\" over a "+
				"run where a check RAN AND DID NOT PASS. A later attempt that observed "+
				"nothing has erased what an earlier one observed, and the weaker line is "+
				"the one that reached the steel's only paperwork:\n%q", doc)
		}
	})

	// MULTISIG BUILD: the registry, one seed per held slot across distinct
	// masters. Its document must stay byte-identical to the S5-reviewed sentence.
	//
	// AND IT CARRIES T20 (flow 3 of 3) and T24.
	//
	// T24 -- P2 ON THE RETRY PATH. The mutation is "drop the && adverseRecorded
	// arm" -- a two-state collapse. A clean retry after a failed check is a real
	// and common operator story, and the document it produces must say so: the
	// plates verify NOW, and an earlier attempt on the same set did not.
	// Collapsing the two loses the class of failure a later success cannot
	// retro-explain -- a plate that read wrong once and right the next time is a
	// plate to re-cut, and this document is the only place that fact survives.
	t.Run("multisig-build", func(t *testing.T) {
		p := passRecord{full: true, legs: 2, suppliedCosigners: 1}
		pass := buildVerifyPassLine(p)
		doc := s6aBuildDocWalk(t,
			s6aVerifyStep{res: verifyIncomplete, adverse: true},
			s6aVerifyStep{res: verifyComplete, pass: &p},
		)
		check(t, "multisig build", doc, manySubject, oneSubject)

		cells := s6aStatusCells(pass)
		s6aAssertOneStatusLine(t, "multisig build", doc,
			"statusVerifiedOnRetry", cells["statusVerifiedOnRetry"], cells)
		s6aAssertStatusFirstAndScope(t, "multisig build", doc,
			cells["statusVerifiedOnRetry"], "Type:", false)
		if !uiContains(doc, t21RetrySuffix) {
			t.Errorf("the document reports a bare pass over a set an earlier check did "+
				"not pass:\n%q", doc)
		}
	})
}

// ─── S6a step 7: the verify status, through the flows that record it ─────────
//
// These are T11, T20, T23, T24, T25 and T27. Every one of them needs something
// step 2 could not build: a RENDERED DOCUMENT carrying a status a flow actually
// recorded. Asserted on buildVerifyStatusLine alone they would all pass on a
// tree where no flow writes a record at all, which is the state steps 2 through
// 6 left behind and exactly the state this step exists to leave.
//
// FOUR TESTS, THREE WALKS, AND THE ARITHMETIC IS THE REASON. A walk that reaches
// a restore document must cut every plate first, and the gui package's test
// binary runs under `go test`'s DEFAULT 10-MINUTE TIMEOUT with 401s already on
// the clock. A first draft of this section gave each row its own walk -- seven
// of them, 435s -- and the package died at 600s mid-engrave, with every
// assertion passing. So each walk carries every row it can OBSERVE, and each
// row's block below names itself:
//
//	single-sig walk  statusVerified        T20 (flow 1 of 3), T11 (absent arm)
//	supply walk      statusCheckDidNotPass T20 (flow 2 of 3), T11 (present arm), T23
//	build walk       statusVerifiedOnRetry T20 (flow 3 of 3), T24
//
// That is not a weakening: no row lost an assertion, and each still fails
// against its own mutation. It is the same consolidation the tree already makes
// where one walk carries T1 and T2.
//
// TWO KINDS OF DRIVING, AND THE SPLIT IS DELIBERATE.
//
//   - WHAT THE RECORD CONTAINS is driven through the REAL verify flows (T27, and
//     the two exit-mapping rows at the end of this file): suppliedCosigners is
//     computed from the policy keys and the covered slots, and no stub can tell
//     you whether that computation is right.
//   - WHERE THE RECORD GOES is driven through the real engrave flows with the
//     verify substituted at the multisigVerifyFn seam. The stub replaces the
//     verdict source and the record's contents; the offer screens, the retry
//     loop, the record's DECLARATION SITE -- outside the loop, which is the whole
//     of stickiness -- and the call that turns it into the document's first line
//     are all production code under the test. A real second attempt would mean
//     driving an entire readback twice inside a walk that has already cut nine
//     plates. The single-sig path has NO seam, so its status reaches its document
//     by a REAL verify or not at all, and that is how this file drives it.

// s6aVerifyStep is one attempt at the verify, as the seam reports it: the
// verdict the callers' retry loop reads, and the two facts the document is
// generated from.
type s6aVerifyStep struct {
	res     multisigVerifyResult
	adverse bool
	pass    *passRecord
}

// s6aRecordingStub substitutes multisigVerifyFn and WRITES THE RECORD, which is
// the one thing s5StubVerifyFn does not do and the only thing this step is
// about. `steps` is consumed one per call; the last one repeats.
func s6aRecordingStub(t *testing.T, steps ...s6aVerifyStep) *int {
	t.Helper()
	if len(steps) == 0 {
		t.Fatal("s6aRecordingStub needs at least one step")
	}
	calls := 0
	prev := multisigVerifyFn
	multisigVerifyFn = func(ctx *Context, th *Colors, full bool, expectedSlots []int,
		engravedMd1 []string, rec *verifyRecord,
	) multisigVerifyResult {
		s := steps[min(calls, len(steps)-1)]
		calls++
		if rec == nil {
			t.Error("the caller dispatched the verify with a NIL record, so nothing it " +
				"observed can reach the restore document")
			return s.res
		}
		if s.adverse {
			rec.adverse = true
		}
		if s.pass != nil {
			p := *s.pass
			rec.pass = &p
		}
		return s.res
	}
	t.Cleanup(func() { multisigVerifyFn = prev })
	return &calls
}

// s6aAnswerVerifyOffers presses through the post-engrave verify offer and every
// re-offer the retry loop makes, one attempt per step, and leaves the flow on
// its restore document.
//
// It presses THE ROW THAT SAYS WHAT IT WANTS, not a row number, for
// s5RowIndexOf's reason: the loop keys on the index Choose returns and looks up
// no label anywhere.
func s6aAnswerVerifyOffers(t *testing.T, ctx *Context, frame func() (string, bool),
	steps []s6aVerifyStep,
) {
	t.Helper()
	offer, ok := pumpUntil(frame, "Verify the engraved plates?", 128)
	if !ok {
		t.Fatalf("the verify offer was not reached after the engrave; got %q", offer)
	}
	s5ChooseRowLabelled(t, ctx, frame, offer, []string{"Verify now", "Skip"}, "Verify now")
	for i, s := range steps {
		if s.res != verifyIncomplete && s.res != verifyFailed {
			return // this verdict leaves the loop; the document is next
		}
		retry, ok := pumpUntil(frame, multisigVerifyRetryLead, 256)
		if !ok {
			t.Fatalf("attempt %d returned a verdict the loop re-offers on, and the offer "+
				"was not made again; the screen reads %q", i+1, retry)
		}
		want := "VERIFY AGAIN"
		if i == len(steps)-1 {
			want = "CONTINUE"
		}
		s5ChooseRowLabelled(t, ctx, frame, retry, []string{"VERIFY AGAIN", "CONTINUE"}, want)
	}
}

// s6aRestoreDocPages returns EVERY PAGE of the restore document the flow is
// currently on, joined in page order.
//
// IT WAITS ON THE TITLE, NOT ON A BODY LINE. "Descriptor:" is on page 1 under a
// short status and is pushed off it by a long one, so a walk that waits for a
// body line reports "the document was never shown" for a document that is on
// the display -- and the two longest of the four status lines are the two this
// step exists to render.
func s6aRestoreDocPages(t *testing.T, ctx *Context, frame func() (string, bool)) string {
	t.Helper()
	if c, ok := pumpUntil(frame, "Restore Doc", 256); !ok {
		t.Fatalf("the restore document was not reached; the screen reads %q", c)
	}
	doc, _ := s5PageForNeedle(t, ctx, frame, "\x00never matches\x00", 32)
	return doc
}

// s6aSupplyDocWalk drives supplyMultisigPolicyFlow end to end -- every plate,
// the verify offer, the retry loop -- and returns every page of the restore
// document it ends on.
//
// IT SUPPLIES THE 2-OF-2 FIXTURE, not Trace B, and the difference is 14 plates
// against 4. Both reach the same screen by the same code; the only thing the
// bigger policy buys this row is 40 seconds of the package's timeout budget.
func s6aSupplyDocWalk(t *testing.T, steps ...s6aVerifyStep) string {
	t.Helper()
	md1, _ := s5PassphrasedSupplyPolicy(t)
	var doc string
	synctest.Test(t, func(t *testing.T) {
		s6aRecordingStub(t, steps...)
		e := newEngraver()
		p := newPlatform()
		p.display = sh2DisplaySize
		p.engraver = e
		ctx := NewContext(p)
		// The session holds the PASSPHRASE this policy was built with; the md1
		// arrives on the bundle seam. Without the passphrase the typed seed fills no
		// slot and the walk dies on the cross-match instead.
		ctx.sysw = sessionHolding(s5PassphraseRecord)
		ctx.syswBundleSeeds = append([]string(nil), md1...)
		done := false
		frame, quit := runUI(ctx, func() {
			supplyMultisigPolicyFlow(ctx, &descriptorTheme)
			done = true
		})
		defer quit()

		if c, ok := pumpUntil(frame, "md1 descriptors: 1", 64); !ok {
			t.Fatalf("the supplied md1 never reached the gatherer's tally; got %q", c)
		}
		click(&ctx.Router, Button3) // Done adding cards
		frame()
		if c, ok := pumpUntil(frame, "Choose number of words", 64); !ok {
			t.Fatalf("the gather did not hand off to seed entry; got %q", c)
		}
		click(&ctx.Router, Button3) // 12 WORDS
		frame()
		driveWords(&ctx.Router, fixtureMasterA)
		if c, ok := pumpUntil(frame, "Add a BIP-39 passphrase?", 240); !ok {
			t.Fatalf("the passphrase prompt was not reached; got %q", c)
		}
		click(&ctx.Router, Down) // "Add passphrase"
		frame()
		click(&ctx.Router, Button3)
		frame()
		if c, ok := pumpUntil(frame, "Password from where?", 64); !ok {
			t.Fatalf("the payload's passphrase was not offered; got %q", c)
		}
		click(&ctx.Router, Button3) // FROM PAYLOAD
		frame()
		if c, ok := pumpUntil(frame, "What to engrave?", 96); !ok {
			t.Fatalf("the engrave-mode choice was not reached; got %q", c)
		}
		click(&ctx.Router, Button3) // the FULL row
		frame()
		if c, ok := pumpUntil(frame, "Plates To Cut", 96); !ok {
			t.Fatalf("the plate census was not shown; got %q", c)
		}
		click(&ctx.Router, Button3)
		frame()

		plates := 0
		for {
			if _, ok := pumpUntil(frame, "Choose engraving", 96); !ok {
				break
			}
			click(&ctx.Router, Button3) // first variant
			frame()
			s5EngraveOnePlate(t, ctx, frame, e)
			plates++
			if plates > 24 {
				t.Fatal("the engrave loop did not terminate")
			}
		}
		if plates == 0 {
			t.Fatal("no plate was cut, so this walk never reached the post-engrave surfaces")
		}
		t.Logf("the supply run cut %d plate(s)", plates)

		s6aAnswerVerifyOffers(t, ctx, frame, steps)
		doc = s6aRestoreDocPages(t, ctx, frame)
		click(&ctx.Router, Button3) // done with the restore doc
		for i := 0; i < 256 && !done; i++ {
			if _, ok := frame(); !ok {
				break
			}
		}
		if !done {
			t.Error("the supply flow did not return after the restore doc")
		}
	})
	return doc
}

// s6aBuildDocWalk is the same for buildMultisigPolicyFlow.
func s6aBuildDocWalk(t *testing.T, steps ...s6aVerifyStep) string {
	t.Helper()
	records := cosignerCardRecords(t, 4) // A@0, B@0, C@0, A@1
	var doc string
	synctest.Test(t, func(t *testing.T) {
		s6aRecordingStub(t, steps...)
		e := newEngraver()
		p := newPlatform()
		p.display = sh2DisplaySize
		p.engraver = e
		ctx := NewContext(p)
		ctx.sysw = sessionHolding(records...)
		done := false
		// THE RASTER HARNESS, for s5EngraveEveryPlate's measured reason: the frames
		// a plate takes depend on which harness pumps them.
		frame, _, _, quit := runUITouchRaster(ctx, func() {
			buildMultisigPolicyFlow(ctx, &descriptorTheme)
			done = true
		})
		defer quit()

		s5DriveBuildToEngravePicker(t, ctx, frame)
		t.Logf("the build run cut %d plate(s)", s5EngraveEveryPlate(t, ctx, frame, e))
		s6aAnswerVerifyOffers(t, ctx, frame, steps)
		doc = s6aRestoreDocPages(t, ctx, frame)
		click(&ctx.Router, Button3) // done with the restore doc
		for i := 0; i < 256 && !done; i++ {
			if _, ok := frame(); !ok {
				break
			}
		}
		if !done {
			t.Error("the build flow did not return after the restore doc")
		}
	})
	return doc
}

// s6aStatusCells is the four §4.7c lines for one pass record, in cell order.
// The two pass cells are GENERATED, so they are assembled from the clause
// constants rather than pasted.
func s6aStatusCells(passLine string) map[string]string {
	return map[string]string{
		"statusNotFullyChecked": t21ZeroCellLine,
		"statusCheckDidNotPass": t21DidNotPassLine,
		"statusVerified":        passLine,
		"statusVerifiedOnRetry": passLine + " " + t21RetrySuffix,
	}
}

// s6aAssertOneStatusLine asserts a rendered document carries `want` and no OTHER
// status line.
//
// THE FOUR LINES ARE NOT PAIRWISE DISJOINT AND THAT IS BY DESIGN: the retry
// cell's line is the pass cell's line plus one sentence. So "exactly one" is
// measured as "the ones present are exactly the ones `want` contains", which is
// the strongest statement that is true of the design -- and a document carrying
// the pass line where the retry line belongs still fails it, because the retry
// sentence is then missing.
func s6aAssertOneStatusLine(t *testing.T, path, doc, wantName, want string, cells map[string]string) {
	t.Helper()
	if want == "" {
		t.Fatalf("%s: the expected status line is empty, so this assertion says nothing", path)
	}
	if !uiContains(doc, want) {
		t.Errorf("the %s restore document does not carry its status line.\n  want %s\n  %q\n"+
			"A document silent about its verification is one a stranger reads as verified:\n%q",
			path, wantName, want, doc)
	}
	for name, line := range cells {
		if strings.Contains(want, line) {
			continue // `want` itself, or the pass line the retry cell extends
		}
		if uiContains(doc, line) {
			t.Errorf("the %s restore document carries %s as well as %s. Exactly one status "+
				"line belongs on a document, and two of the four cells cannot both be true "+
				"of one run:\n  also present %q\n%q", path, name, wantName, line, doc)
		}
	}
}

// s6aUIIndex is uiContains' answer to WHERE, normalised the same way: the space
// glyph inks nothing, so a needle carrying a space matches nothing at all.
func s6aUIIndex(content, needle string) int {
	return strings.Index(strings.ToLower(content),
		strings.ReplaceAll(strings.ToLower(needle), " ", ""))
}

// s6aAssertStatusFirstAndScope is T11, applied to one rendered document.
//
// POSITION IS THE WHOLE CLAIM, and it is the one a compiler is perfectly happy
// to lose. restoreDocScreen is a PAGER: append(lines, status) puts the status
// after the descriptor and both addresses, several pages into a screen whose
// Page button the reader has to know to press, and a verification status the
// reader has to page to is one the reader does not have. That is T11's mutation
// -- "pass it via the trailing extra parameter, as the round-1 fold specified" --
// and it changes nothing a build or a unit test would notice.
//
// THE SCOPE LINE IS THE OTHER HALF. Under a check that RAN AND DID NOT PASS,
// nothing below the status is conditioned on it -- the descriptor, the addresses
// and the plate inventory all describe what the run INTENDED to cut -- so a
// reader who takes the wallet details as confirmed by the page they are printed
// on has read it exactly as laid out. It is deliberately NOT widened to the
// other cells, whose own lines already say what to do and whose modal occupant
// skipped the verify on a backup that is probably fine.
//
// `firstDocLine` is the first line the DOCUMENT ITSELF contributes, so "the
// status is at slice index 0" is checkable as "the status is drawn before it".
func s6aAssertStatusFirstAndScope(t *testing.T, path, doc, status, firstDocLine string, wantScope bool) {
	t.Helper()
	si := s6aUIIndex(doc, status)
	if si < 0 {
		t.Fatalf("the %s restore document does not carry its status line at all:\n%q", path, doc)
	}
	di := s6aUIIndex(doc, firstDocLine)
	if di < 0 {
		t.Fatalf("the %s restore document does not carry %q, so this test cannot say what "+
			"the status precedes:\n%q", path, firstDocLine, doc)
	}
	if si > di {
		t.Errorf("the %s restore document draws its status AFTER %q, so it is not at slice "+
			"index 0. A trailing parameter cannot reach index 0, and a status the reader "+
			"has to page to is one the reader does not have:\n%q", path, firstDocLine, doc)
	}
	gi := s6aUIIndex(doc, verifyStatusScopeLine)
	switch {
	case wantScope && gi < 0:
		t.Errorf("the %s restore document reports a check that RAN AND DID NOT PASS and "+
			"does not scope the page beneath it. Everything below describes what the run "+
			"intended to engrave, and nothing on the page says so:\n%q", path, doc)
	case wantScope && (gi < si || gi > di):
		t.Errorf("the %s restore document carries the scoping line, but not between the "+
			"status and the wallet it scopes (status@%d scope@%d %q@%d):\n%q",
			path, si, gi, firstDocLine, di, doc)
	case !wantScope && gi >= 0:
		t.Errorf("the %s restore document carries the scoping line under a status that is "+
			"not \"a check ran and did not pass\". Its own line already tells the reader "+
			"what to do, and a second warning on a backup that is probably fine teaches "+
			"operators to tap past modals:\n%q", path, doc)
	}
}

// T20(a) -- THE 2x2 RENDERS FOUR DISTINCT, NON-EMPTY, BYTE-EXACT LINES.
//
// The mutations are "return the same string for two cells" -- caught by the
// distinctness check -- and "return an empty slice", which on a document is a
// blank first line: caught here by the emptiness check and on all three flows by
// the three walks below.
func TestVerifyStatusCellsRenderFourDistinctLines(t *testing.T) {
	pass := passRecord{full: true, legs: 1, suppliedCosigners: 0}
	cells := s6aStatusCells(t22OneKeyPlateClause + " " + t22MS1Clause)
	rendered := map[string]string{
		"statusNotFullyChecked": buildVerifyStatusLine(verifyRecord{}),
		"statusCheckDidNotPass": buildVerifyStatusLine(verifyRecord{adverse: true}),
		"statusVerified":        buildVerifyStatusLine(verifyRecord{pass: &pass}),
		"statusVerifiedOnRetry": buildVerifyStatusLine(verifyRecord{pass: &pass, adverse: true}),
	}
	seen := map[string]string{}
	for name, want := range cells {
		got := rendered[name]
		if got != want {
			t.Errorf("%s renders\n  got  %q\n  want %q", name, got, want)
		}
		if got == "" {
			t.Errorf("%s renders the empty string. A status that is absent renders as "+
				"SILENCE, and silence is the one thing this line exists to stop being "+
				"mistakable for a pass", name)
		}
		if prev, dup := seen[got]; dup {
			t.Errorf("%s and %s render the SAME line, so two cells of the 2x2 are "+
				"indistinguishable on the document:\n  %q", name, prev, got)
		}
		seen[got] = name
	}
}

// T25 -- NO VERDICT IS READ.
//
// The status is derived from TWO RECORDED BOOLEANS and from nothing else. A
// verdict is a summary of a flow rather than a record of what it observed, and
// keying a pass arm on `res == verifyComplete` is the proxy two review rounds
// proved unsound: verifyComplete is returned by a run whose earlier attempt
// failed, and verifyFailed is returned at a site where the device observed
// nothing about the plates at all (a re-typed seed that will not derive).
//
// THE SEARCH IS OVER CODE WITH THE COMMENTS STRIPPED, and that is measured
// rather than tidy: at step 4 a guard searching raw source passed with the code
// gutted, because the name it sought appeared in a nearby comment. The
// derivation's own comment says it names no verdict "deliberately not even in
// this comment" -- which is a promise, and this is the check.
func TestVerifyStatusDerivationReadsNoVerdict(t *testing.T) {
	code := s6aCodeOf(t, "verify_status.go")

	// (a) THE DERIVATION NAMES NO VERDICT. Not the result type, and not one of
	// its five constants. Whole-file, because a derivation split across two
	// helpers must not be able to hide a verdict in the other one.
	for _, name := range []string{
		"multisigVerifyResult", "verifyComplete", "verifyIncomplete",
		"verifyFailed", "verifyRefused", "verifyAbandoned",
	} {
		if strings.Contains(code, name) {
			t.Errorf("gui/verify_status.go names %s. The status is derived from what the "+
				"flow RECORDED, never from the verdict it returned: verifyComplete is "+
				"returned by a run whose earlier attempt failed, and verifyFailed is "+
				"returned where the device observed nothing about the plates at all", name)
		}
	}
	// `res` is the name both callers give the verdict; searched with enough
	// context that "restore" and "result" do not match it.
	for _, tok := range []string{" res ", " res.", "res ==", "res !=", "= res\n", "(res)"} {
		if strings.Contains(code, tok) {
			t.Errorf("gui/verify_status.go reads %q -- the callers' verdict variable", tok)
		}
	}

	// (b) AND IT DOES READ BOTH RECORDED BOOLEANS. Without this half the file
	// could satisfy (a) by deriving nothing at all, which is the wholesale
	// deletion a bare negative always admits.
	for _, want := range []string{"rec.pass", "rec.adverse"} {
		if !strings.Contains(code, want) {
			t.Errorf("gui/verify_status.go never reads %s, so the cell it derives is not "+
				"the 2x2 of the two recorded facts", want)
		}
	}

	// (c) AND THE CELL IS THE ONE THE TWO BOOLEANS NAME, driven rather than read:
	// a verdict-keyed arm that happened to spell its constants differently would
	// still have to get these four wrong.
	p := passRecord{full: true, legs: 1}
	for _, c := range []struct {
		name string
		rec  verifyRecord
		want verifyStatus
	}{
		{"neither", verifyRecord{}, statusNotFullyChecked},
		{"adverse only", verifyRecord{adverse: true}, statusCheckDidNotPass},
		{"pass only", verifyRecord{pass: &p}, statusVerified},
		{"both", verifyRecord{pass: &p, adverse: true}, statusVerifiedOnRetry},
	} {
		if got := verifyStatusFor(c.rec); got != c.want {
			t.Errorf("the %s record derived %v, want %v", c.name, got, c.want)
		}
	}
}

// s6aMultisigFullOneSlotVerify drives the REAL multisigVerifyFlow in FULL mode
// over a run that engraved ONE plate of a THREE-key policy, and returns what the
// flow recorded plus the policy's key count.
//
// THIS IS T27'S NON-VACUITY FIXTURE AND IT IS NAMED RATHER THAN ASSUMED. The
// cosigner clause renders iff suppliedCosigners > 0, and the obvious multisig
// fixture -- the self-multisig where the operator holds every slot -- covers
// every key, records 0, and lets T27 pass while asserting nothing. Here the
// expectation is ONE slot of THREE, so two policy keys are never covered by a
// verified leg and the count the flow computes is 2. The test asserts that,
// rather than trusting this comment.
//
// FULL MODE ON BOTH HALVES OF T27, deliberately: a single-sig full verify and a
// one-slot multisig full verify both record {full: true, legs: 1}, and their
// truthful lines still differ. That identity is the defect this field exists to
// fix, so it is the fixture the test is built on.
func s6aMultisigFullOneSlotVerify(t *testing.T) (rec verifyRecord, policyKeys int) {
	t.Helper()
	md1, plates, _ := s5TraceBEngraved(t, true)
	m, err := bip39.ParseMnemonic(fixtureMasterA)
	if err != nil {
		t.Fatalf("ParseMnemonic: %v", err)
	}
	_, keys, err := md.ExpandWalletPolicyChunks(md1)
	if err != nil {
		t.Fatalf("ExpandWalletPolicyChunks: %v", err)
	}
	idx, origin, ok := findUserSlot(m, "", &chaincfg.MainNetParams, keys)
	if !ok {
		t.Fatal("findUserSlot: master A matches no slot of Trace B")
	}
	b, err := deriveMultisigLeg(m, "", &chaincfg.MainNetParams, origin, md1, true)
	if err != nil {
		t.Fatalf("deriveMultisigLeg(@%d): %v", idx, err)
	}
	if b.MS1 == "" {
		t.Fatal("the full-mode leg carries no ms1, so this walk cannot drive full mode")
	}
	plate := plates[s5PlateFor(t, plates, verifyLeg{Slot: idx, B: b})]
	records := append(append([]string(nil), md1...), plate...)

	p := newPlatform()
	p.display = sh2DisplaySize
	ctx := NewContext(p)
	ctx.syswBundleSeeds = records
	frame, quit := runUI(ctx, func() {
		multisigVerifyFlow(ctx, &descriptorTheme, true /* FULL */, []int{idx}, md1, &rec)
	})
	defer quit()

	if c, ok := pumpUntil(frame, "mk1 keys:", 64); !ok {
		t.Fatalf("the readback never reached the gatherer's tally; got %q", c)
	}
	click(&ctx.Router, Button3) // Done adding cards
	frame()
	if c, ok := pumpUntil(frame, "Choose number of words", 64); !ok {
		t.Fatalf("the gather did not hand off to seed entry; got %q", c)
	}
	click(&ctx.Router, Button3) // 12 WORDS
	frame()
	driveWords(&ctx.Router, fixtureMasterA)
	if c, ok := pumpUntil(frame, "passphrase", 240); !ok {
		t.Fatalf("the passphrase prompt was not reached; got %q", c)
	}
	click(&ctx.Router, Button3) // Skip
	frame()
	if c, ok := pumpUntil(frame, "Type ms1", 96); !ok {
		t.Fatalf("full mode did not ask for the ms1; got %q", c)
	}
	runes(&ctx.Router, strings.ToLower(b.MS1))
	click(&ctx.Router, Button3)
	frame()
	if c, ok := pumpUntil(frame, "Verify OK", 128); !ok {
		t.Fatalf("the one-slot full multisig verify did not pass over its own plate; the "+
			"screen reads %q", c)
	}
	return rec, len(keys)
}

// s6aSingleSigFullVerify drives the REAL singleSigVerifyFlow in FULL mode over
// the bundle a bare walk engraves, and returns what it recorded.
func s6aSingleSigFullVerify(t *testing.T) verifyRecord {
	t.Helper()
	opts := s6aSingleSigOpts{} // full mode, no passphrase
	b := s6aSingleSigBundle(t, opts)
	var rec verifyRecord
	p := newPlatform()
	p.display = sh2DisplaySize
	ctx := NewContext(p)
	ctx.syswBundleSeeds = append(append([]string(nil), b.MK1...), b.MD1...)
	frame, quit := runUI(ctx, func() {
		singleSigVerifyFlow(ctx, &descriptorTheme, true /* FULL */, false, &rec)
	})
	defer quit()
	s6aDriveSingleSigVerifyOK(t, ctx, frame, opts)
	return rec
}

// T27 -- THE PATH AXIS.
//
// A single-sig full pass line OMITS "Other cosigners' keys are taken as
// supplied."; a multisig full pass line INCLUDES it. Both records say
// {full: true, legs: 1}, so nothing about the mode or the plate count separates
// them: the only thing that can is the count of policy keys this run took as
// supplied rather than checked, which is why the record carries a COUNT and not
// a flow identifier.
//
// BOTH HALVES COME FROM REAL FLOWS. The multisig count is computed inside
// multisigVerifyFlow from the policy keys and the covered slots, and no stub can
// say whether that computation is right -- its mutation, "leave suppliedCosigners
// unwritten", leaves a zero value that makes the clause vanish EVERYWHERE and
// that no other test in the tree notices.
//
// NON-VACUITY IS ASSERTED, NOT ASSUMED. A fixture where the operator holds every
// slot covers every key, records 0, and would let this test pass while observing
// the clause's PRESENT arm never once.
func TestVerifyPassLineNamesCosignersOnlyWhereThereAreSome(t *testing.T) {
	msRec, policyKeys := s6aMultisigFullOneSlotVerify(t)
	if msRec.pass == nil {
		t.Fatal("the multisig verify passed on screen and recorded no pass, so the " +
			"document it feeds would report a run that never happened")
	}
	if msRec.pass.suppliedCosigners <= 0 {
		t.Fatalf("the multisig fixture covered every one of its %d policy keys, so "+
			"suppliedCosigners is %d and the cosigner clause renders nowhere. This test "+
			"would then pass while never once observing the clause PRESENT, which is the "+
			"regression it exists for", policyKeys, msRec.pass.suppliedCosigners)
	}
	if got, want := msRec.pass.suppliedCosigners, policyKeys-msRec.pass.legs; got != want {
		t.Errorf("the flow counted %d supplied cosigners over a %d-key policy in which %d "+
			"leg(s) were verified, want %d. The count is the policy keys NOT covered by a "+
			"verified leg", got, policyKeys, msRec.pass.legs, want)
	}

	ssRec := s6aSingleSigFullVerify(t)
	if ssRec.pass == nil {
		t.Fatal("the single-sig verify passed on screen and recorded no pass. The " +
			"success exit is the fall-through at the end of the flow, not a return, and " +
			"it is the one an enumeration of return sites misses")
	}
	if ssRec.pass.suppliedCosigners != 0 {
		t.Errorf("a single-sig run recorded %d supplied cosigners. That path derives and "+
			"compares every key in its own descriptor; there is no cosigner key for the "+
			"count to describe", ssRec.pass.suppliedCosigners)
	}

	msLine := buildVerifyStatusLine(msRec)
	ssLine := buildVerifyStatusLine(ssRec)
	// The fixture's own numbers, logged so a reader of the output can see that
	// the PRESENT arm was observed rather than take this test's word for it.
	t.Logf("multisig: %d policy keys, %d leg(s) verified, %d taken as supplied\n  %q",
		policyKeys, msRec.pass.legs, msRec.pass.suppliedCosigners, msLine)
	t.Logf("single-sig: %d leg verified, %d taken as supplied\n  %q",
		ssRec.pass.legs, ssRec.pass.suppliedCosigners, ssLine)
	if !strings.Contains(msLine, t22CosignerClause) {
		t.Errorf("the multisig pass line does not scope itself to the keys this run "+
			"checked. Two of its policy's keys were taken as supplied and the document "+
			"vouches for the wallet without saying so:\n  %q", msLine)
	}
	if strings.Contains(ssLine, t22CosignerClause) {
		t.Errorf("the single-sig pass line names cosigners. This wallet has none, and "+
			"a document describing a different wallet shape is the misdescription G1 "+
			"forbids:\n  %q", ssLine)
	}

	// AND THE TWO DIFFER BY EXACTLY THAT CLAUSE. Both records are {full: true,
	// legs: 1}, so if the path axis were dropped the two lines would be identical
	// -- which is the Critical this field closed, stated as an equation.
	if msRec.pass.full != ssRec.pass.full || msRec.pass.legs != ssRec.pass.legs {
		t.Fatalf("the two fixtures no longer agree on mode and leg count (multisig "+
			"{full:%v legs:%d}, single-sig {full:%v legs:%d}), so a difference between "+
			"their lines no longer isolates the path axis",
			msRec.pass.full, msRec.pass.legs, ssRec.pass.full, ssRec.pass.legs)
	}
	if want := ssLine + " " + t22CosignerClause; msLine != want {
		t.Errorf("the multisig and single-sig pass lines differ by more than the cosigner "+
			"clause\n  multisig   %q\n  single-sig %q\n  want       %q", msLine, ssLine, want)
	}
}

// TestSingleSigVerifyRecordsWhatItObserved drives THREE of the eleven exits of
// singleSigVerifyFlow and asserts which of the two booleans each one writes.
//
// IT IS NOT A ROW OF THE PLAN'S TEST TABLE AND IT IS HERE ON PURPOSE. The
// eleven-exit mapping is the step-1 artifact this step implements, and every
// other test in this file observes it through the SUCCESS exit alone: delete
// both `rec.adverse = true` writes from gui/singlesig_verify.go and the whole
// suite stays green, while a single-sig verify that FAILED produces a document
// reading "these plates were not fully checked" -- the weaker line, over steel
// the device has evidence against. A mutation nothing notices is a line nothing
// tests.
//
// Three exits, one per class the mapping defines:
//
//	:117 ADVERSE -- cards were read and could not be ACCOUNTED for
//	:146 ADVERSE -- the comparator RAN and disagreed
//	:69  BENIGN  -- Back at the seed keyboard, before any plate is touched
//
// THE BENIGN ARM IS THE ONE THAT KEEPS THE OTHER TWO HONEST. "Set adverse at
// every return" passes both adverse arms and is a G2 violation: it prints "a
// verification check ran and did not pass" over a run where the operator pressed
// Back before the device read anything, which is a false statement about the
// device's own behaviour.
func TestSingleSigVerifyRecordsWhatItObserved(t *testing.T) {
	opts := s6aSingleSigOpts{watchOnly: true} // no ms1 to type on any arm
	b := s6aSingleSigBundle(t, opts)
	foreign := func(t *testing.T) []string {
		t.Helper()
		m, err := bip39.ParseMnemonic(fixtureMasterA)
		if err != nil {
			t.Fatalf("ParseMnemonic: %v", err)
		}
		// Another wallet's key plate: a different seed at a different purpose.
		fb, _, _, _, err := deriveSingleSigBundle(m, "", &chaincfg.MainNetParams,
			singleSigPath(44), md.ScriptPkh)
		if err != nil {
			t.Fatalf("deriving a foreign mk1: %v", err)
		}
		return fb.MK1
	}

	// drive runs the flow over `records` and returns the record it wrote. `back`
	// presses Back at the seed keyboard instead of typing.
	drive := func(t *testing.T, records []string, back bool) (verifyRecord, string) {
		t.Helper()
		var rec verifyRecord
		p := newPlatform()
		p.display = sh2DisplaySize
		ctx := NewContext(p)
		ctx.syswBundleSeeds = append([]string(nil), records...)
		done := false
		frame, quit := runUI(ctx, func() {
			singleSigVerifyFlow(ctx, &descriptorTheme, false /* watch-only */, false, &rec)
			done = true
		})
		defer quit()

		if c, ok := pumpUntil(frame, "Choose number of words", 96); !ok {
			t.Fatalf("the verify did not ask for the seed; got %q", c)
		}
		if back {
			click(&ctx.Router, Button1) // Back, before a plate has been touched
			for i := 0; i < 96 && !done; i++ {
				if _, ok := frame(); !ok {
					break
				}
			}
			if !done {
				t.Fatal("Back at the seed keyboard did not leave the verify")
			}
			return rec, ""
		}
		click(&ctx.Router, Button3) // 12 WORDS
		frame()
		driveWords(&ctx.Router, abandonAboutPhrase())
		if c, ok := pumpUntil(frame, "Wallet Type", 240); !ok {
			t.Fatalf("the verify did not reach the wallet-type picker; got %q", c)
		}
		click(&ctx.Router, Button3) // the one-tap default
		if c, ok := pumpUntil(frame, "Add a BIP-39 passphrase?", 96); !ok {
			t.Fatalf("the verify did not reach the passphrase prompt; got %q", c)
		}
		click(&ctx.Router, Button3) // Skip
		frame()
		if c, ok := pumpUntil(frame, "mk1 keys:", 96); !ok {
			t.Fatalf("the readback never reached the gatherer's tally; got %q", c)
		}
		click(&ctx.Router, Button3) // Done adding cards
		last := ""
		for i := 0; i < 128 && !done; i++ {
			c, ok := frame()
			if !ok {
				break
			}
			last = c
		}
		return rec, last
	}

	// :117 -- TWO key cards. The gather returned complete cards and the accounting
	// over them failed, which is "a plate could not be read or accounted for".
	t.Run("readback-not-accounted-for", func(t *testing.T) {
		records := append(append(append([]string(nil), b.MK1...), b.MD1...), foreign(t)...)
		rec, last := drive(t, records, false)
		if !uiContains(last, "Need one key card") {
			t.Fatalf("an ambiguous readback did not reach the accounting refusal; the "+
				"screen reads %q", last)
		}
		if !rec.adverse {
			t.Error("cards came off the plates, the device could not account for them, and " +
				"the record says nothing adverse. The document then reports \"not fully " +
				"checked\" over a pile the device could not make sense of")
		}
		if rec.pass != nil {
			t.Error("a refused readback recorded a PASS")
		}
	})

	// :146 -- the comparator ran and disagreed.
	t.Run("comparator-disagreed", func(t *testing.T) {
		records := append(append([]string(nil), foreign(t)...), b.MD1...)
		rec, last := drive(t, records, false)
		if !uiContains(last, "Verify Failed") {
			t.Fatalf("a foreign key plate did not FAIL the verify; the screen reads %q", last)
		}
		if !rec.adverse {
			t.Error("the comparator ran, disagreed, and the record says nothing adverse. " +
				"The restore document then says these plates were merely not fully checked, " +
				"which is the weaker line over the run where the device has evidence AGAINST " +
				"the steel")
		}
		if rec.pass != nil {
			t.Error("a failed comparison recorded a PASS")
		}
	})

	// :69 -- Back at the seed keyboard. NEITHER bit: the zero cell, from INSIDE
	// the flow.
	t.Run("back-before-any-plate-is-read", func(t *testing.T) {
		records := append(append([]string(nil), b.MK1...), b.MD1...)
		rec, _ := drive(t, records, true)
		if rec.adverse {
			t.Error("pressing Back at the seed keyboard recorded something ADVERSE. No " +
				"plate had been touched -- the readback is 40 lines further on -- so the " +
				"document would claim a verification check ran and did not pass, which is a " +
				"false statement about what this device did")
		}
		if rec.pass != nil {
			t.Error("an abandoned verify recorded a PASS")
		}
		if got := buildVerifyStatusLine(rec); got != t21ZeroCellLine {
			t.Errorf("a benign exit does not land in the zero cell\n  got  %q\n  want %q",
				got, t21ZeroCellLine)
		}
	})
}

// TestMultisigVerifyRecordsWhatItObserved is the multisig twin of the row above,
// and it exists for the same reason: every other multisig assertion in this file
// drives the record through the multisigVerifyFn STUB, so deleting the real
// flow's writes leaves them all green.
//
// Two arms, one per class. The adverse one is the comparator disagreeing over a
// foreign key plate; the benign one is a structural refusal taken BEFORE the
// operator is asked to present anything, which observes nothing about any plate
// and must therefore write neither bit.
func TestMultisigVerifyRecordsWhatItObserved(t *testing.T) {
	t.Run("comparator-disagreed", func(t *testing.T) {
		_, md1, _, slot := s5OneSlotReadback(t)
		m, err := bip39.ParseMnemonic(fixtureMasterA)
		if err != nil {
			t.Fatalf("ParseMnemonic: %v", err)
		}
		foreign, _, _, _, err := deriveSingleSigBundle(m, "", &chaincfg.MainNetParams,
			singleSigPath(44), md.ScriptPkh)
		if err != nil {
			t.Fatalf("deriving a foreign mk1: %v", err)
		}
		records := append(append([]string(nil), md1...), foreign.MK1...)
		var rec verifyRecord
		last, _ := s5DriveVerifyRec(t, records, []int{slot}, md1, fixtureMasterA, &rec)
		if !uiContains(last, "Verify Failed") {
			t.Fatalf("a foreign key plate did not FAIL the verify; the screen reads %q", last)
		}
		if !rec.adverse {
			t.Error("the comparator ran, disagreed, and the record says nothing adverse. " +
				"The restore document then reports these plates as merely not fully checked")
		}
		if rec.pass != nil {
			t.Error("a failed comparison recorded a PASS")
		}
	})

	t.Run("refused-before-any-plate-is-read", func(t *testing.T) {
		records, md1, _, _ := s5OneSlotReadback(t)
		var rec verifyRecord
		p := newPlatform()
		p.display = sh2DisplaySize
		ctx := NewContext(p)
		ctx.syswBundleSeeds = append([]string(nil), records...)
		frame, quit := runUI(ctx, func() {
			multisigVerifyFlow(ctx, &descriptorTheme, false, nil, md1, &rec)
		})
		defer quit()
		if c, ok := pumpUntil(frame, "nothing to verify", 16); !ok {
			t.Fatalf("a verify handed NO engraved slot did not refuse; got %q", c)
		}
		if rec.adverse {
			t.Error("a refusal taken BEFORE the gather recorded something adverse. Nothing " +
				"was read, compared or accounted for, so the document would claim a check " +
				"ran and did not pass over plates this run never looked at")
		}
		if rec.pass != nil {
			t.Error("a refusal recorded a PASS")
		}
	})
}

// ─── T8: the abort-gate justification must name all THREE tail-carrying callers ─
//
// S6a §4.8b comment 1. gui/bundle_flow.go justified where the abort warning's
// text lives with "both engraving callers now gate it on this function's own
// caller returning bundleEngraveDone", and that sentence was FALSE ON THE DAY IT
// WAS WRITTEN: three bundleEngrave call sites carry a post-engrave tail and only
// two of them gated. S5's I-12 fold gated the two multisig callers and
// generalised from the two it happened to be looking at; engraveSingleSigFlow
// already existed, ungated, with a tail, so a single-sig abort still printed the
// restore document. S6a step 5 gated it and step 8 corrects the sentence.
//
// This is the class of claim a reviewer INHERITS AS A GIVEN rather than checks --
// F-196's lesson, one stage earlier in this same codebase -- so the correction is
// pinned by a test rather than left to the next reader to re-derive.
//
// WHY THE POSITIVE HALF IS SCOPED TO ONE COMMENT BLOCK. A bare `!Contains` over
// the file is satisfied by deleting the justification outright, and by any
// differently-false replacement (§5.2). A positive `Contains` over the whole FILE
// is barely better: it can be satisfied by some OTHER comment that happens to
// carry the words, which is exactly how step 4's guard passed with the code
// gutted. So the positive half reads only the doc comment immediately above
// bundleAbortWarningText -- the sentence actually under review -- while the
// negative half keeps the whole file, where it is strictly stronger.
func TestBundleAbortJustificationNamesEveryTailCarryingCaller(t *testing.T) {
	const file = "bundle_flow.go"

	// NEGATIVE, FILE-WIDE. The false claim must be gone from the file, not merely
	// moved out of the paragraph it was in.
	if src := readGuiFile(t, file); strings.Contains(src, "both engraving callers") {
		t.Error("gui/bundle_flow.go still justifies the abort warning's placement with " +
			"\"both engraving callers\". THREE bundleEngrave call sites carry a " +
			"post-engrave tail -- engraveSingleSigFlow (gui/singlesig.go), " +
			"supplyMultisigPolicyFlow (gui/multisig.go) and buildMultisigPolicyFlow " +
			"(gui/multisig_build.go) -- so \"both\" undercounts the callers the gate " +
			"has to hold for, and it undercounted them when it was written")
	}

	// POSITIVE, SCOPED TO THE SENTENCE UNDER REVIEW. Each of the three callers that
	// carries a tail is named, because a corrected COUNT that still does not say
	// WHICH callers leaves the next reader exactly where the last one was.
	doc := guiDocComment(t, file, "func bundleAbortWarningText(")
	for _, caller := range []string{
		"engraveSingleSigFlow",     // gui/singlesig.go:177 -- gated by S6a step 5
		"supplyMultisigPolicyFlow", // gui/multisig.go:291   -- gated by S5's I-12
		"buildMultisigPolicyFlow",  // gui/multisig_build.go:402 -- gated by S5's I-12
	} {
		if !strings.Contains(doc, caller) {
			t.Errorf("the justification above bundleAbortWarningText does not name %s, "+
				"which carries a post-engrave tail and gates on bundleEngraveDone:\n%s",
				caller, doc)
		}
	}
	// AND THE COUNT ITSELF, pinned to the corrected sentence rather than to the
	// bare word. A first draft of this row asserted only that the block contained
	// "three" somewhere, and the mutation run showed it PASSING against the old
	// comment: the same block already says "exactly three options" about something
	// else entirely, four paragraphs up. That is the false-PASS this test exists to
	// forbid, reproduced inside the test itself, so the needle is the phrase that
	// carries the claim.
	const countClaim = "all THREE callers that carry a post-engrave tail"
	if !strings.Contains(doc, countClaim) {
		t.Errorf("the justification above bundleAbortWarningText does not state the "+
			"count as %q. \"both\" was wrong by exactly one and nothing in the source "+
			"said so:\n%s", countClaim, doc)
	}
	if strings.Contains(doc, "both engraving callers") {
		t.Errorf("the justification above bundleAbortWarningText still claims \"both "+
			"engraving callers\":\n%s", doc)
	}
}

// guiDocComment returns the contiguous `//` comment block immediately above `decl`
// in a source file of this package.
//
// It exists so a source assertion can be aimed at ONE comment. readGuiFile hands
// back the whole file, and over a 900-line file a positive `Contains` proves only
// that the words appear SOMEWHERE -- which a comment about something else
// satisfies just as well as the one being policed. Same blunt-source-inspection
// idiom as funcBody and readGuiFile; narrower target.
//
// It FATALS rather than returning "" when the declaration is missing or carries no
// doc comment, because both of those states would otherwise let every `Contains`
// below it assert against the empty string and report a pass.
func guiDocComment(t *testing.T, file, decl string) string {
	t.Helper()
	src := readGuiFile(t, file)
	i := strings.Index(src, decl)
	if i < 0 {
		t.Fatalf("%s does not contain %q -- this guard protects nothing", file, decl)
	}
	if strings.Contains(src[i+len(decl):], decl) {
		t.Fatalf("%s contains %q more than once, so this guard cannot tell which "+
			"comment it is reading", file, decl)
	}
	above := strings.Split(src[:i], "\n")
	above = above[:len(above)-1] // drop the partial line `decl` starts on
	var block []string
	for j := len(above) - 1; j >= 0; j-- {
		if !strings.HasPrefix(strings.TrimSpace(above[j]), "//") {
			break
		}
		block = append([]string{above[j]}, block...)
	}
	if len(block) == 0 {
		t.Fatalf("%s: %q has no doc comment, so a Contains assertion on it would be "+
			"a statement about the empty string", file, decl)
	}
	return strings.Join(block, "\n")
}
