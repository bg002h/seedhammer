package gui

import (
	"strings"
	"testing"
	"testing/synctest"
	"time"
)

// ─── B4 / B5: the post-engrave TAIL, driven through the real screens ─────────
//
// Both engraving programs end the same way: an abort gate, a verify offer that
// re-offers itself, and a restore document. Round 1 measured that neither of the
// two mechanisms this fold added there is watched by anything that EXECUTES.
//
//   - I-4's retry loop was pinned by four strings.Contains calls over the
//     callers' own source (TestBothEngraveFlowsReOfferTheVerify). Two independent
//     mutations left the whole tree green: `for {` reduced to one iteration, and
//     the {"VERIFY AGAIN", "CONTINUE"} labels swapped -- the loop keys on the
//     returned INDEX and no label lookup exists anywhere, so the swap makes
//     VERIFY AGAIN exit the verify and CONTINUE re-run it. Coverage put both loop
//     bodies at execution count 0.
//   - I-12's abort gate is pinned behaviourally on the SUPPLY path only
//     (TestSupplyAbortIsTheLastScreenOfTheProgram). On the BUILD path, replacing
//     the guard's body with a no-op left the suite green -- and the build path is
//     the flow that cuts the most steel.
//
// WHY THE WALKS ARE THIS EXPENSIVE, said plainly rather than apologised for.
// Both mechanisms sit AFTER a completed engrave, so a cheaper harness cannot
// reach them: the offer screen does not exist until every plate is cut, and the
// abort gate does not exist until the engrave has started. That is exactly why
// they were unwatched, and a test that stopped short of the engrave would have
// the same blind spot the greps have.

// s5RowIndexOf reports which ChoiceScreen row carries `want`, by where the LABEL
// is drawn rather than by assuming a position.
//
// It is the whole point of the retry test. The production loop keys on the index
// Choose returns and never looks a label up, so a test that pressed row 0 by
// number would pass with the labels swapped -- which is precisely the mutation
// that ran green. Pressing "the row that SAYS VERIFY AGAIN" is the operator's
// action; pressing row 0 is the implementation's.
func s5RowIndexOf(t *testing.T, content string, labels []string, want string) int {
	t.Helper()
	norm := func(s string) string {
		return strings.ToLower(strings.ReplaceAll(s, " ", ""))
	}
	drawn := norm(content)
	at := make([]int, len(labels))
	for i, l := range labels {
		at[i] = strings.Index(drawn, norm(l))
		if at[i] < 0 {
			t.Fatalf("the choice screen does not draw the row %q, so the operator cannot "+
				"press it. Labels sought: %v. Frame:\n%q", l, labels, content)
		}
	}
	// The row order is the DRAW order, and the draw order is the index order.
	idx := 0
	for i := range labels {
		if labels[i] == want {
			idx = i
		}
	}
	rank := 0
	for i := range labels {
		if i != idx && at[i] < at[idx] {
			rank++
		}
	}
	return rank
}

// s5ChooseRowLabelled presses the row carrying `want` on the ChoiceScreen that
// is currently drawn.
func s5ChooseRowLabelled(t *testing.T, ctx *Context, frame func() (string, bool),
	content string, labels []string, want string,
) {
	t.Helper()
	for i := 0; i < s5RowIndexOf(t, content, labels, want); i++ {
		click(&ctx.Router, Down)
		frame()
	}
	click(&ctx.Router, Button3)
	frame()
}

// s5StubVerifyFn substitutes the verify at the multisigVerifyFn seam and returns
// a pointer to the call count.
//
// The stub replaces the VERDICT SOURCE and nothing else: the offer screens, the
// row mapping, the loop condition and the retry lead are all production code
// under the test, which is where both of round 1's mutations live. Substituting
// the flow itself is what makes the loop reachable at all -- a real second
// attempt would mean driving a whole readback (gather, seed entry, passphrase,
// ms1) twice, inside a walk that has already cut nine plates.
//
// `verdicts` is consumed one per call; the last one repeats.
func s5StubVerifyFn(t *testing.T, verdicts ...multisigVerifyResult) *int {
	t.Helper()
	if len(verdicts) == 0 {
		t.Fatal("s5StubVerifyFn needs at least one verdict")
	}
	calls := 0
	prev := multisigVerifyFn
	multisigVerifyFn = func(ctx *Context, th *Colors, full bool, expectedSlots []int,
		engravedMd1 []string, rec *verifyRecord,
	) multisigVerifyResult {
		calls++
		if calls <= len(verdicts) {
			return verdicts[calls-1]
		}
		return verdicts[len(verdicts)-1]
	}
	t.Cleanup(func() { multisigVerifyFn = prev })
	return &calls
}

