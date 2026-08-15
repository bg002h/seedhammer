package gui

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/btcsuite/btcd/chaincfg/v2"
	"seedhammer.com/bip39"
	"seedhammer.com/md"
	"seedhammer.com/mk"
)

// ─── S4: the seed<->key gate, every failing row demonstrated failing ─────────
//
// SPEC 4.3 ends "A test MUST prove each failing row actually fails,
// mutation-checked." These are those tests. They drive buildSlotGate directly,
// and the plan says why that is the right level rather than a shortcut: S2's
// interim foreign-origin refusal means a divergent-origin input cannot reach the
// gate through the flow during S4, so every S4 gate test is necessarily
// synthetic, and S5 re-proves the same fixture through the rewired flow
// (S5 test 8, TestGateStillFiresAfterOriginsDiverge).
//
// TEST MATERIAL, public by construction: masters A, B and C are BIP-39's own
// published vectors, via cosignerCardFixtures. Never put funds behind them.

// gateNet is the network every gate test derives on. One place, so a test cannot
// pass by comparing two derivations on different networks.
var gateNet = &chaincfg.MainNetParams

// gateCard decodes fixture card `idx` from the shared roster (0=A@0, 1=B@0,
// 2=C@0, 3=A@1, 4=B@1) into the mk.Card shape the gate consumes.
func gateCard(t *testing.T, idx int) mk.Card {
	t.Helper()
	sets := cosignerCardFixtures(t, idx+1)
	card, err := mk.Decode(sets[idx])
	if err != nil {
		t.Fatalf("decoding fixture card %d: %v", idx, err)
	}
	return card
}

// gateRegistry builds a one-seed registry the way the flow does, so a gate test
// and the flow share the registration path (including the master-fingerprint
// capture the fingerprint row reads).
func gateRegistry(t *testing.T, phrase, passphrase string) *seedRegistry {
	t.Helper()
	m, err := bip39.ParseMnemonic(phrase)
	if err != nil {
		t.Fatalf("ParseMnemonic: %v", err)
	}
	reg := &seedRegistry{}
	if _, err := reg.add("your seed", m, passphrase, gateNet); err != nil {
		t.Fatalf("registering the seed: %v", err)
	}
	return reg
}

// TestGateFiresOnBothSlotMismatch is SPEC 4.3 row 2: a `both` slot whose payload
// key does not derive from the payload seed FAILS LOUDLY, naming the slot, and
// nothing is engraved.
//
// The fixture is the one the emulator payload's provenance comment names as the
// failing case: master A's seed asserted over card B@0, which is master B's key.
func TestGateFiresOnBothSlotMismatch(t *testing.T) {
	reg := gateRegistry(t, fixtureMasterA, "")
	cards := []mk.Card{gateCard(t, 1)} // B@0: NOT master A's key
	sources := []slotSource{
		{Kind: slotFromBoth, SeedID: 0, Card: 0},
		{Kind: slotFromCard, Card: 0},
	}
	notices, err := buildSlotGate(sources, reg, cards, gateNet)
	if err == nil {
		t.Fatalf("the gate ACCEPTED a `both` slot whose card is a different master's "+
			"key; notices %q. This is the row the operator asked for by name: "+
			"\"if both seed and key are present, sh2 must verify key can be derived "+
			"from seed and if not, fail loudly\"", notices)
	}
	var mm errBuildSeedKeyMismatch
	if !errors.As(err, &mm) {
		t.Fatalf("gate returned %v (%T), want errBuildSeedKeyMismatch", err, err)
	}
	if mm.Slot != 0 {
		t.Errorf("the refusal names slot @%d, want @0; a mismatch that does not say "+
			"WHICH slot is a message the operator cannot act on", mm.Slot)
	}
	msg := buildSeedKeyMismatchMessage(mm, []cosignerOrigin{{slot: 0, card: 1}})
	if !strings.Contains(msg, "@0") {
		t.Errorf("the FAIL screen does not name the slot: %q", msg)
	}
	if !strings.Contains(msg, "Nothing was engraved") {
		t.Errorf("the FAIL screen does not say nothing was engraved: %q", msg)
	}
}

