package gui

import (
	"image"
	"strings"
	"testing"
	"unicode/utf8"

	"seedhammer.com/gui/assets"
	"seedhammer.com/gui/op"
	"seedhammer.com/passphrase"
)

// The FONTPROOF! test pattern (spec 9.1's O1 procedure; followup
// seedhammer-fontproof-test-pattern).
//
// Two traps this file works around, both of which have already produced false
// passes in this feature:
//
//  1. op.Drawer.ExtractText collects the runes of EVERY drawn text op
//     regardless of occlusion, so it cannot see a label that is overdrawn, off
//     the panel, or below the bottom edge. Fit is therefore asserted by
//     MEASURING RECTANGLES (TestFontProofPromptFitsPanel), never by looking for
//     text.
//  2. A rendered space inks nothing, so ExtractText yields "DEADBEEF", never
//     "DEAD BEEF". Comparing a frame against a spaced constant reads as
//     "absent" for every case, which would pass exactly the rows a test exists
//     to catch. So the grouping is pinned on the STRINGS
//     (TestFontProofFingerprintsCanonicalAndGrouped) and only the space-stripped
//     form is looked for on screen.
//
// And one trap specific to this feature: uiContains strips spaces from its
// needle, so uiContains(frame, "Test Pattern") matches a keyboard readout
// showing "FONTPROOF!"... no -- worse, uiContains(frame, "Font Proof") does.
// Any "is the prompt up?" check must match a phrase that appears ONLY in the
// prompt body. That is what ppProofShown is for.

// ppProofShown reports whether the frame is the font-proof PROMPT.
//
// It matches on the prompt's own warning phrase, not on its title: a title of
// "Font Proof" would be matched by a keyboard readout containing the literal
// "FONTPROOF!", because uiContains lowercases and strips spaces from the
// needle. That false positive would make every "the prompt did NOT appear"
// assertion below vacuous.
func ppProofShown(content string) bool {
	return uiContains(content, "REPLACES ALL THREE")
}

// --- The three fixed constants ------------------------------------------------

// TestFontProofPatternIsEvery95PrintableASCII derives the pattern from the
// codepoint range and compares it to the literal, so a dropped, doubled or
// transposed character in the hand-written constant cannot survive. It then
// pins the three properties the flow depends on: the count, the order, and that
// production validation accepts it.
func TestFontProofPatternIsEvery95PrintableASCII(t *testing.T) {
	var want strings.Builder
	for r := rune(0x20); r <= 0x7E; r++ {
		want.WriteRune(r)
	}
	if ppFontProofPassphrase != want.String() {
		t.Fatalf("the pattern is not 0x20-0x7E in codepoint order.\n got %q\nwant %q",
			ppFontProofPassphrase, want.String())
	}
	if n := utf8.RuneCountInString(ppFontProofPassphrase); n != 95 {
		t.Fatalf("the pattern is %d runes, want 95", n)
	}
	// Order and coverage, stated independently of the builder above so a bug in
	// BOTH would have to be the same bug.
	seen := make(map[rune]bool, 95)
	prev := rune(0x1F)
	for _, r := range ppFontProofPassphrase {
		if r <= prev {
			t.Fatalf("the pattern is not in ascending codepoint order at %q (after %q)", r, prev)
		}
		if r < 0x20 || r > 0x7E {
			t.Fatalf("the pattern contains %#x, which is not printable ASCII", r)
		}
		seen[r] = true
		prev = r
	}
	for r := rune(0x20); r <= 0x7E; r++ {
		if !seen[r] {
			t.Errorf("the pattern omits %q (%#x) -- O1 would engrave a plate missing a glyph", r, r)
		}
	}
	// The plate is only useful if the flow will actually accept it.
	if err := passphrase.ValidatePassphrase(ppFontProofPassphrase); err != nil {
		t.Fatalf("the flow's own validator refuses the pattern: %v", err)
	}
	// It must contain a real 0x20, because proving the space MARK is the point
	// of the plate.
	if !strings.ContainsRune(ppFontProofPassphrase, ' ') {
		t.Error("the pattern has no literal space, so the plate cannot show the space mark")
	}
}

