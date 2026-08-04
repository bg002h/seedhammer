package gui

import (
	"math"
	"strings"
	"testing"
)

func TestPassphraseKeyboardConstruction(t *testing.T) {
	ctx := NewContext(newPlatform())
	k := NewPassphraseKeyboard(ctx)
	if len(k.pages) != 4 {
		t.Fatalf("pages = %d, want 4", len(k.pages))
	}
	for p := 0; p < len(k.pages); p++ {
		rows := k.pages[p]
		// Content rows vary per page (the 4th page is only 2 lines); derive the
		// expected count from ppPages rather than hardcoding a uniform row count.
		wantRows := strings.Count(ppPages[p], "\n") + 1 + 1 // content rows + function row
		if len(rows) != wantRows {
			t.Errorf("page %d: %d rows, want %d", p, len(rows), wantRows)
		}
		fr := rows[len(rows)-1]
		if len(fr) != 4 {
			t.Errorf("page %d function row: %d keys, want 4 (page-cycle/space/reveal/backspace)", p, len(fr))
		}
		// function-row actions, in order.
		wantAct := []ppAction{ppPageCycle, ppRune, ppReveal, ppBackspace}
		for i, a := range wantAct {
			if fr[i].action != a {
				t.Errorf("page %d funcrow[%d].action = %v, want %v", p, i, fr[i].action, a)
			}
		}
		if fr[1].r != ' ' {
			t.Errorf("page %d space key r = %q, want ' '", p, fr[1].r)
		}
	}
	// page 0 row 0 is lowercase qwerty; page 1 uppercase.
	if k.pages[0][0][0].r != 'q' || k.pages[1][0][0].r != 'Q' {
		t.Errorf("page0[0][0]=%q page1[0][0]=%q, want 'q'/'Q'", k.pages[0][0][0].r, k.pages[1][0][0].r)
	}
	// Clear resets.
	k.Fragment = "secret"
	k.page = 2
	k.revealed = true
	k.Clear()
	if k.Fragment != "" || k.page != 0 || k.revealed {
		t.Errorf("after Clear: Fragment=%q page=%d revealed=%v, want \"\"/0/false", k.Fragment, k.page, k.revealed)
	}
}

func TestPassphraseRuneEntryCrossPage(t *testing.T) {
	ctx := NewContext(newPlatform())
	k := NewPassphraseKeyboard(ctx)
	// 'A' is only on page 1, '1'/'!' on page 2, 'b'/' ' on page 0 — cross-page,
	// case-honoring, no page switch.
	runes(&ctx.Router, "Ab 1!")
	for k.Update(ctx) {
	}
	if k.Fragment != "Ab 1!" {
		t.Errorf("Fragment = %q, want %q", k.Fragment, "Ab 1!")
	}
	if k.page != 0 {
		t.Errorf("page = %d, want 0 (RuneEvent must not switch pages)", k.page)
	}
}

func TestPassphraseActions(t *testing.T) {
	ctx := NewContext(newPlatform())
	k := NewPassphraseKeyboard(ctx)
	k.Fragment = "abc"
	k.commit(ppKey{action: ppBackspace})
	if k.Fragment != "ab" {
		t.Errorf("backspace: %q, want \"ab\"", k.Fragment)
	}
	k.commit(ppKey{r: ' ', action: ppRune})
	if k.Fragment != "ab " {
		t.Errorf("space: %q", k.Fragment)
	}
	pg := k.page
	k.commit(ppKey{action: ppPageCycle})
	if k.page != (pg+1)%len(ppPages) {
		t.Errorf("page-cycle: page=%d, want %d", k.page, (pg+1)%len(ppPages))
	}
	rev := k.revealed
	k.commit(ppKey{action: ppReveal})
	if k.revealed == rev {
		t.Errorf("reveal toggle did not flip revealed")
	}
	// backspace on empty Fragment is a no-op (Valid gates it, but commit must be safe).
	k.Fragment = ""
	k.commit(ppKey{action: ppBackspace})
	if k.Fragment != "" {
		t.Errorf("backspace on empty: %q, want \"\"", k.Fragment)
	}
}

func TestPassphraseDpadCommit(t *testing.T) {
	ctx := NewContext(newPlatform())
	k := NewPassphraseKeyboard(ctx)
	// From the centered cursor, Center commits the cursor key (a letter on page 0).
	before := k.Fragment
	press(&ctx.Router, Center)
	for k.Update(ctx) {
	}
	if len(k.Fragment) != len(before)+1 {
		t.Errorf("Center commit appended %d chars, want 1 (Fragment=%q)", len(k.Fragment)-len(before), k.Fragment)
	}
}

func passphraseFrame(t *testing.T, drive func(r *EventRouter)) string {
	t.Helper()
	ctx := NewContext(newPlatform())
	k := NewPassphraseKeyboard(ctx)
	frame, quit := runUI(ctx, func() {
		for !ctx.Done {
			for k.Update(ctx) {
			}
			fop, _ := k.Layout(ctx, &descriptorTheme) // 'fop' not 'op' — avoid shadowing the op package (R0 M-1)
			ctx.Frame(fop)
		}
	})
	defer quit()
	if drive != nil {
		drive(&ctx.Router)
	}
	c, ok := frame()
	if !ok {
		t.Fatal("no frame")
	}
	return c
}

