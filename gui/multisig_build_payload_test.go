package gui

import (
	"fmt"
	"strings"
	"testing"
	"testing/synctest"

	"github.com/btcsuite/btcd/chaincfg/v2"
	"seedhammer.com/bip39"
	"seedhammer.com/md"
	"seedhammer.com/mk"
)

// ─── S1 tests 3–8 — "the payload supplies the whole cosigner set" ────────────
//
// Two rulings bind these and are asserted rather than assumed:
//
//   - F-173 `0..n` (operator, 2026-08-14): the payload may carry zero to n
//     cosigner cards and NO stage may assume n-1. Test 8 is that ruling as a
//     matrix; tests 6 and 7 are its two ends.
//   - The S1 assumption audit rows 1 and 2 (2026-08-15): over-supply resolves by
//     bounded selection with payload record order preserved, and equal-count
//     auto-fills straight to the review; under-supply STAYS a refusal but must
//     name the host route rather than an NFC reader that does not exist.

// buildNRow returns the ChoiceScreen row index for cosigner count n, and the
// row count of that picker. multisigNChoices is {"2","3","4","5"} with
// multisigNFor(idx) = idx+2, MEASURED here rather than transcribed so a change
// to the picker fails this helper instead of silently building a different
// policy.
func buildNRow(t *testing.T, n int) (idx, rows int) {
	t.Helper()
	rows = len(multisigNChoices())
	for i := 0; i < rows; i++ {
		if multisigNFor(i) == n {
			return i, rows
		}
	}
	t.Fatalf("n=%d is not offered by multisigNChoices (%v)", n, multisigNChoices())
	return 0, 0
}

// buildWalkToGather drives buildParamPickFlow with a chosen n and every other
// default (template wsh, k=1, self slot @0, fp Omit), leaving the flow at
// whatever follows the pickers.
func buildWalkToGather(t *testing.T, ctx *Context, frame func() (string, bool), n int) {
	t.Helper()
	nIdx, nRows := buildNRow(t, n)
	if _, ok := pumpUntil(frame, "Template", 16); !ok {
		t.Fatal("template picker not shown")
	}
	click(&ctx.Router, Button3)
	frame()
	if _, ok := pumpUntil(frame, "Cosigners", 16); !ok {
		t.Fatal("n picker not shown")
	}
	for i := 0; i < nIdx; i++ {
		click(&ctx.Router, Down)
		frame()
	}
	click(&ctx.Router, Button3)
	frame()
	// The k picker's lead names the n that was actually chosen, so a mis-tapped
	// row is caught here rather than three screens later.
	if c, ok := pumpUntil(frame, fmt.Sprintf("k of %d", n), 16); !ok {
		t.Fatalf("choosing row %d of %d did not select n=%d; screen reads %q",
			nIdx, nRows, n, c)
	}
	click(&ctx.Router, Button3)
	frame()
	if _, ok := pumpUntil(frame, "Your slot", 16); !ok {
		t.Fatal("self-slot picker not shown")
	}
	click(&ctx.Router, Button3)
	frame()
	if _, ok := pumpUntil(frame, "Fingerprints", 16); !ok {
		t.Fatal("fp picker not shown")
	}
	click(&ctx.Router, Button3)
	frame()
}

// S1 test 3: n=3, two multi-chunk mk1 cards on the payload, ZERO scans — the
// gather yields two complete cards.
//
// "Zero scans" is structural here and asserted as such: the test platform has no
// NFC reader, so nothing can reach the gatherer except the payload feed. That is
// the unit-test half of F-174's rule; the emulator walk asserts the same thing
// against a reader that DOES exist, via shNFC.presented() === 0.
func TestBuildGathersEveryCosignerFromPayload(t *testing.T) {
	records := cosignerCardRecords(t, 2)
	// The fixtures are multi-chunk, so this exercises assembly and not just
	// pass-through — the plan's "each 2 chunks".
	for i, set := range cosignerCardFixtures(t, 2) {
		if len(set) < 2 {
			t.Fatalf("INCONCLUSIVE: fixture card %d is a single chunk, so chunk "+
				"assembly is not exercised", i)
		}
	}

	synctest.Test(t, func(t *testing.T) {
		ctx := NewContext(newPlatform())
		if ctx.Platform.NFCReader() != nil {
			t.Fatal("INCONCLUSIVE: this platform HAS an NFC reader, so a card in the " +
				"tally would not prove it came from the payload")
		}
		ctx.sysw = sessionHolding(records...)
		frame, quit := runUI(ctx, func() { buildMultisigPolicyFlow(ctx, &descriptorTheme) })
		defer quit()
		buildWalkToGather(t, ctx, frame, 3)
		c, ok := pumpUntil(frame, "mk1 keys: 2", 32)
		if !ok {
			t.Fatalf("the payload's two cosigner cards did not both reach the gather; "+
				"screen reads %q", c)
		}
	})

	// And headlessly, so the count is a value rather than a screen string.
	got, incomplete := buildCosignerSupply(records)
	if len(got) != 2 || incomplete {
		t.Errorf("buildCosignerSupply over %d records yielded %d complete card(s) "+
			"(incomplete=%v); want 2, false", len(records), len(got), incomplete)
	}
	for i, c := range got {
		if c.kind != cardMK1 {
			t.Errorf("card %d is kind %v, want cardMK1", i, c.kind)
		}
	}
}

