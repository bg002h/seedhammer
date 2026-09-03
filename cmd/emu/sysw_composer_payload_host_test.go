package main

// UNTAGGED, for sysw_test_payload_host_test.go's reason: sysw_composer_payload.go
// is //go:build js, so nothing here can reference syswComposerPayload or
// syswComposerDigest as SYMBOLS. It reads the .bin and the .go SOURCE off disk.
//
// The THIRD assertion the plan asks of this blob -- that `me sysw show`'s
// digest line equals the pin -- needs the `me` CLI, which no CI runner has, so
// it lives next door in sysw_composer_payload_live_test.go behind
// //go:build oraclelive, where ABSENCE IS FATAL rather than a skip. That is
// this tree's established split (sysw/vendored_vectors_live_test.go,
// gui/chain_fixture_live_test.go): what needs no toolchain runs everywhere with
// no skip path, and the cross-implementation audit is a maintainer's command.

import (
	"os"
	"strings"
	"testing"

	"seedhammer.com/sysw"
)

// TestSyswComposerPayloadMatchesItsDigest recomputes the digest with the
// firmware's own code and requires the pinned constant to equal it -- and
// proves the blob is a container the FIRMWARE can open, not merely one `me`
// could write.
func TestSyswComposerPayloadMatchesItsDigest(t *testing.T) {
	blob, err := os.ReadFile("sysw_composer_payload.bin")
	if err != nil {
		t.Fatalf("reading the composer payload: %v", err)
	}
	p, err := sysw.Open(blob, "")
	if err != nil {
		t.Fatalf("the firmware cannot open its own composer payload: %v", err)
	}
	got := sysw.FormatHash(sysw.PublicDataHash(p.Public, false))

	want := composerDigestPin(t)
	if got != want {
		t.Errorf("digest drift: the blob hashes to %q but syswComposerDigest pins %q.\n"+
			"Regenerate with `go run ./cmd/buildpayloadcomposer | me sysw pack "+
			"--no-passphrase --out cmd/emu/sysw_composer_payload.bin` and update both, "+
			"or the operator compares a value that is not this payload.",
			got, want)
	}
}

// composerDigestPin reads the pinned digest out of the //go:build js source,
// which is the only way an untagged test can see it. Shared with the
// oraclelive audit next door so the two cannot pin different literals.
func composerDigestPin(t *testing.T) string {
	t.Helper()
	src, err := os.ReadFile("sysw_composer_payload.go")
	if err != nil {
		t.Fatalf("reading the source that pins the digest: %v", err)
	}
	const marker = `const syswComposerDigest = "`
	i := strings.Index(string(src), marker)
	if i < 0 {
		t.Fatalf("syswComposerDigest is gone from sysw_composer_payload.go, so this test "+
			"is checking nothing; looked for %q", marker)
	}
	rest := string(src)[i+len(marker):]
	j := strings.Index(rest, `"`)
	if j < 0 {
		t.Fatalf("the syswComposerDigest literal is unterminated in sysw_composer_payload.go")
	}
	return rest[:j]
}

// TestSyswComposerPayloadCarriesTheComposerClasses pins the RECORD INVENTORY,
// IN ORDER, against the list in sysw_composer_payload.go's doc comment.
//
// ORDER IS PART OF THE CLAIM here, not decoration. The composer's seating
// picker walks the key sources in payload order, so a blob holding the right
// FIVE records in a different order sends every itinerary in the S4 plan to a
// different row -- and a set-shaped assertion would call that fine. The first
// payload's gap (three records, zero ClassMDMK) survived five review rounds
// because nobody opened the blob; a count would have caught that one and would
// not catch this one.
func TestSyswComposerPayloadCarriesTheComposerClasses(t *testing.T) {
	blob, err := os.ReadFile("sysw_composer_payload.bin")
	if err != nil {
		t.Fatalf("reading the composer payload: %v", err)
	}
	p, err := sysw.Open(blob, "")
	if err != nil {
		t.Fatalf("opening: %v", err)
	}

	// Named rather than numbered, so a failure says WHICH screen just lost its
	// input instead of "index 2".
	want := []struct {
		class sysw.Class
		why   string
	}{
		{sysw.ClassKey, "slot @0's key source: master A at m/48'/0'/0'/2'"},
		{sysw.ClassKey, "slot @1's key source: master A at m/48'/0'/1'/2' -- the second account, which is what fires the §8g same-seed warning"},
		{sysw.ClassHash, "the hash-lock picker's `hash 1  abababab..abababab` row, and the §8i preimage modal on the way through"},
		{sysw.ClassNow, "the pack-time bound an absolute time lock's echo draws from"},
		{sysw.ClassMnemonic, "master B: the seed slot @2 is seated from, and the second master the keyed policy spans"},
	}

	if len(p.Public) != len(want) {
		t.Fatalf("the composer payload holds %d record(s); the inventory in "+
			"sysw_composer_payload.go documents %d. A payload that silently grew or "+
			"shrank moves every row of the S4 itineraries.", len(p.Public), len(want))
	}
	for i, w := range want {
		got := sysw.Classify(p.Public[i])
		if got != w.class {
			t.Errorf("record %d classifies as %v, want %v -- that record exists for: %s",
				i, got, w.class, w.why)
		}
	}

	// The composer classes are the reason this blob exists; say so if they are
	// gone, rather than only reporting a positional mismatch.
	counts := map[sysw.Class]int{}
	for _, r := range p.Public {
		counts[sysw.Classify(r)]++
	}
	if counts[sysw.ClassKey] != 2 || counts[sysw.ClassHash] != 1 ||
		counts[sysw.ClassNow] != 1 || counts[sysw.ClassMnemonic] != 1 {
		t.Errorf("inventory: %d ClassKey, %d ClassHash, %d ClassNow, %d ClassMnemonic; "+
			"want 2, 1, 1, 1. Neither of the other two blobs carries a composer class at "+
			"all, so this one losing them strands the C8 journey entirely.",
			counts[sysw.ClassKey], counts[sysw.ClassHash],
			counts[sysw.ClassNow], counts[sysw.ClassMnemonic])
	}
	t.Logf("%d records: %d key, %d hash, %d now, %d mnemonic", len(p.Public),
		counts[sysw.ClassKey], counts[sysw.ClassHash],
		counts[sysw.ClassNow], counts[sysw.ClassMnemonic])
}
