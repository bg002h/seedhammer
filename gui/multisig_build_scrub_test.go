package gui

import (
	"fmt"
	"testing"
	"testing/synctest"

	"github.com/btcsuite/btcd/chaincfg/v2"
	"seedhammer.com/bip39"
	"seedhammer.com/md"
	"seedhammer.com/mk"
)

// ─── S4: SPEC 4.2's mandated scrub test, which no stage of the plan had claimed ─
//
// 4.2 is normative REQUIRED and ends "A test MUST prove the scrub, and that test
// MUST be mutation-checked." `grep -i scrub` over the implementation plan
// returned nothing before S4 was written.
//
// WHY IT MATTERS NOW AND DID NOT BEFORE. The shipped flow typed ONE seed and
// scrubbed it with a `defer` at the point of entry: sound, and unfalsifiable in
// the interesting direction, because there was one seed and one deferred site.
// S4 replaces that with a seedID-keyed registry and multiplies the exits -- the
// slot-source picker's Back, the seed picker's Back, the passphrase screens, the
// gate's FAIL screens, the slot review's Back, the plate census's Back, the tail
// abort, a ctx.Done unwind. A registry missing ONE of them leaves N masters'
// seeds in RAM and nothing else would notice.
//
// Precedent: TestBip85DeriveFlow_ScrubsBothMnemonics, which is the same shape
// (observe through a hook, assert the []Word slices are zeroed on exit).

// scrubFixtureRecords is the payload every exit class below runs against: one
// cosigner card plus master A as a ClassMnemonic, so the seed can be taken from
// the payload without a keyboard and each subtest is a few taps rather than
// twelve words.
// The card is B@0, NOT A@0: master A's own account-0 card next to master A's
// seed is the duplicate-key collision S2 refuses, so it would end these walks at
// a different screen than the one each is aiming at.
func scrubFixtureRecords(t *testing.T) []string {
	t.Helper()
	sets := cosignerCardFixtures(t, 2)
	recs := append([]string{}, sets[1]...) // B@0
	return append(recs, fixtureMasterA)
}

