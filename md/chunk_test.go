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
