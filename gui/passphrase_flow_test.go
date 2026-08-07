package gui

import (
	"bytes"
	"fmt"
	"image"
	"log"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"seedhammer.com/backup"
	"seedhammer.com/gui/assets"
	"seedhammer.com/gui/op"
	"seedhammer.com/passphrase"
)

// Every test in this file drives the flow by PointerEvent, never by a
// synthesized ButtonEvent. SeedHammer II has no directional buttons: the only
// production input is the ft6x36 capacitive panel. A screen wired to
// ButtonFilter alone is dead on the machine and green in a button-driven test
// -- that defect shipped once in the StartScreen pager (fixed in 86e0da9), and
// the tests that missed it looked exactly like a button-driven version of these.
//
// Two further rules make the touch claim honest:
//
//  1. The flow is laid out at sh2DisplaySize, the panel the device actually
//     has. The 240x240 default test display is narrower than the passphrase
//     keyboard (340px), which pushes q/p/a/l off the canvas -- reachable by a
//     hit test, unreachable by a finger.
//  2. A tap is aimed at the CENTRE of a hit area read back from the frame that
//     was drawn (op.Drawer.TagBounds), and that centre must be on-screen and
//     must be the topmost target there. A widget that is undrawn, off-panel or
//     covered fails the test rather than being quietly tapped anyway.

// ppHarness runs one flow (or sub-flow) under touch, recording the interactive
// widgets it constructs so taps can be aimed at real drawn geometry.
type ppHarness struct {
	t       *testing.T
	ctx     *Context
	frame   func() (string, bool)
	drawer  func() *op.Drawer
	widgets map[string]any
	content string
}

func newPPHarness(t *testing.T) *ppHarness {
	t.Helper()
	p := newPlatform()
	p.display = sh2DisplaySize
	h := &ppHarness{t: t, ctx: NewContext(p), widgets: make(map[string]any)}
	passphraseWidgetHook = func(name string, w any) { h.widgets[name] = w }
	t.Cleanup(func() { passphraseWidgetHook = nil })
	return h
}

func (h *ppHarness) start(ui func()) {
	h.t.Helper()
	frame, drawer, quit := runUITouch(h.ctx, ui)
	h.frame, h.drawer = frame, drawer
	h.t.Cleanup(quit)
	h.next("first frame")
}

// next pumps one frame and records it.
func (h *ppHarness) next(what string, args ...any) string {
	h.t.Helper()
	c, ok := h.frame()
	if !ok {
		h.t.Fatalf("no frame (%s)", fmt.Sprintf(what, args...))
	}
	h.content = c
	return c
}

// pump advances up to n frames, stopping as soon as cond holds.
func (h *ppHarness) pump(n int, want string) bool {
	h.t.Helper()
	for range n {
		c, ok := h.frame()
		if !ok {
			return false
		}
		h.content = c
		if uiContains(c, want) {
			return true
		}
	}
	return false
}

// mustReach pumps frames until want appears, failing the test if it never
// does. Every step of a flow test goes through this rather than a bare pump
// whose false return could be dropped on the floor.
func (h *ppHarness) mustReach(want string) {
	h.t.Helper()
	if !h.pump(32, want) {
		h.t.Fatalf("never reached %q; last frame %q", want, h.content)
	}
}

// backspace taps the keyboard's backspace key n times.
func (h *ppHarness) backspace(n int) {
	h.t.Helper()
	kbd, ok := h.widget("kbd").(*PassphraseKeyboard)
	if !ok {
		h.t.Fatal("widget \"kbd\" is not a *PassphraseKeyboard")
	}
	for range n {
		tag := ppTagFor(kbd, func(k ppKey) bool { return k.action == ppBackspace })
		if tag == nil {
			h.t.Fatalf("no backspace key on page %d", kbd.page)
		}
		h.tapAt(h.point(tag, "backspace key"))
		h.next("after backspace")
	}
}

// point returns the centre of tag's hit area, failing the test if the target is
// not drawn, is off-panel, or is covered -- each of which makes it unreachable
// by a finger.
func (h *ppHarness) point(tag op.Tag, what string) image.Point {
	h.t.Helper()
	d := h.drawer()
	b, ok := d.TagBounds(tag)
	if !ok {
		h.t.Fatalf("%s: no hit area was drawn -- unreachable by touch", what)
	}
	c := b.Min.Add(b.Max).Div(2)
	screen := image.Rectangle{Max: h.ctx.Platform.DisplaySize()}
	if !c.In(screen) {
		h.t.Fatalf("%s: hit area %v lies off the %v panel -- unreachable by a finger", what, b, screen)
	}
	if hit, _, ok := d.Hit(c); !ok || hit != tag {
		h.t.Fatalf("%s: hit area %v is covered by another target (%v)", what, b, hit)
	}
	return c
}

func (h *ppHarness) tapAt(pos image.Point) {
	h.t.Helper()
	tap(&h.ctx.Router, h.drawer(), pos)
}

// widget returns a widget the flow registered, failing if the step never
// constructed it.
func (h *ppHarness) widget(name string) any {
	h.t.Helper()
	w, ok := h.widgets[name]
	if !ok {
		h.t.Fatalf("the flow never registered a %q widget; have %v", name, h.widgetNames())
	}
	return w
}

func (h *ppHarness) widgetNames() []string {
	var names []string
	for n := range h.widgets {
		names = append(names, n)
	}
	return names
}

// tapWidget taps a registered Clickable by name and pumps a frame if the flow
// is still running -- a tap that completes a step legitimately produces no
// further frame from it.
func (h *ppHarness) tapWidget(name string) {
	h.t.Helper()
	c, ok := h.widget(name).(*Clickable)
	if !ok {
		h.t.Fatalf("widget %q is not a *Clickable", name)
	}
	h.tapAt(h.point(c, "widget "+name))
	h.step()
}

// step pumps one frame, tolerating a flow that has just returned.
func (h *ppHarness) step() bool {
	h.t.Helper()
	c, ok := h.frame()
	if ok {
		h.content = c
	}
	return ok
}

// tapNav taps the nav slot for b and asserts the target actually sitting there
// is the Clickable bound to b. Used for screens that own their buttons
// privately (the error modal), where there is no tag to look up.
func (h *ppHarness) tapNav(b Button) {
	h.t.Helper()
	pos := h.navPoint(b)
	d := h.drawer()
	tag, _, hit := d.Hit(pos)
	if !hit {
		h.t.Fatalf("nav %v: no touch target at %v", b, pos)
	}
	c, ok := tag.(*Clickable)
	if !ok || (c.Button != b && c.AltButton != b) {
		h.t.Fatalf("nav %v: the target at %v is %v, not a Clickable bound to %v", b, pos, tag, b)
	}
	h.tapAt(pos)
	h.step()
}

