package gui

import (
	"bytes"
	"errors"
	"reflect"
	"strings"
	"testing"
	"testing/synctest"

	"seedhammer.com/bip39"
)

// Bug-hunt triage, SeedHammer II fork, slice S1 (seed entry, passphrase
// cancel/Back flows, scan). Every test asserts the CORRECT behaviour, so it
// fails on unmodified code when the reported bug is real. Test numbers are the
// hunt report's (bughunt-2026-10-31-sh2.md).

// bhS1Frames pumps up to n frames and returns the first frame whose content
// contains one of the needles (and which needle), or the last frame seen.
func bhS1Frames(frame func() (string, bool), n int, needles ...string) (content, hit string) {
	for range n {
		c, ok := frame()
		if !ok {
			return content, ""
		}
		content = c
		for _, nd := range needles {
			if uiContains(c, nd) {
				return c, nd
			}
		}
	}
	return content, ""
}

// ---------------------------------------------------------------- #2

// TestBughuntSh202SingleSigPassphraseCancel: choose "Add passphrase" in
// singleSigInputs, then press Back on the passphrase keyboard. Correct: the
// step machine goes back to the passphrase question. Reported wrong behaviour:
// it continues to the engrave-mode choice with passphrase "".
func TestBughuntSh202SingleSigPassphraseCancel(t *testing.T) {
	skipUnderRefugium(t, refugiumSkipPassphrase)
	synctest.Test(t, func(t *testing.T) {
		ctx := NewContext(newPlatform())
		frame, quit := runUI(ctx, func() {
			singleSigInputs(ctx, &descriptorTheme)
		})
		defer quit()
		frame()
		click(&ctx.Router, Button3) // 12 WORDS
		frame()
		driveWords(&ctx.Router, abandonAboutPhrase())
		if c, ok := pumpUntil(frame, "Wallet Type", 160); !ok {
			t.Fatalf("did not reach the wallet-type picker; got %q", c)
		}
		click(&ctx.Router, Button3) // BIP-84 default
		if c, ok := pumpUntil(frame, ppQuestion, 64); !ok {
			t.Fatalf("did not reach the passphrase question; got %q", c)
		}
		click(&ctx.Router, Down) // "Add passphrase"
		frame()
		click(&ctx.Router, Button3)
		if c, ok := pumpUntil(frame, "Enter Passphrase", 64); !ok {
			t.Fatalf("choosing Add passphrase did not reach the keyboard; got %q", c)
		}
		click(&ctx.Router, Button1) // Back on the keyboard
		c, hit := bhS1Frames(frame, 64, "What to engrave?", ppQuestion)
		if hit == "What to engrave?" {
			t.Fatalf("Back on the passphrase keyboard continued to the engrave-mode choice with passphrase \"\"; frame %q", c)
		}
		if hit != ppQuestion {
			t.Fatalf("Back on the passphrase keyboard did not return to the passphrase question; last frame %q", c)
		}
	})
}

// ---------------------------------------------------------------- #3

// TestBughuntSh203Bip85PassphraseCancel: same input in bip85DeriveFlow. Correct:
// stay on the passphrase step. Reported wrong behaviour: the child-parameter
// picker appears, and the child will derive from the master with no passphrase.
func TestBughuntSh203Bip85PassphraseCancel(t *testing.T) {
	skipUnderRefugium(t, refugiumSkipPassphrase)
	synctest.Test(t, func(t *testing.T) {
		ctx := NewContext(newPlatform())
		frame, quit := runUI(ctx, func() {
			bip85DeriveFlow(ctx, &descriptorTheme)
		})
		defer quit()
		frame()
		click(&ctx.Router, Button3) // 12 WORDS
		frame()
		driveWords(&ctx.Router, abandonAboutPhrase())
		if c, ok := pumpUntil(frame, ppQuestion, 160); !ok {
			t.Fatalf("did not reach the passphrase question; got %q", c)
		}
		click(&ctx.Router, Down) // "Add passphrase"
		frame()
		click(&ctx.Router, Button3)
		if c, ok := pumpUntil(frame, "Enter Passphrase", 64); !ok {
			t.Fatalf("choosing Add passphrase did not reach the keyboard; got %q", c)
		}
		click(&ctx.Router, Button1) // Back on the keyboard
		c, hit := bhS1Frames(frame, 64, "Child word count", ppQuestion)
		if hit == "Child word count" {
			t.Fatalf("Back on the passphrase keyboard continued to the child-parameter picker with passphrase \"\"; frame %q", c)
		}
		if hit != ppQuestion {
			t.Fatalf("Back on the passphrase keyboard did not return to the passphrase question; last frame %q", c)
		}
	})
}

