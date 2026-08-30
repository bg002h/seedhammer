package gui

import (
	"errors"
	"io"
	"log"
	"time"
)

// nfcIdlePoll is how long the scanner waits after a poll that produced nothing
// at all before asking the reader again.
//
// F-126, AND THE REASON THIS FILE EXISTS. The loop below lived in five copies
// (StartScreen.Flow, bundleScanFlow, md1GatherFlow, mk1GatherFlow,
// scanAddressFlow), and every copy yielded ONLY on scanFailed. A reader parked
// at EOF returns (0, io.EOF) forever, which Scan reports as "no object, no
// status" -- not a failure -- so the loop spun as fast as the reader could
// answer.
//
// MEASURED, and only this is measured: 4 reads in 150 ms with the wait below,
// against ~198,000 loop iterations in the same window without it
// (TestNFCScannerDoesNotSpinAtEOF, plus its mutant). What that costs on the
// device, and F-126's report that it freezes the wasm emulator outright, are
// INHERITED CLAIMS -- neither was reproduced here, and an attempt in a real
// browser did not reach this loop at all, because a screen fetches
// Platform.NFCReader() once at entry and cmd/emu's source returns nil until a
// record is pending. Believe the number; treat the consequences as the report's.
//
// Stage 10 is what makes this path reachable from the seam, so the spin had to
// stop shipping with it. Fixing it in five places would have left the sixth --
// the one stage 10 adds -- free to reintroduce it, so the loop is one function
// now and its shape exists once.
//
// A poll that reported progress, an unknown format or a failure DID see a tag
// and is not slowed down; only the nothing-at-all case waits.
const nfcIdlePoll = 50 * time.Millisecond

// scannerJoinTimeout bounds the wait for the scanner goroutine to exit after
// its reader has been closed. It is longer than the scanner's own worst-case
// sleep (1 s on a failed scan) so a healthy stop is never cut short.
//
// The two bounds COMPOSE: a Close that succeeds just under its own 2 s limit
// still pays this join, so the worst-case back-edge stall is ~5 s, not 3 --
// visible only when the reader is already misbehaving (the healthy path
// measures ~50 ms end to end). Stated because a 5 s dead panel reads exactly
// like the freeze this bound exists to prevent (REVIEW-F440-F441-r1 M-2).
const scannerJoinTimeout = 3 * time.Second

// startScanner runs the NFC poll loop on its own goroutine.
//
// It returns the results channel and the stop function EVERY caller must defer.
// A nil reader is supported and is not a stub -- a platform with no tag source
// says so by returning nil -- and it yields a channel that never delivers, which
// is exactly what the five call sites relied on when they wrapped the goroutine
// in `if r != nil`. Callers therefore need no nil check of their own.
func startScanner(ctx *Context, r io.ReadCloser) (chan scanResult, func()) {
	scans := make(chan scanResult, 1)
	if r == nil {
		return scans, func() {}
	}
	closer := make(chan struct{})
	closed := make(chan struct{})
	wakeup := ctx.Platform.Wakeup
	go func() {
		s := new(scanner)
		for {
			select {
			case <-closer:
				close(closed)
				return
			default:
			}
			obj, err := s.Scan(r)
			scan := scanResult{Object: obj}
			switch {
			case errors.Is(err, errScanInProgress):
				scan.Status = scanStarted
			case errors.Is(err, errScanUnknownFormat):
				scan.Status = scanUnknownFormat
			case err == nil || err == io.EOF:
			default:
				scan.Status = scanFailed
				log.Printf("nfc scan: %v", err)
			}
			// Computed from THIS poll, BEFORE the merge below. The merge can
			// carry an unconsumed object forward from a previous poll, and
			// keying the backoff on the merged result would then spin for as
			// long as the screen left that object unread -- a second spin, in
			// the case the merge exists to handle.
			idle := scan.Object == nil && scan.Status == scanIdle
			// Merge the previous result.
			select {
			case old := <-scans:
				if scan.Object == nil {
					scan.Object = old.Object
				}
				scan.Status = max(scan.Status, old.Status)
			default:
			}
			scans <- scan
			wakeup()
			switch {
			case scan.Status == scanFailed:
				// Wait a bit before attempting to scan again.
				time.Sleep(1 * time.Second)
			case idle:
				time.Sleep(nfcIdlePoll)
			}
		}
	}()
	return scans, func() {
		close(closer)
		// F-441: ABANDON, NEVER BLOCK. This ran `r.Close()` and then joined the
		// goroutine unconditionally, and both waits were unbounded. They run on
		// the UI goroutine -- the one that owns the frame loop and the event
		// pump -- so a reader that would not stop froze the panel on its last
		// frame with every button dead and no way out but a power cycle. That is
		// what the operator hit: "hung the moment I hit back", checkmark
		// included, still dead minutes later.
		//
		// A leaked goroutine and a degraded reader are strictly better than a
		// bricked machine, so that is the trade taken here. The error is logged
		// rather than surfaced because there is no screen to surface it on: the
		// caller is a deferred teardown on the way OUT of a flow.
		// Any Close error skips the join, not just the abandon: a healthy-arm
		// I2C failure exits the same way, and the goroutine leaves on its own
		// at the top of its next iteration. Not errors.Is(ErrCloseTimeout)
		// because gui does not import nfc/poller (REVIEW-F440-F441-r1 N-1).
		if err := r.Close(); err != nil {
			log.Printf("nfc: %v", err)
			return
		}
		// The join is bounded too, and for its own reason: the scanner sleeps up
		// to a second between polls, so even a healthy stop is not instant --
		// and a stop that is not healthy must not be paid for in frames.
		t := time.NewTimer(scannerJoinTimeout)
		defer t.Stop()
		select {
		case <-closed:
		case <-t.C:
			log.Printf("nfc: scanner did not exit within %v; abandoning it", scannerJoinTimeout)
		}
	}
}
