package gui

import (
	"strings"
	"testing"
	"testing/synctest"

	"github.com/btcsuite/btcd/chaincfg/v2"
	"seedhammer.com/md"
)

// ─── S6b P4: the restore document's passphrase paragraph, spec §6/§6.1 ───────
//
// buildPassphraseInventoryLines' passphrased arm shipped saying "nothing this
// device engraves carries a passphrase." S6b's own passphrase-plate offer
// (engraveSingleSigFlow, S6b P3) can make that false on the SAME predicate
// that shows the offer: a run that CUTS a passphrase plate and then prints
// this document contradicts itself minutes later. GATE 6 requires the
// sentence to condition on whether THIS RUN CUT a plate; GATE 6a requires
// that condition to be CUT, never merely OFFERED -- a declined offer or an
// aborted engrave must leave the shipped text byte-identical; GATE 6.1
// requires the plate, when named, to sit outside `len(plan)` and outside "If
// any of them is missing".

// s6bFullSingleSigCards is a representative full-mode single-sig card set
// (ms1 + mk1 + md1), the ONLY shape engraveSingleSigFlow ever pairs with a
// passphrase-plate offer (S6b spec 2.6: the offer needs `passphrase != ""`,
// and the plate marking's masterFP precondition is full mode's own).
func s6bFullSingleSigCards() []bundleCard {
	return []bundleCard{
		{kind: cardMS1, label: "ms1 secret share", summary: "seed", strings: []string{"ms1a"}},
		{kind: cardMK1, label: "mk1 key", summary: "key", strings: []string{"mk1a"}},
		{kind: cardMD1, label: "md1 descriptor", summary: "policy", strings: []string{"md1a"}},
	}
}

// TestRestoreDocSaysPassphrasePlateWasCutSeparately is GATE 6: on a run that
// CUT a passphrase plate, the document must not claim "nothing this device
// engraves carries a passphrase" -- S6b's own offer falsifies that -- and
// must say a separate plate was cut instead.
func TestRestoreDocSaysPassphrasePlateWasCutSeparately(t *testing.T) {
	doc := strings.Join(buildPassphraseInventoryLines(oneSeedPassphraseFact(true), true), "\n")
	t.Logf("cut-this-run passphrase paragraph:\n%s", doc)

	if strings.Contains(doc, "nothing this device engraves carries a passphrase") {
		t.Errorf("the document still claims no plate carries the passphrase, on a run "+
			"that engraved one:\n%s", doc)
	}
	if !strings.Contains(doc, "A BIP-39 passphrase WAS used") {
		t.Errorf("the document dropped the base fact that a passphrase was used:\n%s", doc)
	}
	if !strings.Contains(doc, "passphrase plate") {
		t.Errorf("the document does not say a passphrase plate exists:\n%s", doc)
	}
	if !strings.Contains(doc, "engraved this run") {
		t.Errorf("the document does not say the passphrase plate belongs to THIS run "+
			"(not some other, unrecorded backup):\n%s", doc)
	}
	if strings.ContainsAny(doc, "—–·‘’“”…") {
		t.Errorf("the replacement text carries a glyph the body face lacks, so it does "+
			"not draw:\n%q", doc)
	}
}

// TestRestoreDocUnchangedWhenPlateNotCut is GATE 6a: the condition is CUT,
// not OFFERED. A declined offer and an aborted engrave are indistinguishable
// to buildPassphraseInventoryLines -- both are passed false -- so this single
// exact-text pin covers both: the shipped two lines must survive BYTE FOR
// BYTE, because the reader of an unaffected run must see exactly what every
// prior release printed.
func TestRestoreDocUnchangedWhenPlateNotCut(t *testing.T) {
	want := []string{
		"A BIP-39 passphrase WAS used. It is not on these plates and cannot be " +
			"recovered from them: nothing this device engraves carries a passphrase.",
		"Without it, these plates do not reach the money. Keep it somewhere " +
			"separate, and make sure whoever needs this backup can also get the " +
			"passphrase.",
	}
	got := buildPassphraseInventoryLines(oneSeedPassphraseFact(true), false)
	if len(got) != len(want) {
		t.Fatalf("got %d line(s), want %d:\n%s", len(got), len(want), strings.Join(got, "\n"))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d changed when the plate was not cut this run:\ngot  %q\nwant %q",
				i, got[i], want[i])
		}
	}
}

