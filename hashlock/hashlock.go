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
	"fmt"
	"golang.org/x/crypto/ripemd160"
	"seedhammer.com/md"
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
	// ErrHex64 IS GONE. A digest-shaped phrase is an ADVISORY now, not a
	// refusal -- see LooksLikeDigest. Operator ruling 2026-09-16, verbatim:
	// "we should warn user whenever the hashlock phrase looks like a digest
	// and force user to confirm but we should not always refuse."
)

// PreimageHardened is preimage_hardened: PBKDF2-HMAC-SHA256(phrase, Salt,
// Iterations, 32). One shot; the screen uses DeriveHardened.
func PreimageHardened(phrase []byte) [32]byte {
	d := seal.NewDeriver(phrase, Salt, Iterations)
	defer d.Wipe()
	d.Step(Iterations)
	var out [32]byte
	// Key() is nil on a dead or incomplete Deriver (seal/pbkdf2.go's contract:
	// the caller fails closed); an all-zero preimage must never be returned as
	// if derived. Unreachable here (the Deriver is local), kept for the contract.
	if k := d.Key(); k != nil {
		copy(out[:], k)
	}
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
	k := d.Key()
	if k == nil {
		return x, false // fail closed on a dead Deriver (post-impl M-2)
	}
	copy(x[:], k)
	return x, true
}

// PreimageSHA256 is preimage_sha256: one SHA-256 of the phrase bytes -- the
// brainwallet construction, warned about every time (L12).
func PreimageSHA256(phrase []byte) [32]byte {
	return sha256.Sum256(phrase)
}

// DigestSHA256 is H = SHA-256(X): what a sha256 policy carries and the plate
// shows.
//
// RENAMED FROM Digest by SPEC_hashlock_kinds phase 4. The bare name could only
// ever mean one of four hashes, and every caller of it was silently the sha256
// caller.
func DigestSHA256(x *[32]byte) [32]byte {
	return sha256.Sum256(x[:])
}

// DigestHash256 is H = SHA-256(SHA-256(X)) -- sha256d.
//
// THE DANGEROUS ONE: written one round short it is still 32 bytes, still
// type-checks, still lowers, and Core still agrees with the address. Only the
// KAT catches it, which is why hashlock_test.go measures all four against the
// vendored corpus rather than against this file.
func DigestHash256(x *[32]byte) [32]byte {
	first := sha256.Sum256(x[:])
	return sha256.Sum256(first[:])
}

// DigestRIPEMD160 is H = RIPEMD-160(X) -- the BARE primitive, not hash160.
func DigestRIPEMD160(x *[32]byte) [20]byte {
	h := ripemd160.New()
	h.Write(x[:])
	var out [20]byte
	copy(out[:], h.Sum(nil))
	return out
}

// DigestHash160 is H = RIPEMD-160(SHA-256(X)) -- hash160. Same width as
// ripemd160 and a different preimage relation (spec §3 F4).
func DigestHash160(x *[32]byte) [20]byte {
	inner := sha256.Sum256(x[:])
	h := ripemd160.New()
	h.Write(inner[:])
	var out [20]byte
	copy(out[:], h.Sum(nil))
	return out
}

