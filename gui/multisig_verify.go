package gui

import (
	"errors"
	"fmt"
	"slices"

	"github.com/btcsuite/btcd/chaincfg/v2"
	"seedhammer.com/bip39"
	"seedhammer.com/bundle"
	"seedhammer.com/codex32"
	"seedhammer.com/md"
	"seedhammer.com/mk"
)

// Multisig verify success copy (L2). HONEST scoping: on an air-gapped device the
// only cross-checkable facts are the operator's own key card (mk1, H1) + xpub/
// origin (findUserSlot) + the secret (ms1 entropy/language, M1). The md1 policy
// string is the supplied input compared to a clone of itself, and foreign
// cosigners' xpubs have no source of truth — so we do NOT claim a full-bundle
// guarantee.
//
// STILL TRUE OF THE COMPARATOR, NO LONGER THE WHOLE STORY OF THE FLOW.
// bundle.Verify's md1 leg does compare the readback against a clone of itself.
// What changed is upstream: multisigVerifyFlow now requires the readback md1 to
// EQUAL the md1 THIS RUN ENGRAVED before anything reaches the comparator, so
// "which wallet is this" is answered outside that self-comparison. The success
// copy is unchanged, because what it scopes — foreign cosigners' xpubs having no
// source of truth — is untouched by that.
const (
	multisigVerifyOKTitle = "Verify OK"
	multisigVerifyOKBody  = "Operator key and secret verified. Other cosigners' keys are taken as supplied."
	// multisigVerifyNoExpectationBody is what an empty engraved-slot set says. ONE
	// STRING, TWO SITES (the entry guard and the derive loop's refusal), so the two
	// cannot drift into telling the operator two different things about one state.
	multisigVerifyNoExpectationBody = "This run engraved no key plate, so there is nothing to verify."
	// multisigVerifyNoPolicyBody is what a caller that lost the ENGRAVED md1 says.
	// It is a different state from an empty slot set and gets its own words: the
	// run cut plates, and the one fact that says WHICH WALLET they belong to did
	// not arrive, so nothing can be proved about them.
	multisigVerifyNoPolicyBody = "This run's wallet policy did not reach the verify, so there is nothing to check the plates against."
	// multisigVerifyForeignPolicyBody is the POLICY-IDENTITY refusal.
	//
	// It says the one thing the operator can act on. A generic "Verify Failed"
	// sends them to re-cut plates that are perfectly good; what actually happened
	// is that the steel in their hands belongs to a different wallet, and no
	// amount of re-presenting THESE plates will ever satisfy this run.
	//
	// NO EM-DASH: it is a zero-pixel glyph in the body face and a line carrying one
	// does not draw at all (F-78/F-151).
	multisigVerifyForeignPolicyBody = "The read-back wallet policy is NOT the wallet policy this run engraved. " +
		"These plates belong to a different wallet. Present the md1 and the key plates this run cut."
	// multisigVerifyRetryLead is the RE-offer all THREE engraving callers make
	// (S6b P9, F2, added engraveSingleSigFlow to the two multisig callers below)
	// after a verify that ended short of a clean pass. ONE STRING, THREE SITES,
	// so no caller can drift into describing one state a different way -- and so
	// it stays a single production site for a walk needle.
	//
	// It is the mechanism the "Verify Incomplete" screen's instruction refers to.
	// Before it, that instruction named nothing: the verify was offered once and
	// the device has no standalone bundle verify anywhere in its program table.
	// The single-sig side had the identical gap (F2): a FAILED comparison or an
	// unreadable readback dead-ended with no route back except re-cutting the
	// whole set, and this text is generic enough ("plate", not "multisig") that
	// reusing it rather than forking a near-duplicate is the smaller diff.
	multisigVerifyRetryLead = "Not every plate is verified. Try again?"
)

// multisigVerifyResult is what multisigVerifyFlow tells its caller, and it
// exists so the incomplete screen's instruction can be TRUE.
//
// The flow returned void, so neither caller could tell a clean pass from an
// incomplete or a failed one, and the screen after every outcome was the same:
// the verify offer fell through to the restore document. That made "Run verify
// again with the remaining seeds" a prescription with no implementation --
// multisigVerifyFlow's only callers are one-shot post-engrave offers (a bare
// `if sel == 0 { ... }`), gui/plate_verify.go states outright that the bundle
// verifies are untouched by the word-plate menu, and the program dispatch table
// carries no standalone bundle verify. The operator's only route was to re-run
// the whole engrave and cut every plate again over hours; the predictable
// response is to fund the wallet anyway.
//
// FIVE OUTCOMES, NOT A BOOL, because the callers owe three different things.
// Only verifyComplete may fall through to the restore document. Incomplete and
// Failed must be RE-OFFERED, since both are states an operator can act on with
// the plates in front of them. Refused and Abandoned must not be: a structural
// refusal (no obligation, no policy) says nothing the operator can fix by trying
// again with the SAME inputs, and an abandon is them choosing to stop.
//
// AN UNREADABLE READBACK IS NOT IN THAT LIST (F-199, S6b spec §3.1), even
// though it too is a refusal before any comparison ran. Presenting the plates
// again is exactly the retry a refusal-class outcome is defined to forbid, so
// that site returns verifyIncomplete instead -- correctly, even though no
// comparison ran there: verifyIncomplete's obligation is "the operator can still
// fix this with what is in their hand", not "a comparison that matched". Of the
// FOUR return sites that used to share verifyRefused, only two remain
// programmer-error refusals (no obligation, no policy) plus one defensive
// re-check of the first that S6b's spec proves unreachable in-process
// (verifyFreshSlots' only error path re-tests len(expectedSlots)==0, and
// expectedSlots is never reassigned in this function) -- see the source
// assertion next to it.
//
// THE HEADLINE SAID "FOUR" UNTIL S6a STEP 8, while the paragraph under it named
// all five and the const block below declares all five. Three obligations over
// five outcomes is the whole point of the type, so the two numbers were never the
// same number -- and S6a round 0 repeated the "four" straight out of this
// headline, which is what a miscount in a comment costs.
type multisigVerifyResult int

const (
	// verifyComplete is the only clean pass: every expected slot got a leg, every
	// leg found its plate, and the bijection closed.
	verifyComplete multisigVerifyResult = iota
	// verifyIncomplete is a verify the operator can still act on: EITHER a
	// comparison ran and MATCHED with slots left unproved, OR (F-199, S6b §3.1)
	// the readback itself could not be gathered into a comparison at all --
	// re-presenting the plates might succeed where the extraction did not.
	verifyIncomplete
	// verifyFailed is a comparison that ran and disagreed.
	verifyFailed
	// verifyRefused is a structural refusal before any comparison could run.
	verifyRefused
	// verifyAbandoned is the operator leaving before any leg existed.
	verifyAbandoned
)

// ─── F-191: a keystroke must not be reported as a wrong wallet ───────────────

// multisigVerifySeedIsInnocent reports whether a seed that matched NO slot with
// the passphrase the operator typed matches one WITHOUT it.
//
// It is the cheapest question that can clear a seed outright. The engrave accepts
// a payload-borne passphrase; the verify requires it RE-TYPED (§7.4, so the
// engrave source is never compared against itself), and one wrong character there
// derives an entirely different wallet. That produced the flow's most alarming
// screen -- "That seed is not a cosigner" -- on plates that were perfectly good
// and a seed that was perfectly right.
//
// A passphrase that was never offered is not re-derived: the empty derivation is
// the one that just failed, and running it again to "clear" the seed would route
// a genuinely foreign seed to the reassuring arm.
//
// Costs one re-derivation, on a path that has already failed and is about to draw
// a modal.
func multisigVerifySeedIsInnocent(m bip39.Mnemonic, passphrase string, net *chaincfg.Params, keys []md.ExpandedKey) bool {
	if passphrase == "" {
		return false
	}
	return len(allUserSlots(m, "", net, keys)) > 0
}

