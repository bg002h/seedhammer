package gui

import (
	"strings"
	"testing"

	"seedhammer.com/gui/widget"
)

// ─── S5: the words an operator reads before putting a seed on steel ──────────

// TestExperimentalWarningStopsTeachingAnUncheckableCheck.
//
// The shipped warning told the operator to verify "the shown policy stub +
// per-slot fingerprints against your coordinator/wallet BEFORE funding". That is
// a check that CANNOT CHECK, and the flow's own code says why:
//
//   - fingerprints are OMITTED by default. The Include/Omit question defaults to
//     Omit, so on the default path the screen the warning points at shows
//     "(no fingerprint)" for every slot and there is nothing to compare.
//   - when they ARE present they are card-self-declared and unbound to the key
//     (mk/mk.go:136,286). cosignerFromCard reads Fingerprint straight off the
//     card string; nothing derives it from the xpub. So an attacker who swaps a
//     key writes a matching fingerprint beside it for free, and the operator's
//     comparison passes on a forged card.
//
// A warning that teaches a ritual which cannot fail is worse than one that says
// nothing: it spends the operator's attention and returns a false negative. So
// the warning must demand a comparison that can actually fail -- the KEYS or the
// DESCRIPTOR against an independent source -- say plainly that a matching
// fingerprint is not that comparison, and name the real backstop.
func TestExperimentalWarningStopsTeachingAnUncheckableCheck(t *testing.T) {
	body := multisigBuildExperimentalWarningBody()
	low := strings.ToLower(body)

	// (1) The uncheckable instruction is GONE. Asserted on the shipped phrasing
	// rather than on the word "fingerprint", which the new body still uses -- to
	// say the opposite.
	for _, gone := range []string{
		"per-slot fingerprints against your",
		"verify the assembled descriptor and the shown policy stub",
	} {
		if strings.Contains(low, strings.ToLower(gone)) {
			t.Errorf("the warning still tells the operator to check %q, which is a check "+
				"that cannot fail: fingerprints are omitted by default and self-declared "+
				"when present.\n%s", gone, body)
		}
	}

	// (2) It demands a comparison the operator CAN perform, against something
	// this device did not author.
	for _, want := range []string{"keys", "descriptor", "coordinator"} {
		if !strings.Contains(low, want) {
			t.Errorf("the warning never names %q, so it does not ask for a comparison "+
				"against an independent source:\n%s", want, body)
		}
	}

	// (3) It says PLAINLY that a matching fingerprint is not verification. The
	// operator has to be told the old ritual is void, or they will keep doing it.
	if !strings.Contains(low, "fingerprint is not") {
		t.Errorf("the warning never says that a matching fingerprint is not the check, "+
			"so an operator who learned that ritual keeps performing it:\n%s", body)
	}

	// (4) It names the real backstop: restoring the engraved plates in a
	// coordinator and seeing the wallet's own first address. That is the one
	// check that exercises the whole chain, and it is what the hardware-validation
	// stage will formalise; the operator needs it named now, not then.
	for _, want := range []string{"restor", "first receive address"} {
		if !strings.Contains(low, want) {
			t.Errorf("the warning never names the real backstop (%q):\n%s", want, body)
		}
	}

	// (5) It still says the operator must act BEFORE funding. That was right in
	// the shipped text and a rewrite must not lose it.
	if !strings.Contains(low, "fund") {
		t.Errorf("the warning no longer names funding as the deadline:\n%s", body)
	}

	// (6) Hold-to-confirm is still the only route past it, and the screen still
	// says so.
	if !strings.Contains(low, "hold button to confirm") {
		t.Errorf("the warning lost its hold-to-confirm instruction:\n%s", body)
	}

	// (7) It draws. No em-dash and no smart punctuation: measured 2026-08-15, a
	// body line carrying one rasters at 2652 px against 7419 for the same text
	// with a hyphen, i.e. the line does not draw at all.
	if strings.ContainsAny(body, "—–·‘’“”…") {
		t.Errorf("the warning carries a glyph the body face lacks, so its line does "+
			"not draw:\n%q", body)
	}

	// (8) F-185: and it draws IN FULL, with room to grow. This is the screen
	// F-185's defect would hurt most -- the operator's last chance to stop.
	assertModalBodyFits(t, "the EXPERIMENTAL warning", confirmWarningBody, body)
}

