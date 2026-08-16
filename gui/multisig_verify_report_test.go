package gui

import (
	"errors"
	"strings"
	"testing"

	"seedhammer.com/md"
)

// ─── C-1 / I-2 / I-3 / I-4 / I-13: the verify's own REPORT ───────────────────
//
// The comparator was already a bijection and already correct. What this file is
// about is everything the flow does with it: WHEN it runs, WHAT the screen says
// the number means, WHETHER the diagnosis reaches the operator, and whether the
// remedy the screen prescribes exists.
//
// TEST MATERIAL, public by construction: masters A, B and C are BIP-39's own
// published vectors. Never put funds behind them.

// s5DriveVerifyStopAfterOneSeed drives multisigVerifyFlow through ONE seed and
// then presses STOP HERE, returning the flow's last frame and its verdict.
//
// It is the INCOMPLETE path's driver, and that path is C-1's site: before this
// fold it `return`ed before the comparator, so verifyMultisigLegs -- the only
// function in the file that touches a read-back plate at all -- was structurally
// unreachable from it.
func s5DriveVerifyStopAfterOneSeed(t *testing.T, records []string, expected []int,
	engravedMd1 []string, phrase string, ms1 string,
) (last string, res multisigVerifyResult) {
	t.Helper()
	p := newPlatform()
	p.display = sh2DisplaySize
	ctx := NewContext(p)
	ctx.syswBundleSeeds = append([]string(nil), records...)

	frame, quit := runUI(ctx, func() {
		res = multisigVerifyFlow(ctx, &descriptorTheme, ms1 != "", expected, engravedMd1)
	})
	defer quit()

	if c, ok := pumpUntil(frame, "mk1 keys:", 64); !ok {
		t.Fatalf("the readback never reached the gatherer's tally; got %q", c)
	}
	click(&ctx.Router, Button3) // Done adding cards
	frame()
	if c, ok := pumpUntil(frame, "Choose number of words", 64); !ok {
		t.Fatalf("the gather did not hand off to seed entry; got %q", c)
	}
	click(&ctx.Router, Button3) // 12 WORDS
	frame()
	driveWords(&ctx.Router, phrase)
	if c, ok := pumpUntil(frame, "passphrase", 240); !ok {
		t.Fatalf("the passphrase prompt was not reached; got %q", c)
	}
	click(&ctx.Router, Button3) // Skip
	frame()

	if ms1 != "" {
		if c, ok := pumpUntil(frame, "Type ms1", 64); !ok {
			t.Fatalf("full mode did not ask for the ms1; got %q", c)
		}
		runes(&ctx.Router, strings.ToLower(ms1))
		click(&ctx.Router, Button3)
		frame()
	}

	// STOP HERE, which is the operator's legitimate "I have not got the other
	// seed with me" choice and the state the incomplete report describes.
	if c, ok := pumpUntil(frame, "not checked yet", 96); !ok {
		t.Fatalf("the flow did not offer the next seed; got %q", c)
	}
	click(&ctx.Router, Down) // STOP HERE
	frame()
	click(&ctx.Router, Button3)
	for i := 0; i < 96; i++ {
		c, ok := frame()
		if !ok {
			break
		}
		last = c
		if uiContains(last, "Verify OK") || uiContains(last, "Verify Failed") ||
			uiContains(last, "Verify Incomplete") {
			break
		}
	}
	return last, res
}

