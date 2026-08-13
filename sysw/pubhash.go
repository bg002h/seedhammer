package sysw

import (
	"crypto/sha256"
	"fmt"
	"strings"
)

// pubLabel differs from MNEMBLOB's by design: that label exists to stop
// cross-context collisions, and the two containers must be distinguishable.
// Reusing it would make two containers the spec separates produce identical
// digests over identical public sections.
const pubLabel = "MNEMSYSW/pub/v1"

// PublicDataHash is EPD §6.6's construction with this container's label.
//
// COMPUTED, NEVER STORED: a hash carried inside the payload is rewritten by
// whoever rewrites the records, and the device would display a value matching
// the tampered data perfectly.
//
// `sealed` is bound in so a DOWNGRADE is visible -- stripping the ciphertext
// changes the digest rather than preserving it. `count` is the number of PUBLIC
// records, so a removed record is visible and not merely a changed one.
func PublicDataHash(records []string, sealed bool) [16]byte {
	h := sha256.New()
	h.Write([]byte(pubLabel))
	h.Write([]byte{0x00})
	if sealed {
		h.Write([]byte{0x01})
	} else {
		h.Write([]byte{0x00})
	}
	h.Write([]byte{byte(len(records))})
	h.Write([]byte(strings.Join(records, "\n")))
	var out [16]byte
	copy(out[:], h.Sum(nil)[:16])
	return out
}

// FormatHash groups in fours so a human can compare it without losing their
// place.
func FormatHash(h [16]byte) string {
	var b strings.Builder
	for i := 0; i < 8; i++ {
		if i > 0 {
			b.WriteByte(' ')
		}
		fmt.Fprintf(&b, "%02x%02x", h[i*2], h[i*2+1])
	}
	return b.String()
}
