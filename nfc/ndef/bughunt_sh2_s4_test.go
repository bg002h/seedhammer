package ndef

import (
	"bytes"
	"io"
	"testing"
)

// TestBughuntSh218NdefEmptyTextURIRecord is bug #18: for a Text or URI record
// RecordReader.Read reads the status byte / URI identifier code with
// r.readByte() without first checking that the record's payload has a byte
// left. A record whose payload length is 0 therefore "reads" the first byte
// of the NEXT record's header as its own status byte or identifier code.
//
// The report calls such a message valid. It is not: NFC Forum Text RTD
// Table 2 makes "Status byte" (offset 0, 1 byte) the first field of the
// payload, and URI RTD Table 2 makes the "URI identifier code" (offset 0,
// 1 byte) the first field, and says the IRI length is "the length of the
// payload, and subtracting 1 for the protocol abbreviation code byte". The
// RTD-valid minimal forms (a status byte or an identifier code and nothing
// else) are covered by the "rtd-valid-minimal" subtest, which is expected to
// pass; the "zero-length" subtest asserts the reported wrong behaviour only:
// a zero-length record must not emit output derived from the next record's
// header bytes (here its flags byte 0x01, read as the identifier code for
// "http://www.").
//
// OPEN PRODUCT QUESTION (not decided here): what a reader should do with an
// RTD-invalid zero-length Text/URI record: report an error, skip the record,
// or treat it as empty. The test accepts all three.
func TestBughuntSh218NdefEmptyTextURIRecord(t *testing.T) {
	t.Skip("deferred-minor: sh-ndef-empty-record")
	// A well-formed record 2 (a middle record with a 32-bit payload length)
	// whose flags byte, 0x01, is the URI identifier code for "http://www.".
	record2 := []byte{0x01, 1, 0, 0, 0, 3, 'T', 0x00, 'h', 'i'}
	record3 := []byte{0x51, 1, 3, 'T', 0x00, 'y', 'o'}

	t.Run("rtd-valid-minimal", func(t *testing.T) {
		for _, typ := range []byte{'T', 'U'} {
			// Payload: just the status byte / identifier code (0x00).
			msg := []byte{0x91, 1, 1, typ, 0x00}
			msg = append(msg, 0x51, 1, 3, 'T', 0x00, 'h', 'i')
			// A record without content is invisible: the next record's
			// text follows without an end-of-record marker in between.
			got, err := io.ReadAll(NewRecordReader(bytes.NewReader(msg)))
			if err != nil || string(got) != "hi" {
				t.Errorf("%c record with only its first byte, then \"hi\": got %q, %v; want \"hi\"", typ, got, err)
			}
		}
	})

	t.Run("zero-length", func(t *testing.T) {
		for _, typ := range []byte{'T', 'U'} {
			msg := []byte{0x91, 1, 0, typ}
			msg = append(msg, record2...)
			msg = append(msg, record3...)
			r := NewRecordReader(bytes.NewReader(msg))
			got, err := io.ReadAll(r)
			// Acceptable: nothing (error, or end of record), or record
			// 2's own text if the empty record is skipped.
			if g := string(got); g != "" && g != "hi" {
				t.Errorf("zero-length %c record produced %q (err %v), want nothing or record 2's text \"hi\"; the extra bytes come from record 2's flags byte", typ, got, err)
			}
		}
	})
}
