package poller

import (
	"errors"
	"io"
	"testing"
	"time"
)

// ═══ F-441: Close must not stake the UI goroutine on a read it cannot stop ═══
//
// From the bench, 2026-08-29: "hung the moment I hit back arrow", every button
// dead including the checkmark, still dead minutes later, power cycle only.
//
// The path: a flow's `defer stopScanner()` (gui/nfc_scan.go) runs on the BACK
// edge and calls Poller.Close, which signalled the in-flight read and then
// waited for it on `p.reading <- struct{}{}` with NO BOUND. That wait is on the
// goroutine that also owns the frame loop and the event pump, so a read that
// does not stop is not a slow reader -- it is a bricked machine: no frame is
// drawn, no input is polled, and nothing times out.
//
// WHY THE SIM NEVER SAW IT, and why this file exists: testPlatform.NFCReader()
// returns nil, and startScanner answers a nil reader with a no-op stop
// function. The entire mechanism was unreachable from Go tests by construction.
// stalledDevice is the stub that gap was missing.

// stalledDevice is a reader that CANNOT be stopped: its read parks on a channel
// the test owns and never watches for cancellation.
//
// It is deliberately not a model of the st25r3916. It is a model of the only
// thing Close can actually assume about a device -- that a read it asked to
// stop might not stop -- because that assumption is what the bound exists for.
// A device that stalls for ANY reason (a wedged I2C transaction, a signal that
// went missing, firmware that never returns) presents to Close identically.
type stalledDevice struct {
	release   chan struct{} // closed by the test to let the read finish
	interrupt int           // how many times Interrupt was called
}

func newStalledDevice() *stalledDevice {
	return &stalledDevice{release: make(chan struct{})}
}

func (d *stalledDevice) Read(p []byte) (int, error) {
	<-d.release
	return 0, io.EOF
}
func (d *stalledDevice) Detect() (bool, error) {
	<-d.release
	return false, io.EOF
}
func (d *stalledDevice) Interrupt()                      { d.interrupt++ }
func (d *stalledDevice) Close() error                    { return nil }
func (d *stalledDevice) Write(p []byte) (int, error)     { return len(p), nil }
func (d *stalledDevice) SetProtocol(prot Protocol) error { return nil }
func (d *stalledDevice) Sleep() error                    { return nil }
func (d *stalledDevice) ReadCapacity() int               { return 256 }

// THE REGRESSION, as a bounded test. Before the fix this blocked forever and the
// only thing that ended it was the harness's own timeout -- which is exactly
// what the device could not do.
func TestCloseReturnsAnErrorWhenTheReadCannotBeStopped(t *testing.T) {
	d := newStalledDevice()
	p := New(d)

	reading := make(chan struct{})
	go func() {
		close(reading)
		p.Read(make([]byte, 16)) // parks, holding p.reading
	}()
	<-reading
	// Give the read time to take the semaphore, so Close takes the in-flight
	// arm rather than the free one. Without this the test could measure the
	// wrong branch and pass for the wrong reason.
	waitForReadInFlight(t, p)

	start := time.Now()
	err := p.Close()
	elapsed := time.Since(start)

	if !errors.Is(err, ErrCloseTimeout) {
		t.Fatalf("Close returned %v after %v, want ErrCloseTimeout -- an unbounded "+
			"wait here freezes the UI goroutine and bricks the device", err, elapsed)
	}
	if elapsed > 2*CloseTimeout {
		t.Errorf("Close took %v, more than twice its own %v bound", elapsed, CloseTimeout)
	}
	if d.interrupt == 0 {
		t.Error("Close gave up without ever asking the read to stop")
	}
	close(d.release) // let the parked read go, so the test leaves nothing behind
}

// The healthy path must be untouched: a read that stops is joined immediately
// and Close reports success, never a timeout.
func TestCloseSucceedsWhenTheReadStops(t *testing.T) {
	d := newStalledDevice()
	p := New(d)

	reading := make(chan struct{})
	go func() {
		close(reading)
		p.Read(make([]byte, 16))
	}()
	<-reading
	waitForReadInFlight(t, p)

	// The device stops as soon as it is asked, which is what a working reader
	// does. Close must then take the fast path.
	go func() {
		for d.interrupt == 0 {
			time.Sleep(time.Millisecond)
		}
		close(d.release)
	}()

	start := time.Now()
	err := p.Close()
	if err != nil {
		t.Fatalf("Close on a stoppable read returned %v, want nil", err)
	}
	if elapsed := time.Since(start); elapsed >= CloseTimeout {
		t.Errorf("Close took %v: it waited out the timeout instead of being "+
			"released by the read", elapsed)
	}
}

// Close on an idle poller neither interrupts nor waits.
func TestCloseOnAnIdlePollerIsImmediate(t *testing.T) {
	d := newStalledDevice()
	p := New(d)
	start := time.Now()
	if err := p.Close(); err != nil {
		t.Fatalf("Close on an idle poller returned %v, want nil", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("Close on an idle poller took %v", elapsed)
	}
	if d.interrupt != 0 {
		t.Errorf("Close interrupted %d times with no read in flight", d.interrupt)
	}
}

// waitForReadInFlight blocks until the poller's read semaphore is taken, so a
// test cannot accidentally measure Close's free-semaphore arm.
func waitForReadInFlight(t *testing.T, p *Poller) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case p.reading <- struct{}{}:
			<-p.reading // it was free; put it back and keep waiting
			time.Sleep(time.Millisecond)
			continue
		default:
			return // taken: a read is in flight
		}
	}
	t.Fatal("INCONCLUSIVE: the read never took the semaphore")
}