// TestPassphrasePlateNotCountedInBackupSet is GATE 6.1: the passphrase plate
// must not enter `len(plan)`, must not appear in the per-card list under
// "This backup is N plates:", and when named at all must be on a line that
// repeats the separation instruction.
func TestPassphrasePlateNotCountedInBackupSet(t *testing.T) {
	cards := s6bFullSingleSigCards()
	wantPlan := len(bundlePlatePlan(engraverParams, cards))

	notCut := buildPlateInventoryLines(engraverParams, cards, oneSeedPassphraseFact(true), seedCapacityOne, false)
	cut := buildPlateInventoryLines(engraverParams, cards, oneSeedPassphraseFact(true), seedCapacityOne, true)

	// THE CENSUS LINE ITSELF MUST NOT MOVE. It is line 0 in both builds and is
	// computed from `plan`, never from the cut flag.
	if cut[0] != notCut[0] {
		t.Fatalf("the plate count line differs by cut status:\ncut:    %q\nnot-cut: %q",
			cut[0], notCut[0])
	}
	if !strings.Contains(cut[0], plateWord(wantPlan, "plate", "plates")) {
		t.Errorf("the plate count line does not name the %d plates bundlePlatePlan "+
			"reports; a passphrase plate may have been counted in:\n%q", wantPlan, cut[0])
	}

	// THE BLOCK BEFORE "If any of them is missing" -- the per-card list -- must
	// never mention a passphrase, on EITHER build: that block is built from
	// `cards` alone, which never gained a passphrase-plate entry.
	missingIdx := -1
	for i, line := range cut {
		if strings.Contains(line, "If any of them is missing") {
			missingIdx = i
			break
		}
	}
	if missingIdx < 0 {
		t.Fatalf("the completeness sentence is missing from the document entirely:\n%s",
			strings.Join(cut, "\n"))
	}
	for i := 0; i <= missingIdx; i++ {
		if strings.Contains(strings.ToLower(cut[i]), "passphrase") {
			t.Errorf("line %d, at or before \"If any of them is missing\", mentions a "+
				"passphrase -- it would read as counted in the set:\n%q", i, cut[i])
		}
	}

	// THE PASSPHRASE PLATE, WHEN NAMED, REPEATS THE SEPARATION INSTRUCTION and
	// says it is not counted -- somewhere AFTER the completeness sentence, never
	// inside the block above it.
	joined := strings.Join(cut[missingIdx+1:], "\n")
	if !strings.Contains(joined, "not counted in this backup") {
		t.Errorf("no line after the completeness sentence says the passphrase plate is "+
			"not counted in this backup:\n%s", joined)
	}
	if !strings.Contains(joined, "somewhere separate") {
		t.Errorf("the passphrase-plate line does not repeat R1's separation "+
			"instruction (\"somewhere separate\"):\n%s", joined)
	}
}

// ─── Wiring: singleSigPassphrasePlateOffer / engravePassphraseFlowPreloaded
// return CUT status, not merely "the offer was shown" ─────────────────────