// TestBuildReviewShowsTheKeysItIsAskingToConfirm.
//
// The shipped review screen printed "@0 fp xxxxxxxx" or "@0 (no fp)" and NOTHING
// ELSE about what each slot holds. So the last screen before an irreversible
// engrave asked the operator to confirm a wallet policy whose contents they had
// never seen -- and on the DEFAULT path (fingerprints omitted) the per-slot lines
// carried no information at all beyond the slot count.
//
// That is what makes the EXPERIMENTAL warning's "compare against your
// coordinator" possible to obey: there has to be something ON the device to
// compare. The keys are shown in full, base58, exactly as an mk1 card and a
// coordinator spell them, because a comparison against a truncated or re-encoded
// form is a comparison the operator cannot complete.
func TestBuildReviewShowsTheKeysItIsAskingToConfirm(t *testing.T) {
	p, _, _, self, cards := s5TraceB(t)
	assembled, stub, slots, err := assembleBuildPolicy(p, self, cards)
	if err != nil {
		t.Fatalf("Trace B did not assemble: %v", err)
	}

	keyStrs, err := buildSlotKeyStrings(assembled, self, cards)
	if err != nil {
		t.Fatalf("buildSlotKeyStrings: %v", err)
	}
	if len(keyStrs) != p.N {
		t.Fatalf("got %d slot keys, want %d", len(keyStrs), p.N)
	}

	// EVERY key the operator supplied is on the screen, IN FULL. Asserted against
	// the inputs rather than against the mapper's own output, so a mapper that
	// returned the same key four times could not pass.
	want := make([]string, 0, p.N)
	for _, h := range self {
		want = append(want, h.Xpub)
	}
	for _, c := range cards {
		want = append(want, c.Xpub)
	}
	lines := buildReviewLines(p.Script, stub, slots, keyStrs, p.IncludeFp, nil)
	joined := strings.Join(lines, "")
	for i, x := range want {
		if len(x) < 100 {
			t.Fatalf("INCONCLUSIVE: input key %d is %d chars, not a base58 xpub", i, len(x))
		}
		if !strings.Contains(joined, x) {
			t.Errorf("the policy review never shows the key %s.\nThe operator is asked to "+
				"confirm a wallet whose contents this screen does not state:\n%s",
				x, strings.Join(lines, "\n"))
		}
	}

	// And each key is against ITS OWN SLOT, not merely present somewhere. A screen
	// that lists four keys in the wrong order describes a different wallet.
	for slot, x := range keyStrs {
		mark := "@" + string(rune('0'+slot))
		idxMark := strings.Index(joined, mark)
		idxKey := strings.Index(joined, x)
		if idxMark < 0 || idxKey < 0 {
			t.Fatalf("slot %d: mark or key missing from the review", slot)
		}
		if idxKey < idxMark {
			t.Errorf("slot %d's key is printed BEFORE its own @N label, so the screen does "+
				"not say which slot holds it:\n%s", slot, strings.Join(lines, "\n"))
		}
	}

	// The instruction that makes the keys useful, and it is PREPENDED: this screen
	// is confirmable from any page, so anything appended can be committed past
	// unread. Same reason the plate census puts its note first.
	if !strings.Contains(strings.ToLower(lines[0]), "check") {
		t.Errorf("the review's FIRST line does not tell the operator to check the keys; "+
			"an instruction below the fold on a screen confirmable from page one is an "+
			"instruction that does not exist:\n%q", lines[0])
	}
}