// s5PumpUntilCalls pumps frames until the seam has been entered `want` times.
func s5PumpUntilCalls(frame func() (string, bool), calls *int, want, maxFrames int) (string, bool) {
	var content string
	for i := 0; i < maxFrames; i++ {
		if *calls >= want {
			return content, true
		}
		c, ok := frame()
		if !ok {
			break
		}
		content = c
	}
	return content, *calls >= want
}

// s5DriveSupplyToEngravePicker drives supplyMultisigPolicyFlow from its first
// screen to the FIRST plate's engrave-style picker, in FULL mode.
func s5DriveSupplyToEngravePicker(t *testing.T, ctx *Context, frame func() (string, bool)) {
	t.Helper()
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
	if c, ok := pumpUntil(frame, "passphrase", 240); !ok {
		t.Fatalf("the passphrase prompt was not reached; got %q", c)
	}
	click(&ctx.Router, Button3) // Skip
	frame()
	// The multi-slot notice, if this payload draws one, then the engrave mode.
	if _, ok := pumpUntil(frame, "What to engrave?", 32); !ok {
		click(&ctx.Router, Button3) // dismiss the multi-slot notice
		frame()
		if c, ok := pumpUntil(frame, "What to engrave?", 64); !ok {
			t.Fatalf("the engrave-mode choice was not reached; got %q", c)
		}
	}
	click(&ctx.Router, Button3) // Full
	frame()
	if c, ok := pumpUntil(frame, "Plates To Cut", 96); !ok {
		t.Fatalf("the plate census was not shown; got %q", c)
	}
	click(&ctx.Router, Button3)
	frame()
	if c, ok := pumpUntil(frame, "Choose engraving", 96); !ok {
		t.Fatalf("the engrave-style picker was not reached; got %q", c)
	}
}

// s5DriveBuildToEngravePicker drives buildMultisigPolicyFlow from the template
// picker to the FIRST plate's engrave-style picker, in FULL mode.
//
// It is Trace A with the self seed TYPED, the same route
// TestBuildWalkTypedSeed walks, with that walk's raster-floor assertions left
// out: the floors are its subject, and the tail is this file's.
func s5DriveBuildToEngravePicker(t *testing.T, ctx *Context, frame func() (string, bool)) {
	t.Helper()
	for _, s := range []buildWalkStep{
		{needle: "Choose policy type", downs: 0},            // wsh
		{needle: "How many keys (n)?", downs: 1},            // n = 3
		{needle: "Required signatures (k of 3)?", downs: 1}, // k = 2
		{needle: "Which slot is your key?", downs: 0},       // self @0
		{needle: "Do you hold another slot?", downs: 0},     // NO, THAT IS ALL
		{needle: "Include key fingerprints?", downs: 0},     // omit
		{needle: "key on a card?", downs: 0},                // NO, JUST MY SEED
		{needle: buildCosignerGatherTitle, downs: 0},        // Done adding cards
		{needle: "Payload cards", downs: 0},                 // the over-supply review
		{needle: "Use payload card 1 of 4?", downs: 1},      // SKIP A@0 (duplicates self)
		{needle: "Use payload card 2 of 4?", downs: 0},      // USE B@0
		{needle: "Use payload card 3 of 4?", downs: 0},      // USE C@0
		{needle: "Choose number of words", downs: 0},        // 12 WORDS
	} {
		c, ok := pumpUntil(frame, s.needle, 96)
		if !ok {
			t.Fatalf("the build never reached %q; screen reads %q", s.needle, c)
		}
		for i := 0; i < s.downs; i++ {
			click(&ctx.Router, Down)
			frame()
		}
		click(&ctx.Router, Button3)
		frame()
	}
	typeWords(&ctx.Router, frame, fixtureMasterA)
	for _, s := range []buildWalkStep{
		{needle: "Add a BIP-39 passphrase?", downs: 0}, // Skip
		{needle: "Key sources", downs: 0},              // the slot-source review
		{needle: "Policy stub", downs: 0},              // Policy Review -> continue
		{needle: "Which md1?", downs: 0},               // Full policy md1
	} {
		c, ok := pumpUntil(frame, s.needle, 96)
		if !ok {
			t.Fatalf("the build never reached %q; screen reads %q", s.needle, c)
		}
		click(&ctx.Router, Button3)
		frame()
	}
	// The EXPERIMENTAL warning: hold to confirm is the only route past it.
	if c, ok := pumpUntil(frame, "EXPERIMENTAL", 96); !ok {
		t.Fatalf("the mandatory EXPERIMENTAL warning was not reached; got %q", c)
	}
	press(&ctx.Router, Button3)
	frame()
	time.Sleep(confirmDelay)
	frame()
	if c, ok := pumpUntil(frame, "What to engrave?", 96); !ok {
		t.Fatalf("the engrave-mode choice was not reached; got %q", c)
	}
	click(&ctx.Router, Button3) // Full (seed + keys)
	frame()
	if c, ok := pumpUntil(frame, "Plate Count", 96); !ok {
		t.Fatalf("the plate census was not shown; got %q", c)
	}
	click(&ctx.Router, Button3)
	frame()
	if c, ok := pumpUntil(frame, "Choose engraving", 96); !ok {
		t.Fatalf("the engrave-style picker was not reached; got %q", c)
	}
}

