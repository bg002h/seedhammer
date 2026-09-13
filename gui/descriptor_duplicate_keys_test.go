package gui

import (
	"strings"
	"testing"
	"testing/synctest"

	"seedhammer.com/address"
)

// ─── F-530: the duplicate-key rule over a *bip380.Descriptor ─────────────────
//
// F-531 closed the md1 routes. This is the other half, and it needs a DIFFERENT
// rule: of descriptorFlow's three callers, TWO arrive with no md1 behind them --
// a scanned descriptor (gui/gui.go) and a payload record through
// nonstandard.OutputDescriptor (gui/wallet_policy.go) -- so there is no
// md.Template to ask repeatsASeat about and no chunk set to ask
// md.DuplicateKeySlotChunks about.
//
// Threading chunks into the one caller that has them would have closed a third
// of it and left the two that matter silent under a suite that then looks
// complete. That is how the first surface stayed hidden through a whole review
// round (F-514 -> F-530).
//
// bip380.Parse admits sortedmulti and single keys and checks nothing about
// repeats, so this is reachable by scanning a QR.

const (
	// Two real mainnet xpubs, the same pair the md1 fixtures use, so the
	// addresses these produce were measured against Bitcoin Core 25.0.0.
	f530XpubA = "xpub6DXuQW1Q2JpZwAAqEXV3TH3EaHDaYUgycatHVPBFG82MVUTmCrMBGkGzVXHVtBbBro3eogN1J1AaB7cJAxtyHeA4Ud2k1H1Xo7ZL2eP89Zk"
	f530XpubB = "xpub6DXuQW1Q2JpZwUinKYo5uGquSNEFRohrpfGAm22JC2rpmMFxn2rBmQte451rsd8nDxB4wQNfhx75NyLKR1W26RJarEnBXK1knZZVVJRsqMD"

	// The defect's shape: one key at two of the three seats.
	descRepeatedKey = "wsh(sortedmulti(2," + f530XpubA + "," + f530XpubA + "," + f530XpubB + "))"
	// The control: three seats, two distinct keys, no repeat.
	descDistinctKeys = "wsh(sortedmulti(2," + f530XpubA + "," + f530XpubB + "))"
	// THE ONE THAT MUST NOT BE REFUSED. Same xpub, DISJOINT derivation -- BIP
	// 388 permits exactly this on one placeholder, and the two key expressions
	// derive different pubkeys, so the script holds two different keys and
	// nothing is reused. A predicate that ignored Children would refuse a legal
	// wallet, which is the expensive direction of wrong.
	descDisjointChildren = "wsh(sortedmulti(2," + f530XpubA + "/0/*," + f530XpubA + "/1/*))"
)

func TestDescriptorRepeatsAKey(t *testing.T) {
	for _, tc := range []struct {
		name string
		desc string
		want bool
		why  string
	}{
		{"repeated key", descRepeatedKey, true,
			"the same key expression fills two of the three seats"},
		{"distinct keys", descDistinctKeys, false,
			"two different keys; a predicate firing here cries wolf"},
		{"same xpub, disjoint children", descDisjointChildren, false,
			"BIP 388 permits one key at DISJOINT paths, and these derive different pubkeys"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			desc := loadTestDesc(t, tc.desc)
			a, b, got := descriptorRepeatsAKey(desc)
			if got != tc.want {
				t.Fatalf("descriptorRepeatsAKey = %v (positions %d,%d), want %v -- %s",
					got, a, b, tc.want, tc.why)
			}
			if got && !(a >= 0 && b > a && b < len(desc.Keys)) {
				t.Errorf("positions %d,%d are not two distinct in-range key indices of %d",
					a, b, len(desc.Keys))
			}
		})
	}
}

// TestDisjointChildrenStillDerive is the other half of the false-positive
// guard: not refusing is only meaningful if the wallet still works.
func TestDisjointChildrenStillDerive(t *testing.T) {
	desc := loadTestDesc(t, descDisjointChildren)
	if !address.Supported(desc) {
		t.Fatal("the disjoint-children descriptor does not derive at all, so the " +
			"not-a-duplicate verdict above proves nothing about an operator's screen")
	}
	a, err := address.Receive(desc, 0)
	if err != nil || a == "" {
		t.Fatalf("address.Receive: %q %v", a, err)
	}
}

