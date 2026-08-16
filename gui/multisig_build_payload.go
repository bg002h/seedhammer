package gui

import (
	"fmt"

	"seedhammer.com/sysw"
)

// ─── S1: the payload supplies the whole cosigner set ─────────────────────────
//
// Before S1 the Build path offered ONE card from the payload ("First card from
// where?") and expected the operator to scan the rest. Phase-1 hardware has no
// NFC reader, so "scan the rest" was an instruction nobody could follow: the
// flow could not complete a build at all. S1 replaces it — every ClassMDMK
// record the payload holds is fed to the gather, and the cosigner set is
// resolved from what arrived.
//
// TWO RULINGS BIND EVERYTHING BELOW, and are built to rather than around.
//
//  1. `0..n` (F-173, operator 2026-08-14): the payload may carry anywhere from
//     ZERO to n cosigner cards, and NO stage may assume it carries exactly n-1.
//     The delivered payload holds four cards and Trace A is a 2-of-3, so
//     over-supply is the NORMAL case, not an error. The exact-count check
//     therefore belongs to the ASSEMBLED set (buildCosignerCards, unchanged) and
//     never to the feed.
//
//  2. `defaults for spelling, never for stakes, and every default is printed`
//     (plan §0.1). Over-supply is resolved by SELECTION, and what got selected
//     is announced on the policy review screen — the last surface before the
//     EXPERIMENTAL warning and the engrave. Under-supply is a REFUSAL, because
//     the device cannot invent a key, and the refusal names the only route that
//     exists on this hardware.

// cosignerSourceState is why the payload could not be read, or that it was.
//
// It exists so the refusal text can name the RIGHT route. "No payload" and
// "payload not authenticated yet" and "payload too small" are three different
// operator problems with three different fixes, and collapsing them into one
// message sends two of the three operators looking in the wrong place.
type cosignerSourceState int

const (
	cosignerSourceLoaded     cosignerSourceState = iota // records were read (possibly zero)
	cosignerSourceNoPayload                             // nothing is loaded
	cosignerSourceUncompared                            // loaded, but [compared] is not earned
)

// buildCosignerSource is THE PER-KEY SOURCE SEAM (§5.1): the single place that
// answers "where does a cosigner key come from" for the Build path.
//
// S1 replaces this seam's PICKER — the syswOffer ChoiceScreen that asked where
// the first card came from — and deliberately keeps the seam itself. Deleting
// the call site outright would guarantee the later NFC plan has to re-open it,
// which is the one thing §5.1's build-it-once seam exists to prevent. Phase 1
// has exactly one answer, so there is nothing to pick between: an "Input" screen
// offering a single option costs a tap and teaches nothing.
//
// (The paragraph above deliberately does NOT quote that screen's lead verbatim.
// cmd/emu/needle_test.go counts a needle's production sites by blunt substring
// match over gui's source, exactly as `git grep -F` would, so a comment carrying
// the literal is counted as a third flow drawing it — measured, it failed that
// test on the first try.)
//
// Records come back in PAYLOAD RECORD ORDER (takeAll's contract) and nothing
// downstream reorders them.
func buildCosignerSource(ctx *Context) ([]string, cosignerSourceState) {
	if ctx.sysw == nil || !ctx.sysw.loaded {
		return nil, cosignerSourceNoPayload
	}
	records, ok := ctx.sysw.takeAll(sysw.ClassMDMK)
	if !ok {
		return nil, cosignerSourceUncompared
	}
	return groupRecordsByCard(records), cosignerSourceLoaded
}

