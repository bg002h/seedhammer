package uf2

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"testing"
	"testing/iotest"
)

// fixShUF2Block builds one 512-byte UF2 block with a family ID, a 256-byte
// payload at addr, and the given block number and declared block total.
func fixShUF2Block(family FamilyID, addr, blockNo, numBlocks uint32) []byte {
	b := make([]byte, blockSize)
	le := binary.LittleEndian
	le.PutUint32(b[0:], magic1)
	le.PutUint32(b[4:], magic2)
	le.PutUint32(b[8:], flagFamilyID)
	le.PutUint32(b[12:], addr)
	le.PutUint32(b[16:], payloadSize)
	le.PutUint32(b[20:], blockNo)
	le.PutUint32(b[24:], numBlocks)
	le.PutUint32(b[28:], uint32(family))
	for i := 0; i < payloadSize; i++ {
		b[headerSize+i] = byte(blockNo + uint32(i))
	}
	le.PutUint32(b[blockSize-footerSize:], magicEnd)
	return b
}

// fixShUF2File builds a complete n-block file of one family at 0x10000000.
func fixShUF2File(family FamilyID, n uint32) []byte {
	var out []byte
	for i := uint32(0); i < n; i++ {
		out = append(out, fixShUF2Block(family, 0x10000000+payloadSize*i, i, n)...)
	}
	return out
}

const fixShOtherFamily FamilyID = 0xe48bff57

func fixShReadAll(rd io.Reader) ([]byte, error) {
	return io.ReadAll(NewReader(rd, FamilyRP2350ARMSigned))
}

// Control: a complete file reads in full with no error.
func TestFixShUF2CompleteFile(t *testing.T) {
	got, err := fixShReadAll(bytes.NewReader(fixShUF2File(FamilyRP2350ARMSigned, 2)))
	if err != nil || len(got) != 2*payloadSize {
		t.Fatalf("complete 2-block file: read %d bytes, err %v; want 512 bytes and no error", len(got), err)
	}
}

// Control: blocks of another family are disregarded, numbering included
// (UF2 spec: "the bootloader should disregard the entire block, including
// blockNo and numBlocks fields"), so concatenated files read in full.
func TestFixShUF2ConcatenatedFamilies(t *testing.T) {
	mine := fixShUF2File(FamilyRP2350ARMSigned, 2)
	other := fixShUF2File(fixShOtherFamily, 3)
	for name, data := range map[string][]byte{
		"other-first": append(append([]byte{}, other...), mine...),
		"other-last":  append(append([]byte{}, mine...), other...),
	} {
		got, err := fixShReadAll(bytes.NewReader(data))
		if err != nil || len(got) != 2*payloadSize {
			t.Errorf("%s: read %d bytes, err %v; want 512 bytes and no error", name, len(got), err)
		}
	}
}

// Control: an empty input is zero blocks and reads as a clean EOF (unchanged).
func TestFixShUF2EmptyInput(t *testing.T) {
	got, err := fixShReadAll(bytes.NewReader(nil))
	if err != nil || len(got) != 0 {
		t.Fatalf("empty input: read %d bytes, err %v; want 0 bytes and no error", len(got), err)
	}
}

// RED: the file ends at a block boundary before the declared block total.
func TestFixShUF2BoundaryCut(t *testing.T) {
	data := fixShUF2File(FamilyRP2350ARMSigned, 3)
	for _, cut := range []int{blockSize, 2 * blockSize} {
		got, err := fixShReadAll(bytes.NewReader(data[:cut]))
		if !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Errorf("cut at %d of 3 blocks: read %d bytes, err %v; want io.ErrUnexpectedEOF", cut, len(got), err)
		}
	}
}

// RED: the file ends inside a block (after the header, inside the payload, or
// between the payload and the footer), read through a reader that returns
// io.EOF after its data and through one that returns it with its last data.
func TestFixShUF2MidBlockCut(t *testing.T) {
	data := fixShUF2File(FamilyRP2350ARMSigned, 2)
	for _, cut := range []int{
		headerSize,                           // block 0: header only
		blockSize + headerSize,               // block 1: header only
		blockSize + headerSize + 100,         // block 1: inside the payload
		blockSize + headerSize + payloadSize, // block 1: payload, no footer
		blockSize + blockSize - 2,            // block 1: inside the footer
	} {
		for name, rd := range map[string]io.Reader{
			"eof-after-data": bytes.NewReader(data[:cut]),
			"eof-with-data":  iotest.DataErrReader(bytes.NewReader(data[:cut])),
		} {
			got, err := fixShReadAll(rd)
			if !errors.Is(err, io.ErrUnexpectedEOF) {
				t.Errorf("cut at %d (%s): read %d bytes, err %v; want io.ErrUnexpectedEOF", cut, name, len(got), err)
			}
		}
	}
}

// RED: a cut inside a disregarded block of another family is still a cut file.
func TestFixShUF2OtherFamilyBlockCut(t *testing.T) {
	data := append(fixShUF2File(FamilyRP2350ARMSigned, 2), fixShUF2File(fixShOtherFamily, 1)...)
	cut := 2*blockSize + headerSize
	got, err := fixShReadAll(bytes.NewReader(data[:cut]))
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Errorf("cut %d bytes into a 3-block concatenation: read %d bytes, err %v; want io.ErrUnexpectedEOF", cut, len(got), err)
	}
}

// RED: only blocks of the reader's family count toward that family's declared
// total; blocks of another family in front do not make up the shortfall.
func TestFixShUF2CountsOwnFamilyOnly(t *testing.T) {
	mine := fixShUF2File(FamilyRP2350ARMSigned, 3)
	data := append(fixShUF2File(fixShOtherFamily, 1), mine[:2*blockSize]...)
	got, err := fixShReadAll(bytes.NewReader(data))
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Errorf("1 other-family block + 2 of 3 own blocks: read %d bytes, err %v; want io.ErrUnexpectedEOF", len(got), err)
	}
}

// RED: the declared total is the first own-family block's numBlocks (R1),
// not a later block's.
func TestFixShUF2TotalFromFirstBlock(t *testing.T) {
	data := append(fixShUF2Block(FamilyRP2350ARMSigned, 0x10000000, 0, 3),
		fixShUF2Block(FamilyRP2350ARMSigned, 0x10000000+payloadSize, 1, 2)...)
	got, err := fixShReadAll(bytes.NewReader(data))
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Errorf("first block declares 3, second declares 2, file ends after 2: read %d bytes, err %v; want io.ErrUnexpectedEOF", len(got), err)
	}
}
