package gui

import (
	"errors"
	"strings"
	"testing"
	"testing/synctest"

	"seedhammer.com/md"
	"seedhammer.com/mk"
)

// ─── M-E: the D-2 window, CLOSED by giving every slot its own origin ─────────
//
// Before S5, cosignerFromCard IGNORED the gathered card's declared origin,
// because OriginShared mode declares one origin for the whole policy. That was
// correct for a card actually minted at m/48'/0'/0'/2' and a lie for any other:
// the assembled md1 would say the key derives at the shared origin when the card
// says it does not. Funds-correct (addresses derive from the KEYS) and restore
// fail-closed -- but the plate carries a false path, and a BIP-48-aware
// coordinator reading it derives at the wrong place.
//
// S2 closed that window by REFUSING the shape, explicitly as an INTERIM measure:
// the alternative was not "permit" but "stamp m/48'/0'/0'/2' over a card that
// says otherwise", which is the device overruling an answer the operator already
// gave. S5 closes it the other way -- the card's origin is RECORDED -- and this
// file changed with it. What survives verbatim is the property both stages exist
// for: THE PLATE NEVER CARRIES A PATH THE CARD DISAGREES WITH.

// TestBuildRecordsTheCardsOwnOrigin is that property, over the shape S2 refused.
func TestBuildRecordsTheCardsOwnOrigin(t *testing.T) {
	// THE PAYLOAD ALREADY CARRIES ONE. Card A@1 is masterA's SECOND account, at
	// m/48h/0h/1h/2h -- deliberately, as S5 Trace B's multi-account shape
	// (cmd/buildpayloadcards/main.go:55).
	t.Run("the payload's second-account card is placed at its own origin", func(t *testing.T) {
		card := dupTestCard(t, 3) // A@1
		if card.Path == multisigSharedOrigin().String() {
			t.Fatalf("card A@1 declares %q, the shared origin: this test's premise is "+
				"that the delivered payload carries a divergent-origin card", card.Path)
		}
		selfXpub, selfFP := dupTestSelf(t, fixtureMasterC)
		p := buildPolicyParams{Script: md.MultisigWsh, N: 3, K: 2, SelfSlots: []int{0}}
		out, _, _, err := assembleBuildPolicy(p,
			selfKeyAt(0, selfXpub, selfFP, multisigSharedOrigin()),
			[]mk.Card{dupTestCard(t, 1), card}) // B@0 at the shared origin, then A@1
		if err != nil {
			t.Fatalf("a card declaring %q was refused: %v", card.Path, err)
		}
		_, keys, err := md.ExpandWalletPolicyChunks(out)
		if err != nil {
			t.Fatalf("the assembled md1 does not decode: %v", err)
		}
		if got := keys[2].OriginPath.String(); got != card.Path {
			t.Errorf("@2 declares %s, but its card says %s: the plate would carry a "+
				"derivation path the card itself disagrees with", got, card.Path)
		}
		if keys[1].OriginPath.String() != multisigSharedOrigin().String() {
			t.Errorf("@1 declares %s, want its card's own %s",
				keys[1].OriginPath, multisigSharedOrigin())
		}
	})

	// PERMISSIVE ON SPELLING, STRICT ON VALUE (§0.1). mk.Decode normalises to the
	// `h` form, but an mk.Card can reach the assembler carrying apostrophes, and
	// m/48'/0'/0'/2' and m/48h/0h/0h/2h are the same path. Treating them as
	// different origins would tip the policy into divergent mode over NOTATION,
	// which is the exact opposite of the rule.
	t.Run("apostrophe and h spellings of one origin are one origin", func(t *testing.T) {
		base := dupTestCard(t, 1) // B@0, at the shared origin
		var minted []string
		for _, spelling := range []string{"m/48h/0h/0h/2h", "m/48'/0'/0'/2'"} {
			card := base
			card.Path = spelling
			selfXpub, selfFP := dupTestSelf(t, fixtureMasterC)
			p := buildPolicyParams{Script: md.MultisigWsh, N: 2, K: 2, SelfSlots: []int{0}}
			out, _, _, err := assembleBuildPolicy(p,
				selfKeyAt(0, selfXpub, selfFP, multisigSharedOrigin()), []mk.Card{card})
			if err != nil {
				t.Fatalf("a card declaring %q (the shared origin) was refused: %v", spelling, err)
			}
			minted = append(minted, strings.Join(out, "|"))
		}
		if minted[0] != minted[1] {
			t.Error("the two spellings of one path minted DIFFERENT policies, so the " +
				"wallet id depends on notation")
		}
	})

	// An origin the device cannot READ is still refused, and for the reason that
	// never changed: the alternative is stamping something nobody parsed.
	t.Run("an unreadable declared origin is refused", func(t *testing.T) {
		card := dupTestCard(t, 1)
		card.Path = "m/48h/0h/notanumber/2h"
		selfXpub, selfFP := dupTestSelf(t, fixtureMasterC)
		p := buildPolicyParams{Script: md.MultisigWsh, N: 2, K: 2, SelfSlots: []int{0}}
		out, _, _, err := assembleBuildPolicy(p,
			selfKeyAt(0, selfXpub, selfFP, multisigSharedOrigin()), []mk.Card{card})
		if err == nil {
			t.Fatalf("a card whose declared origin does not parse was accepted: %v", out)
		}
		if out != nil {
			t.Errorf("md1 chunks were produced alongside the refusal: %v", out)
		}
	})

	// THE ORDER OF THE CHECKS IS RULED, and the delivered payload can trigger both
	// on one build (self = masterA, cards A@0 + A@1). The duplicate wins: it is
	// the graver harm, since a repeated key degrades the quorum invisibly where a
	// declared origin is printed on every artifact, and §4.1's check is the one
	// that OUTLIVES every stage.
	t.Run("a duplicate outranks anything the encoder would say", func(t *testing.T) {
		selfXpub, selfFP := dupTestSelf(t, fixtureMasterA)
		p := buildPolicyParams{Script: md.MultisigWsh, N: 3, K: 2, SelfSlots: []int{0}}
		_, _, _, err := assembleBuildPolicy(p,
			selfKeyAt(0, selfXpub, selfFP, multisigSharedOrigin()),
			[]mk.Card{dupTestCard(t, 0), dupTestCard(t, 3)}) // A@0 (collides with self), A@1
		var dup errBuildDuplicateKey
		if !errors.As(err, &dup) {
			t.Fatalf("a build whose self key is repeated at @1 reported %v; the "+
				"duplicate must win", err)
		}
	})
}