// TestFontProofFingerprintsCanonicalAndGrouped: both values must survive
// passphrase.ValidateFingerprint unchanged -- that is the precondition
// backup.Passphrase documents but does not enforce -- and must render grouped
// 4-and-4 as the plate carries them.
//
// The grouping is asserted on the STRINGS. It is not observable in a rendered
// frame: a space inks nothing, so ExtractText yields "DEADBEEF" whatever the
// grouping does, and a test comparing a frame to "DEAD BEEF" would report
// "absent" for a correct implementation and a broken one alike.
func TestFontProofFingerprintsCanonicalAndGrouped(t *testing.T) {
	for _, tc := range []struct {
		which        ppFingerprintStep
		value, group string
	}{
		{ppSeedFP, ppFontProofSeedFP, "DEAD BEEF"},
		{ppCombinedFP, ppFontProofCombFP, "CAFE BABE"},
	} {
		name := ppFingerprintTitle(tc.which)
		canon, err := passphrase.ValidateFingerprint(tc.value)
		if err != nil {
			t.Errorf("%s: the constant %q is not a valid fingerprint: %v", name, tc.value, err)
			continue
		}
		if canon != tc.value {
			t.Errorf("%s: %q canonicalises to %q; the constant is not already canonical", name, tc.value, canon)
		}
		if got := passphrase.GroupFingerprint(canon); got != tc.group {
			t.Errorf("%s: renders %q, want %q", name, got, tc.group)
		}
		if got := ppFontProofFingerprint(tc.which); got != tc.value {
			t.Errorf("%s: ppFontProofFingerprint returned %q, want %q", name, got, tc.value)
		}
		if got := ppFontProofFragment(tc.which); got != tc.group {
			t.Errorf("%s: the field is seeded with %q, want the grouped %q", name, got, tc.group)
		}
		// What the confirm screen will say, end to end.
		if got := ppFingerprintClaim(ppFingerprintTitle(tc.which), tc.value); !strings.HasSuffix(got, tc.group) {
			t.Errorf("%s: the confirm line is %q, which does not carry the grouped value %q", name, got, tc.group)
		}
	}
	// The two fields must not be wired to the same constant.
	if ppFontProofSeedFP == ppFontProofCombFP {
		t.Error("both fingerprint fields carry the same value")
	}
}

// --- The prompt ---------------------------------------------------------------

// TestFontProofPromptStatesAllThreeFields is the honesty requirement in
// constraint (a): the prompt must say it replaces ALL THREE fields, because
// triggering from a fingerprint field clobbers a passphrase already typed.
// "Load the test pattern?" alone is not honest wording for that.
//
// It also pins the "no" branch's explanation, without which an operator whose
// real passphrase IS FONTPROOF! has no way to know what to answer.
func TestFontProofPromptStatesAllThreeFields(t *testing.T) {
	body := ppFontProofAsk + " " + ppFontProofReplaces + " " + ppFontProofKeep(true) +
		" " + ppFontProofKeep(false)
	for _, want := range []string{
		"ALL THREE",        // the count, said plainly
		"REPLACES",         // that it destroys, not merely fills
		"Passphrase",       // field 1, by the name the flow gives it
		"Seed FP",          // field 2
		"Expected Comb FP", // field 3
		"95",               // what the passphrase becomes
		"printable ASCII",  //
		"DEAD BEEF",        // the seed fingerprint, as rendered
		"CAFE BABE",        // the combined fingerprint, as rendered
		ppFontProofTrigger, // the string the operator typed
		"real passphrase",  // that the trigger can collide with a real one
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the prompt never says %q.\nprompt: %q", want, body)
		}
	}
	// The title must not be the only thing that identifies the screen, and must
	// not itself be a substring of the trigger -- see ppProofShown.
	if uiContains(ppFontProofTrigger, ppFontProofTitle) {
		t.Errorf("the title %q is matched by the trigger literal %q; every "+
			"\"prompt not shown\" assertion would be vacuous", ppFontProofTitle, ppFontProofTrigger)
	}
}