// buildDriveToSeed walks the flow to the point where a seed has been entered and
// registered, taking the seed FROM THE PAYLOAD. It returns with the passphrase
// prompt on screen.
//
// The parameter pickers are all defaults: wsh, n=2, k=1, @0, fp omit. With one
// card on the payload and n=2, `len(supply) >= p.N` is false, so S4's
// slot-source question is not asked -- the flow reaching the seed proves that,
// because a screen that appeared and was not tapped would stall the walk.
func buildDriveToSeed(t *testing.T, ctx *Context, frame func() (string, bool)) {
	t.Helper()
	buildWalkParamPickers(t, ctx, frame)
	if c, ok := pumpUntil(frame, "mk1 keys: 1", 64); !ok {
		t.Fatalf("the gather never showed the payload card; got %q", c)
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
	click(&ctx.Router, Button3) // FROM PAYLOAD, row 0
	frame()
	if c, ok := pumpUntil(frame, "systemwide payload", 64); !ok {
		t.Fatalf("the payload-source acceptance screen was not shown; got %q", c)
	}
	click(&ctx.Router, Button3)
	frame()
	if c, ok := pumpUntil(frame, "Add a BIP-39 passphrase?", 64); !ok {
		t.Fatalf("the passphrase prompt was not reached; got %q", c)
	}
}

// assertScrubbed fails when any observed mnemonic still holds a word.
func assertScrubbed(t *testing.T, exit string, seen []bip39.Mnemonic) {
	t.Helper()
	if len(seen) == 0 {
		t.Fatalf("%s: the hook never observed a seed, so this exit class asserts "+
			"nothing at all", exit)
	}
	for i, m := range seen {
		if len(m) == 0 {
			t.Fatalf("%s: observed seed %d is empty", exit, i)
		}
		for j, w := range m {
			if w != 0 {
				t.Fatalf("%s: seed %d word %d is still %d. A seed left live past this "+
					"exit is readable by whatever runs next on this device", exit, i, j, w)
			}
		}
	}
}

// TestBuildFlowScrubsEverySeedOnEveryExit is SPEC 4.2's test.
//
// Each subtest is one EXIT CLASS, driven to that exit through the real flow and
// then asserted on the words the hook saw. The mutation is deleting the one
// scrub site (`defer reg.scrub()` in buildMultisigPolicyFlow): every subtest
// below must go red, which is what proves each of them is looking.
func TestBuildFlowScrubsEverySeedOnEveryExit(t *testing.T) {
	records := scrubFixtureRecords(t)

	// EXIT 1: the passphrase prompt's own Back, the first exit that exists after
	// a seed is live. It is also the exit the shipped `defer` did NOT cover,
	// because that defer was installed after the passphrase step.
	t.Run("Back at the passphrase prompt", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			var seen []bip39.Mnemonic
			buildMultisigSeedHook = func(m bip39.Mnemonic) { seen = append(seen, m) }
			defer func() { buildMultisigSeedHook = nil }()
			ctx := NewContext(newPlatform())
			ctx.sysw = sessionHolding(records...)
			done := false
			frame, quit := runUI(ctx, func() {
				buildMultisigPolicyFlow(ctx, &descriptorTheme)
				done = true
			})
			defer quit()
			buildDriveToSeed(t, ctx, frame)
			click(&ctx.Router, Down) // "Add passphrase"
			frame()
			click(&ctx.Router, Button3)
			// This payload holds no ClassPassphrase, so syswOffer has nothing to
			// offer and the keyboard is reached directly. The keyboard still names
			// the slot, which is what makes it identifiable here.
			if c, ok := pumpUntil(frame, "Passphrase @0", 64); !ok {
				t.Fatalf("the per-seed passphrase keyboard was not reached; got %q", c)
			}
			click(&ctx.Router, Button1) // Back out of the keyboard: no passphrase
			// The flow continues from here; drive it to ANY exit and the seed must
			// be gone. The nearest one is the slot review's Back.
			if c, ok := pumpUntil(frame, "Key sources", 96); !ok {
				t.Fatalf("the slot-source review was not reached; got %q", c)
			}
			click(&ctx.Router, Button1) // Back -> abandon
			for i := 0; i < 64 && !done; i++ {
				frame()
			}
			if !done {
				t.Fatal("the flow did not return after the slot review's Back")
			}
			assertScrubbed(t, "slot-review Back", seen)
		})
	})

	// EXIT 2: a GATE FAIL screen. This is the exit S4 creates, and the one with
	// the most seeds live behind it.
	t.Run("the gate FAIL screen", func(t *testing.T) {
		// masterC's own card at slot @0 asserted as the operator's, against
		// masterA's seed: the mismatch row.
		sets := cosignerCardFixtures(t, 3)
		var recs []string
		recs = append(recs, sets[2]...) // C@0
		recs = append(recs, sets[1]...) // B@0
		recs = append(recs, fixtureMasterA)
		synctest.Test(t, func(t *testing.T) {
			var seen []bip39.Mnemonic
			buildMultisigSeedHook = func(m bip39.Mnemonic) { seen = append(seen, m) }
			defer func() { buildMultisigSeedHook = nil }()
			ctx := NewContext(newPlatform())
			ctx.sysw = sessionHolding(recs...)
			done := false
			frame, quit := runUI(ctx, func() {
				buildMultisigPolicyFlow(ctx, &descriptorTheme)
				done = true
			})
			defer quit()
			buildWalkParamPickers(t, ctx, frame) // n=2, so 2 cards >= 2 -> the question
			if c, ok := pumpUntil(frame, "Is your @0 key on a card?", 64); !ok {
				t.Fatalf("the slot-source question was not asked; got %q", c)
			}
			click(&ctx.Router, Down) // YES, CHECK THE CARD
			frame()
			click(&ctx.Router, Button3)
			frame()
			if c, ok := pumpUntil(frame, "mk1 keys: 2", 64); !ok {
				t.Fatalf("the gather never showed both cards; got %q", c)
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
			content, ok := pumpUntil(frame, "Key does not match seed", 96)
			if !ok {
				t.Fatalf("the gate did not refuse a `both` slot holding another "+
					"master's key; got %q", content)
			}
			click(&ctx.Router, Button3) // dismiss the modal -> the flow returns
			for i := 0; i < 64 && !done; i++ {
				frame()
			}
			if !done {
				t.Fatal("the flow did not return after the gate FAIL screen")
			}
			assertScrubbed(t, "gate FAIL", seen)
		})
	})

	// EXIT 3: the TAIL ABORT. Back at the EXPERIMENTAL warning is the last exit
	// before anything is cut, and it is furthest from the seed's entry.
	t.Run("Back at the EXPERIMENTAL warning", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			var seen []bip39.Mnemonic
			buildMultisigSeedHook = func(m bip39.Mnemonic) { seen = append(seen, m) }
			defer func() { buildMultisigSeedHook = nil }()
			ctx := NewContext(newPlatform())
			ctx.sysw = sessionHolding(records...)
			done := false
			frame, quit := runUI(ctx, func() {
				buildMultisigPolicyFlow(ctx, &descriptorTheme)
				done = true
			})
			defer quit()
			buildDriveToSeed(t, ctx, frame)
			click(&ctx.Router, Button3) // Skip the passphrase
			if c, ok := pumpUntil(frame, "Key sources", 96); !ok {
				t.Fatalf("the slot-source review was not reached; got %q", c)
			}
			click(&ctx.Router, Button3) // continue past the review
			// The review is PAGED and the stub is on a later page; the title is what
			// is on screen when it opens.
			if c, ok := pumpUntil(frame, "Policy Review", 96); !ok {
				t.Fatalf("the policy review was not reached; got %q", c)
			}
			click(&ctx.Router, Button3)
			if c, ok := pumpUntil(frame, "Which md1?", 64); !ok {
				t.Fatalf("the wallet-policy form was not reached; got %q", c)
			}
			click(&ctx.Router, Button3) // Full policy md1
			if c, ok := pumpUntil(frame, "EXPERIMENTAL", 64); !ok {
				t.Fatalf("the EXPERIMENTAL warning was not reached; got %q", c)
			}
			click(&ctx.Router, Button1) // Back -> ConfirmNo -> abort the engrave
			for i := 0; i < 64 && !done; i++ {
				frame()
			}
			if !done {
				t.Fatal("the flow did not return after aborting at the warning")
			}
			assertScrubbed(t, "tail abort", seen)
		})
	})

	// EXIT 4: the ctx.Done UNWIND. Nothing here presses anything: the UI is torn
	// down under the flow while a seed is live, which is what a shutdown, a wipe
	// or an idle timeout does.
	t.Run("ctx.Done unwind", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			var seen []bip39.Mnemonic
			buildMultisigSeedHook = func(m bip39.Mnemonic) { seen = append(seen, m) }
			defer func() { buildMultisigSeedHook = nil }()
			ctx := NewContext(newPlatform())
			ctx.sysw = sessionHolding(records...)
			frame, quit := runUI(ctx, func() { buildMultisigPolicyFlow(ctx, &descriptorTheme) })
			buildDriveToSeed(t, ctx, frame)
			if len(seen) == 0 {
				t.Fatal("no seed was live when the unwind was triggered")
			}
			quit() // tears the iterator down; the flow's defers run
			synctest.Wait()
			assertScrubbed(t, "ctx.Done unwind", seen)
		})
	})
}