// ---------------------------------------------------------------- #4

// TestBughuntSh204TypedSeedBackHalfTyped: seedEntryFlowTypedOnlyTitled, choose
// 12 words, type 3, press Back. Reported wrong behaviour: the function returns
// the half-typed mnemonic (words 4-12 = -1) as confirmed (ok == true).
//
// OPEN PRODUCT QUESTION: after Back with words typed, should the flow re-show
// the word-count picker (what Back with nothing typed does today) or leave seed
// entry? Either is accepted here; only "returned as confirmed" is rejected.
func TestBughuntSh204TypedSeedBackHalfTyped(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := NewContext(newPlatform())
		var (
			m        bip39.Mnemonic
			ok       bool
			returned bool
		)
		frame, quit := runUI(ctx, func() {
			m, ok = seedEntryFlowTypedOnlyTitled(ctx, &descriptorTheme, seedEntryTitle, "")
			returned = true
		})
		defer quit()
		frame()
		click(&ctx.Router, Button3) // 12 WORDS
		frame()
		driveWords(&ctx.Router, "abandon abandon abandon")
		if c, found := pumpUntil(frame, "Word 4 of 12", 96); !found {
			t.Fatalf("typing 3 words did not reach word 4; got %q", c)
		}
		click(&ctx.Router, Button1) // Back, with 3 words typed
		for range 16 {
			if _, more := frame(); !more {
				break
			}
		}
		if returned && ok {
			filled := 0
			for _, w := range m {
				if w != -1 {
					filled++
				}
			}
			t.Fatalf("Back after typing 3 words returned a %d-slot mnemonic with %d words filled as CONFIRMED (ok=true)",
				len(m), filled)
		}
	})
}

// ---------------------------------------------------------------- #9

// TestBughuntSh209ScannerOverflowSticky: one oversized tag fills the 8 KiB scan
// buffer; once that tag has been read to its end, the next (small, valid) tag
// must scan normally. Reported wrong behaviour: the overflow flag is never
// cleared, so every later Scan on the same scanner returns errScanOverflow.
func TestBughuntSh209ScannerOverflowSticky(t *testing.T) {
	sc := new(scanner)
	big := bytes.NewReader(bytes.Repeat([]byte("A"), 9000))
	first, err := sc.Scan(big)
	if first != nil || !errors.Is(err, errScanOverflow) {
		t.Fatalf("precondition: the first read of a 9000-byte tag gave (%v, %v), want errScanOverflow", first, err)
	}
	// Read the oversized tag to its end: drain until the reader is empty, then
	// one more poll, which observes the end of the tag (io.EOF).
	for big.Len() > 0 {
		sc.Scan(big)
	}
	sc.Scan(big)

	want, perr := bip39.ParseMnemonic(abandonAboutPhrase())
	if perr != nil {
		t.Fatalf("ParseMnemonic: %v", perr)
	}
	rd := strings.NewReader(abandonAboutPhrase())
	for range 8 {
		obj, err := sc.Scan(rd)
		if errors.Is(err, errScanInProgress) {
			continue
		}
		if errors.Is(err, errScanOverflow) {
			t.Fatalf("after the oversized tag ended, a valid %d-byte tag still scans as %v", len(abandonAboutPhrase()), err)
		}
		if err != nil {
			t.Fatalf("scan of the valid tag failed: %v", err)
		}
		if !reflect.DeepEqual(obj, want) {
			t.Fatalf("scan of the valid tag decoded %#v, want %#v", obj, want)
		}
		return
	}
	t.Fatal("scan of the valid tag never completed")
}

// ---------------------------------------------------------------- #31

