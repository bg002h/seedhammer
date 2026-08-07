package seal

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
)

// PBKDF2 and AES-256-GCM open. The device only ever OPENS: seal_bytes is
// deliberately not ported, because a device that can seal is a device that can
// be made to emit a payload.

// DeriveKey must equal the pinned key for every vector that has one. Vector B
// differs from A ONLY in iteration count — it is the one test in the entire
// suite that catches a hardcoded count, because the altered-header negative
// mismatches on the AAD regardless of which count the KDF actually used.
func TestDeriveKeyMatchesTheVectors(t *testing.T) {
	seen := 0
	for _, v := range loadVectors(t) {
		if v.DerivedKeyHex == nil {
			if v.Passphrase != nil {
				t.Errorf("vector %s has a passphrase but no derived key", v.Name)
			}
			continue
		}
		seen++
		t.Run(v.Name, func(t *testing.T) {
			key := DeriveKey(*v.Passphrase, mustHex(t, v.SaltHex), int(v.Iterations))
			if got := hex.EncodeToString(key); got != *v.DerivedKeyHex {
				t.Errorf("DeriveKey = %s, vector declares %s", got, *v.DerivedKeyHex)
			}
			if len(key) != KeyLen {
				t.Errorf("key is %d bytes, want %d", len(key), KeyLen)
			}
		})
	}
	if seen != 6 {
		t.Fatalf("expected 6 keyed vectors (A B C D F G), saw %d", seen)
	}
}

// A and B share salt, iv, passphrase and plaintext and differ only in
// iterations. Stated as its own assertion so the reason vector B exists cannot
// be lost in a refactor.
func TestIterationCountChangesTheKey(t *testing.T) {
	a, b := vectorNamed(t, "A"), vectorNamed(t, "B")
	if a.SaltHex != b.SaltHex || a.IVHex != b.IVHex || *a.Passphrase != *b.Passphrase {
		t.Fatal("A and B must share salt, iv and passphrase for this to mean anything")
	}
	if a.Iterations == b.Iterations {
		t.Fatal("A and B must differ in iteration count")
	}
	ka := DeriveKey(*a.Passphrase, mustHex(t, a.SaltHex), int(a.Iterations))
	kb := DeriveKey(*b.Passphrase, mustHex(t, b.SaltHex), int(b.Iterations))
	if bytes.Equal(ka, kb) {
		t.Error("the iteration count must reach the KDF")
	}
}

// Pinning the blob hash alone proves the bytes are STABLE, not that they are
// PARSEABLE. Every encrypted vector must open to its exact records.
func TestOpenRoundTripsEveryEncryptedVector(t *testing.T) {
	seen := 0
	for _, v := range loadVectors(t) {
		if v.CtLen == 0 {
			continue
		}
		seen++
		t.Run(v.Name, func(t *testing.T) {
			blob := v.Blob(t)
			split := HeaderLen + int(v.PubLen)
			key := DeriveKey(*v.Passphrase, mustHex(t, v.SaltHex), int(v.Iterations))
			pt, err := Open(key, mustHex(t, v.IVHex), blob[:split], blob[split:])
			if err != nil {
				t.Fatalf("vector must decrypt: %v", err)
			}
			got := strings.Split(string(pt), "\n")
			if len(got) != len(v.Secret) {
				t.Fatalf("decrypted to %d records, vector declares %d", len(got), len(v.Secret))
			}
			for i := range got {
				if got[i] != v.Secret[i] {
					t.Errorf("record %d:\n got %q\nwant %q", i, got[i], v.Secret[i])
				}
			}
			// The wire layout is ciphertext ‖ tag, so the tag is the blob's
			// last 16 bytes and is NOT sliced off before Open.
			if tag := hex.EncodeToString(blob[len(blob)-TagLen:]); tag != *v.TagHex {
				t.Errorf("trailing tag = %s, vector declares %s", tag, *v.TagHex)
			}
		})
	}
	if seen != 6 {
		t.Fatalf("expected 6 encrypted vectors, saw %d", seen)
	}
}

func TestOpenFailsOnAFlippedCiphertextByte(t *testing.T) {
	v := vectorNamed(t, "A")
	blob := v.Blob(t)
	split := HeaderLen + int(v.PubLen)
	key := DeriveKey(*v.Passphrase, mustHex(t, v.SaltHex), int(v.Iterations))
	for _, off := range []int{split, len(blob) - 1} {
		bad := append([]byte(nil), blob...)
		bad[off] ^= 0x01
		if _, err := Open(key, mustHex(t, v.IVHex), bad[:split], bad[split:]); !errors.Is(err, ErrAuthentication) {
			t.Errorf("flipping byte %d must fail authentication, got %v", off, err)
		}
	}
}

