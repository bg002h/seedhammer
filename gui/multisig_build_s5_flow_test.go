package gui

import (
	"strings"
	"testing"
	"testing/synctest"

	"seedhammer.com/mk"
)

// ─── S5 test 8: the gate, re-proven through the REAL post-rewire flow ────────
//
// WHY IT MUST BE RE-PROVEN AT ALL. S2's interim foreign-origin refusal meant a
// divergent-origin input could not REACH the gate during S4, so every S4 gate
// test is necessarily synthetic -- they drive buildSlotGate directly, and the
// file says so. S5 removes that refusal and rewires the origins the gate derives
// against, so S5 is the first stage where the gate runs for real. If it is not
// re-proven here, S4's proof expires silently.
//
// THE SPECIFICITY IS THE POINT. "The gate still fires" is satisfiable by
// `assemble(divergentInput); assertNoError()` -- a smoke test that never checks
// WHICH origin the gate derived against, which is the binding the whole check
// exists to protect. So this is a PROCEED/FAIL pair on ONE fixture: the same
// `both` slot declaring m/48'/0'/1'/2', proceeding when the key is honestly
// derived there and failing (naming the slot) when it is not. A gate that
// derived at multisigSharedOrigin() instead fails the PROCEED arm; that is the
// mutation this test is checked with.

// s5LiarCard is S4's TestGateDerivesAtTheCardsOwnOrigin fixture: card A@1's
// DECLARED origin m/48h/0h/1h/2h carrying the key from m/48h/0h/0h/2h. The
// fingerprint is untouched and honest (same master), so only the origin binding
// can separate the two arms.
func s5LiarCard(t *testing.T) mk.Card {
	t.Helper()
	a1 := dupTestCard(t, 3) // A@1, declaring m/48h/0h/1h/2h
	a0 := dupTestCard(t, 0) // A@0, the key from the shared origin
	if a1.Path == a0.Path {
		t.Fatalf("both fixture cards declare %s, so this test cannot tell the two "+
			"origins apart", a1.Path)
	}
	liar := a1
	liar.Xpub = a0.Xpub
	return liar
}

// s5PayloadRecords encodes cards into the flat record list a payload carries,
// cards contiguous and in the given order.
func s5PayloadRecords(t *testing.T, cards ...mk.Card) []string {
	t.Helper()
	var out []string
	for i, c := range cards {
		strs, err := mk.Encode(c)
		if err != nil {
			t.Fatalf("mk.Encode(card %d): %v", i, err)
		}
		if len(strs) < 2 {
			t.Fatalf("card %d encoded to %d chunk(s); the gatherer needs chunked cards", i, len(strs))
		}
		out = append(out, strs...)
	}
	return out
}

// s5DriveToGate drives the Build flow from the pickers up to the passphrase
// prompt, with the operator's own slot @0 answered as `both` and the self seed
// typed as master A. The NEXT screen is the gate's verdict: S4's slot-source
// review on a pass, the FAIL modal on a refusal -- the gate runs at step (4),
// BEFORE the review at (4a), so the caller pumps for whichever it expects.
//
// Every screen is asserted on the way past: tapping through a picker that did
// not appear silently answers the NEXT one, and a walk that does that reports a
// pass for a policy nobody asked for.
func s5DriveToGate(t *testing.T, ctx *Context, frame func() (string, bool)) {
	t.Helper()
	buildWalkParamPickers(t, ctx, frame) // wsh, n=2, k=1, @0, fp Omit

	// S4's ONE slot-source question, answered YES: the operator's @0 key is on a
	// payload card too. That is the `both` assignment, and the only shape the gate
	// fires on.
	if c, ok := pumpUntil(frame, "key on a card?", 32); !ok {
		t.Fatalf("the `both` question was not asked; got %q", c)
	}
	click(&ctx.Router, Down) // "YES, CHECK THE CARD"
	frame()
	click(&ctx.Router, Button3)
	frame()

	if c, ok := pumpUntil(frame, "mk1 keys: 2", 32); !ok {
		t.Fatalf("the payload's two cards never reached the gather; got %q", c)
	}
	click(&ctx.Router, Button3) // Done adding cards
	frame()
	if c, ok := pumpUntil(frame, "Payload cards", 32); !ok {
		t.Fatalf("the payload review never appeared; got %q", c)
	}
	click(&ctx.Router, Button3)
	frame()

	if c, ok := pumpUntil(frame, "Choose number of words", 32); !ok {
		t.Fatalf("typed seed entry not reached; got %q", c)
	}
	click(&ctx.Router, Button3) // 12 WORDS
	frame()
	driveWords(&ctx.Router, fixtureMasterA)
	if c, ok := pumpUntil(frame, "passphrase", 160); !ok {
		t.Fatalf("the passphrase prompt was not reached; got %q", c)
	}
	click(&ctx.Router, Button3) // Skip
}

