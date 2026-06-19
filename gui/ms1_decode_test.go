package gui

import (
	"fmt"
	"testing"

	"seedhammer.com/bip39"
	"seedhammer.com/codex32"
)

func mustCodex32T(t *testing.T, s string) codex32.String {
	t.Helper()
	c, err := codex32.New(s)
	if err != nil {
		t.Fatalf("New(%q): %v", s, err)
	}
	return c
}

// English ms1 (entr) → the decoded BIP-39 words are shown.
func TestMS1DecodeFlowEnglishWords(t *testing.T) {
	const ms1 = "ms10entrsqqqqqqqqqqqqqqqqqqqqqqqqqqqqcj9sxraq34v7f" // entropy 0*16 → 12 abandon... words
	s := mustCodex32T(t, ms1)
	_, _, entropy, err := codex32.DecodeMS1(s)
	if err != nil {
		t.Fatal(err)
	}
	want := bip39.LabelFor(bip39.New(entropy)[0]) // first word label
	ctx := NewContext(newPlatform())
	frame, quit := runUI(ctx, func() { ms1DecodeFlow(ctx, &descriptorTheme, s) })
	defer quit()
	seen := false
	for i := 0; i < 8; i++ {
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
		t.Fatalf("English words not shown (want first word %q)", want)
	}
}

// Non-English mnem (Japanese) → language name + entropy hex shown; NO English words.
func TestMS1DecodeFlowNonEnglish(t *testing.T) {
	const ms1 = "ms10entrsqgqsc83yukgh23xkvmp59xf2eldpkpefrcjje3drdq" // mnem lang=1 (japanese)
	s := mustCodex32T(t, ms1)
	ctx := NewContext(newPlatform())
	frame, quit := runUI(ctx, func() { ms1DecodeFlow(ctx, &descriptorTheme, s) })
	defer quit()
	sawLang, sawHex := false, false
	for i := 0; i < 8; i++ {
		c, ok := frame()
		if !ok {
			break
		}
		if uiContains(c, "Japanese") {
			sawLang = true
		}
		if uiContains(c, "0c1e24e5917544d666c342992acfda1b") {
			sawHex = true
		}
	}
	if !sawLang || !sawHex {
		t.Fatalf("non-English: lang=%v hex=%v (want both)", sawLang, sawHex)
	}
}

// The "Show secret" affordance (Button2) opens the decode view ONLY for the
// unshared secret. (Drive confirmCodex32Flow on an unshared secret, press
// Button2, assert the decoded words appear.)
func TestConfirmShowSecretGate(t *testing.T) {
	const ms1 = "ms10entrsqqqqqqqqqqqqqqqqqqqqqqqqqqqqcj9sxraq34v7f"
	s := mustCodex32T(t, ms1)
	want := bip39.LabelFor(bip39.New(make([]byte, 16))[0])
	ctx := NewContext(newPlatform())
	click(&ctx.Router, Button2) // Show secret (unshared → opens ms1DecodeFlow)
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
		t.Fatal("Button2 did not open the secret view on the unshared secret")
	}
}

// 24-word secret (32B entropy) spans multiple pages → paging must not skip a
// word (the T1 measure-and-advance lesson). Observe each page, then advance.
//
// The zero-entropy mnemonic is ABANDON×23 + ART, so bare word labels for
// indices 0 and 11 both collapse to "ABANDON" and are not per-index unique.
// We therefore assert the FULL "N LABEL" line form (the index prefix
// disambiguates; see the plan Note). The load-bearing property is that the
// LAST word (index 24 / "ART") is reachable only after paging forward.
func TestMS1DecodeFlowPaging24Words(t *testing.T) {
	const ms1 = "ms10entrsqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqcwugpdxtfme2w" // entr 32B zero → 24 words
	s := mustCodex32T(t, ms1)
	_, _, entropy, err := codex32.DecodeMS1(s)
	if err != nil {
		t.Fatal(err)
	}
	m := bip39.New(entropy)
	if len(m) != 24 {
		t.Fatalf("want 24 words, got %d", len(m))
	}
	// Full "N LABEL" lines for the first, a middle, and the last position. The
	// "24 ART" line is the unambiguous last-word marker reachable only via paging.
	want := make(map[string]bool)
	for _, i := range []int{0, 11, 23} {
		want[fmt.Sprintf("%d %s", i+1, bip39.LabelFor(m[i]))] = false
	}
	ctx := NewContext(newPlatform())
	frame, quit := runUI(ctx, func() { ms1DecodeFlow(ctx, &descriptorTheme, s) })
	for i := 0; i < 40; i++ {
		c, ok := frame()
		if !ok {
			break
		}
		for w := range want {
			// match the "N WORD" line form; the index prefix disambiguates
			if uiContains(c, w) {
				want[w] = true
			}
		}
		click(&ctx.Router, Button3) // advance one page AFTER observing
	}
	quit()
	for w, seen := range want {
		if !seen {
			t.Errorf("line %q never shown — paging skipped it", w)
		}
	}
}