// S1 test 4: an md1 riding along on the payload does NOT fail the build (spec P0
// item 3). A systemwide payload carries whatever the operator packed, and the
// Build path wants keys; a descriptor in the same blob is normal.
func TestBuildIgnoresMd1RecordsInThePayload(t *testing.T) {
	const md1 = "md1yqpqqxqq8xtwhw4xwn4qh"
	// md1 FIRST, so a filter that only looked at the tail would fail here.
	records := append([]string{md1}, cosignerCardRecords(t, 2)...)

	supply, _ := buildCosignerSupply(records)
	if len(supply) != 2 {
		t.Fatalf("buildCosignerSupply kept %d card(s) from a payload holding an md1 "+
			"and two mk1s; want 2 (the md1 dropped)", len(supply))
	}

	// The refusal that WOULD fire if the md1 leaked through: buildCosignerCards
	// refuses any cardMD1 outright, so this is the arm the filter protects.
	withMd1 := []bundleCard{{kind: cardMD1, label: "md1 descriptor", strings: []string{md1}}}
	withMd1 = append(withMd1, supply...)
	if _, ok := buildCosignerCards(withMd1, 2); ok {
		t.Fatal("INCONCLUSIVE: buildCosignerCards accepted an md1, so the filter " +
			"above cannot be what keeps the build alive")
	}

	// End to end: the flow reaches the SEED entry, which is past the cosigner
	// resolution — i.e. the md1 did not fail the build.
	synctest.Test(t, func(t *testing.T) {
		ctx := NewContext(newPlatform())
		ctx.sysw = sessionHolding(records...)
		frame, quit := runUI(ctx, func() { buildMultisigPolicyFlow(ctx, &descriptorTheme) })
		defer quit()
		buildWalkToGather(t, ctx, frame, 3)
		if c, ok := pumpUntil(frame, "md1 descriptors: 1", 32); !ok {
			t.Fatalf("the md1 did not reach the gather tally at all, so this test "+
				"would pass without the filter doing anything; screen reads %q", c)
		}
		if c, ok := pumpUntil(frame, "mk1 keys: 2", 32); !ok {
			t.Fatalf("both cosigner cards did not reach the gather; screen reads %q", c)
		}
		click(&ctx.Router, Button3) // Done adding cards
		if c, ok := pumpUntil(frame, "Seed", 32); !ok {
			t.Fatalf("the build did not get past the cosigner set with an md1 on the "+
				"payload; screen reads %q", c)
		}
	})
}

