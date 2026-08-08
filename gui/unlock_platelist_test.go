package gui

import (
	"errors"
	"image"
	"io"
	"slices"
	"strings"
	"testing"

	"seedhammer.com/gui/assets"
	"seedhammer.com/gui/op"
	"seedhammer.com/gui/widget"
	"seedhammer.com/seal"
)

// drawnLabel is a label as it actually reaches the panel. The "·" separator has
// NO GLYPH in this font and contributes zero width -- measured, and pinned by
// TestPlateLabelSeparatorDoesNotRender below -- so it never appears in
// extracted text. Asserting against the raw label would silently pass on a
// screen that never drew the entry at all.
func drawnLabel(s string) string { return strings.ReplaceAll(s, "·", "") }

// TestPlateLabelSeparatorDoesNotRender records a surprising, load-bearing
// measurement rather than leaving it to be rediscovered on hardware.
//
// §10.2.2's multi-card label form is `mk1 2/3 · 1/2`, and the fork already uses
// "·" in shipped UI (bundle_flow.go's "Card X of Y · Plate P of Q",
// codex32_polish.go, slip39_polish.go). It is INVISIBLE in this font: "a·b" and
// "ab" measure identically, while "a-b" does not. So the operator sees
// `mk1 2/3 1/2`, with the two index pairs separated by whitespace alone.
//
// If this test fails, the font gained the glyph -- which is good news; update
// drawnLabel and delete this test.
func TestPlateLabelSeparatorDoesNotRender(t *testing.T) {
	ctx := NewContext(newPlatform())
	measure := func(s string) int {
		_, sz := widget.Labelw(&ctx.B, ctx.Styles.body, 400, descriptorTheme.Text, s)
		return sz.X
	}
	withDot, without := measure("a·b"), measure("ab")
	t.Logf("width(\"a·b\")=%d width(\"ab\")=%d width(\"a-b\")=%d", withDot, without, measure("a-b"))
	if withDot != without {
		t.Errorf("the \"·\" separator now measures %dpx (was 0); it renders, so drawnLabel is stale",
			withDot-without)
	}
	if measure("a-b") == without {
		t.Fatal("premise broken: \"-\" must render, or this measurement proves nothing about \"·\"")
	}
}

// TestPlateLabelForm pins the label rule itself, as a pure function, before any
// screen is involved: one card of an HRP collapses to the plate index (which is
// §10.2.2's own single-sig example), several cards carry the card index first.
func TestPlateLabelForm(t *testing.T) {
	for _, tc := range []struct {
		vector string
		want   []string
	}{
		{"E", []string{"mk1 1/2", "mk1 2/2", "md1 1/3", "md1 2/3", "md1 3/3"}},
		{"G", []string{
			"mk1 1/3 · 1/2", "mk1 1/3 · 2/2",
			"mk1 2/3 · 1/2", "mk1 2/3 · 2/2",
			"mk1 3/3 · 1/2", "mk1 3/3 · 2/2",
			"md1 1/6", "md1 2/6", "md1 3/6", "md1 4/6", "md1 5/6", "md1 6/6",
		}},
	} {
		t.Run(tc.vector, func(t *testing.T) {
			recs := plateRecords(t, tc.vector)
			if len(recs) != len(tc.want) {
				t.Fatalf("vector %s has %d public records, want %d", tc.vector, len(recs), len(tc.want))
			}
			for i, want := range tc.want {
				if got := plateLabel(recs[i], i); got != want {
					t.Errorf("record %d is labelled %q, want %q", i, got, want)
				}
			}
		})
	}
}

// §10.3's plate list: paged (a ChoiceScreen truncates past ~7), labelled from
// the CLASSIFIED type and the §6.3 card grouping, and Back is the session exit.

// plateRecords returns a vector's admitted PUBLIC records, straight out of
// Phase A. The labels under test are §6.3's grouping rendered, so the input has
// to come through the same pipeline the device uses.
func plateRecords(t *testing.T, name string) []seal.AdmittedRecord {
	t.Helper()
	var o seal.Opener
	p, err := o.Inspect(sealVectorBlob(t, name))
	if err != nil {
		t.Fatalf("vector %s: Inspect: %v", name, err)
	}
	return p.Public
}