// s5EngraveEveryPlate cuts every remaining plate from the engrave-style picker.
//
// IT REQUIRES THE RASTER HARNESS, and the reason is measured rather than
// stylistic. engraveOnePlate gives one plate a 4096-frame budget, and the frames
// a plate takes depend on which harness pumps them: under runUITouchRaster the
// engraver closed at frame 881, under plain runUI at frame 10585 -- the same
// plate, twelve times the frames, because virtual time in the synctest bubble
// advances per idle point rather than per frame. The first draft of this file
// used runUI and died on "the engrave never closed the engraver, so no plate was
// cut" with the engrave running perfectly. Named here because the failure looks
// exactly like a broken flow and is not one.
func s5EngraveEveryPlate(t *testing.T, ctx *Context, frame func() (string, bool), e *testEngraver) int {
	t.Helper()
	plates := 0
	for {
		if _, ok := pumpUntil(frame, "Choose engraving", 96); !ok {
			break
		}
		click(&ctx.Router, Button3) // first variant
		frame()
		engraveOnePlate(t, ctx, frame, e)
		plates++
		if plates > 24 {
			t.Fatal("the engrave loop did not terminate")
		}
	}
	if plates == 0 {
		t.Fatal("no plate was cut, so this walk never reached the post-engrave surfaces")
	}
	return plates
}

