package gui

import (
	"fmt"
	"strings"

	"seedhammer.com/codex32"
)

// codex32StatusLine returns the window-aware length readout for an in-progress
// codex32 fragment of length n. There is no single target: BIP-93 short totals
// are 48..93, the firmware long window is 125..127, and 94..124 is a dead zone
// that is not (yet) an error.
func codex32StatusLine(n int) string {
	switch {
	case n < codex32.ShortCodeMinLength:
		return fmt.Sprintf("%d chars", n)
	case n <= codex32.ShortCodeMaxLength:
		return fmt.Sprintf("short · %d chars", n)
	case n < codex32.LongCodeMinLength:
		return fmt.Sprintf("%d chars — keep typing", n)
	case n <= codex32.LongCodeMaxLength:
		return fmt.Sprintf("long · %d chars", n)
	default:
		return "too long"
	}
}

// codex32FieldLine renders the parsed header fields as "id NAME · thr 2 · share C",
// each segment appearing once its field is known. Returns "" if nothing is known.
func codex32FieldLine(f codex32.Fields) string {
	var segs []string
	if f.IdentifierKnown {
		segs = append(segs, "id "+strings.ToUpper(f.Identifier))
	}
	if f.ThresholdKnown {
		segs = append(segs, fmt.Sprintf("thr %d", f.Threshold))
	}
	if f.ShareIndexKnown {
		segs = append(segs, "share "+strings.ToUpper(string(f.ShareIndex)))
	}
	return strings.Join(segs, " · ")
}

// codex32Feedback returns an error label to show under the entry, or "" if the
// fragment is fine so far. Field errors (from ParsePrefix) show eagerly; a
// checksum/structure error from New shows only once the fragment reaches a valid
// length window (so a half-typed string isn't flagged "wrong length").
func codex32Feedback(frag string, perr, nerr error) string {
	if perr != nil {
		return codex32.Describe(perr)
	}
	n := len(frag)
	inWindow := (n >= codex32.ShortCodeMinLength && n <= codex32.ShortCodeMaxLength) ||
		(n >= codex32.LongCodeMinLength && n <= codex32.LongCodeMaxLength)
	if inWindow && nerr != nil {
		return codex32.Describe(nerr)
	}
	return ""
}
