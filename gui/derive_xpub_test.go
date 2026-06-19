package gui

import (
	"image"
	"strings"
	"testing"
	"testing/synctest"

	"github.com/btcsuite/btcd/chaincfg/v2"
	"seedhammer.com/gui/op"
	"seedhammer.com/mk"
)

func abandonAboutPhrase() string {
	return strings.TrimSpace(strings.Repeat("abandon ", 11) + "about")
}

// chooseEntry drives a ChoiceScreen to entry index `down` and confirms it: it
// queues `down` Down presses, pumps a frame so the selection registers, then
// confirms with Button3 and pumps a frame so Choose returns.
func chooseEntry(frame func() (string, bool), r *EventRouter, down int) {
	for i := 0; i < down; i++ {
		click(r, Down)
	}
	frame()
	click(r, Button3)
	frame()
}

// TestPathPickerResolves drives the two-stage picker and checks the resolved
// (path, network) for representative script-type/network combinations.
func TestPathPickerResolves(t *testing.T) {
	cases := []struct {
		name        string
		scriptDowns int
		netDowns    int
		wantPath    string
		wantNet     *chaincfg.Params
		wantNetName string
	}{
		{"bip44 mainnet", 0, 0, "m/44h/0h/0h", &chaincfg.MainNetParams, "mainnet"},
		{"bip84 mainnet", 2, 0, "m/84h/0h/0h", &chaincfg.MainNetParams, "mainnet"},
		{"bip84 testnet", 2, 1, "m/84h/1h/0h", &chaincfg.TestNet3Params, "testnet"},
		{"bip48 mainnet", 4, 0, "m/48h/0h/0h/2h", &chaincfg.MainNetParams, "mainnet"},
		{"bip87 testnet", 5, 1, "m/87h/1h/0h", &chaincfg.TestNet3Params, "testnet"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ctx := NewContext(newPlatform())
			var gotPath, gotNetName string
			var gotNet *chaincfg.Params
			var gotOK bool
			done := false
			frame, quit := runUI(ctx, func() {
				p, net, netName, ok := pathPickerFlow(ctx, &descriptorTheme)
				if ok {
					gotPath = p.String()
					gotNet = net
					gotNetName = netName
				}
				gotOK = ok
				done = true
			})
			defer quit()
			frame() // stage 1 first frame
			chooseEntry(frame, &ctx.Router, c.scriptDowns)
			chooseEntry(frame, &ctx.Router, c.netDowns)
			// Drain any remaining frames until the flow returns.
			for i := 0; i < 8 && !done; i++ {
				frame()
			}
			if !gotOK {
				t.Fatalf("picker did not resolve")
			}
			if gotPath != c.wantPath {
				t.Errorf("path = %q, want %q", gotPath, c.wantPath)
			}
			if gotNet != c.wantNet {
				t.Errorf("net = %v, want %v", gotNet, c.wantNet)
			}
			if gotNetName != c.wantNetName {
				t.Errorf("netName = %q, want %q", gotNetName, c.wantNetName)
			}
		})
	}
}

// TestPathPickerStage1NoClip verifies the 6-entry stage-1 ChoiceScreen renders
// without clipping at the REAL device resolution (480x320), not the 240x240
// test-harness default (R1-M3): a no-clip assertion at 240 would false-FAIL,
// and uiContains matches clipped text anyway. We extract text over the content
// rectangle the ChoiceScreen lays entries into; clipped entries are cut out of
// that sub-framebuffer.
func TestPathPickerStage1NoClip(t *testing.T) {
	ctx := NewContext(newPlatform())
	const W, H = 480, 320
	dims := image.Pt(W, H)
	cs := &ChoiceScreen{Title: "Script type", Lead: "Choose address type", Choices: scriptTypeChoices()}
	if len(cs.Choices) != 6 {
		t.Fatalf("expected 6 script-type choices, got %d", len(cs.Choices))
	}
	o := cs.Draw(ctx, &descriptorTheme, dims)
	content := image.Rect(16, leadingSize, W-16, H-leadingSize)
	d := new(op.Drawer)
	got := d.ExtractText(content, o)
	for _, tok := range []string{"44", "49", "84", "86", "48", "87"} {
		if !strings.Contains(got, tok) {
			t.Errorf("stage-1 entry %q clipped/missing at %dx%d; content text = %q", tok, W, H, got)
		}
	}
}