// TestFontProofPromptFitsPanel: the prompt must fit the panel the machine
// actually has, 480x320, WITHOUT scrolling. The codebase's only scroller
// (Warning.Layout) is bound to ButtonFilter(Up/Down), which nothing on
// SeedHammer II emits, so anything past the bottom edge is unreadable on the
// device.
//
// Measured as rectangles against the same budget the layout spends. It cannot
// be done from ExtractText: that collects runes from every drawn text op
// regardless of where they landed, so an overflowing prompt reads as fully
// present.
func TestFontProofPromptFitsPanel(t *testing.T) {
	p := newPlatform()
	p.display = sh2DisplaySize
	ctx := NewContext(p)
	dims := ctx.Platform.DisplaySize()
	if dims != sh2DisplaySize {
		t.Fatalf("the fit test is running at %v, not the real %v panel", dims, sh2DisplaySize)
	}
	area := ppConfirmArea(dims)
	_, szP := ppFontProofBody(ctx, &descriptorTheme, area.Dx(), true)
	_, szF := ppFontProofBody(ctx, &descriptorTheme, area.Dx(), false)
	// Both wordings must fit; measure the taller one.
	sz := szP
	if szF.Y > sz.Y {
		sz = szF
	}
	if sz.Y > area.Dy() {
		t.Errorf("the prompt needs %dpx of height but only %dpx is available on a %v panel; "+
			"the overflow would be unreadable, because the scroller is bound to buttons the machine does not have",
			sz.Y, area.Dy(), dims)
	}
	if sz.X > area.Dx() {
		t.Errorf("the prompt is %dpx wide in a %dpx area", sz.X, area.Dx())
	}
	// layoutTitle wraps at width-32 and draws at y=8 inside the leadingSize band.
	if h := ctx.Styles.title.Measure(dims.X-32, "%s", ppFontProofTitle).Y; h > leadingSize-8 {
		t.Errorf("the title %q is %dpx tall and wraps out of the %dpx title band",
			ppFontProofTitle, h, leadingSize-8)
	}
}

// TestFontProofPromptButtonsReachable: both answers must be tappable on the
// real panel. A prompt with an unreachable "no" would force the pattern on an
// operator whose passphrase genuinely is FONTPROOF!.
func TestFontProofPromptButtonsReachable(t *testing.T) {
	h := newPPHarness(t)
	var answered, answer bool
	h.start(func() {
		answer = ppFontProofPrompt(h.ctx, &descriptorTheme, true)
		answered = true
	})
	if !ppProofShown(h.content) {
		t.Fatalf("the prompt did not render its warning; got %q", h.content)
	}
	// point() fails unless the target was drawn, sits on the panel, and is the
	// topmost thing at its own centre.
	h.point(h.widget("proofNo").(*Clickable), "the prompt's NO button")
	h.point(h.widget("proofYes").(*Clickable), "the prompt's YES button")
	h.tapWidget("proofYes")
	for i := 0; i < 8 && !answered; i++ {
		h.frame()
	}
	if !answered || !answer {
		t.Fatalf("tapping YES did not accept the prompt (answered=%v answer=%v)", answered, answer)
	}
}

// TestFontProofPromptNoAnswersNo: the two buttons must not both mean yes.
func TestFontProofPromptNoAnswersNo(t *testing.T) {
	h := newPPHarness(t)
	var answered, answer bool
	h.start(func() {
		answer = ppFontProofPrompt(h.ctx, &descriptorTheme, true)
		answered = true
	})
	h.tapWidget("proofNo")
	for i := 0; i < 8 && !answered; i++ {
		h.frame()
	}
	if !answered {
		t.Fatalf("tapping NO left the prompt up; got %q", h.content)
	}
	if answer {
		t.Fatal("tapping NO accepted the pattern")
	}
}

// --- Triggering, through the PRODUCTION flow -----------------------------------

// ppProofFields is what the flow ended up holding, read off the confirm screen
// plus the flow's own secret buffer.
type ppProofFields struct {
	confirm string
	secret  []byte
}

// driveToConfirm walks engravePassphraseFlow to its confirm screen, triggering
// the font proof in the field named by trigger.
//
// Every step is a PointerEvent aimed at a hit area read back from the frame
// that was drawn, at sh2DisplaySize. Nothing here synthesizes a ButtonEvent.
func driveToConfirm(t *testing.T, trigger ppStep, answerYes bool) (*ppFlowRun, ppProofFields) {
	t.Helper()
	r := startPPFlow(t)
	h := r.h

	// Step 1: the passphrase.
	if trigger == ppStepEntry {
		triggerProof(t, h, answerYes, "95/100")
	} else {
		r.enterPassphrase("hunter2")
	}
	h.mustReach("Seed FP")

	// Step 2: the seed fingerprint.
	if trigger == ppStepSeedFP {
		triggerProof(t, h, answerYes, ppFontProofSeedFP)
	} else {
		r.enterFingerprint("")
	}
	h.mustReach("Expected Comb FP")

	// Step 3: the combined fingerprint.
	if trigger == ppStepCombinedFP {
		triggerProof(t, h, answerYes, ppFontProofCombFP)
	} else {
		r.enterFingerprint("")
	}
	h.mustReach("QR Code")
	r.chooseQR(false)
	h.mustReach("Confirm")
	return r, ppProofFields{confirm: h.content, secret: r.secret}
}