// TestVerifyIncompleteDoesNotCallAFOREIGNPlateChecked is C-1, and it is the
// finding with a plate the wallet needs behind it.
//
// Trace B engraves 3 key plates for slots {0,1,2}. The operator presents the md1
// and three mk1 cards, but the card presented for @0 is a FOREIGN mk1 from a
// different wallet -- an earlier generation's plate, a mis-cut, or here a
// single-sig mk1 at m/44'. They type master A, which covers @0 and @1, then
// choose STOP HERE.
//
// EVERY UPSTREAM CHECK PASSES, and that is the point: all three are cardinality
// or provenance and none of them is plate identity. The readback md1 equals the
// engraved md1, the mk1 COUNT matches, and the typed seed accounts for 2 of the
// 3 expected slots. The flow then reported:
//
//	"Checked 2 of the 3 key plates this run engraved. The rest were NOT verified."
//
// -- over a plate that was never decoded, never xpub-matched and never
// stub-bound. The operator concludes @0 and @1 are proven good and only @2 is
// outstanding. On a 3-of-4, @0's plate belongs to a different wallet and the
// wallet needs it.
func TestVerifyIncompleteDoesNotCallAForeignPlateChecked(t *testing.T) {
	md1, plates, _ := s5TraceBEngraved(t, false)
	if len(plates) != 3 {
		t.Fatalf("Trace B engraved %d plate(s), want 3", len(plates))
	}
	_, keys, err := md.ExpandWalletPolicyChunks(md1)
	if err != nil {
		t.Fatalf("ExpandWalletPolicyChunks: %v", err)
	}

	// The plate this run cut for @0, and the foreign plate that replaces it.
	m := bip39FromWords(t, fixtureMasterA)
	b0, err := deriveMultisigLeg(m, "", s5Net, keys[0].OriginPath, md1, false)
	if err != nil {
		t.Fatalf("deriveMultisigLeg(@0): %v", err)
	}
	own := s5PlateFor(t, plates, verifyLeg{Slot: 0, B: b0})
	foreign, _, _, _, err := deriveSingleSigBundle(m, "", s5Net, singleSigPath(44), md.ScriptPkh)
	if err != nil {
		t.Fatalf("deriving a foreign mk1: %v", err)
	}

	// THE PREMISE, MEASURED: the substituted plate is genuinely NOT @0's, and the
	// COUNT is unchanged -- so the length precheck cannot be what catches this.
	if strings.Join(foreign.MK1, "|") == strings.Join(plates[own], "|") {
		t.Fatal("the substituted plate IS @0's own plate, so nothing was substituted")
	}
	records := append([]string(nil), md1...)
	swapped := 0
	for i, p := range plates {
		if i == own {
			records = append(records, foreign.MK1...)
			swapped++
			continue
		}
		records = append(records, p...)
	}
	if swapped != 1 {
		t.Fatalf("substituted %d plate(s), want exactly 1", swapped)
	}

	last, res := s5DriveVerifyStopAfterOneSeed(t, records, []int{0, 1, 2}, md1, fixtureMasterA, "")
	t.Logf("final screen: %q", last)

	if uiContains(last, "Verify Incomplete") {
		t.Fatalf("a readback whose @0 plate belongs to ANOTHER WALLET was reported as an "+
			"INCOMPLETE verify, i.e. as two plates checked and one outstanding. Final "+
			"screen: %q\nThe number on that screen counted slots RE-DERIVED FROM A TYPED "+
			"SEED, not plates compared: the comparator sits behind an unconditional "+
			"return and never ran. The operator walks away believing @0 is proven",
			last)
	}
	if !uiContains(last, "Verify Failed") {
		t.Fatalf("a foreign plate produced neither a failure nor an incomplete report. "+
			"Final screen: %q", last)
	}
	if res != verifyFailed {
		t.Errorf("the flow returned %v, want verifyFailed; the caller cannot re-offer a "+
			"verify whose verdict it does not get", res)
	}
	// I-3: the diagnosis reaches the SCREEN. "Check the engraved plates" over a
	// three-plate run tells an operator to distrust two plates that are perfect.
	if !uiContains(last, "@0") {
		t.Errorf("the failure screen does not name the slot whose plate is missing, "+
			"which is the only thing the operator can act on: %q", last)
	}
}

// TestVerifyIncompleteReportsWhatTheComparatorMATCHED is C-1's non-vacuity arm.
//
// Without it, "fail on every incomplete verify" satisfies the test above. An
// HONEST partial readback -- every presented plate is one this run cut -- must
// still report Verify Incomplete, and the number in it must be the number of
// plates the comparator matched, with the outstanding slot named.
func TestVerifyIncompleteReportsWhatTheComparatorMatched(t *testing.T) {
	md1, plates, _ := s5TraceBEngraved(t, false)
	records := append([]string(nil), md1...)
	for _, p := range plates {
		records = append(records, p...)
	}
	last, res := s5DriveVerifyStopAfterOneSeed(t, records, []int{0, 1, 2}, md1, fixtureMasterA, "")
	t.Logf("final screen: %q", last)

	if !uiContains(last, "Verify Incomplete") {
		t.Fatalf("an HONEST partial verify did not report Incomplete. Final screen: %q\n"+
			"Running the comparator on this path must not turn a legitimate STOP HERE "+
			"into a failure: the plates for slots nobody typed a seed for are "+
			"legitimately unclaimed", last)
	}
	if res != verifyIncomplete {
		t.Errorf("the flow returned %v, want verifyIncomplete", res)
	}
	if !uiContains(last, "Checked 2 key plates") {
		t.Errorf("the incomplete report does not state what the comparator matched: %q", last)
	}
	for _, want := range []string{"@0", "@1", "@2"} {
		if !uiContains(last, want) {
			t.Errorf("the incomplete report does not name %s: %q", want, last)
		}
	}
	// And it must no longer prescribe an action that does not exist. The remedy
	// is the re-offer its callers now make, so the screen names THAT.
	if uiContains(last, "Run verify again with the remaining seeds") {
		t.Errorf("the incomplete screen still prescribes a standalone verify that the "+
			"program table does not contain: %q", last)
	}
}

