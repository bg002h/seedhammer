package gui

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"image"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"seedhammer.com/codex32"
	"seedhammer.com/gui/assets"
	"seedhammer.com/gui/op"
	"seedhammer.com/gui/widget"
	"seedhammer.com/hashlock"
	"seedhammer.com/md"
)

// ─── H6 Task 9: the Done review (spec §5.3, §5.4, §8.3, §8.4, §10.1, §10.3) ───

// composerH6Material is one held digest built from a phrase, so every fixture's
// digest is a real SHA-256 of a real preimage rather than a byte pattern -- the
// plate builder encodes it as an ms1 kind-0x03 string and a pattern would not
// round-trip.
func composerH6Material(phrase string, held bool) ([32]byte, hashlockMaterial) {
	x := hashlock.PreimageSHA256([]byte(phrase))
	h := hashlock.Digest(&x)
	m := hashlockMaterial{preimage: x, method: hashlockSHA256, provenance: hashlockFromPhrase}
	if held {
		m.phrase = []byte(phrase)
	} else {
		m.provenance = hashlockFromPayload
	}
	return h, m
}

// composerH6PlateState is the smallest composition §5.3 applies to: a wsh
// policy whose first path is keyed (md refuses a wholly key-less compose) and
// whose remaining paths are key-less hashed ones, each digest HELD.
func composerH6PlateState(t *testing.T, phrases ...string) *composerState {
	t.Helper()
	st := &composerState{reg: &seedRegistry{}, list: md.PathList{Wrapper: md.ComposeWsh}}
	st.list.Paths = append(st.list.Paths, md.SpendPath{Keys: &md.KeySet{K: 2, N: 2}})
	for _, p := range phrases {
		h, m := composerH6Material(p, true)
		hh := h
		st.list.Paths = append(st.list.Paths, md.SpendPath{Hash: &hh})
		composerHoldHashlockMaterial(st, h, m)
	}
	// The shape's slots, sized exactly as composerFlow sizes them before it
	// composes -- without this composerDeclaredOrigins hands md an empty
	// declaration slice for a two-slot policy.
	composerSizeAssignments(st)
	return st
}

// TestComposerPreimagePlatesAreOrderedByPathNotByMap is the property a census
// read straight out of hashlockHeld cannot have.
//
// MUTATION: range over st.hashlockHeld and append -> Go randomises map
// iteration, so the same composition lists its plates in a different order on
// different runs and this test fails on one of the 32 attempts below.
func TestComposerPreimagePlatesAreOrderedByPathNotByMap(t *testing.T) {
	st := composerH6PlateState(t, "anchor a", "anchor b", "anchor c")
	// A fourth digest on NO path: retained, listed, never cut (§5.3 item 6).
	hd, md4 := composerH6Material("anchor d", true)
	composerHoldHashlockMaterial(st, hd, md4)

	want := []string{}
	for _, p := range composerPreimagePlates(st) {
		want = append(want, hashlockDigestHex(p.digest))
	}
	if len(want) != 4 {
		t.Fatalf("%d plates, want 4", len(want))
	}
	for i := 0; i < 32; i++ {
		var got []string
		for _, p := range composerPreimagePlates(st) {
			got = append(got, hashlockDigestHex(p.digest))
		}
		for j := range got {
			if got[j] != want[j] {
				t.Fatalf("attempt %d: plate %d is %s, want %s -- the order is map order",
					i, j, got[j][:8], want[j][:8])
			}
		}
	}
	plates := composerPreimagePlates(st)
	for i := 0; i < 3; i++ {
		if plates[i].path != i+2 {
			t.Errorf("plate %d reports path %d, want %d", i, plates[i].path, i+2)
		}
	}
	if plates[3].path != 0 {
		t.Errorf("the digest no path carries reports path %d, want 0", plates[3].path)
	}
}

// TestComposerPreimagePlatesCountOneSharedDigestOnce: the plate carries the
// PREIMAGE, not the path, so two paths on one digest is ONE plate. Cutting it
// twice would double the bearer plates for one secret.
func TestComposerPreimagePlatesCountOneSharedDigestOnce(t *testing.T) {
	st := composerH6PlateState(t, "anchor a")
	shared := st.list.Paths[1].Hash
	st.list.Paths = append(st.list.Paths, md.SpendPath{Hash: shared})
	if got := len(composerPreimagePlates(st)); got != 1 {
		t.Fatalf("%d plates for one digest on two paths, want 1", got)
	}
}

// TestComposerPreimagePlateRowsOfferTheFourLabels is §5.3 step (A)'s row set,
// BY LABEL, and the QR rows' condition.
//
// MUTATION: offer the phrase rows with no phrase held -> the payload-preimage
// row below fails, and the plate builder would then be asked to engrave an
// empty phrase.
func TestComposerPreimagePlateRowsOfferTheFourLabels(t *testing.T) {
	_, held := composerH6Material("anchor a", true)
	rows, choices := composerPreimagePlateRows(held)
	want := []string{"preimage string", "phrase + method", "phrase + method + QR", "do not cut this preimage"}
	if len(rows) != len(want) {
		t.Fatalf("a held phrase offers %d rows, want %d: %v", len(rows), len(want), rows)
	}
	for i := range want {
		if rows[i] != want[i] {
			t.Errorf("row %d is %q, want %q", i, rows[i], want[i])
		}
		assertChoiceLabelFits(t, rows[i])
	}
	if choices[len(choices)-1] != hashlockPlateDecline {
		t.Error("the last row is not the decline")
	}

	_, noPhrase := composerH6Material("anchor a", false)
	rows2, choices2 := composerPreimagePlateRows(noPhrase)
	if len(rows2) != 2 || rows2[0] != "preimage string" || rows2[1] != "do not cut this preimage" {
		t.Fatalf("a payload preimage record offers %v; the two phrase forms have no phrase to engrave", rows2)
	}
	for _, c := range choices2 {
		if c == hashlockPlatePhrase || c == hashlockPlatePhraseQR {
			t.Error("a phrase form was offered for material holding no phrase")
		}
	}
}