// triggerProof types the constant into the field the flow is currently showing,
// accepts the field, answers the prompt, and leaves the flow one step further
// on.
//
// The two answers differ in how they leave the screen, and that asymmetry IS
// the requirement:
//
//   - YES loads the pattern and STAYS on the field (constraint (d): the
//     operator must see what landed), so a second OK is needed to advance.
//     onScreen is asserted first, on the frame drawn after the prompt closed --
//     if the flow had advanced instead, that frame would be the next step's and
//     the assertion would fail.
//   - NO changes nothing and falls straight through to the field's own
//     validation, which accepts FONTPROOF! as a passphrase and advances on the
//     SAME tap. A second OK there would silently accept the following step too.
func triggerProof(t *testing.T, h *ppHarness, yes bool, onScreen string) {
	t.Helper()
	h.typeString(ppFontProofTrigger)
	h.tapWidget("ok")
	if !ppProofShown(h.content) {
		t.Fatalf("typing %q and accepting the field did NOT offer the test pattern; got %q",
			ppFontProofTrigger, h.content)
	}
	if !yes {
		h.tapWidget("proofNo")
		return
	}
	h.tapWidget("proofYes")
	if !uiHas(h.content, onScreen) {
		t.Fatalf("after loading the pattern the field does not show %q -- either the pattern "+
			"did not land or the flow advanced off the screen; got %q", onScreen, h.content)
	}
	h.tapWidget("ok")
}

// TestFontProofTriggersFromEveryField is the requirement: the trigger works
// from ANY of the three fields, and each one populates ALL THREE.
//
// It drives engravePassphraseFlow, not the sub-flows, so a step that forgot to
// pass the loader through -- or was handed nil -- fails here.
func TestFontProofTriggersFromEveryField(t *testing.T) {
	for _, tc := range []struct {
		name string
		step ppStep
	}{
		{"passphrase", ppStepEntry},
		{"seed_fp", ppStepSeedFP},
		{"comb_fp", ppStepCombinedFP},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, got := driveToConfirm(t, tc.step, true)
			// The passphrase. A rendered space inks nothing and the confirm
			// screen substitutes the mark for it, so the FIRST character of the
			// pattern (0x20) appears as ppSpaceMark.
			wantMarked := strings.Replace(ppFontProofPassphrase, " ", string(ppSpaceMark), 1)
			if !uiHas(got.confirm, wantMarked) {
				t.Errorf("the confirm screen does not carry the 95-character pattern.\nwant %q inside\n got %q",
					wantMarked, got.confirm)
			}
			// The counts line is derived from the raw secret, so it is
			// independent evidence the buffer -- not just the display -- holds
			// 95 characters with exactly one space.
			if !uiHas(got.confirm, "95chars,1space,1leading") {
				t.Errorf("the confirm screen's counts do not describe the pattern "+
					"(95 chars, 1 space, 1 leading); got %q", got.confirm)
			}
			// Both fingerprints. Only the space-stripped form is observable.
			for _, fp := range []string{ppFontProofSeedFP, ppFontProofCombFP} {
				if !uiHas(got.confirm, fp) {
					t.Errorf("the confirm screen does not carry %q; got %q", fp, got.confirm)
				}
			}
			// And the flow's own buffer really holds it, so the assertions
			// above cannot be satisfied by a display-only write.
			if !strings.Contains(string(got.secret), ppFontProofPassphrase) {
				t.Errorf("the flow's secret buffer does not hold the pattern")
			}
		})
	}
}

// TestFontProofBuildsAPlate: the pattern must survive the whole engrave path,
// or O1 has no plate. The trigger would otherwise be a screen that promises a
// proof and refuses at the last step.
func TestFontProofBuildsAPlate(t *testing.T) {
	for _, qr := range []bool{false, true} {
		plate, err := ppBuildPlate(engraverParams, []byte(ppFontProofPassphrase),
			ppFontProofSeedFP, ppFontProofCombFP, qr)
		if err != nil {
			t.Fatalf("qr=%v: the font-proof pattern does not build a plate: %v", qr, err)
		}
		if plate.Duration == 0 {
			t.Fatalf("qr=%v: built an empty plate", qr)
		}
	}
}

