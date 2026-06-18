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
		inputCodex32Flow(ctx, &descriptorTheme, "Input m*1 string")
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
	if !uiContains(c, "Recover the secret") {
		t.Errorf("share note: want recover affordance; got %q", c)
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

func TestConfirmCodex32ShareOffersRecover(t *testing.T) {
	s, err := codex32.New("MS12NAMEA320ZYXWVUTSRQPNMLKJHGFEDCAXRPP870HKKQRM")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := NewContext(newPlatform())
	click(&ctx.Router, Button2) // Recover
	if got := confirmCodex32Flow(ctx, &descriptorTheme, s); got != codex32Recover {
		t.Errorf("share + Button2 → %v, want codex32Recover", got)
	}
}

func TestConfirmCodex32UnsharedNoRecover(t *testing.T) {
	s, err := codex32.New("ms10testsxxxxxxxxxxxxxxxxxxxxxxxxxx4nzvca9cmczlw")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := NewContext(newPlatform())
	click(&ctx.Router, Button2, Button3) // Button2 must be inert for an unshared secret
	if got := confirmCodex32Flow(ctx, &descriptorTheme, s); got != codex32Engrave {
		t.Errorf("unshared + Button2,Button3 → %v, want codex32Engrave (Button2 ignored)", got)
	}
}

func TestRecoverCodex32(t *testing.T) {
	shareA, err := codex32.New("MS12NAMEA320ZYXWVUTSRQPNMLKJHGFEDCAXRPP870HKKQRM")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := NewContext(newPlatform())
	// k=2: enter the second share (C) and accept it.
	runes(&ctx.Router, "MS12NAMECACDEFGHJKLMNPQRSTUVWXYZ023FTR2GDZMPY6PN")
	click(&ctx.Router, Button3)
	secret, ok := recoverCodex32Flow(ctx, &descriptorTheme, shareA)
	if !ok {
		t.Fatal("recoverCodex32Flow did not recover")
	}
	const want = "MS12NAMES6XQGUZTTXKEQNJSJZV4JV3NZ5K3KWGSPHUH6EVW"
	if got := secret.String(); got != want {
		t.Errorf("recovered %q, want %q", got, want)
	}
}

func TestRecoverCodex32Mismatch(t *testing.T) {
	shareA, err := codex32.New("MS12NAMEA320ZYXWVUTSRQPNMLKJHGFEDCAXRPP870HKKQRM")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := NewContext(newPlatform())
	frame, quit := runUI(ctx, func() { recoverCodex32Flow(ctx, &descriptorTheme, shareA) })
	defer quit()
	// Enter a share from a DIFFERENT set (threshold 3, id CASH) and accept it.
	runes(&ctx.Router, "MS13CASHA320ZYXWVUTSRQPNMLKJHGFEDCA2A8D0ZEHN8A0T")
	click(&ctx.Router, Button3)
	var content string
	for i := 0; i < 8; i++ {
		c, ok := frame()
		if !ok {
			break
		}
		content = c
		if uiContains(content, "mismatched") {
			break
		}
	}
	if !uiContains(content, "mismatched") {
		t.Errorf("expected a mismatch error; got %q", content)
	}
}

// During codex32 share recovery, entering a non-codex32 (md/mk) string is
// rejected — recovery is ms-share-only. (Phase B caller-ripple guard.)
func TestRecoverRejectsNonCodex32(t *testing.T) {
	// A valid ms share with threshold ≥2 (mirrors TestRecoverCodex32's setup).
	shareA, err := codex32.New("MS12NAMEA320ZYXWVUTSRQPNMLKJHGFEDCAXRPP870HKKQRM")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := NewContext(newPlatform())
	// Enter a VALID md1 for "share 2": OK it (Button3 → mdmkText), the
	// type-assert rejects it with a modal (dismissed by Button3), then Back
	// (Button1) exits recovery.
	runes(&ctx.Router, "md1yqpqqxqq8xtwhw4xwn4qh")
	click(&ctx.Router, Button3, Button3, Button1) // OK md1 → dismiss modal → Back
	_, ok := recoverCodex32Flow(ctx, &descriptorTheme, shareA)
	if ok {
		t.Fatal("recovery must not accept a non-codex32 entry")
	}
}

// The correction-confirm screen: Button3 accepts, Button1 rejects, and Button2
// is drained every frame (must not block Button3 — the multishare R0-C1 lesson).
func TestConfirmCorrectionFlow(t *testing.T) {
	res := codex32.CorrectionResult{
		Corrected: "MD1YQPQQXQQ8XTWHW4XWN4QH",
		Edits:     []codex32.Edit{{Pos: 5, Was: 'Z', Now: 'P'}},
	}
	// Accept (Button3).
	ctx := NewContext(newPlatform())
	click(&ctx.Router, Button3)
	if !confirmCorrectionFlow(ctx, &descriptorTheme, res, "md") {
		t.Error("Button3 should accept the correction")
	}
	// Reject (Button1).
	ctx = NewContext(newPlatform())
	click(&ctx.Router, Button1)
	if confirmCorrectionFlow(ctx, &descriptorTheme, res, "md") {
		t.Error("Button1 should reject the correction")
	}
	// Button2 must not block Button3 (drain).
	ctx = NewContext(newPlatform())
	click(&ctx.Router, Button2, Button3)
	if !confirmCorrectionFlow(ctx, &descriptorTheme, res, "md") {
		t.Error("Button2 must be drained so Button3 still accepts")
	}
}
