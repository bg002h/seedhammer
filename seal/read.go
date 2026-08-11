package seal

import "errors"

// §10.1 — detection, and the bounded read of the payload region.
//
// This file is UNTAGGED on purpose. The RegionLen bound lives here, in
// clampRegion, and is called by BOTH the host and the TinyGo implementation, so
// that a host `go test` can kill the unbounded-read mutant. A bound placed
// only inside read_tinygo.go is never compiled by `go test` and no automated
// test can reach it.
//
// No package outside read.go / read_host.go / read_tinygo.go may reference the
// payload's absolute address.

// ErrNoPayload means no blob is present: erased flash, an unprogrammed region,
// or anything whose first 8 bytes are not MNEMBLOB (§6.1). Phase B matches it
// with errors.Is and MUST distinguish it from "present but corrupt" — absent
// means the feature stays invisible, corrupt means "payload unreadable".
//
// Deliberately an error and not (nil, nil): a nil slice with a nil error is the
// shape callers forget to check.
var ErrNoPayload = errors.New("seal: no payload present")

// Reader yields the payload region's bytes. Phase B holds one of these and
// never learns where the bytes came from.
type Reader interface {
	// Probe reports whether the region begins with MNEMBLOB (§6.1). It exists
	// so §10.1's startup detection costs 8 bytes rather than 64 KB: Read's
	// result is retained for as long as the caller holds it, and holding the
	// whole region for the GUI's lifetime is ~14% of free heap (F-79).
	//
	// It is deliberately COARSER than Read: present-but-corrupt probes true,
	// the menu entry appears, and the flow then reports "payload unreadable".
	// Collapsing the two would hide a tampered payload behind an invisible menu
	// entry, which is precisely the signal §2.2 item 4 exists to raise.
	Probe() bool
	Read() ([]byte, error)
}

// clampRegion bounds a read length to the payload region (§5).
//
// This runs BEFORE the header is parsed, so it cannot use pub_len or ct_len —
// those are attacker-controlled and are bound-checked by ParseHeader afterwards.
// At this point the region constant is the only trustworthy bound there is.
func clampRegion(n int) int {
	if n < 0 {
		// A negative length must never be handed to a slice or a memcpy: on a
		// 32-bit target it reappears as a very large unsigned count.
		return 0
	}
	if n > RegionLen {
		return RegionLen
	}
	return n
}

// hasMagic reports whether the region holds a payload (§6.1). Erased flash
// reads 0xFF, which is not MNEMBLOB.
func hasMagic(b []byte) bool {
	return len(b) >= len(Magic) && string(b[:len(Magic)]) == Magic
}

// boundBlob reports how many bytes of region the header declares, so a reader
// allocates the payload's real size instead of the region's.
//
// UNTAGGED and shared by both readers, for this file's own stated reason: a
// bound placed inside read_tinygo.go is never compiled by `go test` and no
// automated test can reach it.
//
// THE ORDER IS THE SAFETY ARGUMENT. ParseHeader is handed a HeaderLen-bounded
// slice -- a CONSTANT bound, so nothing attacker-controlled sizes this -- and it
// rejects pub_len or ct_len above MaxSectionLen. Only after it returns nil may
// either length be consulted for arithmetic: both are then proven <= 8191, so
// int() cannot reinterpret them and the sum cannot wrap a 32-bit int. Reading
// them any earlier reintroduces the negative-length overflow unlock_key.go
// documents.
//
// RegionLen is 65,536 while the format's own caps admit at most
// 52+8191+8191+16 = 16,450, and the largest real vector is 1,421 bytes -- so
// this typically allocates ~46x less than the region, and never needs a 64 KiB
// contiguous run on a non-moving collector.
func boundBlob(region []byte) (int, error) {
	if len(region) < HeaderLen {
		return 0, ErrTooShort
	}
	h, err := ParseHeader(region[:HeaderLen])
	if err != nil {
		return 0, err
	}
	total := HeaderLen + int(h.PubLen) + int(h.CtLen)
	if h.Sealed() {
		total += TagLen
	}
	// Defence in depth, and UNREACHABLE today: clampRegion(RegionLen) is
	// RegionLen and total is <= 16,450. Kept for the reason wire.go keeps its
	// own unreachable total check, and ErrTooLarge rather than ErrTooShort
	// because "does not fit the flash region" is the condition being reported.
	if total > len(region) {
		return 0, ErrTooLarge
	}
	return total, nil
}