func (h *ppHarness) navPoint(b Button) image.Point {
	dims := h.ctx.Platform.DisplaySize()
	sz := assets.NavBtnPrimary.Bounds().Size()
	ys := [3]int{leadingSize, (dims.Y - sz.Y) / 2, dims.Y - leadingSize - sz.Y}
	return image.Pt(dims.X-sz.X/2, ys[int(b-Button1)]+sz.Y/2)
}

// ppTagFor returns the hit-test tag of the first key on the keyboard's CURRENT
// page matching pred, or nil.
func ppTagFor(kbd *PassphraseKeyboard, pred func(ppKey) bool) op.Tag {
	rows := kbd.pages[kbd.page]
	for i, row := range rows {
		for j := range row {
			if pred(row[j]) {
				return &kbd.pages[kbd.page][i][j].clk
			}
		}
	}
	return nil
}

// typeRune types one character on the registered keyboard, cycling pages by
// touch until the character's page is up. Every step is a real tap on a real
// hit area, so a rune the operator cannot reach fails here.
func (h *ppHarness) typeRune(r rune) {
	h.t.Helper()
	kbd, ok := h.widget("kbd").(*PassphraseKeyboard)
	if !ok {
		h.t.Fatal("widget \"kbd\" is not a *PassphraseKeyboard")
	}
	for range len(ppPages) + 1 {
		if tag := ppTagFor(kbd, func(k ppKey) bool { return k.action == ppRune && k.r == r }); tag != nil {
			h.tapAt(h.point(tag, fmt.Sprintf("key %q", string(r))))
			h.next("after typing %q", string(r))
			return
		}
		cyc := ppTagFor(kbd, func(k ppKey) bool { return k.action == ppPageCycle })
		if cyc == nil {
			h.t.Fatalf("no page-cycle key on keyboard page %d", kbd.page)
		}
		h.tapAt(h.point(cyc, "page-cycle key"))
		h.next("after cycling to page %d", kbd.page)
	}
	h.t.Fatalf("%q is not typeable on any keyboard page", string(r))
}

func (h *ppHarness) typeString(s string) {
	h.t.Helper()
	for _, r := range s {
		h.typeRune(r)
	}
}

// uiHas is uiContains WITHOUT the case folding and WITHOUT rewriting the
// needle. The confirm screen has to prove it preserves case and renders a
// visible mark where a space would be, and a case-insensitive,
// space-stripping match can prove neither. Rendered text never contains
// literal spaces (the space glyph advances but inks nothing, so ExtractText
// does not see it), which is the whole reason the mark is required.
func uiHas(content, want string) bool {
	return strings.Contains(content, want)
}

// --- Task 2: passphrase entry -------------------------------------------------

type ppEntryRun struct {
	dst  []byte
	n    int
	ok   bool
	done bool

	// The pass-proof trigger's sink, wired exactly as engravePassphraseFlow
	// wires it, so a sub-flow test exercises the real loader rather than a
	// stub that could not observe a partial write.
	seedFP, combinedFP string
}

func startPPEntry(t *testing.T) (*ppHarness, *ppEntryRun) {
	t.Helper()
	h := newPPHarness(t)
	r := &ppEntryRun{dst: make([]byte, passphrase.MaxLen)}
	load := ppPassProofLoader(r.dst, &r.n, &r.seedFP, &r.combinedFP)
	h.start(func() {
		r.n, r.ok = passphraseEntryFlow(h.ctx, &descriptorTheme, r.dst, 0, load)
		r.done = true
	})
	return h, r
}

// TestPassphraseEntryRequiresContent: the step cannot be completed empty.
// Tapping OK with nothing typed must keep the operator on the entry screen with
// an explanation, and typing then accepting must hand back exactly what was
// typed.
func TestPassphraseEntryRequiresContent(t *testing.T) {
	h, r := startPPEntry(t)
	h.tapWidget("ok")
	if r.done {
		t.Fatal("entry accepted an EMPTY passphrase")
	}
	if !uiContains(h.content, "Enter a passphrase") {
		t.Fatalf("empty OK gave no explanation; got %q", h.content)
	}
	h.tapNav(Button3) // dismiss the modal
	if !h.pump(8, "Passphrase") {
		t.Fatalf("did not return to the entry screen; got %q", h.content)
	}
	if r.done {
		t.Fatal("entry returned after dismissing the empty-passphrase message")
	}
	h.typeString("hi")
	h.tapWidget("ok")
	for i := 0; i < 8 && !r.done; i++ {
		h.frame()
	}
	if !r.done || !r.ok {
		t.Fatalf("entry did not accept %q (done=%v ok=%v)", "hi", r.done, r.ok)
	}
	if got := string(r.dst[:r.n]); got != "hi" {
		t.Fatalf("entry returned %q, want %q", got, "hi")
	}
}

// TestPassphraseEntryCounter: the counter is live and reflects what was typed.
// Asserting only the final value would pass against a hardcoded string, so the
// initial 0/100 is pinned too.
func TestPassphraseEntryCounter(t *testing.T) {
	h, _ := startPPEntry(t)
	if !uiHas(h.content, "0/100") {
		t.Fatalf("entry does not start at 0/100; got %q", h.content)
	}
	h.typeString("abc")
	if !uiHas(h.content, "3/100") {
		t.Fatalf("after typing 3 characters the counter is not 3/100; got %q", h.content)
	}
	h.typeString("d")
	if !uiHas(h.content, "4/100") {
		t.Fatalf("after typing 4 characters the counter is not 4/100; got %q", h.content)
	}
	if uiHas(h.content, "3/100") {
		t.Fatalf("the counter still shows a stale 3/100; got %q", h.content)
	}
}

// TestPassphraseEntryRejectsTooLong: 101 characters is refused, the operator
// stays on the entry screen, and -- the part that matters -- the refusal never
// echoes the secret (spec 5.3).
func TestPassphraseEntryRejectsTooLong(t *testing.T) {
	h, r := startPPEntry(t)
	const secret = "zq" // a digram that does not occur in any UI string
	h.typeString(secret)
	for i := 2; i < passphrase.MaxLen+1; i++ {
		h.typeRune('a')
	}
	if !uiHas(h.content, "101/100") {
		t.Fatalf("counter does not show the overflow; got %q", h.content)
	}
	h.tapWidget("ok")
	if r.done {
		t.Fatal("entry accepted a 101-character passphrase")
	}
	if !uiContains(h.content, "Too long") {
		t.Fatalf("over-length passphrase was not refused with an explanation; got %q", h.content)
	}
	if uiHas(h.content, secret) {
		t.Fatalf("the refusal echoed the passphrase; got %q", h.content)
	}
}

// assertAllKeysReachable checks every valid key on the keyboard's current page
// against h.point, which fails unless the key was drawn, sits on the panel, and
// is the topmost target at its own centre.
func (h *ppHarness) assertAllKeysReachable(kbd *PassphraseKeyboard, what string) {
	h.t.Helper()
	rows := kbd.pages[kbd.page]
	for i, row := range rows {
		for j := range row {
			key := row[j]
			if !kbd.Valid(key) {
				continue
			}
			label := key.label
			if label == "" {
				label = string(key.r)
			}
			if key.action == ppBackspace {
				label = "backspace"
			}
			h.point(&kbd.pages[kbd.page][i][j].clk,
				fmt.Sprintf("%s: page %d key %q", what, kbd.page, label))
		}
	}
}

