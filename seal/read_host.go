//go:build !tinygo

// The build tag is EXPLICIT and must stay that way. Go derives implicit build
// constraints only from _GOOS/_GOARCH filename suffixes, and "_host" is
// neither — without the tag the firmware build compiles this body AND
// read_tinygo.go's and fails on redeclaration. Host `go test` would stay green
// and the break would surface only at flash time.

package seal

import (
	"errors"
	"io"
	"io/fs"
	"os"
)

// FileReader is the host stand-in for the XIP read: it serves the payload
// region out of a file, so `go test` can drive the whole pipeline from
// testdata/vectors.json without any hardware.
type FileReader struct {
	Path string
}

var _ Reader = FileReader{}

// Read returns at most RegionLen bytes, matching what the device sees. A
// region that is absent, unreadable, or does not begin with MNEMBLOB is
// ErrNoPayload (§6.1) — never (nil, nil).
func (r FileReader) Read() ([]byte, error) {
	f, err := os.Open(r.Path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			// On the device an unprogrammed region and an absent one are
			// indistinguishable, so they must be indistinguishable here too.
			return nil, ErrNoPayload
		}
		return nil, err
	}
	defer f.Close()

	buf := make([]byte, RegionLen)
	// A region shorter than 64 KiB is normal, not an error — the device reads
	// a fixed window and the blob simply ends earlier.
	n, err := io.ReadFull(f, buf)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return nil, err
	}
	// Bound by the region, exactly as read_tinygo.go does, and via the same
	// untagged helper — this call is what the unbounded-read mutant has to
	// survive.
	buf = buf[:clampRegion(n)]
	if !hasMagic(buf) {
		return nil, ErrNoPayload
	}
	return buf, nil
}
