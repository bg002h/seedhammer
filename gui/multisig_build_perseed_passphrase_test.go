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
// twice, under DIFFERENT passphrases.
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

// TestRestoreDocNamesEveryPassphrasedSeed is I-5's arm.
func TestRestoreDocNamesEveryPassphrasedSeed(t *testing.T) {
	reg := s5TwoPassphraseRegistry(t, "alpha", "beta")
	facts := reg.passphraseFacts()
	if len(facts) != 2 {
		t.Fatalf("the registry reports %d seed(s), want 2", len(facts))
	}
	// THE PREMISE, MEASURED: two entries, two DISTINCT fingerprints, from ONE
	// word list. That is what makes them look like two masters to every surface
	// downstream, and it is why a single bool cannot describe them.
	if facts[0].MasterFP == facts[1].MasterFP {
		t.Fatalf("both (seed, passphrase) pairs derive fingerprint %08x, so this "+
			"fixture is not two distinct derivation units", facts[0].MasterFP)
	}
	if !facts[0].Uses || !facts[1].Uses {
		t.Fatalf("a pair carrying a passphrase reports Uses=false: %+v", facts)
	}

	cards := []bundleCard{
		{kind: cardMS1, label: "ms1 secret share", summary: "seed", strings: []string{"ms1"}},
		{kind: cardMK1, label: "mk1 key 1 of 2", summary: "key", strings: []string{"mk1a"}},
		{kind: cardMK1, label: "mk1 key 2 of 2", summary: "key", strings: []string{"mk1b"}},
	}
	doc := strings.Join(buildPlateInventoryLines(cards, facts), "\n")
	t.Logf("two-passphrase restore doc:\n%s", doc)

	// TWO DISTINCT PASSPHRASE STATEMENTS, one per passphrased seed. A document
	// with one statement over two required passphrases is the defect.
	statements := strings.Count(doc, "Needs a passphrase:")
	if statements != 2 {
		t.Fatalf("the restore document carries %d per-seed passphrase statement(s), want "+
			"2.\nThe operator typed TWO different passphrases; a document that says \"the "+
			"passphrase\" once has told them one of their two keys is recoverable when it "+
			"is not:\n%s", statements, doc)
	}
	for _, f := range facts {
		if !strings.Contains(doc, f.Label) {
			t.Errorf("the document never names the seed %q, so a reader cannot tell which "+
				"passphrase goes with which plate:\n%s", f.Label, doc)
		}
		if !strings.Contains(doc, fpHex(f.MasterFP)) {
			t.Errorf("the document never carries fingerprint %s, which is what a "+
				"coordinator shows beside the key and therefore the only handle a reader "+
				"has on WHICH seed this sentence is about:\n%s", fpHex(f.MasterFP), doc)
		}
	}
	// And it warns that they may DIFFER, which is the whole point: a reader who
	// assumes one passphrase for both loses the other leg.
	if !strings.Contains(doc, "DIFFERENT passphrases") {
		t.Errorf("the document does not warn that the passphrases may differ:\n%s", doc)
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
	doc := strings.Join(buildPlateInventoryLines(cards, facts), "\n")
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
	one := buildPassphraseInventoryLines(oneSeedPassphraseFact(true))
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
	if got := strings.Join(buildPassphraseInventoryLines(reg.passphraseFacts()), "\n"); got != joined {
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
