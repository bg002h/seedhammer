package gui

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/hex"
	"strings"
	"testing"

	"seedhammer.com/seal"
)

// A sealer for package gui.
//
// seal's own sealForTest is unexported in a _test.go file, so no import reaches
// it from here (R0 round 0, I3), and the canonical vectors cannot express every
// shape B2a must test -- in particular NO vector carries md1/mk1 in BOTH
// sections, which is the only shape that exercises the plate list's "(sealed)"
// disambiguation. Measured: A(pub 0/sec 1), B(0/1), C(0/6), D(5/1), E(5/0),
// F(0/15), G(12/3).
//
// This is deliberately NOT a second implementation of the format: it composes
// seal's own Header.Encode and DeriveKey, so a wire-format change breaks it the
// same way it breaks production. Production has no sealing path and must not
// grow one -- a device that can seal is a device that can be made to emit a
// payload (seal/crypto.go:12-16).
func sealBlobForTest(t *testing.T, public, secret []string, passphrase string, iterations uint32) []byte {
	t.Helper()
	// Synthetic salt and IV: a fixture must be deterministic, and §7.2's
	// one-key-one-message rule is not weakened because nothing here is ever a
	// real payload -- production cannot reach this code at all.
	var salt [seal.SaltLen]byte
	var iv [seal.IVLen]byte
	for i := range salt {
		salt[i] = byte(i + 1)
	}
	for i := range iv {
		iv[i] = byte(i + 0x40)
	}
	return sealBlobWith(t, public, secret, passphrase, iterations, salt, iv)
}

// sealBlobWith is sealBlobForTest with the salt and IV supplied, which is what
// lets the fixture be checked against the NORMATIVE vectors byte-for-byte
// instead of merely round-tripping through the package that produced it.
//
// Why that matters (B2a-ii completeness critic, C1): the old self-check sealed
// vector D's records with a SYNTHETIC salt and IV and then asserted the §6.6
// hash plus a round-trip. §6.6's digest covers the public records, the `sealed`
// byte and the public count -- NOT the salt, the IV, the ciphertext, the tag or
// any other header field. And a round-trip cannot see a layout error either,
// because the fixture encodes with seal.Header.Encode and production decodes
// with the same package, so any byte-order or offset mistake is symmetric and
// invisible. "Agrees with the normative vectors" was true of about 19 of the
// header's 60-odd bytes.
func sealBlobWith(t *testing.T, public, secret []string, passphrase string, iterations uint32, salt [seal.SaltLen]byte, iv [seal.IVLen]byte) []byte {
	t.Helper()
	pub := []byte(strings.Join(public, "\n"))
	h := seal.Header{PubLen: uint32(len(pub))}
	if len(secret) > 0 {
		pt := []byte(strings.Join(secret, "\n"))
		h.Iterations = iterations
		h.Salt = salt
		h.IV = iv
		h.CtLen = uint32(len(pt))
		hdr := h.Encode()
		aad := append(append([]byte(nil), hdr[:]...), pub...)
		key := seal.DeriveKey(seal.NormalisePassphrase(passphrase), h.Salt[:], int(iterations))
		block, err := aes.NewCipher(key)
		if err != nil {
			t.Fatalf("aes: %v", err)
		}
		gcm, err := cipher.NewGCM(block)
		if err != nil {
			t.Fatalf("gcm: %v", err)
		}
		// Seal appends the tag, which is exactly the wire layout (§6).
		return append(aad, gcm.Seal(nil, h.IV[:], pt, aad)...)
	}
	hdr := h.Encode()
	return append(append([]byte(nil), hdr[:]...), pub...)
}

// fixturePassphrase is the §8.1-normalised twelve words every gui fixture uses.
// It is a real checksum-valid BIP-39 mnemonic because unlockPassphraseFlow's
// gate rejects anything else before a KDF runs, so a fixture sealed under a
// non-mnemonic passphrase could never be unlocked through the UI.
const fixturePassphrase = "abandon abandon abandon abandon abandon abandon " +
	"abandon abandon abandon abandon abandon about"

// fixtureIterations is §6.2's FLOOR. The ~31 s of §7.1 is a DEVICE figure: on
// this host DeriveKey at 100,000 iterations is ~13 ms, so the suite cost of a
// real unlock is the frame pumping, not the KDF.
const fixtureIterations = 100_000

