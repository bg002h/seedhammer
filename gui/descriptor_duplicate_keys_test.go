package gui

import (
	"strings"
	"testing"
	"testing/synctest"

	"seedhammer.com/address"
	"seedhammer.com/bip380"
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
// entirely. So a prefix match here is evidence about pixels, not about which
// ops were emitted -- which is the difference between this gate and a false
// PASS.
//
// MEASURED IN THE WORST TITLE THIS SCREEN WILL DRAW, not in an abstract
// character count. The first version padded with strings.Repeat("x", n) and
// certified "44 characters of Title to spare" -- review I-2 then overflowed the
// screen with a 25-character title, because 'x' is a narrow glyph and one
// unbroken token does not wrap. The number was not wrong about x's; it was not
// a claim about titles. maxTitleDrawn now bounds what is drawn, so the gate
// asserts the thing that matters directly: at the longest title the screen will
// render, in the widest glyph the face has, the whole warning is still on it.
func TestTheDescriptorWarningIsDrawnInFull(t *testing.T) {
	both := composerCopyDescriptorRepeatsAKey() + composerCopyNoAddressesDuplicateKeys()

	for _, tc := range []struct {
		name  string
		title string
	}{
		{"no title", ""},
		{"a realistic title", "Company Treasury Multisig Cold Storage 2026"},
		{"the worst title the screen draws", wideWords(maxTitleDrawn)},
		{"an unbounded scanned title", wideWords(400)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			drawn := drawDescriptorScreen(t, descRepeatedKey, tc.title)
			if ok, drew, want := bodyDrawnFully(drawn, both); !ok {
				t.Fatalf("the Engrave Descriptor screen draws %d of the warning's %d "+
					"characters with this title (%d chars). The rest is off the bottom of "+
					"a screen that does not scroll, so it is text the operator cannot "+
					"reach -- and the plate is still engravable from here.\nlost: %q",
					drew, want, len([]rune(tc.title)), normalizeDrawn(both)[drew:])
			}
		})
	}
}

// TestTheTitleCannotPushTheWarningOffTheScreen is review I-1 as an assertion
// rather than a length budget.
//
// The warning is drawn FIRST, above every line a scanned artefact controls, so
// no Title can displace it however long. This drives the length far past
// anything maxTitleDrawn would allow, because the ordering -- not the cap -- is
// what makes the guarantee, and a future edit moving the warning back below the
// Title must fail here even if the cap survives.
func TestTheTitleCannotPushTheWarningOffTheScreen(t *testing.T) {
	both := composerCopyDescriptorRepeatsAKey() + composerCopyNoAddressesDuplicateKeys()
	for _, n := range []int{50, 120, 200, 1000} {
		drawn := drawDescriptorScreen(t, descRepeatedKey, wideWords(n))
		if ok, drew, want := bodyDrawnFully(drawn, both); !ok {
			t.Fatalf("a %d-character scanned Title pushed the warning off the screen "+
				"(%d of %d characters drawn). An attacker-supplied field must not be able "+
				"to silence a funds warning; draw the warning above it.\nlost: %q",
				n, drew, want, normalizeDrawn(both)[drew:])
		}
	}
}

// TestTheDisplayedTitleIsBounded pins the cap itself, and that it is a DISPLAY
// cap: the descriptor keeps its full Title for everything downstream.
func TestTheDisplayedTitleIsBounded(t *testing.T) {
	long := wideWords(400)
	desc := loadTestDesc(t, descDistinctKeys)
	desc.Title = long
	drawn := drawDescriptorScreenDesc(t, desc)
	if strings.Contains(normalizeDrawn(drawn), normalizeDrawn(long)) {
		t.Error("the whole 400-character Title is drawn; a scanned string with no length " +
			"bound displaces whatever the device chose to say after it")
	}
	if desc.Title != long {
		t.Error("Draw mutated the descriptor's Title. The cap is for the screen only -- " +
			"what is engraved must not change because of how it was displayed")
	}
	if got := truncateForDisplay("abc", 48); got != "abc" {
		t.Errorf("truncateForDisplay shortened a short string to %q", got)
	}
	if got := truncateForDisplay(strings.Repeat("a", 60), 48); len([]rune(got)) != 51 {
		t.Errorf("truncateForDisplay returned %d runes, want 48 + the 3-char marker",
			len([]rune(got)))
	}
}

// wideWords builds n characters of word-wrapping text in the widest glyph the
// body face carries, which is what a real Title costs on this screen -- not the
// narrow unbroken token the first version of this gate measured.
func wideWords(n int) string {
	if n <= 0 {
		return ""
	}
	var b strings.Builder
	for b.Len() < n {
		if b.Len() > 0 {
			b.WriteByte(' ')
		}
		b.WriteString("mmmmmmmm")
	}
	return string([]rune(b.String())[:n])
}

