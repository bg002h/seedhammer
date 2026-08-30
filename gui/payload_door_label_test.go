package gui

import (
	"strings"
	"testing"
)

// ═══ F-437: a choice must name what it DOES ══════════════════════════════════
//
// Measured in the S2 journey walk (round 2, F3): the Wallet Policy door reads
// `Wallet policy from where? FROM PAYLOAD / ENTER IT`, and ENTER IT lands in the
// md1 card gather waiting for NFC taps. There is no keyboard for a card, and
// with no camera and a payload in hand the choice is strictly useless.
//
// The two md1-card doors carry the same picker and the same mislabel — measured
// in the same walk, whose J2 offer drew `First card from where? FROM PAYLOAD
// ENTER IT` and then landed in that gather. So the rename covers all four
// offers whose decline arm is a card gather, and no others.
//
// The fixtures and helpers live in payload_door_walk_test.go, beside F-76's
// walk: the two follow-ups are one door, and F-437 asks to be batched with it.

func TestF437CardDoorsDoNotPromiseTyping(t *testing.T) {
	for _, tc := range []struct {
		name string
		lead string
		flow func(*Context)
		sess func(*testing.T) *syswSession
	}{
		{
			name: "wallet policy, md1 card offer",
			lead: "First card from where?",
			flow: func(ctx *Context) { walletPolicyFlow(ctx, &descriptorTheme) },
			sess: func(t *testing.T) *syswSession {
				return f76Session(t, f76Md1CardPayload, f76Md1CardSHA256, wshSortedmultiChunks)
			},
		},
		{
			name: "engrave bundle, md1 card offer",
			lead: "First card from where?",
			flow: func(ctx *Context) { bundleFlow(ctx, &descriptorTheme) },
			sess: func(t *testing.T) *syswSession {
				return f76Session(t, f76Md1CardPayload, f76Md1CardSHA256, wshSortedmultiChunks)
			},
		},
		{
			name: "engrave multisig, supplied md1 card offer",
			lead: "First card from where?",
			flow: func(ctx *Context) { supplyMultisigPolicyFlow(ctx, &descriptorTheme) },
			sess: func(t *testing.T) *syswSession {
				return f76Session(t, f76Md1CardPayload, f76Md1CardSHA256, wshSortedmultiChunks)
			},
		},
		{
			// F-437's own subject: the S2 Descriptor offer, whose decline arm
			// falls through to the md1 CARD gather.
			name: "wallet policy, descriptor offer",
			lead: "Wallet policy from where?",
			flow: func(ctx *Context) { walletPolicyFlow(ctx, &descriptorTheme) },
			sess: func(t *testing.T) *syswSession {
				s, _ := s2DescriptorSession(t)
				return s
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := NewContext(f76Platform())
			ctx.sysw = tc.sess(t)
			frame, quit := runUI(ctx, func() { tc.flow(ctx) })
			defer quit()

			got, ok := pumpUntil(frame, tc.lead, 16)
			if !ok {
				t.Fatalf("the offer never drew.\nLast frame: %q", got)
			}
			if !uiContains(got, "FROM PAYLOAD") {
				t.Fatalf("the offer does not name the payload route.\nFrame: %q", got)
			}
			if uiContains(got, "ENTER IT") {
				t.Errorf("the door still offers ENTER IT, and declining lands in an "+
					"NFC card gather -- this device has no keyboard for cards "+
					"(F-437).\nFrame: %q", got)
			}
			if !uiContains(got, "SCAN CARDS") {
				t.Errorf("the second choice does not name what it does.\nFrame: %q", got)
			}
		})
	}
}

// The classes that really DO reach a keyboard keep ENTER IT: this is a rename
// of the card doors, not of the shared picker. Without this the rename could
// have been made by changing one string in syswChoose, which would then lie in
// the other direction at four honest doors.
func TestF437KeyboardDoorsKeepEnterIt(t *testing.T) {
	ctx := NewContext(f76Platform())
	ctx.sysw = sessionWith(testSeedPhrase)
	frame, quit := runUI(ctx, func() { seedEntryFlow(ctx, &descriptorTheme) })
	defer quit()

	got, ok := pumpUntil(frame, "FROM PAYLOAD", 32)
	if !ok {
		t.Fatalf("the seed offer never drew.\nLast frame: %q", got)
	}
	if !uiContains(got, "ENTER IT") && !uiContains(got, "TYPE IT") {
		t.Errorf("the seed door no longer offers a typing route; the F-437 rename "+
			"reached a class that really does have a keyboard.\nFrame: %q", got)
	}
	if strings.Contains(strings.ToUpper(got), "SCANCARDS") {
		t.Errorf("the seed door now says SCAN CARDS.\nFrame: %q", got)
	}
}
