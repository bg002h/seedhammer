package codex32

import (
	"bytes"
	"testing"
)

// FuzzEncodeMS1 feeds arbitrary entropy bytes: a successful EncodeMS1 must
// produce a string that DecodeMS1 round-trips back to the same entropy, with no
// panic. Invalid lengths return an error (benign skip). (T6a-1 Task 4.)
func FuzzEncodeMS1(f *testing.F) {
	f.Add(make([]byte, 16))
	f.Add(make([]byte, 32))
	f.Add([]byte{0x01, 0x02, 0x03})
	f.Fuzz(func(t *testing.T, entropy []byte) {
		s, err := EncodeMS1(entropy)
		if err != nil {
			return // invalid BIP-39 length — benign skip
		}
		str, err := New(s)
		if err != nil {
			t.Fatalf("New(EncodeMS1(%x))=%q: %v", entropy, s, err)
		}
		prefix, _, got, err := DecodeMS1(str)
		if err != nil {
			t.Fatalf("DecodeMS1(EncodeMS1(%x)): %v", entropy, err)
		}
		if prefix != msPrefixEntr {
			t.Fatalf("prefix=%#x, want entr", prefix)
		}
		if !bytes.Equal(got, entropy) {
			t.Fatalf("round-trip: got %x want %x", got, entropy)
		}
	})
}
