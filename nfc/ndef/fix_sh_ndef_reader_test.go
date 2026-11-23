package ndef

// Tests for design/FIX_sh_ndef_reader.md (bug hunt 2026-11-19, bh-sh-I8 and
// bh-sh-I9). Test function names are the FIX document's TRACE row names.

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"slices"
	"testing"
	"testing/iotest"
)

func fixShHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// fixShReadRecords drains a RecordReader with Reads of bufSize bytes. A Read
// returning io.EOF ends the current record; an io.EOF with no record bytes
// ends the message.
func fixShReadRecords(r io.Reader, bufSize int) ([]string, error) {
	var records []string
	var cur []byte
	buf := make([]byte, bufSize)
	for range 100000 {
		n, err := r.Read(buf)
		cur = append(cur, buf[:n]...)
		switch {
		case err == io.EOF && len(cur) == 0:
			return records, nil
		case err == io.EOF:
			records = append(records, string(cur))
			cur = nil
		case err != nil:
			if len(cur) > 0 {
				records = append(records, string(cur))
			}
			return records, err
		}
	}
	return records, errors.New("no end of message after 100000 reads")
}

// R1: a URI record reads the same at every buffer size.
func TestFixShNdefURIPrefixEveryBuffer(t *testing.T) {
	tests := []struct{ rec, want string }{
		{"d10102550061", "a"},
		{"d10102550161", "http://www.a"},
		{"d10102550261", "https://www.a"},
		{"d10102550361", "http://a"},
		{"d10102550461", "https://a"},
	}
	for _, tc := range tests {
		rec := fixShHex(t, tc.rec)
		for sz := 1; sz <= len(tc.want)+1; sz++ {
			got, err := fixShReadRecords(NewRecordReader(bytes.NewReader(rec)), sz)
			if err != nil || !slices.Equal(got, []string{tc.want}) {
				t.Errorf("%s buffer %d: got %q, %v; want [%q], nil", tc.rec, sz, got, err, tc.want)
			}
		}
		got, err := io.ReadAll(iotest.OneByteReader(NewRecordReader(bytes.NewReader(rec))))
		if err != nil || string(got) != tc.want {
			t.Errorf("%s OneByteReader: got %q, %v; want %q, nil", tc.rec, got, err, tc.want)
		}
	}
}

// R1: every record list reads the same at every buffer size as with a
// 4 KiB buffer, over a whole and a one-byte underlying reader.
func TestFixShNdefRecordsEveryBuffer(t *testing.T) {
	messages := []string{
		"d00000",
		"d1010f5402656e48656c6c6f20776f726c6421",
		"d9010c0155ff046578616d706c652e636f6d",
		"91010f5402656e48656c6c6f20776f726c642151010c55046578616d706c652e636f6d",
		// URI then Text.
		"91010255046151010354006869",
		// A URI record that is only its identifier code, then Text.
		"910101550451010354006869",
		// A URI record with an unknown identifier code (skipped), then URI.
		"910102552461510102550261",
	}
	for _, m := range messages {
		data := fixShHex(t, m)
		want, wantErr := fixShReadRecords(NewRecordReader(bytes.NewReader(data)), 4096)
		if wantErr != nil {
			t.Fatalf("%s 4 KiB control: %v", m, wantErr)
		}
		for sz := 1; sz <= 32; sz++ {
			for _, under := range []struct {
				name string
				r    io.Reader
			}{
				{"bytes", bytes.NewReader(data)},
				{"one-byte", iotest.OneByteReader(bytes.NewReader(data))},
			} {
				got, err := fixShReadRecords(NewRecordReader(under.r), sz)
				if err != nil || !slices.Equal(got, want) {
					t.Errorf("%s %s buffer %d: got %q, %v; want %q, nil", m, under.name, sz, got, err, want)
				}
			}
		}
	}
}

// R2, R3: MessageReader delivers a TLV the same whether the underlying
// reader returns its last bytes with io.EOF or before it.
func TestFixShNdefMessageDataWithEOF(t *testing.T) {
	tests := []struct{ tlvs, want string }{
		{"0303616263", "abc"},
		{"fd03616263", ""},
		{"0103a00c340301ff00000342d1013e5402656e7369676e20776564642068617262206865617220616c6d6f2061757468206c61746920616c726520686f62622064726166206f6e636520726f7365fedeadbeef",
			"ffd1013e5402656e7369676e20776564642068617262206865617220616c6d6f2061757468206c61746920616c726520686f62622064726166206f6e636520726f7365"},
		// NDEF TLV, then a skipped TLV that ends the stream.
		{"030161fd03626364", "61"},
	}
	for _, tc := range tests {
		data := fixShHex(t, tc.tlvs)
		want := tc.want
		if tc.want != "abc" && tc.want != "" {
			want = string(fixShHex(t, tc.want))
		}
		for _, under := range []struct {
			name string
			r    io.Reader
		}{
			{"bytes", bytes.NewReader(data)},
			{"one-byte", iotest.OneByteReader(bytes.NewReader(data))},
			{"data-err", iotest.DataErrReader(bytes.NewReader(data))},
		} {
			got, err := io.ReadAll(NewMessageReader(under.r))
			if err != nil || string(got) != want {
				t.Errorf("%s %s: got %q, %v; want %q, nil", tc.tlvs, under.name, got, err, want)
			}
		}
	}
}