// multisigVerifyNoSlotBody is what the operator reads when their seed filled no
// slot of the read-back policy. THREE STATES, because the device is in three
// genuinely different epistemic positions and the shipped text asserted the
// strongest conclusion in all of them:
//
//	"That seed is not a cosigner of the read-back policy, so it cannot prove any
//	 of these plates."
//
// That sentence is only true when a passphrase cannot be the cause, and it never
// cannot. What it actually did was tell an operator holding the right seed and
// good steel that their money lives in a wallet they are not part of -- over one
// keystroke.
//
//   - PROVED INNOCENT: a passphrase was typed and the seed fills a slot without
//     it. The device can say outright that the seed IS a cosigner, which turns
//     the scariest screen it draws into a typo report -- and tells the operator
//     this is NOT a passphrase-protected wallet, per R-M (S6b spec §3.2a,
//     REQUIREMENTS §2bis): the operator struck the original's "skip the
//     passphrase" advice (it reads as a procedural workaround and buries the
//     finding) and ruled that the fact itself -- not a skip instruction -- is
//     what resolves their confusion. "A passphrase will be necessary to use the
//     key" is FORBIDDEN in this arm: it fires precisely because the plates
//     match the seed with NO passphrase, so no passphrase is needed to use it.
//   - PASSPHRASE TYPED, still nothing: it cannot tell a wrong seed from a wrong
//     passphrase, and says so rather than picking the frightening one.
//   - NO PASSPHRASE TYPED: still not certain. The wallet may have been built with
//     one and the operator pressed Skip, so the action offered is "add it and try
//     again" rather than "distrust your plates".
//
// None of the three re-cuts steel, and none of them says the seed is foreign as a
// fact. NO EM-DASH: a zero-pixel glyph in this face takes its whole line with it.
func multisigVerifyNoSlotBody(passphraseTyped, provedInnocent bool) string {
	switch {
	case provedInnocent:
		// R-M's ADOPTED WORDING, verbatim (REQUIREMENTS §2bis, operator
		// 2026-08-17: "b wording you suggested is great"). Pre-measured: 251
		// characters, every rune drawable, NO EM DASH -- gui/multisig_build.go
		// once shipped an em dash in a modal body that meant the body did not
		// draw AT ALL, so this is a hard constraint, not a style note. Do not
		// reword it; GATE 3.2a in the test suite pins it against the class check
		// and against the forbidden "necessary to use the key" claim.
		return "These plates match this seed with NO passphrase. This is not a " +
			"passphrase-protected wallet. If you meant to use one, these plates are not " +
			"that wallet: try the password again. If you continue without a passphrase, " +
			"these plates are complete as they are."
	case passphraseTyped:
		return "No slot matches that seed with the passphrase you typed. Check the " +
			"passphrase before you doubt the plates: one wrong character derives a " +
			"different wallet."
	default:
		return "No slot matches that seed. If this wallet was built with a BIP-39 " +
			"passphrase, add it and try again: without it the same words derive a " +
			"different wallet."
	}
}

// ─── T6b: verify-bundle for a SUPPLIED multisig bundle (user's slot only) ────
//
// verifyMultisig assembles the read-back bundle and runs the deterministic
// comparator against the freshly re-derived operator leg (mirror verifySingleSig,
// gui/singlesig_verify.go:49). It verifies ONLY the operator's slot (I-5); the
// other cosigner slots are public-given and unverified-by-design (bundle.Verify
// never inspects them).
//
// The ms1 leg follows bundle.Verify's native presence semantics (verify.go:71-79):
// a watch-only verify passes "" for BOTH the derived bundle's MS1 (the leg was
// re-derived with full=false) AND ms1Readback → both empty → the ms1 leg is
// SKIPPED. A full verify carries an ms1 on both sides → the recovered entropy is
// compared. An ms1 present on exactly one side is a PRESENCE MISMATCH and errors
// (we deliberately do NOT mask it by zeroing the derived MS1 — that would let a
// full bundle silently pass an empty readback). Returns the comparator's first
// diverging-field error, or nil on PASS.
func verifyMultisig(derived bundle.Bundle, ms1Readback string, mk1, md1 []string) error {
	readback := bundle.Bundle{MS1: ms1Readback, MK1: mk1, MD1: md1}
	return bundle.Verify(derived, readback)
}

// ─── S5: the verify covers EVERY leg ─────────────────────────────────────────
//
// Before S5 this flow took ONE bundle and recovered the operator's origin
// through findUserSlot, which returns the FIRST slot a seed matches -- so it
// structurally could not verify legs 2..n. Trace B engraves three key plates;
// the shipped verify read back one, compared one, and showed "Verify OK".
//
// A verify that checks one of three plates and reports success is a FALSE GREEN
// on the operator's ONLY readback of what the machine actually cut. It is
// treated as such: the comparator below is a BIJECTION, and both directions are
// funds-bearing.
//
//   - every re-derived leg must find its plate. A leg with no plate is a slot
//     the operator cannot prove membership of, and it is exactly the shipped
//     defect -- check what was shown, call the rest verified.
//   - every plate must be claimed by a leg. An unclaimed plate is steel in the
//     operator's hands that this policy has no slot for, and "Verify OK" over it
//     tells them it belongs to this wallet.

// verifyLeg is ONE re-derived leg plus the ms1 the operator hand-typed for ITS
// seed.
//
// The ms1 travels WITH the leg rather than being a flow-global, for SPEC 4.1's
// reason at the other end of the flow: a build across two masters engraves two
// seed plates, and comparing either leg against whichever ms1 the flow happened
// to hold is how a "Full" backup carrying master A twice verifies clean while
// master B -- which k=3 needs -- is gone.
type verifyLeg struct {
	// Slot is the policy slot this leg proves. Operator-facing: it is what the
	// failure names, so "which plate do I re-cut" has an answer.
	Slot int
	// B is the leg as freshly RE-DERIVED from a re-typed seed, never the leg the
	// engrave produced (§7.4: a readback taken from the session compares the
	// engrave source against itself and passes unconditionally).
	B bundle.Bundle
	// MS1Readback is the ms1 the operator typed for this leg's seed, or "" in a
	// watch-only verify.
	MS1Readback string
}

// errVerifyLegHasNoPlate is a leg the readback does not account for: no plate
// carries this slot's key. It names the SLOT, because that is the only thing the
// operator can act on.
//
// THIS RULE IS LOAD-BEARING AND MUST NOT BE RELAXED. A review proposed dropping
// it, on the reasoning that the unclaimed-plate sweep already carries coverage.
// It does not: with three legs and two plates, both plates get claimed, the
// third leg is skipped, the sweep is satisfied and a readback MISSING A PLATE
// reports Verify OK -- "check what was shown, call the rest verified", which is
// the shipped defect this whole rewrite exists to remove.
// TestVerifyCoversEveryLeg/"a leg with no plate at all FAILS" holds the line.
//
// The real defect that proposal was aimed at is fixed where it belongs: in
// multisigVerifyFlow's derive loop, which collects a leg only for a slot the
// ENGRAVE actually cut a plate for (verifyFreshSlots), so a seed filling slots
// this run did not engrave stops manufacturing legs no plate exists for.
type errVerifyLegHasNoPlate struct{ Slot int }

func (e errVerifyLegHasNoPlate) Error() string {
	return fmt.Sprintf("verify: no read-back key plate carries slot @%d's key", e.Slot)
}

// errVerifyPlateUnclaimed is a read-back plate no leg claims.
//
// It deliberately does NOT name a slot: there isn't one. Saying "@N" here would
// be inventing an attribution for a plate whose whole problem is that it belongs
// to nothing, and the per-leg failure above is the message that has a slot to
// name.
type errVerifyPlateUnclaimed struct{ Plate int }

func (e errVerifyPlateUnclaimed) Error() string {
	return fmt.Sprintf("verify: read-back key plate %d belongs to no leg of this policy",
		e.Plate+1)
}

// errVerifyNoLegs is a verify with nothing to compare. It is an ERROR and not a
// vacuous pass: a comparator whose empty case returns nil reports success for a
// readback it never looked at, which is the single most expensive false GREEN
// this flow can produce.
var errVerifyNoLegs = errors.New("verify: no leg was re-derived, so nothing was checked")

