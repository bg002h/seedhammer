package gui

import (
	"strings"
	"testing"

	"seedhammer.com/bip39"
)

// ─── I-5: N per-seed passphrases must not collapse into one boolean ──────────
//
// SPEC 4.1 makes the (seed, passphrase) PAIR the derivation unit and asks the
// passphrase PER SEED. S5 routed all of them through seedRegistry.usesPassphrase(),
// an any(), and that single bit was the only passphrase signal reaching either
// operator-facing surface.
//
// THE MEASURED SHAPE. multisigSelfSlotPickFlow is multi-select and
// buildSeedForSlot asks a seed AND ITS OWN passphrase per held slot. An operator
// holding @0 and @1 who types the SAME twelve words twice, with `alpha` then
// `beta`, gets: ONE ms1 plate (correct -- ms1 encodes the words only), TWO mk1
// plates differing only in a master fingerprint, and a restore document reading
// "A BIP-39 passphrase WAS used ... Without IT ... Keep IT somewhere separate" --
// every reference singular -- immediately after asserting "If any of them is
// missing, this backup is incomplete." Nothing on steel or on that page lets a
// reader learn a SECOND passphrase exists.
//
// In a 3-of-4 holding three slots, the operator records "the passphrase", two
// legs of three recover years later, and the funds are unreachable, silently,
// with the backup vouching for itself throughout. The commoner and equally fatal
// form is the same bug: two different masters, one passphrased, and the document
// cannot say WHICH of "ms1 secret share 1 of 2" / "2 of 2" needs it.

// s5TwoPassphraseRegistry is the shape the flow builds: ONE master, entered
// twice -- once per HELD SLOT -- with a passphrase typed at each prompt.
//
// The two passphrases are the CALLER'S, because both cells are real and they
// have opposite right answers: two different passphrases are two derivation
// units and must both be named, while the same passphrase twice is ONE
// derivation unit and naming it twice is B1.
func s5TwoPassphraseRegistry(t *testing.T, first, second string) *seedRegistry {
	t.Helper()
	reg := &seedRegistry{}
	for i, pass := range []string{first, second} {
		m, err := bip39.ParseMnemonic(fixtureMasterA)
		if err != nil {
			t.Fatalf("ParseMnemonic: %v", err)
		}
		label := "your seed for @" + string(rune('0'+i))
		if _, err := reg.add(label, m, pass, s5Net); err != nil {
			t.Fatalf("registering %s: %v", label, err)
		}
	}
	return reg
}