// TestSingleSigPassphrasePlateOfferNotCutStates is the cheap half of GATE 6a's
// wiring proof: no passphrase (offer never reached) and a declined offer both
// return passphrasePlateNotCut from the REAL function, not a hand-built
// assumption about it. Mirrors TestPassphrasePlateOfferGate's fixture and
// drive, which already proves these two states reach the right SCREENS; this
// asserts what they RETURN.
func TestSingleSigPassphrasePlateOfferNotCutStates(t *testing.T) {
	m := abandonAboutMnemonic()
	path := singleSigPath(84)
	full, masterFP, _, _, err := deriveSingleSigBundle(m, "hunter2", &chaincfg.MainNetParams, path, md.ScriptWpkh)
	if err != nil {
		t.Fatalf("deriveSingleSigBundle: %v", err)
	}

	t.Run("no passphrase", func(t *testing.T) {
		ctx := NewContext(newPlatform())
		var got passphrasePlateResult
		var done bool
		frame, quit := runUI(ctx, func() {
			got = singleSigPassphrasePlateOffer(ctx, &descriptorTheme, m, "", masterFP, path, full)
			done = true
		})
		defer quit()
		for i := 0; i < 16 && !done; i++ {
			frame()
		}
		if !done {
			t.Fatal("singleSigPassphrasePlateOffer never returned with no passphrase")
		}
		if got != passphrasePlateNotCut {
			t.Errorf("got %v, want passphrasePlateNotCut -- no passphrase means the offer "+
				"is never even reached", got)
		}
	})

	t.Run("offer declined", func(t *testing.T) {
		ctx := NewContext(newPlatform())
		var got passphrasePlateResult
		var done bool
		frame, quit := runUI(ctx, func() {
			got = singleSigPassphrasePlateOffer(ctx, &descriptorTheme, m, "hunter2", masterFP, path, full)
			done = true
		})
		defer quit()
		if c, ok := pumpUntil(frame, "Passphrase Plate", 32); !ok {
			t.Fatalf("the offer was not shown; got %q", c)
		}
		click(&ctx.Router, Button3) // "Skip" is choice 0, the default
		for i := 0; i < 16 && !done; i++ {
			frame()
		}
		if !done {
			t.Fatal("singleSigPassphrasePlateOffer never returned after declining")
		}
		if got != passphrasePlateNotCut {
			t.Errorf("got %v, want passphrasePlateNotCut -- a declined offer must not "+
				"read as a cut plate", got)
		}
	})
}

// TestEngravePassphraseFlowPreloadedAbortReturnsNotCut is the other half of
// GATE 6a's wiring proof: an ACCEPTED offer that is then backed OUT OF --
// Back at the passphrase-entry step, which passphraseEntryFlow's own backBtn
// (Button1) turns into (0, false) -- must also return passphrasePlateNotCut.
// This is the "aborted engrave" case spec 6a names separately from
// "declined": the offer was accepted (unlike the case above), but no plate
// was cut.
func TestEngravePassphraseFlowPreloadedAbortReturnsNotCut(t *testing.T) {
	ctx := NewContext(newPlatform())
	var got passphrasePlateResult
	var done bool
	frame, quit := runUI(ctx, func() {
		got = engravePassphraseFlowPreloaded(ctx, &descriptorTheme, []byte("hunter2"), "A1B2C3D4", "5E6F7A8B", "1A2B3C4D")
		done = true
	})
	defer quit()
	if c, ok := pumpUntil(frame, "Source:", 32); !ok {
		t.Fatalf("never reached the acceptance screen; got %q", c)
	}
	click(&ctx.Router, Button3) // accept the source screen
	if c, ok := pumpUntil(frame, "7/100", 32); !ok {
		t.Fatalf("never reached the preloaded entry screen; got %q", c)
	}
	click(&ctx.Router, Button1) // Back out of the FIRST step -- leaves the program
	for i := 0; i < 16 && !done; i++ {
		frame()
	}
	if !done {
		t.Fatal("engravePassphraseFlowPreloaded never returned after Back from the entry step")
	}
	if got != passphrasePlateNotCut {
		t.Errorf("got %v, want passphrasePlateNotCut -- Back out of the first step must "+
			"leave the plate uncut", got)
	}
}

