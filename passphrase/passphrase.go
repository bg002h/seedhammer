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

// FingerprintLen is the canonical length: a BIP-32 master fingerprint is the
// first 4 bytes of RIPEMD160(SHA256(pubkey)), i.e. exactly 8 hex digits. This
// is the WHOLE fingerprint, not a truncation.
const FingerprintLen = 8

var ErrBadFingerprint = errors.New("fingerprint must be 8 hex digits")

// ValidateFingerprint accepts an empty string (the field is optional) or 8 hex
// digits with optional internal whitespace, and returns the canonical form:
// whitespace stripped, uppercased.
//
// The canonical form is the ONLY value stored or compared. The 4-and-4 grouping
// used on the plate and in the UI is presentation only -- see spec 4.3.
func ValidateFingerprint(s string) (string, error) {
	var buf [FingerprintLen]byte
	n := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == ' ' || c == '\t' {
			continue
		}
		switch {
		case c >= '0' && c <= '9':
		case c >= 'a' && c <= 'f':
			c -= 'a' - 'A'
		case c >= 'A' && c <= 'F':
		default:
			return "", ErrBadFingerprint
		}
		if n == FingerprintLen {
			return "", ErrBadFingerprint
		}
		buf[n] = c
		n++
	}
	if n == 0 {
		return "", nil
	}
	if n != FingerprintLen {
		return "", ErrBadFingerprint
	}
	return string(buf[:]), nil
}

// GroupFingerprint renders a canonical fingerprint for display and engraving,
// as "A1B2 C3D4". The separator is a plain space, NEVER the visible-space mark:
// the mark means "a literal space in the passphrase", and hex is 0-9A-F so a
// gap cannot be misread as a digit.
func GroupFingerprint(canonical string) string {
	if len(canonical) != FingerprintLen {
		return canonical
	}
	return canonical[:4] + " " + canonical[4:]
}
