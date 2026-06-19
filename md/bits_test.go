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
