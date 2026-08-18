package gui

import (
	"fmt"
	"strings"
)

// ─── S6a: what the restore document says about the verification ──────────────
//
// THE DOCUMENT MUST NEVER VOUCH FOR PLATES THE DEVICE HAS EVIDENCE AGAINST, AND
// MUST NEVER CLAIM A CHECK IT DID NOT PERFORM (S6a G2). Before this the restore
// document said nothing at all about the verify, so a set whose read-back check
// had failed, and a set whose check was skipped, and a set that passed cleanly
// all produced the same page -- and silence reads as a pass to the stranger who
// finds the plates years later.
//
// REPORTING THE EPISTEMIC STATUS OF THE VERIFICATION IS AN EXPLICIT NON-GOAL
// (S6a NG1). There is no knowledge lattice here, no observation enum, no
// reserved status and no world-set table to keep complete. G2 is a PROHIBITION
// -- "never claim more than you know" -- and a prohibition is discharged with
// one conservative default and NO enumeration. The obligation form, "always say
// exactly what you know", needs a complete and correct partition of everything
// the device can observe, and is what cost the design nine review rounds. A
// finding that would have this file report more about what the device knew is
// out of scope by default; it is filed, not folded.

// verifyStatus is the 2x2 of TWO RECORDED BOOLEANS: was a pass recorded, and was
// anything adverse recorded.
//
// statusNotFullyChecked IS THE ZERO VALUE, and that is load-bearing rather than
// alphabetical. A status nothing ever assigned is already the conservative one,
// so an omission upstream cannot strengthen a claim.
type verifyStatus int

const (
	// statusNotFullyChecked is the ZERO CELL: neither bit recorded. A skip, a
	// benign refusal, an abort, an incomplete gather, a path nobody enumerated,
	// a path added next year. Safe, and true.
	statusNotFullyChecked verifyStatus = iota
	// statusCheckDidNotPass: something adverse was recorded and no pass was.
	statusCheckDidNotPass
	// statusVerifiedOnRetry: something adverse was recorded AND a later full
	// pass was, within one run.
	statusVerifiedOnRetry
	// statusVerified: a pass was recorded and nothing adverse was.
	statusVerified
)

// passRecord is written AT THE SUCCESS RETURN of a verify flow, where the facts
// it holds are in scope, and it is the ONLY thing the pass line is generated
// from. A claim with no field here is not constructible.
//
// A BOOL CANNOT CARRY A MODE. An earlier design declared a bare `fullPass bool`
// while describing it as recording "which comparisons ran and matched in this
// mode" -- one bit asked to hold two facts -- and that is how a mode-blind pass
// line came to claim "the ms1 you typed matched this seed" on a watch-only run
// where no ms1 is engraved, typed or compared. multisigVerifyOKMessage
// (gui/multisig_verify.go) had already found and fixed that bug in all four of
// its arms; the record exists so the document cannot re-commit it.
type passRecord struct {
	// full is the MODE, captured AT the success return where it is the flow's
	// own parameter. It is RECORDED, never inferred from a verdict: a verdict is
	// the proxy two review rounds proved unsound, and it is not read here at all.
	full bool
	// legs is how many operator key plates were read back and compared.
	legs int
	// suppliedCosigners is THE PATH AXIS. Without it, a single-sig full verify
	// and a one-slot multisig full verify both record {true, 1} -- and their
	// truthful lines DIFFER: every multisig notice ends "Other cosigners' keys
	// are taken as supplied" (gui/multisig_verify.go) and single-sig's
	// deliberately does not, a split TestMultisigVerifyNoticeIsHonest pins.
	// Carrying that clause onto a single-sig document misdescribes the wallet;
	// dropping it from a multisig document is an unscoped claim.
	//
	// A COUNT, NOT A FLOW ENUM, because the line is generated from a RECORDED
	// FACT rather than from an identifier the builder must interpret: the clause
	// renders iff suppliedCosigners > 0, and single-sig records 0 because it has
	// no policy cosigners. Nothing infers the path; the path is what was counted.
	suppliedCosigners int
}

// verifyRecord is the seam that carries the two recorded facts out of a verify
// flow and into the restore document. The flow's VERDICT return is not that
// thing -- a verdict is not read here, by design -- so flows take a
// *verifyRecord out-parameter and keep their verdict return unchanged.
type verifyRecord struct {
	// pass is nil until the success return writes it.
	pass *passRecord
	// adverse is STICKY. It is written at any return site whose world contains a
	// bad-plate world, and nothing clears it: a clean retry after a failed check
	// therefore reports statusVerifiedOnRetry rather than a bare pass.
	adverse bool
}