// TestRestoreDocNamesEveryPassphrasedSeed is I-5's arm, ACROSS BOTH CELLS.
//
// It used to drive `alpha`/`beta` only and to t.Fatal when the two fingerprints
// came out EQUAL. That was the premise check for that fixture and, at the same
// time, a structural blind spot: equal fingerprints are exactly the shape B1
// lives in, so the one cell that could have failed on B1 was the one cell this
// test refused to run. The premise now belongs to its own row, and the
// equal-fingerprint row is ASSERTED rather than rejected.
//
// THE TWO ROWS ARE ONE KEYSTROKE APART. buildSeedForSlot prompts per HELD SLOT
// and the prompt has no confirm-entry, so an operator holding @0 and @1 who
// means to type one passphrase twice either succeeds (row 2: ONE secret) or
// mistypes a character (row 1: TWO secrets). The document has to say which
// happened, and the two answers are not interchangeable in either direction:
// printing one secret as two sends a reader hunting for a passphrase that does
// not exist, and printing two as one loses a leg.
func TestRestoreDocNamesEveryPassphrasedSeed(t *testing.T) {
	cards := []bundleCard{
		{kind: cardMS1, label: "ms1 secret share", summary: "seed", strings: []string{"ms1"}},
		{kind: cardMK1, label: "mk1 key 1 of 2", summary: "key", strings: []string{"mk1a"}},
		{kind: cardMK1, label: "mk1 key 2 of 2", summary: "key", strings: []string{"mk1b"}},
	}
	for _, tc := range []struct {
		name          string
		first, second string
		// wantFacts is how many DERIVATION UNITS the registry reports, and it is
		// the whole of B1: the registry holds one entry per HELD SLOT, while the
		// document is about SECRETS.
		wantFacts int
		// wantStatements is how many "Needs a passphrase:" sentences the document
		// carries. ONE fact takes buildPassphraseInventoryLines' documented
		// single-seed arm, which enumerates nothing -- correctly, because with one
		// secret there is nothing to tell apart.
		wantStatements int
		sameFP         bool
	}{
		{
			// The mistyped-character case: two pairs, two masters, two keys.
			name:  "two DIFFERENT passphrases at the two held slots",
			first: "alpha", second: "beta",
			wantFacts: 2, wantStatements: 2, sameFP: false,
		},
		{
			// B1. One seed, one passphrase, two held slots -- and still two
			// registry entries, because reg.add runs once per held slot.
			name:  "the SAME passphrase at both held slots (B1)",
			first: "alpha", second: "alpha",
			wantFacts: 1, wantStatements: 0, sameFP: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reg := s5TwoPassphraseRegistry(t, tc.first, tc.second)
			if got := len(reg.seeds); got != 2 {
				t.Fatalf("the registry holds %d entry/entries, want 2 -- this fixture is "+
					"not the two-held-slot shape", got)
			}
			// THE PREMISE, MEASURED, IN BOTH DIRECTIONS. Row 1 needs two DISTINCT
			// fingerprints or it is not two derivation units; row 2 needs them EQUAL
			// or it is not B1's cell at all.
			gotSameFP := reg.seeds[0].MasterFP == reg.seeds[1].MasterFP
			if gotSameFP != tc.sameFP {
				t.Fatalf("the two registry entries derive fingerprints %08x and %08x "+
					"(equal=%v), want equal=%v -- this fixture is not the cell the row "+
					"names", reg.seeds[0].MasterFP, reg.seeds[1].MasterFP, gotSameFP, tc.sameFP)
			}

			facts := reg.passphraseFacts()
			if len(facts) != tc.wantFacts {
				t.Fatalf("the registry reports %d passphrase fact(s), want %d.\nThe fact "+
					"list is one entry per SECRET, not one per held slot: %+v",
					len(facts), tc.wantFacts, facts)
			}
			for _, f := range facts {
				if !f.Uses {
					t.Fatalf("a pair carrying a passphrase reports Uses=false: %+v", facts)
				}
			}

			doc := strings.Join(buildPlateInventoryLines(cards, facts, seedCapacityMany, false), "\n")
			t.Logf("restore doc:\n%s", doc)

			if got := strings.Count(doc, "Needs a passphrase:"); got != tc.wantStatements {
				t.Fatalf("the restore document carries %d per-seed passphrase "+
					"statement(s), want %d:\n%s", got, tc.wantStatements, doc)
			}

			if tc.wantFacts == 1 {
				// ONE SECRET, AND THE FACT STILL SAYS WHICH SLOTS IT IS FOR. The label
				// is the only handle the merge leaves on the held set, so dropping one
				// of the two slot names would hide a SLOT rather than a duplicate
				// sentence -- the same failure direction the merge exists to avoid.
				for _, want := range []string{"your seed for @0", "your seed for @1"} {
					if !strings.Contains(facts[0].Label, want) {
						t.Errorf("the merged fact's label %q does not name %q, so the merge "+
							"lost a held slot rather than a duplicate sentence",
							facts[0].Label, want)
					}
				}
				// And the document must not tell the reader a SECOND, different
				// passphrase might exist. There is only one.
				if strings.Contains(doc, "DIFFERENT passphrases") {
					t.Errorf("ONE passphrase, typed at two held slots, is described to the "+
						"reader as possibly TWO DIFFERENT ones. A reader who cannot find "+
						"the second must decide whether a fully recoverable backup is "+
						"unrecoverable:\n%s", doc)
				}
				if got := strings.Count(doc, fpHex(facts[0].MasterFP)); got > 1 {
					t.Errorf("the one master fingerprint %s appears %d times, so the "+
						"document reads as several seeds that happen to share one:\n%s",
						fpHex(facts[0].MasterFP), got, doc)
				}
				return
			}

			for _, f := range facts {
				if !strings.Contains(doc, f.Label) {
					t.Errorf("the document never names the seed %q, so a reader cannot tell "+
						"which passphrase goes with which plate:\n%s", f.Label, doc)
				}
				if !strings.Contains(doc, fpHex(f.MasterFP)) {
					t.Errorf("the document never carries fingerprint %s, which is what a "+
						"coordinator shows beside the key and therefore the only handle a "+
						"reader has on WHICH seed this sentence is about:\n%s",
						fpHex(f.MasterFP), doc)
				}
			}
			// And it warns that they may DIFFER, which is the whole point: a reader
			// who assumes one passphrase for both loses the other leg.
			if !strings.Contains(doc, "DIFFERENT passphrases") {
				t.Errorf("the document does not warn that the passphrases may differ:\n%s", doc)
			}
		})
	}
}

