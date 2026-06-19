package codex32

import (
	"bytes"
	"strings"
	"testing"
)

// EncodeMS1 is the net-new ms1 encoder (the fork ships only DecodeMS1). It must
// round-trip DecodeMS1 (T6a-1, C4) for every BIP-39 entropy length, emit the
// exact ms10entrs prefix (R0-M5), embed the 0x00 entr prefix byte + "entr" id,
// and reproduce the pinned ms10entrsqqqq…cj9sxraq34v7f zero-16 vector (R0-M4).

func TestEncodeMS1RoundTrip(t *testing.T) {
	for _, n := range []int{16, 20, 24, 28, 32} {
		entropy := make([]byte, n)
		for i := range entropy {
			entropy[i] = byte(i * 7) // arbitrary, deterministic
		}
		s, err := EncodeMS1(entropy)
		if err != nil {
			t.Fatalf("EncodeMS1(%d bytes): %v", n, err)
		}
		str, err := New(s)
		if err != nil {
			t.Fatalf("New(%q): %v", s, err)
		}
		prefix, lang, got, err := DecodeMS1(str)
		if err != nil {
			t.Fatalf("DecodeMS1: %v", err)
		}
		if prefix != msPrefixEntr {
			t.Errorf("prefix=%#x, want %#x (entr)", prefix, msPrefixEntr)
		}
		if lang != 0 {
			t.Errorf("language=%d, want 0 (entr carries none)", lang)
		}
		if !bytes.Equal(got, entropy) {
			t.Errorf("round-trip entropy=%x, want %x", got, entropy)
		}
	}
}

func TestEncodeMS1Prefix(t *testing.T) {
	// R0-M5: the wire begins exactly ms10entrs (hrp ms, threshold 0, id entr,
	// share s) and Seed()[0] is the 0x00 entr prefix byte; id == "entr".
	s, err := EncodeMS1(make([]byte, 16))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(s, "ms10entrs") {
		t.Errorf("ms1=%q, want ms10entrs prefix", s)
	}
	str, err := New(s)
	if err != nil {
		t.Fatal(err)
	}
	if seed := str.Seed(); len(seed) == 0 || seed[0] != msPrefixEntr {
		t.Errorf("Seed()[0]=%#x, want %#x", seed[0], msPrefixEntr)
	}
	id, threshold, idx := str.Split()
	if id != "entr" {
		t.Errorf("id=%q, want entr", id)
	}
	if threshold != 1 { // codex32 reports an unshared k=0 as 1
		t.Errorf("threshold=%d, want 1 (unshared)", threshold)
	}
	if idx != 's' {
		t.Errorf("share idx=%q, want s", idx)
	}
}

func TestEncodeMS1ZeroVector(t *testing.T) {
	// R0-M4: the pinned zero-16 vector (the fork's verified vector + live
	// toolkit output, also the abandon-seed bundle's ms1 leg).
	const want = "ms10entrsqqqqqqqqqqqqqqqqqqqqqqqqqqqqcj9sxraq34v7f"
	got, err := EncodeMS1(make([]byte, 16))
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("EncodeMS1(16 zero bytes)=%q, want %q", got, want)
	}
	// The abandon seed's BIP-39 entropy is exactly 16 zero bytes, so its ms1
	// leg equals this vector (cross-checked vs the toolkit bundle output).
}

func TestEncodeMS1BadLength(t *testing.T) {
	for _, n := range []int{0, 15, 17, 33} {
		if _, err := EncodeMS1(make([]byte, n)); err == nil {
			t.Errorf("EncodeMS1(%d bytes) accepted, want error", n)
		}
	}
}