// TestBuildReviewKeysReachThePixels is the same claim about the DRAWN screen.
//
// Every other assertion in this file reads the STRINGS buildReviewLines returns.
// F-179 and F-185 are both cases where a string that was submitted was not
// drawn, so the claim "the operator can see the keys" has to be settled on the
// frame. The review PAGES rather than scrolls, so this pages the way an operator
// would and asserts the keys appear across the pages actually rendered.
func TestBuildReviewKeysReachThePixels(t *testing.T) {
	p, _, _, self, cards := s5TraceB(t)
	assembled, stub, slots, err := assembleBuildPolicy(p, self, cards)
	if err != nil {
		t.Fatalf("Trace B did not assemble: %v", err)
	}
	keyStrs, err := buildSlotKeyStrings(assembled, self, cards)
	if err != nil {
		t.Fatalf("buildSlotKeyStrings: %v", err)
	}

	pl := newPlatform()
	pl.display = sh2DisplaySize
	ctx := NewContext(pl)
	frame, _, ink, quit := runUITouchRaster(ctx, func() {
		buildReviewFlow(ctx, &descriptorTheme, p.Script, stub, slots, keyStrs, p.IncludeFp, nil)
	})
	defer quit()

	// Page with Button2 until the pager wraps back to the first page. Every frame
	// is held to the raster floor, because a blank page carries no key however
	// many text ops it submitted.
	//
	// THE TITLE IS REMOVED FROM EACH PAGE BEFORE THE PAGES ARE JOINED, and the
	// reason is a real property of this screen rather than test convenience: a
	// 111-character xpub is six body lines and a page holds about nine, so keys
	// STRADDLE PAGE BREAKS as a matter of course. confirmReviewScreen redraws
	// "Policy Review" on every frame, so a naive concatenation splices the title
	// into the middle of a key that the operator reads continuously across the
	// page turn. Measured on the first run of this test: @0 came back as
	// "xpub6DkFAXWQ2dHxq2va" + "PolicyReview" + "trt9qy...". Stripping exactly the
	// title, and nothing else, keeps the assertion about the KEY.
	title := normalizeDrawn("Policy Review")
	var seen strings.Builder
	pages := 0
	addPage := func(content string) {
		pages++
		seen.WriteString(strings.ReplaceAll(normalizeDrawn(content), title, ""))
	}
	first, ok := frame()
	if !ok {
		t.Fatal("the policy review never rendered a frame")
	}
	addPage(first)
	t.Logf("review page 1 ink = %d px", ink())
	if ink() < buildWalkRasterFloor {
		t.Errorf("review page 1 drew only %d ink pixels (floor %d)", ink(), buildWalkRasterFloor)
	}
	seenRaw := first
	for page := 2; page <= 12; page++ {
		click(&ctx.Router, Button2)
		content, ok := frame()
		if !ok {
			break
		}
		if strings.Contains(seenRaw, content) {
			break // wrapped to a page already seen
		}
		seenRaw += content
		addPage(content)
		t.Logf("review page %d ink = %d px", page, ink())
		if ink() < buildWalkRasterFloor {
			t.Errorf("review page %d drew only %d ink pixels (floor %d)",
				page, ink(), buildWalkRasterFloor)
		}
	}
	if pages < 2 {
		t.Fatalf("INCONCLUSIVE: the review rendered %d page(s). A four-slot policy is "+
			"more than one screenful, so this test never paged and proves nothing about "+
			"keys below the first page", pages)
	}

	drawn := seen.String()
	for slot, x := range keyStrs {
		if !strings.Contains(drawn, normalizeDrawn(x)) {
			t.Errorf("slot @%d's key never reaches the glass. It is in the strings and "+
				"not in the pixels, which is the F-179/F-185 seam and the reason this "+
				"test rasterises:\nwant %s\ndrawn %q", slot, x, drawn)
		}
	}
	// The prepended instruction is on the FIRST page, where it governs a screen
	// that can be confirmed without ever paging.
	if !strings.Contains(normalizeDrawn(first), normalizeDrawn("Check each key below")) {
		t.Errorf("the review's first PAGE does not carry the check-the-keys instruction; "+
			"Button3 confirms from page one, so an instruction on page two is one the "+
			"operator can commit past unread:\n%q", first)
	}
}

