package ndef

import (
	"bytes"
	"encoding/hex"
	"errors"
	"io"
	"testing"
)

// A long record whose 32-bit payload length does not fit a 32-bit int must
// read as truncated at every word size, never as the records its payload
// happens to contain (run with GOARCH=386 for the device's word size).
func TestS1NdefLongLengthWordSize(t *testing.T) {
	for _, h := range []string{
		"c201ffffffff78d1010354006869",       // skipped media record, 4 GiB
		"c2018000000078d1010354006869",       // skipped media record, 2 GiB
		"c101ffffffff5504d1010254006869",     // URI record, 4 GiB
		"030ec201ffffffff78d1010354006869fe", // device stack: inside an NDEF TLV
	} {
		b, _ := hex.DecodeString(h)
		var r io.Reader = NewRecordReader(bytes.NewReader(b))
		if h[:2] == "03" {
			r = NewRecordReader(NewMessageReader(bytes.NewReader(b)))
		}
		got, err := io.ReadAll(r)
		if !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Errorf("%s: got %q, %v; want io.ErrUnexpectedEOF", h, got, err)
		}
	}
}
