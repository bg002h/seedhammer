package gui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"seedhammer.com/bip39"
	"seedhammer.com/codex32"
	"seedhammer.com/sysw"
)

// ═══ ONE CHAIN PER PACKABLE RECORD CLASS ═════════════════════════════════════
//
// gui/chain_walk_test.go joins the four links for ClassTx and ClassMt and owns
// the harness. This file is the same four links for every other class a
// systemwide payload can hold:
//
//	ClassMnemonic        chain-seed      Backup Wallet -> a seed plate
//	ClassCodex32Secret   chain-codex32   Backup Wallet -> an ms1 plate
//	ClassFreeText        chain-text      Engrave Text  -> a free-text plate
//	ClassPassphrase      chain-pass      BIP-39 Password -> a password plate
//	ClassMDMK            chain-mdmk      Build Multisig Policy -> nine plates
//
// THAT IS EVERY CLASS WITH A CHAIN, and since S2 it is no longer every
// PACKABLE class. `me sysw pack` refuses an address and takes a descriptor
// only under an explicit `--as`, re-measured against the S2 tree:
//
//	me sysw pack --in <an address>                  -> rc=4
//	"record 0 ... is not a form this container can place: ... Addresses are not
//	 classifiable here, and neither is a wallet descriptor `me` refuses -- see
//	 sysw::classify"
//	me sysw pack --in <a descriptor>                -> rc=2  (§5.1's window:
//	                                                   `--as` is REQUIRED, and
//	                                                   there is no default)
//	me sysw pack --in <a descriptor> --as descriptor -> rc=0, ONE Descriptor
//	                                                   record
//
// So ClassAddress still cannot enter a payload, and a class that cannot enter
// one is not an element of one -- it has no chain and cannot have one until the
// Rust primary grows an address decoder (crates/me-cli/src/sysw/mod.rs states
// that this is a known limitation rather than an oversight). ClassDescriptor
// now can enter one, and gui/wallet_policy_descriptor_walk_test.go walks it
// from `me`-written container bytes to a rendered DescriptorScreen; what it
// does NOT do is use this file's harness, so that class has no FOUR-link chain
// and is deliberately absent from the table above. ClassUnknown is not a kind
// at all -- it is the fail-closed arm.
//
// EVERY LIMIT IN chain_walk_test.go's HEADER APPLIES HERE UNCHANGED: no
// hardware, screens are text and not pixels, ExtractText concatenates ops so a
// digest assertion sees 32 nibbles and cannot see the grouping, the plate is a
// build of the same inputs and not a capture from inside the engrave loop, the
// fixtures are pinned rather than live, and the walks press buttons where a
// finger would do.
//
// ONE LIMIT IS THIS FILE'S OWN. The plate each chain compares is built by
// calling the SAME builder the flow calls, on the value the flow received --
// captured out of the flow, not re-derived. So it is bound to the walk by the
// value and by the screens the walk asserted on the way past, and by nothing
// stronger. The free-text and password chains are the exception and say so:
// they capture the finished Plate itself, through production hooks the flow
// already carries.

// ─── ClassMnemonic ──────────────────────────────────────────────────────────