// TestRestoreDocMergesOneSeedHeldAtTwoSlots is B1 in the shape round 1 MEASURED,
// and it is the cell where the merge has to RENDER rather than merely collapse.
//
// Trace B's flagship: @0 and @1 from master A under the SAME passphrase, @2 from
// master B with none. Before the fix the document read
//
//	Needs a passphrase: your seed for @0 (master fingerprint ca2c62d2). If more
//	  than one is listed here they may be DIFFERENT passphrases; record each one
//	  against its fingerprint.
//	Needs a passphrase: your seed for @1 (master fingerprint ca2c62d2). ...
//
// -- one secret, printed twice, with the SAME fingerprint on both lines, on the
// artifact that outlives the operator, and three steps after the Key-sources
// gate had correctly said "Slots @0 and @1 all come from your seed".
//
// It is the MIXED cell rather than the pure one because a bare seed keeps the
// fact list at two entries, so the enumeration arm runs and the merged LABEL is
// actually drawn. The pure two-slot case collapses to the single-seed arm and
// renders no label at all; both are asserted, in different tests, on purpose.
func TestRestoreDocMergesOneSeedHeldAtTwoSlots(t *testing.T) {
	reg := &seedRegistry{}
	for _, tc := range []struct{ label, phrase, pass string }{
		{"your seed for @0", fixtureMasterA, "alpha"},
		{"your seed for @1", fixtureMasterA, "alpha"},
		{"your seed for @2", fixtureMasterB, ""},
	} {
		m, err := bip39.ParseMnemonic(tc.phrase)
		if err != nil {
			t.Fatalf("ParseMnemonic: %v", err)
		}
		if _, err := reg.add(tc.label, m, tc.pass, s5Net); err != nil {
			t.Fatalf("registering %s: %v", tc.label, err)
		}
	}
	// THE PREMISE, MEASURED: three registry entries, one per held slot, and the
	// first two carry the SAME fingerprint because they are the same pair.
	if len(reg.seeds) != 3 {
		t.Fatalf("the registry holds %d entries, want 3", len(reg.seeds))
	}
	if reg.seeds[0].MasterFP != reg.seeds[1].MasterFP {
		t.Fatalf("@0 and @1 derive %08x and %08x; the fixture is not ONE seed at two "+
			"slots", reg.seeds[0].MasterFP, reg.seeds[1].MasterFP)
	}

	facts := reg.passphraseFacts()
	if len(facts) != 2 {
		t.Fatalf("the registry reports %d passphrase fact(s) for TWO secrets held at "+
			"three slots, want 2: %+v", len(facts), facts)
	}

	cards := []bundleCard{
		{kind: cardMS1, label: "ms1 secret share 1 of 2", summary: "seed", strings: []string{"ms1a"}},
		{kind: cardMS1, label: "ms1 secret share 2 of 2", summary: "seed", strings: []string{"ms1b"}},
		{kind: cardMK1, label: "mk1 key 1 of 3", summary: "key", strings: []string{"mk1a"}},
	}
	doc := strings.Join(buildPlateInventoryLines(cards, facts, seedCapacityMany, false), "\n")
	t.Logf("Trace B restore doc:\n%s", doc)

	if got := strings.Count(doc, "Needs a passphrase:"); got != 1 {
		t.Fatalf("ONE passphrase, typed at two held slots, produced %d passphrase "+
			"statement(s), want 1. Two lines carrying the SAME fingerprint tell a reader "+
			"there is a second passphrase somewhere to be found:\n%s", got, doc)
	}
	// The merged line names BOTH slots, in one sentence, against one fingerprint.
	for _, want := range []string{"your seed for @0", "your seed for @1"} {
		if !strings.Contains(doc, want) {
			t.Errorf("the merged passphrase statement does not name %q; the merge dropped "+
				"a held slot:\n%s", want, doc)
		}
	}
	if got := strings.Count(doc, fpHex(facts[0].MasterFP)); got != 1 {
		t.Errorf("master fingerprint %s appears %d time(s) on the document, want 1:\n%s",
			fpHex(facts[0].MasterFP), got, doc)
	}
	// And the seed that needs NO passphrase is still said out loud, unmerged: it
	// is a DIFFERENT secret, and merging it would be the failure in the other
	// direction -- a required passphrase silently dropped.
	if !strings.Contains(doc, "Needs NO passphrase: your seed for @2") {
		t.Errorf("the bare seed at @2 is no longer named, so a reader cannot tell which "+
			"of the two ms1 plates the passphrase warning is about:\n%s", doc)
	}
	// It still draws.
	if strings.ContainsAny(doc, "—–·‘’“”…") {
		t.Errorf("the document carries a glyph the body face lacks:\n%q", doc)
	}
}

