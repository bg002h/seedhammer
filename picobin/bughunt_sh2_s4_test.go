package picobin

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"testing"
)

// TestBughuntSh207PicobinLoadMapZeroFill (sh2 bug-hunt #7): a LOAD_MAP
// with two zero-fill entries (storage_start_address == 0) of different
// sizes. RP2350 datasheet 5.9.3.2 (LOAD_MAP entry 0, Note): "If this
// value is 0x00000000 irrespective of the value of the absolute flag,
// then the runtime address range is filled with zeros. In this case,
// the 32 bit size itself is hashed rather than size zero bytes." So each
// entry's own size word (entry offset + 8) is hashed.
func TestBughuntSh207PicobinLoadMapZeroFill(t *testing.T) {
	le := binary.LittleEndian
	b := make([]byte, 52)
	le.PutUint32(b[0:], header)
	// LOAD_MAP at 4: type 0x06, 7 words (1 + 2 entries * 3), 2 entries,
	// relative.
	le.PutUint32(b[4:], 0x06|7<<8|2<<16)
	// Entry 0: storage 0, runtime, size.
	le.PutUint32(b[8:], 0)
	le.PutUint32(b[12:], 0x20000000)
	le.PutUint32(b[16:], 0x11111111)
	// Entry 1: storage 0, runtime, a different size.
	le.PutUint32(b[20:], 0)
	le.PutUint32(b[24:], 0x20001000)
	le.PutUint32(b[28:], 0x22222222)
	// HASH_DEF at 32: type 0x47, 2 words, SHA-256; second word is the
	// number of block words hashed (8 words = the header and LOAD_MAP).
	le.PutUint32(b[32:], 0x47|2<<8|hashSHA256<<24)
	le.PutUint32(b[36:], 8)
	// LAST item: total of the preceding item sizes (7 + 2 words).
	le.PutUint32(b[40:], 0x7f|9<<8)
	le.PutUint32(b[44:], 0) // Link to self.
	le.PutUint32(b[48:], footer)

	img, err := NewImage(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	got, err := img.HashData(bytes.NewReader(b), 0)
	if err != nil {
		t.Fatal(err)
	}
	// Independent expectation straight from the datasheet text: hash
	// each zero-fill entry's size word, then the first 8 block words.
	h := sha256.New()
	h.Write(b[16:20]) // Entry 0 size.
	h.Write(b[28:32]) // Entry 1 size.
	h.Write(b[0:32])  // 8 block words.
	want := h.Sum(nil)
	if !bytes.Equal(got, want) {
		t.Errorf("HashData = %x, want %x (each zero-fill entry hashes its own size word)", got, want)
	}
}