// s5AssertRetryLoop is B4's assertion, and it is written against the OPERATOR'S
// actions rather than against row numbers.
//
// The verify offer is on the display. The seam returns verifyIncomplete every
// time, so:
//
//	offer 1  "Verify the engraved plates?"      press the row saying "Verify now"
//	offer 2  multisigVerifyRetryLead            press the row saying "VERIFY AGAIN"
//	offer 3  multisigVerifyRetryLead            press the row saying "CONTINUE"
//
// and the verify must have been entered EXACTLY TWICE. That single count kills
// both of round 1's mutations. A one-iteration loop never draws offer 2, so the
// count stops at 1. Swapped labels put "VERIFY AGAIN" at index 1, so pressing it
// is `sel != 0` and breaks the loop -- the count also stops at 1, and CONTINUE
// would have been the button that retried.
func s5AssertRetryLoop(t *testing.T, ctx *Context, frame func() (string, bool),
	calls *int, done *bool, offer string,
) {
	t.Helper()
	if *calls != 0 {
		t.Fatalf("the verify was entered %d time(s) before the offer was answered", *calls)
	}
	s5ChooseRowLabelled(t, ctx, frame, offer, []string{"Verify now", "Skip"}, "Verify now")

	retry, ok := pumpUntil(frame, multisigVerifyRetryLead, 128)
	if !ok {
		t.Fatalf("after an INCOMPLETE verify the offer was not made again. The screen "+
			"reads %q, and the incomplete report the operator just dismissed told them "+
			"to choose VERIFY AGAIN on the next screen.\nThis is the one-shot offer I-4 "+
			"replaced: the program table carries no standalone bundle verify, so the "+
			"operator's only remaining route is to re-cut every plate", retry)
	}
	if *calls != 1 {
		t.Fatalf("the verify was entered %d time(s) before the retry offer, want 1", *calls)
	}
	for _, want := range []string{"VERIFY AGAIN", "CONTINUE"} {
		if !uiContains(retry, want) {
			t.Fatalf("the retry offer does not draw a %q row: %q", want, retry)
		}
	}

	// THE ROW THAT SAYS VERIFY AGAIN, whichever row that is.
	s5ChooseRowLabelled(t, ctx, frame, retry, []string{"VERIFY AGAIN", "CONTINUE"}, "VERIFY AGAIN")
	if c, ok := s5PumpUntilCalls(frame, calls, 2, 256); !ok {
		t.Fatalf("pressing the row LABELLED \"VERIFY AGAIN\" did not run the verify a "+
			"second time (entered %d time(s)); the screen reads %q.\nThe loop keys on the "+
			"index ChoiceScreen.Choose returns and looks up no label anywhere, so if the "+
			"two rows are ordered the other way round then VERIFY AGAIN leaves and "+
			"CONTINUE retries, and the operator who wants another attempt is the one who "+
			"gets the restore document", *calls, c)
	}

	// AND CONTINUE LEAVES. Without this the loop could re-offer forever and the
	// count above would still be satisfied.
	third, ok := pumpUntil(frame, multisigVerifyRetryLead, 128)
	if !ok {
		t.Fatalf("the offer was not made a third time after the second incomplete "+
			"verify; the screen reads %q", third)
	}
	s5ChooseRowLabelled(t, ctx, frame, third, []string{"VERIFY AGAIN", "CONTINUE"}, "CONTINUE")

	tail, ok := pumpUntil(frame, "Descriptor:", 256)
	if !ok {
		t.Fatalf("pressing the row LABELLED \"CONTINUE\" did not leave the verify offer "+
			"for the restore document; the screen reads %q", tail)
	}
	if *calls != 2 {
		t.Errorf("the verify was entered %d time(s), want exactly 2. CONTINUE must LEAVE "+
			"the offer: a third entry means the row labels and the loop's index disagree "+
			"in the other direction", *calls)
	}
	click(&ctx.Router, Button3) // done with the restore doc
	for i := 0; i < 256 && !*done; i++ {
		if _, ok := frame(); !ok {
			break
		}
	}
	if !*done {
		t.Errorf("the program did not end after the restore document")
	}
}

// TestBothEngraveFlowsDriveTheRetryLoop is B4, on BOTH call sites.
//
// Both are driven because both were mutated and both went green: the supply
// path's loop and the build path's loop are separate code, and pinning one says
// nothing about the other -- which is exactly the lesson B5 records about I-12's
// abort gate, one mechanism over.
func TestBothEngraveFlowsDriveTheRetryLoop(t *testing.T) {
	t.Run("supply", func(t *testing.T) {
		md1 := s5SuppliedTraceBMd1(t)
		synctest.Test(t, func(t *testing.T) {
			calls := s5StubVerifyFn(t, verifyIncomplete)
			e := newEngraver()
			p := newPlatform()
			p.display = sh2DisplaySize
			p.engraver = e
			ctx := NewContext(p)
			ctx.syswBundleSeeds = append([]string(nil), md1...)
			done := false
			// THE RASTER HARNESS, for s5EngraveEveryPlate's measured reason.
			frame, _, _, quit := runUITouchRaster(ctx, func() {
				supplyMultisigPolicyFlow(ctx, &descriptorTheme)
				done = true
			})
			defer quit()

			s5DriveSupplyToEngravePicker(t, ctx, frame)
			t.Logf("the supply run cut %d plate(s)", s5EngraveEveryPlate(t, ctx, frame, e))

			offer, ok := pumpUntil(frame, "Verify the engraved plates?", 96)
			if !ok {
				t.Fatalf("the verify offer was not reached after the engrave; got %q", offer)
			}
			s5AssertRetryLoop(t, ctx, frame, calls, &done, offer)
		})
	})

	t.Run("build", func(t *testing.T) {
		records := cosignerCardRecords(t, 4) // A@0, B@0, C@0, A@1
		synctest.Test(t, func(t *testing.T) {
			calls := s5StubVerifyFn(t, verifyIncomplete)
			e := newEngraver()
			p := newPlatform()
			p.display = sh2DisplaySize
			p.engraver = e
			ctx := NewContext(p)
			ctx.sysw = sessionHolding(records...)
			done := false
			// THE RASTER HARNESS, for s5EngraveEveryPlate's measured reason.
			frame, _, _, quit := runUITouchRaster(ctx, func() {
				buildMultisigPolicyFlow(ctx, &descriptorTheme)
				done = true
			})
			defer quit()

			s5DriveBuildToEngravePicker(t, ctx, frame)
			t.Logf("the build run cut %d plate(s)", s5EngraveEveryPlate(t, ctx, frame, e))

			offer, ok := pumpUntil(frame, "Verify the engraved plates?", 96)
			if !ok {
				t.Fatalf("the verify offer was not reached after the engrave; got %q", offer)
			}
			s5AssertRetryLoop(t, ctx, frame, calls, &done, offer)
		})
	})
}

