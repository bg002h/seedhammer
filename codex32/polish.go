// Package codex32 polish helpers (Cycle A1): a fail-soft partial parser and an
// error classifier for on-device input feedback, plus exported length bounds.
// These are advisory: New remains the sole validity authority.
package codex32

import (
	"errors"
	"fmt"
)

// Exported codex32 total-length bounds (BIP-93 / firmware gate). A valid string
// is in [ShortCodeMinLength, ShortCodeMaxLength] (short checksum) or
// [LongCodeMinLength, LongCodeMaxLength] (long checksum). 94..124 is never valid
// (BIP-93: "a data part of 94 or 95 characters is never legal"); the firmware's
// long window is the conservative subset 125..127.
const (
	ShortCodeMinLength = shortCodeMinLength // 48
	ShortCodeMaxLength = shortCodeMaxLength // 93
	LongCodeMinLength  = longCodeMinLength  // 125
	LongCodeMaxLength  = longCodeMaxLength  // 127
)

// Describe returns a short human-readable label for an error returned by New,
// suitable for on-device display, or "" for a nil error. Unknown non-nil errors
// map to "invalid".
func Describe(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, errInvalidChecksum):
		return "bad checksum"
	case errors.Is(err, errInvalidLength):
		return "wrong length"
	case errors.Is(err, errInvalidCharacter):
		return "invalid character"
	case errors.Is(err, errInvalidCase):
		return "mixed case"
	case errors.Is(err, errInvalidThreshold):
		return "bad threshold"
	case errors.Is(err, errInvalidShareIndex):
		return "bad share index"
	case errors.Is(err, errIncompleteGroup):
		return "incomplete group"
	case errors.Is(err, errMismatchedLength):
		return "shares differ in length"
	case errors.Is(err, errMismatchedHRP):
		return "mismatched type"
	case errors.Is(err, errMismatchedThreshold):
		return "mismatched threshold"
	case errors.Is(err, errMismatchedID):
		return "mismatched id"
	case errors.Is(err, errRepeatedIndex):
		return "repeated share"
	case errors.Is(err, errInsufficientShares):
		return "need more shares"
	default:
		return "invalid"
	}
}

// Fields holds the codex32 header fields determinable from an in-progress
// fragment. Each XxxKnown flag is true once that field is present and valid.
type Fields struct {
	HRP             string // "" until the '1' separator is seen
	Threshold       int    // 0,2..9; valid only if ThresholdKnown
	ThresholdKnown  bool
	Identifier      string // up to 4 chars; valid only if IdentifierKnown
	IdentifierKnown bool
	ShareIndex      rune // valid only if ShareIndexKnown
	ShareIndexKnown bool
	Unshared        bool // ShareIndexKnown && ShareIndex is 's'/'S'
}

// ParsePrefix fail-soft-parses the determinable header fields of an in-progress
// codex32 fragment without panicking. The returned error is non-nil ONLY for a
// determinable violation (mixed case, non-bech32 character, bad threshold digit,
// or threshold-0 without index S); a merely-too-short fragment returns
// (partialFields, nil). It never splits payload/checksum — their boundary
// depends on the final total length. Errors are the same sentinels New uses
// (wrapped with %w), so Describe maps them. Advisory only: New stays the sole
// validity authority.
func ParsePrefix(frag string) (Fields, error) {
	var f Fields
	// When splitHRP finds no '1', it returns ("", frag) — so `data` aliases the
	// ENTIRE input in that case. We early-return below before touching `data`,
	// so no field is read from the not-yet-data prefix.
	hrp, data := splitHRP(frag)
	// Case consistency (HRP + data) is determinable at any length.
	if err := checkCase(frag); err != nil {
		return f, fmt.Errorf("codex32: %w", err)
	}
	if hrp == "" {
		// No '1' separator yet: the typed chars are HRP candidates, not data.
		return f, nil
	}
	// HRP is recorded for display, not independently rejected: New is the
	// authority and surfaces a wrong HRP as a checksum mismatch (it folds the
	// HRP into the checksum), so ParsePrefix stays consistent with New.
	f.HRP = hrp

	// Threshold: data[0] at len>=1 (∈ {0,2..9}; '1' and non-digits are invalid).
	if len(data) >= 1 {
		switch data[0] {
		case '0', '2', '3', '4', '5', '6', '7', '8', '9':
			f.Threshold = int(data[0] - '0')
			f.ThresholdKnown = true
		default:
			return f, fmt.Errorf("codex32: %w", errInvalidThreshold)
		}
	}

	// Identifier: data[1:5] at len>=5; each char must be bech32.
	if len(data) >= 5 {
		for _, c := range data[1:5] {
			if _, ok := feFromRune(c); !ok {
				return f, fmt.Errorf("codex32: %w", errInvalidCharacter)
			}
		}
		f.Identifier = data[1:5]
		f.IdentifierKnown = true
	}

	// Share index: data[5] at len>=6; bech32; threshold-0 ⇒ index s/S.
	if len(data) >= 6 {
		// data[5] is a byte; rune(byte) is the codepoint for ASCII (all valid
		// bech32 is ASCII). Non-ASCII bytes (128..255) make feFromRune return
		// false below → "invalid character", never a panic.
		idx := rune(data[5])
		if _, ok := feFromRune(idx); !ok {
			return f, fmt.Errorf("codex32: %w", errInvalidCharacter)
		}
		f.ShareIndex = idx
		f.ShareIndexKnown = true
		f.Unshared = idx == 's' || idx == 'S'
		if f.ThresholdKnown && f.Threshold == 0 && !f.Unshared {
			return f, fmt.Errorf("codex32: %w", errInvalidShareIndex)
		}
	}
	return f, nil
}

// ConsistentShares reports whether a set of codex32 shares can belong to one
// recovery set: all share the same HRP, threshold, identifier, and total length,
// and all share indices are distinct. It does NOT require the set to be complete
// (k shares) — use it to validate shares as they are collected. Returns the same
// sentinels Interpolate uses (errMismatched{Length,HRP,Threshold,ID},
// errRepeatedIndex), so Describe maps them. A set of 0 or 1 share is consistent.
//
// Each share MUST already be New-valid: ConsistentShares calls the unexported
// parts(), which PANICS on a malformed String. Callers must only pass strings
// that passed New without error (the keypad gates the OK button on New==nil).
func ConsistentShares(shares []String) error {
	if len(shares) <= 1 {
		return nil
	}
	s0 := shares[0].parts()
	for _, share := range shares {
		p := share.parts()
		switch {
		case len(shares[0].s) != len(share.s):
			return errMismatchedLength
		case s0.hrp != p.hrp:
			return errMismatchedHRP
		case s0.threshold != p.threshold:
			return errMismatchedThreshold
		case s0.id != p.id:
			return errMismatchedID
		}
	}
	seen := make(map[fe]bool, len(shares))
	for _, share := range shares {
		idx := share.parts().shareIdx
		if seen[idx] {
			return errRepeatedIndex
		}
		seen[idx] = true
	}
	return nil
}

// checkCase returns errInvalidCase if frag mixes upper- and lower-case ASCII
// letters (digits are case-neutral) — matching the engine's case rule, for
// display honesty. Moot on the force-uppercasing keypad, but the package API
// should not silently accept mixed case.
func checkCase(frag string) error {
	hasUpper, hasLower := false, false
	for _, c := range frag {
		switch {
		case c >= 'a' && c <= 'z':
			hasLower = true
		case c >= 'A' && c <= 'Z':
			hasUpper = true
		}
	}
	if hasUpper && hasLower {
		return errInvalidCase
	}
	return nil
}