// TestComposerPreimagePlateRowsDrawOnOneLine measures step (A)'s rows in
// composerPageLines' own band -- the ONE measure site.
func TestComposerPreimagePlateRowsDrawOnOneLine(t *testing.T) {
	p := newPlatform()
	p.display = sh2DisplaySize
	ctx := NewContext(p)
	dims := sh2DisplaySize
	_, width := composerTextBand(dims)
	one := composerRowSize(ctx, width, "X").Y
	if one <= 0 {
		t.Fatalf("a one-line body row measured %d px", one)
	}
	_, held := composerH6Material("anchor a", true)
	rows, _ := composerPreimagePlateRows(held)
	for _, row := range rows {
		sz := composerRowSize(ctx, width, row)
		lines := (sz.Y + one - 1) / one
		t.Logf("%-26q %2d chars %3d px %d line(s)", row, len(row), sz.X, lines)
		if lines != 1 {
			t.Errorf("%q wraps to %d lines in the %d px band", row, lines, width)
		}
	}
}

// TestComposerPreimagePlatePickIsMaskedAndNeverRevealsThePhrase is §5.3 item 7.
//
// MUTATION: put the phrase itself in the lead -> the last assertion fails. There
// is no reveal control to add: the shipped affordance is a KEY ON A KEYBOARD
// GRID that LATCHES (gui/passphrase_keyboard.go:141,221), and a pick screen has
// no key grid.
func TestComposerPreimagePlatePickIsMaskedAndNeverRevealsThePhrase(t *testing.T) {
	const phrase = "correct horse battery staple"
	st := composerH6PlateState(t, phrase)
	h := *st.list.Paths[1].Hash
	synctest.Test(t, func(t *testing.T) {
		p := newPlatform()
		p.display = sh2DisplaySize
		ctx := NewContext(p)
		frame, quit := runUI(ctx, func() { composerPreimagePlatePick(ctx, &descriptorTheme, st, h) })
		defer quit()
		content, ok := pumpUntil(frame, "do not cut this preimage", 16)
		if !ok {
			t.Fatalf("the pick screen never drew.\nLast frame: %q", content)
		}
		if !uiContains(content, fmt.Sprintf("phrase: %d characters", len(phrase))) {
			t.Errorf("the pick screen does not state the phrase's LENGTH.\nFrame: %q", content)
		}
		if !uiContains(content, "method: sha256") {
			t.Errorf("the pick screen does not state the method.\nFrame: %q", content)
		}
		if uiContains(content, phrase) {
			t.Errorf("the pick screen REVEALED the phrase.\nFrame: %q", content)
		}
		if !uiContains(content, hashlockFirst8Last8(h)) {
			t.Errorf("the pick screen does not carry the digest it is deciding about.\nFrame: %q", content)
		}
	})
}

// TestComposerPreimagePlatePickBackDeclinesTheHighlightedPlate is §5.3's Back
// contract: Button1 is `do not cut` for this plate, matching
// composerPickScreen's shipped decline arm.
func TestComposerPreimagePlatePickBackDeclinesTheHighlightedPlate(t *testing.T) {
	st := composerH6PlateState(t, "anchor a")
	h := *st.list.Paths[1].Hash
	synctest.Test(t, func(t *testing.T) {
		p := newPlatform()
		p.display = sh2DisplaySize
		ctx := NewContext(p)
		got := hashlockPlateString
		frame, quit := runUI(ctx, func() { got = composerPreimagePlatePick(ctx, &descriptorTheme, st, h) })
		defer quit()
		if _, ok := pumpUntil(frame, "do not cut this preimage", 16); !ok {
			t.Fatal("the pick screen never drew")
		}
		click(&ctx.Router, Button1)
		for i := 0; i < 8; i++ {
			if _, ok := frame(); !ok {
				break
			}
		}
		if got != hashlockPlateDecline {
			t.Errorf("Back returned %v, want the decline", got)
		}
	})
}

// TestComposerPreimageQRWarningFiresOnTheQRRowAndCanBeDeclined is §8.5.
//
// MUTATION: take the QR row without the warning -> the warning assertion fails.
// MUTATION: treat a declined warning as an acceptance -> the second half fails,
// and the operator would get a QR they explicitly refused.
func TestComposerPreimageQRWarningFiresOnTheQRRowAndCanBeDeclined(t *testing.T) {
	st := composerH6PlateState(t, "anchor a")
	h := *st.list.Paths[1].Hash
	synctest.Test(t, func(t *testing.T) {
		p := newPlatform()
		p.display = sh2DisplaySize
		ctx := NewContext(p)
		got := hashlockPlateString
		frame, drawer, quit := runUITouch(ctx, func() {
			got = composerPreimagePlatePick(ctx, &descriptorTheme, st, h)
		})
		defer quit()
		content, ok := pumpUntil(frame, "do not cut this preimage", 16)
		if !ok {
			t.Fatalf("the pick screen never drew.\nLast frame: %q", content)
		}
		pts := plateHitPoints(ctx, drawer())
		if len(pts) != 4 {
			t.Fatalf("the pick screen drew %d rows, want 4", len(pts))
		}
		tap(&ctx.Router, drawer(), pts[2]) // phrase + method + QR
		frame()
		click(&ctx.Router, Button3)
		content, ok = pumpUntil(frame, "readable by any camera", 16)
		if !ok {
			t.Fatalf("§8.5's QR warning never drew.\nLast frame: %q", content)
		}
		// DECLINED -> back to the rows with nothing decided.
		click(&ctx.Router, Button1)
		content, ok = pumpUntil(frame, "do not cut this preimage", 16)
		if !ok {
			t.Fatalf("declining the QR warning did not return to the rows.\nLast frame: %q", content)
		}
		if got != hashlockPlateString {
			t.Errorf("the pick returned %v after the warning was declined", got)
		}
	})
}