// TestPassphraseKeyboardStaysOnPanel: at passphrase.MaxLen -- the longest
// passphrase the feature accepts -- every key must still be tappable, masked
// and revealed, on every page.
//
// It was not. PassphraseKeyboard.Layout laid the readout out at unbounded
// width and reported max(readout, grid) as its size while drawing the grid at
// x=0, so a caller centring on that size slid the GRID sideways: at 100
// characters the combined width was 600px and the grid's left edge landed at
// x=-60 on the 480px panel, taking q/a/z and the page-cycle key off the glass.
// A D-pad reaches a key drawn off the panel, so no button-driven test could
// ever have seen it.
func TestPassphraseKeyboardStaysOnPanel(t *testing.T) {
	h, _ := startPPEntry(t)
	kbd, ok := h.widget("kbd").(*PassphraseKeyboard)
	if !ok {
		t.Fatal("widget \"kbd\" is not a *PassphraseKeyboard")
	}
	for range passphrase.MaxLen {
		h.typeRune('W') // the widest glyph in the readout face
	}
	if !uiHas(h.content, "100/100") {
		t.Fatalf("did not reach a full-length passphrase; got %q", h.content)
	}
	for _, revealed := range []bool{false, true} {
		if kbd.revealed != revealed {
			rev := ppTagFor(kbd, func(k ppKey) bool { return k.action == ppReveal })
			if rev == nil {
				t.Fatal("no reveal key on the current page")
			}
			h.tapAt(h.point(rev, "reveal key"))
			h.next("after toggling reveal")
			if kbd.revealed != revealed {
				t.Fatalf("reveal toggle did not take effect (revealed=%v)", kbd.revealed)
			}
		}
		state := "masked"
		if revealed {
			state = "revealed"
		}
		for range ppPages {
			h.assertAllKeysReachable(kbd, state)
			cyc := ppTagFor(kbd, func(k ppKey) bool { return k.action == ppPageCycle })
			if cyc == nil {
				t.Fatalf("no page-cycle key on page %d", kbd.page)
			}
			h.tapAt(h.point(cyc, "page-cycle key"))
			h.next("after cycling page")
		}
	}
}

// --- Task 3: the two fingerprint steps ---------------------------------------

type ppFPRun struct {
	fp   string
	ok   bool
	done bool

	// The pass-proof trigger's sink, wired as engravePassphraseFlow wires it.
	secret             []byte
	n                  int
	seedFP, combinedFP string
}

func startPPFingerprint(t *testing.T, which ppFingerprintStep) (*ppHarness, *ppFPRun) {
	t.Helper()
	h := newPPHarness(t)
	r := &ppFPRun{secret: make([]byte, passphrase.MaxLen)}
	load := ppPassProofLoader(r.secret, &r.n, &r.seedFP, &r.combinedFP)
	h.start(func() {
		r.fp, r.ok = fingerprintEntryFlow(h.ctx, &descriptorTheme, which, "", load)
		r.done = true
	})
	return h, r
}

// TestFingerprintStepsSkippable: both fields are optional. Accepting an empty
// screen skips the field and leaves it EMPTY -- not "00000000", and not a
// refusal.
func TestFingerprintStepsSkippable(t *testing.T) {
	for _, which := range []ppFingerprintStep{ppSeedFP, ppCombinedFP} {
		t.Run(ppFingerprintTitle(which), func(t *testing.T) {
			h, r := startPPFingerprint(t, which)
			if !uiContains(h.content, "Optional") {
				t.Fatalf("the step does not say it is optional; got %q", h.content)
			}
			h.tapWidget("ok")
			for i := 0; i < 8 && !r.done; i++ {
				h.frame()
			}
			if !r.done || !r.ok {
				t.Fatalf("accepting an empty fingerprint did not skip the step (done=%v ok=%v)", r.done, r.ok)
			}
			if r.fp != "" {
				t.Fatalf("skipped fingerprint stored %q, want empty", r.fp)
			}
		})
	}
}

// TestFingerprintDisplayCanonical: the screen echoes the CANONICAL value, so
// what the operator proof-reads is what the plate will carry.
//
// The 4-and-4 grouping itself is not observable here: a rendered space inks
// nothing, so ExtractText never sees it (that is precisely why the passphrase
// needs a visible mark). What IS observable -- and what a missing
// canonicalisation would break -- is the case: typing lowercase must display
// uppercase. TestFingerprintPreviewString pins the grouping and the separator.
func TestFingerprintDisplayCanonical(t *testing.T) {
	h, _ := startPPFingerprint(t, ppSeedFP)
	h.typeString("a1b2c3d4")
	if !uiHas(h.content, "A1B2C3D4") {
		t.Fatalf("the screen does not echo the canonical fingerprint; got %q", h.content)
	}
}

// TestFingerprintPreviewString pins the exact string the fingerprint screens
// display. It is a helper test and says so -- it proves the grouping and the
// separator, not that any screen calls it; TestFingerprintDisplayCanonical
// covers the screen.
func TestFingerprintPreviewString(t *testing.T) {
	for _, tc := range []struct{ typed, want string }{
		{"", ""},
		{"a1b2c3d4", "A1B2 C3D4"},
		{"A1B2C3D4", "A1B2 C3D4"},
		{"a1b2 c3d4", "A1B2 C3D4"},
		{"1234567", ""},                          // incomplete: nothing to show yet
		{"12345678901234567890123456789012", ""}, // over-length: never shown
		{"G1B2C3D4", ""},                         // non-hex
	} {
		if got := ppFingerprintPreview(tc.typed); got != tc.want {
			t.Errorf("ppFingerprintPreview(%q) = %q, want %q", tc.typed, got, tc.want)
		}
	}
	// The separator is a plain 0x20 and NEVER the visible-space mark: the mark
	// means "a literal space in the passphrase", and a fingerprint has none.
	got := ppFingerprintPreview("a1b2c3d4")
	if !strings.Contains(got, " ") {
		t.Errorf("grouped fingerprint %q has no separator", got)
	}
	if strings.ContainsRune(got, backup.SpaceMark) {
		t.Errorf("grouped fingerprint %q uses the visible-space mark as a separator", got)
	}
}

// TestFingerprintRejectsBadInput: 7 digits and a non-hex character are refused
// and the step does not advance. A fail-open validator would let either through
// and put an unparseable claim on a permanent plate.
func TestFingerprintRejectsBadInput(t *testing.T) {
	for _, typed := range []string{"1234567", "G1B2C3D4", "123456789"} {
		t.Run(typed, func(t *testing.T) {
			h, r := startPPFingerprint(t, ppSeedFP)
			h.typeString(typed)
			h.tapWidget("ok")
			if r.done {
				t.Fatalf("the step accepted %q as a fingerprint (returned %q)", typed, r.fp)
			}
			if !uiContains(h.content, "8 hex digits") {
				t.Fatalf("refusing %q gave no explanation; got %q", typed, h.content)
			}
		})
	}
}

