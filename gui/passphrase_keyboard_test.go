package gui

import "testing"

func TestPassphraseKeyboardConstruction(t *testing.T) {
	ctx := NewContext(newPlatform())
	k := NewPassphraseKeyboard(ctx)
	if len(k.pages) != 3 {
		t.Fatalf("pages = %d, want 3", len(k.pages))
	}
	for p := 0; p < 3; p++ {
		rows := k.pages[p]
		if len(rows) != 4 { // 3 letter/symbol rows + 1 function row
			t.Errorf("page %d: %d rows, want 4", p, len(rows))
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
	if k.page != (pg+1)%3 {
		t.Errorf("page-cycle: page=%d, want %d", k.page, (pg+1)%3)
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