// --- The "no" branch ----------------------------------------------------------

// TestFontProofNoBranchKeepsItAsAPassphrase is constraint (b), the one that
// makes the prompt REQUIRED rather than a courtesy: any string can be somebody's
// real passphrase, including this one. Declining must engrave FONTPROOF!, not
// the pattern.
func TestFontProofNoBranchKeepsItAsAPassphrase(t *testing.T) {
	_, got := driveToConfirm(t, ppStepEntry, false)
	if !uiHas(got.confirm, ppFontProofTrigger) {
		t.Fatalf("declining the prompt did not keep %q as the passphrase; got %q",
			ppFontProofTrigger, got.confirm)
	}
	if uiHas(got.confirm, "abcdefghijklmnopqrstuvwxyz") {
		t.Fatalf("declining the prompt loaded the pattern anyway; got %q", got.confirm)
	}
	if !uiHas(got.confirm, "10chars,nospaces") {
		t.Fatalf("the confirm screen does not describe a 10-character passphrase; got %q", got.confirm)
	}
	// Both fingerprints must be untouched -- the loader writes all three or
	// none, and "no" means none.
	for _, fp := range []string{ppFontProofSeedFP, ppFontProofCombFP} {
		if uiHas(got.confirm, fp) {
			t.Fatalf("declining the prompt still wrote %q into a fingerprint field; got %q", fp, got.confirm)
		}
	}
	if !uiContains(got.confirm, "not recorded") {
		t.Fatalf("the skipped fingerprints are not reported as empty; got %q", got.confirm)
	}
}

// TestFontProofNoBranchInFingerprintFieldRefuses: declining in a fingerprint
// field must fall through to the field's OWN validation, which refuses
// FONTPROOF! because it is not hex. The operator is told what the field wants
// rather than being silently advanced or silently overwritten.
func TestFontProofNoBranchInFingerprintFieldRefuses(t *testing.T) {
	for _, which := range []ppFingerprintStep{ppSeedFP, ppCombinedFP} {
		t.Run(ppFingerprintTitle(which), func(t *testing.T) {
			h, r := startPPFingerprint(t, which)
			h.typeString(ppFontProofTrigger)
			h.tapWidget("ok")
			if !ppProofShown(h.content) {
				t.Fatalf("the field did not offer the pattern; got %q", h.content)
			}
			h.tapWidget("proofNo")
			if r.done {
				t.Fatalf("the field accepted %q as a fingerprint (returned %q)", ppFontProofTrigger, r.fp)
			}
			if !h.pump(8, "8 hex digits") {
				t.Fatalf("declining gave no explanation of what the field wants; got %q", h.content)
			}
			if r.seedFP != "" || r.combinedFP != "" || r.n != 0 {
				t.Fatalf("declining still loaded the pattern (seedFP=%q combFP=%q n=%d)",
					r.seedFP, r.combinedFP, r.n)
			}
		})
	}
}

// --- Scope: whole field, on OK, this program only ------------------------------

// TestFontProofNeedsTheWholeField is constraint (c): the trigger fires only when
// the constant is the ENTIRE field. A substring match would make every
// passphrase containing the word untypeable.
func TestFontProofNeedsTheWholeField(t *testing.T) {
	for _, typed := range []string{
		ppFontProofTrigger + "x",
		"x" + ppFontProofTrigger,
		"FONTPROOF",  // the trigger without its '!'
		"fontproof!", // wrong case
	} {
		t.Run(typed, func(t *testing.T) {
			h, r := startPPEntry(t)
			h.typeString(typed)
			h.tapWidget("ok")
			if ppProofShown(h.content) {
				t.Fatalf("%q offered the test pattern; only the whole constant may", typed)
			}
			for i := 0; i < 8 && !r.done; i++ {
				h.frame()
			}
			if !r.done || !r.ok {
				t.Fatalf("%q was not accepted as a passphrase (done=%v ok=%v)", typed, r.done, r.ok)
			}
			if got := string(r.dst[:r.n]); got != typed {
				t.Fatalf("the step returned %q, want the typed %q", got, typed)
			}
		})
	}
}