// TestBuildSlotKeyStringsRefusesAnUnmappableSlot.
//
// The mapper reads the ASSEMBLED md1 and matches each slot back to a key string
// the operator holds, so the screen shows what the bytes carry rather than what
// the flow meant to put there. A slot it cannot map is a slot the review would
// have to leave blank, and a blank slot on the one screen that states the
// wallet's contents is the defect this whole item exists to remove. So it is an
// error, and the flow refuses rather than confirming a policy it cannot display.
func TestBuildSlotKeyStringsRefusesAnUnmappableSlot(t *testing.T) {
	p, _, _, self, cards := s5TraceB(t)
	assembled, _, _, err := assembleBuildPolicy(p, self, cards)
	if err != nil {
		t.Fatalf("Trace B did not assemble: %v", err)
	}
	if _, err := buildSlotKeyStrings(assembled, self, nil); err == nil {
		t.Error("buildSlotKeyStrings mapped every slot with the cosigner card withheld, " +
			"so a slot whose key it cannot name would render blank instead of refusing")
	}
	// The honest direction: with every input present it succeeds. Without this,
	// the assertion above passes for a function that always errors.
	if _, err := buildSlotKeyStrings(assembled, self, cards); err != nil {
		t.Errorf("buildSlotKeyStrings failed on complete inputs: %v", err)
	}
}

// ─── The backup must say what is NOT in it ───────────────────────────────────
//
// A BIP-39 passphrase is a REQUIRED SPENDING FACTOR and it is NEVER ENGRAVED.
// ms1 encodes the WORDS; the passphrase is not in the entropy and cannot be
// recovered from any plate in the set. Before S5 neither the engrave-mode label
// ("Full (seed + keys)") nor the restore document mentioned it -- measured, the
// restore file contained zero occurrences of "passphrase".
//
// F-132's device sibling exactly. That finding was a hashlock preimage required
// to spend, absent from the backup and unmentioned by it; here the missing factor
// is one the operator typed on this device ten minutes earlier. The restore doc
// is read years later, alone, often by someone who was not the operator, and a
// backup labelled "Full" that cannot reach the money is the worst thing a backup
// can be: wrong AND trusted.

// TestFullModeLabelSaysThePassphraseIsNotInIt.
func TestFullModeLabelSaysThePassphraseIsNotInIt(t *testing.T) {
	plain := buildFullModeLabel(false)
	pass := buildFullModeLabel(true)

	// Without a passphrase, the shipped label is correct and must not change: a
	// screen that warns about a factor nobody used is a screen whose warnings get
	// ignored (§0.1's corollary).
	if plain != "Full (seed + keys)" {
		t.Errorf("the no-passphrase label changed to %q; nothing about that build is "+
			"incomplete, so there is nothing to say", plain)
	}

	// With one, the label itself says the backup is short of a spending factor.
	// The LABEL, not a note beside it: this is a two-row picker and the row is
	// what the operator reads before pressing.
	if pass == plain {
		t.Error("a build that used a passphrase offers the same \"Full\" label as one " +
			"that did not, so the operator is told a partial backup is a complete one")
	}
	if !strings.Contains(strings.ToLower(pass), "passphrase") {
		t.Errorf("the label never names the passphrase: %q", pass)
	}
	if !strings.Contains(pass, "NOT") {
		t.Errorf("the label names the passphrase without saying it is EXCLUDED, which "+
			"reads as though it were included: %q", pass)
	}
	// It has to fit its row. ChoiceScreen uses widget.Label, which does NOT wrap,
	// so an over-wide label is drawn off the panel rather than onto a second line.
	assertChoiceLabelFits(t, pass)
	assertChoiceLabelFits(t, plain)
}