// TestChainMnemonicFromAMePackedPayloadToASeedPlate is the full chain for a
// BIP-39 mnemonic.
//
// IT IS THE FIRST CHAIN THAT WALKS THE F1 SCREENS FROM A REAL PAYLOAD. A
// container holding a seed in cleartext makes syswLoadFlow draw a warning
// summary and then offer KEEP/UNLOAD, and the transaction chains never reach
// either because Mt and Tx are not secret classes. `me sysw pack` says the same
// thing at the other end of the wire -- "NOT SEALED ... this payload HOLDS
// SECRET MATERIAL (record 0 (BIP-39 mnemonic))" -- so the two ends of the chain
// warn about one fact, and ingest() asserts the device's half.
func TestChainMnemonicFromAMePackedPayloadToASeedPlate(t *testing.T) {
	var words int
	var art string
	synctest.Test(t, func(t *testing.T) {
		var got any
		w := newChainWalkFlow(t, "chain-seed", func(ctx *Context, th *Colors) {
			// newInputFlow THEN engraveObjectFlow, which is what the Backup
			// Wallet program does (gui.go's carousel entry). Calling
			// backupWalletFlow directly would skip the payload offer entirely --
			// the seam under test.
			obj, ok := newInputFlow(ctx, th)
			if !ok {
				return
			}
			got = obj
			engraveObjectFlow(ctx, th, obj)
		})
		defer w.quit()
		w.assertF1(true)
		w.eng.keepWordsIn(t)

		w.ingest()

		// (3) THE WALK.
		screen := w.until("Seed from where?")
		if !uiContains(screen, "FROM PAYLOAD") {
			t.Fatalf("the payload's seed was not offered: %q", screen)
		}
		w.confirm() // FROM PAYLOAD is choice 0 (operator ruling 2026-08-12)

		// The seed screen shows the words the CONTAINER carried. Asserted on
		// the first and last, because a route that offered the payload and then
		// fell through to an empty keyboard draws a screen with the same title.
		screen = w.until("Engrave Seed")
		for _, want := range []string{"1: ABANDON", "12: ABOUT"} {
			if !uiContains(screen, want) {
				t.Fatalf("the seed screen does not carry the payload's words (%q): %q", want, screen)
			}
		}
		w.confirm()

		w.until("Add a BIP-39 passphrase?")
		w.confirm() // Skip is choice 0, and the golden is the bare fingerprint

		w.until("Hold button to start")
		w.engraveOnePlate()

		var digest uint64
		words, digest = w.eng.engraved()
		if words == 0 {
			t.Fatal("the chain completed having written ZERO stepper words: " +
				"nothing was cut, and every screen assertion above would still pass")
		}
		t.Logf("chain-seed cut %d stepper words, digest %#x", words, digest)

		// (4) THE PLATE. Built from the mnemonic the PAYLOAD produced -- not
		// from the constant beside the golden -- through the same two calls
		// backupWalletFlow makes after Skip.
		m, ok := got.(bip39.Mnemonic)
		if !ok {
			t.Fatalf("the payload offer returned %T, not the bip39.Mnemonic "+
				"engraveObjectFlow routes to backupWalletFlow", got)
		}
		if s := chainMnemonicWords(m); s != chainSeedWords {
			t.Fatalf("the payload delivered %q, the golden pins %q", s, chainSeedWords)
		}
		art = chainCompareGolden(t, w, "chain-seed", chainSeedPlate(t, chainMnemonicWords(m)),
			"seed plate, 12 words, bare fingerprint", "TestChainPlateGoldens")
	})
	t.Logf("chain-seed plate artifact %s", art)
}

// chainMnemonicWords renders a mnemonic back to the space-separated lowercase
// form `me sysw pack` was given, so the two ends can be compared as strings.
func chainMnemonicWords(m bip39.Mnemonic) string {
	out := ""
	for i, wrd := range m {
		if i > 0 {
			out += " "
		}
		out += toLowerASCII(bip39.LabelFor(wrd))
	}
	return out
}

func toLowerASCII(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + ('a' - 'A')
		}
	}
	return string(b)
}

// ─── ClassCodex32Secret ─────────────────────────────────────────────────────

