package gui

import (
	"strings"
	"testing"

	"seedhammer.com/md"
)

// consentText joins the consent lines so assertions read against one string,
// the way the screen renders them.
func consentText(t *testing.T, vector string) string {
	t.Helper()
	lines, err := walletPolicyConsentLines(loadVectorChunks(t, vector))
	if err != nil {
		t.Fatalf("%s: consent lines refused: %v", vector, err)
	}
	return strings.Join(lines, "\n")
}

// TestWalletPolicyConsentProvesTheWallet is plan D2, asserted rather than
// described: proof is derived addresses plus a NAMED wallet id, and it sits ON
// the consent path.
//
// The addresses come from the primary Rust implementation, so this fails if the
// device would show an operator a different wallet's addresses under the right
// id — which is the failure that matters, and the one a "did it render" test
// cannot see.
func TestWalletPolicyConsentProvesTheWallet(t *testing.T) {
	// Plan D6 pinned to a literal: receive AND change, roughly 0..2 each. The
	// assertions below are written against 2 directly, so this is the one place
	// the number lives as a claim rather than as a loop bound.
	if addrProofPerChain != 2 {
		t.Fatalf("addrProofPerChain = %d, want 2 (plan D6)", addrProofPerChain)
	}

	for _, vector := range []string{"keyed_tr_with_leaf", "keyed_wsh_thresh", "keyed_wpkh"} {
		t.Run(vector, func(t *testing.T) {
			got := consentText(t, vector)

			// The id, named. A keyed card's authoritative id is the
			// key-DEPENDENT one; labelling it "Template-ID" would send an
			// operator to compare a value their coordinator never shows.
			if !strings.Contains(got, "Policy-ID:") {
				t.Errorf("the consent screen does not name the wallet id it shows:\n%s", got)
			}
			if strings.Contains(got, "Template-ID:") {
				t.Errorf("a keyed policy must not be labelled with the key-stable id:\n%s", got)
			}
			id, kind, err := md.FormAwareIdChunks(loadVectorChunks(t, vector))
			if err != nil {
				t.Fatalf("FormAwareIdChunks: %v", err)
			}
			if kind != md.WalletIdPolicy {
				t.Fatalf("fixture is not a keyed policy; this test proves nothing")
			}
			if !strings.Contains(got, hexString(id[:])) {
				t.Errorf("the id shown is not this card's id %x:\n%s", id[:], got)
			}

			// BOTH chains, and they must be the addresses Rust derives.
			// Change is where a policy mismatch silently loses funds (D6), so a
			// screen proving only receive proves the cheaper half.
			for _, c := range []struct {
				chain string
				label string
			}{{"0", "Receive"}, {"1", "Change"}} {
				if !strings.Contains(got, c.label) {
					t.Errorf("no %s addresses on the consent screen:\n%s", c.label, got)
				}
				// LITERAL 2, not addrProofPerChain. Looping on the constant
				// under test moves both sides together, so the assertion holds
				// for any value it could take — halving it to 1 passed this
				// test unchanged.
				for i := 0; i < 2; i++ {
					want := vectorAddress(t, vector, c.chain, i)
					if !strings.Contains(got, want) {
						t.Errorf("%s %d missing or wrong: want %s\n%s", c.label, i, want, got)
					}
				}
			}
		})
	}
}

// TestWalletPolicyConsentNeverHidesTheAbsenceOfAddresses.
//
// An absent address block looks exactly like a screen that never had addresses
// on it, and "I did not see any addresses" is precisely the observation that
// should stop an operator. So each no-address case must SAY so, and the two
// reasons must be distinguishable: a keyless template was engraved that way on
// purpose (D4), while a keyed policy this device cannot derive is a capability
// gap (F-214). An operator who reads the second as the first concludes their
// card was stripped of its keys.
func TestWalletPolicyConsentNeverHidesTheAbsenceOfAddresses(t *testing.T) {
	keyless := consentText(t, "keyless_tr_with_leaf")
	if !strings.Contains(keyless, "Template-ID:") {
		t.Errorf("a keyless template must be labelled with the key-STABLE id:\n%s", keyless)
	}
	if strings.Contains(keyless, "Policy-ID:") {
		t.Errorf("a keyless template has no policy id to show:\n%s", keyless)
	}
	if !strings.Contains(keyless, "no addresses") {
		t.Errorf("the keyless case does not say why there are none:\n%s", keyless)
	}

	gap := consentText(t, "gap_tr_leaf_pkh")
	if !strings.Contains(gap, "can't derive") {
		t.Errorf("an underivable KEYED policy does not say so:\n%s", gap)
	}
	if strings.Contains(gap, "Keyless") || strings.Contains(gap, "no keys") {
		t.Errorf("an underivable policy that HAS keys must not read as keyless:\n%s", gap)
	}
	// It must also not silently claim an address.
	for _, c := range []string{"Receive 0:", "Change 0:"} {
		if strings.Contains(gap, c) {
			t.Errorf("a policy this device cannot derive shows an address label %q:\n%s", c, gap)
		}
	}
}