// TestGateAcceptsBothSlotMatch is row 1: the honest case proceeds.
//
// This is the emulator payload's own `both` fixture -- card A@0's key genuinely
// derives from the ClassMnemonic record at that card's declared origin -- so the
// Go test and the walk assert the same claim about the same material.
func TestGateAcceptsBothSlotMatch(t *testing.T) {
	reg := gateRegistry(t, fixtureMasterA, "")
	cards := []mk.Card{gateCard(t, 0)} // A@0: master A at the shared origin
	sources := []slotSource{
		{Kind: slotFromBoth, SeedID: 0, Card: 0},
		{Kind: slotFromCard, Card: 0},
	}
	notices, err := buildSlotGate(sources, reg, cards, gateNet)
	if err != nil {
		t.Fatalf("the gate REFUSED an honest `both` slot: %v. A gate that cannot be "+
			"satisfied is not a gate, it is a wall", err)
	}
	if len(notices) != 0 {
		t.Errorf("an ordinary single-slot build emitted notices %q; a notice on a "+
			"shape with nothing to notice teaches the operator to page past them",
			notices)
	}
}

// TestGateIgnoresUnassignedCosigners is row 6, and it is the false positive that
// would make the feature unusable.
//
// SPEC 4.3: "It is NOT inferred across unrelated material: a payload carrying the
// operator's seed alongside other people's cosigner cards is normal and MUST NOT
// fail." Round 0's rule required deriving against UNASSIGNED material, which R-2
// forbids, so the two rules could not both be implemented.
//
// The shape here is exactly the delivered payload: the operator's own seed is in
// the registry and in use at their slot, and two cards belonging to other people
// sit in the policy. Every one of them would fail a derive-and-compare against
// that seed, and none of them may be tried.
func TestGateIgnoresUnassignedCosigners(t *testing.T) {
	reg := gateRegistry(t, fixtureMasterA, "")
	cards := []mk.Card{gateCard(t, 1), gateCard(t, 2)} // B@0 and C@0: other people
	sources := []slotSource{
		{Kind: slotFromSeed, SeedID: 0, Account: 0},
		{Kind: slotFromCard, Card: 0},
		{Kind: slotFromCard, Card: 1},
	}
	notices, err := buildSlotGate(sources, reg, cards, gateNet)
	if err != nil {
		t.Fatalf("the gate refused the NORMAL shape -- the operator's seed plus two "+
			"unrelated cosigner cards -- with %v. This is the false positive that "+
			"makes the feature unusable: every ordinary 2-of-3 looks like this", err)
	}
	if len(notices) != 0 {
		t.Errorf("notices %q on a build where one seed fills one slot", notices)
	}

	// And the same set with the operator's slot as `both` against their OWN card
	// still ignores the other two: the gate must check what is claimed and
	// nothing else.
	cards = []mk.Card{gateCard(t, 0), gateCard(t, 1), gateCard(t, 2)}
	sources = []slotSource{
		{Kind: slotFromBoth, SeedID: 0, Card: 0},
		{Kind: slotFromCard, Card: 1},
		{Kind: slotFromCard, Card: 2},
	}
	if _, err := buildSlotGate(sources, reg, cards, gateNet); err != nil {
		t.Fatalf("a checked own-slot alongside two unrelated cards was refused: %v", err)
	}
}

