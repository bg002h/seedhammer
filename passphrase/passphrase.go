// Package passphrase validates BIP-39 passphrases for engraving.
//
// The device accepts only printable ASCII. That is NOT a BIP-39 rule -- the
// standard permits any string -- it is the boundary within which this
// codebase's derivation is provably conformant: bip39.MnemonicSeed performs no
// NFKD normalization, which is identity on ASCII and divergent otherwise. See
// SPEC_seedhammer_engrave_bip39_password.md D3.
package passphrase

import "errors"

// MaxLen is a plate-capacity limit chosen for legibility, not a BIP-39 rule.
const MaxLen = 100

var (
	ErrEmpty    = errors.New("a passphrase is required")
	ErrTooLong  = errors.New("too long for one plate")
	ErrNonASCII = errors.New("this device can only engrave printable ASCII")
)

// ValidatePassphrase reports whether s can be engraved. It never includes s in
// its error, because s is secret.
func ValidatePassphrase(s string) error {
	if s == "" {
		return ErrEmpty
	}
	n := 0
	for _, r := range s {
		// A malformed UTF-8 byte decodes to U+FFFD, which is > 0x7E and so is
		// rejected here rather than needing a separate check.
		if r < 0x20 || r > 0x7E {
			return ErrNonASCII
		}
		n++
	}
	if n > MaxLen {
		return ErrTooLong
	}
	return nil
}