// ─── I-2: the FULL half of the flow, which no test drove ─────────────────────
//
// All five multisigVerifyFlow call sites in the test tree passed the literal
// `false`, so the ms1 half -- the per-seed ms1 capture, its binding to that
// seed's legs, the ms1-entry Back handling and the full-mode success string --
// was executed by nothing. Measured with a cold GOCACHE, the whole `if full`
// block, all of multisigVerifyMS1Entry and the full-mode multi-leg success
// string had ZERO executions, and two independent mutations to them left the
// suite green. One of those mutations -- blanking the operator-typed ms1 --
// would make EVERY full-mode verify fail, and nothing noticed.

// s5DriveVerifyFullTwoSeeds drives a FULL-mode verify through two seeds, typing
// each seed's own ms1, and returns the last frame plus the verdict.
//
// `backAtSecondMs1` presses Back at the second seed's "Type ms1" instead of
// typing it, which is the M19 mutation's scenario: that break was described as
// the same fix, for the same reason, in the same function as its pinned sibling
// twenty lines up, and only the sibling was held.
func s5DriveVerifyFullTwoSeeds(t *testing.T, records []string, expected []int,
	engravedMd1 []string, first, firstMs1, second, secondMs1 string, backAtSecondMs1 bool,
) (last string, res multisigVerifyResult) {
	t.Helper()
	p := newPlatform()
	p.display = sh2DisplaySize
	ctx := NewContext(p)
	ctx.syswBundleSeeds = append([]string(nil), records...)

	frame, quit := runUI(ctx, func() {
		res = multisigVerifyFlow(ctx, &descriptorTheme, true /* FULL */, expected, engravedMd1)
	})
	defer quit()

	if c, ok := pumpUntil(frame, "mk1 keys:", 64); !ok {
		t.Fatalf("the readback never reached the gatherer's tally; got %q", c)
	}
	click(&ctx.Router, Button3) // Done adding cards
	frame()

	typeSeed := func(phrase, ms1 string, back bool) {
		t.Helper()
		if c, ok := pumpUntil(frame, "Choose number of words", 96); !ok {
			t.Fatalf("seed entry was not reached; got %q", c)
		}
		click(&ctx.Router, Button3) // 12 WORDS
		frame()
		driveWords(&ctx.Router, phrase)
		if c, ok := pumpUntil(frame, "passphrase", 240); !ok {
			t.Fatalf("the passphrase prompt was not reached; got %q", c)
		}
		click(&ctx.Router, Button3) // Skip
		frame()
		if c, ok := pumpUntil(frame, "Type ms1", 96); !ok {
			t.Fatalf("full mode did not ask for this seed's ms1; got %q.\n"+
				"That screen is the whole subject of this driver: if it does not appear, "+
				"the flow is not in full mode and every assertion below is vacuous", c)
		}
		if back {
			click(&ctx.Router, Button1) // Back out of the ms1 entry
			frame()
			return
		}
		runes(&ctx.Router, strings.ToLower(ms1))
		click(&ctx.Router, Button3)
		frame()
	}

	typeSeed(first, firstMs1, false)
	if !backAtSecondMs1 || true {
		// The next-seed offer only appears while slots remain, which is the shape
		// both arms of this driver are in after the first seed.
		if c, ok := pumpUntil(frame, "not checked yet", 96); !ok {
			t.Fatalf("the flow did not offer the next seed after the first one; got %q", c)
		}
		click(&ctx.Router, Button3) // TYPE THE NEXT SEED
		frame()
	}
	typeSeed(second, secondMs1, backAtSecondMs1)

	for i := 0; i < 128; i++ {
		c, ok := frame()
		if !ok {
			break
		}
		last = c
		if uiContains(last, "Verify OK") || uiContains(last, "Verify Failed") ||
			uiContains(last, "Verify Incomplete") {
			break
		}
	}
	return last, res
}