// groupRecordsByCard makes each card's chunks CONTIGUOUS, with the cards in
// order of where each one FIRST appears in the payload. Chunk order within a
// card is left alone — the gatherer reassembles a set in any order.
//
// THIS IS WHAT MAKES "PAYLOAD RECORD ORDER" TRUE RATHER THAN USUALLY TRUE, and
// it is a funds-safety fix, not tidying. `bundleGatherer.cards` is
// "completed + verified, in COMPLETION order" (gui/bundle.go), and a chunked
// card completes when its LAST chunk arrives. So for an interleaved payload the
// two orders diverge — measured, records `A1 B1 B2 A2` put card B in the lower
// slot, because B finished first:
//
//	supply[0] = fp b8688df1  (card B)
//	supply[1] = fp 73c5da0a  (card A)
//
// @N order is identity-bearing (md/encode_multisig.go's ordering contract: the
// same keys in a different order mint a different wallet with a different
// WalletPolicyId), and spec P0 item 5 rules "slot order is payload record
// order". Meanwhile the review screen ANNOUNCES "in payload order"
// unconditionally, and with fingerprints omitted by default every slot renders
// "(no fp)" — so a wrong order would be invisible in every artifact the
// operator keeps. That is §0.1 clause 2's refuse side, and announcing it as
// true instead is the thing that had to be fixed.
//
// Fixed by making the BEHAVIOUR match the promise rather than by weakening the
// promise: group here, once, at the single seam both consumers read
// (buildCosignerSupply's pre-check and the gather's own feed), so completion
// order and record order cannot disagree.
//
// `me sysw pack` preserves caller order among public records, so whether the
// chunks interleave depends on the order the operator handed files to `pack`.
// Uncommon, admitted by the format, and rejected by nothing.
func groupRecordsByCard(records []string) []string {
	type group struct{ recs []string }
	var order []string
	groups := make(map[string]*group, len(records))
	for i, r := range records {
		cls, csid, _ := classify(mdmkText(r))
		// Only CHUNKED cards can interleave. Everything else — a standalone
		// md1, a refusal, a drop — is its own group keyed by position, so it
		// keeps its place exactly.
		key := fmt.Sprintf("solo:%d", i)
		switch cls {
		case clsChunkedMK1:
			key = fmt.Sprintf("mk:%d", csid)
		case clsChunkedMD1:
			key = fmt.Sprintf("md:%d", csid)
		}
		g, seen := groups[key]
		if !seen {
			g = &group{}
			groups[key] = g
			order = append(order, key)
		}
		g.recs = append(g.recs, r)
	}
	out := make([]string, 0, len(records))
	for _, k := range order {
		out = append(out, groups[k].recs...)
	}
	return out
}

// buildCosignerSupply assembles payload records into complete cards through
// bundleGatherer.offer() — the SAME insertion path a scanned card takes, so the
// payload's cards get identical dedup, chunk assembly and integrity gating. It
// reports the completed cards (in completion order, which for a contiguous
// payload is record order) and whether any chunk set was left half-assembled.
//
// This runs BEFORE the gather screen, and the gather then re-assembles the same
// records through the same offer(). That repetition is deliberate and it is not
// a second insertion path: it is one pure function over one input, run twice,
// and the gather's result remains the authoritative one. The alternative —
// letting the operator walk into a gather that cannot possibly succeed and
// meeting a dead end at the far side — is the defect S1 exists to remove.
//
// The returned order is completion order, which equals PAYLOAD RECORD ORDER
// because groupRecordsByCard has already made each card's chunks contiguous.
// Both facts are load-bearing together and neither is enough alone; see that
// function for the interleaved payload that proved it.
func buildCosignerSupply(records []string) (cards []bundleCard, incomplete bool) {
	var g bundleGatherer
	for _, r := range records {
		g.offer(mdmkText(r))
	}
	return mk1CosignerCards(g.cards), g.pending()
}

// mk1CosignerCards keeps the cosigner KEY cards and drops md1 descriptors.
//
// Spec P0 item 3: an md1 riding along on the payload is normal — the payload is
// systemwide and carries whatever the operator packed — and it must not fail the
// build. buildCosignerCards refuses on a cardMD1, so the filter belongs here,
// ahead of it. The refusal there stays as the backstop for any set assembled by
// some other route.
func mk1CosignerCards(cards []bundleCard) []bundleCard {
	var out []bundleCard
	for _, c := range cards {
		if c.kind == cardMK1 {
			out = append(out, c)
		}
	}
	return out
}

