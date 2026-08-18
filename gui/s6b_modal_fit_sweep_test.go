package gui

// ─── P6: GATE 4, the modal-fit SWEEP (F-192, S6b spec §4) ────────────────────
//
// F-185 built the class check (assertModalBodyFits, gui/modal_fits_test.go) and
// applied it to one block's screens. F-192's own filed text says what was left:
// "every other long modal in the firmware carries the same unmeasured
// exposure." Sweeping the WHOLE firmware is not this phase's job -- P6 sweeps
// what THIS CYCLE (S6b) added or changed, because that is the diff nobody has
// looked at with this check yet and the plan schedules P6 last precisely so it
// runs after every phase that could have introduced a new long body,
// including P1's R-M rewrite.
//
// HOW THIS WAS ENUMERATED, so a reader can redo it rather than trust it:
//
//  1. `git diff b1479a1..HEAD -- gui/*.go` (b1479a1 is the fork commit this
//     cycle's spec pins at line 27), excluding *_test.go, restricted to added
//     ('+') lines matching a quoted string of 15+ characters. That is every
//     string literal this cycle introduced or edited anywhere in gui/.
//  2. Each hit was traced to its call site to find which SCREEN SHAPE renders
//     it -- showError/showNotice (→ showModal → ErrorScreen, the Warning.Layout
//     body F-185's check targets), ConfirmWarningScreen (the other shape
//     modalRenderer supports), or something else entirely (ChoiceScreen's
//     fixed Lead line, ppConfirmFlow's own non-scrolling panel, or the
//     restore-document/acceptance-screen PAGERS).
//  3. Only showError/ConfirmWarningScreen-shaped bodies are candidates at all:
//     F-185's mechanism is specifically about Warning.Layout's scroller, which
//     is bound to Up/Down -- a control this device did not have production
//     buttons for until P5b (SPEC_s6b_pre_flash_cycle.md §5) added on-screen
//     arrows. Every other shape this sweep found is reachable by a button the
//     SH2 already has (Page/Button2, or fits without scrolling at all), so the
//     defect class F-185 exists for -- content past the fold with no way to
//     reach it -- cannot occur there. That is a mechanism difference stated
//     for each excluded item below, not a length judgement.
//  4. Among the showError/ConfirmWarningScreen candidates, one is gated per
//     STRING: a body counts as "this cycle changed" when its BYTES are new or
//     edited by this diff. A body whose bytes are byte-identical to what
//     shipped before S6b is the pre-existing, still-unswept backlog F-192's
//     own text names as future work, not this phase's -- even if the function
//     around it gained a new sibling arm or became reachable more often.
//
// ─── COVERAGE: GATED (assertModalBodyFits runs below) ────────────────────────
//
//   - multisigVerifyNoSlotBody(true, true) -- R-M's provedInnocent arm, 251
//     characters (gui/multisig_verify.go). NEW text this cycle (P1, spec
//     §3.2a). Already gated once by P1's own
//     TestProvedInnocentBodyPassesTheModalFitClassCheck
//     (gui/multisig_verify_provedinnocent_test.go), whose coverage note says
//     "P6's sweep will re-run over this body too... and state its own
//     coverage there" -- included here so ALL of this cycle's coverage is
//     readable from one file rather than split across phases.
//   - the F-204 "passphrase entered" arm, 143 characters
//     (gui/singlesig_verify.go). NEW text this cycle (P1, spec §3.2).
//   - the F-204 "no passphrase" arm, 72 characters (gui/singlesig_verify.go).
//     The BYTES are the pre-existing "Check the engraved plates." wording
//     (spec §3.2 quotes it as what already shipped), but the SITE is new --
//     before P1 this text ran unconditionally; now it is one arm of a
//     conditional this cycle wrote, selected by `passphrase != ""`. Gated
//     because the SELECTION is this cycle's, even though this particular
//     arm's prose is not, and the check costs nothing extra to run.
//   - "Read back one wallet-policy md1 AND the operator key card(s) (mk1).",
//     67 characters (gui/multisig_verify.go). Bytes are pre-existing, but
//     F-199 (P1, spec §3.1) changed what happens after it draws: this site
//     used to return verifyRefused (shown once, flow ends) and now returns
//     verifyIncomplete (the callers' retry loop re-shows it). A screen that
//     becomes REPEATEDLY reachable this cycle is swept for the same reason a
//     new one is.
//
// ─── COVERAGE: UNGATED, WITH REASON ───────────────────────────────────────────
//
//   - multisigVerifyNoSlotBody's OTHER two arms (passphraseTyped: 154 chars;
//     default: 152 chars). Bytes untouched by this diff and reachability
//     untouched (both were already loop-reachable before S6b). Pre-existing
//     backlog, not this cycle's.
//   - "Couldn't derive the bare-seed fingerprint." (gui/singlesig.go), 42
//     characters. NEW text in a NEW function (singleSigPassphrasePlateOffer,
//     P3/2.6) but under half this file's own established gating floor (the
//     shortest body TestModalsThisBlockTouchesAreDrawnInFull already treats
//     as worth checking is 87 characters) and under a tenth of the ~500
//     characters F-185 measured before this panel starts cutting text. No
//     wrap behaviour can cut a single short sentence on a body clip that
//     holds 10+ lines.
//   - "This passphrase does not fit a plate." (gui/passphrase_flow.go), 37
//     characters. Bytes pre-existing (engravePassphraseFlowFrom already
//     shipped it); P3 added a second call site with the SAME literal
//     (engravePassphraseFlowPreloaded). Same shape, same trivial length as
//     the item above -- not long by the same measure.
//   - ppConfirmWarning / ppConfirmWarningDerived (gui/passphrase_flow.go),
//     100 and 103 characters. NOT a Warning.Layout body at all:
//     ppConfirmFlow draws them in a FIXED panel with no scroller
//     ("the codebase's only scroller (Warning.Layout) is bound to
//     ButtonFilter(Up/Down), which no production path on SeedHammer II
//     emits" -- TestConfirmFitsPanel's own doc comment). That screen's fit is
//     already asserted by a DIFFERENT, pixel-height-based mechanism
//     (TestConfirmFitsPanel, gui/passphrase_flow_test.go), which P3 already
//     extended to cover the `derived` variant this cycle introduced. F-185's
//     op-tree class check has nothing to add on a screen that cannot
//     scroll-cut in the first place.
//   - "Engrave a passphrase plate?" (gui/singlesig.go), the offer's
//     ChoiceScreen Lead. ChoiceScreen renders a short fixed lead line beside
//     two buttons, never a long scrolling body -- a different screen type,
//     not a candidate.
//   - "Source: this session's own derivation" (srcDerived, gui/sysw_source.go,
//     spec §2.2). Renders through syswSourceAccept → confirmReviewScreen,
//     which is a PAGED review screen (Button2 "Page", start-index loop,
//     gui/multisig_build.go) -- reachable to its end by a button this device
//     has, exactly like the restore document below. Also 30 characters, so
//     doubly not a candidate.
//   - verifyStatusMS1Clause, "The ms1 you typed for each seed matched." (F-206,
//     gui/verify_status.go, spec §3.3), and every line P4 changed in
//     buildPassphraseInventoryLines / buildPlateInventoryLines
//     (gui/multisig_build_census.go, spec §6/§6.1) -- THE RESTORE DOCUMENT'S
//     PASSPHRASE LINES the P6 dispatch brief names explicitly. All of them
//     feed restoreDocFlow / multisigRestoreDocFlow, which render through
//     restoreDocScreen: "a plain, paged screen (NOT DescriptorScreen)"
//     (gui/singlesig_restore.go's own doc comment) with a Page button that
//     mirrors "xpubVerifyFlow's gap-free paging so the long descriptor tail
//     is always reachable". This is the mechanism difference stated in step 3
//     above: the restore document cannot lose content below an unreachable
//     fold, because its fold is reachable. F-185's exposure is specific to
//     Warning.Layout's Up/Down-bound scroller; a document with its own pager
//     is a different, already-solved problem, not a modal this check applies
//     to. (Occlusion -- a glyph drawn UNDER an opaque chip -- is a separate
//     failure mode entirely; see the boundary note below.)
//
// ─── THE BOUNDARY THIS SWEEP DOES NOT CROSS ───────────────────────────────────
//
// GATE 4 and GATE 5.3 answer DIFFERENT questions and this sweep answers only
// the first. bodyDrawnFully (gui/modal_fits_test.go) compares the drawn frame's
// OP TREE against the source string: ExtractText walks that tree with no
// notion of what is visually on top of what, so a glyph the panel draws
// UNDERNEATH P5b's opaque scroll-arrow chip is still "on the frame" as far as
// this check can see -- the text was submitted to the compositor, whether or
// not a chip was painted over it afterwards. That is occlusion, not
// truncation, and it is GATE 5.3's job (one pixel-level assertion that an
// arrow's chip does not overlap the body's first/last drawn text rows,
// gui/scroll_arrows_test.go), not this one's. This sweep does not extend to
// try to catch it, and does not claim to.