// s5TraceBFullReadback is Trace B engraved in FULL mode: the md1, the three key
// plates, and the ms1 each master's own leg carries.
func s5TraceBFullReadback(t *testing.T) (records, md1 []string, ms1A, ms1B string) {
	t.Helper()
	md1, plates, ms1s := s5TraceBEngraved(t, true)
	if len(plates) != 3 {
		t.Fatalf("Trace B engraved %d key plate(s), want 3", len(plates))
	}
	// TWO ms1s FOR TWO MASTERS, measured. Trace B holds @0 and @1 from master A
	// and @2 from master B; one ms1 per DISTINCT master is the tail's dedupe, and
	// a fixture with one ms1 would let the per-seed binding pass by accident.
	if len(ms1s) != 2 {
		t.Fatalf("Trace B cut %d ms1 plate(s), want 2 (masters A and B)", len(ms1s))
	}
	if ms1s[0] == ms1s[1] {
		t.Fatal("the two ms1 plates are byte-identical, so this fixture cannot tell a " +
			"per-seed ms1 binding from a flow-global one")
	}
	records = append(records, md1...)
	for _, p := range plates {
		records = append(records, p...)
	}
	return records, md1, ms1s[0], ms1s[1]
}

// TestVerifyFullModeTwoSeedsReportsTheFULLSuccess is I-2's arm (1).
//
// It is the FIRST test in the tree to drive multisigVerifyFlow with full=true,
// and it kills the M25 mutation on its own: blanking the operator-typed ms1
// makes an empty string land in the leg, which is a hard "ms1 presence mismatch"
// at bundle/verify.go against a derived leg that carries one.
func TestVerifyFullModeTwoSeedsReportsTheFullSuccess(t *testing.T) {
	records, md1, ms1A, ms1B := s5TraceBFullReadback(t)
	last, res := s5DriveVerifyFullTwoSeeds(t, records, []int{0, 1, 2}, md1,
		fixtureMasterA, ms1A, fixtureMasterB, ms1B, false)
	t.Logf("final screen: %q", last)

	if !uiContains(last, "Verify OK") {
		t.Fatalf("a FULL-mode verify of Trace B's own plates and its own two ms1 shares "+
			"did not pass. Final screen: %q\nThis is the mode that ships: every walk and "+
			"every emulator drive picks Full, and until this test nothing in the tree ran "+
			"it", last)
	}
	if res != verifyComplete {
		t.Errorf("the flow returned %v, want verifyComplete", res)
	}
	// THE FULL-MODE SUCCESS STRING, which had zero executions in the coverage
	// profile. A watch-only pass over the same plates says something else.
	if !uiContains(last, "and the ms1 you typed") {
		t.Errorf("the success screen does not say the ms1 shares were checked, so a "+
			"full-mode pass is indistinguishable from a watch-only one: %q", last)
	}
	if !uiContains(last, "All 3 operator key plates") {
		t.Errorf("the success screen does not state how many plates were checked: %q", last)
	}
}

// TestVerifyFullModeBackAtTheSecondMs1ReportsIncomplete is I-2's arm (2), and it
// is the M19 mutation.
//
// Trace B full mode, 3 plates, 2 masters. The operator verifies master A, is
// offered the next seed, types master B, then presses Back at "Type ms1". With
// that break reverted to a `return` the flow exits SILENTLY -- and on the build
// path the next screen is the restore document, with 2 of 3 plates checked and
// no way for the operator to know.
func TestVerifyFullModeBackAtTheSecondMs1ReportsIncomplete(t *testing.T) {
	records, md1, ms1A, ms1B := s5TraceBFullReadback(t)
	last, res := s5DriveVerifyFullTwoSeeds(t, records, []int{0, 1, 2}, md1,
		fixtureMasterA, ms1A, fixtureMasterB, ms1B, true /* Back at the 2nd ms1 */)
	t.Logf("final screen: %q", last)

	if uiContains(last, "Verify OK") {
		t.Fatalf("a verify that never saw master B's ms1 reported a PASS: %q", last)
	}
	if !uiContains(last, "Verify Incomplete") {
		t.Fatalf("Back at the SECOND seed's ms1 entry walked out of a partial verify "+
			"with no report. Final screen: %q\nTwo plates were verified and one was not; "+
			"silence after a verify is indistinguishable from a pass to the operator who "+
			"walks away. Its sibling break twenty lines up is pinned; this one was not",
			last)
	}
	if res != verifyIncomplete {
		t.Errorf("the flow returned %v, want verifyIncomplete", res)
	}
	if !uiContains(last, "@2") {
		t.Errorf("the incomplete report does not name the outstanding slot: %q", last)
	}
}

