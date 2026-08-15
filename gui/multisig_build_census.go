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
func buildPlateInventoryLines(cards []bundleCard) []string {
	plan := bundlePlatePlan(cards)
	lines := []string{
		fmt.Sprintf("This backup is %s:", plateWord(len(plan), "plate", "plates")),
	}
	for _, c := range cards {
		lines = append(lines, fmt.Sprintf("%s: %s (%s)",
			c.label, plateWord(len(c.strings), "plate", "plates"), c.summary))
	}
	lines = append(lines, "If any of them is missing, this backup is incomplete.")
	lines = append(lines, "Seed handling: this build does not time out. A seed you entered "+
		"stays in device memory until the build ends, like the rest of the payload "+
		"surface. Power the device off when you are done.")
	return lines
}
