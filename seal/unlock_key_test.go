package seal

import (
	"bytes"
	"errors"
	"testing"
)

// UnlockWithKey is §10.2 steps 8-9 against a key the caller already derived, so
// that Phase B can draw frames across the ~31 s derivation. The property that
// makes the split a REFACTOR rather than a fork is that Unlock is rewritten to
// call it — so these tests assert the two produce identical results, and
// TestUnlockRefusesAMismatchedBlob and every other existing Unlock test in this
// package go on passing unchanged.

// TestUnlockWithKeyReproducesUnlock — the key half of "one implementation, not
// two". The key comes from the vector file's own derived_key_hex rather than
// from a re-derivation here: a retyped or re-derived constant would let both
// sides of the comparison drift together.
func TestUnlockWithKeyReproducesUnlock(t *testing.T) {
	for _, name := range []string{"A", "B", "C", "D", "F", "G"} {
		t.Run(name, func(t *testing.T) {
			v := vectorNamed(t, name)
			if v.CtLen == 0 {
				t.Fatalf("premise broken: vector %s must be sealed", name)
			}
			if v.DerivedKeyHex == nil {
				t.Fatalf("premise broken: vector %s carries no derived_key_hex", name)
			}
			blob := v.Blob(t)

			var o Opener
			viaUnlock, err := o.Inspect(blob)
			if err != nil {
				t.Fatalf("Inspect: %v", err)
			}
			if err := o.Unlock(blob, viaUnlock, *v.Passphrase); err != nil {
				t.Fatalf("Unlock: %v", err)
			}

			viaKey, err := o.Inspect(blob)
			if err != nil {
				t.Fatalf("Inspect: %v", err)
			}
			key := mustHex(t, *v.DerivedKeyHex)
			if err := o.UnlockWithKey(blob, viaKey, key); err != nil {
				t.Fatalf("UnlockWithKey: %v", err)
			}

			if len(viaKey.Secret) != len(viaUnlock.Secret) {
				t.Fatalf("UnlockWithKey admitted %d secret records, Unlock admitted %d",
					len(viaKey.Secret), len(viaUnlock.Secret))
			}
			if len(viaKey.Secret) != len(v.Secret) {
				t.Fatalf("vector %s declares %d secret records, got %d",
					name, len(v.Secret), len(viaKey.Secret))
			}
			for i := range viaKey.Secret {
				if !bytes.Equal(viaKey.Secret[i].Record, viaUnlock.Secret[i].Record) {
					t.Errorf("record %d: UnlockWithKey gave %q, Unlock gave %q",
						i, viaKey.Secret[i].Record, viaUnlock.Secret[i].Record)
				}
				if got, want := string(viaKey.Secret[i].Record), v.Secret[i]; got != want {
					t.Errorf("record %d: got %q, the vector says %q", i, got, want)
				}
				if viaKey.Secret[i].Class != viaUnlock.Secret[i].Class {
					t.Errorf("record %d: class %v vs %v",
						i, viaKey.Secret[i].Class, viaUnlock.Secret[i].Class)
				}
			}
			// The public half must be untouched by either route: §10.2 step 8
			// keeps the §6.6 hash on screen through the retry loop, which
			// requires Unlock never to disturb what Inspect produced.
			if viaKey.Hash != viaUnlock.Hash || viaKey.HasHash != viaUnlock.HasHash {
				t.Errorf("the public hash differs between the two routes")
			}
		})
	}
}

// TestUnlockWithKeyRefusesAnUnsealedPayload — vector E has ct_len == 0, so there
// is no key for a caller to have derived. §10.2 step 4 stops before the
// passphrase in that shape, so reaching here is a PROGRAMMING error and gets its
// own sentinel rather than being silently treated as success.
func TestUnlockWithKeyRefusesAnUnsealedPayload(t *testing.T) {
	v := vectorNamed(t, "E")
	if v.CtLen != 0 {
		t.Fatalf("premise broken: vector E must be unsealed, ct_len = %d", v.CtLen)
	}
	blob := v.Blob(t)
	var o Opener
	p, err := o.Inspect(blob)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	err = o.UnlockWithKey(blob, p, make([]byte, KeyLen))
	if !errors.Is(err, ErrNotSealed) {
		t.Errorf("UnlockWithKey on an unsealed payload = %v, want ErrNotSealed", err)
	}
	if p.Secret != nil {
		t.Errorf("a refused UnlockWithKey populated %d secret records", len(p.Secret))
	}
	// Unlock's contract DIFFERS and must not have been changed by the refactor:
	// "a payload with nothing encrypted is not an error".
	if err := o.Unlock(blob, p, "irrelevant"); err != nil {
		t.Errorf("Unlock on an unsealed payload = %v, want nil", err)
	}
}

