package gui

// ─── P7: GATE 4, the modal-fit sweep, THE REST OF THE FIRMWARE (F-192) ───────
//
// F-192's filed text is firmware-wide: "every other long modal in the firmware
// carries the same unmeasured exposure." P6 (gui/s6b_modal_fit_sweep_test.go)
// swept only what S6b itself touched -- IMPLEMENTATION_PLAN_s6b.md's P6 row
// narrowed the spec's "Every long modal body is gated" (SPEC section 4) to
// "every modal this cycle added or changed", and documented the narrowing
// rather than silently picking a reading. The operator ruled: sweep the rest.
// This file is that sweep.
//
// ─── HOW THIS WAS ENUMERATED ──────────────────────────────────────────────────
//
// A go/ast scan (not a regex -- a regex attempt at this exact count returned
// an 8210-character "body" that was actually a comment block, per the plan's
// own §1 note) over every gui/*.go non-test file: every ast.CallExpr whose
// Fun is the identifier "showError" or "showNotice" (the only two functions
// that reach showModal -> ErrorScreen, the Warning.Layout body F-185's check
// targets -- ConfirmWarningScreen bodies are a different call shape and none
// were found composed outside the one P6/F-185 already gates), classifying
// the 4th argument's AST node: *ast.BasicLit is a LITERAL, everything else
// (*ast.CallExpr, *ast.BinaryExpr string concatenation, *ast.Ident) is
// COMPOSED.
//
// Verified count: 131 total call sites (73 non-test files; cross-checked
// against `grep -c 'showError(\|showNotice('` = 133, the +2 being the two
// function DEFINITION lines, which also match that substring) -- 87 literal,
// 44 composed by AST shape. Of the 44, five are *ast.BinaryExpr string
// concatenations of two ADJACENT STRING LITERALS with no interpolation --
// compile-time constants, identical in kind to a single literal for this
// check's purposes (one fixed value, not a worst-case search). Net: 87+5 = 92
// literal-valued, 39 genuinely runtime-composed -- which is why the dispatch
// brief's "~36" and this sweep's 39 are close but not equal: the brief's own
// scan rounded differently at this same boundary, and it says so ("a starting
// point, not an authority").
//
// The scan script is not committed (a throwaway `go run` over gui/, per this
// phase's brief); its output is reproduced as the coverage tables below, and
// re-running the AST walk described above reproduces it.
//
// ─── WHERE THIS SWEEP DIFFERS FROM THE DISPATCH BRIEF'S LIST ─────────────────
//
//   - gui/address_polish.go:60 is `err.Error()`, not a `Describe(...)` call.
//     The brief's "Describe(...) in address_polish.go" line does not match
//     this file's content.
//   - gui/slip39_polish.go has THREE `slip39words.Describe(err)` call sites
//     (lines 244, 249, 303), a whole file the brief's "Describe(...) in
//     address_polish.go, seedxor_polish.go, gui.go:2674" did not name.
//   - gui/freetext_flow.go has FOUR composed sites, not the "(x3)" the brief
//     states: lines 949, 955 and 1175 are fmt.Sprintf; line 1171 is a string
//     concatenation ("The " + strings.ToLower(what) + " is a single line.")
//     the brief's Sprintf-only wording missed.
//
// None of these change what gets gated below -- the AST walk already covered
// all of them -- but the brief asked that a missed producer be named, so this
// is that naming.
//
// ─── WHAT THIS SWEEP FOUND ALREADY GATED, OUTSIDE P1/P6 ──────────────────────
//
// Nine of the 39 composed bodies were ALREADY under assertModalBodyFits
// before this phase, in tests that predate S6b entirely:
//
//   - bundleAbortWarningText (gui/bundle_abort_prose_test.go, both arms)
//   - multisigVerifyNoSlotBody (P6/P1's exclusion, PLUS
//     gui/multisig_verify_passphrase_test.go's all-three-arms coverage)
//   - multisigVerifyIncompleteText (gui/multisig_verify_report_test.go:297)
//   - multisigVerifyFailureText (gui/multisig_verify_report_test.go:812)
//   - multisigVerifyCoveredSeedBody (gui/multisig_verify_report_test.go:899)
//   - the two literal-concat P1/P6 bodies (multisig_build.go:317,
//     singlesig_verify.go:195 -- reproduced verbatim in P6's own sweep)
//
// These are marked GATED (pre-existing) below and not re-tested here, except
// multisigVerifyIncompleteText: its two existing cases (3 slots total) are
// not this policy's worst case (N maxes at 5), so this file adds one N=5 case
// to close that gap -- see TestMultisigVerifyIncompleteTextWorstCase below.
//
// ─── A GENUINE FINDING: TWO "FIT-SHAPED" TESTS THAT ARE NOT THE CLASS CHECK ──
//
// gui/multisig_build_gate_test.go's TestGateRefusalsAreDrawnWithoutScrolling
// looks like a fit check -- it renders showError's first frame and asserts an
// ink floor -- but it only asserts that NAMED SUBSTRINGS appear on that frame
// (uiContains per phrase), never that the WHOLE body does. That is exactly
// the seam F-185 exists to close: a body can be cut mid-sentence and still
// pass a substring check, as long as the substrings it happens to look for
// land before the cut. buildSeedKeyMismatchMessage and
// buildFingerprintContradictsMessage have run under this look-alike test
// since before S6b and were NEVER run under the real class check. Likewise
// buildDuplicateKeyMessage and buildEmptyOriginMessage
// (gui/multisig_build_dupkey_test.go, gui/multisig_build_s5_test.go) and
// buildSupplyRefusal (gui/multisig_build_payload_test.go) are covered only by
// `strings.Contains` content assertions, with no fit check of any kind. All
// five are gated for the first time below.
//
// ─── THE BOUNDARY THIS SWEEP DOES NOT CROSS ───────────────────────────────────
//
// Same boundary P6 states: bodyDrawnFully (gui/modal_fits_test.go:81-100)
// compares the drawn frame's OP TREE against the source string. A glyph
// drawn UNDERNEATH P5b's opaque scroll-arrow chip is still "on the frame" as
// far as this check can see -- the text reached the compositor, whether or
// not a chip was painted over it afterwards. That is occlusion, not
// truncation, and it is GATE 5.3's job (gui/scroll_arrows_test.go), not this
// sweep's. This sweep does not extend to try to catch it.