// s5PageForNeedle pages a confirmReviewScreen (Button2) looking for `needle`,
// and returns every page it saw.
//
// It exists because a review screen is a PAGER: asserting on one frame proves
// only that the line is on page 1, and a line the operator has to page to is
// still a line the flow produced. The loop is bounded and stops when the pager
// wraps to the top (start == 0 again), so it cannot spin.
func s5PageForNeedle(t *testing.T, ctx *Context, frame func() (string, bool), needle string, pages int) (string, bool) {
	t.Helper()
	var seen []string
	first := ""
	for i := 0; i < pages; i++ {
		c, ok := frame()
		if !ok {
			break
		}
		if uiContains(c, needle) {
			return strings.Join(append(seen, c), " || "), true
		}
		if i == 0 {
			first = c
		} else if c == first && i > 1 {
			break // the pager wrapped
		}
		seen = append(seen, c)
		click(&ctx.Router, Button2)
	}
	return strings.Join(seen, " || "), false
}

// TestBuildFlowAnnouncesTwoSlotsFromOneSeed is C-2's FLOW-level arm, and it is
// the one that matters: the gate test beside it drives buildSlotGate directly,
// and the defect was that the flow builds a registry the gate's grouping key
// could not see. A unit test of the gate cannot observe that.
//
// The operator builds a 2-of-3 holding @0 and @1 and types the SAME twelve words
// for both. That is a legitimate multi-account wallet -- buildSlotSources
// diverges the accounts off the master fingerprint, so the two slots carry
// DIFFERENT keys -- and the tail correctly cuts ONE ms1 for the shared master.
// SPEC 4.3 row 5's notice on the Key-sources review is the only surface anywhere
// in the flow that tells the operator those two slots share one secret. Without
// it they read "@0 yours: derived from your seed for @0" and "@1 yours: derived
// from your seed for @1, account 1" -- two labels that read as two secrets --
// and a lost ms1 plate takes both keys with it.
func TestBuildFlowAnnouncesTwoSlotsFromOneSeed(t *testing.T) {
	records := s5PayloadRecords(t, dupTestCard(t, 1)) // B@0: the one open slot's cosigner
	synctest.Test(t, func(t *testing.T) {
		p := newPlatform()
		p.display = sh2DisplaySize
		ctx := NewContext(p)
		ctx.sysw = sessionHolding(records...)
		frame, quit := runUI(ctx, func() { buildMultisigPolicyFlow(ctx, &descriptorTheme) })
		defer quit()

		// The pickers: wsh (default), n=3, k=2, @0, YES one more, @1, no more, fp Omit.
		pick := func(stage string, downs int) {
			t.Helper()
			if c, ok := pumpUntil(frame, stage, 32); !ok {
				t.Fatalf("the %q picker was not shown; the flow is on %q", stage, c)
			}
			for d := 0; d < downs; d++ {
				click(&ctx.Router, Down)
				frame()
			}
			click(&ctx.Router, Button3)
			frame()
		}
		pick("Template", 0)
		pick("Cosigners", 1)                  // n = 3
		pick("Threshold", 1)                  // k = 2
		pick("Which slot is your key?", 0)    // @0
		pick("Do you hold another slot?", 1)  // YES, ONE MORE
		pick("Which other slot is yours?", 0) // @1
		pick("Do you hold another slot?", 0)  // NO, THAT IS ALL
		pick("Fingerprints", 0)

		if c, ok := pumpUntil(frame, "mk1 keys: 1", 32); !ok {
			t.Fatalf("the payload's cosigner card never reached the gather; got %q", c)
		}
		click(&ctx.Router, Button3) // Done adding cards
		frame()
		if c, ok := pumpUntil(frame, "Payload cards", 32); !ok {
			t.Fatalf("the payload review never appeared; got %q", c)
		}
		click(&ctx.Router, Button3)
		frame()

		// ONE MASTER, TYPED TWICE -- once per held slot, which is what the flow
		// asks for and what mints two registry entries with two different SeedIDs.
		for _, slot := range []string{"@0", "@1"} {
			if c, ok := pumpUntil(frame, "Choose number of words", 64); !ok {
				t.Fatalf("seed entry for %s was not reached; got %q", slot, c)
			}
			click(&ctx.Router, Button3) // 12 WORDS
			frame()
			driveWords(&ctx.Router, fixtureMasterA)
			if c, ok := pumpUntil(frame, "passphrase", 240); !ok {
				t.Fatalf("the passphrase prompt for %s was not reached; got %q", slot, c)
			}
			click(&ctx.Router, Button3) // Skip
			frame()
		}

		if c, ok := pumpUntil(frame, "Key sources", 64); !ok {
			t.Fatalf("the gate REFUSED a legitimate two-account build, or the review never "+
				"appeared; screen reads %q", c)
		}
		pagesSeen, found := s5PageForNeedle(t, ctx, frame, "multi-account wallet", 12)
		if !found {
			t.Fatalf("the Key-sources review NEVER says the two held slots come from ONE "+
				"seed.\nPages drawn: %q\n"+
				"The operator reads two derived-from-your-seed lines and takes them for two "+
				"independent keys; the run cuts ONE ms1, so losing that plate loses BOTH "+
				"slots and the wallet is permanently unspendable", pagesSeen)
		}
		for _, want := range []string{"@0", "@1"} {
			if !strings.Contains(pagesSeen, want) {
				t.Errorf("the notice does not name %s: %q", want, pagesSeen)
			}
		}
	})
}

