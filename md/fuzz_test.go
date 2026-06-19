package md

import (
	"testing"

	"seedhammer.com/codex32"
)

// FuzzEncodePayload feeds arbitrary bytes as a candidate canonical payload. If
// they decode to a valid descriptor, re-encoding then re-decoding MUST NOT
// panic and MUST round-trip to byte-identical payload (the decoded AST is
// already canonical, so encodePayload reproduces the same bytes). Any decode
// failure is a benign skip — the harness only asserts "no panic + round-trip
// stability on the success path". (I-9 §5.8.)
func FuzzEncodePayload(f *testing.F) {
	// Seed with every golden payload.
	for _, name := range byteParityVectorNames {
		raw, err := readFileBytes(name)
		if err == nil {
			f.Add(raw)
		}
	}
	f.Fuzz(func(t *testing.T, b []byte) {
		// Decode at the byte-aligned bit length (TLV-rollback tolerates padding).
		d, err := decodePayloadValidated(b, len(b)*8)
		if err != nil {
			return // not a valid payload — skip
		}
		// Re-encode the decoded (canonical) descriptor.
		enc, bits, err := encodePayload(d)
		if err != nil {
			// A decodable descriptor must always re-encode.
			t.Fatalf("encodePayload of decoded descriptor failed: %v", err)
		}
		// Re-decode at the exact reported bit length and re-encode again — the
		// second encode MUST equal the first (canonical-form stability).
		d2, err := decodePayloadValidated(enc, bits)
		if err != nil {
			t.Fatalf("re-decode of encoded payload failed: %v", err)
		}
		enc2, _, err := encodePayload(d2)
		if err != nil {
			t.Fatalf("second encode failed: %v", err)
		}
		if string(enc) != string(enc2) {
			t.Fatalf("encode not idempotent:\n a=%x\n b=%x", enc, enc2)
		}
	})
}

// FuzzReassemble feeds arbitrary string slices to Reassemble: it must never
// panic and must only ever return a typed error or a valid descriptor.
func FuzzReassemble(f *testing.F) {
	// Seed with a real chunk set (joined; the fuzzer mutates from here).
	if chunks, err := split(chunkedMD1Vector()); err == nil {
		for _, c := range chunks {
			f.Add(c)
		}
	}
	f.Add("md1qqqqqqqqqqqqq")
	f.Add("")
	f.Fuzz(func(t *testing.T, s string) {
		// Single-element set; arbitrary string. Must not panic.
		_, _ = Reassemble([]string{s})
		// Two-element set duplicating the input — also must not panic.
		_, _ = Reassemble([]string{s, s})
	})
}

// FuzzExpandWalletPolicy feeds arbitrary bytes as a candidate payload; any that
// decode to a valid descriptor are expanded. ExpandWalletPolicy must never
// panic and must always return exactly n ExpandedKey records with Index == i.
func FuzzExpandWalletPolicy(f *testing.F) {
	for _, name := range byteParityVectorNames {
		if raw, err := readFileBytes(name); err == nil {
			f.Add(raw)
		}
	}
	f.Fuzz(func(t *testing.T, b []byte) {
		d, err := decodePayloadValidated(b, len(b)*8)
		if err != nil {
			return // not a valid descriptor — skip
		}
		keys, err := ExpandWalletPolicy(d)
		if err != nil {
			return // a typed error is acceptable; only a panic is a bug
		}
		if len(keys) != int(d.n) {
			t.Fatalf("ExpandWalletPolicy returned %d keys, want n=%d", len(keys), d.n)
		}
		for i, k := range keys {
			if int(k.Index) != i {
				t.Fatalf("key %d Index = %d, want %d", i, k.Index, i)
			}
		}
	})
}

// FuzzParseChunkHeader feeds arbitrary strings to ParseChunkHeader: it must
// never panic and only ever return a typed error or a ChunkHeader.
func FuzzParseChunkHeader(f *testing.F) {
	f.Add("md1yqpqqxqq8xtwhw4xwn4qh") // wpkh_basic
	if chunks, err := split(chunkedMD1Vector()); err == nil && len(chunks) > 0 {
		f.Add(chunks[0])
	}
	f.Add("")
	f.Add("md1")
	f.Add("not-a-codex32-string")
	f.Fuzz(func(t *testing.T, s string) {
		h, err := ParseChunkHeader(s)
		if err != nil {
			return
		}
		// On success, the header must be self-consistent when chunked.
		if h.Chunked {
			if h.TotalChunks < 1 || h.TotalChunks > 64 {
				t.Fatalf("chunked header bad count: %d", h.TotalChunks)
			}
			if h.ChunkIndex < 0 {
				t.Fatalf("chunked header negative index: %d", h.ChunkIndex)
			}
		}
		// ValidMD must hold for any string ParseChunkHeader accepted (it routes
		// through MDDataSymbols which requires a BCH-valid md1 string).
		if !codex32.ValidMD(s) {
			t.Fatalf("ParseChunkHeader accepted a non-ValidMD string: %q", s)
		}
	})
}
