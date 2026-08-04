package gui

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"seedhammer.com/font/sh"
	"seedhammer.com/gui/assets"
	"seedhammer.com/gui/layout"
	"seedhammer.com/gui/op"
	"seedhammer.com/gui/widget"
)

// funcRow returns the keyboard's function row on page p.
func funcRow(k *PassphraseKeyboard, p int) []ppKey {
	rows := k.pages[p]
	return rows[len(rows)-1]
}

// TestTextKeyboardConstruction: five function-row keys, the newline APPENDED
// last so the reveal key keeps index 2, on every page.
func TestTextKeyboardConstruction(t *testing.T) {
	ctx := NewContext(newPlatform())
	k := NewTextKeyboard(ctx)
	for p := range len(ppPages) {
		fr := funcRow(k, p)
		if len(fr) != 5 {
			t.Fatalf("page %d function row: %d keys, want 5 (page-cycle/space/reveal/backspace/newline)", p, len(fr))
		}
		want := []ppAction{ppPageCycle, ppRune, ppReveal, ppBackspace, ppRune}
		for i, a := range want {
			if fr[i].action != a {
				t.Errorf("page %d funcrow[%d].action = %v, want %v", p, i, fr[i].action, a)
			}
		}
		// The reveal key stays at index 2 -- passphrase_keyboard_test.go:200
		// depends on it.
		if fr[2].action != ppReveal {
			t.Errorf("page %d: reveal key is no longer at index 2", p)
		}
		nl := fr[4]
		if nl.r != '\n' {
			t.Errorf("page %d newline key r = %q, want '\\n'", p, nl.r)
		}
		if nl.label != "nl" {
			t.Errorf("page %d newline key label = %q, want \"nl\"", p, nl.label)
		}
	}
	// It types a newline, not a literal "nl".
	k.commit(funcRow(k, 0)[4])
	if k.Fragment != "\n" {
		t.Errorf("committing the newline key produced %q, want \"\\n\"", k.Fragment)
	}
}

// TestNewlineKeyAbsentFromEveryOtherKeyboard is the anti-leak half.
// passphrase.ValidatePassphrase rejects '\n' with ErrNonASCII, so a newline key
// on the passphrase keyboard would type something the operator can see and OK
// would then refuse. NewAddressKeyboard and BIP-85 index entry share the same
// widget.
func TestNewlineKeyAbsentFromEveryOtherKeyboard(t *testing.T) {
	ctx := NewContext(newPlatform())
	for _, tc := range []struct {
		name string
		k    *PassphraseKeyboard
	}{
		{"NewPassphraseKeyboard", NewPassphraseKeyboard(ctx)},
		{"NewAddressKeyboard", NewAddressKeyboard(ctx)},
	} {
		for p := range len(ppPages) {
			if got := len(funcRow(tc.k, p)); got != 4 {
				t.Errorf("%s page %d: %d function keys, want 4", tc.name, p, got)
			}
			for _, row := range tc.k.pages[p] {
				for _, key := range row {
					if key.r == '\n' {
						t.Errorf("%s page %d carries a newline key", tc.name, p)
					}
				}
			}
		}
	}
}

