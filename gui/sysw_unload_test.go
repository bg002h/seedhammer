package gui

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"

	"seedhammer.com/sysw"
)

func loadedSession(sealed, digestShown bool) *syswSession {
	return &syswSession{
		loaded:      true,
		compared:    true,
		sealed:      sealed,
		digestShown: digestShown,
		records:     []syswRecord{{class: sysw.ClassMnemonic, body: testSeedPhrase}},
	}
}

// THE CONFIRM SCREEN IS THE FEATURE, and its wording is what is being asserted.
// A test that only checked the screen APPEARED would pass on silence — and the
// three cases are genuinely asymmetric, so one wording for all of them would be
// wrong twice over.
//
//   - UNSEALED: sysw/open.go returns before DeriveKey, so reloading costs one
//     digest comparison and no KDF at all. Naming a passphrase here would send
//     the operator looking for something that does not exist.
//   - SEALED: a full passphrase entry and its KDF.
//   - SEALED with pub_len == 0: `[digest-shown]` (§12.4) shows nothing, so the
//     AEAD open is the ONLY route to `[compared]` (§12.2). Most expensive
//     unload in the system.
func TestSyswUnloadConfirmStatesWhatReloadingCosts(t *testing.T) {
	for _, tc := range []struct {
		name          string
		sealed        bool
		digestShown   bool
		wantPassword  bool
		wantDigest    bool
		wantEmphatic  bool
		forbidPhrases []string
	}{
		{
			name: "plaintext — one digest comparison, no KDF at all",
			// A passphrase named here is a hunt for something that was never set.
			forbidPhrases: []string{"passphrase"},
			wantDigest:    true,
		},
		{
			name:         "sealed with a digest — the passphrase and its KDF",
			sealed:       true,
			digestShown:  true,
			wantPassword: true,
		},
		{
			name:         "sealed, pub_len == 0 — the passphrase is the ONLY route back",
			sealed:       true,
			digestShown:  false,
			wantPassword: true,
			wantEmphatic: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := newPlatform()
			p.display = sh2DisplaySize
			ctx := NewContext(p)
			ctx.sysw = loadedSession(tc.sealed, tc.digestShown)

			frame, _, quit := runUITouch(ctx, func() { syswUnloadFlow(ctx, &descriptorTheme) })
			defer quit()
			content, ok := pumpUntil(frame, "UNLOAD", 32)
			if !ok {
				t.Fatalf("no unload confirmation; got %q", content)
			}
			if !uiContains(content, "load it again") {
				t.Errorf("the confirmation does not say the payload can be reloaded; got %q", content)
			}
			if got := uiContains(content, "passphrase"); got != tc.wantPassword {
				t.Errorf("names the passphrase = %v, want %v; got %q", got, tc.wantPassword, content)
			}
			if tc.wantDigest && !uiContains(content, "compare the digest") {
				t.Errorf("an unsealed payload's confirmation does not name the digest "+
					"comparison, which is the whole of what reloading costs it; got %q", content)
			}
			if tc.wantEmphatic && !uiContains(content, "nothing else will do") {
				t.Errorf("the pub_len == 0 case is worded no more emphatically than the "+
					"one that still has a digest, and it is the one that cannot be "+
					"recovered any other way; got %q", content)
			}
			if !tc.wantEmphatic && uiContains(content, "nothing else will do") {
				t.Errorf("a payload with another route back claims there is none; got %q", content)
			}
			for _, bad := range tc.forbidPhrases {
				if uiContains(content, bad) {
					t.Errorf("the confirmation names %q for a payload that has none; got %q",
						bad, content)
				}
			}
		})
	}
}

