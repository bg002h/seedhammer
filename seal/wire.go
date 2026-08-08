// Package seal reads a §6 MNEMBLOB payload from flash: it parses and
// bound-checks the header, splits and bounds the record sections, allow-lists
// and card-set-decodes the records, computes the §6.6 public-data hash, and
// decrypts the encrypted section.
//
// It is the DEVICE half, so it only ever OPENS. There is deliberately no seal
// path: a device that can seal is a device that can be made to emit a payload,
// and nothing in §10 needs it.
//
// Normative source: mnemonic-engrave's crates/me-cli/src/seal/. Per the
// project's Rust-primary rule this package is a behaviour-faithful port and may
// never lead — a disagreement with testdata/vectors.json is a bug here.
package seal

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// §6.2's wire constants.
const (
	// Magic is the 8-byte blob signature. §6.1: a blob is present iff the
	// first 8 bytes are this. Erased flash reads 0xFF, which is not.
	Magic   = "MNEMBLOB"
	Version = 0x01

	KDFPBKDF2SHA256 = 0x01
	AEADAES256GCM   = 0x01

	HeaderLen = 52
	SaltLen   = 16
	IVLen     = 12
	TagLen    = 16

	MinIterations = 100_000
	MaxIterations = 2_000_000

	// MaxSectionLen is 8191, not 8192, because gui/scan.go computes
	// `s.overflow = s.overflow || s.n == len(s.buf)` against an 8*1024
	// buffer: overflow triggers when the buffer is exactly FULL. A payload of
	// exactly 8192 bytes would pass every bound here, burn the full KDF,
	// authenticate correctly, and only then die in the classifier — a
	// spec-legal blob that can never engrave.
	MaxSectionLen = 8191

	// RegionLen is the flash region reserved for the payload at 0x10E00000
	// (§5). It bounds the XIP read in read.go, which happens BEFORE any
	// header field is trustworthy.
	RegionLen = 65_536

	// §6.4's record bounds, consumed by container.go. The count cap is
	// bounded by bundleReviewFlow's paged list, not by ChoiceScreen: the
	// smallest multisig wallet that exists is 10 records and a 2-of-3 is 15,
	// so a cap of 7 would have rejected the entire multisig product surface.
	// The cap is over the TOTAL across BOTH sections (§6.4).
	MaxRecords   = 24
	MaxRecordLen = 512
)

// Every §6.2 violation gets its own sentinel so a caller can tell them apart
// with errors.Is. That matters beyond tidiness: §6.4 requires "too many
// records" to be distinguishable from "payload unreadable", because §6.2 uses
// the latter for a corrupt or TAMPERED blob and the operator has been taught
// to read it as "someone replaced my payload".
var (
	ErrTooShort             = errors.New("seal: blob too short")
	ErrBadMagic             = errors.New("seal: not a sealed payload (bad magic)")
	ErrUnknownVersion       = errors.New("seal: unsupported format version")
	ErrUnknownKDF           = errors.New("seal: unsupported kdf id")
	ErrUnknownAEAD          = errors.New("seal: unsupported aead id")
	ErrReservedNotZero      = errors.New("seal: reserved byte must be 0")
	ErrIterations           = errors.New("seal: iteration count out of range")
	ErrPubLen               = errors.New("seal: public section too long")
	ErrCtLen                = errors.New("seal: ciphertext too long")
	ErrEmpty                = errors.New("seal: payload is empty (pub_len and ct_len are both 0)")
	ErrUnsealedFieldNotZero = errors.New("seal: field must be zero when nothing is encrypted")
	ErrTooLarge             = errors.New("seal: blob does not fit the flash region")
)

// Header is the parsed §6 header: 52 bytes, big-endian.
type Header struct {
	// Iterations is 0 when nothing is encrypted.
	Iterations uint32
	Salt       [SaltLen]byte
	IV         [IVLen]byte
	PubLen     uint32
	// CtLen is 0 when nothing is encrypted — and then there is no tag either.
	CtLen uint32
}

// Sealed reports whether the payload carries an encrypted section. When it does
// not, there is no key, therefore no tag, therefore no authentication (§6.1a);
// the §6.6 hash stands in its place.
func (h Header) Sealed() bool { return h.CtLen > 0 }

// Encode writes the 52-byte header. The device never seals, so this exists for
// the AAD's structural definition and for tests; the AAD used by Open is taken
// from the blob's own bytes, never re-encoded.
func (h Header) Encode() [HeaderLen]byte {
	var out [HeaderLen]byte
	copy(out[:8], Magic)
	out[8] = Version
	if h.Sealed() {
		out[9] = KDFPBKDF2SHA256
		out[10] = AEADAES256GCM
	}
	out[11] = 0 // reserved
	binary.BigEndian.PutUint32(out[12:16], h.Iterations)
	copy(out[16:32], h.Salt[:])
	copy(out[32:44], h.IV[:])
	binary.BigEndian.PutUint32(out[44:48], h.PubLen)
	binary.BigEndian.PutUint32(out[48:52], h.CtLen)
	return out
}

