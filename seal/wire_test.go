package seal

import (
	"bytes"
	"encoding/hex"
	"errors"
	"testing"
)

// Port of crates/me-cli/src/seal/wire.rs's tests. The header is parsed BEFORE
// anything authenticates it and it carries the iteration count, so it is
// hostile input by construction: §6.2's bounds are what stop an unbounded KDF
// on firmware with no active watchdog.

func sealedHeader() Header {
	var salt [SaltLen]byte
	var iv [IVLen]byte
	for i := range salt {
		salt[i] = 0xbe
	}
	for i := range iv {
		iv[i] = 0xc0
	}
	return Header{Iterations: 100000, Salt: salt, IV: iv, PubLen: 396, CtLen: 75}
}

func unsealedHeader() Header {
	return Header{Iterations: 0, PubLen: 396, CtLen: 0}
}

func TestBothShapesRoundTrip(t *testing.T) {
	for _, h := range []Header{sealedHeader(), unsealedHeader()} {
		b := h.Encode()
		if len(b) != HeaderLen {
			t.Fatalf("encoded header is %d bytes, want %d", len(b), HeaderLen)
		}
		if string(b[:8]) != Magic {
			t.Fatalf("magic is %q, want %q", b[:8], Magic)
		}
		got, err := ParseHeader(b[:])
		if err != nil {
			t.Fatalf("round-trip of %+v: %v", h, err)
		}
		if got != h {
			t.Fatalf("round-trip changed the header:\n got %+v\nwant %+v", got, h)
		}
	}
	if !sealedHeader().Sealed() {
		t.Error("ct_len > 0 must be the sealed shape")
	}
	if unsealedHeader().Sealed() {
		t.Error("ct_len == 0 must be the unsealed shape")
	}
}

func TestSealedShapeSetsAlgorithmIDsAndUnsealedZeroesThem(t *testing.T) {
	s := sealedHeader().Encode()
	if s[9] != KDFPBKDF2SHA256 || s[10] != AEADAES256GCM {
		t.Errorf("sealed ids are %#x/%#x, want %#x/%#x", s[9], s[10], KDFPBKDF2SHA256, AEADAES256GCM)
	}
	u := unsealedHeader().Encode()
	if u[9] != 0 || u[10] != 0 {
		t.Errorf("unsealed ids are %#x/%#x, want 0/0", u[9], u[10])
	}
}

// Covers every §6.2 header bound. `reserved`, and the SEALED-shape
// kdf_id/aead_id checks, are exercised here and nowhere else — the
// unsealed-fields test only visits offsets 9/10 in the UNSEALED branch.
func TestRejectsBadMagicVersionReservedKDFAndAEAD(t *testing.T) {
	cases := []struct {
		off  int
		val  byte
		want error
	}{
		{0, 'X', ErrBadMagic},
		{8, 0x02, ErrUnknownVersion},
		{11, 0x01, ErrReservedNotZero},
		{9, 0x02, ErrUnknownKDF},
		{10, 0x02, ErrUnknownAEAD},
	}
	for _, c := range cases {
		b := sealedHeader().Encode()
		b[c.off] = c.val
		_, err := ParseHeader(b[:])
		if !errors.Is(err, c.want) {
			t.Errorf("byte %d = %#x: got %v, want %v", c.off, c.val, err, c.want)
		}
	}
}

func TestRejectsAShortBuffer(t *testing.T) {
	if _, err := ParseHeader(make([]byte, HeaderLen-1)); !errors.Is(err, ErrTooShort) {
		t.Errorf("51 bytes: got %v, want %v", err, ErrTooShort)
	}
}

func TestRejectsOutOfRangeIterationsWhenSealed(t *testing.T) {
	for _, bad := range []uint32{0, MinIterations - 1, MaxIterations + 1, ^uint32(0)} {
		h := sealedHeader()
		h.Iterations = bad
		b := h.Encode()
		if _, err := ParseHeader(b[:]); !errors.Is(err, ErrIterations) {
			t.Errorf("iterations %d: got %v, want %v", bad, err, ErrIterations)
		}
	}
}