// S1 test 5: @N assignment follows PAYLOAD RECORD ORDER, and the review screen
// says so.
//
// Order is identity-bearing (md/encode_multisig.go's ordering contract), so this
// is asserted on the assembled policy's own per-slot handle, not on an
// intermediate list: the cards' master fingerprints are carried into
// md.SlotInfo, so "slot @2 holds payload card 2" is checkable against the
// encoder's output rather than against the code that fed it.
func TestBuildSlotOrderIsPayloadRecordOrder(t *testing.T) {
	sets := cosignerCardFixtures(t, 3)
	cards := make([]mk.Card, len(sets))
	for i, s := range sets {
		c, err := mk.Decode(s)
		if err != nil {
			t.Fatalf("decoding fixture %d: %v", i, err)
		}
		cards[i] = c
	}

	// Self at @1, so the cosigners land on @0 and @2 — a self slot in the MIDDLE
	// is what makes "ascending, skipping self" distinguishable from "the first
	// len(cosigners) slots".
	p := buildPolicyParams{Script: md.MultisigWsh, N: 3, K: 2, SelfSlot: 1, IncludeFp: true}
	self := canonicalBip85Master(t)
	selfXpub, selfFP, err := deriveAccountXpub(self, "", &chaincfg.MainNetParams, multisigSharedOrigin())
	if err != nil {
		t.Fatalf("deriveAccountXpub: %v", err)
	}
	// Payload cards 1 and 3 chosen, in that order — a non-contiguous selection,
	// so "record order" cannot be confused with "the first two".
	chosen := []int{0, 2}
	picked := []mk.Card{cards[0], cards[2]}
	_, _, slots, err := assembleBuildPolicy(p, selfXpub, selfFP, picked)
	if err != nil {
		t.Fatalf("assembleBuildPolicy: %v", err)
	}

	wantFp := map[int]string{
		0: cards[0].Fingerprint,
		1: fmt.Sprintf("%08x", selfFP),
		2: cards[2].Fingerprint,
	}
	if wantFp[0] == wantFp[2] {
		t.Fatal("INCONCLUSIVE: the two chosen fixtures share a master fingerprint, " +
			"so slot order is not observable from the assembled policy")
	}
	for _, s := range slots {
		got := fmt.Sprintf("%x", s.Fingerprint)
		if got != wantFp[int(s.Index)] {
			t.Errorf("slot @%d carries fp %s, want %s — payload record order fixes "+
				"@N, and a reorder mints a different wallet-policy id",
				s.Index, got, wantFp[int(s.Index)])
		}
	}

	// The slot->card map the review screen announces must be the SAME mapping
	// assembleBuildPolicy just produced, not a parallel guess at it.
	origins := buildCosignerOrigins(p.N, p.SelfSlot, chosen)
	want := []cosignerOrigin{{slot: 0, card: 1}, {slot: 2, card: 3}}
	if len(origins) != len(want) {
		t.Fatalf("buildCosignerOrigins returned %d entries, want %d", len(origins), len(want))
	}
	for i := range want {
		if origins[i] != want[i] {
			t.Errorf("origin %d = %+v, want %+v", i, origins[i], want[i])
		}
	}

	// And the review screen SHOWS it — §0.1 clause 3 puts the announcement on
	// the confirmation surface, not in scrollback.
	lines := buildReviewLines([4]byte{1, 2, 3, 4}, slots, true,
		buildProvenanceLines(origins, len(cards)))
	joined := strings.Join(lines, "\n")
	for _, want := range []string{"@0", "@2", "payload", "1 and 3", "of 3"} {
		if !strings.Contains(joined, want) {
			t.Errorf("the policy review never announces %q; a default nobody prints "+
				"is a default nobody can catch:\n%s", want, joined)
		}
	}
	// An assembled set that did NOT come from a payload announces nothing — a
	// tool that cries DEFAULT when nothing was assumed is a tool whose warnings
	// get ignored (§0.1's corollary).
	if got := buildProvenanceLines(nil, 0); len(got) != 0 {
		t.Errorf("provenance announced for an empty origin list: %q", got)
	}
}

