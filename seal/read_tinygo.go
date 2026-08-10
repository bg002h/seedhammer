//go:build tinygo

package seal

import "unsafe"

// The real XIP read (§5, §10.1).
//
// There is NO precedent for this shape anywhere in the repo: every other
// unsafe.Pointer/unsafe.Slice site takes the address of an already-typed
// peripheral register struct field (driver/dma/dma_rp2.go:70 is the nearest
// analogue), and driver/otp/otp_rp2350.go is a cgo bootrom CALL, not a fixed
// address dereference. The spec's own citation of otp_rp2350.go:13 is stale —
// that line is a #define.
//
// Which form TinyGo 0.41.1 compiles correctly here is therefore an
// implementation-time question settled by a test ON HARDWARE (§10.1), not a
// design question. Task 7 Step 4 of the plan is that test; until it has run on
// a Pico 2 this body is unverified on target.

// PayloadAddr is the payload region's XIP base (§5). It is fixed and normative:
// `me seal` deliberately has no --addr flag, because any other value produces a
// blob the device never looks at — and past 0x11000000 a write wraps to
// 0x10000000 and destroys the firmware (RP2350 datasheet §5.5.2).
//
// This constant, and the pointer arithmetic below, exist ONLY in this file and
// its untagged sibling. No package outside them may name an absolute address.
const PayloadAddr = 0x10E00000

// XIPReader reads the payload region straight out of execute-in-place flash.
type XIPReader struct{}

var _ Reader = XIPReader{}

// Probe reads only the magic. unsafe.Slice over XIP is a MAPPING, not a copy,
// so this allocates nothing at all — which is the whole point of F-79.
func (XIPReader) Probe() bool {
	region := unsafe.Slice((*byte)(unsafe.Pointer(uintptr(PayloadAddr))), clampRegion(len(Magic)))
	return hasMagic(region)
}

// Read returns the region's bytes, bounded by clampRegion — the SAME untagged
// helper the host reader uses, which is what makes the bound testable at all.
//
// The bound is the region constant alone. The header's own lengths are
// attacker-controlled and are checked by ParseHeader, but that happens AFTER
// this read, so nothing here may consult them.
func (XIPReader) Read() ([]byte, error) {
	region := unsafe.Slice((*byte)(unsafe.Pointer(uintptr(PayloadAddr))), clampRegion(RegionLen))
	if !hasMagic(region) {
		return nil, ErrNoPayload
	}
	// Copy out of XIP before returning. The caller keeps these bytes across a
	// flash write (Phase B's wipe path erases the region), and a slice aliasing
	// XIP would silently change underneath it.
	n, err := boundBlob(region)
	if err != nil {
		return nil, err
	}
	out := make([]byte, n)
	copy(out, region[:n])
	return out, nil
}