// ParseHeader parses and bound-checks the §6 header.
//
// EVERY check here runs before any allocation or KDF work. This firmware has no
// active watchdog, so a hostile blob declaring a huge iteration count is a hang,
// not an error message — and the header is parsed before anything authenticates
// it, because it is what carries the salt and the iteration count.
//
// The order of checks is normative and mirrors wire.rs: length, magic, version,
// reserved, read the integers, section caps, empty, then the sealed/unsealed
// split, then the region fit.
func ParseHeader(buf []byte) (Header, error) {
	if len(buf) < HeaderLen {
		return Header{}, fmt.Errorf("%w: %d bytes, need at least %d", ErrTooShort, len(buf), HeaderLen)
	}
	if string(buf[:8]) != Magic {
		return Header{}, ErrBadMagic
	}
	if buf[8] != Version {
		return Header{}, fmt.Errorf("%w %d", ErrUnknownVersion, buf[8])
	}
	if buf[11] != 0 {
		return Header{}, fmt.Errorf("%w, got %d", ErrReservedNotZero, buf[11])
	}

	iterations := binary.BigEndian.Uint32(buf[12:16])
	pubLen := binary.BigEndian.Uint32(buf[44:48])
	ctLen := binary.BigEndian.Uint32(buf[48:52])

	if pubLen > MaxSectionLen {
		return Header{}, fmt.Errorf("%w: %d exceeds %d", ErrPubLen, pubLen, MaxSectionLen)
	}
	if ctLen > MaxSectionLen {
		return Header{}, fmt.Errorf("%w: %d exceeds %d", ErrCtLen, ctLen, MaxSectionLen)
	}
	if pubLen == 0 && ctLen == 0 {
		return Header{}, ErrEmpty
	}

	if ctLen > 0 {
		if buf[9] != KDFPBKDF2SHA256 {
			return Header{}, fmt.Errorf("%w %d", ErrUnknownKDF, buf[9])
		}
		if buf[10] != AEADAES256GCM {
			return Header{}, fmt.Errorf("%w %d", ErrUnknownAEAD, buf[10])
		}
		if iterations < MinIterations || iterations > MaxIterations {
			return Header{}, fmt.Errorf("%w: %d is not in [%d, %d]",
				ErrIterations, iterations, MinIterations, MaxIterations)
		}
	} else {
		// §6.2's anti-downgrade-staging rule. These fields are meaningless
		// without a key, and accepting junk there would let an attacker stage
		// a downgrade that a later version might honour.
		if buf[9] != 0 {
			return Header{}, fmt.Errorf("%w: kdf_id", ErrUnsealedFieldNotZero)
		}
		if buf[10] != 0 {
			return Header{}, fmt.Errorf("%w: aead_id", ErrUnsealedFieldNotZero)
		}
		if iterations != 0 {
			return Header{}, fmt.Errorf("%w: iterations", ErrUnsealedFieldNotZero)
		}
		if !allZero(buf[16:32]) {
			return Header{}, fmt.Errorf("%w: salt", ErrUnsealedFieldNotZero)
		}
		if !allZero(buf[32:44]) {
			return Header{}, fmt.Errorf("%w: iv", ErrUnsealedFieldNotZero)
		}
	}

	// uint64 deliberately: TinyGo's int is 32-bit on this target, so
	// 52+pub_len+ct_len+16 evaluated natively wraps negative when either
	// length is near 2^32 and would pass a <= 65536 test.
	//
	// NOTE this check is UNREACHABLE behind the section caps above — the
	// largest total they admit is 52+8191+8191+16 = 16450, well under 65536 —
	// and wire.rs has no test for it either. It is defence in depth against a
	// future implementation that drops the caps, and is deliberately excluded
	// from the mutation table. Do not go looking for the test that kills it.
	var tag uint64
	if ctLen > 0 {
		tag = TagLen
	}
	total := uint64(HeaderLen) + uint64(pubLen) + uint64(ctLen) + tag
	if total > RegionLen {
		return Header{}, fmt.Errorf("%w: %d bytes, region is %d", ErrTooLarge, total, RegionLen)
	}

	h := Header{Iterations: iterations, PubLen: pubLen, CtLen: ctLen}
	copy(h.Salt[:], buf[16:32])
	copy(h.IV[:], buf[32:44])
	return h, nil
}

func allZero(b []byte) bool {
	for _, x := range b {
		if x != 0 {
			return false
		}
	}
	return true
}
