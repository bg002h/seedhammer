package gui

import (
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"seedhammer.com/bip39"
	"seedhammer.com/md"
	"seedhammer.com/mk"
	"seedhammer.com/sysw"
)

// ─── C-3: the SUPPLY path must not label a passphrase build "Full" and stay ──
//         silent about it on the one artifact that outlives the operator
//
// A supply-path run in which the operator entered a BIP-39 passphrase engraves
// an ms1 that encodes the WORDS ONLY -- the passphrase is not in that entropy
// and no plate this device cuts can be made to yield it. The engrave-mode picker
// nevertheless read "Full (seed + keys)" and the restore document carried
// `extra == nil`, so no screen and no plate anywhere stated that a required
// spending factor was missing.
//
// That is F-132's shape, a backup that is both wrong and trusted, and S5
// DECLARED it must not ship -- in this very diff, in the code's own words at
// gui/multisig_build_census.go ("...is a LIE for one with a passphrase"). S5
// built buildFullModeLabel and buildPassphraseInventoryLines for exactly this
// harm and wired them to the BUILD path only. The result was an asymmetry S5
// itself created: the path that tells the truth sits behind the mandatory
// EXPERIMENTAL warning, and the hardware-validated FRONT-DOOR path is the one
// that lies.
//
// TEST MATERIAL, public by construction: master A is BIP-39's own published
// vector and "abandon about" is the emulator payload's own passphrase record.
// Never put funds behind them.

// s5PassphraseRecord is the payload's ClassPassphrase cell, verbatim from
// gui/sysw_cells_test.go: `pass:` is a RESERVED prefix (SYSW 5.3.1) and the body
// is lowercase hex because EPD 6.4 forbids the interior space raw.
const (
	s5PassphraseRecord = "pass:6162616e646f6e2061626f7574"
	s5Passphrase       = "abandon about"
)

// s5PassphrasedSupplyPolicy assembles a 2-of-2 wsh policy whose @0 key is master
// A derived UNDER a passphrase.
//
// THE FIXTURE ONLY MATCHES WITH THE PASSPHRASE, and that is measured rather than
// assumed: without it the supply path's cross-match finds no slot at all and
// refuses before the engrave mode is ever offered, so a test that forgot to
// supply the passphrase would fail on the route instead of passing over the
// wrong thing.
func s5PassphrasedSupplyPolicy(t *testing.T) (md1 []string, keys []md.ExpandedKey) {
	t.Helper()
	m, err := bip39.ParseMnemonic(fixtureMasterA)
	if err != nil {
		t.Fatalf("ParseMnemonic(master A): %v", err)
	}
	reg := &seedRegistry{}
	id, err := reg.add("your seed for @0", m, s5Passphrase, s5Net)
	if err != nil {
		t.Fatalf("registering the passphrased pair: %v", err)
	}
	p := buildPolicyParams{Script: md.MultisigWsh, N: 2, K: 2, SelfSlots: []int{0}}
	sources := buildSlotSources(p, []int{id}, []int{0}, reg)
	self, err := buildSelfKeys(sources, p.Script, reg, s5Net)
	if err != nil {
		t.Fatalf("buildSelfKeys: %v", err)
	}
	cards := []mk.Card{dupTestCard(t, 1)} // B@0, the other cosigner
	md1, _, _, err = assembleBuildPolicy(p, self, cards)
	if err != nil {
		t.Fatalf("the passphrased 2-of-2 did not assemble: %v", err)
	}

	_, keys, err = md.ExpandWalletPolicyChunks(md1)
	if err != nil {
		t.Fatalf("the assembled policy does not decode: %v", err)
	}
	// THE PREMISE, MEASURED, in both directions.
	fresh, err := bip39.ParseMnemonic(fixtureMasterA)
	if err != nil {
		t.Fatalf("ParseMnemonic(master A, 2nd): %v", err)
	}
	with := allUserSlots(fresh, s5Passphrase, s5Net, keys)
	if len(with) != 1 || with[0] != 0 {
		t.Fatalf("master A + %q fills slots %v of this policy, want exactly [0]; the "+
			"fixture's whole subject is a passphrase that is a LIVE derivation input",
			s5Passphrase, with)
	}
	without := allUserSlots(fresh, "", s5Net, keys)
	if len(without) != 0 {
		t.Fatalf("master A with NO passphrase already fills slots %v, so this fixture "+
			"does not require the passphrase and the test would pass without it", without)
	}
	return md1, keys
}

