package md

import (
	"testing"

	"seedhammer.com/codex32"
)

// chunkedMD1Vector hand-builds the Go equivalent of me-cli/src/bundle.rs:547-585
// chunked_md1_vector: a 6-key wsh(sortedmulti(2,...)) with 15-deep divergent
// origin paths, whose split() yields >=4 md1 chunks (R0-M2). No .bytes.hex /
// .phrase.txt golden — used only for the chunked round-trip.
func chunkedMD1Vector() *descriptor {
	paths := make([]originPath, 6)
	for c := 0; c < 6; c++ {
		comps := make([]pathComponent, 15)
		for i := 0; i < 15; i++ {
			comps[i] = pathComponent{hardened: true, value: uint32(c*100 + i + 1)}
		}
		paths[c] = originPath{components: comps}
	}
	indices := make([]uint8, 6)
	for i := range indices {
		indices[i] = uint8(i)
	}
	return &descriptor{
		n:        6,
		pathDecl: pathDecl{n: 6, divergent: paths},
		useSite:  useSitePath{hasMultipath: true, multipath: []alternative{{value: 0}, {value: 1}}},
		tree: node{tag: tagWsh, body: childrenBody{children: []node{{
			tag: tagSortedMulti, body: multiKeysBody{k: 2, indices: indices},
		}}}},
	}
}

// TestSplitChunkedMD1Vector: the ≥4-chunk fixture. Each chunk is ValidMD, the
// chunk count matches deriveChunkSetID's sizing, and every chunk's parsed
// header reports the same csid & total with indices 0..count-1.
func TestSplitChunkedMD1Vector(t *testing.T) {
	d := chunkedMD1Vector()
	chunks, err := split(d)
	if err != nil {
		t.Fatalf("split: %v", err)
	}
	if len(chunks) < 4 {
		t.Fatalf("split produced %d chunks, want >=4", len(chunks))
	}

	id, err := computeEncodingID(d)
	if err != nil {
		t.Fatalf("computeEncodingID: %v", err)
	}
	wantCsid := deriveChunkSetID(id)

	seenIdx := make(map[int]bool)
	for i, s := range chunks {
		if !codex32.ValidMD(s) {
			t.Fatalf("chunk %d fails ValidMD: %s", i, s)
		}
		h, err := ParseChunkHeader(s)
		if err != nil {
			t.Fatalf("chunk %d ParseChunkHeader: %v", i, err)
		}
		if !h.Chunked {
			t.Fatalf("chunk %d: Chunked=false", i)
		}
		if h.ChunkSetID != wantCsid {
			t.Fatalf("chunk %d: csid=%#x want %#x", i, h.ChunkSetID, wantCsid)
		}
		if int(h.TotalChunks) != len(chunks) {
			t.Fatalf("chunk %d: total=%d want %d", i, h.TotalChunks, len(chunks))
		}
		seenIdx[int(h.ChunkIndex)] = true
	}
	for i := 0; i < len(chunks); i++ {
		if !seenIdx[i] {
			t.Fatalf("missing chunk index %d", i)
		}
	}
}

// TestSplitForceChunkedVector: wsh_multi_chunked is force-chunked but tiny — its
// payload fits a single chunk, so split() yields exactly 1 chunk that is still
// chunked-format (chunked-flag set). Its csid must equal the golden 0x157ae.
func TestSplitForceChunkedVector(t *testing.T) {
	d := loadDescriptor(t, "wsh_multi_chunked")
	chunks, err := split(d)
	if err != nil {
		t.Fatalf("split: %v", err)
	}
	if len(chunks) != 1 {
		t.Fatalf("split produced %d chunks, want 1 (single-chunk force-chunked)", len(chunks))
	}
	if !codex32.ValidMD(chunks[0]) {
		t.Fatalf("chunk fails ValidMD: %s", chunks[0])
	}
	h, err := ParseChunkHeader(chunks[0])
	if err != nil {
		t.Fatalf("ParseChunkHeader: %v", err)
	}
	if !h.Chunked {
		t.Fatal("Chunked=false for force-chunked vector")
	}
	if h.ChunkSetID != 0x157ae {
		t.Fatalf("csid=%#x want 0x157ae (golden)", h.ChunkSetID)
	}
	if h.TotalChunks != 1 || h.ChunkIndex != 0 {
		t.Fatalf("total=%d index=%d want 1,0", h.TotalChunks, h.ChunkIndex)
	}
}