// TestGateRefusesDuplicateKeyAcrossFinalSlots is SPEC 4.1's discriminator,
// exercised through S4's NEW arrival shape.
//
// The check itself is S2's and lives in assembleBuildPolicy, over the FINAL SLOT
// SET; the plan says S4 "does not replace this check -- it feeds it more
// sources". The new source is a `both` self slot: every slot now arrives from a
// card, so card-vs-card is reachable where before the only hazard was
// self-vs-card. sortedmulti(2,K,K,X) is spendable by K alone.
func TestGateRefusesDuplicateKeyAcrossFinalSlots(t *testing.T) {
	a0 := gateCard(t, 0)
	p := buildPolicyParams{Script: md.MultisigWsh, N: 2, K: 2, SelfSlot: 0, SelfFromCard: true}
	_, _, _, err := assembleBuildPolicy(p, "", 0, []mk.Card{a0, a0})
	if err == nil {
		t.Fatal("a policy whose two slots hold the SAME 65-byte chain code + pubkey " +
			"assembled. sortedmulti(2,K,K) is spendable by K alone, so the displayed " +
			"2-of-2 is a 1-of-1 on steel")
	}
	var dup errBuildDuplicateKey
	if !errors.As(err, &dup) {
		t.Fatalf("assembleBuildPolicy returned %v (%T), want errBuildDuplicateKey", err, err)
	}
	if dup.SlotA != 0 || dup.SlotB != 1 {
		t.Errorf("the refusal names @%d and @%d, want @0 and @1", dup.SlotA, dup.SlotB)
	}
	// And the message must not claim the operator's slot came from their seed
	// when it came from a card: that sends them to change the wrong input.
	msg := buildDuplicateKeyMessage(dup, p.SelfSlot, true,
		[]cosignerOrigin{{slot: 0, card: 1}, {slot: 1, card: 2}})
	if strings.Contains(msg, "from your seed") {
		t.Errorf("the duplicate refusal blames the seed for a slot filled from a card: %q", msg)
	}
	if !strings.Contains(msg, "payload card 1") || !strings.Contains(msg, "payload card 2") {
		t.Errorf("the duplicate refusal does not name both cards: %q", msg)
	}
}

// TestGateAcceptsSameSeedAtDistinctOrigins is row 5, and it is the test round 0's
// C1 would have failed.
//
// One seed at two slots under DISTINCT origins is the legitimate multi-account
// wallet SPEC 4.1 exists to make buildable. It proceeds, with the notice
// findUserSlot already specifies for the same shape.
func TestGateAcceptsSameSeedAtDistinctOrigins(t *testing.T) {
	reg := gateRegistry(t, fixtureMasterA, "")
	cards := []mk.Card{gateCard(t, 0)} // A@0, at m/48'/0'/0'/2'
	sources := []slotSource{
		{Kind: slotFromBoth, SeedID: 0, Card: 0},    // origin m/48h/0h/0h/2h
		{Kind: slotFromSeed, SeedID: 0, Account: 1}, // origin m/48h/0h/1h/2h
		{Kind: slotFromCard, Card: 0},               // somebody else
	}
	notices, err := buildSlotGate(sources, reg, cards, gateNet)
	if err != nil {
		t.Fatalf("the gate REFUSED one seed at two DISTINCT origins: %v. That is the "+
			"multi-account wallet SPEC 4.1 exists to build, and refusing it is the "+
			"Critical round 0 shipped", err)
	}
	if len(notices) != 1 {
		t.Fatalf("got %d notice(s) %q, want exactly 1: proceeding SILENTLY on a "+
			"shape the spec calls out is half the requirement", len(notices), notices)
	}
	for _, want := range []string{"@0", "@1", "different key origins"} {
		if !strings.Contains(notices[0], want) {
			t.Errorf("the multi-account notice omits %q: %q", want, notices[0])
		}
	}

	// The SAME seed at the SAME origin twice is not this shape: it is one key
	// twice, and it belongs to the duplicate-key refusal over the final slot set.
	// The gate must not emit a "that is allowed" notice over it.
	same := []slotSource{
		{Kind: slotFromSeed, SeedID: 0, Account: 0},
		{Kind: slotFromSeed, SeedID: 0, Account: 0},
	}
	notices, err = buildSlotGate(same, reg, cards, gateNet)
	if err != nil {
		t.Fatalf("gate over two same-origin derived slots: %v", err)
	}
	if len(notices) != 0 {
		t.Errorf("the gate blessed one key at two slots as a multi-account wallet: %q", notices)
	}
}

