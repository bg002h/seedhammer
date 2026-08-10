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
		// Probe MUST agree with Read about what "present" means: §10.1 routes
		// the menu's visibility through Probe and the "payload unreadable"
		// message through Read, and the deliberate asymmetry between them only
		// holds if they agree on ABSENT. Asserted here rather than in its own
		// test so the two can never drift apart case by case.
		if r.Probe() {
			t.Errorf("%s: Probe reports present where Read reports ErrNoPayload", c.name)
		}
	}
}

// The POSITIVE half, and it is what makes the negatives above mean something.
// Without it `func (FileReader) Probe() bool { return true }` passes the whole
// suite — measured by the B2a-i whole-diff review, which is how this test came
// to exist. §10.1's "present → the entry appears" had exactly one one-directional
// pin, and a one-directional kill is not a kill.
func TestFileReaderProbeAgreesWithReadOnPresent(t *testing.T) {
	for _, v := range loadVectors(t) {
		t.Run(v.Name, func(t *testing.T) {
			region := append(append([]byte(nil), v.Blob(t)...), bytes.Repeat([]byte{0xFF}, 1024)...)
			r := FileReader{Path: writeRegion(t, region)}
			if !r.Probe() {
				t.Error("Probe reports absent on a vector blob Read accepts")
			}
			if _, err := r.Read(); err != nil {
				t.Errorf("premise broken: Read rejected vector %s: %v", v.Name, err)
			}
		})
	}
}

// A missing region file must Probe false too — the branch Probe reaches by
// os.Open failing, which no other test touches.
func TestFileReaderProbeReportsAbsentWhenTheRegionIsMissing(t *testing.T) {
	r := FileReader{Path: filepath.Join(t.TempDir(), "does-not-exist.bin")}
	if r.Probe() {
		t.Error("Probe reports present for a region file that does not exist")
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
// bounded, and it must still be readable.
//
// The bound TIGHTENED. It used to be RegionLen, because Read returned the whole
// clamped region; it is now what the header itself declares, 52+pub+ct(+16), so
// that no reader needs a 64 KiB CONTIGUOUS run on a non-moving collector to
// hold a payload that cannot legally exceed 16,450 bytes and is typically
// ~1,400. Both readers take the trim through the same untagged helper, so host
// and device return identical lengths.
//
// TWO assertions, because either alone is weak: "<= RegionLen" would pass for
// any truncation at all, and the exact figure alone would pass even if
// clampRegion were deleted. Together they pin the tightened bound and keep the
// original unbounded-read mutant dead -- the 4x-oversize region is what makes
// the second one bite.
func TestFileReaderNeverReturnsMoreThanTheRegion(t *testing.T) {
	v := vectorNamed(t, "A")
	oversize := append(append([]byte(nil), v.Blob(t)...), bytes.Repeat([]byte{0xFF}, 4*RegionLen)...)
	r := FileReader{Path: writeRegion(t, oversize)}
	got, err := r.Read()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	want := HeaderLen + int(v.PubLen) + int(v.CtLen)
	if v.CtLen > 0 {
		want += TagLen
	}
	if len(got) != want {
		t.Errorf("Read returned %d bytes from a %d-byte region, want %d (52 + pub %d + ct %d)",
			len(got), len(oversize), want, v.PubLen, v.CtLen)
	}
	if len(got) > RegionLen {
		t.Errorf("Read returned %d bytes, which exceeds the region bound %d", len(got), RegionLen)
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
