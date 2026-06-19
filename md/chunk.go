package md

import (
	"errors"

	"seedhammer.com/codex32"
)

// ─── Chunked md1 write/read (port of chunk.rs). ──────────────────────────────

// ErrChunkSetIncomplete is returned by Reassemble / DecodeChunks when the
// gathered chunk set is missing one or more chunks (count, gap, or a duplicate
// that inflates the set past the declared count). Exported so package gui can
// errors.Is-dispatch a distinct "more chunks needed" outcome (R0-C1).
var ErrChunkSetIncomplete = errors.New("md: chunk set incomplete")

// ErrChunkSetIDMismatch is returned by Reassemble / DecodeChunks when the chunk
// set is internally consistent (same version/csid/count) but the csid
// re-derived from the decoded descriptor does not match the header csid —
// i.e. a mixed or tampered set. Exported so package gui can errors.Is-dispatch
// the distinct csid-mismatch UX, separate from a generic decode failure (R0-C1).
var ErrChunkSetIDMismatch = errors.New("md: chunk set id integrity mismatch")

// Chunk errors.
var (
	errChunkCountRange   = errors.New("md: chunk count out of range 1..64")
	errChunkIndexRange   = errors.New("md: chunk index >= count")
	errChunkSetIDRange   = errors.New("md: chunk set id exceeds 20 bits")
	errChunkCountExceeds = errors.New("md: payload exceeds 64-chunk maximum")
	errChunkFlagMissing  = errors.New("md: chunk header chunked-flag not set")
	errChunkSetEmpty     = errors.New("md: empty chunk set")
	errChunkSetInconsist = errors.New("md: chunk set inconsistent (version/csid/count)")
	errChunkIndexGap     = errors.New("md: chunk index gap")
	errNotChunked        = errors.New("md: string is not chunked")
)

// SINGLE_STRING_PAYLOAD_BIT_LIMIT — the threshold (in payload bits) above which
// chunking is required: 64 data symbols × 5 = 320 bits (chunk.rs:219).
const singleStringPayloadBitLimit = 64 * 5

// chunkHeaderBits is the fixed chunk-header width: 4+1+20+6+6 = 37 (chunk.rs).
const chunkHeaderBits = 37

// ChunkHeader is the 37-bit chunked-wire header: [version:4][chunked=1:1]
// [csid:20][count-1:6][index:6] — version in the TOP 4 bits, distinct from the
// single Header's low-nibble version (chunk.rs:32-57). When parsed from a
// single (non-chunked) md1 string, Chunked is false and the remaining fields
// are zero (the syms[0]&1 discriminator is consulted FIRST, I-3).
type ChunkHeader struct {
	Version     uint8
	Chunked     bool
	ChunkSetID  uint32
	TotalChunks int
	ChunkIndex  int
}

// write encodes the chunk header as 37 bits (chunk.rs:36-57). Guards
// count∈1..64, index<count, csid<2^20.
func (h ChunkHeader) write(w *bitWriter) error {
	if h.TotalChunks < 1 || h.TotalChunks > 64 {
		return errChunkCountRange
	}
	if h.ChunkIndex < 0 || h.ChunkIndex >= h.TotalChunks {
		return errChunkIndexRange
	}
	if h.ChunkSetID >= (1 << 20) {
		return errChunkSetIDRange
	}
	w.write(uint64(h.Version&0b1111), 4)
	w.write(1, 1) // chunked = 1
	w.write(uint64(h.ChunkSetID), 20)
	w.write(uint64(h.TotalChunks-1), 6) // count-1 offset
	w.write(uint64(h.ChunkIndex), 6)
	return nil
}

