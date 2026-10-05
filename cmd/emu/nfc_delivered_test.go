package main

// UNTAGGED, like nfc.go itself: the counting half runs on the host.

// WHY THIS EXISTS (Refugium plan F7 §4.2, R0 round 2 I-1).
//
// presentedCount answers "what did the PAGE offer", which is the wrong
// question for the Refugium build's NFC-off gate: a walk that presents a tag
// at every screen raises it in every build, whether or not anything reads.
// deliveredCount answers "what did the MACHINE read": it moves only when a
// record is taken off the queue by Read. The Refugium walk asserts
// presented() > 0 and delivered() == 0; the default walk is its positive
// control, delivered() > 0.

import (
	"io"
	"testing"
)

func TestDeliveredCountsOnlyRecordsTheMachineRead(t *testing.T) {
	var n nfcSource
	n.set("one")
	n.set("two")
	if got := n.delivered(); got != 0 {
		t.Fatalf("delivered() = %d before any Read, want 0: presenting is not delivering", got)
	}
	if got := n.presented(); got != 2 {
		t.Fatalf("presented() = %d, want 2", got)
	}
	r := n.reader()
	if got := readTag(t, r); got != "one" {
		t.Fatalf("first tag read as %q", got)
	}
	if got := n.delivered(); got != 1 {
		t.Fatalf("delivered() = %d after one tag, want 1", got)
	}
	if got := readTag(t, r); got != "two" {
		t.Fatalf("second tag read as %q", got)
	}
	if got := n.delivered(); got != 2 {
		t.Fatalf("delivered() = %d after two tags, want 2", got)
	}
	// An idle read delivers nothing.
	if c, err := r.Read(make([]byte, 16)); c != 0 || err != io.EOF {
		t.Fatalf("idle Read = (%d, %v), want (0, EOF)", c, err)
	}
	if got := n.delivered(); got != 2 {
		t.Fatalf("delivered() = %d after an idle Read, want 2", got)
	}
}

// A record big enough to span several Reads is ONE delivery, counted when it
// leaves the queue -- so a tag the scanner started reading counts even if a
// Close cuts it off half way. That errs towards "read", which is the safe
// direction for a gate asserting zero.
func TestDeliveredCountsAMultiReadRecordOnce(t *testing.T) {
	var n nfcSource
	n.set("abcdefgh")
	r := n.reader()
	if c, err := r.Read(make([]byte, 3)); c != 3 || err != nil {
		t.Fatalf("partial Read = (%d, %v), want (3, nil)", c, err)
	}
	if got := n.delivered(); got != 1 {
		t.Fatalf("delivered() = %d after a partial read, want 1", got)
	}
	_ = r.Close()
	if got := n.delivered(); got != 1 {
		t.Fatalf("delivered() = %d after Close, want 1 (no reset, no un-delivery)", got)
	}
}

// clear() drops the queue but never un-delivers, for presentedCount's reason:
// a counter a driver can zero just before asserting is a gate that always
// passes.
func TestClearDoesNotUnDeliver(t *testing.T) {
	var n nfcSource
	n.set("x")
	_ = readTag(t, n.reader())
	n.set("")
	if got := n.delivered(); got != 1 {
		t.Fatalf("delivered() = %d after clear, want 1", got)
	}
}

// A detached source hands out no reader, so nothing can be delivered however
// many tags the page presents.
func TestADetachedSourceDeliversNothing(t *testing.T) {
	var n nfcSource
	n.detach(true)
	n.set("x")
	if r := n.reader(); r != nil {
		t.Fatal("a detached source handed out a reader")
	}
	if got := n.delivered(); got != 0 {
		t.Fatalf("delivered() = %d, want 0", got)
	}
}