// BACK at the confirmation leaves the session alone. Without this, a confirm
// screen that unloaded regardless would pass every wording assertion above.
func TestSyswUnloadBackKeepsTheSession(t *testing.T) {
	p := newPlatform()
	p.display = sh2DisplaySize
	ctx := NewContext(p)
	ctx.sysw = loadedSession(true, true)

	var ret bool
	frame, drawer, quit := runUITouch(ctx, func() {
		ret = syswUnloadFlow(ctx, &descriptorTheme)
	})
	defer quit()
	content, ok := pumpUntil(frame, "UNLOAD", 32)
	if !ok {
		t.Fatalf("no confirmation; got %q", content)
	}
	tapNavSlot(t, ctx, drawer(), Button1) // Back
	for i := 0; i < 32; i++ {
		if _, more := frame(); !more {
			break
		}
	}
	if ret {
		t.Error("Back reported a payload unloaded")
	}
	if ctx.sysw == nil {
		t.Error("Back dropped the session anyway")
	}
}

// UNLOAD drops the session and SAYS WHAT IT DID NOT DO. The bytes are still in
// flash; a screen that let the operator believe otherwise is the over-claim
// F-123 was filed against, and §13 D10 exists precisely because "erase" would
// be that claim.
//
// Driven BY TOUCH on the confirm's nav slot, not by a synthesised button event:
// a control that is never drawn cannot be pressed on this panel, which is how
// §8c's `done` shipped unreachable.
func TestSyswUnloadDropsTheSessionAndSaysTheBytesRemain(t *testing.T) {
	p := newPlatform()
	p.display = sh2DisplaySize
	ctx := NewContext(p)
	ctx.sysw = loadedSession(false, true)

	var ret bool
	frame, drawer, quit := runUITouch(ctx, func() {
		ret = syswUnloadFlow(ctx, &descriptorTheme)
	})
	defer quit()
	content, ok := pumpUntil(frame, "UNLOAD", 32)
	if !ok {
		t.Fatalf("no confirmation; got %q", content)
	}
	click(&ctx.Router, Down)              // BACK -> UNLOAD
	tapNavSlot(t, ctx, drawer(), Button3) // choose
	if content, ok = pumpUntil(frame, "unloaded", 32); !ok {
		t.Fatalf("no result screen after unloading; got %q", content)
	}
	if !uiContains(content, "still in flash") {
		t.Errorf("the result screen does not say the bytes are still there; got %q", content)
	}
	if !uiContains(content, "me sysw wipe") {
		t.Errorf("the result screen does not name the HOST command that actually "+
			"overwrites the region; got %q", content)
	}
	if uiContains(content, "erase") {
		t.Errorf("§13 D10: the word \"erase\" must not appear — the bytes are still "+
			"there and saying otherwise is a lie the operator might act on; got %q", content)
	}
	tapNavSlot(t, ctx, drawer(), Button3) // dismiss the notice (ErrorScreen's only nav)
	for i := 0; i < 32; i++ {
		if _, more := frame(); !more {
			break
		}
	}
	if !ret {
		t.Error("UNLOAD did not report a payload unloaded")
	}
	if ctx.sysw != nil {
		t.Error("UNLOAD left the session in place, so every program would still " +
			"offer FROM PAYLOAD")
	}
}