func runPlateList(t *testing.T, p *testPlatform, recs []seal.AdmittedRecord) (frame func() (string, bool), drawer func() *op.Drawer, done *bool, ctx *Context, quit func()) {
	t.Helper()
	p.display = sh2DisplaySize
	ctx = NewContext(p)
	returned := false
	frame, drawer, quit = runUITouch(ctx, func() {
		unlockPlateListFlow(ctx, &descriptorTheme, recs)
		returned = true
	})
	return frame, drawer, &returned, ctx, quit
}

// plateHitPoints returns a point inside each drawn ENTRY's touch target, top to
// bottom, found the way a finger would find them. Entry Clickables carry the
// zero Button (None); the three nav slots carry Button1..Button3 and sit at the
// right edge, so scanning the horizontal centre cannot confuse the two.
func plateHitPoints(ctx *Context, d *op.Drawer) []image.Point {
	dims := ctx.Platform.DisplaySize()
	var pts []image.Point
	var seen []op.Tag
	for y := 0; y < dims.Y; y++ {
		pt := image.Pt(dims.X/2, y)
		tag, _, ok := d.Hit(pt)
		if !ok {
			continue
		}
		c, isClk := tag.(*Clickable)
		if !isClk || c.Button != None {
			continue
		}
		if slices.Contains(seen, tag) {
			continue
		}
		seen = append(seen, tag)
		pts = append(pts, pt)
	}
	return pts
}

// pageLabels collects the labels visible on the current page by walking the
// pages once and recording what each frame drew.
func collectPages(t *testing.T, frame func() (string, bool), drawer func() *op.Drawer, ctx *Context, pages int) []string {
	t.Helper()
	var out []string
	for i := 0; i < pages; i++ {
		content, ok := frame()
		if !ok {
			t.Fatalf("no frame on page %d", i)
		}
		out = append(out, content)
		tapNavSlot(t, ctx, drawer(), Button2)
	}
	return out
}

// tapNavSlot taps a nav button by slot, asserting the target actually there is
// bound to that button -- a nav tap that lands on nothing would otherwise read
// as "the screen ignored me".
func tapNavSlot(t *testing.T, ctx *Context, d *op.Drawer, b Button) {
	t.Helper()
	dims := ctx.Platform.DisplaySize()
	sz := assets.NavBtnPrimary.Bounds().Size()
	ys := [3]int{leadingSize, (dims.Y - sz.Y) / 2, dims.Y - leadingSize - sz.Y}
	pos := image.Pt(dims.X-sz.X/2, ys[int(b-Button1)]+sz.Y/2)
	tag, _, hit := d.Hit(pos)
	if !hit {
		t.Fatalf("nav %v: no touch target at %v", b, pos)
	}
	c, ok := tag.(*Clickable)
	if !ok || (c.Button != b && c.AltButton != b) {
		t.Fatalf("nav %v: the target at %v is %v, not a Clickable bound to %v", b, pos, tag, b)
	}
	tap(&ctx.Router, d, pos)
}

// §10.2.2's own examples are single-sig, where there is exactly one card of
// each HRP -- so the label collapses to the plate index within the card.
func TestPlateListLabelsASingleCardPerHRP(t *testing.T) {
	recs := plateRecords(t, "E")
	frame, _, _, _, quit := runPlateList(t, newPlatform(), recs)
	defer quit()
	content, ok := frame()
	if !ok {
		t.Fatal("the plate list produced no frame")
	}
	for _, want := range []string{"mk1 1/2", "mk1 2/2", "md1 1/3", "md1 2/3", "md1 3/3"} {
		if !uiContains(content, want) {
			t.Errorf("the plate list does not show %q; got %q", want, content)
		}
	}
	// And it must NOT print the multi-card form when there is one card. The
	// needle carries no "·" on purpose: that glyph never reaches the panel, so
	// an assertion containing it could never fire.
	if uiContains(content, "mk1 1/1") {
		t.Errorf("a single card of an HRP was labelled with a card index: %q", content)
	}
}