// TestRestoreDocSaysThePassphraseIsNotOnThePlates.
//
// The restore document is the artifact that outlives everybody. It already says
// how many plates the set is and what each of them is; the one thing it could not
// say was that a plate is missing which was never a plate at all.
func TestRestoreDocSaysThePassphraseIsNotOnThePlates(t *testing.T) {
	cards := []bundleCard{
		{kind: cardMS1, label: "ms1 share", summary: "seed", strings: []string{"ms1"}},
		{kind: cardMK1, label: "mk1 key", summary: "key", strings: []string{"mk1"}},
	}
	with := strings.Join(buildPlateInventoryLines(cards, true), "\n")
	without := strings.Join(buildPlateInventoryLines(cards, false), "\n")

	lowWith := strings.ToLower(with)
	if !strings.Contains(lowWith, "passphrase") {
		t.Errorf("the restore doc for a passphrase build never mentions it:\n%s", with)
	}
	// It must say the passphrase is NOT here and CANNOT be got from here. "You
	// also need a passphrase" alone leaves the reader hunting the plates for it.
	for _, want := range []string{"not on", "cannot"} {
		if !strings.Contains(lowWith, want) {
			t.Errorf("the restore doc does not say the passphrase is %q recoverable "+
				"from these plates:\n%s", want, with)
		}
	}
	// And it says what the consequence is, in the terms that matter.
	if !strings.Contains(lowWith, "without it") {
		t.Errorf("the restore doc never says what happens without the passphrase:\n%s", with)
	}

	// A build with NO passphrase says so too, and this half is deliberate rather
	// than symmetry for its own sake: the reader in five years is holding a pile of
	// steel and asking "is this everything?". "No passphrase was used" ANSWERS that
	// question; silence leaves them to wonder whether the operator had one and
	// forgot to write it down.
	lowWithout := strings.ToLower(without)
	if !strings.Contains(lowWithout, "passphrase") {
		t.Errorf("the restore doc for a plain build leaves the passphrase question "+
			"unanswered, so a reader cannot tell whether the set is complete:\n%s", without)
	}
	if strings.Contains(lowWithout, "not on these plates") {
		t.Errorf("the plain build's restore doc warns about a passphrase nobody used:\n%s",
			without)
	}
	if with == without {
		t.Error("the passphrase and no-passphrase restore docs are identical")
	}

	// The inventory it already carried is still there. A new paragraph must not
	// have displaced the plate count.
	for _, want := range []string{"2 plates", "ms1 share", "mk1 key"} {
		if !strings.Contains(with, want) {
			t.Errorf("the restore doc lost %q from its inventory:\n%s", want, with)
		}
	}
}

// TestSeedRegistryReportsPassphraseUse: the flow asks the registry, and the
// registry answers from the pairs it actually holds.
//
// SPEC 4.1 makes (seed, passphrase) the derivation unit and the prompt PER SEED,
// so "was a passphrase used" is a question about the SET of registered pairs, not
// a flag the flow can keep on the side. One passphrased seed among three is
// enough to make the backup incomplete.
func TestSeedRegistryReportsPassphraseUse(t *testing.T) {
	reg, ids := s5Registry(t, fixtureMasterA, fixtureMasterB)
	if reg.usesPassphrase() {
		t.Error("a registry of two bare seeds reports a passphrase in use")
	}
	if err := reg.bindPassphrase(ids[1], "correct horse", s5Net); err != nil {
		t.Fatalf("bindPassphrase: %v", err)
	}
	if !reg.usesPassphrase() {
		t.Error("a registry with one passphrased seed reports none; one incomplete " +
			"leg is enough to make the whole backup incomplete")
	}
	// An empty passphrase bound explicitly is still no passphrase. The picker's
	// Skip arm never calls bindPassphrase, but syswPassphraseFlowTitled can return
	// ("", true), and a build that engraves a plain seed must not be labelled as
	// though a factor were missing.
	if err := reg.bindPassphrase(ids[1], "", s5Net); err != nil {
		t.Fatalf("bindPassphrase(empty): %v", err)
	}
	if reg.usesPassphrase() {
		t.Error("an explicitly-bound EMPTY passphrase counts as a passphrase, which " +
			"would label a complete backup incomplete")
	}
}

// assertChoiceLabelFits: ChoiceScreen draws its rows with widget.Label, which
// does not wrap. A row wider than the panel is drawn off the edge, so the
// operator reads a truncated option and chooses it anyway. This is F-185's
// horizontal twin and is measured the same way: against the real face, at the
// real display size, not counted in characters.
func assertChoiceLabelFits(t *testing.T, label string) {
	t.Helper()
	p := newPlatform()
	p.display = sh2DisplaySize
	ctx := NewContext(p)
	_, sz := widget.Label(&ctx.B, ctx.Styles.button, descriptorTheme.Text, label)
	budget := sh2DisplaySize.X - 2*16 - 2*buttonPadX // ChoiceScreen.Draw's own shrink + pad
	t.Logf("choice label %q draws %d px against a %d px row", label, sz.X, budget)
	if sz.X > budget {
		t.Errorf("the choice label %q draws %d px wide on a %d px row, so it is cut "+
			"off the panel: widget.Label does not wrap", label, sz.X, budget)
	}
}