// The carousel entry keeps its ONE meaning when nothing is loaded — which is
// every machine at boot — and grows the choice only when there is one. LOAD
// AGAIN must survive: re-reading the region is journey J-E, and it worked before
// this stage.
func TestSyswPayloadMenuOffersBothOnlyWhenAPayloadIsLoaded(t *testing.T) {
	t.Run("nothing loaded: straight to the load flow", func(t *testing.T) {
		p := newPlatform()
		p.display = sh2DisplaySize
		ctx := NewContext(p) // p.sysw is nil: no region at all
		frame, _, quit := runUITouch(ctx, func() { syswPayloadMenu(ctx, &descriptorTheme) })
		defer quit()
		content, ok := pumpUntil(frame, "No payload found", 32)
		if !ok {
			t.Fatalf("the entry did not go straight to the load flow; got %q", content)
		}
		if uiContains(content, "UNLOAD") {
			t.Errorf("an unload choice was offered with nothing loaded; got %q", content)
		}
	})
	t.Run("loaded: both, with LOAD AGAIN kept", func(t *testing.T) {
		p := newPlatform()
		p.display = sh2DisplaySize
		ctx := NewContext(p)
		ctx.sysw = loadedSession(false, true)
		frame, _, quit := runUITouch(ctx, func() { syswPayloadMenu(ctx, &descriptorTheme) })
		defer quit()
		content, ok := pumpUntil(frame, "UNLOAD", 32)
		if !ok {
			t.Fatalf("a loaded payload offered no unload; got %q", content)
		}
		if !uiContains(content, "LOAD AGAIN") {
			t.Errorf("the entry lost its ability to re-read the region (journey J-E); "+
				"got %q", content)
		}
	})
}

// THE WIRING OF `[digest-shown]` INTO THE SESSION, end to end, over a real
// sealed pub_len == 0 region image.
//
// Found by mutation: hard-coding digestShown to true at the load site SURVIVED
// the wording tests above, because they construct the session by hand. The
// wording is only right if the fact reaching it is right, and this is the only
// test in the tree that opens a sealed systemwide payload for real.
func TestASecretsOnlyPayloadUnloadNamesThePassphraseAsTheOnlyWayBack(t *testing.T) {
	p := newPlatform()
	p.display = sh2DisplaySize
	p.sysw = syswRegionFor(t, "S-D") // sealed, secrets only: pub_len == 0
	ctx := NewContext(p)

	var loaded bool
	frame, drawer, quit := runUITouch(ctx, func() {
		loaded = syswLoadFlow(ctx, &descriptorTheme, ctx.Platform.SyswReader(), false)
		if loaded {
			syswUnloadFlow(ctx, &descriptorTheme)
		}
	})
	defer quit()

	content, ok := pumpUntil(frame, "1:", 64)
	if !ok {
		t.Fatalf("a sealed payload did not ask for a passphrase; got %q", content)
	}
	// The vector's own passphrase: eleven `abandon` and `about`.
	words := make([]string, 12)
	for i := range words {
		words[i] = "abandon"
	}
	words[11] = "about"
	for i, w := range words {
		runes(&ctx.Router, w)
		click(&ctx.Router, Button3)
		if _, more := frame(); !more {
			t.Fatalf("passphrase entry ended after %d words", i)
		}
	}
	tapNavSlot(t, ctx, drawer(), Button2) // `done`
	if content, ok = pumpUntil(frame, "12 words", 32); !ok {
		t.Fatalf("no count confirmation; got %q", content)
	}
	click(&ctx.Router, Down)              // BACK -> UNLOCK
	tapNavSlot(t, ctx, drawer(), Button3) // unlock: runs the KDF and opens

	// pub_len == 0, so `[digest-shown]` (§12.4) shows nothing and the AEAD open
	// is the ONLY route to [compared] (§12.2) — which is exactly why the unload
	// confirmation must say the passphrase is the only way back.
	if content, ok = pumpUntil(frame, "load it again", 64); !ok {
		t.Fatalf("the payload did not open, or the unload confirmation never "+
			"appeared; got %q", content)
	}
	if uiContains(content, "compare the digest") {
		t.Errorf("a payload with NO digest offers a digest comparison as the way "+
			"back; got %q", content)
	}
	if !uiContains(content, "nothing else will do") {
		t.Errorf("the only-route-back case is not worded as one; got %q", content)
	}
	if !uiContains(content, "PASSPHRASE") {
		t.Errorf("the confirmation does not name the passphrase; got %q", content)
	}
}

