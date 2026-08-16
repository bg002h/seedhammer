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
//
// ─── IT IS KIND-AWARE SINCE S5, AND IT HAD TO BECOME SO ──────────────────────
//
// "Every engraved mk1 is a record of the blob" is TRUE OF A GATHERED BUNDLE and
// FALSE OF A BUILT POLICY, and the difference is not cosmetic. Trace A engraves
// the payload's cards VERBATIM. Trace B engraves the operator's own key cards,
// which the device MINTS: each carries this policy's id stub (md's
// policy-id-fingerprint), and the payload's cards carry the placeholder stub
// cmd/buildpayloadcards writes. So the two sets are necessarily different
// strings, and applying Trace A's rule to Trace B's record would have failed
// seven honest plates.
//
// Rather than exempting the new kind — which would have left it anchored to
// nothing — the built-policy arm asserts the CONVERSE: no engraved mk1 may be a
// verbatim payload record, because a build that shipped a gathered card as the
// operator's own key plate would be engraving somebody else's card under this
// wallet's name.
//
// The kind comes from the inputs file beside the record, which is the one place
// it is stated. A record with no inputs file is a failure here, not a skip: the
// expectation is re-derived from that file by basename, so a record without one
// is a record nothing can re-check.
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
	matched, checked, minted, built := 0, 0, 0, 0
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
		base := strings.TrimSuffix(n, oracle.RecordSuffix)
		inf, err := oracle.LoadInputsFile(filepath.Join(dir, base+".inputs.json"))
		if err != nil {
			t.Errorf("%s: the record's inputs file is what states its expectation KIND, and it "+
				"does not load: %v", n, err)
			continue
		}
		if inf.Expect == nil {
			t.Errorf("%s: its inputs file carries no `expect` block, so nothing says whether its "+
				"mk1 plates are gathered cards or minted ones", n)
			continue
		}
		gathered := inf.Expect.Kind == oracle.KindCosignerCards
		if gathered {
			checked++
		} else {
			built++
		}
		for i, s := range g.Walk.Census.Strings {
			if !strings.HasPrefix(s, "mk1") {
				// md1 and ms1 are PRODUCED by a build, not supplied by the
				// payload, so they have no counterpart here. Logged rather
				// than skipped silently: a census that is all md1 would make
				// the match count below zero, and that must be visible.
				t.Logf("%s plate %d is not an mk1 (%.12s…) — not anchored by the payload", n, i, s)
				continue
			}
			if !gathered {
				// THE CONVERSE, for a BUILT policy: this plate must be the
				// device's OWN key card, carrying this policy's id, and therefore
				// must NOT be a verbatim record of the payload.
				if inPayload[s] {
					t.Errorf("%s plate %d is a built policy's key card and yet is a VERBATIM record "+
						"of %s — the build engraved a gathered card as the operator's own key:\n  %s",
						n, i, "cmd/emu/sysw_cards_payload.bin", s)
					continue
				}
				minted++
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
		t.Fatalf("no gathered-bundle gate record names the cosigner payload (%s), so nothing was anchored", cardsDigest)
	}
	if matched == 0 {
		t.Fatalf("%d record(s) name the cosigner payload and not one engraved mk1 matched a payload record — "+
			"this test passed by checking nothing", checked)
	}
	if built > 0 && minted == 0 {
		t.Fatalf("%d built-policy record(s) name the cosigner payload and not one engraved an mk1, "+
			"so the converse arm checked nothing", built)
	}
	t.Logf("anchored %d engraved mk1 string(s) across %d gathered record(s) to the payload's own chunks; "+
		"%d minted mk1 string(s) across %d built-policy record(s) are correctly NOT payload records",
		matched, checked, minted, built)
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
