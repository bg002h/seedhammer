package gui

import (
	"io"
	"testing"

	"seedhammer.com/bip39"
)

const testSeedPhrase = "legal winner thank year wave sausage worth useful legal winner thank yellow"

// nfcTag returns a platform hook handing out one tag, then nothing — the
// behaviour of a real tag crossing the reader once, and of cmd/emu's nfcSource.
func nfcTag(rec string) func() io.ReadCloser {
	return func() io.ReadCloser { return &oneShotNFC{rec: []byte(rec)} }
}

// Stage 10b. §3.1 is NORMATIVE — `seedEntryFlow … offers Typed / Scanned /
// Payload` — and the code offered two of the three. §2.1 made
// NFC-for-everything a deliberate capability; nothing consumed from it.
//
// SCAN is offered whether or not a payload is loaded. It is NOT conditioned on
// NFCReader() != nil, and that is deliberate rather than lazy: on the emulator
// NFCReader() HANDS OUT the pending tag and marks it consumed, so probing for
// nil to decide whether to draw a row would eat the operator's tag before they
// chose anything -- the reader is reported by Features()/FeatureNFC instead.
//
// AMENDED for §13 D9: the platform is given a reader, because the picker is now
// drawn only where a choice exists and a machine with neither source goes
// straight to the keyboard. Without the reader this test would assert that a
// screen D9 deleted still appears. The no-choice case is
// TestSeedEntrySkipsThePickerWhenThereIsNoChoice.
func TestSyswSeedPickerOffersScanWithoutAPayload(t *testing.T) {
	p := newPlatform()
	p.display = sh2DisplaySize
	p.nfc = nfcTag(testSeedPhrase)
	ctx := NewContext(p)
	frame, _, quit := runUITouch(ctx, func() { seedEntryFlow(ctx, &descriptorTheme) })
	defer quit()

	content, ok := pumpUntil(frame, "SCAN", 32)
	if !ok {
		t.Fatalf("the seed seam never offered SCAN; got %q", content)
	}
	if !uiContains(content, "TYPE IT") {
		t.Errorf("the picker dropped the typed source; got %q", content)
	}
	if uiContains(content, "FROM PAYLOAD") {
		t.Errorf("the payload row is offered with no payload loaded; got %q", content)
	}
}

// The consuming half, end to end: a seed on a tag reaches the program.
//
// And it does NOT arrive silently. §3.2 requires the screen where a record
// ENTERS a program to name its source (F3), and §3.3.3's F4 requires a scanned
// SECRET to say that nothing stands behind it — §5.4 scopes digest verification
// to flash, so a tag's contents are admitted on their classification alone.
// This is F4's first production firing; before stage 10 nothing constructed
// srcNFC at all.
func TestSyswSeedScanAcceptsAMnemonicFromATag(t *testing.T) {
	want, err := bip39.ParseMnemonic(testSeedPhrase)
	if err != nil {
		t.Fatal(err)
	}
	p := newPlatform()
	p.display = sh2DisplaySize
	p.nfc = nfcTag(testSeedPhrase)
	ctx := NewContext(p)

	var got bip39.Mnemonic
	var ok bool
	frame, drawer, quit := runUITouch(ctx, func() {
		got, ok = seedEntryFlow(ctx, &descriptorTheme)
	})
	defer quit()

	content, found := pumpUntil(frame, "SCAN", 32)
	if !found {
		t.Fatalf("no source picker; got %q", content)
	}
	click(&ctx.Router, Down)    // TYPE IT -> SCAN
	click(&ctx.Router, Button3) // choose
	content, found = pumpUntil(frame, "no integrity check", 64)
	if !found {
		t.Fatalf("F4 never fired for a secret off a tag; got %q", content)
	}
	if !uiContains(content, "NFC tag") {
		t.Errorf("F3: the acceptance screen does not name the source; got %q", content)
	}
	tapNavSlot(t, ctx, drawer(), Button3) // accept

	for i := 0; i < 32; i++ {
		if _, more := frame(); !more {
			break
		}
	}
	if !ok {
		t.Fatal("the scanned seed was not accepted")
	}
	if got.String() != want.String() {
		t.Errorf("scanned %q, want %q", got.String(), want.String())
	}
}

// A scanned seed may be DECLINED at the acceptance screen, and declining must
// not smuggle it in anyway. Back there returns to the source picker.
func TestSyswSeedScanDeclineDoesNotAcceptTheSeed(t *testing.T) {
	p := newPlatform()
	p.display = sh2DisplaySize
	p.nfc = nfcTag(testSeedPhrase)
	ctx := NewContext(p)

	var got bip39.Mnemonic
	frame, drawer, quit := runUITouch(ctx, func() {
		got, _ = seedEntryFlow(ctx, &descriptorTheme)
	})
	defer quit()

	content, found := pumpUntil(frame, "SCAN", 32)
	if !found {
		t.Fatalf("no source picker; got %q", content)
	}
	click(&ctx.Router, Down)
	click(&ctx.Router, Button3)
	if content, found = pumpUntil(frame, "no integrity check", 64); !found {
		t.Fatalf("no acceptance screen; got %q", content)
	}
	tapNavSlot(t, ctx, drawer(), Button1) // decline
	if content, found = pumpUntil(frame, "TYPE IT", 32); !found {
		t.Fatalf("declining did not return to the source picker; got %q", content)
	}
	quit()
	if got != nil {
		t.Errorf("a declined seed was returned anyway: %q", got.String())
	}
}

