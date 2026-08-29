package gui

import (
	"strings"
	"testing"

	"seedhammer.com/backup"
	"seedhammer.com/bspline"
	"seedhammer.com/engrave"
	"seedhammer.com/font/sh"
)

// TestBundlePlanVerbatim: the engrave plan is the cards' gathered strings, in
// card-then-plate order, UNMODIFIED (I-4). Every plate string equals exactly one
// gathered chunk string — no re-encode, no transform.
//
// THE SUM INVARIANT MOVED WITH F-423. It used to be len(plan) == the number of
// gathered strings; a plate now holds as many as fit, so the invariant is on the
// STRINGS: concatenating every plate's strs in plan order reproduces the cards'
// strings in gather order, exactly once each. That is the property the old
// count was a proxy for, and it survives packing.
func TestBundlePlanVerbatim(t *testing.T) {
	g := &bundleGatherer{}
	offerAll(t, g, md1CardA(t))
	offerAll(t, g, mk1CardA(t))
	cards := g.cards

	plan := bundlePlatePlan(engraverParams, cards)

	// Every gathered string appears once, in order, and nothing else does.
	var want, got []string
	for _, c := range cards {
		want = append(want, c.strings...)
	}
	for _, p := range plan {
		got = append(got, p.strs...)
	}
	if !equalStringSlice(got, want) {
		t.Fatalf("the plan does not carry the gathered strings verbatim in order:\ngot  %q\nwant %q", got, want)
	}
	// And it PACKED: this fixture is 8 strings, and the plan is shorter. Without
	// this the test above passes over a plan that never packs anything.
	if len(plan) >= len(want) {
		t.Fatalf("plan has %d plates for %d strings; F-423 packing did nothing", len(plan), len(want))
	}

	// The card/plate indices are 1-based and contiguous per card, and plateTotal
	// counts the card's PLATES rather than its strings.
	perCard := bundleCardPlateCounts(plan, len(cards))
	k := 0
	for ci := range cards {
		for pi := 0; pi < perCard[ci]; pi++ {
			p := plan[k]
			if p.cardIdx != ci+1 || p.cardTotal != len(cards) {
				t.Fatalf("plate %d card progress = %d of %d, want %d of %d", k, p.cardIdx, p.cardTotal, ci+1, len(cards))
			}
			if p.plateIdx != pi+1 || p.plateTotal != perCard[ci] {
				t.Fatalf("plate %d plate progress = %d of %d, want %d of %d", k, p.plateIdx, p.plateTotal, pi+1, perCard[ci])
			}
			if len(p.strs) == 0 {
				t.Fatalf("plate %d carries no string at all", k)
			}
			k++
		}
	}
	if k != len(plan) {
		t.Fatalf("walked %d plates of %d; the per-card counts do not tile the plan", k, len(plan))
	}
}

// TestBundlePlanSingleMD1OnePlate: a standalone md1 card → exactly 1 plate.
func TestBundlePlanSingleMD1OnePlate(t *testing.T) {
	g := &bundleGatherer{}
	if st := g.offer(mdmkText(singleMD1(t))); st != bundleAddedSingleMD1 {
		t.Fatalf("single md1 not added: %v", st)
	}
	plan := bundlePlatePlan(engraverParams, g.cards)
	if len(plan) != 1 {
		t.Fatalf("single md1 → %d plates, want 1", len(plan))
	}
	if plan[0].plateTotal != 1 {
		t.Fatalf("single md1 plateTotal = %d, want 1", plan[0].plateTotal)
	}
	if len(plan[0].strs) != 1 || plan[0].strs[0] != singleMD1(t) {
		t.Fatalf("single md1 plate carries %q, want the one gathered string", plan[0].strs)
	}
}

