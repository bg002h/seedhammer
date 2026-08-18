package gui

import (
	"testing"

	"github.com/btcsuite/btcd/chaincfg/v2"
	"seedhammer.com/bip39"
)

// TestSingleSigVerifyFailedCopyConditionsOnPassphrase is GATE 3.2 (F-204, S6b
// spec §3.2).
//
// gui/singlesig_verify.go used to say "Check the engraved plates." on EVERY
// FAILED verify. The multisig sibling (multisigVerifyNoSlotBody,
// gui/multisig_verify.go) rules the other way: a mistyped BIP-39 passphrase
// derives an entirely different wallet, so a FAILED comparison is at least as
// often a keying mistake as a bad plate -- and blaming the plates unconditionally
// sends an operator to distrust GOOD steel over one keystroke.
//
// BOTH ARMS ARE DRIVEN THROUGH THE REAL singleSigVerifyFlow, off the SAME
// engraved plates (a watch-only, bare-seed bundle for abandonAboutPhrase()), so
// the only variable between the two subtests is what the operator typed at the
// re-verify passphrase prompt. `passphrase` is in scope at
// gui/singlesig_verify.go:108-112; the failure site is what used to be :182.
func TestSingleSigVerifyFailedCopyConditionsOnPassphrase(t *testing.T) {
	// The plates on the bench: a WATCH-ONLY bare-seed engrave. watchOnly skips
	// the ms1 hand-type step, which this gate does not need.
	opts := s6aSingleSigOpts{watchOnly: true}
	b := s6aSingleSigBundle(t, opts)

	t.Run("passphrase entered", func(t *testing.T) {
		var rec verifyRecord
		p := newPlatform()
		p.display = sh2DisplaySize
		ctx := NewContext(p)
		ctx.syswBundleSeeds = append(append([]string(nil), b.MK1...), b.MD1...)
		frame, quit := runUI(ctx, func() {
			singleSigVerifyFlow(ctx, &descriptorTheme, false /* watch-only */, false, &rec)
		})
		defer quit()

		if c, ok := pumpUntil(frame, "Choose number of words", 96); !ok {
			t.Fatalf("the verify did not ask for the seed; got %q", c)
		}
		click(&ctx.Router, Button3) // 12 WORDS
		frame()
		driveWords(&ctx.Router, abandonAboutPhrase()) // the SAME seed as engraved
		if c, ok := pumpUntil(frame, "Wallet Type", 240); !ok {
			t.Fatalf("the verify did not reach the wallet-type picker; got %q", c)
		}
		click(&ctx.Router, Button3) // BIP-84 default -- matches the engrave
		if c, ok := pumpUntil(frame, "Add a BIP-39 passphrase?", 96); !ok {
			t.Fatalf("the verify did not reach the passphrase prompt; got %q", c)
		}
		click(&ctx.Router, Down) // "Add passphrase"
		frame()
		click(&ctx.Router, Button3)
		frame()
		if c, ok := pumpUntil(frame, "Enter Passphrase", 96); !ok {
			t.Fatalf("choosing Add passphrase did not reach the keyboard; got %q", c)
		}
		// The engrave used NO passphrase, so typing ANY non-empty one here mints a
		// DIFFERENT wallet and the comparator disagrees against the bench plates.
		runes(&ctx.Router, "TREZOR")
		click(&ctx.Router, Button3) // OK
		frame()
		if c, ok := pumpUntil(frame, "mk1 keys:", 96); !ok {
			t.Fatalf("the readback never reached the gatherer's tally; got %q", c)
		}
		click(&ctx.Router, Button3) // Done adding cards
		frame()

		body, ok := pumpUntil(frame, "Verify Failed", 96)
		if !ok {
			t.Fatalf("a passphrase-mismatched re-derivation did not FAIL the verify; got %q", body)
		}
		if !uiContains(body, "Check the passphrase") {
			t.Errorf("a FAILED verify with a passphrase entered must suspect the "+
				"passphrase before the plates (S6b spec §3.2); got %q", body)
		}
		if uiContains(body, "Check the engraved plates") {
			t.Errorf("the plates-first wording is a false lead once a passphrase was "+
				"entered -- the plates may be perfect and the passphrase wrong; got %q", body)
		}
		if !rec.adverse {
			t.Error("the comparator ran and disagreed but the record says nothing adverse")
		}
	})

	t.Run("no passphrase", func(t *testing.T) {
		var rec verifyRecord
		p := newPlatform()
		p.display = sh2DisplaySize
		ctx := NewContext(p)
		ctx.syswBundleSeeds = append(append([]string(nil), b.MK1...), b.MD1...)
		frame, quit := runUI(ctx, func() {
			singleSigVerifyFlow(ctx, &descriptorTheme, false /* watch-only */, false, &rec)
		})
		defer quit()

		if c, ok := pumpUntil(frame, "Choose number of words", 96); !ok {
			t.Fatalf("the verify did not ask for the seed; got %q", c)
		}
		click(&ctx.Router, Button3) // 12 WORDS
		frame()
		// A DIFFERENT seed than what was engraved -- the mismatch this time comes
		// from the seed, not the passphrase, and NO passphrase is entered either.
		driveWords(&ctx.Router, fixtureMasterB)
		if c, ok := pumpUntil(frame, "Wallet Type", 240); !ok {
			t.Fatalf("the verify did not reach the wallet-type picker; got %q", c)
		}
		click(&ctx.Router, Button3) // BIP-84 default
		if c, ok := pumpUntil(frame, "Add a BIP-39 passphrase?", 96); !ok {
			t.Fatalf("the verify did not reach the passphrase prompt; got %q", c)
		}
		click(&ctx.Router, Button3) // Skip -- NO passphrase entered
		frame()
		if c, ok := pumpUntil(frame, "mk1 keys:", 96); !ok {
			t.Fatalf("the readback never reached the gatherer's tally; got %q", c)
		}
		click(&ctx.Router, Button3) // Done adding cards
		frame()

		body, ok := pumpUntil(frame, "Verify Failed", 96)
		if !ok {
			t.Fatalf("a different re-typed seed did not FAIL the verify; got %q", body)
		}
		if uiContains(body, "Check the passphrase") {
			t.Errorf("no passphrase was entered at verify, so blaming the passphrase "+
				"is a FALSE LEAD (S6b spec §3.2); got %q", body)
		}
		if !uiContains(body, "Check the engraved plates") {
			t.Errorf("the no-passphrase arm must say something true of that case; got %q", body)
		}
		if !rec.adverse {
			t.Error("the comparator ran and disagreed but the record says nothing adverse")
		}
	})
}