// bhS1SupplyToPassphraseQuestion drives supplyMultisigPolicyFlow (Trace B's
// policy as the SUPPLIED md1: master A fills @0 and @1 with NO passphrase) to
// the BIP-39 passphrase question.
func bhS1SupplyToPassphraseQuestion(t *testing.T) (*Context, func() (string, bool), func(), *bool) {
	t.Helper()
	md1 := s5SuppliedTraceBMd1(t)
	p := newPlatform()
	p.display = sh2DisplaySize
	ctx := NewContext(p)
	ctx.syswBundleSeeds = append([]string(nil), md1...)
	done := new(bool)
	frame, quit := runUI(ctx, func() {
		supplyMultisigPolicyFlow(ctx, &descriptorTheme)
		*done = true
	})
	if c, ok := pumpUntil(frame, "md1 descriptors: 1", 64); !ok {
		quit()
		t.Fatalf("the supplied md1 never reached the gatherer's tally; got %q", c)
	}
	click(&ctx.Router, Button3) // Done adding cards
	frame()
	if c, ok := pumpUntil(frame, "Choose number of words", 64); !ok {
		quit()
		t.Fatalf("the gather did not hand off to seed entry; got %q", c)
	}
	click(&ctx.Router, Button3) // 12 WORDS
	frame()
	driveWords(&ctx.Router, fixtureMasterA)
	if c, ok := pumpUntil(frame, ppQuestion, 240); !ok {
		quit()
		t.Fatalf("the passphrase question was not reached; got %q", c)
	}
	return ctx, frame, quit, done
}

// bhS1SupplyForwardMarkers are screens that exist only AFTER the supply flow has
// cross-matched the seed with whatever passphrase it settled on.
var bhS1SupplyForwardMarkers = []string{"Your seed is at slots", "What to engrave?", "not a cosigner"}

// TestBughuntSh231aSupplyMultisigPassphraseCancel: supplyMultisigPolicyFlow,
// "Add passphrase", then Back on the keyboard. Reported wrong behaviour: the
// flow cross-matches with passphrase "" (here master A with no passphrase fills
// @0 and @1, so it goes on to the multi-slot notice and the engrave mode).
//
// OPEN PRODUCT QUESTION: abort the flow (the report's fix) or re-ask the
// question? Either is accepted; only "carries on with no passphrase" is rejected.
func TestBughuntSh231aSupplyMultisigPassphraseCancel(t *testing.T) {
	skipUnderRefugium(t, refugiumSkipPassphrase)
	synctest.Test(t, func(t *testing.T) {
		ctx, frame, quit, _ := bhS1SupplyToPassphraseQuestion(t)
		defer quit()
		click(&ctx.Router, Down) // "Add passphrase"
		frame()
		click(&ctx.Router, Button3)
		if c, ok := pumpUntil(frame, "Enter Passphrase", 64); !ok {
			t.Fatalf("choosing Add passphrase did not reach the keyboard; got %q", c)
		}
		click(&ctx.Router, Button1) // Back on the keyboard
		c, hit := bhS1Frames(frame, 96, bhS1SupplyForwardMarkers...)
		if hit != "" {
			t.Fatalf("Back on the passphrase keyboard carried on with passphrase \"\" to %q; frame %q", hit, c)
		}
	})
}

// TestBughuntSh231bSupplyMultisigQuestionBack: the same flow, Back on the
// "Add a BIP-39 passphrase?" question itself. Reported wrong behaviour: the flow
// carries on with passphrase "" exactly as if Skip had been chosen.
//
// OPEN PRODUCT QUESTION: passphrase_off.go:15-27 documents "ok false" at this
// site as possibly meaning "no passphrase"; the report (and the 2026-08-19
// "Back means back" directive quoted in multisig_verify.go) says it must not.
func TestBughuntSh231bSupplyMultisigQuestionBack(t *testing.T) {
	skipUnderRefugium(t, refugiumSkipPassphrase)
	synctest.Test(t, func(t *testing.T) {
		ctx, frame, quit, _ := bhS1SupplyToPassphraseQuestion(t)
		defer quit()
		click(&ctx.Router, Button1) // Back on the question
		c, hit := bhS1Frames(frame, 96, bhS1SupplyForwardMarkers...)
		if hit != "" {
			t.Fatalf("Back on the passphrase question carried on with passphrase \"\" to %q; frame %q", hit, c)
		}
	})
}

// ---------------------------------------------------------------- #32

