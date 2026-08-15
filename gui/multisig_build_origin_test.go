package gui

import (
	"errors"
	"strings"
	"testing"
	"testing/synctest"

	"seedhammer.com/md"
	"seedhammer.com/mk"
)

// ─── M-E: the D-2 window, closed while it is open ────────────────────────────
//
// cosignerFromCard IGNORES the gathered card's declared origin, because
// OriginShared mode declares one origin for the whole policy. That is correct
// for a card actually minted at m/48'/0'/0'/2' and a lie for any other: the
// assembled md1 would say the key derives at the shared origin when the card
// says it does not. Funds-correct (addresses derive from the KEYS) and restore
// fail-closed — but the plate carries a false path, and a BIP-48-aware
// coordinator reading it derives at the wrong place.
//
// SPEC P1 permits refuse OR warn. THIS PLAN PICKS REFUSE, and the reason is the
// exposure rather than the test shape (§0.1): a declared origin the flow cannot
// honour is CONTRADICTED input, not an underdetermined spelling. §0.1 clause 1
// gives a default only where an input leaves the answer open; here the operator
// has already answered, and the device would be overruling them silently.
//
// It is INTERIM. S5 gives each slot its own origin and this refusal becomes an
// accepted shape.

// TestBuildRefusesForeignOriginCardBeforeS5 is spec M-E.
func TestBuildRefusesForeignOriginCardBeforeS5(t *testing.T) {
	// THE PAYLOAD ALREADY CARRIES ONE. Card A@1 is masterA's SECOND account, at
	// m/48h/0h/1h/2h — deliberately, as S5 Trace B's multi-account shape
	// (cmd/buildpayloadcards/main.go:55). Until S5 it cannot be honoured, so
	// until S5 it is refused.
	t.Run("the payload's second-account card is refused", func(t *testing.T) {
		card := dupTestCard(t, 3) // A@1
		if card.Path == multisigSharedOrigin().String() {
			t.Fatalf("card A@1 declares %q, the shared origin — this test's premise "+
				"is that the delivered payload carries a foreign-origin card", card.Path)
		}
		selfXpub, selfFP := dupTestSelf(t, fixtureMasterC)
		p := buildPolicyParams{Script: md.MultisigWsh, N: 3, K: 2, SelfSlot: 0}
		out, _, _, err := assembleBuildPolicy(p, selfXpub, selfFP,
			[]mk.Card{dupTestCard(t, 1), card}) // B@0 at the shared origin, then A@1
		var fo errBuildForeignOrigin
		if !errors.As(err, &fo) {
			t.Fatalf("a card declaring %q was stamped with the shared origin instead "+
				"of refused (err = %v)", card.Path, err)
		}
		if fo.Slot != 2 {
			t.Errorf("blamed slot @%d; A@1 fills @2 here", fo.Slot)
		}
		if fo.Declared != card.Path {
			t.Errorf("reported declared origin %q, want %q", fo.Declared, card.Path)
		}
		if out != nil {
			t.Errorf("md1 chunks were produced alongside the refusal: %v", out)
		}
	})

	// PERMISSIVE ON SPELLING, STRICT ON VALUE (§0.1). mk.Decode normalises to the
	// `h` form, but an mk.Card can reach the assembler carrying apostrophes, and
	// m/48'/0'/0'/2' and m/48h/0h/0h/2h are the same path. Refusing one of them
	// would be a refusal over notation, which is the exact opposite of the rule.
	t.Run("apostrophe and h spellings of the shared origin are both accepted", func(t *testing.T) {
		base := dupTestCard(t, 1) // B@0, at the shared origin
		for _, spelling := range []string{"m/48h/0h/0h/2h", "m/48'/0'/0'/2'"} {
			card := base
			card.Path = spelling
			selfXpub, selfFP := dupTestSelf(t, fixtureMasterC)
			p := buildPolicyParams{Script: md.MultisigWsh, N: 2, K: 2, SelfSlot: 0}
			if _, _, _, err := assembleBuildPolicy(p, selfXpub, selfFP, []mk.Card{card}); err != nil {
				t.Errorf("a card declaring %q (the shared origin) was refused: %v", spelling, err)
			}
		}
	})

	// An origin the device cannot READ is not an origin it can honour. Refusing
	// is the same answer for the same reason; the alternative is stamping the
	// shared origin over something nobody parsed.
	t.Run("an unreadable declared origin is refused too", func(t *testing.T) {
		card := dupTestCard(t, 1)
		card.Path = "m/48h/0h/notanumber/2h"
		selfXpub, selfFP := dupTestSelf(t, fixtureMasterC)
		p := buildPolicyParams{Script: md.MultisigWsh, N: 2, K: 2, SelfSlot: 0}
		_, _, _, err := assembleBuildPolicy(p, selfXpub, selfFP, []mk.Card{card})
		var fo errBuildForeignOrigin
		if !errors.As(err, &fo) {
			t.Fatalf("a card whose declared origin does not parse was accepted: %v", err)
		}
	})

	// THE ORDER OF THE TWO REFUSALS IS RULED, and the delivered payload can
	// trigger both on one build (self = masterA, cards A@0 + A@1). The duplicate
	// wins: it is the graver harm — a repeated key degrades the quorum invisibly
	// where a foreign origin mis-states a path that is printed on every artifact
	// — and it is the check that OUTLIVES this stage. The origin refusal
	// disappears at S5; §4.1's never does.
	t.Run("a duplicate outranks a foreign origin", func(t *testing.T) {
		selfXpub, selfFP := dupTestSelf(t, fixtureMasterA)
		p := buildPolicyParams{Script: md.MultisigWsh, N: 3, K: 2, SelfSlot: 0}
		_, _, _, err := assembleBuildPolicy(p, selfXpub, selfFP,
			[]mk.Card{dupTestCard(t, 0), dupTestCard(t, 3)}) // A@0 (collides), A@1 (foreign)
		var dup errBuildDuplicateKey
		if !errors.As(err, &dup) {
			t.Fatalf("a build that is BOTH a duplicate and a foreign origin reported "+
				"%v; the duplicate must win", err)
		}
	})

	// The refusal text: which slot, what the card says, what the device would
	// have stamped, that nothing was cut, and the route that exists. No "scan".
	t.Run("the refusal names the slot and both origins", func(t *testing.T) {
		msg := buildForeignOriginMessage(
			errBuildForeignOrigin{Slot: 2, Declared: "m/48h/0h/1h/2h"},
			buildCosignerOrigins(3, 0, []int{0, 1}))
		for _, want := range []string{
			"slot @2", "payload card 2", "m/48h/0h/1h/2h",
			multisigSharedOrigin().String(), "Nothing was engraved",
		} {
			if !strings.Contains(msg, want) {
				t.Errorf("refusal is missing %q:\n%s", want, msg)
			}
		}
		if strings.Contains(msg, "scan") || strings.Contains(msg, "Scan") {
			t.Errorf("the refusal tells the operator to scan:\n%s", msg)
		}
		if strings.Contains(msg, "\u2014") {
			t.Errorf("the refusal contains an em-dash, which does not draw:\n%s", msg)
		}
	})
}

// TestBuildFlowRefusesForeignOriginCard drives the whole flow: a payload whose
// only cosigner card is masterA's second account, a self seed that does not
// collide with it, and all five picker defaults.
func TestBuildFlowRefusesForeignOriginCard(t *testing.T) {
	// Roster index 3 is A@1. cosignerCardRecords returns a PREFIX, so take four
	// and drop the first three cards' chunks.
	all := cosignerCardFixtures(t, 4)
	records := all[3]
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
		content, ok := pumpUntil(frame, "Key origin", 64)
		if !ok {
			t.Fatalf("a card declaring a foreign origin did not refuse; got %q", content)
		}
		click(&ctx.Router, Button3)
		for i := 0; i < 64 && !done; i++ {
			frame()
		}
		if !done {
			t.Fatal("the flow did not return after the foreign-origin refusal")
		}
	})
}