// TestChainCodex32FromAMePackedPayloadToAnMs1Plate is the full chain for a
// codex32 secret.
//
// THE SECOND OFFER IS THE SUBJECT. newInputFlow asks for a mnemonic first and
// this payload has none, so that offer never draws; ClassCodex32Secret is a
// SEPARATE syswOffer (gui.go:2763) because syswOffer takes one class. A chain
// that packed both would never reach it.
//
// THE FIXTURE'S LENGTH IS NOT INCIDENTAL. `me`'s ms_codec v0.1 accepts codex32
// string lengths [50, 56, 62, 69, 75] only, and the fork's own committed ms1
// fixtures are 48 characters (backup/backup_test.go's `ms13cash…`) and 74
// (gui/sysw_cells_test.go's cellMs1). Go's codex32.New accepts all three;
// `me sysw pack` REFUSES both of the fork's at rc=4 with the generic
// "not a form this container can place". So this chain could not be written
// with the string the rest of the package uses, and that divergence is a
// finding rather than a fixture detail.
func TestChainCodex32FromAMePackedPayloadToAnMs1Plate(t *testing.T) {
	var words int
	var art string
	synctest.Test(t, func(t *testing.T) {
		var got any
		w := newChainWalkFlow(t, "chain-codex32", func(ctx *Context, th *Colors) {
			obj, ok := newInputFlow(ctx, th)
			if !ok {
				return
			}
			got = obj
			engraveObjectFlow(ctx, th, obj)
		})
		defer w.quit()
		w.assertF1(true) // an ms1 secret is secret material too
		w.eng.keepWordsIn(t)

		w.ingest()

		screen := w.until("Seed from where?")
		if !uiContains(screen, "FROM PAYLOAD") {
			t.Fatalf("the payload's ms1 was not offered: %q", screen)
		}
		w.confirm()

		// engraveCodex32's confirm screen, which for an UNSHARED secret says so
		// and offers no Recover.
		screen = w.until("Confirm Codex32 Secret")
		if !uiContains(screen, "id ENTR") {
			t.Errorf("the confirm screen must name the share identifier: %q", screen)
		}
		if !uiContains(screen, "Unshared secret") {
			t.Errorf("the fixture is an unshared secret and the screen must say so: %q", screen)
		}
		w.confirm() // Button3 is Engrave

		w.until("Hold button to start")
		w.engraveOnePlate()

		var digest uint64
		words, digest = w.eng.engraved()
		if words == 0 {
			t.Fatal("the codex32 chain completed having cut nothing")
		}
		t.Logf("chain-codex32 cut %d stepper words, digest %#x", words, digest)

		s, ok := got.(codex32.String)
		if !ok {
			t.Fatalf("the payload offer returned %T, not the codex32.String "+
				"engraveObjectFlow routes to engraveCodex32", got)
		}
		if s.String() != chainCodex32 {
			t.Fatalf("the payload delivered %q, the golden pins %q", s.String(), chainCodex32)
		}
		art = chainCompareGolden(t, w, "chain-codex32", chainCodex32Plate(t, s.String()),
			"ms1 plate, unshared secret, no fingerprint", "TestChainPlateGoldens")
	})
	t.Logf("chain-codex32 plate artifact %s", art)
}

// ─── the fixtures' records ARE the goldens' constants ───────────────────────

// TestChainFixtureRecordsMatchTheGoldenConstants closes the gap between the two
// halves of every equality above.
//
// Each chain compares a plate reached from a CLI-built container against a
// golden recorded from a constant in gui/chain_plate_goldens_test.go. That is
// only a meaningful comparison if the container HOLDS what the constant says --
// and nothing else checks it, because the fixture is generated by a shell
// script and the constant is typed in Go. Change one without the other and the
// walks would fail with a moved spline and no hint why; this fails with the
// two strings side by side.
func TestChainFixtureRecordsMatchTheGoldenConstants(t *testing.T) {
	for _, tc := range []struct {
		fixture string
		class   sysw.Class
		want    string
	}{
		{"chain-seed", sysw.ClassMnemonic, chainSeedWords},
		{"chain-codex32", sysw.ClassCodex32Secret, chainCodex32},
		{"chain-text", sysw.ClassFreeText, "text:" + chainHex(chainFreeText)},
		{"chain-pass", sysw.ClassPassphrase, "pass:" + chainHex(chainPassphrase)},
	} {
		t.Run(tc.fixture, func(t *testing.T) {
			p := chainPayloadNamed(t, tc.fixture)
			if len(p.Records) != 1 {
				t.Fatalf("%s holds %d records; these single-class chains want one",
					tc.fixture, len(p.Records))
			}
			if p.Records[0] != tc.want {
				t.Fatalf("the fixture's record and the golden's constant have drifted:\n"+
					"  chain_payloads.json: %q\n"+
					"  the Go constant:     %q\n"+
					"Re-run ./scripts/gen-chain-fixtures.sh, or fix the constant.",
					p.Records[0], tc.want)
			}
			// And the DEVICE must place it in the class the chain is named for.
			// The record could match the constant and still be classified
			// somewhere else, which is exactly the divergence these chains exist
			// to find.
			if got := sysw.Classify(p.Records[0]); got != tc.class {
				t.Fatalf("the device classifies %s's record as %v, not %v",
					tc.fixture, got, tc.class)
			}
		})
	}
}

func chainHex(s string) string {
	const digits = "0123456789abcdef"
	out := make([]byte, 0, 2*len(s))
	for _, c := range []byte(s) {
		out = append(out, digits[c>>4], digits[c&0xf])
	}
	return string(out)
}

// ─── ClassFreeText ──────────────────────────────────────────────────────────

