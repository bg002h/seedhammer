package seal

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/pbkdf2"
	"crypto/sha256"
	"errors"
	"fmt"
)

// §7 — PBKDF2-HMAC-SHA256 into AES-256-GCM.
//
// The device only ever OPENS. seal_bytes is deliberately NOT ported: a device
// that can seal is a device that can be made to emit a payload, and nothing in
// §10 needs it.
//
// crypto/pbkdf2 and crypto/sha256 are already linked and already CALLED —
// golang.org/x/crypto/pbkdf2, which bip39 and slip39 import, is a thin wrapper
// over crypto/pbkdf2. crypto/aes and crypto/cipher are ABSENT from today's
// firmware build; importing them is what makes AES-GCM callable and costs the
// measured ~1.6 KB.

// KeyLen is 32 bytes — AES-256.
const KeyLen = 32

var (
	// ErrAuthentication is a tag mismatch: a wrong passphrase, OR an altered
	// payload. Phase B must offer the operator BOTH readings. Reporting only
	// "wrong passphrase" invites them to retype three times and conclude the
	// blob is corrupt, losing the one signal §2.2 item 4 exists to raise —
	// and because the public section is inside the AAD (§6.1a), this is also
	// what a tampered public card looks like.
	ErrAuthentication = errors.New("seal: wrong passphrase, or this payload has been altered")
	ErrSealedTooShort = errors.New("seal: sealed section is shorter than the tag")
)

// DeriveKey runs PBKDF2-HMAC-SHA256 over the §8.1-normalised passphrase.
//
// iterations ALWAYS comes from the header — never a constant, or vector B
// fails. Its signature is the injectable seam Open uses (see openSeam in
// open.go), which is what makes "no KDF ran" observable in a test.
func DeriveKey(passphrase string, salt []byte, iterations int) []byte {
	key, err := pbkdf2.Key(sha256.New, passphrase, salt, iterations, KeyLen)
	if err != nil {
		// Only reachable with an out-of-range iteration count or key length,
		// and ParseHeader has already excluded both (§6.2 bounds iterations
		// to [100_000, 2_000_000] before any KDF work). Return nil rather
		// than panic: on a device a panic is a brick, while a nil key makes
		// aes.NewCipher fail and the payload fails closed. An all-zero key
		// would be worse — it is a VALID AES key and hides the fault.
		return nil
	}
	return key
}

// Open verifies then decrypts.
//
// aad is `header ‖ public section` = blob[:HeaderLen+pub_len] (§6.1a), and
// sealed is blob[HeaderLen+pub_len:] INCLUDING the trailing 16-byte tag. Go's
// gcm.Open expects the tag appended to the ciphertext, which is exactly the
// wire layout — do not slice it off.
//
// Fail closed: no plaintext is ever returned on a tag mismatch. Go's Open
// already guarantees that; there is deliberately no path here that inspects a
// partial result.
func Open(key, iv, aad, sealed []byte) ([]byte, error) {
	if len(sealed) < TagLen {
		return nil, fmt.Errorf("%w: %d bytes, tag alone is %d", ErrSealedTooShort, len(sealed), TagLen)
	}
	// Checked rather than assumed: cipher.NewGCM fixes the nonce at 12 bytes
	// and gcm.Open PANICS on any other length. The IV always arrives as
	// Header.IV[:], so this is structurally unreachable — but a panic on a
	// device is a brick, and this costs one comparison.
	if len(iv) != IVLen {
		return nil, fmt.Errorf("seal: iv is %d bytes, want %d", len(iv), IVLen)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("seal: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("seal: %w", err)
	}
	pt, err := gcm.Open(nil, iv, sealed, aad)
	if err != nil {
		return nil, ErrAuthentication
	}
	return pt, nil
}
