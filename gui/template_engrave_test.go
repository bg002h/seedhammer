package gui

import (
	"testing"
	"testing/synctest"

	"github.com/btcsuite/btcd/chaincfg/v2"
	"seedhammer.com/bundle"
	"seedhammer.com/md"
	"seedhammer.com/mk"
)

// ─── Task 6: single-sig template-engrave transform ───────────────────────────

// TestTemplateizeBundle: templateizeBundle strips the md1 to a keyless template
// and re-mints the mk1 stub on the template's WDT-Id, so the resulting bundle
// VERIFIES against itself (the device's own readback, C2) and the md1 is keyless.
func TestTemplateizeBundle(t *testing.T) {
	m := abandonAboutMnemonic()
	path := singleSigPath(84)
	full, _, _, _, err := deriveSingleSigBundle(m, "", &chaincfg.MainNetParams, path, md.ScriptWpkh)
	if err != nil {
		t.Fatalf("deriveSingleSigBundle: %v", err)
	}

	tmpl, err := templateizeBundle(full)
	if err != nil {
		t.Fatalf("templateizeBundle: %v", err)
	}

	// md1 still decodes (keyless template).
	if _, err := md.Reassemble(tmpl.MD1); err != nil {
		t.Fatalf("Reassemble template md1: %v", err)
	}

	// ms1 leg is unchanged (still the secret seed backup).
	if tmpl.MS1 != full.MS1 {
		t.Errorf("templateize changed ms1 (should be untouched)")
	}

	// The template bundle VERIFIES against itself (own readback, C2).
	if err := bundle.Verify(tmpl, tmpl); err != nil {
		t.Fatalf("template bundle own-readback verify failed: %v (want PASS)", err)
	}

	// The mk1 stub now roots on the WDT-Id (form-aware), and the md1 is keyless:
	// the full md1's stub (WalletPolicyId) must DIFFER from the template stub.
	fullStub, err := md.FormAwareStubChunks(full.MD1)
	if err != nil {
		t.Fatal(err)
	}
	tmplStub, err := md.FormAwareStubChunks(tmpl.MD1)
	if err != nil {
		t.Fatal(err)
	}
	if fullStub == tmplStub {
		t.Error("template stub equals full-policy stub; the strip did not change the id space")
	}
	// The re-minted mk1 carries the template stub.
	card, err := mk.Decode(tmpl.MK1)
	if err != nil {
		t.Fatalf("mk.Decode template mk1: %v", err)
	}
	if len(card.Stubs) != 1 || card.Stubs[0] != tmplStub {
		t.Fatalf("template mk1 stubs = %v, want [%x]", card.Stubs, tmplStub)
	}
	// xpub/path/fingerprint preserved from the full card.
	fullCard, _ := mk.Decode(full.MK1)
	if card.Xpub != fullCard.Xpub || card.Path != fullCard.Path || card.Fingerprint != fullCard.Fingerprint {
		t.Error("templateize altered the mk1 xpub/path/fingerprint (only the stub should change)")
	}
}

// TestEngraveSingleSigFlowTemplate: selecting "Template-only md1" shows the
// loud warning + estimate strings, then engraves the template bundle (full mode:
// 3 cards). Asserts the load-bearing consent strings render (S4/S6).
func TestEngraveSingleSigFlowTemplate(t *testing.T) {
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
		click(&ctx.Router, Button3) // Full
		if c, ok := pumpUntil(frame, "Engrave wallet policy", 64); !ok {
			t.Fatalf("did not reach the wallet-policy form choice; got %q", c)
		}
		// Wallet policy: Template-only md1 (choice 1).
		click(&ctx.Router, Down)
		frame()
		click(&ctx.Router, Button3)
		// The loud warning + estimate appears.
		if c, ok := pumpUntil(frame, "TEMPLATE-ONLY md1", 64); !ok {
			t.Fatalf("template warning not shown; got %q", c)
		}
		// The recovery estimate is later on the paginated review screen — page
		// forward (Button2) until the sortedmulti estimate line renders.
		found := false
		for i := 0; i < 6 && !found; i++ {
			click(&ctx.Router, Button2) // next page
			if c, ok := pumpUntil(frame, "sortedmulti", 16); ok {
				found = true
			} else {
				_ = c
			}
		}
		if !found {
			t.Fatal("recovery estimate (sortedmulti line) not shown across the review pages")
		}
		// Confirm "I understand" → engrave (full = 3 cards).
		click(&ctx.Router, Button3)
		if c, ok := pumpUntil(frame, "Card 1 of 3", 64); !ok {
			t.Fatalf("template engrave did not reach the 3-card engrave; got %q", c)
		}
	})
}