// TestChainFreeTextFromAMePackedPayloadToATextPlate is the full chain for a
// `text:` record.
//
// THIS CHAIN'S LINK (4) IS STRONGER THAN THE OTHERS'. freetextEngraveHook is a
// production seam that receives the finished Plate at the moment the flow hands
// it to the engraver, so what is compared against the golden is the plate the
// walk ENGRAVED and not a rebuild of it. Where the seed and ms1 chains say
// "built from the value the flow received", this one says "captured from the
// flow".
//
// IT IS ALSO THE CHAIN DRIVEN MOST BY TOUCH. Every picker below is chosen by a
// tap hit-tested against the drawn frame (chooseWidget/tapWidget), which is
// what gui/freetext_flow_test.go's header requires and what a Button-driven
// step does not prove.
func TestChainFreeTextFromAMePackedPayloadToATextPlate(t *testing.T) {
	var cut Plate
	var captured bool
	freetextEngraveHook = func(p Plate) { cut, captured = p, true }
	t.Cleanup(func() { freetextEngraveHook = nil })

	var art string
	synctest.Test(t, func(t *testing.T) {
		w := newChainWalkFlow(t, "chain-text", engraveTextFlow)
		defer w.quit()
		// A `text:` record is NOT secret material, so this payload raises no F1
		// and the load flow shows no warning summary. Asserted rather than
		// assumed: it is the difference between this ingest and the seed one.
		w.assertF1(false)
		w.eng.keepWordsIn(t)

		w.ingest()

		screen := w.until("Text from where?")
		if !uiContains(screen, "FROM PAYLOAD") {
			t.Fatalf("the payload's text was not offered: %q", screen)
		}
		w.confirm() // FROM PAYLOAD

		// F3, at the screen where the record ENTERS the program (§3.2).
		screen = w.until("Engrave Text")
		if !uiContains(screen, "Source") {
			t.Errorf("the acceptance screen must name where the record came from: %q", screen)
		}
		w.confirm()

		w.until("QR Code")
		w.chooseWidget("qr", 0) // no QR; the golden is the plain plate
		w.until("Font")
		w.chooseWidget("face", 0) // font/sh
		w.until("Size")
		w.chooseWidget("size", 0) // auto-fit

		// THE FIELD MUST CARRY THE PAYLOAD'S TEXT. A route that showed the
		// offer and then started the program empty draws the same screens, and
		// this is the only assertion that can tell them apart.
		w.until("lines")
		kbd, ok := w.widget("kbd").(*PassphraseKeyboard)
		if !ok {
			t.Fatalf("widget \"kbd\" is %T, not a *PassphraseKeyboard", w.widget("kbd"))
		}
		if kbd.Fragment != chainFreeText {
			t.Fatalf("the text field holds %q; the payload carried %q -- the record "+
				"never reached the program", kbd.Fragment, chainFreeText)
		}
		w.tapWidget("ok")
		w.until("Title")
		w.tapWidget("ok") // left blank, as the golden is
		w.until("Footer")
		w.tapWidget("ok")
		w.until("Confirm")
		w.tapWidget("ok")

		w.until("Hold button to start")
		w.engraveOnePlate()

		words, digest := w.eng.engraved()
		if words == 0 {
			t.Fatal("the free-text chain completed having cut nothing")
		}
		t.Logf("chain-text cut %d stepper words, digest %#x", words, digest)
		if !captured {
			t.Fatal("freetextEngraveHook never fired, so nothing was handed to the " +
				"engraver through the path this chain claims to have walked")
		}
		art = chainCompareGolden(t, w, "chain-text", cut,
			"free-text plate, no QR, auto-fit, no title or footer", "TestChainPlateGoldens")
	})
	t.Logf("chain-text plate artifact %s", art)
}

// ─── ClassPassphrase ────────────────────────────────────────────────────────

