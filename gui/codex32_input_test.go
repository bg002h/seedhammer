package gui

import (
	"strings"
	"testing"

	"seedhammer.com/codex32"
)

// Drives the "Input Seed" menu to the "M*1 STRING" choice (index 2) and enters a
// valid codex32 string on the keypad, asserting newInputFlow returns it as a
// codex32.String (the ms path of the HRP-dispatched entry).
//
// The "M*1 STRING" entry sits at menu index 2 (after "12 WORDS"/"24 WORDS"), so
// two Down presses select it; index 2 routes to inputCodex32Flow, which for an
// ms1 string returns a codex32.String.
//
// NOTE: the keypad stores typed runes UPPERCASE, so the returned string is the
// uppercase form of what we type; compare against strings.ToUpper(share).
func TestInputSeedCodex32(t *testing.T) {
	// A valid "ms" codex32 string from the codex32 package's own test corpus.
	const share = "ms10testsxxxxxxxxxxxxxxxxxxxxxxxxxx4nzvca9cmczlw"

	ctx := NewContext(newPlatform())
	// Menu: move the selection 0 -> 2 (CODEX32) with two Down presses, confirm
	// with Button3 (the ChoiceScreen "choose" button).
	click(&ctx.Router, Down, Down, Button3)
	// Keypad: type the share, then confirm with Button3 (OK).
	runes(&ctx.Router, share)
	click(&ctx.Router, Button3)

	obj, ok := newInputFlow(ctx, &descriptorTheme)
	if !ok {
		t.Fatal("newInputFlow did not return a value")
	}
	s, isCodex := obj.(codex32.String)
	if !isCodex {
		t.Fatalf("newInputFlow returned %T, want codex32.String", obj)
	}
	want := strings.ToUpper(share) // keypad uppercases typed runes
	if got := s.String(); got != want {
		t.Errorf("codex32 entry returned %q, want %q", got, want)
	}
}

// Typing a valid md1 string returns it as mdmkText (routed to mdmkFlow), proving
// the HRP-dispatched entry handles md/mk, not just ms. (Phase B.)
func TestInputMStarMD1(t *testing.T) {
	const valid = "md1yqpqqxqq8xtwhw4xwn4qh"
	ctx := NewContext(newPlatform())
	click(&ctx.Router, Down, Down, Button3) // menu -> M*1 STRING (index 2)
	runes(&ctx.Router, valid)
	click(&ctx.Router, Button3) // OK (valid)
	obj, ok := newInputFlow(ctx, &descriptorTheme)
	if !ok {
		t.Fatal("newInputFlow did not return a value")
	}
	got, isMd := obj.(mdmkText)
	if !isMd {
		t.Fatalf("returned %T, want mdmkText", obj)
	}
	if want := mdmkText(strings.ToUpper(valid)); got != want {
		t.Errorf("md1 entry = %q, want %q", got, want)
	}
}

// Typing a valid mk1 string returns mdmkText.
func TestInputMStarMK1(t *testing.T) {
	const valid = "mk1qpzg69ppsnz4v7cjv3qfjhf76k4t5pt96u0psdrqfqvll8qh7h5athg837pmkf3dpug2mmjtfel6x"
	ctx := NewContext(newPlatform())
	click(&ctx.Router, Down, Down, Button3)
	runes(&ctx.Router, valid)
	click(&ctx.Router, Button3)
	obj, ok := newInputFlow(ctx, &descriptorTheme)
	if !ok {
		t.Fatal("newInputFlow did not return a value")
	}
	if got, isMd := obj.(mdmkText); !isMd || got != mdmkText(strings.ToUpper(valid)) {
		t.Fatalf("mk1 entry = %v (%T), want mdmkText", obj, obj)
	}
}

// A single-substitution-corrupted md1: the "Fix?" affordance (Button3 when
// invalid-in-window) corrects it; the confirm screen's Button3 accepts; the now
// valid string OKs through as mdmkText. (Phase B; orientation/diff end to end.)
func TestInputMStarFixMD1(t *testing.T) {
	const valid = "md1yqpqqxqq8xtwhw4xwn4qh"
	const corrupted = "md1yqzqqxqq8xtwhw4xwn4qh" // data index 2 ('p'->'z'); any single bech32 sub works
	if corrupted == valid {
		t.Fatal("test corruption is a no-op")
	}
	ctx := NewContext(newPlatform())
	click(&ctx.Router, Down, Down, Button3) // menu -> M*1 STRING
	runes(&ctx.Router, corrupted)
	click(&ctx.Router, Button3) // Fix? (invalid-in-window)
	click(&ctx.Router, Button3) // accept the correction (confirmCorrectionFlow)
	click(&ctx.Router, Button3) // OK (now valid)
	obj, ok := newInputFlow(ctx, &descriptorTheme)
	if !ok {
		t.Fatal("newInputFlow did not return a value")
	}
	if got, isMd := obj.(mdmkText); !isMd || got != mdmkText(strings.ToUpper(valid)) {
		t.Fatalf("fixed md1 = %v (%T), want %q", obj, obj, strings.ToUpper(valid))
	}
}

// An uncorrectable (>4-error) entry: pressing Fix? shows the "no fix" modal and
// returns to editing — it never fabricates a correction. **Event sequence
// (plan-R0 C-2):** Fix → dismiss modal → Back out of the ENTRY (returns
// (nil,false)) → Back out of the MENU (the menu loops on ok=false, so a second
// Button1 is required to make newInputFlow return). Omitting the second Back
// hangs on the re-rendered ChoiceScreen.
func TestInputMStarFixUncorrectable(t *testing.T) {
	// 5 substitutions in an md1 — beyond t=4; codex32.Correct returns (_,false).
	const corrupted = "md1zzzzzxqq8xtwhw4xwn4qh"
	ctx := NewContext(newPlatform())
	click(&ctx.Router, Down, Down, Button3) // menu -> M*1 STRING -> entry
	runes(&ctx.Router, corrupted)
	// Fix? -> "no fix" modal -> dismiss -> Back(entry) -> Back(menu).
	click(&ctx.Router, Button3, Button3, Button1, Button1)
	obj, ok := newInputFlow(ctx, &descriptorTheme)
	if ok {
		t.Fatalf("uncorrectable entry must not yield a value, got %v (%T)", obj, obj)
	}
}
