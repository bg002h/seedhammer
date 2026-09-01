package gui

import (
	"fmt"

	"seedhammer.com/engrave"
)

// ─── S4: tell the operator how many plates, before and after ─────────────────
//
// Trace B cuts 3-4 plates over hours -- 6-9 before F-423 packed each card's
// strings onto as many plates as fit. Before this the operator committed to that
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
// Trace B cuts 3-4 plates over hours (6-9 before F-423's packing). Before this,
// the operator committed to that with no count, and neither they nor the person
// who finds the plates in five years could tell whether the set was complete. F-131 and F-132 are both
// cases where a backup document's SILENCE cost more than its errors.
//
// The count is DERIVED from the cards, through bundlePlatePlan -- the same
// function bundleEngrave loops -- so it cannot drift from what is actually cut.
//
// THE PER-CARD COUNTS COME OFF THE SAME PLAN, not off len(c.strings). They were
// the same number until F-423 packed several strings onto a plate; reading the
// string count here now would tell the operator "md1 policy: 6 plates" over a
// total of 2, on the one screen whose whole job is how many blanks to have
// ready.
func buildPlateCensusLines(params engrave.Params, cards []bundleCard) []string {
	plan := bundlePlatePlan(params, cards)
	perCard := bundleCardPlateCounts(plan, len(cards))
	lines := []string{fmt.Sprintf("This engraves %s.", plateWord(len(plan), "plate", "plates"))}
	for i, c := range cards {
		lines = append(lines, fmt.Sprintf("%s: %s (%s)%s",
			c.label, plateWord(perCard[i], "plate", "plates"), c.summary, csidMarker(c)))
	}
	lines = append(lines, "Each plate takes minutes to cut. Have that many blanks ready "+
		"before you start: a set is only a backup when all of it exists.")
	return lines
}

// buildPlateInventoryLines is the same census AFTER the fact, on the restore
// doc, for the reader who is not the operator and is not in the same decade.
//
// IT ALSO CARRIES THIS STAGE'S WALK-AWAY RULING, which is assembled by
// buildSeedHandlingRuling below and keyed on the CAPACITY of the path that
// called here -- the shared function serves three flows, and they do not all
// hold the same number of seeds.
//
// THE ORDER IS what-it-is, what-it-is-NOT, how-to-handle-it: the plate list and
// its completeness claim, then what the set does and does not contain (the seed
// statement, then the passphrase statement), then the ruling.
//
// passphrasePlateCut is S6b spec §6/§6a: whether THIS RUN actually cut a
// separate passphrase plate -- CUT, not offered. It is read only by
// buildPassphraseInventoryLines, below, and NEVER by the plate list built
// here: spec §6.1 forbids the passphrase plate from entering `plan` (it is
// not a bundleCard) or appearing under "If any of them is missing", because
// either would tell a reader it travels WITH the set. Every caller but
// engraveSingleSigFlow passes false -- the multisig paths have no
// passphrase-plate offer at all (R-B).
func buildPlateInventoryLines(params engrave.Params, cards []bundleCard, seeds []seedPassphraseFact,
	capacity seedCapacity, passphrasePlateCut bool) []string {
	plan := bundlePlatePlan(params, cards)
	perCard := bundleCardPlateCounts(plan, len(cards))
	lines := []string{
		fmt.Sprintf("This backup is %s:", plateWord(len(plan), "plate", "plates")),
	}
	for i, c := range cards {
		lines = append(lines, fmt.Sprintf("%s: %s (%s)%s",
			c.label, plateWord(perCard[i], "plate", "plates"), c.summary, csidMarker(c)))
	}
	lines = append(lines, "If any of them is missing, this backup is incomplete.")
	lines = append(lines, buildSeedInventoryLines(cards)...)
	lines = append(lines, buildPassphraseInventoryLines(seeds, passphrasePlateCut)...)
	lines = append(lines, buildSeedHandlingRuling(capacity, bundleSetCarriesASecret(cards)))
	return lines
}

// bundleCardPlateCounts is how many plates each card contributes to plan.
//
// It COUNTS THE PLAN rather than re-running the packer, so the enumerated list
// and the total above it are two readings of one object and cannot disagree --
// which is the property this file's header claims and which len(c.strings)
// quietly stopped providing when plates started holding more than one string.
func bundleCardPlateCounts(plan []bundlePlate, cards int) []int {
	counts := make([]int, cards)
	for _, p := range plan {
		counts[p.cardIdx-1]++
	}
	return counts
}

// ─── The walk-away ruling, and the two axes it is assembled from ─────────────

// seedCapacity is how many seeds a PATH can hold, which is what the
// seed-handling ruling describes.
//
// IT IS DELIBERATELY NOT THE RUNTIME COUNT. A build that happened to take one
// seed can still hold several, and two otherwise identical builds must not
// print different documents because of runtime happenstance. The document
// describes the machine the operator was standing at, not the tally of that
// particular run.
type seedCapacity int

