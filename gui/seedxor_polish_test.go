package gui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/btcsuite/btcd/chaincfg/v2"
	"seedhammer.com/bip39"
)

// Coldcard docs/seed-xor.md worked examples (see seedxor/testdata/SOURCE.md).
var seedXOR24 = []string{
	"romance wink lottery autumn shop bring dawn tongue range crater truth ability miss spice fitness easy legal release recall obey exchange recycle dragon room",
	"lion misery divide hurry latin fluid camp advance illegal lab pyramid unaware eager fringe sick camera series noodle toy crowd jeans select depth lounge",
	"vault nominee cradle silk own frown throw leg cactus recall talent worry gadget surface shy planet purpose coffee drip few seven term squeeze educate",
}

const seedXOR24Result = "silent toe meat possible chair blossom wait occur this worth option bag nurse find fish scene bench asthma bike wage world quit primary indoor"

// driveWords emits per-word input that inputWordsFlow expects: each word's
// (lowercase) letters followed by Button3. A full BIP-39 word is an exact match
// → completeBIP39Word completes it (likewise an exact last-word candidate).
func driveWords(r *EventRouter, phrase string) {
	for _, w := range strings.Fields(phrase) {
		runes(r, strings.ToLower(w))
		click(r, Button3)
	}
}

// pickCombineSeedXOR queues the two pickers (part count, word length) for
// combineSeedXORFlow: parts is index into {2,3,4,5}; words is index into
// {12,18,24}. nDown/wDown are the number of Down presses to reach each.
func pickCombineSeedXOR(r *EventRouter, nDown, wDown int) {
	for i := 0; i < nDown; i++ {
		click(r, Down)
	}
	click(r, Button3) // choose part count
	for i := 0; i < wDown; i++ {
		click(r, Down)
	}
	click(r, Button3) // choose word length
}

func mfpHex(t *testing.T, phrase string) string {
	t.Helper()
	m, err := bip39.ParseMnemonic(phrase)
	if err != nil {
		t.Fatalf("parse result vector: %v", err)
	}
	mfp, err := masterFingerprintFor(m, &chaincfg.MainNetParams, "")
	if err != nil {
		t.Fatalf("fingerprint: %v", err)
	}
	return fmt.Sprintf("%.8X", mfp)
}

func TestCombineSeedXOR(t *testing.T) {
	// N=3, length=24: enter the three Coldcard 24-word parts, confirm the
	// recovered fingerprint matches, Engrave → backupWalletFlow shows the
	// recovered seed words (SILENT ...).
	ctx := NewContext(newPlatform())
	frame, quit := runUI(ctx, func() {
		m, ok := combineSeedXORFlow(ctx, &descriptorTheme)
		if ok {
			backupWalletFlow(ctx, &descriptorTheme, m)
		}
	})
	defer quit()

	// part count {2,3,4,5}: index 1 = 3 (one Down); word length {12,18,24}:
	// index 2 = 24 (two Down).
	pickCombineSeedXOR(&ctx.Router, 1, 2)
	for _, p := range seedXOR24 {
		driveWords(&ctx.Router, p)
	}
	frame()

	wantFp := mfpHex(t, seedXOR24Result)
	if c, ok := pumpUntil(frame, wantFp, 96); !ok {
		t.Fatalf("recovered fingerprint %s not shown at gate; got %q", wantFp, c)
	}
	// Engrave at the mandatory gate → backupWalletFlow renders the seed words.
	click(&ctx.Router, Button3)
	if c, ok := pumpUntil(frame, "SILENT", 96); !ok {
		t.Fatalf("recovered seed words did not reach backupWalletFlow; got %q", c)
	}
}

func TestSeedXORFingerprintMandatory(t *testing.T) {
	// The fingerprint gate is on the only success path: Back at the gate →
	// (nil,false), no engrave.
	ctx := NewContext(newPlatform())
	var got bip39.Mnemonic
	var gotOK bool
	frame, quit := runUI(ctx, func() {
		got, gotOK = combineSeedXORFlow(ctx, &descriptorTheme)
	})
	defer quit()

	pickCombineSeedXOR(&ctx.Router, 1, 2)
	for _, p := range seedXOR24 {
		driveWords(&ctx.Router, p)
	}
	frame()
	wantFp := mfpHex(t, seedXOR24Result)
	if c, ok := pumpUntil(frame, wantFp, 96); !ok {
		t.Fatalf("gate fingerprint %s not shown; got %q", wantFp, c)
	}
	// Back at the gate (drain Button2 first, then Back).
	click(&ctx.Router, Button2, Button1)
	// Drain remaining frames.
	for i := 0; i < 8; i++ {
		if _, ok := frame(); !ok {
			break
		}
	}
	quit()
	if gotOK || got != nil {
		t.Errorf("Back at gate: got (%v,%v) want (nil,false)", got, gotOK)
	}
}

