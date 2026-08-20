package gui

import (
	"image"
	"testing"

	"seedhammer.com/bip39"
	"seedhammer.com/gui/assets"
	"seedhammer.com/gui/op"
)

// s5DriveVerifyToPassphrase drives multisigVerifyFlow up to the passphrase
// prompt for the FIRST seed and hands back the frame pump, so a test can then
// press Back and watch what the flow does with it.
//
// Split out of s5DriveVerifyTwoSeeds (multisig_verify_policy_test.go), which
// always answers the passphrase prompt with Skip and so can never observe the
// Back.
func s5DriveVerifyToPassphrase(t *testing.T, records []string, expected []int,
	engravedMd1 []string, seed string) (*Context, func() (string, bool), func()) {
	t.Helper()
	p := newPlatform()
	p.display = sh2DisplaySize
	ctx := NewContext(p)
	ctx.syswBundleSeeds = append([]string(nil), records...)

	frame, quit := runUI(ctx, func() {
		multisigVerifyFlow(ctx, &descriptorTheme, false, expected, engravedMd1, &verifyRecord{})
	})

	if c, ok := pumpUntil(frame, "mk1 keys:", 64); !ok {
		quit()
		t.Fatalf("the readback never reached the gatherer's tally; got %q", c)
	}
	click(&ctx.Router, Button3) // Done adding cards
	frame()

	if c, ok := pumpUntil(frame, "Choose number of words", 64); !ok {
		quit()
		t.Fatalf("seed entry was not reached; got %q", c)
	}
	click(&ctx.Router, Button3) // 12 WORDS
	frame()
	driveWords(&ctx.Router, seed)
	if c, ok := pumpUntil(frame, "passphrase", 240); !ok {
		quit()
		t.Fatalf("the passphrase prompt was not reached; got %q", c)
	}
	return ctx, frame, quit
}

// TestVerifyBackAtPassphraseDoesNotSkipIt is the directive's case in the flow
// that motivated it (2026-08-19, "going back should lose nothing").
//
// The passphrase prompt is read as
//
//	if sel, ok := ppChoice.Choose(ctx, th); ok && sel == 1 { ... }
//
// so a Back (ok == false) falls straight through with passphrase "" and the
// flow DERIVES. Back does not go back here -- it silently does what "Skip"
// does, and then commits to a verification the operator was trying to leave.
//
// Back must return to the seed entry instead.
func TestVerifyBackAtPassphraseDoesNotSkipIt(t *testing.T) {
	md1, plates, _ := s5TraceBEngraved(t, false)
	records := append([]string(nil), md1...)
	for _, pl := range plates {
		records = append(records, pl...)
	}
	ctx, frame, quit := s5DriveVerifyToPassphrase(t, records, []int{0, 1, 2}, md1, fixtureMasterA)
	defer quit()

	click(&ctx.Router, Button1) // Back at the passphrase prompt
	frame()

	// Where the flow lands. Back must NOT push forward into a verdict.
	last := ""
	for i := 0; i < 160; i++ {
		c, ok := frame()
		if !ok {
			break
		}
		last = c
		if uiContains(c, "Word") || uiContains(c, "Choose number of words") {
			return // stepped back to the seed entry, which is the fix
		}
		if uiContains(c, "Verify OK") || uiContains(c, "Verify Failed") ||
			uiContains(c, "Verify Incomplete") || uiContains(c, "not checked yet") {
			t.Fatalf("Back at the passphrase prompt went FORWARD into %q: it was "+
				"read as Skip, so the flow derived with an empty passphrase and "+
				"committed to a verification the operator was leaving", c)
		}
	}
	t.Fatalf("Back at the passphrase prompt reached neither the seed entry nor a "+
		"verdict; the flow parked at %q", last)
}

