package sysw

import (
	"encoding/binary"
	"errors"
	"testing"
)

// The section cap must be compared in UNSIGNED space. `int` is 32 bits on the
// device (Cortex-M33 via tinygo), so widening a uint32 to int makes any value
// with the top bit set negative, and `int(pubLen) > MaxSectionLen` then reads
// false. Measured under GOARCH=386 before the fix: pub_len=0x80000000 was
// ACCEPTED with TotalLen() == -2147483580, and pub_len=0xFFFFFFFF was accepted
// with TotalLen() == 67 — a small POSITIVE length, so the device would have
// parsed a payload the 64-bit host rejects as malformed.
//
// The Rust primary compares `as usize`, which is unsigned and correct at both
// widths, so this was a Go-only porting error and was fixed here as convergence
// rather than led from Go.
//
// HONEST LIMIT OF THIS TEST: on a 64-bit builder it passes before the fix too,
// because int is wide enough there. It pins intent and it fails on a 32-bit
// builder. Catching the regression automatically needs the suite run for a
// 32-bit GOARCH, which is filed as a follow-up rather than pretended here.
func TestSectionCapIsComparedUnsigned(t *testing.T) {
	for _, pubLen := range []uint32{
		MaxSectionLen + 1,
		1 << 31,    // top bit set: int() goes negative on 32-bit
		0xFFFFFFFF, // int() == -1, and TotalLen() wraps to a plausible 67
	} {
		buf := make([]byte, HeaderLen)
		copy(buf, MAGIC[:])
		buf[8] = Version
		binary.BigEndian.PutUint32(buf[44:48], pubLen)
		if _, err := ParseHeader(buf); !errors.Is(err, ErrSectionTooLong) {
			t.Errorf("pub_len=%#x: want ErrSectionTooLong, got %v", pubLen, err)
		}
	}
}

// Same widening, the ct_len half. Kept separate so a fix to one field that
// misses the other cannot pass.
func TestSectionCapCoversCtLen(t *testing.T) {
	buf := make([]byte, HeaderLen)
	copy(buf, MAGIC[:])
	buf[8] = Version
	binary.BigEndian.PutUint32(buf[48:52], 0xFFFFFFFF)
	if _, err := ParseHeader(buf); !errors.Is(err, ErrSectionTooLong) {
		t.Errorf("ct_len=0xFFFFFFFF: want ErrSectionTooLong, got %v", err)
	}
}
