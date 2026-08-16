package gui

import "fmt"

// ─── S4: tell the operator how many plates, before and after ─────────────────
//
// Trace B cuts 6-9 plates over hours. Before this the operator committed to that
// with no count, and afterwards neither they nor the person who finds the plates
// in five years could tell whether the set was complete. F-131 and F-132 are both
// cases where a backup document's SILENCE cost more than its errors.
//
// Both counts are DERIVED through bundlePlatePlan -- the same function
// bundleEngrave loops -- so neither can drift from what is actually cut.

// ─── The plate census and the set inventory ──────────────────────────────────

// plateWord renders a count with a singular or plural tail, so a census never
// reads "1 plates".
func plateWord(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", one)
	}
	return fmt.Sprintf("%d %s", n, many)
}

// buildPlateCensusLines tells the operator HOW MANY PLATES before the tail
// starts.
//
// Trace B cuts 6-9 plates over hours. Before this, the operator committed to
// that with no count, and neither they nor the person who finds the plates in
// five years could tell whether the set was complete. F-131 and F-132 are both
// cases where a backup document's SILENCE cost more than its errors.
//
// The count is DERIVED from the cards, through bundlePlatePlan -- the same
// function bundleEngrave loops -- so it cannot drift from what is actually cut.
func buildPlateCensusLines(cards []bundleCard) []string {
	plan := bundlePlatePlan(cards)
	lines := []string{fmt.Sprintf("This engraves %s.", plateWord(len(plan), "plate", "plates"))}
	for _, c := range cards {
		lines = append(lines, fmt.Sprintf("%s: %s (%s)",
			c.label, plateWord(len(c.strings), "plate", "plates"), c.summary))
	}
	lines = append(lines, "Each plate takes minutes to cut. Have that many blanks ready "+
		"before you start: a set is only a backup when all of it exists.")
	return lines
}

// buildPlateInventoryLines is the same census AFTER the fact, on the restore
// doc, for the reader who is not the operator and is not in the same decade.
//
// IT ALSO CARRIES THIS STAGE'S WALK-AWAY RULING. The build flow holds seed
// material in a registry with no time bound, and wipeGuard brackets only the
// unlock session, so an operator who walks away mid-build leaves it live. S4
// owns the registry and therefore rules the bound, and the ruling is the second
// arm the plan offers: this flow is NON-WIPING, like the rest of the systemwide
// surface (SYSW 3.2.1), and it says so here rather than leaving it silent.
//
// WHY NOT AN IDLE LIMIT. A timer that scrubs and exits mid-build would fire on
// the operator reading this very document, and would throw away a build that
// costs hours to redo. The registry today holds exactly one seed, which is what
// the shipped flow already held, so an idle limit would buy no reduction in
// exposure over the state of the tree. S5 multiplies the masters in it; the
// bound is filed to be re-decided there, when it would actually change something.
func buildPlateInventoryLines(cards []bundleCard, passphrase bool) []string {
	plan := bundlePlatePlan(cards)
	lines := []string{
		fmt.Sprintf("This backup is %s:", plateWord(len(plan), "plate", "plates")),
	}
	for _, c := range cards {
		lines = append(lines, fmt.Sprintf("%s: %s (%s)",
			c.label, plateWord(len(c.strings), "plate", "plates"), c.summary))
	}
	lines = append(lines, "If any of them is missing, this backup is incomplete.")
	lines = append(lines, buildPassphraseInventoryLines(passphrase)...)
	lines = append(lines, "Seed handling: this build does not time out. A seed you entered "+
		"stays in device memory until the build ends, like the rest of the payload "+
		"surface. Power the device off when you are done.")
	return lines
}

// ─── What is NOT on the plates ───────────────────────────────────────────────

// buildPassphraseInventoryLines states, on the restore document, whether a
// BIP-39 passphrase is part of this wallet and where it is not.
//
// THE PASSPHRASE IS A REQUIRED SPENDING FACTOR AND IS NEVER ENGRAVED. ms1 encodes
// the WORDS; the passphrase is not in that entropy and no plate in the set can be
// made to yield it. Before S5 neither the engrave-mode label nor this document
// said so -- measured, gui/multisig_restore.go contained zero occurrences of the
// word -- so a set labelled "Full (seed + keys)" could be missing the one factor
// that reaches the money and vouch for itself while doing it. F-132's device
// sibling exactly: that finding was a hashlock preimage required to spend, absent
// from the backup and unmentioned by it.
//
// BOTH ARMS SPEAK, and the second is not symmetry for its own sake. This document
// is read years later, alone, often by someone who was not the operator, holding
// a pile of steel and asking one question: is this everything? "No BIP-39
// passphrase was used" ANSWERS it. Silence leaves the reader unable to
// distinguish a complete backup from one whose operator forgot to write the
// passphrase down, and that is the state in which people give up on a recovery
// that would have worked.
//
// NEITHER ARM ASSUMES A SEED PLATE EXISTS. A watch-only build engraves no ms1 at
// all, so "the seed plate encodes the words only" and "these plates are the whole
// backup" -- both in the first draft of this text -- are false there. The claim is
// about the PASSPHRASE and is phrased to stay true in both modes; what the set
// does and does not contain is the inventory's job, immediately above.
func buildPassphraseInventoryLines(passphrase bool) []string {
	if !passphrase {
		return []string{
			"No BIP-39 passphrase was used, so no passphrase is needed to spend from " +
				"this wallet.",
		}
	}
	return []string{
		"A BIP-39 passphrase WAS used. It is not on these plates and cannot be " +
			"recovered from them: nothing this device engraves carries a passphrase.",
		"Without it, these plates do not reach the money. Keep it somewhere " +
			"separate, and make sure whoever needs this backup can also get the " +
			"passphrase.",
	}
}

// buildFullModeLabel is the engrave-mode picker's first row.
//
// "Full (seed + keys)" is correct for a build with no passphrase and is a LIE for
// one with a passphrase: what gets cut is the seed and the keys, and the third
// factor the wallet needs is left in the operator's head. The label is where it
// has to be said, because the label is what the operator reads before pressing --
// a note somewhere else is a note read after the decision.
//
// It is said ONLY when a passphrase was actually used. A picker that warns about
// a factor nobody supplied is §0.1's corollary in the other direction: a tool
// that cries DEFAULT when the operator chose is a tool whose warnings get
// ignored.
//
// The row does NOT wrap (ChoiceScreen.Draw uses widget.Label), so the longer
// label is measured against the panel rather than judged by eye --
// assertChoiceLabelFits, gui/multisig_build_prose_test.go.
func buildFullModeLabel(passphrase bool) string {
	if passphrase {
		return "Full (seed + keys, NOT passphrase)"
	}
	return "Full (seed + keys)"
}
