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

// read returns the next count bits (count<=64) MSB-first, LSB-aligned.
func (r *bitReader) read(count int) (uint64, error) {
	if count == 0 {
		return 0, nil
	}
	if r.remaining() < count {
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
