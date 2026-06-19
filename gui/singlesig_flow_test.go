package gui

import (
	"os"
	"strings"
	"testing"
	"testing/synctest"

	"seedhammer.com/bip39"
)

// TestEngraveSingleSigFlowTypedOnly_Structural (D12): engraveSingleSigFlow's
// source references seedEntryFlow (the typed-only seed entry) and NEVER routes a
// scanned object (act.scan / assembleScan / the scanner) into derivation. The
// seed is SECRET → typed-only, never NFC.
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
		// Back out of the wallet-type picker → the flow returns and scrubs.
		click(&ctx.Router, Button1)
		for i := 0; i < 32 && !done; i++ {
			frame()
		}
		if !done {
			t.Fatal("flow did not return after Back from the picker")
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
