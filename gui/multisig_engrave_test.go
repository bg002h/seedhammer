package gui

import "testing"

// TestMultisigEngraveCards pins the one-of-each engrave SHAPE: full = [ms1, mk1,
// md1], watch-only = [mk1, md1]. The md1 card carries the SUPPLIED strings
// VERBATIM (I-2). The ms1 card is the SECRET, single-string, first.
//
// IT ASSERTS AGAINST multisigEngraveCardsMulti, which is the emitter an
// operator's plates actually come out of. Until F-189 it drove
// multisigEngraveCards, a one-of-each adapter over that function which had had
// no production caller since F-188 -- the supply path derives a leg per matched
// slot and emits through the multi form directly. That adapter is DELETED, and
// this test is the reason it could be: the shape it pinned is real, it is just
// a property of the surviving producer at n=1 rather than of a wrapper.
//
// A RETIRED EMITTER IS NOT NEUTRAL, which is why the deletion is worth the churn
// in this file. It is an invitation to reintroduce the rule it encoded, and that
// has already happened once nearby: a review proposed relaxing a verify rule
// that a deleted symmetry made look optional (errVerifyLegHasNoPlate, whose
// comment now says THIS RULE IS LOAD-BEARING AND MUST NOT BE RELAXED).
func TestMultisigEngraveCards(t *testing.T) {
	md1 := []string{"md1aaa", "md1bbb"}
	mk1 := []string{"mk1xxx", "mk1yyy"}
	ms1 := "ms10secret"

	t.Run("full = ms1, mk1, md1", func(t *testing.T) {
		cards := multisigEngraveCardsMulti([]string{ms1}, [][]string{mk1}, md1)
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
		// A LONE CARD OF A KIND KEEPS THE SHIPPED LABEL, unnumbered. The plate
		// census and the restore-doc inventory both print it, and "1 of 1" on a
		// one-leg build would be noise on the screens an operator reads before and
		// after cutting steel.
		if cards[0].label != "ms1 secret share" || cards[1].label != "mk1 key" {
			t.Errorf("a one-of-each set numbered its cards: %q / %q",
				cards[0].label, cards[1].label)
		}
	})

	t.Run("watch-only = mk1, md1", func(t *testing.T) {
		cards := multisigEngraveCardsMulti(nil, [][]string{mk1}, md1)
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