// TestFingerprintStoresCanonicalNotTyped is the guard for the precondition
// backup.Passphrase DOCUMENTS but does not enforce: SeedFP/CombinedFP are
// "canonical 8-hex-digit or empty". passphrase.GroupFingerprint fails OPEN --
// it returns anything that is not 8 characters unchanged -- so a raw typed
// string reaching the plate is engraved verbatim. Measured downstream
// consequence: a 32-hex-digit SeedFP renders the top band as an 82mm line,
// over spec §4.3's 64mm cap and through both corner screw-hole bands, with no
// error and no panic.
//
// So: non-canonical input must come back canonical, and input that cannot be
// canonicalised must not come back at all.
func TestFingerprintStoresCanonicalNotTyped(t *testing.T) {
	canonical := []struct{ typed, want string }{
		{"a1b2c3d4", "A1B2C3D4"},
		{"A1b2C3d4", "A1B2C3D4"},
		{"a1b2 c3d4", "A1B2C3D4"},
	}
	for _, tc := range canonical {
		t.Run("canonical/"+tc.typed, func(t *testing.T) {
			h, r := startPPFingerprint(t, ppSeedFP)
			h.typeString(tc.typed)
			h.tapWidget("ok")
			for i := 0; i < 8 && !r.done; i++ {
				h.frame()
			}
			if !r.done || !r.ok {
				t.Fatalf("the step refused %q (done=%v ok=%v)", tc.typed, r.done, r.ok)
			}
			if r.fp != tc.want {
				t.Fatalf("typed %q stored %q, want the canonical %q", tc.typed, r.fp, tc.want)
			}
		})
	}
	// The over-length case that produced the 82mm band.
	t.Run("refuses/32-hex", func(t *testing.T) {
		h, r := startPPFingerprint(t, ppSeedFP)
		h.typeString("0123456789ABCDEF0123456789ABCDEF")
		h.tapWidget("ok")
		if r.done {
			t.Fatalf("the step accepted a 32-digit fingerprint (returned %q)", r.fp)
		}
	})
}

// TestFingerprintStepFitsPanel: the warning must fit above the keyboard, and
// the heading (label plus the grouped value) must fit its band, on the panel
// the machine actually has. Text grows silently -- a longer warning is simply
// drawn under the keyboard, and a longer heading wraps down over the warning,
// neither of which anything else would notice -- so the budget is asserted
// from the same measurements the layout spends.
func TestFingerprintStepFitsPanel(t *testing.T) {
	p := newPlatform()
	p.display = sh2DisplaySize
	ctx := NewContext(p)
	dims := ctx.Platform.DisplaySize()
	kbd := NewAddressKeyboard(ctx)
	kbd.Fragment = "A1B2C3D4"
	_, kbdsz := kbd.Layout(ctx, &descriptorTheme)
	avail := dims.Y - leadingSize - ppBottomMargin - kbdsz.Y
	for _, which := range []ppFingerprintStep{ppSeedFP, ppCombinedFP} {
		name := ppFingerprintTitle(which)
		leadH := ctx.Styles.lead.Measure(dims.X-2*ppSideMargin, "%s", ppFingerprintLead(which)).Y
		if leadH > avail {
			t.Errorf("%s: the warning needs %dpx but only %dpx sits above the keyboard on a %v panel",
				name, leadH, avail, dims)
		}
		// layoutTitle wraps at width-32 and draws at y=8 inside the
		// leadingSize band.
		head := ppFingerprintHeading(which, "A1B2C3D4")
		headSz := ctx.Styles.title.Measure(dims.X-32, "%s", head)
		if headSz.Y > leadingSize-8 {
			t.Errorf("%s: heading %q is %dpx tall and wraps out of the %dpx title band",
				name, head, headSz.Y, leadingSize-8)
		}
	}
}

// --- Task 4: QR choice and the confirm screen ---------------------------------

type ppConfirmRun struct {
	ok   bool
	done bool
}

func startPPConfirm(t *testing.T, secret string, seedFP, combinedFP string, qr bool) (*ppHarness, *ppConfirmRun) {
	t.Helper()
	h := newPPHarness(t)
	r := new(ppConfirmRun)
	h.start(func() {
		r.ok = ppConfirmFlow(h.ctx, &descriptorTheme, []byte(secret), seedFP, combinedFP, qr)
		r.done = true
	})
	return h, r
}

// TestConfirmRendersSpacesVisibly: revealing the text is NOT enough. A space
// inks nothing -- on screen exactly as on metal -- so a revealed readout of
// " a b  c " proof-reads as "abc", and "hunter2 " proof-reads as "hunter2"
// while opening a different wallet.
//
// ExtractText models this precisely: it collects the runes of glyphs that
// actually inked, and a space never does. So asserting the marked form is
// present AND the unmarked form is not is a direct test of the property.
func TestConfirmRendersSpacesVisibly(t *testing.T) {
	const secret = " a b  c " // leading, interior, repeated and trailing
	h, _ := startPPConfirm(t, secret, "", "", false)
	if !uiHas(h.content, "_a_b__c_") {
		t.Fatalf("the confirm screen does not mark spaces; got %q", h.content)
	}
	if uiHas(h.content, "abc") {
		t.Fatalf("the confirm screen rendered raw spaces, which ink nothing: %q", h.content)
	}
	// The mark means nothing without a legend saying so.
	if !uiContains(h.content, "= SPACE") {
		t.Fatalf("the space mark is not explained; got %q", h.content)
	}
}

// TestConfirmMarksSurviveWrapping: a 100-character passphrase wraps, and
// text.Layout SWALLOWS the space it breaks a line at -- which would hide
// exactly the spaces adjacent to a break. Marking before layout is what makes
// wrapping safe, so this asserts the whole marked string is present even when
// it spans lines.
func TestConfirmMarksSurviveWrapping(t *testing.T) {
	secret := strings.Repeat("ab c", 25) // 100 chars, 25 spaces, spread across every line
	if len(secret) != passphrase.MaxLen {
		t.Fatalf("test vector is %d chars, want %d", len(secret), passphrase.MaxLen)
	}
	h, _ := startPPConfirm(t, secret, "", "", false)
	marked := strings.ReplaceAll(secret, " ", "_")
	if !uiHas(h.content, marked) {
		t.Fatalf("the wrapped passphrase lost characters or marks.\nwant %q\n got %q", marked, h.content)
	}
}

// TestConfirmShowsDerivedCounts: leading and trailing spaces are called out BY
// NAME, not folded into a total. A count is checkable against intent in a way
// a wall of characters is not.
func TestConfirmShowsDerivedCounts(t *testing.T) {
	h, _ := startPPConfirm(t, " a b  c ", "", "", false)
	// Rendered spaces ink nothing, so the extracted form of
	// "8 chars, 5 spaces, 1 leading, 1 trailing" has its spaces removed.
	const want = "8chars,5spaces,1leading,1trailing"
	if !uiHas(h.content, want) {
		t.Fatalf("derived counts missing or wrong.\nwant %q inside\n got %q", want, h.content)
	}
}