// fixtureMasterBFillsADifferentWallet is a premise check: TestSingleSigVerifyFailedCopyConditionsOnPassphrase's
// "no passphrase" arm needs fixtureMasterB to derive a DIFFERENT single-sig
// bundle than abandonAboutPhrase() at the SAME default path, or the comparator
// would pass instead of failing and the subtest would prove nothing.
func TestFixtureMasterBFillsADifferentSingleSigWallet(t *testing.T) {
	ma, err := bip39.ParseMnemonic(abandonAboutPhrase())
	if err != nil {
		t.Fatalf("ParseMnemonic(abandonAboutPhrase): %v", err)
	}
	mb, err := bip39.ParseMnemonic(fixtureMasterB)
	if err != nil {
		t.Fatalf("ParseMnemonic(fixtureMasterB): %v", err)
	}
	ba, _, _, _, err := deriveSingleSigBundle(ma, "", &chaincfg.MainNetParams,
		singleSigPath(singleSigDefault.purpose), singleSigDefault.script)
	if err != nil {
		t.Fatalf("deriveSingleSigBundle(A): %v", err)
	}
	bb, _, _, _, err := deriveSingleSigBundle(mb, "", &chaincfg.MainNetParams,
		singleSigPath(singleSigDefault.purpose), singleSigDefault.script)
	if err != nil {
		t.Fatalf("deriveSingleSigBundle(B): %v", err)
	}
	if err := verifySingleSig(ba, "", bb.MK1, bb.MD1); err == nil {
		t.Fatal("fixtureMasterB derives the SAME wallet as abandonAboutPhrase() at the " +
			"default path, so the \"no passphrase\" subtest's mismatch does not come from " +
			"the seed it claims to")
	}
}