import "testing"

// TestS6bModalFitSweep is GATE 4: every body listed as GATED above, run
// through the SAME one-line class check F-185 built
// (assertModalBodyFits/errorScreenBody, gui/modal_fits_test.go). Each body is
// reproduced VERBATIM from its production call site, not from a factored-out
// function, matching the precedent TestModalsThisBlockTouchesAreDrawnInFull
// already set for a literal-body table.
func TestS6bModalFitSweep(t *testing.T) {
	for _, tc := range []struct {
		what string
		body string
	}{
		{
			"R-M's provedInnocent arm (multisigVerifyNoSlotBody(true,true), gui/multisig_verify.go)",
			multisigVerifyNoSlotBody(true, true),
		},
		{
			"F-204 'passphrase entered' arm (gui/singlesig_verify.go)",
			"The read-back bundle does NOT match the seed. " +
				"Check the passphrase before you doubt the plates: one wrong character " +
				"derives a different wallet.",
		},
		{
			"F-204 'no passphrase' arm (gui/singlesig_verify.go)",
			"The read-back bundle does NOT match the seed. Check the engraved plates.",
		},
		{
			"F-199's now-looping readback refusal (gui/multisig_verify.go:791)",
			"Read back one wallet-policy md1 AND the operator key card(s) (mk1).",
		},
	} {
		t.Run(tc.what, func(t *testing.T) {
			assertModalBodyFits(t, tc.what, errorScreenBody, tc.body)
		})
	}
}
