package seal

import (
	"encoding/binary"
	"errors"
	"testing"
)

// boundBlob is what makes a reader allocate the payload's real size instead of
// the 64 KiB region size. It lives in the UNTAGGED read.go precisely so this
// test can reach it: a bound inside read_tinygo.go is never compiled by
// `go test`, which is the rule read.go's own header states.

// TestBoundBlobMatchesTheHeadersOwnArithmetic pins the formula against every
// vector, so a payload that grows a section cannot silently be truncated.
func TestBoundBlobMatchesTheHeadersOwnArithmetic(t *testing.T) {
	for _, v := range loadVectors(t) {
		t.Run(v.Name, func(t *testing.T) {
			blob := v.Blob(t)
			h, err := ParseHeader(blob)
			if err != nil {
				t.Fatalf("vector %s has an unparseable header: %v", v.Name, err)
			}
			want := HeaderLen + int(h.PubLen) + int(h.CtLen)
			if h.Sealed() {
				want += TagLen
			}
			got, err := boundBlob(blob)
			if err != nil {
				t.Fatalf("boundBlob: %v", err)
			}
			if got != want {
				t.Errorf("boundBlob = %d, want %d (52 + pub %d + ct %d%s)",
					got, want, h.PubLen, h.CtLen, map[bool]string{true: " + 16"}[h.Sealed()])
			}
			// The point of the change: far below the region, so no 64 KiB
			// contiguous run is ever needed.
			if got > RegionLen {
				t.Errorf("boundBlob = %d, which exceeds the region %d", got, RegionLen)
			}
		})
	}
}

// TestBoundBlobRejectsAnOverlongSectionBeforeArithmetic is the overflow guard.
//
// pub_len = 0xFFFFFFFF is the value that goes NEGATIVE under int() on a 32-bit
// target. The guarantee is one of ORDER: ParseHeader validates against
// MaxSectionLen before boundBlob consults either length, so the arithmetic
// never sees it.
//
// Deliberately NOT an allocation assertion. ParseHeader's reject path is
// fmt.Errorf, which allocates a formatted string and a wrapError, so
// "must not allocate" could never pass and would be a test that fails for a
// reason unrelated to the property.
func TestBoundBlobRejectsAnOverlongSectionBeforeArithmetic(t *testing.T) {
	for _, tc := range []struct {
		name   string
		offset int
		want   error
	}{
		{"pub_len", 44, ErrPubLen},
		{"ct_len", 48, ErrCtLen},
	} {
		t.Run(tc.name, func(t *testing.T) {
			blob := append([]byte(nil), vectorNamed(t, "A").Blob(t)...)
			binary.BigEndian.PutUint32(blob[tc.offset:tc.offset+4], 0xFFFFFFFF)
			got, err := boundBlob(blob)
			if !errors.Is(err, tc.want) {
				t.Errorf("boundBlob error = %v, want %v", err, tc.want)
			}
			if got != 0 {
				t.Errorf("boundBlob returned length %d on a rejected header; "+
					"an attacker-controlled length reached the arithmetic", got)
			}
		})
	}
}

// TestBoundBlobRejectsARegionShorterThanAHeader covers the constant-bounded
// stage: the HeaderLen slice must never be taken from a shorter region.
func TestBoundBlobRejectsARegionShorterThanAHeader(t *testing.T) {
	if _, err := boundBlob(make([]byte, HeaderLen-1)); !errors.Is(err, ErrTooShort) {
		t.Errorf("boundBlob on a short region = %v, want ErrTooShort", err)
	}
}
