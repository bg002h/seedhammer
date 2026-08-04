package gui

import (
	"fmt"
	"image"
	"strings"
	"testing"

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
	h.next("after tapping nav %v", b)
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
		r.n, r.ok = passphraseEntryFlow(h.ctx, &descriptorTheme, r.dst)
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