// TestSplitSmallSingleChunk: a small descriptor whose payload fits one chunk
// yields count==1.
func TestSplitSmallSingleChunk(t *testing.T) {
	d := loadDescriptor(t, "wpkh_basic")
	chunks, err := split(d)
	if err != nil {
		t.Fatalf("split: %v", err)
	}
	if len(chunks) != 1 {
		t.Fatalf("split produced %d chunks, want 1", len(chunks))
	}
}

// reChunkWithCSID re-encodes the chunk at the given index of d's split output
// but stamps a DIFFERENT csid into every chunk header (keeping each chunk's BCH
// valid). Used to exercise the integrity gate: all chunks agree on the wrong
// csid (passing consistency) but the re-derived csid won't match.
func splitWithCSID(t *testing.T, d *descriptor, csid uint32) []string {
	t.Helper()
	payloadBytes, _, err := encodePayload(d)
	if err != nil {
		t.Fatalf("encodePayload: %v", err)
	}
	count := ceilDiv(len(payloadBytes)*8, singleStringPayloadBitLimit)
	if count == 0 {
		count = 1
	}
	bytesPerChunk := ceilDiv(len(payloadBytes), count)
	out := make([]string, 0, count)
	for index := 0; index < count; index++ {
		start := index * bytesPerChunk
		end := (index + 1) * bytesPerChunk
		if end > len(payloadBytes) {
			end = len(payloadBytes)
		}
		cp := payloadBytes[start:end]
		var w bitWriter
		hdr := ChunkHeader{Version: wfRedesignVersion, Chunked: true, ChunkSetID: csid, TotalChunks: count, ChunkIndex: index}
		if err := hdr.write(&w); err != nil {
			t.Fatalf("hdr.write: %v", err)
		}
		for _, by := range cp {
			w.write(uint64(by), 8)
		}
		syms, err := bitsToSymbols(w.intoBytes(), chunkHeaderBits+8*len(cp))
		if err != nil {
			t.Fatalf("bitsToSymbols: %v", err)
		}
		out = append(out, codex32.AssembleMD1(syms))
	}
	return out
}

// TestReassembleRoundTrip: split -> Reassemble -> Decode equals Decode of the
// re-encoded single-string form for the chunked fixtures.
func TestReassembleRoundTrip(t *testing.T) {
	for _, d := range []*descriptor{chunkedMD1Vector(), loadDescriptor(t, "wsh_multi_chunked")} {
		chunks, err := split(d)
		if err != nil {
			t.Fatalf("split: %v", err)
		}
		got, err := Reassemble(chunks)
		if err != nil {
			t.Fatalf("Reassemble: %v", err)
		}
		// The reassembled descriptor must re-encode to the same payload bytes as
		// the (canonical) input — the SHA-derived csid round-trips exactly.
		wantBytes, _, err := encodePayload(d)
		if err != nil {
			t.Fatalf("encodePayload(in): %v", err)
		}
		gotBytes, _, err := encodePayload(got)
		if err != nil {
			t.Fatalf("encodePayload(out): %v", err)
		}
		if string(gotBytes) != string(wantBytes) {
			t.Fatalf("round-trip payload mismatch:\n got %x\nwant %x", gotBytes, wantBytes)
		}
	}
}