// 0xFFFFFFF0 is the case §11.2 names explicitly: a 32-bit native-int region-fit
// check wraps it negative and accepts.
func TestRejectsOutOfRangeLengths(t *testing.T) {
	for _, bad := range []uint32{MaxSectionLen + 1, 0xFFFFFFF0, ^uint32(0)} {
		h := sealedHeader()
		h.CtLen = bad
		b := h.Encode()
		if _, err := ParseHeader(b[:]); !errors.Is(err, ErrCtLen) {
			t.Errorf("ct_len %d: got %v, want %v", bad, err, ErrCtLen)
		}
		h = sealedHeader()
		h.PubLen = bad
		b = h.Encode()
		if _, err := ParseHeader(b[:]); !errors.Is(err, ErrPubLen) {
			t.Errorf("pub_len %d: got %v, want %v", bad, err, ErrPubLen)
		}
	}
}

func TestRejectsAnEmptyPayload(t *testing.T) {
	h := unsealedHeader()
	h.PubLen = 0
	b := h.Encode()
	if _, err := ParseHeader(b[:]); !errors.Is(err, ErrEmpty) {
		t.Errorf("pub_len = ct_len = 0: got %v, want %v", err, ErrEmpty)
	}
}

// §6.2: with ct_len == 0 the crypto fields MUST be zero. Junk there would let
// an attacker stage a downgrade a later version might honour.
func TestRejectsNonZeroCryptoFieldsWhenUnsealed(t *testing.T) {
	for _, c := range []struct {
		off   int
		val   byte
		label string
	}{
		{9, 0x01, "kdf_id"},
		{10, 0x01, "aead_id"},
		{15, 0x01, "iterations"},
		{16, 0x01, "salt"},
		{32, 0x01, "iv"},
	} {
		b := unsealedHeader().Encode()
		b[c.off] = c.val
		if _, err := ParseHeader(b[:]); !errors.Is(err, ErrUnsealedFieldNotZero) {
			t.Errorf("%s must be zero when ct_len == 0: got %v", c.label, err)
		}
	}
}

// The test that actually binds the port. Decode every vector's header_hex and
// assert the parsed fields equal that vector's DECLARED iterations / salt / iv
// / pub_len / ct_len — read from the JSON's own fields, never recovered from
// the header bytes the parser just read.
func TestParseHeaderMatchesTheVectors(t *testing.T) {
	for _, v := range loadVectors(t) {
		t.Run(v.Name, func(t *testing.T) {
			h, err := ParseHeader(mustHex(t, v.HeaderHex))
			if err != nil {
				t.Fatalf("header must parse: %v", err)
			}
			if h.Iterations != v.Iterations {
				t.Errorf("iterations = %d, vector declares %d", h.Iterations, v.Iterations)
			}
			if got := hex.EncodeToString(h.Salt[:]); got != v.SaltHex {
				t.Errorf("salt = %s, vector declares %s", got, v.SaltHex)
			}
			if got := hex.EncodeToString(h.IV[:]); got != v.IVHex {
				t.Errorf("iv = %s, vector declares %s", got, v.IVHex)
			}
			if h.PubLen != v.PubLen {
				t.Errorf("pub_len = %d, vector declares %d", h.PubLen, v.PubLen)
			}
			if h.CtLen != v.CtLen {
				t.Errorf("ct_len = %d, vector declares %d", h.CtLen, v.CtLen)
			}
			if h.Sealed() != (v.CtLen > 0) {
				t.Errorf("Sealed() = %v, ct_len = %d", h.Sealed(), v.CtLen)
			}
			// The whole blob's header must be the same 52 bytes.
			if blob := v.Blob(t); !bytes.Equal(blob[:HeaderLen], mustHex(t, v.HeaderHex)) {
				t.Error("blob_hex's first 52 bytes differ from header_hex")
			}
		})
	}
}