// TestFontProofNotCheckedPerKeystroke is constraint (c)'s other half: the check
// runs on OK, not while typing. A per-keystroke check would interrupt the
// operator mid-passphrase, and -- worse -- would fire on a prefix of a longer
// real passphrase.
func TestFontProofNotCheckedPerKeystroke(t *testing.T) {
	h, _ := startPPEntry(t)
	for i, r := range ppFontProofTrigger {
		h.typeRune(r)
		if ppProofShown(h.content) {
			t.Fatalf("the prompt appeared after keystroke %d (%q) -- the check is per-keystroke, not on OK",
				i+1, string(r))
		}
	}
	// Sanity: the fragment really is the trigger, so the loop above was not
	// passing because nothing was typed.
	kbd := h.widget("kbd").(*PassphraseKeyboard)
	if kbd.Fragment != ppFontProofTrigger {
		t.Fatalf("typed fragment is %q, want %q -- the no-prompt assertions above tested nothing",
			kbd.Fragment, ppFontProofTrigger)
	}
	h.tapWidget("ok")
	if !ppProofShown(h.content) {
		t.Fatalf("the prompt did not appear on OK either; got %q", h.content)
	}
}

// TestFontProofDoesNotFireInAddressVerification is constraint (e). The
// fingerprint fields use NewAddressKeyboard, which typed-address verification
// shares -- so a check placed in the keyboard, or in any shared helper, would
// leak the trigger into a flow where FONTPROOF! is simply a (bad) address.
//
// The returned string is the field's whole contents, so asserting it comes back
// verbatim proves BOTH that the trigger did not fire and that the characters
// really were typed. A test that only looked for the absence of the prompt
// would pass just as happily against a keyboard that dropped every keystroke.
func TestFontProofDoesNotFireInAddressVerification(t *testing.T) {
	ctx := NewContext(newPlatform())
	var got string
	var ok, done bool
	frame, quit := runUI(ctx, func() {
		got, ok = typeAddressFlow(ctx, &descriptorTheme)
		done = true
	})
	defer quit()
	frame()
	runes(&ctx.Router, ppFontProofTrigger)
	content, _ := frame()
	if ppProofShown(content) {
		t.Fatalf("typing %q in address entry offered the test pattern; got %q", ppFontProofTrigger, content)
	}
	click(&ctx.Router, Button3) // OK
	for i := 0; i < 8 && !done; i++ {
		content, _ = frame()
		if ppProofShown(content) {
			t.Fatalf("accepting %q in address entry offered the test pattern; got %q", ppFontProofTrigger, content)
		}
	}
	if !ok || got != ppFontProofTrigger {
		t.Fatalf("address entry returned (%q,%v); want (%q,true) -- the trigger fired, or the "+
			"characters never reached the keyboard and this test proved nothing",
			got, ok, ppFontProofTrigger)
	}
}

// TestFontProofDoesNotFireInBip85IndexEntry is the other shared consumer.
//
// bip85IndexEntryFlow does not hand back its fragment, so the readout is
// checked on screen BEFORE tapping OK: without that, an empty fragment would
// produce the very same "enter a whole number" refusal and the test would pass
// having typed nothing.
func TestFontProofDoesNotFireInBip85IndexEntry(t *testing.T) {
	ctx := NewContext(newPlatform())
	var idx int
	var ok, done bool
	frame, quit := runUI(ctx, func() {
		idx, ok = bip85IndexEntryFlow(ctx, &descriptorTheme)
		done = true
	})
	defer quit()
	frame()
	runes(&ctx.Router, ppFontProofTrigger)
	content, _ := frame()
	if !uiHas(content, ppFontProofTrigger) {
		t.Fatalf("the child-index readout does not show %q, so nothing below is tested; got %q",
			ppFontProofTrigger, content)
	}
	if ppProofShown(content) {
		t.Fatalf("typing %q in child-index entry offered the test pattern; got %q", ppFontProofTrigger, content)
	}
	click(&ctx.Router, Button3) // OK
	for i := 0; i < 8; i++ {
		content, _ = frame()
		if ppProofShown(content) {
			t.Fatalf("accepting %q in child-index entry offered the test pattern; got %q",
				ppFontProofTrigger, content)
		}
		if uiContains(content, "whole number") {
			break
		}
	}
	if !uiContains(content, "whole number") {
		t.Fatalf("child-index entry did not refuse %q with its own message; got %q", ppFontProofTrigger, content)
	}
	if done || ok || idx != 0 {
		t.Fatalf("child-index entry returned (%d,%v,done=%v) for %q", idx, ok, done, ppFontProofTrigger)
	}
}