// TestChainPassphraseFromAMePackedPayloadToAPasswordPlate is the full chain for
// a `pass:` record, engraved as the BIP-39 Password plate -- the artifact, not
// the key-derivation input. progPassword is the one program that admits
// ClassPassphrase and cuts it.
//
// LINK (4) IS CAPTURED, NOT REBUILT FROM A GUESS: passphrasePlateHook receives
// the exact arguments ppBuildPlate was called with, so the plate compared here
// is built from the very slice the flow passed -- which is the point that hook
// exists to make (its own comment: a caller that passed the whole buffer
// instead of secret[:n] would put a stale tail on the plate, and no unit test
// of ppBuildPlate can see that).
func TestChainPassphraseFromAMePackedPayloadToAPasswordPlate(t *testing.T) {
	var gotSecret []byte
	var gotSeedFP, gotCombinedFP string
	var gotQR, captured bool
	passphrasePlateHook = func(secret []byte, seedFP, combinedFP string, qr bool) {
		gotSecret = append([]byte(nil), secret...)
		gotSeedFP, gotCombinedFP, gotQR, captured = seedFP, combinedFP, qr, true
	}
	t.Cleanup(func() { passphrasePlateHook = nil })

	var art string
	synctest.Test(t, func(t *testing.T) {
		w := newChainWalkFlow(t, "chain-pass", engravePassphraseFlow)
		defer w.quit()
		w.assertF1(true) // a passphrase IS secret material
		w.eng.keepWordsIn(t)

		w.ingest()

		screen := w.until("Password from where?")
		if !uiContains(screen, "FROM PAYLOAD") {
			t.Fatalf("the payload's password was not offered: %q", screen)
		}
		w.confirm()

		screen = w.until("BIP-39 Password")
		if !uiContains(screen, "Source") {
			t.Errorf("the acceptance screen must name where the record came from: %q", screen)
		}
		w.confirm()

		// The entry step, with the field PRE-FILLED. "/100" is the live
		// counter, which is what proves this is the entry screen and not the
		// acceptance one still on display.
		w.until("/100")
		kbd, ok := w.widget("kbd").(*PassphraseKeyboard)
		if !ok {
			t.Fatalf("widget \"kbd\" is %T, not a *PassphraseKeyboard", w.widget("kbd"))
		}
		if kbd.Fragment != chainPassphrase {
			t.Fatalf("the password field holds %q; the payload carried %q",
				kbd.Fragment, chainPassphrase)
		}
		w.tapWidget("ok")

		// Both fingerprint fields left BLANK, which the flow's own error copy
		// calls out as allowed ("or leave it blank to skip"). The golden is the
		// bare plate. The two titles are DIFFERENT strings and both are
		// asserted: waiting on a shared prefix would let a flow that skipped
		// one of the steps pass.
		w.until("Seed FP")
		w.tapWidget("ok")
		w.until("Expected Comb FP")
		w.tapWidget("ok")

		w.until("QR Code")
		w.chooseWidget("qr", 0) // no QR
		w.until("Confirm")
		w.tapWidget("ok")

		w.until("Hold button to start")
		w.engraveOnePlate()

		words, digest := w.eng.engraved()
		if words == 0 {
			t.Fatal("the password chain completed having cut nothing")
		}
		t.Logf("chain-pass cut %d stepper words, digest %#x", words, digest)
		if !captured {
			t.Fatal("passphrasePlateHook never fired: no plate was built on the path " +
				"this chain claims to have walked")
		}
		if string(gotSecret) != chainPassphrase {
			t.Fatalf("the plate was built from %q, not the payload's %q",
				gotSecret, chainPassphrase)
		}
		if gotSeedFP != "" || gotCombinedFP != "" || gotQR {
			t.Fatalf("this chain leaves both fingerprints blank and declines the QR; "+
				"the plate was built with seedFP=%q combinedFP=%q qr=%v",
				gotSeedFP, gotCombinedFP, gotQR)
		}
		plate, err := ppBuildPlate(engraverParams, gotSecret, gotSeedFP, gotCombinedFP, gotQR, "", false)
		if err != nil {
			t.Fatalf("rebuilding the plate from what the flow passed: %v", err)
		}
		art = chainCompareGolden(t, w, "chain-pass", plate,
			"BIP-39 Password plate, no fingerprints, no QR", "TestChainPlateGoldens")
	})
	t.Logf("chain-pass plate artifact %s", art)
}

// ─── ClassMDMK ──────────────────────────────────────────────────────────────

// chainMdMkStep is one screen the ClassMDMK walk expects, in order.
type chainMdMkStep struct {
	needle string
	downs  int // Down presses before confirming (ChoiceScreen index)
	why    string
}