// TestComposerCensusReportsEveryDecisionAndCutsNothingItself is §8.3, over the
// four states one review can hold at once.
//
// MUTATION: drop the declined plate silently -> the declined row is absent and
// this fails. MUTATION: list an unused preimage as a plate -> the accepted count
// is 2 and the heading fails.
func TestComposerCensusReportsEveryDecisionAndCutsNothingItself(t *testing.T) {
	st := composerH6PlateState(t, "anchor a", "anchor b")
	hd, m := composerH6Material("anchor d", true)
	composerHoldHashlockMaterial(st, hd, m)
	plates := composerPreimagePlates(st)
	plates[0].choice = hashlockPlatePhraseQR
	plates[1].choice = hashlockPlateDecline
	cards := []bundleCard{{kind: cardMD1, label: "md1 template", strings: []string{"md1abc"}, summary: "key-less wallet policy"}}
	joined := strings.Join(composerCensusLines(newPlatform().EngraverParams(), cards, plates), "\n")

	for _, want := range []string{
		"Plus 1 preimage plate(s), cut first and NOT part of this backup:",
		"path 2  " + hashlockFirst8Last8(plates[0].digest) + "  phrase, sha256, QR",
		"preimage " + hashlockFirst8Last8(plates[1].digest) + ": declined, will not be cut",
		"preimage " + hashlockFirst8Last8(hd) + ": not on any path, will not be cut",
		"Keep each preimage plate apart from the policy plates and from the others.",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("the census does not carry §8.3's row %q:\n%s", want, joined)
		}
	}
	// buildPlateCensusLines' own count and claim are BYTE-UNCHANGED: a preimage
	// plate is not a bundleCard and does not enter `plan`.
	base := strings.Join(composerCensusLines(newPlatform().EngraverParams(), cards, nil), "\n")
	if !strings.Contains(base, "This engraves") || !strings.Contains(joined, "This engraves") {
		t.Fatal("the shipped plate count left the census")
	}
	for _, line := range strings.Split(base, "\n") {
		if !strings.Contains(joined, line) {
			t.Errorf("adding preimage plates changed the shipped census line %q", line)
		}
	}
	if strings.Contains(joined, "a set is only a backup when all of it exists.\nPlus") {
		t.Error("the preimage block was spliced into the set-completeness claim")
	}
}

// TestComposerCensusDrawsTheStandAloneNoticeForAnUnusedPreimage is §8.3's
// notice form: a review whose only preimage entry is one no path carries gets
// the sentence that says what to DO, not the terse row.
func TestComposerCensusDrawsTheStandAloneNoticeForAnUnusedPreimage(t *testing.T) {
	st := &composerState{reg: &seedRegistry{}, list: md.PathList{Wrapper: md.ComposeWsh,
		Paths: []md.SpendPath{{Keys: &md.KeySet{K: 2, N: 2}}}}}
	h, m := composerH6Material("anchor a", true)
	composerHoldHashlockMaterial(st, h, m)
	plates := composerPreimagePlates(st)
	if len(plates) != 1 || plates[0].path != 0 {
		t.Fatalf("the fixture is not one unused preimage: %v", plates)
	}
	joined := strings.Join(composerCensusLines(newPlatform().EngraverParams(), nil, plates), "\n")
	if !strings.Contains(joined, composerCopyPreimageOnlyNotice()) {
		t.Errorf("the stand-alone notice is not drawn:\n%s", joined)
	}
	if strings.Contains(joined, "will not be cut, ") {
		t.Errorf("the terse row was drawn as well as the notice:\n%s", joined)
	}
}

// TestComposerCensusRowsAreDrawnInsideTheBand is Step 2's geometry gate, and
// the reason composerEngraveStep no longer uses confirmReviewScreen.
//
// MUTATION: draw the census through confirmReviewScreen's own wrap
// (`dims.X - 2*8`, centred on the whole panel) -> the rows measured below land
// under the navigation column, which is W-3 verbatim.
func TestComposerCensusRowsAreDrawnInsideTheBand(t *testing.T) {
	p := newPlatform()
	p.display = sh2DisplaySize
	ctx := NewContext(p)
	dims := sh2DisplaySize

	st := composerH6PlateState(t, "anchor a", "anchor b")
	hd, m := composerH6Material("anchor d", true)
	composerHoldHashlockMaterial(st, hd, m)
	plates := composerPreimagePlates(st)
	plates[0].choice = hashlockPlatePhraseQR
	plates[1].choice = hashlockPlateDecline
	cards := []bundleCard{{kind: cardMD1, label: "md1 template", strings: []string{"md1abc"}, summary: "key-less wallet policy"}}
	lines := composerCensusLines(ctx.Platform.EngraverParams(), cards, plates)

	// The two wraps, side by side, because the DIFFERENCE is the finding.
	// confirmReviewScreen wraps at dims.X - 2*8 and centres on the WHOLE panel
	// while the navigation column starts at dims.X - NavBtnPrimary.width, so a
	// panel-centred row is under a button as soon as it is wider than
	// 2*colLeft - dims.X. Every number below is measured, not transcribed.
	_, width := composerTextBand(dims)
	colLeft := dims.X - assets.NavBtnPrimary.Bounds().Size().X
	threshold := 2*colLeft - dims.X
	t.Logf("panel %d px; nav column at %d px; confirmReviewScreen wraps at %d px; "+
		"a panel-centred row is under the column above %d px; composerPageLines band %d px",
		dims.X, colLeft, dims.X-2*8, threshold, width)
	overs := 0
	for _, l := range lines {
		if l == "" {
			continue
		}
		wide := composerRowSize(ctx, dims.X-2*8, l)
		inBand := composerRowSize(ctx, width, l)
		over := ""
		if wide.X > threshold {
			over = "  OVER"
			overs++
		}
		t.Logf("at the 464 px wrap %3d px (%d line(s)) | in the band %3d px%s  %q",
			wide.X, (wide.Y+22)/23, inBand.X, over, l)
	}
	if overs == 0 {
		t.Error("no census row is wide enough to be drawn under the column at " +
			"confirmReviewScreen's wrap, so this test proves nothing about why the " +
			"census moved to composerPageLines")
	}
	if bad := whichLineIntersects(t, ctx, dims, lines); len(bad) > 0 {
		t.Errorf("these census rows are drawn under a navigation button: %q", bad)
	}
	// And every page of it, not only the first.
	for start := 0; start < len(lines); {
		_, shown, _ := composerPageLines(ctx, &descriptorTheme, dims, lines, start, -1)
		if shown == 0 {
			t.Fatalf("composerPageLines drew no row from index %d", start)
		}
		if nav, at, hit := inkUnderNav(t, ctx, dims, lines, start); hit {
			t.Errorf("page from row %d draws under button %v at %v", start, nav, at)
		}
		start += shown
	}
}