// §3.3.3's F1 row reads "offers erase". Since §13 D10 there is no erase, and
// what F1 offers instead is UNLOAD — offered right where the operator learns a
// secret is sitting unencrypted in flash, rather than leaving them to find the
// carousel entry that undoes what they were just warned about.
//
// Driven through the REAL load flow over a real region image, so this fails if
// the offer is wired to a warning list that never fires.
func TestF1OffersUnloadWhereItOnceOfferedErase(t *testing.T) {
	p := newPlatform()
	p.display = sh2DisplaySize
	p.sysw = syswRegionFor(t, "S-B") // plaintext, carrying a SECRET class
	ctx := NewContext(p)

	var ret bool
	frame, drawer, quit := runUITouch(ctx, func() {
		ret = syswLoadFlow(ctx, &descriptorTheme, ctx.Platform.SyswReader(), false)
	})
	defer quit()

	// [compared] route 1: the operator compares the displayed digest. Without
	// this the flow never reaches the flag summary at all.
	content, ok := pumpUntil(frame, "Compare this against", 64)
	if !ok {
		t.Fatalf("no digest comparison for a payload with a public section; got %q", content)
	}
	tapNavSlot(t, ctx, drawer(), Button3)
	if content, ok = pumpUntil(frame, "unencrypted in flash", 64); !ok {
		t.Fatalf("F1 never fired for a plaintext payload carrying a secret; got %q", content)
	}
	tapNavSlot(t, ctx, drawer(), Button3) // acknowledge the warnings
	if content, ok = pumpUntil(frame, "Keep this payload loaded?", 32); !ok {
		t.Fatalf("F1 offered nothing to do about it; got %q", content)
	}
	if uiContains(content, "erase") {
		t.Errorf("§13 D10: the offer still says erase; got %q", content)
	}
	click(&ctx.Router, Down)              // KEEP -> UNLOAD
	tapNavSlot(t, ctx, drawer(), Button3) // choose
	if content, ok = pumpUntil(frame, "load it again", 32); !ok {
		t.Fatalf("the F1 offer did not reach the unload confirmation; got %q", content)
	}
	click(&ctx.Router, Down)              // BACK -> UNLOAD
	tapNavSlot(t, ctx, drawer(), Button3) // confirm
	if content, ok = pumpUntil(frame, "still in flash", 32); !ok {
		t.Fatalf("no unload result screen; got %q", content)
	}
	tapNavSlot(t, ctx, drawer(), Button3) // dismiss
	for i := 0; i < 32; i++ {
		if _, more := frame(); !more {
			break
		}
	}
	if ctx.sysw != nil {
		t.Error("unloading from the F1 offer left the session behind")
	}
	if ret {
		t.Error("syswLoadFlow reported a payload loaded after it was unloaded again")
	}
}

