package codex32

import "errors"

// mt1 (HRP "mt") is the signed-transaction sibling of md1/mk1. It reuses the
// same BIP-93 BCH(93,80,8) regular code, the same non-codex32 initial residue
// (mdmkPolymodInitLo), and its own NUMS target residue.
//
// PORT, NOT PRIMARY. The primary is mnemonic-transaction/crates/mt-codec
// (consts.rs): MT_REGULAR_CONST = 0x0001a2fc877f9528d7c1, the top 65 bits of
// SHA-256("shibbolethnumstransaction"). Split hi/lo for unpackSyms exactly as
// the md/mk targets are; TestMTTargetReproducesFromDomain re-derives it so a
// copied-wrong constant cannot survive.
//
// mt1 is REGULAR-CODE ONLY, like md1: the mt chunking rule caps a data part at
// 88 symbols (11 header + ceil(40*8/5) payload + 13 checksum) against the
// regular code's 93-symbol domain, so the long code is unreachable
// (mt-codec consts.rs, compile-time assertion).
const (
	mtRegularTargetHi uint64 = 0x1
	mtRegularTargetLo uint64 = 0xa2fc877f9528d7c1

	// The 11-symbol chunk header (version 5 | chunk_set_id 20 | count-1 15 |
	// index 15 bits) plus the 13-symbol checksum: nothing shorter can be an
	// mt1 string.
	mtMinDataLen = 11 + 13
	mtMaxDataLen = 93
)

// ValidMT reports whether s is a structurally valid, BCH-correct mt1 string.
// Pure verify, no error correction — the string is engraved verbatim, so a
// record needing repair is refused rather than corrected into steel damage.
// Consistent case only (the codex32 engine's rule); the header fields are the
// mt package's to judge.
func ValidMT(s string) bool {
	_, data := splitHRP(s)
	if len(data) < mtMinDataLen || len(data) > mtMaxDataLen {
		return false
	}
	return verifyMDMK(s, "mt", newShortChecksum().generator,
		mtRegularTargetHi, mtRegularTargetLo, mdmkShortSyms)
}

// errNotMT1 is returned by MTDataSymbols for any string that is not a
// BCH-valid mt1 string.
var errNotMT1 = errors.New("codex32: not a valid mt1 string")

// MTDataSymbols returns the 5-bit data symbols of a BCH-valid mt1 string —
// the 11-symbol chunk header followed by the bytes_to_5bit-encoded payload —
// with the 13-symbol BCH checksum stripped. Mirrors MKDataSymbols.
func MTDataSymbols(s string) ([]byte, error) {
	if !ValidMT(s) {
		return nil, errNotMT1
	}
	_, data := splitHRP(s)
	body := data[:len(data)-mdmkShortSyms]
	syms := make([]byte, 0, len(body))
	for _, c := range body {
		e, ok := feFromRune(c)
		if !ok {
			return nil, errNotMT1 // unreachable: ValidMT verified the charset.
		}
		syms = append(syms, byte(e))
	}
	return syms, nil
}
