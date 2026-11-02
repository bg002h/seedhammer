package uf2

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"testing"
)

// bughuntBlock builds one 512-byte UF2 block for the RP2350 signed family.
func bughuntBlock(addr, payload uint32) []byte {
	b := make([]byte, blockSize)
	le := binary.LittleEndian
	le.PutUint32(b[0:], magic1)
	le.PutUint32(b[4:], magic2)
	le.PutUint32(b[8:], flagFamilyID)
	le.PutUint32(b[12:], addr)
	le.PutUint32(b[16:], payload)
	le.PutUint32(b[28:], uint32(FamilyRP2350ARMSigned))
	le.PutUint32(b[blockSize-footerSize:], magicEnd)
	return b
}

// TestBughuntSh219PayloadSizeOverData is bug #19: a block whose payload size
// is larger than the 476-byte data area. loadBlock computes the leftover
// footer length as len(footer)-idx (480-idx), which is 1..3 for payload sizes
// 477..479, then slices footer[len(footer)-4:], which panics with a negative
// low bound.
//
// UF2 specification block layout: "| 32 | 476 | Data, padded with zeros |" and
// "| 16 | 4 | Number of bytes used in data (often 256) |". A payload size over
// 476 cannot be valid. The correct behaviour for such a file is an error; this
// test asserts no panic and that the file is not accepted. (Whether the
// message names the payload size is not asserted.)
func TestBughuntSh219PayloadSizeOverData(t *testing.T) {
	for _, payload := range []uint32{477, 478, 479, 480} {
		t.Run(fmt.Sprint(payload), func(t *testing.T) {
			data := append(bughuntBlock(0x10000000, payload), bughuntBlock(0x10000000+payload, payload)...)
			r := NewReader(bytes.NewReader(data), FamilyRP2350ARMSigned)
			var (
				got      []byte
				err      error
				panicked bool
			)
			func() {
				defer func() {
					if p := recover(); p != nil {
						panicked = true
						t.Errorf("panic reading a block with payload size %d: %v", payload, p)
					}
				}()
				got, err = io.ReadAll(r)
			}()
			if !panicked && err == nil {
				t.Errorf("payload size %d (data area is 476 bytes) accepted: read %d bytes, no error", payload, len(got))
			}
		})
	}
}

// TestBughuntSh220StartAddrZero is bug #20: Reader uses StartAddr == 0 as its
// "not yet set" marker, so an image whose first block is at address 0 resets
// the contiguity base on its second block and is rejected as non-contiguous.
//
// UF2 specification: "`CURRENT.UF2` - the contents of the entire flash of the
// device, starting at `0x00000000`, with `256` payload size". An image that
// starts at 0 is therefore a legal UF2 file.
func TestBughuntSh220StartAddrZero(t *testing.T) {
	data := append(bughuntBlock(0, 256), bughuntBlock(256, 256)...)
	r := NewReader(bytes.NewReader(data), FamilyRP2350ARMSigned)
	got, err := io.ReadAll(r)
	if err != nil || len(got) != 512 {
		t.Fatalf("read %d bytes, err %v; want 512 bytes and no error for two contiguous blocks at 0 and 256", len(got), err)
	}
	if r.StartAddr != 0 {
		t.Errorf("StartAddr = %#x, want 0", r.StartAddr)
	}
}
