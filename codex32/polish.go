// Package codex32 polish helpers (Cycle A1): a fail-soft partial parser and an
// error classifier for on-device input feedback, plus exported length bounds.
// These are advisory: New remains the sole validity authority.
package codex32

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
