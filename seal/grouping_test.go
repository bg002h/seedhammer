package seal

import (
	"errors"
	"testing"
)

// §6.3's card grouping, PUBLISHED on AdmittedRecord so Phase B's plate list can
// label each entry without re-deriving anything.
//
// Why this exists at all: Classification has exactly ONE value covering both
// formats, ClassMDMK, returned identically for ValidMD and ValidMK. A plate list
// built from Class alone cannot even print "mk1" versus "md1", let alone
// §10.2.2's `mk1 1/2` / `md1 2/3`. seal already knows -- groupCards keys on
// (HRP, chunk_set_id) -- so it publishes what it knows rather than inviting a
// second classifier into the UI.
//
// Every tuple below is written out LITERALLY. Deriving the expected side from
// the same grouping the code computes would pin nothing.

type wantGrouping struct {
	hrp        byte
	cardIdx    int
	cardTotal  int
	plateIdx   int
	plateTotal int
}

func checkGrouping(t *testing.T, name string, records []string, want []wantGrouping) {
	t.Helper()
	got, err := AdmitSection(bs(records), SectionPublic)
	if err != nil {
		t.Fatalf("vector %s: public section must be admitted: %v", name, err)
	}
	if len(got) != len(want) {
		t.Fatalf("vector %s: admitted %d records, want %d", name, len(got), len(want))
	}
	for i, w := range want {
		g := got[i]
		if g.HRP != w.hrp || g.CardIndex != w.cardIdx || g.CardTotal != w.cardTotal ||
			g.PlateIndex != w.plateIdx || g.PlateTotal != w.plateTotal {
			t.Errorf("vector %s record %d: got %c card %d/%d plate %d/%d, want %c card %d/%d plate %d/%d",
				name, i,
				g.HRP, g.CardIndex, g.CardTotal, g.PlateIndex, g.PlateTotal,
				w.hrp, w.cardIdx, w.cardTotal, w.plateIdx, w.plateTotal)
		}
	}
}

// Vector G's public section is FOUR cards: three mk1 cards of two chunks each
// -- one per cosigner of a 2-of-3 -- plus one md1 chunked six ways. It is the
// only vector that can carry this: D and E hold one card per HRP, so a grouping
// that indexed cards across the whole section rather than within an HRP would
// pass both.
//
// The card index is WITHIN an HRP, and that is the point. A flat mk1 1/6..6/6
// silently conflates three distinct cosigners, which is §6.4's
// incomplete-backup-believed-complete hazard wearing a label.
func TestPublicRecordsCarryTheirCardGrouping(t *testing.T) {
	g := vectorNamed(t, "G")
	checkGrouping(t, "G", g.Public, []wantGrouping{
		{'k', 1, 3, 1, 2},
		{'k', 1, 3, 2, 2},
		{'k', 2, 3, 1, 2},
		{'k', 2, 3, 2, 2},
		{'k', 3, 3, 1, 2},
		{'k', 3, 3, 2, 2},
		{'d', 1, 1, 1, 6},
		{'d', 1, 1, 2, 6},
		{'d', 1, 1, 3, 6},
		{'d', 1, 1, 4, 6},
		{'d', 1, 1, 5, 6},
		{'d', 1, 1, 6, 6},
	})
}

// The single-sig shape, which is what §10.2.2's own `mk1 1/2` / `md1 2/3`
// examples are: exactly one card of each HRP, so the card index is 1/1 and the
// label collapses to the plate index alone.
func TestSingleCardPerHRPGrouping(t *testing.T) {
	for _, name := range []string{"D", "E"} {
		v := vectorNamed(t, name)
		checkGrouping(t, name, v.Public, []wantGrouping{
			{'k', 1, 1, 1, 2},
			{'k', 1, 1, 2, 2},
			{'d', 1, 1, 1, 3},
			{'d', 1, 1, 2, 3},
			{'d', 1, 1, 3, 3},
		})
	}
}

