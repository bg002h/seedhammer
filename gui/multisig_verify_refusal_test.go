package gui

import (
	"os"
	"strings"
	"testing"
)

// ─── F-199 (S6b spec §3.1): verifyRefused re-offers at ONE site, and only one ─
//
// Before this fold, ALL FOUR of multisigVerifyFlow's structural-refusal return
// sites shared verifyRefused, and neither engrave caller re-offers on it --
// so a readback that came off the plates but could not be accounted for
// (extractReadbackMd1AndMk1s failing) showed a screen naming EXACTLY what the
// operator should re-present, then fell straight through to the restore
// document with no chance to act on it.
//
// THE FIX IS PER-SITE, NOT PER-VERDICT (spec §3.1: "Widening the verdict is
// prohibited -- it would make all four loop, including two programmer-error
// refusals"). Two of the four sites (:717 empty expectedSlots, :727 empty
// engravedMd1) are genuine programmer errors no caller can trigger by retrying
// with the same inputs; a third (verifyFreshSlots' ferr != nil) is a defensive
// re-check of the first that is unreachable in-process. Only the readback
// site is correctable, so only IT moves to verifyIncomplete -- the verdict the
// callers already re-offer on.
//
// THIS FILE PINS ALL FOUR ARMS, split the way the gate names them:
//
//	TestVerifyReoffersOnAnUnaccountableReadback      :753 -- moved, now LOOPS
//	TestExpectedSlotsNeverReassignedInVerifyFlow     :854 -- SOURCE assertion (unreachable)
//	TestVerifyRefusedIsNotInTheCallerLoopCondition   :717/:727 -- SOURCE assertion (still DO NOT loop)
//
// THE LAST ONE IS A SOURCE ASSERTION, NOT A WALK, AND THAT IS A DELIBERATE COST
// TRADE, RECORDED RATHER THAN SILENT. A behavioural version exists --
// TestBothEngraveFlowsDriveTheRetryLoop (gui/multisig_engrave_tail_walk_test.go)
// already drives BOTH real engrave callers to the offer and proves the loop
// DOES re-offer on verifyIncomplete/verifyFailed; a symmetric "does NOT
// re-offer on verifyRefused" walk was written, run, and measured at 119s
// (73.68s supply + 45.38s build) against a `gui` package the runbook already
// measures at 429-507s of ~600s -- close enough to the ceiling that adding it
// risked reproducing the exact "the package died at 600s mid-test with every
// assertion passing" failure this cycle's own toolchain notes warn about. The
// loop condition itself is one line, identical in both files
// (`if res != verifyIncomplete && res != verifyFailed { break }`), and Go's
// enum values are distinct by construction (multisigVerifyResult's iota block)
// -- so "verifyRefused is excluded from that condition" is a SOURCE fact, and
// checking it as one is both cheaper and more direct than inferring it from a
// walk's absence of a screen. What a source assertion cannot see -- some
// OTHER path also causing a loop -- is exactly what
// TestBothEngraveFlowsDriveTheRetryLoop's own walk already exercises for the
// two verdicts that DO need to loop, on the same inline code.

// TestVerifyReoffersOnAnUnaccountableReadback is GATE 3.1's core fix: the
// readback-accounting failure at gui/multisig_verify.go (extractReadbackMd1AndMk1s
// fails on gathered cards, spec's :753) must return verifyIncomplete, which is
// the ONLY thing that makes the caller loop's `res != verifyIncomplete && res
// != verifyFailed` re-offer.
//
// Driven with ONLY the mk1 plate on the bench and NO md1 among the gathered
// cards, so extractReadbackMd1AndMk1s's `len(md1) == 0` arm fires -- the exact
// "a plate could not be read or accounted for" state the follow-up names.
func TestVerifyReoffersOnAnUnaccountableReadback(t *testing.T) {
	_, md1, plate, idx := s5OneSlotReadback(t)
	p := newPlatform()
	p.display = sh2DisplaySize
	ctx := NewContext(p)
	ctx.syswBundleSeeds = append([]string(nil), plate...) // the mk1 ONLY -- no md1 card
	var rec verifyRecord
	var res multisigVerifyResult
	done := false
	frame, quit := runUI(ctx, func() {
		res = multisigVerifyFlow(ctx, &descriptorTheme, false, []int{idx}, md1, &rec)
		done = true
	})
	defer quit()

	if c, ok := pumpUntil(frame, "mk1 keys:", 32); !ok {
		t.Fatalf("the readback never reached the gatherer's tally; got %q", c)
	}
	click(&ctx.Router, Button3) // Done adding cards -- only NOW does extraction run
	frame()

	c, ok := pumpUntil(frame, "Read back one wallet-policy md1", 32)
	if !ok {
		t.Fatalf("an md1-less readback did not reach the accounting refusal; got %q", c)
	}
	click(&ctx.Router, Button3) // dismiss the error
	for i := 0; i < 32 && !done; i++ {
		if _, ok := frame(); !ok {
			break
		}
	}
	if !done {
		t.Fatal("multisigVerifyFlow did not return after the accounting refusal was dismissed")
	}
	if res != verifyIncomplete {
		t.Fatalf("the readback-accounting failure returned %d, want verifyIncomplete (%d). "+
			"F-199 (S6b spec §3.1) requires THIS SPECIFIC site to re-offer, and the two "+
			"caller loops only re-offer on verifyIncomplete/verifyFailed -- a verdict of "+
			"verifyRefused here leaves this correctable failure un-retryable", res, verifyIncomplete)
	}
	if !rec.adverse {
		t.Error("a readback that could not be accounted for did not record adverse; the " +
			"restore document would then understate what the device observed")
	}
	if rec.pass != nil {
		t.Error("a refused readback recorded a PASS")
	}
}