// errVerifyNoExpectedSlots is a verify whose caller named no engraved slot.
//
// It is an ERROR for errVerifyNoLegs' reason one level down: an empty obligation
// list is satisfied by every readback, so a verify that accepted one would put
// "Verify OK" on screen over a comparison that never ran. Both call sites carry
// at least one slot today (errBuildNoHeldSlot refuses a build that holds none;
// the supply path refuses a seed matching no slot, and otherwise passes every
// slot it cut a plate for), so this fires only when a future caller loses the
// expectation on the way in -- which is exactly the case where a vacuous pass
// would be invisible.
var errVerifyNoExpectedSlots = errors.New("verify: the engrave named no slot, so this readback has nothing to prove")

// verifyFreshSlots is the derive loop's obligation rule: the slots THIS seed
// still owes a proof for.
//
// It is `expected` INTERSECT `filled`, minus what an earlier seed already
// covered, and the intersection is the fix this file exists for.
//
//   - `expected` is what the ENGRAVER cut a plate for, passed in by the caller.
//     Only the engraver knows it: the BUILD path cuts one plate per HELD slot
//     (buildEngraveTail returns those indices) and the SUPPLY path one per
//     MATCHED slot (supplyEngraveTail returns those).
//   - `filled` is allUserSlots -- every slot the seed accounts for AT THAT
//     SLOT'S OWN ORIGIN. It is a different question with a different answer: one
//     seed fills several slots of a policy that puts it at several accounts, and
//     those slots carry DIFFERENT keys.
//
// Deriving a leg per FILLED slot is what made the supply path's one-plate
// engrave verify as "Verify Failed: no read-back key plate carries slot @1's
// key" -- a manufactured leg for a plate the flow had just announced it was not
// cutting, and an honest operator told their good plates are bad.
//
// F-188 REMOVED THAT PARTICULAR DISAGREEMENT AT ITS SOURCE (the supply path now
// cuts a plate for every matched slot, so `expected` and `filled` coincide for a
// supply run's own seed) AND THE INTERSECTION IS STILL LOAD-BEARING. On the
// BUILD path `filled` can still exceed what was engraved: a payload cosigner
// card carrying a DIFFERENT key derived from the same seed at another origin is
// not a duplicate -- duplicateSlotPair refuses only IDENTICAL keys -- and is
// admitted, so the seed accounts for a slot the operator never claimed. It also
// still separates a seed that is a cosigner of the read-back policy but whose
// slots this run did not engrave, which is the third message in the derive
// loop's refusal switch.
//
// NOTHING IS RELAXED HERE. The legs this returns are still re-derived from a
// RE-TYPED seed (§7.4) and still have to find their own plate in
// verifyMultisigLegs' bijection; a plate that was not read back still FAILS,
// naming its slot. What changed is which slots are on the list, not what the
// list has to prove.
func verifyFreshSlots(expected, filled []int, covered map[int]bool) ([]int, error) {
	if len(expected) == 0 {
		return nil, errVerifyNoExpectedSlots
	}
	fresh := make([]int, 0, len(filled))
	for _, s := range filled {
		if covered[s] || !slices.Contains(expected, s) {
			continue
		}
		fresh = append(fresh, s)
	}
	return fresh, nil
}

// verifyMultisigLegs compares EVERY leg against the read-back plate set, and
// requires the pairing to be a bijection.
//
// PAIRING IS BY THE mk1's XPUB, and the choice is load-bearing in both
// directions:
//
//   - it must be UNIQUE, or two legs race for one plate. The origin PATH is not
//     unique across masters -- Trace B's @0 (master A) and @2 (master B) both
//     declare m/48h/0h/0h/2h -- so a path-keyed pairing would hand master B's
//     plate to master A's leg and report a mismatch on an honest readback.
//   - it must leave the comparison with real work to do, or "the plate that
//     matches" is a tautology. It does: bundle.Verify still checks the master
//     fingerprint, the ORIGIN PATH, the md1 exact string, the mk1<->md1 stub
//     binding on BOTH sides, and the ms1's recovered entropy and wordlist. A
//     plate carrying the right key at a lying origin pairs here and fails there.
//
// A plate that does not decode pairs with nothing, so it surfaces as its leg's
// missing plate and then as an unclaimed plate -- never as a silent skip.
func verifyMultisigLegs(legs []verifyLeg, mk1s [][]string, md1 []string) error {
	claimed, err := verifyMultisigLegsPartial(legs, mk1s, md1)
	if err != nil {
		return err
	}
	for i, c := range claimed {
		if !c {
			return errVerifyPlateUnclaimed{Plate: i}
		}
	}
	return nil
}

// verifyMultisigLegsPartial is the FORWARD half of that bijection: every leg
// must find its own plate and that plate must survive the full comparator. It
// returns which plates were claimed, so the caller can decide whether the
// reverse sweep is owed.
//
// IT EXISTS BECAUSE A PARTIAL VERIFY STILL HAS TO COMPARE SOMETHING. Before
// this, a run the operator stopped early returned from the flow at the
// "Verify Incomplete" screen WITHOUT ever calling the comparator -- the only
// function in this file that touches a read-back plate at all -- and the screen
// then reported `len(legs)` as a count of "key plates checked". That number is a
// count of slots RE-DERIVED FROM A TYPED SEED. Every upstream gate on that path
// is cardinality or provenance (the md1 equality, the mk1 count) and none of
// them is plate IDENTITY, so a foreign mk1 among the presented plates was
// reported to the operator as verified. On a 3-of-4 that is a plate the wallet
// needs, believed good.
//
// THE REVERSE SWEEP IS THE PART A PARTIAL RUN CANNOT OWE, and only that part. An
// unclaimed plate means steel this policy has no leg for -- but a run that
// stopped after two of three seeds has legs it never built, so the plates for
// them are legitimately unclaimed and refusing over that would report a FAILURE
// for an operator who did nothing wrong. Everything else is unchanged: each leg
// still has to find its plate, and that plate still has to pass the fingerprint,
// origin, md1 and stub-binding checks in bundle.Verify.
func verifyMultisigLegsPartial(legs []verifyLeg, mk1s [][]string, md1 []string) ([]bool, error) {
	if len(legs) == 0 {
		return nil, errVerifyNoLegs
	}
	claimed := make([]bool, len(mk1s))
	for _, l := range legs {
		idx, ok := verifyClaimPlate(l.B.MK1, mk1s, claimed)
		if !ok {
			return nil, errVerifyLegHasNoPlate{Slot: l.Slot}
		}
		claimed[idx] = true
		if err := verifyMultisig(l.B, l.MS1Readback, mk1s[idx], md1); err != nil {
			return nil, fmt.Errorf("slot @%d: %w", l.Slot, err)
		}
	}
	return claimed, nil
}

// multisigVerifyFailureText turns the comparator's diagnosis into the sentence
// the operator reads, instead of throwing it away.
//
// The shipped screen bound `err` for a nil check and discarded it, so an ms1
// typo, an ms1 presence mismatch and a missing plate for slot @2 all collapsed
// into "The read-back bundle does NOT match the seed. Check the engraved
// plates." -- which tells an operator with perfect steel and one mistyped
// character to distrust the steel. The string dates from an era when exactly ONE
// plate existed and "the engraved plates" was unambiguous; S5 makes a run cut 3
// to 9.
//
// The two structural errors carry the one thing the operator can act on and are
// named: errVerifyLegHasNoPlate knows WHICH slot has no plate (that is the whole
// reason it carries a slot number), and errVerifyPlateUnclaimed knows WHICH
// plate belongs to nothing. Anything else is the comparator's own wrapped
// message, appended -- bundle.Verify's ms1 arms already say "ms1", which is the
// one word that stops an operator scrapping good plates over a keystroke.
//
// NO EM-DASH: it is a zero-pixel glyph in the body face and a line carrying one
// does not draw at all (F-78/F-151).
func multisigVerifyFailureText(err error) string {
	var noPlate errVerifyLegHasNoPlate
	if errors.As(err, &noPlate) {
		return fmt.Sprintf("No read-back key plate carries slot @%d's key. Either that "+
			"plate was not presented, or it is not the one this run cut. Present every "+
			"plate this run engraved, or re-cut slot @%d's.", noPlate.Slot, noPlate.Slot)
	}
	var unclaimed errVerifyPlateUnclaimed
	if errors.As(err, &unclaimed) {
		return fmt.Sprintf("Read-back key plate %d belongs to no slot of this wallet. It "+
			"is steel from another run or another wallet; take it off the bench and "+
			"verify again.", unclaimed.Plate+1)
	}
	return "The read-back bundle does NOT match the seed you typed.\n\n" +
		"Details: " + err.Error() + "\n\n" +
		"Check what that names before you re-cut anything: a hand-typed ms1 is one " +
		"character away from this message, and the plates may be perfect."
}

