//go:build !tinygo

package sysw

import "os"

// FileReader is the host stand-in for XIPReader, so flows are testable off the
// device. It reads at most RegionLen, matching what the device sees.
type FileReader struct{ Path string }

var _ Reader = FileReader{}

func (r FileReader) Probe() bool {
	f, err := os.Open(r.Path)
	if err != nil {
		return false
	}
	defer f.Close()
	var head [8]byte
	n, _ := f.Read(head[:])
	return hasMagic(head[:n])
}

func (r FileReader) Read() ([]byte, error) {
	b, err := os.ReadFile(r.Path)
	if err != nil {
		return nil, err
	}
	b = b[:clampRegion(len(b))]
	if !hasMagic(b) {
		return nil, ErrBadMagic
	}
	n, err := boundBlob(b)
	if err != nil {
		return nil, err
	}
	return b[:n], nil
}
