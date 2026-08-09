package gui

import (
	"fmt"
	"image"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"seedhammer.com/gui/assets"
	"seedhammer.com/gui/widget"
	"seedhammer.com/seal"
)

// §10.2 steps 1-4 at the UI: Inspect, the §6.6 hash screen, the §10.2.3
// unauthenticated warning, and B1's honest stop on a sealed payload.

// runUnlock drives unlockPayloadFlow over a blob and returns a frame pump plus
// a "has the flow returned" flag.
func runUnlock(t *testing.T, r seal.Reader) (frame func() (string, bool), done *bool, ctx *Context, quit func()) {
	t.Helper()
	p := newPlatform()
	p.display = sh2DisplaySize
	ctx = NewContext(p)
	returned := false
	frame, quit = runUI(ctx, func() {
		unlockPayloadFlow(ctx, &descriptorTheme, r)
		returned = true
	})
	return frame, &returned, ctx, quit
}

// hashScreen pumps to the §10.2 step 3 hash screen and returns what it drew.
func hashScreen(t *testing.T, r seal.Reader) string {
	t.Helper()
	frame, _, _, quit := runUnlock(t, r)
	defer quit()
	content, ok := pumpUntil(frame, "Public data hash", 32)
	if !ok {
		t.Fatalf("never reached the public-data-hash screen; last frame %q", content)
	}
	return content
}

// TestUnlockShowsTheHashItsCountAndItsShape — §10.2 step 3. The expected digest
// is the vector file's own pubhash, not a value retyped here.
func TestUnlockShowsTheHashItsCountAndItsShape(t *testing.T) {
	for _, tc := range []struct {
		vector string
		shape  string
	}{
		{"D", "SEALED"},
		{"E", "UNSEALED"},
	} {
		t.Run(tc.vector, func(t *testing.T) {
			v := sealVector(t, tc.vector)
			want := v.PubhashSealed
			if tc.shape == "UNSEALED" {
				want = v.PubhashUnsealed
			}
			if want == nil {
				t.Fatalf("vector %s has no %s pubhash", tc.vector, tc.shape)
			}
			content := hashScreen(t, payloadReaderFrom(t, v.blob(t)))
			if !uiContains(content, *want) {
				t.Errorf("the hash screen does not show %s's %s digest %s; got %q",
					tc.vector, tc.shape, *want, content)
			}
			// The count is the PUBLIC record count, which is what §6.6 hashes.
			if !uiContains(content, "5 records") {
				t.Errorf("the hash screen does not show the public record count; got %q", content)
			}
			// ANCHORED, and the anchor is the whole point: "SEALED" is a
			// SUBSTRING of "UNSEALED" and uiContains is strings.Contains, so a
			// bare "SEALED" needle is satisfied by a screen reading UNSEALED.
			// Hardcoding unlockShape to return "UNSEALED" passed the ENTIRE
			// suite -- measured, whole-diff review round 0. The gap was
			// one-directional (the opposite mutant was caught), which is
			// exactly why it read as correct.
			//
			// unlockHashBody emits "(%d records, %s):", so the leading comma
			// binds directly to the "s": ", SEALED):" cannot match
			// ", UNSEALED):".
			if !uiContains(content, ", "+tc.shape+"):") {
				t.Errorf("the hash screen does not show the %s shape; got %q", tc.shape, content)
			}
			// ...and the other word must be ABSENT, so neither direction can
			// pass by accident.
			other := "SEALED"
			if tc.shape == "SEALED" {
				other = "UNSEALED"
			}
			if uiContains(content, ", "+other+"):") {
				t.Errorf("the hash screen shows %s on a %s payload; got %q", other, tc.shape, content)
			}
		})
	}
}

// TestUnlockHashMakesADowngradeVisible. Vectors D and E carry the SAME five
// public records and differ only in whether anything is encrypted, so a screen
// that ignored the sealed flag would show one digest for both -- and stripping
// the ciphertext and tag off a sealed payload would then be invisible to an
// operator comparing the number they recorded.
//
// This is Phase A's downgrade property re-asserted where the operator actually
// meets it, which is the screen.
func TestUnlockHashMakesADowngradeVisible(t *testing.T) {
	d := sealVector(t, "D")
	e := sealVector(t, "E")
	if strings.Join(d.Public, "\n") != strings.Join(e.Public, "\n") {
		t.Fatal("premise broken: D and E must carry identical public records")
	}
	dScreen := hashScreen(t, payloadReaderFrom(t, d.blob(t)))
	eScreen := hashScreen(t, payloadReaderFrom(t, e.blob(t)))
	if dScreen == eScreen {
		t.Fatal("the sealed and unsealed screens render identically; the downgrade is invisible")
	}
	if uiContains(dScreen, *e.PubhashUnsealed) {
		t.Errorf("the SEALED screen shows the unsealed digest %s: %q", *e.PubhashUnsealed, dScreen)
	}
	if uiContains(eScreen, *d.PubhashSealed) {
		t.Errorf("the UNSEALED screen shows the sealed digest %s: %q", *d.PubhashSealed, eScreen)
	}
}