// multisigVerifyIncompleteText is the partial-verify report, and the number in
// it is the number of plates the COMPARATOR matched.
//
// The sentence it replaces said "Checked %d of the %d key plates this run
// engraved" over `len(legs)` -- a count of slots re-derived from a typed seed,
// produced by the loop counter rather than by the comparison. It also prescribed
// a remedy the device does not have ("Run verify again with the remaining
// seeds"): multisigVerifyFlow's only callers were one-shot post-engrave offers,
// the program table carries no standalone bundle verify, and the operator's only
// route was to re-run the whole engrave. The remedy exists now -- both callers
// re-offer the verify on anything short of a clean pass -- so the instruction
// names the button they are about to be shown rather than a program that is not
// there.
//
// IT NAMES THE SLOTS in both directions. "2 of 3" leaves the operator to work
// out WHICH one is outstanding from a policy they are holding on steel.
//
// AND THE RETRY IS DESCRIBED AS THE RETRY THE DEVICE ACTUALLY OFFERS (B2). The
// sentence used to read "Choose VERIFY AGAIN on the next screen and type the
// remaining seed", which named the right BUTTON and the wrong ACTION: a second
// attempt is a fresh multisigVerifyFlow with empty `legs` and an empty `covered`
// set, so an operator who obeyed that literally read "Checked 1 key plate: @2 ...
// 2 slots are NOT verified: @0 and @1" -- an incomplete verdict, which re-arms
// the loop, which re-offers the same instruction, forever. The only route to
// Verify OK is typing EVERY seed inside ONE attempt, and no screen said so.
//
// THE FIX IS THE WORDING, NOT THE STATE, and that is a ruling rather than a
// shortcut. multisigVerifyFlow re-runs bundleGatherFlow on every invocation, so
// attempt 2 may be presented a DIFFERENT plate set than attempt 1. Carrying
// coverage across attempts would let a "Verify OK" be assembled from two
// readbacks that were never both true at once -- a clean verdict no single
// readback ever proved, which is the false GREEN this whole rewrite exists to
// remove. Coverage carry-over is a design (re-verify the retained legs against
// the new readback), not a hoisted variable, and it is not attempted here.
func multisigVerifyIncompleteText(checked, outstanding []int) string {
	those := "those plates"
	if len(outstanding) == 1 {
		those = "that plate"
	}
	return fmt.Sprintf("Checked %s: %s. Compared against the plates you presented, "+
		"and they match.\n\n%s NOT verified: %s. Nothing has been proved about "+
		"%s.\n\nChoose VERIFY AGAIN on the next screen and type ALL of this wallet's "+
		"seeds in one pass; a new attempt keeps nothing from this one. Until then, do "+
		"not fund this wallet.",
		plateWord(len(checked), "key plate", "key plates"), formatSlotList(checked),
		plateWord(len(outstanding), "slot is", "slots are"), formatSlotList(outstanding),
		those)
}

// multisigVerifyCoveredSeedBody is what the operator reads when the seed they
// just typed cannot advance the verify because its slots are already covered, or
// because this run engraved none of them.
//
// NEITHER ARM ASSERTS A FOREIGN SEED ANY MORE, and that is the fix. Both used to
// end "The plates still outstanding belong to a different seed." -- a fact the
// device cannot support and, at S5, often false: the passphrase prompt is PER
// HELD SLOT (buildSeedForSlot), it has NO confirm-entry, and nothing compares
// two registry entries holding the same words. One mistyped character on the
// second slot's passphrase mints what looks like a second master, at the same
// origin, with a different key -- so the outstanding plate really was built from
// THESE words, under a passphrase that differs by one character, and the device
// sends an operator holding the only seed that exists to look for a second one.
//
// It is the same class as F-191 at a new site, and the unconditional half of the
// remedy costs nothing: never assert "a different seed" when the device cannot
// distinguish that from "the same seed, a different BIP-39 passphrase".
//
// `bareWordsMatch` is multisigVerifySeedIsInnocent's answer, wired in here for
// the one case the device CAN settle outright: a passphrase was typed and these
// same words fill a slot WITHOUT it, so the outstanding plates may simply be the
// bare-words wallet and the operator can try again in seconds.
//
// NO EM-DASH: a zero-pixel glyph in this face takes its whole line with it.
func multisigVerifyCoveredSeedBody(engravedNone, bareWordsMatch bool) string {
	lead := "That seed's slots have already been checked."
	if engravedNone {
		lead = "That seed is a cosigner, but none of its slots were engraved in this run."
	}
	if bareWordsMatch {
		return lead + " These same words DO fill another slot with no passphrase. " +
			"Try again and skip the passphrase."
	}
	return lead + " The outstanding plates were built from different words, or from " +
		"these words with a different BIP-39 passphrase."
}

// verifyClaimPlate finds the UNCLAIMED read-back plate carrying the same account
// xpub as `want`. An undecodable plate is skipped here and caught by the
// unclaimed sweep, so a corrupted plate can never be quietly dropped.
func verifyClaimPlate(want []string, mk1s [][]string, claimed []bool) (int, bool) {
	w, err := mk.Decode(want)
	if err != nil {
		return 0, false
	}
	for i, p := range mk1s {
		if claimed[i] {
			continue
		}
		got, derr := mk.Decode(p)
		if derr != nil {
			continue
		}
		if got.Xpub == w.Xpub {
			return i, true
		}
	}
	return 0, false
}

