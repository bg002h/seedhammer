package gui

import (
	"image"
	"strings"
	"testing"

	"seedhammer.com/gui/assets"
	"seedhammer.com/gui/op"
	"seedhammer.com/image/rgb565"
	"seedhammer.com/seal"
)

// Task 7 -- the plate list AFTER §10.2.2's secret session.
//
// Three properties: it lists the encrypted section's md1/mk1 cards too (§6.3
// says they are not secret wherever they travelled, and leaving them out would
// be §6.4's incomplete-backup-believed-complete with the operator's own
// payload); records already cut this session are marked; and Back reads as
// leaving the session rather than stepping back one screen.

// unlockedFixture unlocks the both-sections fixture headlessly -- public md1/mk1
// AND encrypted md1/mk1, which NO canonical vector carries.
func unlockedFixture(t *testing.T) *seal.Payload {
	t.Helper()
	public, secret := bothSectionRecords(t)
	blob := sealBlobForTest(t, public, secret, fixturePassphrase, fixtureIterations)
	var o seal.Opener
	p, err := o.Inspect(blob)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	key := seal.DeriveKey(seal.NormalisePassphrase(fixturePassphrase), p.Header.Salt[:],
		int(p.Header.Iterations))
	defer clear(key)
	if err := o.UnlockWithKey(blob, p, key); err != nil {
		t.Fatalf("UnlockWithKey: %v", err)
	}
	return p
}

// TestUnlockPlatesIncludesTheEncryptedSectionsCards — vector C is pub_len 0 with
// six encrypted records: one ms1 and FIVE md1/mk1 cards. Those five are ordinary
// plates and must reach the list, or the operator engraves the secret and
// nothing else and believes the backup complete.
func TestUnlockPlatesIncludesTheEncryptedSectionsCards(t *testing.T) {
	p := unlockedPayload(t, "C")
	if len(p.Public) != 0 {
		t.Fatalf("premise broken: vector C must have no public section, got %d records", len(p.Public))
	}
	wantCards := 0
	for _, r := range p.Secret {
		if r.Class == seal.ClassMDMK {
			wantCards++
		}
	}
	if wantCards != 5 {
		t.Fatalf("premise broken: vector C must carry five encrypted md1/mk1 cards, got %d", wantCards)
	}
	got := unlockPlates(p)
	if len(got) != wantCards {
		t.Fatalf("the list holds %d entries, want the %d encrypted cards", len(got), wantCards)
	}
	for i, e := range got {
		if e.rec.Class != seal.ClassMDMK {
			t.Errorf("entry %d classifies as %v", i, e.rec.Class)
		}
		// pub_len == 0, so only ONE section carries cards and the "(sealed)"
		// disambiguation would be pure noise.
		if e.sealed {
			t.Errorf("entry %d is marked (sealed) on a payload with no public cards", i)
		}
	}
}

// TestUnlockPlatesNeverIncludesASecret. seal.IsSecret is the single definition
// of which records those are, and the session has already wiped them by the
// time this list is built -- an entry for one would offer to engrave zeroes.
func TestUnlockPlatesNeverIncludesASecret(t *testing.T) {
	for _, name := range []string{"C", "F", "G"} {
		t.Run(name, func(t *testing.T) {
			p := unlockedPayload(t, name)
			secrets := 0
			for _, r := range p.Secret {
				if seal.IsSecret(r.Class) {
					secrets++
				}
			}
			if secrets == 0 {
				t.Fatalf("premise broken: vector %s carries no secret to exclude", name)
			}
			for i, e := range unlockPlates(p) {
				if seal.IsSecret(e.rec.Class) {
					t.Errorf("entry %d is a %v -- a secret reached the plate list", i, e.rec.Class)
				}
			}
			// And the count is exactly public + encrypted CARDS.
			want := len(p.Public) + len(p.Secret) - secrets
			if got := len(unlockPlates(p)); got != want {
				t.Errorf("vector %s: the list holds %d entries, want %d", name, got, want)
			}
		})
	}
}