// buildCosignerOutcome is what a supply of cards MEANS for a number of open
// slots. Three outcomes, and the set is closed: F-173's ruling is that no stage
// may assume the payload carries exactly n-1, so the case analysis has to be
// total or the assumption comes back as a fall-through.
type buildCosignerOutcome int

const (
	cosignerRefuse   buildCosignerOutcome = iota // fewer cards than slots — named refusal
	cosignerAutoFill                             // exactly as many as slots — nothing to choose
	cosignerSelect                               // more than slots — bounded selection
)

// classifyCosignerSupply is the whole `0..n` ruling as one pure function, and it
// is deliberately the ONLY place the count comparison happens: a second copy of
// this decision inline in the flow is how "the payload carries n-1" grew back
// last time.
//
// OVER-SUPPLY IS NOT AN ERROR. The delivered payload carries four cards and
// Trace A is a 2-of-3, so `have > open` is the normal case; it resolves by
// selection. The exact-count refusal stays on the ASSEMBLED set
// (buildCosignerCards), where it is a structural backstop rather than a
// reachable arm — written on the feed instead, it pins the very behaviour that
// made Trace A unreachable while every unit test stayed green.
//
// A ZERO DEMAND IS SATISFIED BY ANY PAYLOAD, INCLUDING NONE, and that arm is
// FIRST because the switch below cannot express it. Its first case is a
// comma-OR (`state != cosignerSourceLoaded, have < open`), so a build needing no
// cosigner cards at all was refused for want of a cosigner-card payload, and the
// refusal contradicted itself in one sentence: "No payload is loaded, and this
// policy needs no cosigner key cards ... pack the cards on the host". showError
// is dismiss-only and dismissing returns straight out of the build.
//
// Pre-S5 it was unreachable, because `open` was always p.N-1 >= 1. S5's
// multi-select @S picker is what made it reachable: an operator who holds every
// slot of a 2-of-2 and types both seeds on the keyboard needs no payload for
// anything.
//
// autoFill is the value the three states have to AGREE on, and it is the one
// `cosignerSourceLoaded` already yielded here, so the other two are brought to
// it rather than the reverse.
func classifyCosignerSupply(state cosignerSourceState, have, open int) buildCosignerOutcome {
	if open == 0 {
		return cosignerAutoFill
	}
	switch {
	case state != cosignerSourceLoaded, have < open:
		return cosignerRefuse
	case have == open:
		return cosignerAutoFill
	default:
		return cosignerSelect
	}
}

// buildSupplyRefusal is the under-supply message. REFUSALS MUST SPEAK PHASE-1
// LANGUAGE: the shipped ones tell the operator to *scan* a card, an instruction
// phase 1 removed with NFC, and a refusal that prescribes an impossible action
// is worse than a bare failure — the operator goes looking for a reader that is
// not there.
//
// The `0..n` ruling makes a ZERO-card payload a legitimate input, so it gets a
// row here too: a build that dead-ends on an empty payload with no named route
// is the same defect at the other end of the range.
//
// Under-supply stays a refusal (§0.1 clause 1): no authority licenses inventing
// a cosigner key, and no derived-cosigner mechanism exists before S5's slot
// model. What S1 owns is naming the route that does exist.
func buildSupplyRefusal(state cosignerSourceState, have, open int, incomplete bool) string {
	switch state {
	case cosignerSourceNoPayload:
		return fmt.Sprintf("No payload is loaded, and this policy needs %s. "+
			"This device has no card reader: pack the cards on the host with "+
			"`me sysw pack`, load the payload, then build.", cardCount(open))
	case cosignerSourceUncompared:
		return "The loaded payload has not been checked yet. Compare its digest " +
			"(or open it, if it is sealed) before its cosigner cards can be used."
	}
	msg := fmt.Sprintf("The payload holds %s; this policy needs %s.",
		cardCount(have), cardCount(open))
	if incomplete {
		msg += " Another card is missing some of its chunks, so it could not be assembled."
	}
	return msg + " Rewrite the payload on the host with `me sysw pack` and load it again."
}