// multisigVerifyFlow drives the on-device verify-bundle for the multisig flow:
// gather the engraved md1 + EVERY operator mk1 plate over NFC
// (extractReadbackMd1AndMk1s), then for each seed the operator re-types
// (fresh residency) re-derive a leg for every ENGRAVED slot that seed accounts
// for, hand-type that seed's ms1 (full only; never NFC), and report PASS/FAIL —
// comparing the READ-BACK plates against the re-derived legs (H1: never a
// re-derived value against itself). `full` reports whether an ms1 was engraved
// (and so must be hand-typed for verify).
//
// `expectedSlots` IS THE OBLIGATION LIST, AND ONLY THE ENGRAVER HAS IT. The two
// other candidates are both wrong, and one of them shipped:
//
//   - THE SEED cannot say what was engraved. allUserSlots answers "which slots
//     does this seed fill", which is a different question. On the BUILD path it
//     is still strictly larger than what was cut whenever a payload cosigner
//     card carries a DIFFERENT key from the operator's own seed at another
//     origin: duplicateSlotPair refuses only IDENTICAL keys, so that card is
//     admitted and the seed accounts for a slot nobody claimed. Before F-188 it
//     was larger on the SUPPLY path too, and deriving a leg per filled slot
//     reported "Verify Failed" over that path's own complete, correct output.
//   - THE READBACK cannot say it either. It is the evidence being judged; a
//     verify that takes its target from the evidence stops asking for the seed
//     that proves the plate nobody presented.
//
// So the caller passes what it cut — buildEngraveTail's held-slot indices on the
// build path, supplyEngraveTail's matched-slot indices on the supply path — and
// this flow proves those and nothing else. It is not a relaxation: every leg
// still has to find its own plate, and every plate still has to be claimed by a
// leg (verifyMultisigLegs, unchanged).
//
// TWO THINGS CHANGED AT S5, and both are consequences of a build holding several
// slots.
//
//  1. THE GATHER RUNS FIRST. It used to run after the seed, and gathering first
//     matches the build path's own posture (TestBuildFlow_GatherBeforeSeed) — no
//     secret is resident while a public set is being resolved. (The original
//     reason given here was that the readback says how many legs there are to
//     prove. That was the defect: the expectation does, and it arrives with the
//     call. The ordering is still right, for the residency reason.)
//  2. SEVERAL SEEDS. Trace B's three plates span two masters, and one seed can
//     only prove its own. So the flow loops: type a seed, cover the engraved
//     slots it accounts for, and if slots remain, offer the next one. Declining
//     is allowed and is reported as an INCOMPLETE verify — never as a pass over
//     plates nobody checked.
//
// ONE SCRUB SITE, DEFERRED BEFORE THE FIRST SEED EXISTS, is the same design the
// build flow's seedRegistry uses and for the same reason: every exit below (a
// Back, a refusal modal, a ctx.Done unwind, a panic) is covered by construction
// rather than by an implementer remembering to add a wipe to a new return.
//
// `engravedMd1` IS THE OTHER HALF OF THE OBLIGATION, and without it the slot set
// proves less than its own screens claim.
//
// The slot set binds this verify to the run's slot INDICES. It does not bind it
// to the run's POLICY, and the policy is re-read from the evidence being judged:
// the readback md1 supplies `keys`, allUserSlots runs against those keys, every
// leg derives at those origins over that md1, and bundle.Verify's md1 leg is (by
// this file's own header admission) the supplied input compared against a clone
// of itself. So the integers in expectedSlots get re-based onto whatever wallet
// the operator presents.
//
// EXECUTED, and it is exactly the false GREEN the design gate killed the
// plate-COUNT design with, moved up one level from slot identity to policy
// identity: engrave wallet P (3-of-4, expectation [2]), present wallet P' -- the
// SAME four cosigners at 2-of-4, byte-valid, with its own policy-id stub binding
// its own plates -- and every check passes, because each one compares P' against
// P'. Final screen: "Verify OK", over a plate this run did not cut, while the
// plate and md1 this run DID cut were never read. The realistic reach is the
// operator this verify exists for: a re-cut after changing k, with the
// superseded generation's steel on the same bench.
//
// Both callers already held the datum that closes it (gui/multisig.go's
// suppliedMd1, gui/multisig_build.go's engraveMd1) and neither passed it. It
// arrives with the obligation now, and the readback must EQUAL it. §7.4 is not
// implicated: the EVIDENCE still comes from the plates, and every leg is still
// re-derived from a RE-TYPED seed. What gained the policy identity is the
// OBLIGATION, which is the half only the engraver ever knew.
//
// IT RETURNS ITS VERDICT, from this fold. A void verify left both callers unable
// to tell a clean pass from an incomplete or failed one, so the screen after
// EITHER was the restore document -- and the incomplete screen's own instruction
// ("run verify again") named an action that existed nowhere on the device: this
// function's only callers are one-shot post-engrave offers, and the program
// dispatch table has no standalone bundle verify. The verdict is what lets the
// callers re-offer, which is what makes that instruction true.

// multisigVerifyFn is the in-file test-only seam BOTH engrave callers dispatch
// through, and it exists so the RETRY LOOP can be driven (B4).
//
// The loop is unreachable from a unit test without it: reaching the offer means
// completing a real engrave, and reaching a SECOND attempt means driving an
// entire readback -- gather, seed entry, passphrase, ms1 -- twice, inside a walk
// that has already cut every plate of the set. So I-4's whole mechanism was pinned by four
// strings.Contains calls over the callers' SOURCE, and two independent mutations
// left the tree green: turning `for {` into a one-iteration loop restored the
// pre-fold one-shot offer, and swapping the {"VERIFY AGAIN", "CONTINUE"} labels
// made VERIFY AGAIN exit the verify and CONTINUE re-run it -- the loop keys on
// the returned INDEX and there is no label lookup anywhere. Coverage on the
// unmutated tree put both loop bodies at execution count 0.
//
// The seam substitutes the VERDICT SOURCE only. The offer screens, the
// ChoiceScreen row mapping, the loop condition and the retry lead are all the
// production ones under the test, which is where both mutations live.
//
// It mirrors the package's other in-file test seams (bip85SeedHook,
// bip85PkeyHook, freetextPlateHook, freetextEngraveHook, buildMultisigSeedHook).
// nil is not a legal value: it is a var, not a hook, and production never
// reassigns it.
var multisigVerifyFn = multisigVerifyFlow

// countUncoveredPolicyKeys counts the policy keys this run did NOT verify a leg
// for -- the cosigners whose keys the document takes as SUPPLIED rather than
// checked.
//
// IT ITERATES THE KEYS AND ASKS WHETHER EACH IS COVERED, never the other way
// round, and the direction is the whole of it. `len(keys) - len(covered)` is
// arithmetically the same number today and fails in the WRONG DIRECTION: a
// stray or out-of-range entry in `covered` SHRINKS it, so a defect in the map
// would under-report what went unchecked. This form ignores entries outside
// [0, len(keys)) and counts a wrongly-cleared one as uncovered, so every defect
// can only inflate the count -- and an inflated count renders a clause saying
// LESS was checked, which is the direction S6a G2 allows.
func countUncoveredPolicyKeys(keys []md.ExpandedKey, covered map[int]bool) int {
	n := 0
	for i := range keys {
		if !covered[i] {
			n++
		}
	}
	return n
}

// The three input steps of one verify leg. Back moves between them rather than
// ending the verify, per the 2026-08-19 operator directive "going back should
// lose nothing"; only legStepSeed's Back still leaves.
const (
	legStepSeed = iota
	legStepPassphrase
	legStepMS1
)

