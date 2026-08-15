package gui

import (
	"errors"
	"strings"
	"testing"
	"testing/synctest"

	"github.com/btcsuite/btcd/chaincfg/v2"
	"seedhammer.com/bip39"
	"seedhammer.com/md"
	"seedhammer.com/mk"
)

// ─── S2's FIRST landing: SPEC §4.1's duplicate-key refusal ───────────────────
//
// THE WINDOW IS LIVE IN THE TREE, not from some future stage. F-178 recorded a
// hand-drive that reached the engraving-style picker and read it as evidence the
// flow was healthy; the re-read found the policy on those screens was a "2-of-3"
// holding masterA's account-0 key at BOTH @0 (self, seed from the payload) and
// @1 (payload card A@0), with masterA account-1 at @2. One master holds the whole
// wallet, and the duplicated key alone satisfies k=2. Fingerprints are omitted by
// default, so every slot rendered "(no fp)" and the degradation is invisible in
// every artifact the operator keeps — §0.1 clause 2's refuse side exactly.
//
// So this test and its check land BEFORE any S2 work that completes an engrave.

// dupTestCard decodes roster card `i` (see cosignerCardRoster) into the mk.Card
// the gather hands assembleBuildPolicy. Index 0 is A@0, 1 is B@0, 2 is C@0,
// 3 is A@1 — the same four cards cmd/buildpayloadcards writes into the emulator
// payload, derived through the device's own path.
func dupTestCard(t *testing.T, i int) mk.Card {
	t.Helper()
	sets := cosignerCardFixtures(t, i+1)
	card, err := mk.Decode(sets[i])
	if err != nil {
		t.Fatalf("decoding roster card %d: %v", i, err)
	}
	return card
}

// dupTestSelf derives a self key exactly as step (4) of the Build flow does.
func dupTestSelf(t *testing.T, phrase string) (xpub string, masterFP uint32) {
	t.Helper()
	m, err := bip39.ParseMnemonic(phrase)
	if err != nil {
		t.Fatalf("ParseMnemonic(%.20s...): %v", phrase, err)
	}
	x, fp, err := deriveAccountXpub(m, "", &chaincfg.MainNetParams, multisigSharedOrigin())
	if err != nil {
		t.Fatalf("deriveAccountXpub(%.20s...): %v", phrase, err)
	}
	return x, fp
}