// TestSealBlobForTestAgreesWithTheNormativeVectors — §5.1, and it runs BEFORE
// any Task 5-7 test depends on the fixture. If the fixture and production
// disagree about the format, every test built on it measures the fixture
// instead of the code.
//
// The two digests asserted here are vector D's pubhash_sealed and vector E's
// pubhash_unsealed, read from the vector file rather than retyped -- so the
// fixture agrees not merely with production but with the normative §11.4
// vectors, which is the property that makes it safe to build on.
func TestSealBlobForTestAgreesWithTheNormativeVectors(t *testing.T) {
	d := sealVector(t, "D")
	e := sealVector(t, "E")
	if len(d.Secret) != 1 {
		t.Fatalf("premise broken: vector D must carry exactly one secret record, got %d", len(d.Secret))
	}
	if d.Passphrase == nil {
		t.Fatal("premise broken: vector D carries no passphrase")
	}

	t.Run("sealed", func(t *testing.T) {
		blob := sealBlobForTest(t, d.Public, d.Secret, *d.Passphrase, d.Iterations)
		var o seal.Opener
		p, err := o.Inspect(blob)
		if err != nil {
			t.Fatalf("Inspect: %v", err)
		}
		if !p.Header.Sealed() {
			t.Fatal("the fixture produced an UNSEALED blob for a payload with a secret section")
		}
		if len(p.Public) != len(d.Public) {
			t.Fatalf("Inspect admitted %d public records, want %d", len(p.Public), len(d.Public))
		}
		// The vector file stores the digest ungrouped; FormatHash is the
		// eight-groups-of-four form the SCREEN shows. Compare the bytes, then
		// log the shape the operator actually reads.
		if got, want := hashHex(p.Hash), *d.PubhashSealed; got != want {
			t.Errorf("the fixture's §6.6 hash is %s, the normative vector says %s", got, want)
		}
		t.Logf("sealed   hash = %s", seal.FormatHash(p.Hash))
		// The full pipeline, at the fixture's OWN iteration count -- which is
		// the vector's here, so the derived key must be the vector's too.
		key := seal.DeriveKey(seal.NormalisePassphrase(*d.Passphrase), p.Header.Salt[:], int(p.Header.Iterations))
		defer clear(key)
		if err := o.UnlockWithKey(blob, p, key); err != nil {
			t.Fatalf("UnlockWithKey: %v", err)
		}
		if len(p.Secret) != 1 {
			t.Fatalf("UnlockWithKey admitted %d secret records, want 1", len(p.Secret))
		}
		if got := string(p.Secret[0].Record); got != d.Secret[0] {
			t.Errorf("the decrypted record is %q, want %q", got, d.Secret[0])
		}
		if p.Secret[0].Class != seal.ClassCodex32Secret {
			t.Errorf("the decrypted record classifies as %v, want a codex32 secret", p.Secret[0].Class)
		}
	})

	t.Run("unsealed", func(t *testing.T) {
		blob := sealBlobForTest(t, e.Public, nil, "", 0)
		var o seal.Opener
		p, err := o.Inspect(blob)
		if err != nil {
			t.Fatalf("Inspect: %v", err)
		}
		if p.Header.Sealed() {
			t.Fatal("the fixture produced a SEALED blob for a payload with no secret section")
		}
		if got, want := hashHex(p.Hash), *e.PubhashUnsealed; got != want {
			t.Errorf("the fixture's §6.6 hash is %s, the normative vector says %s", got, want)
		}
		t.Logf("unsealed hash = %s", seal.FormatHash(p.Hash))
	})
}

// hashHex is the vector file's own encoding of a §6.6 digest -- ungrouped, so
// it compares against pubhash_sealed / pubhash_unsealed directly.
func hashHex(h [16]byte) string { return hex.EncodeToString(h[:]) }

