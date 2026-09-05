package gui

import (
	"errors"
	"strings"
	"testing"

	"seedhammer.com/bip39"
	"seedhammer.com/codex32"
)

func TestCodex32StatusLine(t *testing.T) {
	cases := []struct {
		n    int
		want string
	}{
		{0, "0 chars"},
		{47, "47 chars"},
		{48, "short | 48 chars"},
		{93, "short | 93 chars"},
		{94, "94 chars - keep typing"},
		{124, "124 chars - keep typing"},
		{125, "long | 125 chars"},
		{127, "long | 127 chars"},
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
	if got := codex32FieldLine(f); got != "id NAME | thr 2" {
		t.Errorf("codex32FieldLine(ms12name) = %q", got)
	}
	f, _ = codex32.ParsePrefix("ms10tests")
	if got := codex32FieldLine(f); got != "id TEST | thr 0 | share S" {
		t.Errorf("codex32FieldLine(ms10tests) = %q", got)
	}
	var empty codex32.Fields
	if got := codex32FieldLine(empty); got != "" {
		t.Errorf("codex32FieldLine(empty) = %q, want \"\"", got)
	}
}

// TestMstarStatusLineSeparator: the md/mk branch of mstarStatusLine
// (codex32_polish.go:286) joins the HRP and length readout with "|", not "·"
// (F-78 — "·" has no glyph in poppins.Regular16/Bold25, the faces that render
// this line, and contributes zero pixels; "|" is the operator's established
// substitute, already used by unlockPlateLabel).
func TestMstarStatusLineSeparator(t *testing.T) {
	const frag = "md1yqpqqxqq8xtwhw4xwn4qh"
	f, _ := codex32.ParsePrefix(frag)
	if !codex32.MStarInWindow(frag) {
		t.Fatalf("setup: %q must be in the md/mk status window", frag)
	}
	got := mstarStatusLine(frag, f)
	if !strings.Contains(got, "|") {
		t.Errorf("mstarStatusLine(%q) = %q, want a \"|\" separator", frag, got)
	}
	if strings.Contains(got, "·") {
		t.Errorf("mstarStatusLine(%q) = %q, still uses the invisible middot", frag, got)
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

// TestRecoverCodex32TitleSeparator: recoverCodex32Flow's per-share prompt title
// (codex32_polish.go:182, "Share %d of %d · id %s") renders in ctx.Styles.title
// (poppins.Bold25). F-78: "·" contributes zero pixels in this font; the title
// must use "|" instead, matching the fix applied throughout this file.
func TestRecoverCodex32TitleSeparator(t *testing.T) {
	shareA, err := codex32.New("MS12NAMEA320ZYXWVUTSRQPNMLKJHGFEDCAXRPP870HKKQRM")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := NewContext(newPlatform())
	frame, quit := runUI(ctx, func() { recoverCodex32Flow(ctx, &descriptorTheme, shareA) })
	defer quit()
	c, ok := frame()
	if !ok {
		t.Fatal("no frame")
	}
	if !uiContains(c, "Share 2 of 2 | id NAME") {
		t.Errorf("recover title missing the \"|\" separator; got %q", c)
	}
	if strings.Contains(c, "·") {
		t.Errorf("recover title still contains the invisible middot separator: %q", c)
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

// TestConfirmCodex32Flow_ShowSecretGate is the L1 regression+convention guard.
// The probe entropy that the fix wipes (codex32_polish.go:103) is consumed and
// scrubbed inside confirmCodex32Flow and unobservable seam-free (spec R0
// Q1/Minor-2), so this is NOT a buffer-zeroed assertion. It proves the additive
// wipeBytes(ent) does NOT perturb the showSecret decision: an unshared ms1
// `entr` secret (msErr == nil && f.Unshared) must still offer "Show secret"
// (Button2 opens the decode view). Mirrors TestConfirmShowSecretGate
// (gui/ms1_decode_test.go:76).
func TestConfirmCodex32Flow_ShowSecretGate(t *testing.T) {
	const ms1 = "ms10entrsqqqqqqqqqqqqqqqqqqqqqqqqqqqqcj9sxraq34v7f" // unshared entr secret (entropy 0*16)
	s := mustCodex32T(t, ms1)
	want := bip39.LabelFor(bip39.New(make([]byte, 16))[0]) // first decoded word label
	ctx := NewContext(newPlatform())
	click(&ctx.Router, Button2) // Show secret -> opens ms1DecodeFlow (only for unshared)
	frame, quit := runUI(ctx, func() { confirmCodex32Flow(ctx, &descriptorTheme, s) })
	defer quit()
	seen := false
	for i := 0; i < 10; i++ {
		c, ok := frame()
		if !ok {
			break
		}
		if uiContains(c, want) {
			seen = true
			break
		}
	}
	if !seen {
		t.Fatal("Show secret did not open the decode view on the unshared secret (showSecret gate perturbed?)")
	}
}

// H0 (SPEC_ms_hashlock §9): a kind-0x03 preimage plate handed to the engrave
// dispatch -- the object both the NFC door and the typed M*1 STRING door
// produce -- is refused by name and never reaches the codex32 confirm screen.
func TestEngraveCodex32RefusesAPreimagePlate(t *testing.T) {
	s, err := codex32.New("ms10hashsqw46h2at4w46h2at4w46h2at4w46h2at4w46h2at4w46h2at4w46kzv2ncy60u7z9c")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := NewContext(newPlatform())
	returned := false
	frame, drawer, quit := runUITouch(ctx, func() {
		engraveObjectFlow(ctx, &descriptorTheme, s)
		returned = true
	})
	h := &sessionHarness{t: t, ctx: ctx, done: &returned}
	h.frame, h.drawer = frame, drawer
	t.Cleanup(quit)
	// MUTATION: drop the IsPreimage check in engraveCodex32 -> the flow shows
	// "Confirm Codex32 Secret" for the plate and never this text.
	h.mustReach("hashlock preimage")
}

// The NFC door: Scan classifies the plate as no known object, exactly as
// seal.Classify does (the two are documented mirrors), so it never becomes a
// codex32.String for engraveObjectFlow. The typed door has no such gate and
// relies on engraveCodex32's refusal above.
func TestScanDoesNotHandAPreimagePlateToEngrave(t *testing.T) {
	const plate = "ms10hashsqw46h2at4w46h2at4w46h2at4w46h2at4w46h2at4w46h2at4w46kzv2ncy60u7z9c"
	// Scan reads until io.EOF; a strings.Reader delivers that on its second Read.
	scanAll := func(s string) (any, error) {
		sc := &scanner{}
		r := strings.NewReader(s)
		for {
			obj, err := sc.Scan(r)
			if !errors.Is(err, errScanInProgress) {
				return obj, err
			}
		}
	}
	obj, err := scanAll(plate)
	if !errors.Is(err, errScanUnknownFormat) {
		t.Fatalf("Scan(preimage plate) = %T, %v; want errScanUnknownFormat", obj, err)
	}
	// And the legitimate populations the guard must not touch still scan.
	for _, s := range []string{
		"ms10testsqv0qqqqqqqqqqqqqqqqqqqqqqq8mzk8tjfdnjn5",                            // plain BIP-93, seed begins 0x03
		"ms12testaqv0qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqdq7pl8qdc5tsp", // a share beginning 0x03
	} {
		obj, err := scanAll(s)
		if _, ok := obj.(codex32.String); err != nil || !ok {
			t.Errorf("Scan(%.20s...) = %T, %v; want a codex32.String", s, obj, err)
		}
	}
}

// H0 (SPEC_ms_hashlock §9), the RECOVER arm — post-implementation review C-1.
//
// engraveCodex32's loop REASSIGNS the object it was handed (`scan = secret`)
// when the operator recovers a secret from shares, so a guard that runs once
// before the loop protects only the object that came through a door. A K-of-N
// set whose interpolation is a kind-0x03 preimage single is the counterexample:
// neither share is a preimage (a share carries no kind byte — §1 rule 2, and
// the door correctly admits it), and the secret the loop manufactures is one.
//
// The screens are: Confirm Codex32 SHARE -> Recover (Button2) -> type share 2
// -> OK (Button3) -> [the guard must fire here]. With the test outside the
// loop the next screens are "Confirm Codex32 Secret" and then "Engrave Plate",
// which is `backup.EngraveSeedString` — the call that cuts metal.
func TestEngraveCodex32RefusesAPreimageRecoveredFromShares(t *testing.T) {
	// A 2-of-N set over a 33-byte payload beginning 0x03, id `hash`, built with
	// the fork's own codex32.NewSeed + Interpolate (the review's construction).
	const (
		shareC = "ms12hashc5yq6ypay5zr2wr9f4796czdw42gtz94nkx2mvyachjdtkx9ahw0cqdhwpr98gd53cq"
		shareD = "ms12hashdf3qqyjsyfsrswsgfgv9qc3qwgcg3zkcnt52pvhsc2qd3k4ga2u0zqcr0mcp2tv27n7"
		secret = "ms12hashsqvqsyqcyq5rqwzqfpg9scrgwpugpzysnzs23v9ccrydpk8qarc0jq9get7tzc6sn5y"
	)
	// UPPERCASE throughout: the codex32 keypad uppercases what it types, and
	// codex32 requires one consistent case across a set (a lowercase share and
	// an uppercase one are "mismatched type" = mismatched HRP). This is the
	// form the device actually holds after share 2 is entered.
	first, err := codex32.New(strings.ToUpper(shareC))
	if err != nil {
		t.Fatalf("New(shareC): %v", err)
	}
	second, err := codex32.New(strings.ToUpper(shareD))
	if err != nil {
		t.Fatalf("New(shareD): %v", err)
	}
	// The premise, asserted rather than assumed: the SHARES are not preimages
	// (so no upstream door refuses them), and their interpolation IS one.
	if codex32.IsPreimage(first) || codex32.IsPreimage(second) {
		t.Fatal("a share must not be a preimage; the guard would fire at the door and the loop never run")
	}
	rec, err := codex32.Interpolate([]codex32.String{first, second}, 'S')
	if err != nil {
		t.Fatalf("Interpolate: %v", err)
	}
	if !strings.EqualFold(rec.String(), secret) || !codex32.IsPreimage(rec) {
		t.Fatalf("the set does not recover a preimage single: got %q IsPreimage=%v", rec.String(), codex32.IsPreimage(rec))
	}

	ctx := NewContext(newPlatform())
	// Queued in the order the operator taps them: Recover at the share's
	// confirm screen, share 2 on the keypad, OK.
	click(&ctx.Router, Button2)
	runes(&ctx.Router, strings.ToUpper(shareD))
	click(&ctx.Router, Button3)

	returned := false
	frame, drawer, quit := runUITouch(ctx, func() {
		engraveObjectFlow(ctx, &descriptorTheme, first)
		returned = true
	})
	h := &sessionHarness{t: t, ctx: ctx, done: &returned}
	h.frame, h.drawer = frame, drawer
	t.Cleanup(quit)

	// MUTATION: move the IsPreimage test back OUTSIDE the `for` body (where it
	// shipped) -> the recovered preimage reaches "Confirm Codex32 Secret" and,
	// one tap later, "Engrave Plate". Both are fatal here, so this test cannot
	// pass by merely finding the refusal somewhere after a cut was offered.
	reached := ""
	for i := 0; i < 256 && reached == ""; i++ {
		c, ok := h.frame()
		if !ok {
			break
		}
		h.content = c
		switch {
		case uiContains(c, "Confirm Codex32 Secret"):
			t.Fatalf("a preimage recovered from shares reached the SECRET confirm screen: %q", c)
		case uiContains(c, "Engrave Plate"):
			t.Fatalf("a preimage recovered from shares reached the ENGRAVE screen: %q", c)
		case uiContains(c, "hashlock preimage"):
			reached = c
		}
	}
	if reached == "" {
		t.Fatalf("never reached the hashlock-preimage refusal; last frame %q", h.content)
	}
}