// readChunkHeader reads a 37-bit chunk header from r (chunk.rs:67-85). Returns
// errWireVersion if the 4-bit version != WF_REDESIGN_VERSION, and
// errChunkFlagMissing if the chunked-flag is clear. This is the post-discriminator
// parse: callers MUST gate it behind the syms[0]&1 bit-0 probe (I-3).
func readChunkHeader(r *bitReader) (ChunkHeader, error) {
	v, err := r.read(4)
	if err != nil {
		return ChunkHeader{}, err
	}
	version := uint8(v)
	if version != wfRedesignVersion {
		return ChunkHeader{}, errWireVersion
	}
	chunked, err := r.readBool()
	if err != nil {
		return ChunkHeader{}, err
	}
	if !chunked {
		return ChunkHeader{}, errChunkFlagMissing
	}
	csid, err := r.read(20)
	if err != nil {
		return ChunkHeader{}, err
	}
	countRaw, err := r.read(6)
	if err != nil {
		return ChunkHeader{}, err
	}
	index, err := r.read(6)
	if err != nil {
		return ChunkHeader{}, err
	}
	return ChunkHeader{
		Version:     version,
		Chunked:     true,
		ChunkSetID:  uint32(csid),
		TotalChunks: int(countRaw) + 1,
		ChunkIndex:  int(index),
	}, nil
}

// split encodes a descriptor into N codex32 md1 chunk strings, each carrying a
// 37-bit chunk header + a byte-aligned slice of the canonical payload
// (chunk.rs:235-290). N = max(1, ceil(payloadBytes*8 / 320)); >64 → error.
func split(d *descriptor) ([]string, error) {
	payloadBytes, _, err := encodePayload(d)
	if err != nil {
		return nil, err
	}
	id, err := computeEncodingID(d)
	if err != nil {
		return nil, err
	}
	csid := deriveChunkSetID(id)

	payloadBitCountForSizing := len(payloadBytes) * 8
	chunksNeeded := ceilDiv(payloadBitCountForSizing, singleStringPayloadBitLimit)
	if chunksNeeded > 64 {
		return nil, errChunkCountExceeds
	}
	count := chunksNeeded
	if count == 0 {
		count = 1
	}

	bytesPerChunk := ceilDiv(len(payloadBytes), count)

	chunks := make([]string, 0, count)
	for index := 0; index < count; index++ {
		startByte := index * bytesPerChunk
		endByte := (index + 1) * bytesPerChunk
		if endByte > len(payloadBytes) {
			endByte = len(payloadBytes)
		}
		if startByte > len(payloadBytes) {
			startByte = len(payloadBytes)
		}
		chunkPayload := payloadBytes[startByte:endByte]

		var w bitWriter
		hdr := ChunkHeader{
			Version:     wfRedesignVersion,
			Chunked:     true,
			ChunkSetID:  csid,
			TotalChunks: count,
			ChunkIndex:  index,
		}
		if err := hdr.write(&w); err != nil {
			return nil, err
		}
		for _, by := range chunkPayload {
			w.write(uint64(by), 8)
		}
		chunkBitCount := chunkHeaderBits + 8*len(chunkPayload)
		syms, err := bitsToSymbols(w.intoBytes(), chunkBitCount)
		if err != nil {
			return nil, err
		}
		chunks = append(chunks, codex32.AssembleMD1(syms))
	}
	return chunks, nil
}

// ParseChunkHeader parses the chunk header of an md1 string. It consults the
// single/chunked discriminator (bit 0 of the first 5-bit data symbol) FIRST
// (I-3): for a single (non-chunked) string it returns ChunkHeader{Chunked:false}
// without attempting a 37-bit parse; for a chunked string it reads the 37-bit
// header. Mirrors the mk.Header shape.
func ParseChunkHeader(s string) (ChunkHeader, error) {
	syms, err := codex32.MDDataSymbols(s)
	if err != nil {
		return ChunkHeader{}, err
	}
	if len(syms) == 0 {
		return ChunkHeader{}, errTruncated
	}
	if syms[0]&1 == 0 {
		// Single (non-chunked) payload — never ParseChunkHeader-then-catch.
		return ChunkHeader{Chunked: false}, nil
	}
	b := symbolsToBytes(syms)
	r := newBitReader(b, 5*len(syms))
	return readChunkHeader(r)
}

