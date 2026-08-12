package gui

import (
	"testing"

	"seedhammer.com/sysw"
)

// §13 D9. The source picker is offered ONLY WHEN A CHOICE ACTUALLY EXISTS.
//
// Stage 10 made §3.1's picker the first screen of every seed entry in BIP-85,
// Account Xpub, Single-Sig and Multisig -- the most-walked path in the firmware
// -- and on a machine with no payload and no tag reader it offered one usable
// row and a dead one. The operator ruled that back: with neither source, entry
// goes straight to typing.
//
// The needle is the word-count picker, which is the screen typed entry OPENS
// with. Asserting only "no picker" would pass on a flow that drew nothing at
// all, which is a different bug with the same first frame.
func TestSeedEntrySkipsThePickerWhenThereIsNoChoice(t *testing.T) {
	p := newPlatform()
	p.display = sh2DisplaySize
	// No payload, and no reader: p.nfc stays nil, so Features() reports no
	// FeatureNFC.
	ctx := NewContext(p)
	if ctx.Platform.Features().Has(FeatureNFC) {
		t.Fatal("INCONCLUSIVE: the test platform claims a reader, so this test " +
			"cannot see the no-choice case it exists for")
	}
	frame, _, quit := runUITouch(ctx, func() { seedEntryFlow(ctx, &descriptorTheme) })
	defer quit()

	content, ok := frame()
	if !ok {
		t.Fatal("seed entry produced no frame at all")
	}
	if !uiContains(content, "Choose number of words") {
		t.Errorf("the FIRST screen of seed entry is not the word-count picker; got %q", content)
	}
	if uiContains(content, "Where from?") {
		t.Errorf("§13 D9: a source picker was drawn with no source to pick; got %q", content)
	}
}

// The reader-alone case is TestSyswSeedPickerOffersScanWithoutAPayload
// (sysw_source_test.go), which stage 10 already wrote and D9 amended.
//
// A payload alone is a choice too -- and with no reader the SCAN row must be
// GONE, not merely useless. Offering a row that leads to a screen the operator
// can only Back out of is the cost D9 was ruled against.
func TestSeedEntryOffersThePickerForAPayloadAloneAndDrawsNoScanRow(t *testing.T) {
	p := newPlatform()
	p.display = sh2DisplaySize
	ctx := NewContext(p)
	ctx.sysw = &syswSession{
		loaded:   true,
		compared: true,
		records:  []syswRecord{{class: sysw.ClassMnemonic, body: testSeedPhrase}},
	}

	frame, _, quit := runUITouch(ctx, func() { seedEntryFlow(ctx, &descriptorTheme) })
	defer quit()

	content, ok := pumpUntil(frame, "Where from?", 32)
	if !ok {
		t.Fatalf("a loaded payload drew no source picker; got %q", content)
	}
	if !uiContains(content, "FROM PAYLOAD") {
		t.Errorf("the payload row is missing with a seed in the session; got %q", content)
	}
	if uiContains(content, "SCAN") {
		t.Errorf("a SCAN row was drawn on a machine with no reader; got %q", content)
	}
}
