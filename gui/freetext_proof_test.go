package gui

import (
	"strings"
	"testing"
	"unicode/utf8"

	"seedhammer.com/backup"
	"seedhammer.com/engrave"
	"seedhammer.com/font/sh"
	"seedhammer.com/gui/op"
)

func proofParams() engrave.Params {
	const mm = 200 / 8 * 256
	return engrave.Params{StrokeWidth: 0.3 * mm, Millimeter: mm}
}

// Both patterns must land at 3.0mm -- the SMALLEST rung and the hardest
// legibility case, which is the entire point of the proof (user directive
// 2026-08-04). A pattern that drifted to a larger rung would still engrave, and
// would silently test an easier question than the operator asked for.
func TestProofPatternsLandAtSmallestRung(t *testing.T) {
	p := proofParams()
	for _, tc := range []struct {
		name string
		qr   bool
	}{{"no QR", false}, {"with QR", true}} {
		text := ftProofFor(tc.qr)
		fontMM, lines, code, err := backup.Fit(p, text, ftProofTitle, ftProofFooter, tc.qr)
		if err != nil {
			t.Fatalf("%s: the proof does not fit: %v", tc.name, err)
		}
		if fontMM != 3.0 {
			t.Errorf("%s: proof lands at %.1fmm, want 3.0mm -- %d chars is the wrong length "+
				"to select the smallest rung", tc.name, fontMM, utf8.RuneCountInString(text))
		}
		if len(lines) == 0 {
			t.Errorf("%s: no lines", tc.name)
		}
		if tc.qr != (code != nil) {
			t.Errorf("%s: qr=%v but code!=nil is %v", tc.name, tc.qr, code != nil)
		}
	}
}

// The QR variant exists because the QR competes for the plate. If the full
// pattern were used with a QR it would be REFUSED and the operator would get no
// proof at all -- so this pins that the split is load-bearing, not cosmetic.
func TestProofFullPatternWouldNotFitWithAQR(t *testing.T) {
	p := proofParams()
	if _, _, _, err := backup.Fit(p, ftProofFull, ftProofTitle, ftProofFooter, true); err == nil {
		t.Error("the full pattern now fits with a QR -- the two-variant split is no longer " +
			"needed, or the capacity changed and the lengths should be revisited")
	}
	if _, _, _, err := backup.Fit(p, ftProofWithQR, ftProofTitle, ftProofFooter, true); err != nil {
		t.Errorf("the QR variant does not fit with a QR: %v", err)
	}
}

// Every printable ASCII character must appear, or the proof is not a proof.
// Derived from the codepoint range, so a typo in the literal cannot survive.
func TestProofCoversEveryPrintableASCII(t *testing.T) {
	for _, tc := range []struct {
		name string
		qr   bool
	}{{"no QR", false}, {"with QR", true}} {
		text := ftProofFor(tc.qr)
		var missing []rune
		for r := rune(0x20); r <= 0x7E; r++ {
			if !strings.ContainsRune(text, r) {
				missing = append(missing, r)
			}
		}
		if len(missing) > 0 {
			t.Errorf("%s: proof omits %d printable ASCII: %q", tc.name, len(missing), missing)
		}
	}
}

// The confusables must be ADJACENT -- that is the whole reason they exist
// separately from the codepoint sweep, which by construction separates them.
func TestProofPutsConfusablesSideBySide(t *testing.T) {
	for _, pair := range []string{"rn m", "rnm", "0O", "1lI|", "5S", "2Z", "adoq"} {
		if !strings.Contains(ftProofConfusables, pair) {
			t.Errorf("the confusable table lost %q", pair)
		}
	}
	// And they must survive into both patterns.
	for _, qr := range []bool{false, true} {
		if !strings.Contains(ftProofFor(qr), "rn m rnm") {
			t.Errorf("qr=%v: the rn/m comparison is not in the pattern", qr)
		}
	}
}

// Title and footer must be exactly at the cap, so the plate proves the cap's
// geometry on the screw-hole rows rather than merely staying clear of it.
func TestProofTitleAndFooterSitExactlyAtTheCap(t *testing.T) {
	for name, s := range map[string]string{"title": ftProofTitle, "footer": ftProofFooter} {
		if n := utf8.RuneCountInString(s); n != ftMaxLineLen {
			t.Errorf("%s is %d characters, want exactly %d (the cap) so the plate tests it",
				name, n, ftMaxLineLen)
		}
	}
	// Every rune in them must decode, or engrave.String panics mid-plate.
	for _, s := range []string{ftProofTitle, ftProofFooter} {
		for _, r := range s {
			if r == ' ' {
				continue
			}
			if _, _, ok := sh.Font.Decode(r); !ok {
				t.Errorf("%q in %q does not decode in font/sh", r, s)
			}
		}
	}
}

// The loader writes ALL THREE fields, which is what the prompt promises.
func TestProofLoaderWritesAllThree(t *testing.T) {
	for _, qr := range []bool{false, true} {
		text, title, footer := "old text", "old title", "old footer"
		useQR := qr
		ftProofLoader(&text, &title, &footer, &useQR)()
		if text != ftProofFor(qr) {
			t.Errorf("qr=%v: text not loaded", qr)
		}
		if title != ftProofTitle {
			t.Errorf("qr=%v: title not loaded, got %q", qr, title)
		}
		if footer != ftProofFooter {
			t.Errorf("qr=%v: footer not loaded, got %q", qr, footer)
		}
		if useQR != qr {
			t.Errorf("qr=%v: the loader changed the QR choice to %v -- it must never do that, "+
				"the operator chose it one step earlier and it decides what a scanner returns", qr, useQR)
		}
	}
}

// Whole-field equality: the trigger must not fire on text that merely contains
// it. The frame callback also proves the prompt was never PUT UP -- without it a
// mutation that showed the prompt and had it dismissed would return false too,
// and it stops such a mutation spinning in ftProofPrompt's loop until timeout.
func TestProofNeedsTheWholeField(t *testing.T) {
	newCtx := func(drew *bool) *Context {
		ctx := NewContext(newPlatform())
		ctx.FrameCallback = func(o op.Op) { *drew = true; ctx.Done = true }
		return ctx
	}
	called, drew := false, false
	ctx := newCtx(&drew)
	load := func() { called = true }
	for _, typed := range []string{"", "see TEXTPROOF! for details", "TEXTPROOF", ftProofTrigger + " "} {
		if ftProofOffer(ctx, &descriptorTheme, typed, load) {
			t.Errorf("%q triggered the offer", typed)
		}
	}
	if called {
		t.Error("the loader ran for a field that is not the trigger")
	}
	if drew {
		t.Error("a field that is not the trigger still put the prompt up")
	}
	// A nil loader disables the trigger: it must not prompt and must not panic.
	drew = false
	if ftProofOffer(newCtx(&drew), &descriptorTheme, ftProofTrigger, nil) {
		t.Error("a nil loader still reported the pattern as loaded")
	}
	if drew {
		t.Error("a nil loader still put the prompt up, which no answer could act on")
	}
}
