package sysw

import (
	"encoding/binary"
	"errors"
	"fmt"
)

var (
	ErrTooShort       = errors.New("sysw: shorter than a header")
	ErrBadMagic       = errors.New("sysw: not a systemwide container")
	ErrVersion        = errors.New("sysw: unknown version")
	ErrKDF            = errors.New("sysw: unknown kdf")
	ErrAEAD           = errors.New("sysw: unknown aead")
	ErrSectionTooLong = errors.New("sysw: section too long")
	ErrIterations     = errors.New("sysw: iteration count out of range")
)

// ParseHeader bound-checks before anything else uses these numbers.
//
// EVERY check runs before any KDF work. The firmware has no active watchdog, so
// an unbounded iteration count is a hang rather than a slow open, and both
// section lengths are attacker-controlled until proven otherwise. Nothing below
// may be used for arithmetic before its bound.
func ParseHeader(buf []byte) (Header, error) {
	if len(buf) < HeaderLen {
		return Header{}, fmt.Errorf("%w: %d bytes", ErrTooShort, len(buf))
	}
	if [8]byte(buf[:8]) != MAGIC {
		return Header{}, ErrBadMagic
	}
	if buf[8] != Version {
		return Header{}, fmt.Errorf("%w: %d", ErrVersion, buf[8])
	}
	var h Header
	h.Iterations = binary.BigEndian.Uint32(buf[12:16])
	copy(h.Salt[:], buf[16:32])
	copy(h.IV[:], buf[32:44])
	h.PubLen = binary.BigEndian.Uint32(buf[44:48])
	h.CtLen = binary.BigEndian.Uint32(buf[48:52])

	if int(h.PubLen) > MaxSectionLen || int(h.CtLen) > MaxSectionLen {
		return Header{}, fmt.Errorf("%w: pub_len=%d ct_len=%d cap=%d",
			ErrSectionTooLong, h.PubLen, h.CtLen, MaxSectionLen)
	}
	if h.Sealed() {
		if buf[9] != KDFPBKDF2SHA256 {
			return Header{}, fmt.Errorf("%w: %d", ErrKDF, buf[9])
		}
		if buf[10] != AEADAES256GCM {
			return Header{}, fmt.Errorf("%w: %d", ErrAEAD, buf[10])
		}
		if h.Iterations < MinIterations || h.Iterations > MaxIterations {
			return Header{}, fmt.Errorf("%w: %d", ErrIterations, h.Iterations)
		}
	}
	return h, nil
}

// Encode exists for TESTS and for the conformance harness. The device never
// writes a container; see the package comment.
func (h Header) Encode() [HeaderLen]byte {
	var out [HeaderLen]byte
	copy(out[:8], MAGIC[:])
	out[8] = Version
	if h.Sealed() {
		out[9] = KDFPBKDF2SHA256
		out[10] = AEADAES256GCM
	}
	binary.BigEndian.PutUint32(out[12:16], h.Iterations)
	copy(out[16:32], h.Salt[:])
	copy(out[32:44], h.IV[:])
	binary.BigEndian.PutUint32(out[44:48], h.PubLen)
	binary.BigEndian.PutUint32(out[48:52], h.CtLen)
	return out
}
