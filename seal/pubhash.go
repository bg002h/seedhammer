package seal

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// §6.6 — the fixed public-data hash.
//
// When a payload has no encrypted section it has no key, so nothing
// authenticates it (§6.1a). What stands in place of a tag is this: a hash of
// the public data that the operator compares against a value they recorded
// themselves — an out-of-band check the attacker does not control.
//
// COMPUTED, NEVER STORED. There is deliberately no hash field on the wire: an
// attacker who rewrites the records rewrites the hash beside them, and the
// device would display a value matching the tampered data perfectly. The check
// works only because the device derives the number from the bytes it is about
// to engrave.
//
// 128 bits, not 64. The attacker grinds a MATCH, not a preimage, over fields
// nothing binds to their key — origin paths, parent fingerprints, record order
// (which also enables SHA-256 midstate reuse). A candidate costs one to two
// SHA-256 compressions rather than a key derivation, so 2^64 is $60k–$250k of
// rented GPU.

const pubHashLabel = "MNEMBLOB/pub/v1"

// PublicDataHash returns
// SHA-256(LABEL ‖ 0x00 ‖ sealed ‖ public_record_count ‖ input)[:16], where
// input is the public section exactly as it appears on the wire: canonical
// lowercase records, LF-joined, no trailing LF (§6.4).
//
// Two inputs are load-bearing and neither is decoration:
//
//   - sealed is what makes a downgrade VISIBLE. Without it, stripping the
//     ciphertext and tag yields a payload that satisfies every §6.2 rule,
//     prompts for no passphrase, and displays the same hash the operator
//     recorded. Vectors D and E pin this: same five public records, different
//     digests.
//
//   - records is the count of records in the PUBLIC section — NOT §6.4's 1..24
//     cap, which counts both sections. Vector D is 5 public of 6 total and the
//     two produce different digests, so a host counting totals and a device
//     counting public records would disagree on every mixed payload, teaching
//     the operator that mismatches are normal.
//
// It is order-sensitive, because record order is plate order and that is
// content.
func PublicDataHash(records []string, sealed bool) [16]byte {
	h := sha256.New()
	h.Write([]byte(pubHashLabel))
	h.Write([]byte{0x00})
	if sealed {
		h.Write([]byte{0x01})
	} else {
		h.Write([]byte{0x00})
	}
	h.Write([]byte{byte(len(records))})
	h.Write([]byte(strings.Join(records, "\n")))
	var out [16]byte
	copy(out[:], h.Sum(nil))
	return out
}

// FormatHash renders the digest as `a26e d22b b747 dfd0 2367 06ad 14c1 9679` —
// eight groups of four, so a human can compare it against what they wrote down.
func FormatHash(h [16]byte) string {
	s := hex.EncodeToString(h[:])
	var b strings.Builder
	b.Grow(len(s) + 7)
	for i := 0; i < 8; i++ {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(s[i*4 : i*4+4])
	}
	return b.String()
}