// TestBughuntSh232Slip39PassphraseQuestionBack: recoverSLIP39Flow, enter the
// second share, then press Back on "SLIP-39 passphrase?". Correct (the
// docstring): (nil, false). Reported wrong behaviour: Back is read as Skip and
// the shares are combined with the EMPTY passphrase, giving a different, valid
// seed.
func TestBughuntSh232Slip39PassphraseQuestionBack(t *testing.T) {
	skipUnderRefugium(t, refugiumSkipSLIP39)
	synctest.Test(t, func(t *testing.T) {
		first := parseFixtureShare(t, slip39Vec3[0])
		ctx := NewContext(newPlatform())
		driveShare(&ctx.Router, slip39Vec3[1])
		var (
			m        bip39.Mnemonic
			ok       bool
			returned bool
		)
		frame, quit := runUI(ctx, func() {
			m, ok = recoverSLIP39Flow(ctx, &descriptorTheme, first)
			returned = true
		})
		defer quit()
		if c, found := pumpUntil(frame, "SLIP-39 passphrase?", 400); !found {
			t.Fatalf("did not reach the SLIP-39 passphrase question; got %q", c)
		}
		click(&ctx.Router, Button1) // Back on the question
		for range 400 {
			if _, more := frame(); !more {
				break
			}
		}
		if !returned {
			t.Fatal("the flow did not return after Back on the SLIP-39 passphrase question")
		}
		if ok || m != nil {
			t.Fatalf("Back on the SLIP-39 passphrase question recovered a seed with the empty passphrase: ok=%v entropy=%s",
				ok, hexOfEntropy(m))
		}
	})
}

// ---------------------------------------------------------------- #33

// TestBughuntSh233MultisigVerifyPassphraseCancel: multisigVerifyFlow, first
// seed typed, "Add passphrase", then Back on the keyboard. Correct: the leg
// stays on the passphrase step (the comment says Back steps back). Reported
// wrong behaviour: the leg derives and compares with passphrase "" and reports
// a verdict.
func TestBughuntSh233MultisigVerifyPassphraseCancel(t *testing.T) {
	skipUnderRefugium(t, refugiumSkipVerify)
	md1, plates, _ := s5TraceBEngraved(t, false)
	records := append([]string(nil), md1...)
	for _, pl := range plates {
		records = append(records, pl...)
	}
	synctest.Test(t, func(t *testing.T) {
		ctx, frame, quit := s5DriveVerifyToPassphrase(t, records, []int{0, 1, 2}, md1, fixtureMasterA)
		defer quit()
		click(&ctx.Router, Down) // "Add passphrase"
		frame()
		click(&ctx.Router, Button3)
		if c, ok := pumpUntil(frame, "Enter Passphrase", 64); !ok {
			t.Fatalf("choosing Add passphrase did not reach the keyboard; got %q", c)
		}
		click(&ctx.Router, Button1) // Back on the keyboard
		c, hit := bhS1Frames(frame, 160, "Verify OK", "Verify Failed", "Verify Incomplete",
			"not checked yet", "Verify Bundle", ppQuestion)
		if hit == "" {
			t.Fatalf("Back on the passphrase keyboard reached neither the passphrase question nor a verdict; last frame %q", c)
		}
		if hit != ppQuestion {
			t.Fatalf("Back on the passphrase keyboard went FORWARD to %q with passphrase \"\"; frame %q", hit, c)
		}
	})
}

// ---------------------------------------------------------------- #42

