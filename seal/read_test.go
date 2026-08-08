package seal

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// §10.1 detection and the bounded XIP read.
//
// The RegionLen bound lives in an UNTAGGED helper — clampRegion in read.go —
// called by BOTH implementations, precisely so a host test can kill the
// unbounded-read mutant. A bound placed only inside read_tinygo.go is never
// compiled by `go test` and no automated test can reach it.

func TestClampRegionBoundsTheRead(t *testing.T) {
	for _, c := range []struct {
		in, want int
	}{
		{0, 0},
		{1, 1},
		{HeaderLen, HeaderLen},
		{RegionLen - 1, RegionLen - 1},
		{RegionLen, RegionLen},
		{RegionLen + 1, RegionLen},
		{1 << 20, RegionLen},
		{-1, 0}, // a negative length must never become a huge unsigned read
	} {
		if got := clampRegion(c.in); got != c.want {
			t.Errorf("clampRegion(%d) = %d, want %d", c.in, got, c.want)
		}
	}
}

// The bound must be the region constant, not a number that happens to match
// today. If §5 ever moves the region this assertion moves with it.
func TestClampRegionIsTheRegionConstant(t *testing.T) {
	if clampRegion(1<<30) != RegionLen {
		t.Fatalf("clampRegion must saturate at RegionLen (%d)", RegionLen)
	}
	if RegionLen != 65536 {
		t.Errorf("RegionLen = %d; §5 reserves 64 KiB at 0x10E00000", RegionLen)
	}
}

func writeRegion(t *testing.T, b []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "payload.bin")
	if err := os.WriteFile(p, b, 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// §6.1: a blob is present iff the first 8 bytes are MNEMBLOB. Anything else —
// including all-0xFF erased flash — means "no payload", and the feature stays
// invisible in the UI.
func TestFileReaderReportsNoPayloadOnAbsentBlob(t *testing.T) {
	for _, c := range []struct {
		name string
		body []byte
	}{
		{"erased flash", bytes.Repeat([]byte{0xFF}, 512)},
		{"zeroed flash", make([]byte, 512)},
		{"altered magic", func() []byte {
			b := append([]byte(nil), vectorNamed(t, "A").Blob(t)...)
			b[0] ^= 0x01
			return b
		}()},
		{"shorter than the magic", []byte("MNEM")},
		{"empty", nil},
	} {
		r := FileReader{Path: writeRegion(t, c.body)}
		got, err := r.Read()
		if !errors.Is(err, ErrNoPayload) {
			t.Errorf("%s: got (%d bytes, %v), want ErrNoPayload", c.name, len(got), err)
		}
		// Not (nil, nil) — that is the shape callers forget to check.
		if err == nil {
			t.Errorf("%s: a missing payload must be an error value, not a nil slice", c.name)
		}
	}
}

// A missing region file is "no payload" too: on the device an unprogrammed
// region and an absent one are indistinguishable, and Phase B must be able to
// tell both apart from "present but corrupt" with a single errors.Is.
func TestFileReaderReportsNoPayloadWhenTheRegionIsMissing(t *testing.T) {
	r := FileReader{Path: filepath.Join(t.TempDir(), "does-not-exist.bin")}
	if _, err := r.Read(); !errors.Is(err, ErrNoPayload) {
		t.Errorf("missing region: got %v, want ErrNoPayload", err)
	}
}

func TestFileReaderReadsEveryVector(t *testing.T) {
	for _, v := range loadVectors(t) {
		t.Run(v.Name, func(t *testing.T) {
			blob := v.Blob(t)
			// The real region is 64 KiB of flash with the blob at the front and
			// undefined bytes after it, so pad exactly as flash would.
			region := append(append([]byte(nil), blob...), bytes.Repeat([]byte{0xFF}, 1024)...)
			r := FileReader{Path: writeRegion(t, region)}
			got, err := r.Read()
			if err != nil {
				t.Fatalf("Read: %v", err)
			}
			if len(got) > RegionLen {
				t.Fatalf("Read returned %d bytes, more than the region's %d", len(got), RegionLen)
			}
			// The reader bounds by the REGION only. Bounding by the header's
			// own lengths is Task 2's job and happens after this, because
			// those lengths are attacker-controlled.
			if !bytes.HasPrefix(got, blob) {
				t.Error("the blob must come back byte-identical at the front of the region")
			}
			h, err := ParseHeader(got)
			if err != nil {
				t.Fatalf("the read region must parse: %v", err)
			}
			if h.PubLen != v.PubLen || h.CtLen != v.CtLen {
				t.Errorf("header from the region: pub=%d ct=%d, vector declares %d/%d",
					h.PubLen, h.CtLen, v.PubLen, v.CtLen)
			}
		})
	}
}

// THE unbounded-read mutant's killer. An oversized region must come back
// clamped, and it must still be readable.
func TestFileReaderNeverReturnsMoreThanTheRegion(t *testing.T) {
	blob := vectorNamed(t, "A").Blob(t)
	oversize := append(append([]byte(nil), blob...), bytes.Repeat([]byte{0xFF}, 4*RegionLen)...)
	r := FileReader{Path: writeRegion(t, oversize)}
	got, err := r.Read()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(got) != RegionLen {
		t.Errorf("Read returned %d bytes from a %d-byte region, want exactly %d",
			len(got), len(oversize), RegionLen)
	}
}

// Reader is the seam Phase B depends on; assert the host implementation
// satisfies it so a signature change cannot pass unnoticed.
func TestFileReaderSatisfiesReader(t *testing.T) {
	var _ Reader = FileReader{}
	var r Reader = FileReader{Path: writeRegion(t, vectorNamed(t, "A").Blob(t))}
	if _, err := r.Read(); err != nil {
		t.Fatalf("Read through the interface: %v", err)
	}
}
