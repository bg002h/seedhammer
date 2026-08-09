package seal

import (
	"bytes"
	"errors"
	"strings"
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

// TestUnlockWithKeyZeroesTheDecryptedPlaintext — §11.2's plaintext record
// buffer, asserted on the buffer itself.
//
// `defer clear(plaintext)` is what removes the DECRYPTED RECORD CONTAINER --
// every ms1 and every bare mnemonic in the payload, in one gcm.Open allocation
// -- from the heap once AdmitSection has copied out of it. Nothing else can
// reach that array: Payload.Wipe and WipeSecretAt walk p.Public/p.Secret, and
// SecretsResident reports on those too, so deleting the line leaves a full
// plaintext copy of the seed live for the rest of the power cycle and reports
// "no secret resident" while it sits there. That is why this needs a seam
// (unlockPlaintextHook) rather than a public-API assertion: no handle escapes.
//
// Vector F is the widest shape there is -- fifteen records, three of them ms1.
func TestUnlockWithKeyZeroesTheDecryptedPlaintext(t *testing.T) {
	var held []byte
	var handedOver string
	unlockPlaintextHook = func(pt []byte) {
		held = pt
		// A copy taken at handover, so the assertion has a positive control:
		// this proves the watched array really was the record container.
		handedOver = string(pt)
	}
	t.Cleanup(func() { unlockPlaintextHook = nil })

	v := vectorNamed(t, "F")
	blob := v.Blob(t)
	var o Opener
	p, err := o.Inspect(blob)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if err := o.UnlockWithKey(blob, p, mustHex(t, *v.DerivedKeyHex)); err != nil {
		t.Fatalf("UnlockWithKey: %v", err)
	}
	if held == nil {
		t.Fatal("unlockPlaintextHook never fired; this test asserted nothing")
	}
	if want := strings.Join(v.Secret, "\n"); handedOver != want {
		t.Fatalf("the watched buffer held %q, the vector's encrypted section is %q -- "+
			"this test is watching the wrong array", handedOver, want)
	}
	if !allZero(held) {
		t.Errorf("the decrypted record container survived UnlockWithKey: %q\n"+
			"§11.2 requires the plaintext record buffer read as zeroed; neither "+
			"Payload.Wipe nor SecretsResident can reach this one", held)
	}
	// And the wipe is not collateral damage: AdmitSection COPIES
	// (seal/record.go), so the admitted records must survive it.
	if len(p.Secret) != len(v.Secret) {
		t.Fatalf("UnlockWithKey admitted %d records, the vector has %d",
			len(p.Secret), len(v.Secret))
	}
	for i, r := range p.Secret {
		if allZero(r.Record) {
			t.Errorf("record %d was zeroed along with the container; the admitted records "+
				"must not alias it", i)
		}
	}
}

// TestUnlockWithKeyZeroesThePlaintextOnAnErrorExit — the same buffer, on the
// exit §11.2 names separately.
//
// §6.4's 1..24 cap is checked AFTER the decryption, so this route returns with a
// full plaintext section already in hand. It is the one non-success exit past
// the defer that a test can construct, and it is the one that matters: a payload
// built to be refused is exactly what an attacker hands the machine.
func TestUnlockWithKeyZeroesThePlaintextOnAnErrorExit(t *testing.T) {
	var held []byte
	unlockPlaintextHook = func(pt []byte) { held = pt }
	t.Cleanup(func() { unlockPlaintextHook = nil })

	v := vectorNamed(t, "A")
	secret := make([]string, MaxRecords+1)
	for i := range secret {
		secret[i] = "md1qqq"
	}
	salt, iv := mustHex(t, v.SaltHex), mustHex(t, v.IVHex)
	blob := sealForTest(t, nil, secret, *v.Passphrase, salt, iv, v.Iterations)
	var o Opener
	p, err := o.Inspect(blob)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	key := DeriveKey(NormalisePassphrase(*v.Passphrase), salt, int(v.Iterations))
	defer clear(key)
	if err := o.UnlockWithKey(blob, p, key); !errors.Is(err, ErrTooManyRecords) {
		t.Fatalf("UnlockWithKey on %d records = %v, want ErrTooManyRecords", len(secret), err)
	}
	if held == nil {
		t.Fatal("unlockPlaintextHook never fired; this test asserted nothing")
	}
	if !allZero(held) {
		t.Errorf("a REFUSED unlock left the decrypted section resident: %q", held)
	}
}