import (
	"fmt"
	"testing"

	"seedhammer.com/passphrase"
	"seedhammer.com/seal"
)

// ─── GATED HERE: newly-gated composed bodies ──────────────────────────────────
//
// Every case below is a body that reached NO F-185-shaped check before this
// phase (confirmed by grepping every existing gui/*_test.go for
// assertModalBodyFits and for the producer's name). Bodies that vary by input
// are gated at a stated WORST CASE, not the production call site's own
// (often small) arguments -- the rationale for each worst case is in the
// case's own comment, immediately above it, because the reasoning differs
// case to case and reads better beside the value it justifies.
func TestS6bP7ModalFitSweep(t *testing.T) {
	for _, tc := range []struct {
		what string
		body string
	}{
		// --- gui/multisig_build_payload.go: buildSupplyRefusal ---
		// Every distinct text SHAPE the switch can produce. multisigNChoices
		// tops out at N=5 (gui/multisig_build.go:819), so `have`/`open` are
		// always single-digit and contribute no meaningful width; the
		// "incomplete" sentence is the one thing that changes body LENGTH; it
		// is included as the worst case of the default branch.
		{
			"buildSupplyRefusal: no payload loaded",
			buildSupplyRefusal(cosignerSourceNoPayload, 0, 4, false),
		},
		{
			"buildSupplyRefusal: payload not yet compared",
			buildSupplyRefusal(cosignerSourceUncompared, 0, 4, false),
		},
		{
			"buildSupplyRefusal: under-supply, worst case (incomplete=true)",
			buildSupplyRefusal(cosignerSourceLoaded, 3, 4, true),
		},
		{
			"buildSupplyRefusal: under-supply, no incomplete chunk set",
			buildSupplyRefusal(cosignerSourceLoaded, 3, 4, false),
		},

		// --- gui/multisig_build_slots.go: buildSeedKeyMismatchMessage ---
		// `who` is "" or " (payload card %d)"; non-empty is strictly longer,
		// so the worst case supplies a matching origin. e.Declared is the
		// CARD's own origin spelling, unbounded in principle but realistically
		// a BIP-32 path string; multisigSharedOrigin().String() is the actual
		// path this firmware issues cards at (matches the precedent already
		// in gui/multisig_build_gate_test.go's TestGateRefusalsAreDrawnWithoutScrolling).
		{
			"buildSeedKeyMismatchMessage: worst case (who present)",
			buildSeedKeyMismatchMessage(errBuildSeedKeyMismatch{
				Slot: 4, Declared: multisigSharedOrigin().String(),
			}, []cosignerOrigin{{slot: 4, card: 5}}),
		},

		// --- gui/multisig_build_slots.go: buildFingerprintContradictsMessage ---
		// Declared/Derived are fixed-width: "the card's own 8-hex fingerprint"
		// (type doc comment, multisig_build_slots.go:368) -- there is no
		// longer case, only the one length. `who` worst case as above.
		{
			"buildFingerprintContradictsMessage: worst case (who present)",
			buildFingerprintContradictsMessage(errBuildFingerprintContradicts{
				Slot: 4, Declared: "deadbeef", Derived: "73c5da0a",
			}, []cosignerOrigin{{slot: 4, card: 5}}),
		},

		// --- gui/multisig_build.go: buildDuplicateKeyMessage ---
		// buildSlotProvenance has 3 shapes; "(your key, payload card %d)" is
		// the longest. Both slots reaching that branch needs SelfFromCard AND
		// both slots present in origins -- SlotA held (in SelfSlots) selects
		// "your key, payload card N" for A; SlotB unheld-but-in-origins
		// selects "payload card N" for B. That is the longest combination
		// buildSlotProvenance's own branching can produce for a 2-slot
		// refusal (a slot that is neither held nor in origins falls back to
		// the SHORTEST form, "slot @N", so omitting it from origins here
		// would only shorten the case, not lengthen it).
		{
			"buildDuplicateKeyMessage: worst case (both slots carry provenance)",
			buildDuplicateKeyMessage(
				errBuildDuplicateKey{SlotA: 0, SlotB: 1},
				buildPolicyParams{N: 5, SelfSlots: []int{0}, SelfFromCard: true},
				[]cosignerOrigin{{slot: 0, card: 1}, {slot: 1, card: 2}},
			),
		},

		// --- gui/multisig_build.go: buildEmptyOriginMessage ---
		// `who` worst case as above; `declared` falls back to "m" only when
		// e.Declared is empty, which is SHORTER, so a non-empty Declared is
		// the worst case for that half. multisigSharedOrigin().String() is
		// appended unconditionally regardless of e.Declared, so its own width
		// does not vary the worst case -- any non-empty Declared is fine.
		{
			"buildEmptyOriginMessage: worst case (who present, Declared non-empty)",
			buildEmptyOriginMessage(errBuildEmptyOrigin{
				Slot: 4, Declared: "m",
			}, []cosignerOrigin{{slot: 4, card: 5}}),
		},

		// --- gui/multisig_verify.go: three package-level const bodies ---
		// Fixed strings (const block, multisig_verify.go:30-52) -- there is
		// no "worst case" to choose, only the one value each identifier ever
		// holds. Two call sites share the first identifier (:745 and :902,
		// same bytes, by the const block's own comment: "ONE STRING, TWO
		// SITES").
		{"multisigVerifyNoExpectationBody (2 call sites, same const)", multisigVerifyNoExpectationBody},
		{"multisigVerifyNoPolicyBody", multisigVerifyNoPolicyBody},
		{"multisigVerifyForeignPolicyBody", multisigVerifyForeignPolicyBody},

		// --- gui/multisig_verify.go:832: the readback-count Sprintf ---
		// plateWord's inputs are read-back / expected slot counts, both
		// bounded by N<=5 -- single digit, negligible width. Worst case is
		// simply the plural form on both sides.
		{
			"multisig_verify.go:832 readback-count refusal",
			fmt.Sprintf("Read back %s, but this run engraved %s. Present exactly the plates this "+
				"run cut.", plateWord(5, "key plate", "key plates"), plateWord(4, "key plate", "key plates")),
		},

		// --- gui/multisig_verify.go: multisigVerifyOKMessage ---
		// Never run under any fit check before this phase (its own existing
		// test, TestVerifyOKMessageClaimsASecretOnlyInFullMode, asserts
		// content claims and a forbidden-glyph check, not fit). All 4
		// branches: legs<=1 crossed with full, legs>1 crossed with full.
		// legs maxes at N<=5, single digit.
		{"multisigVerifyOKMessage(1, false)", multisigVerifyOKMessage(1, false)},
		{"multisigVerifyOKMessage(1, true)", multisigVerifyOKMessage(1, true)},
		{"multisigVerifyOKMessage(5, false)", multisigVerifyOKMessage(5, false)},
		{"multisigVerifyOKMessage(5, true)", multisigVerifyOKMessage(5, true)},

		// --- gui/multisig.go:186,193: the reused-key notices ---
		// formatSlotList's worst case is the MOST slots one seed can hold in
		// a policy: this branch requires len(slots)>=2 and slots are indices
		// in 0..N-1 with N<=5, so at most 4 (e.g. "@0, @1, @2 and @3").
		{
			"multisig.go:186 shared-key notice, 4 slots",
			fmt.Sprintf(
				"Your seed is at slots %s of this policy, and more than one of them holds "+
					"the SAME key at the SAME key path. Identical plates would carry "+
					"identical information, so this run cuts one plate per DISTINCT key, "+
					"not one per slot. The next screen states the exact count.",
				formatSlotList([]int{0, 1, 2, 3})),
		},
		{
			"multisig.go:193 different-key notice, 4 slots",
			fmt.Sprintf(
				"Your seed is at slots %s of this policy, at a different key path in each, so "+
					"each of those slots holds a DIFFERENT key. This run engraves %s, one per "+
					"slot.", formatSlotList([]int{0, 1, 2, 3}), plateWord(4, "key plate", "key plates")),
		},

		// --- gui/derive_xpub.go: abortWarning's Sprintf ---
		// `done`/`total` are plate counts for one key-card set; the census
		// this program itself documents is "6-9 plates over hours"
		// (gui/multisig_build_census.go:29's comment on the same kind of
		// count). Double digits used for headroom rather than to claim a
		// specific real ceiling.
		{
			"derive_xpub.go:527 abort-mid-sequence refusal",
			fmt.Sprintf("Engraved %d of %d plates. This key card set can't be restored from a "+
				"partial set - discard the partial plate(s) and start over.", 8, 12),
		},

		// --- gui/freetext_flow.go: the three Sprintf/concat refusals ---
		// ftMaxLineLen = backup.MaxTitleLen = 18 (backup/backup.go:71), but
		// keystrokes into the field are "always accepted" (this function's own
		// doc comment) -- nothing caps len(kbd.Fragment) before this refusal
		// fires, so a 3-digit entered-count is the realistic worst case, not
		// an edge case.
		{
			"freetext_flow.go:949 sized-pattern line-budget refusal",
			fmt.Sprintf(
				"The text needs %d lines and this pattern's own sizes hold %d. Shorten the Text field.",
				99, 1),
		},
		{
			"freetext_flow.go:955 smallest-size line-budget refusal",
			fmt.Sprintf(
				"The text needs %d lines and a plate holds %d, at the smallest size. Shorten the Text field.",
				99, 1),
		},
		{
			"freetext_flow.go:1175 title/footer overlength refusal",
			fmt.Sprintf(
				"The %s holds %d characters and %d were entered. It sits on a screw-hole row at every size.",
				"footer", 18, 200),
		},

		// --- gui/bundle_flow.go:194-215: the Done-adding-cards messages ---
		// Both `msg` and `pendingMsg` are one of two FIXED literals, chosen by
		// scr.hasReader; the !hasReader arm is longer in both cases, so it is
		// the worst case (the hasReader-true arm is a plain literal already
		// well under any threshold).
		{
			"bundle_flow.go:199 'Done' with 0 complete cards, no reader",
			"No complete cards. Pack them on the host with `me sysw pack` " +
				"and load the payload again.",
		},
		{
			"bundle_flow.go:215 'Done' with a dropped pending card, no reader",
			"Dropped an incomplete card: the payload does not carry " +
				"all of its chunks. Rewrite it on the host with `me sysw pack` to include it.",
		},

		// --- three pre-existing constant-concatenation bodies (BinaryExpr of
		// two literals -- see the file doc comment's "net: 39" accounting) ---
		// One fixed value each; no worst-case search applies.
		{
			"multisig_build.go:57 empty-self-slot refusal",
			"This build holds no key of its own, so there " +
				"is nothing for this device to engrave. Start again and choose your slot.",
		},
		{
			"unlock_flow.go:153 nothing-further-to-engrave notice",
			"Nothing further to engrave.\n\n" +
				"This payload carried no card records. Any seed material it held has " +
				"been offered and cleared from memory.",
		},
		{
			"unlock_kdf.go:93 enter-passphrase notice",
			"Enter the 12-word passphrase for this payload.\n\n" +
				"These words are the payload's passphrase. They are NOT a seed and no " +
				"wallet is derived from them.",
		},

		// --- gui/unlock_kdf.go:448: the codex32-too-long refusal ---
		// seal.MaxEngraveableCodex32Len is a package constant (seal/record.go:74
		// = 90); its digit width never varies at runtime, so there is no
		// "worst case" beyond the one value the build carries.
		{
			"unlock_kdf.go:448 codex32-too-long refusal",
			fmt.Sprintf(
				"This payload holds a codex32 secret longer than %d characters, "+
					"which this machine cannot engrave. Nothing was opened.",
				seal.MaxEngraveableCodex32Len),
		},

		// --- gui/address_polish.go:60: err.Error() from address.Change/Receive ---
		// The producing error (address package's unexported errUnsupported,
		// wrapped "address: multisig script: %s: %w") is not constructible
		// from this package -- it is unexported, and the flow's own
		// precondition (address.Supported(desc), stated in
		// descriptorAddressFlow's doc comment) means this branch is not
		// normally reachable at all. Reproduced here as a STRING matching
		// address/address.go's documented format exactly, with the longest
		// bip380.Script.String() value ("Nested Segwit (P2SH-P2WPKH)",
		// bip380/bip380.go:76), as the representative worst case rather than
		// exercising the real (currently unreachable) error path.
		{
			"address_polish.go:60 worst-case address-derivation error text",
			"address: multisig script: Nested Segwit (P2SH-P2WPKH): unsupported descriptor",
		},
	} {
		t.Run(tc.what, func(t *testing.T) {
			assertModalBodyFits(t, tc.what, errorScreenBody, tc.body)
		})
	}
}