// TestBundlePlanPacksACardOntoFewerPlates is F-423's ARITHMETIC, pinned.
//
// The operator's words were "1 plate per string is something to be addressed,
// it's wasteful", and the case they named is the bare single-sig md1: two
// strings of ~85 characters, cut as two plates. It is one plate now, and so is
// three; the numbers below are the packer's own answers at the SHIPPED font,
// measured, not the analytic bound (MEASURE-S2-P4-1 confirmed N=3 by trial and
// stopped there because that was its directed scope).
//
// bundlePlateMD1Capacity is asserted as a literal AND recomputed, so a layout
// change that moves the boundary fails here rather than silently re-planning
// every plate in the field.
func TestBundlePlanPacksACardOntoFewerPlates(t *testing.T) {
	chunk := md1CardA(t)[0]
	if len(chunk) != 85 {
		t.Fatalf("the fixture chunk is %d chars, not the 85 these numbers were measured at", len(chunk))
	}
	for _, tc := range []struct {
		strings int
		plates  int
		what    string
	}{
		{1, 1, "a one-string card is one plate, exactly as before"},
		{2, 1, "the bare single-sig card F-423 names: two strings, ONE plate"},
		{3, 1, "three strings, one plate (MEASURE-S2-P4-1's trial ceiling)"},
		{bundlePlateMD1Capacity, 1, "the last count that fits one plate"},
		{bundlePlateMD1Capacity + 1, 2, "one more than fits splits, and splits ONCE"},
		{2*bundlePlateMD1Capacity + 1, 3, "and it keeps filling greedily"},
	} {
		strs := make([]string, tc.strings)
		for i := range strs {
			strs[i] = chunk
		}
		cards := []bundleCard{{kind: cardMD1, label: "md1 policy", strings: strs}}
		plan := bundlePlatePlan(engraverParams, cards)
		if len(plan) != tc.plates {
			t.Errorf("%s: %d strings → %d plates, want %d", tc.what, tc.strings, len(plan), tc.plates)
		}
	}
	// The literal is not the definition: recompute it from the packer, so the
	// table above cannot go stale while still reading as measured truth.
	strs := make([]string, 64)
	for i := range strs {
		strs[i] = chunk
	}
	if got := len(bundleCardPlates(engraverParams, strs)[0]); got != bundlePlateMD1Capacity {
		t.Errorf("the packer fits %d %d-char strings on a plate, not the %d pinned here; "+
			"the plate layout moved and every planned plate moved with it",
			got, len(chunk), bundlePlateMD1Capacity)
	}
}

// bundlePlateMD1Capacity is how many 85-character md1 chunk strings fit one
// plate side at the shipped font, packed against the worst-case marking.
// Measured by TestBundlePlanPacksACardOntoFewerPlates, which also re-derives it.
const bundlePlateMD1Capacity = 5

// TestBundlePlanNeverPacksAcrossCards is the M5 ruling as a test.
//
// Two one-string cards would fit ONE plate between them, and must not share
// one: cardIdx/cardTotal drive the "Card X of Y" guidance, label drives the
// abort warning and kind drives bundlePlateMark, so a shared plate has no true
// value for any of them -- and it is the one route by which a secret cardMS1
// string could land on a plate that carries a wallet's fingerprint.
func TestBundlePlanNeverPacksAcrossCards(t *testing.T) {
	a, b := md1CardA(t)[0], md1CardA(t)[1]
	cards := []bundleCard{
		{kind: cardMD1, label: "md1 policy", strings: []string{a}},
		{kind: cardMS1, label: "ms1 secret share", strings: []string{b}},
	}
	// Non-vacuous: the two strings DO fit one plate, so only the card boundary
	// can be what keeps them apart.
	if !bundlePlateTextFits(engraverParams, []string{a, b}) {
		t.Fatal("the two fixture strings do not fit one plate, so this test cannot " +
			"tell a card boundary from a fit refusal")
	}
	plan := bundlePlatePlan(engraverParams, cards)
	if len(plan) != 2 {
		t.Fatalf("two one-string cards → %d plates, want 2 (packing crossed a card)", len(plan))
	}
	for i, p := range plan {
		if len(p.strs) != 1 || p.strs[0] != cards[i].strings[0] {
			t.Errorf("plate %d carries %q; a plate may only hold its own card's strings", i, p.strs)
		}
		if p.kind != cards[i].kind || p.label != cards[i].label {
			t.Errorf("plate %d is labelled %q/%v but holds card %d's string", i, p.label, p.kind, i)
		}
	}
}