// TestSeedEntryScreensNameTheirSlot: with several seeds in one flow, every
// seed-entry and passphrase screen must say WHICH SLOT it is for.
//
// Unlabelled, the operator cannot tell the second seed prompt from a repeat of
// the first, and a passphrase entered against the wrong slot mints a key no SPEC
// 4.3 row can catch -- there is no card to cross-check a new-seed slot against,
// which is exactly why this is a screen-text requirement rather than a check.
//
// The typed route is driven here rather than the payload one, because it is the
// route with the most screens: source picker, word-count picker, word entry.
func TestSeedEntryScreensNameTheirSlot(t *testing.T) {
	// A payload with a card and a PASSPHRASE but no ClassMnemonic: the seed must
	// be typed, and the passphrase source picker still appears.
	records := append(cosignerCardRecords(t, 1), "pass:6162616e646f6e2061626f7574")
	synctest.Test(t, func(t *testing.T) {
		ctx := NewContext(newPlatform())
		ctx.sysw = sessionHolding(records...)
		frame, quit := runUI(ctx, func() { buildMultisigPolicyFlow(ctx, &descriptorTheme) })
		defer quit()
		buildWalkParamPickers(t, ctx, frame) // n=2, @0
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

		// WORD-COUNT PICKER. With no ClassMnemonic the source picker has one row
		// and is deliberately not drawn, so this is the first seed screen.
		c, ok := pumpUntil(frame, "Choose number of words", 64)
		if !ok {
			t.Fatalf("the word-count picker was not reached; got %q", c)
		}
		if !uiContains(c, "Seed for @0") {
			t.Errorf("the word-count screen does not name its slot: %q", c)
		}
		click(&ctx.Router, Button3) // 12 WORDS
		frame()

		// WORD ENTRY, the long screen. It must name the slot AND keep the counter:
		// a title that replaced "Word N of M" would take the only progress
		// indicator a 24-word entry has.
		c, ok = pumpUntil(frame, "word 1 of 12", 64)
		if !ok {
			t.Fatalf("word entry was not reached; got %q", c)
		}
		if !uiContains(c, "@0") {
			t.Errorf("the word screen does not name its slot: %q", c)
		}
		driveWords(&ctx.Router, abandonAboutPhrase())

		// THE PASSPHRASE PROMPT.
		c, ok = pumpUntil(frame, "Add a BIP-39 passphrase?", 96)
		if !ok {
			t.Fatalf("the passphrase prompt was not reached; got %q", c)
		}
		if !uiContains(c, "@0") {
			t.Errorf("the passphrase prompt does not name its slot: %q", c)
		}
		click(&ctx.Router, Down)
		frame()
		click(&ctx.Router, Button3) // Add passphrase

		// AND THE PASSPHRASE SOURCE, which is a second screen with its own title.
		c, ok = pumpUntil(frame, "Password from where?", 64)
		if !ok {
			t.Fatalf("the passphrase source was not offered; got %q", c)
		}
		if !uiContains(c, "@0") {
			t.Errorf("the passphrase source screen does not name its slot: %q", c)
		}
		click(&ctx.Router, Down) // ENTER IT -> the keyboard
		frame()
		click(&ctx.Router, Button3)
		c, ok = pumpUntil(frame, "@0", 64)
		if !ok {
			t.Fatalf("the passphrase keyboard does not name its slot; got %q", c)
		}
	})
}