// TestUnlockWithKeyBoundChecksTheBlobItIsGiven. The offsets come from p.Header,
// which came from a DIFFERENT call (Inspect), and nothing forces the caller to
// hand back the same blob. On a device a panic is a brick.
func TestUnlockWithKeyBoundChecksTheBlobItIsGiven(t *testing.T) {
	v := vectorNamed(t, "D")
	blob := v.Blob(t)
	var o Opener
	p, err := o.Inspect(blob)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	key := mustHex(t, *v.DerivedKeyHex)
	if err := o.UnlockWithKey(blob[:len(blob)-1], p, key); !errors.Is(err, ErrTooShort) {
		t.Errorf("UnlockWithKey(short) = %v, want ErrTooShort", err)
	}
}

// TestUnlockWithKeyFailsClosedOnAWrongKey. The error must be ErrAuthentication
// specifically: Phase B has to offer BOTH readings — "wrong passphrase, or this
// payload has been altered" — and keep the §6.6 hash on screen, which requires
// p to survive the failure intact.
func TestUnlockWithKeyFailsClosedOnAWrongKey(t *testing.T) {
	v := vectorNamed(t, "D")
	blob := v.Blob(t)
	var o Opener
	p, err := o.Inspect(blob)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	wantHash, wantPub := p.Hash, len(p.Public)
	key := mustHex(t, *v.DerivedKeyHex)
	key[0] ^= 0xff
	if err := o.UnlockWithKey(blob, p, key); !errors.Is(err, ErrAuthentication) {
		t.Errorf("UnlockWithKey(wrong key) = %v, want ErrAuthentication", err)
	}
	if p.Secret != nil {
		t.Errorf("a failed unlock populated %d secret records", len(p.Secret))
	}
	if p.Hash != wantHash || len(p.Public) != wantPub {
		t.Error("a failed unlock disturbed the public half; the retry loop cannot keep the hash on screen")
	}
}

// TestUnlockWithKeyDoesNotRetainOrZeroTheKey. Documented contract: "The key is
// the caller's to wipe. This function neither zeroes nor retains it." The gui
// side registers its own `defer clear(key)`, so a zero here would be a
// double-wipe at best and, if UnlockWithKey ever retained it, a live key.
func TestUnlockWithKeyDoesNotRetainOrZeroTheKey(t *testing.T) {
	v := vectorNamed(t, "D")
	blob := v.Blob(t)
	var o Opener
	p, err := o.Inspect(blob)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	key := mustHex(t, *v.DerivedKeyHex)
	before := append([]byte(nil), key...)
	if err := o.UnlockWithKey(blob, p, key); err != nil {
		t.Fatalf("UnlockWithKey: %v", err)
	}
	if !bytes.Equal(key, before) {
		t.Errorf("UnlockWithKey modified the caller's key: %x -> %x", before, key)
	}
}

// TestUnlockWithKeyTwiceWipesTheFirstResult mirrors
// TestUnlockTwiceWipesTheFirstResult for the new entry point. Overwriting
// p.Secret makes the old bytes unreachable, so Phase B calling p.Wipe()
// faithfully at session end would still miss them — the exact gap
// AdmittedRecord.Record is []byte to prevent.
func TestUnlockWithKeyTwiceWipesTheFirstResult(t *testing.T) {
	v := vectorNamed(t, "A")
	blob := v.Blob(t)
	var o Opener
	p, err := o.Inspect(blob)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	key := mustHex(t, *v.DerivedKeyHex)
	if err := o.UnlockWithKey(blob, p, key); err != nil {
		t.Fatalf("UnlockWithKey: %v", err)
	}
	first := p.Secret[0].Record
	if allZero(first) {
		t.Fatal("premise broken: the first unlock produced an all-zero record")
	}
	if err := o.UnlockWithKey(blob, p, key); err != nil {
		t.Fatalf("second UnlockWithKey: %v", err)
	}
	if !allZero(first) {
		t.Errorf("the first unlock's record survived a second unlock: %q", first)
	}
	if allZero(p.Secret[0].Record) {
		t.Error("the second unlock's own record was zeroed")
	}
}
