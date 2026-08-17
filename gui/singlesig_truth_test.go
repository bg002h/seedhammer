package gui

import (
	"reflect"
	"strings"
	"testing"
	"testing/synctest"

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

// TestRestoreDocStatusPlaceholderCannotStrengthenTheDocument pins WHAT the four
// call sites pass while steps 5 and 7 are still outstanding.
//
// A SIGNATURE CHANGE LANDS AHEAD OF ITS VALUES, so for the length of this step
// every restore document in the tree carries a placeholder -- and a placeholder
// is a claim like any other. The rule it has to satisfy is S6a G2's direction:
// if step 7 never arrived, the document must say LESS than the truth, never
// more. verifyStatusNotFullyCheckedLine is the weakest of the four lines and is
// byte-identical to what the builder renders for a record with neither bit set,
// so wiring the real record in later can only ever strengthen a document that
// earned it.
//
// THE FAILURE THIS GUARDS IS "" AND IT IS NOT A HYPOTHETICAL. An empty status
// still occupies index 0 and still compiles; it renders as SILENCE, and silence
// about a verification is precisely what reads as a pass to a stranger years
// later. That is an omission that STRENGTHENS a claim, and it is the one
// direction the design does not allow a mistake in.
func TestRestoreDocStatusPlaceholderCannotStrengthenTheDocument(t *testing.T) {
	// (a) THE PLACEHOLDER IS THE ZERO CELL, exactly. Not "similar to": step 7
	// replaces these literals with buildVerifyStatusLine(rec), and on a run that
	// recorded nothing that substitution has to be a no-op on the page.
	if got := buildVerifyStatusLine(verifyRecord{}); got != verifyStatusNotFullyCheckedLine {
		t.Errorf("the step-4 placeholder is not what an unrecorded verify renders, so "+
			"wiring the record in at step 7 would silently change every document that "+
			"recorded nothing\n  placeholder %q\n  zero cell   %q",
			verifyStatusNotFullyCheckedLine, got)
	}

	// (b) AND EVERY PRODUCTION CALL SITE PASSES IT. Three flows reach a restore
	// document and all three are wired the same way; a site left on "" is invisible
	// to the compiler and to every rendering test, because an empty label draws
	// nothing and asserting on nothing is what the whole cycle is about.
	//
	// THIS ROW IS EXPECTED TO GO RED AT STEP 7 and must be updated then, not
	// loosened: that is the step where these literals become
	// buildVerifyStatusLine(rec) and where T20 takes over the same duty on a
	// rendered document.
	//
	// THE SEARCH IS OVER CODE, NOT SOURCE, and that is not a refinement: all
	// three call sites carry a comment explaining the placeholder BY NAME, so a
	// site edited to "" would leave its own justification behind and a raw
	// readGuiFile would keep passing over it. Found by running this row's own
	// mutation, which is the only reason it is written this way.
	codeOf := func(t *testing.T, file string) string {
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
	for _, file := range []string{"singlesig.go", "multisig.go", "multisig_build.go"} {
		if src := codeOf(t, file); !strings.Contains(src, "verifyStatusNotFullyCheckedLine") {
			t.Errorf("gui/%s reaches a restore document but names no status placeholder. "+
				"A restore-doc call site whose status is \"\" renders a blank first line, "+
				"and a document silent about its verification is one a stranger reads as "+
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
		click(&ctx.Router, Down) // Skip
		frame()
		click(&ctx.Router, Button3)
		frame()

		if c, ok := pumpUntil(frame, "Descriptor:", 96); !ok {
			t.Fatalf("the restore doc was not shown; got %q", c)
		}
		// THE DOCUMENT IS A PAGER and the inventory is appended at the TAIL, after
		// the descriptor chunks and both addresses, so a single-frame assertion
		// misses every line this step adds.
		run.doc, _ = s5PageForNeedle(t, ctx, frame, "\x00never matches\x00", 24)
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

// TestSeedHandlingRulingMatchesEachPathsCapacity is T7c, and it is the row that
// covers the one thing no compiler and no other test can see: WHICH capacity
// each of the three call sites hands over.
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
func TestSeedHandlingRulingMatchesEachPathsCapacity(t *testing.T) {
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
	t.Run("single-sig", func(t *testing.T) {
		run := s6aSingleSigWalk(t, s6aSingleSigOpts{watchOnly: true})
		t.Logf("the watch-only single-sig run cut %d plate(s)", run.plates)
		check(t, "single-sig", run.doc, oneSubject, manySubject)
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
	t.Run("multisig-supply", func(t *testing.T) {
		_, doc := s5SupplyPassphraseWalk(t)
		check(t, "multisig supply", doc, oneSubject, manySubject)
	})

	// MULTISIG BUILD: the registry, one seed per held slot across distinct
	// masters. Its document must stay byte-identical to the S5-reviewed sentence.
	t.Run("multisig-build", func(t *testing.T) {
		records := cosignerCardRecords(t, 4) // A@0, B@0, C@0, A@1
		synctest.Test(t, func(t *testing.T) {
			e := newEngraver()
			p := newPlatform()
			p.display = sh2DisplaySize
			p.engraver = e
			ctx := NewContext(p)
			ctx.sysw = sessionHolding(records...)
			done := false
			// THE RASTER HARNESS, for s5EngraveEveryPlate's measured reason: the
			// frames a plate takes depend on which harness pumps them.
			frame, _, _, quit := runUITouchRaster(ctx, func() {
				buildMultisigPolicyFlow(ctx, &descriptorTheme)
				done = true
			})
			defer quit()

			s5DriveBuildToEngravePicker(t, ctx, frame)
			t.Logf("the build run cut %d plate(s)", s5EngraveEveryPlate(t, ctx, frame, e))

			if c, ok := pumpUntil(frame, "Verify the engraved plates?", 96); !ok {
				t.Fatalf("the verify offer was not reached after the engrave; got %q", c)
			}
			click(&ctx.Router, Down) // Skip
			frame()
			click(&ctx.Router, Button3)
			frame()
			if c, ok := pumpUntil(frame, "Descriptor:", 128); !ok {
				t.Fatalf("the restore doc was not shown; got %q", c)
			}
			doc, _ := s5PageForNeedle(t, ctx, frame, "\x00never matches\x00", 32)
			check(t, "multisig build", doc, manySubject, oneSubject)
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
	})
}