// Reassemble decodes a descriptor from N md1 chunk strings (chunk.rs:305-389):
// unwrap each → read 37-bit header → consistency (version/csid/count all-equal)
// → completeness (len==count, sorted indices 0..count-1, no gaps) → concat
// payload bytes → decodePayloadValidated → integrity gate (re-derive csid from
// the decoded descriptor, compare to the header csid).
func Reassemble(strs []string) (*descriptor, error) {
	if len(strs) == 0 {
		return nil, errChunkSetEmpty
	}

	type parsedChunk struct {
		header  ChunkHeader
		payload []byte
	}
	parsed := make([]parsedChunk, 0, len(strs))
	for _, s := range strs {
		b, symBits, err := unwrapString(s)
		if err != nil {
			return nil, err
		}
		r := newBitReader(b, symBits)
		// The chunked-flag is bit 0 of symbol 0 (I-3): the header's first 4 bits
		// are the version, then the chunked bit. readChunkHeader enforces both.
		h, err := readChunkHeader(r)
		if err != nil {
			return nil, err
		}
		// M-5: recover the exact payload byte count from the symbol-aligned bit
		// count, NOT len(b)*8. The chunk wire is exactly 37+8N bits, and symBits
		// = ceil((37+8N)/5)*5 ∈ [37+8N, 37+8N+4], so (symBits-37)/8 (floor) = N.
		payloadByteCount := (symBits - chunkHeaderBits) / 8
		cp := make([]byte, 0, payloadByteCount)
		for i := 0; i < payloadByteCount; i++ {
			v, err := r.read(8)
			if err != nil {
				return nil, err
			}
			cp = append(cp, byte(v))
		}
		parsed = append(parsed, parsedChunk{header: h, payload: cp})
	}

	// Consistency: same version, csid, count across all chunks.
	expCount := parsed[0].header.TotalChunks
	expCsid := parsed[0].header.ChunkSetID
	expVersion := parsed[0].header.Version
	for _, p := range parsed {
		if p.header.TotalChunks != expCount || p.header.ChunkSetID != expCsid || p.header.Version != expVersion {
			return nil, errChunkSetInconsist
		}
	}
	if len(parsed) != expCount {
		return nil, ErrChunkSetIncomplete
	}

	// Sort by index (stable insertion sort; small N, no reflect); verify
	// 0..count-1 with no gaps.
	for i := 1; i < len(parsed); i++ {
		j := i
		for j > 0 && parsed[j-1].header.ChunkIndex > parsed[j].header.ChunkIndex {
			parsed[j-1], parsed[j] = parsed[j], parsed[j-1]
			j--
		}
	}
	for i, p := range parsed {
		if p.header.ChunkIndex != i {
			return nil, errChunkIndexGap
		}
	}

	// Concatenate payload bytes.
	var full []byte
	for _, p := range parsed {
		full = append(full, p.payload...)
	}

	// Decode (TLV-rollback tolerates ≤7 trailing zero bits, md.go:549-554).
	d, err := decodePayloadValidated(full, len(full)*8)
	if err != nil {
		return nil, err
	}

	// Cross-chunk integrity: re-derive the csid from the decoded descriptor.
	id, err := computeEncodingID(d)
	if err != nil {
		return nil, err
	}
	if deriveChunkSetID(id) != expCsid {
		return nil, ErrChunkSetIDMismatch
	}
	return d, nil
}

// unwrapString verifies an md1 string's BCH, strips the 13-symbol checksum, and
// returns (byte-padded payload bytes, symbol-aligned bit count = 5×dataSymbols).
// The caller uses the symbol-aligned bit count (NOT len(bytes)*8) so the
// recovered payload byte count is exact (codex32.rs:113-161 unwrap_string).
func unwrapString(s string) ([]byte, int, error) {
	syms, err := codex32.MDDataSymbols(s)
	if err != nil {
		return nil, 0, err
	}
	return symbolsToBytes(syms), 5 * len(syms), nil
}

// ceilDiv returns ceil(a/b) for non-negative a and positive b.
func ceilDiv(a, b int) int {
	if b == 0 {
		return 0
	}
	return (a + b - 1) / b
}
