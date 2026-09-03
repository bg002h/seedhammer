package gui

import (
	"strings"
	"testing"
	"testing/synctest"

	"seedhammer.com/sysw"
)

// composerSessionWith builds a loaded, compared session from raw records.
// compared=true because the door's own key-state lines read through `has`,
// which has no compared gate, while everything that CONSUMES a record
// inherits `take`'s (gui/sysw_session.go:118-124).
func composerSessionWith(public, secret []string) *syswSession {
	s := &syswSession{}
	s.load(&sysw.Payload{Public: public, Secret: secret}, [32]byte{}, false, true, true, true)
	return s
}

func TestComposerDoorLinesCoverEveryKeyState(t *testing.T) {
	for _, tc := range []struct {
		name     string
		session  *syswSession
		inFlash  bool
		want     string
		unwanted string
	}{
		{"keys only", composerSessionWith([]string{composerTestKeyRecord, composerTestKeyRecord2}, nil), false,
			"Keys loaded: 2", "plus"},
		{"keys and one seed", composerSessionWith([]string{composerTestKeyRecord}, []string{composerTestMnemonicRecord}), false,
			"Keys loaded: 1, plus 1 seed.", ""},
		{"seed only", composerSessionWith(nil, []string{composerTestMnemonicRecord}), false,
			"A seed is loaded. It can fill any number of slots.", "Keys loaded"},
		{"nothing", composerSessionWith(nil, nil), false,
			"No keys loaded. This builds a key-less template.", "Keys loaded:"},
		{"nothing loaded but flash holds one", nil, true,
			"A payload is in flash but not loaded.", "Keys loaded:"},
		{"inert records", composerSessionWith([]string{composerTestKeyRecord, "hash:zz"}, nil), false,
			"1 payload record was not understood.", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := strings.Join(composerDoorLines(tc.session, tc.inFlash), " | ")
			if !strings.Contains(got, tc.want) {
				t.Errorf("the door does not say %q.\nLines: %s", tc.want, got)
			}
			if tc.unwanted != "" && strings.Contains(got, tc.unwanted) {
				t.Errorf("the door says %q, which this state must not print.\nLines: %s", tc.unwanted, got)
			}
		})
	}
}

// TestComposerDoorCountsIgnoreClassesThatAreNotKeys guards the count itself:
// a card, a descriptor and a now: record are none of them "keys loaded", and
// a malformed key: reduces the count while raising the inert one (§6a).
func TestComposerDoorCountsIgnoreClassesThatAreNotKeys(t *testing.T) {
	s := composerSessionWith([]string{
		composerTestKeyRecord,        // ClassKey
		composerTestNowRecord,        // ClassNow
		"key:zz",                     // malformed -> ClassUnknown
		composerTestDescriptorRecord, // ClassDescriptor
	}, nil)
	keys, seeds, inert := composerDoorCounts(s)
	if keys != 1 {
		t.Errorf("keys = %d, want 1: only the well-formed key: record is a key", keys)
	}
	if seeds != 0 {
		t.Errorf("seeds = %d, want 0", seeds)
	}
	if inert != 1 {
		t.Errorf("inert = %d, want 1: the malformed key: record goes inert and is counted "+
			"once, in the not-understood line (§6a)", inert)
	}
}

// TestComposerDoorOffersFromPayloadOnlyWhenThePayloadHasOne is §7a's
// conditional choice: "From payload" appears only when the loaded payload
// holds a Descriptor or an md1/mk1 record.
func TestComposerDoorOffersFromPayloadOnlyWhenThePayloadHasOne(t *testing.T) {
	for _, tc := range []struct {
		name    string
		session *syswSession
		want    bool
	}{
		{"key records only", composerSessionWith([]string{composerTestKeyRecord}, nil), false},
		{"a descriptor", composerSessionWith([]string{composerTestDescriptorRecord}, nil), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				p := newPlatform()
				p.display = sh2DisplaySize
				ctx := NewContext(p)
				ctx.sysw = tc.session
				frame, quit := runUI(ctx, func() { composerDoorFlow(ctx, &descriptorTheme) })
				defer quit()
				content, ok := pumpUntil(frame, "Build a new policy", 16)
				if !ok {
					t.Fatalf("the door never drew.\nLast frame: %q", content)
				}
				if got := uiContains(content, "From payload"); got != tc.want {
					t.Errorf("From payload offered = %v, want %v.\nFrame: %q", got, tc.want, content)
				}
				if !uiContains(content, "Scan cards") {
					t.Errorf("the door does not offer the NFC route.\nFrame: %q", content)
				}
			})
		})
	}
}

// TestComposerDoorDrawsItsKeyStateAndFitsItsLabels is §12 item 5 for this
// screen: it draws (raster floor) and no choice label is cut off the panel.
func TestComposerDoorDrawsItsKeyStateAndFitsItsLabels(t *testing.T) {
	for _, l := range []string{"Scan cards", "From payload", "Build a new policy"} {
		assertChoiceLabelFits(t, l)
	}
	synctest.Test(t, func(t *testing.T) {
		p := newPlatform()
		p.display = sh2DisplaySize
		ctx := NewContext(p)
		ctx.sysw = composerSessionWith([]string{composerTestKeyRecord}, []string{composerTestMnemonicRecord})
		frame, _, ink, quit := runUITouchRaster(ctx, func() { composerDoorFlow(ctx, &descriptorTheme) })
		defer quit()
		content, ok := pumpUntil(frame, "Build a new policy", 16)
		if !ok {
			t.Fatalf("the door never drew.\nLast frame: %q", content)
		}
		assertFrameHasBody(t, ink(), "the composer door")
		if !uiContains(content, "Keys loaded: 1, plus 1 seed.") {
			t.Errorf("the door does not draw its key state.\nFrame: %q", content)
		}
	})
}