// TestVerifyFullModeBindsEachMs1ToItsOwnSeed is I-2's arm (3).
//
// The ms1 travels WITH the leg for SPEC 4.1's reason: a build across two masters
// engraves two seed plates, and comparing either leg against whichever ms1 the
// flow happened to hold is how a "Full" backup carrying master A twice verifies
// clean while master B -- which k=3 needs -- is gone. The comparator's half of
// that is pinned by TestVerifyCoversEveryMastersSecret, which hand-builds
// verifyLeg values and calls verifyMultisigLegs directly; what was unpinned is
// the ASSIGNMENT in the flow.
//
// It also exercises C-1 from the other side: master A covers 2 of 3 slots, so
// this lands on the INCOMPLETE path -- where, before this fold, the comparator
// never ran and a wrong ms1 would have been reported as two plates checked.
func TestVerifyFullModeBindsEachMs1ToItsOwnSeed(t *testing.T) {
	records, md1, ms1A, ms1B := s5TraceBFullReadback(t)
	if ms1A == ms1B {
		t.Fatal("the two ms1 shares are equal, so swapping them changes nothing")
	}
	// Master A, with master B's ms1. Everything else is honest.
	last, res := s5DriveVerifyStopAfterOneSeed(t, records, []int{0, 1, 2}, md1,
		fixtureMasterA, ms1B)
	t.Logf("final screen: %q", last)

	if uiContains(last, "Verify OK") {
		t.Fatalf("master A's legs verified against master B's ms1: %q", last)
	}
	if !uiContains(last, "Verify Failed") {
		t.Fatalf("a leg compared against ANOTHER master's ms1 did not FAIL. Final "+
			"screen: %q\nA \"Full\" backup that verifies clean while one master's seed "+
			"plate is another master's is the exact loss SPEC 4.1's per-leg ms1 exists "+
			"to prevent", last)
	}
	if res != verifyFailed {
		t.Errorf("the flow returned %v, want verifyFailed", res)
	}
	// I-3 again, on the comparator's own diagnosis: the screen must carry the word
	// that stops the operator scrapping good steel.
	if !uiContains(last, "ms1") {
		t.Errorf("the failure screen never mentions the ms1, so an operator whose plates "+
			"are perfect is sent to re-cut them over a typed secret: %q", last)
	}
}

// ─── I-3: the failure text, as a function ────────────────────────────────────

// TestVerifyFailureTextNamesWhatTheComparatorFOUND.
//
// verifyMultisigLegs' error was bound for a nil check and thrown away, so an ms1
// typo, an ms1 presence mismatch and a missing plate for slot @2 all collapsed
// into "The read-back bundle does NOT match the seed. Check the engraved
// plates." -- while the flow was holding a string that said which. The same
// collapse hit errVerifyLegHasNoPlate, whose ENTIRE reason to carry a slot
// number is that it is "the only thing the operator can act on", and which two
// tests assert on with the rationale "so the operator cannot tell WHICH plate to
// re-cut". Those assertions were exercised by no production path.
func TestVerifyFailureTextNamesWhatTheComparatorFound(t *testing.T) {
	noPlate := multisigVerifyFailureText(errVerifyLegHasNoPlate{Slot: 2})
	unclaimed := multisigVerifyFailureText(errVerifyPlateUnclaimed{Plate: 1})
	wrapped := multisigVerifyFailureText(errors.New("slot @1: verify: ms1 entropy mismatch"))

	if !strings.Contains(noPlate, "@2") {
		t.Errorf("a missing plate for slot @2 produces a message that does not name @2:\n%s",
			noPlate)
	}
	if !strings.Contains(unclaimed, "plate 2") {
		t.Errorf("an unclaimed plate produces a message that does not say WHICH plate "+
			"(1-based, as errVerifyPlateUnclaimed.Error prints it):\n%s", unclaimed)
	}
	if !strings.Contains(wrapped, "ms1") {
		t.Errorf("the comparator's own diagnosis is discarded, so an ms1 typo reads as "+
			"bad steel:\n%s", wrapped)
	}
	// The three are pairwise distinct: a function returning one string for every
	// cause passes any test that checks only one of them.
	if noPlate == unclaimed || noPlate == wrapped || unclaimed == wrapped {
		t.Errorf("two of the three diagnoses read identically:\n%s\n%s\n%s",
			noPlate, unclaimed, wrapped)
	}
	// And NONE of them tells an operator to distrust plates that may be perfect
	// without saying why.
	for name, msg := range map[string]string{"no-plate": noPlate, "unclaimed": unclaimed, "wrapped": wrapped} {
		if strings.ContainsAny(msg, "—–·‘’“”…") {
			t.Errorf("%s carries a glyph the body face lacks, so its line does not draw:\n%q",
				name, msg)
		}
		assertModalBodyFits(t, "the verify's failure text ("+name+")", errorScreenBody, msg)
	}
}