// The seam accepts a bip39.Mnemonic AND NOTHING ELSE. A descriptor on a tag is
// not seed material, and admitting one here would hand a program a class its
// §3.3.2 row never granted it — by the back door of "it came off a tag".
func TestSyswSeedScanRefusesANonSeedTag(t *testing.T) {
	const desc = "wpkh([00000000/84h/0h/0h]xpub6BosfCnifzxcFwrSzQiqu2DBVTshkCXacvNsWGYJVVhhawA7d4R5WSWGFNbi8Aw6ZRc1brxMyWMzG3DSSSSoekkudhUd9yLb6qx39T9nMdj/0/*)"
	p := newPlatform()
	p.display = sh2DisplaySize
	p.nfc = nfcTag(desc)
	ctx := NewContext(p)

	frame, _, quit := runUITouch(ctx, func() { seedEntryFlow(ctx, &descriptorTheme) })
	defer quit()

	content, found := pumpUntil(frame, "SCAN", 32)
	if !found {
		t.Fatalf("no source picker; got %q", content)
	}
	click(&ctx.Router, Down)
	click(&ctx.Router, Button3)
	content, found = pumpUntil(frame, "not a seed", 64)
	if !found {
		t.Fatalf("a descriptor on a tag was not refused as a seed; got %q", content)
	}
	if uiContains(content, "no integrity check") {
		t.Errorf("a non-seed reached the acceptance screen; got %q", content)
	}
}

// Stage 10c. engraveObjectFlow is where a top-level scan becomes a program, and
// the two record forms §5.3 added had no case there -- so a tag carrying either
// reached "Unknown format" even once the scanner could parse it.
//
// Declining the acceptance screen leaves the program: there is no source picker
// to return to at a top-level scan, and proceeding with a record the operator
// just refused is the one thing the screen exists to prevent.
func TestEngraveObjectRoutesTheTwoNewScanTypes(t *testing.T) {
	for _, tc := range []struct {
		name   string
		obj    any
		title  string
		wantF4 bool
	}{
		// ClassFreeText is NOT secret -- the class states what the format
		// guarantees, not what a human might put in it -- so F4 must NOT fire.
		{"free text", freeTextScan("hello"), "Engrave Text", false},
		// ClassPassphrase IS secret, and it arrived over NFC: F4.
		{"passphrase", passScan("abandon about"), "BIP-39 Password", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := newPlatform()
			p.display = sh2DisplaySize
			ctx := NewContext(p)
			var routed bool
			frame, drawer, quit := runUITouch(ctx, func() {
				routed = engraveObjectFlow(ctx, &descriptorTheme, tc.obj)
			})
			defer quit()

			content, ok := pumpUntil(frame, "NFC tag", 32)
			if !ok {
				t.Fatalf("%s off a tag reached no acceptance screen; got %q", tc.name, content)
			}
			if !uiContains(content, tc.title) {
				t.Errorf("the acceptance screen is not %s's; got %q", tc.title, content)
			}
			if got := uiContains(content, "no integrity check"); got != tc.wantF4 {
				t.Errorf("F4 fired = %v, want %v; got %q", got, tc.wantF4, content)
			}
			tapNavSlot(t, ctx, drawer(), Button1) // decline
			for i := 0; i < 32; i++ {
				if _, more := frame(); !more {
					break
				}
			}
			if !routed {
				t.Error("engraveObjectFlow reported the tag unroutable")
			}
		})
	}
}

// The record must actually ARRIVE, not merely be announced. A route that
// dropped the body would show the same acceptance screen and then start the
// program empty, which is indistinguishable on the screen this test would
// otherwise stop at.
func TestEngraveTextFromATagPreFillsTheField(t *testing.T) {
	h := newPPHarness(t)
	// Entered through engraveObjectFlow, the way a scan actually arrives, and
	// NOT by calling engraveTextFlowFrom directly. Calling the inner function
	// leaves the ROUTE untested: a case that drops the body and starts the
	// program empty draws exactly the same acceptance screen. Found by mutation
	// -- that mutant survived a version of this test that called inward.
	h.start(func() { engraveObjectFlow(h.ctx, &descriptorTheme, freeTextScan("hello")) })
	h.mustReach("NFC tag")
	tapNavSlot(t, h.ctx, h.drawer(), Button3) // accept
	ftPastQR(h, false)
	if got := ftKbd(h).Fragment; got != "hello" {
		t.Errorf("text field = %q, want %q -- the tag's body never reached the program", got, "hello")
	}
}

func TestEngravePasswordFromATagPreFillsTheField(t *testing.T) {
	h := newPPHarness(t)
	h.start(func() {
		engraveObjectFlow(h.ctx, &descriptorTheme, passScan("abandon about"))
	})
	h.mustReach("no integrity check")
	tapNavSlot(t, h.ctx, h.drawer(), Button3) // accept
	h.mustReach("/100")                       // the entry step's live counter
	kbd, ok := h.widget("kbd").(*PassphraseKeyboard)
	if !ok {
		t.Fatalf("widget \"kbd\" is %T, not a *PassphraseKeyboard", h.widget("kbd"))
	}
	if got := kbd.Fragment; got != "abandon about" {
		t.Errorf("passphrase field = %q, want %q", got, "abandon about")
	}
}

// The typed path is unchanged and says nothing: F3 is "always, for anything not
// typed", so a keyboard entry gets no acceptance screen at all. Without this the
// obvious implementation -- render the source line unconditionally -- passes
// every test above while adding a screen to the two programs' common path.
func TestTypedEntryGetsNoSourceScreen(t *testing.T) {
	h := newPPHarness(t)
	h.start(func() { engraveTextFlow(h.ctx, &descriptorTheme) })
	h.mustReach("QRCode")
	if uiContains(h.content, "Source") {
		t.Errorf("typed entry drew a source line; got %q", h.content)
	}
}