// S1 test 6, RE-SCOPED by the `0..n` ruling: over-supply is NORMAL and must NOT
// refuse at the feed. The named cannot-fit refusal belongs to the ASSEMBLED set,
// where it is a structural backstop.
//
// Written the old way — a refusal on the feed when the payload holds more cards
// than open slots — this test pinned the very behaviour that made Trace A
// unreachable: the delivered payload carries FOUR cards and Trace A is a 2-of-3.
func TestBuildRefusesMoreCardsThanOpenSlots(t *testing.T) {
	const open = 2 // Trace A: a 2-of-3 has two slots for payload cards
	supply, _ := buildCosignerSupply(cosignerCardRecords(t, 4))
	if len(supply) != 4 {
		t.Fatalf("INCONCLUSIVE: the payload assembled %d cards, want 4 — this test "+
			"is about over-supply", len(supply))
	}

	// The feed does NOT refuse. It selects.
	if got := classifyCosignerSupply(cosignerSourceLoaded, len(supply), open); got != cosignerSelect {
		t.Errorf("a payload carrying 4 cards for %d open slots classified as %v; "+
			"over-supply is the delivered payload's normal state and must resolve "+
			"by selection, not by refusing", open, got)
	}

	// The refusal still EXISTS, on the assembled set: hand buildCosignerCards a
	// set that does not fit and it refuses, as it always did.
	if _, ok := buildCosignerCards(supply, open); ok {
		t.Error("buildCosignerCards accepted 4 cards for 2 slots; the exact-count " +
			"check on the ASSEMBLED set is the backstop the feed's permissiveness " +
			"leans on, and it must stay")
	}

	// And it is a BACKSTOP, not a reachable arm: the selection is bounded to the
	// open-slot count, so what reaches buildCosignerCards always fits.
	for over := 1; over <= 3; over++ {
		have := open + over
		picked := supply[:open] // what bounded selection can produce, at most
		if _, ok := buildCosignerCards(picked, open); !ok {
			t.Errorf("a bounded selection of %d from %d was refused; selection must "+
				"never produce a set the constructor rejects", open, have)
		}
	}
}

// S1 test 7: under-supply refusals must speak PHASE-1 language. The shipped ones
// told the operator to *scan* a card — an instruction phase 1 removed with NFC —
// and a refusal prescribing an impossible action is worse than a bare failure:
// the operator goes looking for a reader that is not there.
//
// The table includes ZERO cards, per the `0..n` ruling: an empty payload is a
// legitimate input, and a build that dead-ends on it with no named route is the
// same defect at the other end of the range.
func TestUnderSupplyRefusalNamesTheHostRoute(t *testing.T) {
	for _, tc := range []struct {
		name       string
		state      cosignerSourceState
		have, open int
		incomplete bool
		want       []string
	}{
		{"zero cards on a loaded payload", cosignerSourceLoaded, 0, 2, false,
			[]string{"no cosigner key cards", "2 cosigner key cards", "me sysw pack"}},
		{"three of four", cosignerSourceLoaded, 3, 4, false,
			[]string{"3 cosigner key cards", "4 cosigner key cards", "me sysw pack"}},
		{"one of two", cosignerSourceLoaded, 1, 2, false,
			[]string{"1 cosigner key card", "2 cosigner key cards", "me sysw pack"}},
		{"a chunk set that never completed", cosignerSourceLoaded, 1, 2, true,
			[]string{"missing some of its chunks", "me sysw pack"}},
		{"no payload at all", cosignerSourceNoPayload, 0, 2, false,
			[]string{"No payload is loaded", "no card reader", "me sysw pack"}},
		{"payload not compared", cosignerSourceUncompared, 0, 2, false,
			[]string{"has not been checked", "digest"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := classifyCosignerSupply(tc.state, tc.have, tc.open); got != cosignerRefuse {
				t.Fatalf("classified as %v, not a refusal — the message below would "+
					"never be shown", got)
			}
			msg := buildSupplyRefusal(tc.state, tc.have, tc.open, tc.incomplete)
			for _, want := range tc.want {
				if !strings.Contains(msg, want) {
					t.Errorf("refusal does not mention %q:\n%s", want, msg)
				}
			}
			// The one thing no phase-1 refusal may say.
			if strings.Contains(strings.ToLower(msg), "scan") {
				t.Errorf("the refusal tells the operator to scan; phase-1 hardware "+
					"has no reader, so this sends them looking for one:\n%s", msg)
			}
			// And it may not read as "0 cosigner key cards" / "1 cards".
			if strings.Contains(msg, "0 cosigner") || strings.Contains(msg, "1 cosigner key cards") {
				t.Errorf("ungrammatical count in an operator-facing refusal:\n%s", msg)
			}
		})
	}
}