// The offer is F1's, NOT "any warning's". Found by mutation: inverting the flag
// test inside syswHasFlag SURVIVED, because the F1 payload also raises F3 and an
// offer keyed on the wrong flag still appeared.
//
// S-E is the discriminator: sealed under a two-word passphrase over a secret, so
// F2 fires and F1 cannot — the secret is not sitting in plaintext. The operator
// is warned and NOT asked whether to unload, because nothing about a weak
// passphrase is fixed by dropping the session.
func TestAWarningThatIsNotF1OffersNoUnload(t *testing.T) {
	p := newPlatform()
	p.display = sh2DisplaySize
	p.sysw = syswRegionFor(t, "S-E") // sealed, 2-word passphrase, carries a secret
	ctx := NewContext(p)

	frame, drawer, quit := runUITouch(ctx, func() {
		syswLoadFlow(ctx, &descriptorTheme, ctx.Platform.SyswReader(), false)
	})
	defer quit()

	content, ok := pumpUntil(frame, "1:", 64)
	if !ok {
		t.Fatalf("a sealed payload did not ask for a passphrase; got %q", content)
	}
	for _, w := range []string{"abandon", "about"} {
		runes(&ctx.Router, w)
		click(&ctx.Router, Button3)
		if _, more := frame(); !more {
			t.Fatal("passphrase entry ended early")
		}
	}
	tapNavSlot(t, ctx, drawer(), Button2) // `done`
	if content, ok = pumpUntil(frame, "2 words", 32); !ok {
		t.Fatalf("no count confirmation; got %q", content)
	}
	click(&ctx.Router, Down)
	tapNavSlot(t, ctx, drawer(), Button3) // UNLOCK
	if content, ok = pumpUntil(frame, "Compare this against", 64); !ok {
		t.Fatalf("no digest comparison; got %q", content)
	}
	tapNavSlot(t, ctx, drawer(), Button3)
	if content, ok = pumpUntil(frame, "word-count floor", 64); !ok {
		t.Fatalf("F2 never fired for a weakly-protected secret; got %q", content)
	}
	if uiContains(content, "unencrypted in flash") {
		t.Fatalf("INCONCLUSIVE: F1 fired too, so this payload cannot tell the two "+
			"apart; got %q", content)
	}
	tapNavSlot(t, ctx, drawer(), Button3) // acknowledge the warning
	for i := 0; i < 32; i++ {
		c, more := frame()
		if !more {
			break
		}
		if uiContains(c, "Keep this payload loaded?") {
			t.Fatalf("an unload was offered for a warning that is not F1; got %q", c)
		}
	}
	if ctx.sysw == nil {
		t.Error("the payload was not loaded at all, so this test proves nothing " +
			"about what happens after the warning")
	}
}

// §13 D10, ASSERTED OVER THE TREE: the firmware has no flash-write path, and
// this stage must not have introduced one. Verified before the ruling that the
// fork had no such capability at all — so an Eraser here would have been the
// first, which is exactly what the ruling removed.
//
// The plan's own green command is `grep -rn "Erase\|erase" gui/ sysw/`; this is
// that command as a test, so it runs on every suite rather than once.
func TestNoErasePathExistsOnTheDevice(t *testing.T) {
	for _, dir := range []string{".", "../sysw"} {
		ents, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("reading %s: %v", dir, err)
		}
		var checked int
		for _, e := range ents {
			name := e.Name()
			if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
				continue
			}
			// The `seal` unlock program has its own, PRE-EXISTING wipe
			// vocabulary (WipeSecretAt, wipe_guard.go) over decrypted RECORDS in
			// RAM. It is a different container, frozen, and it writes no flash.
			// Narrowing the needle to the sysw surface keeps this test about
			// what D10 forbids rather than about the word.
			if !strings.HasPrefix(name, "sysw") && dir == "." {
				continue
			}
			checked++
			src, err := os.ReadFile(dir + "/" + name)
			if err != nil {
				t.Fatal(err)
			}
			if i := strings.Index(strings.ToLower(string(src)), "erase"); i >= 0 {
				// The word is allowed in a comment that says it is NOT done;
				// what may not exist is an identifier.
				fset := token.NewFileSet()
				f, perr := parser.ParseFile(fset, dir+"/"+name, src, 0)
				if perr != nil {
					t.Fatalf("parsing %s: %v", name, perr)
				}
				ast.Inspect(f, func(n ast.Node) bool {
					id, ok := n.(*ast.Ident)
					if ok && strings.Contains(strings.ToLower(id.Name), "erase") {
						t.Errorf("%s/%s declares or names %q — §13 D10: the firmware "+
							"never writes flash, and `erase` would claim bytes are gone "+
							"that are still there", dir, name, id.Name)
					}
					return true
				})
			}
		}
		if checked == 0 {
			t.Errorf("INCONCLUSIVE: no files scanned under %s, so this test guards nothing", dir)
		}
	}
	// And no Platform method for it, which is the seam an Eraser would have
	// needed.
	src, err := os.ReadFile("gui.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(src), "SyswEraser") {
		t.Error("Platform grew a SyswEraser — §13 D10 is a decision, not a deferral")
	}
}
