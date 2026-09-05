package sysw

import (
	"strings"

	"seedhammer.com/bip39"
	"seedhammer.com/codex32"
	"seedhammer.com/mt"
)

// MaxEngraveableMs1Len mirrors the Rust primary's MAX_ENGRAVEABLE_MS1_LEN
// (crates/me-cli/src/seal/record.rs). Above 90 characters the QR needed to
// carry it outgrows a plate, so a longer ms1 is not a secret this machine can
// back up and must not be classified as one.
const MaxEngraveableMs1Len = 90

// classifyConstellation places the record kinds the constellation already
// knows, mirroring the Rust primary's delegation to seal::record.
//
// The ORDER matters and mirrors the primary: the reserved prefixes are matched
// by Classify BEFORE this is reached, because free text is the universal
// fallback -- a sniffer running first would claim `text:...` records whose hex
// body happened to parse as something else.
//
// WHY THIS IS STRICTER THAN THE PACKAGES IT CALLS. `bip39.Parse` and
// `codex32.New` are ENTRY-oriented: forgiving is right when a human is typing
// on a 480x320 touchscreen, where uppercase, a three-letter prefix and a
// nearest-word correction are all kindnesses. Classification is a different
// job. Here the input is a payload someone else may have written, and being
// forgiving means the DEVICE hands a program a secret the HOST tool would have
// refused to touch. The pre-flash conformance review measured eight such
// strings; Rust is primary and none of them needed a Rust change, so the
// convergence is here.
func classifyConstellation(record string) Class {
	// Rust reaches these through seal::record::validate_record, which trims
	// first. Not trimming made the device reject md1 strings the host accepts --
	// the one row where Go was the STRICTER side.
	//
	// THIS TRIM IS UNICODE-WIDE and the descriptor arm's host is not, so the raw
	// record is kept: §4.6's normalisation is ASCII-only, and the arm answers
	// for its own rather than inheriting a wider one. See isDescriptorRecord.
	raw := record
	record = strings.TrimSpace(record)

	if isStrictMnemonic(record) {
		return ClassMnemonic
	}
	if isStrictMs1(record) {
		return ClassCodex32Secret
	}
	if codex32.ValidMD(record) || codex32.ValidMK(record) {
		return ClassMDMK
	}
	// Strict, like the rest of this function: exact BCH validity, no
	// correction, consistent case (codex32's engine), and mt.ParseHeader must
	// read a header or the string is not a chunk of anything.
	if codex32.ValidMT(record) {
		if _, err := mt.ParseHeader(record); err == nil {
			return ClassMt
		}
	}
	// SPEC_descriptor_input.md §5.2's descriptor record, and it is LAST for the
	// same reason the Rust primary puts it last (crates/me-cli/src/sysw/mod.rs):
	// only what would otherwise have fallen through to ClassUnknown can move, so
	// no record an earlier arm placed changes class. Arm order alone is not the
	// whole of that guarantee -- it protects records an earlier arm MATCHED, not
	// records that used to fall through -- which is why the seam file asserts
	// the class of every single-line row in BOTH directions.
	//
	// The predicate is isDescriptorRecord (sysw/descriptor.go), NOT
	// nonstandard.OutputDescriptor: the scan door admits 18 single-line strings
	// this refuses, and every one of them would reach a program and a screen.
	if isDescriptorRecord(raw, record) {
		return ClassDescriptor
	}
	return ClassUnknown
}

// isStrictMnemonic matches Rust's bip39::Mnemonic::parse_normalized: exact
// lowercase wordlist entries, and a real BIP-39 length. bip39.Parse alone
// accepts UPPERCASE, mixed case, >=3-character prefixes and any word count
// divisible by three -- so `abandon abandon about` classified as a SEED.
func isStrictMnemonic(record string) bool {
	words := strings.Split(record, " ")
	switch len(words) {
	case 12, 15, 18, 21, 24:
	default:
		return false
	}
	for _, w := range words {
		// Rust's parse_normalized takes lowercase only, so the CASE check is on
		// the input as given.
		if w == "" || w != strings.ToLower(w) {
			return false
		}
		// THE WORDLIST HERE IS UPPERCASE — LabelFor returns "ABANDON", not
		// "abandon", because the same table drives the touchscreen keyboard.
		// Querying with the raw lowercase word returns (-1,false) for every
		// valid seed, so the lookup is upper-cased and the comparison is
		// case-insensitive. Getting this backwards rejects EVERY real mnemonic,
		// which is how it was caught.
		idx, ok := bip39.ClosestWord(strings.ToUpper(w))
		// ClosestWord matches on PREFIX, so `aban` resolves to ABANDON and
		// reports ok; comparing the label back is what makes it an equality test.
		if !ok || !strings.EqualFold(bip39.LabelFor(idx), w) {
			return false
		}
	}
	m, err := bip39.ParseMnemonic(record)
	return err == nil && m.Valid()
}

// isStrictMs1 matches Rust: HRP `ms` only, and no longer than a plate can
// carry. codex32.New pins no HRP and has no engraveable cap, so a BCH-valid
// string with HRP `aa`, or a 127-character one, classified as a SECRET.
func isStrictMs1(record string) bool {
	if len(record) > MaxEngraveableMs1Len {
		return false
	}
	if !strings.HasPrefix(strings.ToLower(record), "ms1") {
		return false
	}
	c, err := codex32.New(record)
	// H0 (SPEC_ms_hashlock §9): a hashlock preimage plate is BCH-valid and
	// inside the cap, and it is not a seed. Inert here — no class of its own.
	return err == nil && !codex32.IsPreimage(c)
}