// TestFontProofLivesOutsideTheSharedKeyboard is the structural half of
// constraint (e), and the one that would survive a NEW consumer being added:
// PassphraseKeyboard -- which NewAddressKeyboard wraps, and which BIP-85 and
// address verification therefore share -- must not know the trigger exists.
//
// The trigger is TYPED through the keyboard rather than assigned, so the final
// keystroke that completes the constant actually runs through commit(). That is
// where a per-keystroke check would naturally be put, and assigning Fragment
// directly would never reach it.
func TestFontProofLivesOutsideTheSharedKeyboard(t *testing.T) {
	ctx := NewContext(newPlatform())
	// A frame callback that records rather than a nil one: a keyboard that
	// tried to put a screen up would otherwise spin invisibly until the test
	// timeout instead of failing with a reason.
	drew := false
	ctx.FrameCallback = func(o op.Op) { drew = true; ctx.Done = true }
	kbd := NewAddressKeyboard(ctx)
	runes(&ctx.Router, ppFontProofTrigger)
	for kbd.Update(ctx) {
	}
	if kbd.Fragment != ppFontProofTrigger {
		t.Fatalf("the shared keyboard holds %q after typing %q -- the trigger has leaked into "+
			"PassphraseKeyboard, which BIP-85 and address verification also use",
			kbd.Fragment, ppFontProofTrigger)
	}
	if drew {
		t.Fatal("the shared keyboard put a screen up while the trigger was typed")
	}
}

// --- The loader ---------------------------------------------------------------

// TestFontProofLoaderWritesAllThree pins the loader itself, so a partial write
// is caught even if every screen test above were satisfied by a display-only
// change.
func TestFontProofLoaderWritesAllThree(t *testing.T) {
	dst := make([]byte, passphrase.MaxLen)
	n := 0
	seedFP, combFP := "prior-seed", "prior-comb"
	ppFontProofLoader(dst, &n, &seedFP, &combFP)()
	if n != len(ppFontProofPassphrase) {
		t.Errorf("loader set n=%d, want %d", n, len(ppFontProofPassphrase))
	}
	if got := string(dst[:n]); got != ppFontProofPassphrase {
		t.Errorf("loader wrote %q, want the pattern", got)
	}
	if seedFP != ppFontProofSeedFP {
		t.Errorf("loader set seedFP=%q, want %q", seedFP, ppFontProofSeedFP)
	}
	if combFP != ppFontProofCombFP {
		t.Errorf("loader set combinedFP=%q, want %q", combFP, ppFontProofCombFP)
	}
}

// TestFontProofOfferIgnoresOtherText: the offer helper is an equality test
// against the whole field, and a nil loader disables it rather than crashing.
//
// Neither case may reach the prompt at all -- which is asserted, not assumed.
// A bare return value would be a false pass under a mutation that DID put the
// prompt up and then had it dismissed: the offer would return false either way.
// The frame callback is also what stops such a mutation spinning in
// ppFontProofPrompt's loop until the test binary times out.
func TestFontProofOfferIgnoresOtherText(t *testing.T) {
	newCtx := func(drew *bool) *Context {
		ctx := NewContext(newPlatform())
		ctx.FrameCallback = func(o op.Op) { *drew = true; ctx.Done = true }
		return ctx
	}
	called, drew := false, false
	ctx := newCtx(&drew)
	load := func() { called = true }
	for _, typed := range []string{"", "hunter2", "FONTPROOF", ppFontProofTrigger + " "} {
		if ppFontProofOffer(ctx, &descriptorTheme, typed, true, load) {
			t.Errorf("%q triggered the offer", typed)
		}
	}
	if called {
		t.Error("the loader ran for a field that is not the trigger")
	}
	if drew {
		t.Error("a field that is not the trigger still put the prompt up")
	}
	// A nil loader disables the trigger; it must not prompt and must not panic.
	drew = false
	nilCtx := newCtx(&drew)
	if ppFontProofOffer(nilCtx, &descriptorTheme, ppFontProofTrigger, true, nil) {
		t.Error("a nil loader still reported the pattern as loaded")
	}
	if drew {
		t.Error("a nil loader still put the prompt up, which no answer could act on")
	}
}

// --- Golden-adjacent guard ------------------------------------------------------