// TestScannedRepeatedKeyDescriptorOffersNoAddresses drives the REAL screen.
//
// Not descriptorRepeatsAKey, and not address.Supported: the question is what an
// operator who scanned this descriptor can reach. The Addresses button opens a
// choice of "Show addresses" and "Verify an address", and BOTH derive --
// runVerify calls address.Find and answers "Controlled by this descriptor",
// which is an endorsement of a specific address the operator is about to send
// to. One gate covers both because both sit behind the one button.
func TestScannedRepeatedKeyDescriptorOffersNoAddresses(t *testing.T) {
	desc := loadTestDesc(t, descRepeatedKey)

	// PREMISE, MEASURED: without a gate this descriptor derives happily. If
	// address.Supported ever returns false on its own, this test would pass for
	// the wrong reason and the gate below would be untested.
	if !address.Supported(desc) {
		t.Fatal("address.Supported is already false for the repeated-key descriptor, so " +
			"this test cannot tell a policy refusal from a capability gap")
	}

	synctest.Test(t, func(t *testing.T) {
		p := newPlatform()
		p.display = sh2DisplaySize
		ctx := NewContext(p)
		frame, quit := runUI(ctx, func() {
			descriptorFlow(ctx, &descriptorTheme, desc)
		})
		defer quit()

		// The screen must SAY why, not merely withhold. An address button that
		// is simply absent is indistinguishable from a device that has none --
		// the silent-refusal failure the F-531 review caught one level up.
		if got, ok := pumpUntil(frame, composerCopyDescriptorRepeatsAKey()[:40], 24); !ok {
			t.Errorf("the Engrave Descriptor screen shows a wallet with one key at two "+
				"seats and never says so.\nLast frame: %q", got)
		}

		// And Button2 must not open the Addresses choice.
		click(&ctx.Router, Button2)
		for i := 0; i < 8; i++ {
			c, ok := frame()
			if !ok {
				break
			}
			if uiContains(c, "Show addresses") || uiContains(c, "Verify an address") {
				t.Fatalf("the Addresses choice opened for a descriptor that reuses a key; "+
					"both of its branches derive.\nFrame: %q", c)
			}
			if uiContains(c, "bc1q") || uiContains(c, "bc1p") {
				t.Fatalf("an address reached the screen for a descriptor that reuses a "+
					"key.\nFrame: %q", c)
			}
		}
	})
}

// TestDistinctKeyDescriptorStillOffersAddresses is the negative control at the
// same layer: the gate must not close the screen for every multisig.
func TestDistinctKeyDescriptorStillOffersAddresses(t *testing.T) {
	desc := loadTestDesc(t, descDistinctKeys)
	synctest.Test(t, func(t *testing.T) {
		p := newPlatform()
		p.display = sh2DisplaySize
		ctx := NewContext(p)
		frame, quit := runUI(ctx, func() {
			descriptorFlow(ctx, &descriptorTheme, desc)
		})
		defer quit()
		frame()
		click(&ctx.Router, Button2)
		if !frameUntil(frame, "Show addresses", 12) {
			t.Error("the Addresses choice did not open for an ordinary 2-of-2 with two " +
				"distinct keys, so F-530's gate is refusing wallets it has no quarrel with")
		}
	})
}

// TestTheDescriptorWarningIsDrawnInFull is the fit gate for the sentences
// F-530 adds to DescriptorScreen.
//
// THIS SCREEN NEITHER SCROLLS NOR CLIPS -- Draw offsets the body and returns it,
// with no scroll arrows and no page button -- so anything past the bottom of the
// viewport is simply not there. The modal fit gate does not cover it, because
// this is screen body text and not a Warning.
//
// THE EXTRACTOR RESPECTS CLIPPING, measured rather than assumed: padding the
// body past the viewport drops the overflowing lines from the frame text
// entirely (a 20-line filler left 2 of 20 on the frame and lost a trailing
// sentinel). So a prefix match here is evidence about pixels, not about which
// ops were emitted -- which is the difference between this gate and a false
// PASS.
//
// The headroom is measured through the descriptor's own Title, which Draw
// renders above everything else, so it is the screen's own input rather than a
// seam added for the test.
func TestTheDescriptorWarningIsDrawnInFull(t *testing.T) {
	// THE SCREEN IS NEARLY FULL, and the margin is set below the measured
	// slack rather than at it: a threshold a test passes EXACTLY will flap on a
	// font metric change and teach everyone to raise it. If this fails, the
	// answer is shorter copy -- there is no more room on this screen, and the
	// sentences cannot move to a second page because there is no page button.
	const margin = 24 // characters of Title the screen must still absorb

	both := composerCopyDescriptorRepeatsAKey() + composerCopyNoAddressesDuplicateKeys()
	drawn := drawDescriptorScreen(t, descRepeatedKey, 0)
	if ok, drew, want := bodyDrawnFully(drawn, both); !ok {
		t.Fatalf("the Engrave Descriptor screen draws %d of the warning's %d characters. "+
			"The rest is off the bottom of a screen that does not scroll, so it is text "+
			"the operator cannot reach at all.\nlost: %q",
			drew, want, normalizeDrawn(both)[drew:])
	}

	// Headroom: the longest Title that still leaves the whole warning on screen.
	head := 0
	for n := 0; n <= 400; n += 4 {
		if ok, _, _ := bodyDrawnFully(drawDescriptorScreen(t, descRepeatedKey, n), both); !ok {
			break
		}
		head = n
	}
	t.Logf("the warning fits with %d characters of Title to spare (margin %d)", head, margin)
	if head < margin {
		t.Errorf("the warning fits today with only %d characters to spare, under the %d "+
			"margin. F-185's fix failed exactly here: a body that still fit could be "+
			"re-broken without turning a test red. Shorten the sentences rather than "+
			"lowering the margin.", head, margin)
	}
}

// drawDescriptorScreen renders the Engrave Descriptor screen's first frame and
// returns its extracted text. titlePad characters of Title push the body down,
// which is how the headroom above is measured.
func drawDescriptorScreen(t *testing.T, descStr string, titlePad int) string {
	t.Helper()
	desc := loadTestDesc(t, descStr)
	if titlePad > 0 {
		desc.Title = strings.Repeat("x", titlePad)
	}
	var out string
	synctest.Test(t, func(t *testing.T) {
		p := newPlatform()
		p.display = sh2DisplaySize
		ctx := NewContext(p)
		frame, quit := runUI(ctx, func() {
			descriptorFlow(ctx, &descriptorTheme, desc)
		})
		defer quit()
		out, _ = frame()
	})
	return out
}