// drawDescriptorScreen renders the Engrave Descriptor screen's first frame with
// the given Title and returns its extracted text.
func drawDescriptorScreen(t *testing.T, descStr, title string) string {
	t.Helper()
	desc := loadTestDesc(t, descStr)
	desc.Title = title
	return drawDescriptorScreenDesc(t, desc)
}

func drawDescriptorScreenDesc(t *testing.T, desc *bip380.Descriptor) string {
	t.Helper()
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

// ─── review C-1: the four spellings that derive identically ─────────────────
//
// The predicate compared the Children STRUCT while address.derivePubKey
// normalises before deriving, so each row below was a key written two ways that
// the gate called two different keys. The first was verified end to end: the
// screen opened the Addresses choice and listed the byte-identical address the
// refused fixture produces.
//
// EACH ROW IS A SEPARATE BYPASS, so each gets a row rather than one standing in
// for the family. Three of them differ from the fixture by a single token.
func TestEverySpellingOfTheSameKeyIsCaught(t *testing.T) {
	for _, tc := range []struct {
		name string
		a, b string
		why  string
	}{
		{"implicit vs explicit", f530XpubA, f530XpubA + "/<0;1>/*",
			"derivePubKey defaults empty Children to exactly <0;1>/*"},
		{"child vs range on receive", f530XpubA + "/0/*", f530XpubA + "/<0;1>/*",
			"a RangeDerivation resolves to Index on the receive chain -- the chain that gets funded"},
		{"hardened vs not", f530XpubA + "/0h/*", f530XpubA + "/0/*",
			"derivePubKey never consults Hardened"},
		{"hardened wildcard vs not", f530XpubA + "/*h", f530XpubA + "/*",
			"same reason, at the wildcard"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			desc := loadTestDesc(t, "wsh(sortedmulti(2,"+tc.a+","+tc.b+","+f530XpubB+"))")
			i, j, dup := descriptorRepeatsAKey(desc)
			if !dup {
				t.Fatalf("descriptorRepeatsAKey says these are different keys:\n  %s\n  %s\n"+
					"They are not: %s. A gate that reads the spelling instead of the "+
					"derivation is walked past by retyping the same wallet (review C-1).",
					tc.a, tc.b, tc.why)
			}
			if i != 0 || j != 1 {
				t.Errorf("positions %d,%d, want 0,1", i, j)
			}
		})
	}
}

// TestGenuinelyDifferentSpellingsAreNotCaught is the false-positive half, and it
// is the half that costs an operator a working wallet rather than costing them
// a warning.
func TestGenuinelyDifferentSpellingsAreNotCaught(t *testing.T) {
	for _, tc := range []struct {
		name string
		a, b string
		why  string
	}{
		{"different fixed children", f530XpubA + "/0/*", f530XpubA + "/1/*",
			"different child, different key, on both chains"},
		{"disjoint multipath", f530XpubA + "/<0;1>/*", f530XpubA + "/<2;3>/*",
			"BIP 388 permits one key at DISJOINT multipath sets, and these derive differently"},
		{"different xpubs", f530XpubA, f530XpubB,
			"two different keys, the ordinary case"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			desc := loadTestDesc(t, "wsh(sortedmulti(2,"+tc.a+","+tc.b+"))")
			if _, _, dup := descriptorRepeatsAKey(desc); dup {
				t.Fatalf("descriptorRepeatsAKey refuses a wallet whose keys differ:\n  %s\n  %s\n"+
					"%s. Refusing a good wallet is the expensive direction -- the "+
					"operator's recourse is to stop using this device.", tc.a, tc.b, tc.why)
			}
		})
	}
}

// TestTheReceiveChainAloneIsEnough pins the "either chain" rule, which is the
// one a reader is most likely to tighten into "both".
//
// A/0/* and A/<0;1>/* derive the SAME key on receive and DIFFERENT keys on
// change. Requiring both chains to collide would call that wallet safe -- while
// every address an operator is handed to fund comes from the chain where the
// two keys are one.
func TestTheReceiveChainAloneIsEnough(t *testing.T) {
	desc := loadTestDesc(t, "wsh(sortedmulti(2,"+f530XpubA+"/0/*,"+f530XpubA+"/<0;1>/*,"+f530XpubB+"))")
	if _, _, dup := descriptorRepeatsAKey(desc); !dup {
		t.Fatal("a collision on the receive chain alone is not reported. Receive is the " +
			"chain that gets funded and the one descriptorAddressFlow opens on")
	}
	recv, err := address.Receive(desc, 0)
	if err != nil {
		t.Fatalf("address.Receive: %v", err)
	}
	chg, err := address.Change(desc, 0)
	if err != nil {
		t.Fatalf("address.Change: %v", err)
	}
	if recv == chg {
		t.Fatal("receive and change agree for this fixture, so it does not demonstrate a " +
			"receive-only collision and this test is not testing the rule it names")
	}
}