// TestComposerPreimagePlateIsNotABundleCard is Step 8's fourth row.
//
// MUTATION: add the preimage plate as a bundleCard -> bundlePlatePlan's count
// changes and the "a set is only a backup when all of it exists" claim starts
// counting a plate that deliberately does not travel with the set.
func TestComposerPreimagePlateIsNotABundleCard(t *testing.T) {
	cards := []bundleCard{{kind: cardMD1, label: "md1 template", strings: []string{"md1abc"}, summary: "key-less wallet policy"}}
	params := newPlatform().EngraverParams()
	before := buildPlateCensusLines(params, cards)
	st := composerH6PlateState(t, "anchor a")
	plates := composerPreimagePlates(st)
	plates[0].choice = hashlockPlateString
	after := buildPlateCensusLines(params, cards)
	if len(before) != len(after) {
		t.Fatalf("buildPlateCensusLines is not a pure function of the cards")
	}
	for i := range before {
		if before[i] != after[i] {
			t.Errorf("line %d changed: %q -> %q", i, before[i], after[i])
		}
	}
	full := composerCensusLines(params, cards, plates)
	for _, l := range before {
		if !containsLine(full, l) {
			t.Errorf("the shipped census line %q left the census", l)
		}
	}
}

func containsLine(lines []string, want string) bool {
	for _, l := range lines {
		if l == want {
			return true
		}
	}
	return false
}

// TestComposerPreimageMarkTitleMarksMd1AndMk1ButNeverMs1 is §10.3.
//
// WITHOUT THE mk1 ASSERTION THIS ROW WOULD PROVE NOTHING about §10.3's
// sentence: every other assertion here passes whether or not the key cards are
// marked.
//
// MUTATION: mark unconditionally -> the unhashed row fails.
// MUTATION: suppress the mark on cardMK1 -> the key-card row fails.
func TestComposerPreimageMarkTitleMarksMd1AndMk1ButNeverMs1(t *testing.T) {
	unhashed := &composerState{list: md.PathList{Wrapper: md.ComposeWsh,
		Paths: []md.SpendPath{{Keys: &md.KeySet{K: 2, N: 2}}}}}
	if got := composerPreimageMarkTitle(unhashed); got != "" {
		t.Errorf("an unhashed composition marks its plates %q", got)
	}
	hashed := composerH6PlateState(t, "anchor a")
	title := composerPreimageMarkTitle(hashed)
	if title != "PREIMAGE REQUIRED" {
		t.Fatalf("markTitle = %q", title)
	}
	if len(title) > 18 {
		t.Errorf("%q is %d characters against MaxTitleLen = 18", title, len(title))
	}
	for _, tc := range []struct {
		kind bundleCardKind
		want string
	}{
		{cardMD1, "PREIMAGE REQUIRED"},
		{cardMK1, "PREIMAGE REQUIRED"},
		{cardMS1, ""},
	} {
		got, _ := bundlePlateMark(tc.kind, title, "")
		if got != tc.want {
			t.Errorf("bundlePlateMark(%v) = %q, want %q", tc.kind, got, tc.want)
		}
	}
}

// TestComposerHashEveryPathArmsAreInThePresentTenseOfWhatIsHeld is §10.1.
//
// MUTATION: restore the future-tense wording ("this run cuts a plate for each
// one") -> the DECLINE row asserts a body that is false about what the run did.
// The banner is drawn at composerFlow's line 75 and the decision is taken at
// line 125.
// MUTATION: choose the fourth arm on "at least one" phrase rather than every ->
// the mixed row fails, and the body's "for each one" is false.
func TestComposerHashEveryPathArmsAreInThePresentTenseOfWhatIsHeld(t *testing.T) {
	// Nothing held: the two shipped arms are unchanged.
	plain := &composerState{list: md.PathList{Wrapper: md.ComposeWsh}}
	var d [32]byte
	d[0] = 0x11
	plain.list.Paths = []md.SpendPath{{Hash: &d}}
	if got := composerCopyHashEveryPathFor(plain); got != composerCopyHashEveryPath() {
		t.Errorf("a composition holding nothing does not draw the shipped plain arm:\n%q", got)
	}
	composerNotePhraseDigest(plain, d)
	if got := composerCopyHashEveryPathFor(plain); got != composerCopyHashEveryPathPhrase() {
		t.Errorf("a phrase-set digest with no material held does not draw the shipped phrase arm:\n%q", got)
	}

	// Every hashed path held, and every one of them a phrase.
	all := &composerState{list: md.PathList{Wrapper: md.ComposeWsh}}
	for _, p := range []string{"anchor a", "anchor b"} {
		h, m := composerH6Material(p, true)
		hh := h
		all.list.Paths = append(all.list.Paths, md.SpendPath{Hash: &hh})
		composerHoldHashlockMaterial(all, h, m)
	}
	if got := composerCopyHashEveryPathFor(all); got != composerCopyHashEveryPathHeldPhrase() {
		t.Errorf("every hashed path holding a phrase does not draw §10.1's fourth arm:\n%q", got)
	}

	// MIXED: one path's material is a payload preimage record with no phrase.
	mixed := &composerState{list: md.PathList{Wrapper: md.ComposeWsh}}
	h1, m1 := composerH6Material("anchor a", true)
	h2, m2 := composerH6Material("anchor b", false)
	hh1, hh2 := h1, h2
	mixed.list.Paths = []md.SpendPath{{Hash: &hh1}, {Hash: &hh2}}
	composerHoldHashlockMaterial(mixed, h1, m1)
	composerHoldHashlockMaterial(mixed, h2, m2)
	if got := composerCopyHashEveryPathFor(mixed); got != composerCopyHashEveryPathHeld() {
		t.Errorf("a mixed composition claims to hold the phrase for each path:\n%q", got)
	}

	// One hashed path held and one NOT: the held arms may not fire at all.
	partial := &composerState{list: md.PathList{Wrapper: md.ComposeWsh}}
	h3, m3 := composerH6Material("anchor c", true)
	hh3 := h3
	var other [32]byte
	other[0] = 0x22
	partial.list.Paths = []md.SpendPath{{Hash: &hh3}, {Hash: &other}}
	composerHoldHashlockMaterial(partial, h3, m3)
	got := composerCopyHashEveryPathFor(partial)
	if got == composerCopyHashEveryPathHeld() || got == composerCopyHashEveryPathHeldPhrase() {
		t.Errorf("a composition holding material for only one hashed path claims to hold all:\n%q", got)
	}

	// NEITHER new body claims a plate WAS cut.
	for _, b := range []string{composerCopyHashEveryPathHeld(), composerCopyHashEveryPathHeldPhrase()} {
		for _, banned := range []string{"this run cuts", "was cut", "will cut"} {
			if strings.Contains(strings.ToLower(b), banned) {
				t.Errorf("§10.1's body promises a cut the operator may still decline: %q in %q", banned, b)
			}
		}
		if !strings.Contains(b, "can cut a plate") {
			t.Errorf("§10.1's body does not state the possibility in the present tense: %q", b)
		}
	}
}

