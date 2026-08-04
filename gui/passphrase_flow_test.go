package gui

import (
	"fmt"
	"image"
	"strings"
	"testing"

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
}

func startPPEntry(t *testing.T) (*ppHarness, *ppEntryRun) {
	t.Helper()
	h := newPPHarness(t)
	r := &ppEntryRun{dst: make([]byte, passphrase.MaxLen)}
	h.start(func() {
		r.n, r.ok = passphraseEntryFlow(h.ctx, &descriptorTheme, r.dst, 0)
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
}

func startPPFingerprint(t *testing.T, which ppFingerprintStep) (*ppHarness, *ppFPRun) {
	t.Helper()
	h := newPPHarness(t)
	r := new(ppFPRun)
	h.start(func() {
		r.fp, r.ok = fingerprintEntryFlow(h.ctx, &descriptorTheme, which)
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
		ppPassphraseCounts(worst), "A1B2C3D4", "99887766", true)
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
		qr, ok = ppQRChoiceFlow(h.ctx, &descriptorTheme)
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
		qr, ok = ppQRChoiceFlow(h.ctx, &descriptorTheme)
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