// TestS2RefusesDuplicateKeysBeforeS4 is SPEC §4.1's PERMANENT final-slot-set
// check, landed early. S4 does not replace it — it feeds it more sources.
func TestS2RefusesDuplicateKeysBeforeS4(t *testing.T) {
	// THE DELIVERED COLLISION. Self is masterA typed (or taken from the
	// payload's own ClassMnemonic, which is masterA), derived at
	// multisigSharedOrigin(); payload card A@0 is masterA at that same origin.
	// Same master, same path, same key.
	t.Run("the delivered collision is refused", func(t *testing.T) {
		selfXpub, selfFP := dupTestSelf(t, fixtureMasterA)
		p := buildPolicyParams{Script: md.MultisigWsh, N: 3, K: 2, SelfSlot: 0}
		out, _, _, err := assembleBuildPolicy(p, selfXpub, selfFP,
			[]mk.Card{dupTestCard(t, 0), dupTestCard(t, 1)}) // A@0 then B@0
		if err == nil {
			t.Fatal("assembleBuildPolicy accepted a policy whose slots @0 and @1 hold " +
				"the SAME key: one master can spend a 2-of-3 alone, and with " +
				"fingerprints omitted no artifact shows it")
		}
		var dup errBuildDuplicateKey
		if !errors.As(err, &dup) {
			t.Fatalf("refused with %v; want an errBuildDuplicateKey the flow can name "+
				"slots from, not a generic assembly failure", err)
		}
		if dup.SlotA != 0 || dup.SlotB != 1 {
			t.Errorf("named slots @%d and @%d; want @0 (self) and @1 (payload card 1)",
				dup.SlotA, dup.SlotB)
		}
		// NOTHING WAS PRODUCED. The refusal must precede md.EncodeMultisig, or a
		// caller that ignored the error would engrave the degraded policy.
		if out != nil {
			t.Errorf("md1 chunks were produced alongside the refusal: %v", out)
		}
	})

	// THE COLLISION IS BYTE-EQUALITY, not a heuristic. Asserted directly so a
	// future edit that loosened the comparison to something merely correlated
	// (a fingerprint, a truncated hash) fails here rather than in the field.
	t.Run("the collision is byte-identical cc||pk", func(t *testing.T) {
		selfXpub, _ := dupTestSelf(t, fixtureMasterA)
		selfCC, selfPK, _, err := decodeXpubBytes(selfXpub)
		if err != nil {
			t.Fatalf("decodeXpubBytes(self): %v", err)
		}
		a0, err := cosignerFromCard(dupTestCard(t, 0), false) // A@0
		if err != nil {
			t.Fatalf("cosignerFromCard(A@0): %v", err)
		}
		if selfCC != a0.ChainCode || selfPK != a0.CompressedPubkey {
			t.Fatalf("the delivered payload no longer collides with the self key: "+
				"self cc %x pk %x vs card A@0 cc %x pk %x.\nThis test's whole premise "+
				"is that masterA's mnemonic and card A@0 coexist on the payload "+
				"(cmd/buildpayloadcards/main.go); if that changed, S4's honest `both` "+
				"fixture changed with it", selfCC, selfPK, a0.ChainCode, a0.CompressedPubkey)
		}
	})

	// TRACE B MUST SURVIVE. One master contributing account 0 and account 1 as
	// two cosigners is a legitimate wallet, and it is the shape the delivered
	// payload carries as cards A@0 and A@1. A check keyed on the MASTER would
	// refuse it — so this arm is the proof that the ruled basis is cc||pk and not
	// something coarser.
	t.Run("Trace B's multi-account shape is not a duplicate", func(t *testing.T) {
		a0, err := cosignerFromCard(dupTestCard(t, 0), false) // A@0
		if err != nil {
			t.Fatalf("cosignerFromCard(A@0): %v", err)
		}
		a1, err := cosignerFromCard(dupTestCard(t, 3), false) // A@1 (same master, account 1)
		if err != nil {
			t.Fatalf("cosignerFromCard(A@1): %v", err)
		}
		if a0.ChainCode == a1.ChainCode {
			t.Error("A@0 and A@1 share a chain code; the roster is not the " +
				"multi-account shape this arm needs")
		}
		if a0.CompressedPubkey == a1.CompressedPubkey {
			t.Error("A@0 and A@1 share a pubkey; the roster is not the " +
				"multi-account shape this arm needs")
		}
		selfC, _ := dupTestSelf(t, fixtureMasterC)
		cc, pk, _, err := decodeXpubBytes(selfC)
		if err != nil {
			t.Fatalf("decodeXpubBytes(masterC): %v", err)
		}
		all := []md.MultisigCosigner{
			{ChainCode: cc, CompressedPubkey: pk}, a0, a1,
		}
		if i, j, dup := duplicateSlotPair(all); dup {
			t.Fatalf("slots @%d and @%d called duplicates in a legitimate "+
				"multi-account 2-of-3 (masterC, masterA account 0, masterA account 1)",
				i, j)
		}

		// The rejected alternative, measured rather than argued: A@0 and A@1
		// carry the SAME master fingerprint, so a fingerprint-keyed check would
		// have refused the wallet above.
		c0, c1 := dupTestCard(t, 0), dupTestCard(t, 3)
		if c0.Fingerprint == "" || c0.Fingerprint != c1.Fingerprint {
			t.Fatalf("A@0 fp %q, A@1 fp %q: they must be equal and non-empty, or the "+
				"master-fingerprint alternative was never the trap this claims",
				c0.Fingerprint, c1.Fingerprint)
		}
	})

	// The SELF slot is not special: a build whose two PAYLOAD cards are the same
	// key is refused too, and the message names two cards rather than a card and
	// a seed.
	t.Run("card-vs-card duplicates are refused and named as cards", func(t *testing.T) {
		selfXpub, selfFP := dupTestSelf(t, fixtureMasterC)
		p := buildPolicyParams{Script: md.MultisigWsh, N: 3, K: 2, SelfSlot: 2}
		_, _, _, err := assembleBuildPolicy(p, selfXpub, selfFP,
			[]mk.Card{dupTestCard(t, 0), dupTestCard(t, 0)}) // A@0 twice
		var dup errBuildDuplicateKey
		if !errors.As(err, &dup) {
			t.Fatalf("two copies of card A@0 assembled without a duplicate refusal: %v", err)
		}
		if dup.SlotA != 0 || dup.SlotB != 1 {
			t.Fatalf("named @%d and @%d; want @0 and @1", dup.SlotA, dup.SlotB)
		}
		origins := buildCosignerOrigins(3, 2, []int{0, 0})
		msg := buildDuplicateKeyMessage(dup, 2, origins)
		for _, want := range []string{"payload card 1", "Slot @0", "slot @1", "me sysw pack"} {
			if !strings.Contains(msg, want) {
				t.Errorf("card-vs-card refusal is missing %q:\n%s", want, msg)
			}
		}
		if strings.Contains(msg, "your seed") {
			t.Errorf("a card-vs-card duplicate blamed the operator's seed:\n%s", msg)
		}
	})

	// The refusal text is the operator's whole instruction set, so it is asserted
	// rather than trusted: which slots, where each came from, that nothing was
	// cut, and the two routes that exist on this hardware (§0.1b). It must NOT
	// say "scan" — a prompt pointing away from the only route the flow takes
	// dead-ends the operator just as effectively as an impossible one.
	t.Run("the refusal names both slots, their provenance and the way forward", func(t *testing.T) {
		origins := buildCosignerOrigins(3, 0, []int{0, 1}) // @1 <- card 1, @2 <- card 2
		msg := buildDuplicateKeyMessage(errBuildDuplicateKey{SlotA: 0, SlotB: 1}, 0, origins)
		for _, want := range []string{
			"Slot @0 (your key, from your seed)",
			"slot @1 (payload card 1)",
			"SAME key",
			"Nothing was engraved",
			"me sysw pack",
		} {
			if !strings.Contains(msg, want) {
				t.Errorf("refusal is missing %q:\n%s", want, msg)
			}
		}
		if strings.Contains(msg, "scan") || strings.Contains(msg, "Scan") {
			t.Errorf("the refusal tells the operator to scan, which is not the route "+
				"this phase offers:\n%s", msg)
		}
		// F-78: the em-dash draws as nothing in the body face, so a refusal built
		// around one loses its punctuation on the machine.
		if strings.Contains(msg, "\u2014") {
			t.Errorf("the refusal contains an em-dash, a zero-pixel glyph in "+
				"poppins.Regular16:\n%s", msg)
		}
	})
}