// TestBundlePlanPlatesClearTheFooterRow is the check toPlate cannot make.
//
// EngraveText gives the body no budget against the footer: a body drawn over
// the footer row is still inside the safety margin, so the bounds check reports
// a fit (backup.TestAPackedBodyCanCoverTheFooterRow measures exactly that). Every
// plate the planner emits is therefore checked HERE against the row, at the
// worst-case marking every plate is packed against -- because the marking is
// resolved after the census has already committed to a number.
func TestBundlePlanPlatesClearTheFooterRow(t *testing.T) {
	chunk := md1CardA(t)[0]
	strs := make([]string, 17)
	for i := range strs {
		strs[i] = chunk
	}
	cards := []bundleCard{{kind: cardMD1, label: "md1 policy", strings: strs}}
	plan := bundlePlatePlan(engraverParams, cards)
	if len(plan) < 2 {
		t.Fatalf("the fixture packed onto %d plate(s); this test needs a split", len(plan))
	}
	for i, p := range plan {
		plate := backup.Text{
			Paragraphs: bundlePlateParagraphs(p.strs),
			Font:       sh.Font,
			Title:      bundlePlateFitMark,
			Footer:     bundlePlateFitMark,
		}
		body := plate
		body.Footer = ""
		ink := bspline.Measure(engrave.PlanEngraving(engraverParams.StepperConfig,
			backup.EngraveText(engraverParams, body))).Bounds
		if row := plate.FooterRow(engraverParams); ink.Max.Y > row {
			t.Errorf("plate %d (%d strings): the body ends at %d, past the footer row %d — "+
				"the marking would be cut through the last line of the backup",
				i, len(p.strs), ink.Max.Y, row)
		}
	}
}

// TestBundlePlateFitMarkIsTheWorstCase: the marking the planner packs against
// must be at least as wide as any marking a plate can actually carry, or the
// plan's fit claim is a claim about a smaller plate than the one that is cut.
func TestBundlePlateFitMarkIsTheWorstCase(t *testing.T) {
	if len(bundlePlateFitMark) != backup.MaxTitleLen {
		t.Errorf("the trial marking is %d chars and backup.MaxTitleLen is %d",
			len(bundlePlateFitMark), backup.MaxTitleLen)
	}
	for _, r := range bundlePlateFitMark {
		if r != 'W' {
			t.Fatalf("the trial marking contains %q; it is meant to be the font's widest glyph", r)
		}
	}
	// And the real single-sig marking is no wider, on either row.
	title, footer := singleSigPlateMark(true, true, 0xfc60c6df)
	if len(title) > backup.MaxTitleLen || len(footer) > backup.MaxTitleLen {
		t.Errorf("singleSigPlateMark returns title %q (%d) / footer %q (%d), wider than the "+
			"%d the planner packs against", title, len(title), footer, len(footer), backup.MaxTitleLen)
	}
}

// TestBundleEngraveGuidedTitles: confirming a 2-card bundle drives "Card 1 of 2
// | Plate 1 of N" first (and the card-progress is shown). Mirrors
// TestMultiPlateEngravePlateTitles: assert the first guided title appears.
//
// The separator is "|", not "·" (F-78 — bundle_flow.go:339's
// "Card %d of %d · Plate %d of %d" renders in ctx.Styles.title, poppins.Bold25,
// where "·" contributes zero pixels).
func TestBundleEngraveGuidedTitles(t *testing.T) {
	g := &bundleGatherer{}
	offerAll(t, g, mk1CardA(t)) // first card: an mk1 (>=2 plates)
	offerAll(t, g, md1CardA(t)) // second card
	cards := g.cards
	if len(cards) != 2 {
		t.Fatalf("setup: %d cards, want 2", len(cards))
	}

	ctx := NewContext(newPlatform())
	frame, quit := runUI(ctx, func() { bundleEngrave(ctx, &descriptorTheme, "Engrave Bundle", cards, "", "") })
	defer quit()
	c, ok := pumpUntil(frame, "Card 1 of 2", 48)
	if !ok {
		t.Fatalf("guided 'Card 1 of 2' title not shown; got %q", c)
	}
	if !uiContains(c, "Card 1 of 2 | Plate 1 of") {
		t.Errorf("guided title missing the \"|\" separator; got %q", c)
	}
	if strings.Contains(c, "·") {
		t.Errorf("guided title still contains the invisible middot separator: %q", c)
	}
}

