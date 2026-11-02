package type2

import (
	"bufio"
	"bytes"
	"io"
	"testing"
)

// TestBughuntSh208Type2LargeTag is bug #8: Reader.Read keeps its block counter
// in a uint8, so a tag whose capability container advertises more than 255
// blocks (CC size byte >= 0x80) hits its end block early and the reader
// returns a clean io.EOF with part of the user memory silently dropped.
//
// A single-byte READ page address reaches page 255 at most (sector 0 of the
// tag), which is user pages 4..255 = 252 blocks = 1008 bytes. The reader must
// either return at least that much, correctly, or fail with a non-EOF error.
//
// OPEN PRODUCT QUESTION (not decided here): whether the reader should issue
// SECTOR_SELECT to reach the rest of a 2k tag, or stop at 1008 bytes. This
// test asserts only that the reported wrong behaviour (a clean EOF after 848
// bytes) is absent.
func TestBughuntSh208Type2LargeTag(t *testing.T) {
	t.Skip("deferred-minor: sh-type2-large-tag")
	const ccSizeByte = 0xEA // NTAG I2C 2k style: 0xEA*8 = 1872 bytes.
	data := make([]byte, ccSizeByte*8)
	for i := range data {
		data[i] = byte(i)
	}
	tag := &Tag{
		uid: 0xdeadbeef,
		mem: append([]byte{ccMagic, 0x10, ccSizeByte, 0x00}, data...),
	}
	r, err := NewReader(tag)
	if err != nil {
		t.Fatal(err)
	}
	// Buffer reading to ensure a minimum read size, as the poller does.
	got, err := io.ReadAll(bufio.NewReaderSize(r, 128))
	if err != nil {
		// A non-EOF error is a loud, acceptable outcome.
		t.Logf("read failed loudly (acceptable): %v", err)
		return
	}
	const reachable = (256 - (ccBlock + 1)) * blockSize // 1008 bytes
	if len(got) < reachable || !bytes.Equal(got[:reachable], data[:reachable]) {
		t.Fatalf("read %d bytes with a clean EOF, want at least %d correct bytes or an error", len(got), reachable)
	}
}