// TestReassembleReorderOK: shuffled chunk order still reassembles (Reassemble
// sorts by index).
func TestReassembleReorderOK(t *testing.T) {
	chunks, err := split(chunkedMD1Vector())
	if err != nil {
		t.Fatalf("split: %v", err)
	}
	if len(chunks) < 2 {
		t.Skip("need >=2 chunks")
	}
	// Reverse the slice.
	rev := make([]string, len(chunks))
	for i := range chunks {
		rev[len(chunks)-1-i] = chunks[i]
	}
	if _, err := Reassemble(rev); err != nil {
		t.Fatalf("Reassemble(reversed): %v", err)
	}
}

// TestReassembleDropChunk: a missing chunk -> ErrChunkSetIncomplete.
func TestReassembleDropChunk(t *testing.T) {
	chunks, err := split(chunkedMD1Vector())
	if err != nil {
		t.Fatalf("split: %v", err)
	}
	if len(chunks) < 2 {
		t.Skip("need >=2 chunks")
	}
	dropped := chunks[1:]
	if _, err := Reassemble(dropped); err != ErrChunkSetIncomplete {
		t.Fatalf("Reassemble(dropped) = %v, want ErrChunkSetIncomplete", err)
	}
}

// TestReassembleDuplicate: a duplicated chunk (len > count) -> incomplete.
func TestReassembleDuplicate(t *testing.T) {
	chunks, err := split(chunkedMD1Vector())
	if err != nil {
		t.Fatalf("split: %v", err)
	}
	dup := append(append([]string(nil), chunks...), chunks[0])
	if _, err := Reassemble(dup); err != ErrChunkSetIncomplete {
		t.Fatalf("Reassemble(dup) = %v, want ErrChunkSetIncomplete", err)
	}
}

// TestReassembleCorruptCSID: all chunks carry a wrong-but-consistent csid; the
// re-derived integrity csid won't match -> errChunkSetIdMismatch.
func TestReassembleCorruptCSID(t *testing.T) {
	d := chunkedMD1Vector()
	id, _ := computeEncodingID(d)
	realCsid := deriveChunkSetID(id)
	wrong := splitWithCSID(t, d, (realCsid+1)&0xFFFFF)
	if _, err := Reassemble(wrong); err != ErrChunkSetIDMismatch {
		t.Fatalf("Reassemble(wrong csid) = %v, want ErrChunkSetIDMismatch", err)
	}
}

// TestReassembleCrossSet: mixing a chunk from a different descriptor's set
// (different csid/count) -> errChunkSetInconsistent.
func TestReassembleCrossSet(t *testing.T) {
	a, err := split(chunkedMD1Vector())
	if err != nil {
		t.Fatalf("split a: %v", err)
	}
	// A second, distinct multi-chunk descriptor.
	d2 := chunkedMD1Vector()
	d2.tree.body.(childrenBody).children[0].body = multiKeysBody{k: 3, indices: []uint8{0, 1, 2, 3, 4, 5}}
	b, err := split(d2)
	if err != nil {
		t.Fatalf("split b: %v", err)
	}
	// Replace one chunk of a with one from b (its csid differs).
	mixed := append([]string(nil), a...)
	mixed[0] = b[0]
	if _, err := Reassemble(mixed); err != errChunkSetInconsist {
		t.Fatalf("Reassemble(cross-set) = %v, want errChunkSetInconsist", err)
	}
}

// TestParseChunkHeaderSingleIsNotChunked: a single (non-chunked) md1 string
// reports Chunked==false via the bit-0 probe, and Reassemble is only meaningful
// for chunked sets.
func TestParseChunkHeaderSingleIsNotChunked(t *testing.T) {
	s := loadPhrase(t, "wpkh_basic")
	h, err := ParseChunkHeader(s)
	if err != nil {
		t.Fatalf("ParseChunkHeader: %v", err)
	}
	if h.Chunked {
		t.Fatal("single md1 reported Chunked=true")
	}
}