// TestPerSeedPassphraseBindsToItsOwnSeed is SPEC 4.1's per-seed passphrase,
// which had an implementation bullet and no test.
//
// The spec says no row of 4.3 can catch a violation -- a new-seed slot has no
// card to cross-check against -- so this is the only thing that can. Two seeds
// with different passphrases derive two different keys, and the gate must use
// EACH seed's own pairing.
//
// MUTATION: make buildSlotGate derive with reg.seeds[0].Passphrase for every
// slot (a flow-global passphrase) and the second slot's honest card stops
// matching, so this test goes red.
func TestPerSeedPassphraseBindsToItsOwnSeed(t *testing.T) {
	net := &chaincfg.MainNetParams
	mA, err := bip39.ParseMnemonic(fixtureMasterA)
	if err != nil {
		t.Fatalf("ParseMnemonic A: %v", err)
	}
	mB, err := bip39.ParseMnemonic(fixtureMasterB)
	if err != nil {
		t.Fatalf("ParseMnemonic B: %v", err)
	}
	reg := &seedRegistry{}
	idA, err := reg.add("your seed for @0", mA, "alpha passphrase", net)
	if err != nil {
		t.Fatalf("registering seed A: %v", err)
	}
	idB, err := reg.add("your seed for @1", mB, "beta passphrase", net)
	if err != nil {
		t.Fatalf("registering seed B: %v", err)
	}

	// THE PREMISE, asserted rather than assumed: the same words under two
	// passphrases are two different wallets. If this were false the test below
	// could not tell a per-seed binding from a global one.
	xa, _, err := deriveAccountXpub(mA, "alpha passphrase", net, multisigSharedOrigin())
	if err != nil {
		t.Fatalf("deriving A under its own passphrase: %v", err)
	}
	xaWrong, _, err := deriveAccountXpub(mA, "beta passphrase", net, multisigSharedOrigin())
	if err != nil {
		t.Fatalf("deriving A under the other passphrase: %v", err)
	}
	if xa == xaWrong {
		t.Fatal("one seed derived the same account key under two different " +
			"passphrases; the passphrase is not reaching the KDF and nothing below " +
			"can be concluded")
	}

	// An honest card per slot, each carrying the key ITS OWN pair derives.
	honest := func(id int) mk.Card {
		seed, ok := reg.at(id)
		if !ok {
			t.Fatalf("seed %d is not registered", id)
		}
		xpub, fp, derr := deriveAccountXpub(seed.Mnemonic, seed.Passphrase, net, multisigSharedOrigin())
		if derr != nil {
			t.Fatalf("deriving seed %d: %v", id, derr)
		}
		return mk.Card{
			Network:     "mainnet",
			Path:        multisigSharedOrigin().String(),
			Fingerprint: fmt.Sprintf("%08x", fp),
			Stubs:       [][4]byte{fixtureStub},
			Xpub:        xpub,
		}
	}
	cardA, cardB := honest(idA), honest(idB)
	if cardA.Xpub == cardB.Xpub {
		t.Fatal("the two slots' cards carry the same key, so a global passphrase " +
			"would be indistinguishable from a per-seed one")
	}
	sources := []slotSource{
		{Kind: slotFromBoth, SeedID: idA, Card: 0},
		{Kind: slotFromBoth, SeedID: idB, Card: 1},
	}
	if _, gerr := buildSlotGate(sources, md.MultisigWsh, reg, []mk.Card{cardA, cardB}, net); gerr != nil {
		t.Fatalf("the gate refused two slots each holding the key ITS OWN "+
			"(seed, passphrase) pair derives: %v. A flow-global passphrase applied "+
			"to both is exactly what this looks like", gerr)
	}

	// AND THE CONVERSE, so the assertion above is not passing for want of any
	// check at all: seed B's slot pointed at seed A's card must fail.
	crossed := []slotSource{
		{Kind: slotFromBoth, SeedID: idB, Card: 0},
	}
	if _, gerr := buildSlotGate(crossed, md.MultisigWsh, reg, []mk.Card{cardA}, net); gerr == nil {
		t.Fatal("seed B accepted seed A's card, so the gate is not comparing " +
			"anything per-seed")
	}
}
