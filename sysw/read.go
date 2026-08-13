package sysw

// UNTAGGED on purpose, exactly as seal/read.go is: the bound below is called by
// BOTH the TinyGo and host readers, so it must be reachable by `go test`. A
// bound that lives only in the //go:build tinygo file is never compiled by the
// test runner and no automated test can reach it.

// Reader is a source of the region's bytes. nil is a supported value at the
// Platform level, not a stub — the same contract seal.Reader has.
type Reader interface {
	// Probe reports whether the region holds a container, cheaply.
	Probe() bool
	Read() ([]byte, error)
}

// clampRegion bounds a read to the region.
//
// It runs BEFORE the header is parsed, so it may not consult pub_len or ct_len:
// those are attacker-controlled and are bound-checked by ParseHeader afterwards.
// At this point the region constant is the only trustworthy bound there is.
func clampRegion(n int) int {
	if n < 0 {
		// A negative length must never reach a slice or a memcpy: on a 32-bit
		// target it reappears as a very large unsigned count.
		return 0
	}
	if n > RegionLen {
		return RegionLen
	}
	return n
}

func hasMagic(b []byte) bool {
	return len(b) >= len(MAGIC) && [8]byte(b[:8]) == MAGIC
}

// boundBlob reports how many bytes of region the header declares, so a reader
// allocates the payload's real size instead of the region's.
//
// THE ORDER IS THE SAFETY ARGUMENT. ParseHeader is handed a HeaderLen-bounded
// slice — a CONSTANT bound, so nothing attacker-controlled sizes it — and it
// rejects either section length above MaxSectionLen. Only after it returns may
// those lengths be used for arithmetic: both are then proven <= 8191, so the
// sum cannot wrap a 32-bit int.
func boundBlob(region []byte) (int, error) {
	if len(region) < HeaderLen {
		return 0, ErrTooShort
	}
	h, err := ParseHeader(region[:HeaderLen])
	if err != nil {
		return 0, err
	}
	total := h.TotalLen()
	if total > len(region) {
		return 0, ErrTooShort
	}
	return total, nil
}