// ─── I-13: a watch-only verify must not claim a secret ───────────────────────

// TestVerifyOKMessageClaimsASecretOnlyInFullMode.
//
// multisigVerifyOKMessage's single-leg arm ignored `full` and returned the
// secret-claiming body, so an ordinary watch-only run -- one held or matched
// slot, no ms1 created, none requested, none typed, none compared, the ms1 leg
// skipped on both sides by bundle.Verify's presence semantics -- finished on
// "Operator key and secret verified." len(legs)==1 is the COMMON case.
//
// The function's own doc comment states the rule it broke two lines below.
func TestVerifyOKMessageClaimsASecretOnlyInFullMode(t *testing.T) {
	// THE PREDICATE IS "CLAIMS A SECRET WAS CHECKED", not one literal word. The
	// single-leg arm says "secret" and the multi-leg arm says "the ms1 you typed";
	// both are claims about a secret and only one of them contains the word, so a
	// bare `Contains("secret")` would pass the shipped multi-leg full message
	// while calling it a watch-only one.
	claimsASecret := func(msg string) bool {
		low := strings.ToLower(msg)
		return strings.Contains(low, "secret") || strings.Contains(low, "ms1")
	}
	for _, legs := range []int{1, 3} {
		for _, full := range []bool{false, true} {
			msg := multisigVerifyOKMessage(legs, full)
			claims := claimsASecret(msg)
			if claims != full {
				t.Errorf("multisigVerifyOKMessage(%d, %v) = %q\nclaims a secret: %v, want %v.\n"+
					"A watch-only run never asks for an ms1, never compares one, and must "+
					"not report that it verified one", legs, full, msg, claims, full)
			}
			if strings.ContainsAny(msg, "—–·‘’“”…") {
				t.Errorf("multisigVerifyOKMessage(%d, %v) carries a glyph the body face "+
					"lacks:\n%q", legs, full, msg)
			}
		}
	}
	// The single-leg FULL arm is the shipped constant, unchanged: its honesty is
	// pinned by TestMultisigVerifyNoticeIsHonest and this fix must not move it.
	if multisigVerifyOKMessage(1, true) != multisigVerifyOKBody {
		t.Errorf("the single-leg full-mode message is no longer the pinned constant: %q",
			multisigVerifyOKMessage(1, true))
	}
}

// ─── I-14: never assert "a different seed" ───────────────────────────────────