// TestFontProofBodyDrawsSomething guards the case ppProofShown cannot: a body
// that measured correctly but drew nothing would satisfy the fit test and fail
// every screen test with an unhelpful message. Extract over a generous rect,
// because the body is returned un-offset.
func TestFontProofBodyDrawsSomething(t *testing.T) {
	p := newPlatform()
	p.display = sh2DisplaySize
	ctx := NewContext(p)
	area := ppConfirmArea(ctx.Platform.DisplaySize())
	body, sz := ppFontProofBody(ctx, &descriptorTheme, area.Dx(), true)
	if sz.Y == 0 {
		t.Fatal("the prompt body measured zero height")
	}
	d := new(op.Drawer)
	txt := d.ExtractText(image.Rect(-2000, -2000, 2000, 2000), body)
	if !ppProofShown(txt) {
		t.Fatalf("the prompt body does not draw its warning; drew %q", txt)
	}
	// Every line must actually be drawn, not just the one ppProofShown matches.
	for _, want := range []string{"Load", "REPLACES", "Back"} {
		if !uiContains(txt, want) {
			t.Errorf("the prompt body omits the line beginning %q; drew %q", want, txt)
		}
	}
}

// TestFontProofKeepLineMatchesTheField pins the declining branch's sentence to
// what declining ACTUALLY does in each field. The first version of this prompt
// said "Back = no: continue with FONTPROOF! exactly as typed" everywhere, which
// is true only in the passphrase field: a fingerprint field cannot continue with
// it, because ValidateFingerprint refuses a non-hex value and asks again.
//
// This is the one prompt in the feature whose whole justification is honesty --
// it exists so an operator whose real passphrase IS FONTPROOF! knows what to
// answer -- so a sentence that is false in two of the three fields it appears in
// is worth a test of its own.
func TestFontProofKeepLineMatchesTheField(t *testing.T) {
	pass := ppFontProofKeep(true)
	fing := ppFontProofKeep(false)

	if pass == fing {
		t.Fatal("the declining sentence is identical in both fields; it cannot be true in both")
	}
	// The passphrase field is the only one that continues with the value.
	if !strings.Contains(pass, "continue with") {
		t.Errorf("the passphrase wording no longer promises to continue with the typed value: %q", pass)
	}
	// The fingerprint field must NOT promise that, and must say what happens.
	if strings.Contains(fing, "continue with") {
		t.Errorf("the fingerprint wording promises to continue with FONTPROOF!, which "+
			"ValidateFingerprint refuses to do: %q", fing)
	}
	if !strings.Contains(fing, "hex") {
		t.Errorf("the fingerprint wording does not say the field will ask again for hex: %q", fing)
	}
	// Both must name the constant, so the operator can tell which text is meant.
	for _, s := range []string{pass, fing} {
		if !strings.Contains(s, ppFontProofTrigger) {
			t.Errorf("declining sentence does not name %q: %q", ppFontProofTrigger, s)
		}
	}
}

// TestFontProofPromptIconsNotSwapped pins which ICON sits on which answer.
//
// layoutNavigation places a button by its Clickable.Button and draws whatever
// Icon it was given, so the two are independent: swapping the icons leaves every
// tap target where it was, and every other test taps by registered tag rather
// than by glyph. The full suite passes with them swapped. An operator, though,
// reads the glyph -- and a back arrow that means "load the pattern and discard
// the passphrase I just typed" is precisely the outcome the prompt exists to
// prevent.
func TestFontProofPromptIconsNotSwapped(t *testing.T) {
	noBtn := &Clickable{Button: Button1}
	yesBtn := &Clickable{Button: Button3}
	nav := ppFontProofNav(noBtn, yesBtn)
	if len(nav) != 2 {
		t.Fatalf("the prompt offers %d answers, want 2", len(nav))
	}
	for _, tc := range []struct {
		clk  *Clickable
		icon image.Image
		what string
	}{
		{noBtn, assets.IconBack, "NO (declines; changes nothing)"},
		{yesBtn, assets.IconCheckmark, "YES (loads the pattern, discarding all three fields)"},
	} {
		var found *NavButton
		for i := range nav {
			if nav[i].Clickable == tc.clk {
				found = &nav[i]
			}
		}
		if found == nil {
			t.Fatalf("%s is not among the prompt's answers", tc.what)
		}
		if found.Icon != tc.icon {
			t.Errorf("%s carries the wrong icon. The operator reads the glyph, not the "+
				"tag, so swapping these inverts the prompt on screen while every "+
				"tap-by-tag test keeps passing", tc.what)
		}
	}
}