// s5EngraveOnePlate is engraveOnePlate with a budget measured for THIS walk
// rather than inherited from another.
//
// engraveOnePlate's 4096-frame cap is calibrated to Trace A's plates. This
// walk's ms1 plate reports 7:24 of engraving and consumes roughly 4900 frames at
// the UI's tick, so it overruns that cap and the shared helper reports "the
// engrave never closed the engraver" for a plate that was cutting perfectly.
// The budget is stated here, with why, instead of being raised for every caller
// of the shared helper -- a cap nobody can explain is one that gets raised again
// the next time rather than believed.
func s5EngraveOnePlate(t *testing.T, ctx *Context, frame func() (string, bool), e *testEngraver) {
	t.Helper()
	press(&ctx.Router, Button3)
	frame()
	time.Sleep(confirmDelay)
	for i := 0; i < 32768; i++ {
		select {
		case <-e.closes:
			click(&ctx.Router, Button3)
			frame()
			return
		default:
		}
		if _, ok := frame(); !ok {
			return
		}
	}
	t.Fatal("the engrave never closed the engraver, so no plate was cut")
}

// s5SupplyPassphraseWalk drives supplyMultisigPolicyFlow end to end with a
// payload-borne BIP-39 passphrase, in FULL mode, through every plate, to the
// restore document.
//
// IT DRIVES THE REAL SCREENS AND CUTS EVERY PLATE, because both halves of this
// finding are post-decision surfaces: the mode label is what the operator reads
// BEFORE pressing, and the restore doc is printed AFTER the last plate. A
// helper-level check of either would prove the string exists somewhere, which it
// already did -- buildFullModeLabel(true) returns the correct sentence today and
// is unreachable from this flow.
//
// It returns the engrave-mode frame and every page of the restore document.
func s5SupplyPassphraseWalk(t *testing.T) (modeScreen, restoreDoc string) {
	t.Helper()
	md1, _ := s5PassphrasedSupplyPolicy(t)
	if got := sysw.Classify(s5PassphraseRecord); got != sysw.ClassPassphrase {
		t.Fatalf("INCONCLUSIVE: the passphrase fixture classifies as %v", got)
	}
	if raw, err := sysw.DecodeBody(s5PassphraseRecord); err != nil || string(raw) != s5Passphrase {
		t.Fatalf("INCONCLUSIVE: the passphrase fixture decodes to (%q, %v), want %q",
			raw, err, s5Passphrase)
	}

	synctest.Test(t, func(t *testing.T) {
		e := newEngraver()
		p := newPlatform()
		p.display = sh2DisplaySize
		p.engraver = e
		ctx := NewContext(p)
		// The SESSION holds only the passphrase; the md1 arrives on the bundle
		// seam. So syswOffer(ClassMDMK) has nothing to offer and the flow's first
		// screen is the gather, exactly as it is with no payload at all.
		ctx.sysw = sessionHolding(s5PassphraseRecord)
		ctx.syswBundleSeeds = append([]string(nil), md1...)
		done := false
		frame, quit := runUI(ctx, func() {
			supplyMultisigPolicyFlow(ctx, &descriptorTheme)
			done = true
		})
		defer quit()

		if c, ok := pumpUntil(frame, "md1 descriptors: 1", 64); !ok {
			t.Fatalf("the supplied md1 never reached the gatherer's tally; got %q", c)
		}
		click(&ctx.Router, Button3) // Done adding cards
		frame()

		if c, ok := pumpUntil(frame, "Choose number of words", 64); !ok {
			t.Fatalf("the gather did not hand off to seed entry; got %q", c)
		}
		click(&ctx.Router, Button3) // 12 WORDS
		frame()
		driveWords(&ctx.Router, fixtureMasterA)

		// THE PASSPHRASE, taken from the payload rather than typed: syswPassphraseFlow
		// offers it because the session holds a ClassPassphrase record (SYSW 3.3.2
		// admits it to this program), which skips the keyboard entirely.
		if c, ok := pumpUntil(frame, "Add a BIP-39 passphrase?", 240); !ok {
			t.Fatalf("the passphrase prompt was not reached; got %q", c)
		}
		click(&ctx.Router, Down) // "Add passphrase"
		frame()
		click(&ctx.Router, Button3)
		frame()
		if c, ok := pumpUntil(frame, "Password from where?", 64); !ok {
			t.Fatalf("the payload's passphrase was not offered; got %q.\n"+
				"Without it this walk derives the EMPTY-passphrase wallet, which fills no "+
				"slot of the fixture policy and would fail on the cross-match instead", c)
		}
		click(&ctx.Router, Button3) // FROM PAYLOAD (index 0)
		frame()

		c, ok := pumpUntil(frame, "What to engrave?", 96)
		if !ok {
			t.Fatalf("the engrave-mode choice was not reached; got %q.\n"+
				"A refusal here means the passphrase never reached the cross-match", c)
		}
		modeScreen = c
		click(&ctx.Router, Button3) // row 0: the FULL row, whatever it is called
		frame()

		if c, ok := pumpUntil(frame, "Plates To Cut", 96); !ok {
			t.Fatalf("the plate census was not shown; got %q", c)
		}
		click(&ctx.Router, Button3)
		frame()

		plates := 0
		for {
			if _, ok := pumpUntil(frame, "Choose engraving", 96); !ok {
				break
			}
			click(&ctx.Router, Button3) // first variant
			frame()
			s5EngraveOnePlate(t, ctx, frame, e)
			plates++
			if plates > 24 {
				t.Fatal("the engrave loop did not terminate")
			}
		}
		if plates == 0 {
			t.Fatal("no plate was cut, so this walk never reached the post-engrave surfaces")
		}
		t.Logf("the passphrased supply run cut %d plate(s)", plates)

		if c, ok := pumpUntil(frame, "Verify the engraved plates?", 96); !ok {
			t.Fatalf("the verify offer was not reached after %d plate(s); got %q", plates, c)
		}
		click(&ctx.Router, Down) // Skip
		frame()
		click(&ctx.Router, Button3)
		frame()

		if c, ok := pumpUntil(frame, "Descriptor:", 96); !ok {
			t.Fatalf("the restore doc was not shown; got %q", c)
		}
		pages, _ := s5PageForNeedle(t, ctx, frame, "\x00never matches\x00", 16)
		restoreDoc = pages
		click(&ctx.Router, Button3) // done with the restore doc
		for i := 0; i < 256 && !done; i++ {
			frame()
		}
		if !done {
			t.Fatal("the flow did not return after the restore doc")
		}
	})
	return modeScreen, restoreDoc
}