// S1 test 8: the operator's `0..n` ruling as a test, because a ruling with no
// test is a sentence.
//
// Over the product of n ∈ 2..5 (multisigNChoices) and a payload carrying 0..n
// cards: every combination either ASSEMBLES or REFUSES BY NAME, and none falls
// through. Then the mutation, RUN rather than described: restoring the
// exact-count refusal on the feed turns the n=3 rows red — that is the state the
// payload actually shipped in, and it made Trace A unreachable while every unit
// test stayed green.
func TestPayloadCardCountIsIndependentOfN(t *testing.T) {
	ns := make([]int, 0, len(multisigNChoices()))
	for i := range multisigNChoices() {
		ns = append(ns, multisigNFor(i))
	}
	if len(ns) == 0 {
		t.Fatal("INCONCLUSIVE: the n picker offers nothing, so the matrix is empty")
	}

	assembled, refused := 0, 0
	for _, n := range ns {
		open := n - 1
		for c := 0; c <= n; c++ {
			t.Run(fmt.Sprintf("n=%d/cards=%d", n, c), func(t *testing.T) {
				supply, incomplete := buildCosignerSupply(cosignerCardRecords(t, c))
				if len(supply) != c {
					t.Fatalf("INCONCLUSIVE: %d records assembled %d cards, want %d",
						c, len(supply), c)
				}
				switch got := classifyCosignerSupply(cosignerSourceLoaded, c, open); got {
				case cosignerRefuse:
					refused++
					if c >= open {
						t.Fatalf("refused with %d cards for %d slots; a payload that "+
							"CAN fill the slots must never be refused", c, open)
					}
					msg := buildSupplyRefusal(cosignerSourceLoaded, c, open, incomplete)
					if msg == "" || !strings.Contains(msg, "me sysw pack") {
						t.Errorf("the refusal does not name a route:\n%s", msg)
					}
				case cosignerAutoFill, cosignerSelect:
					assembled++
					if (got == cosignerAutoFill) != (c == open) {
						t.Errorf("classified %v with %d cards for %d slots", got, c, open)
					}
					// Bounded selection yields exactly `open` cards, in record
					// order, and the constructor accepts them.
					picked := supply[:open]
					cards, ok := buildCosignerCards(picked, open)
					if !ok {
						t.Fatalf("the resolved set of %d was refused by the "+
							"constructor — this is the fall-through the ruling forbids",
							open)
					}
					if len(cards) != open {
						t.Fatalf("constructor returned %d cards for %d slots",
							len(cards), open)
					}
				default:
					t.Fatalf("unclassified outcome %v — the case analysis must be "+
						"total or the n-1 assumption returns as a fall-through", got)
				}
			})
		}
	}
	if assembled == 0 || refused == 0 {
		t.Fatalf("the matrix produced %d assembling and %d refusing cells; both arms "+
			"must be exercised or one of them is untested", assembled, refused)
	}
	t.Logf("%d cells assembled, %d refused by name, 0 fell through", assembled, refused)

	// THE MUTATION, executed. The feed-side exact-count refusal is what shipped;
	// under it, Trace A (n=3, the delivered 4-card payload) refuses.
	mutant := func(have, open int) buildCosignerOutcome {
		if have != open {
			return cosignerRefuse
		}
		return cosignerAutoFill
	}
	red := 0
	// 0..4, not 0..3: the DELIVERED payload carries four cards, and n=3 is Trace
	// A. That cell is the one the defect actually landed on.
	for c := 0; c <= 4; c++ {
		open := 2 // n=3
		live := classifyCosignerSupply(cosignerSourceLoaded, c, open)
		if mutant(c, open) != live {
			red++
		}
	}
	if red == 0 {
		t.Error("restoring the feed-side exact-count refusal changed NO n=3 row, so " +
			"this matrix would have stayed green through the defect it exists to catch")
	}
	if mutant(4, 2) != cosignerRefuse {
		t.Fatal("INCONCLUSIVE: the mutant does not refuse the delivered payload's " +
			"4-cards-for-2-slots case, so it is not the defect being reproduced")
	}
	if classifyCosignerSupply(cosignerSourceLoaded, 4, 2) == cosignerRefuse {
		t.Error("the LIVE classifier still refuses 4 cards for 2 slots — Trace A is " +
			"unreachable and the ruling is not implemented")
	}
	t.Logf("mutation: %d of 5 n=3 rows flip when the feed-side exact-count refusal "+
		"is restored", red)
}