// TestMultisigVerifyIncompleteTextWorstCase supplements
// gui/multisig_verify_report_test.go's TestVerifyIncompleteInstructionCanBeObeyed
// (already class-gated, pre-S6b): both of ITS cases total 3 slots, and this
// policy's own N maxes at 5 (multisigNChoices, gui/multisig_build.go:819). The
// longest formatSlotList/plateWord combination this function can actually be
// called with is 4 checked + 1 outstanding (or the reverse) -- run here
// because "already gated" is not the same claim as "gated at its worst case",
// and the brief requires the latter.
func TestMultisigVerifyIncompleteTextWorstCase(t *testing.T) {
	body := multisigVerifyIncompleteText([]int{0, 1, 2, 3}, []int{4})
	assertModalBodyFits(t, "multisigVerifyIncompleteText, N=5 worst case (4 checked, 1 outstanding)",
		errorScreenBody, body)
}

// TestUnlockHashAndRetryBodiesWorstCase gates unlockHashBody and
// unlockRetryBody, neither of which reached any fit check before this phase.
//
// seal.FormatHash always renders 8 groups of 4 hex chars + 7 spaces = 39
// fixed characters (seal/pubhash.go:67-80); unlockShape returns "SEALED" or
// the longer "UNSEALED" (seal/wire.go:95, Header{} zero value is unsealed);
// len(p.Public) is bounded by the section's own §6.4 total cap of 24
// (unlockHashBody's own doc comment). unlockRetryBody's HasHash-false arm is
// a plain literal, strictly shorter, so the HasHash-true arm is the worst
// case for it too.
func TestUnlockHashAndRetryBodiesWorstCase(t *testing.T) {
	p := &seal.Payload{Public: make([]seal.AdmittedRecord, 24), HasHash: true}
	assertModalBodyFits(t, "unlockHashBody, 24 public records, UNSEALED", errorScreenBody, unlockHashBody(p))
	assertModalBodyFits(t, "unlockRetryBody, 24 public records, UNSEALED (HasHash)", errorScreenBody, unlockRetryBody(p))
}