// TestGateRefusesContradictingFingerprint is row 3: a fingerprint IS present and
// contradicts the derivation.
//
// It is a separate row from the mismatch, not a stronger form of it: mk1's
// Fingerprint is card-self-declared and unbound to the key, so this fixture
// carries the RIGHT key under the WRONG fingerprint and passes the derivation
// row. Most often that means the card was made from this seed under a different
// passphrase, which is a real and silent wallet split.
func TestGateRefusesContradictingFingerprint(t *testing.T) {
	reg := gateRegistry(t, fixtureMasterA, "")
	card := gateCard(t, 0)
	honest := card.Fingerprint
	if honest == "" {
		t.Fatal("the fixture card carries no fingerprint, so this row has nothing to contradict")
	}
	card.Fingerprint = "deadbeef"
	cards := []mk.Card{card}
	sources := []slotSource{{Kind: slotFromBoth, SeedID: 0, Card: 0}}

	_, err := buildSlotGate(sources, reg, cards, gateNet)
	if err == nil {
		t.Fatal("a card whose key matches the seed but whose fingerprint says another " +
			"master was ACCEPTED. The fingerprint is written by whoever made the card " +
			"and is not bound to the key, so a contradiction is real information")
	}
	var fc errBuildFingerprintContradicts
	if !errors.As(err, &fc) {
		t.Fatalf("gate returned %v (%T), want errBuildFingerprintContradicts", err, err)
	}
	if fc.Slot != 0 || fc.Declared != "deadbeef" {
		t.Errorf("the refusal reads slot @%d / declared %q, want @0 / deadbeef", fc.Slot, fc.Declared)
	}
	if fc.Derived == fc.Declared {
		t.Error("the refusal quotes the same fingerprint twice, so it cannot be read")
	}

	// NON-VACUITY: the honest fingerprint on the same card must NOT fire. Without
	// this the test above would pass against a gate that refuses every card
	// carrying any fingerprint at all.
	card.Fingerprint = honest
	if _, err := buildSlotGate(sources, reg, []mk.Card{card}, gateNet); err != nil {
		t.Fatalf("the card's OWN fingerprint was refused as a contradiction: %v", err)
	}
	// And a card carrying no fingerprint has nothing to contradict.
	card.Fingerprint = ""
	if _, err := buildSlotGate(sources, reg, []mk.Card{card}, gateNet); err != nil {
		t.Fatalf("a card with no fingerprint was refused: %v", err)
	}
}

// TestGateNeverPrintsSeedOrPassphrase: no failure message contains seed words or
// passphrase text -- not the error string, and not the pixels.
//
// Both surfaces, because they are produced by different code and only one of
// them is what the operator standing at the machine sees. The screen is
// rasterised through the real modal, not read off the string that was passed in.
func TestGateNeverPrintsSeedOrPassphrase(t *testing.T) {
	const passphrase = "correcthorsebatterystaple"
	reg := gateRegistry(t, fixtureMasterA, passphrase)
	words := strings.Fields(fixtureMasterA)

	// Every failing row, with its operator-facing message.
	var msgs []string
	var errs []error

	mismatch := []slotSource{{Kind: slotFromBoth, SeedID: 0, Card: 0}}
	_, err := buildSlotGate(mismatch, reg, []mk.Card{gateCard(t, 1)}, gateNet)
	if err == nil {
		t.Fatal("the mismatch row did not fire, so this test has no message to inspect")
	}
	errs = append(errs, err)
	var mm errBuildSeedKeyMismatch
	if errors.As(err, &mm) {
		msgs = append(msgs, buildSeedKeyMismatchMessage(mm, []cosignerOrigin{{slot: 0, card: 1}}))
	}

	// The fingerprint row, with the passphrase live. A passphrase changes the KEY
	// as well as the fingerprint, so the fixture card cannot be reused as-is: this
	// card carries the key THIS (seed, passphrase) pair really derives, under a
	// fingerprint that names another master.
	seed, _ := reg.at(0)
	xpub, _, derr := deriveAccountXpub(seed.Mnemonic, seed.Passphrase, gateNet, multisigSharedOrigin())
	if derr != nil {
		t.Fatalf("deriving the passphrase-bound account key: %v", derr)
	}
	fpCard := gateCard(t, 0)
	fpCard.Xpub = xpub
	fpCard.Fingerprint = "deadbeef"
	_, err = buildSlotGate(mismatch, reg, []mk.Card{fpCard}, gateNet)
	if err == nil {
		t.Fatal("a card carrying this pair's own key under another master's " +
			"fingerprint was accepted; the fingerprint row has nothing to inspect")
	}
	errs = append(errs, err)
	var fc errBuildFingerprintContradicts
	if errors.As(err, &fc) {
		msgs = append(msgs, buildFingerprintContradictsMessage(fc, []cosignerOrigin{{slot: 0, card: 1}}))
	}
	if len(msgs) != 2 {
		t.Fatalf("got %d operator-facing message(s), want 2", len(msgs))
	}

	leaks := func(where, text string) {
		t.Helper()
		low := strings.ToLower(text)
		if strings.Contains(low, strings.ToLower(passphrase)) {
			t.Errorf("%s carries the PASSPHRASE: %q", where, text)
		}
		// "abandon" is 11 of master A's 12 words; one hit is a leak.
		for _, w := range words {
			if strings.Contains(low, strings.ToLower(w)) {
				t.Errorf("%s carries the seed word %q: %q", where, w, text)
			}
		}
	}
	for i, e := range errs {
		leaks(fmt.Sprintf("error %d", i), e.Error())
	}
	for i, m := range msgs {
		leaks(fmt.Sprintf("message %d", i), m)
	}

	// THE PIXELS, not the string. A message is only safe if what is drawn is
	// safe, and the modal is what draws it.
	for i, m := range msgs {
		ctx := NewContext(newPlatform())
		frame, quit := runUI(ctx, func() { showError(ctx, &descriptorTheme, "Key check", m) })
		content, ok := pumpUntil(frame, "@0", 16)
		quit()
		if !ok {
			t.Fatalf("message %d never reached a frame naming the slot; got %q", i, content)
		}
		leaks(fmt.Sprintf("screen %d", i), content)
	}
}