const (
	// seedCapacityOne is a single seed seam by construction: the single-sig
	// flow, and the multisig SUPPLY path, which derives everything it engraves
	// from one mnemonic.
	seedCapacityOne seedCapacity = iota
	// seedCapacityMany is the multisig BUILD path's registry, which holds one
	// seed per held slot, across distinct masters, for the length of the build.
	seedCapacityMany
)

// buildSeedHandlingRuling is the walk-away ruling. The build flow holds seed
// material with no time bound, and wipeGuard brackets only the unlock session,
// so an operator who walks away mid-build leaves it live. This flow is
// NON-WIPING, like the rest of the systemwide surface (SYSW 3.2.1), and it says
// so here rather than leaving it silent.
//
// WHY NOT AN IDLE LIMIT. A timer that scrubs and exits mid-build would fire on
// the operator reading this very document, and would throw away a build that
// costs hours to redo. S5 multiplied what the registry holds -- up to one seed
// per held slot, across distinct masters, for a build that grew to a dozen
// plates over hours -- and the re-decision filed to S5 is now made: STILL NO
// BOUND, on the new premise, stated honestly. On a full build the registry is
// not the marginal copy: the same entropy is in the ms1 engrave strings
// (immutable Go strings, unscrubbable) for the whole engrave, and it
// accumulates on the plates in the tray -- whoever reaches an unattended
// machine reads the steel, not the SRAM. An earlier scrub would protect only
// watch-only builds, against a live-SRAM-extraction attacker this air-gapped,
// physically-custodied device does not defend against anyway, and it would
// break the one-site scrub invariant: BIP-39 derivation is checksum-free
// PBKDF2, so a future read-after-scrub silently derives the all-"abandon"
// wallet. The walk-away exposure is answered where it lives: the ruling tells
// the operator what the machine is still holding until the build ends.
//
// TWO INDEPENDENT AXES, AND THE SECOND ONE IS NOT THE FIRST. The subject is a
// property of the PATH (seedCapacity). The two "plates" clauses are a property
// of THIS RUN, and they are FALSE on every watch-only run of every path: no ms1
// is engraved there, so "the plates are the secret" would sit a few lines under
// the inventory's own statement that no plate in this set holds the seed, and
// the document would contradict itself about the one thing it exists to settle.
// The device does hold seed material in memory in watch-only mode -- it derives
// from a mnemonic either way -- so the warning stays and only the steel half
// goes.
//
// "SEED MATERIAL", and two rejected drafts are worth naming. The shipped
// singular "your seed" is a singular/plural wobble on the multi-seed path. "The
// words you typed" fixes the wobble and introduces a FALSEHOOD: seedEntryFlow is
// a source picker, not a keyboard, and on a payload-sourced run nothing was
// typed. "Seed material" is number-neutral and provenance-neutral, so it is true
// on every path and from every source. A wording fix for a wording defect can
// introduce a factual one; any replacement string is new text and inherits the
// whole truth obligation.
//
// "ON A FULL BUILD" IS KEPT VERBATIM inside the seed-bearing arm even though
// that arm now fires only on seed-bearing builds, which makes the qualifier
// vestigial. Removing it would re-word an S5-reviewed sentence for no gain in
// truth, and byte-identity with reviewed text is worth more than tidiness.
func buildSeedHandlingRuling(capacity seedCapacity, seedOnPlates bool) string {
	subject := "The seed you entered -- this build holds exactly one --"
	if capacity == seedCapacityMany {
		subject = "Every seed you entered -- this build can hold several --"
	}
	base := "Seed handling: this build does not time out. " + subject +
		" stays in device memory until the build ends"
	if seedOnPlates {
		return base + ", and on a full build the words are also on the plates as " +
			"they are cut. Do not leave a mid-build machine unattended: the plates " +
			"are the secret. Power the device off when you are done."
	}
	return base + ". Do not leave a mid-build machine unattended: it is still " +
		"holding seed material. Power the device off when you are done."
}

// ─── What IS on the plates ───────────────────────────────────────────────────