// TestPassphraseCountsString pins the count strings themselves, including the
// cases the screen test cannot reach one at a time. A single-fault table would
// let a mis-ordered or mis-labelled clause through, so each row varies more
// than one thing.
func TestPassphraseCountsString(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"a", "1 char, no spaces"},
		{"hunter2", "7 chars, no spaces"},
		{"hunter2 ", "8 chars, 1 space, 1 trailing"},
		{" hunter2", "8 chars, 1 space, 1 leading"},
		{" a b  c ", "8 chars, 5 spaces, 1 leading, 1 trailing"},
		{"a  b", "4 chars, 2 spaces"},
		{"  ab  ", "6 chars, 4 spaces, 2 leading, 2 trailing"},
	} {
		if got := ppPassphraseCounts([]byte(tc.in)); got != tc.want {
			t.Errorf("ppPassphraseCounts(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestConfirmShowsFingerprintsGrouped: both claims appear grouped as the plate
// groups them, both are labelled, and a skipped field says so rather than
// vanishing -- a missing line is indistinguishable from a screen that forgot to
// draw it.
func TestConfirmShowsFingerprintsGrouped(t *testing.T) {
	h, _ := startPPConfirm(t, "hunter2", "A1B2C3D4", "99887766", false)
	for _, want := range []string{"A1B2C3D4", "99887766"} {
		if !uiHas(h.content, want) {
			t.Fatalf("confirm screen omits fingerprint %q; got %q", want, h.content)
		}
	}
	if !uiContains(h.content, "not verified") {
		t.Fatalf("confirm screen does not warn the fingerprints are unverified; got %q", h.content)
	}
	h2, _ := startPPConfirm(t, "hunter2", "", "", false)
	if !uiContains(h2.content, "not recorded") {
		t.Fatalf("a skipped fingerprint is not called out; got %q", h2.content)
	}
}

// TestConfirmShowsQRState: whether the plate carries a machine-readable copy of
// the secret is the most consequential thing on it, so the confirm screen says
// which way the toggle landed.
func TestConfirmShowsQRState(t *testing.T) {
	on, _ := startPPConfirm(t, "hunter2", "", "", true)
	if !uiContains(on.content, "QR: yes") {
		t.Fatalf("confirm screen does not report the QR is ON; got %q", on.content)
	}
	off, _ := startPPConfirm(t, "hunter2", "", "", false)
	if !uiContains(off.content, "QR: no") {
		t.Fatalf("confirm screen does not report the QR is OFF; got %q", off.content)
	}
}

// TestConfirmFitsPanel: the confirm screen must fit WITHOUT scrolling. The
// codebase's only scroller (Warning.Layout) is bound to ButtonFilter(Up/Down),
// which no production path on SeedHammer II emits -- content below the fold
// would be unreadable on the machine. Measured against the worst case the
// feature accepts: 100 characters with spaces, both fingerprints, QR on.
func TestConfirmFitsPanel(t *testing.T) {
	p := newPlatform()
	p.display = sh2DisplaySize
	ctx := NewContext(p)
	dims := ctx.Platform.DisplaySize()
	area := ppConfirmArea(dims)
	worst := []byte(strings.Repeat("ab c", 25))
	marked := make([]byte, len(worst))
	ppMarkSpaces(marked, worst)
	_, sz := ppConfirmBody(ctx, &descriptorTheme, area.Dx(), marked,
		ppPassphraseCounts(worst), "A1B2C3D4", "99887766", true, true)
	if sz.Y > area.Dy() {
		t.Errorf("the confirm screen needs %dpx of height but only %dpx is available on a %v panel; "+
			"the overflow would be unreadable, because the scroller is bound to buttons the machine does not have",
			sz.Y, area.Dy(), dims)
	}
	if sz.X > area.Dx() {
		t.Errorf("the confirm screen is %dpx wide in a %dpx area", sz.X, area.Dx())
	}
}

// TestQRChoiceDefaultsOff: accepting the QR step without touching anything must
// leave the QR OFF (spec D8). A default that silently added a machine-readable
// copy of the secret to the plate would be the worst possible way to get this
// wrong.
func TestQRChoiceDefaultsOff(t *testing.T) {
	h := newPPHarness(t)
	var qr, ok, done bool
	h.start(func() {
		qr, ok = ppQRChoiceFlow(h.ctx, &descriptorTheme, false)
		done = true
	})
	if !uiContains(h.content, "machine-readable") {
		t.Fatalf("the QR step does not say the QR is a machine-readable copy of the secret; got %q", h.content)
	}
	h.tapNav(Button3)
	for i := 0; i < 8 && !done; i++ {
		h.frame()
	}
	if !done || !ok {
		t.Fatalf("the QR step did not complete (done=%v ok=%v)", done, ok)
	}
	if qr {
		t.Fatal("the QR defaults to ON; spec D8 requires OFF")
	}
}

// TestQRChoiceCanBeTurnedOn: the opt-in is reachable by touch. Without this,
// "defaults off" would also pass on a screen where the other choice cannot be
// selected at all.
func TestQRChoiceCanBeTurnedOn(t *testing.T) {
	h := newPPHarness(t)
	var qr, ok, done bool
	h.start(func() {
		qr, ok = ppQRChoiceFlow(h.ctx, &descriptorTheme, false)
		done = true
	})
	cs, isCS := h.widget("qr").(*ChoiceScreen)
	if !isCS {
		t.Fatal("widget \"qr\" is not a *ChoiceScreen")
	}
	if len(cs.children) < 2 {
		t.Fatalf("the QR step offers %d choices, want 2", len(cs.children))
	}
	h.tapAt(h.point(&cs.children[1].click, "the \"add QR\" choice"))
	h.next("after choosing to add a QR")
	h.tapNav(Button3)
	for i := 0; i < 8 && !done; i++ {
		h.frame()
	}
	if !done || !ok {
		t.Fatalf("the QR step did not complete (done=%v ok=%v)", done, ok)
	}
	if !qr {
		t.Fatal("choosing the QR option did not turn the QR on")
	}
}

// --- Tasks 5 and 6: the whole flow, by touch, and what happens to the secret --

// ppFlowRun drives engravePassphraseFlow end to end by touch. Nothing here
// synthesizes a ButtonEvent: every step is a PointerEvent aimed at a hit area
// read back from the frame that was drawn.
type ppFlowRun struct {
	h        *ppHarness
	engraver *testEngraver
	platform *testPlatform
	secret   []byte // the flow's own buffer, captured for the wipe assertions
	done     bool
}

func startPPFlow(t *testing.T) *ppFlowRun {
	t.Helper()
	p := newPlatform()
	p.display = sh2DisplaySize
	e := newEngraver()
	p.engraver = e
	h := &ppHarness{t: t, ctx: NewContext(p), widgets: make(map[string]any)}
	passphraseWidgetHook = func(name string, w any) { h.widgets[name] = w }
	r := &ppFlowRun{h: h, engraver: e, platform: p}
	passphraseSecretHook = func(secret []byte) { r.secret = secret }
	t.Cleanup(func() {
		passphraseWidgetHook = nil
		passphraseSecretHook = nil
	})
	h.start(func() {
		engravePassphraseFlow(h.ctx, &descriptorTheme)
		r.done = true
	})
	return r
}

// enterPassphrase types the passphrase and accepts it.
func (r *ppFlowRun) enterPassphrase(s string) {
	r.h.t.Helper()
	r.h.typeString(s)
	r.h.tapWidget("ok")
}

// enterFingerprint types a fingerprint (or nothing, to skip) and accepts it.
func (r *ppFlowRun) enterFingerprint(s string) {
	r.h.t.Helper()
	if s != "" {
		r.h.typeString(s)
	}
	r.h.tapWidget("ok")
}

// chooseQR accepts the QR step, optionally selecting the opt-in first.
func (r *ppFlowRun) chooseQR(add bool) {
	r.h.t.Helper()
	if add {
		cs, ok := r.h.widget("qr").(*ChoiceScreen)
		if !ok {
			r.h.t.Fatal("widget \"qr\" is not a *ChoiceScreen")
		}
		r.h.tapAt(r.h.point(&cs.children[1].click, "the \"add QR\" choice"))
		r.h.next("after choosing to add a QR")
	}
	r.h.tapNav(Button3)
}

// hold presses a nav button and holds it past confirmDelay, which is how the
// engrave screen is armed. Requires synctest for the sleep.
func (r *ppFlowRun) hold(b Button) {
	r.h.t.Helper()
	pos := r.h.navPoint(b)
	d := r.h.drawer()
	tag, _, hit := d.Hit(pos)
	if !hit {
		r.h.t.Fatalf("hold %v: no touch target at %v", b, pos)
	}
	if c, ok := tag.(*Clickable); !ok || (c.Button != b && c.AltButton != b) {
		r.h.t.Fatalf("hold %v: the target at %v is %v", b, pos, tag)
	}
	r.h.ctx.Router.Events(d, PointerEvent{Pressed: true, Entered: true, Pos: pos}.Event())
	r.h.step()
	time.Sleep(confirmDelay)
	r.h.step()
}

// engrave drives the engrave screen to a completed job and dismisses it, which
// is the only path that makes the flow return true and exit.
func (r *ppFlowRun) engrave() {
	r.h.t.Helper()
	r.h.mustReach("Engrave Plate")
	r.hold(Button3)
loop:
	for i := 0; i < 4096; i++ {
		r.h.step()
		select {
		case <-r.engraver.closes:
			break loop
		case <-r.platform.wakeups:
		}
	}
	// While the job runs, the nav column carries only a Back button; the
	// dismiss button appears with the completion frame.
	r.h.mustReach("completed successfully")
	r.h.tapNav(Button3) // dismiss the success screen
	synctest.Wait()
	for i := 0; i < 64 && !r.done; i++ {
		r.h.frame()
	}
}

// TestPassphraseFlowByTouchEndToEnd is the Task 6 requirement: every step of
// the flow -- entry, both fingerprints, the QR choice, confirm and engrave --
// completed by PointerEvent alone, on the panel the machine has. If any step
// could only be driven by a button, it would fail here, because no step below
// sends one.
func TestPassphraseFlowByTouchEndToEnd(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := startPPFlow(t)
		if !uiContains(r.h.content, "Passphrase") {
			t.Fatalf("the flow did not open on the passphrase step; got %q", r.h.content)
		}
		r.enterPassphrase("a b")
		r.h.mustReach("Seed FP")
		r.enterFingerprint("a1b2c3d4")
		r.h.mustReach("Expected Comb FP")
		r.enterFingerprint("99887766")
		r.h.mustReach("QR Code")
		r.chooseQR(true)
		r.h.mustReach("Confirm")
		// The confirm screen must carry everything collected on the way here.
		for _, want := range []string{"a_b", "A1B2C3D4", "99887766"} {
			if !uiHas(r.h.content, want) {
				t.Fatalf("the confirm screen lost %q; got %q", want, r.h.content)
			}
		}
		if !uiContains(r.h.content, "QR: yes") {
			t.Fatalf("the confirm screen lost the QR choice; got %q", r.h.content)
		}
		r.h.tapWidget("ok")
		r.engrave()
		if !r.done {
			t.Fatal("the flow did not return after a completed engrave")
		}
	})
}

// TestPassphraseFlowWipesSecretOnExit: the []byte the flow accumulates into is
// zeroed once it returns from a completed engrave (spec 5.3).
func TestPassphraseFlowWipesSecretOnExit(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := startPPFlow(t)
		r.enterPassphrase("hunter2")
		r.h.mustReach("Seed FP")
		r.enterFingerprint("")
		r.h.mustReach("Expected Comb FP")
		r.enterFingerprint("")
		r.h.mustReach("QR Code")
		r.chooseQR(false)
		r.h.mustReach("Confirm")
		if !uiHas(r.h.content, "hunter2") {
			t.Fatalf("the passphrase never reached the confirm screen; got %q", r.h.content)
		}
		// Prove the buffer under test is the one actually holding the secret,
		// so the wipe assertion below cannot pass against a buffer that was
		// always zero.
		assertHolds(t, r.secret, "hunter2")
		r.h.tapWidget("ok")
		r.engrave()
		if !r.done {
			t.Fatal("the flow did not return after a completed engrave")
		}
		assertWiped(t, r.secret, "after a completed engrave")
	})
}

