package main

// UNTAGGED, like nfc.go itself: the counting half runs on the host.

// WHY THIS EXISTS (F-174).
//
// A cosigner gather that completes over the emulated reader is GREEN whether or
// not the payload-supplied-cards feature exists — the operator "scanned" the
// cards, so the gather finished, so the walk passed. That makes every
// stage-gate walk from S1 on able to report success for the wrong reason.
//
// The SH2 HAS a reader — a soldered ST25R3916, defeatable only physically, with
// a knife. What phase 1 changes is SCOPE: the payload and the keyboard are the
// primary data entries, and NFC is a later pass (operator, 2026-08-15).
//
// So a stage-gate walk must be able to assert that NOTHING crossed the reader —
// and the fact that the reader WORKS is what makes that assertion load-bearing
// rather than decorative.
//
// The counter is CUMULATIVE for the session and has no reset, deliberately —
// the same discipline the engraved census keeps (engraved.go). A resettable
// counter is one a driver can zero just before asserting, which is exactly the
// shape of a gate that always passes. Reload the page to start a fresh walk.

import "testing"

func TestPresentedCountsEveryRecordAcrossTheReader(t *testing.T) {
	n := &nfcSource{}
	if got := n.presented(); got != 0 {
		t.Fatalf("a fresh source has presented %d, want 0", got)
	}
	n.set("mk1qexample0")
	n.set("mk1qexample1")
	if got := n.presented(); got != 2 {
		t.Errorf("after two presentations presented() = %d, want 2", got)
	}
}

// clear() drops the QUEUE; it does not un-present what already crossed.
//
// This is the assertion's whole integrity. If clear() reset the count, a driver
// could present a card, clear, and assert zero — reporting "no tag touched this
// reader" about a run in which one did.
func TestClearDoesNotUnPresent(t *testing.T) {
	n := &nfcSource{}
	n.set("mk1qexample0")
	n.set("") // shNFC.clear()
	if got := n.presented(); got != 1 {
		t.Errorf("presented() = %d after a clear, want 1 — clearing the queue must "+
			"not erase the record that a tag crossed the reader", got)
	}
	// And the queue really was dropped, so this is a clear and not a no-op.
	buf := make([]byte, 64)
	if c, err := n.Read(buf); c != 0 {
		t.Errorf("Read returned %d byte(s) after clear; the queue should be empty (err=%v)", c, err)
	}
}

// Reading a record out does not change the count either: "presented" is about
// what the PAGE offered the machine, which is what F-174 asks about.
func TestReadingDoesNotChangeTheCount(t *testing.T) {
	n := &nfcSource{}
	n.set("mk1qexample0")
	buf := make([]byte, 64)
	for i := 0; i < 4; i++ {
		n.Read(buf)
	}
	if got := n.presented(); got != 1 {
		t.Errorf("presented() = %d after draining one record, want 1", got)
	}
}

// A gather driven entirely from the payload must leave this at zero — the
// property a stage gate asserts.
func TestAPayloadOnlyGatherPresentsNothing(t *testing.T) {
	n := &nfcSource{}
	// No set() calls: the cards arrive through syswOffer/bundleGatherFlow.
	if got := n.presented(); got != 0 {
		t.Errorf("presented() = %d with no presentation at all, want 0", got)
	}
	// detach/attach are reader-topology changes, not presentations.
	n.detach(true)
	n.detach(false)
	if got := n.presented(); got != 0 {
		t.Errorf("presented() = %d after detach/attach, want 0", got)
	}
}