func TestSeedXORBackoutRecognized(t *testing.T) {
	// Back during part entry (partial fill) → (nil,false); the I1 guard prevents
	// any Entropy() panic on the partial slice.
	ctx := NewContext(newPlatform())
	pickCombineSeedXOR(&ctx.Router, 1, 2) // N=3, 24 words
	// Enter only a couple of words of part 1, then Back out of the word flow.
	runes(&ctx.Router, "silent")
	click(&ctx.Router, Button3)
	click(&ctx.Router, Button1) // Back out of inputWordsFlow (partial part)
	m, ok := combineSeedXORFlow(ctx, &descriptorTheme)
	if ok || m != nil {
		t.Errorf("Back during part entry: got (%v,%v) want (nil,false)", m, ok)
	}
}

func TestSeedXORPartCountBackout(t *testing.T) {
	// Back at the part-count picker → (nil,false).
	ctx := NewContext(newPlatform())
	click(&ctx.Router, Button1) // Back at "How many parts?"
	m, ok := combineSeedXORFlow(ctx, &descriptorTheme)
	if ok || m != nil {
		t.Errorf("Back at part-count: got (%v,%v) want (nil,false)", m, ok)
	}
}

func TestSeedXORPartLengthBackout(t *testing.T) {
	// Back at the word-length picker → (nil,false).
	ctx := NewContext(newPlatform())
	click(&ctx.Router, Button3) // choose 2 parts (default)
	click(&ctx.Router, Button1) // Back at "Words per part?"
	m, ok := combineSeedXORFlow(ctx, &descriptorTheme)
	if ok || m != nil {
		t.Errorf("Back at word-length: got (%v,%v) want (nil,false)", m, ok)
	}
}

func TestConfirmSeedXORFingerprintButton2NoHang(t *testing.T) {
	// Direct-call: queued Button2 (must be drained) then Button3 (Engrave) → the
	// gate returns true without stalling on the held Button2.
	ctx := NewContext(newPlatform())
	click(&ctx.Router, Button2, Button3)
	if !confirmSeedXORFingerprint(ctx, &descriptorTheme, 0xDEADBEEF) {
		t.Error("gate did not return true after Button2-drain + Engrave (possible hang)")
	}
}

func TestNewInputFlowSeedXOREntry(t *testing.T) {
	// The "SEED XOR" menu entry (index 4) routes into combineSeedXORFlow and
	// returns the recovered bip39.Mnemonic through newInputFlow — riding the
	// existing engraveObjectFlow case bip39.Mnemonic: path. Drive: select the
	// 5th choice (4 Down), enter the 3 parts, confirm at the gate.
	ctx := NewContext(newPlatform())
	var obj any
	var gotOK bool
	frame, quit := runUI(ctx, func() {
		obj, gotOK = newInputFlow(ctx, &descriptorTheme)
	})
	defer quit()

	click(&ctx.Router, Down, Down, Down, Down, Button3) // choose "SEED XOR" (index 4)
	pickCombineSeedXOR(&ctx.Router, 1, 2)               // N=3, 24 words
	for _, p := range seedXOR24 {
		driveWords(&ctx.Router, p)
	}
	frame()
	wantFp := mfpHex(t, seedXOR24Result)
	if c, ok := pumpUntil(frame, wantFp, 96); !ok {
		t.Fatalf("gate fingerprint %s not shown via menu; got %q", wantFp, c)
	}
	click(&ctx.Router, Button3) // Engrave at the gate → newInputFlow returns
	for i := 0; i < 8; i++ {
		if _, ok := frame(); !ok {
			break
		}
	}
	quit()
	if !gotOK {
		t.Fatal("newInputFlow did not return ok for SEED XOR")
	}
	m, isM := obj.(bip39.Mnemonic)
	if !isM {
		t.Fatalf("newInputFlow returned %T, want bip39.Mnemonic", obj)
	}
	want, _ := bip39.ParseMnemonic(seedXOR24Result)
	if m.String() != want.String() {
		t.Errorf("recovered = %q, want %q", m.String(), want.String())
	}
}

func TestConfirmSeedXORFingerprintNamesNoCheck(t *testing.T) {
	// The Seed-XOR gate copy must name the absence of a built-in check.
	ctx := NewContext(newPlatform())
	frame, quit := runUI(ctx, func() {
		confirmSeedXORFingerprint(ctx, &descriptorTheme, 0x01234567)
	})
	defer quit()
	c, _ := frame()
	if !uiContains(c, "01234567") {
		t.Errorf("gate did not render the fingerprint; got %q", c)
	}
	if !uiContains(c, "no built-in check") {
		t.Errorf("gate copy missing the no-check warning; got %q", c)
	}
}