// TestUnlockPlatesMarksSealedOnlyWhenBOTHSectionsCarryCards. Card indices are
// computed PER SECTION, so a public mk1 1/2 and an encrypted mk1 1/2 can both
// exist on one payload; rendered without the suffix the list shows the same
// label twice and the operator cannot tell which plate they are about to cut.
// Added only when both sections actually carry cards, so the ordinary payload
// gains no noise.
func TestUnlockPlatesMarksSealedOnlyWhenBOTHSectionsCarryCards(t *testing.T) {
	t.Run("both sections", func(t *testing.T) {
		p := unlockedFixture(t)
		var pub, enc int
		for _, e := range unlockPlates(p) {
			if e.sealed {
				enc++
			} else {
				pub++
			}
		}
		if pub == 0 || enc == 0 {
			t.Fatalf("the fixture produced %d unsealed and %d sealed entries; it is not "+
				"the both-sections shape", pub, enc)
		}
		// Every entry that came from the ENCRYPTED section is marked, and no
		// entry from the public one is.
		out := unlockPlates(p)
		for i, e := range out {
			fromPublic := i < len(p.Public)
			if fromPublic && e.sealed {
				t.Errorf("public entry %d is marked (sealed)", i)
			}
			if !fromPublic && !e.sealed {
				t.Errorf("encrypted entry %d is not marked (sealed)", i)
			}
		}
	})
	t.Run("encrypted cards only (vector C)", func(t *testing.T) {
		for _, e := range unlockPlates(unlockedPayload(t, "C")) {
			if e.sealed {
				t.Error("a payload with no public cards marked an entry (sealed)")
			}
		}
	})
	t.Run("public cards only (vector G)", func(t *testing.T) {
		// G is 12 public cards and three ms1 -- no encrypted CARDS at all.
		for _, e := range unlockPlates(unlockedPayload(t, "G")) {
			if e.sealed {
				t.Error("a payload with no encrypted cards marked an entry (sealed)")
			}
		}
	})
}

// TestUnlockPlateLabelWrapsPlateLabel. It WRAPS rather than replaces: the
// card/plate arithmetic and the md1/mk1 naming are §6.3's grouping rendered,
// and two functions that both decide what a card is called is the divergence
// F-77 exists to prevent.
func TestUnlockPlateLabelWrapsPlateLabel(t *testing.T) {
	recs := plateRecords(t, "E")
	base := plateLabel(recs[0], 0)
	if base != "mk1 1/2" {
		t.Fatalf("premise broken: plateLabel gave %q", base)
	}
	for _, tc := range []struct {
		sealed, cut bool
		want        string
	}{
		{false, false, "mk1 1/2"},
		{true, false, "mk1 1/2 (sealed)"},
		{false, true, "mk1 1/2 (cut)"},
		{true, true, "mk1 1/2 (sealed) (cut)"},
	} {
		if got := unlockPlateLabel(recs[0], 0, tc.sealed, tc.cut); got != tc.want {
			t.Errorf("unlockPlateLabel(sealed=%v, cut=%v) = %q, want %q",
				tc.sealed, tc.cut, got, tc.want)
		}
	}
	// The marks are WORDS, not glyphs. F-78 measured that "·" contributes zero
	// pixels in this font, and a mark the operator cannot see is worse than no
	// mark at all -- the same finding that put "|" in plateLabel.
	if strings.ContainsAny(unlockPlateLabel(recs[0], 0, true, true), "·•") {
		t.Error("the marks use a glyph this font may not render")
	}
}