// TestChainMdMkFromTheEmulatorsOwnPayloadToNinePlates is the full chain for
// md1/mk1 records, and it is the one chain whose fixture is a FILE.
//
// THE PAYLOAD IS cmd/emu/sysw_cards_payload.bin ITSELF. cmd/emu/walk_trace_a.js
// loads that blob in the browser and drives it to a completed engrave; nothing
// in `go test` knew that, and packing a second ClassMDMK container here would
// have made two CLI-built payloads that can drift apart with only one of them
// failing a CI run. chain_payloads.json therefore names the file rather than
// embedding its bytes, and TestChainMdMkFixtureIsTheEmulatorsOwnPayload binds
// the three places its digest is written down.
//
// WHY buildMultisigPolicyFlow AND NOT bundleFlow. bundleFlow's payload seam is
// `ctx.syswBundleSeeds = []string{body}` -- ONE record, which for a two-chunk
// mk1 card is an incomplete set the gatherer drops (the emulator walk sees
// exactly that: "Dropped an incomplete card"). Completing the set needs NFC,
// which this harness has none of. Build Multisig Policy is the program that
// consumes the payload's cards WHOLE, through its own over-supply picker, so it
// is the only route from these bytes to a cut without a tag reader.
//
// THE CARD ORDER IS THE EMULATOR BLOB'S, NOT THE GO ROSTER'S, and F-180 says
// why in both files: cosignerCardRoster is A@0, B@0, C@0, A@1 while
// cmd/buildpayloadcards writes A@0, A@1, B@0, C@0. Reaching Trace A's B@0 + C@0
// is SKIP, USE, USE over the roster and SKIP, SKIP, USE, USE over this blob. A
// tap sequence carried across from the other file selects A@1 instead.
func TestChainMdMkFromTheEmulatorsOwnPayloadToNinePlates(t *testing.T) {
	var art string
	var census []string
	synctest.Test(t, func(t *testing.T) {
		w := newChainWalkFlow(t, "chain-mdmk", buildMultisigPolicyFlow)
		defer w.quit()
		// The blob carries master A's mnemonic beside the nine mk1 chunks, so
		// it IS secret-bearing and the F1 screens are walked here too.
		w.assertF1(true)
		w.eng.keepWordsIn(t)

		w.ingest()

		steps := []chainMdMkStep{
			{"Choose policy type", 0, "wsh"},
			{"How many keys (n)?", 1, "n = 3"},
			{"Required signatures (k of 3)?", 1, "k = 2"},
			{"Which slot is your key?", 0, "self at @0"},
			{"Do you hold another slot?", 0, "NO, THAT IS ALL"},
			{"Include key fingerprints?", 0, "omit"},
			{"key on a card?", 0, "NO, JUST MY SEED"},
			{buildCosignerGatherTitle, 0, "Done adding cards -- there is no NFC reader here"},
			{"Payload cards", 0, "the over-supply review"},
			{"Use payload card 1 of 4?", 1, "SKIP A@0: it would duplicate the self key"},
			{"Use payload card 2 of 4?", 1, "SKIP A@1: same master, a different account"},
			// AND THERE IS NO THIRD QUESTION. buildCosignerPickFlow takes the
			// rest without asking once the cards that remain are exactly the
			// slots that remain -- "a question with one possible answer is not
			// a choice, and asking it is how an operator skips their way into
			// an under-supply that was never real". So B@0 and C@0 are seated
			// SILENTLY here, and the assertion that they are the ones seated is
			// the policy review below plus the pinned md1 at the end.
		}
		for _, s := range steps {
			w.until(s.needle)
			for i := 0; i < s.downs; i++ {
				click(&w.ctx.Router, Down)
				w.frame()
			}
			w.confirm()
		}

		// THE SEED COMES FROM THE PAYLOAD TOO. The blob carries master A's
		// mnemonic, so the build's seed seam offers it rather than opening the
		// keyboard -- which makes this the one chain where BOTH the cosigner
		// keys and the operator's own seed are payload-sourced.
		got := w.until("Seed for @0")
		if !uiContains(got, "FROM PAYLOAD") {
			t.Fatalf("the payload's seed was not offered to the build: %q", got)
		}
		w.confirm()
		// F3 again, at the seam where the SEED enters the program -- a second,
		// separate acceptance from the cosigner cards'.
		got = w.until("Source:")
		if !uiContains(got, "the systemwide payload") {
			t.Errorf("the seed acceptance screen must name the payload as the "+
				"source: %q", got)
		}
		w.confirm()

		rest := []chainMdMkStep{
			{"Add a BIP-39 passphrase?", 0, "Skip"},
			{"Key sources", 0, "the slot-source review"},
			{"Policy stub", 0, "the Policy Review"},
			{"Which md1?", 0, "the full policy md1"},
		}
		for _, s := range rest {
			got := w.until(s.needle)
			// THE TWO SILENTLY-SEATED CARDS ARE NAMED HERE, on the review the
			// operator confirms from. B@0 and C@0 are the payload's THIRD and
			// FOURTH cards; A@0 and A@1 were skipped. Measured: with
			// fingerprints omitted -- which is this walk's own choice and the
			// screen's default -- the review identifies each cosigner by
			// ORDINAL and by nothing else ("@1 a cosigner: payload card 3,
			// taken as supplied"), so the ordinal is the whole of what an
			// operator can check here. Which pair of KEYS actually landed is
			// settled at the end, by the pinned md1.
			if s.needle == "Key sources" {
				for _, want := range []string{"payload card 3", "payload card 4"} {
					if !uiContains(got, want) {
						t.Errorf("the slot-source review does not name %q, so the "+
							"cards seated without a question are not the ones this "+
							"walk skipped its way to: %q", want, got)
					}
				}
				for _, unwanted := range []string{"payload card 1", "payload card 2"} {
					if uiContains(got, unwanted) {
						t.Errorf("the review names %q, which this walk SKIPPED: %q",
							unwanted, got)
					}
				}
			}
			for i := 0; i < s.downs; i++ {
				click(&w.ctx.Router, Down)
				w.frame()
			}
			w.confirm()
		}

		// The EXPERIMENTAL warning: hold to confirm is the only route past it.
		w.until("EXPERIMENTAL")
		press(&w.ctx.Router, Button3)
		w.frame()
		time.Sleep(confirmDelay)
		w.frame()

		w.until("What to engrave?")
		w.confirm() // Full (seed + keys)

		// The plate census, asserted rather than tapped past: a 2-of-3 full
		// build is ms1(1) + mk1(2) + md1(6) = 9, and every term comes from
		// bundlePlatePlan rather than from this comment.
		got = w.until("Plate Count")
		if !uiContains(got, "This engraves 9 plates") {
			t.Fatalf("the census does not state the plate count this build cuts: %q", got)
		}
		w.confirm()

		plates := 0
		for {
			if _, ok := pumpUntil(w.frame, "Choose engraving", 96); !ok {
				break
			}
			w.confirm() // the first variant, TEXT + QR
			w.engraveOnePlate()
			plates++
			if plates > 24 {
				t.Fatal("the engrave loop did not terminate")
			}
		}
		if plates != 9 {
			t.Fatalf("%d plate(s) were engraved; a full 2-of-3 wsh build cuts "+
				"1 ms1 + 2 mk1 chunks + 6 md1 chunks = 9", plates)
		}

		words, digest := w.eng.engraved()
		if words == 0 {
			t.Fatal("the ClassMDMK chain completed having cut nothing")
		}
		t.Logf("chain-mdmk cut %d plates, %d stepper words, digest %#x", plates, words, digest)

		// THE CENSUS IS THE STRINGS THAT WERE ACTUALLY CUT, by id, through the
		// production seam cmd/emu's own gate uses (gui/engraved_hook.go). It is
		// what makes "nine plates" a statement about content rather than a
		// count.
		census = append(census, w.pl.engraved...)
		if w.pl.unknown != 0 {
			t.Errorf("%d plate(s) finished carrying an id nobody announced -- "+
				"something was cut this census cannot name", w.pl.unknown)
		}
		if len(census) != plates {
			t.Fatalf("%d plates were cut but the census names %d", plates, len(census))
		}
		for i, s := range census {
			t.Logf("plate %d: %s", i+1, s)
		}

		// (4) THE PLATE. The md1's FIRST chunk, re-planned through the same
		// validateMdmk call bundleEngrave makes (title and footer empty on the
		// Build path) and taking variant 0, TEXT + QR, which is the variant the
		// loop above chose. Compared against a golden recorded from the pinned
		// string in gui/chain_plate_goldens_test.go -- so a policy assembled
		// from different keys moves the spline and fails.
		md1 := chainMdMkFirstMd1(t, census)
		if md1 != chainMdMkMd1Chunk1 {
			t.Fatalf("the build cut a DIFFERENT wallet policy than the one pinned:\n"+
				"  engraved: %s\n  pinned:   %s\n"+
				"The payload's cosigner cards or the derivation changed. Do not "+
				"re-pin without establishing which.", md1, chainMdMkMd1Chunk1)
		}
		_, plate, err := chainMdMkPlate(w.ctx.Platform, md1)
		if err != nil {
			t.Fatalf("re-planning the md1 plate: %v", err)
		}
		art = chainCompareGolden(t, w, "chain-mdmk-md1-1", plate,
			"md1 chunk 1 of 6, TEXT + QR", "TestChainPlateGoldens")
	})
	t.Logf("chain-mdmk plate artifact %s; census of %d plates", art, len(census))
}

