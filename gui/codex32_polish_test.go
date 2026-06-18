package gui

import (
	"strings"
	"testing"

	"seedhammer.com/codex32"
)

func TestCodex32StatusLine(t *testing.T) {
	cases := []struct {
		n    int
		want string
	}{
		{0, "0 chars"},
		{47, "47 chars"},
		{48, "short · 48 chars"},
		{93, "short · 93 chars"},
		{94, "94 chars — keep typing"},
		{124, "124 chars — keep typing"},
		{125, "long · 125 chars"},
		{127, "long · 127 chars"},
		{128, "too long"},
	}
	for _, c := range cases {
		if got := codex32StatusLine(c.n); got != c.want {
			t.Errorf("codex32StatusLine(%d) = %q, want %q", c.n, got, c.want)
		}
	}
}

func TestCodex32FieldLine(t *testing.T) {
	f, _ := codex32.ParsePrefix("ms12name")
	if got := codex32FieldLine(f); got != "id NAME · thr 2" {
		t.Errorf("codex32FieldLine(ms12name) = %q", got)
	}
	f, _ = codex32.ParsePrefix("ms10tests")
	if got := codex32FieldLine(f); got != "id TEST · thr 0 · share S" {
		t.Errorf("codex32FieldLine(ms10tests) = %q", got)
	}
	var empty codex32.Fields
	if got := codex32FieldLine(empty); got != "" {
		t.Errorf("codex32FieldLine(empty) = %q, want \"\"", got)
	}
}

func TestCodex32Feedback(t *testing.T) {
	// Eager field error (bad threshold), regardless of length.
	_, perr := codex32.ParsePrefix("MS11")
	if got := codex32Feedback("MS11", perr, nil); got != "bad threshold" {
		t.Errorf("feedback(MS11) = %q, want \"bad threshold\"", got)
	}
	// Dead zone (94..124): no determinable error → suppressed.
	keep := "MS10TESTS" + strings.Repeat("X", 91) // 100 chars
	_, perr = codex32.ParsePrefix(keep)
	_, nerr := codex32.New(keep)
	if got := codex32Feedback(keep, perr, nerr); got != "" {
		t.Errorf("feedback(deadzone) = %q, want \"\"", got)
	}
	// Full-length bad checksum → shown.
	bad := "MS10FAUXSXXXXXXXXXXXXXXXXXXXXXXXXXXVE740YYGE2GHP"
	_, perr = codex32.ParsePrefix(bad)
	_, nerr = codex32.New(bad)
	if got := codex32Feedback(bad, perr, nerr); got != "bad checksum" {
		t.Errorf("feedback(badchecksum) = %q, want \"bad checksum\"", got)
	}
}

// codex32Frame runs inputCodex32Flow, types `typed` (uppercased by the keypad),
// and returns the first rendered frame's extracted text.
func codex32Frame(t *testing.T, typed string) string {
	t.Helper()
	ctx := NewContext(newPlatform())
	frame, quit := runUI(ctx, func() {
		inputCodex32Flow(ctx, &descriptorTheme)
	})
	defer quit()
	if typed != "" {
		runes(&ctx.Router, typed)
	}
	content, ok := frame()
	if !ok {
		t.Fatal("no frame")
	}
	return content
}

func TestCodex32FlowReadout(t *testing.T) {
	if c := codex32Frame(t, ""); !uiContains(c, "0 chars") {
		t.Errorf("empty: want \"0 chars\"; got %q", c)
	}
	if c := codex32Frame(t, "ms12name"); !uiContains(c, "id NAME") || !uiContains(c, "thr 2") {
		t.Errorf("fields: want id NAME + thr 2; got %q", c)
	}
	if c := codex32Frame(t, "ms11"); !uiContains(c, "bad threshold") {
		t.Errorf("bad threshold: got %q", c)
	}
	keep := "ms10tests" + strings.Repeat("x", 91) // 100 chars → dead zone
	if c := codex32Frame(t, keep); !uiContains(c, "keep typing") {
		t.Errorf("keep typing: got %q", c)
	}
	bad := "ms10fauxsxxxxxxxxxxxxxxxxxxxxxxxxxxve740yyge2ghp" // valid len, bad checksum
	if c := codex32Frame(t, bad); !uiContains(c, "bad checksum") {
		t.Errorf("bad checksum: got %q", c)
	}
}

