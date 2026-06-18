// Package codex32 polish helpers (Cycle A1): a fail-soft partial parser and an
// error classifier for on-device input feedback, plus exported length bounds.
// These are advisory: New remains the sole validity authority.
package codex32

import "errors"

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
	default:
		return "invalid"
	}
}
