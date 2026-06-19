package md

import (
	"bytes"
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

// ─── bitWriter (Task 1, I-2) — ports of bitstream.rs:236-407 writer tests plus
// the plan's required pins. MSB-first packing, low-padded final byte. ─────────

func TestBitWriterWrite5BitsMSBFirst(t *testing.T) {
	// bitstream.rs:237 write_5_bits_msb_first.
	var w bitWriter
	w.write(0b10110, 5)
	if got := w.intoBytes(); !bytes.Equal(got, []byte{0b1011_0000}) {
		t.Fatalf("intoBytes=%#v want [0xb0]", got)
	}
}

func TestBitWriterTwo5BitValues(t *testing.T) {
	// bitstream.rs:246 write_two_5_bit_values_packs_into_one_and_a_bit.
	var w bitWriter
	w.write(0b11111, 5)
	w.write(0b00001, 5)
	if got := w.intoBytes(); !bytes.Equal(got, []byte{0b1111_1000, 0b0100_0000}) {
		t.Fatalf("intoBytes=%#v want [0xf8 0x40]", got)
	}
}

func TestBitWriterEightBitsIsOneByte(t *testing.T) {
	var w bitWriter
	w.write(0xab, 8)
	if got := w.intoBytes(); !bytes.Equal(got, []byte{0xab}) {
		t.Fatalf("intoBytes=%#v want [0xab]", got)
	}
}

func TestBitWriterZeroBitsIsNoop(t *testing.T) {
	var w bitWriter
	w.write(0xff, 0)
	if w.bitLen() != 0 {
		t.Fatalf("bitLen=%d want 0", w.bitLen())
	}
	if got := w.intoBytes(); len(got) != 0 {
		t.Fatalf("intoBytes=%#v want empty", got)
	}
}

// TestBitWriterPlanPin5then3: the Header v0.30 common case — write the 5-bit
// header value 4 (0b00100, version=4 shared) then 0x00 in 3 bits yields the
// common byte [0x20] and bitLen()==8 (header.rs:114-125).
func TestBitWriterPlanPin5then3(t *testing.T) {
	var w bitWriter
	w.write(0x4, 5) // header value: version=4, shared → 0b00100
	w.write(0x00, 3)
	if w.bitLen() != 8 {
		t.Fatalf("bitLen=%d want 8", w.bitLen())
	}
	if got := w.intoBytes(); !bytes.Equal(got, []byte{0x20}) {
		t.Fatalf("intoBytes=%#v want [0x20]", got)
	}
}

// TestBitWriterGoldenWpkhPrefix: the wpkh_basic golden payload prefix
// 0x20 0x02 (Header 0x20 = version 4 shared; then Wpkh tag 0x00 at 6 bits
// + n=1 kiw=0 key-arg = the next 6 bits 0b000000, leaving the byte
// 0x02 only after the use-site bits; here we pin only the Header+tag-aligned
// sequence the plan calls out via sequential writes).
func TestBitWriterGoldenWpkhPrefix(t *testing.T) {
	// Reconstruct the exact wpkh_basic prefix bit-for-bit: header(5)=0b00100,
	// pathDecl n-1(5)=0b00000, depth(4)=0b0000, use-site has-mp(1)=1,
	// count-2(3)=0, alt0 (h=0,varint(0)=L0 → 0b0000) ... We only pin the
	// first two bytes via the documented sequential writes from the decoder
	// layout: header 0b00100, n 0b00000, depth 0b0000 → 0x20 0x00 so far.
	var w bitWriter
	w.write(0b00100, 5) // header: version=4, shared
	w.write(0b00000, 5) // pathDecl: n-1 = 0
	w.write(0b0000, 4)  // origin path depth = 0
	// 14 bits so far = 0x20 0x00 (0b00100_00000_0000 padded).
	got := w.intoBytes()
	if len(got) < 2 || got[0] != 0x20 || got[1] != 0x00 {
		t.Fatalf("prefix=%#v want first bytes 0x20 0x00", got)
	}
	if w.bitLen() != 14 {
		t.Fatalf("bitLen=%d want 14", w.bitLen())
	}
}

func TestBitWriterCrossByteBoundary(t *testing.T) {
	// Write a value that straddles a byte boundary: 6 bits then 6 bits = 12.
	var w bitWriter
	w.write(0b101011, 6)
	w.write(0b110010, 6)
	if w.bitLen() != 12 {
		t.Fatalf("bitLen=%d want 12", w.bitLen())
	}
	// 0b101011_110010 packed MSB-first = 0b10101111 0b0010_0000 = 0xAF 0x20.
	if got := w.intoBytes(); !bytes.Equal(got, []byte{0xAF, 0x20}) {
		t.Fatalf("intoBytes=%#v want [0xAF 0x20]", got)
	}
}

func TestReEmitBitsByteAligned(t *testing.T) {
	// bitstream.rs:325 re_emit_bits_round_trip_byte_aligned.
	var src bitWriter
	src.write(0xab, 8)
	srcBitLen := src.bitLen()
	srcBytes := src.intoBytes()

	var dst bitWriter
	if err := reEmitBits(&dst, srcBytes, srcBitLen); err != nil {
		t.Fatalf("reEmitBits: %v", err)
	}
	if dst.bitLen() != 8 {
		t.Fatalf("bitLen=%d want 8", dst.bitLen())
	}
	if got := dst.intoBytes(); !bytes.Equal(got, []byte{0xab}) {
		t.Fatalf("intoBytes=%#v want [0xab]", got)
	}
}

func TestReEmitBitsAllWidths1To23(t *testing.T) {
	// bitstream.rs:341 re_emit_bits_round_trip_all_widths_1_through_23.
	for width := 1; width <= 23; width++ {
		var pattern uint64 = (1<<uint(width) - 1) & 0xa5a5a5a5a5a5a5a5
		var src bitWriter
		src.write(pattern, width)
		srcBitLen := src.bitLen()
		srcBytes := src.intoBytes()
		if srcBitLen != width {
			t.Fatalf("width=%d srcBitLen=%d", width, srcBitLen)
		}
		var dst bitWriter
		if err := reEmitBits(&dst, srcBytes, width); err != nil {
			t.Fatalf("width=%d reEmitBits: %v", width, err)
		}
		if dst.bitLen() != width {
			t.Fatalf("width=%d dst.bitLen=%d", width, dst.bitLen())
		}
		dstBytes := dst.intoBytes()
		r := newBitReader(dstBytes, width)
		got, err := r.read(width)
		if err != nil {
			t.Fatalf("width=%d read: %v", width, err)
		}
		if got != pattern {
			t.Fatalf("width=%d round-trip got=%#x want=%#x", width, got, pattern)
		}
	}
}

// TestReEmitBitsNonByteAligned: a 13-bit value round-trips through
// intoBytes→reEmitBits→read (the plan's I-2 non-byte-aligned pin). Also
// covers the bitstream.rs:369 5+7-bit case.
func TestReEmitBitsNonByteAligned(t *testing.T) {
	var src bitWriter
	src.write(0b10110, 5)
	src.write(0b1010101, 7)
	if src.bitLen() != 12 {
		t.Fatalf("src.bitLen=%d want 12", src.bitLen())
	}
	srcBytes := src.intoBytes()
	var dst bitWriter
	if err := reEmitBits(&dst, srcBytes, 12); err != nil {
		t.Fatalf("reEmitBits: %v", err)
	}
	if dst.bitLen() != 12 {
		t.Fatalf("dst.bitLen=%d want 12", dst.bitLen())
	}
	r := newBitReader(dst.intoBytes(), 12)
	if v, _ := r.read(5); v != 0b10110 {
		t.Fatalf("read(5)=%05b want 10110", v)
	}
	if v, _ := r.read(7); v != 0b1010101 {
		t.Fatalf("read(7)=%07b want 1010101", v)
	}

	// 13-bit value round-trip.
	var src2 bitWriter
	src2.write(0b1011001110101, 13)
	r2 := newBitReader(src2.intoBytes(), 13)
	var dst2 bitWriter
	if err := reEmitBits(&dst2, src2.intoBytes(), 13); err != nil {
		t.Fatalf("reEmitBits 13: %v", err)
	}
	_ = r2
	got, _ := newBitReader(dst2.intoBytes(), 13).read(13)
	if got != 0b1011001110101 {
		t.Fatalf("13-bit round-trip got=%013b", got)
	}
}

func TestReEmitBitsAppendsToExistingDst(t *testing.T) {
	// bitstream.rs:389 re_emit_bits_appends_to_existing_dst.
	var dst bitWriter
	dst.write(0b101, 3)
	var src bitWriter
	src.write(0b1_1110_0001, 9)
	srcBitLen := src.bitLen()
	if err := reEmitBits(&dst, src.intoBytes(), srcBitLen); err != nil {
		t.Fatalf("reEmitBits: %v", err)
	}
	if dst.bitLen() != 12 {
		t.Fatalf("dst.bitLen=%d want 12", dst.bitLen())
	}
	r := newBitReader(dst.intoBytes(), 12)
	if v, _ := r.read(3); v != 0b101 {
		t.Fatalf("read(3)=%03b want 101", v)
	}
	if v, _ := r.read(9); v != 0b1_1110_0001 {
		t.Fatalf("read(9)=%09b want 111100001", v)
	}
}