func TestConfirmCodex32Unshared(t *testing.T) {
	s, err := codex32.New("ms10testsxxxxxxxxxxxxxxxxxxxxxxxxxx4nzvca9cmczlw")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := NewContext(newPlatform())
	frame, quit := runUI(ctx, func() { confirmCodex32Flow(ctx, &descriptorTheme, s) })
	defer quit()
	c, ok := frame()
	if !ok {
		t.Fatal("no frame")
	}
	if !uiContains(c, "Unshared secret") {
		t.Errorf("unshared: want \"Unshared secret\"; got %q", c)
	}
	if !uiContains(c, "id TEST") {
		t.Errorf("unshared id: got %q", c)
	}
}

func TestConfirmCodex32Share(t *testing.T) {
	s, err := codex32.New("MS12NAMEA320ZYXWVUTSRQPNMLKJHGFEDCAXRPP870HKKQRM")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := NewContext(newPlatform())
	frame, quit := runUI(ctx, func() { confirmCodex32Flow(ctx, &descriptorTheme, s) })
	defer quit()
	c, ok := frame()
	if !ok {
		t.Fatal("no frame")
	}
	if !uiContains(c, "Share A") {
		t.Errorf("share: want \"Share A\"; got %q", c)
	}
	if !uiContains(c, "not a recovered seed") {
		t.Errorf("share note: got %q", c)
	}
}

func TestCodex32KeyboardDimsBIO(t *testing.T) {
	ctx := NewContext(newPlatform())
	kbd := newCodex32Keyboard(ctx)
	dimmed := map[rune]bool{'b': true, 'i': true, 'o': true}
	for _, k := range kbd.allKeys {
		if dimmed[k.r] && !k.disabled {
			t.Errorf("codex32 key %q should be disabled", k.r)
		}
		if k.r >= 'a' && k.r <= 'z' && !dimmed[k.r] && k.disabled {
			t.Errorf("codex32 key %q should be enabled", k.r)
		}
	}
	// Every codex32.Alphabet char (lowercased) + the '1' separator is present and enabled.
	enabled := map[rune]bool{}
	for _, k := range kbd.allKeys {
		if !k.disabled {
			enabled[k.r] = true
		}
	}
	for _, c := range codex32.Alphabet {
		lc := []rune(strings.ToLower(string(c)))[0]
		if !enabled[lc] {
			t.Errorf("codex32 Alphabet char %q (lc %q) missing/disabled on keypad", c, lc)
		}
	}
	if !enabled['1'] {
		t.Error("codex32 keypad must keep '1' (HRP separator) enabled")
	}
}

func TestBIP39KeyboardNotDimmed(t *testing.T) {
	// Regression: dimming the codex32 instance must not affect the BIP-39 keyboard.
	ctx := NewContext(newPlatform())
	kbd := NewKeyboard(ctx, wordKeys)
	for _, k := range kbd.allKeys {
		switch k.r {
		case 'b', 'i', 'o':
			if k.disabled {
				t.Errorf("BIP-39 key %q must NOT be disabled (no cross-contamination)", k.r)
			}
		}
	}
}

// Backing out of the pre-engrave confirm screen must NOT surface as
// "Unknown format": the codex32 string was recognized, the user just declined
// to engrave. engraveObjectFlow returns true for recognized objects (only the
// default/unrecognized case returns false → scanUnknownFormat), so a cancel
// must also return true.
func TestEngraveCodex32BackoutNotUnknown(t *testing.T) {
	s, err := codex32.New("ms10testsxxxxxxxxxxxxxxxxxxxxxxxxxx4nzvca9cmczlw")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := NewContext(newPlatform())
	click(&ctx.Router, Button1) // Back at the confirm screen
	if !engraveObjectFlow(ctx, &descriptorTheme, s) {
		t.Error("cancel at codex32 confirm returned false (→ \"Unknown format\"); want true (recognized, not engraved)")
	}
}