// DigestOf is THE ONE NAMED DISPATCH: kind to function. Callers do not write
// their own switch -- four correct functions behind one mis-wired arm is the
// same lost-funds outcome with a different cause, which is why the KAT
// exercises this and not only the four.
//
// Returns the digest at the kind's own width.
func DigestOf(k md.HashKind, x *[32]byte) []byte {
	switch k {
	case md.KindHash256:
		d := DigestHash256(x)
		return d[:]
	case md.KindRipemd160:
		d := DigestRIPEMD160(x)
		return d[:]
	case md.KindHash160:
		d := DigestHash160(x)
		return d[:]
	case md.KindSha256:
		d := DigestSHA256(x)
		return d[:]
	}
	// NOT a sha256 default: a fifth kind would silently get sha256's digest,
	// which is the exact funds-loss the per-kind KAT exists to catch.
	panic("hashlock: unknown HashKind -- a kind was added without updating DigestOf")
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
	// The digest-shaped check used to refuse here. It advises now; see
	// LooksLikeDigest, which the phrase SCREEN consults so the operator can
	// confirm rather than be turned away.
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
	// ASCII-only case fold, as the host's to_ascii_lowercase (post-impl N-2):
	// strings.ToLower would fold non-ASCII runes the host leaves alone.
	t := strings.Map(func(r rune) rune {
		if r >= 'A' && r <= 'Z' {
			return r + ('a' - 'A')
		}
		return r
	}, strings.TrimSpace(s))
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

// MethodLine is §8.6's METHOD DEFINITION, the line a hashlock PHRASE plate
// carries and the line the QR text embeds -- ms_codec::hashlock::qr_text's own
// method string, ported byte for byte (Rust is primary).
//
// A WIRE RECORD CARRIES A SELECTOR AND A PLATE CARRIES THE DEFINITION, and the
// difference is the whole reason this string is long. `phrase:` carries
// `hardened` because it is read by a tool that already knows the parameter set;
// a plate is read years later by a person who may have neither the tool nor
// this firmware, so it spells the parameters out.
//
// IT IS BUILT FROM THIS PACKAGE'S OWN CONSTANTS. A literal here could drift
// from Iterations, Salt or PreimageLen without a single test noticing -- and
// the failure would be a plate whose method line describes a derivation that
// does not produce its digest.
func MethodLine(hardened bool) string {
	if !hardened {
		return "method: sha256"
	}
	return fmt.Sprintf("method: pbkdf2-hmac-sha256 iterations=%d salt=%s dklen=%d",
		Iterations, Salt, PreimageLen)
}

// QRText is ms_codec::hashlock::qr_text: the text §8.6's QR encodes.
//
// NEVER THE ms1 STRING (§6.4, decision 1): a QR on a phrase plate carries the
// phrase and its method, and the string form's plate carries no QR at all.
func QRText(hardened bool, kind md.HashKind, phrase string) string {
	// The `hash:` line is ITS OWN LINE, never appended to `method:`
	// (SPEC_hashlock_H6 §6.5 pins that line at 73 characters and the plate
	// refuses an eleventh row at every font rung), and it is UNCONDITIONAL --
	// a sha256 plate cut now differs from one cut before this cycle, which is
	// deliberate: a plate read years later must name the hash it commits to
	// rather than leaving it to the md1 card the operator was told to store
	// SEPARATELY (SPEC_hashlock_kinds §13.1).
	//
	// Measured: worst case 194 -> 210 bytes (hardened + ripemd160, the longest
	// token) and the QR version does NOT move -- dim 53 for all four kinds, the
	// envelope already reserved. ACCEPTANCE item 8's scan gate is unaffected.
	return "hashlock v1\n" + MethodLine(hardened) +
		"\nhash: " + kind.Token() +
		"\nphrase: " + phrase
}

// LooksLikeDigest reports whether a phrase looks like a digest in hex, and at
// what width.
//
// THE GO HALF OF F-539, converging on ms-codec's `looks_like_digest` (the
// Rust-primary rule: the refusal semantics changed there first, with vectors).
//
// WHY IT IS WORTH SAYING SOMETHING. An operator holding a digest in hex may
// type it here, and the KDF then commits the wallet to the ASCII OF THE DIGEST
// rather than to the digest -- a preimage they do not hold, on a path they
// cannot open.
//
// WHY IT MUST NOT REFUSE. A phrase that happens to be all hex at one of these
// widths may be one the operator really chose, and refusing outright leaves
// them no way to use it. A warning costs one confirmation; a refusal costs the
// phrase. That is the operator's ruling and it reversed shipped behaviour.
//
// THE WIDTHS COME FROM THE KINDS, never a literal. This was `len == 64`, which
// was every digest in the world while sha256 was the only composable kind;
// SPEC_hashlock_kinds made 40 hex a digest too and the check was blind to it.
func LooksLikeDigest(phrase []byte) (int, bool) {
	if len(phrase) == 0 || !isHex(phrase) {
		return 0, false
	}
	for _, k := range []md.HashKind{md.KindSha256, md.KindHash256, md.KindRipemd160, md.KindHash160} {
		if len(phrase) == 2*k.DigestLen() {
			return len(phrase), true
		}
	}
	return 0, false
}
