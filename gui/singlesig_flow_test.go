package gui

import (
	"os"
	"strings"
	"testing"
	"testing/synctest"

	"seedhammer.com/bip39"
)

// TestEngraveSingleSigFlowTypedOnly_Structural (D12): engraveSingleSigFlow's
// source references seedEntryFlow (the ONE seed seam) and NEVER routes a scanned
// object (act.scan / assembleScan / the scanner) into derivation ITSELF.
//
// WHAT IT STILL PROVES, now that seedEntryFlow is no longer keyboard-only: this
// file has no seed path of its own. Every source lives behind the one seam, where
// it is picked deliberately and acknowledged, instead of in a second route only
// this flow would have. The test NAME is kept so its history stays greppable; the
// property it asserts has not changed.
func TestEngraveSingleSigFlowTypedOnly_Structural(t *testing.T) {
	src, err := os.ReadFile("singlesig.go")
	if err != nil {
		t.Fatalf("read singlesig.go: %v", err)
	}
	// Strip // line-comments so the assertion tests CODE, not the security-spine
	// prose (which legitimately names the forbidden primitives to explain the
	// prohibition).
	var b strings.Builder
	for _, line := range strings.Split(string(src), "\n") {
		if i := strings.Index(line, "//"); i >= 0 {
			line = line[:i]
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	code := b.String()
	if !strings.Contains(code, "seedEntryFlow") {
		t.Fatal("engraveSingleSigFlow must obtain the seed via seedEntryFlow (typed-only)")
	}
	for _, forbidden := range []string{"assembleScan", "act.scan", ".Scan(", "new(scanner)"} {
		if strings.Contains(code, forbidden) {
			t.Fatalf("engraveSingleSigFlow code references %q — the SECRET seed must never come from a scan (D12)", forbidden)
		}
	}
}

// TestEngraveSingleSigFlowFull: driving the orchestrator with a typed seed →
// BIP-84 default → skip passphrase → "Full" reaches the engrave with 3 cards
// (the first guided title is "Card 1 of 3").
func TestEngraveSingleSigFlowFull(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := NewContext(newPlatform())
		frame, quit := runUI(ctx, func() {
			engraveSingleSigFlow(ctx, &descriptorTheme)
		})
		defer quit()
		frame()
		// Seed entry: 12 WORDS (choice 0).
		click(&ctx.Router, Button3)
		frame()
		driveWords(&ctx.Router, abandonAboutPhrase())
		if c, ok := pumpUntil(frame, "Wallet Type", 160); !ok {
			t.Fatalf("did not reach wallet-type picker; got %q", c)
		}
		// Wallet type: BIP-84 default (choice 0).
		click(&ctx.Router, Button3)
		if c, ok := pumpUntil(frame, "passphrase", 64); !ok {
			t.Fatalf("did not reach passphrase prompt; got %q", c)
		}
		// Passphrase: Skip (choice 0).
		click(&ctx.Router, Button3)
		if c, ok := pumpUntil(frame, "Engrave Mode", 64); !ok {
			t.Fatalf("did not reach the full/watch-only choice; got %q", c)
		}
		// Engrave mode: Full (choice 0).
		click(&ctx.Router, Button3)
		if c, ok := pumpUntil(frame, "Engrave wallet policy", 64); !ok {
			t.Fatalf("did not reach the wallet-policy form choice; got %q", c)
		}
		// Wallet policy: Full policy md1 (choice 0, default).
		click(&ctx.Router, Button3)
		// S6a/F-202: the pre-engrave plate census now stands between the policy
		// form and the first plate. confirmReviewScreen loops until Button1/
		// Button3/Center, and pumpUntil only pumps frames -- it never presses --
		// so this walk parks here for its whole budget without the press below.
		// The census is CONFIRMED rather than backed out of, because the assertion
		// this walk exists for is on the screen after it.
		if c, ok := pumpUntil(frame, "Plates To Cut", 64); !ok {
			t.Fatalf("the plate census was not shown before the engrave; got %q", c)
		}
		click(&ctx.Router, Button3)
		if c, ok := pumpUntil(frame, "Card 1 of 3", 64); !ok {
			t.Fatalf("full mode did not reach engrave with 3 cards; got %q", c)
		}
	})
}