// TestUnlockShowsNoHashWhenThePublicSectionIsEmpty — §10.2 step 3's other half.
// pub_len == 0 means the digest of an EMPTY record set, which is a constant:
// displaying it on every fully encrypted payload would teach the operator the
// number is furniture.
func TestUnlockShowsNoHashWhenThePublicSectionIsEmpty(t *testing.T) {
	v := sealVector(t, "F")
	if v.PubLen != 0 {
		t.Fatalf("premise broken: vector F must have pub_len 0, got %d", v.PubLen)
	}
	frame, _, _, quit := runUnlock(t, payloadReaderFrom(t, v.blob(t)))
	defer quit()
	// The constant an empty public section would hash to, computed here rather
	// than retyped, so the assertion tracks §6.6 rather than a snapshot.
	constant := seal.FormatHash(seal.PublicDataHash(nil, true))
	for i := 0; i < 32; i++ {
		content, ok := frame()
		if !ok {
			break
		}
		if uiContains(content, "Public data hash") {
			t.Fatalf("a payload with no public section drew a hash region: %q", content)
		}
		if uiContains(content, constant) {
			t.Fatalf("a payload with no public section drew the empty-set constant %s: %q", constant, content)
		}
	}
}

// TestUnlockRefusesAnUnreadablePayload. Every §6.2/§6.3 violation but one reads
// as "payload unreadable", which is what §2.2 item 4 has taught the operator to
// read as "someone replaced my payload".
func TestUnlockRefusesAnUnreadablePayload(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mangle func(b []byte) []byte
	}{
		{"bad magic", func(b []byte) []byte { b[0] = 'X'; return b }},
		{"unknown version", func(b []byte) []byte { b[8] = 0x02; return b }},
		{"truncated", func(b []byte) []byte { return b[:len(b)-8] }},
		{"corrupt public record", func(b []byte) []byte {
			// A byte inside the public section: BCH-valid no longer, so the
			// card set does not decode.
			b[seal.HeaderLen+4] = 'q'
			return b
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			blob := tc.mangle(sealVectorBlob(t, "E"))
			frame, _, _, quit := runUnlock(t, payloadReaderFrom(t, blob))
			defer quit()
			content, ok := pumpUntil(frame, "Payload unreadable", 32)
			if !ok {
				t.Fatalf("a %s payload did not report \"Payload unreadable\"; got %q", tc.name, content)
			}
		})
	}
}

// TestUnlockDistinguishesTooManyRecords — §6.4 requires it. Conflating a wallet
// larger than the machine accepts with a tampered blob would send the operator
// chasing a compromise that did not happen.
func TestUnlockDistinguishesTooManyRecords(t *testing.T) {
	recs := make([]string, seal.MaxRecords+1)
	for i := range recs {
		recs[i] = "md1qqq"
	}
	section := strings.Join(recs, "\n")
	h := seal.Header{PubLen: uint32(len(section))}
	enc := h.Encode()
	blob := append(enc[:], section...)

	frame, _, _, quit := runUnlock(t, payloadReaderFrom(t, blob))
	defer quit()
	content, ok := pumpUntil(frame, "more records than the machine accepts", 32)
	if !ok {
		t.Fatalf("a %d-record payload did not get its own message; got %q", len(recs), content)
	}
	if uiContains(content, "Payload unreadable") {
		t.Errorf("too-many-records was reported as an unreadable payload: %q", content)
	}
}

