package seal

import (
	"crypto/aes"
	"crypto/cipher"
	"errors"
	"fmt"
)

// §7 — PBKDF2-HMAC-SHA256 into AES-256-GCM.
//
// The device only ever OPENS. seal_bytes is deliberately NOT ported: a device
// that can seal is a device that can be made to emit a payload, and nothing in
// §10 needs it.
//
// crypto/aes and crypto/cipher were ABSENT from the firmware build before this
// feature; importing them is what makes AES-GCM callable and costs the measured
// ~1.6 KB. Nothing else here is new weight: crypto/hmac and crypto/sha256 are
// already linked and already CALLED by bip85, nonstandard and slip39, and
// crypto/pbkdf2 stays in the image via bip39 and slip39, which import
// golang.org/x/crypto/pbkdf2 -- a thin wrapper over it. seal no longer calls it
// itself: DeriveKey below wraps pbkdf2.go's Deriver, so this package holds ONE
// PBKDF2. The stdlib remains the differential ORACLE in pbkdf2_test.go, which
// costs no flash because _test.go is never linked into the firmware.

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
// It is the one-shot form of Deriver (pbkdf2.go), and it is a WRAPPER rather
// than a second implementation: two PBKDF2s in one package must agree forever,
// and a divergence between them is a wrong key that surfaces as a tag mismatch
// indistinguishable from a wrong passphrase. There is one implementation now,
// and the vectors pin it.
//
// The []byte conversion is an unwipeable copy of the passphrase -- not a
// regression, the old body handed a string to pbkdf2.Key which copies it the
// same way, but it is why a caller that wants a ZEROABLE passphrase calls
// NewDeriver directly with its own buffer, as the device unlock path does.
//
// iterations ALWAYS comes from the header — never a constant, or vector B fails.
func DeriveKey(passphrase string, salt []byte, iterations int) []byte {
	d := NewDeriver([]byte(passphrase), salt, iterations)
	defer d.Wipe()
	// One call: Step never runs past total, so the largest legal iteration
	// count (§6.2's 2,000,000) completes here in a single pass.
	d.Step(iterations)
	return d.Key()
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