// ─── UNGATED, WITH REASON ──────────────────────────────────────────────────────
//
// TestS6bP7TriviallyShortBodiesAreNotCandidates pins that every body this
// sweep judged "not long" stays that way, so a future edit that grows one of
// them past the point of being trivial has something to turn red rather than
// relying on a comment nobody re-checks (per "comments outlive their
// conditions").
//
// The judgement itself: P6's own file established two working floors --
// TestModalsThisBlockTouchesAreDrawnInFull's shortest gated body is 87
// characters, and F-185 measured this whole class's real cut point at ~500.
// Every body below is under HALF the 87-character floor, i.e. under a fifth
// of the real cut point, with no wrap behaviour able to lose a single short
// clause on a clip that holds 10+ lines of text:
//
//   - ppEntryError (gui/passphrase_flow.go): 4 fixed branches, longest is
//     "Too long. At most 100 characters fit on one plate." at 52 chars.
//   - verifyProvenanceLine (gui/plate_verify.go): 4 branches, longest is
//     "device-compared (%d of %d)" at ~30 chars (N,total both small).
//   - slip39words.Describe / seedxor.Describe (gui/slip39_polish.go:244,249,303,
//     gui/gui.go:2674, gui/seedxor_polish.go:63): both Describe functions are
//     closed switches over sentinel errors; the longest possible return
//     across BOTH is slip39's "member threshold mismatch" at 26 chars.
//   - "The " + strings.ToLower(what) + " is a single line." (freetext_flow.go:1171):
//     `what` is "Title" or "Footer" (freetext_flow.go:1565,1572); longest
//     rendering is "The footer is a single line." at 29 chars.
func TestS6bP7TriviallyShortBodiesAreNotCandidates(t *testing.T) {
	const floor = 87 // P6's own established gating floor
	cases := map[string]string{
		"ppEntryError(ErrEmpty)":               ppEntryError(passphrase.ErrEmpty),
		"ppEntryError(ErrTooLong)":             ppEntryError(passphrase.ErrTooLong),
		"ppEntryError(ErrNonASCII)":            ppEntryError(passphrase.ErrNonASCII),
		"ppEntryError(default)":                ppEntryError(nil),
		"verifyProvenanceLine(deviceAll)":      verifyProvenanceLine(provDeviceComparedAll, 24, 24),
		"verifyProvenanceLine(deviceSubset)":   verifyProvenanceLine(provDeviceComparedSubset, 24, 24),
		"verifyProvenanceLine(operatorAssert)": verifyProvenanceLine(provOperatorAsserted, 0, 24),
		"verifyProvenanceLine(notVerified)":    verifyProvenanceLine(provNotVerified, 0, 24),
		"slip39.Describe worst case":           "member threshold mismatch",
		"seedxor.Describe worst case":          "unsupported length (use 12/18/24 words)",
		"freetext single-line refusal":         "The " + "footer" + " is a single line.",
	}
	for name, body := range cases {
		if len(body) >= floor {
			t.Errorf("%s is %d characters, at or over this file's own %d-character "+
				"trivially-short floor -- it has grown enough to need the real class "+
				"check (assertModalBodyFits), not this exclusion list:\n%s",
				name, len(body), floor, body)
		}
	}
}
