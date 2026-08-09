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
	// dead is set by Wipe and is one-way. Without it a wiped Deriver is
	// RESURRECTABLE: Wipe zeroes u and acc and resets done, but leaves total,
	// so a later Step(n) re-runs a full derivation from a ZEROED u and Key()
	// then hands back 32 bytes that are not the right key -- surfacing ~31 s
	// later as a tag mismatch indistinguishable from a wrong passphrase, which
	// is the exact failure Key()'s contract exists to prevent. Unreachable in
	// B2a (unlockDerive builds a fresh Deriver per attempt and defers Wipe),
	// but Deriver, Step, Wipe and Key are all exported and B2b holds one of
	// these across a timer.
	dead bool
}

// NewDeriver starts a derivation over the §8.1-normalised passphrase.
//
// passphrase is []byte and not string DELIBERATELY: it is the caller's buffer
// and the caller can zero it, which Unlock's string parameter makes impossible.
//
// HONEST CAVEAT, stated precisely because "key-equivalent" understates it.
// hmac.New folds the passphrase into an ipad/opad pair held in unexported
// fields; Wipe clears everything this type owns and cannot clear those. What
// they hold changes once, and it matters which side of the change you are on
// (read from crypto/internal/fips140/hmac/hmac.go, go1.26.3, not from memory):
//
//   - Between NewDeriver and the FIRST Step, ipad is literally K0^0x36 and opad
//     K0^0x5c. §8.1's passphrase is 47..107 bytes (12 BIP-39 words of 3..8
//     letters plus 11 separators) against SHA-256's 64-byte block, so when it
//     is 64 bytes or shorter -- every canonical vector's is 59 -- the
//     PASSPHRASE ITSELF is recoverable from that array by XOR, not merely
//     something equivalent to the derived key. Over 64 bytes, hmac.New hashes
//     it first and what is recoverable is SHA-256(passphrase).
//   - The first Step calls Reset, which (SHA-256 being marshalable) REPLACES
//     both arrays with the marshaled compression state over K0^ipad / K0^opad.
//     The passphrase is no longer recoverable by XOR from those, but they stay
//     fully key-equivalent -- FIPS 198-1 §6 says in terms that these
//     precomputed intermediates "shall be treated and protected in the same
//     manner as secret keys". Reset never clears them again.
//
// So the passphrase-recoverable window is brief and the key-equivalent one is
// the whole life of the Deriver. Avoiding either means hand-rolling HMAC, which
// this design correctly refuses. Same defence-in-depth-not-a-guarantee framing
// as the rest of the firmware; recorded so no later reader mistakes Wipe for a
// guarantee about the passphrase.
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
//
// After Wipe it is a TERMINATING no-op: it reports complete so a caller's
// `for !d.Step(n)` loop cannot spin forever on a device with no watchdog, and
// Key() then returns nil, so the caller fails closed rather than proceeding
// with a plausible-looking wrong key.
func (d *Deriver) Step(n int) bool {
	if d.dead {
		return true
	}
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
	// `d.dead` here is REDUNDANT behind Step's own guard and is deliberately
	// excluded from the mutation table: Wipe sets done = 0, and a dead Step
	// returns without advancing done, so `d.done < d.total` already covers
	// every reachable post-Wipe shape. Measured -- deleting this clause leaves
	// TestWipedDeriverStaysDead green. It stays because it is the direct
	// expression of the contract at the function the contract is about, and a
	// future Step that advances done before checking dead would otherwise hand
	// out a wrong key. Do not go looking for the test that kills it.
	if d.dead || d.total == 0 || d.done < d.total {
		return nil
	}
	return append([]byte(nil), d.acc[:]...)
}

// Wipe zeroes everything this Deriver owns and marks it permanently dead. See
// NewDeriver for what it cannot reach.
//
// done is reset so a post-Wipe Key() returns nil rather than 32 zero bytes.
// The rule this obeys, stated here because this is now the only place in the
// package that enforces it: an all-zero key would be worse than none -- it is a
// VALID AES key and hides the fault. Not reachable from unlockDerive,
// where Key()'s result is evaluated before the deferred Wipe runs, but this is a
// public seam and B2b will hold one of these across a timer.
//
// The dead flag exists because zeroing alone is not enough: total survives, so
// Step would happily re-derive from a zeroed u and Key() would then report
// complete. Clearing total instead would make Step return true immediately --
// the same terminating behaviour -- but leaves Key()'s `total == 0` zero-value
// branch doing double duty for two different faults, so the flag is explicit.
func (d *Deriver) Wipe() {
	d.dead = true
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