// `rec` IS AN OUT-PARAMETER, and the verdict return is deliberately unchanged
// (S6a §4.7b-seam). The two facts the restore document needs -- was a pass
// recorded, was anything adverse recorded -- are written HERE, at the return
// sites where they are in scope, and NOT inferred downstream from the verdict:
// a verdict is a summary of the flow rather than a record of what it observed,
// and two review rounds proved it unsound as a proxy for either bit. Widening
// the return instead would have broken three shipped tests, one of them a source
// assertion, for no gain the callers can use.
//
// THE CALLERS' RECORD OUTLIVES THE LOOP, which is what makes `adverse` sticky
// across a retry: both engrave callers declare one verifyRecord OUTSIDE their
// re-offer loop and pass it to every attempt, so a first attempt that failed and
// a second that passed report statusVerifiedOnRetry rather than a bare pass.
func multisigVerifyFlow(ctx *Context, th *Colors, full bool, expectedSlots []int, engravedMd1 []string, rec *verifyRecord) multisigVerifyResult {
	// A NIL RECORD IS DISCARDED, NOT DEREFERENCED. Every caller passes one and
	// this is unreachable today; it is here because the alternative failure is a
	// panic on the device, mid-verify, on the paths that are already the bad news.
	if rec == nil {
		rec = &verifyRecord{}
	}
	// THE ENGRAVER'S OBLIGATION LIST ARRIVES FROM THE CALLER, and an empty one is
	// refused BEFORE the operator is asked to present anything. A verify with no
	// slot to prove cannot be satisfied by any readback, so sending them to the
	// reader first would collect plates for a comparison that was never going to
	// run. errVerifyNoExpectedSlots carries the same refusal for the derive loop.
	if len(expectedSlots) == 0 {
		showError(ctx, th, "Verify Bundle", multisigVerifyNoExpectationBody)
		return verifyRefused
	}
	// AND SO IS A MISSING POLICY, for the same reason one level over. An absent
	// engraved md1 would make the equality check below vacuous -- every readback
	// would be "the policy this run engraved" -- which is the shape of a guard
	// that reports success for a comparison that never ran. Both call sites carry
	// it today; this fires only if a future one loses it on the way in, which is
	// exactly when a vacuous pass would be invisible.
	if len(engravedMd1) == 0 {
		showError(ctx, th, "Verify Bundle", multisigVerifyNoPolicyBody)
		return verifyRefused
	}

	// Read back the PUBLIC cards over NFC via the T5 gatherer.
	//
	// NO PAYLOAD OFFER HERE, deliberately (plan stage 13c). §3.3.2 admits
	// ClassMDMK to this program, but a verify READBACK must come from the
	// plate's own cards: §7.4's reasoning applied to the bundle rather than to
	// the seed — a readback taken from the session would compare the engrave
	// source against itself and pass unconditionally, certifying a wrong plate.
	// The passphrase step below uses passphraseFlow for the same reason.
	cards, ok := bundleGatherFlow(ctx, th, "Engrave Bundle")
	if !ok {
		// Back at the gather is the operator declining to present plates at all.
		// It is an ABANDON rather than an incomplete: nothing was compared, so
		// there is nothing to re-offer them a second attempt at.
		return verifyAbandoned
	}
	readbackMd1, readbackMk1s, ok := extractReadbackMd1AndMk1s(cards)
	if !ok {
		// ADVERSE: cards came off the plates and could not be accounted for. The
		// gather returned ok, so complete cards were read; the filter then could not
		// find one md1 and the key card(s) among them. "A plate could not be read or
		// accounted for" is a literal description of this state.
		//
		// verifyIncomplete, NOT verifyRefused (F-199, S6b spec §3.1). This screen
		// names exactly what the operator should re-present, and unlike the
		// programmer-error refusals above (empty expectedSlots, empty engravedMd1)
		// the operator CAN fix this by trying again with the same plates read more
		// carefully, or the right ones found. Widening verifyRefused itself to
		// re-offer was considered and rejected: three other return sites still
		// share that verdict and must never loop (two programmer errors plus one
		// unreachable defensive re-check), so the fix is PER-SITE -- this site
		// alone moves to the verdict the callers already re-offer on.
		rec.adverse = true
		showError(ctx, th, "Verify Bundle", "Read back one wallet-policy md1 AND the operator key card(s) (mk1).")
		return verifyIncomplete
	}
	// THE POLICY IDENTITY, CHECKED FIRST AND ON THE BYTES.
	//
	// It runs before the decode, before the count precheck and before any secret
	// is asked for, because a readback of ANOTHER wallet is not a mis-count and
	// not a mis-cut: nothing further this flow does can produce a true answer
	// about it, and every check downstream would compare that wallet against
	// itself and agree.
	//
	// EXACT CHUNK EQUALITY, not a decoded-policy comparison. The chunk strings are
	// what went on steel; the encoders are deterministic (plan S5 item 7,
	// TestReRunMintsByteIdenticalPlates), so an honest re-read of this run's own
	// md1 reproduces them exactly. Comparing decoded fields instead would accept a
	// re-chunked or re-encoded descriptor that is not the plate the operator is
	// holding, which is the one thing this check exists to notice.
	if !slices.Equal(readbackMd1, engravedMd1) {
		// ADVERSE: the plate presented carries a policy this run did not engrave.
		rec.adverse = true
		showError(ctx, th, "Verify Bundle", multisigVerifyForeignPolicyBody)
		return verifyFailed
	}
	_, keys, err := md.ExpandWalletPolicyChunks(readbackMd1)
	if err != nil {
		// ADVERSE: a plate was read and would not decode.
		rec.adverse = true
		showError(ctx, th, "Verify Bundle", "Couldn't decode the read-back wallet policy.")
		return verifyFailed
	}
	// A FAST LENGTH PRECHECK, NAMED IN BOTH DIRECTIONS. It is a courtesy, not the
	// mechanism: the per-slot bijection below is what decides, and it decides on
	// IDENTITY rather than on cardinality -- a readback of the right NUMBER of
	// plates carrying the wrong ones still fails there. What this buys is the
	// moment it fails: an operator who has mislaid a plate, or brought one from an
	// earlier run, learns it before typing a seed rather than after.
	if len(readbackMk1s) != len(expectedSlots) {
		// ADVERSE: the plates were read and counted, and the count is not the one
		// this run cut. A plate from another run, or one this run cut and nobody can
		// find, is inside this world-set.
		rec.adverse = true
		showError(ctx, th, "Verify Bundle", fmt.Sprintf(
			"Read back %s, but this run engraved %s. Present exactly the plates this "+
				"run cut.",
			plateWord(len(readbackMk1s), "key plate", "key plates"),
			plateWord(len(expectedSlots), "key plate", "key plates")))
		return verifyIncomplete
	}

	var typed []bip39.Mnemonic
	defer func() {
		for _, m := range typed {
			for i := range m {
				m[i] = 0
			}
		}
	}()

	var legs []verifyLeg
	// correctable records that the operator was shown a screen NAMING A REMEDY
	// they can act on, as opposed to walking out (B3).
	//
	// It exists because the verdict alone cannot tell those apart once `legs` is
	// empty, and the two callers branch their retry loop on the verdict. A
	// first-seed failure broke out with no legs and returned verifyAbandoned, so
	// "add the passphrase and try again", the provedInnocent arm's remedy
	// (superseded by R-M, S6b spec §3.2a -- at the time this was "Your plates
	// are fine. Try again and skip the passphrase.") and "That isn't an ms1
	// secret share." were each followed by the restore document and no retry
	// offer -- the exact shape I-4 existed to remove, in the three places I-4
	// did not look.
	//
	// IT IS SET INSIDE THE FLOW RATHER THAN WIDENING THE CALLERS' GATE. Making
	// the callers re-offer on verifyAbandoned would also re-offer after a Back at
	// the gather, which is the operator declining to present plates at all, and
	// after a Back at the ms1 entry, which is them declining to type one. Neither
	// was shown anything to correct.
	correctable := false
	covered := make(map[int]bool, len(keys))
	// ONE LEG PER EXPECTED SLOT IS THE TERMINATION RULE, not one per plate read
	// back. The obligation is what the engrave cut; the readback is the evidence
	// offered against it, and letting the evidence set the target is how a verify
	// stops asking for the seed that proves the plate nobody presented.
	for len(legs) < len(expectedSlots) {
		// ONE LEG IS THREE INPUT STEPS -- seed, passphrase, ms1 -- AND BACK MOVES
		// BETWEEN THEM (2026-08-19 operator directive: "going back should lose
		// nothing"). Before this, NEITHER Back inside a leg went back:
		//
		//   passphrase  the prompt was read as `if sel, ok := ...; ok && sel == 1`,
		//               so a Back fell THROUGH with an empty passphrase and the flow
		//               derived. Back did what Skip does, and then committed to a
		//               verification the operator was trying to leave.
		//   ms1         a Back ended the whole verify and reported it incomplete.
		//
		// Only the SEED step's Back still leaves, which is the directive's rule for
		// a first step.
		//
		// The steps are a machine rather than a straight line because the
		// passphrase determines `fresh`: stepping back onto the passphrase and
		// forward again must RE-DERIVE, never reuse a slot set computed from the
		// passphrase that was just replaced. Every value a step produces is
		// therefore ASSIGNED here, not appended to -- `typed` is the one exception,
		// and deliberately so (see the seed step).
		var reMnemonic bip39.Mnemonic
		passphrase := ""
		ms1Readback := ""
		var fresh []int
		// Built ONCE per leg so that stepping back onto it remembers the selection
		// the operator made. A Back that forgets the answer to the step it returns
		// to loses precisely what the directive says it must not.
		ppChoice := &ChoiceScreen{Title: "Passphrase", Lead: "Add a BIP-39 passphrase?", Choices: []string{"Skip", "Add passphrase"}}
		legState := legStepSeed
		legLeft := false    // Back out of the FIRST step: abandon the verify
		legRefused := false // a screen was shown that ends the verify

	legSteps:
		for {
			switch legState {
			case legStepSeed:
				m, ok := seedEntryFlowTypedOnlyResume(ctx, th, reMnemonic)
				if !ok {
					legLeft = true
					break legSteps
				}
				reMnemonic = m
				// Owned by the deferred scrub from this line on, before any screen
				// below can return early. Stepping back and re-entering APPENDS
				// another copy rather than replacing this one: every array the
				// operator's words ever landed in has to be scrubbed, and the copy
				// they abandoned is no less live than the copy they kept.
				typed = append(typed, reMnemonic)
				legState = legStepPassphrase
			case legStepPassphrase:
				sel, ok := ppChoice.Choose(ctx, th)
				if !ok {
					legState = legStepSeed
					continue
				}
				// REASSIGNED, not left from the previous pass: an operator who added
				// a passphrase, stepped back, and then chose Skip must be verified
				// against the empty one they just chose.
				passphrase = ""
				if sel == 1 {
					if pass, ok := passphraseFlow(ctx, th); ok {
						passphrase = pass
					}
				}

				slots := allUserSlots(reMnemonic, passphrase, &chaincfg.MainNetParams, keys)
				// ASSIGNED, NOT DECLARED. `fresh` belongs to the leg, not to this
				// step: the derive loop below the step machine reads it. Written
				// with `:=` here it silently shadows the leg's copy -- which still
				// compiles, because ferr is new -- and the derive loop then iterates
				// over a nil slice and appends no legs at all. Caught by
				// TestMS1ClauseIsCountFreeAcrossSeedAndLegCounts, not by the build.
				var ferr error
				fresh, ferr = verifyFreshSlots(expectedSlots, slots, covered)
				if ferr != nil {
					// UNREACHABLE IN-PROCESS (F-199, S6b spec §3.1), and left as
					// verifyRefused deliberately -- it stays non-looping IN CASE it ever
					// does fire, rather than being upgraded alongside :753's genuinely
					// correctable readback failure. verifyFreshSlots' only error is
					// errVerifyNoExpectedSlots on len(expected)==0 (:324-336), and
					// expectedSlots is a parameter this function never reassigns (see the
					// source assertion pinning that), so this is a defensive re-check of
					// the SAME condition :717 already refused before any seed was typed.
					showError(ctx, th, "Verify Bundle", multisigVerifyNoExpectationBody)
					return verifyRefused
				}
				if len(fresh) == 0 {
					// THREE DIFFERENT PROBLEMS, THREE DIFFERENT MESSAGES, and the third is new
					// with the expected-slot restriction. A seed that is in the policy but
					// already checked is an operator repeating themselves; a seed in no slot at
					// all is the wrong wallet, and telling them to "try another" would send
					// them looking for a seed that exists. The third is a seed that IS a
					// cosigner and whose slots this run simply did not engrave -- the supply
					// path's other cosigner, or a build that held a subset -- and calling that
					// "not a cosigner" would tell an operator holding the right seed that their
					// wallet is wrong.
					//
					// The distinction is drawn with the SAME rule, run against an empty covered
					// set, rather than with a second hand-rolled intersection.
					//
					// AND THE FIRST ARM ASKS ABOUT THE PASSPHRASE BEFORE IT CONDEMNS THE SEED
					// (F-191). "No slot matched" is produced just as readily by one wrong
					// character in a re-typed passphrase as by a foreign seed, and the two have
					// opposite remedies -- retype, or stop trusting your plates. The flow knows
					// which passphrase it was handed, so it asks the question it can afford:
					// does this seed fill a slot with the EMPTY passphrase? One re-derivation,
					// on a path that has already failed and is about to show a screen.
					everOwed, _ := verifyFreshSlots(expectedSlots, slots, nil)
					switch {
					case len(slots) == 0:
						innocent := multisigVerifySeedIsInnocent(reMnemonic, passphrase, &chaincfg.MainNetParams, keys)
						showError(ctx, th, "Verify Bundle",
							multisigVerifyNoSlotBody(passphrase != "", innocent))
					default:
						// AND THE OTHER TWO ARMS ASK THE SAME QUESTION (I-14). Both used to
						// assert "The plates still outstanding belong to a different seed." --
						// a fact the device cannot support, and at S5 frequently false: the
						// passphrase prompt is per HELD SLOT with no confirm-entry, so one
						// mistyped character on the second slot mints what looks like a second
						// master from the SAME words. The routine that can settle part of it
						// was already here and wired into the first arm only.
						bare := multisigVerifySeedIsInnocent(reMnemonic, passphrase,
							&chaincfg.MainNetParams, keys)
						showError(ctx, th, "Verify Bundle",
							multisigVerifyCoveredSeedBody(len(everOwed) == 0, bare))
					}
					// BREAK, NOT RETURN, for the ms1 entry's reason twenty lines down and
					// against the same evidence. All three screens above are about the SEED
					// the operator just typed; none of them is a verdict on the plates
					// already checked. On the first seed the break reaches the silent
					// abandon below, which is the shipped behaviour. On the second or later
					// seed `legs` already holds VERIFIED plates, and returning here walked
					// out with no verdict at all -- some plates checked, some not, nothing
					// said, and on the build path the next thing the operator saw was the
					// restore document. Breaking falls into the "Verify Incomplete" report,
					// which is what a partial verify is.
					//
					// It is placed AFTER the switch rather than inside it because a `break`
					// in a Go switch breaks the SWITCH, not the loop -- three arms that each
					// looked like they stopped the verify and did not would be a worse defect
					// than the one being fixed.
					//
					// AND ALL THREE ARMS PRESCRIBE A REMEDY (B3), which is why this is set
					// unconditionally here rather than per arm: "add it and try again", "try
					// the password again" (R-M's provedInnocent wording, S6b spec §3.2a --
					// superseding this comment's earlier "skip the passphrase" quote, struck
					// by the same ruling), "Check the passphrase before you doubt the
					// plates", and both covered-seed arms tell the operator to change an
					// input. A screen that says try again and is followed by the restore
					// document is worse than one that says nothing.
					correctable = true
					legRefused = true
					break legSteps
				}
				legState = legStepMS1
			case legStepMS1:
				// Hand-type the SECRET ms1 (full mode only; never NFC). ONE PER SEED:
				// a build across two masters engraves two seed plates, and each must
				// be checked against the master it claims.
				if !full {
					break legSteps
				}

				s, ok, rejected := multisigVerifyMS1Entry(ctx, th)
				if !ok {
					// BREAK, NOT RETURN. On the FIRST seed this reaches the silent
					// abandon below, which is the shipped behaviour. On the SECOND or
					// later seed `legs` already holds verified plates, and a bare return
					// here walked out of the flow with NO SCREEN AT ALL -- some plates
					// checked, some not, nothing said, and on the build path the next
					// thing the operator saw was the restore document. Breaking falls
					// into the "Verify Incomplete" report, which is what a partial
					// verify is. (len(legs) < len(expectedSlots) necessarily holds here:
					// the coverage break is tested only after this seed's derive loop.)
					//
					// ONLY A REJECTION IS CORRECTABLE (B3), AND A BACK IS NOT. The helper
					// returns two different failures through one bool: it either SHOWED a
					// screen naming what was wrong with the object the operator typed --
					// which they can retype -- or the operator pressed Back at the
					// keyboard, which is them declining to type an ms1 at all. Re-offering
					// the verify after a Back loops them against a choice they just made.
					//
					// A BACK NOW STEPS BACK TO THE PASSPHRASE (2026-08-19 directive),
					// which is the step before this one -- not a re-offer of the ms1
					// keyboard the operator just declined, which is the loop the
					// paragraph above refuses. A REJECTION still stops the verify: the
					// helper already showed the screen naming what was wrong, and B3
					// gives that screen a remedy the callers' re-offer provides.
					if rejected {
						correctable = true
						legRefused = true
						break legSteps
					}
					legState = legStepPassphrase
					continue
				}
				ms1Readback = s
				break legSteps
			}
		}
		if legLeft || legRefused {
			break
		}

		for _, s := range fresh {
			b, derr := deriveMultisigLeg(reMnemonic, passphrase, &chaincfg.MainNetParams,
				keys[s].OriginPath, readbackMd1, full)
			if derr != nil {
				showError(ctx, th, "Verify Bundle", "Couldn't re-derive the bundle from the seed.")
				return verifyFailed
			}
			legs = append(legs, verifyLeg{Slot: s, B: b, MS1Readback: ms1Readback})
			covered[s] = true
		}
		if len(legs) >= len(expectedSlots) {
			break
		}
		next := &ChoiceScreen{
			Title:   "Verify Bundle",
			Lead:    fmt.Sprintf("%s not checked yet. Next seed?", plateWord(len(expectedSlots)-len(legs), "key plate is", "key plates are")),
			Choices: []string{"TYPE THE NEXT SEED", "STOP HERE"},
		}
		if sel, ok := next.Choose(ctx, th); !ok || sel != 0 {
			break
		}
	}

	// Back before ANY leg is verified is abandoning the verify, not completing a
	// partial one, and it is the shipped behaviour: return in silence.
	//
	// The earlier wording here claimed zero legs was reachable only from the first
	// seed's entry "because every other exit above returns on its own screen".
	// That was false: the ms1 entry's Back returned bare from any seed, so a
	// partial verify could walk out silently. It breaks now, so the condition this
	// tests is genuinely "nothing was verified" rather than "we came from one
	// particular screen" -- a state, which is checkable, instead of a provenance,
	// which was not.
	//
	// UNLESS A REMEDY WAS PRESCRIBED (B3), in which case this is not an abandon at
	// all: the operator was told to change an input and try again, and the retry
	// they were told about is the callers' re-offer. verifyIncomplete is the
	// verdict that reaches it, and it is TRUE of this state -- not one plate of
	// the obligation is verified.
	//
	// IT SHOWS NO SCREEN OF ITS OWN, deliberately. The remedy screen was drawn a
	// moment ago and the caller's re-offer leads with multisigVerifyRetryLead
	// ("Not every plate is verified. Try again?"). A second modal restating what
	// was just dismissed is how an operator learns to tap past modals.
	if len(legs) == 0 {
		if correctable {
			return verifyIncomplete
		}
		return verifyAbandoned
	}

	// NOT A PASS, AND NOT SILENCE. Stopping early with plates still outstanding is
	// a legitimate operator choice and it produces an INCOMPLETE verify, which is
	// a different thing from a failed one and from a clean one. Reporting it as
	// either would be the false GREEN this whole rewrite exists to remove.
	//
	// AND IT IS NOT A COUNT OF ANYTHING UNTIL THE COMPARATOR HAS RUN (C-1). This
	// branch used to `return` here, which made the call below structurally
	// unreachable on this path -- so the screen reported `len(legs)`, a count of
	// slots RE-DERIVED FROM A TYPED SEED, as a count of "key plates checked". A
	// foreign mk1 among the presented plates was never decoded, never xpub-matched
	// and never stub-bound, and the operator read it as verified.
	//
	// The forward half of the bijection runs; only the "every plate must be
	// claimed" sweep is skipped, because a run that stopped early legitimately has
	// plates no leg was ever built for. A leg whose plate FAILS is a Verify
	// Failed, not an Incomplete: the operator is holding bad steel and stopping
	// early does not make that provisional.
	if len(legs) < len(expectedSlots) {
		if _, err := verifyMultisigLegsPartial(legs, readbackMk1s, readbackMd1); err != nil {
			// ADVERSE: the comparator ran over the plates that WERE presented and
			// disagreed. Stopping early does not make a bad plate provisional.
			rec.adverse = true
			showError(ctx, th, "Verify Failed", multisigVerifyFailureText(err))
			return verifyFailed
		}
		checked := make([]int, 0, len(legs))
		for _, l := range legs {
			checked = append(checked, l.Slot)
		}
		outstanding := make([]int, 0, len(expectedSlots)-len(legs))
		for _, s := range expectedSlots {
			if !covered[s] {
				outstanding = append(outstanding, s)
			}
		}
		slices.Sort(checked)
		slices.Sort(outstanding)
		showError(ctx, th, "Verify Incomplete",
			multisigVerifyIncompleteText(checked, outstanding))
		return verifyIncomplete
	}

	if err := verifyMultisigLegs(legs, readbackMk1s, readbackMd1); err != nil {
		// ADVERSE: the comparator ran over every expected plate and disagreed. All
		// eleven of bundle.Verify's errors classify identically here -- the
		// provenance distinction inside it is one nothing on the document consumes.
		rec.adverse = true
		showError(ctx, th, "Verify Failed", multisigVerifyFailureText(err))
		return verifyFailed
	}
	// THE SUCCESS RETURN, AND THE ONLY PLACE A PASS IS RECORDED. `full` is the
	// flow's own parameter and is captured HERE, where it is in scope, so a
	// watch-only run cannot produce a document claiming an ms1 comparison that
	// never ran. `legs` is what was compared, not what was expected.
	rec.pass = &passRecord{
		full:              full,
		legs:              len(legs),
		suppliedCosigners: countUncoveredPolicyKeys(keys, covered),
	}
	showNotice(ctx, th, multisigVerifyOKTitle, multisigVerifyOKMessage(len(legs), full))
	return verifyComplete
}

