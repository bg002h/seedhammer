package main

// UNTAGGED, for sysw_test_payload_host_test.go's reason: sysw_cards_payload.go
// is //go:build js, so nothing here can reference syswCardsPayload or
// syswCardsDigest as SYMBOLS. It reads the .bin and the .go SOURCE off disk.

import (
	"os"
	"strings"
	"testing"

	"seedhammer.com/mk"
	"seedhammer.com/sysw"
)

// TestSyswCardsPayloadMatchesItsDigest recomputes the digest with the
// firmware's own code and requires the pinned constant to equal it — and proves
// the blob is a container the FIRMWARE can open, not merely one `me` could
// write.
func TestSyswCardsPayloadMatchesItsDigest(t *testing.T) {
	blob, err := os.ReadFile("sysw_cards_payload.bin")
	if err != nil {
		t.Fatalf("reading the cosigner payload: %v", err)
	}
	p, err := sysw.Open(blob, "")
	if err != nil {
		t.Fatalf("the firmware cannot open its own cosigner payload: %v", err)
	}
	got := sysw.FormatHash(sysw.PublicDataHash(p.Public, false))

	src, err := os.ReadFile("sysw_cards_payload.go")
	if err != nil {
		t.Fatalf("reading the source that pins the digest: %v", err)
	}
	const marker = `const syswCardsDigest = "`
	i := strings.Index(string(src), marker)
	if i < 0 {
		t.Fatalf("syswCardsDigest is gone from sysw_cards_payload.go, so this test "+
			"is checking nothing; looked for %q", marker)
	}
	rest := string(src)[i+len(marker):]
	want := rest[:strings.Index(rest, `"`)]

	if got != want {
		t.Errorf("digest drift: the blob hashes to %q but syswCardsDigest pins %q.\n"+
			"Regenerate with `go run ./cmd/buildpayloadcards | me sysw pack …` and "+
			"update both, or the operator compares a value that is not this payload.",
			got, want)
	}
}

// TestSyswCardsPayloadCoversEveryStagesWalk pins the RECORD INVENTORY against
// the coverage list in sysw_cards_payload.go's doc comment.
//
// This test exists because the FIRST payload's gap — three records, no
// ClassMDMK — survived five review rounds and was found only when someone
// opened the blob. A payload that silently shrinks strands whichever stage's
// walk needed the record that went missing, and no other test would notice.
func TestSyswCardsPayloadCoversEveryStagesWalk(t *testing.T) {
	blob, err := os.ReadFile("sysw_cards_payload.bin")
	if err != nil {
		t.Fatalf("reading the cosigner payload: %v", err)
	}
	p, err := sysw.Open(blob, "")
	if err != nil {
		t.Fatalf("opening: %v", err)
	}

	// Every mk1 card in the payload, decoded through the fork's own decoder —
	// the chunk sets must reassemble, or the gather cannot use them.
	var mdmk []string
	seeds := 0
	for _, r := range p.Public {
		switch sysw.Classify(r) {
		case sysw.ClassMDMK:
			mdmk = append(mdmk, r)
		case sysw.ClassMnemonic:
			seeds++
		}
	}
	if seeds < 1 {
		t.Errorf("no ClassMnemonic record: S4's `both` slot has no seed to derive "+
			"from, and S5's derived slots have no master (got %d)", seeds)
	}

	// The coverage list, asserted as origins rather than as a count: a count
	// tells you a card vanished, an origin tells you WHICH STAGE just lost its
	// walk.
	// mk.Decode returns the `h` hardened notation, not apostrophes. Asserting
	// the apostrophe form failed here first — the expectation was wrong, not the
	// payload, which is why this comment exists rather than a normaliser.
	want := map[string]string{
		"m/48h/0h/0h/2h": "Trace A's cosigners, and S4's honest `both` case",
		"m/48h/0h/1h/2h": "S5 Trace B's second account — the multi-account shape",
	}
	// Cards are contiguous chunk runs; try every window up to 4 chunks wide.
	found := map[string]int{}
	for i := range mdmk {
		for j := i + 1; j <= len(mdmk) && j <= i+4; j++ {
			if c, err := mk.Decode(mdmk[i:j]); err == nil {
				found[c.Path]++
			}
		}
	}
	for origin, why := range want {
		if found[origin] == 0 {
			t.Errorf("no cosigner card at %s — that origin exists for: %s", origin, why)
		}
	}

	if len(mdmk) < 8 {
		t.Errorf("only %d md1/mk1 records; the inventory in sysw_cards_payload.go "+
			"documents 9 across four cards. A shrunk payload strands a walk.", len(mdmk))
	}
	t.Logf("%d md1/mk1 records, %d seed(s)", len(mdmk), seeds)
}
