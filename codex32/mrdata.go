package codex32

// mr1 (HRP "mr") is mnemonic-refugium's backup-card format. It reuses the same
// BIP-93 BCH(93,80,8) regular code and the same non-codex32 initial residue
// (mdmkPolymodInitLo) as md1/mk1/mt1, with its own NUMS target residue.
//
// PORT, NOT PRIMARY. The primary is mnemonic-refugium crates/mr-codec
// (bch.rs): the residue 0x085d21404aee15059 (65 bits, so hi = 0 and lo =
// 0x85d21404aee15059 as a 64-bit pair). SPEC_systemwide_payloads.md section
// 14b states the string rule this verifier is the checksum step of; the
// vendored sysw_mr_v1_vectors.json pins it.
//
// mr1 is REGULAR-CODE ONLY: 14 to 93 symbols after "mr1", checksum included.
const (
	mrRegularTargetHi uint64 = 0x0
	mrRegularTargetLo uint64 = 0x85d21404aee15059

	// The 13-symbol checksum plus at least one data symbol (section 14b's
	// MR_LENGTH: L under 14 is refused).
	mrMinDataLen = 14
	mrMaxDataLen = 93
)

// ValidMR reports whether s is a structurally valid, BCH-correct mr1 string.
// Pure verify, no error correction: the string is engraved verbatim, so a
// record needing repair is refused rather than corrected into steel damage.
// Consistent case only (the codex32 engine's rule); the header fields and the
// case policy of section 14b are the sysw package's to judge.
func ValidMR(s string) bool {
	_, data := splitHRP(s)
	if len(data) < mrMinDataLen || len(data) > mrMaxDataLen {
		return false
	}
	return verifyMDMK(s, "mr", newShortChecksum().generator,
		mrRegularTargetHi, mrRegularTargetLo, mdmkShortSyms)
}