// TestPlateListShowsTheSealedSuffixOnDuplicateLabels — the shape the suffix
// exists for, and the one no canonical vector can supply.
func TestPlateListShowsTheSealedSuffixOnDuplicateLabels(t *testing.T) {
	p := unlockedFixture(t)
	plates := unlockPlates(p)

	// The collision is REAL: without the suffix two entries would render
	// identically.
	bare := make(map[string]int)
	for _, e := range plates {
		bare[plateLabel(e.rec, e.idx)]++
	}
	dup := ""
	for lbl, n := range bare {
		if n > 1 {
			dup = lbl
		}
	}
	if dup == "" {
		t.Fatal("no two entries share a bare label; this fixture cannot exercise the suffix")
	}
	// ...and WITH it, every label is unique.
	seen := make(map[string]bool)
	for _, e := range plates {
		l := unlockPlateLabel(e.rec, e.idx, e.sealed, e.cut)
		if seen[l] {
			t.Errorf("two entries both render as %q even with the suffix", l)
		}
		seen[l] = true
	}

	frame, drawer, _, ctx, quit := runPlateList(t, newPlatform(), plates)
	defer quit()
	pages := collectPages(t, frame, drawer, ctx, 4)
	joined := strings.Join(pages, "\n")
	if !uiContains(joined, dup+" (sealed)") {
		t.Errorf("no page shows %q with the (sealed) suffix; pages were %q", dup, pages)
	}
	// The PUBLIC one is there too, unsuffixed -- the suffix disambiguates, it
	// does not replace.
	if !uiContains(joined, dup) {
		t.Errorf("no page shows the public %q at all", dup)
	}
}

// TestPlateListMarksCutAfterACompletedEngraveAndNotAfterACancelledOne.
//
// The mark is a CONVENIENCE, not a guarantee: it does not survive a power cut,
// and §10.2.2 requires the UI not imply that it does. What it must do is track
// COMPLETION -- marking a cancelled plate would tell the operator a plate is in
// the tin when it is not, which is §6.4's hazard wearing a label.
func TestPlateListMarksCutAfterACompletedEngraveAndNotAfterACancelledOne(t *testing.T) {
	t.Run("cancelled", func(t *testing.T) {
		plates := asPlates(plateRecords(t, "E"))
		frame, drawer, _, ctx, quit := runPlateList(t, newPlatform(), plates)
		defer quit()
		if _, ok := frame(); !ok {
			t.Fatal("the plate list produced no frame")
		}
		tapNavSlot(t, ctx, drawer(), Button3) // OK on the first entry
		if c, ok := pumpUntil(frame, "Choose engraving", 32); !ok {
			t.Fatalf("OK did not reach the variant choice; got %q", c)
		}
		tapNavSlot(t, ctx, drawer(), Button1) // Back out without engraving
		c, ok := pumpUntil(frame, "mk1 1/2", 32)
		if !ok {
			t.Fatalf("did not return to the plate list; got %q", c)
		}
		if uiContains(c, "(cut)") {
			t.Errorf("a CANCELLED engrave marked the plate as cut: %q", c)
		}
	})

	t.Run("completed", func(t *testing.T) {
		plates := asPlates(plateRecords(t, "E"))
		p := newPlatform()
		p.engraver = newEngraver()
		frame, drawer, _, ctx, quit := runPlateList(t, p, plates)
		defer quit()
		if _, ok := frame(); !ok {
			t.Fatal("the plate list produced no frame")
		}
		tapNavSlot(t, ctx, drawer(), Button3) // OK
		if c, ok := pumpUntil(frame, "Choose engraving", 32); !ok {
			t.Fatalf("OK did not reach the variant choice; got %q", c)
		}
		tapNavSlot(t, ctx, drawer(), Button3) // choose the first variant
		if c, ok := pumpUntil(frame, "Insert a blank plate", 64); !ok {
			t.Fatalf("did not reach the engrave screen; got %q", c)
		}
		// Hold to start. Measured: this plate completes in ~29,500 frames.
		press(&ctx.Router, Button3)
		c, ok := pumpUntil(frame, "Engraving completed", 200000)
		if !ok {
			t.Fatalf("the engrave never completed; last frame %q", c)
		}
		click(&ctx.Router, Button3) // acknowledge -> Engrave returns true
		c, ok = pumpUntil(frame, "(cut)", 64)
		if !ok {
			t.Fatalf("a COMPLETED engrave did not mark the plate as cut; last frame %q", c)
		}
		if !uiContains(c, "mk1 1/2 (cut)") {
			t.Errorf("the mark is not on the engraved entry: %q", c)
		}
		// The OTHER entries are untouched.
		if strings.Count(c, "(cut)") != 1 {
			t.Errorf("a completed engrave marked more than one entry: %q", c)
		}
	})
}