// R2, R3: RecordReader delivers a record the same whether the underlying
// reader returns its last bytes with io.EOF or before it.
func TestFixShNdefRecordDataWithEOF(t *testing.T) {
	tests := []struct {
		rec  string
		want []string
	}{
		// Text "abc".
		{"d101045400616263", []string{"abc"}},
		// Well-known type "ab" (skipped), empty payload: type ends the stream.
		{"d10200" + "6162", nil},
		// Media-type record (skipped), payload "abc" ends the stream.
		{"d2010378616263", nil},
		// URI "https://a".
		{"d10102550461", []string{"https://a"}},
	}
	for _, tc := range tests {
		data := fixShHex(t, tc.rec)
		for _, under := range []string{"bytes", "one-byte", "data-err"} {
			for _, sz := range []int{1, 4096} {
				got, err := fixShReadRecords(NewRecordReader(fixShUnder(under, data)), sz)
				if err != nil || !slices.Equal(got, tc.want) {
					t.Errorf("%s %s buffer %d: got %q, %v; want %q, nil", tc.rec, under, sz, got, err, tc.want)
				}
			}
		}
	}
}

// fixShUnder wraps data in the named underlying reader.
func fixShUnder(name string, data []byte) io.Reader {
	switch name {
	case "one-byte":
		return iotest.OneByteReader(bytes.NewReader(data))
	case "data-err":
		return iotest.DataErrReader(bytes.NewReader(data))
	}
	return bytes.NewReader(data)
}

// R4: a stream that ends inside a TLV or a record is reported as
// io.ErrUnexpectedEOF, never as io.EOF; R3: a skipped container exposes no
// bytes even then.
func TestFixShNdefTruncatedIsUnexpectedEOF(t *testing.T) {
	tests := []struct {
		name    string
		data    string
		message bool // MessageReader, else RecordReader
		want    string
		wantErr string
	}{
		{"tlv value short", "0305616263", true, "abc", "ndef: tlv: unexpected EOF"},
		{"skipped tlv value short", "fd05616263", true, "", "ndef: tlv: unexpected EOF"},
		{"tlv length missing", "03", true, "", "ndef: tlv: unexpected EOF"},
		{"tlv 2-byte length cut", "03ff01", true, "", "ndef: tlv: unexpected EOF"},
		{"tlv 2-byte length missing", "03ff", true, "", "ndef: tlv: unexpected EOF"},
		{"record 32-bit length missing", "c101", false, "", "ndef: message: unexpected EOF"},
		{"record 32-bit length cut", "c1010000", false, "", "ndef: message: unexpected EOF"},
		{"record payload length missing", "d101", false, "", "ndef: message: unexpected EOF"},
		{"record id length missing", "d90101", false, "", "ndef: message: unexpected EOF"},
		{"record type missing", "d10100", false, "", "ndef: message: unexpected EOF"},
		{"record type cut", "d1020061", false, "", "ndef: message: unexpected EOF"},
		{"record payload short", "d10105540061", false, "a", "ndef: message: unexpected EOF"},
		{"skipped record payload short", "d2010578616263", false, "", "ndef: message: unexpected EOF"},
		{"text status byte missing", "d1010154", false, "", "ndef: message: unexpected EOF"},
		{"text language cut", "d10103540261", false, "", "ndef: message: unexpected EOF"},
		{"uri code missing", "d1010155", false, "", "ndef: message: unexpected EOF"},
	}
	for _, tc := range tests {
		data := fixShHex(t, tc.data)
		for _, under := range []string{"bytes", "data-err"} {
			var u io.Reader = bytes.NewReader(data)
			if under == "data-err" {
				u = iotest.DataErrReader(u)
			}
			var r io.Reader
			if tc.message {
				r = NewMessageReader(u)
			} else {
				r = NewRecordReader(u)
			}
			got, err := io.ReadAll(r)
			if string(got) != tc.want || err == nil || err.Error() != tc.wantErr ||
				!errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF) {
				t.Errorf("%s (%s) %s: got %q, %s; want %q, %q wrapping io.ErrUnexpectedEOF only",
					tc.name, tc.data, under, got, fixShErr(err), tc.want, tc.wantErr)
			}
		}
	}
	// Controls: a stream that ends between structures is a clean io.EOF.
	for _, c := range []struct {
		data    string
		message bool
	}{{"", true}, {"fe", true}, {"0300", true}, {"", false}, {"d00000", false}} {
		data := fixShHex(t, c.data)
		var r io.Reader = NewRecordReader(bytes.NewReader(data))
		if c.message {
			r = NewMessageReader(bytes.NewReader(data))
		}
		if n, err := r.Read(make([]byte, 16)); n != 0 || err != io.EOF {
			t.Errorf("control %q message=%v: got %d, %v; want 0, io.EOF", c.data, c.message, n, err)
		}
	}
}

func fixShErr(err error) string {
	if err == nil {
		return "<nil>"
	}
	return fmt.Sprintf("%q (Is io.EOF=%v, Is io.ErrUnexpectedEOF=%v)", err.Error(),
		errors.Is(err, io.EOF), errors.Is(err, io.ErrUnexpectedEOF))
}
