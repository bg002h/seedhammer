package gui

import (
	"testing"

	"github.com/btcsuite/btcd/chaincfg/v2"
	"seedhammer.com/bundle"
	"seedhammer.com/md"
)

// TestSingleSigEngraveCardsFull: full mode → exactly 3 cards (ms1, mk1, md1) in
// that order, each carrying its derived strings VERBATIM.
func TestSingleSigEngraveCardsFull(t *testing.T) {
	m := abandonAboutMnemonic()
	b, _, _, _, err := deriveSingleSigBundle(m, "", &chaincfg.MainNetParams, singleSigPath(84), md.ScriptWpkh)
	if err != nil {
		t.Fatalf("derive: %v", err)
	}
	cards := singleSigEngraveCards(b, true /*full*/)
	if len(cards) != 3 {
		t.Fatalf("full mode: %d cards, want 3", len(cards))
	}
	if cards[0].kind != cardMS1 {
		t.Fatalf("card[0].kind = %v, want cardMS1", cards[0].kind)
	}
	if cards[1].kind != cardMK1 {
		t.Fatalf("card[1].kind = %v, want cardMK1", cards[1].kind)
	}
	if cards[2].kind != cardMD1 {
		t.Fatalf("card[2].kind = %v, want cardMD1", cards[2].kind)
	}
	if len(cards[0].strings) != 1 || cards[0].strings[0] != b.MS1 {
		t.Fatalf("ms1 card strings = %v, want [%q]", cards[0].strings, b.MS1)
	}
	if !equalStringSlice(cards[1].strings, b.MK1) {
		t.Fatalf("mk1 card strings not verbatim: %v vs %v", cards[1].strings, b.MK1)
	}
	if !equalStringSlice(cards[2].strings, b.MD1) {
		t.Fatalf("md1 card strings not verbatim: %v vs %v", cards[2].strings, b.MD1)
	}
}

// TestSingleSigEngraveCardsWatchOnly: watch-only mode → exactly 2 cards (mk1,
// md1); NO ms1 card. The end-of-engrave reminder gate fires for this set.
func TestSingleSigEngraveCardsWatchOnly(t *testing.T) {
	m := abandonAboutMnemonic()
	b, _, _, _, err := deriveSingleSigBundle(m, "", &chaincfg.MainNetParams, singleSigPath(84), md.ScriptWpkh)
	if err != nil {
		t.Fatalf("derive: %v", err)
	}
	cards := singleSigEngraveCards(b, false /*watch-only*/)
	if len(cards) != 2 {
		t.Fatalf("watch-only: %d cards, want 2", len(cards))
	}
	if cards[0].kind != cardMK1 || cards[1].kind != cardMD1 {
		t.Fatalf("watch-only cards = [%v %v], want [cardMK1 cardMD1]", cards[0].kind, cards[1].kind)
	}
	for _, c := range cards {
		if c.kind == cardMS1 {
			t.Fatal("watch-only must not include an ms1 card")
		}
	}
	// R0-I2: the ms1 reminder SHOWS for a watch-only set (no cardMS1).
	if !bundleShowMs1Reminder(cards) {
		t.Fatal("watch-only: ms1 reminder gate must fire (no ms1 card engraved)")
	}
}

// TestBundleShowMs1ReminderGate (R0-I2): the reminder is suppressed iff an ms1
// card was engraved (any(card.kind==cardMS1)) — derived from the cards slice
// inside bundleEngrave, with NO signature change. A T5-style set (mk1+md1,
// never an ms1) still shows the reminder.
func TestBundleShowMs1ReminderGate(t *testing.T) {
	full := []bundleCard{
		{kind: cardMS1, strings: []string{"ms1..."}},
		{kind: cardMK1, strings: []string{"mk1..."}},
		{kind: cardMD1, strings: []string{"md1..."}},
	}
	if bundleShowMs1Reminder(full) {
		t.Fatal("full mode (ms1 engraved) must SUPPRESS the reminder")
	}
	// A T5 bundle (mk1+md1, no ms1) → reminder shown (unchanged T5 behaviour).
	t5 := []bundleCard{
		{kind: cardMK1, strings: []string{"mk1..."}},
		{kind: cardMD1, strings: []string{"md1..."}},
	}
	if !bundleShowMs1Reminder(t5) {
		t.Fatal("T5 set (no ms1) must SHOW the reminder")
	}
}

// TestSingleSigEngravePlatesValidate: every plate string of every card (incl. the
// ms1 card) lays out to at least one engravable plate via validateMdmk — the
// guided engrave never produces an empty plate set for a derived card.
func TestSingleSigEngravePlatesValidate(t *testing.T) {
	m := abandonAboutMnemonic()
	b, _, _, _, err := deriveSingleSigBundle(m, "", &chaincfg.MainNetParams, singleSigPath(84), md.ScriptWpkh)
	if err != nil {
		t.Fatalf("derive: %v", err)
	}
	cards := singleSigEngraveCards(b, true)
	params := newPlatform().EngraverParams()
	for _, p := range bundlePlatePlan(cards) {
		labels, plates, err := validateMdmk(params, p.str)
		if err != nil || len(plates) == 0 || len(labels) == 0 {
			t.Fatalf("plate %q (card %d) does not fit any variant: err=%v plates=%d", p.str, p.cardIdx, err, len(plates))
		}
	}
}

// TestSingleSigEngraveLongestMs1Fits (R0-m2): a 24-word seed yields the LONGEST
// ms1 (75 chars). Its single-plate ms1 card must lay out (validateMdmk) and must
// NOT trip the whole-bundle abort in bundleEngrave (bundle_flow.go) — i.e. it
// fits a plate as its own 1-plate card.
func TestSingleSigEngraveLongestMs1Fits(t *testing.T) {
	m := validMnemonic(24)
	b, _, _, _, err := deriveSingleSigBundle(m, "", &chaincfg.MainNetParams, singleSigPath(84), md.ScriptWpkh)
	if err != nil {
		t.Fatalf("derive 24-word: %v", err)
	}
	if len(b.MS1) != 75 {
		t.Fatalf("24-word ms1 length = %d, want 75 (16-byte=49, 32-byte=75)", len(b.MS1))
	}
	cards := singleSigEngraveCards(b, true)
	if cards[0].kind != cardMS1 || len(cards[0].strings) != 1 {
		t.Fatalf("ms1 card shape wrong: %+v", cards[0])
	}
	params := newPlatform().EngraverParams()
	labels, plates, err := validateMdmk(params, b.MS1)
	if err != nil || len(plates) == 0 || len(labels) == 0 {
		t.Fatalf("75-char ms1 does not fit a plate: err=%v plates=%d", err, len(plates))
	}
	// The full plan validates (no whole-bundle abort would fire).
	for _, p := range bundlePlatePlan(cards) {
		if _, pl, err := validateMdmk(params, p.str); err != nil || len(pl) == 0 {
			t.Fatalf("plate %q does not fit (would abort the bundle): err=%v", p.str, err)
		}
	}
}

func equalStringSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// ensure the bundle import is used even if a future refactor drops it.
var _ = bundle.Bundle{}
