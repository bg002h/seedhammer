// Package md decodes single-string md1 (descriptor) constellation strings into
// a human-readable BIP-388 template. md1 is PUBLIC; no secret handling. Wire
// format: descriptor-mnemonic/crates/md-codec @ 0.36.0 (decode_md1_string path).
// Chunked md1 is detected and refused (ErrChunkedUnsupported); reassembly +
// wallet-policy xpub-expansion are out of scope (ledger #10).
package md

import "errors"

var errTruncated = errors.New("md: bit stream truncated")

// bitReader is an MSB-first bit unpacker over a byte slice with a bit limit
// (port of md-codec bitstream.rs BitReader).
type bitReader struct {
	bytes    []byte
	bitPos   int
	bitLimit int
}

func newBitReader(b []byte, bitLimit int) *bitReader {
	return &bitReader{bytes: b, bitPos: 0, bitLimit: bitLimit}
}

func (r *bitReader) remaining() int {
	if r.bitLimit < r.bitPos {
		return 0
	}
	return r.bitLimit - r.bitPos
}

// availBits is the number of bits the reader may legally read from the current
// position, bounded by BOTH the logical bitLimit and the backing slice length.
// md-codec asserts bit_limit <= len(bytes)*8 via debug_assert!; the Go port
// promotes that to a hard runtime guard so a violated precondition (e.g.
// bitLimit > len(bytes)*8) returns errTruncated from read() instead of panicking
// out of bounds. Unreachable from Decode (the sole caller passes
// bitLimit = 5*len(syms) <= len(symbolsToBytes(syms))*8), but defended here.
func (r *bitReader) availBits() int {
	avail := r.remaining()
	sliceBits := len(r.bytes)*8 - r.bitPos
	if sliceBits < 0 {
		sliceBits = 0
	}
	if sliceBits < avail {
		return sliceBits
	}
	return avail
}

// read returns the next count bits (count<=64) MSB-first, LSB-aligned.
func (r *bitReader) read(count int) (uint64, error) {
	if count == 0 {
		return 0, nil
	}
	if r.availBits() < count {
		return 0, errTruncated
	}
	var result uint64
	rem := count
	for rem > 0 {
		byteIdx := r.bitPos / 8
		bitInByte := r.bitPos % 8
		freeInByte := 8 - bitInByte
		chunk := rem
		if chunk > freeInByte {
			chunk = freeInByte
		}
		shift := uint(freeInByte - chunk)
		var mask byte
		if chunk == 8 {
			mask = 0xff
		} else {
			mask = byte(1<<uint(chunk)) - 1
		}
		bits := (r.bytes[byteIdx] >> shift) & mask
		result = (result << uint(chunk)) | uint64(bits)
		r.bitPos += chunk
		rem -= chunk
	}
	return result, nil
}

func (r *bitReader) readBool() (bool, error) { v, err := r.read(1); return v != 0, err }
func (r *bitReader) pos() int                { return r.bitPos }
func (r *bitReader) restore(p int)           { r.bitPos = p }
func (r *bitReader) limit() int              { return r.bitLimit }
func (r *bitReader) setLimit(l int)          { r.bitLimit = l }

// bitWriter is an MSB-first bit packer (port of md-codec bitstream.rs:11-84
// BitWriter). The first bit written occupies the most-significant bit of the
// first byte; the final in-progress byte is zero-padded on its LOW bits when
// intoBytes is called. Mirrors the throwaway testBitWriter algorithm (md_test.go).
type bitWriter struct {
	bytes []byte
	// bitPosition is the bit offset within the last byte, in 0..8. Zero means
	// no in-progress byte (the next write pushes a fresh byte).
	bitPosition int
}

// write packs count bits from value (LSB-aligned in value) into the stream
// MSB-first. Bits beyond count in value are ignored. count must be <=64.
func (w *bitWriter) write(value uint64, count int) {
	if count == 0 {
		return
	}
	var masked uint64
	if count == 64 {
		masked = value
	} else {
		masked = value & ((uint64(1) << uint(count)) - 1)
	}
	remaining := count
	for remaining > 0 {
		if w.bitPosition == 0 {
			w.bytes = append(w.bytes, 0)
		}
		lastIdx := len(w.bytes) - 1
		freeInByte := 8 - w.bitPosition
		chunk := remaining
		if chunk > freeInByte {
			chunk = freeInByte
		}
		shift := uint(remaining - chunk)
		bitsVal := byte((masked >> shift) & ((uint64(1) << uint(chunk)) - 1))
		byteShift := uint(freeInByte - chunk)
		w.bytes[lastIdx] |= bitsVal << byteShift
		w.bitPosition += chunk
		if w.bitPosition == 8 {
			w.bitPosition = 0
		}
		remaining -= chunk
	}
}

// bitLen returns the total number of bits written.
func (w *bitWriter) bitLen() int {
	if w.bitPosition == 0 {
		return len(w.bytes) * 8
	}
	return (len(w.bytes)-1)*8 + w.bitPosition
}

// intoBytes returns the byte stream; the final in-progress byte is already
// low-bit zero-padded (the buffer is returned as-is, mirroring Rust's
// into_bytes which hands back the Vec). The returned slice aliases the
// writer's buffer; callers must not mutate it while reusing the writer.
func (w *bitWriter) intoBytes() []byte {
	return w.bytes
}

// reEmitBits reads the first bitLen bits of payload MSB-first (as if payload
// were the output of a bitWriter finalized with intoBytes, so the trailing
// partial byte is in the high bits of the last source byte) and writes them
// into dst. The destination is extended in place — no padding inserted. Port
// of bitstream.rs:220-230 re_emit_bits.
func reEmitBits(dst *bitWriter, payload []byte, bitLen int) error {
	src := newBitReader(payload, bitLen)
	remaining := bitLen
	for remaining > 0 {
		chunk := remaining
		if chunk > 8 {
			chunk = 8
		}
		v, err := src.read(chunk)
		if err != nil {
			return err
		}
		dst.write(v, chunk)
		remaining -= chunk
	}
	return nil
}
