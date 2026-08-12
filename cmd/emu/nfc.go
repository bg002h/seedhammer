// This file deliberately carries NO build tag, for the reason toolpath.go
// gives: the js half cannot be tested on this host, so the part that can be is
// kept separate. nfc_js.go is the //go:build js half.

package main

import (
	"bytes"
	"io"
	"sync"
)

// nfcSource is a one-shot reader over a record the PAGE supplied.
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
type nfcSource struct {
	mu   sync.Mutex
	rec  []byte
	done bool
}

// set replaces the pending record. Called from the page.
func (n *nfcSource) set(rec string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.rec = []byte(rec)
	n.done = false
}

// reader hands out the record ONCE, then reports no tag.
//
// One-shot because a real tag crosses the reader once. A source that replayed
// forever would let a flow that polls see a tag it never presented, which is a
// behaviour the machine does not have.
func (n *nfcSource) reader() io.ReadCloser {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.done || len(n.rec) == 0 {
		return nil
	}
	n.done = true
	return io.NopCloser(bytes.NewReader(n.rec))
}