// TestBuildFlowAcceptsDivergentOriginCard drives the WHOLE flow over the shape S2
// refused: a payload whose only cosigner card is masterA's second account, a self
// seed that does not collide with it, and all five picker defaults.
//
// It is the behavioural half of the removal. The old test asserted that this walk
// ended on a "Key origin mismatch" modal; the assertion is now that it reaches
// the policy review, which is the screen a build that WORKS reaches.
func TestBuildFlowAcceptsDivergentOriginCard(t *testing.T) {
	// Roster index 3 is A@1. cosignerCardFixtures returns a PREFIX, so take four
	// and use only the fourth card's chunks.
	all := cosignerCardFixtures(t, 4)
	records := all[3]
	synctest.Test(t, func(t *testing.T) {
		p := newPlatform()
		p.display = sh2DisplaySize
		ctx := NewContext(p)
		ctx.sysw = sessionHolding(records...)
		// RASTERED, for TestBuildFlowRefusesDuplicateBeforeReview's reason: a screen
		// nobody can read is a screen that did not happen, and content assertions
		// cannot see that.
		frame, _, ink, quit := runUITouchRaster(ctx, func() {
			buildMultisigPolicyFlow(ctx, &descriptorTheme)
		})
		defer quit()
		buildWalkParamPickers(t, ctx, frame)
		if c, ok := pumpUntil(frame, "mk1 keys: 1", 32); !ok {
			t.Fatalf("the card never reached the gather; got %q", c)
		}
		click(&ctx.Router, Button3)
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
		driveWords(&ctx.Router, fixtureMasterC) // does NOT collide with A@1
		if c, ok := pumpUntil(frame, "passphrase", 160); !ok {
			t.Fatalf("the passphrase prompt was not reached; got %q", c)
		}
		click(&ctx.Router, Button3) // Skip
		if c, ok := pumpUntil(frame, "Key sources", 64); !ok {
			t.Fatalf("S4's slot-source review was not reached; got %q", c)
		}
		click(&ctx.Router, Button3)
		content, ok := pumpUntil(frame, "Policy stub", 64)
		if !ok {
			t.Fatalf("a card declaring a divergent origin did not reach the policy "+
				"review; got %q", content)
		}
		if ink() < buildWalkRasterFloor {
			t.Errorf("the policy review drew only %d ink pixels (floor %d)",
				ink(), buildWalkRasterFloor)
		}
		t.Logf("divergent-origin policy review: ink = %d px", ink())
	})
}
