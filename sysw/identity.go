package sysw

import "crypto/sha256"

const idLabel = "MNEMSYSW/id/v1"

// Identity is a payload's equality key, and is NOT the displayed digest.
//
// The §6.6 digest is displayed only when pub_len > 0 -- the digest of an empty
// record set is a constant -- so a secrets-only sealed payload has no such
// value. Using it as identity would give EVERY secrets-only payload one shared
// identity, and a payload swapped between two consumptions would inherit the
// previous one's compared flag.
//
// So identity hashes the REGION BYTES: it always exists, it is content-derived,
// and it covers the ciphertext, which the §6.6 digest deliberately does not.
//
// 32 bytes, not 16. This is never read by a human, so there is no reason to
// truncate an equality key.
func Identity(region []byte) [32]byte {
	h := sha256.New()
	h.Write([]byte(idLabel))
	h.Write([]byte{0x00})
	h.Write(region)
	return [32]byte(h.Sum(nil))
}
