package seal

import (
	"testing"

	"seedhammer.com/codex32"
)

// F-77. The encrypted section's md1/mk1 records must carry the same card
// labels the public section's do, or §10.2.2's secret-session plate list cannot
// name a plate on any multisig payload.
//
// Vector C is the discriminating fixture on the small side: its secret set is
// ms1 x1 / mk1 x2 / md1 x3, so it exercises the SUBSET path (an ms1 sits among
// the cards) as well as the labelling.
func TestEncryptedSectionCardsAreLabelled(t *testing.T) {
	v := vectorNamed(t, "C")
	out, err := AdmitSection(bs(v.Secret), SectionEncrypted)
	if err != nil {
		t.Fatalf("admit vector C's secret section: %v", err)
	}
	if len(out) != 6 {
		t.Fatalf("vector C's secret section is %d records, want 6", len(out))
	}
	var cards, secrets int
	for i, r := range out {
		switch r.Class {
		case ClassMDMK:
			cards++
			if r.HRP != 'd' && r.HRP != 'k' {
				t.Errorf("record %d is a card with HRP %q, want 'd' or 'k'", i, r.HRP)
			}
			if r.PlateTotal < 1 || r.PlateIndex < 1 || r.PlateIndex > r.PlateTotal {
				t.Errorf("record %d has plate %d/%d, which is not a 1-based index",
					i, r.PlateIndex, r.PlateTotal)
			}
			if r.CardTotal < 1 || r.CardIndex < 1 || r.CardIndex > r.CardTotal {
				t.Errorf("record %d has card %d/%d, which is not a 1-based index",
					i, r.CardIndex, r.CardTotal)
			}
		case ClassCodex32Secret:
			secrets++
			// An ms1 is not a card and must keep its zero label. A non-zero HRP
			// here would mean the subset filter leaked.
			if r.HRP != 0 || r.CardIndex != 0 || r.PlateIndex != 0 {
				t.Errorf("record %d is an ms1 but carries card labels: %+v", i, r)
			}
		}
	}
	if cards != 5 || secrets != 1 {
		t.Fatalf("vector C's secret section classified as %d cards + %d secrets, want 5 + 1",
			cards, secrets)
	}
}

// Vector F is the one that discriminates plural from singular: 15 secret
// records, ms1 x3 / mk1 x6 / md1 x6, with the six mk1 records spanning THREE
// cosigner cards. A flat mk1 1/6..6/6 conflates them, which is §6.4's
// incomplete-backup-believed-complete hazard wearing a label.
func TestEncryptedMultisigCardsAreDistinguishable(t *testing.T) {
	v := vectorNamed(t, "F")
	out, err := AdmitSection(bs(v.Secret), SectionEncrypted)
	if err != nil {
		t.Fatalf("admit vector F's secret section: %v", err)
	}
	if len(out) != 15 {
		t.Fatalf("vector F's secret section is %d records, want 15", len(out))
	}
	seen := make(map[[2]int]bool)
	var mk int
	for _, r := range out {
		if r.Class != ClassMDMK || r.HRP != 'k' {
			continue
		}
		mk++
		if r.CardTotal != 3 {
			t.Fatalf("mk1 record reports %d cards of its HRP, want 3", r.CardTotal)
		}
		key := [2]int{r.CardIndex, r.PlateIndex}
		if seen[key] {
			t.Fatalf("two mk1 records share card %d plate %d — the three cosigner "+
				"cards have been conflated", r.CardIndex, r.PlateIndex)
		}
		seen[key] = true
	}
	if mk != 6 {
		t.Fatalf("vector F carries %d mk1 records, want 6", mk)
	}
}

// A record the grouping cannot read must NOT reject the payload: §10.2.1
// requires the decode for the public section only, and turning a label failure
// into a rejection would change ADMISSION, which lands in Rust first.
//
// THE FIXTURE MUST REACH groupRecords, and an earlier draft of this test did
// not (R0 round 0, I4): it used a mnemonic-only section, so labelEncryptedCards
// returned at `len(strs) == 0` and the mutant "return the grouping error instead
// of discarding it" survived the entire suite.
//
// codex32.AssembleMD1(nil) is the fixture that does reach it. Measured against
// the real packages, not assumed:
//
//	assembled          = "md1t7yjcvgk6xetg" (16 bytes)
//	codex32.ValidMD    = true          -> Classify = ClassMDMK, so it is IN the subset
//	md.ParseChunkHeader = "md: bit stream truncated"
//	cardKey            = ErrUndecodableCardSet: record 0: md: bit stream truncated
//
// Note the seal package's existing smuggledMD1 fixture CANNOT serve here:
// md.ParseChunkHeader SUCCEEDS on it ({Version:0 Chunked:false ChunkSetID:0},
// err=nil), so cardKey returns cleanly and the error path is never taken.
func TestUnreadableEncryptedCardDoesNotReject(t *testing.T) {
	v := vectorNamed(t, "C")
	var realMD1 string
	for _, r := range v.Secret {
		if codex32.ValidMD(r) {
			realMD1 = r
			break
		}
	}
	if realMD1 == "" {
		t.Fatal("vector C carries no md1 record; the premise of this test is broken")
	}
	broken := codex32.AssembleMD1(make([]byte, 0))
	if Classify([]byte(broken)) != ClassMDMK {
		t.Fatalf("the fixture classifies as %v, not a card, so it never reaches the grouping",
			Classify([]byte(broken)))
	}
	if _, err := cardKey(broken, 0); err == nil {
		t.Fatal("the fixture's card key resolves; it cannot exercise the failure path")
	}

	out, err := AdmitSection([][]byte{[]byte(broken), []byte(realMD1)}, SectionEncrypted)
	// (a) a grouping failure must NOT reject.
	if err != nil {
		t.Fatalf("a label failure rejected the payload: %v — §10.2.1 requires the "+
			"decode for the public section only, and this is an ADMISSION change", err)
	}
	// (b) every record is still admitted.
	if len(out) != 2 {
		t.Fatalf("admitted %d records, want 2", len(out))
	}
	// (c) and the whole subset falls back to zero labels rather than to wrong
	// ones — groupRecords is all-or-nothing, so one unreadable card costs the
	// labels of every card beside it. gui's plateLabel renders that as
	// "record N" (gui/unlock_platelist.go:50-55), never as a mislabelled md1.
	for i, r := range out {
		if r.HRP != 0 || r.CardIndex != 0 || r.PlateIndex != 0 {
			t.Errorf("record %d carries a label (%c card %d/%d plate %d/%d) although the "+
				"grouping failed", i, r.HRP, r.CardIndex, r.CardTotal, r.PlateIndex, r.PlateTotal)
		}
	}
}
