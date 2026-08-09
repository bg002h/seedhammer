package seal

import (
	"bytes"
	"crypto/pbkdf2"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

// keyed unpacks what a derivation needs from a fixture.
//
// Passphrase and DerivedKeyHex are *string on the fixture (seal/vectors_test.go:23,33)
// because vector E has NEITHER: it encrypts nothing, so no key exists.
// Returning ok == false rather than dereferencing keeps E in the loop as a
// deliberate skip instead of a nil panic.
func keyed(t *testing.T, v vector) (pass, salt, want []byte, ok bool) {
	t.Helper()
	if v.Passphrase == nil || v.DerivedKeyHex == nil {
		return nil, nil, nil, false
	}
	return []byte(*v.Passphrase), mustHex(t, v.SaltHex), mustHex(t, *v.DerivedKeyHex), true
}

// The chunked deriver must be BYTE-IDENTICAL to the one-shot DeriveKey the
// vectors pin. Asserted against the vector file's derived_key_hex literals
// rather than against DeriveKey alone, because agreeing with a wrong
// implementation is not a result.
func TestDeriverReproducesEveryVectorKey(t *testing.T) {
	var checked int
	for _, v := range loadVectors(t) {
		pass, salt, want, ok := keyed(t, v)
		if !ok {
			continue
		}
		d := NewDeriver(pass, salt, int(v.Iterations))
		for !d.Step(1000) {
		}
		if got := d.Key(); !bytes.Equal(got, want) {
			t.Errorf("vector %s: chunked key %s, want %s",
				v.Name, hex.EncodeToString(got), hex.EncodeToString(want))
		}
		d.Wipe()
		checked++
	}
	// Six of the seven vectors carry a key; only E does not. Asserted so a
	// fixture that silently lost its keys cannot leave this test green while
	// checking nothing.
	if checked != 6 {
		t.Fatalf("checked %d vectors, want 6", checked)
	}
}

// The step size must not change the result. A deriver that resynchronised on a
// block boundary, or that double-counted the U_1 NewDeriver performs, would
// agree with itself at one step size and disagree at another.
func TestDeriverIsStepSizeIndependent(t *testing.T) {
	v := vectorNamed(t, "A")
	pass, salt, want, ok := keyed(t, v)
	if !ok {
		t.Fatal("vector A carries no derived key")
	}
	for _, step := range []int{1, 2, 7, 499, 500, 100000, 1 << 20} {
		d := NewDeriver(pass, salt, int(v.Iterations))
		for !d.Step(step) {
		}
		if got := d.Key(); !bytes.Equal(got, want) {
			t.Errorf("step %d: key %s, want %s",
				step, hex.EncodeToString(got), hex.EncodeToString(want))
		}
		d.Wipe()
	}
}

// Vector B is iterations = 100001 where A is 100000. A deriver that treated the
// count as a constant, or that was off by one against DeriveKey, passes A and
// fails here.
func TestDeriverHonoursTheIterationCount(t *testing.T) {
	a, b := vectorNamed(t, "A"), vectorNamed(t, "B")
	if a.Iterations == b.Iterations {
		t.Fatal("vectors A and B no longer differ in iteration count; this test proves nothing")
	}
	pa, sa, wa, oka := keyed(t, a)
	pb, sb, wb, okb := keyed(t, b)
	if !oka || !okb {
		t.Fatal("vectors A and B must both carry a derived key")
	}
	da := NewDeriver(pa, sa, int(a.Iterations))
	for !da.Step(4096) {
	}
	db := NewDeriver(pb, sb, int(b.Iterations))
	for !db.Step(4096) {
	}
	if bytes.Equal(da.Key(), db.Key()) {
		t.Fatal("one iteration of difference produced the same key")
	}
	if !bytes.Equal(da.Key(), wa) || !bytes.Equal(db.Key(), wb) {
		t.Fatal("a derived key does not match its own vector")
	}
}

// An incomplete derivation must yield nil, never a partial accumulator. A short
// key is not a slightly-wrong key: it fails ~31 s later as a tag mismatch
// indistinguishable from a wrong passphrase.
func TestDeriverWithholdsAnIncompleteKey(t *testing.T) {
	v := vectorNamed(t, "A")
	pass, salt, _, ok := keyed(t, v)
	if !ok {
		t.Fatal("vector A carries no derived key")
	}
	d := NewDeriver(pass, salt, int(v.Iterations))
	if d.Step(10) {
		t.Fatalf("10 of %d iterations reported complete", v.Iterations)
	}
	if k := d.Key(); k != nil {
		t.Fatalf("an incomplete deriver returned a %d-byte key", len(k))
	}
	if d.Done() != 11 {
		t.Fatalf("Done reports %d after NewDeriver's U_1 plus Step(10), want 11", d.Done())
	}
}

// Wipe must zero what it owns, and Key's copy must survive it — the property
// that lets a caller `defer d.Wipe()` at construction.
func TestDeriverWipeLeavesTheReturnedKeyIntact(t *testing.T) {
	v := vectorNamed(t, "A")
	pass, salt, want, ok := keyed(t, v)
	if !ok {
		t.Fatal("vector A carries no derived key")
	}
	d := NewDeriver(pass, salt, int(v.Iterations))
	for !d.Step(4096) {
	}
	key := d.Key()
	d.Wipe()
	if !bytes.Equal(key, want) {
		t.Fatal("Wipe zeroed the key it had already handed out")
	}
	zero := make([]byte, len(d.acc))
	if !bytes.Equal(d.acc[:], zero) {
		t.Fatal("Wipe left the accumulator non-zero")
	}
	if !bytes.Equal(d.u[:], zero) {
		t.Fatal("Wipe left the U buffer non-zero")
	}
	// A wiped Deriver must not hand out 32 zero bytes: that is a VALID AES key
	// and it hides the fault (the rule Key's own doc comment states).
	if k := d.Key(); k != nil {
		t.Fatalf("Key() after Wipe returned %d bytes, want nil", len(k))
	}
}

// The ZERO VALUE must fail closed. Deriver, Step and Key are all exported, and
// B2b holds one across a timer, so `var d Deriver` is reachable state. Measured
// before Key()'s `d.total == 0` clause existed: Step reported COMPLETE and Key
// returned 32 zero bytes -- a valid AES key (R0 §3d review, I1).
func TestZeroValueDeriverFailsClosed(t *testing.T) {
	var d Deriver
	if k := d.Key(); k != nil {
		t.Errorf("zero value yields a %d-byte key, want nil", len(k))
	}
	if !d.Step(1000) {
		t.Error("Step on a zero value should report complete-and-empty, not run")
	}
	if k := d.Key(); k != nil {
		t.Errorf("zero value after Step yields a %d-byte key, want nil", len(k))
	}
	// Must not panic: on a device a panic is a brick, and mac is nil here.
	d.Wipe()
}

// The DIFFERENTIAL against the stdlib, over the input space the six vectors do
// NOT reach: they carry two iteration counts, one salt length, and one
// passphrase length of 59 bytes -- UNDER SHA-256's 64-byte HMAC block, so the
// key-hashing path is never exercised by them (R0 §3d review, I2).
//
// crypto/pbkdf2 leaves production in §3d and stays here. It costs no flash --
// _test.go is never linked into the firmware -- and it bottoms out in
// crypto/internal/fips140/pbkdf2, a separate implementation of exactly the
// layer pbkdf2.go hand-rolls.
func TestDeriverMatchesTheStdlibAcrossInputSpace(t *testing.T) {
	for _, iters := range []int{1, 2, 3, 17, 4999, 100_000, 2_000_000} {
		for _, saltLen := range []int{0, 1, 16, 64, 65} {
			for _, passLen := range []int{0, 59, 64, 65, 200} {
				salt := bytes.Repeat([]byte{0xa5}, saltLen)
				pass := string(bytes.Repeat([]byte{'x'}, passLen))
				want, err := pbkdf2.Key(sha256.New, pass, salt, iters, KeyLen)
				if err != nil {
					t.Fatalf("oracle failed at iters=%d: %v", iters, err)
				}
				d := NewDeriver([]byte(pass), salt, iters)
				for !d.Step(4096) {
				}
				if got := d.Key(); !bytes.Equal(got, want) {
					t.Fatalf("iters=%d saltLen=%d passLen=%d: %s, want %s",
						iters, saltLen, passLen,
						hex.EncodeToString(got), hex.EncodeToString(want))
				}
				d.Wipe()
			}
		}
	}
}
