package main

import (
	"io"
	"testing"
)

// nfc.go is untagged so this suite can exist at all: nfc_js.go cannot be tested
// on this host.
//
// The properties here are the ones gui/scan.go's contract rests on. It
// accumulates across Reads and parses when Read reports io.EOF, so EOF is what
// delimits a tag -- and every test below is really a statement about where the
// EOFs fall.

// readTag drains exactly one record, the way gui/scan.go does: read until the
// reader reports EOF, then treat what accumulated as one tag.
func readTag(t *testing.T, r io.Reader) string {
	t.Helper()
	var got []byte
	buf := make([]byte, 8*1024)
	for {
		n, err := r.Read(buf)
		got = append(got, buf[:n]...)
		if err == io.EOF {
			return string(got)
		}
		if err != nil {
			t.Fatalf("reading a tag: %v", err)
		}
	}
}

// TestTagsCrossTheReaderInOrder is the property that makes a multi-card walk
// possible, and the one the previous one-shot source could not provide.
//
// A bundle needs several cards through ONE reader: a screen fetches
// Platform.NFCReader() exactly once at entry (gui/bundle_flow.go:110), so every
// card of a gather arrives through that single reader or not at all. Measured
// in a browser against the old source: an md1 presented before entering Engrave
// Bundle gave "md1 descriptors: 1", and an mk1 presented while inside left it
// at 1 forever.
func TestTagsCrossTheReaderInOrder(t *testing.T) {
	var n nfcSource
	n.set("md1qqq")
	n.set("mk1qqq")

	r := n.reader()
	if r == nil {
		t.Fatal("a source with tags queued must hand out a reader")
	}
	if got := readTag(t, r); got != "md1qqq" {
		t.Errorf("first tag read as %q, want md1qqq", got)
	}
	if got := readTag(t, r); got != "mk1qqq" {
		t.Errorf("second tag read as %q, want mk1qqq -- the SAME reader must deliver every "+
			"card of a gather, because the flow only ever asks for one", got)
	}
}

// TestATagCrossesOnce keeps the property the one-shot design existed for. A
// source that replayed a tag would let a polling flow see a card the operator
// presented once as though they had presented it twice -- and a bundle
// deduplicates by chunk-set id, so the symptom would be a silently doubled
// census rather than an obvious repeat.
func TestATagCrossesOnce(t *testing.T) {
	var n nfcSource
	n.set("md1qqq")
	r := n.reader()

	if got := readTag(t, r); got != "md1qqq" {
		t.Fatalf("read %q, want the presented record", got)
	}
	if got := readTag(t, r); got != "" {
		t.Errorf("the same tag came past a second time as %q -- a tag crosses the reader "+
			"once, which is what a physical one does", got)
	}
}

// TestAnIdleReaderIsNotAnAbsentReader is the distinction the rewrite turns on.
//
// (0, io.EOF) means "no tag on the reader right now", which gui/nfc_scan.go
// backs off on and re-polls. nil from reader() means "this machine has no
// reader", which flows answer by offering Back-only where a scan row would go.
// Conflating them is what made a tag presented after flow entry unreachable
// forever: the flow had already taken its nil.
func TestAnIdleReaderIsNotAnAbsentReader(t *testing.T) {
	var n nfcSource

	r := n.reader()
	if r == nil {
		t.Fatal("an attached reader with no tag queued must still be a reader -- a flow that " +
			"takes nil here can never see a tag presented later")
	}
	got, err := r.Read(make([]byte, 64))
	if got != 0 || err != io.EOF {
		t.Errorf("an idle reader returned (%d, %v), want (0, EOF) -- gui/scan.go reads that "+
			"as idle, and anything else is a phantom tag", got, err)
	}

	// And the absent-reader state is still reachable, because gui treats it
	// differently and a walk must be able to reach a machine that has no reader.
	n.detach(true)
	if n.reader() != nil {
		t.Error("a detached source must report no reader at all")
	}
	n.detach(false)
	if n.reader() == nil {
		t.Error("re-attaching must restore the reader")
	}
}

// TestTheSourceSurvivesTheFlowThatUsedIt pins the second half of the walk bug.
//
// startScanner's stop function Closes the reader when its screen exits
// (gui/nfc_scan.go:102), and EVERY screen exits. If Close killed the source,
// the emulator's NFC path would be dead after the first flow -- so a walk could
// scan a card in Engrave Bundle and then never scan again anywhere, which is
// indistinguishable from the machine having no reader.
func TestTheSourceSurvivesTheFlowThatUsedIt(t *testing.T) {
	var n nfcSource
	n.set("first")
	r := n.reader()
	if got := readTag(t, r); got != "first" {
		t.Fatalf("read %q, want first", got)
	}
	if err := r.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// A second flow entry, as a walk that leaves one program and enters another.
	n.set("second")
	r2 := n.reader()
	if r2 == nil {
		t.Fatal("the source did not survive its first flow -- NFC is dead for the rest of the walk")
	}
	if got := readTag(t, r2); got != "second" {
		t.Errorf("the second flow read %q, want second", got)
	}
}

// TestCloseDropsAHalfReadTag: a tag interrupted mid-read did not finish
// crossing the reader, so its remaining bytes must not be delivered to the NEXT
// flow as though they were a tag of their own. That would hand a screen a
// truncated record, which is the shape that decodes to a DIFFERENT valid string
// rather than to an obvious error.
func TestCloseDropsAHalfReadTag(t *testing.T) {
	var n nfcSource
	n.set("md1qqqqqqqqqq")
	r := n.reader()

	// One short read, leaving the record half-crossed.
	small := make([]byte, 4)
	if got, err := r.Read(small); got != 4 || err != nil {
		t.Fatalf("short read returned (%d, %v), want (4, nil) -- a partially delivered record "+
			"must report progress, not EOF", got, err)
	}
	r.Close()

	if got := readTag(t, n.reader()); got != "" {
		t.Errorf("the remains of an interrupted tag were delivered as %q -- a truncated record "+
			"can decode to a different valid string, not to an error", got)
	}
}

// TestClearRemovesQueuedTags -- window.shNFC.clear() calls set(""), and a walk
// resetting between stages must not inherit the previous stage's cards.
func TestClearRemovesQueuedTags(t *testing.T) {
	var n nfcSource
	n.set("md1qqq")
	n.set("mk1qqq")
	n.set("")

	if got := readTag(t, n.reader()); got != "" {
		t.Errorf("clear left %q queued", got)
	}
}