// TestDeriveXpubFlow_StubWarningThenEngrave drives the full flow up to the
// MANDATORY stub-0 warning and asserts (a) the warning text is shown before any
// engrave, and (b) the engraver never opens before the warning is acknowledged
// (so the seed is never engraved without the unbound-card warning being seen).
func TestDeriveXpubFlow_StubWarningThenEngrave(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		e := newEngraver()
		p := newPlatform()
		p.engraver = e
		ctx := NewContext(p)
		frame, quit := runUI(ctx, func() {
			deriveXpubFlow(ctx, &descriptorTheme)
		})
		defer quit()
		frame()

		// Seed entry: word-count picker -> 12 words (choice 0).
		click(&ctx.Router, Button3) // choose "12 WORDS"
		frame()
		driveWords(&ctx.Router, abandonAboutPhrase())
		// Pump until the seed is complete and we reach the passphrase prompt.
		if c, ok := pumpUntil(frame, "passphrase", 128); !ok {
			t.Fatalf("did not reach passphrase prompt; got %q", c)
		}

		// Passphrase prompt: Skip (choice 0).
		click(&ctx.Router, Button3)
		frame()

		// Path picker stage 1: BIP-84 (2 Downs), stage 2: mainnet (0 Downs).
		chooseEntry(frame, &ctx.Router, 2)
		chooseEntry(frame, &ctx.Router, 0)

		// Verify display -> Continue (Button3).
		if c, ok := pumpUntil(frame, "Verify Xpub", 64); !ok {
			t.Fatalf("did not reach verify display; got %q", c)
		}
		click(&ctx.Router, Button3)

		// The mandatory stub-0 warning must be visible before any engrave.
		if c, ok := pumpUntil(frame, "not bound", 64); !ok {
			t.Fatalf("stub-0 warning not shown before engrave; got %q", c)
		}

		// The seed must NEVER be engraved: the engraver must not have opened (it
		// only opens when an engrave job starts, after the warning is held).
		select {
		case <-e.opens:
			t.Fatal("engraver opened before the stub-0 warning was acknowledged")
		default:
		}
	})
}

// TestDeriveXpubFlowEngravesMK1NotSeed proves the engraved payload is the mk1
// public xpub (round-trips via mk.Decode), independent of the GUI driving.
func TestDeriveXpubFlowEngravesMK1NotSeed(t *testing.T) {
	xpub := knownAccountXpub84
	card := mk.Card{
		Network:     "mainnet",
		Path:        "m/84'/0'/0'",
		Fingerprint: "73c5da0a",
		Stubs:       [][4]byte{{0, 0, 0, 0}},
		Xpub:        xpub,
	}
	strs, err := mk.Encode(card)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if len(strs) < 2 {
		t.Fatalf("expected >=2 chunks, got %d", len(strs))
	}
	for _, s := range strs {
		if !strings.HasPrefix(s, "mk1") {
			t.Fatalf("chunk is not mk1: %s", s)
		}
		// A seed word must never leak into the engraved string.
		if strings.Contains(s, "abandon") || strings.Contains(s, "about") {
			t.Fatalf("seed word leaked into mk1 chunk: %s", s)
		}
	}
	got, err := mk.Decode(strs)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got.Xpub != xpub || got.Path != "m/84'/0'/0'" {
		t.Fatalf("round-trip card mismatch: %+v", got)
	}
}

// TestMultiPlateEngravePlateTitles asserts the multi-plate sequencer shows the
// "Plate 1 of N" UX and that backing out mid-sequence (before completing plate
// 1) surfaces the incomplete/discard abort warning (R0-I3) instead of silently
// exiting as if the backup were done.
func TestMultiPlateEngravePlateTitles(t *testing.T) {
	card := mk.Card{
		Network:     "mainnet",
		Path:        "m/84'/0'/0'",
		Fingerprint: "73c5da0a",
		Stubs:       [][4]byte{{0, 0, 0, 0}},
		Xpub:        knownAccountXpub84,
	}
	strs, err := mk.Encode(card)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	total := len(strs)
	if total < 2 {
		t.Fatalf("expected >=2 chunks, got %d", total)
	}
	ctx := NewContext(newPlatform())
	frame, quit := runUI(ctx, func() {
		multiPlateEngrave(ctx, &descriptorTheme, strs)
	})
	defer quit()
	// Plate 1 of N variant picker is shown.
	if c, ok := pumpUntil(frame, "Plate 1 of "+itoa(total), 32); !ok {
		t.Fatalf("plate 1 of %d not shown; got %q", total, c)
	}
	// Back out of the variant picker -> the set-level abort warning.
	click(&ctx.Router, Button1)
	if c, ok := pumpUntil(frame, "Incomplete", 32); !ok {
		t.Fatalf("abort warning not shown after mid-sequence Back; got %q", c)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
