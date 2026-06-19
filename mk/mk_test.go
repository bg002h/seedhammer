package mk

import (
	"errors"
	"testing"
)

func TestParseHeader(t *testing.T) {
	// V1 chunk 0 (chunked, index 0 of 2) and chunk 1 (index 1 of 2).
	const c0 = "mk1qpzg69pqqsq3zg3ngj4thnxaq5zg3vs7zqsrqqdt4w46h2at4w46h2at4w46h2at4w46h2at4w46h2at4w46h2at4vp3kx98j76m4mjlwphf"
	const c1 = "mk1qpzg69ppsnz4v7cjv3qfjhf76k4t5pt96u0psdrqfqvll8qh7h5athg837pmkf3dpug2mmjtfel6x"
	h0, err := ParseHeader(c0)
	if err != nil {
		t.Fatalf("ParseHeader(c0): %v", err)
	}
	if !h0.Chunked || h0.TotalChunks != 2 || h0.ChunkIndex != 0 {
		t.Fatalf("c0 header = %+v; want chunked total=2 index=0", h0)
	}
	h1, err := ParseHeader(c1)
	if err != nil {
		t.Fatalf("ParseHeader(c1): %v", err)
	}
	// R0-C1 guard: chunk_index is 0-based verbatim (NOT value-1) — chunk 1 is index 1.
	if !h1.Chunked || h1.TotalChunks != 2 || h1.ChunkIndex != 1 {
		t.Fatalf("c1 header = %+v; want chunked total=2 index=1", h1)
	}
	// Both chunks share chunk_set_id.
	if h0.ChunkSetID != h1.ChunkSetID {
		t.Fatalf("chunk_set_id mismatch: %d vs %d", h0.ChunkSetID, h1.ChunkSetID)
	}
}

func TestFiveBitToBytes(t *testing.T) {
	// 8 zero symbols = 40 bits = 5 bytes, zero padding → ok.
	out, err := fiveBitToBytes([]byte{0, 0, 0, 0, 0, 0, 0, 0})
	if err != nil || len(out) != 5 {
		t.Fatalf("zero pad: out=%v err=%v", out, err)
	}
	// A symbol >= 32 → reject.
	if _, err := fiveBitToBytes([]byte{0, 32}); !errors.Is(err, errMalformedPadding) {
		t.Fatalf("symbol>=32: want errMalformedPadding, got %v", err)
	}
	// Non-zero trailing pad bits → reject (one symbol = 5 bits, all leftover, value 1).
	if _, err := fiveBitToBytes([]byte{1}); !errors.Is(err, errMalformedPadding) {
		t.Fatalf("nonzero pad: want errMalformedPadding, got %v", err)
	}
}
