//go:build tinygo

package sysw

import "unsafe"

// XIPReader reads the systemwide region straight out of execute-in-place flash.
//
// The absolute address appears in THIS FILE AND NOWHERE ELSE, mirroring
// seal/read_tinygo.go's rule: no package outside these two may name one.
type XIPReader struct{}

var _ Reader = XIPReader{}

// Probe reads only the magic. unsafe.Slice over XIP is a MAPPING, not a copy,
// so this allocates nothing.
func (XIPReader) Probe() bool {
	region := unsafe.Slice((*byte)(unsafe.Pointer(uintptr(RegionAddr))), clampRegion(len(MAGIC)))
	return hasMagic(region)
}

func (XIPReader) Read() ([]byte, error) {
	region := unsafe.Slice((*byte)(unsafe.Pointer(uintptr(RegionAddr))), clampRegion(RegionLen))
	if !hasMagic(region) {
		return nil, ErrBadMagic
	}
	n, err := boundBlob(region)
	if err != nil {
		return nil, err
	}
	out := make([]byte, n)
	copy(out, region[:n])
	return out, nil
}