// TestPlateListBackIconIsDiscardNotBack — §10.3 and F-80's B2 item: Back reads
// as LEAVING THE SESSION, not as stepping back one screen. There is no lock
// glyph in gui/assets; assets.IconDiscard already means exactly this
// (gui/gui.go uses it for discarding a seed, gui/freetext_flow.go for clearing
// entered text), and "discard this session and everything in it" is what Back
// does here.
//
// Asserted on PIXELS, because the icon is an image and ExtractText cannot see
// it. The Back slot is rendered and compared against references drawn with each
// candidate icon: it must MATCH IconDiscard and DIFFER from IconBack, so
// neither direction can pass by accident.
func TestPlateListBackIconIsDiscardNotBack(t *testing.T) {
	dims := sh2DisplaySize
	plates := asPlates(plateRecords(t, "E"))

	pf := newPlatform()
	pf.display = dims
	ctx := NewContext(pf)
	slot := backSlotRect(dims)
	got := firstFrameRegion(t, ctx, slot, func() {
		unlockPlateListFlow(ctx, &descriptorTheme, plates)
	})
	if len(got) == 0 {
		t.Fatal("the plate list drew no frame")
	}
	discard := referenceRegion(t, dims, slot, assets.IconDiscard)
	back := referenceRegion(t, dims, slot, assets.IconBack)

	// Premise: the two icons must actually LOOK different, or nothing below
	// discriminates anything.
	if sameRegion(discard, back) {
		t.Fatal("IconDiscard and IconBack render identically; this test cannot discriminate")
	}
	if !sameRegion(got, discard) {
		t.Errorf("the plate list's Back slot does not render assets.IconDiscard.\n" +
			"§10.3 requires Back to read as leaving the SESSION -- in B2 it discards a " +
			"decrypted payload and costs twelve words and a ~31 s KDF to get back")
	}
	if sameRegion(got, back) {
		t.Errorf("the plate list's Back slot still renders assets.IconBack")
	}
}

// firstFrameRegion runs ui until it draws its FIRST frame and returns that
// frame's pixels over r.
//
// The rendering happens INSIDE the callback, deliberately: Context.Frame calls
// c.B.Reset() the moment the callback returns (gui/gui.go:78), so an op tree
// carried out of it refers into a buffer that has already been recycled.
// Measured the hard way -- doing it afterwards panics with an index-out-of-
// range inside Drawer.draw.
func firstFrameRegion(t *testing.T, ctx *Context, r image.Rectangle, ui func()) []byte {
	t.Helper()
	dims := ctx.Platform.DisplaySize()
	var got []byte
	ctx.FrameCallback = func(o op.Op) {
		if got == nil {
			got = renderRegion(t, dims, o, r)
		}
		ctx.Done = true
	}
	ui()
	return got
}

// referenceRegion draws the nav alone, with one icon in the Back slot, over the
// same background the flow uses, and returns the same region's pixels.
func referenceRegion(t *testing.T, dims image.Point, r image.Rectangle, icon image.Image) []byte {
	t.Helper()
	pf := newPlatform()
	pf.display = dims
	ctx := NewContext(pf)
	th := &descriptorTheme
	nav, _ := layoutNavigation(&ctx.B, th, dims, []NavButton{
		{Clickable: &Clickable{Button: Button1}, Style: StyleSecondary, Icon: icon},
		{Clickable: &Clickable{Button: Button2}, Style: StyleSecondary, Icon: assets.IconRight},
		{Clickable: &Clickable{Button: Button3}, Style: StylePrimary, Icon: assets.IconHammer},
	}...)
	return renderRegion(t, dims, op.Layer(nav, op.Color(&ctx.B, th.Background)), r)
}

