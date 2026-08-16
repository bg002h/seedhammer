package gui

import (
	"strings"
	"testing"
	"testing/synctest"
)

// ─── §0.1b's OTHER primary data entry, never once exercised ──────────────────
//
// The self seed can come from the keyboard or from the payload, and the operator
// ruled both PRIMARY for this phase. Until now only one of them had ever been
// rendered on the Build path.
//
// The gap was structural, not an oversight anyone could see: every Go test on
// this flow seeds the session with `cosignerCardRecords(...)`, which is mk1
// chunks and nothing else. With no `ClassMnemonic` in the payload,
// `syswSeedPicker` finds exactly one row, and "a picker with one row is not a
// choice, and is not drawn" — so the source screen never appeared and the
// payload arm below it was unreachable BY CONSTRUCTION. Seven test files, none
// of them wrong, and between them zero coverage of half of §0.1b.
//
// The emulator payload has always carried a ClassMnemonic (master A). These
// tests give the Go side the same shape.

// payloadWithSeedAndCards is the emulator payload's shape: cosigner key cards
// plus the ClassMnemonic the `both`/self slots derive from. Card order is the Go
// roster's (A@0, B@0, C@0, A@1), which is NOT the emulator's — see F-180.
func payloadWithSeedAndCards(t *testing.T, nCards int, seedPhrase string) []string {
	t.Helper()
	records := cosignerCardRecords(t, nCards)
	// Last, so the cards read first — the order cmd/buildpayloadcards writes.
	return append(records, seedPhrase)
}

// TestBuildTakesTheSelfSeedFromThePayload drives the route: the source picker
// APPEARS (proving it is a real choice, not a menu of one), FROM PAYLOAD is
// taken, the acceptance surface is shown, and the build proceeds on a seed no
// keyboard entered.
func TestBuildTakesTheSelfSeedFromThePayload(t *testing.T) {
	// masterC as the self seed against cards B@0 and C@0 would collide (C@0 IS
	// masterC at the shared origin), so the self seed here is masterA and the
	// cards are B@0 + C@0 — Trace A exactly.
	sets := cosignerCardFixtures(t, 3)
	var records []string
	records = append(records, sets[1]...) // B@0
	records = append(records, sets[2]...) // C@0
	records = append(records, fixtureMasterA)

	synctest.Test(t, func(t *testing.T) {
		p := newPlatform()
		p.display = sh2DisplaySize
		ctx := NewContext(p)
		ctx.sysw = sessionHolding(records...)
		frame, _, ink, quit := runUITouchRaster(ctx, func() {
			buildMultisigPolicyFlow(ctx, &descriptorTheme)
		})
		defer quit()

		// template wsh -> n=3 -> k=2 -> @0 -> no further slots -> fp omit.
		// (S5's @S picker is multi-select; the "another slot?" default row is
		// "NO, THAT IS ALL", so the held set stays {@0}.)
		for i, downs := range []int{0, 1, 1, 0, 0, 0} {
			stage := []string{"Choose policy type", "How many keys (n)?",
				"Required signatures (k of 3)?", "Which slot is your key?",
				"Do you hold another slot?", "Include key fingerprints?"}[i]
			if c, ok := pumpUntil(frame, stage, 64); !ok {
				t.Fatalf("%s not shown; got %q", stage, c)
			}
			for j := 0; j < downs; j++ {
				click(&ctx.Router, Down)
				frame()
			}
			click(&ctx.Router, Button3)
			frame()
		}
		if c, ok := pumpUntil(frame, "mk1 keys: 2", 64); !ok {
			t.Fatalf("the gather never showed the two cosigner cards; got %q", c)
		}
		click(&ctx.Router, Button3)
		frame()
		if c, ok := pumpUntil(frame, "Payload cards", 64); !ok {
			t.Fatalf("the payload review never appeared; got %q", c)
		}
		click(&ctx.Router, Button3)
		frame()

		// THE SCREEN THAT HAD NEVER BEEN DRAWN. With a ClassMnemonic present the
		// picker has two rows (FROM PAYLOAD / TYPE IT) and is therefore drawn.
		content, ok := pumpUntil(frame, "Where from?", 64)
		if !ok {
			t.Fatalf("the seed SOURCE picker never appeared, so the payload arm is "+
				"still unreachable; got %q", content)
		}
		if !uiContains(content, "FROM PAYLOAD") {
			t.Errorf("the source picker does not offer the payload: %q", content)
		}
		if ink() < buildWalkRasterFloor {
			t.Errorf("the seed source picker drew only %d ink pixels (floor %d)",
				ink(), buildWalkRasterFloor)
		}
		click(&ctx.Router, Button3) // FROM PAYLOAD is row 0, the default
		frame()

		// The acceptance surface for a payload-sourced SECRET.
		if c, ok := pumpUntil(frame, "systemwide payload", 64); !ok {
			t.Fatalf("the payload-source acceptance screen was not shown; got %q", c)
		}
		if ink() < buildWalkRasterFloor {
			t.Errorf("the source-acceptance screen drew only %d ink pixels (floor %d)",
				ink(), buildWalkRasterFloor)
		}
		click(&ctx.Router, Button3)
		frame()

		if c, ok := pumpUntil(frame, "Add a BIP-39 passphrase?", 64); !ok {
			t.Fatalf("the passphrase prompt was not reached; got %q", c)
		}
		click(&ctx.Router, Button3) // Skip
		frame()

		// S4's pre-assembly slot-source review (SPEC 4.3).
		if c, ok := pumpUntil(frame, "Key sources", 96); !ok {
			t.Fatalf("S4's slot-source review was not reached; got %q", c)
		}
		click(&ctx.Router, Button3)
		frame()

		// AND THE BUILD PROCEEDS on a seed that no keyboard entered.
		review, ok := pumpUntil(frame, "Policy stub", 96)
		if !ok {
			t.Fatalf("a payload-sourced self seed did not reach the policy review; got %q", review)
		}
		if ink() < buildWalkRasterFloor {
			t.Errorf("the policy review drew only %d ink pixels (floor %d)",
				ink(), buildWalkRasterFloor)
		}
		// The same stub the emulator walk and the byte-identity gate produce for
		// Trace A. Pinned so a silently different policy cannot pass as this one.
		if !uiContains(review, "06215ac0") {
			t.Errorf("payload-sourced Trace A assembled a different policy: %q", review)
		}
		if !uiContains(review, multisigSharedOrigin().String()) {
			t.Errorf("the review reached the display without §0.1a's announcement: %q", review)
		}
	})
}