// TestComposerBuildHashlockPlateCarriesTheLocatorAndTheRightForm is §6.3 and
// §6.2 at the composer's own call site.
//
// MUTATION: omit the locator's `hash` row -> the non-empty assertion fails,
// which is the row standing between the operator and a bearer plate with no
// locator at all.
// MUTATION: put the ms1 string in QRText -> the QR text assertion fails; §6.4
// decision 1 forbids QR-encoding the plate string.
func TestComposerBuildHashlockPlateCarriesTheLocatorAndTheRightForm(t *testing.T) {
	const phrase = "correct horse battery staple"
	st := composerH6PlateState(t, phrase)
	template, err := composerTemplateChunksFor(st)
	if err != nil {
		t.Fatalf("composerTemplateChunksFor: %v", err)
	}
	var keyed []string
	plates := composerPreimagePlates(st)
	loc := composerHashlockLocator(plates[0], template, keyed)
	t.Logf("locator: %q", loc)
	if len(loc) < 2 {
		t.Fatalf("the locator is %q; §6.3 requires at least the path and the hash", loc)
	}
	if loc[0] != "path 2" {
		t.Errorf("the locator's first row is %q, want the path", loc[0])
	}
	foundHash := false
	for _, r := range loc {
		if strings.HasPrefix(r, "hash  ") {
			foundHash = true
			if !strings.Contains(r, hashlockFirst8Last8(plates[0].digest)) {
				t.Errorf("the locator's hash row is %q", r)
			}
		}
	}
	if !foundHash {
		t.Error("the locator has no `hash` row: a phrase-form plate with no digest, no path " +
			"and no payload position is the worst artifact this stage can cut")
	}

	for _, tc := range []struct {
		choice   hashlockPlateChoice
		wantForm int
		qr       bool
	}{
		{hashlockPlateString, 0, false},
		{hashlockPlatePhrase, 1, false},
		{hashlockPlatePhraseQR, 1, true},
	} {
		p := plates[0]
		p.choice = tc.choice
		desc, err := composerBuildHashlockPlate(p, loc)
		if err != nil {
			t.Fatalf("choice %v: %v", tc.choice, err)
		}
		if int(desc.Form) != tc.wantForm {
			t.Errorf("choice %v: form %v", tc.choice, desc.Form)
		}
		if desc.QR != tc.qr {
			t.Errorf("choice %v: QR = %v", tc.choice, desc.QR)
		}
		switch tc.wantForm {
		case 0:
			if desc.MS1 == "" || desc.Phrase != "" {
				t.Errorf("the string form carries phrase %q and ms1 %q", desc.Phrase, desc.MS1)
			}
			// The ms1 string is a PLATE string: kind 0x03 under the id `hash`.
			if c32, err := codex32.New(desc.MS1); err != nil || !codex32.IsPreimagePlate(c32) {
				t.Errorf("the string form's ms1 %q is not a preimage plate string", desc.MS1)
			}
		case 1:
			if desc.Phrase != phrase {
				t.Errorf("the phrase form carries %q", desc.Phrase)
			}
			if desc.Method != hashlock.MethodLine(false) {
				t.Errorf("the phrase form's method line is %q", desc.Method)
			}
			if desc.MS1 != "" {
				t.Errorf("the phrase form carries the ms1 string %q", desc.MS1)
			}
			if tc.qr && desc.QRText != hashlock.QRText(false, phrase) {
				t.Errorf("the QR text is %q", desc.QRText)
			}
			if tc.qr && strings.Contains(desc.QRText, "ms10hash") {
				t.Error("the QR encodes the ms1 plate string, which §6.4 decision 1 forbids")
			}
		}
	}
	// A declined plate has no form to build, and asking for one is a defect
	// rather than a silent empty plate.
	p := plates[0]
	p.choice = hashlockPlateDecline
	if _, err := composerBuildHashlockPlate(p, loc); err == nil {
		t.Error("a declined plate built a backup.Hashlock")
	}
}

// TestComposerPreimagePlateFitsTheRealPlate drives the whole builder into
// engrave.Params, which is what refuses an over-large plate.
func TestComposerPreimagePlateFitsTheRealPlate(t *testing.T) {
	st := composerH6PlateState(t, strings.Repeat("a", hashlock.PhraseMaxChars))
	template, err := composerTemplateChunksFor(st)
	if err != nil {
		t.Fatal(err)
	}
	var keyed []string
	plates := composerPreimagePlates(st)
	loc := composerHashlockLocator(plates[0], template, keyed)
	for _, c := range []hashlockPlateChoice{hashlockPlateString, hashlockPlatePhrase, hashlockPlatePhraseQR} {
		p := plates[0]
		p.choice = c
		pl, err := composerHashlockPlateFor(newPlatform(), p, loc)
		if err != nil {
			t.Errorf("the worst-case %v plate does not fit: %v", c, err)
			continue
		}
		t.Logf("choice %v: %d locator rows, duration %d", c, len(loc), pl.Duration)
	}
}

// composerRowSize measures one row exactly as composerPageLines lays it out:
// the same style, the same band width. A test that re-derived the layout would
// pass on its own arithmetic rather than on the screen's.
func composerRowSize(ctx *Context, width int, s string) image.Point {
	_, sz := widget.Labelw(&ctx.B, ctx.Styles.body, width, descriptorTheme.Text, s)
	return sz
}

// runComposerEngraveStep drives §7f's whole Done step, which is where H6 §5.3,
// §5.4, §8.3, §8.4 and §10.3 all land.
func runComposerEngraveStep(t *testing.T, st *composerState, template []string, ret *bool) (
	func() (string, bool), func() *op.Drawer, *Context) {
	t.Helper()
	p := newEngravedAwarePlatform()
	p.engraver = newEngraver()
	p.display = sh2DisplaySize
	ctx := NewContext(p)
	frame, drawer, quit := runUITouch(ctx, func() {
		*ret = composerEngraveStep(ctx, &descriptorTheme, st, template, nil)
	})
	t.Cleanup(quit)
	return frame, drawer, ctx
}