// TestGateStillFiresAfterOriginsDiverge is plan test 8.
func TestGateStillFiresAfterOriginsDiverge(t *testing.T) {
	a1 := dupTestCard(t, 3) // master A at m/48h/0h/1h/2h -- the HONEST card
	b0 := dupTestCard(t, 1) // master B at the shared origin -- the other cosigner

	t.Run("PROCEED when the key is genuinely derived at the card's own origin", func(t *testing.T) {
		records := s5PayloadRecords(t, a1, b0)
		synctest.Test(t, func(t *testing.T) {
			p := newPlatform()
			p.display = sh2DisplaySize
			ctx := NewContext(p)
			ctx.sysw = sessionHolding(records...)
			frame, quit := runUI(ctx, func() { buildMultisigPolicyFlow(ctx, &descriptorTheme) })
			defer quit()
			s5DriveToGate(t, ctx, frame)

			// THE GATE PASSED: its verdict screen is S4's slot-source review, not a
			// refusal modal.
			if c, ok := pumpUntil(frame, "Key sources", 64); !ok {
				t.Fatalf("the gate did not PROCEED on a card declaring %s and carrying the "+
					"key that seed derives there; screen reads %q", a1.Path, c)
			}
			click(&ctx.Router, Button3)

			// And the assembler accepted a policy whose two slots declare DIFFERENT
			// origins. Before S5 this exact input was refused by S2's interim "Key
			// origin mismatch", one screen later.
			content, ok := pumpUntil(frame, "Policy stub", 64)
			if !ok {
				t.Fatalf("a `both` slot whose card declares %s, carrying the key that seed "+
					"derives AT %s, did not reach the policy review; got %q.\n"+
					"A gate deriving at %s instead would fail exactly here",
					a1.Path, a1.Path, content, multisigSharedOrigin())
			}
			// The review must NOT be reporting the shared origin for a build whose
			// slots diverge -- that would mean the origins never diverged at all.
			if !uiContains(content, "Policy stub") {
				t.Fatalf("the policy review frame does not carry the stub: %q", content)
			}
		})
	})

	t.Run("FAIL naming the slot when the key was derived somewhere else", func(t *testing.T) {
		records := s5PayloadRecords(t, s5LiarCard(t), b0)
		synctest.Test(t, func(t *testing.T) {
			p := newPlatform()
			p.display = sh2DisplaySize
			ctx := NewContext(p)
			ctx.sysw = sessionHolding(records...)
			done := false
			frame, _, ink, quit := runUITouchRaster(ctx, func() {
				buildMultisigPolicyFlow(ctx, &descriptorTheme)
				done = true
			})
			defer quit()
			s5DriveToGate(t, ctx, frame)

			content, ok := pumpUntil(frame, "Key does not match seed", 64)
			if !ok {
				t.Fatalf("a card declaring %s while carrying the key from %s was ACCEPTED. "+
					"Its mk1 would assert membership in a wallet at a path its own key is "+
					"not at. Screen reads %q", a1.Path, dupTestCard(t, 0).Path, content)
			}
			// NAMES THE SLOT, and QUOTES THE CARD'S OWN ORIGIN. The second half is
			// what makes this more than a smoke test: the refusal must be about the
			// origin the gate actually derived against, and a gate that derived at
			// the shared origin must not be able to launder that into its message.
			for _, want := range []string{"@0", a1.Path} {
				if !uiContains(content, want) {
					t.Errorf("the FAIL screen does not carry %q, so the operator cannot tell "+
						"which slot or which path disagreed:\n%q", want, content)
				}
			}
			if uiContains(content, multisigSharedOrigin().String()) {
				t.Errorf("the FAIL screen quotes the SHARED origin %s rather than the card's "+
					"own %s:\n%q", multisigSharedOrigin(), a1.Path, content)
			}
			if ink() < buildWalkRasterFloor {
				t.Errorf("the gate's FAIL screen drew only %d ink pixels (floor %d): the "+
					"operator is stopped by a screen with no body", ink(), buildWalkRasterFloor)
			}
			t.Logf("gate FAIL screen: ink = %d px", ink())
			click(&ctx.Router, Button3)
			for i := 0; i < 64 && !done; i++ {
				frame()
			}
			if !done {
				t.Fatal("the flow did not return after the gate refusal")
			}
		})
	})

	// The two arms must be DIFFERENT INPUTS to the same flow, or the pair proves
	// nothing: assert the fixture actually differs only in the xpub.
	liar := s5LiarCard(t)
	if liar.Path != a1.Path || liar.Fingerprint != a1.Fingerprint {
		t.Fatalf("the FAIL fixture differs from the PROCEED fixture in more than the "+
			"key (path %q vs %q, fp %q vs %q), so the pair does not isolate the origin "+
			"binding", liar.Path, a1.Path, liar.Fingerprint, a1.Fingerprint)
	}
	if liar.Xpub == a1.Xpub {
		t.Fatal("the FAIL fixture carries the honest key, so both arms are the same input")
	}
	if !strings.HasPrefix(liar.Path, "m/48") {
		t.Fatalf("the fixture's declared origin %q is not a BIP-48 path", liar.Path)
	}
}