// TestRestoreDocSaysWhichSeedsNeedNoPassphrase is the mixed case, and it is the
// commoner and equally fatal one: two DIFFERENT masters, one passphrased.
//
// With a single bool the document could only say "a passphrase WAS used", which
// leaves a reader holding "ms1 secret share 1 of 2" and "2 of 2" unable to tell
// which of them the passphrase belongs to. Trying it against the wrong one
// produces a valid-looking wallet that is not theirs.
func TestRestoreDocSaysWhichSeedsNeedNoPassphrase(t *testing.T) {
	reg := &seedRegistry{}
	for _, tc := range []struct{ label, phrase, pass string }{
		{"your seed for @0", fixtureMasterA, "alpha"},
		{"your seed for @1", fixtureMasterB, ""},
	} {
		m, err := bip39.ParseMnemonic(tc.phrase)
		if err != nil {
			t.Fatalf("ParseMnemonic: %v", err)
		}
		if _, err := reg.add(tc.label, m, tc.pass, s5Net); err != nil {
			t.Fatalf("registering %s: %v", tc.label, err)
		}
	}
	facts := reg.passphraseFacts()
	cards := []bundleCard{
		{kind: cardMS1, label: "ms1 secret share 1 of 2", summary: "seed", strings: []string{"ms1a"}},
		{kind: cardMS1, label: "ms1 secret share 2 of 2", summary: "seed", strings: []string{"ms1b"}},
	}
	doc := strings.Join(buildPlateInventoryLines(cards, facts, seedCapacityMany, false), "\n")
	t.Logf("mixed restore doc:\n%s", doc)

	if strings.Count(doc, "Needs a passphrase:") != 1 {
		t.Errorf("the document does not name exactly the ONE seed that needs a "+
			"passphrase:\n%s", doc)
	}
	// AND THE BARE ONE IS SAID OUT LOUD. Silence reads as "all of them need it",
	// which sends the reader hunting for a passphrase that never existed -- and,
	// worse, lets them conclude the one passphrase they hold is the only factor
	// missing from the whole set.
	if !strings.Contains(doc, "Needs NO passphrase: your seed for @1") {
		t.Errorf("the document does not say which seed needs NO passphrase, so a reader "+
			"cannot tell which of the two ms1 plates the warning is about:\n%s", doc)
	}
	// It still draws.
	if strings.ContainsAny(doc, "—–·‘’“”…") {
		t.Errorf("the document carries a glyph the body face lacks:\n%q", doc)
	}
}

// TestSingleSeedInventoryIsUnchanged is the regression floor.
//
// The overwhelmingly common build has ONE seed, and every sentence in the
// shipped two-line text is singular AND TRUE there. A fix that made the ordinary
// backup document read like a table would be a regression on the case that
// actually ships.
func TestSingleSeedInventoryIsUnchanged(t *testing.T) {
	one := buildPassphraseInventoryLines(oneSeedPassphraseFact(true), false)
	if len(one) != 2 {
		t.Fatalf("a one-seed passphrase build produced %d line(s), want the shipped 2:\n%s",
			len(one), strings.Join(one, "\n"))
	}
	joined := strings.Join(one, "\n")
	if strings.Contains(joined, "Needs a passphrase:") {
		t.Errorf("a ONE-seed build now enumerates its seeds; there is nothing to tell "+
			"apart:\n%s", joined)
	}
	// A single registered seed goes down the same arm, even when it has a
	// fingerprint: the enumeration is what more-than-one buys.
	reg := &seedRegistry{}
	m, err := bip39.ParseMnemonic(fixtureMasterA)
	if err != nil {
		t.Fatalf("ParseMnemonic: %v", err)
	}
	if _, err := reg.add("your seed for @0", m, "alpha", s5Net); err != nil {
		t.Fatalf("registering: %v", err)
	}
	if got := strings.Join(buildPassphraseInventoryLines(reg.passphraseFacts(), false), "\n"); got != joined {
		t.Errorf("a single REGISTERED seed reads differently from the single-seed arm:\n%s\n---\n%s",
			got, joined)
	}
	// A zero fingerprint is never printed as 00000000: a reader would go looking
	// for a key that carries it.
	if strings.Contains(joined, "00000000") {
		t.Errorf("a placeholder fingerprint reached the restore document:\n%s", joined)
	}
}

// fpHex renders a master fingerprint the way the inventory does.
func fpHex(fp uint32) string {
	const hex = "0123456789abcdef"
	out := make([]byte, 8)
	for i := 7; i >= 0; i-- {
		out[i] = hex[fp&0xf]
		fp >>= 4
	}
	return string(out)
}