// buildSeedInventoryLines states, on the restore document, whether a SEED is on
// the plates the reader is holding.
//
// THE SILENCE IT REPLACES COST BOTH DIRECTIONS. A watch-only set engraves no ms1
// at all, and nothing on the page said so, so a stranger holding a COMPLETE
// backup could only conclude that the seed plate had been lost -- which is the
// state in which recoverable backups get abandoned, the same failure mode
// buildPassphraseInventoryLines exists to prevent. On a seed-bearing set the
// document never said which plate must be treated as the secret itself.
//
// "YOUR seed", NOT "THE seed". The definite article, sitting directly under "If
// any of them is missing, this backup is incomplete.", answers the reader's one
// question -- is this everything? -- with YES. On a 2-of-3 that answer is false
// and costs the recovery. "Your seed" is true in every mode, and is already this
// codebase's own word for the fact (oneSeedPassphraseFact).
//
// NO SUFFICIENCY CLAIM IN EITHER DIRECTION. It does not say the seed plates
// alone can spend (false on k-of-n) and does not say they cannot (false on
// single-sig). It states presence and consequence and stops; how many keys must
// sign is the descriptor's job, on the same page. Nor does it claim the plates
// rebuild the wallet's ADDRESSES: on a policy shape expandedToDescriptor cannot
// render, the same document already says "Addresses unavailable for this policy
// shape.", and the two would contradict each other in print.
//
// THE SINGULAR AND PLURAL ARMS ARE CHOSEN BY THE ms1 CARD COUNT, which is a fact
// of THIS RUN and not of the path -- unlike seedCapacity, one function up.
// Keying it on capacity would print "Each plate marked 'ms1 secret share'" over
// the ORDINARY one-slot multisig build, where numberedLabel leaves the single
// card UNNUMBERED; a reader counting one plate against a document saying "each"
// concludes a plate is missing from a complete set, and stops.
//
// THE LABEL IS NAMED AS A PREFIX, not as an exact string: single-sig labels the
// card exactly "ms1 secret share" and multisig numbers it ("ms1 secret share 1
// of 2"), so "the plate marked 'ms1 secret share'" is true of both. A future
// relabel must propagate into these lines; the coupling is deliberate and
// greppable.
//
// ABSENCE IS ASKED THROUGH bundleSetCarriesASecret rather than through a second
// hand-written scan, so this document and the abort warning can never disagree
// about whether the set holds a seed.
//
// PASSPHRASE FACTS STAY OUT: that is the passphrase lines' job, immediately
// after.
func buildSeedInventoryLines(cards []bundleCard) []string {
	if !bundleSetCarriesASecret(cards) {
		return []string{
			"Seed: this set contains NO seed. It is watch-only: it records the " +
				"wallet, but it can never spend. If funds must be recovered, the seed " +
				"words must come from somewhere else -- no plate in this set holds them.",
		}
	}
	ms1s := 0
	for _, c := range cards {
		if c.kind == cardMS1 {
			ms1s++
		}
	}
	if ms1s == 1 {
		return []string{
			"Seed: this set contains YOUR seed, on the plate marked 'ms1 secret " +
				"share'. Treat that plate as the secret itself.",
		}
	}
	return []string{
		"Seed: this set contains YOUR seeds, on the plates marked 'ms1 secret " +
			"share'. Treat each of those plates as the secret itself.",
	}
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
//
// passphrasePlateCut is S6b spec §6/§6a (P4): whether THIS RUN cut a SEPARATE
// passphrase plate. Before S6b "nothing this device engraves carries a
// passphrase" was true unconditionally; S6b's own passphrase-plate offer
// (engraveSingleSigFlow) can make it false, on the SAME predicate that shows
// the offer -- so R-D forbids leaving it unconditional. The condition reads
// CUT, never "was the offer shown": a declined offer or an aborted engrave
// must render the shipped two lines exactly as they always did.
//
// WHEN CUT, THE REPLACEMENT DOES TWO JOBS AT ONCE, per spec §6.1: it retracts
// the false "nothing carries a passphrase" claim (§6), AND it says the
// passphrase plate is not one of the plates enumerated above and is not
// counted in this backup (§6.1) -- because the nearest alternative mechanism,
// giving it a bundleCard, would put it INSIDE that count and under "If any of
// them is missing", the exact inverse of the separation instruction this
// function already carries. "Keep it somewhere separate" is repeated
// verbatim rather than paraphrased, so the two arms read as one instruction
// applied twice: once to the passphrase itself, once to the plate that now
// carries it.
func buildPassphraseInventoryLines(seeds []seedPassphraseFact, passphrasePlateCut bool) []string {
	var passphrased, bare []seedPassphraseFact
	for _, s := range seeds {
		if s.Uses {
			passphrased = append(passphrased, s)
		} else {
			bare = append(bare, s)
		}
	}
	if len(passphrased) == 0 {
		return []string{
			"No BIP-39 passphrase was used, so no passphrase is needed to spend from " +
				"this wallet.",
		}
	}
	var lines []string
	if passphrasePlateCut {
		lines = []string{
			"A BIP-39 passphrase WAS used. It is not on these plates: this device " +
				"also cut a separate passphrase plate, engraved this run.",
			"That passphrase plate is not one of the plates listed above and is not " +
				"counted in this backup. Keep it somewhere separate, and make sure " +
				"whoever needs this backup can also get it.",
		}
	} else {
		lines = []string{
			"A BIP-39 passphrase WAS used. It is not on these plates and cannot be " +
				"recovered from them: nothing this device engraves carries a passphrase.",
			"Without it, these plates do not reach the money. Keep it somewhere " +
				"separate, and make sure whoever needs this backup can also get the " +
				"passphrase.",
		}
	}
	// ONE SEED: the shipped two lines (or, when a passphrase plate was cut this
	// run, their §6/§6.1 counterpart). Everything in them is singular and
	// everything singular is true, so a build with one master reads exactly as
	// it always did.
	if len(seeds) < 2 {
		return lines
	}
	// SEVERAL SEEDS: name them, because "the passphrase" is a phrase with one
	// referent and this backup has more than one. Every reference in the two lines
	// above is singular; an operator holding three slots who records "the
	// passphrase" and had typed two different ones loses the legs the other one
	// derives, silently, with the backup vouching for itself throughout.
	//
	// THE LABEL AND THE FINGERPRINT TOGETHER, because neither alone identifies the
	// seed to a reader in five years: the label says which slot it was entered
	// for, and the fingerprint is what a coordinator shows beside the key so the
	// reader can tell which ms1 plate the sentence is about.
	for _, s := range passphrased {
		lines = append(lines, fmt.Sprintf("Needs a passphrase: %s%s. If more than one "+
			"is listed here they may be DIFFERENT passphrases; record each one "+
			"against its fingerprint.", s.Label, seedFingerprintSuffix(s.MasterFP)))
	}
	// AND THE BARE ONES ARE SAID OUT LOUD. Silence here reads as "all of them need
	// it", which sends a reader hunting for a passphrase that never existed for
	// that seed -- and, worse, lets them conclude the one passphrase they have is
	// the only factor missing.
	for _, s := range bare {
		lines = append(lines, fmt.Sprintf("Needs NO passphrase: %s%s.",
			s.Label, seedFingerprintSuffix(s.MasterFP)))
	}
	return lines
}

// seedFingerprintSuffix renders " (master fingerprint xxxxxxxx)", or nothing at
// all when the fingerprint is not known.
//
// A ZERO FINGERPRINT IS NOT PRINTED AS 00000000. Every fact the BUILD path emits
// carries a real one -- seedRegistry.add derives it at registration and refuses a
// seed whose master key cannot be built -- so this arm is for a caller that has
// none to give, and printing a placeholder on a document read years later would
// invite the reader to look for a key with that fingerprint.
func seedFingerprintSuffix(fp uint32) string {
	if fp == 0 {
		return ""
	}
	return fmt.Sprintf(" (master fingerprint %08x)", fp)
}

// oneSeedPassphraseFact is the single-seed caller's fact.
//
// It exists so a flow with ONE seed says so in one place rather than
// hand-building a struct literal with a zero fingerprint at every call site --
// which is the shape that makes a reader wonder whether the zero is meaningful.
// With one seed there is nothing to tell apart, so the inventory's single-seed
// arm renders no fingerprint and none is needed.
func oneSeedPassphraseFact(uses bool) []seedPassphraseFact {
	return []seedPassphraseFact{{Label: "your seed", Uses: uses}}
}

// seedPassphraseFact is one registered (seed, passphrase) pair's passphrase
// status, WITHOUT the secret: a label, the pair's master fingerprint, and
// whether a passphrase is part of it.
//
// IT REPLACES A BOOLEAN, and the boolean was seedRegistry.usesPassphrase() --
// an any() over the registry, whose single bit was the only passphrase signal
// reaching either operator-facing surface. SPEC 4.1 makes the (seed, passphrase)
// pair the derivation unit and asks the passphrase PER SEED, so a build holding
// @0 and @1 can carry `alpha` and `beta`; the document then read "A BIP-39
// passphrase WAS used ... Without IT ... Keep IT somewhere separate", every
// reference singular, immediately after asserting "If any of them is missing,
// this backup is incomplete." Nothing on steel or on that page let a reader
// learn a SECOND passphrase exists. The commoner and equally fatal form is two
// different masters with one passphrased, where the document could not say WHICH
// of "ms1 secret share 1 of 2" / "2 of 2" needs it.
//
// NO MNEMONIC AND NO PASSPHRASE TEXT: this crosses into a display path, and the
// only things a display path may hold are the ones an operator is meant to read.
type seedPassphraseFact struct {
	// Label is what the screens already call this seed ("your seed for @0").
	Label string
	// MasterFP is the fingerprint of the PAIR, which is what distinguishes two
	// entries holding the same words under different passphrases.
	MasterFP uint32
	// Uses reports whether this pair carries a passphrase. An EXPLICITLY BOUND
	// EMPTY passphrase is no passphrase: syswPassphraseFlowTitled can return
	// ("", true), and a build that engraves a plain seed must not be labelled as
	// though a factor were missing.
	Uses bool
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
