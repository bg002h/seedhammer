package gui

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"strings"
	"testing"
	"testing/synctest"

	"seedhammer.com/sysw"
)

// ═══ THE FIRST EXECUTION OF admits(progWalletPolicy, ClassDescriptor) ════════
//
// That cell has been `true` in gui/sysw_admit.go since the table was written,
// and until this walk it had NEVER FIRED: sysw.Classify returned ClassUnknown
// for every descriptor, so no record could carry the class, so the path from
// the cell to a rendered screen was untested BY CONSTRUCTION.
// SPEC_descriptor_input.md §9 item 2 says so in those words.
//
// A gate that has never executed is a hypothesis, not a gate. So running it is
// a phase gate rather than later polish, and this is that run: bytes `me`
// wrote, opened by the firmware's own sysw.Open, classified by the real
// sysw.Classify, offered by the real syswOffer at walletPolicyFlow's real
// payload door, and rendered by the real DescriptorScreen.
//
// WHAT IT DOES NOT PROVE, said plainly:
//
//   - NO HARDWARE, and no plate. The walk stops at the screen.
//   - IT DOES NOT START AT syswLoadFlow. gui/chain_walk_test.go's four-link
//     chain reads a padded flash region through sysw.FileReader and walks the
//     load screens; this starts one link in, at an OPENED payload, because what
//     is new in S2 is the classification and the consumption and not the
//     ingest. The container is `me` output either way.
//   - THE SCREEN IS TEXT, NOT PIXELS (op.Drawer.ExtractText), and the choice is
//     driven by a synthesized Button3 rather than by a tap hit-tested against
//     drawn geometry.

// The container was produced by the S2 `me` on branch impl/descriptor-s2, from
// the seam file's own `formats-happy/bip380-sortedmulti-multipath` input:
//
//	me sysw pack --no-passphrase --as descriptor \
//	    --in <that row's input> --out gui/testdata/s2_descriptor_payload.bin
//
// It is UNSEALED because only the unsealed variant is byte-deterministic, and a
// fixture that cannot be reproduced cannot be re-pinned. `me` reported
// wallet-id 9e95257e60aacbb260129dac7b36d9f4 and digest
// 9c16 bfa9 bb3b ecd4 6c3c f20f e48c 12a9 for these bytes.
const (
	s2DescriptorPayloadPath   = "testdata/s2_descriptor_payload.bin"
	s2DescriptorPayloadSHA256 = "672d8d2c49b6c2004c38849c7b68b6dffa8629eb6bf9ac61f6ebc1e1657c58bb"
	s2DescriptorPayloadBytes  = 509
)

// s2DescriptorSession opens the committed container the way the firmware does
// and loads it into a session, asserting the fixture's own identity first.
//
// The hash check is not ceremony: this fixture is the ONLY thing binding the
// walk to what `me` actually emits, so a silently edited container would make
// every assertion below measure a payload nobody produced.
func s2DescriptorSession(t *testing.T) (*syswSession, string) {
	t.Helper()
	blob, err := os.ReadFile(s2DescriptorPayloadPath)
	if err != nil {
		// FATAL, never a skip: the file is in the repo, so its absence means a
		// broken checkout, and a test that answers "I could not tell" by
		// reporting success is the default failure mode in this tree.
		t.Fatalf("INCONCLUSIVE: %s is unreadable: %v", s2DescriptorPayloadPath, err)
	}
	if len(blob) != s2DescriptorPayloadBytes {
		t.Fatalf("%s is %d bytes, the fixture pins %d", s2DescriptorPayloadPath, len(blob), s2DescriptorPayloadBytes)
	}
	sum := sha256.Sum256(blob)
	if got := hex.EncodeToString(sum[:]); got != s2DescriptorPayloadSHA256 {
		t.Fatalf("%s hashes to %s, the fixture pins %s -- regenerate it with the "+
			"invocation in this file's header and re-pin, or the walk is measuring "+
			"a container `me` never wrote", s2DescriptorPayloadPath, got, s2DescriptorPayloadSHA256)
	}
	pay, err := sysw.Open(blob, "")
	if err != nil {
		t.Fatalf("the firmware's own sysw.Open cannot read what `me` wrote: %v", err)
	}
	if len(pay.Public) != 1 || len(pay.Secret) != 0 {
		t.Fatalf("`--as descriptor` packs ONE public record; got %d public / %d secret",
			len(pay.Public), len(pay.Secret))
	}
	s := &syswSession{}
	// sealed=false (this variant carries no ciphertext), cliffAbove=true and
	// compared=true: `take` refuses while compared is false, and the digest
	// comparison is syswLoadFlow's business, which this walk starts after.
	s.load(pay, sysw.Identity(blob), false, true, true, true)
	return s, pay.Public[0]
}

// TestS2ContainerRecordClassifiesAsADescriptor is the link the walk stands on.
// If the record `me` packs does not classify, walletPolicyFlow's new offer is
// never reached and the walk below would fail for the wrong reason.
func TestS2ContainerRecordClassifiesAsADescriptor(t *testing.T) {
	_, record := s2DescriptorSession(t)
	if got := sysw.Classify(record); got != sysw.ClassDescriptor {
		t.Fatalf("Classify(the record `me sysw pack --as descriptor` wrote) = %v, "+
			"want ClassDescriptor", got)
	}
	// §5.2 packs the CANONICAL re-encoding, not the operator's bytes, and the
	// canonical is what the device's own parser round-trips. Asserting the
	// checksum is present is the cheap half of that; the seam file asserts the
	// fixed point.
	if !strings.Contains(record, "#") {
		t.Errorf("the packed record carries no BIP-380 checksum: %q", record)
	}
	if !admits(progWalletPolicy, sysw.ClassDescriptor) {
		t.Fatal("progWalletPolicy no longer admits ClassDescriptor -- the walk below " +
			"would then be walking a path the admission table forbids")
	}
}

