package md

import (
	"errors"
	"testing"
)

func TestBitReader(t *testing.T) {
	// 0xA5 = 1010_0101. MSB-first reads.
	r := newBitReader([]byte{0xA5}, 8)
	if v, _ := r.read(4); v != 0b1010 {
		t.Fatalf("read(4)=%04b want 1010", v)
	}
	if v, _ := r.read(2); v != 0b01 {
		t.Fatalf("read(2)=%02b want 01", v)
	}
	if v, _ := r.read(2); v != 0b01 {
		t.Fatalf("read(2)=%02b want 01", v)
	}
	if _, err := r.read(1); !errors.Is(err, errTruncated) {
		t.Fatalf("over-read: want errTruncated, got %v", err)
	}
	// bitLimit shorter than the byte buffer.
	r2 := newBitReader([]byte{0xFF}, 3)
	if v, _ := r2.read(3); v != 0b111 {
		t.Fatalf("read(3)=%03b", v)
	}
	if _, err := r2.read(1); !errors.Is(err, errTruncated) {
		t.Fatalf("limit over-read: want errTruncated, got %v", err)
	}
	// save/restore + scoped limit (for TLV).
	r3 := newBitReader([]byte{0xFF, 0xFF}, 16)
	save := r3.pos()
	r3.read(5)
	r3.restore(save)
	if r3.pos() != save {
		t.Fatal("restore failed")
	}
}

// TestBitReaderLimitExceedsSlice exercises the MINOR-1 hardening: a violated
// precondition (bitLimit > len(bytes)*8 — the dropped Rust
// debug_assert!(bit_limit <= bytes.len()*8)) MUST return errTruncated, not
// panic out-of-bounds. Unreachable from Decode (proven), but defended.
func TestBitReaderLimitExceedsSlice(t *testing.T) {
	defer func() {
		if p := recover(); p != nil {
			t.Fatalf("read panicked on over-long bitLimit: %v", p)
		}
	}()
	r := newBitReader([]byte{}, 64) // bitLimit 64 > 0 available bits
	if _, err := r.read(5); !errors.Is(err, errTruncated) {
		t.Fatalf("read on empty slice: want errTruncated, got %v", err)
	}
	// A non-empty slice with bitLimit past its end: read past the real bytes
	// must also truncate, not panic.
	r2 := newBitReader([]byte{0xFF}, 64) // 8 real bits, limit claims 64
	if v, err := r2.read(8); err != nil || v != 0xFF {
		t.Fatalf("read(8) on full byte: v=%#x err=%v", v, err)
	}
	if _, err := r2.read(1); !errors.Is(err, errTruncated) {
		t.Fatalf("read past slice end: want errTruncated, got %v", err)
	}
}
