package codex32

import (
	"bytes"
	"testing"
)

// TestEncodeMS1WipesItsPayload pins the wipe of EncodeMS1's internal payload
// buffer (0x00 entr prefix followed by a copy of the entropy). The hook hands
// over that slice itself; its snapshot must be the real payload first, or the
// zero check would prove nothing. The returned string is unchanged by the
// wipe: NewSeed has finished reading the payload before it runs.
func TestEncodeMS1WipesItsPayload(t *testing.T) {
	entropy := bytes.Repeat([]byte{0x7f}, 16)
	want := append([]byte{msPrefixEntr}, entropy...)
	var captured, snapshot []byte
	encodeMS1PayloadHook = func(p []byte) {
		captured = p
		snapshot = append([]byte(nil), p...)
	}
	t.Cleanup(func() { encodeMS1PayloadHook = nil })

	s, err := EncodeMS1(entropy)
	if err != nil {
		t.Fatal(err)
	}
	// Byte-identical to ms-codec 0.9.0/0.10.0 on 16 x 0x7f (R0 round 2 M-5).
	if s != "ms10entrsqplh7lml0alh7lml0alh7lml0als5cclar2zmksh6" {
		t.Fatalf("EncodeMS1 = %s", s)
	}
	if !bytes.Equal(snapshot, want) {
		t.Fatalf("hook saw %x, want the payload %x", snapshot, want)
	}
	for i, b := range captured {
		if b != 0 {
			t.Fatalf("payload byte %d is still %#02x after EncodeMS1 returned", i, b)
		}
	}
	if !bytes.Equal(entropy, bytes.Repeat([]byte{0x7f}, 16)) {
		t.Fatal("EncodeMS1 wiped the caller's entropy; only its own copy is its to wipe")
	}
}
