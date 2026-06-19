package gui

import "testing"

// TestMultisigEngraveCards mirrors singleSigEngraveCards: full = [ms1, mk1, md1],
// watch-only = [mk1, md1]. The md1 card carries the SUPPLIED strings VERBATIM
// (I-2). The ms1 card is the SECRET, single-string, first.
func TestMultisigEngraveCards(t *testing.T) {
	md1 := []string{"md1aaa", "md1bbb"}
	mk1 := []string{"mk1xxx", "mk1yyy"}
	ms1 := "ms10secret"

	t.Run("full = ms1, mk1, md1", func(t *testing.T) {
		cards := multisigEngraveCards(ms1, mk1, md1, true)
		if len(cards) != 3 {
			t.Fatalf("full produced %d cards, want 3", len(cards))
		}
		if cards[0].kind != cardMS1 || len(cards[0].strings) != 1 || cards[0].strings[0] != ms1 {
			t.Fatalf("card[0] = %+v, want a single-string ms1", cards[0])
		}
		if cards[1].kind != cardMK1 {
			t.Fatalf("card[1].kind = %v, want cardMK1", cards[1].kind)
		}
		if cards[2].kind != cardMD1 {
			t.Fatalf("card[2].kind = %v, want cardMD1", cards[2].kind)
		}
		// md1 verbatim.
		for i := range md1 {
			if cards[2].strings[i] != md1[i] {
				t.Fatalf("md1 card[%d] = %q, want verbatim %q", i, cards[2].strings[i], md1[i])
			}
		}
	})

	t.Run("watch-only = mk1, md1", func(t *testing.T) {
		cards := multisigEngraveCards("", mk1, md1, false)
		if len(cards) != 2 {
			t.Fatalf("watch-only produced %d cards, want 2", len(cards))
		}
		if cards[0].kind != cardMK1 || cards[1].kind != cardMD1 {
			t.Fatalf("watch-only card kinds = %v/%v, want cardMK1/cardMD1", cards[0].kind, cards[1].kind)
		}
		// No cardMS1 -> bundleEngrave will show the ms1 reminder.
		if bundleShowMs1Reminder(cards) != true {
			t.Fatal("watch-only should trigger the ms1 reminder (no cardMS1)")
		}
	})
}