// cardCount renders a cosigner-card count in words that read correctly at 0 and
// at 1, so the refusal never says "0 cosigner key cards" or "1 cards".
func cardCount(n int) string {
	switch n {
	case 0:
		return "no cosigner key cards"
	case 1:
		return "1 cosigner key card"
	default:
		return fmt.Sprintf("%d cosigner key cards", n)
	}
}

// buildPayloadCardsLines lists what the payload supplied. Numbered in payload
// record order, because that is the order the cards fill slots in and, on the
// over-supply arm, the number the picker refers to each card by.
//
// `selecting` says whether a picker follows, and only changes the closing line.
func buildPayloadCardsLines(cards []bundleCard, open int, selecting bool) []string {
	lines := []string{
		fmt.Sprintf("The payload supplied %s.", cardCount(len(cards))),
		fmt.Sprintf("This policy has %d open slot(s).", open),
	}
	for i, c := range cards {
		lines = append(lines, fmt.Sprintf("%d. %s", i+1, c.summary))
	}
	if selecting {
		lines = append(lines, fmt.Sprintf("Choose %d of them next; they fill the "+
			"slots in this order.", open))
	} else {
		lines = append(lines, "All of them fill the open slots, in this order.")
	}
	return lines
}

// buildPayloadReviewFlow is SPEC P0 item 6 — "the gather screen becomes a review
// of what the payload supplied, not a 'Scan a card' prompt", ruled there, not
// deferred — and it runs on BOTH arms.
//
// It used to live inside buildCosignerPickFlow, which meant it appeared only on
// OVER-supply. On the auto-fill arm the operator's entire review of what the
// payload supplied was the shared gather's "md1 descriptors: 0 / mk1 keys: 2 /
// Scan a card, or Done." — a count and an instruction phase-1 hardware cannot
// perform, which is this stage's own motivating defect surviving on the arm the
// stage reaches by default.
//
// This does NOT re-introduce selection on the auto-fill arm; the audit's
// "selection appears only on over-supply" still holds, and `selecting` is what
// keeps the two apart. A list of what arrived is a review, not a choice.
func buildPayloadReviewFlow(ctx *Context, th *Colors, cards []bundleCard, open int, selecting bool) bool {
	return confirmReviewScreen(ctx, th, "Payload cards",
		buildPayloadCardsLines(cards, open, selecting))
}

// buildCosignerPickFlow resolves OVER-SUPPLY. It returns the chosen card
// indices, always in PAYLOAD RECORD ORDER and always exactly `open` of them.
//
// SHAPE, ruled by the S1 assumption audit (row 1): bounded selection preserving
// payload record order among the selected. There is no reorder step — @N order
// is identity-bearing (md/encode_multisig.go's ordering contract: the same keys
// in a different order mint a different, equally valid policy), so a picker that
// let the operator assign cards to slots would be a second way to decide policy
// identity, and only one of them would be announced.
//
// It cannot end short, structurally: once the cards that remain are exactly the
// slots that remain, they are all taken without asking — a question with one
// possible answer is not a choice, and asking it is how an operator skips their
// way into an under-supply that was never real.
func buildCosignerPickFlow(ctx *Context, th *Colors, cards []bundleCard, open int) ([]int, bool) {
	chosen := make([]int, 0, open)
	for i := 0; i < len(cards) && len(chosen) < open; i++ {
		if len(cards)-i == open-len(chosen) {
			for ; i < len(cards); i++ {
				chosen = append(chosen, i)
			}
			break
		}
		cs := &ChoiceScreen{
			Title:   "Cosigner cards",
			Lead:    fmt.Sprintf("Use payload card %d of %d?", i+1, len(cards)),
			Choices: []string{"USE THIS CARD", "SKIP IT"},
		}
		sel, ok := cs.Choose(ctx, th)
		if !ok {
			// Back ABANDONS the Build flow, discarding the five picked
			// parameters — the same thing Back at the gather does, and NOT what
			// buildParamPickFlow does (that steps back exactly one stage). Said
			// plainly because the earlier comment claimed the parameter
			// pickers' behaviour, which is the opposite one.
			return nil, false
		}
		if sel == 0 {
			chosen = append(chosen, i)
		}
	}
	if len(chosen) != open {
		// Unreachable by the bound above; kept so a future edit to the loop
		// cannot silently hand a short set to the constructor.
		return nil, false
	}
	return chosen, true
}