// TestVerifyCoveredSeedBodyDoesNotAssertAForeignSeed.
//
// S5 makes the passphrase prompt PER HELD SLOT, with NO confirm-entry and no
// comparison between two registry entries holding the same words. An operator
// holding @0 and @1 from one seed who types `hunter2` and then `huntre2` gets
// two entries that look like two masters: buildSlotSources keys its account
// counter on MasterFP, which a passphrase changes, so BOTH get account 0, both
// derive at m/48'/0'/0'/2', the keys differ so 4.1's duplicate refusal does not
// fire, and the review screen suppresses the account suffix at account 0 so both
// lines read identically. At verify, the device told them:
//
//	"That seed's slots have already been checked. The plates still outstanding
//	 belong to a different seed."
//
// sending an operator holding the ONLY seed that exists to look for a second
// one, and never naming the real cause -- while already owning the routine that
// distinguishes part of it.
func TestVerifyCoveredSeedBodyDoesNotAssertAForeignSeed(t *testing.T) {
	cases := map[string]string{
		"already covered":      multisigVerifyCoveredSeedBody(false, false),
		"none engraved":        multisigVerifyCoveredSeedBody(true, false),
		"bare words do match":  multisigVerifyCoveredSeedBody(false, true),
		"none engraved + bare": multisigVerifyCoveredSeedBody(true, true),
	}
	for name, msg := range cases {
		low := strings.ToLower(msg)
		// THE UNCONDITIONAL HALF, and it costs nothing: never assert "a different
		// seed" when the device cannot distinguish that from "the same seed, a
		// different BIP-39 passphrase".
		if strings.Contains(low, "belong to a different seed") ||
			strings.Contains(low, "belongs to a different seed") {
			t.Errorf("%s: still asserts a foreign seed, which the device cannot know and "+
				"which a one-character passphrase divergence produces:\n%s", name, msg)
		}
		if strings.ContainsAny(msg, "—–·‘’“”…") {
			t.Errorf("%s carries a glyph the body face lacks:\n%q", name, msg)
		}
		assertModalBodyFits(t, "the verify's covered-seed refusal ("+name+")", errorScreenBody, msg)
	}
	// The arm that CAN name the passphrase does.
	if !strings.Contains(strings.ToLower(cases["already covered"]), "passphrase") {
		t.Errorf("the covered-seed refusal never offers the passphrase as an explanation, "+
			"so the cheapest cause is the one the operator is not told:\n%s",
			cases["already covered"])
	}
	// And the arm the empty re-derivation SETTLED says so concretely rather than
	// leaving the operator to guess.
	if !strings.Contains(cases["bare words do match"], "skip the passphrase") {
		t.Errorf("the proved arm does not tell the operator the action that works:\n%s",
			cases["bare words do match"])
	}
	// The two engravedNone arms stay distinguishable from the covered ones: they
	// are different states and a shared sentence would be false for one of them.
	if cases["already covered"] == cases["none engraved"] {
		t.Error("the already-covered and none-engraved refusals read identically")
	}
}

// ─── I-4 / I-12: the two flows' post-engrave wiring ──────────────────────────

// TestBothEngraveFlowsGateOnACompletedSet is I-12.
//
// bundleEngrave returned void and both abort paths were bare returns, so neither
// caller could tell a completed run from an aborted one. An operator who aborted
// at card 4 of 6 read "Bundle Incomplete ... This set is not a usable backup
// yet" and was then asked "Verify the engraved plates?" over a verify that
// structurally cannot succeed, followed by a restore document headed "This
// backup is 9 plates ... If any of them is missing, this backup is incomplete."
//
// Asserted at the CALL SITES, because the verify offer and the restore document
// both sit after a real engrave and no unit test in this package drives one.
// It is the same idiom as TestBuildPassesTheTailsSlotsToTheVerify, and it is
// stated as a limitation rather than hidden: what this pins is that the abort is
// CHECKED, and TestSupplyAbortIsTheLastScreen drives the real screens for the
// behaviour.
func TestBothEngraveFlowsGateOnACompletedSet(t *testing.T) {
	for _, tc := range []struct{ file, fn, want string }{
		{"multisig.go", "func supplyMultisigPolicyFlow(",
			`if bundleEngrave(ctx, th, "Engrave Multisig", cardsOut) != bundleEngraveDone {`},
		{"multisig_build.go", "func buildMultisigPolicyFlow(",
			`if bundleEngrave(ctx, th, "Build Policy", cardsOut) != bundleEngraveDone {`},
	} {
		body := funcBody(t, tc.file, tc.fn)
		if !strings.Contains(body, "bundleEngrave(") {
			t.Fatalf("funcBody did not capture %s in %s", tc.fn, tc.file)
		}
		if !strings.Contains(body, tc.want) {
			t.Errorf("%s does not stop when the engrave is ABORTED.\nwant the call gated as:\n"+
				"  %s\nAn aborted set then reaches the verify offer (which cannot succeed: "+
				"the md1 is emitted last and was never cut) and the restore document, which "+
				"describes a partial set as a backup", tc.file, tc.want)
		}
	}
}