// TestGateDerivesAtTheCardsOwnOrigin is the ORIGIN BINDING, as a PROCEED/FAIL
// pair on one fixture: a `both` slot whose card declares m/48'/0'/1'/2'.
//
// WHY IT EXISTS. findUserSlot is origin-correct by construction -- it derives at
// each key's own OriginPath -- but the wrapper built on top of it is new code and
// is not. A wrapper that hardcoded multisigSharedOrigin() passes every other test
// in this file, because S2's interim foreign-origin refusal makes the two values
// indistinguishable for the whole of S4. The mutation is the point: make
// bothSlotKey use multisigSharedOrigin() instead of card.Path, and the PROCEED
// case below must go red.
func TestGateDerivesAtTheCardsOwnOrigin(t *testing.T) {
	reg := gateRegistry(t, fixtureMasterA, "")
	a1 := gateCard(t, 3) // master A at m/48'/0'/1'/2'
	a0 := gateCard(t, 0) // master A at m/48'/0'/0'/2'
	if a1.Path == a0.Path {
		t.Fatalf("both fixture cards declare %s, so this test cannot tell the two "+
			"origins apart", a1.Path)
	}
	sources := []slotSource{{Kind: slotFromBoth, SeedID: 0, Card: 0}}

	t.Run("PROCEED when the key is genuinely derived there", func(t *testing.T) {
		if _, err := buildSlotGate(sources, reg, []mk.Card{a1}, gateNet); err != nil {
			t.Fatalf("a card declaring %s, carrying the key that seed derives AT %s, "+
				"was refused: %v. A gate deriving at the shared origin instead would "+
				"fail exactly here", a1.Path, a1.Path, err)
		}
	})

	t.Run("FAIL LOUDLY when the key was derived at the shared origin instead", func(t *testing.T) {
		// The SAME card, declaring m/48'/0'/1'/2', carrying the key from
		// m/48'/0'/0'/2'. The fingerprint is untouched and honest, so only the
		// origin binding can separate the two arms.
		liar := a1
		liar.Xpub = a0.Xpub
		_, err := buildSlotGate(sources, reg, []mk.Card{liar}, gateNet)
		if err == nil {
			t.Fatalf("a card declaring %s while carrying the key from %s was ACCEPTED. "+
				"Its mk1 would assert membership in a wallet at a path its own key is "+
				"not at", liar.Path, a0.Path)
		}
		var mm errBuildSeedKeyMismatch
		if !errors.As(err, &mm) {
			t.Fatalf("gate returned %v (%T), want errBuildSeedKeyMismatch", err, err)
		}
		if mm.Slot != 0 {
			t.Errorf("the refusal names slot @%d, want @0", mm.Slot)
		}
		if mm.Declared != liar.Path {
			t.Errorf("the refusal quotes origin %q, want the CARD's own %q: a gate "+
				"that derived at the wrong origin must not be able to launder that "+
				"into its own error message", mm.Declared, liar.Path)
		}
	})
}