// A 2-of-3 is mk1 x6 across THREE cards. A flat mk1 1/6..6/6 conflates three
// distinct cosigners -- the operator cannot tell which cosigner a plate belongs
// to, which is §6.4's incomplete-backup-believed-complete hazard wearing a
// label.
func TestPlateListLabelsSeveralCardsOfOneHRP(t *testing.T) {
	recs := plateRecords(t, "G")
	if len(recs) != 12 {
		t.Fatalf("premise broken: vector G's public section must be 12 records, got %d", len(recs))
	}
	frame, drawer, _, ctx, quit := runPlateList(t, newPlatform(), recs)
	defer quit()
	// Vector G's cards: mk1 x3 of two plates each, then md1 x1 of six.
	want := []string{
		"mk1 1/3 · 1/2", "mk1 1/3 · 2/2",
		"mk1 2/3 · 1/2", "mk1 2/3 · 2/2",
		"mk1 3/3 · 1/2", "mk1 3/3 · 2/2",
		"md1 1/6", "md1 2/6", "md1 3/6", "md1 4/6", "md1 5/6", "md1 6/6",
	}
	pages := collectPages(t, frame, drawer, ctx, 4)
	for _, w := range want {
		found := false
		for _, p := range pages {
			if uiContains(p, drawnLabel(w)) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("no page shows %q; pages were %q", w, pages)
		}
	}
}

// THE paging test, and the reason §10.3 forbids a ChoiceScreen here.
//
// It asserts two things a non-paged list cannot satisfy at once: no single
// frame carries all twelve entries, AND every entry is reachable by paging. A
// ChoiceScreen draws all twelve children -- over its own title, past the panel
// -- so it fails the first; a list that pages but loses entries fails the
// second.
func TestPlateListPagesThroughEveryRecord(t *testing.T) {
	recs := plateRecords(t, "G")
	frame, drawer, _, ctx, quit := runPlateList(t, newPlatform(), recs)
	defer quit()

	first, ok := frame()
	if !ok {
		t.Fatal("the plate list produced no frame")
	}
	onFirst := 0
	for i := range recs {
		if uiContains(first, drawnLabel(plateLabel(recs[i], i))) {
			onFirst++
		}
	}
	if onFirst == len(recs) {
		t.Fatalf("all %d entries were drawn on one frame; §10.3's paged shape is not in use: %q", len(recs), first)
	}
	if onFirst == 0 {
		t.Fatalf("the first page drew no entries at all: %q", first)
	}
	// Every entry drawn on the first page must have a real touch target.
	if got := len(plateHitPoints(ctx, drawer())); got != onFirst {
		t.Errorf("%d entries are drawn but %d have touch targets", onFirst, got)
	}

	seen := make([]bool, len(recs))
	for page := 0; page < len(recs)+1; page++ {
		content, ok := frame()
		if !ok {
			t.Fatalf("no frame on page %d", page)
		}
		for i := range recs {
			if uiContains(content, drawnLabel(plateLabel(recs[i], i))) {
				seen[i] = true
			}
		}
		if !slices.Contains(seen, false) {
			break
		}
		tapNavSlot(t, ctx, drawer(), Button2)
	}
	for i, s := range seen {
		if !s {
			t.Errorf("record %d (%s) is unreachable by paging", i, plateLabel(recs[i], i))
		}
	}
}

// Back is the session exit and must work from ANY page, not only the first.
func TestPlateListBackReturnsFromAnyPage(t *testing.T) {
	recs := plateRecords(t, "G")
	frame, drawer, done, ctx, quit := runPlateList(t, newPlatform(), recs)
	defer quit()
	if _, ok := frame(); !ok {
		t.Fatal("the plate list produced no frame")
	}
	tapNavSlot(t, ctx, drawer(), Button2) // to page 2
	if _, ok := frame(); !ok {
		t.Fatal("no frame after paging")
	}
	tapNavSlot(t, ctx, drawer(), Button1) // Back
	for i := 0; i < 32 && !*done; i++ {
		frame()
	}
	if !*done {
		t.Fatal("Back on page 2 did not leave the plate list")
	}
}

// OK engraves the HIGHLIGHTED entry, and the bytes that reach the engrave path
// are that record's.
func TestPlateListOKEngravesTheSelectedRecord(t *testing.T) {
	recs := plateRecords(t, "G")
	var gotLabel string
	var gotRecord []byte
	unlockEngraveHook = func(label string, rec []byte) { gotLabel, gotRecord = label, rec }
	t.Cleanup(func() { unlockEngraveHook = nil })

	frame, drawer, _, ctx, quit := runPlateList(t, newPlatform(), recs)
	defer quit()
	if _, ok := frame(); !ok {
		t.Fatal("the plate list produced no frame")
	}
	pts := plateHitPoints(ctx, drawer())
	if len(pts) < 3 {
		t.Fatalf("only %d entries have touch targets; cannot select a third", len(pts))
	}
	tap(&ctx.Router, drawer(), pts[2]) // select the third entry
	if _, ok := frame(); !ok {
		t.Fatal("no frame after selecting")
	}
	tapNavSlot(t, ctx, drawer(), Button3) // OK
	if content, ok := pumpUntil(frame, "Choose engraving", 32); !ok {
		t.Fatalf("OK did not reach the engrave-variant choice; got %q", content)
	}
	if gotRecord == nil {
		t.Fatal("the engrave path never saw a record")
	}
	if string(gotRecord) != string(recs[2].Record) {
		t.Errorf("OK engraved %q, want the third entry %q", gotRecord, recs[2].Record)
	}
	if want := plateLabel(recs[2], 2); gotLabel != want {
		t.Errorf("the engrave screen is titled %q, want the plate's own label %q", gotLabel, want)
	}
}