// TestWalkWalletPolicyFromAPackedDescriptorRecordToTheDescriptorScreen drives
// the whole thing: a container `me` wrote -> the payload door's SECOND offer ->
// DescriptorScreen.Draw.
//
// It reaches Draw WITHOUT PANICKING, and that is the assertion S2's admission
// port exists for. DescriptorScreen encodes on the way to a plate, and
// Descriptor.encode's default arm panics on the zero Script -- the shape a
// titled zero-key BlueWallet file produces (§4.2 defect 1). A classified record
// is §4.7-admitted, so it can never be that shape; a scan-door-keyed classifier
// would have made it reachable from one line of text.
func TestWalkWalletPolicyFromAPackedDescriptorRecordToTheDescriptorScreen(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		session, _ := s2DescriptorSession(t)
		e := newEngraver()
		p := newEngravedAwarePlatform()
		p.engraver = e
		p.display = sh2DisplaySize
		ctx := NewContext(p)
		ctx.sysw = session

		frame, quit := runUI(ctx, func() { walletPolicyFlow(ctx, &descriptorTheme) })
		defer quit()

		// (1) THE DOOR. The payload holds no md1 card, so the ClassMDMK offer
		// never draws and the ClassDescriptor offer is the first screen. Its
		// lead names the wallet policy rather than a card, which is the whole
		// point of the second offer.
		got, ok := pumpUntil(frame, "Wallet policy from where?", 16)
		if !ok {
			t.Fatalf("the Descriptor offer never drew.\nLast frame: %q", got)
		}
		if !uiContains(got, "FROM PAYLOAD") {
			t.Errorf("the offer does not name the payload route.\nFrame: %q", got)
		}

		// (2) FROM PAYLOAD. ChoiceScreen opens on index 0 because the payload
		// holds the class; Button3 confirms.
		click(&ctx.Router, Button3)

		// (3) THE SCREEN. This is the first time in the tree that
		// admits(progWalletPolicy, ClassDescriptor) has led anywhere.
		got, ok = pumpUntil(frame, "Engrave Descriptor", 64)
		if !ok {
			t.Fatalf("the walk never reached DescriptorScreen.\nLast frame: %q", got)
		}
		// The wallet the fixture actually holds, so the screen is shown to be
		// describing THIS descriptor and not merely drawing.
		for _, want := range []string{"2-of-3 multisig", "Segwit (P2WSH)"} {
			if !uiContains(got, want) {
				t.Errorf("DescriptorScreen does not say %q.\nFrame: %q", want, got)
			}
		}
		// Not a testnet wallet, and the screen says so by NOT saying so -- the
		// suffix is appended from the first key's network.
		if uiContains(got, "(testnet)") {
			t.Errorf("mainnet keys rendered as testnet.\nFrame: %q", got)
		}
	})
}

// REVIEW-S2-P3-r1 I1, as a walk rather than as a comment.
//
// The consumer used to argue that re-parsing "cannot fail here … over these
// exact bytes". It is not the same bytes: classification parses
// `strings.TrimSpace(record)` and `syswSession.take` returns `r.body`
// unmodified. The shipped corpus row `whitespace/leading-space-bip380` is the
// standing counterexample -- `host_admits: true`, single-line, so
// `TestDescriptorSeamSyswClass` REQUIRES it to classify, and its raw bytes do
// not re-parse.
//
// So this walk feeds the fixture's own record with a leading space and requires
// the DescriptorScreen to RENDER. Before the fold it reached
// "Couldn't read the wallet policy from the payload." instead -- a wallet the
// operator can see on the host, refused at the machine.
func TestWalkWalletPolicyRendersARecordWithLeadingWhitespace(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		_, record := s2DescriptorSession(t)
		padded := " " + record
		// The premise, asserted rather than assumed: the padded record still
		// classifies, which is what puts it in front of the consumer at all.
		if got := sysw.Classify(padded); got != sysw.ClassDescriptor {
			t.Fatalf("Classify(leading-space record) = %v, want ClassDescriptor -- "+
				"§4.6 trims ASCII whitespace on the host too", got)
		}
		e := newEngraver()
		p := newEngravedAwarePlatform()
		p.engraver = e
		p.display = sh2DisplaySize
		ctx := NewContext(p)
		ctx.sysw = sessionWith(padded)

		frame, quit := runUI(ctx, func() { walletPolicyFlow(ctx, &descriptorTheme) })
		defer quit()

		got, ok := pumpUntil(frame, "Wallet policy from where?", 16)
		if !ok {
			t.Fatalf("the Descriptor offer never drew.\nLast frame: %q", got)
		}
		click(&ctx.Router, Button3)
		got, ok = pumpUntil(frame, "Engrave Descriptor", 64)
		if !ok {
			t.Fatalf("a record the classifier ADMITTED did not reach the screen.\n"+
				"Last frame: %q", got)
		}
		if uiContains(got, "Couldn't read the wallet policy") {
			t.Errorf("the consumer refused a record classification admitted.\nFrame: %q", got)
		}
	})
}