func TestOpenFailsOnATamperedAAD(t *testing.T) {
	v := vectorNamed(t, "A")
	blob := v.Blob(t)
	split := HeaderLen + int(v.PubLen)
	key := DeriveKey(*v.Passphrase, mustHex(t, v.SaltHex), int(v.Iterations))
	bad := append([]byte(nil), blob[:split]...)
	bad[8] ^= 0x01 // the version byte, inside the header, inside the AAD
	if _, err := Open(key, mustHex(t, v.IVHex), bad, blob[split:]); !errors.Is(err, ErrAuthentication) {
		t.Errorf("a tampered header must fail authentication, got %v", err)
	}
}

// THE §6.1a test. §2.2 item 4 concedes the blob is attacker-WRITABLE, so
// without the public section in the AAD an attacker swaps an mk1 for one
// encoding THEIR xpub and the operator engraves a steel backup of a wallet they
// do not control. Nothing else in this file notices that mutant.
func TestOpenFailsOnAFlippedPublicSectionByte(t *testing.T) {
	for _, name := range []string{"D", "G"} {
		v := vectorNamed(t, name)
		if v.PubLen == 0 || v.CtLen == 0 {
			t.Fatalf("vector %s must be the mixed shape", name)
		}
		blob := v.Blob(t)
		split := HeaderLen + int(v.PubLen)
		key := DeriveKey(*v.Passphrase, mustHex(t, v.SaltHex), int(v.Iterations))
		// First and last byte OF THE PUBLIC SECTION.
		for _, off := range []int{HeaderLen, split - 1} {
			bad := append([]byte(nil), blob...)
			bad[off] ^= 0x01
			_, err := Open(key, mustHex(t, v.IVHex), bad[:split], bad[split:])
			if !errors.Is(err, ErrAuthentication) {
				t.Errorf("%s: flipping public-section byte %d must fail the tag, got %v", name, off, err)
			}
		}
	}
}

// §11.4: iterations altered 100000 -> 100002 on vector A. NOT 50000 — that is
// rejected by §6.2's floor before any tag work and proves nothing about the
// AAD. 100002 is inside the legal range, so the header parses and the failure
// has to come from the AEAD.
func TestOpenFailsWhenIterationsAlteredInTheHeader(t *testing.T) {
	v := vectorNamed(t, "A")
	blob := append([]byte(nil), v.Blob(t)...)
	binary.BigEndian.PutUint32(blob[12:16], 100002)
	h, err := ParseHeader(blob)
	if err != nil {
		t.Fatalf("100002 is inside §6.2's range and must parse: %v", err)
	}
	if h.Iterations != 100002 {
		t.Fatalf("altered iterations did not reach the parser: %d", h.Iterations)
	}
	split := HeaderLen + int(h.PubLen)
	key := DeriveKey(*v.Passphrase, h.Salt[:], int(h.Iterations))
	if _, err := Open(key, h.IV[:], blob[:split], blob[split:]); !errors.Is(err, ErrAuthentication) {
		t.Errorf("altered iteration count must fail the tag, got %v", err)
	}
}

// Fail closed: no plaintext is ever returned on a tag mismatch.
func TestOpenReturnsNoPlaintextOnFailure(t *testing.T) {
	v := vectorNamed(t, "A")
	blob := v.Blob(t)
	split := HeaderLen + int(v.PubLen)
	key := DeriveKey(*v.Passphrase, mustHex(t, v.SaltHex), int(v.Iterations))
	bad := append([]byte(nil), blob...)
	bad[len(bad)-1] ^= 0x01
	pt, err := Open(key, mustHex(t, v.IVHex), bad[:split], bad[split:])
	if err == nil {
		t.Fatal("tampered payload must not open")
	}
	if len(pt) != 0 {
		t.Errorf("Open released %d bytes of plaintext on failure", len(pt))
	}
}

func TestOpenRejectsASealedSectionShorterThanTheTag(t *testing.T) {
	v := vectorNamed(t, "A")
	key := DeriveKey(*v.Passphrase, mustHex(t, v.SaltHex), int(v.Iterations))
	if _, err := Open(key, mustHex(t, v.IVHex), []byte("aad"), make([]byte, TagLen-1)); !errors.Is(err, ErrSealedTooShort) {
		t.Error("a sealed section shorter than the tag must be refused")
	}
}