// TestWalletPolicyRefusesAnAmbiguousSupply.
//
// One md1 is the wallet. Two is a question this flow cannot answer, and a secret
// card must never be tolerated alongside a policy — ms1 is refused upstream at
// classify, and this is the defensive second line.
func TestWalletPolicyRefusesAnAmbiguousSupply(t *testing.T) {
	one := bundleCard{kind: cardMD1, strings: []string{"md1a"}}
	key := bundleCard{kind: cardMK1, strings: []string{"mk1a"}}
	secret := bundleCard{kind: cardMS1, strings: []string{"ms1a"}}

	if _, ok := walletPolicyMd1([]bundleCard{one}); !ok {
		t.Error("one md1 must be accepted")
	}
	// Key cards are legitimate cargo here, unlike Engrave Multisig's supply:
	// bundleEngrave cuts every card, and an operator may hold the key plates.
	if _, ok := walletPolicyMd1([]bundleCard{one, key}); !ok {
		t.Error("an mk1 key card alongside the policy must be accepted")
	}
	if _, ok := walletPolicyMd1([]bundleCard{one, one}); ok {
		t.Error("two md1 cards are ambiguous and must be refused")
	}
	if _, ok := walletPolicyMd1([]bundleCard{key}); ok {
		t.Error("no md1 means nothing to prove or engrave")
	}
	if _, ok := walletPolicyMd1([]bundleCard{one, secret}); ok {
		t.Error("a SECRET card alongside a wallet policy must be refused")
	}
}

// TestWalletPolicyProgramIsNavigableAndOpens is the can-a-user-do-the-thing
// gate: the enum entry, the title, the dispatch arm and the flow have to be
// joined, and every one of them is a separate edit in a different file.
//
// It taps to the entry through the drawer and selects it, rather than calling
// walletPolicyFlow directly — calling the flow proves the flow runs, which is
// the one thing that was never in doubt.
func TestWalletPolicyProgramIsNavigableAndOpens(t *testing.T) {
	ctx := NewContext(newPlatform())
	frame, drawer, quit := runUITouch(ctx, func() { uiFlow(ctx, "test") })
	defer quit()
	if _, ok := frame(); !ok {
		t.Fatal("uiFlow produced no frame")
	}
	_, right := arrowPoints(ctx)
	// Wallet Policy sits directly after Engrave Multisig, which is the seventh
	// entry — so seven right taps from Backup Wallet.
	var content string
	for i := 0; i < 7; i++ {
		tap(&ctx.Router, drawer(), right)
		c, ok := frame()
		if !ok {
			t.Fatalf("no frame after tap #%d", i+1)
		}
		content = c
	}
	if !uiContains(content, "Wallet Policy") {
		t.Fatalf("seven right taps did not reach Wallet Policy; got %q", content)
	}
	// click, not tapNavSlot, and the reason is specific to this screen: the
	// carousel's right-arrow Clip covers the WHOLE right edge, so a hit test at
	// any nav slot resolves to the arrow rather than to the checkmark drawn
	// under it. Measured — b1, b2 and b3 all report the `right` Clickable here.
	// Button3 is the physical select on this device, so this is still a real
	// operator action, just not one the drawer can be asked about.
	click(&ctx.Router, Button3)
	got, ok := pumpUntil(frame, "Wallet Policy", 32)
	if !ok {
		t.Fatalf("selecting Wallet Policy did not open its flow; got %q", got)
	}
	// The carousel entry and the flow share a title, so confirm we left the
	// carousel: the gather screen is what the flow opens on.
	if uiContains(got, "SeedHammer") {
		t.Fatalf("still on the start screen after selecting Wallet Policy; got %q", got)
	}
}
