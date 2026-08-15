package main

// UNTAGGED, for sysw_cards_payload_host_test.go's reason: this reads the .bin
// and the .go SOURCE off disk rather than referencing the //go:build js
// symbols.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"seedhammer.com/oracle"
	"seedhammer.com/sysw"
)

// TestGateRecordStringsAreRecordsOfTheCardsPayload is what stops a gate record
// from being self-referential.
//
// oracle.VerifyRecord proves a record and the walk file beside it agree, which
// is a real check and a CLOSED one: both halves are produced by the same run,
// so a consistent pair says nothing about whether the walk engraved the right
// thing. This test opens the other end. Trace A's engraved strings are the
// cosigner payload's OWN chunks, byte for byte — walk_trace_a.js's header says
// so and that is the property that makes the walk a gate rather than a demo —
// so every mk1 in a record's census must be a record of the blob the record
// names.
//
// It lives here rather than in package oracle because this is where the payload
// can be opened with the firmware's own reader.
func TestGateRecordStringsAreRecordsOfTheCardsPayload(t *testing.T) {
	dir := filepath.Join("..", "..", "oracle", oracle.GateRecordsDir)
	names, err := oracle.Records(dir)
	if err != nil {
		t.Fatalf("no gate records directory at %s: %v", dir, err)
	}
	if len(names) == 0 {
		t.Fatalf("%s holds no gate record, so this anchor checks nothing; "+
			"see oracle.TestS0GateHasARecord for how to produce one", dir)
	}

	blob, err := os.ReadFile("sysw_cards_payload.bin")
	if err != nil {
		t.Fatal(err)
	}
	p, err := sysw.Open(blob, "")
	if err != nil {
		t.Fatalf("opening the cosigner payload: %v", err)
	}
	inPayload := map[string]bool{}
	for _, r := range p.Public {
		if sysw.Classify(r) == sysw.ClassMDMK {
			inPayload[r] = true
		}
	}
	if len(inPayload) == 0 {
		t.Fatal("the cosigner payload holds no ClassMDMK record, so this test cannot anchor anything")
	}

	cardsDigest := pinnedCardsDigest(t)
	matched, checked := 0, 0
	for _, n := range names {
		g, err := oracle.LoadRecord(filepath.Join(dir, n))
		if err != nil {
			t.Errorf("%s: %v", n, err)
			continue
		}
		if !sameDigest(g.Payload.Digest, cardsDigest) {
			t.Logf("%s ran against payload %q, not the cosigner blob — not anchored here", n, g.Payload.Digest)
			continue
		}
		checked++
		for i, s := range g.Walk.Census.Strings {
			if !strings.HasPrefix(s, "mk1") {
				// md1 and ms1 are PRODUCED by a build, not supplied by the
				// payload, so they have no counterpart here. Logged rather
				// than skipped silently: a census that is all md1 would make
				// the match count below zero, and that must be visible.
				t.Logf("%s plate %d is not an mk1 (%.12s…) — not anchored by the payload", n, i, s)
				continue
			}
			if !inPayload[s] {
				t.Errorf("%s plate %d engraved an mk1 that is NOT a record of %s:\n  %s",
					n, i, "cmd/emu/sysw_cards_payload.bin", s)
				continue
			}
			matched++
		}
	}
	if checked == 0 {
		t.Fatalf("no gate record names the cosigner payload (%s), so nothing was anchored", cardsDigest)
	}
	if matched == 0 {
		t.Fatalf("%d record(s) name the cosigner payload and not one engraved mk1 matched a payload record — "+
			"this test passed by checking nothing", checked)
	}
	t.Logf("anchored %d engraved mk1 string(s) across %d record(s) to the payload's own chunks", matched, checked)
}

// pinnedCardsDigest reads syswCardsDigest out of the SOURCE, the way
// TestSyswCardsPayloadMatchesItsDigest does — the symbol is //go:build js and
// cannot be referenced from here.
func pinnedCardsDigest(t *testing.T) string {
	t.Helper()
	src, err := os.ReadFile("sysw_cards_payload.go")
	if err != nil {
		t.Fatal(err)
	}
	const marker = `const syswCardsDigest = "`
	i := strings.Index(string(src), marker)
	if i < 0 {
		t.Fatalf("syswCardsDigest is gone from sysw_cards_payload.go; looked for %q", marker)
	}
	rest := string(src)[i+len(marker):]
	return rest[:strings.Index(rest, `"`)]
}

// sameDigest compares digests written grouped ("2527 1e58 …", as the device
// shows them) or bare, which is how the walk reads one off the screen.
func sameDigest(a, b string) bool {
	strip := func(s string) string { return strings.ToLower(strings.Join(strings.Fields(s), "")) }
	return strip(a) != "" && strip(a) == strip(b)
}