// TestNewlineLabelHasATapTarget. "\u21b5" measures 0px in ctx.Styles.keyboard,
// which leaves a keyPadX-only tap target -- and a synthetic touch test aimed at
// the centre of that target still passes, because the centre is on the panel.
// So the width is asserted directly.
func TestNewlineLabelHasATapTarget(t *testing.T) {
	ctx := NewContext(newPlatform())
	k := NewTextKeyboard(ctx)
	nl := funcRow(k, 0)[4]
	bs := funcRow(k, 0)[3]
	if nl.size.X < bs.size.X {
		t.Errorf("newline key glyph extent is %dpx, narrower than the backspace key's %dpx -- the smallest target already on this keyboard", nl.size.X, bs.size.X)
	}
	// Measured when "nl" was chosen: "nl" 24px, "W" 26px, "enter" 68px,
	// "return" 81px, and U+21B5 exactly 0. If the arrow ever starts measuring,
	// that is a change of face, not a licence to use it.
	if got := ctx.Styles.keyboard.Measure(1<<30, "%s", "\u21b5").X; got != 0 {
		t.Logf("note: U+21B5 now measures %dpx; it measured 0 when \"nl\" was chosen", got)
	}
	if got := ctx.Styles.keyboard.Measure(1<<30, "%s", "nl").X; got != 24 {
		t.Errorf("\"nl\" measures %dpx, not the 24px the row width was sized from", got)
	}
	// The whole function row, pinned: 285px on page 0 against a 480px panel.
	if got := ppRowWidth(funcRow(k, 0), 2); got != 285 {
		t.Errorf("free-text function row is %dpx, want the measured 285px", got)
	}
	if got := ppRowWidth(funcRow(NewPassphraseKeyboard(ctx), 0), 2); got != 253 {
		t.Errorf("passphrase function row is %dpx, want the measured 253px", got)
	}
}

// TestTextKeyboardFunctionRowFitsPanel: adding a fifth key must not push the
// function row wider than the 480px panel, or the outermost keys leave the
// glass and touch -- the only input the machine has -- cannot reach them.
func TestTextKeyboardFunctionRowFitsPanel(t *testing.T) {
	ctx := NewContext(newPlatform())
	for _, tc := range []struct {
		name string
		k    *PassphraseKeyboard
	}{
		{"passphrase", NewPassphraseKeyboard(ctx)},
		{"free text", NewTextKeyboard(ctx)},
	} {
		for p := range len(ppPages) {
			if got := ppRowWidth(funcRow(tc.k, p), 2); got > sh2DisplaySize.X {
				t.Errorf("%s page %d function row is %dpx wide, past the %dpx panel", tc.name, p, got, sh2DisplaySize.X)
			}
			if tc.k.size[p].X > sh2DisplaySize.X {
				t.Errorf("%s page %d grid is %dpx wide, past the %dpx panel", tc.name, p, tc.k.size[p].X, sh2DisplaySize.X)
			}
		}
	}
}

// tkScreen lays a keyboard out the way passphraseEntryFlow does -- bottom
// aligned under a title, height-bounded -- so a reachability test exercises the
// real Layout path rather than a geometry calculation.
func tkScreen(ctx *Context, k *PassphraseKeyboard) {
	for !ctx.Done {
		for k.Update(ctx) {
		}
		dims := ctx.Platform.DisplaySize()
		screen := layout.Rectangle{Max: dims}
		_, content := screen.CutTop(leadingSize)
		content, _ = content.CutBottom(8)
		cntOp, cntsz := widget.Labelf(&ctx.B, ctx.Styles.subtitle, descriptorTheme.Text, "%d", len(k.Fragment))
		counterBand, content := content.CutTop(cntsz.Y)
		cntOp = cntOp.Offset(counterBand.N(cntsz))
		k.MaxHeight = content.Dy()
		kbdOp, kbdsz := k.Layout(ctx, &descriptorTheme)
		kbdOp = kbdOp.Offset(content.S(kbdsz))
		nav, _ := layoutNavigation(&ctx.B, &descriptorTheme, dims, []NavButton{
			{Clickable: &Clickable{Button: Button1}, Style: StyleSecondary, Icon: assets.IconBack},
		}...)
		title, _ := layoutTitle(ctx, dims.X, descriptorTheme.Text, "Text")
		ctx.Frame(op.Layer(kbdOp, cntOp, nav, title, op.Color(&ctx.B, descriptorTheme.Background)))
	}
}