// TestSupplyPassphraseRunTellsTheOperatorWhatIsMissing is C-3's arm, and both
// halves of it are funds-bearing.
func TestSupplyPassphraseRunTellsTheOperatorWhatIsMissing(t *testing.T) {
	modeScreen, restoreDoc := s5SupplyPassphraseWalk(t)
	t.Logf("engrave-mode screen: %q", modeScreen)
	t.Logf("restore doc: %q", restoreDoc)

	// (1) THE LABEL THE OPERATOR READS BEFORE PRESSING. "Full (seed + keys)" over
	// a passphrase build tells them a partial backup is a complete one, and the
	// label is where it has to be said: a note anywhere else is a note read after
	// the decision.
	if !uiContains(modeScreen, "NOT passphrase") {
		t.Errorf("the engrave-mode picker calls a PASSPHRASE build \"Full (seed + keys)\":\n%q\n"+
			"ms1 encodes the WORDS ONLY. The passphrase is a required spending factor, is "+
			"never engraved and cannot be recovered from any plate in the set, so this row "+
			"promises a backup that does not reach the money. buildFullModeLabel(true) "+
			"already returns the correct string; it is simply not called here", modeScreen)
	}

	// (2) THE ARTIFACT THAT OUTLIVES THE OPERATOR. The restore doc is read years
	// later, alone, often by someone who was not the operator, and it is the only
	// place a reader can learn that the steel in their hands is not everything.
	if !uiContains(restoreDoc, "BIP-39 passphrase WAS used") {
		t.Errorf("the supply path's restore document never mentions the passphrase:\n%q\n"+
			"A reader holding this pile of steel has no way to learn a third spending "+
			"factor was ever in play. That is F-132's shape: a backup that is both wrong "+
			"and trusted", restoreDoc)
	}
	// And the SET INVENTORY, which the same edit carries (review M-13): the supply
	// path passed nil and printed no plate count at all, which F-188 made bite by
	// letting this path cut several plates.
	if !uiContains(restoreDoc, "This backup is") {
		t.Errorf("the supply path's restore document carries no plate inventory:\n%q\n"+
			"The one fact that tells a reader whether they hold ALL of it is how many "+
			"plates there are", restoreDoc)
	}
}

// TestSupplyRestoreDocSaysSoWhenNoPassphraseWasUsed is the non-vacuity arm.
//
// Without it, "always print the passphrase warning" satisfies the test above --
// and a picker that cries DEFAULT when the operator chose is a picker whose
// warnings get ignored (SPEC 0.1's corollary). The bare run must read exactly as
// it always did, and must ANSWER the reader's question rather than going silent:
// silence leaves them unable to tell a complete backup from one whose operator
// forgot to write the passphrase down.
func TestSupplyRestoreDocSaysSoWhenNoPassphraseWasUsed(t *testing.T) {
	md1 := s5SuppliedTraceBMd1(t)
	d := s5DriveSupply(t, md1, fixtureMasterA, true /* full */)
	if !uiContains(d.engrave, "Card 1 of") {
		t.Fatalf("the bare supply run did not reach the engrave picker: %q", d.engrave)
	}
	// The mode picker is behind s5DriveSupply, so assert on the label function's
	// bare arm plus the census the drive DID capture: a bare run must not be
	// labelled as though a factor were missing.
	if strings.Contains(buildFullModeLabel(false), "NOT passphrase") {
		t.Errorf("a build with no passphrase is labelled as though one were missing: %q",
			buildFullModeLabel(false))
	}
	if !strings.Contains(strings.Join(buildPassphraseInventoryLines(false), " "),
		"No BIP-39 passphrase was used") {
		t.Error("the bare arm of the inventory does not answer the reader's question, " +
			"so a complete backup is indistinguishable from one missing a factor")
	}
}