// TestBundleEngraveSetAbort: backing out of the first plate's variant picker
// surfaces the SET-LEVEL abort warning (partial bundle unusable) and records no
// completed state (I-5).
//
// IT LAYS OUT AT sh2DisplaySize, and that changed at S5 for a reason worth
// recording. This test used newPlatform()'s 240x240 default, which is "a fiction
// that no shipped device has" (gui_test.go's own words at sh2DisplaySize). When
// S5 rewrote the abort text -- DESTROY instead of discard, plus the re-run
// recovery -- the longer body was DRAWN IN FULL on the real 480x320 panel and
// CUT MID-SENTENCE on the 240x240 fiction, at "...so you only cut the ones you
// are". The failure was therefore a report about a display nobody owns, arriving
// on a screen whose whole subject is reachability. Asserting on the real panel is
// what makes it a report about the machine; the F-185 class check
// (gui/modal_fits_test.go) gates the same body's length there with margin.
func TestBundleEngraveSetAbort(t *testing.T) {
	g := &bundleGatherer{}
	offerAll(t, g, mk1CardA(t))
	offerAll(t, g, md1CardA(t))
	cards := g.cards

	p := newPlatform()
	p.display = sh2DisplaySize
	ctx := NewContext(p)
	frame, quit := runUI(ctx, func() { bundleEngrave(ctx, &descriptorTheme, "Engrave Bundle", cards, "", "") })
	defer quit()
	if c, ok := pumpUntil(frame, "Card 1 of 2", 48); !ok {
		t.Fatalf("guided title not shown; got %q", c)
	}
	// Back out of the variant picker → the set-level abort warning.
	click(&ctx.Router, Button1)
	if c, ok := pumpUntil(frame, "not a usable backup", 32); !ok {
		t.Fatalf("set-level abort warning not shown; got %q", c)
	}
}

// TestBundleEngraveMs1Reminder: the ms1 reminder text exists and instructs the
// operator to hand-engrave the ms1 share(s) (mirror host bundle.rs:296-306).
func TestBundleEngraveMs1Reminder(t *testing.T) {
	msg := bundleMs1ReminderText()
	low := strings.ToLower(msg)
	if !strings.Contains(low, "ms1") || !strings.Contains(low, "hand") {
		t.Fatalf("ms1 reminder %q must mention hand-engraving the ms1 share(s)", msg)
	}
}

// TestBundlePlanValidatesEachPlate: every plate the plan emits lays out to at
// least one engravable plate via the call bundleEngrave itself makes, so the
// guided engrave never silently produces an empty plate set for a verified card.
//
// IT ALSO PINS WHICH VARIANTS A PACKED PLATE OFFERS. A one-string plate offers
// all three, unchanged. A packed plate offers TEXT ONLY and nothing else: on a
// multi-paragraph plate EngraveText draws each paragraph's code across the
// paragraphs after it, and centers a text-less paragraph's code on the plate so
// they stack -- and both of those lay out INSIDE the plate, so toPlate would
// report a fit for ink no reader could scan.
func TestBundlePlanValidatesEachPlate(t *testing.T) {
	g := &bundleGatherer{}
	offerAll(t, g, mk1CardA(t))
	offerAll(t, g, md1CardA(t))
	pl := newPlatform()
	packed := 0
	for _, p := range bundlePlatePlan(engraverParams, g.cards) {
		labels, plates, err := validateMdmkStrings(pl, p.strs, "", "")
		if err != nil || len(plates) == 0 || len(labels) == 0 {
			t.Fatalf("plate %q does not fit any engraving variant: err=%v plates=%d", p.strs, err, len(plates))
		}
		if len(labels) != len(plates) {
			t.Fatalf("plate %q offers %d labels for %d plates", p.strs, len(labels), len(plates))
		}
		if len(p.strs) == 1 {
			if !equalStringSlice(labels, []string{"TEXT + QR", "TEXT ONLY", "QR ONLY"}) {
				t.Errorf("a one-string plate offers %q; it used to offer all three and must still", labels)
			}
			continue
		}
		packed++
		if !equalStringSlice(labels, []string{"TEXT ONLY"}) {
			t.Errorf("a packed plate of %d strings offers %q; a code on a packed plate is "+
				"drawn over its neighbours", len(p.strs), labels)
		}
	}
	if packed == 0 {
		t.Fatal("no plate in this plan holds more than one string, so the packed arm was never checked")
	}
}