// TestExpectedSlotsNeverReassignedInVerifyFlow is GATE 3.1's SOURCE ASSERTION
// for the :854 arm (verifyFreshSlots' ferr != nil), which spec §3.1 rules
// unreachable in-process rather than testing behaviourally.
//
// verifyFreshSlots' only error return is errVerifyNoExpectedSlots, on
// len(expected) == 0 (gui/multisig_verify.go:324-336). multisigVerifyFlow
// already refuses that exact condition on expectedSlots BEFORE any seed is
// typed (len(expectedSlots) == 0, the :717 site) and returns before the derive
// loop that calls verifyFreshSlots can run. So the :854 branch can only differ
// from :717's premise if expectedSlots is reassigned somewhere in between --
// this test is the thing that would have to stop being true for that argument
// to fail, checked the way TestEngraveSingleSigFlowTypedOnly_Structural checks
// an analogous absence: strip comments, then grep the CODE.
func TestExpectedSlotsNeverReassignedInVerifyFlow(t *testing.T) {
	src, err := os.ReadFile("multisig_verify.go")
	if err != nil {
		t.Fatalf("read multisig_verify.go: %v", err)
	}
	// Strip // line-comments so the assertion tests CODE, not prose that is free
	// to say "expectedSlots" while explaining exactly this guarantee (as the
	// comments beside :717, :753 and :854 now do).
	var b strings.Builder
	for _, line := range strings.Split(string(src), "\n") {
		if i := strings.Index(line, "//"); i >= 0 {
			line = line[:i]
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	code := b.String()
	// expectedSlots occurs ONLY inside multisigVerifyFlow's signature, doc
	// comment and body in this file (measured: every occurrence of the
	// identifier in gui/multisig_verify.go falls between the function's `func`
	// line and its closing brace) -- so a whole-file scan is equivalent to a
	// function-scoped one and does not need a brace-matching extractor that
	// would itself need testing.
	for _, forbidden := range []string{"expectedSlots =", "expectedSlots :="} {
		if strings.Contains(code, forbidden) {
			t.Fatalf("multisig_verify.go's code contains %q -- expectedSlots is reassigned "+
				"somewhere, so :854 (verifyFreshSlots' ferr != nil) is no longer provably "+
				"unreachable and needs a BEHAVIOURAL test in place of this source assertion", forbidden)
		}
	}
}

// TestVerifyRefusedIsNotInTheCallerLoopCondition is GATE 3.1's non-loop half
// for :717/:727, as a SOURCE assertion -- see the file comment above for why
// this is not a behavioural walk.
//
// It pins the loop condition VERBATIM in both engrave callers, so a future
// fold that widens it to `res != verifyIncomplete && res != verifyFailed &&
// res != verifyRefused` -- the exact move spec §3.1 forbids, since verifyRefused
// is still shared by the two programmer-error refusals at :717/:727 -- turns
// this red rather than silently re-arming a loop with nothing the operator can
// fix by retrying.
func TestVerifyRefusedIsNotInTheCallerLoopCondition(t *testing.T) {
	const want = "if res != verifyIncomplete && res != verifyFailed {"
	for _, f := range []string{"multisig_build.go", "multisig.go"} {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		if !strings.Contains(string(src), want) {
			t.Errorf("%s does not contain the caller loop condition verbatim (%q). Either "+
				"it was reworded (re-verify by hand what it now re-offers on, and update "+
				"this pin) or it was widened to include verifyRefused, which would re-arm "+
				"the loop on TWO programmer-error refusals (empty expectedSlots, empty "+
				"engravedMd1) that spec §3.1 requires stay non-looping", f, want)
		}
	}
}