// TestBuildFlowRefusesDuplicateBeforeReview drives the WHOLE flow to prove the
// two placement rulings behaviourally, not structurally: the refusal is reached
// on a real build, and the Policy Review screen is never drawn.
//
// The shape is the smallest one that collides on the delivered material: all
// five picker defaults (wsh, n=2, k=1, self @0, fp Omit) with ONE payload card,
// A@0, and the self seed typed as masterA. n=2 leaves one open slot, so the
// payload auto-fills it and the operator's own key lands at @0 — the same key,
// twice, in a 1-of-2.
func TestBuildFlowRefusesDuplicateBeforeReview(t *testing.T) {
	records := cosignerCardRecords(t, 1) // card A@0 only
	synctest.Test(t, func(t *testing.T) {
		ctx := NewContext(newPlatform())
		ctx.sysw = sessionHolding(records...)
		done := false
		frame, quit := runUI(ctx, func() {
			buildMultisigPolicyFlow(ctx, &descriptorTheme)
			done = true
		})
		defer quit()
		buildWalkParamPickers(t, ctx, frame)

		// The gather, fed from the payload. Done adding cards.
		if c, ok := pumpUntil(frame, "mk1 keys: 1", 32); !ok {
			t.Fatalf("the payload's card never reached the gather; got %q", c)
		}
		click(&ctx.Router, Button3)
		frame()

		// The payload review (spec P0 item 6's auto-fill arm), then straight on.
		if c, ok := pumpUntil(frame, "Payload cards", 32); !ok {
			t.Fatalf("the auto-fill payload review never appeared; got %q", c)
		}
		click(&ctx.Router, Button3)
		frame()

		// Seed entry. The payload holds no ClassMnemonic, so the source picker is
		// a menu of one and is skipped: the word-count picker is the next screen.
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

		content, ok := pumpUntil(frame, "Duplicate key", 64)
		if !ok {
			t.Fatalf("a build whose two slots hold one key did not refuse; got %q", content)
		}
		if !strings.Contains(strings.ToLower(strings.ReplaceAll(content, " ", "")),
			strings.ToLower(strings.ReplaceAll("your key, from your seed", " ", ""))) {
			t.Errorf("the refusal did not name the self slot's provenance: %q", content)
		}
		click(&ctx.Router, Button3) // dismiss -> the flow returns
		for i := 0; i < 64 && !done; i++ {
			frame()
		}
		if !done {
			t.Fatal("the flow did not return after the duplicate refusal")
		}
	})
}

// TestBuildFlowDuplicateNeverReachesReview is the other half of the placement
// ruling, and it is a NEGATIVE so it gets its own name: the review screen is a
// surface the operator reads as confirmation, and a duplicate that reaches it has
// already been implicitly blessed. Driven as one pass over every frame the flow
// produced, because "the modal appeared" does not prove the review did not.
func TestBuildFlowDuplicateNeverReachesReview(t *testing.T) {
	records := cosignerCardRecords(t, 1)
	synctest.Test(t, func(t *testing.T) {
		ctx := NewContext(newPlatform())
		ctx.sysw = sessionHolding(records...)
		frame, quit := runUI(ctx, func() { buildMultisigPolicyFlow(ctx, &descriptorTheme) })
		defer quit()
		var seen []string
		pump := func(want string, n int) string {
			c, ok := pumpUntil(func() (string, bool) {
				c, ok := frame()
				if ok {
					seen = append(seen, c)
				}
				return c, ok
			}, want, n)
			if !ok {
				t.Fatalf("never reached %q; last screen %q", want, c)
			}
			return c
		}
		buildWalkParamPickers(t, ctx, frame)
		pump("mk1 keys: 1", 32)
		click(&ctx.Router, Button3)
		frame()
		pump("Payload cards", 32)
		click(&ctx.Router, Button3)
		frame()
		pump("Choose number of words", 32)
		click(&ctx.Router, Button3)
		frame()
		driveWords(&ctx.Router, fixtureMasterA)
		pump("passphrase", 160)
		click(&ctx.Router, Button3)
		pump("Duplicate key", 64)
		for i, c := range seen {
			if uiContains(c, "Policy Review") || uiContains(c, "Policy stub") {
				t.Fatalf("frame %d drew the Policy Review for a duplicate policy: %q", i, c)
			}
		}
	})
}