// TestPassphraseFlowWipesSecretOnAbort: the same buffer is zeroed when the
// operator backs out instead of engraving. An abort is the likelier exit, and
// a wipe that only runs on success is the easiest one to get wrong.
func TestPassphraseFlowWipesSecretOnAbort(t *testing.T) {
	r := startPPFlow(t)
	r.enterPassphrase("hunter2")
	r.h.mustReach("Seed FP")
	r.enterFingerprint("a1b2c3d4")
	r.h.mustReach("Expected Comb FP")
	r.enterFingerprint("")
	r.h.mustReach("QR Code")
	r.chooseQR(false)
	r.h.mustReach("Confirm")
	if !uiHas(r.h.content, "hunter2") {
		t.Fatalf("the passphrase never reached the confirm screen; got %q", r.h.content)
	}
	assertHolds(t, r.secret, "hunter2")
	// Back out one step at a time, all the way off the program.
	for i := 0; i < 5 && !r.done; i++ {
		r.h.tapNav(Button1)
		r.h.step()
	}
	for i := 0; i < 16 && !r.done; i++ {
		r.h.frame()
	}
	if !r.done {
		t.Fatalf("backing out of every step did not leave the program; last frame %q", r.h.content)
	}
	assertWiped(t, r.secret, "after an abort")
}

