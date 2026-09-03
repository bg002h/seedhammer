package gui

import (
	"strings"
	"testing"

	"seedhammer.com/sysw"
)

// ═══ F-437: a choice must name what it DOES ══════════════════════════════════
//
// Measured in the S2 journey walk (round 2, F3): the Wallet Policy door reads
// `Wallet policy from where? FROM PAYLOAD / ENTER IT`, and ENTER IT lands in the
// md1 card gather waiting for NFC taps. There is no keyboard for a card, and
// with no camera and a payload in hand the choice is strictly useless.
//
// The three md1-card doors carry the same picker and the same mislabel —
// measured in the same walk, whose J2 offer drew `First card from where? FROM
// PAYLOAD ENTER IT` (the lead has since been reworded for F-76's M1) and then
// landed in that gather. So the rename covers all four offers whose decline arm
// is a card gather, and no others.
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
			lead: "Cards from where?",
			flow: func(ctx *Context) { walletPolicyFlow(ctx, &descriptorTheme) },
			sess: func(t *testing.T) *syswSession {
				return f76Session(t, f76Md1CardPayload, f76Md1CardSHA256, wshSortedmultiChunks)
			},
		},
		{
			name: "engrave bundle, md1 card offer",
			lead: "Cards from where?",
			flow: func(ctx *Context) { bundleFlow(ctx, &descriptorTheme) },
			sess: func(t *testing.T) *syswSession {
				return f76Session(t, f76Md1CardPayload, f76Md1CardSHA256, wshSortedmultiChunks)
			},
		},
		{
			name: "engrave multisig, supplied md1 card offer",
			lead: "Cards from where?",
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

			// (0) THE COMPOSER'S DOOR, which is now the first screen of the
			// wallet-policy program in every state (SPEC_wallet_policy_composer
			// §7a). "Scan cards" is index 0, so one Down selects "From payload",
			// which is the route both wallet-policy rows take.
			//
			// GUARDED, not applied to all four: the other two rows walk Engrave
			// Bundle and Engrave Multisig, whose doors are unchanged, and adding
			// the step to them would make this table lie about what it walks.
			if strings.HasPrefix(tc.name, "wallet policy") {
				if _, ok := pumpUntil(frame, "Build a new policy", 16); !ok {
					t.Fatal("the composer door never drew")
				}
				click(&ctx.Router, Down)
				click(&ctx.Router, Button3)
			}
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

// ═══ The other direction: the doors that really DO open a keyboard ══════════
//
// THIS TEST REPLACES ONE THAT COULD NOT FAIL. REVIEW-F76-F437-r1 I1 applied the
// exact mutation the old test claimed to forbid — `syswAltEnter` "ENTER IT" ->
// "SCAN CARDS" — and all 1028 gui tests stayed GREEN. The old test walked
// `seedEntryFlow`, which routes through `syswSeedPickerTitled`
// (gui/derive_xpub.go): a picker that builds its own rows and never calls
// `syswChoose`. It draws "TYPE IT", a string `syswAltEnter` does not control, so
// the assertion's `ENTER IT` disjunct was dead and the five doors that really
// use the constant were asserted by nothing. A gate that cannot fail is the
// class this tree treats as blocking, and the shipped behaviour was never in
// doubt — only the guard was.
//
// So this walks doors that DRAW the constant, and asserts the rendered string.
// Proven red under I1's mutation before being committed: with `syswAltEnter` set
// to "SCAN CARDS" both subtests fail with
// `the keyboard door no longer offers a typing route`.
//
// Why it matters in this diff: F-437 could have been "fixed" by editing one
// string inside the shared picker, which would have reintroduced the same
// falsehood in mirror image at five honest doors. `syswOfferAlt` is what keeps
// the two answers apart, and this is what holds it there.
func TestF437KeyboardDoorsKeepEnterIt(t *testing.T) {
	for _, tc := range []struct {
		name   string
		lead   string
		record string
		flow   func(*Context)
	}{
		{
			// gui.go's newInputFlow -> syswOffer(ClassMnemonic).
			name:   "backup wallet, seed door",
			lead:   "Seed from where?",
			record: testSeedPhrase,
			flow:   func(ctx *Context) { newInputFlow(ctx, &descriptorTheme) },
		},
		{
			// freetext_flow.go's engraveTextFlowFrom -> syswOffer(ClassFreeText).
			name:   "engrave text, text door",
			lead:   "Text from where?",
			record: "text:48656c6c6f",
			flow:   func(ctx *Context) { engraveTextFlowFrom(ctx, &descriptorTheme, "", srcTyped) },
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := sysw.Classify(tc.record); got == sysw.ClassUnknown {
				t.Fatalf("INCONCLUSIVE: the fixture classifies as %v, so this door "+
					"is never offered and the test measures nothing", got)
			}
			ctx := NewContext(f76Platform())
			ctx.sysw = sessionWith(tc.record)
			frame, quit := runUI(ctx, func() { tc.flow(ctx) })
			defer quit()

			got, ok := pumpUntil(frame, tc.lead, 32)
			if !ok {
				t.Fatalf("the offer never drew.\nLast frame: %q", got)
			}
			// The door draws syswAltEnter ITSELF -- not a look-alike from some
			// other picker. This is the assertion I1's mutation has to break.
			if !uiContains(got, "ENTER IT") {
				t.Errorf("the keyboard door no longer offers a typing route: this "+
					"door DOES open a keyboard when declined, and F-437's rename "+
					"was supposed to reach the card doors only.\nFrame: %q", got)
			}
			if uiContains(got, "SCAN CARDS") {
				t.Errorf("a keyboard door now says SCAN CARDS -- F-437 in mirror "+
					"image.\nFrame: %q", got)
			}
		})
	}
}