// TestVerifyBackAtPassphraseKeepsTheSeed is the directive's actual words --
// "going back should LOSE NOTHING" -- rather than merely its control flow.
//
// Stepping back to a blank keyboard would satisfy the test above and still cost
// the operator all twelve words. So this one walks FORWARD again from the
// resumed entry WITHOUT retyping anything, and requires the flow to arrive back
// at the passphrase prompt. It can only do that if the words were still there.
func TestVerifyBackAtPassphraseKeepsTheSeed(t *testing.T) {
	md1, plates, _ := s5TraceBEngraved(t, false)
	records := append([]string(nil), md1...)
	for _, pl := range plates {
		records = append(records, pl...)
	}
	ctx, frame, quit := s5DriveVerifyToPassphrase(t, records, []int{0, 1, 2}, md1, fixtureMasterA)
	defer quit()

	click(&ctx.Router, Button1) // Back at the passphrase prompt
	if c, ok := pumpUntil(frame, "Word", 96); !ok {
		t.Fatalf("Back did not return to the seed entry; got %q", c)
	}

	// NOT ONE WORD IS RETYPED. There is deliberately no runes() call here:
	// anything typed would be the test supplying the seed the flow is supposed
	// to have kept. Button2 is the `done` affordance, which a resumed entry
	// draws exactly while every slot is filled.
	//
	// Before that affordance existed this cost one retyped word -- the commit
	// path skips filled slots and confirms on running off the end, so word 1 was
	// the only way forward out of a complete entry. Eleven words survived the
	// Back and one did not.
	click(&ctx.Router, Button2)
	frame()
	if c, ok := pumpUntil(frame, "passphrase", 240); !ok {
		t.Fatalf("walking forward through the resumed seed did not reach the "+
			"passphrase prompt; got %q.\nBack returned to the seed entry but did "+
			"not bring the operator's words with it", c)
	}
}

// TestResumingDrawsDoneOnlyWhenComplete pins the `resuming` affordance against
// the rule it must not break.
//
// §8c: `done` "cannot appear where a length is already known". A resumed entry
// is exactly that case, so the affordance is allowed only while the known length
// is also SATISFIED -- otherwise a 12-word entry holding six words could be
// terminated as if it were finished, which is a short seed confirmed as a whole
// one.
//
// The script is the same on every arm: press done, then press back. Back always
// ends the entry, so the flow terminates whether or not done is live, and the
// two are told apart by the returned bool rather than by a hang.
func TestResumingDrawsDoneOnlyWhenComplete(t *testing.T) {
	run := func(filled int, opts wordEntryOpts) bool {
		ctx := NewContext(newPlatform())
		m := emptyBIP39Mnemonic(12)
		for i := 0; i < filled; i++ {
			m[i] = bip39.Word(i)
		}
		var confirmed, ended bool
		frame, drawer, quit := runUITouch(ctx, func() {
			_, confirmed = inputWordsFlow(ctx, &descriptorTheme, m, 0, "", opts)
			ended = true
		})
		defer quit()
		frame()

		// TAPPED, NOT CLICKED. click() dispatches by button and so reaches a
		// handler whose Clickable was never drawn -- which is the historical
		// defect this code carries a paragraph about: an undrawn Clickable
		// installs no touch target, so `done` was accepted in code and
		// unreachable on the machine. A tap resolves a POSITION through the
		// drawer, so it can only succeed if the button is really on screen.
		if hasNavSlot(ctx, drawer(), Button2) {
			tapNavSlot(t, ctx, drawer(), Button2)
		}
		for i := 0; i < 64 && !ended; i++ {
			frame()
		}
		if ended {
			return confirmed
		}
		// done was not live. Back always ends the entry, so the arm still
		// terminates and reports its answer instead of hanging.
		click(&ctx.Router, Button1)
		for i := 0; i < 64 && !ended; i++ {
			frame()
		}
		if !ended {
			t.Fatal("neither done nor back ended the word entry")
		}
		return confirmed
	}

	if !run(12, wordEntryOpts{checksumGate: true, resuming: true}) {
		t.Error("a COMPLETE resumed entry did not offer done, so the only way " +
			"out of it forwards is retyping word 1 -- the loss the affordance exists to remove")
	}
	if run(6, wordEntryOpts{checksumGate: true, resuming: true}) {
		t.Error("a PARTIAL resumed entry accepted done: that confirms a 6-word " +
			"mnemonic as a finished 12-word one, and breaks the rule that done " +
			"cannot appear where a length is already known")
	}
	if run(12, wordEntryOpts{checksumGate: true}) {
		t.Error("a NON-resuming entry drew done; the affordance must be opt-in")
	}
}

// hasNavSlot reports whether a nav button is DRAWN in b's slot, without
// failing the test when it is not -- tapNavSlot's job is to assert presence,
// this one's is to ask.
func hasNavSlot(ctx *Context, d *op.Drawer, b Button) bool {
	dims := ctx.Platform.DisplaySize()
	sz := assets.NavBtnPrimary.Bounds().Size()
	ys := [3]int{leadingSize, (dims.Y - sz.Y) / 2, dims.Y - leadingSize - sz.Y}
	pos := image.Pt(dims.X-sz.X/2, ys[int(b-Button1)]+sz.Y/2)
	tag, _, hit := d.Hit(pos)
	if !hit {
		return false
	}
	c, ok := tag.(*Clickable)
	return ok && (c.Button == b || c.AltButton == b)
}