// TestSealedPayloadReachesThePassphraseAndCancelsToTheMenu REPLACES
// TestSealedPayloadStopsAtATerminalScreen, which asserted the exact behaviour
// B2a removes: B1 had no passphrase flow, so a sealed payload said so and
// stopped. B2a has one, so the terminal screen is gone.
//
// What does NOT relax is the rule underneath it. Cancelling must still return
// to the menu and must still NOT fall through to the plate list: p.Public on a
// sealed payload is a legitimate record set, and engraving it while silently
// dropping the encrypted half is §6.4's incomplete-backup-believed-complete,
// the worst available outcome.
//
// Driven by touch, because the screen it now reaches has a keyboard.
func TestSealedPayloadReachesThePassphraseAndCancelsToTheMenu(t *testing.T) {
	for _, name := range []string{"D", "G"} {
		t.Run(name, func(t *testing.T) {
			v := sealVector(t, name)
			if v.CtLen == 0 {
				t.Fatalf("premise broken: vector %s must be sealed", name)
			}
			h := newUnlockHarness(t, payloadReaderFrom(t, v.blob(t)))
			// The B1 terminal screen must be GONE, not merely bypassed.
			content := h.mustReach("Public data hash")
			if uiContains(content, "Unlocking is not available") {
				t.Fatalf("the B1 terminal screen is still on the hash frame: %q", content)
			}
			h.tapNav(Button3)
			content = h.mustReach("Enter the 12-word passphrase")
			if uiContains(content, "Unlocking is not available") {
				t.Fatalf("a sealed payload still reports unlocking unavailable: %q", content)
			}
			h.tapNav(Button3)
			h.mustReach("Word 1 of 12")

			// Back out with nothing typed: the flow leaves, and the plate list
			// is NEVER constructed. Asserted against the labels the list would
			// have drawn, not against a return value.
			h.tapNav(Button1)
			labels := plateRecords(t, name)
			for i := 0; i < 64 && !*h.done; i++ {
				c, ok := h.frame()
				if !ok {
					break
				}
				for j := range labels {
					if uiContains(c, plateLabel(labels[j], j)) {
						t.Fatalf("cancelling a sealed payload reached the plate list: %q", c)
					}
				}
			}
			if !*h.done {
				t.Fatal("cancelling the passphrase did not return to the menu")
			}
		})
	}
}

// TestUnauthenticatedWarningShownOnlyWhenNothingIsEncrypted. The negative half
// is the one that matters: a warning shown on a SEALED payload is a false claim
// that the payload is unauthenticated.
func TestUnauthenticatedWarningShownOnlyWhenNothingIsEncrypted(t *testing.T) {
	// ct_len == 0 -> shown, with the same digest the hash screen produced.
	e := sealVector(t, "E")
	frame, _, ctx, quit := runUnlock(t, payloadReaderFrom(t, e.blob(t)))
	defer quit()
	if content, ok := pumpUntil(frame, "Public data hash", 32); !ok {
		t.Fatalf("never reached the hash screen; got %q", content)
	}
	click(&ctx.Router, Button3)
	content, ok := pumpUntil(frame, "NOT AUTHENTICATED", 32)
	if !ok {
		t.Fatalf("an unsealed payload did not reach the §10.2.3 warning; got %q", content)
	}
	if !uiContains(content, *e.PubhashUnsealed) {
		t.Errorf("the warning body does not carry the hash %s; got %q", *e.PubhashUnsealed, content)
	}
	// It must not claim the device checked anything.
	for _, forbidden := range []string{"verified", "signature valid", "authenticated OK"} {
		if uiContains(content, forbidden) {
			t.Errorf("the warning implies the device verified the payload (%q): %q", forbidden, content)
		}
	}
	quit()

	// ct_len > 0 -> NEVER shown.
	d := sealVector(t, "D")
	frame2, _, ctx2, quit2 := runUnlock(t, payloadReaderFrom(t, d.blob(t)))
	defer quit2()
	if content, ok := pumpUntil(frame2, "Public data hash", 32); !ok {
		t.Fatalf("never reached the hash screen; got %q", content)
	}
	click(&ctx2.Router, Button3)
	for i := 0; i < 32; i++ {
		c, ok := frame2()
		if !ok {
			break
		}
		if uiContains(c, "NOT AUTHENTICATED") {
			t.Fatalf("a SEALED payload was declared unauthenticated: %q", c)
		}
	}
}

// TestUnauthenticatedWarningCancelReturnsToTheMenu — §10.2 step 4 requires an
// explicit confirmation, so declining must leave the flow entirely.
func TestUnauthenticatedWarningCancelReturnsToTheMenu(t *testing.T) {
	frame, done, ctx, quit := runUnlock(t, payloadReaderFor(t, "E"))
	defer quit()
	if content, ok := pumpUntil(frame, "Public data hash", 32); !ok {
		t.Fatalf("never reached the hash screen; got %q", content)
	}
	click(&ctx.Router, Button3)
	if content, ok := pumpUntil(frame, "NOT AUTHENTICATED", 32); !ok {
		t.Fatalf("never reached the warning; got %q", content)
	}
	click(&ctx.Router, Button1) // Cancel
	// The plate list must never be constructed on the declined path.
	labels := plateRecords(t, "E")
	for i := 0; i < 32 && !*done; i++ {
		c, ok := frame()
		if !ok {
			break
		}
		for j := range labels {
			if uiContains(c, plateLabel(labels[j], j)) {
				t.Fatalf("declining the §10.2.3 warning still reached the plate list: %q", c)
			}
		}
	}
	if !*done {
		t.Fatal("cancelling the §10.2.3 warning did not return to the menu")
	}
}

