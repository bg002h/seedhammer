// Package md is the SeedHammer fork's Go port of the md1 (descriptor)
// constellation codec. md1 is PUBLIC; no secret handling.
//
// PROVENANCE PIN (Rust-primary rule). The wire format -- versions {4, 8}, the
// taproot internal key's three states and kind bit -- and the identity hashes
// track descriptor-mnemonic/crates/md-codec 0.47.0 (descriptor-mnemonic
// cf35d61a, tag descriptor-mnemonic-md-cli-v0.19.0), with the kind-1 vectors
// vendored from descriptor-mnemonic 430ea478.
//
// Converged since, against md-codec 0.48.7 (descriptor-mnemonic c8cde234),
// bug hunt 2026-11-25: Check over a bare PkK/PkH emits the bare key
// (to_miniscript.rs Check-idempotence, SH#1); policy_shape refuses a
// disjunction inside a branch (0.48.5, SH#2); decode refuses an @0 or
// redundant use-site override (0.37.0), an empty origin override (0.42.0) and
// a non-zero TLV padding tail (0.41.0) (SH#3).
//
// policy_shape converged against md-codec 0.49.0 (descriptor-mnemonic
// 6620a7dd; shape vectors vendored from d05655ac, whose only change on top is
// design/FOLLOWUPS.md), seedhammer I-1: the disjunction rule above is no longer
// the whole rule for a thresh. A thresh(k<n) at a path root whose every child
// is on the allowlist is ONE any-of entry (Branch.AnyK/AnyN); a k == n thresh
// is a conjunction only over the same allowlist; every other thresh makes the
// shape incomplete. THE TYPE RULE (md reviews r2 I-1, r3, r4; descriptor-mnemonic
// ca48a05e, f8b4a675, 5ad5a9b2; vectors re-vendored from 1dac0e10 with `git show`)
// then ANDs in whether the tree is valid miniscript in its context: Complete =
// walk && treeTypeChecks, a port of rust-miniscript's type, placement,
// script-size and satisfaction-limit checks (within_resource_limits) over
// md's conversion rules, every script root B and every tap leaf satisfiable
// (md/policy_typecheck.go, by the six groups of policy_type_checks' doc).
// md/policy_shape_cases_test.go holds the walk and the type rule to Rust on
// every vendored case; md/policy_shape_props_test.go enumerates the thresh
// allowlist.
//
// Known lags, each with a follow-up: pathless shared origins (F-166);
// md-codec's compose `--unspendable` selection and SPEC §6's kind-1 mint
// refusals (F-654); policy_shape's 0.48.0 "duplicate-index multi reports no
// k-of-n" in its bare spelling (plainMulti still reports one). NOT AUDITED
// against this port: the rest of 0.48.0-0.48.7 (among them 0.48.5's thresh
// older() BIP-68 check, origin/multipath values above 2^29-1, non-ASCII
// aliasing, and 0.48.6's out-of-order slot numbering at mint). The
// emitter has no miniscript type checker, so it emits trees Rust refuses at
// type-check (e.g. Check(Check(PkK))) instead of refusing them. compose.go
// carries its own, older pin (md-codec::compose at 66bdf2f4). Vendored
// corpora are pinned per file in md/testdata/*.provenance.json.
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