// cosignerOrigin records which payload card filled which policy slot.
type cosignerOrigin struct {
	slot int // @N in the assembled policy
	card int // 1-based index into the payload's cosigner card list
}

// buildCosignerOrigins maps the chosen cards onto the slots assembleBuildPolicy
// will put them in: every slot except the operator's own, ascending, in the
// order the cards were chosen. It mirrors assembleBuildPolicy's own loop, and
// the S1 tests assert the two agree rather than trusting that they do.
// `p.SelfFromCard` is S4's `both` assignment: the operator's own slot is filled
// from a card too, so it is no longer skipped. It takes the whole `p` from S5,
// because the skip test is now set membership over `p.SelfSlots` -- and a wrong
// map silently mis-numbers every card in every refusal and on the provenance
// line.
func buildCosignerOrigins(p buildPolicyParams, chosen []int) []cosignerOrigin {
	held := make(map[int]bool, len(p.SelfSlots))
	for _, s := range p.SelfSlots {
		held[s] = true
	}
	out := make([]cosignerOrigin, 0, len(chosen))
	gi := 0
	for slot := 0; slot < p.N && gi < len(chosen); slot++ {
		if held[slot] && !p.SelfFromCard {
			continue
		}
		out = append(out, cosignerOrigin{slot: slot, card: chosen[gi] + 1})
		gi++
	}
	return out
}

// buildProvenanceLines are the §0.1 ANNOUNCEMENT: which slots were filled from
// which payload cards, printed on the policy review screen — the last surface
// before the EXPERIMENTAL warning and the engrave, which is where §0.1 clause 3
// requires an assumption to be announced.
//
// The assumption being announced is "the payload's cards fill the open slots, in
// payload record order". It is eligible to be assumed at all only because it is
// auditable: the resulting slot order is carried in the md1, shown as the
// per-slot list on this same screen, and printed on the restore doc. That makes
// it §0.1 clause 2 — printed in an artifact the operator keeps — rather than the
// invisible kind that must be refused.
func buildProvenanceLines(origins []cosignerOrigin, total int) []string {
	if len(origins) == 0 {
		return nil
	}
	slots := make([]string, len(origins))
	cards := make([]string, len(origins))
	for i, o := range origins {
		slots[i] = fmt.Sprintf("@%d", o.slot)
		cards[i] = fmt.Sprintf("%d", o.card)
	}
	noun := "cards"
	if len(origins) == 1 {
		noun = "card"
	}
	return []string{fmt.Sprintf("Slots %s filled from the payload (%s %s of %d, "+
		"in payload order).", joinAnd(slots), noun, joinAnd(cards), total)}
}

// joinAnd renders a short list as "a", "a and b", or "a, b and c".
func joinAnd(parts []string) string {
	switch len(parts) {
	case 0:
		return ""
	case 1:
		return parts[0]
	}
	out := parts[0]
	for _, p := range parts[1 : len(parts)-1] {
		out += ", " + p
	}
	return out + " and " + parts[len(parts)-1]
}
