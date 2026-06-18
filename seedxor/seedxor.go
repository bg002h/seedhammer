// Package seedxor implements Coldcard Seed XOR combine: bit-wise XOR of the
// BIP-39 entropy of N parts (each itself a valid BIP-39 mnemonic), recovering
// the original seed. Strictly N-of-N, all parts the same Coldcard-interop
// length (16/24/32-byte = 12/18/24-word). Pure: no RNG, no SHA, no math/big.
// Port of mnemonic_toolkit::seed_xor::seed_xor_combine.
package seedxor

import (
	"errors"

	"seedhammer.com/bip39"
)

var (
	errTooFewParts       = errors.New("seedxor: need at least 2 parts")
	errBadLength         = errors.New("seedxor: unsupported length (use 12/18/24 words)")
	errMismatchedLengths = errors.New("seedxor: all parts must be the same length")
)

// interopLen reports whether n is a Coldcard-interop entropy length. The
// {16,24,32} guard is LOAD-BEARING: bip39.New accepts any 16..32 multiple-of-4
// (i.e. also 20/28 = 15/21-word), so this is the only thing enforcing interop.
func interopLen(n int) bool { return n == 16 || n == 24 || n == 32 }

func wipe(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

// Combine reconstructs the original BIP-39 seed from N Seed-XOR parts. Each
// part must be a VALID BIP-39 mnemonic (Entropy panics otherwise — the GUI
// flow enforces validity per-part before calling; tests pass parsed vectors).
func Combine(parts []bip39.Mnemonic) (bip39.Mnemonic, error) {
	if len(parts) < 2 {
		return nil, errTooFewParts
	}
	out := append([]byte(nil), parts[0].Entropy()...)
	if !interopLen(len(out)) {
		wipe(out)
		return nil, errBadLength
	}
	for _, p := range parts[1:] {
		e := p.Entropy()
		if len(e) != len(out) {
			wipe(out)
			return nil, errMismatchedLengths
		}
		for i := range out {
			out[i] ^= e[i]
		}
	}
	m := bip39.New(out) // safe: len(out) ∈ {16,24,32}, all valid for New
	wipe(out)
	return m, nil
}

// Describe maps a Combine error to a short GUI label.
func Describe(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, errTooFewParts):
		return "need at least 2 parts"
	case errors.Is(err, errBadLength):
		return "unsupported length (use 12/18/24 words)"
	case errors.Is(err, errMismatchedLengths):
		return "all parts must be the same length"
	default:
		return "invalid"
	}
}
