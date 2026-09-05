// Package hashlock is the SeedHammer port of ms_codec::hashlock (ms-codec 0.8.0,
// mnemonic-secret cd0a60f): a memorable phrase becomes a 32-byte hashlock
// PREIMAGE X, and the digest H = SHA-256(X) is what a spend path's script holds.
//
// Rust is primary (CLAUDE.md): nothing here is decided in Go. The vendored corpus
// testdata/hashlock-v0.8.json pins every value, compared against its constants.
//
// SPEC_hashlock_H2_device §2, §3.
package hashlock

import (
	"crypto/sha256"
	"errors"
	"strings"

	"seedhammer.com/seal"
)

// Salt is HASHLOCK_SALT: fourteen bytes, passed to the KDF as a SLICE. Never
// through seal.Header's Salt [16]byte -- zero-padding it changes every digest.
var Salt = []byte("ms-hashlock-v1")

// Iterations is HASHLOCK_ITERATIONS; about 10 s on the SH2 (9,715 it/s measured).
const Iterations = 100000

// PreimageLen is HASHLOCK_DKLEN: a hashlock preimage is exactly 32 bytes (OP_SIZE 32).
const PreimageLen = 32

// PhraseMaxChars is ms-cli's HASHLOCK_PHRASE_MAX_CHARS: the counter's denominator
// and the rule's bound, from this one constant.
const PhraseMaxChars = 100

// The phrase rule's refusals, SPEC_ms_hashlock §4.3 / SPEC_hashlock_H2_device §2,
// in the order the rule checks them.
var (
	ErrEmpty             = errors.New("hashlock: the phrase is empty")
	ErrNotPrintableASCII = errors.New("hashlock: the phrase has a byte outside 0x20..=0x7E")
	ErrMS1Shaped         = errors.New("hashlock: that is a preimage plate, not a phrase")
	ErrTooLong           = errors.New("hashlock: the phrase is longer than 100 characters")
	ErrHex64             = errors.New("hashlock: that is a preimage in hex, not a phrase")
)

// PreimageHardened is preimage_hardened: PBKDF2-HMAC-SHA256(phrase, Salt,
// Iterations, 32). One shot; the screen uses DeriveHardened.
func PreimageHardened(phrase []byte) [32]byte {
	d := seal.NewDeriver(phrase, Salt, Iterations)
	defer d.Wipe()
	d.Step(Iterations)
	var out [32]byte
	copy(out[:], d.Key())
	return out
}

// DeriveHardened is PreimageHardened in steps, so a screen can show progress and
// honour Back: progress(done, total) is called after every 500 iterations and
// returns false to abandon (then ok is false and the result is zero).
func DeriveHardened(phrase []byte, progress func(done, total int) bool) (x [32]byte, ok bool) {
	d := seal.NewDeriver(phrase, Salt, Iterations)
	defer d.Wipe()
	for !d.Step(500) {
		if !progress(d.Done(), d.Total()) {
			return x, false
		}
	}
	copy(x[:], d.Key())
	return x, true
}

// PreimageSHA256 is preimage_sha256: one SHA-256 of the phrase bytes -- the
// brainwallet construction, warned about every time (L12).
func PreimageSHA256(phrase []byte) [32]byte {
	return sha256.Sum256(phrase)
}

// Digest is digest: H = SHA-256(X).
func Digest(x *[32]byte) [32]byte {
	return sha256.Sum256(x[:])
}

// ValidatePhrase applies SPEC_ms_hashlock §4.3 to the typed BYTES, in the host's
// order, and changes nothing: no trim, no case fold, no normalisation. The shape
// test works on a copy.
func ValidatePhrase(phrase []byte) error {
	if len(phrase) == 0 {
		return ErrEmpty
	}
	for _, b := range phrase {
		if b < 0x20 || b > 0x7e {
			return ErrNotPrintableASCII
		}
	}
	if IsMS1Shaped(string(phrase)) {
		return ErrMS1Shaped
	}
	if len(phrase) > PhraseMaxChars {
		return ErrTooLong
	}
	if len(phrase) == 64 && isHex(phrase) {
		return ErrHex64
	}
	return nil
}

const bech32Charset = "qpzry9x8gf2tvdw0s3jn54khce6mua7l"

// minMS1Len is ms-cli's MIN_MS1_LEN.
const minMS1Len = 48

// IsMS1Shaped is the host's looks_like_ms1 (ms-cli argv_guard.rs:148-164): trim,
// lowercase, strip the display separators (whitespace, '-', ','), then at least
// 48 characters, an `ms1` prefix and only bech32 characters. NO checksum -- a
// grouped or mistyped plate the host refuses is refused here too.
func IsMS1Shaped(s string) bool {
	t := strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range t {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '-' || r == ',' {
			continue
		}
		b.WriteRune(r)
	}
	t = b.String()
	if len(t) < minMS1Len || !strings.HasPrefix(t, "ms1") {
		return false
	}
	for _, r := range t[3:] {
		if !strings.ContainsRune(bech32Charset, r) {
			return false
		}
	}
	return true
}

func isHex(b []byte) bool {
	for _, c := range b {
		switch {
		case c >= '0' && c <= '9', c >= 'a' && c <= 'f', c >= 'A' && c <= 'F':
		default:
			return false
		}
	}
	return true
}