// assertHolds fails unless secret currently contains want. Without it, a wipe
// test would pass just as happily against a buffer nothing was ever written to.
func assertHolds(t *testing.T, secret []byte, want string) {
	t.Helper()
	if !bytes.Contains(secret, []byte(want)) {
		t.Fatalf("the flow's secret buffer does not hold the passphrase; it is the wrong buffer to be asserting a wipe on")
	}
}

func assertWiped(t *testing.T, secret []byte, when string) {
	t.Helper()
	if secret == nil {
		t.Fatal("the flow never handed its secret buffer to the test")
	}
	if len(secret) != passphrase.MaxLen {
		t.Fatalf("the secret buffer is %d bytes, want %d", len(secret), passphrase.MaxLen)
	}
	for i, b := range secret {
		if b != 0 {
			t.Fatalf("secret[%d] = %#x %s -- the buffer was not wiped", i, b, when)
		}
	}
}

// TestPassphraseFlowNeverLogs captures everything the flow writes to the log
// while a full engrave runs and asserts the passphrase is not in it. The
// passphrase is a distinctive string that occurs in no UI text, so a match
// could only come from the secret itself.
func TestPassphraseFlowNeverLogs(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const secret = "zqxjv7"
		var logbuf bytes.Buffer
		out, flags, prefix := log.Writer(), log.Flags(), log.Prefix()
		log.SetOutput(&logbuf)
		t.Cleanup(func() {
			log.SetOutput(out)
			log.SetFlags(flags)
			log.SetPrefix(prefix)
		})
		r := startPPFlow(t)
		// Force the refusal paths too: they are the ones most likely to
		// interpolate the input into a message.
		r.h.tapWidget("ok") // empty -> refused
		r.h.mustReach("Enter a passphrase")
		r.h.tapNav(Button3) // dismiss
		r.h.mustReach("0/100")
		r.enterPassphrase(secret)
		r.h.mustReach("Seed FP")
		r.h.typeString("zzz") // not a fingerprint -> refused
		r.h.tapWidget("ok")
		r.h.mustReach("8 hex digits")
		r.h.tapNav(Button3) // dismiss
		r.h.mustReach("Seed FP")
		r.h.backspace(3) // clear the rejected input
		r.enterFingerprint("a1b2c3d4")
		r.h.mustReach("Expected Comb FP")
		r.enterFingerprint("")
		r.h.mustReach("QR Code")
		r.chooseQR(true)
		r.h.mustReach("Confirm")
		r.h.tapWidget("ok")
		r.engrave()
		if !r.done {
			t.Fatal("the flow did not return after a completed engrave")
		}
		if strings.Contains(logbuf.String(), secret) {
			t.Fatalf("the passphrase reached the log: %q", logbuf.String())
		}
	})
}

// TestPassphrasePlateBuildsWorstCase: the flow must be able to engrave the
// worst case it accepts. If it cannot, the operator types 100 characters, both
// fingerprints and a QR, proof-reads the confirm screen, and is then told the
// passphrase "does not fit a plate" -- with nothing to do about it.
func TestPassphrasePlateBuildsWorstCase(t *testing.T) {
	secret := []byte(strings.Repeat("ab c", 25)) // 100 chars, spaces throughout
	if len(secret) != passphrase.MaxLen {
		t.Fatalf("test vector is %d chars, want %d", len(secret), passphrase.MaxLen)
	}
	for _, qr := range []bool{false, true} {
		plate, err := ppBuildPlate(engraverParams, secret, "A1B2C3D4", "99887766", qr)
		if err != nil {
			t.Fatalf("qr=%v: the worst case the flow accepts does not build: %v", qr, err)
		}
		if plate.Duration == 0 {
			t.Fatalf("qr=%v: built an empty plate", qr)
		}
	}
}

// TestPassphrasePlateCarriesEveryField: each field the flow collects must
// actually reach backup.Passphrase. Engraving is constant-time for a given
// length, so a field that changed nothing about the plan would be a field that
// was dropped on the way.
func TestPassphrasePlateCarriesEveryField(t *testing.T) {
	build := func(t *testing.T, secret, seedFP, combFP string, qr bool) uint64 {
		t.Helper()
		p, err := ppBuildPlate(engraverParams, []byte(secret), seedFP, combFP, qr)
		if err != nil {
			t.Fatalf("build(%q, %q, %q, qr=%v): %v", secret, seedFP, combFP, qr, err)
		}
		return p.Duration
	}
	base := build(t, "hunter2", "", "", false)
	if d := build(t, "hunter2xy", "", "", false); d == base {
		t.Error("a longer passphrase engraves identically: the passphrase is not reaching the plate")
	}
	if d := build(t, "hunter2", "A1B2C3D4", "", false); d == base {
		t.Error("the seed fingerprint is not reaching the plate")
	}
	if d := build(t, "hunter2", "", "99887766", false); d == base {
		t.Error("the combined fingerprint is not reaching the plate")
	}
	if d := build(t, "hunter2", "", "", true); d == base {
		t.Error("the QR choice is not reaching the plate")
	}
	// The two fingerprints are distinct fields, not one written twice.
	if a, b := build(t, "hunter2", "A1B2C3D4", "", false), build(t, "hunter2", "", "A1B2C3D4", false); a == b {
		t.Error("the two fingerprint fields produce the same plate: they are wired to the same slot")
	}
}

// TestConfirmLegendGatesOnRealSpaces pins Phase D review I1.
//
// The legend was gated on the SUBSTITUTED buffer, which carries the mark for a
// literal '_' exactly as it does for a space. So "hunter_2" confirmed as
// `hunter_2 / 8 chars, no spaces / _ = SPACE` -- asserting there are no spaces
// while simultaneously claiming every '_' is one, and promising a legend the
// plate does not engrave (backup gates on a real space).
func TestConfirmLegendGatesOnRealSpaces(t *testing.T) {
	p := newPlatform()
	p.display = sh2DisplaySize
	ctx := NewContext(p)
	area := ppConfirmArea(ctx.Platform.DisplaySize())
	for _, tc := range []struct {
		secret string
		want   bool
	}{
		{"hunter_2", false}, // underscores only -- NO legend
		{"a b", true},       // a real space -- legend
		{"a_b c_d", true},   // both -- legend
		{"plain", false},    // neither
	} {
		secret := []byte(tc.secret)
		marked := make([]byte, len(secret))
		ppMarkSpaces(marked, secret)
		body, _ := ppConfirmBody(ctx, &descriptorTheme, area.Dx(), marked,
			ppPassphraseCounts(secret), "", "", false,
			bytes.IndexByte(secret, ' ') >= 0)
		// The body is returned un-offset, so extract over a generous rect
		// rather than the panel -- otherwise text drawn outside it is missed
		// and every case reads as "no legend", which would pass the two cases
		// this test exists to catch and hide the other two.
		d := new(op.Drawer)
		// Match against the SPACE-STRIPPED legend: a rendered space inks
		// nothing, so ExtractText yields "_=SPACE", never "_ = SPACE".
		// Comparing to the raw constant reads as "no legend" for every case,
		// which would pass the two rows this test exists to catch.
		got := uiHas(d.ExtractText(image.Rect(-2000, -2000, 2000, 2000), body),
			strings.ReplaceAll(ppSpaceLegend, " ", ""))
		if got != tc.want {
			t.Errorf("%q: legend shown = %v, want %v", tc.secret, got, tc.want)
		}
	}
}

