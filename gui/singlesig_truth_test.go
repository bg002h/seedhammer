package gui

import (
	"reflect"
	"strings"
	"testing"
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