// backSlotRect is where layoutNavigation puts Button1 (gui/gui.go:1918-1932).
func backSlotRect(dims image.Point) image.Rectangle {
	sz := assets.NavBtnPrimary.Bounds().Size()
	min := image.Pt(dims.X-sz.X, leadingSize)
	return image.Rectangle{Min: min, Max: min.Add(sz)}
}

func renderRegion(t *testing.T, dims image.Point, o op.Op, r image.Rectangle) []byte {
	t.Helper()
	clip := image.Rectangle{Max: dims}
	fb := rgb565.New(clip)
	maskfb := image.NewAlpha(clip)
	d := new(op.Drawer)
	d.Draw(fb, maskfb, o)
	out := make([]byte, 0, r.Dx()*r.Dy()*8)
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			c := fb.RGBA64At(x, y)
			out = append(out, byte(c.R>>8), byte(c.G>>8), byte(c.B>>8), byte(c.A>>8))
		}
	}
	return out
}

func sameRegion(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestUnlockFlowWipesEveryRecordOnExit — §10.2 step 10 and §10.2.2's "Lock,
// Back, an error, ctx.Done are ONE exit, not four".
//
// This is the test that kills "defer p.Wipe() removed". §10.2.2's per-record
// wipe already zeroes the SECRETS, so nothing in Task 6 could see the flow-level
// backstop go missing -- what only p.Wipe() reaches is the PUBLIC records and
// the encrypted md1/mk1 cards, and unlockEngraveHook is the one seam that hands
// a test a live handle on one of those buffers.
func TestUnlockFlowWipesEveryRecordOnExit(t *testing.T) {
	var held [][]byte
	unlockEngraveHook = func(_ string, rec []byte) { held = append(held, rec) }
	t.Cleanup(func() { unlockEngraveHook = nil })

	v := sealVector(t, "D")
	h := newUnlockHarness(t, payloadReaderFrom(t, v.blob(t)))
	h.toPassphrase(true)
	h.typePassphrase(vectorPassphrase(t, v))
	// Past the derivation and the single-ms1 secret session.
	h.mustReach("SECRET seed material")
	pts := plateHitPoints(h.ctx, h.drawer())
	if len(pts) < 2 {
		t.Fatalf("the choice screen drew %d targets", len(pts))
	}
	tap(&h.ctx.Router, h.drawer(), pts[1]) // Skip
	h.next("after selecting Skip")
	h.tapNav(Button3)
	// The plate list.
	h.mustReach("mk1 1/2")
	h.tapNav(Button3) // OK -- fires unlockEngraveHook with a PUBLIC record
	h.mustReach("Choose engraving")
	if len(held) == 0 {
		t.Fatal("the engrave path never handed the test a record buffer")
	}
	rec := held[0]
	if allZeroBytes(rec) {
		t.Fatal("premise broken: the public record was already zero when offered")
	}

	// Leave: Back out of the variant choice, then Back out of the list, which
	// IS Lock (§10.3).
	h.tapNav(Button1)
	h.mustReach("mk1 1/2")
	h.tapNav(Button1)
	for i := 0; i < 128 && !*h.done; i++ {
		if _, ok := h.frame(); !ok {
			break
		}
	}
	if !*h.done {
		t.Fatal("Back on the plate list did not leave the flow")
	}
	if !allZeroBytes(rec) {
		t.Errorf("a PUBLIC record survived the flow's exit: %q\n"+
			"§10.2 step 10 requires every record in BOTH sections zeroed on every exit "+
			"path; §10.2.2's per-record wipe only reaches the secrets", rec)
	}
}

// asPlates wraps public AdmittedRecords as list entries, which is the shape the
// UNSEALED path produces.
func asPlates(recs []seal.AdmittedRecord) []unlockPlate {
	out := make([]unlockPlate, len(recs))
	for i, r := range recs {
		out[i] = unlockPlate{rec: r, idx: i}
	}
	return out
}
