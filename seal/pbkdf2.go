package seal

import (
	"crypto/hmac"
	"crypto/sha256"
	"hash"
)

// §7's PBKDF2-HMAC-SHA256, run in SLICES.
//
// DeriveKey (crypto.go) is the one-shot WRAPPER over this type -- there is one
// implementation of PBKDF2 in this package, not two. This is that primitive
// decomposed so §10.2 step 7's progress indicator can be a real one: crypto/pbkdf2.Key blocks for ~31 s with no callback and no counter, so
// the frame loop stops dead and the operator sees the hang the step exists to
// prevent.
//
// Two properties make this a decomposition rather than new crypto:
//
//   - dkLen == KeyLen == sha256.Size, so RFC 8018's OUTER loop runs exactly
//     once and the block index is always 1. What is left is the inner
//     U_i = PRF(P, U_{i-1}) chain and an XOR accumulator — no branching, no
//     block bookkeeping, nothing to get subtly wrong that a vector cannot see.
//   - Every vector's derived key is a literal in testdata/vectors.json, and
//     pbkdf2_test.go asserts that this reproduces all six. It ALSO runs a
//     differential against crypto/pbkdf2 over input space the vectors do not
//     reach -- they carry two iteration counts, one salt length and one
//     passphrase length, and that length is under SHA-256's HMAC block size.
//     The stdlib left production; it stays as a TEST oracle, which costs no
//     flash because _test.go is never linked into the firmware.
//
// It allocates nothing per Step: the HMAC is constructed once and Reset()
// restores its keyed state, and both buffers are arrays.
var _ [0]struct{} = [KeyLen - sha256.Size]struct{}{}

// Deriver is one in-progress derivation. The zero value is NOT usable; call
// NewDeriver.
type Deriver struct {
	mac   hash.Hash
	u     [sha256.Size]byte
	acc   [sha256.Size]byte
	done  int
	total int
}

// NewDeriver starts a derivation over the §8.1-normalised passphrase.
//
// passphrase is []byte and not string DELIBERATELY: it is the caller's buffer
// and the caller can zero it, which Unlock's string parameter makes impossible.
// The honest caveat: hmac.New folds the passphrase into an ipad/opad pair
// inside the hash state, and those are key-equivalent and not reachable to be
// zeroed. Wipe clears everything this type owns; it cannot clear that. Same
// defence-in-depth-not-a-guarantee framing as the rest of the firmware.
func NewDeriver(passphrase, salt []byte, iterations int) *Deriver {
	if iterations < 1 {
		// Unreachable behind ParseHeader, which bounds iterations to
		// [100_000, 2_000_000] before any KDF work (§6.2). Clamped rather than
		// panicked: on a device a panic is a brick.
		iterations = 1
	}
	d := &Deriver{
		mac:   hmac.New(sha256.New, passphrase),
		total: iterations,
	}
	// U_1 = PRF(P, S || INT_32_BE(1)). The block index is a literal 1 because
	// there is exactly one block; see the type comment.
	d.mac.Write(salt)
	d.mac.Write([]byte{0, 0, 0, 1})
	d.mac.Sum(d.u[:0])
	d.acc = d.u
	d.done = 1
	return d
}

// Step runs at most n further iterations and reports whether the derivation is
// complete. It never runs past total, so a caller that oversteps is harmless.
func (d *Deriver) Step(n int) bool {
	for i := 0; i < n && d.done < d.total; i++ {
		d.mac.Reset()
		d.mac.Write(d.u[:])
		// Sum appends into u's own backing array. Write has already consumed
		// the previous value, so overwriting it here is correct.
		d.mac.Sum(d.u[:0])
		for j := range d.acc {
			d.acc[j] ^= d.u[j]
		}
		d.done++
	}
	return d.done >= d.total
}

// Done and Total drive the progress indicator. Done counts iterations already
// applied, including the U_1 that NewDeriver performed.
func (d *Deriver) Done() int  { return d.done }
func (d *Deriver) Total() int { return d.total }

// Key returns a FRESH copy of the derived key, which the caller owns and MUST
// zero. It is a copy so that Wipe can be deferred at the point the Deriver is
// created without zeroing the result out from under the caller — the shape a
// shared buffer would make impossible to get right.
//
// It returns nil while the derivation is incomplete: a partial accumulator is
// not a short key, it is the wrong key, and returning it would fail as an
// indistinguishable tag mismatch ~31 s later.
func (d *Deriver) Key() []byte {
	// `d.total == 0` is the ZERO VALUE, and it must be caught explicitly: with
	// both fields 0 the `done < total` test is FALSE, so without this clause a
	// `var d Deriver` reports complete and hands out 32 zero bytes -- a VALID
	// AES key, which is exactly what this file's own Wipe/Key contract forbids
	// (see Key below) and what Phase A's DeriveKey doc argued. Measured before
	// this clause existed: `Step(1000)=true, Key() len=32, allzero=true`.
	// NewDeriver clamps total >= 1, so this is unreachable through it; Deriver,
	// Step and Key are all EXPORTED, and B2b holds one across a timer.
	if d.total == 0 || d.done < d.total {
		return nil
	}
	return append([]byte(nil), d.acc[:]...)
}

// Wipe zeroes everything this Deriver owns. See NewDeriver for what it cannot
// reach.
//
// done is reset so a post-Wipe Key() returns nil rather than 32 zero bytes.
// The rule this obeys, stated here because this is now the only place in the
// package that enforces it: an all-zero key would be worse than none -- it is a
// VALID AES key and hides the fault. Not reachable from unlockDerive,
// where Key()'s result is evaluated before the deferred Wipe runs, but this is a
// public seam and B2b will hold one of these across a timer.
func (d *Deriver) Wipe() {
	clear(d.u[:])
	clear(d.acc[:])
	// nil on a zero-value Deriver, where an unguarded Reset panics -- and on a
	// device a panic is a brick. Same reason DeriveKey returns nil rather than
	// panicking: on a device a panic is a brick, and this costs one comparison.
	if d.mac != nil {
		d.mac.Reset()
	}
	d.done = 0
}
