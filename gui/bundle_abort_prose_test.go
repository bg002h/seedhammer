package gui

import (
	"os"
	"strings"
	"testing"
)

// ─── S5: the abort screen, which is the only screen an interrupted operator ──
//     ever reaches
//
// The engrave tail cuts 6 to 9 plates over hours. Nothing records which of them
// were cut, and a power loss loses that state. The restore document says what the
// set is -- but it is printed at the END of a SUCCESSFUL run, so the operator
// whose engrave died never sees it. The abort warning is what they see instead,
// and until S5 it said, in full:
//
//	"Stopped at card N of M (label). A partial bundle can't be used - discard
//	 the engraved plate(s) and start the bundle over."
//
// Two things wrong with that, and the first one is live today on shipped code.

// TestAbortWarningSaysDestroyForASetCarryingASeed.
//
// "Discard the engraved plate(s)" is correct for a public plate and is an
// instruction to BIN A SECRET when the set carries an ms1. Full mode cuts the ms1
// seed plate FIRST, so at almost any abort in a full build the plate already on
// the bench is the one with the seed on it. Nothing in the shipped text
// distinguishes them.
//
// This is not a new requirement being added at S5; it is text that is WRONG
// TODAY on a device that engraves seeds, corrected in the stage that owns the
// tail. The gate is CARDS-DERIVED, following the R0-I2 pattern that already
// decides the ms1 reminder, so no other flow's decision changes shape.
func TestAbortWarningSaysDestroyForASetCarryingASeed(t *testing.T) {
	secret := bundleAbortWarningText(bundlePlate{cardIdx: 1, cardTotal: 3, label: "ms1 share"}, true)
	public := bundleAbortWarningText(bundlePlate{cardIdx: 1, cardTotal: 3, label: "md1 policy"}, false)

	// The seed-bearing set says DESTROY, in a word the operator cannot read as
	// "put it in the bin".
	if !strings.Contains(secret, "DESTROY") {
		t.Errorf("a set carrying a seed plate does not say DESTROY:\n%s", secret)
	}
	// And it must not still be telling them to discard it.
	if strings.Contains(strings.ToLower(secret), "discard the engraved plate") {
		t.Errorf("a set carrying a seed plate still says to discard it, which is an "+
			"instruction to bin a secret:\n%s", secret)
	}
	// It says HOW, because "destroy" without a method is a word an operator
	// satisfies by putting the plate somewhere else.
	if !strings.Contains(strings.ToLower(secret), "grind") &&
		!strings.Contains(strings.ToLower(secret), "cut it up") {
		t.Errorf("DESTROY is stated with no method, so it is a word rather than an "+
			"action:\n%s", secret)
	}

	// A set with NO secret in it says no such thing. A warning that cries DESTROY
	// over a public plate is a warning that gets ignored on the run that matters
	// (§0.1's corollary, applied to a hazard rather than to a default).
	if strings.Contains(public, "DESTROY") {
		t.Errorf("a set with no seed plate is told to destroy it anyway:\n%s", public)
	}

	// The two are genuinely different sentences. A helper returning one string for
	// both cases passes any test that checks only one of them.
	if secret == public {
		t.Errorf("the seed-bearing and public aborts read identically:\n%s", secret)
	}

	// Both still say WHERE the operator stopped. That was right in the shipped
	// text and a rewrite must not lose it.
	for name, msg := range map[string]string{"secret": secret, "public": public} {
		if !strings.Contains(msg, "1 of 3") {
			t.Errorf("%s abort no longer says where the run stopped:\n%s", name, msg)
		}
	}
}

// TestAbortWarningTellsTheOperatorHowToFinishTheSet.
//
// Re-running the same inputs mints BYTE-IDENTICAL plates: no rand in md/, mk/ or
// codex32/, and mk's chunk_set_id derives from the bytecode rather than from
// randomness. TestReRunMintsByteIdenticalPlates pins that property. It is what
// makes an interrupted 9-plate engrave recoverable -- the operator re-runs and
// cuts only what is missing -- and until S5 the device never told them, on the
// one screen an interrupted operator actually reaches. "Start the bundle over"
// says the opposite of the truth: it tells them to throw away hours of work that
// is still good.
func TestAbortWarningTellsTheOperatorHowToFinishTheSet(t *testing.T) {
	for _, secret := range []bool{false, true} {
		msg := bundleAbortWarningText(bundlePlate{cardIdx: 2, cardTotal: 4, label: "mk1 key"}, secret)
		low := strings.ToLower(msg)

		// The property, in the operator's terms rather than the encoder's.
		if !strings.Contains(low, "same") {
			t.Errorf("secret=%v: the abort never says a re-run reproduces the same "+
				"plates:\n%s", secret, msg)
		}
		if !strings.Contains(low, "missing") {
			t.Errorf("secret=%v: the abort never says only the MISSING plates need "+
				"cutting, which is the whole recovery:\n%s", secret, msg)
		}
		// And it must have stopped saying the opposite.
		if strings.Contains(low, "start the bundle over") {
			t.Errorf("secret=%v: the abort still tells the operator to start over, "+
				"throwing away plates a re-run would reproduce exactly:\n%s", secret, msg)
		}
		// It draws: no glyph the body face lacks.
		if strings.ContainsAny(msg, "—–·‘’“”…") {
			t.Errorf("secret=%v: the abort carries a glyph the body face lacks, so its "+
				"line does not draw:\n%q", secret, msg)
		}
	}
}