// multisigVerifyMS1Entry hand-types ONE ms1 and returns its canonical string.
//
// Extracted so the per-seed loop above reads as a loop rather than as sixteen
// lines of secret handling repeated inline. The L1 scrub is the codebase
// convention (gui/ms1_decode.go): DecodeMS1 allocates a fresh entropy slice this
// probe would otherwise abandon to the GC.
//
// `rejected` SEPARATES THE TWO WAYS ok CAN BE FALSE (B3), which this function
// has always distinguished internally and never reported. A Back at the keyboard
// is the operator declining to type an ms1; a wrong OBJECT or an undecodable one
// is a screen naming what they typed wrongly, which they can retype. Only the
// second is a state the caller should re-offer the verify for, and returning one
// bool for both is what made the caller treat a correctable mistake on the first
// seed as walking out.
func multisigVerifyMS1Entry(ctx *Context, th *Colors) (canonical string, ok, rejected bool) {
	obj, ok := inputCodex32Flow(ctx, th, "Type ms1")
	if !ok {
		// Back: no screen was shown, so there is nothing to correct.
		return "", false, false
	}
	s, isStr := obj.(codex32.String)
	if !isStr {
		showError(ctx, th, "Verify Bundle", "That isn't an ms1 secret share.")
		return "", false, true
	}
	_, _, ent, err := codex32.DecodeMS1(s)
	if err != nil {
		showError(ctx, th, "Verify Bundle", "That isn't a valid ms1 secret share.")
		return "", false, true
	}
	wipeBytes(ent)
	return s.String(), true, false
}

