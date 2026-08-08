package seal

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

// §6.4 — the LF section encoding, used identically for the public and the
// encrypted section. Port of crates/me-cli/src/seal/container.rs plus the
// device-only pre-split separator scan.

func joinRecords(recs []string) []byte { return []byte(strings.Join(recs, "\n")) }

func TestSplitSectionRoundTripsAVectorSection(t *testing.T) {
	for _, name := range []string{"D", "G"} {
		v := vectorNamed(t, name)
		recs, n, err := SplitSection(joinRecords(v.Public))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if n != len(v.Public) {
			t.Errorf("%s: n = %d, want %d", name, n, len(v.Public))
		}
		if len(recs) != len(v.Public) {
			t.Fatalf("%s: split into %d records, want %d", name, len(recs), len(v.Public))
		}
		for i := range recs {
			if string(recs[i]) != v.Public[i] {
				t.Errorf("%s record %d:\n got %q\nwant %q", name, i, recs[i], v.Public[i])
			}
		}
	}
}

// THE allocation test. A plaintext of 8191 LF bytes satisfies ct_len <= 8191;
// an implementation that splits first materialises ~8192 slice headers
// (~98 KB on a 32-bit target), a fifth of the free heap, transiently. With
// ct_len == 0 this is reachable with NO passphrase and NO KDF at all.
//
// A return-value assertion here is a GUARANTEED false PASS: 8191 LF bytes gives
// 8192 empty records, so a correct pre-split scan and a split-then-count mutant
// both reach "too many records" and both reject with the same error. The only
// observable difference is the allocation, and the bound is 0 — NOT "O(1)".
// bytes.Split performs exactly ONE allocation regardless of record count, and a
// correct bytes.Count-style scan performs ZERO. Both are "O(1)", so a threshold
// of `allocs <= 2` passes the mutant and ships the defect with everything green.
func TestOverlongSectionRejectsBeforeSplitting(t *testing.T) {
	buf := bytes.Repeat([]byte{'\n'}, 8191)
	var gotN int
	var gotErr error
	allocs := testing.AllocsPerRun(100, func() {
		_, gotN, gotErr = SplitSection(buf)
	})
	if !errors.Is(gotErr, ErrTooManyRecords) {
		t.Fatalf("8191 LF bytes must be refused as too many records, got %v", gotErr)
	}
	if gotN != 8192 {
		t.Errorf("record count reported as %d, want 8192 — the caller needs it to name the count", gotN)
	}
	if allocs != 0 {
		t.Errorf("SplitSection allocated %v times on the reject path, want 0 — "+
			"the count must be scanned before any split", allocs)
	}
}

func TestRejectsMoreThan24Records(t *testing.T) {
	rec := vectorNamed(t, "D").Public[0]
	ok := make([]string, MaxRecords)
	for i := range ok {
		ok[i] = rec
	}
	if _, n, err := SplitSection(joinRecords(ok)); err != nil || n != MaxRecords {
		t.Errorf("%d records is legal (a 2-of-3 bundle is 15): n=%d err=%v", MaxRecords, n, err)
	}
	too := append(ok, rec) //nolint:gocritic // deliberately one over the cap
	_, n, err := SplitSection(joinRecords(too))
	if !errors.Is(err, ErrTooManyRecords) {
		t.Errorf("%d records must be refused, got %v", MaxRecords+1, err)
	}
	// The count comes back OUT OF BAND so the error itself can stay a
	// preallocated sentinel; the caller composes the message that names it.
	if n != MaxRecords+1 {
		t.Errorf("n = %d, want %d", n, MaxRecords+1)
	}
}

func TestRejectsARecordOver512Bytes(t *testing.T) {
	if _, _, err := SplitSection([]byte(strings.Repeat("q", MaxRecordLen))); err != nil {
		t.Errorf("%d bytes is legal: %v", MaxRecordLen, err)
	}
	if _, _, err := SplitSection([]byte(strings.Repeat("q", MaxRecordLen+1))); !errors.Is(err, ErrRecordTooLong) {
		t.Errorf("%d bytes must be refused, got %v", MaxRecordLen+1, err)
	}
	// ... and in a multi-record section, not only alone.
	long := "a\n" + strings.Repeat("q", MaxRecordLen+1) + "\nb"
	if _, _, err := SplitSection([]byte(long)); !errors.Is(err, ErrRecordTooLong) {
		t.Errorf("an overlong record in position 1 must be refused, got %v", err)
	}
}

// §6.4: "No CR. A 0x0D anywhere is a malformed bundle. CRLF is rejected, not
// tolerated."
func TestRejectsACarriageReturnAnywhere(t *testing.T) {
	for _, s := range []string{"a\r\nb", "a\rb", "a\nb\r", "\ra\nb", "a\nb\rc"} {
		if _, _, err := SplitSection([]byte(s)); !errors.Is(err, ErrCarriageReturn) {
			t.Errorf("%q must be refused as containing CR, got %v", s, err)
		}
	}
}

// No empty record. This independently rejects "\n\n", a leading LF and a
// trailing LF, which is why §6.4 states it as one rule.
func TestRejectsEmptyRecords(t *testing.T) {
	for _, s := range []string{"", "\n", "a\n", "\na", "a\n\nb", "a\n\n"} {
		if _, _, err := SplitSection([]byte(s)); !errors.Is(err, ErrEmptyRecord) {
			t.Errorf("%q must be refused as containing an empty record, got %v", s, err)
		}
	}
}

// A single record with no separator at all is a legal section.
func TestSplitsASingleRecord(t *testing.T) {
	recs, n, err := SplitSection([]byte("md1abc"))
	if err != nil || n != 1 || len(recs) != 1 || string(recs[0]) != "md1abc" {
		t.Errorf("single record: recs=%q n=%d err=%v", recs, n, err)
	}
}