// TestSupplyAbortIsTheLastScreenOfTheProgram is I-12 driven through the real
// screens, and it is the behavioural half of the assertion above.
//
// A probe on the frozen tree drove this exact route and read:
//
//	abort warning frame: "Stopped at card 1 of 4 (ms1 secret share). This set is
//	not a usable backup yet. ... Bundle Incomplete"
//	post-abort frame 0:  "Verify the engraved plates? Verify now Skip Verify Bundle"
//
// with ZERO guard screens between. TestBundleEngraveSetAbort could not see it:
// it calls bundleEngrave directly and stops at the warning, never invoking the
// caller that contains the unguarded continuation.
func TestSupplyAbortIsTheLastScreenOfTheProgram(t *testing.T) {
	md1 := s5SuppliedTraceBMd1(t)
	p := newPlatform()
	p.display = sh2DisplaySize
	ctx := NewContext(p)
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
	if c, ok := pumpUntil(frame, "passphrase", 240); !ok {
		t.Fatalf("the passphrase prompt was not reached; got %q", c)
	}
	click(&ctx.Router, Button3) // Skip
	frame()
	// The multi-slot notice, then the engrave mode.
	if c, ok := pumpUntil(frame, "What to engrave?", 32); !ok {
		click(&ctx.Router, Button3) // dismiss the multi-slot notice
		frame()
		if c, ok = pumpUntil(frame, "What to engrave?", 64); !ok {
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

	// THE ABORT. Back at the first plate's style picker is bundleEngrave's
	// set-level abort: nothing has been cut and the set on the bench is empty.
	click(&ctx.Router, Button1)
	if c, ok := pumpUntil(frame, "Bundle Incomplete", 96); !ok {
		t.Fatalf("Back at the engrave picker did not produce the abort warning; got %q", c)
	}
	click(&ctx.Router, Button3) // dismiss the abort modal

	var after []string
	for i := 0; i < 128 && !done; i++ {
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
			t.Errorf("after \"This set is not a usable backup yet\", the flow still drew "+
				"%q.\nEverything past the engrave vouches for a COMPLETE set: the verify "+
				"cannot succeed (the md1 is emitted LAST and was never cut, so it dies "+
				"reading as \"your plates are unreadable\"), and the restore document is "+
				"headed \"If any of them is missing, this backup is incomplete\".\nDrawn "+
				"after the abort:\n%q", banned, joined)
		}
	}
}

// TestBothEngraveFlowsReOfferTheVerify is I-4.
//
// "Verify Incomplete" prescribed "Run verify again with the remaining seeds" and
// nothing on the device could run it again: multisigVerifyFlow's only callers
// were one-shot post-engrave offers, gui/plate_verify.go states that the bundle
// verifies are untouched by the word-plate menu, and the program dispatch table
// contains no standalone bundle verify. The operator's only route was to re-run
// the whole engrave and cut every plate again over hours; the predictable
// response is to fund the wallet anyway.
//
// It also re-asserts the OBLIGATION WIRING the two shipped tests pin, because
// the retry loop is where a fold could most easily start passing something else:
// the tail's own slot indices and the md1 that was ENGRAVED.
func TestBothEngraveFlowsReOfferTheVerify(t *testing.T) {
	for _, tc := range []struct{ file, fn, call string }{
		{"multisig.go", "func supplyMultisigPolicyFlow(",
			"multisigVerifyFlow(ctx, th, full, engravedSlots, suppliedMd1)"},
		{"multisig_build.go", "func buildMultisigPolicyFlow(",
			"multisigVerifyFlow(ctx, th, full, engravedSlots, engraveMd1)"},
	} {
		body := funcBody(t, tc.file, tc.fn)
		if !strings.Contains(body, tc.call) {
			t.Fatalf("%s no longer hands the tail's own slots and the ENGRAVED md1 to the "+
				"verify; the obligation has lost its provenance", tc.file)
		}
		// The verdict is READ. A call whose result is discarded cannot re-offer.
		if !strings.Contains(body, "res := "+tc.call) {
			t.Errorf("%s discards the verify's verdict, so it cannot tell a clean pass "+
				"from an incomplete or a failed one", tc.file)
		}
		if !strings.Contains(body, "res != verifyIncomplete && res != verifyFailed") {
			t.Errorf("%s does not re-offer the verify after an incomplete or a failed "+
				"attempt, so the Incomplete screen's instruction still names an action "+
				"the device cannot perform", tc.file)
		}
		if !strings.Contains(body, "multisigVerifyRetryLead") {
			t.Errorf("%s does not draw the retry offer", tc.file)
		}
	}
	// The retry offer says what state the operator is in. A second identical
	// "Verify the engraved plates?" would read as the device having forgotten.
	if !strings.Contains(multisigVerifyRetryLead, "Not every plate is verified") {
		t.Errorf("the retry lead does not state why it is being offered: %q",
			multisigVerifyRetryLead)
	}
}