// TestBughuntSh242BuildWhichMd1Back: buildMultisigPolicyFlow, Back on the
// "Which md1?" (Full/Template) screen. Correct: abort, like every other choice
// screen in the flow. Reported wrong behaviour: Back is taken as "Full policy"
// and the flow continues to the EXPERIMENTAL warning.
func TestBughuntSh242BuildWhichMd1Back(t *testing.T) {
	skipUnderRefugium(t, refugiumSkipPassphrase) // the walk answers the passphrase question
	records := cosignerCardRecords(t, 4)         // A@0, B@0, C@0, A@1
	synctest.Test(t, func(t *testing.T) {
		p := newPlatform()
		p.display = sh2DisplaySize
		ctx := NewContext(p)
		ctx.sysw = sessionHolding(records...)
		done := false
		frame, quit := runUI(ctx, func() {
			buildMultisigPolicyFlow(ctx, &descriptorTheme)
			done = true
		})
		defer quit()
		// Trace A with the self seed TYPED (as s5DriveBuildToEngravePicker),
		// stopped at the form choice.
		for _, s := range []buildWalkStep{
			{needle: "Choose policy type", downs: 0},            // wsh
			{needle: "How many keys (n)?", downs: 1},            // n = 3
			{needle: "Required signatures (k of 3)?", downs: 1}, // k = 2
			{needle: "Which slot is your key?", downs: 0},       // self @0
			{needle: "Do you hold another slot?", downs: 0},     // NO
			{needle: "Include key fingerprints?", downs: 0},     // omit
			{needle: "key on a card?", downs: 0},                // NO, JUST MY SEED
			{needle: buildCosignerGatherTitle, downs: 0},        // Done adding cards
			{needle: "Payload cards", downs: 0},                 // the over-supply review
			{needle: "Use payload card 1 of 4?", downs: 1},      // SKIP A@0
			{needle: "Use payload card 2 of 4?", downs: 0},      // USE B@0
			{needle: "Use payload card 3 of 4?", downs: 0},      // USE C@0
			{needle: "Choose number of words", downs: 0},        // 12 WORDS
		} {
			c, ok := pumpUntil(frame, s.needle, 96)
			if !ok {
				t.Fatalf("the build never reached %q; screen reads %q", s.needle, c)
			}
			for range s.downs {
				click(&ctx.Router, Down)
				frame()
			}
			click(&ctx.Router, Button3)
			frame()
		}
		typeWords(&ctx.Router, frame, fixtureMasterA)
		for _, s := range []buildWalkStep{
			{needle: ppQuestion, downs: 0},    // Skip
			{needle: "Key sources", downs: 0}, // the slot-source review
			{needle: "Policy stub", downs: 0}, // Policy Review -> continue
		} {
			c, ok := pumpUntil(frame, s.needle, 96)
			if !ok {
				t.Fatalf("the build never reached %q; screen reads %q", s.needle, c)
			}
			click(&ctx.Router, Button3)
			frame()
		}
		if c, ok := pumpUntil(frame, "Which md1?", 96); !ok {
			t.Fatalf("the build never reached \"Which md1?\"; screen reads %q", c)
		}
		click(&ctx.Router, Button1) // Back on "Which md1?"
		c, hit := bhS1Frames(frame, 96, "EXPERIMENTAL")
		if hit != "" {
			t.Fatalf("Back on \"Which md1?\" was taken as \"Full policy\" and the flow went on to the %s warning; frame %q", hit, c)
		}
		if !done {
			t.Fatal("Back on \"Which md1?\" neither aborted the flow nor reached the next screen")
		}
	})
}

// ---------------------------------------------------------------- #48

// TestBughuntSh248ComposerSeatResumeBack: composerSeatFlow re-entered with @0
// and @1 already seated (it resumes at @2). Back on @2, the first slot it asks,
// must return false and keep @1's seat ("Back keeps assignments"; the doc says
// it returns false on Back from the FIRST slot it asks). Reported wrong
// behaviour: it releases @1's seat and asks for @1 again.
func TestBughuntSh248ComposerSeatResumeBack(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		st := &composerState{list: composerTwoPathList()} // 4 slots
		for range 4 {
			st.sources = append(st.sources, composerSource{kind: composerSourceKey, label: "key", seedID: -1})
		}
		st.sources[0].used = true
		st.sources[1].used = true
		st.assigned = []composerAssignment{{src: 0}, {src: 1}, {src: -1}, {src: -1}}

		ctx := NewContext(newPlatform())
		var (
			got      bool
			returned bool
		)
		frame, quit := runUI(ctx, func() {
			got = composerSeatFlow(ctx, &descriptorTheme, st)
			returned = true
		})
		defer quit()
		if c, ok := pumpUntil(frame, "Slot @2", 24); !ok {
			t.Fatalf("the seating flow did not resume at @2; got %q", c)
		}
		click(&ctx.Router, Button1) // Back on @2, the first slot asked
		for range 8 {
			if _, more := frame(); !more {
				break
			}
		}
		if st.assigned[1].src < 0 || !st.sources[1].used {
			t.Fatalf("Back on the first slot asked (@2) released @1's seat: assigned[1].src=%d sources[1].used=%v",
				st.assigned[1].src, st.sources[1].used)
		}
		if !returned || got {
			t.Fatalf("Back on the first slot asked did not leave seating with false: returned=%v result=%v", returned, got)
		}
	})
}
