package gui

import (
	"seedhammer.com/backup"
	"seedhammer.com/engrave"
	"seedhammer.com/font/constant"
)

// The composer's plate census (SPEC §7f).
//
// IT REUSES buildPlateCensusLines (gui/multisig_build_census.go:63), whose
// counts are DERIVED through bundlePlatePlan -- the same function
// bundleEngrave loops -- so they cannot drift from what is actually cut. The
// composer contributes the cards and counts nothing itself. That matters more
// here than in Multisig Build: appending both stubs can push a card into a
// THIRD chunk (mk/encode.go:26-29), so a count taken before minting would be
// short by exactly the plates the composer added.
//
// THE DESCRIPTOR CEILING IS A SEARCH, NEVER A CONSTANT, and qrCeilingBytes
// says why on the QR side (gui/transaction.go:1361-1367): the answer depends
// on plate geometry, stroke width and the encoder's own choices, so a
// constant goes stale the first time any of them moves -- silently, inside a
// refusal message nobody reads until the day it matters.

// composerDescriptorPlateFits asks the REAL thing whether a descriptor fits
// one plate: build the plate, plan the engraving, let toPlate reject
// overflow. The same one-source-of-truth rule planTransactionTextPlates
// states for its own packing (gui/transaction.go:1153-1156).
func composerDescriptorPlateFits(pl Platform, text string) bool {
	params := pl.EngraverParams()
	plate := backup.Text{
		Paragraphs: []backup.Paragraph{{Text: text}},
		Font:       constant.Font,
	}
	plan, err := backup.EngraveText(params, plate)
	if err != nil {
		return false
	}
	_, err = toPlate(plan, params)
	return err == nil
}

// composerDescriptorCeilingChars is the largest descriptor that COULD have
// fitted, found by the same doubling-then-bisecting search qrCeilingBytes
// uses (gui/transaction.go:1381-1397). Called only on the refusal path, so
// its cost never lands on a working cut.
func composerDescriptorCeilingChars(pl Platform) int {
	fits := func(n int) bool {
		b := make([]byte, n)
		for i := range b {
			b[i] = 'a'
		}
		return composerDescriptorPlateFits(pl, string(b))
	}
	if !fits(1) {
		return 0
	}
	lo, hi := 1, 2
	for hi < 1<<16 && fits(hi) {
		lo, hi = hi, hi*2
	}
	for lo+1 < hi {
		mid := (lo + hi) / 2
		if fits(mid) {
			lo = mid
		} else {
			hi = mid
		}
	}
	return lo
}

// THE CENSUS REFUSAL LIVES WITH THE PLATE IT REFUSES, and that plate is
// deferred. §7f's "the census REFUSES a concrete descriptor longer than the
// plate holds" belongs to form A's TEXT plates -- and md deliberately emits no
// descriptor text ("a rendering that cannot be re-parsed is the defect this
// package's invariant exists to prevent"), so form A ships as the keyed md1
// this cycle and the text/QR plates are F-457, Rust-first. A refusal function
// with nothing to refuse is the same defect class as a picker whose answer
// nothing reads, so it is not carried here. The two measurement functions
// above stay: §13 item 1 asks for the ceiling as a number, and that is what
// TestComposerMeasureSection13Numbers records.

// composerCensusLines is the census the operator confirms before the first
// cut, plus §7f's read-back-integrity line.
func composerCensusLines(params engrave.Params, cards []bundleCard) []string {
	lines := buildPlateCensusLines(params, cards)
	// RECOVERY-TIME ERROR DETECTION DIFFERS BY FORM AND THE CENSUS SAYS SO
	// (§7f). md1 and mk1 carry BCH; a text or QR descriptor carries only its
	// BIP-380 checksum, which detects a typo and corrects nothing.
	return append(lines, "",
		"md1 and mk1 plates carry error correction. A plain descriptor plate "+
			"carries only its checksum, which finds a mistake but cannot fix one.")
}
