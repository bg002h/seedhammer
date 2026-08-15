// This file deliberately carries NO build tag, for the reason toolpath.go
// gives: the js half cannot be tested on this host, so the part that can be is
// kept separate. nfc_js.go is the //go:build js half.

package main

import (
	"io"
	"sync"
)

// nfcSource is a QUEUE of records the page supplied, behind a reader that
// outlives any single tag.
//
// WHY THIS EXISTS. Platform.NFCReader returned nil — "this emulator has no tag
// source" — which was fine while nothing above it cared. It is not fine now: the
// systemwide work makes NFC a first-class secret path for eight programs, and
// the tool used to qualify screens would be blind to the one path §5.4 removed
// all integrity checking from. SPEC_systemwide_payloads §8.2.
//
// The mechanism is the one cmd/emu/platform.go already sketched for exactly
// this: "a syscall/js read of location.search or a JS global set from the host
// page". Here it is `window.shNFC`.
//
// WHY IT IS A QUEUE BEHIND A PERSISTENT READER, and not the one-shot it was.
// The first version handed out a fresh reader per pending record and nil when
// none was pending. That made a multi-card walk IMPOSSIBLE, and the reason is
// worth writing down because nothing about it is visible from this file:
//
//   - A screen fetches Platform.NFCReader() exactly ONCE, at entry
//     (gui/bundle_flow.go:110, gui/md1_gather.go:85, and three more).
//   - startScanner(ctx, nil) returns a channel that never delivers
//     (gui/nfc_scan.go:47).
//
// So a tag presented AFTER a flow was entered could never arrive — the flow had
// already taken its nil — and a tag presented before arrived exactly once, after
// which the reader was drained for the life of the flow. Measured in a browser:
// presenting an md1 before entering Engrave Bundle gave "md1 descriptors: 1";
// presenting an mk1 while inside left it at 1 forever. Trace A needs TWO
// cosigner cards and Trace B several, so no bundle walk could ever have
// completed.
//
// A persistent reader is also the more faithful model. A machine HAS a reader;
// tags come and go past it. Idle is "no tag on the reader right now", which is
// what (0, io.EOF) means to gui/scan.go — not "there is no reader".
type nfcSource struct {
	mu sync.Mutex
	// queue holds presented records not yet delivered, in presentation order.
	queue [][]byte
	// cur is the record currently crossing the reader, and off how much of it
	// has been read. A record larger than the scanner's read buffer is
	// delivered over several Reads, which gui/scan.go reports as progress.
	cur []byte
	off int
	// presentedCount is how many records the PAGE has offered this reader for
	// the life of the session. It only ever goes up: clear() drops the queue
	// and Read drains it, but neither un-presents a tag that already crossed.
	//
	// It exists for F-174. A stage-gate walk of the Build-policy flow must be
	// able to assert that the cosigner cards came from the payload and NOT from
	// the reader, because a gather completed over the emulated reader is green
	// whether or not that feature exists — and phase-1 hardware has no reader.
	//
	// There is deliberately NO reset, for engraved.go's reason: a counter a
	// driver can zero just before asserting is a gate that always passes.
	// Reload the page for a fresh walk.
	presentedCount int
	// detached emulates a machine with NO READER AT ALL, which is a distinct
	// state from "a reader with no tag on it" and one gui treats differently:
	// nil is a supported value and the flows offer Back-only where a scan row
	// would go. Losing the ability to walk that path would be a regression, so
	// it is kept as an explicit mode rather than as a side effect of the queue
	// being empty.
	detached bool
}

// set queues a record. An empty string clears the queue, which is what
// window.shNFC.clear() calls.
func (n *nfcSource) set(rec string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if rec == "" {
		n.queue = nil
		n.cur = nil
		n.off = 0
		return
	}
	n.queue = append(n.queue, []byte(rec))
	n.presentedCount++
}

// presented reports how many records have crossed this reader this session.
//
// Zero is the assertion a Build-policy stage gate makes: the cosigner set came
// from the payload, not from a tag the driver waved at the machine.
func (n *nfcSource) presented() int {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.presentedCount
}

// detach and attach switch between a machine with no reader and one with a
// reader. Takes effect on the NEXT flow entry, because a screen fetches the
// reader once at entry.
func (n *nfcSource) detach(off bool) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.detached = off
}

// reader hands out the source itself, for the life of the flow that asked.
//
// nil is still a SUPPORTED value and still reachable, via detach — a platform
// with no tag source says so by returning nil, and gui relies on that. What is
// gone is nil meaning merely "no tag is pending right now", which conflated an
// absent reader with an idle one and cost every walk its second card.
func (n *nfcSource) reader() io.ReadCloser {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.detached {
		return nil
	}
	return n
}

// Read delivers at most one record per EOF, because EOF is what delimits a tag.
//
// gui/scan.go accumulates across Reads and parses when Read reports io.EOF, so:
// bytes plus io.EOF is a complete tag, and (0, io.EOF) with nothing queued is an
// idle reader — which gui/nfc_scan.go backs off on rather than spinning (F-126).
// Returning nil error while a record still has bytes left reports progress.
func (n *nfcSource) Read(p []byte) (int, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.cur == nil {
		if len(n.queue) == 0 {
			return 0, io.EOF
		}
		n.cur, n.queue = n.queue[0], n.queue[1:]
		n.off = 0
	}
	c := copy(p, n.cur[n.off:])
	n.off += c
	if n.off >= len(n.cur) {
		n.cur = nil
		n.off = 0
		return c, io.EOF
	}
	return c, nil
}

// Close ends the flow's use of the reader without ending the SOURCE.
//
// startScanner's stop function closes the reader when its screen exits
// (gui/nfc_scan.go:102), and every screen exits. A Close that killed the source
// would leave the emulator with a dead NFC path after the first flow, which is
// the same walk-stopping shape this type was just rewritten to remove. It drops
// a half-read record, because a tag interrupted mid-read did not finish
// crossing the reader.
//
// That interruption is a NEW possibility the one-shot source did not have, so
// it is written down rather than left implicit: a record big enough to span
// several Reads can be cut off by Close or by shNFC.clear(), and the bytes
// already handed over reach gui/scan.go as a truncated record. Probed rather
// than reasoned about -- the parser rejects them (errScanUnknownFormat), never
// mistaking them for a different valid tag, and the result lands on a channel
// that is already tearing down. Benign, but it is the kind of benign that stops
// being benign if the parser ever gains a lenient path.
func (n *nfcSource) Close() error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.cur = nil
	n.off = 0
	return nil
}
