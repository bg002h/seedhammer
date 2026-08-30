package gui

import (
	"errors"
	"io"
	"sync"
	"testing"
	"time"
)

// ═══ F-441: the teardown must never block the UI goroutine ══════════════════
//
// stopScanner runs from a flow's `defer` -- on the BACK edge -- on the goroutine
// that owns the frame loop and the event pump. It used to call r.Close() and
// then join the scanner goroutine, both unbounded. A reader that would not stop
// therefore froze the panel on its last frame with every button dead, which is
// what the operator hit and what no test could see: testPlatform.NFCReader()
// returns nil, and startScanner answers a nil reader with a no-op stop.

// unstoppableReader is the stub the sim lacked: a reader whose Read parks and
// whose Close reports that it could not stop it -- exactly Poller.Close's
// ErrCloseTimeout contract.
type unstoppableReader struct {
	release  chan struct{}
	entered  chan struct{} // closed when Read is first entered
	once     sync.Once
	closeErr error
}

func (r *unstoppableReader) Read(p []byte) (int, error) {
	r.once.Do(func() { close(r.entered) })
	<-r.release
	return 0, io.EOF
}
func (r *unstoppableReader) Close() error { return r.closeErr }

// The regression: stop() must RETURN. Before the fix it blocked forever joining
// a goroutine that was parked in Read.
func TestStopScannerAbandonsAReaderThatWillNotStop(t *testing.T) {
	r := &unstoppableReader{
		release:  make(chan struct{}),
		entered:  make(chan struct{}),
		closeErr: errors.New("poller: the reader did not stop; abandoning it"),
	}
	defer close(r.release) // let the parked goroutine go when the test ends

	ctx := NewContext(newPlatform())
	_, stop := startScanner(ctx, r)

	// ARM IT, do not assume it. The scanner checks its closer at the TOP of each
	// iteration, so a stop() issued before the first Read finds the goroutine
	// already leaving and joins instantly -- which would make this test pass
	// without ever exercising a stalled reader.
	select {
	case <-r.entered:
	case <-time.After(10 * time.Second):
		t.Fatal("INCONCLUSIVE: the scanner never entered Read, so nothing was stalled")
	}

	done := make(chan struct{})
	go func() { stop(); close(done) }()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("stopScanner never returned. On the device this is the UI " +
			"goroutine, so the panel is frozen, every button is dead and the " +
			"only way out is a power cycle (F-441).")
	}
}

// The healthy path is untouched: a reader that stops is joined normally, and
// quickly -- the bound must not have turned a working teardown into a 3-second
// stall.
func TestStopScannerJoinsAReaderThatStops(t *testing.T) {
	r := &unstoppableReader{release: make(chan struct{}), entered: make(chan struct{})}
	close(r.release) // Read returns immediately: a well-behaved reader

	ctx := NewContext(newPlatform())
	_, stop := startScanner(ctx, r)

	done := make(chan struct{})
	start := time.Now()
	go func() { stop(); close(done) }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("stopScanner did not join a reader that stopped")
	}
	if elapsed := time.Since(start); elapsed >= scannerJoinTimeout {
		t.Errorf("a healthy teardown took %v and so waited out the %v join bound; "+
			"the bound is for readers that stall, not for working ones",
			elapsed, scannerJoinTimeout)
	}
}

// The join-timeout arm itself, pinned (REVIEW-F440-F441-r1 M-1): a reader whose
// Close SUCCEEDS while its Read stays parked -- Poller.Close's free arm leaves a
// token in p.reading, so a Read issued after Close blocks forever -- must be
// abandoned by the JOIN bound, the one arm the other two tests never reach.
// Unreachable on the device today (-scheduler tasks has no yield point between
// the closer check and the reading send), so this is a regression pin.
func TestStopScannerJoinTimeoutArmAbandons(t *testing.T) {
	r := &unstoppableReader{
		release: make(chan struct{}),
		entered: make(chan struct{}),
		// closeErr nil: Close reports success, the goroutine stays parked.
	}
	defer close(r.release)

	ctx := NewContext(newPlatform())
	_, stop := startScanner(ctx, r)

	select {
	case <-r.entered:
	case <-time.After(10 * time.Second):
		t.Fatal("INCONCLUSIVE: the scanner never entered Read, so nothing was stalled")
	}

	begin := time.Now()
	done := make(chan struct{})
	go func() { stop(); close(done) }()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("stopScanner never returned: the join bound did not fire (F-441)")
	}
	if e := time.Since(begin); e < scannerJoinTimeout {
		t.Fatalf("stop returned in %v -- before the join bound, so this test did "+
			"not exercise the timeout arm and cannot pin it", e)
	}
}