// TestTextKeyboardEveryKeyReachableByTouch. SeedHammer II has no directional
// buttons: a key drawn off the panel or under another target is simply gone.
// Checked on every page, at the real 480x320 panel, with a readout long enough
// to be the tallest the block ever gets.
func TestTextKeyboardEveryKeyReachableByTouch(t *testing.T) {
	h := newPPHarness(t)
	k := NewTextKeyboard(h.ctx)
	hookPPWidget("kbd", k)
	h.start(func() { tkScreen(h.ctx, k) })

	for _, fragment := range []string{"", strings.Repeat("W", 100)} {
		k.Fragment = fragment
		h.next("after setting a %d-character fragment", len(fragment))
		for range ppPages {
			for i, row := range k.pages[k.page] {
				for j := range row {
					key := row[j]
					if !k.Valid(key) {
						continue
					}
					label := key.label
					if label == "" {
						label = string(key.r)
					}
					if key.action == ppBackspace {
						label = "backspace"
					}
					h.point(&k.pages[k.page][i][j].clk,
						fmt.Sprintf("%d-char readout: page %d key %q", len(fragment), k.page, label))
				}
			}
			cyc := ppTagFor(k, func(key ppKey) bool { return key.action == ppPageCycle })
			if cyc == nil {
				t.Fatalf("no page-cycle key on page %d", k.page)
			}
			h.tapAt(h.point(cyc, "page-cycle key"))
			h.next("after cycling page")
		}
	}
}

// TestNewlineKeyTypesANewlineByTouch drives the key itself, not commit().
func TestNewlineKeyTypesANewlineByTouch(t *testing.T) {
	h := newPPHarness(t)
	k := NewTextKeyboard(h.ctx)
	hookPPWidget("kbd", k)
	h.start(func() { tkScreen(h.ctx, k) })

	h.typeRune('a')
	tag := ppTagFor(k, func(key ppKey) bool { return key.r == '\n' })
	if tag == nil {
		t.Fatal("no newline key on the current page")
	}
	h.tapAt(h.point(tag, "newline key"))
	h.next("after tapping newline")
	h.typeRune('b')
	if k.Fragment != "a\nb" {
		t.Errorf("Fragment = %q, want %q", k.Fragment, "a\nb")
	}
	// And it is present on all four pages, so a newline never costs a page
	// cycle.
	for range ppPages {
		if ppTagFor(k, func(key ppKey) bool { return key.r == '\n' }) == nil {
			t.Errorf("no newline key on page %d", k.page)
		}
		cyc := ppTagFor(k, func(key ppKey) bool { return key.action == ppPageCycle })
		h.tapAt(h.point(cyc, "page-cycle key"))
		h.next("after cycling page")
	}
}

// TestTextKeyboardRunesAllDecodeInTheEngravingFace binds the two halves of the
// panic guard. engrave.String PANICS on a rune the face lacks
// (engrave/engrave.go:1531), so "the face covers printable ASCII" and "the
// keyboard emits only printable ASCII" are each only half an answer: this
// asserts every rune THIS keyboard can actually emit is engravable.
func TestTextKeyboardRunesAllDecodeInTheEngravingFace(t *testing.T) {
	ctx := NewContext(newPlatform())
	k := NewTextKeyboard(ctx)
	seen := 0
	for p := range len(ppPages) {
		for _, row := range k.pages[p] {
			for _, key := range row {
				if key.action != ppRune {
					continue
				}
				seen++
				switch key.r {
				case ' ':
					continue // advance-only; inks nothing
				case '\n':
					continue // a control character, never a glyph (spec 3)
				}
				if _, _, ok := sh.Font.Decode(key.r); !ok {
					t.Errorf("page %d key %q does not decode in font/sh; engrave.String would panic on it", p, string(key.r))
				}
			}
		}
	}
	if seen == 0 {
		t.Fatal("no rune keys found; the test walked nothing")
	}
}

// TestOnlyTheFreeTextProgramBuildsATextKeyboard: the anti-leak guard at the
// call sites, not just at construction. NewTextKeyboard must appear in exactly
// one file.
func TestOnlyTheFreeTextProgramBuildsATextKeyboard(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	var callers []string
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") || f == "passphrase_keyboard.go" {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(src), "NewTextKeyboard(") {
			callers = append(callers, f)
		}
	}
	if !slices.Equal(callers, []string{"freetext_flow.go"}) {
		t.Errorf("NewTextKeyboard is constructed in %v, want only freetext_flow.go", callers)
	}
}