// TestComposerPreimagePlatePickDrawsAllFourRows is the measurement the masked
// lead makes necessary: composerPickScreen draws the lead as a per-page header
// through composerPageLines, so every line it spends is a ROW lost from page 1.
//
// MUTATION: restore a four-line lead (the digest, the path, the character
// count and the method each on their own line) -> three rows draw and
// `do not cut this preimage` -- the row an operator reaches for to UNDO -- is
// on page 2.
func TestComposerPreimagePlatePickDrawsAllFourRows(t *testing.T) {
	st := composerH6PlateState(t, "correct horse battery staple")
	h := *st.list.Paths[1].Hash
	synctest.Test(t, func(t *testing.T) {
		p := newPlatform()
		p.display = sh2DisplaySize
		ctx := NewContext(p)
		frame, drawer, quit := runUITouch(ctx, func() {
			composerPreimagePlatePick(ctx, &descriptorTheme, st, h)
		})
		defer quit()
		content, ok := pumpUntil(frame, "do not cut this preimage", 16)
		if !ok {
			t.Fatalf("the pick screen never drew.\nLast frame: %q", content)
		}
		pts := plateHitPoints(ctx, drawer())
		t.Logf("the pick screen draws %d tappable rows on page 1", len(pts))
		if len(pts) != 4 {
			t.Errorf("page 1 draws %d of the four rows; the lead is spending them", len(pts))
		}
	})
}

// TestComposerCutsPreimagePlatesBeforeThePolicySet is §5.4's ORDER, and §8.4a.
//
// Nothing at the screen layer can see the order -- both runs draw the same
// shapes -- so the flow records each plate as it hands it to the engraver.
//
// MUTATION: move the preimage loop AFTER bundleEngrave -> the first recorded
// plate is "policy" and the order assertion fails; and aborting the first
// engrave then draws bundleEngrave's set-level copy instead of §8.4a.
// MUTATION: fire §8.4b here (a plate was cut) -> the body assertion fails.
func TestComposerCutsPreimagePlatesBeforeThePolicySet(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		st := composerH6PlateState(t, "anchor a")
		template, err := composerTemplateChunksFor(st)
		if err != nil {
			t.Fatal(err)
		}
		var order []string
		composerPlateCutHook = func(kind string) { order = append(order, kind) }
		t.Cleanup(func() { composerPlateCutHook = nil })

		var ret bool
		frame, drawer, ctx := runComposerEngraveStep(t, st, template, &ret)
		content, ok := pumpUntil(frame, "No slot is seated", 24)
		if !ok {
			t.Fatalf("the form choice never drew.\nLast frame: %q", content)
		}
		click(&ctx.Router, Button3)
		content, ok = pumpUntil(frame, "do not cut this preimage", 24)
		if !ok {
			t.Fatalf("step (A) never drew.\nLast frame: %q", content)
		}
		pts := plateHitPoints(ctx, drawer())
		tap(&ctx.Router, drawer(), pts[0]) // preimage string
		frame()
		click(&ctx.Router, Button3)
		content, ok = pumpUntil(frame, "Plates To Cut", 24)
		if !ok {
			t.Fatalf("the census never drew.\nLast frame: %q", content)
		}
		// THE PLATE IS NOT A bundleCard: the shipped count is still 1, the md1
		// template, even though a preimage plate is about to be cut first.
		// MUTATION: append the accepted plates to `cards` -> this reads
		// "This engraves 2 plates." and the set-completeness claim starts
		// counting a plate that deliberately does not travel with the set.
		if !uiContains(content, "This engraves 1 plate.") {
			t.Errorf("the shipped plate count changed when a preimage plate was accepted.\nFrame: %q", content)
		}
		// §8.3's block is on a later page: composerReadScreen pages, and the
		// checkmark is withheld until the last page has been laid out once.
		content, ok = composerPageUntil(t, ctx, frame, "Plus 1 preimage plate", 12)
		if !ok {
			t.Fatalf("the census never drew §8.3's block.\nLast frame: %q", content)
		}
		if !uiContains(content, "cut first and NOT part of this backup") {
			t.Errorf("§8.3's heading does not say the plate is not part of the backup.\nFrame: %q", content)
		}
		composerPageToEnd(t, ctx, frame) // continue past the census
		content, ok = pumpUntil(frame, "Engrave Plate", 32)
		if !ok {
			t.Fatalf("the first engrave screen never drew.\nLast frame: %q", content)
		}
		// THE ORDER, at the moment only the preimage plate has been handed over.
		if len(order) != 1 || order[0] != "preimage" {
			t.Fatalf("the first plate handed to the engraver is %v, want [preimage]: "+
				"the order is what removes the window in which the md1 plates exist "+
				"and the preimage does not", order)
		}
		// §8.4a: abort with NOTHING cut.
		click(&ctx.Router, Button1)
		content, ok = pumpUntil(frame, "NO PREIMAGE PLATE WAS CUT", 32)
		if !ok {
			t.Fatalf("§8.4a never drew.\nLast frame: %q", content)
		}
		if uiContains(content, "A PREIMAGE PLATE WAS CUT") {
			t.Error("§8.4b fired on a run that cut nothing")
		}
		// "dies with this composition", NOT "is now gone": composerFlow loops
		// back with the state intact, so the phrase is still held.
		if !uiContains(content, "dies with this composition") {
			t.Errorf("§8.4a does not say the phrase dies with the COMPOSITION.\nFrame: %q", content)
		}
		if _, held := st.hashlockHeld[*st.list.Paths[1].Hash]; !held {
			t.Error("the abort dropped the held material, which would make §8.4a's own wording false")
		}
		click(&ctx.Router, Button3)
		for i := 0; i < 8; i++ {
			if _, ok := frame(); !ok {
				break
			}
		}
		if ret {
			t.Error("composerEngraveStep reported the run finished after §8.4a")
		}
		if containsLine(order, "policy") {
			t.Errorf("the policy set was handed to the engraver after §8.4a: %v", order)
		}
	})
}