// TestAbortWarningsAreDrawnInFull is F-185's class check on this block's own
// screens.
//
// The abort warning is a showError modal: its body scrolls, nothing on the frame
// says so, and the SH2 has no button that scrolls it. It is also the screen most
// likely to be made longer by a later edit, because it is where every "we should
// also tell them..." lands. Both arms are gated, with margin.
func TestAbortWarningsAreDrawnInFull(t *testing.T) {
	for _, tc := range []struct {
		what   string
		secret bool
	}{
		{"the abort warning for a set carrying a seed", true},
		{"the abort warning for a public-only set", false},
	} {
		body := bundleAbortWarningText(bundlePlate{cardIdx: 2, cardTotal: 9, label: "mk1 key card"}, tc.secret)
		assertModalBodyFits(t, tc.what, errorScreenBody, body)
	}
}

// TestBundleSetCarriesASecret is the cards-derived gate itself.
//
// It answers the same question bundleShowMs1Reminder does and answers it the
// opposite way round, so the two are stated as one rule rather than as two
// hand-written scans that can drift apart.
func TestBundleSetCarriesASecret(t *testing.T) {
	public := []bundleCard{{kind: cardMD1}, {kind: cardMK1}}
	withSeed := []bundleCard{{kind: cardMS1}, {kind: cardMD1}}
	if bundleSetCarriesASecret(public) {
		t.Error("a set of md1 + mk1 was reported as carrying a secret")
	}
	if !bundleSetCarriesASecret(withSeed) {
		t.Error("a set containing an ms1 was reported as carrying no secret")
	}
	// The two questions are complements: exactly one of them is true of any set.
	for _, cards := range [][]bundleCard{public, withSeed} {
		if bundleSetCarriesASecret(cards) == bundleShowMs1Reminder(cards) {
			t.Errorf("the secret gate and the ms1 reminder gate agree on %v; they are "+
				"the same question and must answer it oppositely", cards)
		}
	}
}

// TestMs1ReminderIsTitledForTheProgramThatShowedIt closes F-182.
//
// S2 made bundleGatherFlow's title the caller's, which fixed the gather for all
// five callers. bundleEngrave still hard-coded one:
//
//	showError(ctx, th, "Engrave Bundle", bundleMs1ReminderText())
//
// and that modal is shown at the END of a Build-policy engrave too -- measured:
// S2's walk cuts 9 plates and reaches it. An operator who chose "Build policy"
// off the menu is told, by a screen with no other context, that they are in
// "Engrave Bundle". It is a DIFFERENT screen from D-4's, and bundleEngrave is
// shared by the bundle flow, single-sig and the supplied-md1 path as well, so
// the title is a PARAMETER rather than a rename.
func TestMs1ReminderIsTitledForTheProgramThatShowedIt(t *testing.T) {
	body := funcBody(t, "bundle_flow.go", "func bundleEngrave(")
	if strings.Contains(body, `"Engrave Bundle"`) {
		t.Error(`bundleEngrave still hard-codes "Engrave Bundle", so on the Build path ` +
			"the end-of-engrave reminder names the wrong program")
	}
	// Non-vacuous: the slice must actually contain the function.
	if !strings.Contains(body, "bundlePlatePlan") {
		t.Fatalf("funcBody did not capture bundleEngrave; got %d bytes", len(body))
	}

	// And every production caller passes its OWN program's name. Asserted on the
	// call sites rather than on one walk, because four of the five are flows this
	// stage does not otherwise touch.
	for file, want := range map[string]string{
		"bundle_flow.go":    `"Engrave Bundle"`,
		"singlesig.go":      `"Engrave Single-Sig"`,
		"multisig.go":       `"Engrave Multisig"`,
		"multisig_build.go": `"Build Policy"`,
	} {
		src := readGuiFile(t, file)
		idx := strings.Index(src, "bundleEngrave(ctx, th, ")
		if idx < 0 {
			t.Errorf("%s no longer calls bundleEngrave; this table is stale", file)
			continue
		}
		call := src[idx:min(idx+120, len(src))]
		if !strings.Contains(call, want) {
			t.Errorf("%s calls bundleEngrave without its own title %s; the end-of-engrave "+
				"reminder would name another program:\n%s", file, want, call)
		}
	}
}

// readGuiFile reads a source file in this package for the call-site assertions
// above. Same blunt-source-inspection idiom as funcBody, and for the same
// reason: a title that is concatenated or Sprintf'd is one these guards must
// still see.
func readGuiFile(t *testing.T, file string) string {
	t.Helper()
	b, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("reading %s: %v", file, err)
	}
	return string(b)
}
