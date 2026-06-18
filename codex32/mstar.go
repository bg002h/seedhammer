package codex32

import "strings"

// MStarInWindow reports whether an in-progress m*1 fragment is at a valid LENGTH
// for its HRP's code(s) — the per-HRP window that arms "bad checksum" feedback
// and the on-demand correction ("Fix?") affordance. ms uses total-string windows
// (48..93 / 125..127); md/mk use data-part windows (the chars after "xx1"): md
// data ≥13 (no upper bound), mk data 14..93 (regular) or 96..108 (long), with
// 94..95 reserved-invalid. Advisory; New/ValidMD/ValidMK remain the validity
// authority. (Phase B; SPEC §4.1(b).)
func MStarInWindow(frag string) bool {
	hrp, data := splitHRP(frag)
	switch {
	case strings.EqualFold(hrp, "ms"):
		n := len(frag)
		return (n >= shortCodeMinLength && n <= shortCodeMaxLength) ||
			(n >= longCodeMinLength && n <= longCodeMaxLength)
	case strings.EqualFold(hrp, "md"):
		return len(data) >= mdmkShortSyms
	case strings.EqualFold(hrp, "mk"):
		return (len(data) >= mkRegularMinLen && len(data) <= mkRegularMaxLen) ||
			(len(data) >= mkLongMinLen && len(data) <= mkLongMaxLen)
	}
	return false
}