func TestPassphraseMaskReveal(t *testing.T) {
	// Masked by default: typing 4 runes shows '*'×4, not the cleartext.
	c := passphraseFrame(t, func(r *EventRouter) { runes(r, "ab1!") })
	if !uiContains(c, "****") {
		t.Errorf("masked readout: want \"****\"; got %q", c)
	}
	if uiContains(c, "ab1!") {
		t.Errorf("masked readout leaked cleartext: %q", c)
	}
	// Reveal via the reveal key (drive by D-pad to it, or assert through a second flow):
	// simplest: a dedicated test toggling revealed then rendering.
	ctx := NewContext(newPlatform())
	k := NewPassphraseKeyboard(ctx)
	k.Fragment = "ab1!"
	k.revealed = true
	frame, quit := runUI(ctx, func() {
		for !ctx.Done {
			for k.Update(ctx) {
			}
			o, _ := k.Layout(ctx, &descriptorTheme)
			ctx.Frame(o)
		}
	})
	defer quit()
	c2, ok := frame()
	if !ok {
		t.Fatal("no frame")
	}
	if !uiContains(c2, "ab1!") {
		t.Errorf("revealed readout: want cleartext \"ab1!\"; got %q", c2)
	}
}

func TestPassphrasePageCycleRender(t *testing.T) {
	// Render the symbols page (page 2) directly and assert its content: a digit key
	// '1' (symbols page) + the page-cycle cap "#+=" (page 2 now cycles on to the
	// 4th page, the extra-symbols page) are rendered. (Driving the page-cycle key
	// itself via D-pad is exercised by TestPassphraseActions' commit(ppPageCycle);
	// this test covers the per-page render.)
	ctx := NewContext(newPlatform())
	k := NewPassphraseKeyboard(ctx)
	k.page = 2
	frame, quit := runUI(ctx, func() {
		for !ctx.Done {
			for k.Update(ctx) {
			}
			o, _ := k.Layout(ctx, &descriptorTheme)
			ctx.Frame(o)
		}
	})
	defer quit()
	got, ok := frame()
	if !ok {
		t.Fatal("no frame")
	}
	if !uiContains(got, "1") || !uiContains(got, "#+=") { // page 2 has digit '1' + the page-cycle cap "#+="
		t.Errorf("symbols page render: want '1' and the '#+=' page-cap; got %q", got)
	}
}

func TestPassphraseRevealKeyFitsBothLabels(t *testing.T) {
	// The reveal cap toggles "show"/"hide"; its cell must fit the wider label so
	// neither overflows/clips its tap target (exec-review M-1).
	ctx := NewContext(newPlatform())
	k := NewPassphraseKeyboard(ctx)
	fr := k.pages[0][len(k.pages[0])-1]
	reveal := fr[2] // page-cycle, space, reveal, backspace
	if reveal.action != ppReveal {
		t.Fatalf("funcrow[2].action = %v, want ppReveal", reveal.action)
	}
	hideW := ctx.Styles.keyboard.Measure(math.MaxInt, "%s", "hide").X
	showW := ctx.Styles.keyboard.Measure(math.MaxInt, "%s", "show").X
	want := hideW
	if showW > want {
		want = showW
	}
	if reveal.size.X < want {
		t.Errorf("reveal key size.X = %d, want >= %d (max of show/hide widths)", reveal.size.X, want)
	}
}

// Every printable ASCII character the spec promises must be typeable.
func TestKeyboardCoversPrintableASCII(t *testing.T) {
	ctx := NewContext(newPlatform())
	k := NewPassphraseKeyboard(ctx)
	seen := map[rune]bool{}
	for p := range k.pages {
		for _, row := range k.pages[p] {
			for _, key := range row {
				if key.action == ppRune {
					seen[key.r] = true
				}
			}
		}
	}
	var missing []rune
	for r := rune(0x20); r <= 0x7E; r++ {
		if !seen[r] {
			missing = append(missing, r)
		}
	}
	if len(missing) > 0 {
		t.Errorf("keyboard cannot type %d of 95 printable ASCII: %q",
			len(missing), string(missing))
	}
}

func TestKeyboardPageCycleVisitsAllPages(t *testing.T) {
	ctx := NewContext(newPlatform())
	k := NewPassphraseKeyboard(ctx)
	start := k.page
	seen := map[int]bool{start: true}
	for i := 0; i < len(ppPages)*2; i++ {
		k.commit(ppKey{action: ppPageCycle})
		seen[k.page] = true
	}
	if len(seen) != len(ppPages) {
		t.Errorf("cycle visited %d of %d pages", len(seen), len(ppPages))
	}
	if k.page != start {
		t.Errorf("after %d cycles page is %d, want %d", len(ppPages)*2, k.page, start)
	}
}