// TestSealBlobForTestBuildsTheBothSectionsShape. No vector can supply it: C's
// and F's encrypted cards sit on pub_len == 0 payloads, and D's and G's
// encrypted records are ms1 only. This is the shape Task 7's "(sealed)" suffix
// exists for -- card indices are computed PER SECTION, so a public mk1 1/2 and
// an encrypted mk1 1/2 both exist here, and without the suffix the list shows
// the same label twice.
func TestSealBlobForTestBuildsTheBothSectionsShape(t *testing.T) {
	public, secret := bothSectionRecords(t)
	blob := sealBlobForTest(t, public, secret, fixturePassphrase, fixtureIterations)
	var o seal.Opener
	p, err := o.Inspect(blob)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	key := seal.DeriveKey(seal.NormalisePassphrase(fixturePassphrase), p.Header.Salt[:], int(p.Header.Iterations))
	defer clear(key)
	if err := o.UnlockWithKey(blob, p, key); err != nil {
		t.Fatalf("UnlockWithKey: %v", err)
	}
	t.Logf("both-sections fixture: %d bytes, %d public + %d encrypted records",
		len(blob), len(p.Public), len(p.Secret))

	// BOTH sections must carry ClassMDMK cards, or the shape this fixture
	// exists for does not exist.
	var pubCards, encCards, encSecrets int
	for _, r := range p.Public {
		if r.Class == seal.ClassMDMK {
			pubCards++
		}
	}
	for _, r := range p.Secret {
		switch {
		case r.Class == seal.ClassMDMK:
			encCards++
		case seal.IsSecret(r.Class):
			encSecrets++
		}
	}
	if pubCards == 0 || encCards == 0 {
		t.Fatalf("the fixture is not the both-sections shape: %d public cards, %d encrypted cards",
			pubCards, encCards)
	}
	if encSecrets == 0 {
		t.Fatal("the fixture carries no secret record, so it cannot drive §10.2.2's session")
	}
	// And the collision the "(sealed)" suffix disambiguates is real: the same
	// plateLabel is produced by a public record and an encrypted one.
	collided := false
	for i, pr := range p.Public {
		for j, sr := range p.Secret {
			if sr.Class != seal.ClassMDMK {
				continue
			}
			if plateLabel(pr, i) == plateLabel(sr, j) {
				collided = true
			}
		}
	}
	if !collided {
		t.Error("no public and encrypted card share a label; the (sealed) suffix has nothing to disambiguate")
	}
}

// bothSectionRecords composes vector D's public card set with vector C's
// encrypted set (ms1 x1 / mk1 x2 / md1 x3). Both come from the normative vector
// file, so the RECORDS are never hand-written here -- only their arrangement
// into a shape no single vector has. Eleven records, under §6.4's cap of 24.
func bothSectionRecords(t *testing.T) (public, secret []string) {
	t.Helper()
	d := sealVector(t, "D")
	c := sealVector(t, "C")
	if len(d.Public) == 0 || len(c.Secret) == 0 {
		t.Fatalf("premise broken: D has %d public and C has %d secret records",
			len(d.Public), len(c.Secret))
	}
	return d.Public, c.Secret
}

// THE STRONG BINDING: the fixture must reproduce every sealed vector's blob
// BYTE-FOR-BYTE, given that vector's own salt and IV.
//
// This is what "agrees with the normative vectors" ought to mean. The sibling
// check seals with a synthetic salt/IV and asserts the §6.6 hash plus a
// round-trip, and neither can see a wire-format error: §6.6 covers the public
// records, the `sealed` byte and the public count and nothing else, and a
// round-trip through the same package is symmetric under any byte-order or
// offset mistake. This one compares against blob_hex, which the RUST
// implementation produced — so the header layout, the AAD, the key schedule, the
// GCM tag and the record joining are all bound at once.
//
// (B2a-ii completeness critic, C1.)
func TestSealBlobForTestReproducesEveryVectorByteForByte(t *testing.T) {
	var checked int
	for _, v := range sealVectors(t) {
		if v.Passphrase == nil {
			continue // vector E seals nothing; no key, no salt, no IV.
		}
		var salt [seal.SaltLen]byte
		var iv [seal.IVLen]byte
		copy(salt[:], mustHexBytes(t, v.SaltHex))
		copy(iv[:], mustHexBytes(t, v.IVHex))
		got := sealBlobWith(t, v.Public, v.Secret, *v.Passphrase, v.Iterations, salt, iv)
		if h := hex.EncodeToString(got); h != v.BlobHex {
			t.Errorf("vector %s: fixture produced %d bytes that differ from the "+
				"normative blob_hex (%d bytes) — the fixture and the Rust "+
				"implementation disagree about the wire format",
				v.Name, len(got), len(v.BlobHex)/2)
		}
		checked++
	}
	// Six of the seven vectors are sealed; only E is not. Asserted so a fixture
	// file that lost its passphrases cannot leave this test green over nothing.
	if checked != 6 {
		t.Fatalf("checked %d vectors, want 6", checked)
	}
}