// TestBackPreservesEnteredValues pins Phase D review M2.
//
// Stepping Back from Confirm to fix a typo and tapping forward again used to
// clear a fingerprint already entered and reset the QR to off, because both
// screens were rebuilt empty. A cleared field is indistinguishable from one
// never filled, and it contradicts the flow's own promise that Back walks
// backwards rather than abandoning.
// The version of this test that shipped with the M2 fix asserted
// GroupFingerprint("DEADBEEF") == "DEAD BEEF" and that a ChoiceScreen whose
// choice field was set to 1 still held 1. It invoked neither flow, and the
// whole-feature review proved it vacuous by deleting BOTH M2 fixes and watching
// the entire gui suite stay green. This drives the real flow instead: back up
// two steps from Confirm, walk forward again, and require every value to have
// survived the round trip.
func TestBackPreservesEnteredValues(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := startPPFlow(t)
		r.enterPassphrase("a b")
		r.h.mustReach("Seed FP")
		r.enterFingerprint("DEADBEEF")
		r.h.mustReach("Expected Comb FP")
		r.enterFingerprint("CAFEBABE")
		r.h.mustReach("QR Code")
		r.chooseQR(true)
		r.h.mustReach("Confirm")

		// Back to the QR step. The prior opt-in must still be selected --
		// re-showing the screen defaulted to "No QR", which silently drops a
		// QR the operator had already asked for.
		r.h.tapNav(Button1)
		r.h.mustReach("QR Code")
		cs, ok := r.h.widget("qr").(*ChoiceScreen)
		if !ok {
			t.Fatal("widget \"qr\" is not a *ChoiceScreen")
		}
		if cs.choice != 1 {
			t.Errorf("stepping Back to the QR screen reset the choice to %d; the "+
				"operator had opted in, and index 0 is \"No QR\"", cs.choice)
		}

		// Back once more, to the combined fingerprint. Its field must still
		// hold what was typed: a cleared field is indistinguishable from one
		// deliberately left blank.
		r.h.tapNav(Button1)
		r.h.mustReach("Expected Comb FP")
		if !uiHas(r.h.content, "CAFEBABE") {
			t.Errorf("stepping Back to the combined fingerprint lost it; got %q", r.h.content)
		}

		// Forward again, touching nothing: every value must arrive at Confirm.
		r.h.tapWidget("ok")
		r.h.mustReach("QR Code")
		r.h.tapNav(Button3)
		r.h.mustReach("Confirm")
		for _, want := range []string{"a_b", "DEADBEEF", "CAFEBABE"} {
			if !uiHas(r.h.content, want) {
				t.Errorf("after Back and forward the confirm screen lost %q; got %q",
					want, r.h.content)
			}
		}
		if !uiContains(r.h.content, "QR: yes") {
			t.Errorf("after Back and forward the confirm screen lost the QR opt-in; got %q",
				r.h.content)
		}
	})
}

// TestPassphraseEntryFitsPanel is the test gui/passphrase_flow.go's counter
// comment claimed existed. It did not -- the whole-feature review found the
// citation dangling, and measuring the layout showed the cited fix was a NO-OP:
// the keyboard block is bottom-aligned (Max.Y-size.Y) and CutTop leaves Max.Y
// unchanged, so reserving the counter's band did not move the block by a pixel.
// The occlusion the fix claimed to remove was still there, at exactly the
// lengths its own commit message named -- covering the counter from ~70
// characters and the title past ~90.
//
// It must measure RECTANGLES. op.Drawer.ExtractText collects runes from every
// drawn text op regardless of occlusion, so a text assertion reads "101/100"
// perfectly well while the keyboard is drawn on top of it by op.Layer. That is
// why the defect survived a phase review, a fix, and an integration review.
func TestPassphraseEntryFitsPanel(t *testing.T) {
	// 100 = MaxLen; 101 = the over-length signal, the state whose counter
	// matters most and the tallest the readout ever gets. Revealed only:
	// a masked readout is shorter, so it never exercises the bug.
	for _, n := range []int{70, 90, passphrase.MaxLen, passphrase.MaxLen + 1} {
		h := newPPHarness(t)
		dst := make([]byte, passphrase.MaxLen+8)
		// nil loader: ppPassProofOffer is inert without one, so the pass-proof
		// trigger cannot interfere with a pure layout measurement.
		h.start(func() { passphraseEntryFlow(h.ctx, &descriptorTheme, dst, 0, nil) })

		kbd, ok := h.widget("kbd").(*PassphraseKeyboard)
		if !ok {
			t.Fatal("widget \"kbd\" is not a *PassphraseKeyboard")
		}
		kbd.Fragment = strings.Repeat("W", n)
		kbd.revealed = true
		content := h.next("after filling to %d chars revealed", n)

		// The flow must BOUND the block, not merely reserve around it.
		if kbd.MaxHeight <= 0 {
			t.Fatalf("%d chars: the entry flow left the keyboard block unbounded "+
				"(MaxHeight=%d); the readout is free to grow over the counter and title",
				n, kbd.MaxHeight)
		}
		_, kbdsz := kbd.Layout(h.ctx, &descriptorTheme)
		if kbdsz.Y > kbd.MaxHeight {
			t.Errorf("%d chars revealed: the keyboard block is %d tall but only %d was "+
				"available below the counter -- it overflows upward by %d and op.Layer "+
				"draws it ON TOP, covering the %d/%d counter and the over-length signal "+
				"in exactly the state a user proof-reads in",
				n, kbdsz.Y, kbd.MaxHeight, kbdsz.Y-kbd.MaxHeight, n, passphrase.MaxLen)
		}
		// The clamp must keep the counter legible WITHOUT emptying the readout:
		// dropping every rune would satisfy the bound and show the operator
		// nothing of what they typed.
		// A RUN, not a single "W": every key cap is drawn too, so a one-character
		// check matches the keyboard itself and passes even when the readout is
		// empty. Ten in a row can only have come from the readout.
		if !uiContains(content, strings.Repeat("W", 10)) {
			t.Errorf("%d chars revealed: the readout shows no run of passphrase text; "+
				"the height clamp emptied it instead of keeping the tail. frame=%q", n, content)
		}
		if !uiContains(content, fmt.Sprintf("%d/%d", n, passphrase.MaxLen)) {
			t.Errorf("%d chars: the counter is not drawn at all; got %q", n, content)
		}
	}
}