// TestComposerDeclinedPlateCutsNothingAndDoesNotAbortTheRun is §5.3's decline
// arm: declining does NOT abort, because a preimage plate is not part of the
// policy set.
//
// MUTATION: treat a decline as an abort -> the census never draws.
func TestComposerDeclinedPlateCutsNothingAndDoesNotAbortTheRun(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		st := composerH6PlateState(t, "anchor a")
		template, err := composerTemplateChunksFor(st)
		if err != nil {
			t.Fatal(err)
		}
		var order []string
		composerPlateCutHook = func(kind string) { order = append(order, kind) }
		t.Cleanup(func() { composerPlateCutHook = nil })

		var ret bool
		frame, drawer, ctx := runComposerEngraveStep(t, st, template, &ret)
		if c, ok := pumpUntil(frame, "No slot is seated", 24); !ok {
			t.Fatalf("the form choice never drew.\nLast frame: %q", c)
		}
		click(&ctx.Router, Button3)
		if c, ok := pumpUntil(frame, "do not cut this preimage", 24); !ok {
			t.Fatalf("step (A) never drew.\nLast frame: %q", c)
		}
		pts := plateHitPoints(ctx, drawer())
		tap(&ctx.Router, drawer(), pts[3]) // do not cut this preimage
		frame()
		click(&ctx.Router, Button3)
		content, ok := pumpUntil(frame, "Plates To Cut", 24)
		if !ok {
			t.Fatalf("the census never drew.\nLast frame: %q", content)
		}
		content, ok = composerPageUntil(t, ctx, frame, "declined, will not be cut", 12)
		if !ok {
			t.Fatalf("the census does not NAME the declined plate.\nLast frame: %q", content)
		}
		if uiContains(content, "Plus 1 preimage plate") {
			t.Errorf("a declined plate is still counted as one to cut.\nFrame: %q", content)
		}
		composerPageToEnd(t, ctx, frame)
		if c, ok := pumpUntil(frame, "Choose engraving", 32); !ok {
			t.Fatalf("the policy set was not reached after a decline.\nLast frame: %q", c)
		}
		if len(order) != 1 || order[0] != "policy" {
			t.Errorf("a declined plate was still handed to the engraver: %v", order)
		}
	})
}

// composerEngraveOnePlate is engraveOnePlate with a bound this stage's plates
// need. MEASURED: one `preimage string` plate is a 9:02 cut and closes the
// engraver after 12,802 frames, well past the shipped helper's 4,096 -- which
// fails with "the engrave never closed the engraver, so no plate was cut" and
// would read as a defect in the flow rather than in the harness.
func composerEngraveOnePlate(t *testing.T, ctx *Context, frame func() (string, bool), e *testEngraver) {
	t.Helper()
	click(&ctx.Router, Button3, Button3, Button3)
	press(&ctx.Router, Button3)
	frame()
	time.Sleep(confirmDelay)
	for i := 0; i < 1<<17; i++ {
		select {
		case <-e.closes:
			click(&ctx.Router, Button3)
			frame()
			return
		default:
		}
		if _, ok := frame(); !ok {
			return
		}
	}
	t.Fatal("the engrave never closed the engraver, so no plate was cut")
}

// TestComposerAbortAfterAPreimagePlateWasCutDrawsSection84b is §8.4b -- the
// window the ORDER creates, and the one whose danger is specific.
//
// bundleEngrave's set-level copy tells the operator a partial bundle cannot be
// used, whose natural response is to run the composition again; §5.3 sets no
// cap, so the second run cuts a SECOND bearer plate for the same secret, one of
// which the operator has no record of -- while §8.3's own line tells them to
// store them apart from each other.
//
// MUTATION: attach either arm to bundleAbortWarningText -> it is reachable only
// from bundleAbortWarning at gui/bundle_flow.go:625 and :640, both INSIDE
// bundleEngrave, and §5.4 cuts every accepted preimage plate BEFORE that, so
// the clause could never fire and this test fails.
// MUTATION: fire §8.4a here -> the "NO PREIMAGE PLATE WAS CUT" assertion fails,
// and the operator with a bearer plate on the bench is told none exists.
func TestComposerAbortAfterAPreimagePlateWasCutDrawsSection84b(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		st := composerH6PlateState(t, "anchor a")
		template, err := composerTemplateChunksFor(st)
		if err != nil {
			t.Fatal(err)
		}
		var order []string
		composerPlateCutHook = func(kind string) { order = append(order, kind) }
		t.Cleanup(func() { composerPlateCutHook = nil })

		e := newEngraver()
		p := newEngravedAwarePlatform()
		p.engraver = e
		p.display = sh2DisplaySize
		ctx := NewContext(p)
		var ret bool
		frame, drawer, quit := runUITouch(ctx, func() {
			ret = composerEngraveStep(ctx, &descriptorTheme, st, template, nil)
		})
		defer quit()

		if c, ok := pumpUntil(frame, "No slot is seated", 24); !ok {
			t.Fatalf("the form choice never drew.\nLast frame: %q", c)
		}
		click(&ctx.Router, Button3)
		if c, ok := pumpUntil(frame, "do not cut this preimage", 24); !ok {
			t.Fatalf("step (A) never drew.\nLast frame: %q", c)
		}
		pts := plateHitPoints(ctx, drawer())
		tap(&ctx.Router, drawer(), pts[0]) // preimage string
		frame()
		click(&ctx.Router, Button3)
		if c, ok := pumpUntil(frame, "Plates To Cut", 24); !ok {
			t.Fatalf("the census never drew.\nLast frame: %q", c)
		}
		composerPageToEnd(t, ctx, frame)
		if c, ok := pumpUntil(frame, "Engrave Plate", 32); !ok {
			t.Fatalf("the preimage plate's engrave screen never drew.\nLast frame: %q", c)
		}
		// CUT IT IN FULL. This is what makes the second window real.
		composerEngraveOnePlate(t, ctx, frame, e)

		if c, ok := pumpUntil(frame, "Choose engraving", 64); !ok {
			t.Fatalf("the policy set never followed the preimage plate.\nLast frame: %q", c)
		}
		if len(order) != 2 || order[0] != "preimage" || order[1] != "policy" {
			t.Fatalf("plate order %v, want [preimage policy]", order)
		}
		// The set-level abort: a partial bundle cannot be used. bundleEngrave
		// draws its own warning first (`Bundle Incomplete`), and §8.4b follows
		// it -- the arm is the COMPOSER's, on the composer's own screen, which
		// is why it is not a clause inside bundleAbortWarningText.
		click(&ctx.Router, Button1)
		if c, ok := pumpUntil(frame, "Bundle Incomplete", 64); !ok {
			t.Fatalf("bundleEngrave's own set-level warning never drew.\nLast frame: %q", c)
		}
		click(&ctx.Router, Button3)
		c, ok := pumpUntil(frame, "A PREIMAGE PLATE WAS CUT", 64)
		if !ok {
			t.Fatalf("§8.4b never drew after a preimage plate was cut and the policy "+
				"set was not.\nLast frame: %q", c)
		}
		if uiContains(c, "NO PREIMAGE PLATE WAS CUT") {
			t.Error("§8.4a fired on a run that DID cut a preimage plate")
		}
		if !uiContains(c, "Store or destroy it now") {
			t.Errorf("§8.4b does not tell the operator what to do with the bearer plate "+
				"on the bench.\nFrame: %q", c)
		}
		click(&ctx.Router, Button3)
		for i := 0; i < 16; i++ {
			if _, ok := frame(); !ok {
				break
			}
		}
		if ret {
			t.Error("composerEngraveStep reported the run finished after §8.4b")
		}
	})
}