// TestBuildAbortIsTheLastScreenOfTheProgram is B5, and it is I-12's BUILD-path
// twin of TestSupplyAbortIsTheLastScreenOfTheProgram.
//
// The build path's abort `return` is real and correct in production, and nothing
// executed it: replacing its body with a no-op left the whole suite green, while
// the identical mutation on the supply path went red. The fold's own report
// stated the call-site-only pin as a limitation for BOTH paths; measurement said
// it was a limitation for exactly one, and the unwatched one is the flow that
// cuts the most steel.
//
// An operator who runs out of blanks at plate 12 of 17 on the built-policy path
// reads "Bundle Incomplete ... This set is not a usable backup yet", is then
// offered a verify over a set whose md1 was never cut -- the md1 is emitted LAST,
// so the readback dies reading as "your plates are unreadable" -- and is shown a
// restore document headed "This backup is 17 plates ... If any of them is
// missing, this backup is incomplete."
func TestBuildAbortIsTheLastScreenOfTheProgram(t *testing.T) {
	records := cosignerCardRecords(t, 4)
	synctest.Test(t, func(t *testing.T) {
		e := newEngraver()
		p := newPlatform()
		p.display = sh2DisplaySize
		p.engraver = e
		ctx := NewContext(p)
		ctx.sysw = sessionHolding(records...)
		done := false
		frame, quit := runUI(ctx, func() {
			buildMultisigPolicyFlow(ctx, &descriptorTheme)
			done = true
		})
		defer quit()

		s5DriveBuildToEngravePicker(t, ctx, frame)

		// THE ABORT. Back at the FIRST plate's style picker is bundleEngrave's
		// set-level abort: nothing has been cut and the set on the bench is empty.
		click(&ctx.Router, Button1)
		if c, ok := pumpUntil(frame, "Bundle Incomplete", 96); !ok {
			t.Fatalf("Back at the engrave picker did not produce the abort warning; got %q", c)
		}
		click(&ctx.Router, Button3) // dismiss the abort modal

		var after []string
		for i := 0; i < 256 && !done; i++ {
			c, ok := frame()
			if !ok {
				break
			}
			after = append(after, c)
		}
		joined := strings.Join(after, " || ")
		if !done {
			t.Fatalf("the program did not end after the abort; it drew:\n%q", joined)
		}
		for _, banned := range []string{"Verify the engraved plates?", "This backup is", "Descriptor:"} {
			if uiContains(joined, banned) {
				t.Errorf("after \"This set is not a usable backup yet\", the BUILD flow "+
					"still drew %q.\nEverything past the engrave vouches for a COMPLETE "+
					"set: the verify cannot succeed (the md1 is emitted LAST and was never "+
					"cut, so it dies reading as \"your plates are unreadable\"), and the "+
					"restore document is headed \"If any of them is missing, this backup "+
					"is incomplete\".\nDrawn after the abort:\n%q", banned, joined)
			}
		}
	})
}