// verifyStatusFor is the cell, not a search.
//
// MONOTONICITY IS STRUCTURAL HERE, NOT PROMISED. The `default:` arm is the zero
// cell, so a fact that was not recorded cannot set a bit, and an unset bit can
// only move the cell TOWARD statusNotFullyChecked. There is no arm an omission
// can strengthen, which is precisely what an earlier design got backwards.
//
// IT READS NO VERDICT. Neither a flow's result type nor any of its result
// constants appears in this function -- deliberately not even in this comment,
// so a source assertion on the derivation can search for their names without
// matching prose about them. A verdict is a summary of a flow rather than a
// record of what it observed, and two rounds of review proved it unsound as a
// proxy.
func verifyStatusFor(rec verifyRecord) verifyStatus {
	switch {
	case rec.pass != nil && rec.adverse:
		return statusVerifiedOnRetry
	case rec.pass != nil:
		return statusVerified
	case rec.adverse:
		return statusCheckDidNotPass
	default:
		return statusNotFullyChecked
	}
}

// The two literal lines and the three generated clauses. Named constants rather
// than inline text so the document's wording has one home, and so a test can
// pin it without re-typing it.
//
// NO EM-DASH AND NO SMART QUOTES anywhere below: they are zero-pixel glyphs in
// the body face and a line carrying one does not draw at all (F-78/F-151), on a
// page whose entire job is to say what the backup does and does not contain.
const (
	// verifyStatusNotFullyCheckedLine is the zero cell's line. It asks the
	// reader to confirm before relying, and deliberately does NOT cry wolf: the
	// modal case reaching it is an operator who skipped the verify, whose backup
	// is probably fine.
	//
	// IT NAMES NO ARTIFACT ON THE PAGE, and that is load-bearing. An earlier
	// draft said "(master fingerprint below)" -- true of the single-sig document
	// (gui/singlesig_restore.go:107 prints "Master fp: %08x") and FALSE of both
	// multisig documents, which carry a policy, a descriptor and addresses but no
	// fingerprint. This one constant renders on all three, so any artifact it
	// names must exist on all three. Whole-diff review I-1.
	verifyStatusNotFullyCheckedLine = "These plates were not fully checked. " +
		"Confirm they restore this wallet before relying on this backup."
	// verifyStatusDidNotPassLine says a check RAN and did not pass. It does not
	// say which check or how, and that is deliberate: the device could not
	// reliably earn the stronger claim, and the stranger holding the steel can
	// only rely, confirm, or re-cut. Diagnosis has a reader -- the operator at
	// the machine, at verify time -- and the fork's screens already serve them.
	verifyStatusDidNotPassLine = "A verification check ran and did not pass: " +
		"a comparison did not match, or a plate could not be read or accounted " +
		"for. Do NOT rely on this backup until a full check passes. Check again " +
		"with every plate this run engraved; if this repeats, engrave a fresh set."
	// verifyStatusRetryClause is appended to the generated pass line when
	// something adverse was also recorded in the same run.
	verifyStatusRetryClause = "An earlier check did not pass; a later full check passed."
	// verifyStatusMS1Clause is entitled by passRecord.full, and by nothing else.
	//
	// F-206 (S6b spec §3.3): COUNT-FREE, DELIBERATELY. The filed remedy proposed
	// pluralising over passRecord.legs, and that is unsound: legs is len(legs),
	// one leg per FILLED SLOT, not one per seed -- one seed fills several slots
	// of a policy that puts it at several accounts. Singular text at 2 legs from
	// ONE typed seed would already be wrong; pluralising it would be wrong the
	// OTHER way at 2 legs from TWO seeds, where the operator typed one ms1 per
	// seed, not "ms1 secrets" plural in the sense that phrase implies. The
	// record cannot distinguish "1 seed filling 2 legs" from "2 seeds filling 2
	// legs" (passRecord carries only {full, legs, suppliedCosigners}), so no
	// wording keyed on legs can be honest at every count. This clause is true at
	// ANY seed count and requires none -- the same shape the device's own
	// success notice already uses (multisigVerifyOKMessage, "the ms1 you typed
	// for each seed").
	verifyStatusMS1Clause = "The ms1 you typed for each seed matched."
	// verifyStatusNoMS1Clause is the same field's other arm, and it is the
	// "states what was not read" half of a watch-only pass line. Omission must
	// WEAKEN a claim, so watch-only says out loud that no secret was compared
	// rather than merely leaving the ms1 clause off.
	verifyStatusNoMS1Clause = "No secret seed share was read back or compared."
	// verifyStatusCosignerClause is entitled by suppliedCosigners > 0.
	verifyStatusCosignerClause = "Other cosigners' keys are taken as supplied."
	// verifyStatusScopeLine SCOPES THE PAGE BENEATH IT, and it renders under
	// statusCheckDidNotPass alone. Everything below the status line -- the
	// descriptor, the addresses, the plate inventory -- describes what this run
	// INTENDED to cut, and nothing on the page is conditioned on the check having
	// passed. A reader who takes the wallet details as confirmed by the document
	// they are printed on has read it exactly as it was laid out.
	//
	// IT IS NOT WIDENED TO statusNotFullyChecked. That cell's own line already
	// says "confirm before relying", and its modal occupant is an operator who
	// skipped the verify on a backup that is probably fine; a second warning there
	// cries wolf, and an operator who learns to tap past one modal taps past the
	// next.
	verifyStatusScopeLine = "Everything below describes what this run INTENDED to " +
		"engrave. Until the check above is resolved, do not assume the plates match it."
)