// TestEngraveSingleSigFlowWatchOnly: choosing "Watch-only" reaches the engrave
// with 2 cards (the first guided title is "Card 1 of 2").
func TestEngraveSingleSigFlowWatchOnly(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := NewContext(newPlatform())
		frame, quit := runUI(ctx, func() {
			engraveSingleSigFlow(ctx, &descriptorTheme)
		})
		defer quit()
		frame()
		click(&ctx.Router, Button3) // 12 WORDS
		frame()
		driveWords(&ctx.Router, abandonAboutPhrase())
		if c, ok := pumpUntil(frame, "Wallet Type", 160); !ok {
			t.Fatalf("did not reach wallet-type picker; got %q", c)
		}
		click(&ctx.Router, Button3) // BIP-84
		if c, ok := pumpUntil(frame, "passphrase", 64); !ok {
			t.Fatalf("did not reach passphrase prompt; got %q", c)
		}
		click(&ctx.Router, Button3) // Skip passphrase
		if c, ok := pumpUntil(frame, "Engrave Mode", 64); !ok {
			t.Fatalf("did not reach the full/watch-only choice; got %q", c)
		}
		// Engrave mode: Watch-only (choice 1 → 1 Down then confirm).
		click(&ctx.Router, Down)
		frame()
		click(&ctx.Router, Button3)
		if c, ok := pumpUntil(frame, "Engrave wallet policy", 64); !ok {
			t.Fatalf("did not reach the wallet-policy form choice; got %q", c)
		}
		// Wallet policy: Full policy md1 (choice 0, default).
		click(&ctx.Router, Button3)
		// S6a/F-202's census, pressed through -- see the Full walk above.
		if c, ok := pumpUntil(frame, "Plates To Cut", 64); !ok {
			t.Fatalf("the plate census was not shown before the engrave; got %q", c)
		}
		click(&ctx.Router, Button3)
		if c, ok := pumpUntil(frame, "Card 1 of 2", 64); !ok {
			t.Fatalf("watch-only mode did not reach engrave with 2 cards; got %q", c)
		}
	})
}

// TestEngraveSingleSigFlowSeedScrubbed: the typed seed mnemonic is zeroed when
// the flow returns (the abort path: backing out of the wallet-type picker). We
// capture the mnemonic slice the flow holds via a seed-entry hook and assert it
// is zeroed after return.
func TestEngraveSingleSigFlowSeedScrubbed(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var captured bip39.Mnemonic
		singleSigSeedHook = func(m bip39.Mnemonic) { captured = m }
		defer func() { singleSigSeedHook = nil }()

		ctx := NewContext(newPlatform())
		done := false
		frame, quit := runUI(ctx, func() {
			engraveSingleSigFlow(ctx, &descriptorTheme)
			done = true
		})
		defer quit()
		frame()
		click(&ctx.Router, Button3) // 12 WORDS
		frame()
		driveWords(&ctx.Router, abandonAboutPhrase())
		if c, ok := pumpUntil(frame, "Wallet Type", 160); !ok {
			t.Fatalf("did not reach wallet-type picker; got %q", c)
		}
		// EXIT ROUTE CHANGED 2026-08-19, the scrub assertion did not.
		//
		// Back from the wallet-type picker used to return from the flow. Under
		// the "going back should lose nothing" directive it now steps BACK to
		// the seed, which is the point — so this test leaves by pressing Back
		// TWICE: once to the seed, once more to leave the program from its
		// first step.
		//
		// What is being tested here is D11 (the mnemonic is zeroed on exit),
		// not where Back lands; that is covered by
		// TestSingleSigBackStepsBackAndLosesNothing.
		click(&ctx.Router, Button1) // picker → seed
		for i := 0; i < 32 && !done; i++ {
			frame()
		}
		if done {
			t.Fatal("Back from the picker LEFT the flow — the pre-2026-08-19 behaviour")
		}
		click(&ctx.Router, Button1) // seed (first step) → leave
		for i := 0; i < 64 && !done; i++ {
			frame()
		}
		if !done {
			t.Fatal("Back at the seed step did not leave the flow")
		}
		if captured == nil {
			t.Fatal("seed hook did not capture the mnemonic")
		}
		for i, w := range captured {
			if w != 0 {
				t.Fatalf("mnemonic[%d] = %d, not zeroed on exit (D11)", i, w)
			}
		}
	})
}

// TestSingleSigBackStepsBackAndLosesNothing — the 2026-08-19 operator directive
// on the single-sig program.
//
// BEFORE: Back at the wallet-type picker returned from engraveSingleSigFlow,
// discarding a typed 12-word seed.
//
// AFTER: it steps back to the seed, RE-ENTERED HOLDING THE WORDS, and only a
// second Back (from the first step) leaves.
func TestSingleSigBackStepsBackAndLosesNothing(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := NewContext(newPlatform())
		done := false
		frame, quit := runUI(ctx, func() {
			engraveSingleSigFlow(ctx, &descriptorTheme)
			done = true
		})
		defer quit()
		frame()

		click(&ctx.Router, Button3) // 12 WORDS
		frame()
		driveWords(&ctx.Router, abandonAboutPhrase())
		if c, ok := pumpUntil(frame, "Wallet Type", 160); !ok {
			t.Fatalf("did not reach the wallet-type picker; got %q", c)
		}

		// Back → the seed, holding the words. A word-entry screen shows "1:",
		// the blank path would show the 12/24 word-count picker instead.
		click(&ctx.Router, Button1)
		c, ok := pumpUntil(frame, "1:", 160)
		if !ok {
			t.Fatalf("Back at the picker did not return to a seed screen holding "+
				"the typed words; got %q", c)
		}
		if done {
			t.Fatal("Back at the picker LEFT the flow — the pre-2026-08-19 behaviour")
		}
	})
}