// TestBuildRefusesDuplicateOnAPayloadSourcedSeed is the emulator walk's refusal
// arm, in Go: the payload carries master A AND master A's own account-0 card, so
// taking the seed from the payload repeats the self key.
//
// The emulator drives the identical shape and reaches the identical screen; this
// is the fast guard for it.
func TestBuildRefusesDuplicateOnAPayloadSourcedSeed(t *testing.T) {
	records := payloadWithSeedAndCards(t, 1, fixtureMasterA) // card A@0 + master A
	synctest.Test(t, func(t *testing.T) {
		ctx := NewContext(newPlatform())
		ctx.sysw = sessionHolding(records...)
		frame, quit := runUI(ctx, func() { buildMultisigPolicyFlow(ctx, &descriptorTheme) })
		defer quit()
		buildWalkParamPickers(t, ctx, frame) // all defaults: n=2, one open slot
		if c, ok := pumpUntil(frame, "mk1 keys: 1", 64); !ok {
			t.Fatalf("the gather never showed the card; got %q", c)
		}
		click(&ctx.Router, Button3)
		frame()
		if c, ok := pumpUntil(frame, "Payload cards", 64); !ok {
			t.Fatalf("the payload review never appeared; got %q", c)
		}
		click(&ctx.Router, Button3)
		frame()
		if c, ok := pumpUntil(frame, "Where from?", 64); !ok {
			t.Fatalf("the seed source picker never appeared; got %q", c)
		}
		click(&ctx.Router, Button3) // FROM PAYLOAD
		frame()
		if c, ok := pumpUntil(frame, "systemwide payload", 64); !ok {
			t.Fatalf("the acceptance screen was not shown; got %q", c)
		}
		click(&ctx.Router, Button3)
		frame()
		if c, ok := pumpUntil(frame, "Add a BIP-39 passphrase?", 64); !ok {
			t.Fatalf("the passphrase prompt was not reached; got %q", c)
		}
		click(&ctx.Router, Button3) // Skip
		if c, ok := pumpUntil(frame, "Key sources", 96); !ok {
			t.Fatalf("the slot-source review was not reached; got %q", c)
		}
		click(&ctx.Router, Button3)
		content, ok := pumpUntil(frame, "Duplicate key", 96)
		if !ok {
			t.Fatalf("a payload-sourced seed that repeats a payload card's key was "+
				"NOT refused; got %q", content)
		}
		if !strings.Contains(strings.ReplaceAll(content, " ", ""),
			strings.ReplaceAll("your key, from your seed", " ", "")) {
			t.Errorf("the refusal did not name the self slot's provenance: %q", content)
		}
	})
}