// The encrypted section carries ITS grouping -- and the reason it once did not
// was never that secrets aren't cards. SPEC §6.3 is explicit that the encrypted
// section may carry ms1, mk1, md1 or a bare mnemonic, permitted() admits
// ClassMDMK unconditionally, and vector F's secret set is ms1 x3 / mk1 x6 /
// md1 x6: TWELVE OF FIFTEEN SECRET RECORDS ARE CARDS.
//
// Until F-77 they were zero for one reason only: pass 3, the sole place a
// grouping is computed, ran for SectionPublic alone. This test USED to pin that
// gap as a measured fact rather than a recollection; labelEncryptedCards
// (label_encrypted.go) closes it, so the same fixture now pins the closure.
// The inversion is deliberate: the assertion encoded a documented DEFICIENCY,
// named with the follow-up number that closes it, not an invariant. The
// ordering invariant beside it -- grouping must run AFTER the allow-list --
// is untouched, and TestGroupingRunsAfterTheAllowList still asserts it.
//
// The labelling is LABEL-ONLY: no decode runs over the encrypted section and no
// grouping failure rejects a payload (TestUnreadableEncryptedCardDoesNotReject).
func TestEncryptedRecordsCarryTheirGrouping(t *testing.T) {
	f := vectorNamed(t, "F")
	if len(f.Secret) != 15 {
		t.Fatalf("vector F's secret section must be 15 records, got %d", len(f.Secret))
	}
	got, err := AdmitSection(bs(f.Secret), SectionEncrypted)
	if err != nil {
		t.Fatalf("vector F's secret section must be admitted: %v", err)
	}
	cards := 0
	for i, r := range got {
		if r.Class == ClassMDMK {
			cards++
			// A 1-based card/plate identity, not merely a non-zero one: an
			// off-by-one or a 0-based index would name the wrong plate in
			// §10.2.2's list.
			if r.HRP != 'd' && r.HRP != 'k' {
				t.Errorf("secret record %d is a card with HRP %q, want 'd' or 'k'", i, r.HRP)
			}
			if r.CardTotal < 1 || r.CardIndex < 1 || r.CardIndex > r.CardTotal {
				t.Errorf("secret record %d has card %d/%d, which is not a 1-based index",
					i, r.CardIndex, r.CardTotal)
			}
			if r.PlateTotal < 1 || r.PlateIndex < 1 || r.PlateIndex > r.PlateTotal {
				t.Errorf("secret record %d has plate %d/%d, which is not a 1-based index",
					i, r.PlateIndex, r.PlateTotal)
			}
			continue
		}
		// Everything that is NOT a card keeps its zero label -- vector F's three
		// ms1 records among them. A label here would mean the ClassMDMK subset
		// filter leaked, and cardKey fails closed on an ms1.
		if r.HRP != 0 || r.CardIndex != 0 || r.CardTotal != 0 || r.PlateIndex != 0 || r.PlateTotal != 0 {
			t.Errorf("secret record %d is a %s yet carries a grouping (%c card %d/%d plate %d/%d)",
				i, r.Class, r.HRP, r.CardIndex, r.CardTotal, r.PlateIndex, r.PlateTotal)
		}
	}
	// Without this the test would pass just as green on a payload whose secret
	// section held no cards at all, and would prove nothing about §6.3.
	if cards != 12 {
		t.Fatalf("vector F's secret section holds %d md1/mk1 cards, want 12 — the premise of this test is broken", cards)
	}
}

// The grouping must be computed AFTER the allow-list, never before. cardKey
// fails closed with ErrUndecodableCardSet for anything that is not an md1/mk1
// card, so grouping first would send a BIP-39 mnemonic in the public section to
// cardKey before permitted() ever saw it -- changing the sentinel
// TestPublicSectionRefusesASecret asserts on, and inviting a "fix" to that test
// rather than to the ordering.
//
// This is the ordering constraint stated as a test rather than as a comment.
func TestGroupingRunsAfterTheAllowList(t *testing.T) {
	d := vectorNamed(t, "D")
	mnemonic := vectorNamed(t, "A").Secret[0]
	// A public section whose FIRST record is a secret: if grouping ran first it
	// would reach cardKey before the allow-list.
	recs := append([]string{mnemonic}, d.Public...)
	_, err := AdmitSection(bs(recs), SectionPublic)
	if err == nil {
		t.Fatal("a BIP-39 mnemonic in the public section must be refused")
	}
	if !errors.Is(err, ErrRecordNotPermitted) {
		t.Errorf("refused with %v, want ErrRecordNotPermitted — the grouping is running before the allow-list", err)
	}
}
