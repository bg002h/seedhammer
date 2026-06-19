// Package mk decodes mk1 (account-xpub) constellation strings into the
// account metadata they carry: network, derivation path, origin fingerprint,
// policy-id stubs, and the BIP-32 account xpub. mk1 is PUBLIC; this package
// performs no secret handling. Wire format: mnemonic-key/crates/mk-codec
// (family_token "mk-codec 0.2").
package mk

// Task 2 imports ONLY what Task 2's code uses (Go rejects unused imports).
// Task 3 expands this block to the full set when it appends Decode/reassemble.
import (
	"errors"
	"fmt"

	"seedhammer.com/codex32"
)

// Decode/reassembly error sentinels.
var (
	errEmptyInput             = errors.New("mk: empty input")
	errUnexpectedEnd          = errors.New("mk: unexpected end of data")
	errTrailingBytes          = errors.New("mk: trailing bytes after xpub")
	errReservedBits           = errors.New("mk: reserved header bits set")
	errStubCount              = errors.New("mk: stub_count must be >= 1")
	errMalformedPadding       = errors.New("mk: malformed payload padding")
	errChunkedHeaderMalformed = errors.New("mk: chunked header malformed")
	errMixedHeaderTypes       = errors.New("mk: mixed header types in chunk set")
	errChunkSetIDMismatch     = errors.New("mk: chunk_set_id mismatch")
	errDuplicateChunk         = errors.New("mk: duplicate chunk index")
	errCrossChunkHash         = errors.New("mk: cross-chunk integrity hash mismatch")
	errPathTooDeep            = errors.New("mk: path too deep")
	errPathComponent          = errors.New("mk: invalid path component")
)

const (
	mkVersionV01      = 0x00
	typeSingle        = 0x00
	typeChunked       = 0x01
	singleHeaderSyms  = 2
	chunkedHeaderSyms = 8
	maxChunks         = 32
)

// Header is a parsed string-layer header for one mk1 string.
type Header struct {
	Chunked     bool
	ChunkSetID  uint32
	TotalChunks int // 1 for single-string; >=2 in practice.
	ChunkIndex  int // 0-based.
}

// ParseHeader extracts the string-layer header from one BCH-valid mk1 string.
func ParseHeader(s string) (Header, error) {
	syms, err := codex32.MKDataSymbols(s)
	if err != nil {
		return Header{}, err
	}
	h, _, err := parseHeaderSyms(syms)
	return h, err
}

// parseHeaderSyms reads the string-layer header off the front of syms and
// returns it plus the number of symbols consumed (2 single / 8 chunked).
func parseHeaderSyms(syms []byte) (Header, int, error) {
	if len(syms) < singleHeaderSyms {
		return Header{}, 0, errUnexpectedEnd
	}
	if version := syms[0] & 0x1f; version != mkVersionV01 {
		return Header{}, 0, fmt.Errorf("mk: unsupported version: 0x%02x", version)
	}
	switch syms[1] & 0x1f {
	case typeSingle:
		return Header{Chunked: false, TotalChunks: 1, ChunkIndex: 0}, singleHeaderSyms, nil
	case typeChunked:
		if len(syms) < chunkedHeaderSyms {
			return Header{}, 0, errUnexpectedEnd
		}
		csid := uint32(syms[2]&0x1f)<<15 | uint32(syms[3]&0x1f)<<10 |
			uint32(syms[4]&0x1f)<<5 | uint32(syms[5]&0x1f)
		total := int(syms[6]&0x1f) + 1 // value-1 on the wire.
		index := int(syms[7] & 0x1f)   // verbatim, 0-based — NOT value-1 (R0-C1).
		if total > maxChunks || index >= total {
			return Header{}, 0, errChunkedHeaderMalformed
		}
		return Header{Chunked: true, ChunkSetID: csid, TotalChunks: total, ChunkIndex: index}, chunkedHeaderSyms, nil
	default:
		return Header{}, 0, fmt.Errorf("mk: unsupported card type: 0x%02x", syms[1]&0x1f)
	}
}

// fiveBitToBytes repacks 5-bit symbols into bytes, rejecting any symbol >= 32,
// a leftover group of >= 5 bits, or non-zero trailing padding bits (mk-codec
// string_layer/bch.rs:78-100). Unlike codex32's parts.data() it never panics
// and never silently drops a partial byte.
func fiveBitToBytes(syms []byte) ([]byte, error) {
	var acc uint32
	var bits uint
	out := make([]byte, 0, len(syms)*5/8)
	for _, v := range syms {
		if v >= 32 {
			return nil, errMalformedPadding
		}
		acc = acc<<5 | uint32(v)
		bits += 5
		if bits >= 8 {
			bits -= 8
			out = append(out, byte(acc>>bits&0xff))
		}
	}
	if bits >= 5 {
		return nil, errMalformedPadding
	}
	if acc&(1<<bits-1) != 0 {
		return nil, errMalformedPadding
	}
	return out, nil
}