// TestRestoreDocReflectsARealCutPassphrasePlate is the expensive, end-to-end
// half of GATE 6/6.1's wiring proof: it drives the REAL engraveSingleSigFlow
// through an ACTUAL passphrase-plate engrave (not a hand-built call to
// singleSigPassphrasePlateOffer) and reads the FINAL restore document off the
// real screen. TestRestoreDocSaysPassphrasePlateWasCutSeparately and
// TestPassphrasePlateNotCountedInBackupSet already prove
// buildPassphraseInventoryLines/buildPlateInventoryLines render correctly
// GIVEN the right bool; this test is the one that proves engraveSingleSigFlow
// actually COMPUTES that bool from a real cut, and does not, say, always pass
// false, or pass whether the offer was merely shown. T7c's own finding in
// this file's neighbour is exactly this shape: "a swapped argument compiles,
// renders, and looks entirely healthy" is invisible to a helper-level test.
//
// WATCH-ONLY MODE, to keep this bounded: 2 CARDS (mk1+md1, no ms1), which
// this fixture chunks into 5 PLATES, plus the 1 passphrase plate is 6 real
// engraves. Reuses TestPassphrasePlateOfferReachableFromTheOrchestrator's
// exact walk up to the "Passphrase Plate" offer (same fixture, same
// driving), then continues past it rather than duplicating a second
// base-plate walk.
//
// COST, DISCLOSED per the P4 brief: TestPassphrasePlateOfferReachableFromThe-
// Orchestrator alone (the same 5-plate watch-only walk, stopping AT the
// offer) measures ~24.9s. This test adds ONE more real engrave (the
// passphrase plate) plus a few more screens, measured at 26-28s across
// repeated runs -- roughly 1-3s more than the walk already paid for
// elsewhere in this package. Against the package's ~430-450s baseline this
// is a small addition with real margin to Go's 600s per-package default.
func TestRestoreDocReflectsARealCutPassphrasePlate(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		e := newEngraver()
		p := newPlatform()
		p.display = sh2DisplaySize
		p.engraver = e
		ctx := NewContext(p)
		frame, _, _, quit := runUITouchRaster(ctx, func() {
			engraveSingleSigFlow(ctx, &descriptorTheme)
		})
		defer quit()
		frame()
		click(&ctx.Router, Button3) // 12 WORDS
		frame()
		driveWords(&ctx.Router, abandonAboutPhrase())
		if c, ok := pumpUntil(frame, "Wallet Type", 160); !ok {
			t.Fatalf("did not reach wallet-type picker; got %q", c)
		}
		click(&ctx.Router, Button3) // BIP-84
		if c, ok := pumpUntil(frame, "passphrase", 64); !ok {
			t.Fatalf("did not reach passphrase prompt; got %q", c)
		}
		click(&ctx.Router, Down) // Skip -> Add passphrase
		frame()
		click(&ctx.Router, Button3)
		if c, ok := pumpUntil(frame, "Enter Passphrase", 64); !ok {
			t.Fatalf("did not reach passphrase entry; got %q", c)
		}
		runes(&ctx.Router, "hunter2")
		click(&ctx.Router, Button3)
		if c, ok := pumpUntil(frame, "Engrave Mode", 64); !ok {
			t.Fatalf("did not reach the full/watch-only choice; got %q", c)
		}
		click(&ctx.Router, Down) // Watch-only
		frame()
		click(&ctx.Router, Button3)
		if c, ok := pumpUntil(frame, "Engrave wallet policy", 64); !ok {
			t.Fatalf("did not reach the wallet-policy form choice; got %q", c)
		}
		click(&ctx.Router, Button3) // Full policy md1
		if c, ok := pumpUntil(frame, "Plates To Cut", 64); !ok {
			t.Fatalf("the plate census was not shown; got %q", c)
		}
		click(&ctx.Router, Button3)
		if c, ok := pumpUntil(frame, "Card 1 of 2", 64); !ok {
			t.Fatalf("watch-only mode did not reach engrave with 2 cards; got %q", c)
		}
		plates := 0
		for plates < 8 {
			if _, ok := pumpUntil(frame, "Choose engraving", 96); !ok {
				break
			}
			click(&ctx.Router, Button3) // first variant
			frame()
			engraveOnePlate(t, ctx, frame, e)
			plates++
		}
		if plates == 0 {
			t.Fatal("no base plate was cut, so this walk never reached the post-engrave surfaces")
		}
		t.Logf("watch-only mode cut %d base plate(s)", plates)
		if c, ok := pumpUntil(frame, "hand-engrave your ms1", 32); ok {
			click(&ctx.Router, Button3)
			frame()
		} else {
			t.Logf("no ms1 reminder shown; last frame %q", c)
		}
		if c, ok := pumpUntil(frame, "Verify Bundle", 96); !ok {
			t.Fatalf("did not reach the verify offer after engraving %d plate(s); got %q", plates, c)
		}
		click(&ctx.Router, Down) // Skip verify
		frame()
		click(&ctx.Router, Button3)
		if c, ok := pumpUntil(frame, "Passphrase Plate", 96); !ok {
			t.Fatalf("the passphrase-plate offer was not reached after skipping verify; got %q", c)
		}

		// ACCEPT the offer (diverges from
		// TestPassphrasePlateOfferReachableFromTheOrchestrator, which stops
		// here) and drive the preloaded flow to a REAL, ACCEPTED engrave.
		click(&ctx.Router, Down) // Skip -> Engrave
		frame()
		click(&ctx.Router, Button3) // choose Engrave
		if c, ok := pumpUntil(frame, "Source:", 32); !ok {
			t.Fatalf("did not reach the preloaded source-acceptance screen; got %q", c)
		}
		click(&ctx.Router, Button3) // accept the source
		if c, ok := pumpUntil(frame, "/100", 32); !ok {
			t.Fatalf("did not reach the preloaded passphrase-entry screen; got %q", c)
		}
		click(&ctx.Router, Button3) // accept the preloaded passphrase as-is
		if c, ok := pumpUntil(frame, "QR Code", 32); !ok {
			t.Fatalf("did not reach the QR choice; got %q", c)
		}
		click(&ctx.Router, Button3) // accept the QR default (off)
		if c, ok := pumpUntil(frame, "Confirm", 32); !ok {
			t.Fatalf("did not reach the preloaded confirm screen; got %q", c)
		}
		click(&ctx.Router, Button3) // accept: advances to the engrave step
		if c, ok := pumpUntil(frame, "Engrave", 32); !ok {
			t.Fatalf("did not reach the passphrase-plate engrave screen; got %q", c)
		}
		engraveOnePlate(t, ctx, frame, e)

		// The passphrase plate is now CUT. The next screen must be the restore
		// document (GATE 6/6a/6.1's own consumer), not another offer or picker.
		doc := s6aRestoreDocPages(t, ctx, frame)
		t.Logf("restore doc after a real passphrase-plate cut:\n%s", doc)

		if uiContains(doc, "nothing this device engraves carries a passphrase") {
			t.Errorf("the restore document still claims no plate carries the passphrase, "+
				"after this exact run cut one:\n%s", doc)
		}
		if !uiContains(doc, "passphrase plate") || !uiContains(doc, "engraved this run") {
			t.Errorf("the restore document does not say a separate passphrase plate was "+
				"engraved this run:\n%s", doc)
		}
		// GATE 6.1, end to end: the census must still name exactly the `plates`
		// base plates this walk actually cut, not plates+1 -- the passphrase
		// plate engraved above must not have entered the count.
		if !uiContains(doc, plateWord(plates, "plate", "plates")) {
			t.Errorf("the plate count does not name the %d base plate(s) this walk cut; "+
				"the passphrase plate may have been counted in:\n%s", plates, doc)
		}
		if !uiContains(doc, "not counted in this backup") {
			t.Errorf("the restore document does not say the passphrase plate is not "+
				"counted in this backup:\n%s", doc)
		}
	})
}