// The bounded-selection walk itself, driven through the real screens on the
// FLAGSHIP case: Trace A's shape (n=3, two open slots) against a payload
// carrying four cards. This is the cell the `0..n` ruling exists for, and the
// matrix above proves it classifies — this proves an operator can actually walk
// it.
func TestBuildOverSupplySelectionIsWalkable(t *testing.T) {
	records := cosignerCardRecords(t, 4)
	synctest.Test(t, func(t *testing.T) {
		seedTyped := false
		buildMultisigSeedHook = func(bip39.Mnemonic) { seedTyped = true }
		defer func() { buildMultisigSeedHook = nil }()
		ctx := NewContext(newPlatform())
		ctx.sysw = sessionHolding(records...)
		frame, quit := runUI(ctx, func() { buildMultisigPolicyFlow(ctx, &descriptorTheme) })
		defer quit()
		buildWalkToGather(t, ctx, frame, 3)
		if c, ok := pumpUntil(frame, "mk1 keys: 4", 32); !ok {
			t.Fatalf("all four payload cards did not reach the gather; screen reads %q", c)
		}
		click(&ctx.Router, Button3) // Done adding cards

		// The read-only list of what the payload supplied — the picker that
		// follows refers to cards by these numbers.
		c, ok := pumpUntil(frame, "Payload cards", 32)
		if !ok {
			t.Fatalf("over-supply did not show what the payload supplied; screen reads %q", c)
		}
		if !uiContains(c, "open slot") {
			t.Errorf("the payload-card list never says how many slots are open: %q", c)
		}
		click(&ctx.Router, Button3) // continue to the picker

		// Card 1 of 4: USE (choice 0 is the default).
		if c, ok := pumpUntil(frame, "Use payload card 1 of 4", 32); !ok {
			t.Fatalf("no per-card selection screen; screen reads %q", c)
		}
		click(&ctx.Router, Button3)
		frame()
		// Card 2 of 4: SKIP.
		if c, ok := pumpUntil(frame, "Use payload card 2 of 4", 32); !ok {
			t.Fatalf("selection did not advance to card 2; screen reads %q", c)
		}
		click(&ctx.Router, Down)
		frame()
		click(&ctx.Router, Button3)
		frame()
		// Two cards remain for one open slot, so card 3 is still a choice.
		if c, ok := pumpUntil(frame, "Use payload card 3 of 4", 32); !ok {
			t.Fatalf("selection did not advance to card 3; screen reads %q", c)
		}
		click(&ctx.Router, Button3) // USE -> the set is full
		if c, ok := pumpUntil(frame, "Seed", 32); !ok {
			t.Fatalf("a completed selection did not reach seed entry; screen reads %q", c)
		}
		if seedTyped {
			t.Fatal("the seed hook fired before the operator typed anything")
		}
	})
}

// The other half of the bound: when the cards that REMAIN are exactly the slots
// that remain, they are taken without asking. A question with one possible
// answer is not a choice, and asking it is how an operator skips their way into
// an under-supply that was never real.
func TestBoundedSelectionCannotEndShort(t *testing.T) {
	supply, _ := buildCosignerSupply(cosignerCardRecords(t, 4))
	if len(supply) != 4 {
		t.Fatalf("INCONCLUSIVE: assembled %d cards, want 4", len(supply))
	}
	synctest.Test(t, func(t *testing.T) {
		ctx := NewContext(newPlatform())
		var got []int
		var ok bool
		done := false
		frame, quit := runUI(ctx, func() {
			got, ok = buildCosignerPickFlow(ctx, &descriptorTheme, supply, 2)
			done = true
		})
		defer quit()
		if c, found := pumpUntil(frame, "Payload cards", 16); !found {
			t.Fatalf("no payload-card list; screen reads %q", c)
		}
		click(&ctx.Router, Button3)
		// SKIP cards 1 and 2. That leaves cards 3 and 4 for two slots, so the
		// picker must stop asking and take both.
		for i := 1; i <= 2; i++ {
			if c, found := pumpUntil(frame, fmt.Sprintf("Use payload card %d of 4", i), 16); !found {
				t.Fatalf("card %d not offered; screen reads %q", i, c)
			}
			click(&ctx.Router, Down)
			frame()
			click(&ctx.Router, Button3)
			frame()
		}
		for i := 0; i < 32 && !done; i++ {
			frame()
		}
		if !done {
			t.Fatal("the picker never returned; it is still asking about a card set " +
				"that has only one possible answer")
		}
		if !ok {
			t.Fatal("the picker abandoned rather than taking the only remaining set")
		}
		want := []int{2, 3}
		if len(got) != len(want) {
			t.Fatalf("picked %v, want %v", got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("picked %v, want %v — the remainder must be taken in "+
					"payload record order", got, want)
			}
		}
	})
}