// TestUnauthenticatedWarningConfirmProceeds. Hold-to-confirm is the "explicit
// confirmation" of §10.2 step 4; a tap must not be enough, and only a
// confirmation reaches the plate list.
func TestUnauthenticatedWarningConfirmProceeds(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		frame, _, ctx, quit := runUnlock(t, payloadReaderFor(t, "E"))
		defer quit()
		if content, ok := pumpUntil(frame, "Public data hash", 32); !ok {
			t.Fatalf("never reached the hash screen; got %q", content)
		}
		click(&ctx.Router, Button3)
		if content, ok := pumpUntil(frame, "NOT AUTHENTICATED", 32); !ok {
			t.Fatalf("never reached the warning; got %q", content)
		}
		// A tap is NOT a confirmation: the warning must still be on screen.
		click(&ctx.Router, Button3)
		content, ok := frame()
		if !ok {
			t.Fatal("no frame after the tap")
		}
		if !uiContains(content, "NOT AUTHENTICATED") {
			t.Fatalf("a bare tap dismissed the warning: %q", content)
		}
		press(&ctx.Router, Button3)
		frame()
		time.Sleep(confirmDelay)
		// §10.2 step 4: "require an explicit confirmation, then go to the plate
		// list". Vector E's first entry is the single mk1 card's first plate.
		if content, ok := pumpUntil(frame, "mk1 1/2", 32); !ok {
			t.Fatalf("holding to confirm did not reach the plate list; got %q", content)
		}
	})
}

// TestUnauthenticatedWarningFitsThePanel — §10.2.3's copy against the screen it
// is drawn on (lens 4 MINOR 4).
//
// The paragraph at the bottom of this body is §2.2 item 10's downgrade
// instruction: "the encrypted part has been REMOVED. Do not continue." It is
// the one sentence that tells the operator to stop, and it is the one that
// falls off first.
//
// Three facts stack, and this test pins the third:
//
//  1. Warning.Layout computes maxScroll = bodysz.Y - (bodyClip.Dy() -
//     2*scrollFadeDist), and for this copy that is POSITIVE -- the widget
//     believes the last line is scrolled out of view.
//  2. Its only scroll input is ButtonFilter(Up)/ButtonFilter(Down). The SH2 has
//     no directional buttons, so the body cannot be scrolled on the machine.
//  3. Nothing is cut off today only because fadeClip is a no-op stub, so the
//     body renders past bodyClip.Max.Y into the last few pixels of the panel.
//
// So the guarantee that the operator can READ the instruction rests entirely on
// the body still fitting the PANEL. That is what is asserted, with the margin
// reported, so a copy edit that eats it fails here rather than on hardware.
func TestUnauthenticatedWarningFitsThePanel(t *testing.T) {
	pf := newPlatform()
	pf.display = sh2DisplaySize
	ctx := NewContext(pf)
	dims := ctx.Platform.DisplaySize()

	// Warning.Layout's own geometry, reproduced from gui.go's constants rather
	// than hardcoded, so a change to either is caught.
	const btnMargin = 4
	const boxMargin = 6
	btnOff := assets.NavBtnPrimary.Bounds().Dx() + btnMargin
	bodyClip := image.Rectangle{
		Min: image.Pt(boxMargin, leadingSize),
		Max: image.Pt(dims.X-btnOff, dims.Y-boxMargin),
	}

	// Every legal public record count, since the count and the digest are both
	// interpolated into the copy and a wider count wraps a line.
	for _, n := range []int{1, 5, 9, 12, 24} {
		t.Run(fmt.Sprintf("%d records", n), func(t *testing.T) {
			p := &seal.Payload{Public: make([]seal.AdmittedRecord, n), HasHash: true}
			body := unlockUnauthenticatedBody(p)
			if !strings.Contains(body, "Do not continue.") {
				t.Fatal("premise broken: §10.2.3's copy must carry the downgrade instruction")
			}
			_, sz := widget.Labelw(&ctx.B, ctx.Styles.body, bodyClip.Dx(), descriptorTheme.Text, body)

			top := bodyClip.Min.Y + scrollFadeDist
			bottom := top + sz.Y
			maxScroll := sz.Y - (bodyClip.Dy() - 2*scrollFadeDist)
			t.Logf("bodyClip=%v body=%dx%d top=%d bottom=%d panel=%d maxScroll=%d",
				bodyClip, sz.X, sz.Y, top, bottom, dims.Y, maxScroll)

			if bottom > dims.Y {
				t.Errorf("§10.2.3's body ends at y=%d on a %d-pixel panel (%d px over)\n"+
					"the paragraph that falls off is §2.2 item 10's \"the encrypted part "+
					"has been REMOVED. Do not continue.\" -- and Warning scrolls only on "+
					"Up/Down buttons, which the SeedHammer II does not have, so it is "+
					"unreachable rather than merely below the fold",
					bottom, dims.Y, bottom-dims.Y)
			}
		})
	}
}