// verifyStatusScopeLines returns the scoping line that belongs immediately after
// `status` on a restore document, or nothing.
//
// IT IS DERIVED INSIDE THE DOCUMENT RATHER THAN PASSED IN, and that is the whole
// reason it is a function. THREE production flows reach a restore document, and
// "wired into two of the three and forgotten on the third" is not a hypothetical
// here -- it is the exact shape that let this cycle's Critical ship on the
// multisig side while the single-sig side went without. A parameter can be
// omitted at a call site and the compiler is happy; a line the document builds
// for itself cannot be.
//
// THE TEST IS IDENTITY, NOT INFERENCE. verifyStatusDidNotPassLine is the one
// string buildVerifyStatusLine emits for statusCheckDidNotPass and it is a
// package constant, so the two cannot drift apart: rewording the status reworks
// this comparison in the same edit. Nothing here reads a verdict, and nothing
// here re-derives a cell.
func verifyStatusScopeLines(status string) []string {
	if status != verifyStatusDidNotPassLine {
		return nil
	}
	return []string{verifyStatusScopeLine}
}

// buildVerifyPassLine generates the pass line FROM THE RECORD. It is not a
// literal, and that is the whole point: it names exactly the comparisons this
// mode ran and states what was not read.
//
// EVERY CLAUSE IS NAMED BY A RECORDED OBSERVATION, and a clause with none is
// deleted rather than shipped. That is why there is no descriptor-plate clause
// here even though the read-back md1 is compared: the record carries no field
// that says so, and a claim the record cannot back does not belong on a
// document that outlives everybody who could correct it.
func buildVerifyPassLine(p passRecord) string {
	// plateWord already renders a count with a singular or plural tail, so a
	// pass line never reads "1 key plates"; the verb has to agree with it.
	verb := "were"
	if p.legs == 1 {
		verb = "was"
	}
	clauses := []string{
		fmt.Sprintf("%s %s read back and matched what this run engraved.",
			plateWord(p.legs, "key plate", "key plates"), verb),
	}
	if p.full {
		clauses = append(clauses, verifyStatusMS1Clause)
	} else {
		clauses = append(clauses, verifyStatusNoMS1Clause)
	}
	if p.suppliedCosigners > 0 {
		clauses = append(clauses, verifyStatusCosignerClause)
	}
	return strings.Join(clauses, " ")
}

// buildVerifyStatusLine renders the restore document's verification status.
//
// IT TAKES THE RECORD AND DERIVES THE CELL ITSELF. Nothing upstream computes a
// status and hands it over, because a verifyStatus has already lost the mode,
// and the mode is exactly what the pass line must not lose.
//
// IT RETURNS A STRING, NOT A []string. A slice's zero value is the empty slice,
// which renders as SILENCE, and silence is the one thing this line exists to
// stop being mistakable for a pass. A string cannot be absent.
//
// Exactly one line. The scoping line that follows statusCheckDidNotPass on the
// page is the document's job, not this function's.
func buildVerifyStatusLine(rec verifyRecord) string {
	status := verifyStatusFor(rec)

	// A PASS LINE IS NOT CONSTRUCTIBLE WITHOUT A PASS RECORD, and this guard is
	// what makes that structural rather than a property of the switch above.
	// Today the two pass cells are reachable only when rec.pass is non-nil, so
	// this arm is unreachable -- but the version of that switch somebody writes
	// next year is not covered by today's reasoning, and the failure it would
	// otherwise produce is a nil dereference: a CRASH where the design calls for
	// the weakest line. Found by executing this row's own mutation, which is the
	// only reason it is here rather than in a follow-up.
	if (status == statusVerified || status == statusVerifiedOnRetry) && rec.pass == nil {
		return verifyStatusNotFullyCheckedLine
	}

	switch status {
	case statusVerified:
		return buildVerifyPassLine(*rec.pass)
	case statusVerifiedOnRetry:
		return buildVerifyPassLine(*rec.pass) + " " + verifyStatusRetryClause
	case statusCheckDidNotPass:
		return verifyStatusDidNotPassLine
	default:
		// THE ZERO CELL, and it is the default here for the same reason it is the
		// default in verifyStatusFor: a status this function does not recognise
		// must render the weakest line, never a stronger one.
		return verifyStatusNotFullyCheckedLine
	}
}