// chainMdMkFirstMd1 returns the first md1 string in an engraved census.
func chainMdMkFirstMd1(t *testing.T, census []string) string {
	t.Helper()
	for _, s := range census {
		if hasMDPrefix(s) {
			return s
		}
	}
	t.Fatalf("the census holds no md1 at all, so the wallet policy was never cut: %v", census)
	return ""
}

// chainMdMkPlate re-plans one md1/mk1 string exactly as bundleEngrave does on
// the Build path -- empty title and footer -- and returns variant 0, TEXT + QR.
func chainMdMkPlate(pl Platform, s string) (string, Plate, error) {
	labels, plates, err := validateMdmk(pl, s, "", "")
	if err != nil {
		return "", Plate{}, err
	}
	return labels[0], plates[0], nil
}

// ─── the ClassMDMK fixture is the emulator's, and this proves it ────────────

// TestChainMdMkFixtureIsTheEmulatorsOwnPayload closes the drift the whole
// file-backed-fixture design exists to close.
//
// The cards payload's digest is written down in THREE places, and until this
// test nothing required them to agree:
//
//	cmd/emu/sysw_cards_payload.go   const syswCardsDigest  (the device's pin)
//	cmd/emu/walk_trace_a.js         const CARDS_DIGEST     (the browser walk's)
//	gui/testdata/chain/…json        "digest"               (this chain's)
//
// cmd/emu's own host test binds the first to the blob. This binds the other
// two, so all three are one fact. It reads the SOURCE files rather than the
// symbols because sysw_cards_payload.go is //go:build js and walk_trace_a.js is
// not Go at all -- the same reason cmd/emu/sysw_cards_payload_host_test.go does.
func TestChainMdMkFixtureIsTheEmulatorsOwnPayload(t *testing.T) {
	p := chainPayloadNamed(t, "chain-mdmk")
	if p.File == "" {
		t.Fatal("chain-mdmk no longer names a file, so it is a SECOND copy of the " +
			"emulator's payload and can drift from it. Re-run " +
			"./scripts/gen-chain-fixtures.sh.")
	}
	// The file the fixture names must be the emulator's blob, not some other
	// container that happens to be readable.
	if filepath.Base(p.File) != "sysw_cards_payload.bin" ||
		!strings.Contains(filepath.ToSlash(p.File), "cmd/emu/") {
		t.Fatalf("chain-mdmk names %q; it must be cmd/emu/sysw_cards_payload.bin", p.File)
	}
	// chainBytes checks the sha256, so calling it here is the "the file has not
	// changed under us" half.
	b := chainBytes(t, p)

	squash := func(s string) string { return strings.ReplaceAll(s, " ", "") }
	for _, src := range []struct {
		path, marker string
	}{
		{filepath.Join("..", "cmd", "emu", "sysw_cards_payload.go"), `const syswCardsDigest = "`},
		{filepath.Join("..", "cmd", "emu", "walk_trace_a.js"), `export const CARDS_DIGEST = "`},
	} {
		raw, err := os.ReadFile(src.path)
		if err != nil {
			t.Fatalf("reading %s: %v", src.path, err)
		}
		i := strings.Index(string(raw), src.marker)
		if i < 0 {
			t.Fatalf("%s no longer contains %q, so this test is checking nothing",
				src.path, src.marker)
		}
		rest := string(raw)[i+len(src.marker):]
		got := rest[:strings.Index(rest, `"`)]
		if squash(got) != squash(p.Digest) {
			t.Errorf("digest drift between %s and gui/testdata/chain/chain_payloads.json:\n"+
				"  %s: %q\n  the chain fixture: %q\n"+
				"These are the same bytes; one of the two was updated without the "+
				"other, and an operator comparing by hand would be told two "+
				"different numbers.", src.path, src.path, got, p.Digest)
		}
	}

	// And the DEVICE must agree with all of them, computed from the bytes.
	pay, err := sysw.Open(b, "")
	if err != nil {
		t.Fatalf("the firmware cannot open the emulator's payload: %v", err)
	}
	if got := sysw.FormatHash(sysw.PublicDataHash(pay.Public, false)); got != p.Digest {
		t.Errorf("the blob hashes to %q; the fixture pins %q", got, p.Digest)
	}
}