// TestDerivedSlotAccountIsTheBip48AccountComponent pins SPEC M-B's second
// binding, which had no test: in a `derived(seedID, account)` slot, `account`
// occupies the BIP-48 ACCOUNT component of m/48'/0'/account'/2'. 4.1a item 1's
// example implies it and nothing ruled it until the spec did.
func TestDerivedSlotAccountIsTheBip48AccountComponent(t *testing.T) {
	for account, want := range map[uint32]string{
		0: "m/48h/0h/0h/2h",
		1: "m/48h/0h/1h/2h",
		7: "m/48h/0h/7h/2h",
	} {
		if got := derivedSlotOrigin(account).String(); got != want {
			t.Errorf("derivedSlotOrigin(%d) = %s, want %s", account, got, want)
		}
	}
	if derivedSlotOrigin(0).String() != multisigSharedOrigin().String() {
		t.Errorf("account 0 must BE the shared origin (%s), got %s",
			multisigSharedOrigin(), derivedSlotOrigin(0))
	}
}

// TestGateRefusalsAreDRAWN, not merely composed.
//
// FOUND BY THE WALK, not by a test, and that is the point of it existing. The
// gate's FAIL screen must name the likely causes, say that reassigning the slot
// SUPPRESSES the check rather than fixing it, and name the host route. Every one
// of those was in the string. The emulator's first frame ended
//
//	"...rewrite the payload on the host with `"
//
// because ErrorScreen's body SCROLLS (gui.go, Warning.Layout binds Up/Down) and
// the text ran past the fold. There is no scroll affordance on that screen, so a
// route below the fold is a route the operator is not told about -- which is
// plan 0.1 clause 3's rule for the confirmation surface, one screen earlier.
//
// So the assertion is on the FIRST frame, deliberately: pumpUntil would advance
// past it, and scrolling would prove only that the text exists somewhere. A
// refusal whose remedy needs a button nobody suggested is the shape of "a safety
// gate whose obvious next action disables it", arrived at from the other side.
func TestGateRefusalsAreDrawnWithoutScrolling(t *testing.T) {
	origins := []cosignerOrigin{{slot: 0, card: 3}}
	for _, tc := range []struct {
		name string
		msg  string
		want []string
	}{
		{
			"seed-key mismatch",
			buildSeedKeyMismatchMessage(errBuildSeedKeyMismatch{
				Slot: 0, Declared: multisigSharedOrigin().String()}, origins),
			[]string{"slot @0", "Nothing was engraved", "SUPPRESSES", "me sysw pack"},
		},
		{
			"fingerprint contradiction",
			buildFingerprintContradictsMessage(errBuildFingerprintContradicts{
				Slot: 0, Declared: "deadbeef", Derived: "73c5da0a"}, origins),
			[]string{"@0", "Nothing was engraved", "me sysw pack"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := newPlatform()
			p.display = sh2DisplaySize
			ctx := NewContext(p)
			frame, _, ink, quit := runUITouchRaster(ctx, func() {
				showError(ctx, &descriptorTheme, "Key check", tc.msg)
			})
			defer quit()
			first, ok := frame()
			if !ok {
				t.Fatal("the refusal never rendered a frame")
			}
			t.Logf("%s: %d chars, ink %d px", tc.name, len(tc.msg), ink())
			if ink() < buildWalkRasterFloor {
				t.Errorf("the refusal drew only %d ink pixels (floor %d)", ink(), buildWalkRasterFloor)
			}
			for _, w := range tc.want {
				if !uiContains(first, w) {
					t.Errorf("the FIRST frame does not carry %q, so the operator has to "+
						"scroll a screen that offers no scroll affordance to find it.\n"+
						"drawn: %q", w, first)
				}
			}
		})
	}
}