// multisigVerifyOKMessage is the success notice, SCOPED TO WHAT WAS CHECKED.
//
// The one-leg text is the shipped constant unchanged, because its honesty is
// pinned (TestMultisigVerifyNoticeIsHonest: it must scope the guarantee and must
// not carry the full-bundle over-claim). A multi-leg run says HOW MANY plates it
// checked, for the same reason the one-leg text says what it did not check: a
// bare "Verify OK" after a three-plate readback is indistinguishable from a bare
// "Verify OK" after a one-plate one, and the operator has no other way to learn
// which they got.
//
// IT DOES NOT CLAIM THE SEED PLATES WERE COUNTED. Only what the operator typed
// was compared: a secret is never read back over the reader (§7.4), so the
// device cannot know how many ms1 plates exist and must not imply it checked
// them all. `full` is the mode, so a watch-only run does not claim a secret it
// never asked for.
//
// NO EM-DASH: it is a zero-pixel glyph in the body face and a line carrying one
// does not draw at all (F-78/F-151).
func multisigVerifyOKMessage(legs int, full bool) string {
	if legs <= 1 {
		// AND THE SINGLE-LEG ARM OBEYS `full` TOO (I-13). It ignored it and
		// returned the secret-claiming body unconditionally, so an ordinary
		// watch-only run -- one held or matched slot, "Watch-only (keys)" chosen,
		// no ms1 created, none requested, none typed, none compared, the ms1 leg
		// skipped on both sides by bundle.Verify's presence semantics -- finished
		// on "Operator key and secret verified." That is the doc comment's own rule
		// broken two lines below where it is stated, in the commonest case this
		// screen has.
		if !full {
			return "Operator key verified. Other cosigners' keys are taken as supplied."
		}
		return multisigVerifyOKBody
	}
	if full {
		return fmt.Sprintf("All %d operator key plates verified, and the ms1 you typed "+
			"for each seed. Other cosigners' keys are taken as supplied.", legs)
	}
	return fmt.Sprintf("All %d operator key plates verified. Other cosigners' keys "+
		"are taken as supplied.", legs)
}