// TestComposerCompletedRunDrawsNeitherAbortArm is Step 8's "NEITHER on a
// completed run", and it is the direction a check that fires unconditionally
// would pass.
//
// MUTATION: fire either arm unconditionally -> this fails.
func TestComposerCompletedRunDrawsNeitherAbortArm(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		st := composerH6PlateState(t, "anchor a")
		template, err := composerTemplateChunksFor(st)
		if err != nil {
			t.Fatal(err)
		}
		e := newEngraver()
		p := newEngravedAwarePlatform()
		p.engraver = e
		p.display = sh2DisplaySize
		ctx := NewContext(p)
		var ret, done bool
		var seen []string
		frame, drawer, quit := runUITouch(ctx, func() {
			ret = composerEngraveStep(ctx, &descriptorTheme, st, template, nil)
			done = true
		})
		defer quit()
		record := func() (string, bool) {
			c, ok := frame()
			if ok {
				seen = append(seen, c)
			}
			return c, ok
		}

		if c, ok := pumpUntil(record, "No slot is seated", 24); !ok {
			t.Fatalf("the form choice never drew.\nLast frame: %q", c)
		}
		click(&ctx.Router, Button3)
		if c, ok := pumpUntil(record, "do not cut this preimage", 24); !ok {
			t.Fatalf("step (A) never drew.\nLast frame: %q", c)
		}
		pts := plateHitPoints(ctx, drawer())
		tap(&ctx.Router, drawer(), pts[0])
		record()
		click(&ctx.Router, Button3)
		if c, ok := pumpUntil(record, "Plates To Cut", 24); !ok {
			t.Fatalf("the census never drew.\nLast frame: %q", c)
		}
		composerPageToEnd(t, ctx, frame)
		if c, ok := pumpUntil(record, "Engrave Plate", 32); !ok {
			t.Fatalf("the preimage plate's engrave screen never drew.\nLast frame: %q", c)
		}
		composerEngraveOnePlate(t, ctx, record, e)
		if c, ok := pumpUntil(record, "Choose engraving", 64); !ok {
			t.Fatalf("the policy set never followed.\nLast frame: %q", c)
		}
		click(&ctx.Router, Button3) // take the first engraving variant
		if c, ok := pumpUntil(record, "Engrave Plate", 64); !ok {
			t.Fatalf("the md1 plate's engrave screen never drew.\nLast frame: %q", c)
		}
		composerEngraveOnePlate(t, ctx, record, e)
		for i := 0; i < 128 && !done; i++ {
			if _, ok := record(); !ok {
				break
			}
			// The end-of-run ms1 reminder, if it is drawn, is dismissed with the
			// same button the rest of this walk uses.
			click(&ctx.Router, Button3)
		}
		if !done {
			t.Fatalf("composerEngraveStep never returned.\nLast frame: %q", seen[len(seen)-1])
		}
		if !ret {
			t.Error("a run that cut every plate did not report the set finished")
		}
		for _, c := range seen {
			if uiContains(c, "NO PREIMAGE PLATE WAS CUT") || uiContains(c, "A PREIMAGE PLATE WAS CUT") {
				t.Fatalf("an abort arm was drawn on a COMPLETED run.\nFrame: %q", c)
			}
		}
		t.Logf("the completed run drew %d frames and neither abort arm", len(seen))
	})
}

// TestComposerCensusIsDrawnThroughTheComposerBand is Step 2's requirement at
// the only layer that can carry it.
//
// THERE IS NOTHING AT RUNTIME TO OBSERVE, which is why this is an AST
// assertion -- composer_hashlock_held_test.go's third test makes the same move
// for the same reason. Both screens draw a paged read-only body with Button1
// back, Button3 continue and Button2 page; they differ only in WHERE the ink
// lands, and op.Drawer.ExtractText collects a glyph's rune wherever it lands,
// under a button included. So no text assertion on this screen can tell the two
// apart, and the raster probes cannot either: the real frame draws the buttons.
//
// MUTATION: restore confirmReviewScreen in composerEngraveStep -> this fails,
// and the census's rows measured at 385..459 px are drawn under the navigation
// column (TestComposerCensusRowsAreDrawnInsideTheBand's own numbers).
func TestComposerCensusIsDrawnThroughTheComposerBand(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "composer_flow.go", nil, 0)
	if err != nil {
		t.Fatalf("parsing composer_flow.go: %v", err)
	}
	var fn *ast.FuncDecl
	for _, d := range f.Decls {
		if g, ok := d.(*ast.FuncDecl); ok && g.Name.Name == "composerEngraveStep" {
			fn = g
		}
	}
	if fn == nil {
		t.Fatal("composerEngraveStep is not in composer_flow.go")
	}
	calls := map[string]bool{}
	ast.Inspect(fn, func(n ast.Node) bool {
		if c, ok := n.(*ast.CallExpr); ok {
			if id, ok := c.Fun.(*ast.Ident); ok {
				calls[id.Name] = true
			}
		}
		return true
	})
	if !calls["composerReadScreen"] {
		t.Error("composerEngraveStep does not draw its census through composerReadScreen, " +
			"which is the only paged confirm that wraps inside composerTextBand")
	}
	if calls["confirmReviewScreen"] {
		t.Error("composerEngraveStep draws through confirmReviewScreen, which wraps at " +
			"dims.X-2*8 and centres on the whole panel: every census row over 374 px " +
			"has its right edge under a navigation button (W-3)")
	}
}