// Returning from the engrave path lands back on the plate list, ON THE SAME
// PAGE. Dropping the operator back to page 1 after every plate is how a
// fifteen-record bundle gets one engraved twice and another not at all.
func TestPlateListReturnsToTheSamePageAfterEngrave(t *testing.T) {
	recs := plateRecords(t, "G")
	frame, drawer, _, ctx, quit := runPlateList(t, newPlatform(), recs)
	defer quit()
	if _, ok := frame(); !ok {
		t.Fatal("the plate list produced no frame")
	}
	tapNavSlot(t, ctx, drawer(), Button2) // page 2
	page2, ok := frame()
	if !ok {
		t.Fatal("no frame after paging")
	}
	tapNavSlot(t, ctx, drawer(), Button3) // OK on page 2's selection
	if content, ok := pumpUntil(frame, "Choose engraving", 32); !ok {
		t.Fatalf("OK did not reach the engrave-variant choice; got %q", content)
	}
	tapNavSlot(t, ctx, drawer(), Button1) // Back out of the variant choice
	content, ok := pumpUntil(frame, "Sealed Payload", 32)
	if !ok {
		t.Fatalf("did not return to the plate list; got %q", content)
	}
	if content != page2 {
		t.Errorf("returned to a different page than the one engraved from:\n got %q\nwant %q", content, page2)
	}
}

// failingNFC fails the test the moment it is opened.
type failingNFC struct{ t *testing.T }

func (f failingNFC) Read(p []byte) (int, error) { return 0, errors.New("nfc: must not be read") }
func (f failingNFC) Close() error               { return nil }

// No path from the plate list may reach mk1GatherFlow or md1GatherFlow. Both
// prime a fresh gatherer from the single string handed to them and then wait on
// PHYSICAL NFC tags for the rest of the chunk set -- and a payload-derived
// record has no tags to tap, so the operator would be stranded on a scanning
// screen for the ordinary case of a chunked card.
//
// Without this the Inspect branch can be reintroduced by a later edit and
// nothing notices: the failure only shows up on a device with an NFC reader
// attached, which no test has.
func TestPlateListNeverOpensTheNFCReader(t *testing.T) {
	p := newPlatform()
	opened := 0
	p.nfc = func() io.ReadCloser {
		opened++
		t.Errorf("the plate list opened the NFC reader; the md1/mk1 Inspect branch is reachable again")
		return failingNFC{t}
	}
	recs := plateRecords(t, "G")
	frame, drawer, _, ctx, quit := runPlateList(t, p, recs)
	defer quit()
	if _, ok := frame(); !ok {
		t.Fatal("the plate list produced no frame")
	}
	tapNavSlot(t, ctx, drawer(), Button3) // OK
	// "TEXT + QR" is drawn by the variant list whether or not an Inspect entry
	// was prepended, so this waits for the screen WITHOUT assuming the clean
	// shape. Waiting on "Choose engraving" instead would abandon the test on
	// the very mutant it exists to catch, since mdmkFlow retitles the lead
	// "Choose action" -- measured.
	if content, ok := pumpUntil(frame, "TEXT + QR", 32); !ok {
		t.Fatalf("OK did not reach the engrave-variant choice; got %q", content)
	}
	// Choose the FIRST entry, which is exactly where mdmkFlow puts "Inspect
	// key" / "Inspect descriptor", then keep pumping: a gather flow opens the
	// reader as soon as the single string it was primed with turns out to be an
	// incomplete card, which every chunked record is.
	for i := 0; i < 8; i++ {
		tapNavSlot(t, ctx, drawer(), Button3)
		frame()
	}
	if opened != 0 {
		t.Fatalf("the NFC reader was opened %d times", opened)
	}
}
